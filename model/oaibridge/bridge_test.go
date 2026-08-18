package oaibridge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// TestGenerateContent_StreamsIncrementalDeltasThenOneFinalEvent exercises the
// real streaming path against a fake OpenAI-compatible SSE server (not
// mocked at the Go interface level) — this is the fix for oaibridge
// previously blocking until the full reply was generated before delivering
// it as a single chunk, which made every OpenAI-compatible provider
// (including opencode) feel unresponsive compared to Gemini's real
// incremental streaming.
func TestGenerateContent_StreamsIncrementalDeltasThenOneFinalEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}
		chunks := []string{"Sure", ", let's", " do it."}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":\"\"}]}\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	ctx := context.Background()
	m, err := NewModel(ctx, Config{
		Provider:  "test",
		BaseURL:   srv.URL,
		APIKey:    "fake-key",
		ModelName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
	}

	var partials []string
	finals := 0
	var finalText string
	for resp, err := range m.GenerateContent(ctx, req, true) {
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		if resp.Content == nil {
			continue
		}
		var text string
		for _, p := range resp.Content.Parts {
			text += p.Text
		}
		if resp.Partial {
			partials = append(partials, text)
		} else {
			finals++
			finalText = text
		}
	}

	wantPartials := []string{"Sure", ", let's", " do it."}
	if len(partials) != len(wantPartials) {
		t.Fatalf("got %d partial chunks %v, want %d %v", len(partials), partials, len(wantPartials), wantPartials)
	}
	for i, want := range wantPartials {
		if partials[i] != want {
			t.Errorf("partial[%d] = %q, want %q", i, partials[i], want)
		}
	}
	if finals != 1 {
		t.Fatalf("got %d final (non-partial) events, want exactly 1", finals)
	}
	if finalText != "Sure, let's do it." {
		t.Errorf("final event text = %q, want full concatenated text", finalText)
	}
}

// TestGenerateContent_NonStreamingUnaffected confirms stream=false still
// yields exactly one response with no Partial event — the pre-existing
// behavior must be provably unchanged by the streaming addition.
func TestGenerateContent_NonStreamingUnaffected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":1,"model":"test-model",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"hello there"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	m, err := NewModel(ctx, Config{
		Provider:  "test",
		BaseURL:   srv.URL,
		APIKey:    "fake-key",
		ModelName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
	}

	count := 0
	var gotText string
	for resp, err := range m.GenerateContent(ctx, req, false) {
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		count++
		if resp.Partial {
			t.Errorf("non-streaming response should never be Partial")
		}
		for _, p := range resp.Content.Parts {
			gotText += p.Text
		}
	}
	if count != 1 {
		t.Fatalf("got %d yielded responses, want exactly 1 for non-streaming", count)
	}
	if gotText != "hello there" {
		t.Errorf("text = %q, want %q", gotText, "hello there")
	}
}

// TestGenerateContent_DeadlineExceededSurfacesInsteadOfSwallowed is the fix
// for a real live bug: a caller-supplied context whose deadline expires
// mid-call (e.g. failover's per-attempt timeout) used to be silently
// swallowed — GenerateContent yielded nothing at all, which upstream
// (failover.validateResponse) reported as a misleading "empty response:
// provider returned no content" instead of the true timeout cause. Only an
// explicitly *cancelled* context (the caller genuinely stopped listening)
// should yield nothing; a deadline expiry must still be reported.
func TestGenerateContent_DeadlineExceededSurfacesInsteadOfSwallowed(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never responds until the test unblocks it, after the deadline fires
	}))
	defer srv.Close()
	defer close(block)

	m, err := NewModel(context.Background(), Config{
		Provider:  "test",
		BaseURL:   srv.URL,
		APIKey:    "fake-key",
		ModelName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	var gotErr error
	yielded := 0
	for _, err := range m.GenerateContent(ctx, req, false) {
		yielded++
		gotErr = err
	}

	if yielded != 1 {
		t.Fatalf("got %d yields, want exactly 1 (the deadline error) — a deadline expiry must not be silently swallowed", yielded)
	}
	if gotErr == nil {
		t.Fatal("got nil error, want the deadline-exceeded error surfaced")
	}
}

// TestGenerateContent_ExplicitCancelYieldsNothing confirms the original
// intent of the swallow guard is preserved: when the CALLER explicitly
// cancels (not a deadline expiring), there is truly nothing left to yield to,
// so GenerateContent correctly yields nothing.
func TestGenerateContent_ExplicitCancelYieldsNothing(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	m, err := NewModel(context.Background(), Config{
		Provider:  "test",
		BaseURL:   srv.URL,
		APIKey:    "fake-key",
		ModelName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	yielded := 0
	for range m.GenerateContent(ctx, req, false) {
		yielded++
	}

	if yielded != 0 {
		t.Errorf("got %d yields, want 0 — an explicitly cancelled context has no listener left", yielded)
	}
}
