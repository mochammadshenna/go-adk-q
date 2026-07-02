# dev-task-pubsite — CLAUDE.md

MCP server bridging YouTrack task management and GitHub code activity.

## Project layout

```
cmd/dev-task-pubsite/   binary entry point
internal/
  cache/                TTL cache with stale-while-revalidate
  domain/               shared types (Task, PullRequest, Sprint, …)
  providers/
    yt/                 YouTrack REST API client + circuit breaker
    gh/                 GitHub client (go-github v68) + circuit breaker
  resources/            MCP App dashboard (ext-apps protocol)
  server/               MCP server wiring + Gin HTTP transport
  tools/                10 MCP tool registrations
docs/                   Diátaxis documentation suite
ui/                     Vite + TypeScript MCP App frontend
```

## Key patterns

**Provider resilience**: each provider (YouTrack, GitHub) degrades independently.
When a provider is down, cached data is returned with `providerStatus.degraded=true`
and `providerStatus.staleAge` set. Never substitute one provider for the other.

**Tool handler signature**: `func(ctx, req *mcp.CallToolRequest, input T) (*mcp.CallToolResult, OutputT, error)`
The SDK auto-serializes OutputT. Always include `providerStatus` in OutputT.

**Resource registration**: `s.AddResource(...)` is a method on `*mcp.Server`.
`mcp.AddTool` is a generic package-level function.

**Circuit breaker**: 5 failures → 120s cooldown, tracked per provider.
Never share a circuit breaker across providers.

## Running

```bash
# stdio (Claude Desktop)
make dev-stdio

# HTTP (MCP Inspector / Postman)
make dev-http
# Opens: http://localhost:9012/mcp   (MCP Streamable HTTP)
#        http://localhost:9012/dashboard (standalone UI)
#        http://localhost:9012/health

# Full build (Go + Vite UI)
make build
```

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `YOUTRACK_URL` | For YT tools | e.g. `https://youtrack.example.com` |
| `YOUTRACK_TOKEN` | For YT tools | Permanent token or bearer token |
| `GITHUB_TOKEN` | For GH tools | Personal access token or app installation token |
| `GITHUB_OWNER` | Optional | Default owner used when tools omit `owner` arg |
| `PORT` | Optional | HTTP listen port (default: 9012) |
| `DEBUG` | Optional | `1` or `true` to enable Gin access logging |

Missing env vars → provider starts in nil state → those tools simply absent from
the MCP server. The server still starts and any available provider still works.

## Adding a tool

1. Add method to provider client (`yt/client.go` or `gh/client.go`)
2. Add domain type to `internal/domain/task.go` if needed
3. Add handler file in `internal/tools/`
4. Call `RegisterXxx(s, client)` from `internal/server/server.go`

## go.mod module path

`github.com/msw/dev-task-pubsite`

## Port convention

`:9012` — chosen to avoid conflict with archipelago-hotels-mcp (`:9011`).
