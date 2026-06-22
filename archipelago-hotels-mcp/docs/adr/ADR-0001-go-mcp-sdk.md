# ADR-0001: Go + modelcontextprotocol/go-sdk v1.6.1

**File(s):** `cmd/archipelago-hotels-mcp/main.go`, `internal/server/server.go`
**Decision date:** 2026-06-22

---

## Decision

The server is implemented in Go using the official `modelcontextprotocol/go-sdk` v1.6.1. This gives us type-safe tool handlers, a single self-contained binary with no runtime dependency, and full compatibility with the parent `go-adk-q` monorepo toolchain.

### Implementation

```go
// internal/server/server.go
s := mcp.NewServer(
    &mcp.Implementation{Name: "archipelago-hotels-mcp", Version: Version},
    &mcp.ServerOptions{
        Instructions: `You have access to the full Archipelago Hotels & Resorts catalog...`,
        Logger:       logger,
    },
)

// Type-safe tool registration — TArgs is deserialized automatically from JSON input
mcp.AddTool(s, &mcp.Tool{
    Name:        "search_hotels",
    Description: "...",
    InputSchema: map[string]any{...},
    Meta: mcp.Meta{"ui": map[string]any{"resourceUri": resources.ResourceURI}},
}, searchHandler(pool, rateSvc))

// stdio transport — used by Claude Desktop (binary launched as child process)
s.Run(ctx, &mcp.StdioTransport{})

// Streamable HTTP transport — used by browser agents and CI
handler := mcp.NewStreamableHTTPHandler(func(...) *mcp.Server { return s }, options)
router.POST("/mcp", gin.WrapH(handler))
router.GET("/mcp", gin.WrapH(handler))   // SSE for server-initiated messages
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Server creation | `mcp.NewServer()` with `Implementation` + `ServerOptions` | `server.go:35` |
| Tool registration | `mcp.AddTool[TArgs, TResult]()` — generic, type-safe | `search.go:18`, `recommend.go:18` |
| Resource registration | `s.AddResource()` with embedded HTML handler | `resources/dashboard.go:21` |
| stdio transport | `s.Run(ctx, &mcp.StdioTransport{})` — blocks, JSON-RPC on stdin/stdout | `server.go:73` |
| HTTP transport | `mcp.NewStreamableHTTPHandler()` wrapped by `gin.WrapH()` | `server.go:87` |
| structured content | Second return value of handler serialised as `result.structuredContent` | All tool handlers |
| Binary size | ~23 MB self-contained (embeds 383 KB UI HTML, all Go deps) | `bin/archipelago-hotels-mcp` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| TypeScript SDK (`@modelcontextprotocol/sdk`) | Requires Node.js runtime on deployment target; heavier binary; mismatches parent monorepo's Go toolchain |
| Python SDK | Dynamic typing makes large-scale refactors risky; slower startup; harder to ship as a single binary |
| Raw JSON-RPC (no SDK) | Re-implementing the protocol is error-prone and bypasses future SDK improvements |
| gRPC transport only | MCP protocol is JSON-RPC 2.0; going off-spec breaks Claude Desktop compatibility |
