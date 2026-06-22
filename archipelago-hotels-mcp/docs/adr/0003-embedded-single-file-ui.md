# ADR-0003: Embedded Single-File Vite UI via MCP App Resource

**File(s):** `ui/src/mcp-app.ts`, `internal/resources/dashboard.go`, `internal/resources/mcp-app.html`
**Status:** Accepted
**Decision date:** 2025-06

---

## Context

Claude Desktop renders MCP App resources in sandboxed iframes enforcing a strict Content Security Policy. The hotel dashboard requires hotel cards, thumbnails, brand themes, and price display — none of which can be implemented with plain MCP text responses. Three options were evaluated:

| Option | Description |
|--------|-------------|
| (a) Separate web app | Independent deployment at a distinct URL |
| (b) Inline HTML with CDN links | Single file but loads scripts/styles from external CDN |
| (c) Vite single-file build embedded in Go binary | All assets inlined, served as MCP resource |

Option (b) fails under Claude Desktop's CSP because `<script src="https://cdn.jsdelivr.net/...">` is blocked. Option (a) breaks the single-binary deployment model and requires separate auth and CORS handling.

## Decision

Use Vite with `vite-plugin-singlefile` to produce a single self-contained HTML file. That file is committed to `internal/resources/mcp-app.html`, embedded into the Go binary via `//go:embed`, and served as an MCP Resource with MIME type `text/html;profile=mcp-app`.

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
| Built size | ~383 KB HTML (minified, inlined); ~94 KB gzipped | `ui/dist/index.html` |
| UI → server calls | TypeScript calls `get_hotel_detail` via MCP `tools/call` over active session | `ui/src/mcp-app.ts` |
| App-only tool | `get_hotel_detail` not shown to Claude; invoked only by the TypeScript | `internal/tools/detail.go` |
| Rebuild required | After any `mcp-app.ts` change, run `make build` to re-embed and recompile | `Makefile` |

## Consequences

- No CDN dependencies: the UI works under Claude Desktop's restrictive CSP because every byte is inlined.
- Single-binary deployment: `go:embed` means the binary ships the UI; no separate file distribution is required.
- Full rebuild on UI changes: modifying TypeScript requires `make build-ui && make build-go` before the new UI is active.
- Bundle size is ~383 KB HTML / ~94 KB gzip, added to the Go binary at compile time.

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Separate React web app at a different URL | Requires separate server, CORS, auth; breaks the single-binary deployment model |
| Serve HTML from filesystem at runtime | Deployment requires distributing both the binary and the HTML file; breaks self-contained binary guarantee |
| Plain MCP text responses | Cannot render hotel cards, thumbnails, or interactive filters in a visual grid |
| Inline HTML with CDN `<script>` tags | CDN origins blocked by Claude Desktop's iframe CSP |
