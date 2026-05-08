---
name: design-system
description: Audit, document, or extend a design system. Use when checking for naming inconsistencies or hardcoded values across components, writing documentation for a component's variants and states, or designing a new pattern that fits the existing system.
compatibility: Designed for software engineering and UI/UX workflows.
---
# Design System

Manage a design system — audit for consistency, document components, or design new patterns.

## Modes

```
audit      — Find inconsistencies, hardcoded values, undocumented components
document   — Write documentation for a component (variants, states, accessibility)
extend     — Design a new component or pattern that fits the existing system
```

## Audit mode

Check for:
- **Hardcoded values**: colours, spacing, font sizes that should reference tokens
- **Naming inconsistencies**: similar components with different naming conventions
- **Missing states**: components missing hover, focus, disabled, or error states
- **Undocumented components**: used in production but not in the design system

Output: list of issues grouped by severity (blocking, major, minor), with file/location references.

## Document mode

For each component, document:
1. **Purpose**: what problem does it solve?
2. **Variants**: what configurations exist? (size, colour, state)
3. **States**: default, hover, focus, active, disabled, error, loading
4. **Props/API**: what does the caller control?
5. **Usage guidelines**: when to use, when NOT to use, common mistakes
6. **Accessibility**: ARIA roles, keyboard navigation, screen reader behaviour
7. **Example**: one minimal, one realistic

## Extend mode

Before designing a new pattern:
1. Check if an existing component can be composed or extended
2. Review the existing visual language (colours, spacing, typography, motion)
3. Prototype with the existing token set — don't introduce new values unless truly necessary
4. Document the new pattern before implementing it
5. Get design review before shipping

## For go-adk-q TUI

The design system is `styledSet` in `cmd/tui/styles.go`. Rules:
- All colours must use `styledSet` — never raw hex or `lipgloss.Color` literals
- Spacing uses lipgloss padding/margin — never fixed strings of spaces
- Border style: `lipgloss.RoundedBorder()` throughout — don't mix border styles
- All colours should use `AdaptiveColor` — light and dark terminal support

When extending: add to `styledSet`, update all themes, verify with each of the 5 themes.
