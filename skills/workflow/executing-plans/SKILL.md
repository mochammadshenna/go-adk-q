---
name: executing-plans
description: Discipline for executing a written plan. Stay on the plan, track progress, surface blockers early, and avoid scope creep.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Executing Plans

Use this skill when you have a written plan (from writing-plan) and are ready to implement. The plan is the contract — follow it.

## Pre-execution checklist

Before writing the first line:

```bash
git status --short          # clean working tree?
go test ./... -count=1      # green baseline?
go build ./...              # builds clean?
```

If any check fails, fix it before starting. Do not implement on a dirty baseline — you won't be able to tell what you broke.

## Execution discipline

### One step at a time
Complete step N fully before starting step N+1. Partial implementations compound into confusion.

### Verify each step
Every step in the plan has a verification. Run it. Do not move on until it passes.

### Build after every change
```bash
go build ./...
```
Takes 2 seconds. Catches type errors before they compound across multiple files.

### Commit atomically
Each completed step is one commit:
```bash
git add -p              # stage only the changes for this step
git commit -m "feat: <what this step does>"
```

Atomic commits mean any step can be reverted without affecting others.

### When you hit a blocker
A blocker is anything that prevents completing the current step as planned.

Do not:
- Skip the step and continue
- Silently change the approach

Do:
1. Stop
2. Name the blocker in one sentence
3. Check if the plan needs updating
4. If yes, update the plan, then continue
5. If the blocker reveals the plan is wrong, run brainstorming before continuing

### Scope creep detection
During implementation you will notice things you could also fix or improve. Log them — do not implement them.

Maintain a running list:
```
# Noticed but deferred
- chat.go line 412: fmt.Sprintf could be simplified (staticcheck S1039)
- settings form doesn't reset slashMenuIdx after apply
```

Add these to the next plan after this one finishes.

## For go-adk-q specifically

### Execution order for TUI changes
1. Update data structures (add fields to `chatModel`, `styledSet`, etc.)
2. Update `newChatModel` initializer
3. Add new functions/methods
4. Wire into `Update()` (key handling)
5. Wire into `View()` (rendering)
6. Add/update tests
7. `go build ./... && go test ./...`

### Execution order for provider changes
1. Create `model/<name>/<name>.go` with `Config`, `ConfigFromEnv()`, `NewModel()`
2. Add to failover chain in `cmd/tui/main.go:buildRunner()`
3. Add to `main.go:buildRunner()` (CLI path)
4. Add to `applyProviderSelected()` substring match
5. `go build ./...`

### Never do this
- Edit a file without reading it first
- Run `go test` and ignore a FAIL
- Add an import without checking if it's already imported
- Rename a function without searching for all call sites: `grep -rn "funcName" .`
