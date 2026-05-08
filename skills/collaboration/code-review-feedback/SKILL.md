---
name: code-review-feedback
description: Give structured Go code review feedback. Covers correctness, concurrency, error handling, style, and test coverage for go-adk-q.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Review Feedback

Use when reviewing a Go change in this codebase. Be specific, be constructive, and prioritise by severity.

## Severity levels

| Level | Meaning | Must fix before merge? |
|-------|---------|----------------------|
| **Critical** | Bug, data loss, race condition, security issue | Yes |
| **Major** | Wrong abstraction, missing error handling, performance problem | Yes |
| **Minor** | Style violation, naming, missing comment | Preferred |
| **Nit** | Formatting, whitespace, trivial rename | Optional |

## Review checklist

### Correctness
- [ ] Does the logic handle the zero value / nil case?
- [ ] Are slice bounds checked before indexing?
- [ ] Are errors returned, not silently dropped?
- [ ] Does the function do what its name says — exactly that, nothing more?

### Concurrency (TUI-specific)
- [ ] Are fields accessed from multiple goroutines protected? (`chat.go` fields are owned by the Update loop — only `streamingText` and `loading` are written from the stream goroutine via `tea.Msg`)
- [ ] Are channels buffered appropriately? (The stream channel is buffered 64 — don't make it unbuffered)
- [ ] Is `invalidateRendererCache()` called under the mutex? (It takes the lock itself — don't double-lock)

### Error handling
- [ ] All `err != nil` checks present?
- [ ] Errors wrapped with `%w` where the caller might need to `errors.Is`?
- [ ] Fallback paths for all failure modes?

### TUI conventions
- [ ] Only `styledSet` styles used in view functions — no raw `lipgloss.Color()`?
- [ ] New view functions are methods on `chatModel` returning `string`?
- [ ] `refreshViewport()` called after any state change that affects display?
- [ ] `slashMenuIdx` reset to 0 after any input clear?

### Tests
- [ ] New function has at least one test?
- [ ] Table-driven test with both happy and edge cases?
- [ ] Error/failure path tested, not just happy path?

## Feedback format

```
## Review — <PR/branch name>

### Critical
- [file:line] <issue> — <fix>

### Major
- [file:line] <issue> — <fix>

### Minor
- [file:line] <issue> — <fix>

### Praise
- <what was done well>

### Verdict: APPROVE / REQUEST CHANGES
```

## Go-specific patterns to watch for in this codebase

```go
// Wrong: drops error
_ = copyToClipboard(content)

// Right: handle it
if err := copyToClipboard(content); err != nil {
    m.statusMsg = "Copy failed: " + err.Error()
}
```

```go
// Wrong: raw colour bypasses theme
lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))

// Right: use the theme palette
s.prompt.Foreground(...)  // or the appropriate styledSet field
```

```go
// Wrong: hardcoded width
lipgloss.NewStyle().Width(80)

// Right: derive from terminal width
lipgloss.NewStyle().Width(m.width - 4)
```
