---
name: code-review
description: Review code changes for security, performance, correctness, and maintainability. Use when given a PR URL, diff, or file path; when checking a change before merge; or when looking for N+1 queries, injection risks, missing edge cases, or error handling gaps.
compatibility: Designed for software engineering workflows.
---
# Code Review

Review code changes with a structured lens on security, performance, correctness, and maintainability.

## Review checklist

### Correctness
- Logic bugs, off-by-one errors, missing edge cases
- Null/nil dereferences, unchecked type assertions
- Return values on all code paths

### Security
- Injection risks (SQL, shell, template)
- Improper input validation or sanitisation
- Secret or credential exposure in code or logs
- Authentication and authorisation bypass paths

### Error handling
- Unhandled errors (especially in Go: `_` discarding errors)
- Silent failures — errors swallowed without logging or propagation
- Missing cleanup on error paths (deferred close, unlock, etc.)

### Concurrency
- Data races, missing mutex protection
- Deadlocks: lock ordering, circular waits
- Goroutine leaks: goroutines started but never terminated

### Performance
- Unnecessary allocations in hot paths
- O(n²) where O(n) is achievable
- Missing indexes, N+1 query patterns
- Unbounded memory growth

### Readability and maintainability
- Naming clarity — does the name explain the intent?
- Function length — over ~50 lines is a smell worth noting
- Duplication — copy-paste that should be extracted
- Missing comments on non-obvious logic

### Test coverage
- Is the changed behaviour tested?
- Are negative cases and boundary conditions covered?
- Are tests testing behaviour, not implementation?

## Severity levels

| Level | Meaning | Must fix? |
|-------|---------|-----------|
| **Critical** | Bug, data loss, race, security hole | Yes |
| **Major** | Wrong abstraction, missing error handling | Yes |
| **Minor** | Style, naming, missing comment | Preferred |
| **Nit** | Whitespace, trivial rename | Optional |

## Output format

```
## Review — <description of change>

Critical: N | Major: N | Minor: N | Nit: N

### Issues

**[Critical] file.go:42** — Description of what's wrong and why it matters.
Suggested fix: ...

**[Major] file.go:108** — Description.

### Approved
[What's done well or what makes this safe to merge after fixes]

### Verdict: APPROVE / CHANGES REQUESTED
```

## Review principles

- Lead with the most severe issues
- Be specific: cite file and line number
- Explain why, not just what — "this causes a data race because..." not just "fix this"
- Distinguish objective issues (correctness, security) from preferences (style)
- Acknowledge good work — a review that's all criticism is demoralising
