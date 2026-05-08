---
name: retro
description: Engineering retrospective for go-adk-q. Review what shipped, what broke, what slowed us down, and what to do differently next time.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Retro

Run after a sprint, milestone, or significant feature. Keep it short. The goal is actionable output, not a therapy session.

## Format (timebox: 20 minutes)

### What shipped
List everything merged or completed since the last retro. Be specific: feature names, bug fixes, refactors.

Example:
```
- Glamour markdown rendering (replaces hand-rolled renderer)
- Slash command autocomplete menu
- Theme change now invalidates renderer cache
- /skills TUI command
- 6 new skill files: qa, test-driven, health, writing-plan, retro, brainstorming
```

### What worked well
Patterns, tools, or decisions that made work faster or higher quality. Keep for next time.

Example:
```
- Writing failing tests before implementing (caught the renderer cache bug immediately)
- Small atomic commits — easy to revert individual changes
- Reading existing code before touching anything
```

### What was painful
Friction, mistakes, or time sinks. Be honest.

Example:
```
- Edit tool appended instead of replacing — need to use Write for full-file rewrites
- Forgot to run go build after edits twice — costs 30 seconds each time
- Provider key management during testing is manual and error-prone
```

### What to change
One action per pain point. Concrete, not vague.

Example:
```
- Pain: Edit tool duplicate content → Action: use Write for whole-file rewrites
- Pain: Build not run after edits → Action: always run go build before marking done
- Pain: Manual provider testing → Action: add make smoke-test target
```

### Metrics snapshot
Run these and record the numbers:

```bash
go test -count=1 ./... 2>&1 | tail -5          # test count and status
go test -cover ./cmd/tui/ 2>&1 | grep coverage # coverage %
git log --oneline -10                           # recent commits
wc -l cmd/tui/chat.go                          # file size (watch for growth)
ls skills/ | wc -l                             # skill count
```

### One thing to try next sprint
Pick one experiment. Small enough to evaluate in one week.

Example: "Add a Makefile target `make qa` that runs build + vet + test + staticcheck in sequence."

## Output

Write the retro as a dated entry in `docs/RETRO.md` (create if it doesn't exist). Append — never overwrite previous retros.

Format:
```markdown
## Retro — YYYY-MM-DD

### Shipped
...

### Worked well
...

### Painful
...

### Changes
...

### Metrics
Tests: N passing | Coverage: X% | Skills: N | chat.go: N lines

### Next experiment
...
```
