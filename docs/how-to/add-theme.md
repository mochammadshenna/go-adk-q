# How-to: Add a colour theme

**Goal:** Add a new built-in colour theme that appears in the `/theme` picker
and the settings overlay.

---

## Where themes live

All theme data is in `cmd/tui/chat.go`, starting at the comment
`// ── Theme system ───`.

The key types:

```go
// palette is a set of semantic design tokens for one theme.
type palette struct {
    name    string
    // dark themes leave bg empty (transparent = terminal default)
    // light themes set bg to an explicit hex color
    bg      lipgloss.TerminalColor
    surface lipgloss.TerminalColor  // chat bubble background
    overlay lipgloss.TerminalColor  // overlay / picker background
    text    lipgloss.TerminalColor  // primary text
    subtle  lipgloss.TerminalColor  // secondary / dimmed text
    accent  lipgloss.TerminalColor  // input border, provider name
    // ... more fields
}

var builtinThemes = []palette{ ... }
```

Index 0 is the default (Catppuccin). `/theme` cycles the index.

---

## Step 1 — Choose your colour palette

Decide on hex values for each semantic role. The 8 current themes are:

| # | Name | Type |
|---|---|---|
| 0 | Catppuccin | Dark |
| 1 | Tokyo Night | Dark |
| 2 | Rosé Pine | Dark |
| 3 | Nord | Dark |
| 4 | Gruvbox | Dark |
| 5 | GitHub Light | Light |
| 6 | Solarized Light | Light |
| 7 | Tango | Light |

**Dark themes:** Leave `bg` empty (`lipgloss.TerminalColor("")`). The terminal
default background shows through — no global fill is applied.

**Light themes:** Set `bg` to an explicit colour. Every render pass calls
`fillLines`/`paintLines` to ensure the background covers the full terminal
width and survives ANSI reset sequences.

---

## Step 2 — Add the palette entry

Open `cmd/tui/chat.go` and append to `builtinThemes`:

```go
{
    name:    "My Theme",
    bg:      lipgloss.Color(""),            // dark: empty; light: e.g. "#fafafa"
    surface: lipgloss.Color("#1e1e2e"),
    overlay: lipgloss.Color("#313244"),
    text:    lipgloss.Color("#cdd6f4"),
    subtle:  lipgloss.Color("#6c7086"),
    accent:  lipgloss.Color("#89b4fa"),
    // add any other palette fields defined in the struct
},
```

---

## Step 3 — Add a glamour Markdown style

Open `cmd/tui/markdown.go`. The function `glamourStyleConfig` has a `switch`
on `themeIdx`. Add a new `case` for your theme's index (N = length of
`builtinThemes` before you added yours):

```go
case N:
    // My Theme — dark, Catppuccin-like colour scheme
    cfg.Heading.Color = stringPtr("#89b4fa")
    cfg.Code.BackgroundColor = stringPtr("#313244")
    // ... other overrides
    return cfg
```

For **light themes**, do **not** set `cfg.Document.BackgroundColor` — it
causes a white-on-white rendering artifact in some terminals.

---

## Step 4 — Verify

```sh
go build ./cmd/tui
./tui chat
```

Type `/theme` and cycle until you reach your new theme. Confirm:

- Header and footer have correct colours.
- Input area background matches the theme.
- Markdown code blocks render with the configured background.
- No black gaps between UI sections.

---

## Troubleshooting

**Black strip between sections:** Verify that your new theme is not the only
one missing its entry in `glamourStyleConfig`. The `default` case returns a
generic config; add an explicit `case` for your index.

**Light theme shows white-on-white text:** Remove any `Document.BackgroundColor`
setting from the glamour config for that theme.

**`/theme` skips your theme:** Check that `builtinThemes` slice length and
`glamourStyleConfig` case count are consistent.

---

## Related

- [Explanation: TUI rendering](../explanation/tui-rendering.md)
- [Reference: Themes](../reference/themes.md)
- [ADR-0004: Why JoinVertical was removed](../adr/ADR-0004-lipgloss-join-vertical.md)
