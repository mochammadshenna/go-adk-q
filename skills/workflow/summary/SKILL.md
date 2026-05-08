---
name: summary
description: Produce a concise, accurate summary of a conversation, code review, design session, or document. Captures decisions, open questions, and next steps.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Summary

Use to close out a working session, document a decision, or produce a handoff note.

## When a summary is useful

- End of a coding session: what was done, what's left, what decisions were made
- After a code review: what feedback was given, what was accepted, what was deferred
- After a design discussion: what was decided and why, what was explicitly rejected
- Before handing off work to someone else

## What a good summary contains

### 1. What happened (facts, not opinions)
List what was actually done or discussed. Verifiable against the work.

Example: "Added slash command autocomplete menu to the TUI. 4 commands: /settings, /theme, /help, /clear, /skills. Menu appears on '/', navigated with ↑/↓, completed with Tab, dismissed with Esc."

### 2. Decisions made (and why)
Not just what was decided — the reasoning too. The reasoning is what prevents future re-litigation.

Example: "Removed tea.WithMouseCellMotion() to restore native terminal text selection. Touchpad scroll was deemed less important than copy/paste. Later reversed after user feedback: both can coexist with Shift+drag for selection."

### 3. What was explicitly NOT done
Scope boundaries. Prevents "I thought we were also going to..." conversations.

Example: "Did not implement per-message copy buttons (too much visual noise). Did not add mouse click navigation (out of scope)."

### 4. Open questions
Anything unresolved that will affect future work.

Example: "Unknown: does tea.WithMouseCellMotion block Shift+drag in all terminals, or only iTerm2?"

### 5. Next steps
Concrete, actionable, assigned if possible.

Example:
```
- [ ] Add /skills to the help overlay binding list (chat.go:1081)
- [ ] Write test for listSkillsSummary() with empty skills dir
- [ ] Manual QA: verify slash menu at 60-col terminal width
```

## Format

For a session summary:
```markdown
## Session summary — YYYY-MM-DD

### Done
- ...

### Decisions
- [decision]: [why]

### Not done / deferred
- ...

### Open questions
- ...

### Next steps
- [ ] ...
```

For a code review summary:
```markdown
## Review summary — <branch/PR>

### Feedback given (N comments)
Critical: N | Major: N | Minor: N | Nit: N

### Accepted
- ...

### Deferred
- ...

### Verdict: APPROVED / CHANGES REQUESTED
```

## Rules

- Write the summary before you lose context — do it right after the session ends
- Be specific: file names, line numbers, function names
- If you can't summarise a decision in one sentence, the decision wasn't clear enough
- A summary that takes more than 10 minutes to write is too long
