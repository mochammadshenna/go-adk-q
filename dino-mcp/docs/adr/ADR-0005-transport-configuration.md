# ADR-0005: Transport Configuration

**File:** `internal/server/server.go`, `cmd/dino-mcp/main.go`
**Transports:** `mcp.StdioTransport{}`, `mcp.NewStreamableHTTPHandler()`

---

## Implementation

The server supports two transports from the MCP spec. SSE transport is not implemented because it has been removed from the MCP specification (confirmed by Go SDK source comment: *"SSE transports have been removed from the spec"*).

### stdio Transport

**File:** `cmd/dino-mcp/main.go` line 57, `internal/server/server.go` line 73

Used for Claude Desktop, Cursor, Copilot — any tool that spawns a subprocess.

```go
func RunStdio(ctx context.Context, s *mcp.Server, logger *slog.Logger) error {
    logger.Info("starting dino-mcp on stdio transport")
    return s.Run(ctx, &mcp.StdioTransport{})
}
```

**Characteristics:**
- JSON-RPC over stdin/stdout
- Session is implicit (one process = one session)
- All log output goes to stderr (keeps stdout clean for JSON-RPC)
- Blocking call — runs until SIGINT/SIGTERM
- No port binding, no CORS, no HTTP

**Claude Desktop configuration:**
```json
{
  "mcpServers": {
    "dino-mcp": {
      "command": "/path/to/bin/dino-mcp",
      "args": ["stdio"]
    }
  }
}
```

### Streamable HTTP Transport

**File:** `cmd/dino-mcp/main.go` line 65, `internal/server/server.go` lines 77-130

Used for MCP Inspector, curl, browser, Cloudflare Tunnel.

```go
func RunStreamableHTTP(ctx context.Context, s *mcp.Server, addr string, logger *slog.Logger) error {
    gin.SetMode(gin.ReleaseMode)
    // ... Gin setup ...
    
    mcpHandler := mcp.NewStreamableHTTPHandler(
        func(r *http.Request) *mcp.Server { return s },
        &mcp.StreamableHTTPOptions{
            Logger:                    logger,
            DisableLocalhostProtection: true,
        },
    )
    r.Any("/mcp", gin.WrapH(mcpHandler))
    
    // ... dashboard, API, health routes ...
    httpServer := &http.Server{Addr: addr, Handler: r}
    return httpServer.ListenAndServe()
}
```

**Characteristics:**
- HTTP POST requests to `/mcp` on the configured address (default `:9010`)
- Session tracked via `Mcp-Session-Id` header (returned on `initialize`)
- Client must send `Accept: application/json, text/event-stream`
- Immediate responses use JSON; long-running can stream via SSE
- `DisableLocalhostProtection: true` enables Cloudflare Tunnel

### CLI Interface

The CLI (`cmd/dino-mcp/main.go`) selects the transport based on subcommand:

| Subcommand | Transport | Entry point |
|------------|-----------|-------------|
| `stdio` | `mcp.StdioTransport{}` | `runStdio()` — line 57 |
| `http` | `mcp.NewStreamableHTTPHandler()` | `runHTTP()` — line 65 |
| `help` | none | `printUsage()` — line 74 |

### Key Implementation Details

| Aspect | stdio | Streamable HTTP |
|--------|-------|-----------------|
| Server call | `s.Run(ctx, &mcp.StdioTransport{})` | `mcp.NewStreamableHTTPHandler(func(){return s}, opts)` |
| Session | Implicit (process) | Explicit (`Mcp-Session-Id` header) |
| Port binding | None | Configurable via `-addr` (default `:9010`) |
| Logging | stderr (any level) | stderr (DEBUG with `-verbose`) |
| CORS needed | No | Yes (Inspector browser) |
| Graceful shutdown | Context cancellation | `httpServer.Shutdown()` on ctx.Done |
