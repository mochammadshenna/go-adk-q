---
name: qa
description: QA checklist and verification workflow for the go-adk-q TUI. Covers build, tests, provider smoke tests, UI behaviour, and regression checks.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# QA — go-adk-q

Use this skill when asked to QA, test, or verify the application works correctly.

## Quick checklist (run first)

```
go build ./...                          # must produce zero output
go test ./... -count=1 -race            # all tests green, no races
go vet ./...                            # zero warnings
```

Any failure here is a **blocker** — stop and fix before continuing.

## Provider smoke tests

For each provider you want to verify, set the key and send a test prompt:

```
GITHUB_PAT=<pat> go run ./cmd/tui        → type: "say hi"  expect: non-empty reply
GOOGLE_API_KEY=<key> go run ./cmd/tui    → type: "say hi"
GROQ_API_KEY=<key> go run ./cmd/tui      → type: "say hi"
```

Failover test: unset all keys → expect the echo provider to respond.

## TUI behaviour checklist

Work through these in order. Screenshot or describe what you see for each.

### Startup
- [ ] Header shows correct model name
- [ ] Footer hint line is readable
- [ ] Input box is focused, cursor visible

### Slash command menu
- [ ] Type `/` → menu appears above input with all 4 commands
- [ ] Type `/th` → only `/theme` shown
- [ ] `↑`/`↓` moves selection highlight
- [ ] `Tab` completes `/theme` into input
- [ ] `Esc` clears input and closes menu
- [ ] `Enter` on single match executes the command

### Theme cycling
- [ ] `t` key (empty input) cycles theme; all message colours update immediately
- [ ] `/theme` slash command does the same
- [ ] Markdown code blocks change colour with theme (open a new session, ask for a Go snippet, then cycle theme — verify the code block colour changes)

### Markdown rendering
Ask the model: *"Show me a Go code block, a table, a blockquote, and a bullet list."*
- [ ] Code block has syntax highlighting
- [ ] Table columns are aligned
- [ ] Blockquote is visually indented
- [ ] Bullet list has bullet characters

### Scrolling
- [ ] `pgup` / `pgdn` scrolls viewport
- [ ] Touchpad two-finger scroll works
- [ ] Header shows `▼ more` when scrolled up

### Copy
- [ ] `ctrl+y` copies last agent reply; status bar shows "Copied reply" or "Copied code"
- [ ] Shift+click-drag selects terminal text (terminal-dependent)

### Settings overlay
- [ ] `/settings` opens the huh form
- [ ] Change theme in settings, confirm → theme updates
- [ ] `Esc` in settings cancels without applying

### History navigation
- [ ] Send two messages; `↑` cycles back through them
- [ ] `↓` restores draft

### Edge cases
- [ ] Empty input + Enter does nothing
- [ ] `/clear` resets message list to the system greeting
- [ ] `ctrl+l` does the same as `/clear`
- [ ] Very long agent reply is scrollable

## Regression guard

After any change to `cmd/tui/`:

```
go test ./cmd/tui/ -count=1 -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL)"
```

All 28 tests must pass.

## Report format

```
## QA run — <date>

Build:   PASS / FAIL
Tests:   PASS (N) / FAIL (list failures)
Vet:     PASS / FAIL

Provider smoke:
  github-models: PASS / FAIL / SKIP (no key)
  gemini:        PASS / FAIL / SKIP
  echo:          PASS

TUI checklist:  N/M items verified
Issues found:
  - [severity] description

Overall: SHIP / NEEDS FIXES
```
