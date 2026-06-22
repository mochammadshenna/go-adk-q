# HTTP API Reference

This document covers all HTTP endpoints exposed when the server runs in **HTTP mode** (default port `:9011`).

HTTP mode is started via:

```sh
./archipelago-hotels-mcp --http
# or
make dev-http
```

Stdio mode (Claude Desktop) does not expose any of these endpoints.

---

## Base URL

```
http://localhost:9011
```

---

## CORS

All endpoints include the following CORS headers on every response (including preflight):

| Header | Value |
|--------|-------|
| `Access-Control-Allow-Origin` | `*` |
| `Access-Control-Allow-Methods` | `GET, POST, OPTIONS` |
| `Access-Control-Allow-Headers` | `Content-Type, Mcp-Session-Id, Authorization` |
| `Access-Control-Expose-Headers` | `Mcp-Session-Id` |

`OPTIONS` preflight requests return `204 No Content` with no body.

---

## Endpoints

### `POST /mcp` — MCP Streamable HTTP (write)

The primary MCP transport endpoint. Accepts MCP JSON-RPC requests from any MCP client (Claude Desktop, Claude.ai, custom clients).

**Content-Type:** `application/json`

**Request body:** Standard MCP JSON-RPC 2.0 envelope.

**Session management:** Include `Mcp-Session-Id` in the request header to resume an existing session. The server echoes the assigned session ID in the `Mcp-Session-Id` response header.

```sh
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }'
```

**Example response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      { "name": "search_hotels", "description": "...", "inputSchema": {} },
      { "name": "recommend_hotel", "description": "...", "inputSchema": {} },
      { "name": "find_hotels", "description": "...", "inputSchema": {} }
    ]
  }
}
```

---

### `GET /mcp` — MCP Streamable HTTP (read / SSE)

Server-Sent Events stream for receiving server-initiated messages (notifications, streamed tool results) on an established MCP session.

**Headers required:** `Mcp-Session-Id` — session ID obtained from a prior `POST /mcp` response.

```sh
curl -N http://localhost:9011/mcp \
  -H "Mcp-Session-Id: <session-id>"
```

**Response:** `text/event-stream` — SSE stream of JSON-RPC messages.

---

### `GET /dashboard` — Standalone HTML Dashboard

Returns the embedded single-page dashboard UI. This is the same UI registered as an MCP App resource (`ui://hotel-dashboard`) but served directly as a standalone web page — useful for browsers that are not MCP clients.

**Query parameters:** None.

**Response:** `text/html; charset=utf-8`

```sh
curl http://localhost:9011/dashboard
# or open in a browser:
open http://localhost:9011/dashboard
```

The page is fully self-contained (all CSS and JS inlined). It calls `/api/hotels`, `/api/brands`, and `/api/regions` to populate its filters and hotel cards.

---

### `GET /api/hotels` — Hotel List

Returns a JSON array of hotels matching the given filters. Used by the dashboard and by any REST client.

**Query parameters:**

| Parameter | Type | Required | Max length | Description |
|-----------|------|----------|-----------|-------------|
| `city` | string | No | 100 chars | Filter by city name or address fragment (case-insensitive `LIKE` match against `region_name` and `hotel_address`) |
| `brand` | string | No | 100 chars | Filter by brand name (case-insensitive `LIKE` match) |

When both parameters are omitted, up to 100 hotels are returned (server-side hard limit).

**Response:** `application/json` — array of hotel objects.

**Hotel object schema:**

```jsonc
{
  "HotelID":       1042,          // int — internal hotel ID
  "APIHotelID":    { "Int64": 55, "Valid": true }, // sql.NullInt64
  "BrandID":       3,             // int
  "BrandName":     "Hotel Neo",   // string
  "DBPrefix":      "neo",         // string — per-brand DB prefix
  "RegionName":    "Jakarta",     // string
  "Name":          "Hotel Neo Mangga Dua Square", // string
  "Address":       "Jl. Mangga Dua Raya...",      // string
  "Rating":        8.4,           // float64 — guest score (0–10)
  "Stars":         0,             // int (reserved, always 0 currently)
  "Latitude":      -6.1374,       // float64
  "Longitude":     106.8249,      // float64
  "StartingPrice": 350000,        // float64 — fallback starting price
  "Currency":      "IDR",         // string — raw ISO 4217 currency code
  "ImageStyle":    "bg-gradient-to-br from-orange-600 to-yellow-500", // string — Tailwind gradient
  "BrandColor":    "#F97316"      // string — hex brand colour
}
```

**Example:**

```sh
curl "http://localhost:9011/api/hotels?city=Bali&brand=Aston"
```

```json
[
  {
    "HotelID": 207,
    "APIHotelID": { "Int64": 88, "Valid": true },
    "BrandID": 1,
    "BrandName": "Aston",
    "DBPrefix": "aston",
    "RegionName": "Bali",
    "Name": "Aston Kuta Hotel & Residence",
    "Address": "Jl. Wana Segara No.2, Kuta, Bali",
    "Rating": 8.7,
    "Stars": 0,
    "Latitude": -8.7182,
    "Longitude": 115.1706,
    "StartingPrice": 500000,
    "Currency": "IDR",
    "ImageStyle": "bg-gradient-to-br from-blue-800 to-sky-700",
    "BrandColor": "#1D4ED8"
  }
]
```

**Error response (HTTP 500):**

```json
{ "error": "central ping: dial tcp 127.0.0.1:3306: connect: connection refused" }
```

---

### `GET /api/brands` — Brand List

Returns a sorted list of all distinct brand names found in the hotel catalog. Brands with no active hotels are excluded.

**Query parameters:** None.

**Response:** `application/json` — array of brand objects.

**Brand object schema:**

```jsonc
{ "brand": "Aston" }   // string — brand display name
```

The list is sorted alphabetically.

**Example:**

```sh
curl http://localhost:9011/api/brands
```

```json
[
  { "brand": "Aston" },
  { "brand": "favehotels" },
  { "brand": "Four Corners" },
  { "brand": "Grand Aston" },
  { "brand": "Harper" },
  { "brand": "Hotel Neo" },
  { "brand": "Huxley" },
  { "brand": "Kamuela Villas" },
  { "brand": "Nordic" },
  { "brand": "Quest" },
  { "brand": "The Alana" }
]
```

**Error response (HTTP 500):**

```json
{ "error": "<db error message>" }
```

---

### `GET /api/regions` — Region List

Returns the list of all regions (cities/provinces) that have at least one active hotel.

**Query parameters:** None.

**Response:** `application/json` — array of region objects. The exact schema mirrors the `tb_region` columns returned by `Pool.ListRegions`.

**Example:**

```sh
curl http://localhost:9011/api/regions
```

```json
[
  { "region_id": 1, "region_name": "Jakarta" },
  { "region_id": 2, "region_name": "Bali" },
  { "region_id": 3, "region_name": "Surabaya" },
  { "region_id": 4, "region_name": "Bandung" }
]
```

**Error response (HTTP 500):**

```json
{ "error": "<db error message>" }
```

---

### `GET /health` — Health Check

Lightweight liveness and readiness probe. Pings the central database and all lazily-connected brand databases within a 5-second timeout.

**Query parameters:** None.

**Response:** `application/json`

**Response schema:**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `"ok"` when all DB pings succeed; `"degraded"` if any ping fails |
| `version` | string | Server version set at build time via `-ldflags` (falls back to `"dev"`) |
| `db` | bool | `true` when the central DB (and all connected brand DBs) are reachable; `false` otherwise |

**Example — healthy:**

```sh
curl http://localhost:9011/health
```

```json
{
  "status": "ok",
  "version": "1.2.0",
  "db": true
}
```

**Example — degraded (DB unreachable):**

```json
{
  "status": "degraded",
  "version": "1.2.0",
  "db": false
}
```

The health endpoint always returns **HTTP 200**, regardless of DB state. Use the `status` and `db` fields in your monitoring checks.

---

## Timeouts

| Endpoint | Server-side timeout |
|----------|-------------------|
| `POST /mcp` | Governed by MCP session / tool handler |
| `GET /mcp` | Governed by MCP session |
| `GET /dashboard` | No DB call; effectively instant |
| `GET /api/hotels` | 10 seconds |
| `GET /api/brands` | 5 seconds |
| `GET /api/regions` | 5 seconds |
| `GET /health` | 5 seconds |

---

## Error Handling

All REST endpoints (`/api/*`, `/health`) return `application/json` error bodies on failure:

```json
{ "error": "<message>" }
```

HTTP status codes used:

| Code | Meaning |
|------|---------|
| 200 | Success (also used by `/health` even when degraded) |
| 204 | OPTIONS preflight response |
| 500 | Internal server error (DB failure, query error) |

---

## Related

- MCP tools reference: [`docs/reference/tools.md`](tools.md) *(if present)*
- Architecture decisions: [`docs/adr/`](../adr/)
- Environment variables: see [`README.md`](../../README.md) or the project task sheet
