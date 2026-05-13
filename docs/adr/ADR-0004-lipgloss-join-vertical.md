# ADR-0004: Remove lipgloss.JoinVertical from View()

**Status:** Accepted  
**Date:** 2025  
**Deciders:** Project leads

---

## Context

The Bubbletea TUI's `View()` function must assemble several UI sections
(header, viewport, input, footer) into a single string frame. The natural
Lipgloss primitive for this is `lipgloss.JoinVertical(lipgloss.Left, ...)`.

## Problem discovered

`JoinVertical` (lipgloss `join.go` lines 144–145) pads shorter blocks to the
width of the widest block using plain `strings.Repeat(" ", w)`. These padding
spaces have no background style applied. On terminals where the default
background is black (the majority), this produces visible black horizontal
gaps between sections — especially visible on light themes with an explicit
background colour.

The source:

```go
// lipgloss/join.go:144-145
row += strings.Repeat(" ", w-lipgloss.Width(str))
```

No style is applied to the padding — it inherits the terminal default.

## Decision

Replace all `lipgloss.JoinVertical(...)` calls in `View()` with
`strings.Join([]string{...}, "\n")`. Each section is independently
responsible for filling itself to exactly `m.width` columns using
`fillLines` with the theme's background style.

## Implementation

```go
// Before (produces black gaps):
return lipgloss.JoinVertical(lipgloss.Left, header, viewport, input, footer)

// After (no gaps):
return strings.Join([]string{header, viewport, input, footer}, "\n")
```

Each section calls `fillLines(content, s.chromeBg, m.width)` before being
joined, ensuring every row is full-width with the correct background.

## Consequences

- No more black gaps between sections on any theme.
- Each component must manage its own width padding — slightly more code, but
  correctly encapsulated.
- `fillLines` and `paintLines` are the canonical mechanisms for background fill.
- This pattern must be preserved when adding new UI sections.

## Verification

The fix is covered by `TestRenderMarkdown*` tests in `cmd/tui/mdtest_test.go`,
which assert that rendered frames contain no unexpected bare spaces at section
boundaries.
