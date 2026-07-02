# Resilience Model

This document explains how dev-task-pubsite handles provider failures, and why the design choices were made.

## The core principle: degrade, don't crash

When a provider (YouTrack or GitHub) is unavailable, the server does not return an error to the MCP caller. Instead, it:

1. Returns the most recently cached data
2. Sets `providerStatus.degraded = true` in the response
3. Sets `providerStatus.staleAge` to show how old the data is

The AI agent (Claude) receives data that may be minutes or hours old, but it receives _something_. It can note the degraded status in its response to the user.

## Two independent failure domains

YouTrack and GitHub are completely isolated:

```
YouTrack unavailable                GitHub unavailable
        │                                   │
        ▼                                   ▼
yt_* tools → stale cache          gh_* tools → stale cache
gh_* tools → live data ✓          yt_* tools → live data ✓
```

There is no cross-provider fallback. If YouTrack is down, the server does not attempt to satisfy a `list_my_tasks` call with GitHub data. That would be meaningless — they serve different purposes (task management vs. code activity).

## The cache

Each provider has an in-memory TTL cache keyed by operation parameters:

```
cache key: "tasks:PROJ:sprint=Sprint14:state=In Progress"
cache value: []domain.Task
TTL: 5 minutes
```

`cache.Get()` returns a `GetResult`:

```go
type GetResult struct {
    Value any
    Found bool   // entry exists (may be stale)
    Stale bool   // entry exists but past TTL
    Age   time.Duration
}
```

When `Stale` is true, the provider returns the value anyway — the tool handler detects this and sets `Degraded = true` in `ProviderStatus`.

## The circuit breaker

The circuit breaker prevents hammering a failing upstream with repeated requests:

```
Normal:  requests pass through → success resets failure count
         5 failures → circuit opens (120 second cooldown)

Open:    requests return immediately with "circuit breaker open" error
         cache is used if available

Reset:   after 120 seconds, next request is allowed through
         success → circuit closes; failure → reopens
```

Parameters (hardcoded in provider clients):

| Parameter | Value |
|---|---|
| Failure threshold | 5 consecutive failures |
| Cooldown duration | 120 seconds |
| Request timeout | 15 seconds |

These are deliberately hardcoded rather than configurable. Making them env vars would add operational complexity for marginal benefit — the values work well for typical task/PR workloads.

## What happens at startup

```
YOUTRACK_URL + YOUTRACK_TOKEN set?
  → yt.Client created → yt_* tools registered
  → missing? → warning logged → yt_* tools absent (not degraded, just absent)

GITHUB_TOKEN set?
  → gh.Client created → gh_* tools registered
  → missing? → warning logged → gh_* tools absent

Both absent?
  → server starts in fully degraded mode
  → ui://task-dashboard resource still available
  → health endpoint returns {"status":"degraded"}
```

The server never refuses to start due to missing configuration. This is intentional for containerised deployments where secrets may be injected after the binary starts.

## Degraded vs. absent

| Condition | Tool present? | Response |
|---|---|---|
| Provider configured, live | Yes | Live data, `degraded: false` |
| Provider configured, unreachable, cache warm | Yes | Stale data, `degraded: true`, `staleAge` set |
| Provider configured, unreachable, cache cold | Yes | Error returned to caller |
| Provider not configured (missing env vars) | No | Tool not registered; caller sees "unknown tool" |

"Cache cold" means the tool was never successfully called since startup. In this case there is nothing to serve as stale, so the error propagates. The circuit breaker still opens to prevent repeated hammering.

## Observability

Currently, resilience state is communicated through:

- **Startup logs**: `WARN: YouTrack unavailable: ...` 
- **Tool responses**: `providerStatus.degraded` and `providerStatus.staleAge`
- **Health endpoint**: `{"youtrack": true/false, "github": true/false}`
- **Structured slog output**: circuit breaker open/close events at `WARN` level

There is no metrics endpoint (Prometheus, etc.) — this is appropriate for the current deployment scale. See [ADR 003](../adr/003-circuit-breaker-design.md) for the decision rationale.

## Limitations of this model

1. **No background refresh**: cache entries only refresh when a tool is called. A tool with no callers for 6 minutes will serve stale data on its next call.
2. **In-memory only**: cache state is lost on restart. First calls after a restart always go to the live provider.
3. **No partial degradation within a provider**: if a single YouTrack project is unavailable but others are fine, the entire YouTrack circuit breaker treats it as a provider failure.
4. **No jitter on circuit breaker reset**: the 120-second cooldown is fixed, which could cause a thundering herd if many clients restart simultaneously.
