---
name: code-refactor
description: Plan and execute a focused refactor. Identify the target, define the boundary, keep behaviour unchanged, and verify with tests.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Refactor

Use when changing the structure of existing code without changing its behaviour. A refactor that changes behaviour is a bug.

## The golden rule

**Refactor = behaviour unchanged, structure improved.**

Before starting, write down what the code does today. After finishing, verify it still does exactly that. If you can't describe the current behaviour, you're not ready to refactor.

## When to refactor in go-adk-q

Good candidates:
- `chat.go:Update()` — over 700 lines; individual key handlers should be extracted to helper methods
- `renderMessages()` — each role could be a method (`renderUserMsg`, `renderAgentMsg`, etc.)
- Provider packages with copy-pasted config parsing
- Any function over ~60 lines that mixes concerns

Bad candidates (leave alone):
- Code you don't fully understand yet
- Code with no tests — refactor without tests is gambling
- Code that's about to be deleted

## Process

### 1. Define the boundary
What exactly are you refactoring? Name the file, function, or type. Write it down.

"Extracting the `"enter"` key handling block in `Update()` (~150 lines) into `handleEnterKey() (tea.Model, tea.Cmd)`"

### 2. Confirm green baseline
```bash
go test -count=1 ./...    # must be green before you start
```

If tests are red before you start, you can't tell if your refactor broke anything.

### 3. Refactor in the smallest safe steps

**Step types (in order of safety):**

| Step | Safety | Example |
|------|--------|---------|
| Rename | Very safe | `slashCmd` → `menuEntry` |
| Extract function | Safe | inline code → named function |
| Move to method | Safe | package func → method on `chatModel` |
| Restructure logic | Risky | reorder conditionals |
| Change types | Risky | `[]string` → `[]skillEntry` |

Do safe steps first. Run tests after each step.

### 4. Verify after each step
```bash
go build ./...
go test -count=1 ./...
```

If tests go red: `git diff` to see exactly what changed. Revert the bad step and try again. Don't pile changes on top of a failing state.

### 5. Commit when green
```bash
git commit -m "refactor: extract handleEnterKey from Update()"
```

One commit per logical refactor step.

## For go-adk-q specifically

### Extracting from `Update()`
The pattern is: pull the case body into a method, replace the body with a call.

```go
// Before
case "enter":
    if m.loading { break }
    // ... 80 lines ...

// After
case "enter":
    return m.handleEnterKey(cmds)
```

The method signature should be `func (m chatModel) handleXxx(cmds []tea.Cmd) (tea.Model, tea.Cmd)`.

### Renaming exported names
Use `grep -rn "OldName" .` to find all call sites before renaming. In this codebase, exported names from `cmd/tui/` are only used within that package, but `model/*/` exports are used in `main.go` and `cmd/tui/main.go`.

### Moving logic to `slash.go`
Anything related to slash command parsing or display belongs in `slash.go`, not `chat.go`. Move it and keep `chat.go` as the orchestrator.
