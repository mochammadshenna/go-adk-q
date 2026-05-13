# ADR-0003: Buffered (non-streaming) failover

**Status:** Accepted  
**Date:** 2025  
**Deciders:** Project leads

---

## Context

The failover model must retry a failed provider transparently. The ADK model
interface supports both streaming (incremental token delivery) and non-streaming
(complete response) modes via the `stream bool` parameter to `GenerateContent`.

## Decision

`failover.Model.GenerateContent` always calls providers with `stream=false`
internally, regardless of the `stream` parameter passed by the caller. The
full response is buffered before being forwarded.

## Rationale

Once the first token is yielded to the caller, the caller has committed to
that provider. If the provider errors mid-stream, the caller has already
received partial output and the request cannot be retried cleanly. By
buffering the full response, failover can:

1. Detect errors before committing to the caller.
2. Try the next provider with the original request.
3. Forward a complete, clean response.

## Trade-offs

| Benefit | Cost |
|---|---|
| Transparent retry on any error | First token delayed until full response is ready |
| Caller sees complete responses only | Higher memory usage for large responses |
| Simple, correct semantics | Streaming latency benefit is lost |

## Future consideration

If streaming latency becomes important, the failover model could implement a
"probe" mode: send a small preflight request to verify the provider is
available before committing to a streaming response. This was not implemented
because:
- Most failures are transient (rate limits, brief outages) and resolve quickly.
- The extra round-trip would add latency in the common (no-failure) case.
- The current design is significantly simpler to reason about.

## Consequences

- `failover.Model` is reliable but introduces latency proportional to response length.
- The `stream` parameter to `GenerateContent` is accepted but ignored internally.
- `collectAll` drops nil `*model.LLMResponse` values (provider heartbeats) to
  prevent nil-pointer dereferences in the ADK runner.
