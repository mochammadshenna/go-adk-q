package main

// acp_server.go — Agent Client Protocol (ACP) server for go-adk-q.
//
// ACP (https://agentclientprotocol.com) is an industry-standard JSON-RPC 2.0
// protocol that allows IDEs, third-party tools, and automation systems to
// communicate with this agent over HTTP or stdio.
//
// Protocol overview:
//   • Transport: HTTP/SSE (Server-Sent Events for streaming) or stdio
//   • Framing:   JSON-RPC 2.0 — every message is {"jsonrpc":"2.0","id":…,"method":…,"params":…}
//   • Methods:   initialize, session/create, message/send, message/stream, ping
//
// Usage:
//   /acp        — start server on default port (6175) or $ACP_PORT
//   /acpstop    — stop the running server
//
// From an IDE or tool:
//   curl -X POST http://localhost:6175/acp \
//        -H 'Content-Type: application/json' \
//        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"curl","version":"1.0"}}}'

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ── JSON-RPC 2.0 types ─────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── ACP capability types ───────────────────────────────────────────────────

type initializeParams struct {
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
	Capabilities *clientCapabilities `json:"capabilities,omitempty"`
}

type clientCapabilities struct {
	Streaming bool `json:"streaming,omitempty"`
}

type initializeResult struct {
	ServerInfo   serverInfo       `json:"serverInfo"`
	Capabilities serverCapabilities `json:"capabilities"`
	ProtocolVersion string         `json:"protocolVersion"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type serverCapabilities struct {
	Streaming bool `json:"streaming"`
	Sessions  bool `json:"sessions"`
	FileContext bool `json:"fileContext"`
}

type messageSendParams struct {
	SessionID string `json:"sessionId,omitempty"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

type messageSendResult struct {
	SessionID string `json:"sessionId"`
	MessageID string `json:"messageId"`
	Status    string `json:"status"` // "accepted" | "completed"
}

type sessionCreateResult struct {
	SessionID   string `json:"sessionId"`
	CreatedAt   string `json:"createdAt"`
}

// ── acpServer ──────────────────────────────────────────────────────────────

// acpServer runs the HTTP ACP server and bridges incoming requests to the
// agent runner through the tuiBridge callback.
type acpServer struct {
	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
	port     int
	// tuiBridge is called when a message/send arrives. It returns the agent
	// response or an error. This bridges ACP requests to the TUI's runner.
	tuiBridge func(ctx context.Context, input string) (string, error)
	sessions  map[string]*acpSession
}

type acpSession struct {
	ID        string
	CreatedAt time.Time
}

// acpServerInstance is the singleton ACP server managed by /acp and /acpstop.
var acpServerInstance *acpServer
var acpServerMu sync.Mutex

// newACPServer creates a new ACP server using the given bridge function.
func newACPServer(bridge func(ctx context.Context, input string) (string, error)) *acpServer {
	return &acpServer{
		tuiBridge: bridge,
		sessions:  make(map[string]*acpSession),
	}
}

// Start begins listening on the given port (or $ACP_PORT, defaulting to 6175).
func (s *acpServer) Start(port int) error {
	if port == 0 {
		if v := os.Getenv("ACP_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				port = p
			}
		}
		if port == 0 {
			port = 6175
		}
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("ACP listen: %w", err)
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/acp", s.handleRPC)
	mux.HandleFunc("/health", s.handleHealth)

	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() { _ = s.httpSrv.Serve(ln) }()
	return nil
}

// Stop shuts the server down gracefully.
func (s *acpServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// Port returns the actual bound port.
func (s *acpServer) Port() int {
	return s.port
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

func (s *acpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"server":  "go-adk-q ACP",
		"version": "1.0.0",
	})
}

func (s *acpServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, nil, -32700, "Parse error: "+err.Error())
		return
	}
	if req.JSONRPC != "2.0" {
		s.writeError(w, req.ID, -32600, "Invalid Request: jsonrpc must be \"2.0\"")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch req.Method {
	case "initialize":
		s.handleInitialize(w, req)
	case "ping":
		s.writeResult(w, req.ID, map[string]string{"pong": "pong"})
	case "session/create":
		s.handleSessionCreate(w, req)
	case "message/send":
		s.handleMessageSend(w, req, r.Context())
	case "message/stream":
		s.handleMessageStream(w, req, r)
	default:
		s.writeError(w, req.ID, -32601, "Method not found: "+req.Method)
	}
}

func (s *acpServer) handleInitialize(w http.ResponseWriter, req rpcRequest) {
	s.writeResult(w, req.ID, initializeResult{
		ProtocolVersion: "1.0",
		ServerInfo: serverInfo{
			Name:    "go-adk-q",
			Version: "1.0.0",
		},
		Capabilities: serverCapabilities{
			Streaming:   true,
			Sessions:    true,
			FileContext: true,
		},
	})
}

func (s *acpServer) handleSessionCreate(w http.ResponseWriter, req rpcRequest) {
	id := fmt.Sprintf("acp-session-%d", time.Now().UnixNano())
	sess := &acpSession{ID: id, CreatedAt: time.Now()}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	s.writeResult(w, req.ID, sessionCreateResult{
		SessionID: id,
		CreatedAt: sess.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *acpServer) handleMessageSend(w http.ResponseWriter, req rpcRequest, ctx context.Context) {
	var params messageSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.ID, -32602, "Invalid params: "+err.Error())
		return
	}
	if params.Message.Content == "" {
		s.writeError(w, req.ID, -32602, "Invalid params: message.content is required")
		return
	}

	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// If no bridge, return accepted with a hint.
	if s.tuiBridge == nil {
		s.writeResult(w, req.ID, messageSendResult{
			SessionID: params.SessionID,
			MessageID: msgID,
			Status:    "accepted",
		})
		return
	}

	// Run the agent synchronously.
	reply, err := s.tuiBridge(ctx, params.Message.Content)
	if err != nil {
		s.writeError(w, req.ID, -32000, "Agent error: "+err.Error())
		return
	}

	s.writeResult(w, req.ID, map[string]interface{}{
		"sessionId": params.SessionID,
		"messageId": msgID,
		"status":    "completed",
		"response": map[string]string{
			"role":    "agent",
			"content": reply,
		},
	})
}

func (s *acpServer) handleMessageStream(w http.ResponseWriter, req rpcRequest, r *http.Request) {
	var params messageSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.ID, -32602, "Invalid params: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	sendEvent := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// Emit start event.
	startPayload, _ := json.Marshal(map[string]string{
		"messageId": msgID,
		"status":    "started",
	})
	sendEvent("message.started", string(startPayload))

	if s.tuiBridge == nil {
		donePayload, _ := json.Marshal(map[string]interface{}{
			"messageId": msgID,
			"status":    "completed",
			"response":  map[string]string{"role": "agent", "content": "(no agent bridge configured)"},
		})
		sendEvent("message.completed", string(donePayload))
		return
	}

	reply, err := s.tuiBridge(r.Context(), params.Message.Content)
	if err != nil {
		errPayload, _ := json.Marshal(map[string]string{
			"messageId": msgID,
			"error":     err.Error(),
		})
		sendEvent("message.error", string(errPayload))
		return
	}

	donePayload, _ := json.Marshal(map[string]interface{}{
		"messageId": msgID,
		"status":    "completed",
		"response":  map[string]string{"role": "agent", "content": reply},
	})
	sendEvent("message.completed", string(donePayload))
}

// ── Helpers ────────────────────────────────────────────────────────────────

func (s *acpServer) writeResult(w http.ResponseWriter, id *json.RawMessage, result interface{}) {
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *acpServer) writeError(w http.ResponseWriter, id *json.RawMessage, code int, msg string) {
	w.WriteHeader(http.StatusOK) // JSON-RPC errors use 200 OK
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
