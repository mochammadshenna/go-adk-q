# ADR 003 — Inline Circuit Breaker, Hardcoded Parameters, No Metrics

**Status**: Accepted  
**Date**: 2026-06-26  
**Deciders**: Senior Principal Architecture

---

## Context

dev-task-pubsite calls two external APIs (YouTrack and GitHub) that can fail or become slow. A circuit breaker pattern was chosen to prevent cascading failures and unnecessary API hammering. Three design questions arose:

1. **Implementation**: use a library (e.g. `sony/gobreaker`, `mercadolibre/hystrix-go`) or implement inline?
2. **Configuration**: make threshold/cooldown configurable via env vars, or hardcode?
3. **Observability**: add a Prometheus metrics endpoint for circuit state, or rely on logs?

---

## Decision

1. **Inline implementation** using `sync.Mutex` and basic counters — no external library
2. **Hardcoded parameters**: threshold=5, cooldown=120s, per provider
3. **Log-only observability**: `slog.Warn` on open/close events; no metrics endpoint

---

## Rationale

### Inline over library

The circuit breaker in dev-task-pubsite is simple: count failures, trip at a threshold, cooldown, reset. The implementation is ~30 lines of Go:

```go
type circuitBreaker struct {
    mu        sync.Mutex
    failures  int
    openUntil time.Time
}

func (cb *circuitBreaker) allow() bool { ... }
func (cb *circuitBreaker) success()    { ... }
func (cb *circuitBreaker) failure()    { ... }
```

Adding a library like `sony/gobreaker` would introduce:
- A new dependency to audit and update
- Configuration types to learn
- Abstractions that obscure what's actually happening

For a circuit breaker this simple, the library provides no material benefit over the inline version. The inline version is also easier to read during incident response.

### Hardcoded parameters

The parameters (5 failures, 120s cooldown) were chosen based on typical task/PR workload characteristics:

- **5 failures**: enough to distinguish transient network blips (1-2 failures) from genuine outages
- **120 seconds**: long enough to avoid hammering a recovering service; short enough that users notice restored functionality within 2 minutes

Making these configurable (via env vars) would add:
- Documentation burden (explain what each parameter means, safe ranges)
- Operational complexity (operators must understand circuit breaker semantics to tune them safely)
- Testing complexity (all failure/cooldown combinations must be tested)

For the anticipated deployment scale (single-instance, developer tooling), the hardcoded values are appropriate. If operational needs change significantly, parameters can be made configurable in a future iteration.

### Log-only observability

A Prometheus metrics endpoint would expose:
- Circuit state (open/half-open/closed)
- Failure counts
- Cache hit/miss rates

For a developer tooling server, this is not necessary. dev-task-pubsite runs as a personal or team tool, not as infrastructure. Connecting it to a metrics stack would add:
- A new runtime dependency (Prometheus client library)
- Infrastructure requirements (Prometheus scrape config, Grafana dashboards)
- Operational burden disproportionate to the tool's purpose

The current observability surfaces are sufficient:
- **Startup logs**: which providers are available
- **Tool responses**: `providerStatus.degraded` visible to AI agent and user
- **Health endpoint**: `/health` shows live provider status
- **Structured slog**: circuit breaker open/close events

If this server is promoted to shared infrastructure, metrics can be added at that point.

---

## Consequences

### Positive

- Zero new dependencies for circuit breaking
- Circuit breaker logic is fully visible and auditable inline
- No configuration surface area to document or misuse
- Operational simplicity: no metrics infrastructure required

### Negative

- Circuit breaker parameters cannot be tuned without recompiling
- No time-series visibility into failure rates or circuit state history
- If requirements change (higher volume, multi-instance deployment), the inline implementation may need to be replaced with a proper library

### Neutral

- The two circuit breakers (YouTrack and GitHub) share no state, which is correct — independent failure domains should have independent breakers

---

## Upgrade path

If the project outgrows this design:

1. **Configurable parameters**: introduce `CB_THRESHOLD` and `CB_COOLDOWN_SECONDS` env vars; add to `reference/environment-variables.md`
2. **Library**: replace inline `circuitBreaker` with `sony/gobreaker`; the provider clients' public APIs are unchanged
3. **Metrics**: add `prometheus/client_golang`; expose `/metrics` from the Gin router in `server.go`

These can be done independently without touching the tool or domain layers.
