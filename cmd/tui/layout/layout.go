// Package layout holds go-adk-q's small, framework-agnostic rendering
// helpers: text wrapping, background-fill padding, ANSI-safe repainting,
// label-line composition, and two general utilities (a Bubbletea one-shot
// timer and a clipboard helper) that don't merit their own package.
//
// Extracted from cmd/tui's monolithic package as the second step of the
// opencode-style package split (theme/layout/components/{chat,core,dialog}) —
// mirrors opencode-ai/opencode's internal/tui/layout package. Every function
// here is a pure leaf: none take or return a chatModel, so moving them is a
// physical relocation (compiler-checked, exported names for cross-package
// call sites), not a behavior change.
package layout

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Small helpers ─────────────────────────────────────────────────────────────

// HardWrapText breaks lines that exceed maxW runes, preferring a natural
// break point (space, slash, comma, colon) in the trailing quarter of the line,
// then falling back to a hard break at maxW.  ANSI-unsafe: only call on
// plain-text strings (error messages, not pre-styled output).
func HardWrapText(text string, maxW int) string {
	if maxW <= 4 {
		return text
	}
	var out strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		runes := []rune(line)
		for len(runes) > maxW {
			breakAt := maxW
			// Prefer a natural break point in the last quarter of the window.
			for j := maxW - 1; j >= maxW*3/4; j-- {
				switch runes[j] {
				case ' ', '/', ',', ':', '.', '-':
					breakAt = j + 1
				}
				if breakAt != maxW {
					break
				}
			}
			out.WriteString(string(runes[:breakAt]))
			out.WriteByte('\n')
			runes = runes[breakAt:]
		}
		out.WriteString(string(runes))
	}
	return out.String()
}

// FillLines pads every line in content to exactly w visible columns by
// appending spaces rendered with bgStyle.  This is the single point of truth
// for right-edge fill: apply it at every viewport SetContent call and every
// View() component boundary so no terminal-default cells are ever exposed on
// light themes.  For dark themes bgStyle is empty so the call is a cheap no-op.
func FillLines(content string, w int, bgStyle lipgloss.Style) string {
	if w <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	sb.Grow(len(content) + len(lines)*8)
	for i, line := range lines {
		lw := lipgloss.Width(line)
		sb.WriteString(line)
		if pad := w - lw; pad > 0 {
			sb.WriteString(bgStyle.Render(strings.Repeat(" ", pad)))
		}
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// PaintLines re-renders every line in content so that the theme background
// colour is preserved even when the source content (e.g. textarea.View())
// contains ANSI reset sequences (\e[0m) that would otherwise drop back to the
// terminal default (black).
//
// Strategy: after every \e[0m in the stream re-inject the bg ANSI sequence so
// it is never lost.  For dark themes bgStyle has no Background and bgSeq is
// "", making this a no-op string replacement.
func PaintLines(content string, w int, bgStyle lipgloss.Style) string {
	if w <= 0 {
		return content
	}

	// Derive the bare ANSI bg sequence from bgStyle, e.g. "\e[48;2;255;255;255m".
	// Render a single space: if bgStyle has a background the result will be
	// "<bgSeq> <resetSeq>"; if not it will just be " ".
	bgSeq := ""
	if probe := bgStyle.Render(" "); probe != " " {
		// Extract the prefix before the space character.
		if idx := strings.Index(probe, " "); idx > 0 {
			bgSeq = probe[:idx]
		}
	}

	reset := "\x1b[0m"
	replacement := reset + bgSeq

	lines := strings.Split(content, "\n")
	var sb strings.Builder
	sb.Grow(len(content) + len(lines)*(len(bgSeq)+8))
	for i, line := range lines {
		// Start each line with the bg sequence so even the first cell is painted.
		if bgSeq != "" {
			sb.WriteString(bgSeq)
		}
		// Replace every reset with reset+bgSeq so bg survives resets mid-line.
		if bgSeq != "" {
			sb.WriteString(strings.ReplaceAll(line, reset, replacement))
		} else {
			sb.WriteString(line)
		}
		// Pad to full width with bg-coloured spaces.
		lw := lipgloss.Width(line)
		if pad := w - lw; pad > 0 {
			if bgSeq != "" {
				sb.WriteString(bgSeq)
			}
			sb.WriteString(strings.Repeat(" ", pad))
		}
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// OneShotTimer delivers msg after d by blocking in a background goroutine.
// This is the standard Bubbletea pattern for one-shot delayed messages.
func OneShotTimer(d time.Duration, msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return msg
	}
}

// CalcInputHeight returns the number of textarea rows needed to display text
// without horizontal scrolling.  effectiveWidth is the characters available
// per line (no prompt); maxH caps the result.  minH is the minimum height.
func CalcInputHeight(text string, effectiveWidth, maxH int) int {
	if effectiveWidth <= 0 || maxH <= 0 {
		return 3
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return 3
	}
	h := (len(runes) + effectiveWidth - 1) / effectiveWidth
	if h < 3 {
		h = 3
	}
	if h > maxH {
		h = maxH
	}
	return h
}

// CopyToClipboard writes text to the macOS system clipboard via pbcopy.
func CopyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
