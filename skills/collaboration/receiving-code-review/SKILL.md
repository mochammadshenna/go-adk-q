---
name: receiving-code-review
description: How to process incoming code review feedback. Distinguish signal from noise, ask clarifying questions, implement with precision, and close the loop.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Receiving Code Review

Getting a code review is not the end of the process — it's the middle. How you respond determines whether the review made the code better or just added noise.

## Mindset

The reviewer has context you don't. They see your code with fresh eyes. Even if a comment feels wrong, it means something wasn't clear — that's worth fixing.

The reviewer is also human. Some comments are preferences, not requirements. You need to distinguish them.

## Processing feedback

### Categorise every comment

| Category | Definition | Response |
|----------|------------|----------|
| **Bug** | Reviewer found a correctness issue | Fix it. No discussion needed. |
| **Design** | Structural concern (wrong abstraction, wrong layer) | Discuss before implementing. |
| **Style** | Formatting, naming, idiom preference | Fix if it matches project conventions. |
| **Question** | Reviewer is asking for clarification | Answer inline, then clarify the code. |
| **Nitpick** | Minor preference with no real impact | Fix if cheap; mark as acknowledged if expensive. |
| **Wrong** | Reviewer misunderstood the code | Explain, then improve clarity so the next reader doesn't misunderstand. |

### For each comment:
1. Read it twice before responding
2. Categorise it
3. If you disagree, say so — but explain why with evidence, not opinion
4. If you agree, implement the fix and reference the comment in the commit message

## For go-adk-q specifically

Common review patterns in this codebase:

**"This should use the styledSet instead of a raw colour"**
→ Correct. Fix it. Look up the right style name in `styledSet`.

**"This function is too long"**
→ Probably correct. `chat.go:Update()` is already complex — new logic should be in helper functions.

**"Missing test for this"**
→ Correct. Add a table-driven test in `mdtest_test.go` or a new `_test.go` file.

**"This leaks a goroutine"**
→ Critical bug. Look for unbuffered channels, missing `ctx.Done()` handling, or `go func()` without a cancel path.

**"Style nit: use errors.Is instead of == for error comparison"**
→ Fix it. This is a Go idiom, not a preference.

## Closing the loop

After implementing all feedback:
1. Run `go build ./... && go test -race ./...`
2. Write a response to each comment: "Fixed in commit abc1234" or "Acknowledged — deferred to follow-up issue"
3. Request re-review only when all blocking comments are resolved

Never re-request review with unaddressed comments. It wastes the reviewer's time and signals you didn't read the feedback.
