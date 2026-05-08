---
name: writing-plan
description: Structure a clear implementation plan before touching code. Covers goal, constraints, steps, risks, and verification criteria.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Writing a Plan

Use before any non-trivial change. A plan written in 10 minutes prevents an hour of backtracking.

## When to use this skill

- New feature spanning more than one file
- Refactor that changes interfaces or package boundaries
- Bug whose root cause is not yet confirmed
- Any work that will take longer than 30 minutes to implement

## Plan structure

### 1. Goal (one sentence)
What does "done" look like? Be concrete.
Bad: "Improve the TUI."
Good: "Add a `/skills` slash command that reads ./skills/ and lists each skill's name and description as a system message."

### 2. Context
- What files are affected?
- What existing patterns does this work build on?
- Any constraints (API compatibility, performance, no new dependencies)?

### 3. Steps (ordered)
Number each step. Each step should be small enough to verify independently.

Example:
```
1. Add /skills to allSlashCmds in cmd/tui/slash.go
2. Implement listSkillsSummary() — reads ./skills/, parses YAML front matter
3. Handle /skills case in the enter key switch in chat.go
4. Add /skills to the help overlay bindings
5. Write a test: TestListSkillsSummary_NonEmpty
6. go build ./... && go test ./...
```

### 4. Risks and unknowns
- What could go wrong?
- What do you need to verify before starting?
- Any parts you're uncertain about?

### 5. Verification
How will you know it works?
- Which tests will pass?
- What manual behaviour confirms it?
- What does failure look like?

## Rules

- Do not start coding until steps 1-4 are written.
- If you discover a risk during implementation that changes the plan, update the plan first.
- Each step must be completable and verifiable on its own — no "and also" steps.
- If a step depends on an earlier one, say so explicitly.

## For this codebase specifically

Before planning any TUI change:
1. Read the affected function in `cmd/tui/chat.go` or `cmd/tui/slash.go`
2. Run `go test ./cmd/tui/ -count=1` to confirm green baseline
3. Check `cmd/tui/mdtest_test.go` for existing test patterns to follow

Before planning any provider change:
1. Read `model/oaibridge/bridge.go` — all providers go through this
2. Check `cmd/tui/main.go:buildRunner()` for the failover wiring
