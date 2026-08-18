package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rpcCall posts a real JSON-RPC request to the server's real HTTP handler
// (httptest.NewServer — a real listening HTTP server, not a mock) and
// decodes the real response.
func rpcCall(t *testing.T, srv *httptest.Server, method string, params interface{}) map[string]interface{} {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(srv.URL+"/acp", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", method, err)
	}
	defer resp.Body.Close()

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response for %s: %v", method, err)
	}
	return out
}

func TestACP_Initialize_MatchesSpecShape(t *testing.T) {
	srv := newACPServer(nil)
	ts := httptest.NewServer(http.HandlerFunc(srv.handleRPC))
	defer ts.Close()

	out := rpcCall(t, ts, "initialize", map[string]interface{}{
		"protocolVersion": 1,
		"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
	})

	result, ok := out["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result object, got %v", out)
	}
	if _, ok := result["protocolVersion"]; !ok {
		t.Error("missing protocolVersion field (spec-required)")
	}
	agentInfo, ok := result["agentInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agentInfo object (spec name, not serverInfo), got %v", result)
	}
	if agentInfo["name"] != "go-adk-q" {
		t.Errorf("agentInfo.name = %v, want go-adk-q", agentInfo["name"])
	}
	caps, ok := result["agentCapabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agentCapabilities object (spec name, not capabilities), got %v", result)
	}
	if _, ok := caps["promptCapabilities"]; !ok {
		t.Error("missing agentCapabilities.promptCapabilities (spec-required nested field)")
	}
	if _, ok := caps["mcpCapabilities"]; !ok {
		t.Error("missing agentCapabilities.mcpCapabilities (spec-required nested field)")
	}
	if _, ok := result["authMethods"]; !ok {
		t.Error("missing authMethods field (spec-required, may be empty array)")
	}
}

func TestACP_SessionNew_MatchesSpecShape(t *testing.T) {
	srv := newACPServer(nil)
	ts := httptest.NewServer(http.HandlerFunc(srv.handleRPC))
	defer ts.Close()

	out := rpcCall(t, ts, "session/new", map[string]interface{}{
		"cwd":        "/tmp",
		"mcpServers": []interface{}{},
	})
	result, ok := out["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result object, got %v", out)
	}
	sid, ok := result["sessionId"].(string)
	if !ok || sid == "" {
		t.Fatalf("expected non-empty sessionId, got %v", result)
	}
}

func TestACP_SessionPrompt_RealBridgeRoundTrip(t *testing.T) {
	var gotInput string
	bridge := func(ctx context.Context, input string) (string, error) {
		gotInput = input
		return "echoed: " + input, nil
	}
	srv := newACPServer(bridge)
	ts := httptest.NewServer(http.HandlerFunc(srv.handleRPC))
	defer ts.Close()

	out := rpcCall(t, ts, "session/prompt", map[string]interface{}{
		"sessionId": "test-session",
		"prompt": []map[string]string{
			{"type": "text", "text": "hello from a real test"},
		},
	})
	result, ok := out["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result object, got %v", out)
	}
	if result["stopReason"] != "end_turn" {
		t.Errorf("stopReason = %v, want end_turn", result["stopReason"])
	}
	if gotInput != "hello from a real test" {
		t.Errorf("bridge received %q, want the prompt's text content extracted correctly", gotInput)
	}
	response, ok := result["response"].(map[string]interface{})
	if !ok || response["content"] != "echoed: hello from a real test" {
		t.Errorf("response.content = %v, want the real bridge reply", result["response"])
	}
}

func TestACP_SessionPrompt_RejectsEmptyPrompt(t *testing.T) {
	srv := newACPServer(func(ctx context.Context, input string) (string, error) { return "should not be called", nil })
	ts := httptest.NewServer(http.HandlerFunc(srv.handleRPC))
	defer ts.Close()

	out := rpcCall(t, ts, "session/prompt", map[string]interface{}{
		"sessionId": "test-session",
		"prompt":    []map[string]string{},
	})
	if out["error"] == nil {
		t.Fatal("expected an error for an empty prompt, got a result")
	}
}

func TestACP_UnknownMethod_ReturnsMethodNotFound(t *testing.T) {
	srv := newACPServer(nil)
	ts := httptest.NewServer(http.HandlerFunc(srv.handleRPC))
	defer ts.Close()

	// fs/read_text_file is real ACP but not implemented here (see file
	// header) — must fail cleanly, not panic or hang.
	out := rpcCall(t, ts, "fs/read_text_file", map[string]interface{}{
		"sessionId": "test-session",
		"path":      "/tmp/whatever",
	})
	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a JSON-RPC error for an unimplemented method, got %v", out)
	}
	if errObj["code"].(float64) != -32601 {
		t.Errorf("error.code = %v, want -32601 (Method not found)", errObj["code"])
	}
}
