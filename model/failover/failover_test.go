package failover_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

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
	failErr error              // if non-nil, GenerateContent yields this error
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
//
// p1's fixture error text ("rate limited") is intentionally 429-shaped: since
// F9, that now triggers exactly one same-provider retry before p1 is marked
// failed and the chain moves on — hence p1.calls == 2, not 1. The backoff is
// zeroed so the retry does not add real wall-clock delay to this test.
func TestChainOfThreeFirstTwoFail(t *testing.T) {
	p1 := newFailModel("p1", "rate limited")
	p2 := newFailModel("p2", "model overloaded")
	p3 := newOKModel("p3", "hello from p3")

	m := failover.New(p1, p2, p3)
	m.SetRateLimitBackoff(0)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if p1.calls != 2 {
		t.Errorf("p1.calls = %d, want 2 (429-shaped error retries once before escalating)", p1.calls)
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

// okResponse builds a minimal successful *model.LLMResponse carrying text.
func okResponse(text string) *model.LLMResponse {
	return &model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}}
}

// errorPayloadMockLLM simulates an OpenAI-compatible provider that answers
// with HTTP 200 but an embedded {"error": ...} body — surfaced on the ADK
// side as a (nil-error) LLMResponse with ErrorCode/ErrorMessage set.
type errorPayloadMockLLM struct {
	name    string
	code    string
	message string
	calls   int
}

func (m *errorPayloadMockLLM) Name() string { return m.name }

func (m *errorPayloadMockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		yield(&model.LLMResponse{ErrorCode: m.code, ErrorMessage: m.message}, nil)
	}
}

// rateLimitMockLLM fails with a 429-shaped error for the first failCount
// calls, then succeeds with resp (or a default response if resp is nil).
type rateLimitMockLLM struct {
	name      string
	failCount int
	resp      *model.LLMResponse
	calls     int
}

func (m *rateLimitMockLLM) Name() string { return m.name }

func (m *rateLimitMockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		if m.calls <= m.failCount {
			yield(nil, errors.New("429 Too Many Requests"))
			return
		}
		resp := m.resp
		if resp == nil {
			resp = okResponse("ok")
		}
		yield(resp, nil)
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

// TestNilResponsesFiltered verifies that a provider whose entire response is
// (nil, nil) heartbeat sentinels (no real content at all) is treated as an
// empty-response failure (F6) and the chain escalates to the next provider —
// rather than forwarding zero responses as a silent "success". The ADK runner
// does not nil-guard LLMResponse before dereferencing, so the nil entries
// themselves must still never reach the caller.
func TestNilResponsesFiltered(t *testing.T) {
	nilProvider := &nilMockLLM{name: "nil-provider"}
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(nilProvider, backup)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if nilProvider.calls != 1 {
		t.Errorf("nilProvider.calls = %d, want 1", nilProvider.calls)
	}
	if backup.calls != 1 {
		t.Errorf("backup.calls = %d, want 1 — empty response must escalate", backup.calls)
	}
	if len(responses) != 1 || responses[0].Content.Parts[0].Text != "hello from backup" {
		t.Errorf("expected backup response, got %+v", responses)
	}
	if _, fellBack, failed := m.Stats(); !fellBack || len(failed) != 1 || failed[0] != "nil-provider" {
		t.Errorf("Stats() fellBack=%v failed=%v, want fellBack=true failed=[nil-provider]", fellBack, failed)
	}
}

// TestEmptyOnlyChainFails verifies that when every provider in the chain
// returns an all-heartbeat (nil-only) response, the whole call fails with an
// error that mentions the empty response, instead of silently succeeding
// with zero forwarded responses.
func TestEmptyOnlyChainFails(t *testing.T) {
	nilProvider := &nilMockLLM{name: "nil-provider"}
	m := failover.New(nilProvider)

	var finalErr error
	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			finalErr = err
			continue
		}
		responses = append(responses, resp)
	}

	if len(responses) != 0 {
		t.Errorf("got %d responses, want 0", len(responses))
	}
	if finalErr == nil {
		t.Fatal("expected an error for an all-empty chain, got nil")
	}
	if !containsStr(finalErr.Error(), "empty response") {
		t.Errorf("error %q does not mention the empty response", finalErr.Error())
	}
}

// TestValidateResponse_RejectsErrorPayload verifies that a response carrying
// ErrorCode/ErrorMessage (the ADK-side shape of an OpenAI-compatible 200 with
// an {"error": ...} body) is treated as a failure, not a success, and the
// chain escalates to the next provider (F6).
func TestValidateResponse_RejectsErrorPayload(t *testing.T) {
	primary := &errorPayloadMockLLM{name: "primary", code: "content_filter", message: "response withheld"}
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
		t.Errorf("backup.calls = %d, want 1 — error payload must escalate", backup.calls)
	}
	if len(responses) != 1 || responses[0].Content.Parts[0].Text != "hello from backup" {
		t.Errorf("expected backup response, got %+v", responses)
	}
}

// TestRateLimit_RetriesOnceThenSucceeds verifies the F9 behaviour: a 429 on
// the primary triggers exactly one same-provider retry (not an immediate
// escalation to the next provider), and a successful retry keeps the primary
// as the serving provider (fellBack stays false).
func TestRateLimit_RetriesOnceThenSucceeds(t *testing.T) {
	primary := &rateLimitMockLLM{name: "primary", failCount: 1, resp: okResponse("recovered after backoff")}
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(primary, backup)
	m.SetRateLimitBackoff(0) // keep the test fast

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if primary.calls != 2 {
		t.Errorf("primary.calls = %d, want 2 (initial 429 + one retry)", primary.calls)
	}
	if backup.calls != 0 {
		t.Errorf("backup.calls = %d, want 0 — the 429 retry should have succeeded on primary", backup.calls)
	}
	if len(responses) != 1 || responses[0].Content.Parts[0].Text != "recovered after backoff" {
		t.Errorf("expected primary's post-retry response, got %+v", responses)
	}
	if provider, fellBack, _ := m.Stats(); provider != "primary" || fellBack {
		t.Errorf("Stats() = (%q, %v), want (\"primary\", false) — retry-on-same-provider is not a fallback", provider, fellBack)
	}
}

// TestRateLimit_RetryFailsThenEscalates verifies that when the single 429
// retry also fails, the chain moves on to the next provider exactly once
// (no infinite retry loop).
func TestRateLimit_RetryFailsThenEscalates(t *testing.T) {
	primary := &rateLimitMockLLM{name: "primary", failCount: 99} // always 429s
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(primary, backup)
	m.SetRateLimitBackoff(0)

	var responses []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if primary.calls != 2 {
		t.Errorf("primary.calls = %d, want 2 (initial + exactly one retry)", primary.calls)
	}
	if backup.calls != 1 {
		t.Errorf("backup.calls = %d, want 1", backup.calls)
	}
	if len(responses) != 1 || responses[0].Content.Parts[0].Text != "hello from backup" {
		t.Errorf("expected backup response, got %+v", responses)
	}
}

// TestRateLimit_BackoffCancellationSurfacesCause verifies that when the
// context is cancelled while attempt() is sleeping out a 429 backoff, the
// reported error is the cancellation reason, not the stale rate-limit error
// that triggered the backoff in the first place — a caller debugging "why
// did my request fail" should see "context deadline exceeded", not "429 Too
// Many Requests", when the real cause was an unrelated caller-side timeout.
func TestRateLimit_BackoffCancellationSurfacesCause(t *testing.T) {
	primary := &rateLimitMockLLM{name: "primary", failCount: 99} // always 429s
	m := failover.New(primary)                                   // single-provider chain: no fallback to obscure the result
	m.SetRateLimitBackoff(200 * time.Millisecond)                // long enough for the ctx timeout below to fire first

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var gotErr error
	for _, err := range m.GenerateContent(ctx, &model.LLMRequest{}, false) {
		if err != nil {
			gotErr = err
		}
	}

	if gotErr == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(gotErr, context.DeadlineExceeded) {
		t.Errorf("gotErr = %v, want it to wrap context.DeadlineExceeded (the real cancellation cause)", gotErr)
	}
	if strings.Contains(gotErr.Error(), "429 Too Many Requests") {
		t.Errorf("gotErr = %v, should not surface the stale rate-limit error as the reported cause once the backoff wait was cancelled", gotErr)
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

// TestWithStats_ReportsThisCallOnly verifies that a *CallStats passed via
// WithStats reflects exactly the outcome of the call it was attached to.
func TestWithStats_ReportsThisCallOnly(t *testing.T) {
	primary := newFailModel("primary", "503 service unavailable")
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(primary, backup)

	call := &failover.CallStats{}
	ctx := failover.WithStats(context.Background(), call)
	for _, err := range m.GenerateContent(ctx, &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if call.Provider != "backup" {
		t.Errorf("call.Provider = %q, want %q", call.Provider, "backup")
	}
	if !call.FellBack {
		t.Error("call.FellBack = false, want true")
	}
	if len(call.Failed) != 1 || call.Failed[0] != "primary" {
		t.Errorf("call.Failed = %v, want [primary]", call.Failed)
	}
}

// TestWithStats_ConcurrentCallsDoNotCrossContaminate is the regression test
// for the Stats() race: the shared m.last* fields (read via Stats()) are only
// safe for a single-threaded caller, but WithStats gives each concurrent
// GenerateContent call its own *CallStats that must reflect only that call's
// outcome, never another goroutine's. Run with -race to also catch any data
// race on the shared fields this test exercises concurrently.
func TestWithStats_ConcurrentCallsDoNotCrossContaminate(t *testing.T) {
	// Each provider name is unique per goroutine so a cross-contaminated
	// result is trivially detectable: goroutine i must observe "primary-i" or
	// "backup-i", never another index's provider name.
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			primaryName := fmt.Sprintf("primary-%d", i)
			backupName := fmt.Sprintf("backup-%d", i)
			// Odd indices fail on the primary to exercise the fellBack path too.
			var primary model.LLM
			if i%2 == 0 {
				primary = newOKModel(primaryName, "ok")
			} else {
				primary = newFailModel(primaryName, "503 service unavailable")
			}
			backup := newOKModel(backupName, "ok")
			m := failover.New(primary, backup)

			call := &failover.CallStats{}
			ctx := failover.WithStats(context.Background(), call)
			for _, err := range m.GenerateContent(ctx, &model.LLMRequest{}, false) {
				if err != nil {
					t.Errorf("goroutine %d: unexpected error: %v", i, err)
				}
			}

			wantProvider := primaryName
			wantFellBack := false
			if i%2 != 0 {
				wantProvider = backupName
				wantFellBack = true
			}
			if call.Provider != wantProvider {
				t.Errorf("goroutine %d: call.Provider = %q, want %q (cross-contamination if it matches a different index)", i, call.Provider, wantProvider)
			}
			if call.FellBack != wantFellBack {
				t.Errorf("goroutine %d: call.FellBack = %v, want %v", i, call.FellBack, wantFellBack)
			}
		}(i)
	}
	wg.Wait()
}

// ── Streaming pass-through tests ─────────────────────────────────────────────
//
// These lock in the fix for a real live bug: GenerateContent used to discard
// the caller's stream flag and always buffer every provider call as
// non-streaming, which meant a genuine per-provider streaming implementation
// (e.g. oaibridge) never actually delivered incremental output to the TUI.

// streamingMockLLM simulates a real streaming provider. When stream is true,
// it yields one Partial response per entry in partials (in order), then
// either errAfterPartials (if set, no final response) or one final
// non-partial response carrying finalText. When stream is false — matching
// how a real OpenAI-compatible provider behaves under non-streaming mode —
// it never emits any Partial response at all, only the single final one.
type streamingMockLLM struct {
	name             string
	calls            int
	partials         []string
	finalText        string
	errAfterPartials error // if set, yielded after the partials instead of a final response
}

func (m *streamingMockLLM) Name() string { return m.name }

func (m *streamingMockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.calls++
		if !stream {
			yield(&model.LLMResponse{
				Content: &genai.Content{Parts: []*genai.Part{{Text: strings.Join(m.partials, "") + m.finalText}}},
			}, nil)
			return
		}
		for _, p := range m.partials {
			resp := &model.LLMResponse{
				Content: &genai.Content{Parts: []*genai.Part{{Text: p}}},
				Partial: true,
			}
			if !yield(resp, nil) {
				return
			}
		}
		if m.errAfterPartials != nil {
			yield(nil, m.errAfterPartials)
			return
		}
		yield(&model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: m.finalText}}}}, nil)
	}
}

// TestStreamingPartialsForwardedLive verifies that Partial responses reach
// the caller as they arrive, in order, followed by exactly one final
// non-partial response — real incremental delivery, not buffer-then-forward.
func TestStreamingPartialsForwardedLive(t *testing.T) {
	primary := &streamingMockLLM{name: "primary", partials: []string{"Hel", "lo"}, finalText: "Hello"}
	m := failover.New(primary)

	var gotPartials []string
	finals := 0
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, true) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := resp.Content.Parts[0].Text
		if resp.Partial {
			gotPartials = append(gotPartials, text)
		} else {
			finals++
			if text != "Hello" {
				t.Errorf("final text = %q, want %q", text, "Hello")
			}
		}
	}

	want := []string{"Hel", "lo"}
	if len(gotPartials) != len(want) {
		t.Fatalf("got %d partials %v, want %d %v", len(gotPartials), gotPartials, len(want), want)
	}
	for i, w := range want {
		if gotPartials[i] != w {
			t.Errorf("partial[%d] = %q, want %q", i, gotPartials[i], w)
		}
	}
	if finals != 1 {
		t.Errorf("got %d final responses, want 1", finals)
	}
}

// TestStreamingCommittedAttemptSurfacesErrorInsteadOfFailover is the core new
// safety behavior: once a provider has already streamed Partial output to the
// caller, a subsequent failure for that SAME attempt must surface directly —
// not silently retry the same provider or fail over to the next one, which
// would either restart the reply from scratch mid-display or mix two
// providers' text in one message.
func TestStreamingCommittedAttemptSurfacesErrorInsteadOfFailover(t *testing.T) {
	primary := &streamingMockLLM{
		name:             "primary",
		partials:         []string{"Hel"},
		errAfterPartials: errors.New("connection reset"),
	}
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(primary, backup)

	var gotErr error
	partials := 0
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, true) {
		if err != nil {
			gotErr = err
			continue
		}
		if resp.Partial {
			partials++
		}
	}

	if partials != 1 {
		t.Errorf("got %d partials, want 1 (should have streamed before the error)", partials)
	}
	if gotErr == nil {
		t.Fatal("want a surfaced error, got nil")
	}
	if backup.calls != 0 {
		t.Errorf("backup.calls = %d, want 0 — must not fail over after partial output was already shown", backup.calls)
	}
	if !containsStr(gotErr.Error(), "connection reset") {
		t.Errorf("error %q should mention the real cause directly, not a generic all-providers-failed summary", gotErr.Error())
	}
}

// TestStreamingFailsBeforeAnyPartialStillFailsOver confirms clean failover is
// fully preserved for the common case: a provider that fails BEFORE its
// first Partial response reaches the caller has shown nothing yet, so the
// next provider is tried exactly as before the streaming change.
func TestStreamingFailsBeforeAnyPartialStillFailsOver(t *testing.T) {
	primary := &streamingMockLLM{name: "primary", errAfterPartials: errors.New("connection refused")}
	backup := newOKModel("backup", "hello from backup")
	m := failover.New(primary, backup)

	var texts []string
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, true) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		texts = append(texts, resp.Content.Parts[0].Text)
	}

	if backup.calls != 1 {
		t.Errorf("backup.calls = %d, want 1 — a pre-first-token failure must still fail over cleanly", backup.calls)
	}
	if len(texts) != 1 || texts[0] != "hello from backup" {
		t.Errorf("got %v, want [\"hello from backup\"]", texts)
	}
}

// TestStreamingMockNonStreamingModeYieldsNoPartials is a sanity check on the
// mock itself and on stream=false behavior with a provider capable of
// streaming: stream=false must still yield exactly one non-partial response.
func TestStreamingMockNonStreamingModeYieldsNoPartials(t *testing.T) {
	primary := &streamingMockLLM{name: "primary", partials: []string{"a", "b"}, finalText: "c"}
	m := failover.New(primary)

	partials := 0
	finals := 0
	for resp, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Partial {
			partials++
		} else {
			finals++
		}
	}
	if partials != 0 {
		t.Errorf("got %d partials in non-streaming mode, want 0", partials)
	}
	if finals != 1 {
		t.Errorf("got %d finals, want 1", finals)
	}
}
