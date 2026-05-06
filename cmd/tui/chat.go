package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	chrome   lipgloss.TerminalColor
	onChrome lipgloss.TerminalColor
	text     lipgloss.TerminalColor

	// ── Semantic roles ────────────────────────────────────────────────────
	user    lipgloss.TerminalColor // user message label
	agent   lipgloss.TerminalColor // agent reply label
	errC    lipgloss.TerminalColor // errors (avoids the "error" keyword)
	system  lipgloss.TerminalColor // muted: timestamps, help text, system msgs
	loading lipgloss.TerminalColor // spinner + "Thinking…"

	// ── Code block ────────────────────────────────────────────────────────
	// codeBg     = entire block background
	// codeFg     = code text foreground
	// codeBorder = box-drawing characters + language tag
	// codeInline = background for inline `code` spans inside prose
	codeBg     lipgloss.TerminalColor
	codeFg     lipgloss.TerminalColor
	codeBorder lipgloss.TerminalColor
	codeInline lipgloss.TerminalColor
}

// builtinThemes is the ordered list of colour themes.  Index 0 (Catppuccin)
// is the default; pressing 't' advances the index cyclically.
//
// All five palettes are based on widely-used developer colour schemes so they
// look familiar and are carefully balanced for contrast.
var builtinThemes = []palette{
	// ── 1. Catppuccin Mocha ───────────────────────────────────────────────
	// Warm purple-tinted dark.  The most-starred dark theme in the
	// developer community (2024-2025); renders beautifully in 24-bit colour.
	{
		name:       "Catppuccin",
		chrome:     lipgloss.Color("#1e1e2e"), // base
		onChrome:   lipgloss.Color("#cdd6f4"), // text
		text:       lipgloss.Color("#cdd6f4"),
		user:       lipgloss.Color("#89b4fa"), // blue
		agent:      lipgloss.Color("#a6e3a1"), // green
		errC:       lipgloss.Color("#f38ba8"), // red
		system:     lipgloss.Color("#6c7086"), // overlay0 (muted)
		loading:    lipgloss.Color("#cba6f7"), // mauve
		codeBg:     lipgloss.Color("#181825"), // mantle (slightly darker)
		codeFg:     lipgloss.Color("#cdd6f4"),
		codeBorder: lipgloss.Color("#585b70"), // surface2
		codeInline: lipgloss.Color("#313244"), // surface0
	},
	// ── 2. Tokyo Night ────────────────────────────────────────────────────
	// Deep city-at-night blue.  Inspired by the VSCode theme of the same
	// name; cool and high-contrast.
	{
		name:       "Tokyo Night",
		chrome:     lipgloss.Color("#1a1b26"),
		onChrome:   lipgloss.Color("#c0caf5"),
		text:       lipgloss.Color("#c0caf5"),
		user:       lipgloss.Color("#7aa2f7"), // blue
		agent:      lipgloss.Color("#9ece6a"), // green
		errC:       lipgloss.Color("#f7768e"), // red
		system:     lipgloss.Color("#565f89"), // comment (muted)
		loading:    lipgloss.Color("#bb9af7"), // purple
		codeBg:     lipgloss.Color("#16161e"), // darker bg
		codeFg:     lipgloss.Color("#c0caf5"),
		codeBorder: lipgloss.Color("#414868"),
		codeInline: lipgloss.Color("#292e42"),
	},
	// ── 3. Rosé Pine ──────────────────────────────────────────────────────
	// Warm, muted purple.  Designed for long reading sessions; easy on the
	// eyes with a cosy, literary feel.
	{
		name:       "Rosé Pine",
		chrome:     lipgloss.Color("#191724"), // base
		onChrome:   lipgloss.Color("#e0def4"), // text
		text:       lipgloss.Color("#e0def4"),
		user:       lipgloss.Color("#ebbcba"), // rose
		agent:      lipgloss.Color("#9ccfd8"), // foam
		errC:       lipgloss.Color("#eb6f92"), // love
		system:     lipgloss.Color("#6e6a86"), // muted
		loading:    lipgloss.Color("#c4a7e7"), // iris
		codeBg:     lipgloss.Color("#1f1d2e"), // surface
		codeFg:     lipgloss.Color("#e0def4"),
		codeBorder: lipgloss.Color("#403d52"), // highlight low
		codeInline: lipgloss.Color("#26233a"), // overlay
	},
	// ── 4. Nord ───────────────────────────────────────────────────────────
	// Arctic, cool blue.  Clean, minimal, widely used in system configs and
	// terminal dotfiles.
	{
		name:       "Nord",
		chrome:     lipgloss.Color("#2e3440"), // polar night 1
		onChrome:   lipgloss.Color("#eceff4"), // snow storm 3
		text:       lipgloss.Color("#eceff4"),
		user:       lipgloss.Color("#88c0d0"), // frost 3 (cyan)
		agent:      lipgloss.Color("#a3be8c"), // aurora green
		errC:       lipgloss.Color("#bf616a"), // aurora red
		system:     lipgloss.Color("#4c566a"), // polar night 4 (muted)
		loading:    lipgloss.Color("#81a1c1"), // frost 2
		codeBg:     lipgloss.Color("#242933"), // between polar nights
		codeFg:     lipgloss.Color("#eceff4"),
		codeBorder: lipgloss.Color("#434c5e"), // polar night 3
		codeInline: lipgloss.Color("#3b4252"), // polar night 2
	},
	// ── 5. Gruvbox ────────────────────────────────────────────────────────
	// Warm, earthy retro.  High-contrast with amber/terracotta accents;
	// a classic in the Vim/Neovim community.
	{
		name:       "Gruvbox",
		chrome:     lipgloss.Color("#282828"), // bg hard
		onChrome:   lipgloss.Color("#ebdbb2"), // fg1
		text:       lipgloss.Color("#ebdbb2"),
		user:       lipgloss.Color("#83a598"), // aqua
		agent:      lipgloss.Color("#b8bb26"), // bright green
		errC:       lipgloss.Color("#fb4934"), // bright red
		system:     lipgloss.Color("#928374"), // gray
		loading:    lipgloss.Color("#d3869b"), // bright purple
		codeBg:     lipgloss.Color("#1d2021"), // bg hard (darkest)
		codeFg:     lipgloss.Color("#ebdbb2"),
		codeBorder: lipgloss.Color("#504945"), // bg3
		codeInline: lipgloss.Color("#3c3836"), // bg1
	},
}

// styledSet is a collection of pre-built lipgloss styles for one theme.
// Code block colours are kept as raw TerminalColor values so markdown.go can
// build custom-width box-drawing borders at render time.
type styledSet struct {
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

	// Raw colours passed through to the code-block renderer in markdown.go.
	codeBg     lipgloss.TerminalColor
	codeFg     lipgloss.TerminalColor
	codeBorder lipgloss.TerminalColor
	codeInline lipgloss.Style // inline `code` span style
}

func makeStyles(p palette) styledSet {
	return styledSet{
		// Header: bold text on the chrome background.
		header: lipgloss.NewStyle().Bold(true).
			Foreground(p.onChrome).Background(p.chrome).Padding(0, 1),

		// Separator: a muted horizontal rule.
		sep: lipgloss.NewStyle().Foreground(p.system),

		// User message: colored bold label, readable body text.
		userLabel: lipgloss.NewStyle().Bold(true).Foreground(p.user),
		userText:  lipgloss.NewStyle().Foreground(p.text),

		// Agent reply: colored bold label, readable body text.
		agentLabel: lipgloss.NewStyle().Bold(true).Foreground(p.agent),
		agentText:  lipgloss.NewStyle().Foreground(p.text),

		// Errors: accent-coloured label and body for immediate visibility.
		errorLabel: lipgloss.NewStyle().Bold(true).Foreground(p.errC),
		errorText:  lipgloss.NewStyle().Foreground(p.errC),

		// System: dim italic — timestamps, status notes.
		system: lipgloss.NewStyle().Foreground(p.system).Italic(true),

		// Loading: spinner + "Thinking…" line.
		loading: lipgloss.NewStyle().Foreground(p.loading),

		// Prompt prefix "> ".
		prompt: lipgloss.NewStyle().Bold(true).Foreground(p.user),

		// Footer hint text.
		help: lipgloss.NewStyle().Foreground(p.system).Italic(true),

		// Code block — raw colours for the box-drawing renderer.
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

// labelLine renders "Label           HH:MM" with the timestamp right-aligned
// to contentW columns.  If at is the zero time, only the label is returned.
func labelLine(labelStyle, tsStyle lipgloss.Style, label string, at time.Time, contentW int) string {
	left := labelStyle.Render(label)
	if at.IsZero() {
		return left + "\n"
	}
	right := tsStyle.Render(at.Format("15:04"))
	pad := contentW - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right + "\n"
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
// per line after the "> " prompt; maxH caps the result.
func calcInputHeight(text string, effectiveWidth, maxH int) int {
	if effectiveWidth <= 0 || maxH <= 0 {
		return 1
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return 1
	}
	h := (len(runes) + effectiveWidth - 1) / effectiveWidth
	if h < 1 {
		h = 1
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

// ── Bubbletea model ───────────────────────────────────────────────────────────

// chatModel is the root Bubbletea model.  It owns:
//   - a scrollable viewport for message history
//   - a single-line textinput for composing messages
//   - a spinner shown while waiting for the agent
//
// Features wired in:
//   - ↑/↓   shell-style input history cycling
//   - t      cycle colour theme
//   - s      open settings overlay (theme + char limit)
//   - ?/esc  toggle help overlay (rendered inside the viewport)
//   - ctrl+l clear conversation history
//   - ctrl+s save conversation to ~/.go-adk-q/session.json
//   - ctrl+y copy last agent reply to clipboard
//   - streaming responses via channel + recursive tea.Cmd
//   - markdown code-block rendering via markdown.go
//   - timestamps on every message (HH:MM, right-aligned)
//   - character counter + token usage in the footer
//   - mouse-wheel scroll (via tea.WithMouseCellMotion)
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

	themeIdx int  // index into builtinThemes; cycled by 't'
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

	// Settings overlay (huh form).
	settingsMode      bool
	settingsForm      *huh.Form
	settingsThemeIdx  int
	settingsCharLimit int

	runner     *runner.Runner
	sessionSvc session.Service
	memorySvc  memory.Service // nil when GOOGLE_API_KEY is not set
	userID     string
	sessionID  string
	modelName  string
}

// styles returns the styledSet for the current theme.  Called once per frame.
func (m chatModel) styles() styledSet {
	return makeStyles(builtinThemes[m.themeIdx])
}

// newChatModel constructs the initial chatModel.  The viewport is not yet
// sized — that happens on the first tea.WindowSizeMsg.
func newChatModel(r *runner.Runner, svc session.Service, memorySvc memory.Service, modelName string) chatModel {
	ti := textarea.New()
	ti.Placeholder = "Type a message and press Enter…"
	ti.Prompt = "> "      // prepended to every wrapped line
	ti.ShowLineNumbers = false
	ti.CharLimit = 2000
	ti.KeyMap.InsertNewline.SetEnabled(false) // Enter sends; Shift+Enter would insert newline
	ti.SetHeight(1)                           // grows dynamically as content wraps
	ti.Focus()                               // ignore returned Cmd; Init() fires textarea.Blink

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = makeStyles(builtinThemes[0]).loading

	return chatModel{
		textInput:  ti,
		spinner:    sp,
		userID:     "tui-user",
		sessionID:  "tui-session",
		runner:     r,
		sessionSvc: svc,
		memorySvc:  memorySvc,
		historyIdx: -1,
		msgs: []chatMsg{{
			role: "system",
			text: "Connected • " + modelName,
			at:   time.Now(),
		}},
		modelName: modelName,
	}
}

// Init issues the initial commands: cursor blinking and the spinner tick loop.
func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// ── Terminal resize — always handled, even in settings mode ───────────
	if wm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wm.Width
		m.height = wm.Height
		m.textInput.SetWidth(wm.Width - 5) // prompt + right margin
		// Re-fit input height at the new width before measuring layout.
		if wm.Width > 7 {
			m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), wm.Width-7, 5))
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
			m.viewport.SetContent(m.renderMessages(s))
			m.ready = true
		} else {
			m.viewport.Width = wm.Width
			m.viewport.Height = vpH
			m.refreshViewport()
		}

		// Resize settings form if open.
		if m.settingsMode && m.settingsForm != nil {
			m.settingsForm = m.settingsForm.WithWidth(wm.Width - 4)
		}
		return m, nil
	}

	// ── Settings overlay intercepts all non-resize events ─────────────────
	if m.settingsMode {
		return m.updateSettings(msg)
	}

	switch msg := msg.(type) {

	// ── Keyboard ──────────────────────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c":
			return m, tea.Quit

		// ── esc: dismiss help overlay ─────────────────────────────────
		case "esc":
			if m.showHelp {
				m.showHelp = false
				m.refreshViewport()
				return m, nil
			}

		// ── ctrl+l: clear history ─────────────────────────────────────
		case "ctrl+l":
			m.msgs = []chatMsg{{
				role: "system",
				text: "Connected • " + m.modelName,
				at:   time.Now(),
			}}
			m.streamingText = ""
			m.showHelp = false
			m.refreshViewport()
			return m, nil

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
				return m, tea.Batch(cmds...)
			}

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

		// ── t: cycle colour theme ─────────────────────────────────────
		case "t":
			if m.textInput.Value() == "" {
				m.themeIdx = (m.themeIdx + 1) % len(builtinThemes)
				m.spinner.Style = m.styles().loading // keep spinner colour in sync
				m.refreshViewport()
				return m, nil
			}

		// ── s: open settings overlay ──────────────────────────────────
		case "s":
			if m.textInput.Value() == "" && !m.loading {
				m.settingsThemeIdx = m.themeIdx
				m.settingsCharLimit = m.textInput.CharLimit
				m.settingsForm = buildSettingsForm(&m.settingsThemeIdx, &m.settingsCharLimit).
					WithWidth(m.width - 4)
				m.settingsMode = true
				return m, m.settingsForm.Init()
			}

		// ── ↑: walk backward through input history ────────────────────
		case "up":
			if !m.loading && len(m.inputHistory) > 0 {
				if m.historyIdx == -1 {
					// Enter browse mode: save the current draft.
					m.inputDraft = m.textInput.Value()
					m.historyIdx = len(m.inputHistory) - 1
				} else if m.historyIdx > 0 {
					m.historyIdx--
				}
			m.textInput.SetValue(m.inputHistory[m.historyIdx])
			m.textInput.CursorEnd()
			if m.width > 7 {
				m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), m.width-7, 5))
			}
			return m, nil // intercept; don't scroll the viewport
			}
			// No history → fall through so the viewport can scroll.

		// ── ↓: walk forward through input history ────────────────────
		case "down":
			if m.historyIdx != -1 {
				if m.historyIdx < len(m.inputHistory)-1 {
					m.historyIdx++
					m.textInput.SetValue(m.inputHistory[m.historyIdx])
				} else {
					// End of history: restore the saved draft.
					m.historyIdx = -1
					m.textInput.SetValue(m.inputDraft)
					m.inputDraft = ""
				}
			m.textInput.CursorEnd()
			if m.width > 7 {
				m.textInput.SetHeight(calcInputHeight(m.textInput.Value(), m.width-7, 5))
			}
			return m, nil // intercept
			}
			// Not browsing → fall through so the viewport can scroll.

		// ── enter: send message ───────────────────────────────────────
		case "enter":
			if m.loading {
				break
			}
			input := strings.TrimSpace(m.textInput.Value())
			if input == "" {
				break
			}
			m.showHelp = false
			// Record in history; avoid consecutive duplicates.
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
		}

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

		case msg.done:
			m.loading = false
			text := strings.TrimSpace(m.streamingText)
			if text == "" {
				text = "(no response)"
			}
			m.msgs = append(m.msgs, chatMsg{role: "agent", text: text, at: time.Now()})
			m.streamingText = ""
			// Accumulate token counts from the completed turn.
			m.totalPromptTokens += msg.promptTokens
			m.totalCandidateTokens += msg.candidateTokens

		default:
			m.streamingText += msg.text
			if msg.next != nil {
				cmds = append(cmds, msg.next)
			}
		}
		m.refreshViewport()

	// ── Status bar expiry ─────────────────────────────────────────────────
	case statusClearMsg:
		m.statusMsg = ""
		// No explicit viewport refresh: the footer reads statusMsg directly
		// and will update on the next natural redraw (next key or tick).

	// ── Spinner tick ──────────────────────────────────────────────────────
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		// Keep ticking only while loading AND no streaming text is visible.
		// Once text is showing we don't need spinner redraws.
		if m.loading && m.streamingText == "" {
			cmds = append(cmds, cmd)
		}
	}

	// Always forward events to the sub-models so they update their own state.
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	// Dynamically grow/shrink the input area as the user types.
	// ">" prompt is 2 chars; SetWidth arg is m.width-5; inner text width = m.width-7.
	if m.ready && m.width > 7 {
		newH := calcInputHeight(m.textInput.Value(), m.width-7, 5)
		m.textInput.SetHeight(newH)
		// Recompute viewport height to account for the changed input height.
		s2 := m.styles()
		headerH := lipgloss.Height(m.headerView(s2))
		footerH := lipgloss.Height(m.footerView(s2))
		inputH := lipgloss.Height(m.inputView(s2))
		if vpH := m.height - headerH - footerH - inputH; vpH >= 1 {
			m.viewport.Height = vpH
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// updateSettings handles all input events while the settings overlay is open.
// ctrl+c quits the app; esc cancels without applying; form completion applies.
func (m chatModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always allow quit.
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "ctrl+c" {
		return m, tea.Quit
	}

	formModel, cmd := m.settingsForm.Update(msg)
	m.settingsForm = formModel.(*huh.Form)

	switch m.settingsForm.State {
	case huh.StateCompleted:
		// Apply the chosen values.
		m.themeIdx = m.settingsThemeIdx
		m.textInput.CharLimit = m.settingsCharLimit
		m.spinner.Style = m.styles().loading
		m.settingsMode = false
		m.refreshViewport()
		return m, nil

	case huh.StateAborted:
		// User pressed esc — discard changes.
		m.settingsMode = false
		return m, nil
	}

	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m chatModel) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}
	s := m.styles()
	if m.settingsMode {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.headerView(s),
			m.settingsView(),
			m.settingsFooter(s),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(s),
		m.viewport.View(),
		m.inputView(s),
		m.footerView(s),
	)
}

// ── Sub-views ─────────────────────────────────────────────────────────────────

func (m chatModel) headerView(s styledSet) string {
	title := "  go-adk-q  •  " + m.modelName

	// Only show a scroll hint when the user has scrolled up and there is
	// content below the visible area.  "0%" / "100%" are meaningless noise
	// — a directional cue is unambiguous and space-efficient.
	var right string
	if !m.viewport.AtBottom() {
		right = "▼ more  "
	}

	// Right-align `right` within the header.  The header style has
	// Padding(0,1) which accounts for the 2-column inset already applied
	// by Width(m.width).Render().
	inner := m.width - 2
	pad := inner - lipgloss.Width(title) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return s.header.Width(m.width).Render(title + strings.Repeat(" ", pad) + right)
}

func (m chatModel) inputView(s styledSet) string {
	sep := s.sep.Render(strings.Repeat("─", m.width))
	return sep + "\n" + m.textInput.View()
}

func (m chatModel) footerView(s styledSet) string {
	// ── Character counter (right side, first line) ───────────────────────
	charCount := len([]rune(m.textInput.Value()))
	limit := m.textInput.CharLimit
	counterStr := fmt.Sprintf(" %d/%d", charCount, limit)

	var counterStyle lipgloss.Style
	if limit > 0 {
		ratio := float64(charCount) / float64(limit)
		switch {
		case ratio >= 1.0:
			counterStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
		case ratio >= 0.9:
			counterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
		default:
			counterStyle = s.system
		}
	} else {
		counterStyle = s.system
	}
	counter := counterStyle.Render(counterStr)
	counterW := lipgloss.Width(counter)

	// ── Hint text (left side, first line) ────────────────────────────────
	var hint string
	switch {
	case m.statusMsg != "":
		hint = "✓ " + m.statusMsg
	case m.loading:
		hint = m.spinner.View() + " Thinking…  •  ctrl+c: quit"
	default:
		hint = "enter: send  •  ↑/↓: history  •  s: settings  •  ?: help  •  t: theme  •  ctrl+l: clear  •  ctrl+c: quit"
	}

	hintW := m.width - counterW
	if hintW < 1 {
		hintW = 1
	}

	var hintRendered string
	if m.loading && m.statusMsg == "" {
		// Preserve the spinner's own colour; don't overlay the help style.
		hintRendered = lipgloss.NewStyle().Width(hintW).MaxWidth(hintW).Render(hint)
	} else {
		hintRendered = s.help.Width(hintW).MaxWidth(hintW).Render(hint)
	}

	line1 := hintRendered + counter

	// ── Token usage (second line) ─────────────────────────────────────────
	var line2 string
	if m.totalPromptTokens > 0 || m.totalCandidateTokens > 0 {
		tokenInfo := fmt.Sprintf(
			" tokens  in: %d  out: %d  total: %d",
			m.totalPromptTokens,
			m.totalCandidateTokens,
			m.totalPromptTokens+m.totalCandidateTokens,
		)
		line2 = s.system.Width(m.width).MaxWidth(m.width).Render(tokenInfo)
	} else {
		line2 = s.system.Width(m.width).MaxWidth(m.width).Render(" tokens  —")
	}

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
	return s.help.Width(m.width).MaxWidth(m.width).Render(hint)
}

// ── Viewport content ──────────────────────────────────────────────────────────

// refreshViewport re-renders the viewport content and scrolls to the bottom.
// It switches between the help overlay and the message history depending on
// m.showHelp.
func (m *chatModel) refreshViewport() {
	s := m.styles()
	if m.showHelp {
		m.viewport.SetContent(m.helpView(s))
	} else {
		m.viewport.SetContent(m.renderMessages(s))
	}
	m.viewport.GotoBottom()
}

// helpView renders the keyboard shortcut reference and theme colour picker
// inside the viewport so it is scrollable with no layout changes.
func (m chatModel) helpView(s styledSet) string {
	var sb strings.Builder

	sb.WriteString(s.userLabel.Render("Keyboard shortcuts") + "\n\n")

	bindings := [][2]string{
		{"enter", "Send message"},
		{"↑ / ↓", "Browse sent message history"},
		{"? / esc", "Toggle this help overlay"},
		{"t", "Cycle colour theme"},
		{"s", "Open settings (theme, char limit)"},
		{"ctrl+l", "Clear conversation history"},
		{"ctrl+s", "Save conversation → ~/.go-adk-q/session.json"},
		{"ctrl+y", "Copy last agent reply to clipboard"},
		{"ctrl+c", "Quit"},
		{"pgup / pgdn", "Scroll message history"},
		{"mouse scroll", "Scroll message history"},
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
	sb.WriteString("  " + strings.Join(badges, "  ") + "\n\n")
	sb.WriteString(s.system.Render("  Press t to cycle  •  s for settings  •  ? or esc to close") + "\n")

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
	contentW := w - 1

	if len(m.msgs) == 0 && !m.loading {
		return s.system.Render("  No messages yet.")
	}

	var sb strings.Builder

	for i, msg := range m.msgs {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch msg.role {
		case "user":
			sb.WriteString(labelLine(s.userLabel, s.system, "You", msg.at, contentW))
			sb.WriteString(s.userText.PaddingLeft(2).Width(contentW).Render(msg.text) + "\n")

		case "agent":
			sb.WriteString(labelLine(s.agentLabel, s.system, "Agent", msg.at, contentW))
			sb.WriteString(renderMarkdown(s, msg.text, contentW, s.agentText))

		case "error":
			sb.WriteString(labelLine(s.errorLabel, s.system, "Error", msg.at, contentW))
			// Hard-wrap before rendering: error messages contain URLs and JSON
			// with no spaces, which lipgloss's word-wrap can't break.
			wrapped := hardWrapText(msg.text, contentW-2)
			sb.WriteString(s.errorText.PaddingLeft(2).Render(wrapped) + "\n")

		case "system":
			sb.WriteString(s.system.PaddingLeft(2).Width(contentW).Render(msg.text) + "\n")
		}
	}

	// In-progress agent response while the goroutine is still running.
	if m.loading {
		if len(m.msgs) > 0 {
			sb.WriteString("\n")
		}
		if m.streamingText != "" {
			// Show partial text as it arrives (streaming).
			sb.WriteString(labelLine(s.agentLabel, s.system, "Agent", time.Time{}, contentW))
			sb.WriteString(renderMarkdown(s, m.streamingText, contentW, s.agentText))
		} else {
			// No text yet — show the spinner.
			sb.WriteString(s.loading.Render("  " + m.spinner.View() + " Thinking…") + "\n")
		}
	}

	return sb.String()
}

// ── Agent streaming ───────────────────────────────────────────────────────────
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

	m := newChatModel(r, svc, memorySvc, modelName)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		// WithMouseCellMotion lets the viewport's built-in mouse-wheel handler
		// scroll the history.  Text selection still works via shift-click in
		// most terminals.
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
