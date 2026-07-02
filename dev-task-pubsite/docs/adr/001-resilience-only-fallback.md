# ADR 001 — Resilience-Only Provider Fallback

**Status**: Accepted  
**Date**: 2026-06-26  
**Deciders**: Senior Principal Architecture

---

## Context

dev-task-pubsite connects to two external providers:

- **YouTrack**: task management — projects, sprints, issues
- **GitHub**: code activity — PRs, issues, commits, repository stats

Both providers are external SaaS systems that can be temporarily unavailable. The question was: when one provider is down, what should the MCP server return?

Two options were considered:

1. **Substitution fallback**: if YouTrack is down, attempt to satisfy YouTrack tool calls using GitHub data (or vice versa)
2. **Resilience fallback**: if a provider is down, return that provider's cached data with a `degraded` flag; never substitute with the other provider

---

## Decision

We adopt **resilience-only fallback** (option 2).

Each provider degrades independently:
- When unavailable: return stale cached data with `providerStatus.degraded = true`
- When cache is cold: return an error
- Never: return data from the other provider

---

## Rationale

### YouTrack and GitHub serve fundamentally different data

YouTrack tasks have sprints, custom fields, and project-scoped IDs (`PROJ-123`). GitHub issues have labels, milestones, and repository-scoped numbers (`#42`). These are not interchangeable:

- A sprint breakdown (`stateBreakdown`) has no equivalent in GitHub
- A GitHub CI status (`ciStatus`) has no equivalent in YouTrack
- Cross-provider substitution would silently drop or misrepresent these fields

### Silent substitution is deceptive

If `list_my_tasks` silently returned GitHub issues when YouTrack was down, the AI agent would present GitHub data as if it were YouTrack data. The user would make decisions based on wrong information without knowing it.

By contrast, `degraded: true` makes the situation explicit. The AI agent can acknowledge the staleness and the user can decide how to proceed.

### Identity integrity

YouTrack task IDs and GitHub issue numbers live in different namespaces. Cross-provider substitution would corrupt task references used in commit messages, PR descriptions, and project tracking.

### Implementation simplicity

Resilience fallback requires no cross-provider knowledge. The `yt/` package does not import `gh/`, and vice versa. This makes each provider independently testable and replaceable.

Substitution fallback would require:
- Schema mapping logic (YouTrack field → GitHub equivalent)
- Identity translation (PROJ-123 → #42 ?)
- Explicit caller notification of what actually happened
- Testing of all cross-provider failure combinations

The complexity cost is not justified for the benefit delivered.

---

## Consequences

### Positive

- Each provider is independently observable: `providerStatus.source` always accurately names the data origin
- Simpler code: no cross-provider imports or mapping logic
- Clear degradation signal for AI agents and users
- Each provider can be replaced or extended independently

### Negative

- When both providers are down and cache is cold, some tools return errors rather than any data
- Users who expect "best effort across all sources" must understand that YouTrack and GitHub are separate domains

### Neutral

- Cross-provider aggregation (if ever needed) should be implemented as an explicit aggregation tool, not as silent fallback

---

## Alternatives considered

### Substitution fallback (rejected)

Return GitHub issues in response to YouTrack queries when YouTrack is down. Rejected because the data schemas are incompatible, identity namespaces differ, and silent substitution misleads callers.

### Single provider (rejected)

Connect to only one provider. Rejected because the value proposition of the server is precisely the bridge between task management and code activity.

### No fallback at all (rejected)

Return errors immediately when a provider is unavailable. Rejected because stale-but-useful data is significantly better than an error for an AI agent's ability to help the user.
