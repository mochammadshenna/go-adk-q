package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)


// ── Theme system ───────────────────────────────────────────────────────────────
//
// palette is a semantic design-token set for one visual theme.  Using
// descriptive role names instead of raw colour indices keeps the rendering
// code readable and makes each theme self-documenting.  Hex truecolor values
// are used for modern terminal emulators (iTerm2, Kitty, Ghostty, WezTerm,
// Alacritty); lipgloss degrades gracefully to the nearest xterm-256 colour on
// older terminals.

type palette struct {
	name string

	// ── Chrome ────────────────────────────────────────────────────────────
	// chrome    = header bar background
	// onChrome  = text/icons drawn on top of the chrome background
	// text      = primary body text on the raw terminal background
	// bg        = viewport/body background (empty = use terminal default)
	chrome   lipgloss.TerminalColor
	onChrome lipgloss.TerminalColor
	text     lipgloss.TerminalColor
	bg       lipgloss.TerminalColor // explicit fill; empty for transparent dark themes

	// ── Semantic roles ────────────────────────────────────────────────────
	user    lipgloss.TerminalColor // user message label
	agent   lipgloss.TerminalColor // agent reply label
	accent  lipgloss.TerminalColor // input border, provider name highlight
	errC    lipgloss.TerminalColor // errors (avoids the "error" keyword)
	system  lipgloss.TerminalColor // muted: timestamps, help text, system msgs
	loading lipgloss.TerminalColor // spinner + "Thinking…"

	// ── Token counter colors ──────────────────────────────────────────────
	tokenIn  lipgloss.TerminalColor // input token count
	tokenOut lipgloss.TerminalColor // output token count

	// ── Code block ────────────────────────────────────────────────────────
	codeBg     lipgloss.TerminalColor
	codeFg     lipgloss.TerminalColor
	codeBorder lipgloss.TerminalColor
	codeInline lipgloss.TerminalColor
}

// builtinThemes is the ordered list of colour themes.  Index 0 (Catppuccin)
// is the default; /theme advances the index cyclically.
//
// All palettes use descriptive role names; dark themes leave bg empty
// (transparent = terminal default) while light themes set bg explicitly.
var builtinThemes = []palette{
	// ── 1. Catppuccin Mocha ───────────────────────────────────────────────
	{
		name:       "Catppuccin",
		chrome:     lipgloss.Color("#1e1e2e"),
		onChrome:   lipgloss.Color("#cdd6f4"),
		text:       lipgloss.Color("#cdd6f4"),
		bg:         lipgloss.Color(""),        // transparent — use terminal bg
		user:       lipgloss.Color("#89b4fa"), // blue
		agent:      lipgloss.Color("#a6e3a1"), // green
		accent:     lipgloss.Color("#89b4fa"), // blue — input border + provider
		errC:       lipgloss.Color("#f38ba8"),
		system:     lipgloss.Color("#6c7086"),
		loading:    lipgloss.Color("#cba6f7"),
		tokenIn:    lipgloss.Color("#89dceb"), // sky
		tokenOut:   lipgloss.Color("#a6e3a1"), // green
		codeBg:     lipgloss.Color("#181825"),
		codeFg:     lipgloss.Color("#cdd6f4"),
		codeBorder: lipgloss.Color("#585b70"),
		codeInline: lipgloss.Color("#313244"),
	},
	// ── 2. Tokyo Night ────────────────────────────────────────────────────
	{
		name:       "Tokyo Night",
		chrome:     lipgloss.Color("#1a1b26"),
		onChrome:   lipgloss.Color("#c0caf5"),
		text:       lipgloss.Color("#c0caf5"),
		bg:         lipgloss.Color(""),
		user:       lipgloss.Color("#7aa2f7"),
		agent:      lipgloss.Color("#9ece6a"),
		accent:     lipgloss.Color("#7aa2f7"),
		errC:       lipgloss.Color("#f7768e"),
		system:     lipgloss.Color("#565f89"),
		loading:    lipgloss.Color("#bb9af7"),
		tokenIn:    lipgloss.Color("#73daca"), // teal
		tokenOut:   lipgloss.Color("#9ece6a"), // green
		codeBg:     lipgloss.Color("#16161e"),
		codeFg:     lipgloss.Color("#c0caf5"),
		codeBorder: lipgloss.Color("#414868"),
		codeInline: lipgloss.Color("#292e42"),
	},
	// ── 3. Rosé Pine ──────────────────────────────────────────────────────
	{
		name:       "Rosé Pine",
		chrome:     lipgloss.Color("#191724"),
		onChrome:   lipgloss.Color("#e0def4"),
		text:       lipgloss.Color("#e0def4"),
		bg:         lipgloss.Color(""),
		user:       lipgloss.Color("#ebbcba"),
		agent:      lipgloss.Color("#9ccfd8"),
		accent:     lipgloss.Color("#c4a7e7"), // iris
		errC:       lipgloss.Color("#eb6f92"),
		system:     lipgloss.Color("#6e6a86"),
		loading:    lipgloss.Color("#c4a7e7"),
		tokenIn:    lipgloss.Color("#ebbcba"), // rose
		tokenOut:   lipgloss.Color("#9ccfd8"), // foam
		codeBg:     lipgloss.Color("#1f1d2e"),
		codeFg:     lipgloss.Color("#e0def4"),
		codeBorder: lipgloss.Color("#403d52"),
		codeInline: lipgloss.Color("#26233a"),
	},
	// ── 4. Nord ───────────────────────────────────────────────────────────
	{
		name:       "Nord",
		chrome:     lipgloss.Color("#2e3440"),
		onChrome:   lipgloss.Color("#eceff4"),
		text:       lipgloss.Color("#eceff4"),
		bg:         lipgloss.Color(""),
		user:       lipgloss.Color("#88c0d0"),
		agent:      lipgloss.Color("#a3be8c"),
		accent:     lipgloss.Color("#88c0d0"), // frost cyan
		errC:       lipgloss.Color("#bf616a"),
		system:     lipgloss.Color("#4c566a"),
		loading:    lipgloss.Color("#81a1c1"),
		tokenIn:    lipgloss.Color("#88c0d0"), // frost 3
		tokenOut:   lipgloss.Color("#a3be8c"), // aurora green
		codeBg:     lipgloss.Color("#242933"),
		codeFg:     lipgloss.Color("#eceff4"),
		codeBorder: lipgloss.Color("#434c5e"),
		codeInline: lipgloss.Color("#3b4252"),
	},
	// ── 5. Gruvbox ────────────────────────────────────────────────────────
	{
		name:       "Gruvbox",
		chrome:     lipgloss.Color("#282828"),
		onChrome:   lipgloss.Color("#ebdbb2"),
		text:       lipgloss.Color("#ebdbb2"),
		bg:         lipgloss.Color(""),
		user:       lipgloss.Color("#83a598"),
		agent:      lipgloss.Color("#b8bb26"),
		accent:     lipgloss.Color("#fabd2f"), // bright yellow
		errC:       lipgloss.Color("#fb4934"),
		system:     lipgloss.Color("#928374"),
		loading:    lipgloss.Color("#d3869b"),
		tokenIn:    lipgloss.Color("#83a598"), // aqua
		tokenOut:   lipgloss.Color("#b8bb26"), // green
		codeBg:     lipgloss.Color("#1d2021"),
		codeFg:     lipgloss.Color("#ebdbb2"),
		codeBorder: lipgloss.Color("#504945"),
		codeInline: lipgloss.Color("#3c3836"),
	},

	// ── 6. GitHub Light ───────────────────────────────────────────────────
	{
		name:       "GitHub Light",
		chrome:     lipgloss.Color("#f6f8fa"),
		onChrome:   lipgloss.Color("#24292f"),
		text:       lipgloss.Color("#24292f"),
		bg:         lipgloss.Color("#ffffff"), // explicit white fill
		user:       lipgloss.Color("#0550ae"),
		agent:      lipgloss.Color("#116329"),
		accent:     lipgloss.Color("#0550ae"), // blue border
		errC:       lipgloss.Color("#82071e"),
		system:     lipgloss.Color("#57606a"),
		loading:    lipgloss.Color("#8250df"),
		tokenIn:    lipgloss.Color("#0550ae"), // blue
		tokenOut:   lipgloss.Color("#116329"), // green
		codeBg:     lipgloss.Color("#f6f8fa"),
		codeFg:     lipgloss.Color("#24292f"),
		codeBorder: lipgloss.Color("#d0d7de"),
		codeInline: lipgloss.Color("#eaeef2"),
	},

	// ── 7. Solarized Light ────────────────────────────────────────────────
	{
		name:       "Solarized Light",
		chrome:     lipgloss.Color("#eee8d5"),
		onChrome:   lipgloss.Color("#657b83"),
		text:       lipgloss.Color("#657b83"),
		bg:         lipgloss.Color("#fdf6e3"), // base3 warm cream
		user:       lipgloss.Color("#268bd2"),
		agent:      lipgloss.Color("#859900"),
		accent:     lipgloss.Color("#2aa198"), // cyan
		errC:       lipgloss.Color("#dc322f"),
		system:     lipgloss.Color("#93a1a1"),
		loading:    lipgloss.Color("#6c71c4"),
		tokenIn:    lipgloss.Color("#268bd2"), // blue
		tokenOut:   lipgloss.Color("#2aa198"), // cyan
		codeBg:     lipgloss.Color("#fdf6e3"),
		codeFg:     lipgloss.Color("#586e75"),
		codeBorder: lipgloss.Color("#93a1a1"),
		codeInline: lipgloss.Color("#e8e0c8"),
	},

	// ── 8. Tango (Cyan) ───────────────────────────────────────────────────
	{
		name:       "Tango",
		chrome:     lipgloss.Color("#d3eef9"),
		onChrome:   lipgloss.Color("#204a87"),
		text:       lipgloss.Color("#2e3436"),
		bg:         lipgloss.Color("#e8f4fb"), // light cyan fill
		user:       lipgloss.Color("#204a87"),
		agent:      lipgloss.Color("#4e9a06"),
		accent:     lipgloss.Color("#3465a4"), // tango blue
		errC:       lipgloss.Color("#cc0000"),
		system:     lipgloss.Color("#888a85"),
		loading:    lipgloss.Color("#75507b"),
		tokenIn:    lipgloss.Color("#3465a4"), // blue
		tokenOut:   lipgloss.Color("#4e9a06"), // green
		codeBg:     lipgloss.Color("#e8f4fb"),
		codeFg:     lipgloss.Color("#2e3436"),
		codeBorder: lipgloss.Color("#a8d8ea"),
		codeInline: lipgloss.Color("#d3eef9"),
	},
}

// styledSet is a collection of pre-built lipgloss styles for one theme.
// Code block colours are kept as raw TerminalColor values so markdown.go can
// build custom-width box-drawing borders at render time.
type styledSet struct {
	themeIdx   int // index into builtinThemes; forwarded to glamour renderer
	header     lipgloss.Style
	sep        lipgloss.Style
	userLabel  lipgloss.Style
	userText   lipgloss.Style
	agentLabel lipgloss.Style
	agentText  lipgloss.Style
	errorLabel lipgloss.Style
	errorText  lipgloss.Style
	system     lipgloss.Style
	loading    lipgloss.Style
	prompt     lipgloss.Style
	help       lipgloss.Style

	// Input box border (accent color, rounded corners).
	inputBox lipgloss.Style
	// Full-width background fill for light themes (empty bg = no-op for dark).
	viewBg lipgloss.Style
	// Full-width background for footer/chrome on light themes (empty = transparent).
	chromeBg lipgloss.Style
	// Token counter — distinct colors for in/out/total.
	tokenIn    lipgloss.Style
	tokenOut   lipgloss.Style
	tokenTotal lipgloss.Style
	// Provider/model name in footer.
	providerName lipgloss.Style

	// Raw colours passed through to the code-block renderer in markdown.go.
	codeBg     lipgloss.TerminalColor
	codeFg     lipgloss.TerminalColor
	codeBorder lipgloss.TerminalColor
	codeInline lipgloss.Style // inline `code` span style
}

func makeStyles(p palette) styledSet {
	// bg applies the theme's explicit background colour to a style.
	// For dark themes p.bg is "" — Background("") is a no-op in lipgloss,
	// so dark styles remain transparent (terminal default).
	bg := func(s lipgloss.Style) lipgloss.Style {
		if p.bg == lipgloss.Color("") {
			return s
		}
		return s.Background(p.bg)
	}

	// viewBg / chromeBg: plain fill styles used for viewport and chrome areas.
	viewBgStyle := lipgloss.NewStyle()
	chromeBgStyle := lipgloss.NewStyle()
	if p.bg != lipgloss.Color("") {
		viewBgStyle = lipgloss.NewStyle().Background(p.bg)
		chromeBgStyle = lipgloss.NewStyle().Background(p.bg)
	}

	return styledSet{
		// Header always uses its own chrome colour — not the body bg.
		header: lipgloss.NewStyle().Bold(true).
			Foreground(p.onChrome).Background(p.chrome).Padding(0, 1),

		sep: bg(lipgloss.NewStyle().Foreground(p.system)),

		userLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.user)),
		userText:  bg(lipgloss.NewStyle().Foreground(p.text)),

		agentLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.agent)),
		agentText:  bg(lipgloss.NewStyle().Foreground(p.text)),

		errorLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.errC)),
		errorText:  bg(lipgloss.NewStyle().Foreground(p.errC)),

		system:  bg(lipgloss.NewStyle().Foreground(p.system).Italic(true)),
		loading: bg(lipgloss.NewStyle().Foreground(p.loading)),
		prompt:  bg(lipgloss.NewStyle().Bold(true).Foreground(p.user)),
		help:    bg(lipgloss.NewStyle().Foreground(p.system).Italic(true)),

		inputBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.accent).
			Background(p.bg).
			Padding(0, 1),

		viewBg:   viewBgStyle,
		chromeBg: chromeBgStyle,

		tokenIn:      bg(lipgloss.NewStyle().Bold(true).Foreground(p.tokenIn)),
		tokenOut:     bg(lipgloss.NewStyle().Bold(true).Foreground(p.tokenOut)),
		tokenTotal:   bg(lipgloss.NewStyle().Foreground(p.system)),
		providerName: bg(lipgloss.NewStyle().Bold(true).Foreground(p.accent)),

		codeBg:     p.codeBg,
		codeFg:     p.codeFg,
		codeBorder: p.codeBorder,
		codeInline: lipgloss.NewStyle().
			Background(p.codeInline).
			Foreground(p.codeFg),
	}
}

// ── Small helpers ─────────────────────────────────────────────────────────────

// hardWrapText breaks lines that exceed maxW runes, preferring a natural
// break point (space, slash, comma, colon) in the trailing quarter of the line,
// then falling back to a hard break at maxW.  ANSI-unsafe: only call on
// plain-text strings (error messages, not pre-styled output).
func hardWrapText(text string, maxW int) string {
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

// fillLines pads every line in content to exactly w visible columns by
// appending spaces rendered with bgStyle.  This is the single point of truth
// for right-edge fill: apply it at every viewport SetContent call and every
// View() component boundary so no terminal-default cells are ever exposed on
// light themes.  For dark themes bgStyle is empty so the call is a cheap no-op.
func fillLines(content string, w int, bgStyle lipgloss.Style) string {
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

// paintLines re-renders every line in content so that the theme background
// colour is preserved even when the source content (e.g. textarea.View())
// contains ANSI reset sequences (\e[0m) that would otherwise drop back to the
// terminal default (black).
//
// Strategy: after every \e[0m in the stream re-inject the bg ANSI sequence so
// it is never lost.  For dark themes bgStyle has no Background and bgSeq is
// "", making this a no-op string replacement.
func paintLines(content string, w int, bgStyle lipgloss.Style) string {
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

// labelLine renders "Label           HH:MM" with the timestamp right-aligned
// to fullW columns.  Padding spaces are rendered with bgStyle so that on light
// themes the entire row carries the theme background colour.
// If at is the zero time, only the label is returned (padded to fullW).
func labelLine(labelStyle, tsStyle, bgStyle lipgloss.Style, label string, at time.Time, fullW int) string {
	left := labelStyle.Render(label)
	if at.IsZero() {
		trail := fullW - lipgloss.Width(left)
		if trail > 0 {
			return left + bgStyle.Render(strings.Repeat(" ", trail)) + "\n"
		}
		return left + "\n"
	}
	right := tsStyle.Render(at.Format("15:04"))
	pad := fullW - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + bgStyle.Render(strings.Repeat(" ", pad)) + right + "\n"
}

// oneShotTimer delivers msg after d by blocking in a background goroutine.
// This is the standard Bubbletea pattern for one-shot delayed messages.
func oneShotTimer(d time.Duration, msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return msg
	}
}

// calcInputHeight returns the number of textarea rows needed to display text
// without horizontal scrolling.  effectiveWidth is the characters available
// per line (no prompt); maxH caps the result.  minH is the minimum height.
func calcInputHeight(text string, effectiveWidth, maxH int) int {
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

// copyToClipboard writes text to the macOS system clipboard via pbcopy.
func copyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// smartCopy picks what to copy from an agent reply:
//   - If the text contains one or more code fences, the body of the first
//     fence is copied (without the ``` delimiters).
//   - Otherwise the full reply text is copied.
//
// Returns the string to copy and a short status label.
func smartCopy(text string) (content, statusLabel string) {
	for _, seg := range parseSegments(text) {
		if seg.code {
			return seg.body, "Copied code block"
		}
	}
	return text, "Copied reply"
}

// ── Message types ─────────────────────────────────────────────────────────────

// chatMsg is one entry in the conversation history.
type chatMsg struct {
	role string    // "user" | "agent" | "error" | "system"
	text string
	at   time.Time // wall-clock time the message was created; zero → no timestamp
}

// streamChunkMsg carries one increment of a streaming agent response.
// When done is true the stream has ended; err holds any terminal error.
// next is the tea.Cmd to issue to receive the following chunk.
// promptTokens and candidateTokens are non-zero only on the final (done) chunk.
type streamChunkMsg struct {
	text            string
	done            bool
	err             error
	next            tea.Cmd
	promptTokens    int32
	candidateTokens int32
}

// statusClearMsg is delivered by oneShotTimer to erase the temporary status bar.
type statusClearMsg struct{}

// switchModelMsg carries the result of an async model-switch command.
// On success, newRunner and newModelName are set and err is nil.
// On failure, err describes why the switch failed.
type switchModelMsg struct {
	newRunner    *runner.Runner
	newModelName string
	err          error
}

// ── Bubbletea model ───────────────────────────────────────────────────────────

// chatModel is the root Bubbletea model.  It owns:
//   - a scrollable viewport for message history
//   - a single-line textinput for composing messages
//   - a spinner shown while waiting for the agent
//
// Features wired in:
//   - ↑/↓       shell-style input history cycling
//   - /theme     cycle colour theme
//   - /settings  open settings overlay (theme + char limit)
//   - /help      toggle help overlay (also ?/esc when input empty)
//   - esc        close settings overlay / dismiss help
//   - ctrl+l     clear conversation history
//   - ctrl+s     save conversation to ~/.go-adk-q/session.json
//   - ctrl+y     copy last agent reply to clipboard
//   - streaming responses via channel + recursive tea.Cmd
//   - markdown rendering (bold, headers, lists, code blocks) via markdown.go
//   - timestamps on every message (HH:MM, right-aligned)
//   - character counter + token usage in the footer
//   - pgup/pgdn/arrow key scroll + mouse-wheel scroll (via tea.WithMouseCellMotion)
type chatModel struct {
	viewport  viewport.Model
	textInput textarea.Model
	spinner   spinner.Model

	msgs          []chatMsg // completed conversation history
	streamingText string    // accumulates in-progress streaming response
	loading       bool      // true while agent goroutine is running
	ready         bool      // true once the first WindowSizeMsg has been received
	width         int       // current terminal width
	height        int       // current terminal height

	themeIdx int  // index into builtinThemes; cycled by /theme
	showHelp bool // true when the help overlay is displayed in the viewport

	// Cumulative token counts across all completed turns.
	totalPromptTokens    int32
	totalCandidateTokens int32

	// Input history — shell-style ↑/↓ cycling through previously sent messages.
	inputHistory []string // all sent messages, oldest → newest
	historyIdx   int      // -1 = not browsing; ≥0 = index into inputHistory
	inputDraft   string   // draft saved when entering history-browse mode

	// Temporary status bar message (auto-cleared after a few seconds).
	statusMsg string

	// Slash command autocomplete menu.
	slashMenuIdx int // selected row in the menu (-1 = none)

	// Settings overlay (huh form).
	settingsMode      bool
	settingsForm      *huh.Form
	settingsThemeIdx  int
	settingsCharLimit int
	mouseEnabled      bool // tracks mouse mode for ctrl+t toggle

	// Model picker overlay (/model command).
	modelPickerMode bool
	modelPicker     modelPickerState

	// Theme picker overlay (/theme command).
	themePickerMode bool
	themePickerIdx  int // highlighted row in theme picker

	// activeProviderIDs are the provider name substrings from the failover
	// chain, used to filter the /model picker to only configured providers.
	activeProviderIDs []string

	runner     *runner.Runner
	sessionSvc session.Service
	memorySvc  memory.Service // nil when GOOGLE_API_KEY is not set
	userID     string
	sessionID  string
	modelName  string
}

// styles returns the styledSet for the current theme with themeIdx populated.
// Called once per frame.
func (m chatModel) styles() styledSet {
	s := makeStyles(builtinThemes[m.themeIdx])
	s.themeIdx = m.themeIdx
	return s
}


// displayModelName returns a short model name suitable for the header.
// For a failover chain like "failover(github-models/gpt-4o → groq/llama...)"
// it returns just the primary part "github-models/gpt-4o".
// For a plain name it returns it unchanged.
func (m chatModel) displayModelName() string {
	s := m.modelName
	if idx := strings.Index(s, "("); idx >= 0 {
		s = s[idx+1:]
		if end := strings.Index(s, " →"); end >= 0 {
			s = s[:end]
		} else if end := strings.LastIndex(s, ")"); end >= 0 {
			s = s[:end]
		}
	}
	return strings.TrimSpace(s)
}

// newChatModel constructs the initial chatModel.  The viewport is not yet
// sized — that happens on the first tea.WindowSizeMsg.
// activeProviderIDs is the list of provider name substrings from the failover
// chain (used to filter the /model picker to only configured providers).
func newChatModel(r *runner.Runner, svc session.Service, memorySvc memory.Service, modelName string, activeProviderIDs []string) chatModel {
	ti := textarea.New()
	ti.Placeholder = "Type your message or @path/to/file"
	ti.Prompt = ""
	ti.ShowLineNumbers = false
	ti.CharLimit = 2000
	ti.KeyMap.InsertNewline.SetEnabled(false) // Enter sends; Shift+Enter would insert newline
	ti.SetHeight(3)                           // default 3 lines; grows dynamically as content wraps
	// Remove the textarea's own border — we wrap it in a lipgloss box instead.
	ti.FocusedStyle.Base = lipgloss.NewStyle()
	ti.BlurredStyle.Base = lipgloss.NewStyle()
	// Remove the black CursorLine background so the theme bg shows through.
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ti.Focus() // ignore returned Cmd; Init() fires textarea.Blink

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = makeStyles(builtinThemes[0]).loading

	return chatModel{
		textInput:         ti,
		spinner:           sp,
		userID:            "tui-user",
		sessionID:         "tui-session",
		runner:            r,
		sessionSvc:        svc,
		memorySvc:         memorySvc,
		historyIdx:        -1,
		slashMenuIdx:      0,
		mouseEnabled:      true,
		activeProviderIDs: activeProviderIDs,
		msgs: []chatMsg{{
			role: "system",
			text: "Session started",
			at:   time.Now(),
		}},
		modelName: modelName,
	}
}

// Init issues the initial commands: cursor blinking and the spinner tick loop.
func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// ── Terminal resize — always handled, even in settings mode ───────────
	if wm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wm.Width
		m.height = wm.Height
		m.textInput.SetWidth(wm.Width - 6) // 4 box border+padding (no prompt)
		// Re-fit input height at the new width before measuring layout.
		if wm.Width > 6 {
			m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), wm.Width-6, 5))
		}

		s := m.styles()
		headerH := lipgloss.Height(m.headerView(s))
		footerH := lipgloss.Height(m.footerView(s))
		inputH := lipgloss.Height(m.inputView(s))
		vpH := wm.Height - headerH - footerH - inputH
		if vpH < 1 {
			vpH = 1
		}
		if !m.ready {
			m.viewport = viewport.New(wm.Width, vpH)
			// Override the default KeyMap to remove vim-style letter bindings
			// (j/k/b/f/u/d/h/l and spacebar) that would fire while the user
			// is typing those characters into the text input.
			m.viewport.KeyMap = viewport.KeyMap{
				PageDown:     key.NewBinding(key.WithKeys("pgdown")),
				PageUp:       key.NewBinding(key.WithKeys("pgup")),
				HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
				HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
				Up:           key.NewBinding(key.WithKeys("up")),
				Down:         key.NewBinding(key.WithKeys("down")),
			}
			m.setViewportContent(s, m.renderMessages(s))
			m.ready = true
			m.applyThemeToTextarea()
		} else {
			m.viewport.Width = wm.Width
			m.viewport.Height = vpH
			m.applyThemeToTextarea() // refresh Base.Width after resize
			m.refreshViewport()
		}

		// Resize settings form if open.
		if m.settingsMode && m.settingsForm != nil {
			m.settingsForm = m.settingsForm.WithWidth(wm.Width - 4)
		}
		return m, nil
	}

	// ── Terminal focus / blur (e.g. switching tabs) ───────────────────────
	// Re-focus the textarea when the terminal window regains focus so the
	// user can type immediately without clicking.
	switch msg.(type) {
	case tea.FocusMsg:
		return m, m.textInput.Focus()
	case tea.BlurMsg:
		m.textInput.Blur()
		return m, nil
	}

	// ── Settings overlay intercepts all non-resize events ─────────────────
	if m.settingsMode {
		return m.updateSettings(msg)
	}

	// ── Model picker overlay intercepts keyboard when open ─────────────────
	if m.modelPickerMode {
		return m.updateModelPicker(msg)
	}

	// ── Theme picker overlay intercepts keyboard when open ────────────────
	if m.themePickerMode {
		return m.updateThemePicker(msg)
	}
	switch msg := msg.(type) {

	// ── Keyboard ──────────────────────────────────────────────────────────
	case tea.KeyMsg:
		// If the textarea lost focus (e.g. terminal window switched away and
		// back without a FocusMsg, or the terminal doesn't support focus
		// reporting), re-focus on the very first keypress — no click needed.
		var refocusCmd tea.Cmd
		if !m.textInput.Focused() {
			refocusCmd = m.textInput.Focus()
		}
		m2, cmd := m.handleKey(msg)
		return m2, tea.Batch(refocusCmd, cmd)

	// ── Streaming chunk ───────────────────────────────────────────────────
	case streamChunkMsg:
		switch {
		case msg.err != nil:
			m.loading = false
			m.msgs = append(m.msgs, chatMsg{
				role: "error",
				text: msg.err.Error(),
				at:   time.Now(),
			})
			m.streamingText = ""
			m.refreshViewport()

		case msg.done:
			m.loading = false
			text := strings.TrimSpace(m.streamingText)
			if text == "" {
				text = "(no response)"
			}
			m.msgs = append(m.msgs, chatMsg{role: "agent", text: text, at: time.Now()})
			m.streamingText = ""
			m.totalPromptTokens += msg.promptTokens
			m.totalCandidateTokens += msg.candidateTokens
			// Scroll to the START of the completed reply so long responses
			// (code blocks, lists) are visible from the top rather than being
			// scrolled past with only the last few lines in view.
			m.refreshViewportShowLast()

		default:
			m.streamingText += msg.text
			if msg.next != nil {
				cmds = append(cmds, msg.next)
			}
			m.refreshViewport()
		}

	// ── Status bar expiry ─────────────────────────────────────────────────
	case statusClearMsg:
		m.statusMsg = ""

	// ── Model switch result ───────────────────────────────────────────────
	case switchModelMsg:
		m.loading = false
		if msg.err != nil {
			m.msgs = append(m.msgs, chatMsg{
				role: "error",
				text: "Model switch failed: " + msg.err.Error(),
				at:   time.Now(),
			})
		} else {
			m.runner = msg.newRunner
			m.modelName = msg.newModelName
			m.statusMsg = "Switched to " + msg.newModelName
			m.msgs = append(m.msgs, chatMsg{
				role: "system",
				text: "Model switched to **" + msg.newModelName + "**",
				at:   time.Now(),
			})
			cmds = append(cmds, oneShotTimer(3*time.Second, statusClearMsg{}))
		}
		m.refreshViewport()

	// ── Spinner tick ──────────────────────────────────────────────────────
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading && m.streamingText == "" {
			cmds = append(cmds, cmd)
		}
	} // end switch msg.(type)

	// ── Route mouse events away from the textarea ─────────────────────────
	//
	// tea.MouseMsg (scroll wheel, clicks) must NOT be forwarded to the
	// textarea: the bubbles/textarea widget does not handle mouse events and
	// passes the raw SGR escape bytes straight into the text value, producing
	// garbage like "[<64;58;35M" whenever the user scrolls over the input box.
	//
	// Mouse wheel events are routed exclusively to the viewport so scrolling
	// always works regardless of where the pointer is positioned.
	// Non-wheel mouse messages (clicks, moves) are silently discarded for the
	// textarea — they carry no useful action for a text input field.
	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		if mouseMsg.Action == tea.MouseActionPress || mouseMsg.Action == tea.MouseActionRelease {
			// clicks/motion: only viewport gets them
		}
		// Always let the viewport handle scroll wheel and other mouse events.
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(mouseMsg)
		cmds = append(cmds, vpCmd)
		// Do NOT forward mouse events to textInput.
	} else {
		// Non-mouse events: forward to both textarea and viewport as normal.
		var tiCmd tea.Cmd
		m.textInput, tiCmd = m.textInput.Update(msg)
		cmds = append(cmds, tiCmd)

		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	// Dynamic textarea height + viewport resize after any input change.
	// effectiveW = SetWidth arg (no prompt) = m.width-6
	if m.ready && m.width > 6 {
		newH := calcInputHeight(m.textInput.Value(), m.width-6, 5)
		m.textInput.SetHeight(newH)
		s2 := m.styles()
		headerH := lipgloss.Height(m.headerView(s2))
		footerH := lipgloss.Height(m.footerView(s2))
		inputH := lipgloss.Height(m.inputView(s2))
		if vpH := m.height - headerH - footerH - inputH; vpH >= 1 {
			m.viewport.Height = vpH
		}
	}

	return m, tea.Batch(cmds...)
}

// updateSettings handles all input events while the settings overlay is open.
// ctrl+c quits the app; esc always cancels without applying; form completion applies.
func (m chatModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Always close settings on esc — don't forward to huh which may
			// consume it for field navigation without closing the overlay.
			m.settingsMode = false
			m.settingsForm = nil
			return m, m.textInput.Focus()
		}
	}

	formModel, cmd := m.settingsForm.Update(msg)
	m.settingsForm = formModel.(*huh.Form)

	switch m.settingsForm.State {
	case huh.StateCompleted:
		// Apply the chosen values.
		m.themeIdx = m.settingsThemeIdx
		invalidateRendererCache()
		m.applyThemeToTextarea()
		m.textInput.CharLimit = m.settingsCharLimit
		m.spinner.Style = m.styles().loading
		m.settingsMode = false
		m.settingsForm = nil
		m.refreshViewport()
		return m, m.textInput.Focus()

	case huh.StateAborted:
		// huh internally aborted (e.g. ctrl+c inside form) — discard changes.
		m.settingsMode = false
		m.settingsForm = nil
		return m, m.textInput.Focus()
	}

	return m, cmd
}

// updateModelPicker handles all input events while the model picker overlay is
// open.  ctrl+c quits, esc goes back (or closes), ↑/↓ navigates, enter selects.
func (m chatModel) updateModelPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.modelPicker.pickerBack() {
			m.modelPickerMode = false
			m.refreshViewport()
			return m, m.textInput.Focus()
		} else {
			m.refreshViewportModelPicker()
		}
	case "up":
		m.modelPicker.pickerMoveUp()
		m.refreshViewportModelPicker()
	case "down":
		m.modelPicker.pickerMoveDown()
		m.refreshViewportModelPicker()
	case "enter":
		providerID, modelID, done := m.modelPicker.pickerConfirm()
		if done {
			m.modelPickerMode = false
			m.loading = true
			m.refreshViewport()
			return m, tea.Batch(m.textInput.Focus(), m.spinner.Tick, switchModelCmd(providerID, modelID, m.sessionSvc, m.memorySvc))
		}
		// Advanced to model stage — re-render.
		m.refreshViewportModelPicker()
	}
	return m, nil
}

// refreshViewportModelPicker renders the picker content into the viewport.
func (m *chatModel) refreshViewportModelPicker() {
	s := m.styles()
	content := m.modelPicker.pickerView(s, m.width)
	m.setViewportContent(s, content)
	m.viewport.GotoTop()
}

// ── Theme picker ──────────────────────────────────────────────────────────────

func (m chatModel) updateThemePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.themePickerMode = false
		m.refreshViewport()
		return m, m.textInput.Focus()
	case "up", "k":
		if m.themePickerIdx > 0 {
			m.themePickerIdx--
		}
		m.refreshViewportThemePicker()
	case "down", "j":
		if m.themePickerIdx < len(builtinThemes)-1 {
			m.themePickerIdx++
		}
		m.refreshViewportThemePicker()
	case "enter", " ":
		m.themeIdx = m.themePickerIdx
		invalidateRendererCache()
		m.applyThemeToTextarea()
		m.spinner.Style = m.styles().loading
		m.themePickerMode = false
		m.refreshViewport()
		return m, m.textInput.Focus()
	}
	return m, nil
}

func (m *chatModel) refreshViewportThemePicker() {
	s := m.styles()
	m.setViewportContent(s, m.themePickerView(s))
	m.viewport.GotoTop()
}

func (m chatModel) themePickerView(s styledSet) string {
	var sb strings.Builder

	// Title line: pad to full width with chromeBg so no dark strip on right.
	titleContent := s.agentLabel.Render("Select Theme") +
		s.system.Render("  esc: cancel  •  ↑/↓: navigate  •  enter: apply")
	titlePad := m.width - lipgloss.Width(titleContent)
	if titlePad < 0 {
		titlePad = 0
	}
	sb.WriteString(titleContent + s.chromeBg.Render(strings.Repeat(" ", titlePad)) + "\n\n")

	rowW := m.width
	if rowW < 10 {
		rowW = 10
	}

	for i, p := range builtinThemes {
		// Swatch: render the theme name in its own chrome colours as a preview.
		swatchW := 22
		swatch := lipgloss.NewStyle().
			Bold(true).
			Foreground(p.onChrome).
			Background(p.chrome).
			Padding(0, 1).
			Width(swatchW).
			Render(p.name)

		activeMark := "  "
		if i == m.themeIdx {
			activeMark = " ●"
		}

		var cursor string
		if i == m.themePickerIdx {
			cursor = s.agentLabel.Render(">")
		} else {
			cursor = s.system.Render(" ")
		}

		// Build the full row content (cursor + mark + swatch) then pad to rowW
		// using chromeBg so light-theme rows don't have a dark strip on the right.
		inner := cursor + activeMark + "  " + swatch
		innerW := lipgloss.Width(inner)
		pad := rowW - innerW
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(inner + s.chromeBg.Render(strings.Repeat(" ", pad)) + "\n")
	}

	// Footer hint: pad to full width.
	hint := fmt.Sprintf("  %d themes  •  ● = active", len(builtinThemes))
	hintRendered := s.system.Render(hint)
	hintPad := m.width - lipgloss.Width(hintRendered)
	if hintPad < 0 {
		hintPad = 0
	}
	sb.WriteString("\n")
	sb.WriteString(hintRendered + s.chromeBg.Render(strings.Repeat(" ", hintPad)))
	return sb.String()
}

func (m chatModel) View() string {
	if !m.ready {
		return "\n  Initializing\u2026"
	}
	s := m.styles()
	// Apply the theme's background fill to the viewport (no-op for dark themes).
	m.viewport.Style = s.viewBg

	if m.settingsMode {
		return strings.Join([]string{
			m.headerView(s),
			m.settingsView(),
			m.settingsFooter(s),
		}, "\n")
	}
	if m.modelPickerMode {
		return strings.Join([]string{
			m.headerView(s),
			m.viewport.View(),
			m.footerView(s),
		}, "\n")
	}
	if m.themePickerMode {
		return strings.Join([]string{
			m.headerView(s),
			m.viewport.View(),
			m.footerView(s),
		}, "\n")
	}
	slashMenu := m.slashMenuViewIfVisible(s)
	parts := []string{m.headerView(s), m.viewport.View()}
	if slashMenu != "" {
		parts = append(parts, slashMenu)
	}
	parts = append(parts, m.inputView(s), m.footerView(s))
	return strings.Join(parts, "\n")
}

// ── Sub-views ─────────────────────────────────────────────────────────────────

func (m chatModel) headerView(s styledSet) string {
	// "Connected" always shown in green regardless of theme.
	connected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#a6e3a1")). // green — same across all themes
		Background(s.header.GetBackground()).
		Padding(0, 1).
		Render("Connected")

	// Build a bg-fill style using the header's chrome colour, then use
	// fillLines to pad to m.width without plain-space gaps (which appear as
	// terminal-default black on light themes).
	headerBg := lipgloss.NewStyle().Background(s.header.GetBackground())
	return fillLines(connected, m.width, headerBg)
}

func (m chatModel) slashMenuViewIfVisible(s styledSet) string {
	val := m.textInput.Value()
	if !slashMenuVisible(val) {
		return ""
	}
	matches := slashMatches(val)
	return slashMenuView(s, matches, m.slashMenuIdx, m.width)
}

func (m chatModel) inputView(s styledSet) string {
	// Wrap the textarea in a rounded border box colored with the accent color.
	// Width is set to fill the terminal; the textarea was already sized to
	// leave room for the 4-char box overhead (2 borders + 2 padding).
	//
	// textarea.View() returns Base.Render(viewport.View()). The viewport pads
	// lines to width and height using lipgloss.NewStyle() (no bg), and
	// Base.Background cannot repaint already-rendered interior cells with ANSI
	// resets. The output is therefore pre-padded plain-space lines with no bg.
	//
	// paintLines re-renders each line wrapped in chromeBg so every cell —
	// including lines that are already full-width spaces — gets the theme bg.
	//
	// Inner width = SetWidth arg (m.width-6): no prompt + no Base frame means
	// textarea internal m.width = SetWidth arg exactly.
	innerW := m.width - 6
	taView := paintLines(m.textInput.View(), innerW, s.chromeBg)
	box := s.inputBox.Width(m.width - 2).Render(taView)
	return box
}

func (m chatModel) footerView(s styledSet) string {
	p := builtinThemes[m.themeIdx]
	// bg applies the theme body background to any inline style, ensuring no
	// dark cells appear on light themes.  No-op for dark themes (p.bg == "").
	bg := func(st lipgloss.Style) lipgloss.Style {
		if p.bg == lipgloss.Color("") {
			return st
		}
		return st.Background(p.bg)
	}

	// ── Character counter (right side, first line) ───────────────────────
	charCount := len([]rune(m.textInput.Value()))
	limit := m.textInput.CharLimit
	counterStr := fmt.Sprintf(" %d/%d", charCount, limit)

	var counterStyle lipgloss.Style
	if limit > 0 {
		ratio := float64(charCount) / float64(limit)
		switch {
		case ratio >= 1.0:
			counterStyle = bg(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")))
		case ratio >= 0.9:
			counterStyle = bg(lipgloss.NewStyle().Foreground(lipgloss.Color("220")))
		default:
			counterStyle = s.system
		}
	} else {
		counterStyle = s.system
	}
	counter := counterStyle.Render(counterStr)
	counterW := lipgloss.Width(counter)

	// ── Scroll indicator ──────────────────────────────────────────────────
	var scrollIndicator string
	if !m.viewport.AtBottom() {
		pct := 0
		if m.viewport.TotalLineCount() > 0 {
			pct = int(m.viewport.ScrollPercent() * 100)
		}
		scrollIndicator = fmt.Sprintf(" ▼ %d%%", pct)
	}
	scrollW := lipgloss.Width(scrollIndicator)
	scrollRendered := bg(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f9e2af"))).
		Render(scrollIndicator)

	// ── Hint text ─────────────────────────────────────────────────────────
	var hint string
	switch {
	case m.statusMsg != "":
		hint = "✓ " + m.statusMsg
	case m.loading:
		hint = m.spinner.View() + " Thinking…  •  ctrl+c: quit"
	default:
		hint = "enter: send  •  ↑/↓: scroll  •  / for commands  •  ctrl+t: copy mode  •  ctrl+c: quit"
	}

	hintW := m.width - counterW - scrollW
	if hintW < 1 {
		hintW = 1
	}

	var hintRendered string
	if m.loading && m.statusMsg == "" {
		hintRendered = bg(lipgloss.NewStyle()).Width(hintW).MaxWidth(hintW).Render(hint)
	} else {
		hintRendered = s.help.Width(hintW).MaxWidth(hintW).Render(hint)
	}

	line1 := hintRendered + scrollRendered + counter

	// ── Provider / model + token usage (second line) ─────────────────────
	displayName := m.displayModelName()
	var line2 string
	if m.totalPromptTokens > 0 || m.totalCandidateTokens > 0 {
		total := m.totalPromptTokens + m.totalCandidateTokens
		line2 = s.providerName.Render(" "+displayName) +
			s.system.Render("  •  tokens ") +
			s.system.Render("in: ") + s.tokenIn.Render(fmt.Sprintf("%d", m.totalPromptTokens)) +
			s.system.Render("  out: ") + s.tokenOut.Render(fmt.Sprintf("%d", m.totalCandidateTokens)) +
			s.system.Render("  total: ") + s.tokenTotal.Render(fmt.Sprintf("%d", total))
	} else {
		line2 = s.providerName.Render(" "+displayName) + s.system.Render("  •  tokens —")
	}

	// Pad both lines to full terminal width so no dark strip appears on light themes.
	// fillLines appends bg-coloured spaces rather than plain ones (Width().Render()
	// uses plain spaces which show as terminal-default black on light themes).
	line1 = fillLines(line1, m.width, s.chromeBg)
	line2 = fillLines(line2, m.width, s.chromeBg)

	return line1 + "\n" + line2
}

// settingsView renders the huh form in the viewport area height.
func (m chatModel) settingsView() string {
	if m.settingsForm == nil {
		return ""
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.viewport.Height).
		Render(m.settingsForm.View())
}

// settingsFooter renders a one-line hint bar shown below the settings form.
func (m chatModel) settingsFooter(s styledSet) string {
	hint := "enter/space: select  •  ↑/↓: navigate  •  esc: cancel  •  ctrl+c: quit"
	rendered := s.help.MaxWidth(m.width).Render(hint)
	return fillLines(rendered, m.width, s.chromeBg)
}

// ── Viewport content ──────────────────────────────────────────────────────────

// refreshViewport re-renders the viewport content.
//
// Scrolling strategy:
//   - Help overlay  → always GotoBottom (content is short)
//   - Loading/streaming → GotoBottom so new chunks stream into view
//   - After a completed agent reply → scrollToLastMessage so the START of the
// setViewportContent fills every line to m.width with the theme background
// before handing content to the viewport.  This is required because the
// viewport's internal lipgloss.NewStyle().Width(contentWidth).Render() pads
// short lines with spaces that carry no background colour — so on light themes
// those cells expose terminal-default (black).  By pre-filling here we ensure
// every stored line is already full-width with the correct bg.
func (m *chatModel) setViewportContent(s styledSet, content string) {
	// Fill every line to m.width so trailing cells carry the theme bg colour
	// (the viewport's inner lipgloss.NewStyle().Width() pads with no bg).
	filled := fillLines(content, m.width, s.chromeBg)

	// The viewport pads short content to its Height with blank lines rendered
	// by lipgloss.NewStyle().Height() — no bg, so they appear terminal-default
	// (black) on light themes. Pre-pad the content ourselves so the viewport
	// sees enough lines and adds none of its own.
	if m.viewport.Height > 0 {
		lineCount := strings.Count(filled, "\n") + 1
		if lineCount < m.viewport.Height {
			blank := s.chromeBg.Render(strings.Repeat(" ", m.width))
			extra := strings.Repeat("\n"+blank, m.viewport.Height-lineCount)
			filled += extra
		}
	}

	m.viewport.SetContent(filled)
}


// field via reflect+unsafe and sets its Style.  This is the only way to paint
// the textarea interior cells with the theme background; all other approaches
// (Base.Width, CursorLine.Background) fail because Inline(true) strips bg.
// The field layout is stable across bubbles v1.x.
func setTextareaViewportStyle(ta *textarea.Model, style lipgloss.Style) {
	rv := reflect.ValueOf(ta).Elem()
	vField := rv.FieldByName("viewport")
	if !vField.IsValid() || vField.IsNil() {
		return
	}
	// vField is *viewport.Model (unexported ptr).  Make it addressable.
	vp := (*viewport.Model)(unsafe.Pointer(vField.Pointer()))
	vp.Style = style
}

// applyThemeToTextarea updates the textarea's internal style fields to match
// the current palette.  For light themes (explicit bg) we set Base.Background
// so the entire textarea block fills with the theme colour — Base is the only
// style that is NOT stripped by Inline(true) in the computed sub-styles.
// For dark themes we clear Base background so the terminal default shows through.
//
// We also set CursorLine, EndOfBuffer, Placeholder and Text foreground colours
// so text is readable on every theme.
func (m *chatModel) applyThemeToTextarea() {
	p := builtinThemes[m.themeIdx]

	if p.bg != lipgloss.Color("") {
		// Light theme: set Base background + paint the internal viewport with
		// the same bg so every cell (including right-side trailing space) is
		// filled.  Base.Width is NOT set — we rely on the viewport Style instead.
		baseBg := lipgloss.NewStyle().Background(p.bg)
		m.textInput.FocusedStyle.Base = baseBg
		m.textInput.BlurredStyle.Base = baseBg
		// Internal viewport: paints trailing cells on each line with p.bg.
		setTextareaViewportStyle(&m.textInput, lipgloss.NewStyle().Background(p.bg))
		// EndOfBuffer tilde: fg only (inline anyway).
		m.textInput.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(p.system)
		m.textInput.BlurredStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(p.system)
		// CursorLine: clear (Inline strips bg; viewport Style handles fill).
		m.textInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
		m.textInput.BlurredStyle.CursorLine = lipgloss.NewStyle()
	} else {
		// Dark theme: transparent — use terminal default.
		m.textInput.FocusedStyle.Base = lipgloss.NewStyle()
		m.textInput.BlurredStyle.Base = lipgloss.NewStyle()
		setTextareaViewportStyle(&m.textInput, lipgloss.NewStyle())
		m.textInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
		m.textInput.BlurredStyle.CursorLine = lipgloss.NewStyle()
		m.textInput.FocusedStyle.EndOfBuffer = lipgloss.NewStyle()
		m.textInput.BlurredStyle.EndOfBuffer = lipgloss.NewStyle()
	}

	// Set foreground colours so text is readable regardless of theme.
	textFg := lipgloss.NewStyle().Foreground(p.text)
	phFg := lipgloss.NewStyle().Foreground(p.system)
	m.textInput.FocusedStyle.Text = textFg
	m.textInput.BlurredStyle.Text = textFg
	m.textInput.FocusedStyle.Placeholder = phFg
	m.textInput.BlurredStyle.Placeholder = phFg
}

//     reply is visible; the user can scroll down to read code blocks below
//   - Everything else (user msg, error, system) → GotoBottom
func (m *chatModel) refreshViewport() {
	s := m.styles()
	if m.showHelp {
		m.setViewportContent(s, m.helpView(s))
		m.viewport.GotoBottom()
		return
	}
	m.setViewportContent(s, m.renderMessages(s))
	m.viewport.GotoBottom()
}

// refreshViewportShowLast re-renders and scrolls to the TOP of the last agent
// message, so long responses (code blocks, lists) start at the top of the
// visible area rather than being scrolled past.
//
// If the last agent reply fits entirely within the viewport height, we fall
// back to GotoBottom so short replies don't leave dead space at the bottom.
func (m *chatModel) refreshViewportShowLast() {
	if m.showHelp {
		m.refreshViewport()
		return
	}

	s := m.styles()
	w := m.width
	if w < 20 {
		w = 80
	}
	contentW := w - 4

	// Build the content up to (but not including) the last agent message so
	// we can measure its line offset.  We need to count rendered lines, not
	// raw bytes, so we render each message individually.
	var before strings.Builder
	lastAgentIdx := -1
	for i, msg := range m.msgs {
		if msg.role == "agent" {
			lastAgentIdx = i
		}
	}

	if lastAgentIdx < 0 {
		// No agent message yet — just go to bottom.
		m.setViewportContent(s, m.renderMessages(s))
		m.viewport.GotoBottom()
		return
	}

	// Render everything before the last agent message.
	for i, msg := range m.msgs {
		if i == lastAgentIdx {
			break
		}
		if i > 0 {
			before.WriteByte('\n')
		}
		switch msg.role {
		case "user":
			before.WriteString(labelLine(s.userLabel, s.system, s.chromeBg, "You", msg.at, w))
			before.WriteString(s.userText.PaddingLeft(2).Width(w).Render(msg.text) + "\n")
		case "agent":
			before.WriteString(labelLine(s.agentLabel, s.system, s.chromeBg, "Agent", msg.at, w))
			before.WriteString(renderMarkdown(s, msg.text, contentW, s.agentText))
		case "error":
			before.WriteString(labelLine(s.errorLabel, s.system, s.chromeBg, "Error", msg.at, w))
			wrapped := hardWrapText(msg.text, contentW-2)
			before.WriteString(s.errorText.PaddingLeft(2).Width(w).Render(wrapped) + "\n")
		case "system":
			before.WriteString(s.system.PaddingLeft(2).Width(w).Render(msg.text) + "\n")
		}
	}

	// The Y offset is the number of rendered lines before the last agent msg.
	// strings.Count(s, "\n") counts newline characters.  The loop above already
	// writes the inter-message separator "\n" (line 1157), so that byte is already
	// included in beforeStr — we must NOT add an extra +1 here.
	beforeStr := before.String()
	yOffset := strings.Count(beforeStr, "\n")

	// Render the last agent reply to measure its height.
	lastMsg := m.msgs[lastAgentIdx]
	var lastBlock strings.Builder
	lastBlock.WriteString(labelLine(s.agentLabel, s.system, s.chromeBg, "Agent", lastMsg.at, w))
	lastBlock.WriteString(renderMarkdown(s, lastMsg.text, contentW, s.agentText))
	lastH := strings.Count(lastBlock.String(), "\n") + 1

	// Full content for SetContent.
	full := m.renderMessages(s)
	m.setViewportContent(s, full)

	if lastH <= m.viewport.Height {
		// Short reply fits: scroll to bottom so no dead space below.
		m.viewport.GotoBottom()
	} else {
		// Long reply: position viewport so the label line is at the top.
		m.viewport.SetYOffset(yOffset)
	}
}

// helpView renders the keyboard shortcut reference and theme colour picker
// inside the viewport so it is scrollable with no layout changes.
func (m chatModel) helpView(s styledSet) string {
	var sb strings.Builder

	sb.WriteString(s.userLabel.Render("Keyboard shortcuts") + "\n\n")

	bindings := [][2]string{
		{"enter", "Send message"},
		{"/settings", "Open settings overlay (theme, char limit)"},
		{"/model", "Switch model or provider (2-level picker)"},
		{"/help", "Toggle this help overlay"},
		{"/clear", "Clear conversation history"},
		{"/theme", "Cycle colour theme"},
		{"/skills", "List available agent skills"},
		{"↑ / ↓", "Browse sent message history"},
		{"? / esc", "Toggle help (when input empty)"},
		{"ctrl+l", "Clear conversation history"},
		{"ctrl+s", "Save conversation → ~/.go-adk-q/session.json"},
		{"ctrl+y", "Copy last agent reply to clipboard"},
		{"shift+drag", "Select and copy any text (works natively, no mouse mode)"},
		{"ctrl+c", "Quit"},
		{"pgup / pgdn", "Scroll message history"},
		{"↑ / ↓", "Scroll or browse input history"},
	}
	for _, b := range bindings {
		k := s.prompt.Render(fmt.Sprintf("  %-16s", b[0]))
		desc := s.system.Render(b[1])
		sb.WriteString(k + "  " + desc + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(s.agentLabel.Render("Colour themes") + "\n\n")

	badges := make([]string, len(builtinThemes))
	for i, t := range builtinThemes {
		marker := "○"
		if i == m.themeIdx {
			marker = "●"
		}
		badges[i] = lipgloss.NewStyle().
			Bold(i == m.themeIdx).
			Foreground(t.onChrome).
			Background(t.chrome).
			Padding(0, 1).
			Render(marker + " " + t.name)
	}
	// Gaps between badges must carry the chrome background so they don't show
	// terminal-default (black) on light themes.
	gap := s.chromeBg.Render("  ")
	sb.WriteString(gap + strings.Join(badges, gap) + "\n\n")
	sb.WriteString(s.system.Render("  /theme to cycle  •  /settings for full settings  •  /help or esc to close") + "\n")

	return sb.String()
}

// renderMessages converts the chatMsg slice into styled, word-wrapped output
// for the viewport.
//
// Layout per message:
//
//	User   →  "You           HH:MM\n  <wrapped text>\n"
//	Agent  →  "Agent         HH:MM\n  <markdown-rendered text>\n"
//	Error  →  "Error         HH:MM\n  <wrapped text>\n"
//	System →  "  <italic text>\n"
//
// While loading and streaming text has already arrived, the in-progress agent
// reply is appended live (no spinner, just text).  Before the first chunk the
// spinner "Thinking…" line is shown instead.
//
// contentW = m.width − 1  (1-column right margin so text never kisses the edge).
// Falls back to 80 columns before the first WindowSizeMsg is received.
func (m chatModel) renderMessages(s styledSet) string {
	w := m.width
	if w < 20 {
		w = 80
	}
	contentW := w - 4 // leave glamour room for its 2-col left margin

	if len(m.msgs) == 0 && !m.loading {
		return s.system.Width(w).Render("  No messages yet.")
	}

	var sb strings.Builder

	for i, msg := range m.msgs {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch msg.role {
		case "user":
			sb.WriteString(labelLine(s.userLabel, s.system, s.chromeBg, "You", msg.at, w))
			sb.WriteString(s.userText.PaddingLeft(2).Width(w).Render(msg.text) + "\n")

		case "agent":
			sb.WriteString(labelLine(s.agentLabel, s.system, s.chromeBg, "Agent", msg.at, w))
			sb.WriteString(renderMarkdown(s, msg.text, contentW, s.agentText))

		case "error":
			sb.WriteString(labelLine(s.errorLabel, s.system, s.chromeBg, "Error", msg.at, w))
			// Hard-wrap before rendering: error messages contain URLs and JSON
			// with no spaces, which lipgloss's word-wrap can't break.
			wrapped := hardWrapText(msg.text, contentW-2)
			sb.WriteString(s.errorText.PaddingLeft(2).Width(w).Render(wrapped) + "\n")

		case "system":
			sb.WriteString(s.system.PaddingLeft(2).Width(w).Render(msg.text) + "\n")
		}
	}

	// In-progress agent response while the goroutine is still running.
	if m.loading {
		if len(m.msgs) > 0 {
			sb.WriteString("\n")
		}
		if m.streamingText != "" {
			// Show partial text as it arrives (streaming).
			sb.WriteString(labelLine(s.agentLabel, s.system, s.chromeBg, "Agent", time.Time{}, w))
			sb.WriteString(renderMarkdown(s, m.streamingText, contentW, s.agentText))
		} else {
			// No text yet — show the spinner.
			sb.WriteString(s.loading.Width(w).Render("  " + m.spinner.View() + " Thinking…") + "\n")
		}
	}

	return sb.String()
}

// ── Agent streaming ───────────────────────────────────────────────────────────

// switchModelCmd creates a tea.Cmd that calls newModelForEntry in a background
// goroutine and delivers a switchModelMsg when done.  This keeps model
// creation (which may involve network probing) off the UI thread.
// sessionSvc and memorySvc are captured from the running chatModel so the
// new runner reuses the same in-memory session history.
func switchModelCmd(providerID, modelID string, sessionSvc session.Service, memorySvc memory.Service) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		llm, err := newModelForEntry(ctx, providerID, modelID)
		if err != nil {
			return switchModelMsg{err: err}
		}
		r, err := rebuildRunnerWithModel(ctx, llm, sessionSvc, memorySvc)
		if err != nil {
			return switchModelMsg{err: fmt.Errorf("rebuild runner: %w", err)}
		}
		return switchModelMsg{newRunner: r, newModelName: llm.Name()}
	}
}
//
// Architecture:
//  1. startAgentStream launches a goroutine that runs the ADK iterator and
//     writes streamChunkMsg values to a buffered channel.
//  2. nextChunk returns a tea.Cmd that blocks on one channel read and then
//     embeds the next tea.Cmd in the returned streamChunkMsg (recursive).
//  3. Update handles streamChunkMsg: appends text, issues msg.next, and calls
//     refreshViewport so the partial response renders immediately.
//
// Provider compatibility:
//
//	Some providers emit cumulative text (full response so far on each event);
//	others emit incremental deltas.  startAgentStream handles both by tracking
//	the accumulated string and only forwarding the delta.

// startAgentStream starts the ADK runner in a background goroutine and returns
// a tea.Cmd that will deliver the first streamChunkMsg to the Update loop.
func (m chatModel) startAgentStream(input string) tea.Cmd {
	r := m.runner
	userID := m.userID
	sessionID := m.sessionID
	sessionSvc := m.sessionSvc
	memorySvc := m.memorySvc

	// Buffered channel so the goroutine is never blocked by a slow UI frame.
	ch := make(chan streamChunkMsg, 64)

	go func() {
		defer close(ch)

		ctx := context.Background()
		content := &genai.Content{
			Parts: []*genai.Part{{Text: input}},
			Role:  genai.RoleUser,
		}

		// accumulated tracks what we have already forwarded so we can compute
		// the delta when a provider returns cumulative text.
		var accumulated string

		// lastUsage holds the most recent UsageMetadata seen; included in the
		// final done message so the UI can display token counts.
		var promptToks, candidateToks int32

		for event, err := range r.Run(ctx, userID, sessionID, content, agent.RunConfig{}) {
			if err != nil {
				ch <- streamChunkMsg{err: err, done: true}
				return
			}
			if event == nil {
				continue
			}
			// Capture token usage from any event that reports it.
			if event.UsageMetadata != nil {
				promptToks = event.UsageMetadata.PromptTokenCount
				candidateToks = event.UsageMetadata.CandidatesTokenCount
			}
			if event.LLMResponse.Content == nil {
				continue
			}
			for _, part := range event.LLMResponse.Content.Parts {
				if part.Text == "" {
					continue
				}
				// Detect cumulative vs incremental provider.
				if strings.HasPrefix(part.Text, accumulated) {
					// Cumulative: only send the new suffix.
					delta := part.Text[len(accumulated):]
					if delta != "" {
						ch <- streamChunkMsg{text: delta}
						accumulated = part.Text
					}
				} else {
					// Incremental: the whole Part.Text is the delta.
					ch <- streamChunkMsg{text: part.Text}
					accumulated += part.Text
				}
			}
		}

		// Save the session to memory so preload_memory and load_memory tools
		// can recall earlier turns within the same session.  This is a no-op
		// when memorySvc is nil (non-Gemini provider, no GOOGLE_API_KEY).
		if memorySvc != nil && sessionSvc != nil {
			if resp, getErr := sessionSvc.Get(ctx, &session.GetRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			}); getErr == nil {
				_ = memorySvc.AddSessionToMemory(ctx, resp.Session)
			}
		}

		ch <- streamChunkMsg{
			done:            true,
			promptTokens:    promptToks,
			candidateTokens: candidateToks,
		}
	}()

	return nextChunk(ch)
}

// nextChunk returns a tea.Cmd that reads one value from ch and embeds the
// command for the following chunk in the returned message.
func nextChunk(ch <-chan streamChunkMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamChunkMsg{done: true}
		}
		if !msg.done && msg.err == nil {
			msg.next = nextChunk(ch)
		}
		return msg
	}
}

// ── Entry point ───────────────────────────────────────────────────────────────

// runChat starts the Bubbletea program.  Called by the 'chat' Cobra subcommand.
func runChat(r *runner.Runner, svc session.Service, memorySvc memory.Service, modelName string) error {
	// Redirect slog and stdlib log to a temp file for the lifetime of the TUI.
	// Background goroutines (failover retries, ADK internals) emit WARN/INFO
	// lines via slog; if those reach stderr they punch through the Bubbletea
	// alt-screen and corrupt the layout.  All diagnostics are captured to:
	//   $TMPDIR/go-adk-q-tui.log
	logPath := filepath.Join(os.TempDir(), "go-adk-q-tui.log")
	if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		h := slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelDebug})
		slog.SetDefault(slog.New(h))
		log.SetOutput(lf)
		defer lf.Close()
	}

	// Derive active provider IDs from the failover chain name string so the
	// /model picker only shows providers that are actually configured.
	// modelName is typically "failover(github-models → groq)" or just "github-models".
	activeProviderIDs := parseProviderIDs(modelName)

	m := newChatModel(r, svc, memorySvc, modelName, activeProviderIDs)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithReportFocus(), // sends FocusMsg/BlurMsg when terminal tab gains/loses focus
	)
	_, err := p.Run()
	return err
}

// parseProviderIDs extracts provider name substrings from a failover chain
// model name string.
//
// Examples:
//
//	"github-models"                           → ["github-models"]
//	"failover(github-models → groq → nvidia)" → ["github-models", "groq", "nvidia"]
func parseProviderIDs(modelName string) []string {
	// Strip outer "failover(...)" wrapper if present.
	s := modelName
	if idx := strings.Index(s, "("); idx >= 0 {
		s = s[idx+1:]
		if end := strings.LastIndex(s, ")"); end >= 0 {
			s = s[:end]
		}
	}
	// Split on " → " (en-dash arrow used by failover.Name()).
	parts := strings.Split(s, " → ")
	var ids []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			ids = append(ids, p)
		}
	}
	if len(ids) == 0 {
		ids = []string{modelName}
	}
	return ids
}

// handleKey processes tea.KeyMsg events. It is called from Update after
// re-focusing the textarea if needed.
func (m chatModel) handleKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg.String() {

	// ── ctrl+c: quit ──────────────────────────────────────────────
	case "ctrl+c":
		return m, tea.Quit

	// ── esc: dismiss help overlay ─────────────────────────────────
	case "esc":
		// Close slash menu if open.
		if slashMenuVisible(m.textInput.Value()) {
			m.textInput.Reset()
			m.slashMenuIdx = 0
			return m, nil
		}
		if m.showHelp {
			m.showHelp = false
			m.refreshViewport()
			return m, nil
		}

	// ── ctrl+l: clear history ─────────────────────────────────────
	case "ctrl+l":
		m.msgs = []chatMsg{{
			role: "system",
			text: "Session started",
			at:   time.Now(),
		}}
		m.streamingText = ""
		m.showHelp = false
		m.refreshViewport()
		return m, nil

	// ── ctrl+t: toggle mouse on/off for scroll vs copy ───────────
	case "ctrl+t":
		m.mouseEnabled = !m.mouseEnabled
		if m.mouseEnabled {
			m.statusMsg = "Scroll mode — touchpad scrolls"
			return m, tea.Batch(
				oneShotTimer(2*time.Second, statusClearMsg{}),
				func() tea.Msg { return tea.EnableMouseCellMotion() },
			)
		}
		m.statusMsg = "Copy mode — select text freely  (ctrl+t to scroll)"
		return m, tea.Batch(
			oneShotTimer(3*time.Second, statusClearMsg{}),
			func() tea.Msg { return tea.DisableMouse() },
		)

	// ── ctrl+s: save session ──────────────────────────────────────
	case "ctrl+s":
		if !m.loading {
			path, err := saveSession(m.msgs)
			if err != nil {
				m.statusMsg = "Save failed: " + err.Error()
			} else {
				m.statusMsg = "Saved → " + path
			}
			cmds = append(cmds, oneShotTimer(3*time.Second, statusClearMsg{}))
			m.refreshViewport()
		}
		return m, tea.Batch(cmds...)

	// ── ctrl+y: copy last agent reply ─────────────────────────────
	case "ctrl+y":
		for i := len(m.msgs) - 1; i >= 0; i-- {
			if m.msgs[i].role == "agent" {
				content, label := smartCopy(m.msgs[i].text)
				if err := copyToClipboard(content); err != nil {
					m.statusMsg = "Copy failed: " + err.Error()
				} else {
					m.statusMsg = label
				}
				cmds = append(cmds, oneShotTimer(2*time.Second, statusClearMsg{}))
				m.refreshViewport()
				return m, tea.Batch(cmds...)
			}
		}

	// ── ?: toggle help overlay ────────────────────────────────────
	case "?":
		// Only when the input box is empty so '?' is not typed into it.
		if m.textInput.Value() == "" {
			m.showHelp = !m.showHelp
			m.refreshViewport()
			return m, nil
		}

	// ── ↑: slash menu navigation OR input history ─────────────────
	case "up":
		val := m.textInput.Value()
		if slashMenuVisible(val) {
			if m.slashMenuIdx > 0 {
				m.slashMenuIdx--
			}
			return m, nil
		}
		// History navigation only when the input box is empty.
		if !m.loading && len(m.inputHistory) > 0 && val == "" {
			if m.historyIdx == -1 {
				m.inputDraft = ""
				m.historyIdx = len(m.inputHistory) - 1
			} else if m.historyIdx > 0 {
				m.historyIdx--
			}
			m.textInput.SetValue(m.inputHistory[m.historyIdx])
			m.textInput.CursorEnd()
			if m.width > 6 {
				m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), m.width-6, 5))
			}
			return m, nil
		}
		// Non-empty input or no history → fall through to textarea.

	// ── ↓: slash menu navigation OR input history ────────────────
	case "down":
		val := m.textInput.Value()
		if slashMenuVisible(val) {
			matches := slashMatches(val)
			if m.slashMenuIdx < len(matches)-1 {
				m.slashMenuIdx++
			}
			return m, nil
		}
		if m.historyIdx != -1 {
			if m.historyIdx < len(m.inputHistory)-1 {
				m.historyIdx++
				m.textInput.SetValue(m.inputHistory[m.historyIdx])
			} else {
				m.historyIdx = -1
				m.textInput.SetValue(m.inputDraft)
				m.inputDraft = ""
			}
			m.textInput.CursorEnd()
			if m.width > 6 {
				m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), m.width-6, 5))
			}
			return m, nil
		}
		// Not browsing → fall through to textarea.

	// ── tab: complete slash command ───────────────────────────────
	case "tab":
		val := m.textInput.Value()
		if slashMenuVisible(val) {
			matches := slashMatches(val)
			idx := m.slashMenuIdx
			if idx >= len(matches) {
				idx = 0
			}
			m.textInput.SetValue(matches[idx].name)
			m.textInput.CursorEnd()
			m.slashMenuIdx = 0
			return m, nil
		}

	// ── enter: send message or execute slash command ─────────────
	case "enter":
		if m.loading {
			break
		}

		// If the slash menu is open, resolve the selection first.
		val := m.textInput.Value()
		if slashMenuVisible(val) {
			matches := slashMatches(val)
			if len(matches) > 1 {
				idx := m.slashMenuIdx
				if idx >= len(matches) {
					idx = 0
				}
				m.textInput.SetValue(matches[idx].name)
				m.textInput.CursorEnd()
				m.slashMenuIdx = 0
				return m, nil
			}
			if len(matches) == 1 {
				m.textInput.SetValue(matches[0].name)
				m.textInput.CursorEnd()
			}
		}

		input := strings.TrimSpace(m.textInput.Value())
		if input == "" {
			break
		}
		m.slashMenuIdx = 0

		// ── Slash commands ─────────────────────────────────────────
		switch strings.ToLower(input) {
		case "/settings":
			m.textInput.Reset()
			m.settingsThemeIdx = m.themeIdx
			m.settingsCharLimit = m.textInput.CharLimit
			m.settingsForm = buildSettingsForm(&m.settingsThemeIdx, &m.settingsCharLimit).
				WithWidth(m.width - 4)
			m.settingsMode = true
			return m, m.settingsForm.Init()

		case "/help":
			m.textInput.Reset()
			m.showHelp = !m.showHelp
			m.refreshViewport()
			return m, nil

		case "/clear":
			m.textInput.Reset()
			m.msgs = []chatMsg{{
				role: "system",
				text: "Session started",
				at:   time.Now(),
			}}
			m.streamingText = ""
			m.showHelp = false
			m.refreshViewport()
			return m, nil

		case "/theme":
			m.textInput.Reset()
			m.themePickerIdx = m.themeIdx // pre-select current theme
			m.themePickerMode = true
			m.refreshViewportThemePicker()
			return m, nil

		case "/skills":
			m.textInput.Reset()
			m.showHelp = false
			m.msgs = append(m.msgs, chatMsg{
				role: "system",
				text: listSkillsSummary(),
				at:   time.Now(),
			})
			m.refreshViewport()
			return m, nil

		case "/model":
			m.textInput.Reset()
			m.showHelp = false
			m.modelPicker = newModelPickerState(m.activeProviderIDs, m.modelName)
			m.modelPickerMode = true
			m.refreshViewportModelPicker()
			return m, nil
		}

		// Not a slash command — send to agent.
		m.showHelp = false
		if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != input {
			m.inputHistory = append(m.inputHistory, input)
		}
		m.historyIdx = -1
		m.inputDraft = ""
		m.msgs = append(m.msgs, chatMsg{role: "user", text: input, at: time.Now()})
		m.textInput.Reset()
		m.loading = true
		m.refreshViewport()
		cmds = append(cmds, m.spinner.Tick, m.startAgentStream(input))
		return m, tea.Batch(cmds...)
	}

	// All other keys (printable chars, backspace, arrows when input non-empty,
	// etc.) are forwarded to the textarea widget so typing works normally.
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	// Keep textarea height in sync with content.
	if m.ready && m.width > 6 {
		m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), m.width-6, 5))
	}

	return m, tea.Batch(cmds...)
}
