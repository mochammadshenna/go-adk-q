# Tutorial: Modifying the Dashboard UI

This tutorial walks through the full UI development cycle: editing the TypeScript source,
rebuilding the bundle, embedding it into the Go binary, and seeing the change in Claude Desktop.

---

## Prerequisites

- Node.js 18+ and npm are installed (`node --version`, `npm --version`).
- You have run `make build` at least once so `ui/node_modules/` and `ui/dist/` exist.
- Claude Desktop is configured to run this server.

---

## The single-file architecture

The entire dashboard UI lives in one TypeScript file:

```
ui/src/mcp-app.ts
```

The build pipeline (esbuild via a Vite-style script) bundles and minifies it to:

```
ui/dist/index.html
```

`make build-ui` then copies that file to:

```
internal/resources/mcp-app.html
```

`make build-go` then compiles the Go binary, and the `//go:embed mcp-app.html` directive in
`internal/resources/dashboard.go` bakes the HTML into the binary at compile time.

**There is no dev server and no hot-reload.** Every change requires the full cycle:
`make build-ui` → `make build-go` → restart Claude Desktop.

---

## The ext-apps SDK pattern

The UI communicates with the MCP server through `@modelcontextprotocol/ext-apps`, imported
at the top of `mcp-app.ts`:

```typescript
import {
  App,
  applyDocumentTheme,
  applyHostFonts,
  applyHostStyleVariables,
} from "@modelcontextprotocol/ext-apps";
```

`App` is the bridge between the dashboard iframe and Claude Desktop. The lifecycle in
`main()` is:

```typescript
const app = new App({ name: "Archipelago Hotels Dashboard", version: "2.0.0" });

// Called when a tool result arrives (full result at once)
app.ontoolresult = (result) => { ... };

// Called as streaming chunks arrive (partial result)
app.ontoolinputpartial = (params) => { ... };

// Called when the host theme changes (dark mode toggle, etc.)
app.onhostcontextchanged = (ctx) => {
  applyDocumentTheme(ctx.theme);
  applyHostStyleVariables(ctx.styles.variables);
  applyHostFonts(ctx.styles.css.fonts);
};

await app.connect();
```

The `appRef` module-level variable stores the `App` instance so that card click handlers
(which fire later, outside `main`) can call `appRef.callServerTool(...)` to fetch room
detail without going through Claude.

---

## Tool result to UI update: the data flow

When Claude calls `hotel_dashboard` or `search_hotels`, the Go handler returns a typed
`searchResult` struct (or `DashboardData`). The MCP SDK serialises it as `structuredContent`
in the tool result envelope. The dashboard receives it via `app.ontoolresult`:

```typescript
app.ontoolresult = (result: any): void => {
    if (result?.isError) {
        const t = result.content?.find((c: any) => c.type === "text");
        showError(t?.text ?? "Tool returned an error.");
        return;
    }
    const data = result?.structuredContent as DashboardData | undefined;
    if (data?.hotels?.length) showDashboard(data);
};
```

`showDashboard` stores the hotels in `state.allHotels` and calls `applyFilters()`, which
calls `renderHotels()`, which writes the hotel card grid to the DOM. The whole path is:

```
Go handler returns typed struct
  → SDK serialises to structuredContent
    → app.ontoolresult fires in UI
      → showDashboard(data)
        → applyFilters()
          → renderHotels(filtered)
            → DOM updated
```

If you add a new field to the Go result struct, add the matching property to the TypeScript
interface (e.g. `HotelSummary`) and it will be available in the render functions.

---

## How fmtPrice works

```typescript
function fmtPrice(v: number, currency: string): string {
  if (v <= 0 || !currency) return "";
  const locale = currency.toUpperCase() === "IDR" ? "id-ID" : "en-US";
  return currency + " " + Math.round(v).toLocaleString(locale);
}

const fmtPriceShort = fmtPrice;
const fmtPriceFull  = fmtPrice;
```

The currency code comes from the hotel row in the database (`h.Currency` in Go), which
stores the raw ISO 4217 code (e.g. `"IDR"`). When the code is IDR, the locale `id-ID` is
used so that `toLocaleString` formats the number with Indonesian thousand separators
(a period as the thousands separator, e.g. `IDR 850.000`). For all other currencies the
`en-US` locale is used (comma separators). `fmtPriceShort` and `fmtPriceFull` are aliases
for `fmtPrice`; they exist because an earlier version had separate compact and full
formatters. All three now behave identically.

---

## How thumbnails are displayed

The Go backend fetches a thumbnail URL from the database and rewrites it to a resized
version before placing it in the result struct:

```go
Thumbnail: thumbMap[h.HotelID],  // already a resized CDN URL
```

In the UI, the thumbnail is rendered as a plain `<img>` tag inside the card photo area:

```typescript
${h.thumbnail
  ? `<img class="card-photo-thumb" src="${esc(h.thumbnail)}" alt="" loading="lazy" onerror="this.remove()">`
  : ""}
```

The `onerror="this.remove()"` attribute silently removes the broken image if the CDN
returns an error, allowing the CSS gradient fallback to show instead. The `loading="lazy"`
attribute defers off-screen images. The `esc()` function HTML-encodes the URL to prevent
XSS.

For the overlay hero image (the large header in the room detail panel), the same pattern
applies:

```typescript
const heroThumb = detail.thumbnail
  ? `<img class="overlay-hero-thumb" src="${esc(detail.thumbnail)}" alt="" loading="lazy" onerror="this.remove()">`
  : "";
```

---

## Walkthrough: adding a new stat to the header bar

The header bar currently shows five stat cards: Hotels, Brands, Cities, Avg Rating, and
From/Night. This walkthrough adds a sixth stat: **Stars** (average star rating across
displayed hotels).

### 1. Add the DOM element

Find the `stats-row` block in the `buildDOM` function (search for `stats-row` in
`mcp-app.ts`). It looks like this:

```typescript
<div class="stats-row">
  <div class="stat-card"><span class="stat-val" id="s-hotels">—</span><span class="stat-lbl">Hotels</span></div>
  <div class="stat-card"><span class="stat-val" id="s-brands">—</span><span class="stat-lbl">Brands</span></div>
  <div class="stat-card"><span class="stat-val" id="s-cities">—</span><span class="stat-lbl">Cities</span></div>
  <div class="stat-card"><span class="stat-val" id="s-rating">—</span><span class="stat-lbl">Avg Rating</span></div>
  <div class="stat-card"><span class="stat-val" id="s-price">—</span><span class="stat-lbl">From/Night</span></div>
</div>
```

Add a sixth card at the end of the row:

```typescript
<div class="stat-card"><span class="stat-val" id="s-stars">—</span><span class="stat-lbl">Avg Stars</span></div>
```

### 2. Populate the stat in renderStats

Find the `renderStats` function. It currently ends with:

```typescript
setText("s-price", minHotel ? fmtPriceShort(minHotel.priceFrom, minHotel.currency) : "—");
```

Add the new calculation after it:

```typescript
const withStars = hotels.filter(h => h.stars > 0);
const avgStars = withStars.length > 0
  ? withStars.reduce((s, h) => s + h.stars, 0) / withStars.length
  : 0;
setText("s-stars", avgStars > 0 ? avgStars.toFixed(1) : "—");
```

### 3. Rebuild the UI bundle

```bash
make build-ui
```

This runs `npm install --silent && npm run build` inside `ui/`, then copies
`ui/dist/index.html` to `internal/resources/mcp-app.html`.

### 4. Rebuild the Go binary

```bash
make build-go
```

This compiles the Go binary at `bin/archipelago-hotels-mcp`. The `//go:embed mcp-app.html`
directive in `internal/resources/dashboard.go` embeds the updated HTML file into the binary.
If you skip this step, Claude Desktop will still serve the old binary with the old HTML.

### 5. Restart Claude Desktop

Quit Claude Desktop completely (Cmd+Q on macOS) and reopen it. Claude Desktop launches a
fresh server process from the binary path configured in `claude_desktop_config.json`. The
new stat card will appear the next time you trigger a `hotel_dashboard` or `search_hotels`
call.

---

## Full build shortcut

To rebuild everything in one command:

```bash
make build
```

This runs `build-ui` then `build-go` in sequence. You still need to restart Claude Desktop
manually afterward.

---

## Pitfalls

### Binary must be rebuilt AND Claude Desktop must restart

This is the most common mistake. There are two separate caches to invalidate:

1. The **binary** embeds the HTML. A change to `mcp-app.ts` does not affect the running
   binary until you run `make build-go`.
2. Claude Desktop **caches the running server process**. Saving a new binary while Claude
   Desktop is open has no effect until you restart Claude Desktop — it continues to serve
   calls to the old process.

The correct sequence for every UI change is always:

```
edit mcp-app.ts → make build-ui → make build-go → restart Claude Desktop
```

Skipping any step will result in your changes not appearing.

### CSS changes are also compiled in

All CSS is injected via the `STYLES` template literal in `mcp-app.ts`, not a separate
stylesheet. If you add new CSS classes, they must be part of the same string and they are
subject to the same rebuild-and-restart cycle.

### The stat card may be hidden on narrow screens

The existing CSS hides the 4th and 5th stat cards on narrow viewports:

```css
@media (max-width: 600px) {
  .stats-row .stat-card:nth-child(4),
  .stats-row .stat-card:nth-child(5) { display: none; }
}
```

A sixth card will also be hidden by `nth-child` rules if the viewport is narrow. To keep
your new card visible, add it to the media query exception, or accept that it only appears
on wider screens.

### ext-apps SDK is a peer dependency, not bundled

`@modelcontextprotocol/ext-apps` is installed as a local npm package in `ui/node_modules/`.
The build step bundles it into the output HTML. If `node_modules/` is missing (e.g. after a
clean), run `make build-ui` which calls `npm install` automatically, or run
`cd ui && npm install` manually before building.
