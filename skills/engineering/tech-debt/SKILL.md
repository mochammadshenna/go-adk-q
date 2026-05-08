---
name: tech-debt
description: Identify, categorize, and prioritize technical debt. Use when doing a code health audit, deciding what to refactor, building a maintenance backlog, or making the case to stakeholders for paying down debt.
compatibility: Designed for software engineering workflows.
---
# Tech Debt Management

Systematically identify, categorize, and prioritize technical debt.

## Debt categories

| Type | Examples | Risk |
|------|----------|------|
| **Code debt** | Duplicated logic, poor abstractions, magic numbers | Bugs, slow development |
| **Architecture debt** | Monolith that should be split, wrong data store | Scaling limits |
| **Test debt** | Low coverage, flaky tests, missing integration tests | Regressions ship |
| **Dependency debt** | Outdated libraries, unmaintained dependencies | Security vulns |
| **Documentation debt** | Missing runbooks, outdated READMEs, tribal knowledge | Onboarding pain |
| **Infrastructure debt** | Manual deploys, no monitoring, no IaC | Incidents, slow recovery |

## Prioritization framework

Score each debt item on three dimensions (1-5 scale):

- **Impact**: How much does it slow the team down or risk breakage?
- **Risk**: What's the cost of NOT fixing it?
- **Effort**: How hard is the fix? (Lower effort = higher priority, so invert this)

**Priority score = (Impact + Risk) × (6 − Effort)**

High-priority items: high impact + high risk + low effort. These pay for themselves quickly.

## Identification process

### For code debt
```bash
# Find long functions (>50 lines is a smell)
grep -n "^func " cmd/tui/chat.go | head -20

# Find duplicated error handling patterns
grep -rn "if err != nil" . --include="*.go" | wc -l

# Find TODO/FIXME/HACK markers
grep -rn "TODO\|FIXME\|HACK" . --include="*.go"
```

### For test debt
```bash
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1  # total coverage
```

### For dependency debt
```bash
go list -m -u all 2>/dev/null | grep '\['  # modules with available updates
```

## Output format

```markdown
## Tech Debt Audit — [Date]

### Summary
Total items: N | Critical: N | High: N | Medium: N | Low: N

### Critical (fix before next release)
| Item | Type | Impact | Risk | Effort | Score |
|------|------|--------|------|--------|-------|
| [description] | code | 5 | 5 | 2 | 40 |

### High priority
[same table]

### Remediation plan
Phase 1 (this sprint): [specific items]
Phase 2 (next sprint): [specific items]
Phase 3 (backlog): [remaining items]

### What NOT to fix
[Items that are too costly relative to benefit]
```

## Communicating debt to stakeholders

Translate technical debt into business terms:
- **Code debt**: "This function is 800 lines long. Every bug fix takes 3x longer than it should and risks introducing regressions."
- **Test debt**: "We have 40% test coverage. When we shipped the last release, 3 bugs made it to production that tests would have caught."
- **Dependency debt**: "We're 2 major versions behind on our auth library. Known CVEs affect our current version."

Make the cost of NOT fixing it concrete.
