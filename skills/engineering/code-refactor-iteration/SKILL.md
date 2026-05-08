---
name: code-refactor-iteration
description: Iterate on a refactor in progress. Handle unexpected scope growth, failing tests, and mid-refactor discoveries cleanly.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Refactor Iteration

Use when a refactor is in progress and you've hit a complication: tests went red, scope grew, or you discovered the current structure is more tangled than expected.

## Common mid-refactor problems

### Tests went red after a "safe" step

This means the step wasn't as safe as it looked. The most common causes:

1. **Implicit state dependency** — the extracted function relies on side effects from earlier in the original function. Fix: pass the state explicitly as parameters.
2. **Interface change** — the refactor changed what callers expected. Fix: update callers first, or add a compatibility wrapper temporarily.
3. **Test was testing implementation, not behaviour** — the test breaks on rename or restructure. Fix: rewrite the test to test observable output, not internal structure.

Immediately run:
```bash
git stash         # get back to green
git stash pop     # review what changed
```

Identify the smallest sub-step that stays green. Do that, commit, then continue.

### Scope grew unexpectedly

You started refactoring function A and discovered that doing it right requires also changing B, C, and the interface between them.

**Stop.** Revert to the last green commit. Reassess.

Options:
1. **Narrow the scope** — refactor A only, leave B and C as-is with a comment noting the future work
2. **Plan the full refactor** — write a plan covering A, B, and C before touching anything
3. **Abandon this refactor** — if the scope is too large, it's not a refactor, it's a rewrite. Plan it as one.

Do not continue expanding scope mid-refactor. You'll end up with a 500-line diff that's impossible to review.

### You don't understand part of the code

If you encounter code you can't confidently say "I know what this does", stop refactoring that part.

Options:
- Read it carefully, trace through it, write a test that documents its behaviour
- Leave it alone and refactor around it
- Ask for clarification before touching it

## Iteration rhythm

```
green → make smallest safe change → build → test → green? commit : revert+retry
```

Never let the codebase stay red for more than one commit worth of changes. If you've been in a red state for more than 15 minutes, revert and try a smaller step.

## Tracking progress

Keep a running diff of what the refactor has accomplished:
```bash
git diff main...HEAD --stat
```

If the diff is growing unexpectedly, you're likely doing more than a refactor. Stop and re-evaluate.
