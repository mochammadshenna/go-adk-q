# Explanation: TUI rendering

This document explains how the Bubbletea TUI renders correctly across 8 themes,
including the specific techniques required to avoid background colour loss.

---

## The core problem

Bubbletea's Bubble components (textarea, viewport) emit raw ANSI escape
sequences. When a component emits `\x1b[0m` (the ANSI "reset all attributes"
sequence), it clears **every** style attribute — including the background colour
that was set by the surrounding shell. On light themes with an explicit
background, this produces a black rectangle where the terminal default
(usually black) shows through.

Additionally, `lipgloss.JoinVertical` — the obvious tool for stacking UI
sections vertically — pads each section to a common width using plain spaces,
with no background style. On terminals where the default background is not the
theme background, this produces visible black horizontal gaps.

---

## Solution: no JoinVertical, use strings.Join

The `View()` method in `chat.go` assembles the final frame as:

```go
return strings.Join([]string{
    header,
    viewportSection,
    inputSection,
    footer,
}, "\n")
```

Each section is independently responsible for filling itself to exactly
`m.width` columns with the correct background colour. This way no inter-section
padding is needed.

---

## fillLines

```go
func fillLines(content string, bgStyle lipgloss.Style, fullW int) string
```

Called on every rendered section before it is joined. For each line:
1. Measure the visible width (excluding ANSI escapes) using `lipgloss.Width`.
2. Append `bgStyle.Render(strings.Repeat(" ", fullW - visibleWidth))` to pad.

For dark themes, `bgStyle` has no background set — `Background("")` is a
no-op in lipgloss — so `fillLines` is a cheap pass-through.

For light themes, `bgStyle` carries the theme's explicit background colour,
so each padded space extends the background to the right edge.

---

## paintLines

```go
func paintLines(content string, bgSeq string, fullW int) string
```

Used specifically on the textarea output. The textarea emits `\x1b[0m`
internally. `paintLines` processes the content byte-by-byte, and after every
`\x1b[0m` it finds, it re-injects `bgSeq` (the ANSI sequence for the theme
background colour).

`bgSeq` is obtained by probing a one-character render:

```go
bgSeq := lipgloss.NewStyle().Background(p.bg).Render(" ")[:len(p.bg)+...] // probe
```

For dark themes, `p.bg` is `""` — `Render(" ")` with no background emits just
`" "`, so `bgSeq` is empty and `paintLines` becomes a no-op.

---

## setViewportContent

```go
func setViewportContent(vp *viewport.Model, content string, height int, bgStyle lipgloss.Style)
```

Before setting viewport content, pre-pads with `height` blank lines filled
with `bgStyle`. This ensures that when the conversation history is shorter
than the viewport, the empty space below the last message still carries the
theme background (not terminal default black).

---

## Textarea width formula

The textarea's `SetWidth` call uses `m.width - 6`. This value was derived
empirically to match the visual width of the input area after accounting for:
- 1-column padding on each side of the input border: `- 2`
- 1-column left margin: `- 1`
- Internal textarea padding: `- 3`

The `inputView` function also calls `paintLines` on the raw textarea output
and then `fillLines` on each line to ensure the background extends to the
right edge.

---

## Light theme rendering pipeline

For a light theme, a single render frame follows this path:

```
m.headerView()
    → lipgloss.Render with accent/text styles
    → fillLines(..., bgStyle, m.width)

m.viewport.View()
    → glamour renders Markdown with explicit colours
    → setViewportContent pre-pads and fills
    → fillLines each line

m.inputView()
    → m.textarea.View() — emits \x1b[0m internally
    → paintLines(raw, bgSeq, m.width) — re-arms bg after each reset
    → fillLines each painted line

m.footerView()
    → fillLines(..., chromeBgStyle, m.width)

strings.Join([header, viewport, input, footer], "\n")
```

---

## Dark theme rendering pipeline

Identical path, but `bgSeq` is `""` and `bgStyle` has no background. All
fill and paint operations are no-ops. The terminal's own background colour
provides the visual fill.

---

## Related

- [How-to: Debug a TUI rendering bug](../how-to/debug-tui.md)
- [ADR-0004: Why JoinVertical was removed](../adr/ADR-0004-lipgloss-join-vertical.md)
- [Reference: Themes](../reference/themes.md)
