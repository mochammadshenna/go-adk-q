# ADR-0001: Use Google ADK Go as the agent layer

**Status:** Accepted  
**Date:** 2025  
**Deciders:** Project leads

---

## Context

We need an agent framework that supports:
- Typed tool definitions with JSON schema generation.
- Session and memory management.
- Composable agent patterns (sequential, parallel, loop, multi-agent).
- Artifact storage.
- Skill toolsets.
- A stable Go API.

## Decision

Use `google.golang.org/adk` v1.2.0 as the sole agent orchestration layer.

## Alternatives considered

| Alternative | Rejected because |
|---|---|
| Genkit flows | Genkit's flow system is optimised for HTTP-triggered pipelines, not interactive agents. No built-in session or memory services. |
| LangChain Go | Less idiomatic Go; fewer ADK-native patterns (sequential/parallel agents, skill toolsets). |
| Raw HTTP + custom orchestration | Too much boilerplate; session management, tool injection, and multi-agent delegation would all need to be written from scratch. |

## Consequences

- All agent, tool, session, memory, and artifact code uses ADK types and interfaces.
- Genkit is limited to the `compat_oai` model bridge (see ADR-0002).
- Any ADK API changes in v1.3+ may require migration effort.
- The ADK's `iter.Seq2` streaming model is used throughout the codebase.
