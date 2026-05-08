---
name: code-refactor-consensus
description: Reach agreement on a contested refactor approach. Separate structural preference from correctness, and decide with evidence.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Code Refactor Consensus

Use when two people disagree on how to refactor a piece of code.

## Refactor disagreements are usually about values

Unlike bug fixes (right/wrong), refactor choices trade between:
- **Readability** vs **brevity**
- **Explicitness** vs **DRY**
- **Consistency with existing code** vs **better patterns**
- **Small safe steps** vs **clean final state**

None of these is objectively correct. The project's existing patterns are the tiebreaker.

## Framework for deciding

### 1. State both approaches concretely
Write out both options in code, not prose. "Method A" vs "function B" is vague. Show the actual signatures and call sites.

### 2. Apply the project's existing patterns
Look at 3 similar cases in the codebase. What pattern do they use?

For go-adk-q:
- Key handlers in `Update()`: inline switch cases (current), or extracted methods?
- State mutation: direct field assignment or return new struct?
- Style rendering: methods on `chatModel` returning `string` (current)

If the codebase already has a clear pattern, match it unless there's a compelling reason not to.

### 3. Ask: does this actually matter?

Many refactor disagreements are about code that is read occasionally and changed rarely. Ask:
- Will this be read by someone new to the codebase? (If yes, readability matters more)
- Will this be changed frequently? (If yes, structure matters more)
- Is this on a hot path? (Rarely relevant in a TUI)

If the answer to all three is "no", pick one and move on.

### 4. Decide

One of:
- **Adopt the existing pattern** — consistency wins when both options are reasonable
- **Use the new pattern** — only if it's clearly better AND you'll update all similar cases for consistency
- **Document and defer** — refactor disagreement isn't blocking; merge as-is, track the cleanup separately

### For go-adk-q specifically

| Contested topic | Position |
|----------------|----------|
| Long `Update()` vs extracted methods | Extracted methods win above ~50 lines |
| Package functions vs methods | Methods for anything accessing `chatModel` fields |
| `strings.Builder` vs `fmt.Sprintf` | `strings.Builder` for multi-line assembly; `fmt.Sprintf` for single-line formatting |
| Chained lipgloss vs intermediate variables | Intermediate variables when chain > 3 calls |
