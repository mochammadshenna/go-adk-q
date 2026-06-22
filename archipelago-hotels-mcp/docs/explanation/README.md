# Explanation

> **Understanding-oriented.** Background concepts and design rationale.
> For task recipes see [How-To Guides](../how-to/README.md);
> for exact specifications see [Reference](../reference/README.md).

---

## Architecture overview {#architecture}

The server is a single Go binary that speaks two transports:

- **stdio** — used by Claude Desktop; the binary is launched as a child process
  and communicates over stdin/stdout using JSON-RPC.
- **Streamable HTTP** — used by Claude API agents and the ext-app UI; Gin listens
  on `:9011` and handles both the MCP `/mcp` endpoint and the REST API endpoints
  that power the embedded dashboard.

All MCP logic is handled by `github.com/modelcontextprotocol/go-sdk`. The server
registers four tools and one resource (the ext-app HTML/JS bundle). Tool handlers
are thin — they validate input, call into the `repository` and `rate` packages,
and serialize results as JSON text content.

The database layer uses `database/sql` with `go-sql-driver/mysql`. A single
`repository.Pool` struct holds one `*sql.DB` per database. The central catalog
connection is established eagerly at startup; brand DB connections are opened
lazily on the first query that needs them.

---

## Rate resolution algorithm {#rate-resolution}

Fetching a live rate requires three steps: identify the booking channel, call
the appropriate API, and fall back if the call fails.

### Step 1 — Identify the channel

```
hotel_channel column in brand DB tb_hotels
    'SENTEC'  → call Sentec REST API
    'SB' / '' / NULL → call SimpleBooking XML API
    brand has no per-brand DB → skip live rate, use starting_price
```

### Step 2 — Call the live API

For SimpleBooking: build an `OTA_HotelAvailRQ` XML payload with the hotel's
`simplebooking_id`, a one-night stay window starting tomorrow, and 2 adult guests.
Parse the minimum `AmountAfterTax` from all returned room stays.

For Sentec: POST to the availability search endpoint with the hotel's
`sentec_booking_id`. Parse `final_rate` from the JSON response.

Both calls run through a shared worker pool (5 workers) so that a
`BatchMinRates` call for 20 hotels issues at most 5 concurrent outbound
requests.

### Step 3 — Fallback chain

```
Live API call succeeds               → rate_source: "live"
Live API call fails / times out      → use tb_hrooms.room_rate
                                       → rate_source: "stored"
No tb_hrooms row found               → use hotel_starting_price from central DB
                                       → rate_source: "starting_price"
hotel_starting_price is NULL / zero  → return null rate (hotel cannot be priced)
```

Responses always include `rate_source` so the caller can surface a freshness
warning to the end user.

---

## Brand-to-database mapping {#brand-db-mapping}

Archipelago's historical growth created a fragmented database topology. Each
major brand has its own MySQL database, all on the same host, named
`db_{prefix}website` where `prefix` comes from `tb_brands.db_prefix_name` in
the central catalog.

Three brands — Huxley, NORDIC, Four Corners — have no per-brand database. Their
hotels appear in the central catalog but only have `hotel_starting_price` for
rates; no room-level data is available.

PBA (Powered By Archi) uses `db_pba` and has schema differences:
- `tb_hroom` (singular, not `tb_hrooms`)
- Primary key is a UUID string, not an integer
- `hotel_simplebooking` column instead of `simplebooking_id`
- No `hotel_channel` column — always treated as SimpleBooking

The server handles these divergences through `INFORMATION_SCHEMA.COLUMNS`
introspection performed once per brand DB on first connection. Results are
cached for the lifetime of the process.

---

## MCP transport choices {#mcp-transport}

**Why stdio for Claude Desktop?** Claude Desktop launches MCP servers as child
processes and communicates over stdin/stdout. This requires no open ports and no
network configuration, which is the right default for a local developer setup.

**Why HTTP for agents?** Claude API agents need to call MCP tools over a network.
The MCP Streamable HTTP transport (`POST /mcp`) supports this while reusing the
same tool registration code. Gin was chosen over `net/http` because several team
members are already familiar with it and it adds negligible overhead for this
use case (see [ADR-4](../adr/README.md#adr-4)).

**Why not SSE?** The MCP spec supports both SSE and Streamable HTTP. Streamable
HTTP is simpler to implement and does not require the client to maintain a
persistent connection, which is preferable for serverless or short-lived agent
invocations.

---

## Ext-app (embedded UI) {#ext-app}

The dashboard is a TypeScript single-page application compiled by Vite into
`ui/dist/`. The Go binary embeds this directory via `//go:embed` and serves it
as an MCP App resource (`ui://hotel-dashboard`). Claude Desktop's ext-apps panel
loads this resource in an iframe.

The ext-app calls the server's REST API endpoints (`/api/hotels`, `/api/brands`,
`/api/regions`) and the `get_hotel_detail` MCP tool (via `visibility: app`) to
render hotel cards and room details. Because the iframe is same-origin with the
Go HTTP server, there are no CORS issues. Images are proxied through the
`url_image_resizer` CDN to avoid Content Security Policy violations that would
occur if the iframe tried to load images directly from third-party domains.

---

## Failure modes and graceful degradation {#failure-modes}

The server is designed so that partial data is always better than no data.

| Failure | Behaviour |
|---------|-----------|
| Central DB unreachable on startup | Process exits immediately — tools cannot function without the catalog |
| Brand DB unreachable | Tool returns hotel profile from central DB; rooms and live rates are omitted; `rate_source: "starting_price"` |
| SimpleBooking API timeout | Falls back to stored `room_rate`; logs warning |
| SimpleBooking circuit breaker open | Same as timeout; `/health` reports breaker state |
| Hotel has no `simplebooking_id` | Skips live API call; uses stored rate |
| Hotel has `hotel_channel = 'SENTEC'` but Sentec call fails | Falls back to stored rate |
| `room_rate` is zero or NULL | Falls back to `hotel_starting_price` |
| `hotel_starting_price` is NULL | Returns hotel with `min_rate: null` |

This means a search result will always contain hotel names, locations, and brand
information even when rate data is completely unavailable. The `rate_source` field
lets the LLM tell the user when prices may be stale.
