---
name: health
description: Code health dashboard for go-adk-q. Runs build, vet, tests, race detector, coverage, and staticcheck. Produces a scored report and flags regressions.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Health — go-adk-q

Use this skill to get an objective picture of codebase health. Run it before a release, after a big refactor, or whenever things feel shaky.

## Full health check

Run all checks in order. A failure in an earlier check doesn't skip later ones — collect all results.

### 1. Build

```bash
go build ./...
```

Pass: zero output.
Fail: any output. **Blocker — nothing else matters until this is green.**

### 2. Vet

```bash
go vet ./...
```

Pass: zero output.
Fail: any output. Fix before merging.

### 3. Tests

```bash
go test -count=1 ./... 2>&1
```

Pass: `ok` for every package.
Fail: any `FAIL` line. List the failing tests.

### 4. Race detector

```bash
go test -race -count=1 ./... 2>&1
```

Pass: same as tests, no `DATA RACE` output.
Fail: any `DATA RACE` report. Races are bugs — fix them.

### 5. Coverage

```bash
go test -coverprofile=/tmp/go-adk-q-cov.out ./... 2>&1
go tool cover -func=/tmp/go-adk-q-cov.out | tail -1
```

Report the total coverage percentage. Targets:
- `cmd/tui/`: ≥ 60% (complex TUI logic)
- `model/*/`: ≥ 40% (provider adapters, hard to unit test fully)
- Overall: ≥ 40%

Below target is a warning, not a blocker.

### 6. staticcheck (if installed)

```bash
staticcheck ./... 2>&1
```

If not installed:
```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...
```

Pass: zero output.
Fail: list all findings. SA-class findings are bugs. S1-class are style.

### 7. Module hygiene

```bash
go mod tidy
git diff --exit-code go.mod go.sum
```

Pass: no diff (tree is clean after tidy).
Fail: `go.mod` or `go.sum` changed — commit the tidy result.

### 8. Dead code (optional, slow)

```bash
go install golang.org/x/tools/cmd/deadcode@latest
deadcode -test ./... 2>&1 | head -40
```

Report any unexported functions or types with zero callers. These are candidates for deletion.

## Scoring

| Check | Weight | Score |
|-------|--------|-------|
| Build | 30 | PASS=30, FAIL=0 |
| Vet | 15 | PASS=15, FAIL=0 |
| Tests | 25 | PASS=25, partial=(passing/total)*25 |
| Race | 15 | PASS=15, any race=0 |
| Coverage | 10 | ≥60%=10, ≥40%=7, ≥20%=4, <20%=0 |
| staticcheck | 5 | 0 findings=5, SA only=3, any=0 |

Total: 100 points.

| Score | Grade | Meaning |
|-------|-------|---------|
| 95-100 | A | Ship it |
| 80-94 | B | Minor issues, ship with awareness |
| 60-79 | C | Technical debt accumulating, address soon |
| < 60 | D | Needs work before next feature |

## Report format

```
## Health report — <date>

Build:        PASS                           [30/30]
Vet:          PASS                           [15/15]
Tests:        PASS  28/28                    [25/25]
Race:         PASS                           [15/15]
Coverage:     62.3% (cmd/tui: 64.1%)        [10/10]
staticcheck:  2 findings (S1 only)          [ 3/ 5]

Total: 98/100 — Grade A

Findings:
  staticcheck:
    cmd/tui/chat.go:412: S1039: unnecessary use of fmt.Sprintf
    cmd/tui/markdown.go:83: S1023: redundant return statement

Recommendations:
  - Fix the 2 staticcheck S1 findings (15 min)
  - No blockers
```

## Quick check (60 seconds)

When you just want a sanity check before committing:

```bash
go build ./... && go test -count=1 ./... && go vet ./...
```

All three must produce zero output. If any fails, don't commit.

## Regression tracking

Compare today's score to the last run. If the score dropped more than 5 points, flag it and identify what changed:

```bash
git log --oneline -10   # what changed recently
git diff HEAD~5 -- '*.go' | grep '^+' | wc -l   # rough churn estimate
```
