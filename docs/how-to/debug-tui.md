# How-to: Debug a TUI rendering bug

**Goal:** Systematically diagnose and fix visual artifacts in the Bubbletea
TUI — black gaps, incorrect backgrounds, misaligned text.

---

## Step 0 — Reproduce in isolation

Always reproduce with the smallest possible terminal and theme combination:

```sh
go build -o tui ./cmd/tui
./tui chat
```

Resize the terminal to 80×24 and cycle through themes with `/theme`.
Note which theme and which component the artifact appears in.

---

## Common artifacts and fixes

### Black gap between UI sections

**Symptom:** A horizontal band of black between the header, viewport, and
input area, even on a light theme.

**Root cause:** `lipgloss.JoinVertical` was used. It pads shorter blocks
with plain spaces (no background colour), producing a black gap on terminals
with a non-default background.

**Fix:** Replace any `lipgloss.JoinVertical(...)` in `View()` with
`strings.Join([]string{header, viewport, input}, "\n")` and ensure every
component calls `fillLines` to pad itself to `m.width` columns.

See [ADR-0004](../adr/ADR-0004-lipgloss-join-vertical.md) for the full
investigation.

---

### Textarea interior is black on a light theme

**Symptom:** The input area shows a black rectangle on light themes.

**Root cause:** The `bubbles/textarea` component emits `\x1b[0m` (ANSI reset)
internally. A reset kills any background colour set before the textarea is
rendered. Subsequent characters are drawn on the terminal default (black on
most emulators).

**Fix:** Use `paintLines` after calling `m.textarea.View()`:

```go
raw := m.textarea.View()
painted := paintLines(raw, bgSeq, m.width)
```

`paintLines` re-injects the background escape sequence after every `\x1b[0m`
it finds, so the background is never lost.

---

### Wrong column width — content overflows or underflows

**Symptom:** Text wraps too early or line padding leaves a gap at the right edge.

**Cause:** Width passed to `fillLines` or `SetWidth` is wrong.

**Checklist:**
- Viewport content width: `m.width - 2` (account for 1-column border each side).
- Textarea width: `m.width - 6` (matches `SetWidth(m.width-6)` with no prompt).
- Every `fillLines` call uses `m.width` as `fullW`, not a derived value.

---

### Overlay (picker) has incorrect background

**Symptom:** The `/model` or `/theme` picker has wrong colours.

**Cause:** The overlay uses `s.overlay` (lipgloss style) but the content
inside was built with `s.text`. Verify `styledSet.overlay` is constructed
correctly in `newStyledSet()`.

---

## Debugging technique: unit test the renderer

Every rendering path is covered by tests in `cmd/tui/mdtest_test.go`. To
add a test for a new artifact:

```go
func TestMyArtifact(t *testing.T) {
    m := newTestModel(t, 80, 24)
    m.themeIdx = 5 // GitHub Light
    view := m.View()
    // assert: no bare \x1b[0m followed by non-background character
    if strings.Contains(view, "\x1b[0m ") {
        t.Error("bare reset found in view — background will be lost")
    }
}
```

Run:

```sh
go test ./cmd/tui/ -run TestMyArtifact -v
```

---

## Key functions to understand

| Function | File | Purpose |
|---|---|---|
| `fillLines` | `chat.go` | Pads each line to full terminal width with bg style |
| `paintLines` | `chat.go` | Re-injects bg escape after every ANSI reset |
| `setViewportContent` | `chat.go` | Pre-pads content to viewport height, then sets |
| `applyThemeToTextarea` | `chat.go` | Propagates current theme into the textarea widget |
| `inputView` | `chat.go` | Renders input area: textarea + painted background |
| `View()` | `chat.go` | Top-level render; uses `strings.Join("\n")` not `JoinVertical` |

---

## Related

- [Explanation: TUI rendering deep-dive](../explanation/tui-rendering.md)
- [ADR-0004: JoinVertical removed](../adr/ADR-0004-lipgloss-join-vertical.md)
- [Reference: Themes](../reference/themes.md)
