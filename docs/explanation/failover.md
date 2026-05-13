# Explanation: Failover design

This document explains the design of `model/failover` — why it works the way
it does and what trade-offs were made.

---

## Motivation

LLM providers return transient errors: rate-limit 429s, 503 overloads, model
routing failures. Without failover, a single provider error halts the
conversation. With failover, the agent automatically retries on the next
available provider.

---

## The buffering trade-off

The most natural implementation would stream directly from the winning
provider's response iterator to the caller. But streaming creates a problem:

> Once the first token is yielded to the caller, the caller has committed to
> that provider. A mid-stream error cannot be retried transparently — the
> caller already received partial output.

`failover.Model` resolves this by buffering: every provider's
`GenerateContent` is called with `stream=false` and the full response is
collected before it is forwarded to the caller. This means:

- If a provider errors mid-response, the error is caught before any output
  reaches the caller.
- The next provider is tried with the original request.
- The caller receives a complete, single response.

**Trade-off:** The first token is delayed until the full response is buffered.
For long responses this adds latency. The design prioritises **reliability
over streaming latency**.

See [ADR-0003](../adr/ADR-0003-failover-buffered.md) for the decision record.

---

## Provider ordering

Providers are tried left-to-right in the order passed to `failover.New`.
In `cmd/tui/main.go`:

```
GitHub Models → Google Gemini → Groq → NVIDIA → OpenRouter → Hugging Face
```

The ordering reflects:
1. Free/low-cost providers first (GitHub Models has a free tier).
2. High-quality providers before speed-optimised providers.
3. Well-tested integrations before newer ones.

You can reorder by changing the argument order to `failover.New`.

---

## Nil model handling

`failover.New` silently drops nil models. This lets provider constructors
return `(nil, err)` when the API key is missing, without requiring the
caller to conditionally include them:

```go
g, _ := groq.NewModel(ctx, groq.ConfigFromEnv())
n, _ := nvidia.NewModel(ctx, nvidia.ConfigFromEnv())

// If GROQ_API_KEY is not set, g is nil and is dropped from the chain.
chain := failover.New(primary, g, n)
```

---

## Error logging

Each provider failure is logged at `WARN` level with:
- `provider`: the provider's `model.LLM.Name()` string
- `index`: position in the chain
- `remaining`: how many providers are left to try
- `error`: the underlying error

A successful recovery via fallback is logged at `INFO` level.

When all providers fail, the final error joins all individual errors with
`errors.Join` so the caller sees the complete failure chain.

---

## Name string

`Model.Name()` returns a composite identifier:

```
failover(gemini-2.0-flash → groq/llama-3.3-70b-versatile → ...)
```

This appears in the TUI footer and in logs, making it easy to see which
chain is active at a glance.

---

## collectAll helper

```go
func collectAll(ctx context.Context, llm model.LLM, req *model.LLMRequest) ([]*model.LLMResponse, error)
```

Drains the provider's iterator into a `[]*model.LLMResponse` slice. Nil
responses (emitted by some providers as streaming heartbeats) are silently
dropped. Forwarding nil responses causes nil-pointer dereferences in the ADK
runner, so the drop is intentional.

---

## Context propagation

Between provider attempts, `failover.Model.GenerateContent` checks
`ctx.Err()`. If the context is cancelled (user pressed Ctrl+C, or the TUI
is shutting down), the chain stops immediately rather than trying the next
provider.

---

## Related

- [Reference: failover package](../reference/providers.md#failover)
- [ADR-0003: Buffered failover](../adr/ADR-0003-failover-buffered.md)
- [How-to: Add a new provider](../how-to/add-provider.md)
