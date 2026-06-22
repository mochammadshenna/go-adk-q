# MCP Apps UI Protocol — How the Archipelago Hotel Dashboard Works

This document explains the MCP Apps (`ext-apps`) protocol and how the Archipelago Hotels
dashboard uses it: from Go resource registration, through Claude Desktop opening the panel,
to the TypeScript lifecycle and every detail of image safety and price formatting.

---

## Contents

1. [What MCP Apps Are](#1-what-mcp-apps-are)
2. [Resource Registration](#2-resource-registration)
3. [Tool Linkage](#3-tool-linkage)
4. [App-Only Tool Visibility](#4-app-only-tool-visibility)
5. [How Claude Desktop Opens the UI](#5-how-claude-desktop-opens-the-ui)
6. [UI Lifecycle: Events and Data Flow](#6-ui-lifecycle-events-and-data-flow)
7. [Frontend Architecture](#7-frontend-architecture)
8. [State Management with the App Class](#8-state-management-with-the-app-class)
9. [Price Formatting](#9-price-formatting)
10. [Image CSP and resourceDomains](#10-image-csp-and-resourcedomains)
11. [Sequence Diagram](#11-sequence-diagram)

---

## 1. What MCP Apps Are

An MCP App is an HTML page that runs inside the MCP host (Claude Desktop) rather than in a
separate browser tab. It communicates with the MCP server over the **same session** that
handles tool calls — no separate HTTP server, no CORS, no authentication layer.

The host recognises an MCP App by its MIME type:

```
text/html;profile=mcp-app
```

The `profile=mcp-app` parameter is the protocol signal. Without it the host treats the
resource as static HTML and does not inject the `ext-apps` JavaScript bridge. With it the
host:

- Renders the HTML in an embedded webview (iframe equivalent).
- Injects the `@modelcontextprotocol/ext-apps` runtime, making the `App` class available.
- Forwards tool call results to the running page via the `ontoolresult` / `ontoolinput`
  callbacks.
- Applies the host's visual theme (dark/light mode, font variables, safe-area insets) via
  `onhostcontextchanged`.
- Enforces a Content Security Policy that blocks all external origins except those
  explicitly declared in `resourceDomains`.

The MCP App model is **server-push, not client-pull**. The page does not issue HTTP
requests to fetch its data; the host delivers tool results directly into the page's
JavaScript callbacks.

---

## 2. Resource Registration

The dashboard HTML is compiled from TypeScript at build time and embedded into the Go
binary with `//go:embed`. It is registered as a MCP resource in
`internal/resources/dashboard.go`:

```go
// internal/resources/dashboard.go

const ResourceURI = "ui://hotel-dashboard"

//go:embed mcp-app.html
var dashboardHTML string

func RegisterDashboardResource(s *mcp.Server) {
    s.AddResource(&mcp.Resource{
        URI:         ResourceURI,
        Name:        "Archipelago Hotels Dashboard",
        Description: "Interactive hotel dashboard …",
        MIMEType:    "text/html;profile=mcp-app",   // ← protocol signal
        Meta: mcp.Meta{
            "ui": map[string]any{
                "resourceDomains": []string{"images.archipelagohotels.com"},
            },
        },
    }, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
        return &mcp.ReadResourceResult{
            Contents: []*mcp.ResourceContents{{
                URI:      ResourceURI,
                MIMEType: "text/html;profile=mcp-app",
                Text:     dashboardHTML,
            }},
        }, nil
    })
}
```

Three details matter here:

| Field | Value | Why |
|---|---|---|
| `URI` | `ui://hotel-dashboard` | Custom scheme; not HTTP. Avoids ambiguity with web resources and is stable across transports (stdio and HTTP). |
| `MIMEType` | `text/html;profile=mcp-app` | Activates the ext-apps host treatment. |
| `_meta.ui.resourceDomains` | `["images.archipelagohotels.com"]` | CSP allowlist. See [Section 10](#10-image-csp-and-resourcedomains). |

The HTML is served verbatim from the embedded string — the Go server does no templating.
All dynamic content arrives at runtime through the ext-apps event callbacks.

---

## 3. Tool Linkage

A tool is linked to an MCP App resource by adding `_meta.ui.resourceUri` to the tool's
`Meta` field. When Claude calls a linked tool, the host:

1. Reads the resource at the declared URI.
2. Opens (or focuses) the MCP App panel.
3. Delivers the tool result into the page's `ontoolresult` callback.

`find_hotels` and `search_hotels` are both linked:

```go
// internal/tools/dashboard.go (find_hotels)
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     resources.ResourceURI,                       // "ui://hotel-dashboard"
        "resourceDomains": []string{"images.archipelagohotels.com"},    // CSP allowlist on tool results
    },
},
```

The `resourceDomains` field on the **tool** is the CSP grant for content delivered through
that specific tool call. It mirrors the resource-level declaration because tool results can
carry thumbnail URLs that reference the CDN, and the host needs the allowlist at the point
of delivery, not only at panel-open time.

A tool can be linked to exactly one resource URI. The resource URI must match a registered
resource in the same MCP server.

---

## 4. App-Only Tool Visibility

`get_hotel_detail` is registered with `visibility: ["app"]`:

```go
// internal/tools/detail.go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri": resources.ResourceURI,
        "visibility":  []string{"app"},
    },
},
```

`visibility: ["app"]` tells the host to hide this tool from Claude's tool list. Claude
never sees it, never calls it, and cannot include it in a response. Only the TypeScript
running inside the MCP App panel can invoke it.

This solves a real problem: `get_hotel_detail` fetches full room inventory for a single
hotel. It is not useful to Claude as a conversational tool (too low-level, too narrow); it
is useful only when a user clicks a hotel card in the UI. Keeping it app-only:

- Prevents Claude from calling it speculatively and wasting rate quota.
- Keeps Claude's tool list clean (only the three discovery tools are visible).
- Makes the tool's security model explicit — it is an internal API for the UI, not a
  general-purpose capability.

The TypeScript calls it via the `App` instance, not via a raw `fetch`:

```typescript
// ui/src/mcp-app.ts
async function openOverlay(hotelId: string, hotelName: string): Promise<void> {
    const result = await appRef.callTool("get_hotel_detail", { hotelId });
    const detail = result.structuredContent as HotelDetail;
    renderDetailOverlay(detail);
}
```

---

## 5. How Claude Desktop Opens the UI

When Claude calls a tool that has `_meta.ui.resourceUri` set, the host executes this
sequence automatically — no special prompt engineering or tool output required:

1. **Tool call received.** Claude issues a `tools/call` request for `find_hotels` or
   `search_hotels`.
2. **Resource lookup.** The host reads the `resourceUri` from the tool's `_meta` and
   fetches `ui://hotel-dashboard` from the server via a `resources/read` request.
3. **Panel open.** The host renders the HTML in its embedded webview. If the panel is
   already open (user called the tool a second time), it is focused rather than re-created.
4. **Result delivery.** The tool's structured result is pushed into the page's
   `ontoolinput` callback (during streaming) and `ontoolresult` callback (on completion).
5. **Theme sync.** `onhostcontextchanged` fires with the host's current theme, font
   variables, and safe-area insets. The page calls `applyDocumentTheme`,
   `applyHostStyleVariables`, and `applyHostFonts` to stay visually consistent with the
   host UI.

The user does not click "open dashboard." Calling any linked tool opens it automatically.

---

## 6. UI Lifecycle: Events and Data Flow

The TypeScript defines four lifecycle callbacks on the `App` instance:

```typescript
// ui/src/mcp-app.ts — main()

const app = new App({ name: "Archipelago Hotels Dashboard", version: "2.0.0" });

// Streaming partial results — render as data arrives
app.ontoolinputpartial = (params: ToolInputParams): void => {
    if (params.structuredContent?.hotels?.length) showDashboard(params.structuredContent);
};

// Final tool input (same shape, confirmed complete)
app.ontoolinput = (params: ToolInputParams): void => {
    const data = params.structuredContent as DashboardData | undefined;
    if (data?.hotels?.length) showDashboard(data);
};

// Tool result after execution
app.ontoolresult = (result: any): void => {
    if (result?.isError) { showError(…); return; }
    const data = result?.structuredContent as DashboardData | undefined;
    if (data?.hotels?.length) showDashboard(data);
};

// Host theme / layout changes (dark mode toggle, window resize, etc.)
app.onhostcontextchanged = (ctx: any): void => {
    if (ctx?.theme)               applyDocumentTheme(ctx.theme);
    if (ctx?.styles?.variables)   applyHostStyleVariables(ctx.styles.variables);
    if (ctx?.styles?.css?.fonts)  applyHostFonts(ctx.styles.css.fonts);
    if (ctx?.safeAreaInsets) { … apply padding … }
};

// Panel closing — clean up state
app.onteardown = async (): Promise<Record<string, unknown>> => {
    state.allHotels = [];
    return {};
};

await app.connect(); // establish session with host
```

The page starts in a loading state (shimmer skeleton cards). The first `ontoolinput` or
`ontoolresult` event carrying a non-empty `hotels` array calls `showDashboard`, which
populates the hotel grid and filter dropdowns from the delivered data.

### Data shape delivered by tools

`find_hotels` and `search_hotels` both return a `DashboardData` object as their
`structuredContent`. The Go type is `dashboardData`; the TypeScript type is `DashboardData`:

```typescript
interface DashboardData {
    filter:  string;          // city or brand filter active, or ""
    hotels:  HotelSummary[];  // array of hotel summaries
    total:   number;          // total matching (before limit)
    match:   number;          // hotels returned
    message: string;          // human-readable summary
}
```

Each `HotelSummary` carries everything the card grid needs: `id`, `name`, `brand`, `city`,
`priceFrom`, `currency`, `imageStyle`, `brandColor`, `thumbnail`, `rating`, `stars`,
`tags`. The detail overlay is loaded on demand via a separate `get_hotel_detail` call.

---

## 7. Frontend Architecture

### Single TypeScript file, Vite bundle, `//go:embed`

The entire frontend lives in `ui/src/mcp-app.ts` (~1 200 lines). This is a deliberate
constraint of the MCP Apps protocol: the resource must be a **single self-contained HTML
file**. No external scripts, no CSS imports, no remote fonts.

The build pipeline enforces this:

```makefile
# Makefile
build-ui:
    cd ui && npm install --silent && npm run build
    cp ui/dist/index.html internal/resources/mcp-app.html

build-go:
    go build -o bin/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp

build: build-ui build-go
```

Vite uses `vite-plugin-singlefile` to inline all JavaScript and CSS into `index.html`. The
result (`ui/dist/index.html`, ~383 KB minified) is then copied to
`internal/resources/mcp-app.html`, where `//go:embed` picks it up at `go build` time.

The binary is therefore entirely self-contained: no separate HTML file to distribute, no
filesystem dependency at runtime.

### No framework

The TypeScript does not use React, Vue, or any component library. The UI is built with
direct DOM manipulation and innerHTML assignment. This keeps the bundle small (no framework
runtime), keeps the build simple (Vite + esbuild only), and avoids hydration or
reconciliation overhead in a single-page panel that re-renders on every tool call anyway.

HTML is generated as template literal strings, with every value passed through the `esc()`
function (HTML entity encoding) before insertion:

```typescript
function esc(s: string): string {
    return String(s)
        .replace(/&/g, "&amp;").replace(/</g, "&lt;")
        .replace(/>/g, "&gt;").replace(/"/g, "&quot;")
        .replace(/'/g, "&#x27;");
}
```

### CSS injection

Styles are defined as a template literal constant (`STYLES`) and injected into a `<style>`
element at startup by `injectStyles()`. The host's CSS custom properties (`--font-sans`,
`--color-background-primary`, `--color-text-primary`) are consumed directly as CSS variable
fallbacks, so the page inherits the host's typography and colour scheme for neutral
surfaces while applying brand-specific gradients for hotel cards.

---

## 8. State Management with the App Class

The `App` class from `@modelcontextprotocol/ext-apps` is the bridge between the host and
the page. It does two things:

1. **Receives events** from the host (tool inputs, host context changes, teardown).
2. **Sends requests** back to the server (`callTool`).

The page maintains its own lightweight state object:

```typescript
const state: State = {
    allHotels:   [],  // full hotel list from last tool result
    searchQuery: "",  // user's search box value
    cityFilter:  "",  // dropdown selection
    brandFilter: "",  // dropdown selection
    sortBy:      "",  // sort order key
};
```

`showDashboard(data)` writes `data.hotels` into `state.allHotels`, then calls
`applyFilters()`, which filters and sorts the in-memory list and calls `renderHotels()` to
rewrite the grid. Filter changes (dropdown, search input) call `applyFilters()` directly —
no new tool call is needed because the full hotel list is already in memory.

The detail overlay is the one place where a new tool call is issued. When a user clicks a
hotel card, `openOverlay()` calls `appRef.callTool("get_hotel_detail", { hotelId })`. The
overlay renders from the `structuredContent` of the response.

---

## 9. Price Formatting

Hotel prices are stored and transmitted as raw numeric values with a raw ISO 4217 currency
code (e.g. `"IDR"`, `"USD"`). The formatting function handles locale-appropriate
separators:

```typescript
// ui/src/mcp-app.ts
function fmtPrice(v: number, currency: string): string {
    if (v <= 0 || !currency) return "";
    const locale = currency.toUpperCase() === "IDR" ? "id-ID" : "en-US";
    return currency + " " + Math.round(v).toLocaleString(locale);
}
```

For Indonesian Rupiah (`IDR`), `toLocaleString("id-ID")` produces dot-separated thousands
(e.g. `IDR 1.250.000`), matching the convention Indonesian users expect. For all other
currencies, `"en-US"` produces comma-separated thousands (e.g. `USD 1,250`).

The currency code is prepended as a plain string prefix rather than using
`toLocaleString`'s built-in currency formatting (`style: "currency"`), because the
built-in inserts the symbol (`Rp`, `$`) rather than the ISO code. Archipelago Hotels
operates across 13 brands and multiple markets; the ISO code is unambiguous and does not
require a maintained symbol map. See [ADR-0007](../adr/ADR-0007-raw-currency-code.md).

---

## 10. Image CSP and resourceDomains

MCP App pages run under a strict Content Security Policy. By default, **all external
origins are blocked** — including image sources. A thumbnail URL like
`https://images.archipelagohotels.com/photo-aston-jakarta.jpg` would fail silently without
an explicit allowlist entry.

`resourceDomains` is the allowlist mechanism. It is declared in two places:

### On the resource (panel-level allowlist)

```go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

This grants the embedded webview permission to load images from
`images.archipelagohotels.com` for the lifetime of the panel.

### On each linked tool (result-level allowlist)

```go
// find_hotels and search_hotels
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     resources.ResourceURI,
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

When a tool result carries thumbnail URLs, the host needs the allowlist at delivery time
too. The tool-level declaration covers this.

### Why not base64-encode thumbnails?

Base64-encoding images server-side would bypass the CSP concern but would:

- Inflate tool result payloads significantly (images are 20–100 KB each; a 50-hotel result
  would add several MB to the MCP response).
- Require HTTP fetches on the Go server for every tool call, adding latency and coupling
  the server to image availability.
- Break the rate fallback chain's performance guarantees.

Instead, `resizeImageURL()` in `internal/repository/hotel.go` rewrites raw database image
paths into CDN-proxied URLs under `images.archipelagohotels.com`. The rewrite is a pure
string operation (no HTTP call). The browser then fetches directly from the CDN, which is
fast, cached, and covered by the `resourceDomains` allowlist. See
[ADR-0006](../adr/ADR-0006-resize-image-url-csp.md).

---

## 11. Sequence Diagram

```
User (Claude Desktop)          Claude (LLM)           MCP Server (Go)          UI (TypeScript)
        │                           │                        │                        │
        │  "Show hotels in Bali"    │                        │                        │
        │ ─────────────────────────>│                        │                        │
        │                           │  tools/call            │                        │
        │                           │  find_hotels           │                        │
        │                           │  { city: "Bali" }      │                        │
        │                           │ ──────────────────────>│                        │
        │                           │                        │  SearchHotels()        │
        │                           │                        │  BatchMinRates()       │
        │                           │                        │  GetThumbnails()       │
        │                           │                        │  resizeImageURL()      │
        │                           │                        │ ──────────────────┐    │
        │                           │                        │ <─────────────────┘    │
        │                           │  tool result           │                        │
        │                           │  structuredContent:    │                        │
        │                           │  { hotels: [...] }     │                        │
        │                           │ <──────────────────────│                        │
        │                           │                        │                        │
        │  Host sees _meta.ui       │                        │                        │
        │  .resourceUri set         │                        │                        │
        │ <─────────────────────────│                        │                        │
        │                           │  resources/read        │                        │
        │                           │  ui://hotel-dashboard  │                        │
        │ ──────────────────────────────────────────────────>│                        │
        │                           │                        │  return dashboardHTML  │
        │ <──────────────────────────────────────────────────│                        │
        │                           │                        │                        │
        │  Open MCP App panel       │                        │                        │
        │  (webview, CSP applied)   │                        │                        │
        │ ─────────────────────────────────────────────────────────────────────────> │
        │                           │                        │                        │  new App()
        │                           │                        │                        │  app.connect()
        │                           │                        │                        │  [loading state]
        │                           │                        │                        │
        │  Deliver tool result      │                        │                        │
        │  (ontoolresult / ontoolinput)                      │                        │
        │ ─────────────────────────────────────────────────────────────────────────> │
        │                           │                        │                        │  showDashboard(data)
        │                           │                        │                        │  renderHotels(hotels)
        │                           │                        │                        │  [hotel grid visible]
        │                           │                        │                        │
        │  User clicks hotel card   │                        │                        │
        │ ─────────────────────────────────────────────────────────────────────────> │
        │                           │                        │                        │  appRef.callTool(
        │                           │                        │                        │    "get_hotel_detail",
        │                           │                        │                        │    { hotelId: "42" }
        │                           │                        │                        │  )
        │                           │  tools/call            │                        │
        │                           │  get_hotel_detail      │                        │
        │                           │  { hotelId: "42" }     │                        │
        │                           │ ──────────────────────>│                        │
        │                           │                        │  GetHotelByID()        │
        │                           │                        │  GetRooms()            │
        │                           │                        │  GetRates()            │
        │                           │                        │ ──────────────────┐    │
        │                           │                        │ <─────────────────┘    │
        │                           │  tool result           │                        │
        │                           │  structuredContent:    │                        │
        │                           │  HotelDetail + rooms   │                        │
        │ ─────────────────────────────────────────────────────────────────────────> │
        │                           │                        │                        │  renderDetailOverlay()
        │                           │                        │                        │  [room cards visible]
        │                           │                        │                        │
        │  Claude responds to user  │                        │                        │
        │ <─────────────────────────│                        │                        │
```

### Key points the diagram illustrates

- **Claude does not open the UI.** The host opens it automatically when it sees
  `_meta.ui.resourceUri` on the tool result. Claude's response to the user is a separate
  text reply that happens in parallel.
- **`get_hotel_detail` is called by the TypeScript, not by Claude.** The call goes from
  the webview, through the host, to the MCP server. Claude is not involved.
- **The host fetches the resource HTML once.** Subsequent tool calls deliver data into the
  already-open panel; the HTML is not re-fetched on every call.
- **`resources/read` runs over the MCP session.** In stdio mode (Claude Desktop) this is
  over stdin/stdout. In HTTP mode it is over the Streamable HTTP connection on `:9011`.
  The panel works identically in both transports.

---

## Related

- [ADR-0005: Vite Single-File TypeScript UI as MCP App Resource](../adr/ADR-0005-mcp-apps-embedded-ui.md)
- [ADR-0006: resizeImageURL for CSP-Safe Thumbnails](../adr/ADR-0006-resize-image-url-csp.md)
- [ADR-0007: Raw Currency Code](../adr/ADR-0007-raw-currency-code.md)
- [Architecture Overview](architecture.md)
- Source: `internal/resources/dashboard.go`, `internal/tools/detail.go`,
  `internal/tools/search.go`, `internal/tools/dashboard.go`, `ui/src/mcp-app.ts`
