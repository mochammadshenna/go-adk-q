# Tools API Reference

> **archipelago-hotels-mcp** — MCP tool specification  
> Stack: Go 1.25 · `github.com/modelcontextprotocol/go-sdk` v1.6.1 · Transport: stdio (Claude Desktop) or Streamable HTTP `:9011`

---

## Tool Summary

| Tool | Visibility | Required params | Response type | Shown in UI |
|------|-----------|-----------------|---------------|-------------|
| [`search_hotels`](#search_hotels) | Claude + app | none (all optional) | `searchResult` | yes |
| [`recommend_hotel`](#recommend_hotel) | Claude + app | `destination` | `recommendResult` | yes |
| [`find_hotels`](#find_hotels) | Claude + app | none (all optional) | `dashboardData` | yes |
| [`get_hotel_detail`](#get_hotel_detail) | app only | `hotelId` | `map[string]any` | yes (detail panel) |

**Visibility** describes who can call the tool:

- **Claude + app** — listed in the LLM's tool manifest and callable by both Claude and the embedded UI.
- **app only** — `visibility: ["app"]` is set in the tool's `_meta.ui` block; the tool is hidden from Claude's tool list and is callable only by the dashboard UI resource.

---

## Shared type: HotelSummary

All three list tools (`search_hotels`, `recommend_hotel`, `find_hotels`) embed arrays of `HotelSummary` objects.

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Numeric hotel ID serialised as a string, e.g. `"1042"` |
| `name` | string | Full hotel display name |
| `brand` | string | Brand name, e.g. `"Aston"`, `"Harper"`, `"NEO"` |
| `city` | string | Region/city name from central DB (`tb_region.region_name`) |
| `country` | string | Country string; `"Indonesia"` for all current properties |
| `rating` | number | Guest rating, 0.0–10.0 |
| `stars` | integer | Star classification, 0–5 |
| `priceFrom` | number | Minimum nightly rate resolved via the rate fallback chain (see [Rate sources](#rate-sources)) |
| `currency` | string | Raw ISO 4217 code, e.g. `"IDR"`, `"USD"` |
| `imageStyle` | string | CSS gradient string for card background, e.g. `"linear-gradient(135deg, #1a3a5c 0%, #2e6da4 100%)"` |
| `brandColor` | string | Hex brand accent colour, e.g. `"#C8922A"` (empty string if unset) |
| `thumbnail` | string | CDN URL rewritten through `images.archipelagohotels.com`; empty string when unavailable |
| `tags` | string[] | Heuristic tags: `"beach"`, `"city"`, `"resort"`, `"business"`, `"premium"` |

**Tag derivation rules** (applied in `internal/tools/search.go › deriveTags`):

| Tag | Condition |
|-----|-----------|
| `beach` | Region name contains: bali, kuta, seminyak, sanur, canggu, lombok |
| `city` | All other regions (default) |
| `business` | Hotel name contains "conference" or "convention" |
| `resort` | Hotel name contains "resort" |
| `premium` | Rating >= 8.5 |

---

## Rate sources

`priceFrom` in list tools and `startingPrice` / `roomTypes[].rateSource` in `get_hotel_detail` are resolved through a three-level fallback chain. Rates are cached for 5 minutes (TTL). The SimpleBooking client uses a circuit breaker: 5 consecutive failures open the breaker for 120 seconds.

| `rateSource` value | Origin | Notes |
|--------------------|--------|-------|
| `simplebooking` | SimpleBooking XML API (`OTA_HotelAvailRQ`) — live | 5-worker bounded pool; `TotalAfterTax` used as `pricePerNight`; `AmountAfterTax` exposed as `baseRate` |
| `stored` | `tb_hrooms.room_rate` in per-brand DB | Fallback when SB credentials are missing or the API is unavailable |
| `starting_price` | `hotel_starting_price` in central DB | Last resort; single scalar price, no room-type breakdown |

For list tools, the `source` is not surfaced in the response — it is resolved internally by `BatchMinRates` and logged server-side only.

---

## search_hotels

**Visibility**: Claude + app

### Description

Priority tool for any hotel query. Searches all Archipelago Hotels & Resorts properties across Indonesia, returning hotel cards with live pricing. Claude should call this tool first whenever the user mentions hotels, accommodation, resorts, or specific brand names (Aston, Harper, NEO, FAVE, Kamuela, Alana, Quest, PBA).

### Input schema

All parameters are optional strings. The tool accepts no required fields.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `city` | string | no | all cities | City name, e.g. `"Jakarta"`, `"Bali"`, `"Yogyakarta"`. Partial match against `tb_region.region_name`. |
| `country` | string | no | `"Indonesia"` | Country filter. Applied automatically when omitted. |
| `brand` | string | no | all brands | Brand name filter, e.g. `"Aston"`, `"Harper"`, `"NEO"`. Case-insensitive. |
| `query` | string | no | none | Free-text search matched against hotel name and related fields. |

Internal result limit: **50 hotels** per call.

### Response: `searchResult`

| Field | Type | Description |
|-------|------|-------------|
| `hotels` | HotelSummary[] | Matched hotels with resolved rates and thumbnails |
| `total` | integer | Total rows matching the query in the DB (before the 50-hotel limit) |
| `filtered` | integer | Number of hotels returned in `hotels` |
| `city` | string | Echoed city filter; `"all cities"` when `city` was not provided |
| `country` | string | Echoed country filter; always `"Indonesia"` unless overridden |

### Example call

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

| Condition | Error message |
|-----------|--------------|
| No hotels found | `"no hotels found for '<city>' in <country>"` — response struct still populated with empty `hotels` and `filtered: 0` |
| DB unavailable | `"search failed: <db error>"` |
| Internal panic | `"internal error: <panic value>"` |

---

## recommend_hotel

**Visibility**: Claude + app

### Description

Priority tool for hotel recommendations. Ranks Archipelago Hotels & Resorts properties by vibe, budget, and trip purpose using an additive scoring heuristic. Returns results sorted by score, with the top pick named in a human-readable summary string.

Claude should call this tool first when the user asks for suggestions, best picks, or travel advice ("recommend a hotel", "best hotel in Bali", "suggest hotel for honeymoon/business/family", "luxury resort", "romantic getaway").

### Input schema

| Parameter | Type | Required | Values | Description |
|-----------|------|----------|--------|-------------|
| `destination` | string | **yes** | Any city/area name | Used as city search query; partial match against region name |
| `vibe` | string | no | `luxury`, `romantic`, `business`, `family`, `culture`, `nature`, `budget`, `backpacker` | Preferred atmosphere; drives scoring algorithm |
| `budget` | string | no | `budget`, `midscale`, `upscale`, `luxury` | Budget tier matched against resolved `priceFrom` in IDR |
| `purpose` | string | no | `leisure`, `business`, `honeymoon`, `family`, `solo` | Trip purpose; provides additional scoring signal |

**Budget tier thresholds (IDR):**

| Tier | Condition |
|------|-----------|
| `budget` | `priceFrom` > 0 and <= 500,000 |
| `midscale` | `priceFrom` > 500,000 and <= 1,000,000 |
| `upscale` | `priceFrom` > 1,000,000 and <= 2,500,000 |
| `luxury` | `priceFrom` > 2,500,000 or `stars` >= 5 |

**Scoring algorithm** (additive; candidates are sorted descending by total score):

| Condition | Points |
|-----------|--------|
| Price falls within the requested `budget` tier | +3 |
| Hotel name or region matches `vibe` heuristics (e.g. name contains "villa" or region is Bali for `romantic`) | +2 |
| Hotel name or region matches `purpose` heuristics (e.g. name contains "villa" or rating >= 8.5 for `honeymoon`) | +2 |
| Rating >= 8.5 | +2 |
| Rating >= 8.0 (and < 8.5) | +1 |

Internal candidate limit: **50 hotels** fetched before scoring.

### Response: `recommendResult`

| Field | Type | Description |
|-------|------|-------------|
| `recommendation` | string | Human-readable summary naming the number of hotels found and the top pick, e.g. `"I found 14 Archipelago hotels in Bali matching a 'romantic' vibe... Top pick: Kamuela Villas Seminyak (Kamuela). romantic setting."` |
| `hotels` | HotelSummary[] | All candidates sorted descending by score |
| `destination` | string | Echoed input |
| `vibe` | string | Echoed input |
| `budget` | string | Echoed input |

Note: `purpose` influences scoring but is not echoed in the response struct.

### Example call

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

| Condition | Error message |
|-----------|--------------|
| No hotels found in destination | `"no hotels found in <destination>"` — response struct populated with `recommendation` suggesting alternatives (Jakarta, Bali, Yogyakarta, Kuala Lumpur, Tokyo) and empty `hotels` |
| DB unavailable | `"search failed: <db error>"` |
| Internal panic | `"internal error: <panic value>"` |

---

## find_hotels

**Visibility**: Claude + app

> Previously registered under the name `hotel_booking`. The current registered name is `find_hotels`.

### Description

Priority tool for browsing or booking Archipelago Hotels. Returns the full hotel portfolio with optional city/brand filtering. Intended for "show all hotels", "browse hotels", "open hotel list", "book a hotel", or any request that names a brand without a specific search query.

Unlike `search_hotels` (limit 50), this tool returns up to **200 hotels** per call, making it suitable as the entry point for the full dashboard view.

### Input schema

All parameters are optional strings.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `city` | string | no | City filter; partial match against region name |
| `brand` | string | no | Brand filter; case-insensitive name match |

### Response: `dashboardData`

| Field | Type | Description |
|-------|------|-------------|
| `filter` | string | Echoed `city` input; empty string when not provided |
| `hotels` | HotelSummary[] | Matching hotels up to the 200-hotel limit |
| `total` | integer | Count of returned hotels (same as `match`) |
| `match` | integer | Count of returned hotels (same as `total`) |
| `message` | string | Human-readable summary, e.g. `"Showing 47 hotels in Jakarta"` or `"Showing 279 hotels"` |

Note: `total` and `match` always carry the same value. Unlike `search_hotels`, this tool does not return the DB-level count before the limit is applied.

### Example call

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

| Condition | Error message |
|-----------|--------------|
| DB unavailable | `"search failed: <db error>"` |
| No hotels match filters | Returns success with empty `hotels`; `message` still set |
| Internal panic | `"internal error: <panic value>"` |

---

## get_hotel_detail

**Visibility**: app only (hidden from Claude's tool list)

### Description

Returns full detail for a single hotel including room types, per-room pricing, location coordinates, and street address. Called exclusively by the dashboard UI when a user clicks a hotel card. The tool is registered with `visibility: ["app"]`, which hides it from the LLM's tool manifest — Claude cannot call it directly.

### Input schema

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `hotelId` | string | **yes** | Numeric hotel ID as a string, e.g. `"1042"`. Use the `id` field from any `HotelSummary`. The value is parsed as an integer internally (`fmt.Sscanf`). |

### Response

The response is `map[string]any`. Fields present in every response:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Numeric hotel ID as string |
| `name` | string | Full hotel name |
| `brand` | string | Brand name |
| `city` | string | Region name |
| `country` | string | Always `"Indonesia"` |
| `address` | string | Street address |
| `rating` | number | Guest rating, 0.0–10.0 |
| `stars` | integer | Star classification, 0–5 |
| `latitude` | number | Decimal latitude |
| `longitude` | number | Decimal longitude |
| `currency` | string | Raw ISO 4217 code |
| `imageStyle` | string | CSS gradient string |
| `brandColor` | string | Hex brand accent colour |
| `thumbnail` | string | CDN thumbnail URL |

Additional fields when room rate data is available (hotel has a valid `api_hotel_id` and `db_prefix`):

| Field | Type | Description |
|-------|------|-------------|
| `startingPrice` | number | Minimum rate across all room types |
| `roomTypes` | object[] | Array of room type objects (see below) |

**`roomTypes` object fields:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Room type name from the rate source |
| `pricePerNight` | number | Rate after tax (`TotalAfterTax` from SB; `room_rate` from stored) |
| `baseRate` | number | Pre-tax / pre-discount rate; populated for `simplebooking` source only, `0.0` otherwise |
| `currency` | string | Same as hotel currency |
| `maxGuests` | integer | Always `2` (hardcoded) |
| `rateSource` | string | `"simplebooking"`, `"stored"`, or `"starting_price"` |

When no room-type data is available but `hotel_starting_price > 0` in the central DB, only `startingPrice` is added; `roomTypes` is absent.

### Example call

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

| Condition | Error message |
|-----------|--------------|
| `hotelId` is not a valid integer | `"invalid hotel ID '<value>': <parse error>"` |
| Hotel not found in DB | `"hotel not found: <hotelId>"` |
| DB unavailable | `"database error: <db error>"` |
| Internal panic | `"internal error: <panic value>"` |

---

## Error handling

All tool errors are returned as non-nil `error` values. The MCP SDK wraps these into the standard MCP error response. No tool returns HTTP-level errors — those are handled by the Gin layer when running in HTTP transport mode.

| Tool | Error pattern | Cause |
|------|--------------|-------|
| all | `"internal error: <v>"` | Recovered panic |
| `search_hotels` | `"search failed: <err>"` | DB query failure |
| `search_hotels` | `"no hotels found for '<city>' in <country>"` | Empty result set |
| `recommend_hotel` | `"search failed: <err>"` | DB query failure |
| `recommend_hotel` | `"no hotels found in <destination>"` | Empty result set |
| `find_hotels` | `"search failed: <err>"` | DB query failure |
| `get_hotel_detail` | `"invalid hotel ID '<id>': <err>"` | Non-integer `hotelId` |
| `get_hotel_detail` | `"hotel not found: <id>"` | No matching row in DB |
| `get_hotel_detail` | `"database error: <err>"` | DB query failure |
