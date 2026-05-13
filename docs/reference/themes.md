# Reference: Themes

The TUI ships with 8 built-in colour themes. Cycle with `/theme` or set the
starting theme via the `--theme` flag (index 0–7).

---

## Theme index

| Index | Name | Type | Default |
|---|---|---|---|
| 0 | Catppuccin | Dark | ✓ |
| 1 | Tokyo Night | Dark | |
| 2 | Rosé Pine | Dark | |
| 3 | Nord | Dark | |
| 4 | Gruvbox | Dark | |
| 5 | GitHub Light | Light | |
| 6 | Solarized Light | Light | |
| 7 | Tango | Light | |

---

## Dark vs light themes

**Dark themes** leave `bg` empty in the palette definition. The terminal's
own background colour shows through. ANSI resets (`\x1b[0m`) are harmless
because the default terminal background is already dark.

**Light themes** set `bg` to an explicit hex colour. The rendering pipeline
uses `fillLines` and `paintLines` to ensure every row is filled with this
colour, even after ANSI reset sequences emitted by third-party widgets.

---

## Palette fields

Each theme defines the following semantic tokens:

| Field | Role |
|---|---|
| `name` | Display name in `/theme` picker |
| `bg` | Terminal background fill (empty for dark themes) |
| `surface` | Chat bubble and card background |
| `overlay` | Picker and overlay background |
| `text` | Primary text colour |
| `subtle` | Secondary / dimmed text |
| `accent` | Input border, provider name in footer |

---

## Glamour Markdown styles

Each theme index maps to a `glamourStyleConfig` case in `cmd/tui/markdown.go`.
This configures heading colours, code block backgrounds, link colours, etc.
for the Markdown renderer.

Adding a new theme requires both:
1. A `palette` entry in `builtinThemes` (in `chat.go`)
2. A `case N:` in `glamourStyleConfig` (in `markdown.go`)

See [how-to/add-theme.md](../how-to/add-theme.md) for the step-by-step guide.

---

## Source locations

| File | Contains |
|---|---|
| `cmd/tui/chat.go` | `palette`, `builtinThemes`, `styledSet`, `fillLines`, `paintLines` |
| `cmd/tui/markdown.go` | `glamourStyleConfig` per theme index |
