---
name: plan-design-review
description: Review a written plan for UI/UX quality before implementation. Catch layout, colour, interaction, and accessibility issues at plan stage.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Plan Design Review

Run this skill on a written plan before implementing any visible TUI change. Catching design problems in the plan is free. Catching them after implementation costs a full rewrite.

## Review dimensions

### 1. Layout correctness
- Does the plan account for dynamic terminal width?
- Does any new element have a fixed height? If yes, does the viewport height calculation account for it?
- Where exactly does this element appear in the vertical stack? (header → viewport → slash menu → sep → input → footer)
- Does the element appear/disappear? If yes, is the viewport height recalculated on each state change?

Flag: any plan that says "render X between the input and footer" without updating `vpH` calculation.

### 2. Colour discipline
- Does the plan use `styledSet` colours or introduce new raw values?
- If introducing new colours: are they in the theme palette? Do they work across all 5 themes?
- Will the element be visible in both dark and light-adjacent themes (Nord vs Catppuccin)?

Flag: any `lipgloss.Color("196")` or `lipgloss.Color("#ff0000")` that bypasses the theme system.

### 3. Interaction completeness
- What keyboard bindings does this introduce?
- Are they in the help overlay?
- Do they conflict with existing bindings? (Check the binding table in the help overlay)
- What happens when the user presses Esc, Enter, or Ctrl+C in the new state?

Flag: any new interactive state without an Esc/Cancel path.

### 4. Empty and error states
- What does the element look like when it has no data?
- What does it look like when data loading fails?
- Is there a loading state?

Flag: any plan that only describes the happy path.

### 5. Resize behaviour
- Does `tea.WindowSizeMsg` handling need updating?
- Does any hardcoded width/height appear in the plan?

Flag: any width/height constant that isn't derived from `m.width` or `m.height`.

## Review output format

For each dimension, give a verdict: **PASS**, **WARN**, or **BLOCK**.

```
Layout:      PASS / WARN: vpH not recalculated / BLOCK: fixed width hardcoded
Colour:      PASS / WARN: new colour not in theme / BLOCK: raw hex used
Interaction: PASS / WARN: no Esc path / BLOCK: conflicts with ctrl+y
Empty state: PASS / WARN: only happy path described
Resize:      PASS / WARN: width assumed to be 80

Overall: APPROVE / REVISE (list changes needed) / REJECT (fundamental issue)
```

**BLOCK** on any dimension means the plan must be revised before implementation starts.
