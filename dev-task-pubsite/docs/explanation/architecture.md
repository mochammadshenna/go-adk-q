# Architecture

dev-task-pubsite is a Go MCP server that bridges two external systems — YouTrack and GitHub — through a layered architecture. This document explains why the project is structured the way it is.

## Layer diagram

```
┌─────────────────────────────────────────────────────┐
│  cmd/dev-task-pubsite   (binary entry point)         │
│  · reads env vars                                    │
│  · constructs provider clients (nil if unconfigured)  │
│  · dispatches stdio or HTTP                          │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│  internal/server                                     │
│  · mcp.NewServer with instructions                   │
│  · calls Register* for each available tool           │
│  · registers ui://task-dashboard resource            │
│  · Gin HTTP wrapper with CORS (HTTP mode only)       │
└──────┬──────────────────────────┬────────────────────┘
       │                          │
┌──────▼──────┐          ┌────────▼────────┐
│ internal/   │          │ internal/       │
│ tools       │          │ resources       │
│ · 10 MCP    │          │ · MCP App HTML  │
│   handlers  │          │   (go:embed)    │
│ · 3-value   │          │ · ext-apps      │
│   return    │          │   protocol      │
└──────┬──────┘          └─────────────────┘
       │
┌──────▼──────────────────────────────────────────────┐
│  internal/providers                                  │
│  ┌─────────────┐         ┌─────────────────────┐    │
│  │  yt/        │         │  gh/                │    │
│  │  YouTrack   │         │  GitHub             │    │
│  │  REST API   │         │  go-github v68      │    │
│  │  client     │         │  client             │    │
│  └──────┬──────┘         └──────┬──────────────┘    │
│         │    circuit breaker    │                    │
│         └──────────┬────────────┘                    │
└────────────────────┼────────────────────────────────┘
                     │
         ┌───────────▼────────────┐
         │  internal/cache        │
         │  · TTL cache           │
         │  · stale-while-revalid │
         └────────────────────────┘
                     │
         ┌───────────▼────────────┐
         │  internal/domain       │
         │  · Task, PullRequest   │
         │  · Sprint, Issue       │
         │  · RepoStats           │
         │  · CommitDiff          │
         │  · ProviderStatus      │
         └────────────────────────┘
```

## Why these layers exist

### `internal/domain` — shared language

Domain types are the contract between providers and tools. They ensure that `yt/client.go` and `gh/client.go` never know about each other — both translate their raw API responses into the same domain types, which tools consume uniformly.

Adding a new provider (e.g. Linear, Jira) means implementing the domain types for it without touching tools.

### `internal/cache` — stale-while-revalidate

The cache layer sits inside each provider. When a live request fails, the cache returns whatever it last stored along with `GetResult.Stale = true`. This allows the tool layer to signal `providerStatus.degraded = true` without panicking or returning an error.

The `5-minute TTL` reflects the typical pace of change in task/PR data. Lowering it increases API calls; raising it increases staleness risk.

### `internal/providers/yt` and `internal/providers/gh` — isolated resilience

Each provider owns its own circuit breaker and cache. They never share state. This is the key resilience design decision: YouTrack being down does not affect GitHub responses, and vice versa.

The providers expose only high-level methods returning domain types — they never leak HTTP responses or library types (like `github.PullRequest`) to the layers above.

### `internal/tools` — thin handlers

Tool handlers are deliberately thin. They call one provider method, check the result, and return a typed output struct. No business logic lives here. This makes each handler easy to read in isolation and easy to test.

The 3-value return `(*mcp.CallToolResult, OutputT, error)` is the MCP Go SDK's pattern. The SDK serialises `OutputT` to JSON automatically — there's no manual marshalling in any handler.

### `internal/server` — wiring, not logic

The server package assembles the MCP server from its parts. It calls every `Register*` function and sets up the Gin HTTP wrapper for HTTP mode. No logic lives here — it's a pure composition root.

The nil-check pattern (`if ytClient != nil`) means the server adapts to whatever providers are available at runtime.

### `cmd/dev-task-pubsite` — thin entry point

The binary entry point reads environment, constructs clients, logs warnings, and hands off to the server. It does no I/O beyond reading env vars and logging. This makes it straightforward to test the server logic independently.

## Request flow

### stdio mode (Claude Desktop)

```
Claude Desktop
  → JSON-RPC over stdin/stdout
  → mcp.Server.Run (mcp.StdioTransport)
  → tool handler
  → provider client (cache hit or live API call)
  → domain type → OutputT → JSON back to Claude
```

### HTTP mode (MCP Inspector / Postman)

```
HTTP POST /mcp
  → Gin router
  → mcp.NewStreamableHTTPHandler
  → mcp.Server
  → tool handler
  → provider client
  → JSON response
```

## What is not in this architecture

- **No database**: all persistence is the in-memory TTL cache. State does not survive restarts.
- **No authentication on the HTTP endpoint**: the server trusts all callers. Add a reverse proxy (nginx, Caddy) with auth if exposing to the network.
- **No background refresh**: cache entries are populated on demand. A cold start means the first caller waits for live API responses.
- **No cross-provider joins**: tools do not call both providers in a single operation. Cross-provider linking (e.g. commit message → YouTrack task) is done by the AI agent across multiple tool calls.
