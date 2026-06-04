package main

// markdown.go — Markdown rendering for the chat viewport.
//
// Architecture (two-pass hybrid):
//  1. parseSegments splits the raw markdown into alternating prose and
//     fenced-code-block segments.
//  2. Prose segments  → glamour (headings, bold, tables, lists, blockquotes, …)
//  3. Code segments   → renderCodeBlock (Chroma syntax highlight inside a
//     visible full-width background box, like pi's own code display)
//
// Why a custom code-block renderer instead of glamour's built-in one?
//  • glamour renders code with Chroma colours but does NOT extend the
//    background to the full terminal width, so the code block is invisible
//    against the dark terminal background.
//  • Our renderCodeBlock fills every line to contentW with the code-block
//    background, giving an obvious visual box identical to pi's style.
//
// ANSI persistence across Chroma token boundaries:
//  Chroma inserts \x1b[0m resets between tokens.  Those resets clear our
//  background.  We strip Chroma's own background sequences and re-inject
//  our background after every reset, so the box background is unbroken.

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	chromaQuick "github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func uintPtr(u uint) *uint    { return &u }

// hexToANSIBg converts a "#rrggbb" hex colour to a 24-bit ANSI background
// escape sequence: \x1b[48;2;r;g;bm
func hexToANSIBg(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 32)
	g, _ := strconv.ParseInt(hex[2:4], 16, 32)
	b, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// bgStripRe matches any ANSI 48;2 (24-bit background) sequence so we can
// strip Chroma's own background colours and substitute our own.
var bgStripRe = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*48;2;[0-9]+;[0-9]+;[0-9]+(?:;[0-9]+)*m`)

// ── Per-theme code-block palette ──────────────────────────────────────────────

type codeBlockPalette struct {
	codeBg     string // #rrggbb — code area background (fills to full width)
	headerBg   string // #rrggbb — language label bar background
	headerFg   string // #rrggbb — language label bar foreground
	chromaStyle string // Chroma style name for syntax highlighting
}

// codeBlockPalettes is indexed by builtinThemes index (0 = Catppuccin, etc.)
var codeBlockPalettes = []codeBlockPalette{
	// 0: Catppuccin Mocha
	{"#11111b", "#313244", "#89b4fa", "monokai"},
	// 1: Tokyo Night
	{"#16161e", "#24283b", "#7aa2f7", "monokai"},
	// 2: Rosé Pine
	{"#191724", "#2a2837", "#9ccfd8", "monokai"},
	// 3: Nord
	{"#242933", "#3b4252", "#88c0d0", "nord"},
	// 4: Gruvbox
	{"#1d2021", "#3c3836", "#fabd2f", "monokai"},
	// 5: GitHub Light
	{"#f6f8fa", "#e1e4e8", "#24292e", "friendly"},
	// 6: Solarized Light
	{"#fdf6e3", "#eee8d5", "#268bd2", "friendly"},
	// 7: Dracula (fallback)
	{"#1e1f29", "#2d2f3f", "#bd93f9", "monokai"},
}

func getCodePalette(themeIdx int) codeBlockPalette {
	if themeIdx >= 0 && themeIdx < len(codeBlockPalettes) {
		return codeBlockPalettes[themeIdx]
	}
	return codeBlockPalettes[0]
}

// ── Code block renderer ───────────────────────────────────────────────────────

// renderCodeBlock renders a single fenced code block as a full-width box:
//
//	  ▸ go  ──────────────────────────────────────── (header bar)
//	  package main                                   (code lines, bg fills width)
//	  …                                              (bottom bar)
//
// Every code line's background extends to contentW so the box is clearly
// visible regardless of how long the code is.
func renderCodeBlock(s styledSet, lang, body string, contentW int) string {
	pal := getCodePalette(s.themeIdx)

	langDisplay := strings.TrimSpace(lang)
	if langDisplay == "" {
		langDisplay = "code"
	}

	// ── Header bar ────────────────────────────────────────────────────────
	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(pal.headerBg)).
		Foreground(lipgloss.Color(pal.headerFg)).
		Bold(true)
	header := headerStyle.Width(contentW).Render("  ▸ " + langDisplay)

	// ── Bottom separator ──────────────────────────────────────────────────
	// Thin bottom accent: ▁ characters in header colour, no fill background.
	// This gives a clean underline without the heavy full-height bar.
	bottom := lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.headerBg)).
		Render(strings.Repeat("▁", contentW))

	// ── Syntax highlight with Chroma ──────────────────────────────────────
	codeBgANSI := hexToANSIBg(pal.codeBg)
	const reset = "\x1b[0m"

	var chromaBuf bytes.Buffer
	err := chromaQuick.Highlight(&chromaBuf, body+"\n", lang, "terminal16m", pal.chromaStyle)

	var highlightedLines []string
	if err != nil || chromaBuf.Len() == 0 {
		// Fallback: plain body lines with no colour.
		highlightedLines = strings.Split(body, "\n")
	} else {
		h := chromaBuf.String()
		// 1. Strip any background sequences Chroma emitted (we replace them).
		h = bgStripRe.ReplaceAllString(h, "")
		// 2. After every ANSI reset, re-inject our background so it persists
		//    across token boundaries (Chroma resets between tokens).
		h = strings.ReplaceAll(h, reset, reset+codeBgANSI)
		// 3. Trim the trailing newline Chroma always appends.
		h = strings.TrimRight(h, "\n")
		highlightedLines = strings.Split(h, "\n")
	}

	// ── Build the box ─────────────────────────────────────────────────────
	var sb strings.Builder
	sb.WriteString(header + "\n")

	for _, line := range highlightedLines {
		// Visible width (ANSI-stripped).
		lw := lipgloss.Width(line)
		// Padding: fill remaining columns after 2-char left indent + line content.
		// Reserve 2 chars on the right for a small margin.
		pad := contentW - 2 - lw - 2
		if pad < 0 {
			pad = 0
		}
		// [bg][2-space indent][colored code][right-fill spaces][reset]
		sb.WriteString(codeBgANSI)
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString(strings.Repeat(" ", pad+2))
		sb.WriteString(reset)
		sb.WriteByte('\n')
	}

	sb.WriteString(bottom + "\n")
	return sb.String()
}

// ── Prose renderer (glamour) ──────────────────────────────────────────────────

// renderProse passes non-code markdown prose through glamour.
// Used by the hybrid renderMarkdown path when there are code segments mixed
// with prose.
func renderProse(s styledSet, prose string, contentW int, baseStyle lipgloss.Style) string {
	prose = strings.TrimSpace(prose)
	if prose == "" {
		return ""
	}
	r := cachedRenderer(s.themeIdx, contentW)
	if r == nil {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(prose) + "\n"
	}
	rendered, err := r.Render(prose)
	if err != nil || strings.TrimSpace(rendered) == "" {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(prose) + "\n"
	}
	return strings.TrimRight(rendered, "\n") + "\n"
}

// ── Top-level hybrid renderer ─────────────────────────────────────────────────

// renderMarkdown is the single entry point for rendering an LLM response.
//
//   - Pure prose (no code blocks): fast-path directly through glamour.
//   - Mixed prose + code: hybrid — prose through glamour, code through
//     renderCodeBlock (Chroma + full-width background box).
//
// Streaming safety: if the text has an unclosed ``` fence (partial stream),
// a closing ``` is appended before parsing so glamour/Chroma see a complete
// document.
func renderMarkdown(s styledSet, text string, contentW int, baseStyle lipgloss.Style) string {
	if text == "" {
		return ""
	}
	if contentW < 12 {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}

	// Close unclosed fences for streaming safety.
	textToRender := text
	if countFences(text)%2 != 0 {
		textToRender = text + "\n```"
	}

	segments := parseSegments(textToRender)

	// Fast-path: no code blocks — full text through glamour unchanged.
	hasCode := false
	for _, seg := range segments {
		if seg.code {
			hasCode = true
			break
		}
	}
	if !hasCode {
		r := cachedRenderer(s.themeIdx, contentW)
		if r != nil {
			rendered, err := r.Render(textToRender)
			if err == nil && strings.TrimSpace(rendered) != "" {
				return strings.TrimRight(rendered, "\n") + "\n"
			}
		}
		// Glamour failed — plain fallback.
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}

	// Hybrid path: prose → glamour, code → renderCodeBlock.
	var out strings.Builder
	for _, seg := range segments {
		if seg.code {
			out.WriteString(renderCodeBlock(s, seg.lang, seg.body, contentW))
		} else {
			out.WriteString(renderProse(s, seg.body, contentW, baseStyle))
		}
	}

	result := strings.TrimRight(out.String(), "\n")
	if result == "" {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}
	return result + "\n"
}

// countFences counts ``` fence-delimiter lines (odd = unclosed fence).
func countFences(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			n++
		}
	}
	return n
}

// ── Per-theme glamour StyleConfig ─────────────────────────────────────────────

func glamourStyleConfig(themeIdx int) ansi.StyleConfig {
	switch themeIdx {

	case 0: // ── Catppuccin Mocha ─────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#cdd6f4")},
				Margin:         uintPtr(0),
			},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#cdd6f4")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#a6adc8"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#cba6f7"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#f38ba8"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#89b4fa"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#45475a"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#89dceb")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#89b4fa"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#cba6f7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#a6e3a1"),
					BackgroundColor: strPtr("#313244"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "monokai",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#cdd6f4")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 1: // ── Tokyo Night ───────────────────────────────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#565f89"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#bb9af7"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#f7768e"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#7aa2f7"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#3b4261"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#73daca")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#7aa2f7"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#bb9af7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#9ece6a"),
					BackgroundColor: strPtr("#24283b"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "monokai",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 2: // ── Rosé Pine ────────────────────────────────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#6e6a86"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c4a7e7"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#eb6f92"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#9ccfd8"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#403d52"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#31748f")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#9ccfd8"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#c4a7e7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#f6c177"),
					BackgroundColor: strPtr("#26233a"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "monokai",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 3: // ── Nord ──────────────────────────────────────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#616e88"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#81a1c1"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#ebcb8b"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#88c0d0"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#3b4252"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#8fbcbb")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#81a1c1"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#88c0d0"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#a3be8c"),
					BackgroundColor: strPtr("#2e3440"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "nord",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 4: // ── Gruvbox ───────────────────────────────────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#928374"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#fe8019"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#d3869b"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#fabd2f"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#504945"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#8ec07c")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#83a598"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#fe8019"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#b8bb26"),
					BackgroundColor: strPtr("#3c3836"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "monokai",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 5: // ── GitHub Light ──────────────────────────────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292e")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292e")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#6a737d"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#005cc5"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#e36209"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#005cc5"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#e1e4e8"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#0366d6")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#0366d6"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#005cc5"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#24292e"),
					BackgroundColor: strPtr("#f6f8fa"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "friendly",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292e")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	default: // ── Solarized Light (case 6) / fallback ──────────────────────
		return ansi.StyleConfig{
			Document:    ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#657b83")}, Margin: uintPtr(0)},
			Paragraph:   ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#657b83")}},
			BlockQuote:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#93a1a1"), Italic: boolPtr(true)}, Indent: uintPtr(1), IndentToken: strPtr("│ ")},
			List:        ansi.StyleList{LevelIndent: 2},
			Heading:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#268bd2"), Bold: boolPtr(true)}},
			H1:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6:          ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph:        ansi.StylePrimitive{Color: strPtr("#cb4b16"), Italic: boolPtr(true)},
			Strong:      ansi.StylePrimitive{Color: strPtr("#268bd2"), Bold: boolPtr(true)},
			HorizontalRule: ansi.StylePrimitive{Color: strPtr("#eee8d5"), Format: "\n─────────────────────────────────────\n"},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#2aa198")},
			Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
			Link:        ansi.StylePrimitive{Color: strPtr("#268bd2"), Underline: boolPtr(true)},
			LinkText:    ansi.StylePrimitive{Color: strPtr("#6c71c4"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#2e3436"),
					BackgroundColor: strPtr("#eee8d5"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "friendly",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#2e3436")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}
	}
}

// ── Renderer cache ────────────────────────────────────────────────────────────

type rendererKey struct {
	themeIdx int
	wordWrap int
}

var (
	rendererMu    sync.Mutex
	rendererCache = map[rendererKey]*glamour.TermRenderer{}
)

func invalidateRendererCache() {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	rendererCache = map[rendererKey]*glamour.TermRenderer{}
}

func cachedRenderer(themeIdx, wordWrap int) *glamour.TermRenderer {
	key := rendererKey{themeIdx, wordWrap}
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if r, ok := rendererCache[key]; ok {
		return r
	}
	cfg := glamourStyleConfig(themeIdx)
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cfg),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		r, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wordWrap),
		)
	}
	if r != nil {
		rendererCache[key] = r
	}
	return r
}

// ── parseSegments ─────────────────────────────────────────────────────────────

// textSegment is one parsed piece of a message — prose or a fenced code block.
type textSegment struct {
	code bool
	lang string
	body string
}

// parseSegments splits text into alternating prose and fenced code-block segments.
// Used by renderMarkdown (hybrid renderer) and smartCopy (clipboard).
func parseSegments(text string) []textSegment {
	var segs []textSegment
	var buf strings.Builder
	inCode := false
	var lang string

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)

		if !inCode && strings.HasPrefix(trimmed, "```") {
			if buf.Len() > 0 {
				segs = append(segs, textSegment{body: strings.Trim(buf.String(), "\n")})
				buf.Reset()
			}
			lang = strings.TrimPrefix(trimmed, "```")
			inCode = true
			continue
		}

		if inCode && trimmed == "```" {
			segs = append(segs, textSegment{code: true, lang: lang, body: strings.Trim(buf.String(), "\n")})
			buf.Reset()
			inCode = false
			lang = ""
			continue
		}

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}

	if buf.Len() > 0 {
		segs = append(segs, textSegment{code: inCode, lang: lang, body: strings.Trim(buf.String(), "\n")})
	}
	return segs
}
