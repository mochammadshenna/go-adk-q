# ADR-0001: Go Server Implementation

**File:** `cmd/dino-mcp/main.go`, `internal/server/server.go`
**SDK:** `github.com/modelcontextprotocol/go-sdk` v1.6.1

---

## Implementation

The server is written in Go using the MCP Go SDK. This is the same language as the parent `go-adk-q` project, simplifying dependency management and build tooling.

### Server Creation

`internal/server/server.go` line 30 — `New()` creates an `mcp.Server` with:

```go
s := mcp.NewServer(
    &mcp.Implementation{Name: "dino-mcp", Version: Version},
    &mcp.ServerOptions{
        Instructions: `...`,  // LLM instructions for tool usage
        Logger: logger,
    },
)
```

Three tools and one resource are registered on the same server instance via:
- `tools.RegisterThink(s)` — zero-arg fact tool
- `tools.RegisterAsk(s)` — single-arg Q&A tool
- `tools.RegisterDashboardTool(s) + resources.RegisterDashboardResource(s, logger)` — MCP App tool + HTML resource

### Transport Support

The same `mcp.Server` instance works with both transports:

- **stdio**: `s.Run(ctx, &mcp.StdioTransport{})` — blocking call, JSON-RPC over stdin/stdout
- **Streamable HTTP**: `mcp.NewStreamableHTTPHandler(func(...) *mcp.Server { return s }, options)` — wrapped by Gin

### Asset Embedding

`//go:embed` compiles the Vite-built HTML dashboard into the binary at build time:

```go
//go:embed dashboard_ui.html
var uiFS embed.FS
```

No runtime filesystem access needed — the binary is self-contained (~11MB).

### Key Implementation Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Server creation | `mcp.NewServer()` with `Implementation` + `ServerOptions` | `server.go:30` |
| Tool registration | `mcp.AddTool[T,A,R]()` generic function | `think_tool.go:28` |
| Resource registration | `s.AddResource()` with `//go:embed` handler | `dashboard.go:35` |
| HTML embedding | `//go:embed dashboard_ui.html` | `dashboard.go:14` |
| stdio transport | `s.Run(ctx, &mcp.StdioTransport{})` | `server.go:73` |
| HTTP transport | `mcp.NewStreamableHTTPHandler()` + `gin.WrapH()` | `server.go:87` |
