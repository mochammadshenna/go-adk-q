# ADR-0005: Vite Single-File TypeScript UI as MCP App Resource

**File(s):** `ui/src/mcp-app.ts`, `internal/resources/dashboard.go`, `internal/resources/mcp-app.html`
**Decision date:** 2026-06-22

---

## Decision

The interactive hotel dashboard is delivered as an MCP App resource (`text/html;profile=mcp-app`) embedded directly in the Go binary via `//go:embed`. The UI is written in TypeScript, bundled by Vite into a single self-contained HTML file, and served over the same MCP session that delivers tool results. No separate web server, CDN, or authentication layer is required.

### Implementation

```go
// resources/dashboard.go — resource registration
//go:embed mcp-app.html
var dashboardHTML string

s.AddResource(&mcp.Resource{
    URI:      "ui://hotel-dashboard",
    Name:     "Archipelago Hotels Dashboard",
    MIMEType: "text/html;profile=mcp-app",   // signals MCP App to the host
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceDomains": []string{"images.archipelagohotels.com"},
        },
    },
}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
    return &mcp.ReadResourceResult{
        Contents: []*mcp.ResourceContents{{
            URI:      "ui://hotel-dashboard",
            MIMEType: "text/html;profile=mcp-app",
            Text:     dashboardHTML,
        }},
    }, nil
})
```

```go
// Tool _meta.ui links tool to UI resource — triggers automatic open on tool call
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     "ui://hotel-dashboard",
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

```typescript
// ui/src/mcp-app.ts — UI calls back to the server via MCP tools/call
// get_hotel_detail is app-only (not shown to Claude) — only the TypeScript calls it
async function loadHotelDetail(hotelId: string) {
    const result = await app.callTool("get_hotel_detail", { id: hotelId });
    renderDetailOverlay(result.structuredContent as HotelDetail);
}
```

```makefile
# Makefile build pipeline
build-ui:
    cd ui && npm install --silent && npm run build
    cp ui/dist/index.html internal/resources/mcp-app.html

build-go:
    go build -o bin/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| MIME type signal | `text/html;profile=mcp-app` — MCP Apps protocol marker | `resources/dashboard.go` |
| Resource URI | `ui://hotel-dashboard` — custom scheme, not HTTP | `resources/dashboard.go` |
| Binary embedding | `//go:embed mcp-app.html` compiled at `go build` time | `resources/dashboard.go:13` |
| Single-file constraint | Vite `vite-plugin-singlefile` inlines all JS/CSS — required by MCP Apps protocol | `ui/vite.config.ts` |
| Built size | ~383 KB HTML (minified, inlined) | `ui/dist/index.html` |
| UI → server calls | TypeScript calls `get_hotel_detail` via MCP `tools/call` over active session | `ui/src/mcp-app.ts` |
| App-only tool | `get_hotel_detail` not shown to Claude; invoked only by the TypeScript | `internal/tools/detail.go` |
| Rebuild required | After any `mcp-app.ts` change, run `make build` to re-embed and recompile | `Makefile` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Separate React web app at a different URL | Requires separate server, CORS, auth; breaks the single-binary deployment model |
| Serve HTML from filesystem at runtime | Deployment requires distributing both the binary and the HTML file; breaks self-contained binary guarantee |
| Plain MCP text responses | Cannot render hotel cards, thumbnails, or interactive filters in a visual grid |
| Server-side rendering via HTTP `/dashboard` | Works in HTTP mode but unavailable in stdio mode (no browser access from Claude Desktop's iframe model) |
