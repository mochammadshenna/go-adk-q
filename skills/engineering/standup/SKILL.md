---
name: standup
description: Generate or structure a standup update. Use when preparing for daily standup, summarizing recent commits and PRs, formatting work into yesterday/today/blockers, or turning rough notes into a concise shareable update.
compatibility: Designed for software engineering team workflows.
---
# Standup

Generate a concise, accurate standup update.

## Format

```markdown
## Standup — [Date]

### Yesterday
- [Completed item — be specific, include ticket/PR ref if available]
- [Completed item]

### Today
- [Planned item with ticket reference]
- [Planned item]

### Blockers
- [Blocker with context and who can help]
```

## Process

### Option A: Tell me what you did

Describe your work in rough notes. Example:
> "Worked on the auth migration, reviewed 3 PRs, got blocked on the API rate limiting issue."

I'll structure it into yesterday/today/blockers format.

### Option B: Pull from recent activity

If connected to source control or a project tracker:
1. Pull commits and PRs from the last working day
2. Pull ticket status changes (in-progress → done, backlog → in-progress)
3. Surface any threads or discussions needing response

## What makes a good standup

**Good yesterday item**: "Merged PR #412 — slash command autocomplete menu. Resolves #398."

**Bad yesterday item**: "Worked on the TUI."

**Good blocker**: "Blocked on provider API key — need @alice to share the test credential."

**Bad blocker**: "Blocked on something."

## Tips

- Keep total length under 5 bullets per section
- Lead with completed work, not in-progress work
- Blockers should name the blocker and the person who can unblock it
- Skip ceremony: no "I worked on..." — just list the work
