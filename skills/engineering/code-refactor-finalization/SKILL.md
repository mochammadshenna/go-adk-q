---
name: code-refactor-finalization
description: Complete and close a refactor. Final verification, cleanup, and merge checklist.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Refactor Finalization

Use when a refactor is complete and ready to merge.

## The refactor is done when

- [ ] All tests pass: `go test -count=1 -race ./...`
- [ ] Build is clean: `go build ./...`
- [ ] Vet is clean: `go vet ./...`
- [ ] The diff contains only structural changes — no behaviour changes
- [ ] The diff contains no leftover debug code, TODOs added during refactor, or commented-out old code

## Final diff review

```bash
git diff main...HEAD
```

For each changed function, ask: **does this function do the same thing it did before?**

If yes: structural refactor, safe to merge.
If no: you changed behaviour. That's a bug fix or feature, not a refactor. Separate it into its own commit or PR.

## Verify the rename/move is complete

If you renamed or moved something:
```bash
grep -rn "OldName" .    # should return 0 hits
```

A partial rename is worse than no rename — it creates inconsistency.

## Update any references in docs or skills

If the refactor changed a function name, file location, or API that's referenced in:
- `docs/TESTING.md`
- Any `skills/*/SKILL.md` file (e.g., file:line references in code-review-feedback)
- `README.md`

Update those references now, not later.

## Commit message

Refactor commits should say what was moved/extracted/renamed, not why:

```
refactor: extract handleEnterKey from Update() in chat.go
refactor: move slash menu view logic to slash.go
refactor: rename styledSet.themeId to themeIdx for consistency
```

Not:
```
refactor: improve code quality
refactor: clean up chat.go
refactor: make it better
```

## Merge

```bash
go test -count=1 -race ./...   # final check
git log --oneline main..HEAD   # review commit list
git checkout main
git merge --no-ff feature/refactor-xxx -m "refactor: <summary>"
go build ./... && go test ./... # verify main is green
```
