---
name: code-review-finalization
description: Close out a code review cycle. Final verification, merge checklist, and post-merge steps for go-adk-q.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Review Finalization

Use when the review is approved and you're about to merge. Don't skip this — the last step before merge is where surprises hide.

## Pre-merge checklist

```bash
# 1. Rebase or merge with main to catch any conflicts
git fetch origin
git rebase origin/main    # or: git merge origin/main

# 2. Full verification
go build ./...
go test -count=1 -race ./...
go vet ./...

# 3. Check for stray debug output or TODOs added during the review cycle
git diff main...HEAD | grep -E "fmt.Print|log.Print|TODO|FIXME|HACK|XXX"
```

All three build/test/vet commands must produce zero output. Any grep hit must be intentional.

## Final diff review

Read `git diff main...HEAD` one more time.

Look specifically for:
- Any change that wasn't discussed in the review (scope creep that snuck in)
- Any revert that was accidentally included
- Any merge conflict marker left behind (`<<<<<<<`, `=======`, `>>>>>>>`)

## Merge

For go-adk-q: squash-merge is preferred for feature branches to keep `main` history clean. Keep separate commits only for genuinely independent changes.

```bash
# Option A: squash merge (preferred for features)
git checkout main
git merge --squash feature/my-branch
git commit -m "feat: <one-line summary of what the PR does>"

# Option B: merge commit (preferred for multi-part work)
git merge --no-ff feature/my-branch -m "feat: <summary>"
```

## Post-merge steps

### 1. Delete the branch
```bash
git branch -d feature/my-branch
git push origin --delete feature/my-branch
```

### 2. Verify main is green
```bash
git checkout main
go build ./... && go test ./...
```

### 3. Update relevant docs (if applicable)
- `docs/TESTING.md` — if new test patterns were introduced
- `docs/RETRO.md` — note the merge if it's significant
- Skill files — if the change affects how a skill should be used

### 4. Check the skill list
If a new skill was added:
```bash
ls skills/
```
Verify `/skills` in the TUI shows the new entry correctly.

## What "done" looks like

- `main` builds clean
- All tests pass
- Branch deleted
- Any doc updates committed
- The feature works end-to-end (run a manual smoke test if the change was significant)
