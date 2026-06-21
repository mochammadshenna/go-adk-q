# ADR-0006: Standalone Dashboard Fallback

**File:** `internal/server/server.go`, `internal/resources/dashboard_ui.html`
**Endpoints:** `GET /dashboard`, `GET /api/dinosaurs`

---

## Implementation

The dashboard HTML is available both as an MCP App resource (served via `resources/read` to Claude Desktop iframe) and as a standalone web page (served via `GET /dashboard` directly in a browser). The same HTML file handles both modes.

### Dual Route Registration

In `server.go` lines 97-116, two routes serve the same embedded HTML:

```go
// MCP App resource — served via resources/read
// Registered in dashboard.go: s.AddResource(resource, handler)

// Standalone UI — served via HTTP GET
r.GET("/dashboard", func(c *gin.Context) {
    htmlData, err := resources.DashboardHTML()
    if err != nil {
        c.String(http.StatusInternalServerError, "dashboard not found")
        return
    }
    c.Data(http.StatusOK, "text/html; charset=utf-8", htmlData)
})
r.GET("/dashboard/*any", func(c *gin.Context) {
    // Same handler — supports /dashboard/anything
})
```

### API Fallback

A REST API endpoint (`/api/dinosaurs`) provides data for the standalone mode:

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
```

The API supports the same `?filter=Carnivore` parameter as the MCP tool's `arguments.filter`.

### Dual Code Path in HTML

The HTML (`ui/src/mcp-app.ts`) detects which mode it's running in:

**MCP Apps mode** (inside Claude Desktop iframe):
```typescript
const app = new App({ name: "Dino Dashboard", version: "0.1.0" });
app.ontoolinput = (params) => {
    if (params.structuredContent?.dinosaurs) {
        showDashboard(params.structuredContent);  // data from host
    }
};
await app.connect(); // auto-detects PostMessageTransport inside iframe
```

**Standalone mode** (in browser):
```typescript
if (window === window.parent) {
    // Not in an iframe — fetch data via REST API
    fetch("/api/dinosaurs")
        .then(r => r.json())
        .then(data => showDashboard(data));
}
```

### Why Both Modes

| Mode | Client | How data arrives |
|------|--------|-----------------|
| MCP App | Claude Desktop iframe | postMessage `tool-input` / `tool-result` events |
| Standalone | Browser tab | `fetch("/api/dinosaurs")` REST API |

The standalone mode ensures the dashboard is accessible without Claude Desktop — for quick previews, demos, and testing.

### Key Implementation Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Standalone route | `r.GET("/dashboard", ...)` serves embedded HTML | `server.go:97` |
| API endpoint | `r.GET("/api/dinosaurs", ...)` returns filtered JSON | `server.go:112` |
| MCP App resource | `s.AddResource({MIMEType: "text/html;profile=mcp-app"}, handler)` | `dashboard.go:35` |
| Mode detection | `window === window.parent` check in JavaScript | `mcp-app.ts:262` |
| Shared data | Same `dinoData` slice + `tools.FilteredDinosaurs()` filtering | `dashboard.go:88` |
