package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"go-adk-q/cmd/tui/layout"
	"go-adk-q/cmd/tui/theme"
	"go-adk-q/model/catalog"
)

// makeTestStyles returns a theme.StyledSet for theme index 0 (Catppuccin) for use
// in tests.  themeIdx is set explicitly so glamourStyleName resolves correctly.
func makeTestStyles(themeIdx int) theme.StyledSet {
	s := theme.MakeStyles(theme.BuiltinThemes[themeIdx])
	s.ThemeIdx = themeIdx
	return s
}

// baseStyle returns a plain lipgloss style suitable for the fallback path.
func testBaseStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
}

// TestRenderMarkdownNonEmpty checks that renderMarkdown returns non-empty output
// for typical LLM responses across all themes.
func TestRenderMarkdownNonEmpty(t *testing.T) {
	inputs := []struct {
		name string
		md   string
	}{
		{"prose", "Hello, world!"},
		{"bold", "This is **bold** text."},
		{"heading", "## Installation\n\nRun `go get`."},
		{"code block", "```go\nfunc main() {}\n```"},
		{"list", "- item one\n- item two\n- item three"},
		{"mixed", "## Title\n\nSome **bold** text.\n\n```go\nfmt.Println(\"hi\")\n```"},
	}

	for themeIdx := range theme.BuiltinThemes {
		s := makeTestStyles(themeIdx)
		base := testBaseStyle()
		for _, tc := range inputs {
			t.Run(theme.BuiltinThemes[themeIdx].Name+"/"+tc.name, func(t *testing.T) {
				out := renderMarkdown(s, tc.md, 80, base)
				if strings.TrimSpace(out) == "" {
					t.Errorf("renderMarkdown(%q, theme=%d): got empty output", tc.md, themeIdx)
				}
			})
		}
	}
}

// TestRenderMarkdownEmpty verifies empty input returns empty output.
func TestRenderMarkdownEmpty(t *testing.T) {
	s := makeTestStyles(0)
	base := testBaseStyle()
	out := renderMarkdown(s, "", 80, base)
	if out != "" {
		t.Errorf("renderMarkdown(\"\", 80): want empty, got %q", out)
	}
}

// TestRenderMarkdownNarrow verifies narrow terminals fall back gracefully
// without crashing.
func TestRenderMarkdownNarrow(t *testing.T) {
	s := makeTestStyles(0)
	base := testBaseStyle()
	out := renderMarkdown(s, "hello **world**", 10, base)
	if out == "" {
		t.Error("renderMarkdown narrow: want non-empty fallback output, got empty")
	}
}

// TestRenderMarkdownNoTrailingBlanks checks that the output ends with exactly
// one newline (no double-blank padding that would break viewport spacing).
func TestRenderMarkdownNoTrailingBlanks(t *testing.T) {
	s := makeTestStyles(0)
	base := testBaseStyle()
	cases := []string{
		"Hello.",
		"## Heading",
		"```go\nfmt.Println()\n```",
		"- a\n- b\n- c",
	}
	for _, md := range cases {
		out := renderMarkdown(s, md, 80, base)
		if out == "" {
			continue // empty is fine for empty input
		}
		if !strings.HasSuffix(out, "\n") {
			t.Errorf("renderMarkdown(%q): output does not end with newline", md)
		}
		if strings.HasSuffix(out, "\n\n") {
			t.Errorf("renderMarkdown(%q): output ends with double newline", md)
		}
	}
}

// TestParseSegmentsCodeFence verifies parseSegments correctly identifies fenced
// code blocks — used by smartCopy for clipboard extraction.
func TestParseSegmentsCodeFence(t *testing.T) {
	input := "Some prose.\n```go\nfunc main() {}\n```\nMore prose."
	segs := parseSegments(input)

	var codeSegs []textSegment
	for _, s := range segs {
		if s.code {
			codeSegs = append(codeSegs, s)
		}
	}
	if len(codeSegs) != 1 {
		t.Fatalf("parseSegments: want 1 code segment, got %d", len(codeSegs))
	}
	if codeSegs[0].lang != "go" {
		t.Errorf("code segment lang: want %q, got %q", "go", codeSegs[0].lang)
	}
	if !strings.Contains(codeSegs[0].body, "func main") {
		t.Errorf("code segment body: want 'func main', got %q", codeSegs[0].body)
	}
}

// TestParseSegmentsUnclosedFence verifies streaming-safe behaviour: an unclosed
// fence at EOF is emitted as a code segment (not lost).
func TestParseSegmentsUnclosedFence(t *testing.T) {
	input := "Prose.\n```python\ndef foo():"
	segs := parseSegments(input)

	var codeSegs []textSegment
	for _, s := range segs {
		if s.code {
			codeSegs = append(codeSegs, s)
		}
	}
	if len(codeSegs) != 1 {
		t.Fatalf("parseSegments unclosed: want 1 code segment, got %d", len(codeSegs))
	}
	if codeSegs[0].lang != "python" {
		t.Errorf("unclosed code segment lang: want %q, got %q", "python", codeSegs[0].lang)
	}
}

// TestParseSegmentsNone verifies plain prose produces no code segments.
func TestParseSegmentsNone(t *testing.T) {
	input := "Just plain text with no fences."
	segs := parseSegments(input)
	for _, s := range segs {
		if s.code {
			t.Errorf("parseSegments: unexpected code segment in plain text: %+v", s)
		}
	}
}

// stripANSI removes ANSI escape codes (crude but sufficient for test assertions).
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// TestRenderMarkdownContainsANSI verifies glamour produces real ANSI colour
// codes (not plain text fallback) for all 5 themes.
func TestRenderMarkdownContainsANSI(t *testing.T) {
	// Use a rich input to ensure headings, bold, and code are exercised.
	md := "## Heading\n\nSome **bold** text and `inline code`.\n\n```go\nfmt.Println(\"hi\")\n```\n"
	base := testBaseStyle()

	for themeIdx := range theme.BuiltinThemes {
		s := makeTestStyles(themeIdx)
		out := renderMarkdown(s, md, 80, base)
		if !strings.ContainsRune(out, '\x1b') {
			t.Errorf("theme %d (%s): renderMarkdown produced no ANSI escape codes — check glamour config",
				themeIdx, theme.BuiltinThemes[themeIdx].Name)
		}
	}
}

// TestRenderMarkdownFencedMarkdownRendersAsHeading is the regression test for
// the bug documented in SESSION_HANDOFF.md: an LLM wrapping a whole markdown
// document in a ```md/```markdown fence must have block-level markdown
// (headings, lists, bold) actually rendered through glamour — not boxed as
// Chroma-highlighted code — because Chroma's markdown lexer only does inline
// tokenization, not block-level rendering.
//
// Note: this codebase's own glamour style config deliberately keeps the
// literal "## " prefix on real headings too (see H2.Prefix in
// glamourStyleConfig), so "no literal ##" is NOT the right signal — a
// correctly-rendered fenced heading looks the same as a normal heading,
// hash prefix included. The actual bug signal is *which renderer* handles
// the segment: this asserts the fenced version renders identically to the
// same content as plain (unfenced) prose, and differently from what the
// old Chroma code-block path would have produced.
func TestRenderMarkdownFencedMarkdownRendersAsHeading(t *testing.T) {
	base := testBaseStyle()
	langs := []string{"md", "markdown"}
	body := "## Heading\n\nSome prose."

	for _, lang := range langs {
		for themeIdx := range theme.BuiltinThemes {
			s := makeTestStyles(themeIdx)

			fenced := "```" + lang + "\n" + body + "\n```"
			gotFenced := renderMarkdown(s, fenced, 80, base)
			wantProse := renderMarkdown(s, body, 80, base)
			gotOldCodeBlockPath := renderCodeBlock(s, lang, body, 80)

			if gotFenced != wantProse {
				t.Errorf("lang=%s theme=%s: fenced markdown did not render like plain prose (still routed through renderCodeBlock?)\ngot:  %q\nwant: %q",
					lang, theme.BuiltinThemes[themeIdx].Name, gotFenced, wantProse)
			}
			if gotFenced == gotOldCodeBlockPath {
				t.Errorf("lang=%s theme=%s: fenced markdown matches the old Chroma code-block rendering — routing fix did not take effect",
					lang, theme.BuiltinThemes[themeIdx].Name)
			}
			if !strings.Contains(stripANSI(gotFenced), "Heading") {
				t.Errorf("lang=%s theme=%s: expected heading text 'Heading' in output, got:\n%s",
					lang, theme.BuiltinThemes[themeIdx].Name, stripANSI(gotFenced))
			}
		}
	}
}

func TestParseProviderIDs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{
			input: "github-models",
			want:  []string{"github-models"},
		},
		{
			input: "failover(github-models → groq → nvidia)",
			want:  []string{"github-models", "groq", "nvidia"},
		},
		{
			input: "failover(openrouter → huggingface)",
			want:  []string{"openrouter", "huggingface"},
		},
		{
			input: "groq",
			want:  []string{"groq"},
		},
	}
	for _, tc := range cases {
		got := parseProviderIDs(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseProviderIDs(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseProviderIDs(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestModelPickerFiltering(t *testing.T) {
	// With a failover chain name, only matching providers should appear.
	chainName := "failover(github-models/gpt-4o → groq/llama-3.1-8b-instant)"
	ids := parseProviderIDs(chainName)
	// Should be ["github-models/gpt-4o", "groq/llama-3.1-8b-instant"]
	if len(ids) != 2 {
		t.Fatalf("parseProviderIDs got %v, want 2 entries", ids)
	}

	picker := newModelPickerState(ids, "github-models/Llama-4-Scout-17B-16E-Instruct")
	if len(picker.providers) == 0 {
		t.Fatal("picker.providers is empty — catalog not registered or filtering too aggressive")
	}
	// Every visible provider must match at least one active ID substring.
	for _, p := range picker.providers {
		found := false
		for _, id := range ids {
			if strings.Contains(id, p.Provider) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("provider %q shown but not in active IDs %v", p.Provider, ids)
		}
	}
}

func TestModelPickerPreSelectsActiveModel(t *testing.T) {
	// Simulate: only GitHub Models key is set; active model is Scout (not the default Maverick).
	ids := parseProviderIDs("github-models/Llama-4-Scout-17B-16E-Instruct")
	activeModel := "github-models/Llama-4-Scout-17B-16E-Instruct"

	picker := newModelPickerState(ids, activeModel)

	// With 1 provider, picker auto-advances to model stage.
	if picker.stage != pickerStageModel {
		t.Fatalf("expected pickerStageModel (single provider auto-advance), got %v", picker.stage)
	}
	if len(picker.models) == 0 {
		t.Fatal("picker.models is empty")
	}
	selectedID := picker.models[picker.modelIdx].ID
	// Must be Scout, not the Default (Maverick).
	if selectedID != "Llama-4-Scout-17B-16E-Instruct" {
		t.Errorf("pre-selected model ID = %q, want Llama-4-Scout-17B-16E-Instruct (default Maverick would be wrong)", selectedID)
	}
}

func TestActiveModelIdxIn(t *testing.T) {
	models := []struct{ id string }{
		{"Llama-4-Maverick-17B-128E-Instruct-FP8"},
		{"Llama-4-Scout-17B-16E-Instruct"},
		{"gpt-4o"},
	}
	entries := make([]catalog.ModelEntry, len(models))
	for i, m := range models {
		entries[i] = catalog.ModelEntry{ID: m.id, Default: i == 0}
	}

	cases := []struct {
		activeID string
		wantIdx  int
	}{
		{"Llama-4-Scout-17B-16E-Instruct", 1},
		{"github-models/Llama-4-Scout-17B-16E-Instruct", 1}, // with provider prefix
		{"gpt-4o", 2},
		{"unknown-model", 0}, // falls back to Default (index 0)
	}
	for _, tc := range cases {
		got := activeModelIdxIn(entries, tc.activeID)
		if got != tc.wantIdx {
			t.Errorf("activeModelIdxIn(entries, %q) = %d, want %d", tc.activeID, got, tc.wantIdx)
		}
	}
}

// TestCalcInputHeight verifies that the textarea grows to accommodate long
// text instead of scrolling horizontally.
//
// Layout constants (must stay in sync with chat.go):
//
//	SetWidth(terminalWidth - 6)  →  internal text width = terminalWidth - 8
//	layout.CalcInputHeight(text, terminalWidth-8, maxH)
func TestCalcInputHeight(t *testing.T) {
	const termW = 160
	const effectiveW = termW - 6 // = 154 (no prompt; SetWidth(termW-6), prompt="")

	cases := []struct {
		desc  string
		text  string
		wantH int
	}{
		{"empty", "", 3},
		{"short — fits on one line", "hello world", 3},
		{"exactly one line", strings.Repeat("x", effectiveW), 3},
		{"one char over — must wrap to 2 but min=3", strings.Repeat("x", effectiveW+1), 3},
		{"double width — exactly 2 lines but min=3", strings.Repeat("x", effectiveW*2), 3},
		{"double+1 — must be 3", strings.Repeat("x", effectiveW*2+1), 3},
		{"triple+1 — must be 4", strings.Repeat("x", effectiveW*3+1), 4},
		{"capped at maxH=5", strings.Repeat("x", effectiveW*10), 5},
	}
	for _, tc := range cases {
		got := layout.CalcInputHeight(tc.text, effectiveW, 5)
		if got != tc.wantH {
			t.Errorf("%s: layout.CalcInputHeight(%d chars, %d) = %d, want %d",
				tc.desc, len(tc.text), effectiveW, got, tc.wantH)
		}
	}
}

// newFooterTestModel builds a minimal chatModel sufficient to exercise
// footerView without a real runner/session (neither is read by footerView).
func newFooterTestModel(modelName, routeProvider string, fellBack bool) chatModel {
	ti := textarea.New()
	return chatModel{
		textInput:     ti,
		viewport:      viewport.New(80, 24),
		width:         120,
		modelName:     modelName,
		routeProvider: routeProvider,
		routeFellBack: fellBack,
	}
}

// TestFooterViewRouteProviderBadge guards against the footer showing the
// same provider name twice (e.g. "opencode/deepseek-v4-flash-free ...
// ⚡ opencode/deepseek-v4-flash-free") when the route provider that served
// the turn is the same as the configured/displayed model — the common
// no-failover case. It also confirms a genuinely different fallback
// provider name still renders.
func TestFooterViewRouteProviderBadge(t *testing.T) {
	s := makeTestStyles(0)

	t.Run("no fallback: provider name shown once, not duplicated", func(t *testing.T) {
		m := newFooterTestModel("opencode/deepseek-v4-flash-free", "opencode/deepseek-v4-flash-free", false)
		got := m.footerView(s)
		count := strings.Count(got, "opencode/deepseek-v4-flash-free")
		if count != 1 {
			t.Errorf("footer contains provider name %d times, want 1 (no duplicate); footer:\n%s", count, got)
		}
		if !strings.Contains(got, "⚡") {
			t.Errorf("footer missing direct-serve icon ⚡; footer:\n%s", got)
		}
	})

	t.Run("fell back: differing provider name still shown", func(t *testing.T) {
		m := newFooterTestModel("opencode/deepseek-v4-flash-free", "groq/llama-3.3-70b", true)
		got := m.footerView(s)
		if !strings.Contains(got, "opencode/deepseek-v4-flash-free") {
			t.Errorf("footer missing configured model name; footer:\n%s", got)
		}
		if !strings.Contains(got, "groq/llama-3.3-70b") {
			t.Errorf("footer missing fallback provider name; footer:\n%s", got)
		}
		if !strings.Contains(got, "🔀") {
			t.Errorf("footer missing fallback icon 🔀; footer:\n%s", got)
		}
	})
}
