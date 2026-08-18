package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

// newRenderCacheTestModel builds a minimal chatModel sufficient to exercise
// renderMessages/renderCompletedMessages without a real runner/session.
func newRenderCacheTestModel(msgs []chatMsg) chatModel {
	return chatModel{
		textInput: textarea.New(),
		viewport:  viewport.New(80, 24),
		width:     100,
		msgs:      msgs,
	}
}

// TestRenderMessages_CachesCompletedPortionAcrossStreamingChunks guards
// against the regression this session found: renderMessages used to re-render
// every completed message (glamour parse + border wrap) from scratch on
// every single streaming chunk tick, an O(n)-per-token cost that made agent
// replies feel progressively later as a conversation grew. The completed
// portion must now be computed once and reused unless the message count,
// width, or theme actually changes.
func TestRenderMessages_CachesCompletedPortionAcrossStreamingChunks(t *testing.T) {
	s := makeTestStyles(0)
	m := newRenderCacheTestModel([]chatMsg{
		{role: "user", text: "hello"},
		{role: "agent", text: "hi there, how can I help?"},
	})

	// First render (no streaming) populates the cache.
	first := m.renderMessages(s)
	if m.completedRenderCacheCount != len(m.msgs) {
		t.Fatalf("cache count = %d, want %d after first render", m.completedRenderCacheCount, len(m.msgs))
	}
	cachedAfterFirst := m.completedRenderCache
	if cachedAfterFirst == "" {
		t.Fatal("completedRenderCache is empty after first render")
	}
	if first == "" {
		t.Fatal("renderMessages returned empty output")
	}

	// glamour/lipgloss wrap individual words in ANSI escapes and pad lines
	// with styled spaces, so comparisons below strip escapes and collapse
	// whitespace rather than doing an exact substring match.
	collapse := func(s string) string { return strings.Join(strings.Fields(stripANSI(s)), " ") }

	// Simulate streaming: message count is unchanged, only streamingText
	// grows across chunks. The cached completed-portion string must be
	// byte-identical across every chunk tick — proving the completed
	// messages were not re-rendered — while the overall output still grows
	// to include the new streaming tail each time.
	m.loading = true
	chunks := []string{"Sure", "Sure, ", "Sure, let's", "Sure, let's do it."}
	for _, streamed := range chunks {
		m.streamingText = streamed
		out := m.renderMessages(s)

		if m.completedRenderCache != cachedAfterFirst {
			t.Fatalf("completedRenderCache changed mid-stream (message count unchanged): cache was re-rendered instead of reused")
		}
		if !strings.HasPrefix(out, cachedAfterFirst) {
			t.Fatalf("streamed render does not start with the cached completed portion")
		}
		if !strings.Contains(collapse(out), collapse(streamed)) {
			t.Fatalf("streamed render missing current chunk text %q\nrendered:\n%s", streamed, out)
		}
	}

	// A genuinely new completed message must invalidate the cache.
	m.loading = false
	m.streamingText = ""
	m.msgs = append(m.msgs, chatMsg{role: "agent", text: "Sure, let's do it."})
	out := m.renderMessages(s)
	if m.completedRenderCacheCount != len(m.msgs) {
		t.Fatalf("cache count = %d, want %d after appending a new completed message", m.completedRenderCacheCount, len(m.msgs))
	}
	if m.completedRenderCache == cachedAfterFirst {
		t.Fatal("completedRenderCache did not update after a new message was appended")
	}
	if !strings.Contains(collapse(out), "Sure, let's do it.") {
		t.Fatal("final render missing the newly appended message content")
	}
}

// TestRenderMessages_MatchesUncachedOutput confirms the cached code path
// produces byte-identical output to a from-scratch render of the same
// messages — the caching change must be purely a performance fix, not a
// rendering behavior change.
func TestRenderMessages_MatchesUncachedOutput(t *testing.T) {
	s := makeTestStyles(0)
	msgs := []chatMsg{
		{role: "user", text: "explain this code"},
		{role: "agent", text: "## Heading\n\nSome **bold** text and a list:\n\n- one\n- two"},
		{role: "error", text: "boom: connection refused"},
	}

	m1 := newRenderCacheTestModel(msgs)
	direct := renderCompletedMessages(msgs, s, m1.width-4)

	m2 := newRenderCacheTestModel(msgs)
	cached := m2.renderMessages(s)

	if cached != direct {
		t.Fatalf("cached renderMessages output diverges from renderCompletedMessages:\ncached:\n%s\ndirect:\n%s", cached, direct)
	}
}
