package main

// acp_stdio.go — the real Agent Client Protocol transport: newline-delimited
// JSON-RPC 2.0 over stdio, per protocol/v1/transports.md (fetched live,
// 2026-07-18). Exact spec text: "Messages are delimited by newlines (\n),
// and MUST NOT contain embedded newlines" / "The client launches the agent
// as a subprocess. The agent reads JSON-RPC messages from its standard
// input (stdin) and sends messages to its standard output (stdout)." /
// "The agent MAY write UTF-8 strings to its standard error (stderr) for
// logging purposes." — stdout carries wire protocol only, nothing else may
// write there.
//
// Why this exists alongside acp_server.go's HTTP server: HTTP request/
// response has no persistent connection, so it can only ever reply to
// requests the client initiates. ACP's Agent→Client methods
// (fs/read_text_file, fs/write_text_file, terminal/*,
// session/request_permission) require the opposite: the agent originates a
// request mid-turn and blocks for the client's reply on the same
// connection. That needs one persistent bidirectional stream — which is
// exactly what stdio is, and precisely why the spec's file-system.md and
// tool-calls.md pages define these as Agent→Client in the first place. Per
// the same transports.md page, ACP's only HTTP option ("Streamable HTTP")
// is itself still "in discussion, draft proposal in progress" — so the
// existing HTTP server was never standards-track; this is the actual
// spec-conformant transport, not an alternative to it. No WebSocket
// transport is defined anywhere in the spec, so none is built here.
//
// Scope for this pass (deliberate, see docs/adr/ADR-0008-agent-harness-tools.md
// addendum — not an oversight):
//   - Full duplex plumbing: an outbound-request registry keyed by
//     self-originated id, a mutex-serialized writer, and a read loop that
//     demuxes "request from the client" (dispatch immediately) from
//     "response to one of our own outbound requests" (deliver to the
//     waiting caller).
//   - Client→agent methods (initialize/session/new/session/prompt) reuse
//     the exact same dispatch used by the HTTP server in acp_server.go —
//     one implementation, not two copies that can drift apart.
//   - Every Agent→Client method the spec defines is now built end-to-end:
//     fs/read_text_file (requestReadTextFile), fs/write_text_file
//     (requestWriteTextFile), terminal/create|output|wait_for_exit|kill|
//     release (requestTerminalCreate/Output/WaitForExit/Kill/Release), and
//     session/request_permission (requestPermission) — 2026-07-21, plumbing
//     only per explicit user scope (see SESSION_HANDOFF.md): each follows
//     requestReadTextFile's original shape exactly (now factored through a
//     shared sendRequest helper), capability-gated where the spec defines a
//     clientCapabilities flag (fs.readTextFile/fs.writeTextFile/terminal;
//     session/request_permission has no such flag — always available per
//     spec), and tested against an in-process mock client over a real
//     io.Pipe in acp_stdio_test.go — there is no real ACP client (e.g. Zed)
//     available in this environment to test against.
//   - Deliberately NOT wired into any real caller yet (e.g. write_file/
//     exec_command routing through these when running under ACP instead of
//     the local TUI) — that's a materially bigger integration decision
//     (mode detection, behavior change to already-shipped tools) the user
//     explicitly deferred this pass in favor of "plumbing only." Wire one in
//     following this same pattern once a real caller is scoped and asked for.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

// stdioReply is what the read loop delivers to a pending outbound request —
// the raw result bytes (decoded by the specific requestX caller, since each
// method has its own result shape) or an error.
type stdioReply struct {
	Result json.RawMessage
	Error  *rpcError
}

// stdioPeek is enough of a JSON-RPC message's shape to tell a client→agent
// request/notification apart from a response to our own outbound request,
// without committing to either type upfront.
type stdioPeek struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

// acpStdio adapts acpServer's shared dispatch logic onto the stdio
// transport and adds the Agent→Client request capability the HTTP server
// structurally cannot support.
type acpStdio struct {
	srv *acpServer

	outMu sync.Mutex
	out   io.Writer

	nextID int64 // atomic; ids this agent originates for its own requests

	pendingMu sync.Mutex
	pending   map[string]chan stdioReply

	// inFlight tracks incoming-request goroutines spawned by handleLine.
	// Serve waits on it after EOF before returning — without this, a fast
	// local pipe can deliver EOF before a just-spawned dispatch goroutine
	// gets scheduled, and the process exits having silently dropped every
	// response it never got the chance to write. Found live: piping two
	// requests into the real binary and observing empty stdout.
	inFlight sync.WaitGroup

	// done is closed the moment Serve's read loop exits (before inFlight.Wait),
	// so a requestReadTextFile call that's already registered in pending and
	// parked in its select — e.g. called from inside an in-flight
	// session/prompt turn — unblocks immediately on transport close instead of
	// waiting on failPending, which only runs *after* inFlight.Wait() returns
	// and would otherwise never fire for that very goroutine (inFlight.Wait
	// can't return until this goroutine does, and this goroutine can't return
	// until something unblocks its select — a real ordering gap, currently
	// unreachable since nothing calls requestReadTextFile except tests, but
	// latent for whoever wires a real caller in later).
	done chan struct{}
}

// newACPStdio wraps an existing acpServer (built the same way as the HTTP
// transport, via newACPServer) so both transports share one tuiBridge,
// session map, and negotiated clientCaps.
func newACPStdio(srv *acpServer) *acpStdio {
	return &acpStdio{
		srv:     srv,
		pending: make(map[string]chan stdioReply),
		done:    make(chan struct{}),
	}
}

// Serve runs the read loop until in reaches EOF, ctx is cancelled, or a
// fatal scanner error occurs. It blocks the calling goroutine — callers
// should invoke it directly from a command's RunE, not in a goroutine.
func (s *acpStdio) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out

	scanner := bufio.NewScanner(in)
	// Default bufio.Scanner line cap (64KiB) is too small for a document- or
	// tool-output-sized single-line JSON-RPC message; raise it. The spec
	// forbids embedded newlines, so a message is always exactly one line no
	// matter how large its content.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// Copy out of the scanner's reused buffer before handing off — the
		// client→agent branch dispatches in its own goroutine, and the
		// buffer's contents are only valid until the next Scan() call.
		msg := make([]byte, len(line))
		copy(msg, line)
		s.handleLine(ctx, msg)
	}

	// Signal transport-closed before waiting on inFlight — see done's doc
	// comment: a goroutine parked in requestReadTextFile's select must be
	// able to unblock on this to let inFlight.Wait() return at all.
	close(s.done)

	// Wait for every in-flight incoming-request dispatch to finish writing
	// its response before returning — see inFlight's doc comment for why
	// this is load-bearing, not defensive.
	s.inFlight.Wait()

	closeErr := errors.New("acp stdio: transport closed")
	s.failPending(closeErr)
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("acp stdio: read: %w", err)
	}
	return nil
}

// handleLine classifies one line as either a client→agent request/
// notification (has "method") or a response to one of our own outbound
// requests (no "method"; correlated by id against the pending map).
func (s *acpStdio) handleLine(ctx context.Context, line []byte) {
	var peek stdioPeek
	if err := json.Unmarshal(line, &peek); err != nil {
		_ = s.writeLine(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error: " + err.Error()}})
		return
	}

	if peek.Method != "" {
		req := rpcRequest{JSONRPC: peek.JSONRPC, ID: peek.ID, Method: peek.Method, Params: peek.Params}
		// Dispatched in its own goroutine so a long-running session/prompt
		// (an LLM turn) never blocks this read loop from receiving the
		// response to a request the agent itself originates mid-turn (e.g.
		// fs/read_text_file called from inside that same turn's tool use).
		// inFlight.Add before the goroutine starts (not inside it) — Serve's
		// EOF-time Wait() must never race a Add() that hasn't happened yet.
		s.inFlight.Add(1)
		go func() {
			defer s.inFlight.Done()
			s.handleIncomingRequest(ctx, req)
		}()
		return
	}

	if peek.ID == nil {
		return // malformed or an unmatched notification-shaped reply; nothing to correlate, safe to drop
	}
	key := string(*peek.ID)
	s.pendingMu.Lock()
	ch, ok := s.pending[key]
	if ok {
		delete(s.pending, key)
	}
	s.pendingMu.Unlock()
	if ok {
		ch <- stdioReply{Result: peek.Result, Error: peek.Error}
	}
}

// handleIncomingRequest runs a client→agent method through the same
// dispatch acp_server.go's HTTP handler uses, then writes the response
// frame back over stdout. Per JSON-RPC 2.0, a notification (no id) gets no
// response at all.
func (s *acpStdio) handleIncomingRequest(ctx context.Context, req rpcRequest) {
	if req.Method == "message/stream" {
		// message/stream is the HTTP transport's own SSE adaptation for
		// approximating session/update on a connectionless transport (see
		// acp_server.go's file header) — stdio is already a real persistent
		// duplex stream, so that adaptation has no meaning here.
		if req.ID != nil {
			_ = s.writeLine(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "Method not found: " + req.Method}})
		}
		return
	}

	result, rpcErr := s.srv.dispatch(ctx, req)
	if req.ID == nil {
		return // notification: JSON-RPC 2.0 defines no response
	}
	if rpcErr != nil {
		_ = s.writeLine(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
		return
	}
	_ = s.writeLine(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
}

// writeLine marshals v and writes it as one newline-terminated line,
// serialized behind outMu since both the client-request-response path and
// the agent-originated-request path share this one stdout stream.
func (s *acpStdio) writeLine(v interface{}) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if _, err := s.out.Write(payload); err != nil {
		return err
	}
	_, err = s.out.Write([]byte("\n"))
	return err
}

// failPending unblocks every in-flight agent-originated request with err —
// called once the transport closes, so a caller waiting in
// requestReadTextFile doesn't hang forever past EOF.
func (s *acpStdio) failPending(err error) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for id, ch := range s.pending {
		ch <- stdioReply{Error: &rpcError{Code: -32000, Message: err.Error()}}
		delete(s.pending, id)
	}
}

// fsReadTextFileParams mirrors protocol/v1/file-system.md's fs/read_text_file
// request shape (Agent→Client; fetched live, 2026-07-18).
type fsReadTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      *int   `json:"line,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

// fsReadTextFileResult mirrors the spec's fs/read_text_file response shape.
type fsReadTextFileResult struct {
	Content string `json:"content"`
}

// sendRequest is the common outbound Agent→Client call shape every
// requestX method below (and requestReadTextFile above) needs: allocate a
// self-originated id, register a pending reply channel, write the request
// line, then wait for a reply, ctx cancellation, or transport close —
// whichever comes first. Extracted once a second, third, ... consumer needed
// the exact same pattern requestReadTextFile established first; it still
// owns the outcome, the caller just gets the raw result bytes to decode into
// its own result type (each ACP method has a different result shape).
func (s *acpStdio) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("acp: marshal %s params: %w", method, err)
	}

	id := atomic.AddInt64(&s.nextID, 1)
	idRaw := json.RawMessage(strconv.FormatInt(id, 10))
	key := string(idRaw)

	reply := make(chan stdioReply, 1)
	s.pendingMu.Lock()
	s.pending[key] = reply
	s.pendingMu.Unlock()

	if err := s.writeLine(rpcRequest{JSONRPC: "2.0", ID: &idRaw, Method: method, Params: raw}); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
		return nil, fmt.Errorf("acp: send %s: %w", method, err)
	}

	select {
	case resp := <-reply:
		if resp.Error != nil {
			return nil, fmt.Errorf("acp: %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-s.done:
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
		return nil, fmt.Errorf("acp: transport closed while awaiting %s reply", method)
	}
}

// requestReadTextFile sends an Agent→Client fs/read_text_file request and
// blocks for the client's reply or ctx cancellation, whichever comes first.
// Per file-system.md: "Agents MUST NOT attempt to call the corresponding
// filesystem method" without the client having advertised
// clientCapabilities.fs.readTextFile in its initialize request — checked
// here, not left to the client to reject.
func (s *acpStdio) requestReadTextFile(ctx context.Context, sessionID, path string, line, limit *int) (string, error) {
	s.srv.mu.Lock()
	caps := s.srv.clientCaps
	s.srv.mu.Unlock()
	if caps == nil || caps.FS == nil || !caps.FS.ReadTextFile {
		return "", fmt.Errorf("acp: client did not advertise fs.readTextFile support")
	}

	result, err := s.sendRequest(ctx, "fs/read_text_file", fsReadTextFileParams{SessionID: sessionID, Path: path, Line: line, Limit: limit})
	if err != nil {
		return "", err
	}
	var res fsReadTextFileResult
	if err := json.Unmarshal(result, &res); err != nil {
		return "", fmt.Errorf("acp: fs/read_text_file: decode result: %w", err)
	}
	return res.Content, nil
}

// fsWriteTextFileParams mirrors protocol v1 schema's WriteTextFileRequest
// (fetched from the ACP repo's authoritative schema/v1/schema.json, not
// guessed) — all three fields required. Per spec, "the Client MUST create
// the file if it doesn't exist"; the result is empty on success.
type fsWriteTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// requestWriteTextFile sends an Agent→Client fs/write_text_file request,
// gated on clientCapabilities.fs.writeTextFile — same pattern as
// requestReadTextFile above, for the sibling capability.
func (s *acpStdio) requestWriteTextFile(ctx context.Context, sessionID, path, content string) error {
	s.srv.mu.Lock()
	caps := s.srv.clientCaps
	s.srv.mu.Unlock()
	if caps == nil || caps.FS == nil || !caps.FS.WriteTextFile {
		return fmt.Errorf("acp: client did not advertise fs.writeTextFile support")
	}
	_, err := s.sendRequest(ctx, "fs/write_text_file", fsWriteTextFileParams{SessionID: sessionID, Path: path, Content: content})
	return err
}

// terminalCapable reports whether the client's negotiated clientCapabilities
// advertised terminal support — gates every terminal/* method below, per the
// same "agents MUST NOT attempt" posture file-system.md states for fs/*.
func (s *acpStdio) terminalCapable() bool {
	s.srv.mu.Lock()
	defer s.srv.mu.Unlock()
	return s.srv.clientCaps != nil && s.srv.clientCaps.Terminal
}

// envVariable mirrors protocol v1 schema's EnvVariable (used by
// terminal/create's optional env field).
type envVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// terminalCreateParams mirrors protocol v1 schema's CreateTerminalRequest.
// Only sessionId/command are required; args/env/cwd/outputByteLimit are all
// optional per spec.
type terminalCreateParams struct {
	SessionID       string        `json:"sessionId"`
	Command         string        `json:"command"`
	Args            []string      `json:"args,omitempty"`
	Env             []envVariable `json:"env,omitempty"`
	CWD             string        `json:"cwd,omitempty"`
	OutputByteLimit *int          `json:"outputByteLimit,omitempty"`
}

type terminalCreateResult struct {
	TerminalID string `json:"terminalId"`
}

// requestTerminalCreate sends an Agent→Client terminal/create request and
// returns the client-assigned terminalId used by every other terminal/*
// method below (output/wait_for_exit/kill/release).
func (s *acpStdio) requestTerminalCreate(ctx context.Context, params terminalCreateParams) (string, error) {
	if !s.terminalCapable() {
		return "", fmt.Errorf("acp: client did not advertise terminal support")
	}
	result, err := s.sendRequest(ctx, "terminal/create", params)
	if err != nil {
		return "", err
	}
	var res terminalCreateResult
	if err := json.Unmarshal(result, &res); err != nil {
		return "", fmt.Errorf("acp: terminal/create: decode result: %w", err)
	}
	return res.TerminalID, nil
}

// terminalIDParams is the {sessionId, terminalId} shape shared by
// terminal/output, terminal/wait_for_exit, terminal/kill, and
// terminal/release per protocol v1 schema.
type terminalIDParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// terminalExitStatus mirrors protocol v1 schema's TerminalExitStatus — both
// fields optional (a still-running terminal reports neither).
type terminalExitStatus struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

type terminalOutputResult struct {
	Output     string              `json:"output"`
	Truncated  bool                `json:"truncated"`
	ExitStatus *terminalExitStatus `json:"exitStatus,omitempty"`
}

// requestTerminalOutput sends an Agent→Client terminal/output request — a
// non-blocking snapshot of output-so-far, unlike wait_for_exit below.
func (s *acpStdio) requestTerminalOutput(ctx context.Context, sessionID, terminalID string) (terminalOutputResult, error) {
	if !s.terminalCapable() {
		return terminalOutputResult{}, fmt.Errorf("acp: client did not advertise terminal support")
	}
	result, err := s.sendRequest(ctx, "terminal/output", terminalIDParams{SessionID: sessionID, TerminalID: terminalID})
	if err != nil {
		return terminalOutputResult{}, err
	}
	var res terminalOutputResult
	if err := json.Unmarshal(result, &res); err != nil {
		return terminalOutputResult{}, fmt.Errorf("acp: terminal/output: decode result: %w", err)
	}
	return res, nil
}

type waitForTerminalExitResult struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

// requestTerminalWaitForExit sends an Agent→Client terminal/wait_for_exit
// request and blocks (on the CLIENT side — this call itself only blocks on
// ctx/transport-close, same as every other requestX here) until the command
// exits.
func (s *acpStdio) requestTerminalWaitForExit(ctx context.Context, sessionID, terminalID string) (waitForTerminalExitResult, error) {
	if !s.terminalCapable() {
		return waitForTerminalExitResult{}, fmt.Errorf("acp: client did not advertise terminal support")
	}
	result, err := s.sendRequest(ctx, "terminal/wait_for_exit", terminalIDParams{SessionID: sessionID, TerminalID: terminalID})
	if err != nil {
		return waitForTerminalExitResult{}, err
	}
	var res waitForTerminalExitResult
	if err := json.Unmarshal(result, &res); err != nil {
		return waitForTerminalExitResult{}, fmt.Errorf("acp: terminal/wait_for_exit: decode result: %w", err)
	}
	return res, nil
}

// requestTerminalKill sends an Agent→Client terminal/kill request — kills
// the process WITHOUT releasing the terminal (its output remains readable
// via terminal/output until requestTerminalRelease is called). Result is
// empty per spec.
func (s *acpStdio) requestTerminalKill(ctx context.Context, sessionID, terminalID string) error {
	if !s.terminalCapable() {
		return fmt.Errorf("acp: client did not advertise terminal support")
	}
	_, err := s.sendRequest(ctx, "terminal/kill", terminalIDParams{SessionID: sessionID, TerminalID: terminalID})
	return err
}

// requestTerminalRelease sends an Agent→Client terminal/release request,
// freeing the client's resources for this terminal. Per spec, a terminal
// referenced by an embedded ToolCallContent (type "terminal") must be
// released only after it's no longer needed for display. Result is empty.
func (s *acpStdio) requestTerminalRelease(ctx context.Context, sessionID, terminalID string) error {
	if !s.terminalCapable() {
		return fmt.Errorf("acp: client did not advertise terminal support")
	}
	_, err := s.sendRequest(ctx, "terminal/release", terminalIDParams{SessionID: sessionID, TerminalID: terminalID})
	return err
}

// toolCallLocation mirrors protocol v1 schema's ToolCallLocation.
type toolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// toolCallUpdate mirrors protocol v1 schema's ToolCallUpdate — only
// toolCallId is required, "only changed fields need to be included" per
// spec. Content is left as raw JSON (ToolCallContent is itself a 3-way
// discriminated union of content/diff/terminal blocks) — no caller in this
// repo constructs one yet, so a fully-typed union would be speculative;
// json.RawMessage lets a future real caller supply exact wire JSON without
// this file needing every variant modeled up front.
type toolCallUpdate struct {
	ToolCallID string             `json:"toolCallId"`
	Kind       string             `json:"kind,omitempty"`
	Status     string             `json:"status,omitempty"`
	Title      string             `json:"title,omitempty"`
	Content    []json.RawMessage  `json:"content,omitempty"`
	Locations  []toolCallLocation `json:"locations,omitempty"`
}

// permissionOption mirrors protocol v1 schema's PermissionOption. Kind must
// be one of "allow_once"/"allow_always"/"reject_once"/"reject_always" per
// PermissionOptionKind's enum — passed as a plain string rather than a new
// Go type since nothing in this repo constructs these yet (same
// no-speculative-typing call as toolCallUpdate.Content above).
type permissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

type requestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  toolCallUpdate     `json:"toolCall"`
	Options   []permissionOption `json:"options"`
}

// requestPermissionOutcome mirrors protocol v1 schema's
// RequestPermissionOutcome, a discriminated union flattened into one struct:
// {"outcome":"cancelled"} (client sent session/cancel mid-request) or
// {"outcome":"selected","optionId":"..."} (user picked one of the options).
type requestPermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

type requestPermissionResult struct {
	Outcome requestPermissionOutcome `json:"outcome"`
}

// requestPermission sends an Agent→Client session/request_permission
// request. Unlike fs/*/terminal/*, this method has no corresponding
// clientCapabilities flag in protocol v1 schema — it is always available,
// so there is no capability gate here (unlike every requestX above).
func (s *acpStdio) requestPermission(ctx context.Context, sessionID string, toolCall toolCallUpdate, options []permissionOption) (requestPermissionOutcome, error) {
	result, err := s.sendRequest(ctx, "session/request_permission", requestPermissionParams{SessionID: sessionID, ToolCall: toolCall, Options: options})
	if err != nil {
		return requestPermissionOutcome{}, err
	}
	var res requestPermissionResult
	if err := json.Unmarshal(result, &res); err != nil {
		return requestPermissionOutcome{}, fmt.Errorf("acp: session/request_permission: decode result: %w", err)
	}
	return res.Outcome, nil
}
