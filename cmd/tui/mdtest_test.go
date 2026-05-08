package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// makeTestStyles returns a styledSet for theme index 0 (Catppuccin) for use
// in tests.  themeIdx is set explicitly so glamourStyleName resolves correctly.
func makeTestStyles(themeIdx int) styledSet {
	s := makeStyles(builtinThemes[themeIdx])
	s.themeIdx = themeIdx
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

	for themeIdx := range builtinThemes {
		s := makeTestStyles(themeIdx)
		base := testBaseStyle()
		for _, tc := range inputs {
			t.Run(builtinThemes[themeIdx].name+"/"+tc.name, func(t *testing.T) {
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

	for themeIdx := range builtinThemes {
		s := makeTestStyles(themeIdx)
		out := renderMarkdown(s, md, 80, base)
		if !strings.ContainsRune(out, '\x1b') {
			t.Errorf("theme %d (%s): renderMarkdown produced no ANSI escape codes — check glamour config",
				themeIdx, builtinThemes[themeIdx].name)
		}
	}
}
