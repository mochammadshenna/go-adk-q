---
name: doc-iteration
description: Process doc review feedback and improve the document through revision cycles.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Doc Iteration

Use after receiving doc review feedback to revise and improve the document.

## Processing feedback

Read all feedback before making any changes. Categorise each item:

| Category | Action |
|----------|--------|
| Factual error | Fix immediately. Verify the correct information before writing. |
| Missing section | Add it. Check other docs in the same category for the expected structure. |
| Clarity issue | Rewrite the unclear passage. Read it aloud — if it sounds awkward, rewrite again. |
| Stale reference | Update to current state. Verify by running the command or checking the file. |
| Style/tone | Fix if it affects readability. Skip pure preferences. |

## Revision principles

### Fix accuracy before clarity
A clearly written wrong statement is worse than an unclear correct one.

### Don't add words to fix clarity
Most clarity problems are caused by too many words, not too few. Rewrite by cutting.

Before: "In order to be able to use the slash command menu, the user must first type a '/' character into the text input field at the bottom of the TUI."

After: "Type '/' to open the slash command menu."

### Update examples when updating text
If you change a description, check that any associated code examples still match.

### One revision pass per reviewer
Don't make micro-edits across 5 passes. Read all feedback, plan your revisions, make them in one pass.

## Verification after revision

For any factual change:
```bash
# Verify file exists at stated path
ls <path>

# Verify function has stated signature
grep -n "func <name>" <file>

# Verify command produces stated output
<command> 2>&1 | head -5
```

For skill documents: read through the revised skill as if you're the LLM executing it. Does every instruction make sense? Is there any step that requires information not yet established?

## When to stop iterating

Stop when:
- All factual issues are resolved
- All completeness gaps are filled
- The document passes a re-read by someone other than the author (or a cold re-read after 30 minutes)

Don't iterate forever on clarity and style. "Good enough to be accurate and useful" is the bar. Perfect prose is not the goal.

## Commit message for doc revisions
```
docs: fix stale function references in engineering/qa
docs: add echo provider coverage to testing guide
docs: clarify slash menu navigation instructions
```
