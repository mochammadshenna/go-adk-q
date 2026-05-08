---
name: code-review-consensus
description: Reach agreement on contested review feedback. Distinguish preference from correctness, escalate when stuck, and document decisions.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Review Consensus

Use when a review comment has generated disagreement and both parties need to reach a decision.

## Types of disagreement

### Preference vs correctness
Most review disagreements are actually preference disagreements disguised as correctness claims.

**Correctness disagreement:** One of you is objectively wrong. There's a right answer.
- "This code has a data race" — verify with `-race` and the race detector
- "This returns the wrong value for empty input" — write a test and run it
- "This leaks a goroutine" — trace the goroutine lifecycle

For correctness disputes: write a test. The test settles it.

**Preference disagreement:** Both approaches work. You're debating style, naming, or structure.
- "I'd call this `entries` not `skills`"
- "I prefer a method over a package function here"
- "This could use a table instead of a switch"

For preference disputes: use the project's existing patterns as the tiebreaker. If the codebase uses one pattern consistently, follow it. If no pattern exists, the author's choice wins unless the reviewer has a strong reason.

## Reaching consensus

### Step 1: Clarify the claim
Restate what both parties actually believe. Often the disagreement dissolves once the claims are stated precisely.

"You're saying this is a bug because X. I'm saying it's correct because Y. Is that right?"

### Step 2: Find the evidence
For correctness: write a test, read the spec, or trace the execution.
For style: find 3 other examples in the codebase. What pattern do they use?

### Step 3: Make a decision
One of:
- Author changes it (reviewer was right)
- Reviewer accepts it (author was right or it's genuinely a preference)
- Both agree on a third option
- Document the disagreement and move on (rare, for deep design questions)

### Step 4: Document it
If the decision will recur, document it. For go-adk-q: add a comment at the decision site explaining why.

```go
// We use styledSet styles exclusively in view functions (not raw lipgloss.Color)
// so that theme cycling recolours all elements together. See cmd/tui/styles.go.
```

## Escalation

If you've gone through 3 rounds of the same comment without resolution, escalate:
1. State the decision that needs to be made in one sentence
2. List the two options with their tradeoffs
3. Ask a third person (or just pick one and document why)

Don't let a single comment block a PR for more than 2 iterations. Merge with a documented disagreement if necessary.

## For go-adk-q specifically

Common areas of legitimate disagreement and the project's position:

| Contested topic | Project position |
|----------------|-----------------|
| Raw colour vs styledSet | Always styledSet. No exceptions. |
| Method vs package function | Methods for anything that reads chatModel state. |
| New file vs adding to existing file | New file if > ~100 lines or logically separate. |
| Buffered vs unbuffered channels | Buffered (64) for stream channels. Unbuffered only for synchronisation signals. |
| Table-driven vs individual test functions | Table-driven. It's the Go convention and this codebase follows it. |
