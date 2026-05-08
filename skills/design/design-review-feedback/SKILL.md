---
name: design-review-feedback
description: Give structured feedback on an implemented UI change. Covers visual quality, consistency, interaction feel, and terminal-specific issues.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Design Review Feedback

Use after implementing a TUI change to give or receive structured visual feedback. Be specific — "it looks off" is not actionable.

## How to give feedback

Every piece of feedback needs three parts:
1. **What** — exact element and location ("the slash menu border at narrow widths")
2. **Problem** — what's wrong ("border columns overlap the text at < 50 cols")
3. **Fix** — concrete suggestion ("clamp rowW minimum to 8, not 10, and truncate name column first")

## Common TUI issues to look for

### Alignment
- Are columns consistent across rows? (padding, truncation)
- Does the selected row in menus stay the same width as unselected rows?
- Does the content shift when the selection moves?

### Colour contrast
- Is the selected state visually distinct from unselected? (not just bold — colour change too)
- Does the border colour work against the terminal background?
- Are error messages clearly different from system messages?

### Truncation
- Does long text overflow or truncate gracefully?
- Does truncation happen before clipping (no partial characters)?

### Spacing
- Is there a consistent margin/padding rhythm? (The TUI uses 2-column left padding for message bodies)
- Are blank lines between elements consistent?

### Resize
- Resize the terminal to 50% width while the feature is active. Does it hold?
- Resize back to full. Does it recover?

### Interaction feel
- Is feedback immediate? (No delay between keypress and visual change)
- Is the state transition obvious? (User knows what happened)

## Feedback template

```
Element: <what you're looking at>
Terminal: <width x height when observed>
Theme: <which theme>

Issues:
1. [visual] <what> — <problem> — Fix: <suggestion>
2. [interaction] <what> — <problem> — Fix: <suggestion>
3. [resize] <what> — <problem> — Fix: <suggestion>

Verdict: APPROVE / REVISE
```

If revising: list the minimum set of changes needed. Don't scope-creep the review into a full redesign.
