# Provider Fallback: Resilience, Not Substitution

This document explains a design decision that is easy to misunderstand: the difference between **resilience fallback** and **substitution fallback**, and why dev-task-pubsite implements only the former.

## Two types of fallback

### Substitution fallback

"If provider A is down, use provider B instead."

This pattern makes sense when two providers serve the same data. Example: a primary database and a read replica. If the primary fails, queries fall back to the replica — the caller gets the same type of data from a different source.

### Resilience fallback

"If provider A is down, return A's last known data with a degraded flag."

This pattern makes sense when each provider serves fundamentally different data. The fallback is within the same provider's cache, not across providers.

## Why dev-task-pubsite uses resilience fallback only

YouTrack and GitHub do not serve the same data:

| YouTrack | GitHub |
|---|---|
| Tasks, sprints, stories | Pull requests, issues, commits |
| Project management | Code activity |
| `yt_*` tools | `gh_*` tools |

If YouTrack goes down, there is no meaningful way to satisfy `list_my_tasks` using GitHub. GitHub has no concept of YouTrack projects or sprints. Returning GitHub data in response to a YouTrack query would be:

- **Wrong**: the data types are incompatible
- **Confusing**: the caller would receive silently incorrect results
- **Misleading**: `providerStatus.source` would misrepresent the data origin

## The implementation consequence

Each provider has exactly one fallback path: its own cache.

```
list_my_tasks called
  → YouTrack live? → return live data
  → YouTrack down?
      → cache warm? → return stale data, degraded=true
      → cache cold? → return error
  → NEVER: return GitHub data
```

This is enforced by the architecture: the `yt/` package has no knowledge of the `gh/` package, and vice versa. The server layer registers them independently.

## What the AI agent does with degraded data

When `providerStatus.degraded = true`, Claude (or any MCP client) receives a signal that the data may be stale. A well-behaved AI agent should:

1. Include the staleness in its response: *"Note: YouTrack data is from 4 minutes ago (provider degraded)"*
2. Not block on stale data — proceed with the best available information
3. Optionally suggest the user retry after the circuit breaker resets (120 seconds)

This gives the user accurate situational awareness without completely blocking their workflow.

## Why not implement substitution at all?

Even for data types that could theoretically overlap — for example, GitHub Issues and YouTrack tasks both represent work items — substitution was rejected for these reasons:

1. **Schema mismatch**: YouTrack tasks have sprints, priorities, and custom fields that GitHub issues lack. Substituting would silently lose information.
2. **Identity confusion**: YouTrack task IDs (`PROJ-123`) and GitHub issue numbers (`#42`) live in different namespaces. Cross-provider substitution would corrupt references.
3. **User expectations**: A user asking for YouTrack tasks expects YouTrack tasks, not a GitHub approximation.
4. **Operational clarity**: Degradation is obvious when `degraded: true`. Substitution would require additional metadata to explain what actually happened.

## Future considerations

If a future use case genuinely requires cross-provider aggregation (e.g. "show me all work items across all systems"), that should be implemented as a **new aggregation tool** that explicitly combines both providers, rather than as silent substitution. The tool name and output schema would make the cross-provider nature explicit to the caller.

See [ADR 001](../adr/001-resilience-only-fallback.md) for the formal decision record.
