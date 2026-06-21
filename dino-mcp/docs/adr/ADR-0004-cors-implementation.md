# ADR-0004: CORS Implementation

**File:** `internal/server/server.go`
**Middleware:** `corsMiddleware()`

---

## Implementation

The Gin server includes a CORS middleware that handles cross-origin requests from the MCP Inspector browser and Cloudflare Tunnel.

### Handler

Defined in `server.go` lines 139-152:

```go
func corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, MCP-Session-Id")
        c.Header("Access-Control-Expose-Headers", "MCP-Session-Id, Content-Type")

        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusOK)
            return
        }

        c.Next()
    }
}
```

### Headers

| Header | Value | Purpose |
|--------|-------|---------|
| `Access-Control-Allow-Origin` | `*` | Allow any origin (Inspector at :5173, Tunnel URL) |
| `Access-Control-Allow-Methods` | `GET, POST, OPTIONS` | MCP uses POST, dashboard uses GET |
| `Access-Control-Allow-Headers` | `Content-Type, Accept, Origin, MCP-Session-Id` | `MCP-Session-Id` is a custom header required by Streamable HTTP |
| `Access-Control-Expose-Headers` | `MCP-Session-Id, Content-Type` | Client needs to read session ID from response |

### OPTIONS Preflight

The preflight returns HTTP 200 (not 204) because the MCP Inspector browser requires a response body for its CORS handling. The handler aborts the middleware chain immediately with `c.AbortWithStatus(http.StatusOK)`.

### Scope

The middleware wraps **all** routes on the Gin engine, registered at `server.go` line 86:

```go
r.Use(corsMiddleware())
```

This means `/mcp`, `/dashboard`, `/api/dinosaurs`, and `/health` all receive CORS headers. For `/mcp`, CORS is required by the MCP Inspector browser. For `/dashboard` and `/api`, CORS allows the standalone dashboard to be accessed from browser extensions or iframe embeds.

### Production Considerations

The wildcard origin (`*`) is appropriate for:
- **MCP Inspector** — runs at `localhost:5173` (dynamic port)
- **Cloudflare Tunnel** — `trycloudflare.com` subdomain (dynamic)

For production behind a reverse proxy, replace `*` with the specific origin:
```go
c.Header("Access-Control-Allow-Origin", "https://myapp.example.com")
```
