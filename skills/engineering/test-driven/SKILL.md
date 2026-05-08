---
name: test-driven
description: TDD workflow for go-adk-q. Write the failing test first, implement the minimum code to pass, then refactor. Covers unit, integration, and table-driven patterns used in this codebase.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Test-Driven Development — go-adk-q

Use this skill when adding a new feature, fixing a bug, or refactoring. Write the test first. Always.

## The cycle

```
RED   → write a failing test that describes the desired behaviour
GREEN → write the minimum code to make it pass
CLEAN → refactor without breaking the test
```

Never write implementation code without a failing test first. If you catch yourself thinking "I'll add the test after" — stop and reverse.

## Project test conventions

- Test files live next to the code they test: `foo.go` → `foo_test.go`
- Use `package main` (white-box) for TUI internals; `package foo_test` (black-box) for public APIs
- Table-driven tests with `t.Run`:
  ```go
  tests := []struct {
      name  string
      input string
      want  string
  }{
      {"empty", "", ""},
      {"plain text", "hello", "hello\n"},
  }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
          got := renderMarkdown(s, tt.input, 80, lipgloss.NewStyle())
          if got != tt.want {
              t.Errorf("got %q, want %q", got, tt.want)
          }
      })
  }
  ```
- Use `t.Helper()` in shared assertion helpers
- Use `-race` flag when testing anything concurrent: `go test -race ./...`

## Adding a new TUI feature — step by step

1. **Describe the behaviour** in plain English before touching code.
   Example: "When the user types `/skills`, the slash menu should show it as an option."

2. **Write the test** in `cmd/tui/`:
   ```go
   func TestSlashMatchesSkills(t *testing.T) {
       matches := slashMatches("/sk")
       found := false
       for _, m := range matches {
           if m.name == "/skills" {
               found = true
           }
       }
       if !found {
           t.Error("expected /skills in slash matches for prefix /sk")
       }
   }
   ```

3. **Run it — watch it fail:**
   ```
   go test ./cmd/tui/ -run TestSlashMatchesSkills -v
   ```
   A test that never fails is not a test.

4. **Implement the minimum code** to make it pass.

5. **Run the full suite** — no regressions allowed:
   ```
   go test ./cmd/tui/ -count=1
   ```

6. **Refactor** if needed, keeping tests green.

## Bug fix workflow

1. **Reproduce first.** Write a test that fails because of the bug.
2. **Commit the failing test** (optional but recommended — proves the bug existed).
3. **Fix the bug.** The test goes green.
4. **Run full suite.** Ship.

Never fix a bug without a regression test. The next person to touch that code will thank you.

## What to test in this codebase

| Area | What to cover |
|------|---------------|
| `markdown.go` | `renderMarkdown`: all themes × input types; narrow terminal fallback; empty input |
| `slash.go` | `slashMatches`: prefix filtering, case insensitivity, empty prefix, unknown prefix |
| `slashMenuView` | non-empty output for all themes; selectedIdx clamping |
| `parseSegments` | prose only, code only, mixed, unclosed fence, empty |
| `glamourStyleName` | all 5 theme indices return valid glamour style strings |
| `invalidateRendererCache` | cache is empty after call; subsequent render doesn't panic |
| Provider model names | `oaibridge.Name()` returns `provider/model` format |

## What NOT to unit-test

- The full `tea.Program` lifecycle (too much setup, no value)
- HTTP calls to real providers (use integration tests or manual smoke tests)
- `copyToClipboard` (system call — test with the QA skill instead)

## Running tests

```bash
go test ./...                    # all packages
go test ./cmd/tui/ -v            # TUI tests with names
go test ./cmd/tui/ -run TestMd   # filter by name prefix
go test -race ./...              # race detector
go test -cover ./...             # coverage report
```

Coverage goal: keep `cmd/tui/` above 60%. Check with:
```bash
go test -coverprofile=cov.out ./cmd/tui/ && go tool cover -func=cov.out
```
