package main

// agent_turn_test.go exercises runTurnWithConfirmations end-to-end against a
// REAL google.golang.org/adk Runner/llmagent/functiontool — only the
// model.LLM is faked (same test-double approach as model/echo.Model, which
// this repo already trusts as a real failover leg, not a mock). This proves
// the pause/resume mechanics against ADK's actual toolconfirmation flow
// rather than against an assumption of how it behaves.

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

// fakeConfirmModel is a two-turn scripted model.LLM: the first
// GenerateContent call emits a FunctionCall to dummy_tool (which
// requireConfirmation gates), and every call after that emits a fixed final
// text reply. It never touches the network.
type fakeConfirmModel struct {
	calls int
}

func (m *fakeConfirmModel) Name() string { return "fake-confirm-model" }

func (m *fakeConfirmModel) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	m.calls++
	first := m.calls == 1
	return func(yield func(*model.LLMResponse, error) bool) {
		// Role must be set to "model", exactly as a real provider's SDK
		// response always carries it: ADK's history-reassembly filter
		// (contents_processor.go) silently drops any event whose Content.Role
		// is empty, which orphans the auto-synthesized confirmation-pending
		// FunctionResponse and breaks the resume round trip on turn 2.
		if first {
			yield(&model.LLMResponse{TurnComplete: true, Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: "call-1", Name: "dummy_tool", Args: map[string]any{"x": 1.0}},
			}}}}, nil)
			return
		}
		yield(&model.LLMResponse{TurnComplete: true, Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{{Text: "done"}}}}, nil)
	}
}

type dummyArgs struct {
	X float64 `json:"x"`
}

type dummyResult struct {
	Ran bool `json:"ran"`
}

// newDummyRunner builds a real Runner around a real llmagent + fakeConfirmModel
// with one RequireConfirmation-gated tool, "dummy_tool". ran flips to true iff
// the tool handler actually executes (i.e. confirmation was granted).
func newDummyRunner(t *testing.T, ran *atomic.Bool) *runner.Runner {
	t.Helper()

	dummyTool, err := functiontool.New(functiontool.Config{
		Name:                "dummy_tool",
		Description:         "test tool gated on human confirmation",
		RequireConfirmation: true,
	}, func(_ tool.Context, _ dummyArgs) (dummyResult, error) {
		ran.Store(true)
		return dummyResult{Ran: true}, nil
	})
	if err != nil {
		t.Fatalf("functiontool.New: %v", err)
	}

	ag, err := llmagent.New(llmagent.Config{
		Name:  "test-agent",
		Model: &fakeConfirmModel{},
		Tools: []tool.Tool{dummyTool},
	})
	if err != nil {
		t.Fatalf("llmagent.New: %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             ag,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	return r
}

func TestRunTurnWithConfirmations_Approved(t *testing.T) {
	var ran atomic.Bool
	r := newDummyRunner(t, &ran)

	var gotToolCallID, gotToolName string
	var gotArgs map[string]any
	confirmCalls := 0

	text, _, _, err := runTurnWithConfirmations(
		context.Background(), r, "user1", "sess1", "hello",
		nil,
		func(_ context.Context, toolCallID, toolName string, args map[string]any) (bool, error) {
			confirmCalls++
			gotToolCallID, gotToolName, gotArgs = toolCallID, toolName, args
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("runTurnWithConfirmations: %v", err)
	}
	if text != "done" {
		t.Errorf("text = %q, want %q", text, "done")
	}
	if confirmCalls != 1 {
		t.Errorf("onConfirm called %d times, want 1", confirmCalls)
	}
	if gotToolName != "dummy_tool" {
		t.Errorf("toolName = %q, want dummy_tool", gotToolName)
	}
	if gotToolCallID == "" {
		t.Error("toolCallID is empty, want the original pending call's ID")
	}
	if x, _ := gotArgs["x"].(float64); x != 1.0 {
		t.Errorf(`args["x"] = %v, want 1.0`, gotArgs["x"])
	}
	if !ran.Load() {
		t.Error("dummy_tool handler never ran despite approval")
	}
}

func TestRunTurnWithConfirmations_Denied(t *testing.T) {
	var ran atomic.Bool
	r := newDummyRunner(t, &ran)

	text, _, _, err := runTurnWithConfirmations(
		context.Background(), r, "user1", "sess1", "hello",
		nil,
		func(context.Context, string, string, map[string]any) (bool, error) { return false, nil },
	)
	if err != nil {
		t.Fatalf("runTurnWithConfirmations: %v", err)
	}
	if !strings.Contains(text, "done") {
		t.Errorf("text = %q, want it to still reach the model's final reply", text)
	}
	if ran.Load() {
		t.Error("dummy_tool handler ran despite denial")
	}
}

func TestRunTurnWithConfirmations_NilHandlerErrors(t *testing.T) {
	var ran atomic.Bool
	r := newDummyRunner(t, &ran)

	_, _, _, err := runTurnWithConfirmations(context.Background(), r, "user1", "sess1", "hello", nil, nil)
	if err == nil {
		t.Fatal("expected an error when the turn pauses with no confirmation handler configured")
	}
	if !strings.Contains(err.Error(), "dummy_tool") {
		t.Errorf("error %q should name the tool awaiting confirmation", err.Error())
	}
	if ran.Load() {
		t.Error("dummy_tool handler ran despite no confirmation handler being available")
	}
}

func TestRunTurnWithConfirmations_ConfirmErrorAbortsTurn(t *testing.T) {
	var ran atomic.Bool
	r := newDummyRunner(t, &ran)
	wantErr := errors.New("client disconnected")

	_, _, _, err := runTurnWithConfirmations(
		context.Background(), r, "user1", "sess1", "hello",
		nil,
		func(context.Context, string, string, map[string]any) (bool, error) { return false, wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if ran.Load() {
		t.Error("dummy_tool handler ran despite onConfirm returning an error")
	}
}

func TestRunTurnWithConfirmations_OnTextForwardsDeltas(t *testing.T) {
	// No confirmation-gated tool in play here — a plain single-response
	// model — to isolate onText's delta-forwarding behavior.
	ag, err := llmagent.New(llmagent.Config{
		Name:  "plain-agent",
		Model: &plainTextModel{text: "hello world"},
	})
	if err != nil {
		t.Fatalf("llmagent.New: %v", err)
	}
	r, err := runner.New(runner.Config{
		AppName:           "test-app",
		Agent:             ag,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}

	var forwarded strings.Builder
	text, _, _, err := runTurnWithConfirmations(
		context.Background(), r, "user1", "sess1", "hi",
		func(delta string) { forwarded.WriteString(delta) },
		nil,
	)
	if err != nil {
		t.Fatalf("runTurnWithConfirmations: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if forwarded.String() != "hello world" {
		t.Errorf("forwarded = %q, want %q", forwarded.String(), "hello world")
	}
}

// plainTextModel always yields a single fixed text response — no tool calls.
type plainTextModel struct{ text string }

func (m *plainTextModel) Name() string { return "plain-text-model" }

func (m *plainTextModel) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{{Text: m.text}}}}, nil)
	}
}
