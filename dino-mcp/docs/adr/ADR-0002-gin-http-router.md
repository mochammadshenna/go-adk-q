# ADR-0002: Gin HTTP Router

**File:** `internal/server/server.go`
**Package:** `github.com/gin-gonic/gin` v1.12.0

---

## Implementation

The HTTP layer uses `github.com/gin-gonic/gin` v1.12.0 as the router. Gin handles route matching, middleware chaining, JSON responses, and CORS — all required by the MCP Streamable HTTP transport.

### Route Table

Defined in `RunStreamableHTTP()` (`server.go` lines 77-130):

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| ANY | `/mcp` | `gin.WrapH(mcpHandler)` | MCP protocol — all JSON-RPC methods |
| ANY | `/mcp/*any` | `gin.WrapH(mcpHandler)` | MCP subpaths (trailing slash) |
| GET | `/dashboard` | Serve embedded HTML | Standalone dashboard UI |
| GET | `/api/dinosaurs` | Filtered JSON | Data source for standalone UI |
| GET | `/health` | JSON status | Health check |

### Middleware Chain

```go
r := gin.New()
r.Use(gin.Recovery())     // catch panics → 500
r.Use(ginLogger(logger))   // slog-based request logging (DEBUG level)
r.Use(corsMiddleware())    // CORS headers → OPTIONS → 200
```

**Recovery** (`gin.Recovery()`): catches any panic in handlers, writes a 500 response, and logs the stack trace. Required because a panic in an MCP handler would otherwise crash the process.

**Logger** (custom `ginLogger`): adapts `slog` to Gin's request format. Only logs at DEBUG level to avoid noise. Records method, path, status code, and latency.

**CORS** (custom `corsMiddleware`): sets headers on every response and returns 200 for OPTIONS preflight.

### MCP SDK Bridge — `gin.WrapH()`

The MCP Go SDK's `StreamableHTTPHandler` implements the standard `http.Handler` interface. Gin uses `gin.HandlerFunc`. The bridge:

```go
r.Any("/mcp", gin.WrapH(mcpHandler))
```

`gin.WrapH()` converts any `http.Handler` to a `gin.HandlerFunc` by:
1. Reading the request from `c.Request`
2. Writing the response through `c.Writer`
3. Letting the SDK handle all JSON-RPC routing internally

### JSON Responses

Gin's `c.JSON()` is used for the REST API endpoints:

```go
r.GET("/api/dinosaurs", func(c *gin.Context) {
    filter := c.Query("filter")
    dinos := tools.FilteredDinosaurs(filter)
    c.JSON(http.StatusOK, gin.H{
        "dinosaurs": dinos,
        "filter":    filter,
        "timestamp": time.Now().UTC().Format(time.RFC3339),
        "total":     len(dinos),
    })
})

r.GET("/health", func(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"status": "ok", "server": Name, "version": Version})
})
```

### CORS Implementation

```go
func corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, MCP-Session-Id")
        c.Header("Access-Control-Expose-Headers", "MCP-Session-Id, Content-Type")
        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusOK)  // MCP Inspector requires 200
            return
        }
        c.Next()
    }
}
```

Key detail: OPTIONS returns **200**, not 204. The MCP Inspector browser's CORS preflight handling requires a 200 response body.

### Mode Control

```go
gin.SetMode(gin.ReleaseMode)
if logger.Enabled(ctx, slog.LevelDebug) {
    gin.SetMode(gin.DebugMode)  // prints route table to stderr
}
```

Release mode suppresses Gin's debug output. Debug mode shows all registered routes on startup — useful during development but too verbose for production.

### Key Implementation Details

| Aspect | Implementation | Line |
|--------|---------------|------|
| Engine creation | `gin.New()` (not `gin.Default()` — we use custom middleware) | `server.go:82` |
| MCP handler bridge | `gin.WrapH(mcpHandler)` | `server.go:93` |
| HTML serving | `c.Data(http.StatusOK, "text/html; charset=utf-8", htmlData)` | `server.go:101` |
| JSON response | `c.JSON(http.StatusOK, gin.H{...})` | `server.go:118` |
| Option parsing | `c.Query("filter")` | `server.go:113` |
| Static files | None needed — everything is `//go:embed` | `dashboard.go:14` |
