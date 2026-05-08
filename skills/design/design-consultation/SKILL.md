---
name: design-consultation
description: TUI and CLI design consultation. Covers layout, colour, information hierarchy, interaction patterns, and terminal constraints for go-adk-q.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Design Consultation

Use when making visible changes to the TUI: new views, changed layouts, new colour uses, or new interaction patterns.

## Terminal design constraints

These are hard limits — not preferences:

- **Width unknown at design time.** The TUI must work from 40 to 220 columns. Never hardcode widths.
- **Colour support varies.** Terminals range from 8 colours to truecolour. Use lipgloss `AdaptiveColor` or theme palette colours — never raw hex.
- **No hover state.** Everything must be discoverable via text or key hints. There is no tooltip.
- **Mouse is unreliable.** Design for keyboard-first. Mouse scroll is a bonus.
- **Alt screen means no scroll buffer.** What's off-screen is gone. Important info must be in the viewport.

## Layout principles for this TUI

```
┌─ header (1 line) ──────────────────────────────┐
│  go-adk-q  •  model-name            ▼ more     │
├─ viewport (fills remaining height) ────────────┤
│  message history (scrollable)                  │
│                                                │
├─ slash menu (0-N lines, shown on demand) ──────┤
│  ╭──────────────────────────────────────────╮  │
│  │  /theme    Cycle colour theme            │  │
│  ╰──────────────────────────────────────────╯  │
├─ separator (1 line) ───────────────────────────┤
│  ──────────────────────────────────────────    │
│  > input text                                  │
├─ footer (2 lines) ─────────────────────────────┤
│  hint text                     N/2000          │
│   tokens  in: N  out: N  total: N              │
└────────────────────────────────────────────────┘
```

**Viewport height = terminal height − header − slash menu − sep − input − footer.**
Any new element inserted between header and footer shrinks the viewport. Keep insertions minimal.

## Colour usage (styledSet)

| Style | Used for |
|-------|----------|
| `header` | top bar background + text |
| `userLabel` | "You" label + timestamp |
| `agentLabel` | "Agent" label + timestamp |
| `errorLabel` | "Error" label |
| `userText` | user message body |
| `agentText` | fallback for agent body (glamour overrides this) |
| `errorText` | error message body |
| `system` | system messages, timestamps, secondary text |
| `loading` | spinner + "Thinking…" |
| `prompt` | selected slash menu item, input prompt colour |
| `sep` | separator line |
| `help` | footer hint text |

**Rule:** only use styles from `styledSet`. Never call `lipgloss.Color()` directly in a view function.

## Interaction patterns

**Slash menu:**
- Appears when input starts with `/`
- Navigated with `↑`/`↓`; completed with `Tab`; dismissed with `Esc`
- One-line entries: command name (left-padded to align) + description
- Selected row uses `prompt` style; others use `system` style

**Overlays (help, settings):**
- Full-viewport replacement (settings) or viewport content replacement (help)
- Always show an escape hint in the footer
- Never use a modal that blocks the entire terminal — use the viewport area

**Status messages:**
- One-line ephemeral messages in the footer hint area
- Auto-clear after 2-3 seconds via `oneShotTimer`
- Prefix with "✓ " for success

## Reviewing a design change

Before implementing, answer:
1. What is the smallest terminal width this works at? (Test at 60 columns)
2. Does it degrade gracefully when colour is unavailable? (`TERM=xterm-mono`)
3. Does it need a key binding hint? If yes, add it to the help overlay.
4. Does it resize correctly on `tea.WindowSizeMsg`?
5. Does it add to or reuse existing `styledSet` styles?
