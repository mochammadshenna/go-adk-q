---
name: code-review-iteration
description: Process a round of code review feedback, implement fixes, and prepare for the next review round.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Review Iteration

Use after receiving review feedback (see receiving-code-review) and implementing fixes. This skill manages the iteration cycle cleanly.

## The iteration loop

```
Receive feedback
  → Categorise all comments
  → Implement fixes (one commit per logical fix)
  → Verify: go build && go test -race
  → Respond to each comment
  → Request re-review
```

Repeat until all Critical and Major issues are resolved.

## Implementing fixes

### Work in order of severity
1. Critical first (bugs, races)
2. Major second (design, error handling)
3. Minor last (style, naming)

Don't mix severities in a single commit. If fixing a bug also requires a rename, do them in separate commits.

### One commit per fix
```bash
# Fix the goroutine leak
git add cmd/tui/chat.go
git commit -m "fix: cancel stream goroutine on ctx done"

# Fix the naming nit
git add cmd/tui/slash.go
git commit -m "style: rename slashCmds to allSlashCmds for clarity"
```

This makes the review history readable and individual fixes revertable.

### After each fix
```bash
go build ./...      # catches type errors immediately
go test ./...       # catches regressions
```

Don't batch all fixes and then run tests once at the end. You won't know which fix caused a regression.

## Responding to comments

For each comment, write a one-line response:

| Comment type | Response |
|-------------|----------|
| Fixed | "Fixed in abc1234 — moved channel buffer from 1 to 64" |
| Acknowledged, deferred | "Acknowledged — logged as follow-up, out of scope for this PR" |
| Disagree | "I prefer the current approach because X — open to discussing" |
| Question | "Good question — added a comment at line 83 explaining why" |

Never leave a comment unacknowledged. Even "I saw this and decided not to change it" is better than silence.

## Determining when to stop iterating

Stop when:
- All Critical and Major comments are either fixed or explicitly accepted by the reviewer
- `go test -race ./...` is green
- You and the reviewer agree on any remaining Minor/Nit items

Don't stop when:
- You "think" the reviewer will be happy (ask them)
- You've addressed some but not all Critical issues
- Tests are failing

## For go-adk-q specifically

After every iteration:
```bash
go build ./...
go test -count=1 -race ./cmd/tui/
go vet ./...
```

All three must be green before requesting re-review. Run them in this order — `go build` failing makes `go test` output misleading.
