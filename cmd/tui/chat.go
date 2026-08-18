package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"go-adk-q/cmd/tui/components/dialog"
	"go-adk-q/cmd/tui/layout"
	"go-adk-q/cmd/tui/theme"
	"go-adk-q/model/catalog"
	"go-adk-q/model/chain"
	"go-adk-q/model/failover"
)

// ── Message types ─────────────────────────────────────────────────────────────

// chatMsg is one entry in the conversation history.
type chatMsg struct {
	role string // "user" | "agent" | "error" | "system"
	text string
	at   time.Time // wall-clock time the message was created; zero → no timestamp
}

// streamChunkMsg carries one increment of a streaming agent response.
// When done is true the stream has ended; err holds any terminal error.
// next is the tea.Cmd to issue to receive the following chunk.
// promptTokens and candidateTokens are non-zero only on the final (done) chunk.
// permission is non-nil exactly once per paused tool call: the agent is
// blocked awaiting a human y/n decision (see permissionRequest below) before
// it can continue.
type streamChunkMsg struct {
	text            string
	done            bool
	err             error
	next            tea.Cmd
	promptTokens    int32
	candidateTokens int32
	permission      *permissionRequest
}

// permissionRequest surfaces one ADK Human-in-the-Loop confirmation request
// (google.golang.org/adk/tool/toolconfirmation) to the TUI. toolName/args are
// the *original* tool call the agent wants to run (e.g. exec_command with its
// command argument) — not the wrapping "adk_request_confirmation" call, which
// callID identifies so the answer can be routed back to the right pending
// request. resp is buffered (size 1): Update sends the human's decision and
// returns immediately, the paused agent goroutine reads it whenever it gets
// there.
type permissionRequest struct {
	toolName string
	args     map[string]any
	hint     string
	callID   string
	resp     chan<- bool
}

// statusClearMsg is delivered by layout.OneShotTimer to erase the temporary status bar.
type statusClearMsg struct{}

// switchModelMsg carries the result of an async model-switch command.
// On success, newRunner and newModelName are set and err is nil.
// On failure, err describes why the switch failed.
type switchModelMsg struct {
	newRunner     *runner.Runner
	newModelName  string
	failoverModel *failover.Model
	err           error
}

// filePickerState wraps bubbles/filepicker for the /filepicker overlay.
// Use /filepicker to browse and attach files; ESC or back-navigation cancels.
type filePickerState struct {
	fp      filepicker.Model
	showing bool
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

	themeIdx int  // index into theme.BuiltinThemes; cycled by /theme
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

	// @ file autocomplete menu — activated when input ends with @<filter>.
	atFileActive bool     // menu is open
	atFileIdx    int      // currently highlighted row
	atFileItems  []string // all files cached from atFileCwd
	atFileCwd    string   // CWD when atFileItems was built

	// Settings overlay (huh form).
	settingsMode      bool
	settingsForm      *huh.Form
	settingsThemeIdx  int
	settingsCharLimit int
	mouseEnabled      bool // tracks mouse mode for ctrl+t toggle

	// Login overlay (/login command) — add/switch provider credentials.
	//
	// loginResult MUST be a pointer, not a plain loginFormResult value: every
	// Update call runs on a value-receiver copy of chatModel, so a captured
	// &m.loginResult would point into a specific call's stack copy rather
	// than anything later code re-reads — huh's field widgets would mutate
	// an orphaned copy that this model never sees again. A pointer field's
	// target is the same object no matter how many times chatModel itself
	// gets copied by value, which is what buildLoginForm's bound Value(...)
	// pointers need.
	loginMode   bool
	loginForm   *huh.Form
	loginResult *loginFormResult
	// loginAwaitingKey is false while the auth-method form (stage 1) is
	// showing, true once it completed with API Key and the provider/key
	// form (stage 2) has been swapped in — see login.go's doc comment for
	// why this is two separate huh.Forms instead of one multi-group Form.
	loginAwaitingKey bool

	// Model picker overlay (/model command).
	modelPickerMode bool
	modelPicker     modelPickerState

	// Theme picker overlay (/theme command).
	themePickerMode bool
	themePickerIdx  int // highlighted row in theme picker

	// File picker overlay (bubbles/filepicker).
	filePickerMode  bool
	filePickerState filePickerState
	// selectedFiles are files chosen via /filepicker or @path that will be
	// appended as context to the next message sent to the agent.
	selectedFiles []string

	// cancelFn cancels the in-flight agent request goroutine.
	// nil when no request is running.  Call it (then nil it) to interrupt.
	cancelFn context.CancelFunc

	// pendingPermission is non-nil while the agent goroutine is paused awaiting
	// a human y/n decision on a sensitive tool call (see permissionRequest).
	// Update intercepts all key input while this is set; View renders the
	// approve/deny prompt in place of the normal footer.
	pendingPermission *permissionRequest

	// activeProviderIDs are the provider name substrings from the failover
	// chain, used to filter the /model picker to only configured providers.
	activeProviderIDs []string

	// failoverModel is the live *failover.Model backing the runner. It is the
	// single source of truth for the provider chain and exposes Stats()/Names()
	// so the UI can show which provider actually served the last response.
	failoverModel *failover.Model

	// routeProvider / routeFellBack surface the outcome of the most recent
	// turn (read from failoverModel.Stats()) so the footer can show a live
	// "⚡ groq" / "🔀 fell back to groq" badge.
	routeProvider string
	routeFellBack bool

	runner     *runner.Runner
	sessionSvc session.Service
	memorySvc  memory.Service // nil when GOOGLE_API_KEY is not set
	userID     string
	sessionID  string
	modelName  string

	// completedRenderCache holds the rendered output of every completed
	// (non-streaming) message in msgs. Completed messages never change once
	// appended, so re-parsing/re-bordering all of them on every streaming
	// chunk tick (as renderMessages used to) made responses feel late as a
	// conversation grew. Invalidated on message-count/width/theme change.
	completedRenderCache      string
	completedRenderCacheCount int
	completedRenderCacheWidth int
	completedRenderCacheTheme int
}

// styles returns the theme.StyledSet for the current theme with themeIdx populated.
// Called once per frame.
func (m chatModel) styles() theme.StyledSet {
	s := theme.MakeStyles(theme.BuiltinThemes[m.themeIdx])
	s.ThemeIdx = m.themeIdx
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

// providersSummary renders the configured failover chain and the outcome of
// the most recent turn as a markdown string for the /providers command. It
// makes the resilience design tangible: you can see every configured provider
// in priority order and precisely which one answered the last message.
func (m chatModel) providersSummary() string {
	if m.failoverModel == nil {
		return "Provider chain unavailable."
	}
	var sb strings.Builder
	sb.WriteString("**Provider chain** (priority order, ▶ = active primary):\n\n")
	for i, n := range m.failoverModel.Names() {
		mark := "  "
		if i == 0 {
			mark = "▶ "
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", mark, n))
	}
	p, fb, failed := m.failoverModel.Stats()
	switch {
	case p == "":
		sb.WriteString("\nNo response served yet this session.\n")
	case fb:
		failedList := strings.Join(failed, ", ")
		if failedList == "" {
			failedList = "primary"
		}
		sb.WriteString(fmt.Sprintf("\n🔀 Last response served by **%s** (fell back after: %s)\n", p, failedList))
	default:
		sb.WriteString(fmt.Sprintf("\n⚡ Last response served by **%s** (primary — no fallback needed)\n", p))
	}
	return sb.String()
}

// newChatModel constructs the initial chatModel.  The viewport is not yet
// sized — that happens on the first tea.WindowSizeMsg.
// activeProviderIDs is the list of provider name substrings from the failover
// chain (used to filter the /model picker to only configured providers).
func newChatModel(r *runner.Runner, svc session.Service, memorySvc memory.Service, modelName string, activeProviderIDs []string, fm *failover.Model) chatModel {
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
	sp.Style = theme.MakeStyles(theme.BuiltinThemes[0]).Loading

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
		modelName:     modelName,
		failoverModel: fm,
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
		m.textInput.SetWidth(wm.Width - 4) // 2 outer margin + 2 padding, no border (no prompt)
		// Re-fit input height at the new width before measuring layout.
		if wm.Width > 6 {
			m.textInput.SetHeight(layout.CalcInputHeight(m.textInput.Value(), wm.Width-4, 5))
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
			invalidateRendererCache() // word-wrap width changed
			m.applyThemeToTextarea()  // refresh Base.Width after resize
			m.refreshViewport()
		}

		// Resize settings form if open.
		if m.settingsMode && m.settingsForm != nil {
			m.settingsForm = m.settingsForm.WithWidth(wm.Width - 4)
		}
		// Resize login form if open.
		if m.loginMode && m.loginForm != nil {
			m.loginForm = m.loginForm.WithWidth(wm.Width - 4)
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

	// ── Pending tool-confirmation prompt intercepts all key input ──────────
	// The agent goroutine is blocked on m.pendingPermission.resp — only y/n/
	// esc/ctrl+c are meaningful here. Any other key (or non-key message,
	// e.g. spinner.TickMsg so the "awaiting approval" line keeps animating)
	// falls through to the normal handling below.
	if m.pendingPermission != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y", "Y":
				m.pendingPermission.resp <- true
				m.pendingPermission = nil
				return m, nil
			case "n", "N", "esc":
				m.pendingPermission.resp <- false
				m.pendingPermission = nil
				return m, nil
			}
			return m, nil
		}
	}

	// ── Settings overlay intercepts all non-resize events ─────────────────
	if m.settingsMode {
		return m.updateSettings(msg)
	}

	// ── Login overlay intercepts all non-resize events ────────────────────
	if m.loginMode {
		return m.updateLogin(msg)
	}

	// ── Model picker overlay intercepts keyboard when open ─────────────────
	if m.modelPickerMode {
		return m.updateModelPicker(msg)
	}

	// ── Theme picker overlay intercepts keyboard when open ────────────────
	if m.themePickerMode {
		return m.updateThemePicker(msg)
	}

	// ── File picker overlay intercepts all events when open ──────────────
	if m.filePickerMode {
		// ESC or back key closes the picker without a selection.
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "ctrl+c" {
				return m, tea.Quit
			}
			if km.String() == "esc" {
				m.filePickerMode = false
				m.filePickerState.showing = false
				return m, nil
			}
		}
		var fpCmd tea.Cmd
		m.filePickerState.fp, fpCmd = m.filePickerState.fp.Update(msg)
		if ok, path := m.filePickerState.fp.DidSelectFile(msg); ok {
			m.selectedFiles = append(m.selectedFiles, path)
			m.statusMsg = fmt.Sprintf("📎 Attached: %s (send a message to include it)", filepath.Base(path))
			cmds = append(cmds, layout.OneShotTimer(4*time.Second, statusClearMsg{}))
			m.filePickerMode = false
			m.filePickerState.showing = false
			return m, tea.Batch(fpCmd, tea.Batch(cmds...))
		}
		return m, fpCmd
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
			m.cancelFn = nil
			if errors.Is(msg.err, context.Canceled) {
				// User interrupted — preserve any partial text.
				if strings.TrimSpace(m.streamingText) != "" {
					partial := strings.TrimSpace(m.streamingText) + "\n\n*⛔ interrupted*"
					m.msgs = append(m.msgs, chatMsg{role: "agent", text: partial, at: time.Now()})
				} else {
					m.msgs = append(m.msgs, chatMsg{role: "system", text: "⛔ Request interrupted.", at: time.Now()})
				}
			} else {
				m.msgs = append(m.msgs, chatMsg{
					role: "error",
					text: msg.err.Error(),
					at:   time.Now(),
				})
			}
			m.streamingText = ""
			m.refreshViewport()

		case msg.done:
			m.loading = false
			m.cancelFn = nil
			text := strings.TrimSpace(m.streamingText)
			if text == "" {
				text = "(no response)"
			}
			m.msgs = append(m.msgs, chatMsg{role: "agent", text: text, at: time.Now()})
			m.streamingText = ""
			m.totalPromptTokens += msg.promptTokens
			m.totalCandidateTokens += msg.candidateTokens
			// Surface which provider actually served this turn (and whether it
			// fell back). This makes the failover mechanism visible in the UI.
			if m.failoverModel != nil {
				if p, fb, _ := m.failoverModel.Stats(); p != "" {
					m.routeProvider = p
					m.routeFellBack = fb
				}
			}
			m.refreshViewport()

		case msg.permission != nil:
			// Agent paused on a Human-in-the-Loop tool confirmation (e.g.
			// exec_command) — surface the prompt; loading stays true until
			// the human answers and the goroutine resumes or errors out.
			m.pendingPermission = msg.permission
			if msg.next != nil {
				cmds = append(cmds, msg.next)
			}
			m.refreshViewport()

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
			m.failoverModel = msg.failoverModel
			// Refresh the live route display for the new chain.
			if p, fb, _ := m.failoverModel.Stats(); p != "" {
				m.routeProvider = p
				m.routeFellBack = fb
			}
			m.statusMsg = "Switched to " + msg.newModelName
			m.msgs = append(m.msgs, chatMsg{
				role: "system",
				text: "Model switched to **" + msg.newModelName + "**",
				at:   time.Now(),
			})
			cmds = append(cmds, layout.OneShotTimer(3*time.Second, statusClearMsg{}))
		}
		m.refreshViewport()

	// ── Spinner tick ──────────────────────────────────────────────────────
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading {
			cmds = append(cmds, cmd)
			m.refreshViewport() // redraw viewport so ⠋ Thinking… frame advances
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
	// effectiveW = SetWidth arg (no prompt) = m.width-4
	if m.ready && m.width > 6 {
		newH := layout.CalcInputHeight(m.textInput.Value(), m.width-4, 5)
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
		m.spinner.Style = m.styles().Loading
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

// updateLogin handles all input events while the /login overlay is open.
// See login.go's buildLoginForm doc comment for the form's shape and the
// Subscription-is-a-placeholder rationale.
func (m chatModel) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.loginMode = false
			m.loginForm = nil
			return m, m.textInput.Focus()
		}
	}

	formModel, cmd := m.loginForm.Update(msg)
	m.loginForm = formModel.(*huh.Form)

	switch m.loginForm.State {
	case huh.StateCompleted:
		if !m.loginAwaitingKey {
			// Stage 1 (auth method) just completed.
			if m.loginResult.authMethod == loginAuthSubscription {
				m.loginMode = false
				m.loginForm = nil
				m.statusMsg = "Subscription login isn't available yet — use API Key"
				return m, m.textInput.Focus()
			}
			// API Key chosen — swap in stage 2 (provider + key), staying in
			// loginMode. See login.go's doc comment for why this is two
			// separate huh.Forms rather than one form with a hidden group.
			m.loginAwaitingKey = true
			m.loginForm = buildAPIKeyForm(catalog.All(), m.loginResult).
				WithWidth(m.width - 4)
			return m, m.loginForm.Init()
		}

		// Stage 2 (provider + key) just completed.
		m.loginMode = false
		m.loginForm = nil

		provider := m.loginResult.provider
		if err := saveCredential(provider, m.loginResult.key); err != nil {
			m.statusMsg = "Login save failed: " + err.Error()
			return m, m.textInput.Focus()
		}
		if envVar := envVarForProvider(provider); envVar != "" {
			os.Setenv(envVar, m.loginResult.key)
		}
		m.statusMsg = "Saved credentials for " + provider + " — switching to it"
		m.loading = true
		m.refreshViewport()
		return m, tea.Batch(m.textInput.Focus(), m.spinner.Tick, switchModelCmd(provider, "", m.sessionSvc, m.memorySvc))

	case huh.StateAborted:
		m.loginMode = false
		m.loginForm = nil
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
		if m.themePickerIdx < len(theme.BuiltinThemes)-1 {
			m.themePickerIdx++
		}
		m.refreshViewportThemePicker()
	case "enter", " ":
		m.themeIdx = m.themePickerIdx
		invalidateRendererCache()
		m.applyThemeToTextarea()
		m.spinner.Style = m.styles().Loading
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

func (m chatModel) themePickerView(s theme.StyledSet) string {
	var sb strings.Builder

	// Title line: pad to full width with chromeBg so no dark strip on right.
	titleContent := s.AgentLabel.Render("Select Theme") +
		s.System.Render("  esc: cancel  •  ↑/↓: navigate  •  enter: apply")
	titlePad := m.width - lipgloss.Width(titleContent)
	if titlePad < 0 {
		titlePad = 0
	}
	sb.WriteString(titleContent + s.ChromeBg.Render(strings.Repeat(" ", titlePad)) + "\n\n")

	rowW := m.width
	if rowW < 10 {
		rowW = 10
	}

	for i, p := range theme.BuiltinThemes {
		// Swatch: render the theme name in its own chrome colours as a preview.
		swatchW := 22
		swatch := lipgloss.NewStyle().
			Bold(true).
			Foreground(p.OnChrome).
			Background(p.Chrome).
			Padding(0, 1).
			Width(swatchW).
			Render(p.Name)

		activeMark := "  "
		if i == m.themeIdx {
			activeMark = " ●"
		}

		var cursor string
		if i == m.themePickerIdx {
			cursor = s.AgentLabel.Render(">")
		} else {
			cursor = s.System.Render(" ")
		}

		// Build the full row content (cursor + mark + swatch) then pad to rowW
		// using chromeBg so light-theme rows don't have a dark strip on the right.
		inner := cursor + activeMark + "  " + swatch
		innerW := lipgloss.Width(inner)
		pad := rowW - innerW
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(inner + s.ChromeBg.Render(strings.Repeat(" ", pad)) + "\n")
	}

	// Footer hint: pad to full width.
	hint := fmt.Sprintf("  %d themes  •  ● = active", len(theme.BuiltinThemes))
	hintRendered := s.System.Render(hint)
	hintPad := m.width - lipgloss.Width(hintRendered)
	if hintPad < 0 {
		hintPad = 0
	}
	sb.WriteString("\n")
	sb.WriteString(hintRendered + s.ChromeBg.Render(strings.Repeat(" ", hintPad)))
	return sb.String()
}

func (m chatModel) View() string {
	if !m.ready {
		return "\n  Initializing\u2026"
	}
	s := m.styles()
	// Apply the theme's background fill to the viewport (no-op for dark themes).
	m.viewport.Style = s.ViewBg

	if m.pendingPermission != nil {
		return strings.Join([]string{
			m.headerView(s),
			m.viewport.View(),
			m.permissionPromptView(s),
		}, "\n")
	}
	if m.settingsMode {
		return strings.Join([]string{
			m.headerView(s),
			m.settingsView(),
			m.settingsFooter(s),
		}, "\n")
	}
	if m.loginMode {
		return strings.Join([]string{
			m.headerView(s),
			m.loginView(),
			m.loginFooter(s),
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
	if m.filePickerMode {
		// Render the bubbles/filepicker overlay with header and a brief hint.
		title := s.AgentLabel.Render(" 📂 Select a file to attach")
		hint := s.System.Render("  ↑/↓ navigate  •  enter: select  •  h/backspace: go up  •  esc: cancel")
		var attachInfo string
		if len(m.selectedFiles) > 0 {
			attachInfo = s.Prompt.Render(fmt.Sprintf("  📎 %d file(s) pending attachment", len(m.selectedFiles)))
		}
		body := title + "\n" + hint + "\n"
		if attachInfo != "" {
			body += attachInfo + "\n"
		}
		body += "\n" + m.filePickerState.fp.View()
		return strings.Join([]string{
			m.headerView(s),
			body,
		}, "\n")
	}
	slashMenu := m.slashMenuViewIfVisible(s)
	atFileMenu := m.atFileMenuViewIfVisible(s)
	parts := []string{m.headerView(s), m.viewport.View()}
	if slashMenu != "" {
		parts = append(parts, slashMenu)
	}
	if atFileMenu != "" {
		parts = append(parts, atFileMenu)
	}
	parts = append(parts, m.inputView(s), m.footerView(s))
	return strings.Join(parts, "\n")
}

// ── Sub-views ─────────────────────────────────────────────────────────────────

func (m chatModel) headerView(s theme.StyledSet) string {
	// "Connected" always shown in green regardless of theme.
	connected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#a6e3a1")). // green — same across all themes
		Background(s.Header.GetBackground()).
		Padding(0, 1).
		Render("Connected")

	// Working directory, crush/opencode-style header convention: identify
	// which project the session is rooted in at a glance. Muted so it reads
	// as secondary information next to the Connected pill.
	headerBg := lipgloss.NewStyle().Background(s.Header.GetBackground())
	left := connected
	if cwd, err := os.Getwd(); err == nil {
		dir := headerBg.Foreground(s.System.GetForeground()).Render("  •  " + filepath.Base(cwd))
		left += dir
	}

	// Build a bg-fill style using the header's chrome colour, then use
	// layout.FillLines to pad to m.width without plain-space gaps (which appear as
	// terminal-default black on light themes).
	return layout.FillLines(left, m.width, headerBg)
}

func (m chatModel) slashMenuViewIfVisible(s theme.StyledSet) string {
	val := m.textInput.Value()
	if !dialog.SlashMenuVisible(val) {
		return ""
	}
	matches := dialog.SlashMatches(val)
	return dialog.SlashMenuView(s, matches, m.slashMenuIdx, m.width)
}

// atFileMenuViewIfVisible renders the @ file autocomplete dropdown when active.
func (m chatModel) atFileMenuViewIfVisible(s theme.StyledSet) string {
	if !m.atFileActive {
		return ""
	}
	filter, _ := extractAtFilter(m.textInput.Value())
	items := filterAtFileItems(m.atFileItems, filter)
	if len(items) == 0 {
		return ""
	}
	return atFileMenuView(s, items, m.atFileIdx, filter, m.width)
}

// permissionPromptView renders the approve/deny prompt shown in place of the
// input box while m.pendingPermission is set (Update intercepts y/n/esc/
// ctrl+c for the same state — see the early interception block there).
// Reuses ErrorAccent (the same left-border style error messages get) since
// this is, functionally, a warning: an agent-initiated action needs a human
// decision before it runs.
func (m chatModel) permissionPromptView(s theme.StyledSet) string {
	p := m.pendingPermission
	call := p.toolName
	if cmd, ok := p.args["command"].(string); ok && cmd != "" {
		call = fmt.Sprintf("%s(%s)", p.toolName, cmd)
	}
	body := strings.Join([]string{
		s.ErrorLabel.Render("🔒 Confirm tool call"),
		s.System.Render(call),
		s.Prompt.Render("[y] approve   [n]/[esc] deny   [ctrl+c] quit"),
	}, "\n")
	return wrapAccent(s.ErrorAccent, body+"\n")
}

func (m chatModel) inputView(s theme.StyledSet) string {
	// Wrap the textarea in a background-tinted box, no border — matching
	// opencode-ai/opencode's real editor.go, which has no Border at all.
	// Width is set to fill the terminal; the textarea was already sized to
	// leave room for the 4-char box overhead (2 outer margin + 2 padding).
	//
	// textarea.View() returns Base.Render(viewport.View()). The viewport pads
	// lines to width and height using lipgloss.NewStyle() (no bg), and
	// Base.Background cannot repaint already-rendered interior cells with ANSI
	// resets. The output is therefore pre-padded plain-space lines with no bg.
	//
	// layout.PaintLines re-renders each line wrapped in chromeBg so every cell —
	// including lines that are already full-width spaces — gets the theme bg.
	//
	// Inner width = SetWidth arg (m.width-4): no prompt + no Base frame means
	// textarea internal m.width = SetWidth arg exactly.
	innerW := m.width - 4
	taView := layout.PaintLines(m.textInput.View(), innerW, s.ChromeBg)
	box := s.InputBox.Width(m.width - 2).Render(taView)
	return box
}

// footerView renders a single-line status bar matching opencode's real
// convention (internal/tui/components/core/status.go, confirmed via live
// source fetch, not guessed): a flexible-width plain-text help area on the
// left, then colored theme.Chip "pill" segments packed directly adjacent —
// zero gap between them, no bullet separators — ending with the model-name
// chip at the absolute right edge. The two-line, gap-separated layout this
// footer originally shipped with this session was a divergence from the
// real convention; this is the corrected version.
func (m chatModel) footerView(s theme.StyledSet) string {
	p := theme.BuiltinThemes[m.themeIdx]

	// ── Chips, built right-to-left so each can measure its own width ─────

	displayName := m.displayModelName()
	modelChip := theme.Chip(p.Accent).Render(displayName)

	var routeChip string
	if m.routeProvider != "" {
		icon := "⚡" // primary served directly
		routeBg := p.Agent
		if m.routeFellBack {
			icon = "🔀" // fell back past the primary
			routeBg = p.ErrC
		}
		badge := icon
		if m.routeProvider != displayName {
			badge += " " + m.routeProvider
		}
		routeChip = theme.Chip(routeBg).Render(badge)
	}

	var tokensChip string
	if m.totalPromptTokens > 0 || m.totalCandidateTokens > 0 {
		total := m.totalPromptTokens + m.totalCandidateTokens
		tokensChip = theme.Chip(p.TokenIn).Render(fmt.Sprintf("in %d out %d total %d", m.totalPromptTokens, m.totalCandidateTokens, total))
	} else {
		tokensChip = theme.Chip(p.TokenIn).Render("tokens —")
	}

	charCount := len([]rune(m.textInput.Value()))
	limit := m.textInput.CharLimit
	counterBg := p.Accent
	if limit > 0 {
		ratio := float64(charCount) / float64(limit)
		switch {
		case ratio >= 1.0:
			counterBg = lipgloss.Color("#e64553") // over limit — red
		case ratio >= 0.9:
			counterBg = lipgloss.Color("#f9e2af") // near limit — amber
		}
	}
	counterChip := theme.Chip(counterBg).Render(fmt.Sprintf("%d/%d", charCount, limit))

	var scrollChip string
	if !m.viewport.AtBottom() {
		pct := 0
		if m.viewport.TotalLineCount() > 0 {
			pct = int(m.viewport.ScrollPercent() * 100)
		}
		scrollChip = theme.Chip(p.Loading).Render(fmt.Sprintf("▼ %d%%", pct))
	}

	rightChips := scrollChip + counterChip + tokensChip + routeChip + modelChip
	rightW := lipgloss.Width(rightChips)

	hintW := m.width - rightW
	if hintW < 1 {
		hintW = 1
	}

	// ── Help-widget (plain text, not a chip) — opencode's own convention ──
	var hint string
	switch {
	case m.statusMsg != "":
		hint = "✓ " + m.statusMsg
	case m.loading:
		hint = "ctrl+c / esc: interrupt"
	case len(m.selectedFiles) > 0:
		hint = fmt.Sprintf("📎 %d file(s) ready  •  /filepicker: add more  •  ctrl+c: quit", len(m.selectedFiles))
	default:
		hint = "↑/↓: history/scroll  •  @path: attach  •  / for commands  •  ctrl+c: quit"
	}
	// Width+MaxWidth alone would word-WRAP long hint text across multiple
	// lines (Width triggers wrap; MaxWidth only caps each wrapped line's
	// length, it doesn't prevent wrapping) — Inline(true) forces single-line
	// rendering while still padding short content and truncating long
	// content to exactly hintW, which a single-line status bar requires.
	hintRendered := s.Help.Width(hintW).MaxWidth(hintW).Inline(true).Render(hint)

	line := hintRendered + rightChips

	// Pad to full terminal width so no dark strip appears on light themes.
	// layout.FillLines appends bg-coloured spaces rather than plain ones
	// (Width().Render() uses plain spaces which show as terminal-default
	// black on light themes).
	return layout.FillLines(line, m.width, s.ChromeBg)
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
func (m chatModel) settingsFooter(s theme.StyledSet) string {
	hint := "enter/space: select  •  ↑/↓: navigate  •  esc: cancel  •  ctrl+c: quit"
	rendered := s.Help.MaxWidth(m.width).Render(hint)
	return layout.FillLines(rendered, m.width, s.ChromeBg)
}

// loginView renders the /login form — see login.go's buildLoginForm.
func (m chatModel) loginView() string {
	if m.loginForm == nil {
		return ""
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.viewport.Height).
		Render(m.loginForm.View())
}

// loginFooter renders a one-line hint bar shown below the /login form.
func (m chatModel) loginFooter(s theme.StyledSet) string {
	hint := "enter/space: select  •  ↑/↓: navigate  •  esc: cancel  •  ctrl+c: quit"
	rendered := s.Help.MaxWidth(m.width).Render(hint)
	return layout.FillLines(rendered, m.width, s.ChromeBg)
}

// ── Viewport content ──────────────────────────────────────────────────────────

// refreshViewport re-renders the viewport content.
//
// Scrolling strategy:
//   - Help overlay  → always GotoBottom (content is short)
//   - Loading/streaming → GotoBottom so new chunks stream into view
//   - After a completed agent reply → scrollToLastMessage so the START of the
//
// setViewportContent fills every line to m.width with the theme background
// before handing content to the viewport.  This is required because the
// viewport's internal lipgloss.NewStyle().Width(contentWidth).Render() pads
// short lines with spaces that carry no background colour — so on light themes
// those cells expose terminal-default (black).  By pre-filling here we ensure
// every stored line is already full-width with the correct bg.
func (m *chatModel) setViewportContent(s theme.StyledSet, content string) {
	// Fill every line to m.width so trailing cells carry the theme bg colour
	// (the viewport's inner lipgloss.NewStyle().Width() pads with no bg).
	filled := layout.FillLines(content, m.width, s.ChromeBg)
	// Note: do NOT pad filled to viewport.Height with blank lines here.
	// The viewport's own Style (= s.ViewBg, set in View()) wraps all rendered
	// content — including the empty rows lipgloss.Height() adds — in the theme
	// background.  Adding extra blank lines here causes GotoBottom to scroll
	// PAST the actual content into the blank fill area, making responses look
	// cropped when the content is shorter than the viewport height.
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
	p := theme.BuiltinThemes[m.themeIdx]

	if p.Bg != lipgloss.Color("") {
		// Light theme: set Base background + paint the internal viewport with
		// the same bg so every cell (including right-side trailing space) is
		// filled.  Base.Width is NOT set — we rely on the viewport Style instead.
		baseBg := lipgloss.NewStyle().Background(p.Bg)
		m.textInput.FocusedStyle.Base = baseBg
		m.textInput.BlurredStyle.Base = baseBg
		// Internal viewport: paints trailing cells on each line with p.Bg.
		setTextareaViewportStyle(&m.textInput, lipgloss.NewStyle().Background(p.Bg))
		// EndOfBuffer tilde: fg only (inline anyway).
		m.textInput.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(p.System)
		m.textInput.BlurredStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(p.System)
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
	textFg := lipgloss.NewStyle().Foreground(p.Text)
	phFg := lipgloss.NewStyle().Foreground(p.System)
	m.textInput.FocusedStyle.Text = textFg
	m.textInput.BlurredStyle.Text = textFg
	m.textInput.FocusedStyle.Placeholder = phFg
	m.textInput.BlurredStyle.Placeholder = phFg
}

//	reply is visible; the user can scroll down to read code blocks below
//
// - Everything else (user msg, error, system) → GotoBottom
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
		if i > 0 && m.msgs[i-1].role != msg.role {
			before.WriteByte('\n')
		}
		switch msg.role {
		case "user":
			body := s.UserLabel.PaddingLeft(2).Render("You") + "\n" + s.UserText.PaddingLeft(2).Width(contentW).Render(msg.text)
			before.WriteString(s.UserAccent.Render(body) + "\n")
		case "agent":
			body := s.AgentLabel.PaddingLeft(2).Render("Agent") + "\n" + indentBlock(renderMarkdown(s, msg.text, contentW, s.AgentText), 2)
			before.WriteString(wrapAccent(s.AgentAccent, body))
		case "error":
			wrapped := layout.HardWrapText(msg.text, contentW-2)
			body := s.ErrorLabel.PaddingLeft(2).Render("Error") + "\n" + s.ErrorText.PaddingLeft(2).Width(contentW).Render(wrapped)
			before.WriteString(s.ErrorAccent.Render(body) + "\n")
		case "system":
			before.WriteString(s.System.PaddingLeft(2).Width(w).Render(msg.text) + "\n")
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
	lastBody := s.AgentLabel.PaddingLeft(2).Render("Agent") + "\n" + indentBlock(renderMarkdown(s, lastMsg.text, contentW, s.AgentText), 2)
	lastBlock := wrapAccent(s.AgentAccent, lastBody)
	lastH := strings.Count(lastBlock, "\n") + 1

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
func (m chatModel) helpView(s theme.StyledSet) string {
	var sb strings.Builder

	sb.WriteString(s.UserLabel.Render("Keyboard shortcuts") + "\n\n")

	bindings := [][2]string{
		{"enter", "Send message (with any attached files)"},
		{"/settings", "Open settings overlay (theme, char limit)"},
		{"/login", "Add/switch provider credentials"},
		{"/model", "Switch model or provider (2-level picker)"},
		{"/providers", "Show provider chain & last route"},
		{"/filepicker", "Browse & attach files (bubbles filepicker)"},
		{"/acp", "Start ACP server (Agent Client Protocol) for IDE integration"},
		{"/acpstop", "Stop the ACP server"},
		{"/help", "Toggle this help overlay"},
		{"/clear", "Clear conversation history"},
		{"/theme", "Cycle colour theme"},
		{"/skills", "List available agent skills"},
		{"@path/to/file", "Inline attach a file — content sent as context"},
		{"#tag", "Add a tag to your message"},
		{"↑ / ctrl+p", "History: prev message (from empty OR with ctrl+p)"},
		{"↓ / ctrl+n", "History: next message / restore draft"},
		{"? / esc", "Toggle help (when input empty)"},
		{"ctrl+l", "Clear conversation history"},
		{"ctrl+s", "Save conversation → ~/.go-adk-q/session.json"},
		{"ctrl+y", "Copy last agent reply to clipboard"},
		{"ctrl+t", "Toggle copy/scroll mode"},
		{"shift+drag", "Select and copy any text (native; no mouse mode needed)"},
		{"ctrl+c", "Quit"},
		{"pgup / pgdn", "Scroll message history"},
	}
	for _, b := range bindings {
		k := s.Prompt.Render(fmt.Sprintf("  %-16s", b[0]))
		desc := s.System.Render(b[1])
		sb.WriteString(k + "  " + desc + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(s.AgentLabel.Render("Colour themes") + "\n\n")

	badges := make([]string, len(theme.BuiltinThemes))
	for i, t := range theme.BuiltinThemes {
		marker := "○"
		if i == m.themeIdx {
			marker = "●"
		}
		badges[i] = lipgloss.NewStyle().
			Bold(i == m.themeIdx).
			Foreground(t.OnChrome).
			Background(t.Chrome).
			Padding(0, 1).
			Render(marker + " " + t.Name)
	}
	// Gaps between badges must carry the chrome background so they don't show
	// terminal-default (black) on light themes.
	gap := s.ChromeBg.Render("  ")
	sb.WriteString(gap + strings.Join(badges, gap) + "\n\n")
	sb.WriteString(s.System.Render("  /theme to cycle  •  /settings for full settings  •  /help or esc to close") + "\n")

	return sb.String()
}

// renderMessages converts the chatMsg slice into styled, word-wrapped output
// for the viewport.
//
// Layout per message — opencode/crush's colored-left-border-accent
// convention: no You/Agent/Error text label, no timestamp, just a per-role
// colored bar on the left edge of the message block:
//
//	User   →  "│ <wrapped text>\n"          (border colored per theme's User)
//	Agent  →  "│ <markdown-rendered text>\n" (border colored per theme's Agent)
//	Error  →  "│ <wrapped text>\n"          (border colored per theme's ErrC)
//	System →  "  <italic text>\n"            (no border — unchanged)
//
// While loading and streaming text has already arrived, the in-progress agent
// reply is appended live (no spinner, just text).  Before the first chunk the
// spinner "Thinking…" line is shown instead.
//
// contentW leaves room for the border accent (1 col) plus glamour's own 2-col
// left margin. Falls back to 80 columns before the first WindowSizeMsg is
// received.
func (m *chatModel) renderMessages(s theme.StyledSet) string {
	w := m.width
	if w < 20 {
		w = 80
	}
	contentW := w - 4 // leave glamour room for its 2-col left margin

	if len(m.msgs) == 0 && !m.loading {
		return s.System.Width(w).Render("  No messages yet.")
	}

	// Completed messages are immutable once appended, so their rendered
	// form (glamour parse + border wrap) is cached and reused across every
	// streaming chunk tick instead of re-rendering the whole history per
	// token — that O(n)-per-chunk cost is what made replies feel late as a
	// conversation grew.
	if m.completedRenderCacheCount != len(m.msgs) || m.completedRenderCacheWidth != w || m.completedRenderCacheTheme != s.ThemeIdx {
		m.completedRenderCache = renderCompletedMessages(m.msgs, s, contentW)
		m.completedRenderCacheCount = len(m.msgs)
		m.completedRenderCacheWidth = w
		m.completedRenderCacheTheme = s.ThemeIdx
	}

	var sb strings.Builder
	sb.WriteString(m.completedRenderCache)

	// In-progress agent response while the goroutine is still running.
	if m.loading {
		if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].role != "agent" {
			sb.WriteString("\n")
		}
		if m.streamingText != "" {
			// Show partial text as it arrives (streaming).
			body := s.AgentLabel.PaddingLeft(2).Render("Agent") + "\n" + indentBlock(renderMarkdown(s, m.streamingText, contentW, s.AgentText), 2)
			sb.WriteString(wrapAccent(s.AgentAccent, body))
		} else {
			// No text yet — show the spinner.
			sb.WriteString(s.Loading.Width(w).Render("  "+m.spinner.View()+" Thinking…") + "\n")
		}
	}

	return sb.String()
}

// renderCompletedMessages renders the fixed portion of conversation history
// (every message already appended to msgs, no in-progress streaming tail).
// Extracted from renderMessages so its result can be cached: this is the
// exact same rendering the loop used to redo, from scratch, on every
// streaming chunk.
func renderCompletedMessages(msgs []chatMsg, s theme.StyledSet, contentW int) string {
	var sb strings.Builder
	for i, msg := range msgs {
		// Tighter grouping: a blank separator line only appears between
		// messages of DIFFERENT roles. Consecutive same-role messages (e.g.
		// several system announcements in a row) sit flush against each
		// other — each still has its own border/style, so they read as a
		// grouped run rather than losing separation entirely.
		if i > 0 && msgs[i-1].role != msg.role {
			sb.WriteString("\n")
		}
		switch msg.role {
		case "user":
			body := s.UserLabel.PaddingLeft(2).Render("You") + "\n" + s.UserText.PaddingLeft(2).Width(contentW).Render(msg.text)
			sb.WriteString(s.UserAccent.Render(body) + "\n")

		case "agent":
			body := s.AgentLabel.PaddingLeft(2).Render("Agent") + "\n" + indentBlock(renderMarkdown(s, msg.text, contentW, s.AgentText), 2)
			sb.WriteString(wrapAccent(s.AgentAccent, body))

		case "error":
			// Hard-wrap before rendering: error messages contain URLs and JSON
			// with no spaces, which lipgloss's word-wrap can't break.
			wrapped := layout.HardWrapText(msg.text, contentW-2)
			body := s.ErrorLabel.PaddingLeft(2).Render("Error") + "\n" + s.ErrorText.PaddingLeft(2).Width(contentW).Render(wrapped)
			sb.WriteString(s.ErrorAccent.Render(body) + "\n")

		case "system":
			// System messages may contain markdown (announcements, skill lists, etc.)
			// Use glamour so **bold**, `code`, lists etc. render properly.
			sb.WriteString(renderMarkdown(s, msg.text, contentW, s.System))
		}
	}
	return sb.String()
}

// wrapAccent wraps an already-rendered, newline-terminated message block
// (from renderMarkdown) with a left-border accent style, trimming the
// trailing newline first so lipgloss's border measurement doesn't count a
// spurious empty final line, then restoring exactly one trailing newline.
func wrapAccent(accent lipgloss.Style, renderedBlock string) string {
	return accent.Render(strings.TrimSuffix(renderedBlock, "\n")) + "\n"
}

// indentBlock prepends n plain spaces to every non-empty line of an
// already-rendered (possibly ANSI-styled) block. glamour's Document margin
// is 0 across every theme in glamourStyleConfig (chosen so nested block
// elements like lists — which already carry their own Margin:2 — don't get
// double-indented), so a plain-prose agent reply with no list/blockquote
// renders flush left with zero indent. That reads fine standalone but
// collides visually with the border-accent bar now that agent replies are
// wrapped in one — this restores the same 2-space gap user/error text gets
// via its own explicit PaddingLeft(2), without changing glamour's shared
// config (which would also re-indent lists/code, a much bigger blast radius).
func indentBlock(rendered string, n int) string {
	if n <= 0 {
		return rendered
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// ── Agent streaming ───────────────────────────────────────────────────────────

// switchModelCmd creates a tea.Cmd that rebuilds the full failover chain with
// the chosen provider as primary (and the other configured providers as
// fallbacks) in a background goroutine, then delivers a switchModelMsg.
//
// It deliberately does NOT drop to a single provider: switching models via
// /model keeps the entire fallback safety net intact. sessionSvc and
// memorySvc are captured from the running chatModel so the new runner reuses
// the same in-memory session history.
func switchModelCmd(providerID, modelID string, sessionSvc session.Service, memorySvc memory.Service) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		// Build a FULL failover chain; chain.Build honours the provider/model
		// selection and keeps every other configured provider as a fallback.
		fm, err := chain.Build(ctx, chain.WithSelected(providerID, modelID))
		if err != nil {
			return switchModelMsg{err: err}
		}
		r, err := rebuildRunnerWithModel(ctx, fm, sessionSvc, memorySvc)
		if err != nil {
			return switchModelMsg{err: fmt.Errorf("rebuild runner: %w", err)}
		}
		return switchModelMsg{newRunner: r, newModelName: fm.Name(), failoverModel: fm}
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
// a tea.Cmd that will deliver the first streamChunkMsg to the Update loop,
// plus a cancel func that stops the goroutine (context.Canceled → interrupted).
func (m chatModel) startAgentStream(input string) (tea.Cmd, context.CancelFunc) {
	r := m.runner
	userID := m.userID
	sessionID := m.sessionID
	sessionSvc := m.sessionSvc
	memorySvc := m.memorySvc

	ctx, cancel := context.WithCancel(context.Background())

	// Buffered channel so the goroutine is never blocked by a slow UI frame.
	ch := make(chan streamChunkMsg, 64)

	go func() {
		defer close(ch)

		// Harness observability: additive slog events around the agent turn,
		// same $TMPDIR/go-adk-q-tui.log sink as every other log line in this
		// file. No new UI plumbing — see SESSION_HANDOFF.md for why the
		// tea.Msg/visible-event-pane version of this was scoped out.
		slog.Info("agent_turn", "kind", "AgentStarted", "session", sessionID)
		defer slog.Info("agent_turn", "kind", "AgentFinished", "session", sessionID)

		// lastUsage holds the most recent UsageMetadata seen; included in the
		// final done message so the UI can display token counts. Shared across
		// every runTurn call below (a confirmation pause resumes as a new
		// r.Run call, but usage should still reflect the whole exchange).
		var promptToks, candidateToks int32

		// The turn-driving loop itself — including pausing on an ADK
		// Human-in-the-Loop tool confirmation (toolconfirmation.FunctionCallName;
		// exec_command, tools/exec.go, is the only tool that sets
		// RequireConfirmation today) and resuming once the human answers — is
		// shared with the ACP bridges in main.go; see agent_turn.go.
		_, promptToks, candidateToks, turnErr := runTurnWithConfirmations(
			ctx, r, userID, sessionID, input,
			func(delta string) { ch <- streamChunkMsg{text: delta} },
			func(ctx context.Context, callID, toolName string, args map[string]any) (bool, error) {
				respCh := make(chan bool, 1)
				ch <- streamChunkMsg{permission: &permissionRequest{
					toolName: toolName,
					args:     args,
					hint:     fmt.Sprintf("Approve running %s?", toolName),
					callID:   callID,
					resp:     respCh,
				}}
				select {
				case confirmed := <-respCh:
					slog.Info("tool_confirmation", "kind", "ToolConfirmation", "tool", toolName, "confirmed", confirmed)
					return confirmed, nil
				case <-ctx.Done():
					return false, ctx.Err()
				}
			},
		)
		if turnErr != nil {
			ch <- streamChunkMsg{err: turnErr, done: true}
			return
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

	return nextChunk(ch), cancel
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

// runAgentSync drives the ADK runner to completion and returns the full text
// response as a string.  It mirrors startAgentStream but runs synchronously
// on the calling goroutine — used by the ACP server bridge so that HTTP
// handlers can call the agent without interacting with the BubbleTea loop.
//
// It has no confirmation handler: the HTTP-only ACP transport (acp_server.go)
// is plain request/response and cannot support an Agent→Client
// session/request_permission round trip mid-turn (see that file's header).
// If the agent pauses on a confirmation-required tool (e.g. exec_command)
// over this transport, runTurnWithConfirmations returns a clear error instead
// of hanging forever. The real stdio ACP transport (acp command, main.go)
// uses runTurnWithConfirmations directly with a working confirmation handler.
func runAgentSync(ctx context.Context, r *runner.Runner, userID, sessionID, input string) (string, error) {
	text, _, _, err := runTurnWithConfirmations(ctx, r, userID, sessionID, input, nil, nil)
	if err != nil {
		return strings.TrimSpace(text), err
	}
	return strings.TrimSpace(text), nil
}

// ── Entry point ───────────────────────────────────────────────────────────────

// runChat starts the Bubbletea program.  Called by the 'chat' Cobra subcommand.
func runChat(r *runner.Runner, svc session.Service, memorySvc memory.Service, modelName string, fm *failover.Model) error {
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

	m := newChatModel(r, svc, memorySvc, modelName, activeProviderIDs, fm)
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

	// ── @ file autocomplete menu — intercept navigation keys ────────────────
	// Must run before the generic key switch so ↑/↓/Tab/Esc go to the menu
	// and not to history navigation or the textarea when the menu is open.
	if m.atFileActive {
		filter, _ := extractAtFilter(m.textInput.Value())
		filtered := filterAtFileItems(m.atFileItems, filter)
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.atFileActive = false
			m.atFileIdx = 0
			return m, nil
		case "up", "ctrl+p":
			// Wrap to the bottom item from the top, matching normal
			// dropdown/select navigation instead of stopping dead at index 0.
			if n := len(filtered); n > 0 {
				m.atFileIdx = (m.atFileIdx - 1 + n) % n
			}
			return m, nil
		case "down", "ctrl+n":
			if n := len(filtered); n > 0 {
				m.atFileIdx = (m.atFileIdx + 1) % n
			}
			return m, nil
		case "tab", "shift+tab":
			// Tab cycles through items without closing the menu; wraps the
			// same way up/down do.
			if n := len(filtered); n > 0 {
				if msg.String() == "shift+tab" {
					m.atFileIdx = (m.atFileIdx - 1 + n) % n
				} else {
					m.atFileIdx = (m.atFileIdx + 1) % n
				}
			}
			return m, nil
		case "enter":
			// Select the highlighted item and close the menu.
			if len(filtered) > 0 {
				idx := m.atFileIdx
				if idx >= len(filtered) {
					idx = 0
				}
				newVal := replaceAtFilter(m.textInput.Value(), filtered[idx])
				m.textInput.SetValue(newVal)
				m.textInput.CursorEnd()
				if m.width > 6 {
					m.textInput.SetHeight(layout.CalcInputHeight(m.textInput.Value(), m.width-4, 5))
				}
			}
			m.atFileActive = false
			m.atFileIdx = 0
			return m, nil
		default:
			// Any other key (typing): let it fall through to normal key
			// handling so the textarea gets updated and we re-filter.
		}
	}

	switch msg.String() {

	// ── ctrl+c: quit ──────────────────────────────────────────────
	case "ctrl+c":
		// ctrl+c while agent is running → interrupt the request.
		// ctrl+c when idle → quit (standard shell behaviour).
		if m.loading && m.cancelFn != nil {
			m.cancelFn()
			m.cancelFn = nil
			return m, nil
		}
		return m, tea.Quit

	case "esc":
		// esc while agent is running → interrupt (same as ctrl+c).
		if m.loading && m.cancelFn != nil {
			m.cancelFn()
			m.cancelFn = nil
			return m, nil
		}
		// Close @ file menu if open.
		if m.atFileActive {
			m.atFileActive = false
			m.atFileIdx = 0
			return m, nil
		}
		// Close slash menu if open.
		if dialog.SlashMenuVisible(m.textInput.Value()) {
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
				layout.OneShotTimer(2*time.Second, statusClearMsg{}),
				func() tea.Msg { return tea.EnableMouseCellMotion() },
			)
		}
		m.statusMsg = "Copy mode — select text freely  (ctrl+t to scroll)"
		return m, tea.Batch(
			layout.OneShotTimer(3*time.Second, statusClearMsg{}),
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
			cmds = append(cmds, layout.OneShotTimer(3*time.Second, statusClearMsg{}))
			m.refreshViewport()
		}
		return m, tea.Batch(cmds...)

	// ── ctrl+y: copy last agent reply ─────────────────────────────
	case "ctrl+y":
		for i := len(m.msgs) - 1; i >= 0; i-- {
			if m.msgs[i].role == "agent" {
				content, label := smartCopy(m.msgs[i].text)
				if err := layout.CopyToClipboard(content); err != nil {
					m.statusMsg = "Copy failed: " + err.Error()
				} else {
					m.statusMsg = label
				}
				cmds = append(cmds, layout.OneShotTimer(2*time.Second, statusClearMsg{}))
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

	// ── ↑ / ctrl+p : slash menu nav OR history browse (oldest first) ──────
	case "up", "ctrl+p":
		val := m.textInput.Value()
		if dialog.SlashMenuVisible(val) {
			if m.slashMenuIdx > 0 {
				m.slashMenuIdx--
			}
			return m, nil
		}
		// Enter history-browse mode:
		//  • always when input is empty
		//  • always when already browsing (historyIdx != -1)
		//  • also on ctrl+p even when input has text (shell ctrl+p behaviour)
		wantBrowse := !m.loading && len(m.inputHistory) > 0 &&
			(m.historyIdx != -1 || val == "" || msg.String() == "ctrl+p")
		if wantBrowse {
			if m.historyIdx == -1 {
				// First entry — save the current draft so ctrl+n can restore it.
				m.inputDraft = val
				m.historyIdx = len(m.inputHistory) - 1
			} else if m.historyIdx > 0 {
				// Navigate toward the oldest entry.
				m.historyIdx--
			}
			// At historyIdx==0: stay at the oldest entry (clamp; don't wrap).
			m.textInput.SetValue(m.inputHistory[m.historyIdx])
			m.textInput.CursorEnd()
			if m.width > 6 {
				m.textInput.SetHeight(layout.CalcInputHeight(m.textInput.Value(), m.width-4, 5))
			}
			return m, nil
		}
		// Non-empty, not ctrl+p, not browsing → textarea handles cursor-up.

	// ── ↓ / ctrl+n : slash menu nav OR history forward (back to draft) ────
	case "down", "ctrl+n":
		val := m.textInput.Value()
		if dialog.SlashMenuVisible(val) {
			matches := dialog.SlashMatches(val)
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
				// Past the end of history — restore the saved draft.
				m.historyIdx = -1
				m.textInput.SetValue(m.inputDraft)
				m.inputDraft = ""
			}
			m.textInput.CursorEnd()
			if m.width > 6 {
				m.textInput.SetHeight(layout.CalcInputHeight(m.textInput.Value(), m.width-4, 5))
			}
			return m, nil
		}
		// Not browsing → textarea handles cursor-down.

	// ── tab: complete slash command ───────────────────────────────
	case "tab":
		val := m.textInput.Value()
		if dialog.SlashMenuVisible(val) {
			matches := dialog.SlashMatches(val)
			idx := m.slashMenuIdx
			if idx >= len(matches) {
				idx = 0
			}
			m.textInput.SetValue(matches[idx].Name)
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
		if dialog.SlashMenuVisible(val) {
			matches := dialog.SlashMatches(val)
			// A command that is itself a valid name (e.g. "/acp") but is also
			// a strict prefix of another (e.g. "/acpstop") produced >1 matches
			// forever, so Enter never fell through to execute it — typing the
			// exact name and pressing Enter silently no-op'd. If val already
			// equals one of the candidates exactly, treat it as resolved
			// regardless of how many longer commands also share the prefix.
			exact := false
			for _, mtch := range matches {
				if strings.EqualFold(mtch.Name, val) {
					exact = true
					break
				}
			}
			if !exact && len(matches) > 1 {
				idx := m.slashMenuIdx
				if idx >= len(matches) {
					idx = 0
				}
				m.textInput.SetValue(matches[idx].Name)
				m.textInput.CursorEnd()
				m.slashMenuIdx = 0
				return m, nil
			}
			if !exact && len(matches) == 1 {
				m.textInput.SetValue(matches[0].Name)
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

		case "/login":
			m.textInput.Reset()
			m.loginResult = &loginFormResult{}
			m.loginAwaitingKey = false
			m.loginForm = buildAuthMethodForm(m.loginResult).
				WithWidth(m.width - 4)
			m.loginMode = true
			return m, m.loginForm.Init()

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
				text: dialog.ListSkillsSummary(),
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

		case "/providers":
			m.textInput.Reset()
			m.showHelp = false
			if m.failoverModel == nil {
				m.msgs = append(m.msgs, chatMsg{role: "system", text: "Provider chain unavailable.", at: time.Now()})
			} else {
				m.msgs = append(m.msgs, chatMsg{role: "system", text: m.providersSummary(), at: time.Now()})
			}
			m.refreshViewport()
			return m, nil

		case "/acp":
			m.textInput.Reset()
			acpServerMu.Lock()
			defer acpServerMu.Unlock()
			if acpServerInstance != nil {
				m.statusMsg = fmt.Sprintf("🟢 ACP already running on port %d", acpServerInstance.Port())
				cmds = append(cmds, layout.OneShotTimer(3*time.Second, statusClearMsg{}))
				return m, tea.Batch(cmds...)
			}
			// Capture runner + session at start time.  The ACP session is kept
			// separate from the TUI session so concurrent requests don’t mix.
			capturedRunner := m.runner
			capturedUserID := m.userID
			acpSessionID := "acp-" + m.sessionID
			bridge := func(ctx context.Context, input string) (string, error) {
				// Hard timeout: prevent runaway agent calls from blocking the HTTP handler forever.
				ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
				defer cancel()
				return runAgentSync(ctx, capturedRunner, capturedUserID, acpSessionID, input)
			}
			srv := newACPServer(bridge)
			if err := srv.Start(0); err != nil {
				m.statusMsg = "ACP start failed: " + err.Error()
			} else {
				acpServerInstance = srv
				m.msgs = append(m.msgs, chatMsg{
					role: "system",
					text: fmt.Sprintf("🟢 **ACP server started** on `http://127.0.0.1:%d/acp`\n\n"+
						"Spec-conformant methods (real ACP names/shapes): `initialize` `session/new` `session/prompt`.\n"+
						"Non-spec extension: `message/stream` (SSE, session/update-shaped frames — see acp_server.go header).\n"+
						"Not implemented (needs a bidirectional transport — stdio/WebSocket, not this HTTP server): "+
						"`fs/read_text_file` `fs/write_text_file` `terminal/*` `session/request_permission`.\n"+
						"Stop with `/acpstop`.", srv.Port()),
					at: time.Now(),
				})
				m.refreshViewport()
			}
			cmds = append(cmds, layout.OneShotTimer(3*time.Second, statusClearMsg{}))
			return m, tea.Batch(cmds...)

		case "/acpstop":
			m.textInput.Reset()
			acpServerMu.Lock()
			defer acpServerMu.Unlock()
			if acpServerInstance == nil {
				m.statusMsg = "ACP server is not running"
			} else {
				_ = acpServerInstance.Stop()
				acpServerInstance = nil
				m.statusMsg = "🔴 ACP server stopped"
				m.msgs = append(m.msgs, chatMsg{
					role: "system",
					text: "🔴 ACP server stopped.",
					at:   time.Now(),
				})
				m.refreshViewport()
			}
			cmds = append(cmds, layout.OneShotTimer(3*time.Second, statusClearMsg{}))
			return m, tea.Batch(cmds...)

		case "/filepicker":
			m.textInput.Reset()
			m.showHelp = false
			cwd, _ := os.Getwd()
			fp := filepicker.New()
			fp.CurrentDirectory = cwd
			fp.ShowHidden = false
			fp.DirAllowed = false
			fp.FileAllowed = true
			fp.AutoHeight = false
			fp.Height = m.height - 8
			if fp.Height < 5 {
				fp.Height = 5
			}
			m.filePickerState = filePickerState{
				fp:      fp,
				showing: true,
			}
			m.filePickerMode = true
			return m, fp.Init()

		} // end switch strings.ToLower(input) — no slash command matched

		// Not a slash command — send to agent.
		m.showHelp = false
		// ── Process @path file attachments from inline text ────────────────────
		processedInput, inlineAttachments, missingPaths, tags := processInputForFilesAndTags(input)
		_ = tags // tags noted but not yet surfaced as metadata

		// Warn about @paths that could not be resolved.
		if len(missingPaths) > 0 {
			cwd, _ := os.Getwd()
			m.statusMsg = fmt.Sprintf("⚠️ File not found: %s — CWD is %s",
				strings.Join(missingPaths, ", "), cwd)
			cmds = append(cmds, layout.OneShotTimer(6*time.Second, statusClearMsg{}))
		}

		// Merge /filepicker-selected files + inline @path files, then drop any
		// that exceed maxAttachmentSize (F10) before they ever reach the
		// display message or the content-reading loop below.
		mergedAttachments := append(m.selectedFiles, inlineAttachments...)
		m.selectedFiles = nil // clear after consumption
		allAttachments, oversizedPaths := splitAttachmentsBySize(mergedAttachments)
		if len(oversizedPaths) > 0 {
			warn := fmt.Sprintf("⚠️ Skipped (over %dKB): %s", maxAttachmentSize/1024, strings.Join(oversizedPaths, ", "))
			if m.statusMsg != "" {
				m.statusMsg += "  " + warn
			} else {
				m.statusMsg = warn
			}
			cmds = append(cmds, layout.OneShotTimer(6*time.Second, statusClearMsg{}))
		}

		// Build the display message shown in the chat bubble.
		var messageToSend string
		if len(allAttachments) > 0 {
			attachNames := make([]string, 0, len(allAttachments))
			for _, a := range allAttachments {
				attachNames = append(attachNames, filepath.Base(a))
			}
			messageToSend = processedInput + "\n\n📎 *Attached: " + strings.Join(attachNames, ", ") + "*"
		} else {
			messageToSend = processedInput
		}

		// Build the agent input: append file contents as fenced code blocks.
		agentInput := processedInput
		if len(allAttachments) > 0 {
			var attachBuf strings.Builder
			attachBuf.WriteString(processedInput)
			for _, path := range allAttachments {
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				ext := strings.TrimPrefix(filepath.Ext(path), ".")
				if ext == "" {
					ext = "text"
				}
				attachBuf.WriteString(fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```", filepath.Base(path), ext, string(content)))
			}
			agentInput = attachBuf.String()
		}

		if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != input {
			m.inputHistory = append(m.inputHistory, input)
		}
		m.historyIdx = -1
		m.inputDraft = ""
		m.atFileActive = false // close @ menu on send
		m.atFileIdx = 0
		m.msgs = append(m.msgs, chatMsg{role: "user", text: messageToSend, at: time.Now()})
		m.textInput.Reset()
		m.loading = true
		m.refreshViewport()
		streamCmd, cancelFn := m.startAgentStream(agentInput)
		m.cancelFn = cancelFn
		cmds = append(cmds, m.spinner.Tick, streamCmd)
		return m, tea.Batch(cmds...)

	} // end case "enter" / end switch msg.String()

	// All other keys (printable chars, backspace, arrows when input non-empty,
	// etc.) are forwarded to the textarea widget so typing works normally.
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	// Keep textarea height in sync with content.
	if m.ready && m.width > 6 {
		m.textInput.SetHeight(layout.CalcInputHeight(m.textInput.Value(), m.width-4, 5))
	}

	// ── @ file menu: activate / deactivate based on new textarea value ──────
	if !m.loading {
		val := m.textInput.Value()
		_, hasAt := extractAtFilter(val)
		if hasAt {
			// Scan files once per working directory; reuse cache otherwise.
			cwd, _ := os.Getwd()
			if !m.atFileActive || m.atFileCwd != cwd {
				m.atFileItems = loadAtFileItems(cwd)
				m.atFileCwd = cwd
				m.atFileIdx = 0
			}
			m.atFileActive = true
			// Reset index when filter text changes so cursor stays at top.
			m.atFileIdx = 0
		} else {
			if m.atFileActive {
				m.atFileActive = false
				m.atFileIdx = 0
			}
		}
	}

	return m, tea.Batch(cmds...)
}
