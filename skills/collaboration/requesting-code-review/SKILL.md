---
name: requesting-code-review
description: How to prepare and request a code review. Write a useful description, scope the review, and give reviewers what they need to be effective.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Requesting Code Review

A bad review request wastes everyone's time. A good one gets precise, useful feedback in one round.

## Before requesting

### Verify the basics yourself first
```bash
go build ./...              # must pass
go test -race ./...         # must pass
go vet ./...                # must pass
```

Do not ask someone to review code that doesn't build or has failing tests. Fix those first.

### Self-review the diff
```bash
git diff main...HEAD
```

Read every line. Ask yourself:
- Would I understand this code if I saw it for the first time?
- Is there anything I'm not confident about?
- Is there any code I added "just in case" that isn't needed?

Remove anything you added "just in case." Remove commented-out code. Remove debug logging.

## Writing the review description

A review description answers three questions:

**1. What does this change do?**
One paragraph. No implementation details — describe the user-visible or developer-visible behaviour.

Example: "Adds a `/skills` slash command to the TUI. Typing `/skills` and pressing Enter appends a system message listing all skills in `./skills/` with their names and one-line descriptions."

**2. Why is this the right approach?**
One paragraph. Explain the key design decision and why you chose it over alternatives.

Example: "Skills are read from disk at command invocation time (not at startup) so that new skill files are immediately available without restarting the TUI. The YAML front matter parser is custom (no new dependency) since the format is simple enough."

**3. What should the reviewer focus on?**
Explicit scoping saves time. If you're confident about the tests but uncertain about the concurrency model, say so.

Example:
```
Focus areas:
- parseSkillFrontMatter: is the front-matter parsing robust enough?
- listSkillsSummary: does the output format feel right for a TUI?
- Not needed: test coverage (I'm happy with it), imports (clean)
```

## For go-adk-q specifically

### Always include in the description:
- Whether `go build ./...` and `go test ./...` pass (they must)
- Which providers were tested (if the change touches provider code)
- Which TUI behaviours were manually verified (if the change touches `cmd/tui/`)

### Standard review checklist for this codebase:

**TUI changes:**
- [ ] Works at narrow terminal (60 cols)
- [ ] Resizes correctly on window resize
- [ ] No raw `lipgloss.Color()` calls — uses `styledSet`
- [ ] New key bindings are in the help overlay

**Provider changes:**
- [ ] `Config` struct with `ConfigFromEnv()` following existing pattern
- [ ] Wired into failover chain in both `main.go` and `cmd/tui/main.go`
- [ ] `Name()` returns `provider/model` format
- [ ] `PROVIDER_SELECTED` substring match works

**Skill changes:**
- [ ] YAML front matter has `name:` and `description:`
- [ ] Skill directory name matches `name:` field
- [ ] No external dependencies required to use the skill
