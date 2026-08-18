// Package theme holds go-adk-q's colour-palette system: the semantic
// design-token set for one visual theme (Palette), the ordered built-in
// theme list (BuiltinThemes), and the pre-built lipgloss style set derived
// from a palette (StyledSet, via MakeStyles).
//
// Extracted from cmd/tui's monolithic package as the first step of the
// opencode-style package split (theme/layout/components/{chat,core,dialog}) —
// mirrors opencode-ai/opencode's internal/tui/theme package. This is a pure
// leaf: it imports only lipgloss and has no dependency on the Bubbletea
// model, so every symbol here that other cmd/tui files reference had to
// become exported — a mechanical, compiler-checked rename, not a behavior
// change.
package theme

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Theme system ───────────────────────────────────────────────────────────────
//
// Palette is a semantic design-token set for one visual theme.  Using
// descriptive role names instead of raw colour indices keeps the rendering
// code readable and makes each theme self-documenting.  Hex truecolor values
// are used for modern terminal emulators (iTerm2, Kitty, Ghostty, WezTerm,
// Alacritty); lipgloss degrades gracefully to the nearest xterm-256 colour on
// older terminals.

type Palette struct {
	Name string

	// ── Chrome ────────────────────────────────────────────────────────────
	// Chrome    = header bar background
	// OnChrome  = text/icons drawn on top of the chrome background
	// Text      = primary body text on the raw terminal background
	// Bg        = viewport/body background (empty = use terminal default)
	Chrome   lipgloss.TerminalColor
	OnChrome lipgloss.TerminalColor
	Text     lipgloss.TerminalColor
	Bg       lipgloss.TerminalColor // explicit fill; empty for transparent dark themes

	// ── Semantic roles ────────────────────────────────────────────────────
	User    lipgloss.TerminalColor // user message label
	Agent   lipgloss.TerminalColor // agent reply label
	Accent  lipgloss.TerminalColor // input border, provider name highlight
	ErrC    lipgloss.TerminalColor // errors (avoids the "error" keyword)
	System  lipgloss.TerminalColor // muted: timestamps, help text, system msgs
	Loading lipgloss.TerminalColor // spinner + "Thinking…"

	// ── Token counter colors ──────────────────────────────────────────────
	TokenIn  lipgloss.TerminalColor // input token count
	TokenOut lipgloss.TerminalColor // output token count

	// ── Code block ────────────────────────────────────────────────────────
	CodeBg     lipgloss.TerminalColor
	CodeFg     lipgloss.TerminalColor
	CodeBorder lipgloss.TerminalColor
	CodeInline lipgloss.TerminalColor
}

// BuiltinThemes is the ordered list of colour themes.  Index 0 (Catppuccin)
// is the default; /theme advances the index cyclically.
//
// All palettes use descriptive role names; dark themes leave Bg empty
// (transparent = terminal default) while light themes set Bg explicitly.
var BuiltinThemes = []Palette{
	// ── 1. Catppuccin Mocha ───────────────────────────────────────────────
	{
		Name:       "Catppuccin",
		Chrome:     lipgloss.Color("#1e1e2e"),
		OnChrome:   lipgloss.Color("#cdd6f4"),
		Text:       lipgloss.Color("#cdd6f4"),
		Bg:         lipgloss.Color(""),        // transparent — use terminal bg
		User:       lipgloss.Color("#89b4fa"), // blue
		Agent:      lipgloss.Color("#a6e3a1"), // green
		Accent:     lipgloss.Color("#89b4fa"), // blue — input border + provider
		ErrC:       lipgloss.Color("#f38ba8"),
		System:     lipgloss.Color("#6c7086"),
		Loading:    lipgloss.Color("#cba6f7"),
		TokenIn:    lipgloss.Color("#89dceb"), // sky
		TokenOut:   lipgloss.Color("#a6e3a1"), // green
		CodeBg:     lipgloss.Color("#181825"),
		CodeFg:     lipgloss.Color("#cdd6f4"),
		CodeBorder: lipgloss.Color("#585b70"),
		CodeInline: lipgloss.Color("#313244"),
	},
	// ── 2. Tokyo Night ────────────────────────────────────────────────────
	{
		Name:       "Tokyo Night",
		Chrome:     lipgloss.Color("#1a1b26"),
		OnChrome:   lipgloss.Color("#c0caf5"),
		Text:       lipgloss.Color("#c0caf5"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#7aa2f7"),
		Agent:      lipgloss.Color("#9ece6a"),
		Accent:     lipgloss.Color("#7aa2f7"),
		ErrC:       lipgloss.Color("#f7768e"),
		System:     lipgloss.Color("#565f89"),
		Loading:    lipgloss.Color("#bb9af7"),
		TokenIn:    lipgloss.Color("#73daca"), // teal
		TokenOut:   lipgloss.Color("#9ece6a"), // green
		CodeBg:     lipgloss.Color("#16161e"),
		CodeFg:     lipgloss.Color("#c0caf5"),
		CodeBorder: lipgloss.Color("#414868"),
		CodeInline: lipgloss.Color("#292e42"),
	},
	// ── 3. Rosé Pine ──────────────────────────────────────────────────────
	{
		Name:       "Rosé Pine",
		Chrome:     lipgloss.Color("#191724"),
		OnChrome:   lipgloss.Color("#e0def4"),
		Text:       lipgloss.Color("#e0def4"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#ebbcba"),
		Agent:      lipgloss.Color("#9ccfd8"),
		Accent:     lipgloss.Color("#c4a7e7"), // iris
		ErrC:       lipgloss.Color("#eb6f92"),
		System:     lipgloss.Color("#6e6a86"),
		Loading:    lipgloss.Color("#c4a7e7"),
		TokenIn:    lipgloss.Color("#ebbcba"), // rose
		TokenOut:   lipgloss.Color("#9ccfd8"), // foam
		CodeBg:     lipgloss.Color("#1f1d2e"),
		CodeFg:     lipgloss.Color("#e0def4"),
		CodeBorder: lipgloss.Color("#403d52"),
		CodeInline: lipgloss.Color("#26233a"),
	},
	// ── 4. Nord ───────────────────────────────────────────────────────────
	{
		Name:       "Nord",
		Chrome:     lipgloss.Color("#2e3440"),
		OnChrome:   lipgloss.Color("#eceff4"),
		Text:       lipgloss.Color("#eceff4"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#88c0d0"),
		Agent:      lipgloss.Color("#a3be8c"),
		Accent:     lipgloss.Color("#88c0d0"), // frost cyan
		ErrC:       lipgloss.Color("#bf616a"),
		System:     lipgloss.Color("#4c566a"),
		Loading:    lipgloss.Color("#81a1c1"),
		TokenIn:    lipgloss.Color("#88c0d0"), // frost 3
		TokenOut:   lipgloss.Color("#a3be8c"), // aurora green
		CodeBg:     lipgloss.Color("#242933"),
		CodeFg:     lipgloss.Color("#eceff4"),
		CodeBorder: lipgloss.Color("#434c5e"),
		CodeInline: lipgloss.Color("#3b4252"),
	},
	// ── 5. Gruvbox ────────────────────────────────────────────────────────
	{
		Name:       "Gruvbox",
		Chrome:     lipgloss.Color("#282828"),
		OnChrome:   lipgloss.Color("#ebdbb2"),
		Text:       lipgloss.Color("#ebdbb2"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#83a598"),
		Agent:      lipgloss.Color("#b8bb26"),
		Accent:     lipgloss.Color("#fabd2f"), // bright yellow
		ErrC:       lipgloss.Color("#fb4934"),
		System:     lipgloss.Color("#928374"),
		Loading:    lipgloss.Color("#d3869b"),
		TokenIn:    lipgloss.Color("#83a598"), // aqua
		TokenOut:   lipgloss.Color("#b8bb26"), // green
		CodeBg:     lipgloss.Color("#1d2021"),
		CodeFg:     lipgloss.Color("#ebdbb2"),
		CodeBorder: lipgloss.Color("#504945"),
		CodeInline: lipgloss.Color("#3c3836"),
	},

	// ── 6. GitHub Light ───────────────────────────────────────────────────
	{
		Name:       "GitHub Light",
		Chrome:     lipgloss.Color("#f6f8fa"),
		OnChrome:   lipgloss.Color("#24292f"),
		Text:       lipgloss.Color("#24292f"),
		Bg:         lipgloss.Color("#ffffff"), // explicit white fill
		User:       lipgloss.Color("#0550ae"),
		Agent:      lipgloss.Color("#116329"),
		Accent:     lipgloss.Color("#0550ae"), // blue border
		ErrC:       lipgloss.Color("#82071e"),
		System:     lipgloss.Color("#57606a"),
		Loading:    lipgloss.Color("#8250df"),
		TokenIn:    lipgloss.Color("#0550ae"), // blue
		TokenOut:   lipgloss.Color("#116329"), // green
		CodeBg:     lipgloss.Color("#f6f8fa"),
		CodeFg:     lipgloss.Color("#24292f"),
		CodeBorder: lipgloss.Color("#d0d7de"),
		CodeInline: lipgloss.Color("#eaeef2"),
	},

	// ── 7. Solarized Light ────────────────────────────────────────────────
	{
		Name:       "Solarized Light",
		Chrome:     lipgloss.Color("#eee8d5"),
		OnChrome:   lipgloss.Color("#657b83"),
		Text:       lipgloss.Color("#657b83"),
		Bg:         lipgloss.Color("#fdf6e3"), // base3 warm cream
		User:       lipgloss.Color("#268bd2"),
		Agent:      lipgloss.Color("#859900"),
		Accent:     lipgloss.Color("#2aa198"), // cyan
		ErrC:       lipgloss.Color("#dc322f"),
		System:     lipgloss.Color("#93a1a1"),
		Loading:    lipgloss.Color("#6c71c4"),
		TokenIn:    lipgloss.Color("#268bd2"), // blue
		TokenOut:   lipgloss.Color("#2aa198"), // cyan
		CodeBg:     lipgloss.Color("#fdf6e3"),
		CodeFg:     lipgloss.Color("#586e75"),
		CodeBorder: lipgloss.Color("#93a1a1"),
		CodeInline: lipgloss.Color("#e8e0c8"),
	},

	// ── 8. Tango (Cyan) ───────────────────────────────────────────────────
	{
		Name:       "Tango",
		Chrome:     lipgloss.Color("#d3eef9"),
		OnChrome:   lipgloss.Color("#204a87"),
		Text:       lipgloss.Color("#2e3436"),
		Bg:         lipgloss.Color("#e8f4fb"), // light cyan fill
		User:       lipgloss.Color("#204a87"),
		Agent:      lipgloss.Color("#4e9a06"),
		Accent:     lipgloss.Color("#3465a4"), // tango blue
		ErrC:       lipgloss.Color("#cc0000"),
		System:     lipgloss.Color("#888a85"),
		Loading:    lipgloss.Color("#75507b"),
		TokenIn:    lipgloss.Color("#3465a4"), // blue
		TokenOut:   lipgloss.Color("#4e9a06"), // green
		CodeBg:     lipgloss.Color("#e8f4fb"),
		CodeFg:     lipgloss.Color("#2e3436"),
		CodeBorder: lipgloss.Color("#a8d8ea"),
		CodeInline: lipgloss.Color("#d3eef9"),
	},

	// ── opencode-ai/opencode palette parity ──────────────────────────────
	// The 6 palettes below are ported from opencode-ai/opencode's
	// internal/tui/theme/*.go (dark-mode hex values, since this repo's
	// Palette struct is single-value per role rather than opencode's
	// dark/light AdaptiveColor pairs). Catppuccin, Tokyo Night, and Gruvbox
	// above already existed here before this session and are left
	// unchanged — opencode ships those three too, so this repo already had
	// 3/9 of opencode's named palettes.

	// ── 9. Dracula ────────────────────────────────────────────────────────
	{
		Name:       "Dracula",
		Chrome:     lipgloss.Color("#282a36"), // background
		OnChrome:   lipgloss.Color("#f8f8f2"), // foreground
		Text:       lipgloss.Color("#f8f8f2"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#bd93f9"), // purple — primary
		Agent:      lipgloss.Color("#50fa7b"), // green — success
		Accent:     lipgloss.Color("#8be9fd"), // cyan — accent
		ErrC:       lipgloss.Color("#ff5555"),
		System:     lipgloss.Color("#6272a4"), // comment
		Loading:    lipgloss.Color("#ff79c6"), // pink — secondary
		TokenIn:    lipgloss.Color("#8be9fd"), // cyan
		TokenOut:   lipgloss.Color("#50fa7b"), // green
		CodeBg:     lipgloss.Color("#21222c"), // background darker
		CodeFg:     lipgloss.Color("#f8f8f2"),
		CodeBorder: lipgloss.Color("#44475a"), // border / current line
		CodeInline: lipgloss.Color("#44475a"), // selection
	},
	// ── 10. Flexoki ───────────────────────────────────────────────────────
	{
		Name:       "Flexoki",
		Chrome:     lipgloss.Color("#100F0F"), // bg (darkest)
		OnChrome:   lipgloss.Color("#B7B5AC"), // tx-3 (dark text)
		Text:       lipgloss.Color("#B7B5AC"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#4385BE"), // blue-400 — primary
		Agent:      lipgloss.Color("#879A39"), // green-400 — success
		Accent:     lipgloss.Color("#DA702C"), // orange-400 — accent
		ErrC:       lipgloss.Color("#D14D41"), // red-400
		System:     lipgloss.Color("#575653"), // tx-2 (muted)
		Loading:    lipgloss.Color("#8B7EC8"), // purple-400 — secondary
		TokenIn:    lipgloss.Color("#3AA99F"), // cyan-400
		TokenOut:   lipgloss.Color("#879A39"), // green-400
		CodeBg:     lipgloss.Color("#1C1B1A"), // bg-2 (dark)
		CodeFg:     lipgloss.Color("#B7B5AC"),
		CodeBorder: lipgloss.Color("#282726"), // ui (dark border)
		CodeInline: lipgloss.Color("#343331"), // ui-2 (dark)
	},
	// ── 11. Monokai Pro ───────────────────────────────────────────────────
	{
		Name:       "Monokai Pro",
		Chrome:     lipgloss.Color("#2d2a2e"),
		OnChrome:   lipgloss.Color("#fcfcfa"),
		Text:       lipgloss.Color("#fcfcfa"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#78dce8"), // cyan — primary
		Agent:      lipgloss.Color("#a9dc76"), // green — success
		Accent:     lipgloss.Color("#fc9867"), // orange — accent
		ErrC:       lipgloss.Color("#ff6188"),
		System:     lipgloss.Color("#727072"), // comment
		Loading:    lipgloss.Color("#ab9df2"), // purple/blue — secondary
		TokenIn:    lipgloss.Color("#78dce8"), // cyan
		TokenOut:   lipgloss.Color("#a9dc76"), // green
		CodeBg:     lipgloss.Color("#221f22"), // background darker
		CodeFg:     lipgloss.Color("#fcfcfa"),
		CodeBorder: lipgloss.Color("#403e41"), // border / current line
		CodeInline: lipgloss.Color("#5b595c"), // selection
	},
	// ── 12. One Dark ──────────────────────────────────────────────────────
	{
		Name:       "One Dark",
		Chrome:     lipgloss.Color("#282c34"),
		OnChrome:   lipgloss.Color("#abb2bf"),
		Text:       lipgloss.Color("#abb2bf"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#61afef"), // blue — primary
		Agent:      lipgloss.Color("#98c379"), // green — success
		Accent:     lipgloss.Color("#d19a66"), // orange — accent
		ErrC:       lipgloss.Color("#e06c75"),
		System:     lipgloss.Color("#5c6370"), // comment
		Loading:    lipgloss.Color("#c678dd"), // purple — secondary
		TokenIn:    lipgloss.Color("#56b6c2"), // cyan
		TokenOut:   lipgloss.Color("#98c379"), // green
		CodeBg:     lipgloss.Color("#21252b"), // background darker
		CodeFg:     lipgloss.Color("#abb2bf"),
		CodeBorder: lipgloss.Color("#3b4048"), // border
		CodeInline: lipgloss.Color("#3e4451"), // selection
	},
	// ── 13. Tron ──────────────────────────────────────────────────────────
	{
		Name:       "Tron",
		Chrome:     lipgloss.Color("#0c141f"),
		OnChrome:   lipgloss.Color("#caf0ff"),
		Text:       lipgloss.Color("#caf0ff"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#00d9ff"), // cyan — primary
		Agent:      lipgloss.Color("#00ff8f"), // green — success
		Accent:     lipgloss.Color("#ff9000"), // orange — accent
		ErrC:       lipgloss.Color("#ff3333"),
		System:     lipgloss.Color("#4d6b87"), // comment
		Loading:    lipgloss.Color("#007fff"), // blue — secondary
		TokenIn:    lipgloss.Color("#00d9ff"), // cyan
		TokenOut:   lipgloss.Color("#00ff8f"), // green
		CodeBg:     lipgloss.Color("#070d14"), // background darker
		CodeFg:     lipgloss.Color("#caf0ff"),
		CodeBorder: lipgloss.Color("#1a2633"), // border / current line / selection
		CodeInline: lipgloss.Color("#1a2633"),
	},
	// ── 14. OpenCode ──────────────────────────────────────────────────────
	// opencode-ai/opencode's own brand theme — the one it ships as default.
	{
		Name:       "OpenCode",
		Chrome:     lipgloss.Color("#212121"),
		OnChrome:   lipgloss.Color("#e0e0e0"),
		Text:       lipgloss.Color("#e0e0e0"),
		Bg:         lipgloss.Color(""),
		User:       lipgloss.Color("#fab283"), // primary orange/gold — brand color
		Agent:      lipgloss.Color("#7fd88f"), // green — success
		Accent:     lipgloss.Color("#9d7cd8"), // accent purple
		ErrC:       lipgloss.Color("#e06c75"),
		System:     lipgloss.Color("#6a6a6a"), // comment
		Loading:    lipgloss.Color("#5c9cf5"), // secondary blue
		TokenIn:    lipgloss.Color("#56b6c2"), // info cyan
		TokenOut:   lipgloss.Color("#7fd88f"), // green
		CodeBg:     lipgloss.Color("#121212"), // background darker
		CodeFg:     lipgloss.Color("#e0e0e0"),
		CodeBorder: lipgloss.Color("#4b4c5c"), // border
		CodeInline: lipgloss.Color("#303030"), // selection
	},
}

// StyledSet is a collection of pre-built lipgloss styles for one theme.
// Code block colours are kept as raw TerminalColor values so the markdown
// renderer can build custom-width box-drawing borders at render time.
type StyledSet struct {
	ThemeIdx   int // index into BuiltinThemes; forwarded to glamour renderer
	Header     lipgloss.Style
	Sep        lipgloss.Style
	UserLabel  lipgloss.Style
	UserText   lipgloss.Style
	AgentLabel lipgloss.Style
	AgentText  lipgloss.Style
	ErrorLabel lipgloss.Style
	ErrorText  lipgloss.Style
	System     lipgloss.Style

	// Left-border accent bars — opencode/crush's per-role message convention
	// (colored left border, no text label, no timestamp). Wraps an already-
	// rendered message block; UserLabel/AgentLabel/ErrorLabel above are kept
	// for helpView's "Keyboard shortcuts"/"Colour themes" section headers,
	// which still use text labels.
	UserAccent  lipgloss.Style
	AgentAccent lipgloss.Style
	ErrorAccent lipgloss.Style
	Loading     lipgloss.Style
	Prompt      lipgloss.Style
	Help        lipgloss.Style

	// Input box border (accent color, rounded corners).
	InputBox lipgloss.Style
	// Full-width background fill for light themes (empty bg = no-op for dark).
	ViewBg lipgloss.Style
	// Full-width background for footer/chrome on light themes (empty = transparent).
	ChromeBg lipgloss.Style
	// Token counter — distinct colors for in/out/total.
	TokenIn    lipgloss.Style
	TokenOut   lipgloss.Style
	TokenTotal lipgloss.Style
	// Provider/model name in footer.
	ProviderName lipgloss.Style

	// Raw colours passed through to the code-block renderer.
	CodeBg     lipgloss.TerminalColor
	CodeFg     lipgloss.TerminalColor
	CodeBorder lipgloss.TerminalColor
	CodeInline lipgloss.Style // inline `code` span style
}

func MakeStyles(p Palette) StyledSet {
	// bg applies the theme's explicit background colour to a style.
	// For dark themes p.Bg is "" — Background("") is a no-op in lipgloss,
	// so dark styles remain transparent (terminal default).
	bg := func(s lipgloss.Style) lipgloss.Style {
		if p.Bg == lipgloss.Color("") {
			return s
		}
		return s.Background(p.Bg)
	}

	// viewBg / chromeBg: plain fill styles used for viewport and chrome areas.
	viewBgStyle := lipgloss.NewStyle()
	chromeBgStyle := lipgloss.NewStyle()
	if p.Bg != lipgloss.Color("") {
		viewBgStyle = lipgloss.NewStyle().Background(p.Bg)
		chromeBgStyle = lipgloss.NewStyle().Background(p.Bg)
	}

	return StyledSet{
		// Header always uses its own chrome colour — not the body bg.
		Header: lipgloss.NewStyle().Bold(true).
			Foreground(p.OnChrome).Background(p.Chrome).Padding(0, 1),

		Sep: bg(lipgloss.NewStyle().Foreground(p.System)),

		UserLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.User)),
		UserText:  bg(lipgloss.NewStyle().Foreground(p.Text)),

		AgentLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.Agent)),
		AgentText:  bg(lipgloss.NewStyle().Foreground(p.Text)),

		ErrorLabel: bg(lipgloss.NewStyle().Bold(true).Foreground(p.ErrC)),
		ErrorText:  bg(lipgloss.NewStyle().Foreground(p.ErrC)),

		// A colored bar on the left edge only (no top/right/bottom border),
		// matching opencode/crush's message convention — see the field doc
		// comment on StyledSet. ThickBorder's left glyph ("┃") reads more
		// clearly as a role-color anchor than NormalBorder's thin "│" now
		// that it's the *only* per-message role signal (no text label). No
		// extra PaddingLeft here: the wrapped content (UserText/AgentText/
		// ErrorText, or renderMarkdown's own baseStyle) already carries its
		// own left indent.
		UserAccent:  bg(lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(p.User)),
		AgentAccent: bg(lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(p.Agent)),
		ErrorAccent: bg(lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(p.ErrC)),

		System:  bg(lipgloss.NewStyle().Foreground(p.System).Italic(true)),
		Loading: bg(lipgloss.NewStyle().Foreground(p.Loading)),
		Prompt:  bg(lipgloss.NewStyle().Bold(true).Foreground(p.User)),
		Help:    bg(lipgloss.NewStyle().Foreground(p.System).Italic(true)),

		// No border: opencode-ai/opencode's real editor.go (internal/tui/
		// components/chat/editor.go) has zero Border references — it tints
		// the textarea's background only. Matching that instead of the
		// rounded-border box this repo previously drew around the input.
		InputBox: lipgloss.NewStyle().
			Background(p.Bg).
			Padding(0, 1),

		ViewBg:   viewBgStyle,
		ChromeBg: chromeBgStyle,

		TokenIn:      bg(lipgloss.NewStyle().Bold(true).Foreground(p.TokenIn)),
		TokenOut:     bg(lipgloss.NewStyle().Bold(true).Foreground(p.TokenOut)),
		TokenTotal:   bg(lipgloss.NewStyle().Foreground(p.System)),
		ProviderName: bg(lipgloss.NewStyle().Bold(true).Foreground(p.Accent)),

		CodeBg:     p.CodeBg,
		CodeFg:     p.CodeFg,
		CodeBorder: p.CodeBorder,
		CodeInline: lipgloss.NewStyle().
			Background(p.CodeInline).
			Foreground(p.CodeFg),
	}
}

// Chip builds a "pill" style with the given background and an automatically
// contrasting (black or white) foreground — opencode's real footer/status
// bar convention (internal/tui/components/status/status.go) gives every
// segment its own distinct background hue rather than plain bullet-
// separated text on the shared chrome background. Callers pass one of the
// palette's already-distinct semantic colours (Accent, Loading, TokenIn,
// Agent, ErrC, ...) per segment, so each theme's chips stay visually
// consistent with that theme's own established hues instead of a fresh,
// unvetted set of 14-themes-worth of new hex values.
func Chip(bg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(contrastFg(bg)).
		Bold(true).
		Padding(0, 1)
}

// contrastFg picks black or white — whichever gives better contrast against
// bg — using the standard perceptual-luminance approximation
// (0.299R + 0.587G + 0.114B). Falls back to white if bg isn't a parseable
// "#rrggbb" lipgloss.Color (e.g. an ANSI-256 index or empty/transparent).
func contrastFg(bg lipgloss.TerminalColor) lipgloss.TerminalColor {
	c, ok := bg.(lipgloss.Color)
	if !ok {
		return lipgloss.Color("#ffffff")
	}
	hex := strings.TrimPrefix(string(c), "#")
	if len(hex) != 6 {
		return lipgloss.Color("#ffffff")
	}
	r, err1 := strconv.ParseInt(hex[0:2], 16, 32)
	g, err2 := strconv.ParseInt(hex[2:4], 16, 32)
	b, err3 := strconv.ParseInt(hex[4:6], 16, 32)
	if err1 != nil || err2 != nil || err3 != nil {
		return lipgloss.Color("#ffffff")
	}
	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luminance > 149 {
		return lipgloss.Color("#1a1a1a")
	}
	return lipgloss.Color("#ffffff")
}
