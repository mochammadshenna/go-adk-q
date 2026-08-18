package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// stdioHarness drives acpStdio over a real io.Pipe in each direction, the
// same way a real ACP client subprocess would: writing ndjson lines into
// the agent's stdin and reading ndjson lines back from its stdout. Nothing
// about acpStdio itself is mocked — only "the other end of the pipe" (a
// real ACP client, e.g. Zed) is unavailable in this environment.
type stdioHarness struct {
	stdio   *acpStdio
	toIn    *io.PipeWriter
	fromOut *bufio.Scanner
}

func newStdioHarness(bridge func(ctx context.Context, input string) (string, error)) *stdioHarness {
	srv := newACPServer(bridge)
	stdio := newACPStdio(srv)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	go func() { _ = stdio.Serve(context.Background(), inR, outW) }()

	return &stdioHarness{stdio: stdio, toIn: inW, fromOut: bufio.NewScanner(outR)}
}

func (h *stdioHarness) send(t *testing.T, v interface{}) {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := h.toIn.Write(append(payload, '\n')); err != nil {
		t.Fatalf("write to agent stdin: %v", err)
	}
}

// recvLine reads one ndjson line from the agent's stdout with a timeout, so
// a transport bug fails the test with a clear message instead of hanging.
func (h *stdioHarness) recvLine(t *testing.T) map[string]interface{} {
	t.Helper()
	lineCh := make(chan string, 1)
	go func() {
		if h.fromOut.Scan() {
			lineCh <- h.fromOut.Text()
		} else {
			lineCh <- ""
		}
	}()
	select {
	case line := <-lineCh:
		if line == "" {
			t.Fatalf("agent stdout closed/empty: %v", h.fromOut.Err())
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			t.Fatalf("decode agent stdout line %q: %v", line, err)
		}
		return out
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a line on agent stdout")
		return nil
	}
}

func TestACPStdio_SessionPromptRoundTrip_OverRealPipe(t *testing.T) {
	var gotInput string
	h := newStdioHarness(func(ctx context.Context, input string) (string, error) {
		gotInput = input
		return "echoed: " + input, nil
	})

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": 1,
			"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
		},
	})
	if resp := h.recvLine(t); resp["error"] != nil {
		t.Fatalf("initialize returned an error: %v", resp["error"])
	}

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]interface{}{"cwd": "/tmp", "mcpServers": []interface{}{}},
	})
	newResp := h.recvLine(t)
	result, ok := newResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("session/new: expected a result object, got %v", newResp)
	}
	sid, _ := result["sessionId"].(string)
	if sid == "" {
		t.Fatal("session/new: empty sessionId")
	}

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 3, "method": "session/prompt",
		"params": map[string]interface{}{
			"sessionId": sid,
			"prompt":    []map[string]string{{"type": "text", "text": "hello over stdio"}},
		},
	})
	promptResp := h.recvLine(t)
	promptResult, ok := promptResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("session/prompt: expected a result object, got %v", promptResp)
	}
	if promptResult["stopReason"] != "end_turn" {
		t.Errorf("stopReason = %v, want end_turn", promptResult["stopReason"])
	}
	if gotInput != "hello over stdio" {
		t.Errorf("bridge received %q, want the prompt's extracted text", gotInput)
	}
	response, ok := promptResult["response"].(map[string]interface{})
	if !ok || response["content"] != "echoed: hello over stdio" {
		t.Errorf("response.content = %v, want the real bridge reply", promptResult["response"])
	}
}

// TestACPStdio_RequestReadTextFile_MockClientRespondsOverPipe is the one
// full end-to-end proof this pass commits to: a real Agent→Client request
// sent from the agent, received and answered by a mock client acting over
// the same real pipe, correlated back to the waiting caller by id.
func TestACPStdio_RequestReadTextFile_MockClientRespondsOverPipe(t *testing.T) {
	h := newStdioHarness(nil)

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": 1,
			"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
			"clientCapabilities": map[string]interface{}{
				"fs": map[string]bool{"readTextFile": true, "writeTextFile": false},
			},
		},
	})
	if resp := h.recvLine(t); resp["error"] != nil {
		t.Fatalf("initialize returned an error: %v", resp["error"])
	}

	contentCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		content, err := h.stdio.requestReadTextFile(context.Background(), "sess-1", "/tmp/foo.txt", nil, nil)
		contentCh <- content
		errCh <- err
	}()

	req := h.recvLine(t)
	if req["method"] != "fs/read_text_file" {
		t.Fatalf("expected an fs/read_text_file request, got %v", req)
	}
	params, _ := req["params"].(map[string]interface{})
	if params["path"] != "/tmp/foo.txt" || params["sessionId"] != "sess-1" {
		t.Errorf("unexpected request params: %v", params)
	}

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": req["id"],
		"result": map[string]string{"content": "real file content"},
	})

	if err := <-errCh; err != nil {
		t.Fatalf("requestReadTextFile returned an error: %v", err)
	}
	if got := <-contentCh; got != "real file content" {
		t.Errorf("content = %q, want %q", got, "real file content")
	}
}

func TestACPStdio_RequestReadTextFile_RejectsWithoutCapability(t *testing.T) {
	h := newStdioHarness(nil)
	// No initialize call at all — clientCaps stays nil, so this must fail
	// fast per file-system.md's "Agents MUST NOT attempt to call the
	// corresponding filesystem method" without a negotiated capability.
	if _, err := h.stdio.requestReadTextFile(context.Background(), "sess-1", "/tmp/foo.txt", nil, nil); err == nil {
		t.Fatal("expected an error when the client never advertised fs.readTextFile support")
	}
}

func TestACPStdio_RequestReadTextFile_ContextCancelledCleansUpPending(t *testing.T) {
	h := newStdioHarness(nil)
	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": 1,
			"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
			"clientCapabilities": map[string]interface{}{
				"fs": map[string]bool{"readTextFile": true},
			},
		},
	})
	h.recvLine(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := h.stdio.requestReadTextFile(ctx, "sess-1", "/tmp/never-answered.txt", nil, nil)
		errCh <- err
	}()

	// Drain the outgoing request (unblocks the writer) then simply never
	// reply — the mock client "goes silent", forcing the ctx.Done() branch
	// instead of the reply branch.
	if req := h.recvLine(t); req["method"] != "fs/read_text_file" {
		t.Fatalf("expected an fs/read_text_file request, got %v", req)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected a context-deadline error since the mock client never replies")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("requestReadTextFile did not return after its context was cancelled")
	}

	h.stdio.pendingMu.Lock()
	n := len(h.stdio.pending)
	h.stdio.pendingMu.Unlock()
	if n != 0 {
		t.Errorf("pending map has %d leftover entries after cancellation, want 0 (goroutine/memory leak)", n)
	}
}

// TestACPStdio_EOFImmediatelyAfterInput_StillDeliversAllResponses is a
// regression test for a real bug found by piping input into the live
// binary (not caught by the other tests above, since they never close the
// input pipe): dispatching each incoming request via a bare `go` statement
// let Serve's EOF path return before those goroutines got scheduled, so the
// process exited having silently written nothing. Reproduced here with a
// io.Pipe that is closed (EOF) immediately after both requests are
// written — the fast-EOF race that triggered it live.
func TestACPStdio_EOFImmediatelyAfterInput_StillDeliversAllResponses(t *testing.T) {
	srv := newACPServer(nil)
	stdio := newACPStdio(srv)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	serveDone := make(chan error, 1)
	go func() { serveDone <- stdio.Serve(context.Background(), inR, outW) }()

	go func() {
		_, _ = inW.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"t","version":"1.0"}}}` + "\n"))
		_, _ = inW.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp","mcpServers":[]}}` + "\n"))
		_ = inW.Close() // EOF immediately after — the exact race that dropped responses live
	}()

	scanner := bufio.NewScanner(outR)
	got := 0
	for got < 2 {
		lineCh := make(chan bool, 1)
		go func() { lineCh <- scanner.Scan() }()
		select {
		case ok := <-lineCh:
			if !ok {
				t.Fatalf("agent stdout closed after only %d/2 responses (the EOF-race regression)", got)
			}
			got++
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out after %d/2 responses", got)
		}
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after EOF and all responses delivered")
	}
}

// TestACPStdio_EOFWhileOutboundRequestInFlight_UnblocksViaDone is a
// regression test for a real ordering gap found by advisor review (not by a
// failing test — the prior EOF-race test only covered simple request/
// response, never "an outbound request is in flight at EOF"): Serve calls
// inFlight.Wait() before failPending(), so a goroutine parked in
// requestReadTextFile's select — reached from inside an in-flight
// session/prompt turn — could never be released by failPending (which runs
// only after Wait returns) and had no other way to observe transport
// closure. Fixed by closing acpStdio.done before inFlight.Wait() and adding
// a <-s.done arm to requestReadTextFile's select. This test drives exactly
// that scenario: a bridge that calls requestReadTextFile mid-turn, with the
// client going away (EOF) before ever answering.
func TestACPStdio_EOFWhileOutboundRequestInFlight_UnblocksViaDone(t *testing.T) {
	var stdio *acpStdio
	bridge := func(ctx context.Context, input string) (string, error) {
		_, err := stdio.requestReadTextFile(ctx, "sess-1", "/tmp/mid-turn.txt", nil, nil)
		return "", err
	}
	srv := newACPServer(bridge)
	stdio = newACPStdio(srv)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	serveDone := make(chan error, 1)
	go func() { serveDone <- stdio.Serve(context.Background(), inR, outW) }()

	scanner := bufio.NewScanner(outR)
	send := func(v interface{}) {
		payload, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := inW.Write(append(payload, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": 1,
			"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
			"clientCapabilities": map[string]interface{}{
				"fs": map[string]bool{"readTextFile": true},
			},
		},
	})
	if !scanner.Scan() {
		t.Fatalf("no initialize response: %v", scanner.Err())
	}

	// This session/prompt turn calls requestReadTextFile inside the bridge,
	// which sends fs/read_text_file and parks in its select awaiting a reply.
	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "session/prompt",
		"params": map[string]interface{}{
			"sessionId": "sess-1",
			"prompt":    []map[string]string{{"type": "text", "text": "hi"}},
		},
	})

	if !scanner.Scan() {
		t.Fatalf("no fs/read_text_file request observed: %v", scanner.Err())
	}
	var req map[string]interface{}
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req["method"] != "fs/read_text_file" {
		t.Fatalf("expected fs/read_text_file, got %v", req)
	}

	// The client goes away with the outbound request still unanswered.
	if err := inW.Close(); err != nil {
		t.Fatalf("close input pipe: %v", err)
	}
	// Drain the session/prompt turn's eventual response so its writer isn't
	// left blocked on an unread pipe — that would itself hang Serve.
	go func() { scanner.Scan() }()

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after EOF while an outbound request was in flight — the ordering bug is back")
	}
}

func TestACPStdio_MalformedLine_DoesNotCrashTransport(t *testing.T) {
	h := newStdioHarness(nil)

	if _, err := h.toIn.Write([]byte("{not valid json\n")); err != nil {
		t.Fatalf("write malformed line: %v", err)
	}
	resp := h.recvLine(t)
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"].(float64) != -32700 {
		t.Fatalf("expected a -32700 parse error for a malformed line, got %v", resp)
	}

	// The transport must still be alive afterward — not just not-panicking,
	// but genuinely still able to serve the next real request.
	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": 1,
			"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
		},
	})
	resp2 := h.recvLine(t)
	if resp2["error"] != nil {
		t.Fatalf("initialize after a malformed line failed: %v", resp2["error"])
	}
}

// ── fs/write_text_file, terminal/*, session/request_permission ────────────
//
// Same real-io.Pipe mock-client technique as the fs/read_text_file tests
// above — nothing about acpStdio is mocked, only "the other end of the pipe"
// (a real ACP client) is unavailable in this environment.

func initWithCaps(t *testing.T, h *stdioHarness, caps map[string]interface{}) {
	t.Helper()
	params := map[string]interface{}{
		"protocolVersion": 1,
		"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
	}
	if caps != nil {
		params["clientCapabilities"] = caps
	}
	h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": params})
	if resp := h.recvLine(t); resp["error"] != nil {
		t.Fatalf("initialize returned an error: %v", resp["error"])
	}
}

func TestACPStdio_RequestWriteTextFile_MockClientRespondsOverPipe(t *testing.T) {
	h := newStdioHarness(nil)
	initWithCaps(t, h, map[string]interface{}{"fs": map[string]bool{"writeTextFile": true}})

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.stdio.requestWriteTextFile(context.Background(), "sess-1", "/tmp/out.txt", "new content")
	}()

	req := h.recvLine(t)
	if req["method"] != "fs/write_text_file" {
		t.Fatalf("expected an fs/write_text_file request, got %v", req)
	}
	params, _ := req["params"].(map[string]interface{})
	if params["path"] != "/tmp/out.txt" || params["content"] != "new content" || params["sessionId"] != "sess-1" {
		t.Errorf("unexpected request params: %v", params)
	}

	h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]interface{}{}})

	if err := <-errCh; err != nil {
		t.Fatalf("requestWriteTextFile returned an error: %v", err)
	}
}

func TestACPStdio_RequestWriteTextFile_RejectsWithoutCapability(t *testing.T) {
	h := newStdioHarness(nil)
	initWithCaps(t, h, map[string]interface{}{"fs": map[string]bool{"writeTextFile": false}})
	if err := h.stdio.requestWriteTextFile(context.Background(), "sess-1", "/tmp/out.txt", "x"); err == nil {
		t.Fatal("expected an error when the client did not advertise fs.writeTextFile support")
	}
}

func TestACPStdio_TerminalMethods_MockClientRespondsOverPipe(t *testing.T) {
	h := newStdioHarness(nil)
	initWithCaps(t, h, map[string]interface{}{"terminal": true})

	t.Run("create", func(t *testing.T) {
		idCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			id, err := h.stdio.requestTerminalCreate(context.Background(), terminalCreateParams{
				SessionID: "sess-1", Command: "echo", Args: []string{"hi"},
			})
			idCh <- id
			errCh <- err
		}()
		req := h.recvLine(t)
		if req["method"] != "terminal/create" {
			t.Fatalf("expected terminal/create, got %v", req)
		}
		params, _ := req["params"].(map[string]interface{})
		if params["command"] != "echo" {
			t.Errorf("unexpected params: %v", params)
		}
		h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]string{"terminalId": "term-1"}})
		if err := <-errCh; err != nil {
			t.Fatalf("requestTerminalCreate error: %v", err)
		}
		if id := <-idCh; id != "term-1" {
			t.Errorf("terminalId = %q, want %q", id, "term-1")
		}
	})

	t.Run("output", func(t *testing.T) {
		resCh := make(chan terminalOutputResult, 1)
		errCh := make(chan error, 1)
		go func() {
			res, err := h.stdio.requestTerminalOutput(context.Background(), "sess-1", "term-1")
			resCh <- res
			errCh <- err
		}()
		req := h.recvLine(t)
		if req["method"] != "terminal/output" {
			t.Fatalf("expected terminal/output, got %v", req)
		}
		h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]interface{}{"output": "hello\n", "truncated": false}})
		if err := <-errCh; err != nil {
			t.Fatalf("requestTerminalOutput error: %v", err)
		}
		if res := <-resCh; res.Output != "hello\n" || res.Truncated {
			t.Errorf("unexpected result: %+v", res)
		}
	})

	t.Run("wait_for_exit", func(t *testing.T) {
		resCh := make(chan waitForTerminalExitResult, 1)
		errCh := make(chan error, 1)
		go func() {
			res, err := h.stdio.requestTerminalWaitForExit(context.Background(), "sess-1", "term-1")
			resCh <- res
			errCh <- err
		}()
		req := h.recvLine(t)
		if req["method"] != "terminal/wait_for_exit" {
			t.Fatalf("expected terminal/wait_for_exit, got %v", req)
		}
		h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]interface{}{"exitCode": 0}})
		if err := <-errCh; err != nil {
			t.Fatalf("requestTerminalWaitForExit error: %v", err)
		}
		res := <-resCh
		if res.ExitCode == nil || *res.ExitCode != 0 {
			t.Errorf("unexpected result: %+v", res)
		}
	})

	t.Run("kill", func(t *testing.T) {
		errCh := make(chan error, 1)
		go func() { errCh <- h.stdio.requestTerminalKill(context.Background(), "sess-1", "term-1") }()
		req := h.recvLine(t)
		if req["method"] != "terminal/kill" {
			t.Fatalf("expected terminal/kill, got %v", req)
		}
		h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]interface{}{}})
		if err := <-errCh; err != nil {
			t.Fatalf("requestTerminalKill error: %v", err)
		}
	})

	t.Run("release", func(t *testing.T) {
		errCh := make(chan error, 1)
		go func() { errCh <- h.stdio.requestTerminalRelease(context.Background(), "sess-1", "term-1") }()
		req := h.recvLine(t)
		if req["method"] != "terminal/release" {
			t.Fatalf("expected terminal/release, got %v", req)
		}
		h.send(t, map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": map[string]interface{}{}})
		if err := <-errCh; err != nil {
			t.Fatalf("requestTerminalRelease error: %v", err)
		}
	})
}

func TestACPStdio_TerminalMethods_RejectWithoutCapability(t *testing.T) {
	h := newStdioHarness(nil)
	initWithCaps(t, h, map[string]interface{}{"terminal": false})

	ctx := context.Background()
	if _, err := h.stdio.requestTerminalCreate(ctx, terminalCreateParams{SessionID: "s", Command: "echo"}); err == nil {
		t.Error("requestTerminalCreate: expected an error without terminal capability")
	}
	if _, err := h.stdio.requestTerminalOutput(ctx, "s", "t"); err == nil {
		t.Error("requestTerminalOutput: expected an error without terminal capability")
	}
	if _, err := h.stdio.requestTerminalWaitForExit(ctx, "s", "t"); err == nil {
		t.Error("requestTerminalWaitForExit: expected an error without terminal capability")
	}
	if err := h.stdio.requestTerminalKill(ctx, "s", "t"); err == nil {
		t.Error("requestTerminalKill: expected an error without terminal capability")
	}
	if err := h.stdio.requestTerminalRelease(ctx, "s", "t"); err == nil {
		t.Error("requestTerminalRelease: expected an error without terminal capability")
	}
}

func TestACPStdio_RequestPermission_Selected_MockClientRespondsOverPipe(t *testing.T) {
	h := newStdioHarness(nil)
	// session/request_permission has no clientCapabilities gate per spec —
	// deliberately initializing with NO capabilities at all to prove that.
	initWithCaps(t, h, nil)

	outcomeCh := make(chan requestPermissionOutcome, 1)
	errCh := make(chan error, 1)
	go func() {
		outcome, err := h.stdio.requestPermission(context.Background(), "sess-1",
			toolCallUpdate{ToolCallID: "call-1", Title: "Run rm -rf /tmp/scratch", Kind: "execute"},
			[]permissionOption{
				{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
				{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
			})
		outcomeCh <- outcome
		errCh <- err
	}()

	req := h.recvLine(t)
	if req["method"] != "session/request_permission" {
		t.Fatalf("expected session/request_permission, got %v", req)
	}
	params, _ := req["params"].(map[string]interface{})
	toolCall, _ := params["toolCall"].(map[string]interface{})
	if toolCall["toolCallId"] != "call-1" {
		t.Errorf("unexpected toolCall in request: %v", toolCall)
	}
	options, _ := params["options"].([]interface{})
	if len(options) != 2 {
		t.Fatalf("expected 2 options, got %v", options)
	}

	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": req["id"],
		"result": map[string]interface{}{"outcome": map[string]interface{}{"outcome": "selected", "optionId": "allow"}},
	})

	if err := <-errCh; err != nil {
		t.Fatalf("requestPermission error: %v", err)
	}
	outcome := <-outcomeCh
	if outcome.Outcome != "selected" || outcome.OptionID != "allow" {
		t.Errorf("unexpected outcome: %+v", outcome)
	}
}

func TestACPStdio_RequestPermission_Cancelled_MockClientRespondsOverPipe(t *testing.T) {
	h := newStdioHarness(nil)
	initWithCaps(t, h, nil)

	outcomeCh := make(chan requestPermissionOutcome, 1)
	errCh := make(chan error, 1)
	go func() {
		outcome, err := h.stdio.requestPermission(context.Background(), "sess-1",
			toolCallUpdate{ToolCallID: "call-2"},
			[]permissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}})
		outcomeCh <- outcome
		errCh <- err
	}()

	req := h.recvLine(t)
	h.send(t, map[string]interface{}{
		"jsonrpc": "2.0", "id": req["id"],
		"result": map[string]interface{}{"outcome": map[string]interface{}{"outcome": "cancelled"}},
	})

	if err := <-errCh; err != nil {
		t.Fatalf("requestPermission error: %v", err)
	}
	if outcome := <-outcomeCh; outcome.Outcome != "cancelled" {
		t.Errorf("outcome = %+v, want cancelled", outcome)
	}
}
