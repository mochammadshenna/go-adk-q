---
name: debug
description: Structured debugging session — reproduce, isolate, diagnose, and fix. Use when given an error message or stack trace, when behavior diverges from expected, when something broke after a deploy, or when an issue works in one environment but not another.
compatibility: Designed for software engineering workflows.
---
# Debug

Run a structured debugging session to find and fix issues systematically.

## Iron Law

**Never propose a fix without a confirmed root cause.**

If you don't know why it's broken, you don't know what to fix. A fix without root cause is a guess — it may mask the symptom while the real cause persists.

## The four phases

### Phase 1: Reproduce

Get the bug to happen reliably before touching any code.

- What is the exact input that triggers it?
- What is the exact output you see vs. what you expect?
- Can you reproduce it with a minimal case (stripped of unrelated code)?
- Is it consistent or intermittent?

If you can't reproduce it, you can't verify a fix. Spend time here first.

### Phase 2: Isolate

Narrow the search space.

- Binary search through the call stack: where does correct behaviour stop?
- Check the last change — what was different before this broke?
- Check the environment — does it fail in all environments or just one?
- Check inputs — does it fail on all inputs or just specific cases?

Useful techniques:
```bash
git bisect start
git bisect bad HEAD
git bisect good <last-known-good-commit>
```

### Phase 3: Diagnose

Find the root cause. This is the hardest phase.

- Add targeted logging at the point of divergence (not everywhere)
- Read the full error message and stack trace — especially the bottom of the stack
- Check assumptions: are the types/values actually what you think they are?
- For concurrency bugs: use the race detector (`go test -race ./...`)

Document your hypothesis before testing it: "I believe the bug is X because Y. To verify, I'll Z."

### Phase 4: Fix

Fix the root cause, not the symptom.

- The fix should be the minimum change that eliminates the root cause
- Write a test that would have caught this bug
- Verify the fix: the test passes, and the original reproduction case is gone

## Common patterns in go-adk-q

**Goroutine leak**: Add `go tool pprof` goroutine profile. Look for goroutines blocked on channel send/recv with no receiver/sender.

**Renderer cache stale**: Check whether `invalidateRendererCache()` was called at all `themeIdx` mutation sites (there are three). See `cmd/tui/markdown.go`.

**Model not initializing**: Check `PROVIDER_SELECTED` env var. `applyProviderSelected()` in `cmd/tui/main.go` must substring-match against `m.Name()` returning `provider/model`.

**Test flaking on viewport size**: The test model uses `defaultWidth`/`defaultHeight` constants. Viewport-dependent assertions must account for `vpH = height - headerH - footerH - sepH - inputH`.

## Output format

```
## Debug session — <problem description>

### Reproduction
Steps to reproduce: ...
Minimal case: ...

### Isolation
Narrowed to: <file>:<line> or <component>

### Root cause
[Specific, verifiable statement of what is wrong and why]
Evidence: [quote from logs, test output, or code]

### Fix
[What changed and why this addresses the root cause]
Verification: [test command and expected output]
```
