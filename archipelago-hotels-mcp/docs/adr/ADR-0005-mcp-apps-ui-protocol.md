# ADR-0005: MCP Apps ext-apps Protocol for Embedded UI

**File(s):** `internal/resources/dashboard.go`, `internal/tools/search.go`, `internal/tools/recommend.go`, `internal/tools/dashboard.go`, `internal/tools/detail.go`, `ui/src/mcp-app.ts`
**Decision date:** 2026-06-22

---

## Context

Operators of Archipelago Hotels wanted a visual hotel-browsing dashboard accessible from Claude Desktop without requiring external hosting, a separate web server, or user-managed URLs. Claude Desktop enforces a strict Content Security Policy on any embedded iframe, blocking arbitrary external origins. The standard MCP protocol does not define a UI layer.

Three options were evaluated:

| Option | Problem |
|--------|---------|
| Serve a standalone web app on a fixed port and document the URL | Requires a running HTTP server; users must navigate to the URL manually; CSP blocks localhost iframes in some Claude Desktop builds |
| Generate HTML as tool output (artifact-style) | Claude's artifact sandbox does not permit `fetch()` calls back to `localhost`; cannot call MCP tools; no persistent state |
| MCP Apps ext-apps protocol (chosen) | Protocol-native; UI lives inside the MCP session; can call back to the server's MCP tools directly |

---

## Decision

Register the compiled UI as an MCP App resource using the ext-apps protocol. This protocol is a Claude Desktop extension over MCP that causes the host to open a managed iframe panel when a tool with a `_meta.ui.resourceUri` reference is invoked.

### Protocol mechanics

#### 1. MIME type signal

The resource must be served with `MIMEType: "text/html;profile=mcp-app"`. The `profile=mcp-app` parameter is the flag the Claude Desktop host inspects to decide whether to open the ext-apps panel rather than displaying the resource as plain text.

```go
// internal/resources/dashboard.go
s.AddResource(&mcp.Resource{
    URI:      "ui://hotel-dashboard",
    Name:     "Archipelago Hotels Dashboard",
    MIMEType: "text/html;profile=mcp-app",
    ...
}, handler)
```

#### 2. Resource URI scheme

The resource URI uses the custom `ui://` scheme rather than `http://` or `https://`. This communicates to the host that the resource is not a remote URL — the host fetches it via MCP `resources/read`, not via a network request. The binary serves the UI over the MCP session itself, so the server works in both stdio and HTTP transport modes.

```
ui://hotel-dashboard
```

#### 3. Tool linkage via `_meta.ui.resourceUri`

Any tool that should trigger the UI panel declares `_meta.ui.resourceUri` pointing at the resource URI. When Claude Desktop invokes such a tool, it automatically opens (or focuses) the ext-apps panel showing that resource. This linkage is set on all three public tools so the panel appears on any hotel search or browse action.

```go
// Example from internal/tools/search.go
mcp.AddTool(s, &mcp.Tool{
    Name: "search_hotels",
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceUri":     "ui://hotel-dashboard",
            "resourceDomains": []string{"images.archipelagohotels.com"},
        },
    },
}, handler)
```

#### 4. CSP domain allowlist via `resourceDomains`

The ext-apps iframe enforces a Content Security Policy that blocks all external origins by default. The `resourceDomains` field in `_meta.ui` adds entries to the iframe's `img-src` (and related) CSP directives. This field must be set on **both** the resource registration and every tool registration that links to that resource; the host union-merges all declarations it sees for a given resource URI.

The only declared domain is `images.archipelagohotels.com`, which is the single CDN endpoint that all thumbnail URLs are rewritten to via `resizeImageURL` (see ADR-0006).

```go
// Same field on the resource registration:
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

#### 5. App-only tool (`visibility: app`)

`get_hotel_detail` is registered with `visibility: "app"` in its metadata. This causes Claude Desktop to hide the tool from Claude's tool list — Claude cannot call it directly. Only the TypeScript UI (running inside the ext-apps iframe) calls it via `app.callTool("get_hotel_detail", { id })`. This keeps the full hotel detail payload (room types, policies, all photos) out of Claude's context window and avoids token cost for every hotel browsed.

```go
// internal/tools/detail.go
mcp.AddTool(s, &mcp.Tool{
    Name: "get_hotel_detail",
    Meta: mcp.Meta{
        "visibility": "app",
        "ui": map[string]any{
            "resourceUri":     "ui://hotel-dashboard",
            "resourceDomains": []string{"images.archipelagohotels.com"},
        },
    },
}, detailHandler(pool, rateSvc))
```

```typescript
// ui/src/mcp-app.ts — UI side
async function loadHotelDetail(hotelId: string): Promise<void> {
    const result = await app.callTool("get_hotel_detail", { id: hotelId });
    renderDetailOverlay(result.structuredContent as HotelDetail);
}
```

### Summary of protocol fields

| Field | Location | Value | Effect |
|-------|----------|-------|--------|
| `MIMEType` | Resource | `text/html;profile=mcp-app` | Host opens ext-apps panel |
| `URI` | Resource | `ui://hotel-dashboard` | Custom scheme — fetched via MCP, not HTTP |
| `_meta.ui.resourceUri` | Tool | `ui://hotel-dashboard` | Links tool invocation to panel open |
| `_meta.ui.resourceDomains` | Resource + Tool | `["images.archipelagohotels.com"]` | Adds domain to iframe CSP allowlist |
| `_meta.visibility` | Tool | `"app"` | Hides tool from Claude; accessible only from the UI |

---

## Consequences

**Positive:**

- Zero external hosting. The UI is embedded in the binary; no separate server, CDN, or deployment step is needed.
- Works in both stdio (Claude Desktop) and HTTP transport modes without code divergence.
- The ext-apps iframe can call back to the MCP server's tools directly via the active session, enabling rich interactivity (detail overlays, filter state) without a separate REST handshake.
- App-only tools keep large payloads out of Claude's context, reducing token usage and preventing accidental exposure of detailed pricing or room inventory to the LLM.

**Negative / risks:**

- The ext-apps protocol (`profile=mcp-app`, `resourceDomains`, `visibility: app`) is a Claude Desktop extension. It is not part of the core MCP specification. Other MCP clients (e.g., headless agents, third-party hosts) will not render the panel; tool calls will succeed but no UI will appear.
- A UI change requires a full binary rebuild (`make build`). There is no live-reload path for the embedded production asset; developers use `make dev-http` with the Vite dev server for iteration.
- The `resourceDomains` allowlist must be kept in sync across all tool registrations and the resource registration manually. A mismatch (e.g., a new tool that does not declare the domain) will cause broken thumbnails in some Claude Desktop versions that evaluate per-tool rather than unioning all declarations.
