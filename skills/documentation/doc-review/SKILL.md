---
name: doc-review
description: Review a document for accuracy, clarity, completeness, and fitness for its audience.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Doc Review

Use when a document draft is ready for feedback before being published or merged.

## Review dimensions

### 1. Accuracy
Is every factual claim correct?

For go-adk-q docs:
- Are file names and paths accurate? (verify with `ls`)
- Are function signatures correct? (verify with `grep -n "func Name" <file>`)
- Are commands runnable? (run them)
- Are version numbers current? (check `go.mod`)

Flag every unverified factual claim. Don't assume — check.

### 2. Audience fit
Does the document match its intended reader?

- New user: no assumed knowledge, concrete steps, expected output shown
- Developer/contributor: can assume Go knowledge, can reference file:line, should show patterns
- Skill (for LLM): imperative, concrete actions, explicit decision criteria

Flag: jargon without explanation for a new-user doc; over-explanation of basics for a developer doc.

### 3. Completeness
Does the document cover everything its title promises?

A document titled "Setting up providers" must cover ALL providers documented in the codebase — not just the ones the author used.

A skill titled "code-review-feedback" must cover what to do when you disagree with feedback — not just the happy path.

Flag: missing sections, happy-path-only coverage, undocumented failure modes.

### 4. Clarity
Can the intended reader follow this without asking questions?

Read the document as if you've never seen this codebase. Where do you get confused? Where do you make an assumption? Those are the unclear parts.

Flag: ambiguous pronouns ("it does this"), undefined terms, steps that assume context not yet established.

### 5. Freshness
Does this reflect the current state of the code?

For go-adk-q: the codebase evolves quickly. A doc written last week may already be stale.

Check: do the file names exist? Do the functions mentioned exist with those signatures? Do the commands produce the described output?

## Review output format

```
## Doc review — <document title>

Accuracy:     PASS / N issues
Audience fit: PASS / WARN / FAIL
Completeness: PASS / missing: X, Y
Clarity:      PASS / N unclear passages
Freshness:    PASS / N stale references

Issues:
1. [accuracy] cmd/tui/chat.go:412 — function signature shown is outdated
2. [clarity] "it" in paragraph 3 is ambiguous — what does "it" refer to?
3. [completeness] no coverage of the echo provider fallback

Verdict: APPROVE / REVISE
```

APPROVE means: accurate, complete, and clear for the intended audience.
REVISE means: at least one accuracy or completeness issue must be fixed first.
