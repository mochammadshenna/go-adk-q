package main

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Markdown renderer ─────────────────────────────────────────────────────────
//
// Handles the subset of Markdown that LLMs commonly produce:
//
//   - Fenced code blocks  (``` … ```) with an optional language tag
//   - Inline code spans   (`code`)
//   - Plain prose between blocks (word-wrapped by lipgloss)
//
// Code blocks are rendered as a bordered box using Unicode box-drawing
// characters.  The language tag, when present, is inlined into the top border:
//
//   ╭─ go ──────────────────────────────────────────╮
//   │                                               │
//   │  func main() {                                │
//   │      fmt.Println("Hello, World!")             │
//   │  }                                            │
//   │                                               │
//   ╰───────────────────────────────────────────────╯
//
// The parser is streaming-safe: an unclosed fence at EOF is rendered as an
// in-progress code block so the partial output stays coherent while tokens
// arrive from the model.

// ── Parser ────────────────────────────────────────────────────────────────────

// textSegment is one parsed piece of a message — prose or a fenced code block.
type textSegment struct {
	code bool   // true for a ```…``` block
	lang string // fence language tag ("go", "python", …); empty if absent
	body string // raw body text of the segment
}

// parseSegments splits text into alternating prose and code-fence segments.
// The line-by-line state machine deliberately avoids regex to keep the
// streaming-partial-text behaviour predictable and allocation-free.
func parseSegments(text string) []textSegment {
	var segs []textSegment
	var buf strings.Builder
	inCode := false
	var lang string

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)

		if !inCode && strings.HasPrefix(trimmed, "```") {
			// Opening fence: ```[lang]
			if buf.Len() > 0 {
				segs = append(segs, textSegment{body: strings.Trim(buf.String(), "\n")})
				buf.Reset()
			}
			lang = strings.TrimPrefix(trimmed, "```")
			inCode = true
			continue
		}

		if inCode && trimmed == "```" {
			// Closing fence
			segs = append(segs, textSegment{
				code: true,
				lang: lang,
				body: strings.Trim(buf.String(), "\n"),
			})
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

	// Flush trailing buffer (may be an unclosed fence during streaming).
	if buf.Len() > 0 {
		segs = append(segs, textSegment{
			code: inCode,
			lang: lang,
			body: strings.Trim(buf.String(), "\n"),
		})
	}
	return segs
}

// ── Inline code ───────────────────────────────────────────────────────────────

// inlineCodeRE matches `code` spans inside prose (non-greedy, single line).
var inlineCodeRE = regexp.MustCompile("`([^`\n]+)`")

// applyInlineCode replaces `code` spans in text with ANSI-styled equivalents.
// The output contains ANSI escape codes; lipgloss measures their visual width
// correctly when performing subsequent word-wrap operations.
func applyInlineCode(text string, style lipgloss.Style) string {
	return inlineCodeRE.ReplaceAllStringFunc(text, func(match string) string {
		inner := match[1 : len(match)-1]
		return style.Render(inner)
	})
}

// ── Code block box-drawing ────────────────────────────────────────────────────
//
// Box geometry:
//
//   contentW    — available column width for the message body
//   boxW        = contentW − 4   (2-col left indent + 2-col right clearance)
//   innerW      = boxW   − 2     (inside the │ border chars)
//   padW        = innerW − 2     (inside the 1-col inner padding on each side)
//
// Each rendered row is prefixed with a 2-space indent so it aligns with prose.
//
// Visual width per row:
//   2 (indent) + 1 (│) + 1 (pad) + padW (content) + 1 (pad) + 1 (│) = padW+6
//   = innerW+4 = boxW+2 = contentW−2  ✓

// codeTopBorder returns the top border line including an optional language tag.
//
//	With tag:    ╭─ go ──────────────────────────────────────╮
//	Without tag: ╭──────────────────────────────────────────╮
func codeTopBorder(lang string, innerW int) string {
	if lang == "" {
		return "╭" + strings.Repeat("─", innerW) + "╮"
	}
	label := " " + lang + " "
	labelW := lipgloss.Width(label)
	fill := innerW - 1 - labelW // 1 for the leading "─" after "╭"
	if fill < 1 {
		fill = 1
	}
	return "╭─" + label + strings.Repeat("─", fill) + "╮"
}

// renderCodeBlock renders one fenced code block as a bordered, padded box.
// borderStyle colours the box-drawing characters; lineStyle colours the body.
func renderCodeBlock(
	lang, body string,
	boxW int,
	borderColor, bgColor, fgColor lipgloss.TerminalColor,
) string {
	innerW := boxW - 2
	padW := innerW - 2
	if padW < 4 {
		padW = 4
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Code content: fixed-width lines on the block background.
	lineStyle := lipgloss.NewStyle().
		Background(bgColor).
		Foreground(fgColor).
		Width(padW).
		MaxWidth(padW)

	// Empty padding line (dark bg, no text).
	emptyLine := lineStyle.Render("")

	var sb strings.Builder
	indent := "  "

	// ── Top border ────────────────────────────────────────────────────────
	sb.WriteString(indent + borderStyle.Render(codeTopBorder(lang, innerW)) + "\n")

	// ── Top inner padding ─────────────────────────────────────────────────
	sb.WriteString(
		indent +
			borderStyle.Render("│") + " " +
			emptyLine + " " +
			borderStyle.Render("│") + "\n",
	)

	// ── Code lines ────────────────────────────────────────────────────────
	lines := strings.Split(strings.ReplaceAll(body, "\t", "    "), "\n")
	for _, line := range lines {
		rendered := lineStyle.Render(line)
		sb.WriteString(
			indent +
				borderStyle.Render("│") + " " +
				rendered + " " +
				borderStyle.Render("│") + "\n",
		)
	}

	// ── Bottom inner padding ──────────────────────────────────────────────
	sb.WriteString(
		indent +
			borderStyle.Render("│") + " " +
			emptyLine + " " +
			borderStyle.Render("│") + "\n",
	)

	// ── Bottom border ─────────────────────────────────────────────────────
	sb.WriteString(indent + borderStyle.Render("╰"+strings.Repeat("─", innerW)+"╯") + "\n")

	return sb.String()
}

// ── Top-level renderer ────────────────────────────────────────────────────────

// renderMarkdown converts an LLM reply into styled terminal output.
//
//	Prose  →  word-wrapped, left-padded, inline `code` highlighted
//	Code   →  bordered box with language tag in top-left corner
//
// Falls back to unstyled text on very narrow terminals (contentW < 12).
func renderMarkdown(s styledSet, text string, contentW int, baseStyle lipgloss.Style) string {
	segs := parseSegments(text)
	if len(segs) == 0 {
		return ""
	}

	// Narrow terminal: skip all box styling.
	if contentW < 12 {
		var sb strings.Builder
		for _, seg := range segs {
			if b := strings.TrimSpace(seg.body); b != "" {
				sb.WriteString(baseStyle.Render(b) + "\n")
			}
		}
		return sb.String()
	}

	boxW := contentW - 4
	if boxW < 8 {
		boxW = 8
	}

	var sb strings.Builder

	for i, seg := range segs {
		// Blank line between segments for visual breathing room.
		if i > 0 {
			sb.WriteByte('\n')
		}

		if seg.code {
			sb.WriteString(renderCodeBlock(
				seg.lang,
				seg.body,
				boxW,
				s.codeBorder,
				s.codeBg,
				s.codeFg,
			))
			continue
		}

		// Prose: apply inline code highlighting, then word-wrap.
		body := strings.TrimSpace(seg.body)
		if body == "" {
			continue
		}
		body = applyInlineCode(body, s.codeInline)
		sb.WriteString(baseStyle.PaddingLeft(2).Width(contentW).Render(body))
		sb.WriteByte('\n')
	}

	return sb.String()
}
