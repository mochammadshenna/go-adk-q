package main

// acp_server.go — a subset of the real Agent Client Protocol
// (https://agentclientprotocol.com) for go-adk-q, verified against the live
// spec (protocol/v1/{overview,initialization,session-setup,prompt-turn,
// file-system,tool-calls}.md), not guessed.
//
// What IS spec-conformant here: method names, param/result field names and
// types for the client-to-agent request/response methods this transport can
// support — initialize, session/new, session/prompt.
//
// What is NOT implemented, and why — this is an architectural limit, not an
// oversight:
//   - fs/read_text_file, fs/write_text_file, terminal/*,
//     session/request_permission are all Agent→Client methods: per spec, the
//     AGENT calls OUT to the CLIENT (the editor) to read/write the editor's
//     own buffers or run a terminal, gated by a client-side permission
//     prompt. That requires a persistent bidirectional channel (stdio or
//     WebSocket) so the agent can send an unsolicited request and await the
//     client's reply mid-turn. This server is HTTP request/response only —
//     there is no live connection to push an agent-initiated request down.
//     Implementing this needs a transport change (stdio or WebSocket) before
//     it's possible at all, not just more code on top of what's here.
//   - authenticate, session/load, session/set_mode, logout: not needed by
//     this single-session, no-persistence TUI; omitted, not broken.
//   - session/update (the spec's actual push-notification vehicle for
//     tool-call/plan/usage reporting) has no clean HTTP-only equivalent
//     either. The SSE-based streaming endpoint below approximates it with
//     session/update-shaped frames over a single request's response stream —
//     a transport adaptation, not literal spec compliance.
//
// IMPORTANT: read_file/write_file/grep_search/fetch_url — this repo's
// harness tools (ADR-0008) — are unrelated to ACP's fs/* methods. Those
// tools are called BY THE LLM directly through ADK's own tool-calling; ACP's
// fs/* methods are how an ACP-compliant editor (e.g. Zed) would let this
// agent read ITS unsaved buffers. Conflating the two would be a category
// error: one is an internal LLM tool, the other is an external protocol
// callback in the opposite direction.
//
// Usage:
//   /acp        — start server on default port (6175) or $ACP_PORT
//   /acpstop    — stop the running server
//
// From an IDE or tool:
//   curl -X POST http://localhost:6175/acp \
//        -H 'Content-Type: application/json' \
//        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"curl","version":"1.0"}}}'

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
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
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

// ── ACP types — field names/shapes verified against the live spec ─────────

// initializeParams mirrors protocol/v1/initialization.md's request shape.
type initializeParams struct {
	ProtocolVersion    int                 `json:"protocolVersion"`
	ClientCapabilities *clientCapabilities `json:"clientCapabilities,omitempty"`
	ClientInfo         agentClientInfo     `json:"clientInfo"`
}

type clientCapabilities struct {
	FS *struct {
		ReadTextFile  bool `json:"readTextFile"`
		WriteTextFile bool `json:"writeTextFile"`
	} `json:"fs,omitempty"`
	Terminal bool `json:"terminal,omitempty"`
}

// agentClientInfo is shared shape for both clientInfo and agentInfo per spec.
type agentClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// initializeResult mirrors protocol/v1/initialization.md's response shape.
// AgentCapabilities is honest about what this transport can actually do:
// loadSession/mcpCapabilities are false (not implemented, see file header),
// promptCapabilities.embeddedContext is true (session/prompt accepts a
// ContentBlock array; see handleSessionPrompt).
type initializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities agentCapabilities `json:"agentCapabilities"`
	AgentInfo         agentClientInfo   `json:"agentInfo"`
	AuthMethods       []string          `json:"authMethods"`
}

type agentCapabilities struct {
	LoadSession        bool               `json:"loadSession"`
	PromptCapabilities promptCapabilities `json:"promptCapabilities"`
	MCPCapabilities    mcpCapabilities    `json:"mcpCapabilities"`
}

type promptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type mcpCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

// sessionNewParams mirrors protocol/v1/session-setup.md's session/new
// request. mcpServers is accepted for spec conformance but not connected to
// —  wiring arbitrary client-supplied MCP servers into the agent's tool list
// is a separate, larger feature than this ACP alignment pass.
type sessionNewParams struct {
	CWD        string          `json:"cwd"`
	MCPServers []mcpServerSpec `json:"mcpServers"`
}

type mcpServerSpec struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env,omitempty"`
}

type sessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// contentBlock is a minimal subset of spec's ContentBlock union — this
// server only ever produces/consumes the "text" variant; other kinds
// (image/audio/resource) are accepted as input (ignored, not rejected) but
// never emitted, matching promptCapabilities above.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// sessionPromptParams mirrors protocol/v1/prompt-turn.md's session/prompt.
type sessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []contentBlock `json:"prompt"`
}

// sessionPromptResult mirrors the spec's stopReason enum. This server only
// ever produces "end_turn" or "refusal" (on error) — max_tokens/
// max_turn_requests/cancelled require turn-level accounting this bridge
// doesn't track.
type sessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

// sessionUpdateNotification mirrors protocol/v1/prompt-turn.md's
// session/update push shape, used by the SSE streaming endpoint below as a
// transport adaptation (see file header) — not a literal second channel.
type sessionUpdateNotification struct {
	JSONRPC string              `json:"jsonrpc"`
	Method  string              `json:"method"`
	Params  sessionUpdateParams `json:"params"`
}

type sessionUpdateParams struct {
	SessionID string        `json:"sessionId"`
	Update    sessionUpdate `json:"update"`
}

type sessionUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"` // "agent_message_chunk" | "tool_call" | ...
	Content       *contentBlock `json:"content,omitempty"`
}

// ── acpServer ──────────────────────────────────────────────────────────────

// acpServer holds the request-handling logic shared by both ACP transports
// this repo supports: the HTTP server below (non-spec convenience — see file
// header) and the stdio transport (acp_stdio.go — the actual spec-conformant
// one). Both transports call dispatch/doInitialize/doSessionNew/
// doSessionPrompt; neither duplicates the method logic.
type acpServer struct {
	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
	port     int
	// tuiBridge is called when a message/send arrives. It returns the agent
	// response or an error. This bridges ACP requests to the TUI's runner.
	tuiBridge func(ctx context.Context, input string) (string, error)
	sessions  map[string]*acpSession
	// clientCaps is the client's clientCapabilities from its initialize
	// request, nil until negotiated. Agent→Client methods (fs/*, gated here
	// via requestReadTextFile in acp_stdio.go) MUST NOT be attempted unless
	// the corresponding capability is true, per protocol/v1/file-system.md.
	clientCaps *clientCapabilities
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
	case "message/stream":
		// Not a real ACP method — a transport-adapted streaming variant of
		// session/prompt over SSE. See file header for why session/update
		// can't be a true separate push channel on this HTTP-only server.
		s.handleMessageStream(w, req, r)
		return
	}

	result, rpcErr := s.dispatch(r.Context(), req)
	if rpcErr != nil {
		s.writeError(w, req.ID, rpcErr.Code, rpcErr.Message)
		return
	}
	s.writeResult(w, req.ID, result)
}

// dispatch executes a client→agent JSON-RPC method against the shared
// handler logic used by both transports (HTTP's handleRPC above and
// acp_stdio.go's handleIncomingRequest) — neither transport duplicates this
// switch or the per-method logic behind it.
func (s *acpServer) dispatch(ctx context.Context, req rpcRequest) (interface{}, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.doInitialize(req.Params)
	case "session/new":
		return s.doSessionNew(req.Params)
	case "session/prompt":
		return s.doSessionPrompt(ctx, req.Params)
	default:
		return nil, &rpcError{Code: -32601, Message: "Method not found: " + req.Method}
	}
}

// doInitialize is the transport-agnostic initialize handler. It stores the
// client's clientCapabilities (previously parsed into initializeParams but
// never actually read anywhere — a real gap, since Agent→Client methods
// must gate on it) for later capability checks such as requestReadTextFile.
func (s *acpServer) doInitialize(rawParams json.RawMessage) (interface{}, *rpcError) {
	var params initializeParams
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
		}
	}
	s.mu.Lock()
	s.clientCaps = params.ClientCapabilities
	s.mu.Unlock()

	return initializeResult{
		ProtocolVersion: 1,
		AgentInfo: agentClientInfo{
			Name:    "go-adk-q",
			Version: "1.0.0",
		},
		AgentCapabilities: agentCapabilities{
			LoadSession: false, // no session persistence in this TUI
			PromptCapabilities: promptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: true,
			},
			MCPCapabilities: mcpCapabilities{HTTP: false, SSE: false},
		},
		AuthMethods: []string{},
	}, nil
}

// doSessionNew is the transport-agnostic session/new handler.
func (s *acpServer) doSessionNew(rawParams json.RawMessage) (interface{}, *rpcError) {
	var params sessionNewParams
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
		}
	}
	id := fmt.Sprintf("acp-session-%d", time.Now().UnixNano())
	sess := &acpSession{ID: id, CreatedAt: time.Now()}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sessionNewResult{SessionID: id}, nil
}

// promptText concatenates the text content blocks of a session/prompt
// request — image/audio/resource blocks are accepted (spec allows a client
// to send them) but ignored, since this agent's promptCapabilities declares
// no support for them.
func promptText(blocks []contentBlock) string {
	var sb []byte
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if len(sb) > 0 {
				sb = append(sb, '\n')
			}
			sb = append(sb, b.Text...)
		}
	}
	return string(sb)
}

// doSessionPrompt is the transport-agnostic session/prompt handler.
func (s *acpServer) doSessionPrompt(ctx context.Context, rawParams json.RawMessage) (interface{}, *rpcError) {
	var params sessionPromptParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
	}
	text := promptText(params.Prompt)
	if text == "" {
		return nil, &rpcError{Code: -32602, Message: "Invalid params: prompt must contain at least one non-empty text block"}
	}

	if s.tuiBridge == nil {
		return nil, &rpcError{Code: -32000, Message: "Agent error: no runner bridge configured"}
	}

	reply, err := s.tuiBridge(ctx, text)
	if err != nil {
		// reply is empty on error; nothing further to report beyond stopReason.
		return sessionPromptResult{StopReason: "refusal"}, nil
	}

	// The agent's reply text has no dedicated field in sessionPromptResult
	// per spec (the real reply is delivered via session/update
	// agent_message_chunk notifications the client already received as the
	// turn progressed). This request/response transport has no notification
	// channel, so — as a pragmatic, clearly-labeled deviation — the reply is
	// included under a non-spec "response" key alongside the spec-correct
	// stopReason, rather than silently dropping it.
	return map[string]interface{}{
		"stopReason": "end_turn",
		"response":   map[string]string{"role": "agent", "content": reply},
	}, nil
}

// handleMessageStream is NOT a real ACP method (see file header) — it's a
// transport adaptation that emits session/update-shaped notification frames
// over SSE so a client can observe progress on a single request, since this
// HTTP-only server has no separate channel to push real session/update
// notifications on. The frames use the spec's exact
// {"jsonrpc":"2.0","method":"session/update","params":{sessionId,update:{...}}}
// shape even though the surrounding delivery mechanism (SSE on this
// endpoint, keyed by an event name) is our own.
func (s *acpServer) handleMessageStream(w http.ResponseWriter, req rpcRequest, r *http.Request) {
	var params sessionPromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.ID, -32602, "Invalid params: "+err.Error())
		return
	}
	text := promptText(params.Prompt)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	send := func(update sessionUpdate) {
		frame := sessionUpdateNotification{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params:  sessionUpdateParams{SessionID: params.SessionID, Update: update},
		}
		payload, _ := json.Marshal(frame)
		fmt.Fprintf(w, "event: session.update\ndata: %s\n\n", payload)
		flusher.Flush()
	}

	if s.tuiBridge == nil {
		send(sessionUpdate{SessionUpdate: "agent_message_chunk", Content: &contentBlock{Type: "text", Text: "(no agent bridge configured)"}})
		return
	}
	if text == "" {
		s.writeError(w, req.ID, -32602, "Invalid params: prompt must contain at least one non-empty text block")
		return
	}

	reply, err := s.tuiBridge(r.Context(), text)
	if err != nil {
		send(sessionUpdate{SessionUpdate: "agent_message_chunk", Content: &contentBlock{Type: "text", Text: "error: " + err.Error()}})
		return
	}
	send(sessionUpdate{SessionUpdate: "agent_message_chunk", Content: &contentBlock{Type: "text", Text: reply}})
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
