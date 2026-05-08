---
name: testing-strategy
description: Design test strategies and test plans. Use when deciding how to test a new system, auditing existing test coverage, writing a test plan for a feature, or determining the right balance of unit, integration, and end-to-end tests.
compatibility: Designed for software engineering workflows.
---
# Testing Strategy

Design effective testing strategies balancing coverage, speed, and maintenance.

## Testing pyramid

```
        /   E2E   \         Few, slow, high confidence
       / Integration \      Some, medium speed
      /   Unit Tests  \     Many, fast, focused
```

More unit tests than integration tests. More integration tests than E2E. Invert this and your test suite is slow and fragile.

## Strategy by component type

| Component | Unit tests | Integration tests | E2E / smoke |
|-----------|------------|-------------------|-------------|
| Business logic | Yes — table-driven, edge cases | No | No |
| API endpoints | Yes — handler logic | Yes — HTTP layer | Smoke only |
| Database access | No | Yes — against real DB | No |
| CLI / TUI | Yes — model logic | Yes — key sequence tests | Manual QA |
| Data pipelines | Yes — transformation logic | Yes — idempotency, ordering | Smoke only |

## What to test

**Definitely test:**
- Business-critical paths — if it breaks, users notice immediately
- Error handling — especially error paths that are rarely exercised manually
- Edge cases — empty input, nil, zero, max values, concurrent access
- Security boundaries — auth checks, input sanitisation
- Data integrity — transforms that must be lossless

**Skip:**
- Trivial getters and setters
- Framework code (test your use of the framework, not the framework itself)
- One-off scripts not used in production
- Tests that only verify that a mock was called

## Test quality criteria

A good test:
1. Has a clear name that describes what it's testing: `TestRenderMessage_AgentRole_RendersWithPromptStyle`
2. Has one logical assertion (it can be multiple `t.Error` calls, but one concept)
3. Fails for only one reason
4. Runs in < 1 second without network or disk I/O
5. Is deterministic — same result every run

A bad test:
1. Tests implementation, not behaviour (breaks on refactor)
2. Is flaky — sometimes passes, sometimes fails
3. Tests multiple unrelated things
4. Requires a specific environment to run

## For go-adk-q

See `cmd/tui/mdtest_test.go` for the project's test conventions:
- Table-driven with `t.Run`
- `testModel()` helper to create a model at a specific terminal size
- `assertContains` / `assertNotContains` helpers for viewport content
- Race detector always on: `go test -race ./...`

New tests go in `_test.go` files alongside the code they test. Use `package main` (same package) for white-box tests, `package main_test` (external) for black-box.

## Output format

```markdown
## Test Plan — [Feature/Component]

### What to test
[List of behaviours to verify]

### Test types
[Unit / integration / E2E for each area, with rationale]

### Coverage targets
Minimum: X% | Target: Y%

### Example test cases
[2-3 concrete examples showing test name and what it verifies]

### Existing coverage gaps
[What's currently untested and why it matters]
```
