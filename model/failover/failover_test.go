package failover_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"

	"go-adk-q/model/failover"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// ── Mock LLM helpers ──────────────────────────────────────────────────────────

// mockLLM is a test double for model.LLM.
// calls tracks how many times GenerateContent was invoked.
type mockLLM struct {
	name    string
	calls   int
	failErr error           // if non-nil, GenerateContent yields this error
	resp    *model.LLMResponse // returned on success
}

func (m *mockLLM) Name() string { return m.name }

func (m *mockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		if m.failErr != nil {
			yield(nil, m.failErr)
			return
		}
		yield(m.resp, nil)
	}
}

// newOKModel returns a mock that succeeds with a fixed text response.
func newOKModel(name, text string) *mockLLM {
	return &mockLLM{
		name: name,
		resp: &model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: text}},
			},
		},
	}
}

// newFailModel returns a mock that always returns errMsg as an error.
func newFailModel(name, errMsg string) *mockLLM {
	return &mockLLM{name: name, failErr: errors.New(errMsg)}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestPrimarySucceeds verifies that when the primary model works, no fallback
// is ever called and the primary response is returned unchanged.
func TestPrimarySucceeds(t *testing.T) {
	primary := newOKModel("primary", "hello from primary")
	backup := newOKModel("backup", "hello from backup")

	m := failover.New(primary, backup)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if primary.calls != 1 {
		t.Errorf("primary.calls = %d, want 1", primary.calls)
	}
	if backup.calls != 0 {
		t.Errorf("backup.calls = %d, want 0 (should not be called)", backup.calls)
	}
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(responses))
	}
	if got := responses[0].Content.Parts[0].Text; got != "hello from primary" {
		t.Errorf("response text = %q, want %q", got, "hello from primary")
	}
}

// TestPrimaryFailsFallbackSucceeds verifies the core failover behaviour: when
// the primary errors, the second provider is tried and its response is returned.
func TestPrimaryFailsFallbackSucceeds(t *testing.T) {
	primary := newFailModel("primary", "503 service unavailable")
	backup := newOKModel("backup", "hello from backup")

	m := failover.New(primary, backup)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if primary.calls != 1 {
		t.Errorf("primary.calls = %d, want 1", primary.calls)
	}
	if backup.calls != 1 {
		t.Errorf("backup.calls = %d, want 1", backup.calls)
	}
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(responses))
	}
	if got := responses[0].Content.Parts[0].Text; got != "hello from backup" {
		t.Errorf("response text = %q, want %q", got, "hello from backup")
	}
}

// TestChainOfThreeFirstTwoFail verifies that the model walks the full chain and
// stops at the first success even when multiple leading providers fail.
func TestChainOfThreeFirstTwoFail(t *testing.T) {
	p1 := newFailModel("p1", "rate limited")
	p2 := newFailModel("p2", "model overloaded")
	p3 := newOKModel("p3", "hello from p3")

	m := failover.New(p1, p2, p3)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if p1.calls != 1 {
		t.Errorf("p1.calls = %d, want 1", p1.calls)
	}
	if p2.calls != 1 {
		t.Errorf("p2.calls = %d, want 1", p2.calls)
	}
	if p3.calls != 1 {
		t.Errorf("p3.calls = %d, want 1", p3.calls)
	}
	if len(responses) == 0 || responses[0].Content.Parts[0].Text != "hello from p3" {
		t.Errorf("expected p3 response, got %+v", responses)
	}
}

// TestAllProvidersFail verifies that when every provider fails, a single
// wrapped error is returned that mentions all provider names.
func TestAllProvidersFail(t *testing.T) {
	p1 := newFailModel("gemini", "quota exceeded")
	p2 := newFailModel("groq", "503 upstream")
	p3 := newFailModel("nvidia", "timeout")

	m := failover.New(p1, p2, p3)

	var finalErr error
	for _, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			finalErr = err
		}
	}

	if finalErr == nil {
		t.Fatal("expected an error when all providers fail, got nil")
	}
	for _, name := range []string{"gemini", "groq", "nvidia"} {
		if !errors.Is(finalErr, findErr(finalErr, name)) {
			// Just check the string contains each provider name for readability.
			errStr := finalErr.Error()
			if !containsStr(errStr, name) {
				t.Errorf("error %q does not mention provider %q", errStr, name)
			}
		}
	}
	if p1.calls != 1 || p2.calls != 1 || p3.calls != 1 {
		t.Errorf("each provider should be tried exactly once; calls: p1=%d p2=%d p3=%d",
			p1.calls, p2.calls, p3.calls)
	}
}

// TestNameShowsChain verifies the composite name lists all providers with →.
func TestNameShowsChain(t *testing.T) {
	m := failover.New(
		newOKModel("gemini-2.5-flash", ""),
		newOKModel("groq/llama-3.3-70b", ""),
	)
	want := "failover(gemini-2.5-flash → groq/llama-3.3-70b)"
	if got := m.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// TestNilFallbacksIgnored verifies that nil entries in the fallback list are
// silently ignored rather than panicking.
func TestNilFallbacksIgnored(t *testing.T) {
	primary := newOKModel("primary", "ok")
	// Pass nil as a fallback (simulating an unconfigured provider).
	m := failover.New(primary, nil, nil)

	if m.Name() != "failover(primary)" {
		t.Errorf("Name() = %q, want %q", m.Name(), "failover(primary)")
	}

	for _, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// TestPrimaryNotCalledAgainAfterSuccess verifies idempotency: calling
// GenerateContent twice on the same failover model calls the primary twice
// (each call is independent; no state is shared).
func TestIndependentCalls(t *testing.T) {
	primary := newOKModel("primary", "ok")
	m := failover.New(primary)

	req := &model.LLMRequest{}
	for range m.GenerateContent(context.Background(), req, false) {
	}
	for range m.GenerateContent(context.Background(), req, false) {
	}

	if primary.calls != 2 {
		t.Errorf("primary.calls = %d, want 2 (one per GenerateContent call)", primary.calls)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func findErr(err error, substr string) error {
	return fmt.Errorf("%s", substr) // just for Is-compatibility check placeholder
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ── Additional mock types ─────────────────────────────────────────────────────

// nilMockLLM always yields (nil, nil) — simulates a provider heartbeat or
// streaming sentinel. Forwarding these to the ADK runner without filtering
// causes a nil-pointer dereference.
type nilMockLLM struct {
	name  string
	calls int
}

func (m *nilMockLLM) Name() string { return m.name }

func (m *nilMockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		yield(nil, nil)
	}
}

// multiMockLLM yields one *model.LLMResponse per text entry.
type multiMockLLM struct {
	name  string
	texts []string
	calls int
}

func (m *multiMockLLM) Name() string { return m.name }

func (m *multiMockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		for _, text := range m.texts {
			resp := &model.LLMResponse{
				Content: &genai.Content{Parts: []*genai.Part{{Text: text}}},
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// ── Additional tests ──────────────────────────────────────────────────────────

// TestNilResponsesFiltered verifies that (nil, nil) yields from a provider are
// dropped before being forwarded. The ADK runner does not nil-guard LLMResponse
// before dereferencing, so forwarding a nil entry would cause a panic.
func TestNilResponsesFiltered(t *testing.T) {
	nilProvider := &nilMockLLM{name: "nil-provider"}
	m := failover.New(nilProvider)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error from nil-response provider: %v", err)
		}
		responses = append(responses, resp)
	}

	if len(responses) != 0 {
		t.Errorf("got %d responses, want 0 — nil entries must be filtered", len(responses))
	}
	if nilProvider.calls != 1 {
		t.Errorf("nilProvider.calls = %d, want 1", nilProvider.calls)
	}
}

// TestMultipleResponsesForwarded verifies that when a provider yields several
// responses (streaming chunks), all of them are forwarded in order.
func TestMultipleResponsesForwarded(t *testing.T) {
	texts := []string{"alpha", "beta", "gamma"}
	primary := &multiMockLLM{name: "primary", texts: texts}
	m := failover.New(primary)

	var got []string
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, resp.Content.Parts[0].Text)
	}

	if len(got) != len(texts) {
		t.Fatalf("got %d responses, want %d", len(got), len(texts))
	}
	for i, want := range texts {
		if got[i] != want {
			t.Errorf("response[%d] = %q, want %q", i, got[i], want)
		}
	}
}

// TestContextCancelledStopsChain verifies that a pre-cancelled context prevents
// any provider from being invoked and causes a context-wrapped error to be
// returned immediately.
func TestContextCancelledStopsChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before any provider is tried

	p1 := newFailModel("p1", "rate limited")
	p2 := newOKModel("p2", "hello from p2")

	m := failover.New(p1, p2)

	var finalErr error
	for _, err := range m.GenerateContent(ctx, &model.LLMRequest{}, false) {
		if err != nil {
			finalErr = err
		}
	}

	if finalErr == nil {
		t.Fatal("expected a context error, got nil")
	}
	if !errors.Is(finalErr, context.Canceled) {
		t.Errorf("expected context.Canceled in error chain, got: %v", finalErr)
	}
	if p1.calls != 0 {
		t.Errorf("p1.calls = %d, want 0 (context was pre-cancelled)", p1.calls)
	}
	if p2.calls != 0 {
		t.Errorf("p2.calls = %d, want 0 (context was pre-cancelled)", p2.calls)
	}
}

// TestYieldFalseStopsEarly verifies that breaking out of the consumer's range
// loop (which signals yield=false) causes the failover iterator to stop
// forwarding further responses.
func TestYieldFalseStopsEarly(t *testing.T) {
	primary := &multiMockLLM{name: "primary", texts: []string{"first", "second", "third"}}
	m := failover.New(primary)

	var count int
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = resp
		count++
		break // stop after the first response
	}

	if count != 1 {
		t.Errorf("got %d responses, want 1 (stopped via break)", count)
	}
	if primary.calls != 1 {
		t.Errorf("primary.calls = %d, want 1", primary.calls)
	}
}
