# ADR-0004: Gin v1.11 as HTTP Transport Wrapper

**File(s):** `internal/server/server.go`
**Decision date:** 2026-06-22

---

## Decision

The Streamable HTTP transport for MCP is wrapped by Gin v1.11 rather than the standard `net/http` `ServeMux`. Gin provides concise middleware wiring (CORS, panic recovery, request logging) and hosts the REST convenience endpoints (`/api/hotels`, `/api/brands`, `/api/regions`, `/health`, `/dashboard`) on the same port as the MCP handler.

### Implementation

```go
// server.go — Gin router setup
router := gin.New()
router.Use(gin.Recovery())
router.Use(corsMiddleware())
if verbose {
    router.Use(gin.Logger())
}

// MCP Streamable HTTP — POST for client requests, GET for SSE
mcpHandler := mcp.NewStreamableHTTPHandler(func(...) *mcp.Server { return s }, &mcp.StreamableHTTPHandlerOptions{
    SessionIDGenerator: func() string { return uuid.NewString() },
})
router.POST("/mcp", gin.WrapH(mcpHandler))
router.GET("/mcp", gin.WrapH(mcpHandler))

// REST convenience endpoints
router.GET("/dashboard", serveDashboardHTML)
router.GET("/api/hotels", handleHotels(svc))
router.GET("/api/brands", handleBrands(svc))
router.GET("/api/regions", handleRegions(svc))
router.GET("/health", handleHealth(svc))
```

```go
// CORS — required for browser-based MCP hosts and standalone dashboard
func corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Authorization")
        c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS, DELETE")
        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }
        c.Next()
    }
}
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| MCP POST handler | `mcp.NewStreamableHTTPHandler()` wrapped by `gin.WrapH()` | `server.go` |
| SSE GET handler | Same handler; go-sdk distinguishes by method internally | `server.go` |
| Session affinity | `Mcp-Session-Id` header; forwarded through Gin without modification | CORS headers |
| Default listen addr | `:9011` — configurable via `-addr` flag | `main.go` |
| Panic recovery | `gin.Recovery()` — prevents one bad request from crashing the process | `server.go` |
| CORS origin | `*` — intentional; no sensitive data served; allows any AI host to connect | `server.go` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| `net/http` `ServeMux` | CORS and recovery middleware require more boilerplate; no request logging without manual wrapping |
| Echo | Functionally equivalent to Gin; no reason to deviate from the go-adk-q ecosystem convention |
| Chi | Lighter than Gin but requires separate CORS library; adds a dependency for marginal gain |
| Separate HTTP server for REST | Requires two listen ports; complicates firewall rules and Claude Desktop config |
