# ADR-0002: Use Genkit compat_oai only for model bridging

**Status:** Accepted  
**Date:** 2025  
**Deciders:** Project leads

---

## Context

Most LLM providers expose an OpenAI-compatible `/v1/chat/completions` endpoint.
We need a Go HTTP client that handles authentication, retries, and request/response
serialisation for this API.

Genkit (`github.com/firebase/genkit/go`) provides exactly this via its
`compat_oai` package, which wraps the `github.com/openai/openai-go` client.

## Decision

Use Genkit v1.7.0 **only** as an OpenAI-compatible HTTP client, accessed
through the `model/oaibridge` package. Genkit is not used for:
- Flow management
- Prompt templates
- Telemetry/tracing
- Agent orchestration
- Any user-facing feature

## Boundaries

```
go-adk-q/model/oaibridge   ← only file that imports firebase/genkit
model/{groq,nvidia,...}    ← import oaibridge, not genkit directly
all other packages         ← must not import firebase/genkit
```

## Alternatives considered

| Alternative | Rejected because |
|---|---|
| `github.com/openai/openai-go` directly | `compat_oai` already wraps this cleanly; avoids duplicating auth + retry logic. |
| Per-provider raw HTTP | 6+ providers × boilerplate = too much maintenance. |
| Full Genkit pipeline | Entangles agent logic with Genkit's flow system; conflicts with ADK's agent model. |

## Consequences

- Adding a new OpenAI-compatible provider requires only a thin package using `oaibridge`.
- Providers with non-compatible APIs must implement `model.LLM` directly.
- Genkit version bumps only affect `model/oaibridge` — isolated blast radius.
