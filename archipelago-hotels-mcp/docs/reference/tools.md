# MCP Tools Reference

Archipelago Hotels MCP server — tool reference for **archipelago-hotels-mcp**.

> **Stack**: Go 1.25 · `github.com/modelcontextprotocol/go-sdk` v1.6.1 · Transport: stdio (Claude Desktop) or Streamable HTTP on `:9011`

---

## Contents

- [Return value convention](#return-value-convention)
- [_meta.ui fields](#_metaui-fields)
- [Rate data sources](#rate-data-sources)
- [Shared types](#shared-types)
- [search_hotels](#search_hotels)
- [recommend_hotel](#recommend_hotel)
- [find_hotels](#find_hotels)
- [get_hotel_detail](#get_hotel_detail)
- [open_booking_url](#open_booking_url)
- [Error conditions](#error-conditions)

---

## Return value convention

All four tools use the **two-return structuredContent pattern** from `go-sdk`:

```go
func handler(...) (*mcp.CallToolResult, <ResponseType>, error)
```

- The **first return** (`*mcp.CallToolResult`) is always `nil`. The SDK serialises the second return value as the tool's structured content automatically.
- The **second return** is the typed response struct (or `map[string]any` for `get_hotel_detail`).
- Returning a non-nil `error` signals a tool-level failure to the MCP client; the second return value is still populated in some cases (e.g. `recommend_hotel` returns a human-readable `recommendation` string even when no hotels are found).

---

## _meta.ui fields

Every tool carries a `Meta` block that enables the MCP Apps UI extension:

| Field | Value | Purpose |
|-------|-------|---------|
| `ui.resourceUri` | `ui://hotel-dashboard` | Tells the MCP client which resource to open as the app frame |
| `ui.resourceDomains` | `["images.archipelagohotels.com"]` | CSP allowlist for thumbnail images |
| `ui.visibility` | `["app"]` | Present only on `get_hotel_detail`; hides the tool from Claude's tool list so only the dashboard UI can call it |

Public tools (`search_hotels`, `recommend_hotel`, `find_hotels`) do **not** include `visibility`, so they appear in the LLM's tool list normally.

---

## Rate data sources

`priceFrom` / `startingPrice` values in all responses are resolved through a three-level fallback chain. The `source` field on each room type in `get_hotel_detail` identifies which level was used:

| `source` value | Origin | Notes |
|----------------|--------|-------|
| `simplebooking` | SimpleBooking XML API (live) | `OTA_HotelAvailRQ`; 5-worker bounded pool; 5-min TTL cache; circuit breaker (5 failures → 120 s cooldown) |
| `stored` | `tb_hrooms.room_rate` in per-brand DB | Fallback when SB credentials missing or API unavailable |
| `starting_price` | `hotel_starting_price` in central DB | Last resort; single scalar, no room breakdown |

For list tools (`search_hotels`, `recommend_hotel`, `find_hotels`) the `priceFrom` field reflects the minimum rate resolved for that hotel via `BatchMinRates`, but the source is **not** surfaced in the response — it is logged server-side only.

---

## Shared types

### HotelSummary

Used by `search_hotels`, `recommend_hotel`, and `find_hotels` inside their `hotels` arrays.

```jsonc
{
  "id":         "string",   // numeric hotel ID as string, e.g. "1042"
  "name":       "string",   // full hotel name
  "brand":      "string",   // brand name, e.g. "Aston", "Harper"
  "city":       "string",   // region/city name from central DB
  "country":    "string",   // always "Indonesia" (or echoed from search params)
  "rating":     0.0,        // guest rating, 0.0–10.0
  "stars":      0,          // star classification, 0–5
  "priceFrom":  0.0,        // minimum nightly rate (see rate fallback chain)
  "currency":   "string",   // raw ISO currency code, e.g. "IDR", "USD"
  "imageStyle": "string",   // CSS gradient string for card background
  "brandColor": "string",   // hex brand accent colour, e.g. "#C8922A"
  "thumbnail":  "string",   // CDN URL rewritten via images.archipelagohotels.com; empty string if unavailable
  "tags":       ["string"]  // heuristic tags: "beach"|"city"|"resort"|"business"|"premium"
}
```

**Tag derivation rules** (from `deriveTags` in `internal/tools/search.go`):

- `beach` — region name contains: bali, kuta, seminyak, sanur, canggu, lombok
- `city` — all other regions (including Jakarta, Bandung, Surabaya, Medan, Makassar)
- `business` — hotel name contains "conference" or "convention"
- `resort` — hotel name contains "resort"
- `premium` — rating >= 8.5

---

## search_hotels

**Visibility**: public (appears in LLM tool list)

### Description

Priority tool for any hotel query. Searches all Archipelago Hotels & Resorts properties across Indonesia by city, brand, and/or free-text query, returning hotel cards with live pricing.

Trigger phrases: "hotels in Bali", "show hotels in Jakarta", "find hotel", "hotels near", "accommodation in", "where to stay", brand names (Aston, Harper, NEO, FAVE, Kamuela, Alana, Quest, PBA).

### Input schema

```jsonc
{
  "type": "object",
  "properties": {
    "city":    { "type": "string", "description": "City name, e.g. Jakarta, Bali, Yogyakarta." },
    "country": { "type": "string", "description": "Country filter. Defaults to Indonesia." },
    "brand":   { "type": "string", "description": "Brand filter, e.g. Aston, Harper, NEO." },
    "query":   { "type": "string", "description": "Free-text search." }
  }
  // no required fields — all parameters are optional
}
```

| Parameter | Type | Required | Default | Notes |
|-----------|------|----------|---------|-------|
| `city` | string | no | (empty — all cities) | Partial match against region name |
| `country` | string | no | `"Indonesia"` | Applied automatically if omitted |
| `brand` | string | no | (all brands) | Case-insensitive brand name match |
| `query` | string | no | (none) | Free-text match against hotel name and other fields |

Internal query limit: **50 hotels** per call.

### Response structure

```jsonc
{
  "hotels":   [ /* HotelSummary[] */ ],
  "total":    0,   // total rows in DB matching the query (before limit)
  "filtered": 0,   // number of hotels returned in this response
  "city":     "string",   // echoed city filter, or "all cities" if omitted
  "country":  "string"    // echoed country (always "Indonesia" unless overridden)
}
```

### Example request

```json
{
  "city": "Bali",
  "brand": "Aston"
}
```

### Example response (abbreviated)

```json
{
  "hotels": [
    {
      "id": "1042",
      "name": "Aston Kuta Hotel & Residence",
      "brand": "Aston",
      "city": "Kuta",
      "country": "Indonesia",
      "rating": 8.3,
      "stars": 4,
      "priceFrom": 850000,
      "currency": "IDR",
      "imageStyle": "linear-gradient(135deg, #1a3a5c 0%, #2e6da4 100%)",
      "brandColor": "#2e6da4",
      "thumbnail": "https://images.archipelagohotels.com/resize?url=...&w=400",
      "tags": ["beach", "resort"]
    }
  ],
  "total": 12,
  "filtered": 12,
  "city": "Bali",
  "country": "Indonesia"
}
```

### Error conditions

| Condition | Behaviour |
|-----------|-----------|
| No hotels found | Returns error: `"no hotels found for '<city>' in <country>"`. Response struct still populated with empty `hotels` array and `filtered: 0`. |
| DB unavailable | Returns error: `"search failed: <db error>"` |
| Internal panic | Recovered; returns error: `"internal error: <panic value>"` |

---

## recommend_hotel

**Visibility**: public (appears in LLM tool list)

### Description

Priority tool for hotel recommendations. Ranks Archipelago Hotels & Resorts by vibe, budget, and trip purpose, returning results sorted by a relevance score.

Trigger phrases: "recommend a hotel", "best hotel in Bali", "where should I stay", "suggest hotel for honeymoon/business/family", "budget hotel", "luxury resort", "romantic getaway".

### Input schema

```jsonc
{
  "type": "object",
  "properties": {
    "destination": { "type": "string", "description": "Destination city or area." },
    "vibe":        { "type": "string", "description": "Preferred atmosphere: luxury, romantic, business, family, culture, nature, budget." },
    "budget":      { "type": "string", "description": "Budget tier: budget, midscale, upscale, luxury." },
    "purpose":     { "type": "string", "description": "Trip purpose: leisure, business, honeymoon, family, solo." }
  },
  "required": ["destination"]
}
```

| Parameter | Type | Required | Values | Notes |
|-----------|------|----------|--------|-------|
| `destination` | string | **yes** | Any city / area name | Used as city search query; partial match |
| `vibe` | string | no | `luxury`, `romantic`, `business`, `family`, `culture`, `nature`, `budget`, `backpacker` | Drives scoring algorithm |
| `budget` | string | no | `budget`, `midscale`, `upscale`, `luxury` | Matched against resolved `priceFrom` (IDR thresholds: budget ≤500k, midscale 500k–1M, upscale 1M–2.5M, luxury >2.5M or stars≥5) |
| `purpose` | string | no | `leisure`, `business`, `honeymoon`, `family`, `solo` | Additional scoring signal |

Internal query limit: **50 candidates** (before scoring).

**Scoring algorithm** (additive, higher = better ranked):

| Condition | Points |
|-----------|--------|
| Price matches `budget` tier | +3 |
| Name/region matches `vibe` heuristics | +2 |
| Name/region matches `purpose` heuristics | +2 |
| Rating >= 8.5 | +2 |
| Rating >= 8.0 (but < 8.5) | +1 |

### Response structure

```jsonc
{
  "recommendation": "string",   // human-readable summary, e.g. "I found 8 hotels in Bali matching a 'romantic' vibe... Top pick: Kamuela Villas Ubud (Kamuela). romantic setting."
  "hotels":         [ /* HotelSummary[], sorted descending by score */ ],
  "destination":    "string",   // echoed input
  "vibe":           "string",   // echoed input
  "budget":         "string"    // echoed input
}
```

Note: `purpose` is **not** echoed in the response struct even though it influences scoring.

### Example request

```json
{
  "destination": "Bali",
  "vibe": "romantic",
  "budget": "upscale",
  "purpose": "honeymoon"
}
```

### Example response (abbreviated)

```json
{
  "recommendation": "I found 14 Archipelago hotels in Bali matching a 'romantic' vibe within 'upscale' budget for a honeymoon trip. Top pick: Kamuela Villas Seminyak (Kamuela). romantic setting, luxury experience.",
  "hotels": [
    {
      "id": "2105",
      "name": "Kamuela Villas Seminyak",
      "brand": "Kamuela",
      "city": "Seminyak",
      "country": "Indonesia",
      "rating": 9.1,
      "stars": 5,
      "priceFrom": 2100000,
      "currency": "IDR",
      "imageStyle": "linear-gradient(135deg, #5c3d1e 0%, #a0724a 100%)",
      "brandColor": "#a0724a",
      "thumbnail": "https://images.archipelagohotels.com/resize?url=...&w=400",
      "tags": ["beach", "resort", "premium"]
    }
  ],
  "destination": "Bali",
  "vibe": "romantic",
  "budget": "upscale"
}
```

### Error conditions

| Condition | Behaviour |
|-----------|-----------|
| No hotels found in destination | Returns error: `"no hotels found in <destination>"`. Response struct populated with `recommendation` suggesting alternative cities (Jakarta, Bali, Yogyakarta, Kuala Lumpur, Tokyo) and empty `hotels` array. |
| DB unavailable | Returns error: `"search failed: <db error>"` |
| Internal panic | Recovered; returns error: `"internal error: <panic value>"` |

---

## find_hotels

**Visibility**: public (appears in LLM tool list)

### Description

Priority tool for browsing or booking. Returns the full visual hotel portfolio with optional city/brand filtering. Intended for "show all hotels" or "open hotel list" requests and for users who name a brand without a specific search term.

Trigger phrases: "show all hotels", "browse hotels", "open hotel list", "book a hotel", "booking", "all Archipelago hotels", any brand name without a search context.

### Input schema

```jsonc
{
  "type": "object",
  "properties": {
    "city":  { "type": "string", "description": "City filter (optional)." },
    "brand": { "type": "string", "description": "Brand filter (optional)." }
  }
  // no required fields
}
```

| Parameter | Type | Required | Notes |
|-----------|------|----------|-------|
| `city` | string | no | Partial match against region name |
| `brand` | string | no | Case-insensitive brand name match |

Internal query limit: **200 hotels** per call (vs 50 for the other list tools).

### Response structure

```jsonc
{
  "filter":  "string",          // echoed city filter; empty string if not provided
  "hotels":  [ /* HotelSummary[] */ ],
  "total":   0,                 // same as match (count of returned hotels)
  "match":   0,                 // count of returned hotels
  "message": "string"           // e.g. "Showing 47 hotels in Jakarta"
}
```

Note: `total` and `match` always have the same value (both are set to `len(summaries)`). Unlike `search_hotels`, this tool does not return the DB-side `total` before the limit is applied.

### Example request

```json
{
  "brand": "Harper"
}
```

### Example response (abbreviated)

```json
{
  "filter": "",
  "hotels": [
    {
      "id": "3011",
      "name": "Harper Kuta",
      "brand": "Harper",
      "city": "Kuta",
      "country": "Indonesia",
      "rating": 8.6,
      "stars": 4,
      "priceFrom": 720000,
      "currency": "IDR",
      "imageStyle": "linear-gradient(135deg, #2c2c2c 0%, #555555 100%)",
      "brandColor": "#555555",
      "thumbnail": "https://images.archipelagohotels.com/resize?url=...&w=400",
      "tags": ["beach", "premium"]
    }
  ],
  "total": 18,
  "match": 18,
  "message": "Showing 18 hotels"
}
```

### Error conditions

| Condition | Behaviour |
|-----------|-----------|
| DB unavailable | Returns error: `"search failed: <db error>"` |
| No hotels match filters | Returns success with empty `hotels` array; `message` still set |
| Internal panic | Recovered; returns error: `"internal error: <panic value>"` |

---

## get_hotel_detail

**Visibility**: app-only (`visibility: ["app"]` — hidden from LLM tool list)

### Description

Returns full detail for a single hotel including all room types, amenities, location coordinates, and per-room pricing. Called exclusively by the dashboard UI when a user clicks a hotel card; not available to the LLM directly.

### Input schema

```jsonc
{
  "type": "object",
  "properties": {
    "hotelId": { "type": "string", "description": "The hotel ID (numeric string), e.g. \"1042\"." }
  },
  "required": ["hotelId"]
}
```

| Parameter | Type | Required | Notes |
|-----------|------|----------|-------|
| `hotelId` | string | **yes** | Numeric hotel ID as a string (parsed internally with `fmt.Sscanf`). Use the `id` field from any `HotelSummary`. |

### Response structure

The response is `map[string]any`. Fields always present:

```jsonc
{
  "id":         "string",    // numeric hotel ID as string
  "name":       "string",
  "brand":      "string",
  "city":       "string",    // region name
  "country":    "string",    // always "Indonesia"
  "address":    "string",    // street address
  "rating":     0.0,
  "stars":      0,
  "latitude":   0.0,
  "longitude":  0.0,
  "currency":   "string",    // raw ISO code, e.g. "IDR"
  "imageStyle": "string",
  "brandColor": "string",
  "thumbnail":  "string"
}
```

Additional fields present when room data is available (hotel has `APIHotelID` and `DBPrefix`):

```jsonc
{
  "startingPrice": 0.0,         // minimum rate across all room types
  "roomTypes": [
    {
      "name":          "string",  // room type name from source
      "pricePerNight": 0.0,       // rate after tax (SB: TotalAfterTax; stored: room_rate)
      "baseRate":      0.0,       // pre-tax / pre-discount rate; present for simplebooking only, 0.0 otherwise
      "currency":      "string",  // same as hotel currency
      "maxGuests":     2,         // always 2 (hardcoded)
      "rateSource":    "string"   // "simplebooking" | "stored" | "starting_price"
    }
  ]
}
```

When room data is **not** available but `hotel_starting_price > 0`:

```jsonc
{
  "startingPrice": 0.0   // from central DB; roomTypes key absent
}
```

`bookingUrl` field (present when the brand DB has a booking URL configured):

```jsonc
{
  "bookingUrl": "string"  // direct booking URL; empty string or absent when unavailable
}
```

`bookingUrl` is resolved by `GetBookingURL` in `internal/repository/hotel.go`. It reads `hotel_channel` from the brand DB (`SENTEC` → `hotel_sentec_booking` column, `SB` → `hotel_simplebooking` column). For brand DBs that lack `hotel_channel` (PBA), it falls back to `hotel_simplebooking` directly. The UI renders a "Book Now" button when this field is non-empty.

### Example request

```json
{
  "hotelId": "1042"
}
```

### Example response (abbreviated)

```json
{
  "id": "1042",
  "name": "Aston Kuta Hotel & Residence",
  "brand": "Aston",
  "city": "Kuta",
  "country": "Indonesia",
  "address": "Jl. Bentarayasa No.1, Kuta, Bali 80361",
  "rating": 8.3,
  "stars": 4,
  "latitude": -8.7215,
  "longitude": 115.1714,
  "currency": "IDR",
  "imageStyle": "linear-gradient(135deg, #1a3a5c 0%, #2e6da4 100%)",
  "brandColor": "#2e6da4",
  "thumbnail": "https://images.archipelagohotels.com/resize?url=...&w=400",
  "startingPrice": 850000,
  "roomTypes": [
    {
      "name": "Superior Room",
      "pricePerNight": 850000,
      "baseRate": 772727,
      "currency": "IDR",
      "maxGuests": 2,
      "rateSource": "simplebooking"
    },
    {
      "name": "Deluxe Room",
      "pricePerNight": 1050000,
      "baseRate": 954545,
      "currency": "IDR",
      "maxGuests": 2,
      "rateSource": "simplebooking"
    }
  ]
}
```

### Error conditions

| Condition | Behaviour |
|-----------|-----------|
| `hotelId` is not a valid integer | Returns error: `"invalid hotel ID '<value>': <parse error>"` |
| Hotel not found in DB | Returns error: `"hotel not found: <hotelId>"` |
| DB unavailable | Returns error: `"database error: <db error>"` |
| Internal panic | Recovered; returns error: `"internal error: <panic value>"` |

---

## open_booking_url

**Visibility**: app-only (`visibility: ["app"]` — hidden from LLM tool list)

### Description

Opens a hotel booking URL in the system browser. Called by the dashboard UI when the user clicks the "Book Now" button. Uses `exec.Command` in the Go server process to bypass the Electron iframe sandbox, which blocks all JavaScript-level browser opens (`window.open`, `location.href`, etc.).

### Input schema

```jsonc
{
  "type": "object",
  "properties": {
    "url": { "type": "string", "description": "The booking URL to open." }
  },
  "required": ["url"]
}
```

| Parameter | Type | Required | Notes |
|-----------|------|----------|-------|
| `url` | string | **yes** | Must be `http` or `https` scheme. Any other scheme returns an error without executing. |

### OS dispatch

| OS | Command |
|----|---------|
| macOS | `open <url>` |
| Linux | `xdg-open <url>` |
| Windows | `rundll32 url.dll,FileProtocolHandler <url>` |

### Response structure

```jsonc
{ "ok": true }
```

### Error conditions

| Condition | Behaviour |
|-----------|-----------|
| Non-http/https URL | Returns error: `"invalid URL"` — no exec call made |
| `cmd.Start()` failure | Returns error: `"open failed: <os error>"` |
| Unsupported OS | Returns error: `"unsupported OS: <runtime.GOOS>"` |

---

## Error conditions summary

All errors are returned as MCP tool errors (non-nil `error` return). The SDK wraps these into the standard MCP error response format. No tool returns HTTP-level errors — those are handled by the Gin layer in HTTP transport mode.

| Tool | Error string pattern | Cause |
|------|---------------------|-------|
| all | `"internal error: <v>"` | Recovered panic |
| `search_hotels` | `"search failed: <err>"` | DB query failure |
| `search_hotels` | `"no hotels found for '<city>' in <country>"` | Empty result set |
| `recommend_hotel` | `"search failed: <err>"` | DB query failure |
| `recommend_hotel` | `"no hotels found in <destination>"` | Empty result set |
| `find_hotels` | `"search failed: <err>"` | DB query failure |
| `get_hotel_detail` | `"invalid hotel ID '<id>': <err>"` | Non-integer `hotelId` |
| `get_hotel_detail` | `"hotel not found: <id>"` | No row in DB |
| `get_hotel_detail` | `"database error: <err>"` | DB query failure |
