# dino-mcp Documentation Index

> **Every document maps to actual source code.** Open the referenced files alongside.

---

## Documentation Index

| Document | Source files referenced | What it covers |
|---|---|---|
| [`tutorials/get-started.md`](tutorials/get-started.md) | `cmd/dino-mcp/main.go`, `internal/server/server.go`, `Makefile`, `test_mcp.sh` | Build, run, test, connect to Claude Desktop |
| [`tutorials/first-tool.md`](tutorials/first-tool.md) | `internal/tools/think.go + ask.go` (`tools.RegisterThink`, `tools.RegisterAsk`) | `mcp.AddTool[T,A,R]()` generic pattern, handler signature, registration |
| [`how-to/add-dinosaur.md`](how-to/add-dinosaur.md) | `internal/tools/dashboard.go` (`dinoData`, `DinoSummary`, `FilteredDinosaurs`) | Data model extension, filtering logic, UI update |
| [`how-to/test-inspector.md`](how-to/test-inspector.md) | `test_mcp.sh`, `Makefile` | MCP Inspector setup, protocol debugging, troubleshooting |
| [`reference/cli.md`](reference/cli.md) | `cmd/dino-mcp/main.go` (`runStdio`, `runHTTP`, `printUsage`) | CLI subcommands, flags, routes, environment, exit codes |
| [`explanation/architecture.md`](explanation/architecture.md) | All source files | MCP protocol lifecycle, MCP Apps postMessage protocol, Go SDK internals, Gin bridge, end-to-end flow |
| [`adr/ADR-0001-go-server-implementation.md`](adr/ADR-0001-go-server-implementation.md) | `internal/server/server.go` | Go server creation, SDK usage, transport support, asset embedding |
| [`adr/ADR-0002-gin-http-router.md`](adr/ADR-0002-gin-http-router.md) | `internal/server/server.go` | Gin engine setup, `gin.WrapH()` MCP bridge, route table, middleware chain, CORS, mode control |
| [`adr/ADR-0003-mcp-apps-view-protocol.md`](adr/ADR-0003-mcp-apps-view-protocol.md) | `ui/src/mcp-app.ts`, `internal/tools/dashboard.go` | `App` class, `PostMessageTransport`, postMessage messages, MIME type, `_meta.ui.resourceUri` |
| [`adr/ADR-0004-cors-implementation.md`](adr/ADR-0004-cors-implementation.md) | `internal/server/server.go` | CORS handler, headers, OPTIONS preflight, production considerations |
| [`adr/ADR-0005-transport-configuration.md`](adr/ADR-0005-transport-configuration.md) | `cmd/dino-mcp/main.go`, `internal/server/server.go` | stdio transport, Streamable HTTP transport, CLI subcommand mapping |
| [`adr/ADR-0006-standalone-dashboard-fallback.md`](adr/ADR-0006-standalone-dashboard-fallback.md) | `internal/server/server.go`, `ui/src/mcp-app.ts` | Dual routes, API fallback, mode detection (`window === window.parent`) |

---

## Pi Agent Integration

Add to `~/.pi/agent/mcp.json`:

```json
{
  "mcpServers": {
    "dino-mcp": {
      "command": "/path/to/dino-mcp/bin/dino-mcp",
      "args": ["stdio"],
      "lifecycle": "lazy"
    }
  }
}
```

Then search/call tools via `mcp({tool: "dino_mcp_dino_think", args: "{}"})`.

---

## Quick Reference

```bash
# Build (Go only, ≈2s)
make build-fast

# Build (Full UI + Go, ≈10s)
make build

# Run
make dev-http              # HTTP mode with verbose logging on :9010
make run-stdio             # stdio mode for Claude Desktop
make run-tunnel            # HTTP + Cloudflare Tunnel

# Test
make test                  # 7 integration tests
make test-inspector        # Launch MCP Inspector at :5173

# Lint
make lint                  # go vet + go fmt
```

---

## Source File Responsibilities

| File | What it does |
|---|---|
| `cmd/dino-mcp/main.go` | CLI entry point. Parses subcommand (`stdio`, `http`, `help`), flags (`-addr`, `-verbose`, `-version`), calls `RunStdio()` or `RunStreamableHTTP()` |
| `internal/server/server.go` | `New()` creates `mcp.Server` with implemention info + instructions. Registers 3 tools + 1 resource. `RunStdio()`, `RunStreamableHTTP()` start transports. `corsMiddleware()` handles CORS. Types: `DinoThinkResult`, `DinoAskArgs`, `DinoAskResult`, `DinoDashboardArgs`, `DinoDashboardResult`, `DinoSummary` |
| `internal/tools/think.go + ask.go` | `tools.RegisterThink()` — zero-arg tool, `dino_think`. `tools.RegisterAsk()` — single-arg tool `dino_ask`. Both use `mcp.AddTool` generic pattern. `answerDinoQuestion()` provides curated Q&A |
| `internal/tools/dashboard.go` | `tools.RegisterDashboardTool() + resources.RegisterDashboardResource()` — MCP App tool `dino_dashboard` with `_meta.ui.resourceUri`. Resource handler serves `//go:embed dashboard_ui.html`. `dinoData` — 12 `DinoSummary` entries. `tools.FilteredDinosaurs()` — filtering by diet/period/name |
| `internal/resources/dashboard_ui.html` | Vite-built single-file HTML (354KB). Contains bundled TypeScript, CSS, and `@modelcontextprotocol/ext-apps` SDK |
| `ui/src/mcp-app.ts` | TypeScript source. `App` class with standard handlers (`ontoolinput`, `ontoolresult`, `ontoolcancelled`, `onhostcontextchanged`, `onerror`, `onteardown`). `app.connect()` auto-detects `PostMessageTransport` in iframe. `renderDinos()` card grid rendering |

---

## External Protocol References

| Resource | URL | What it defines |
|---|---|---|
| MCP Specification | https://spec.modelcontextprotocol.io | JSON-RPC message format, lifecycle (initialize → initialized → running), tool/resource registration, transports (stdio, Streamable HTTP) |
| MCP Apps Overview | https://modelcontextprotocol.io/extensions/apps/overview | `ui/initialize` handshake, postMessage JSON-RPC, `_meta.ui.resourceUri` metadata, MIME type `text/html;profile=mcp-app` |
| MCP UI Gallery | https://mcpui.dev | Community MCP Apps examples, patterns, and reference implementations |
| MCP Go SDK | https://github.com/modelcontextprotocol/go-sdk | `mcp.NewServer()`, `mcp.AddTool[T,A,R]()`, `mcp.NewStreamableHTTPHandler()`, `mcp.StdioTransport{}` |
| ext-apps SDK | https://github.com/modelcontextprotocol/ext-apps | `App` class, `PostMessageTransport`, `applyDocumentTheme()`, `applyHostStyleVariables()` |
| Gin Web Framework | https://gin-gonic.com | `gin.New()`, `gin.WrapH()`, `c.AbortWithStatus()`, `c.Header()`, `c.JSON()`, `r.GET()`, `r.Any()` |
