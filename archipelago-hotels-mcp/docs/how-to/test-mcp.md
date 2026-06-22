# Testing the Archipelago Hotels MCP Server

This guide covers all testing approaches for the `archipelago-hotels-mcp` server: build verification, MCP Inspector, transport-level testing, and HTTP endpoint checks.

---

## 1. Build Verification

Always verify the binary compiles cleanly before running any tests.

```bash
# From project root
go vet ./...
go build ./...

# Or use the Makefile target (builds UI + Go binary together)
make build
```

A clean `go vet` with no output and a zero-exit `go build` means the binary is ready.

---

## 2. Health Endpoint (HTTP mode only)

Start the server in HTTP mode, then confirm it is up before testing tools.

```bash
# Terminal 1 — start server
make dev-http
# or: go run ./cmd/archipelago-hotels-mcp --http

# Terminal 2 — health check
curl -s http://localhost:9011/health | jq .
```

Expected response:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "db": "connected"
}
```

If `db` is `"disconnected"`, verify `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, and `MYSQL_PASS` are set correctly.

---

## 3. MCP Inspector

[MCP Inspector](https://github.com/modelcontextprotocol/inspector) is the canonical interactive tool for exercising MCP servers. It requires Node.js 18+.

### 3a. Inspect the stdio transport

```bash
npx @modelcontextprotocol/inspector \
  go run ./cmd/archipelago-hotels-mcp
```

Inspector launches the binary as a child process over stdio and opens a browser UI at `http://localhost:5173`.

### 3b. Inspect the HTTP transport

Start the server first, then point Inspector at it:

```bash
# Terminal 1
make dev-http

# Terminal 2
npx @modelcontextprotocol/inspector \
  --transport http \
  --url http://localhost:9011/mcp
```

### 3c. Using Inspector interactively

1. Open `http://localhost:5173` in your browser.
2. Click **Tools** in the left sidebar — all four tools should appear.
3. Click **Resources** — `ui://hotel-dashboard` should be listed with MIME type `text/html;profile=mcp-app`.
4. Select a tool, fill in the input fields, and click **Run** to see the raw JSON response.

---

## 4. Testing the stdio Transport Directly

You can feed raw JSON-RPC to the server over stdin.

```bash
# initialize handshake
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}' \
  | go run ./cmd/archipelago-hotels-mcp
```

Expected: an `InitializeResult` containing `serverInfo`, `capabilities`, and the `tools` list.

---

## 5. Testing the HTTP Transport (curl)

All MCP calls go to `POST /mcp`. Include `Content-Type: application/json`.

### Initialize

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "curl-test", "version": "0.1"}
    }
  }' | jq .
```

### List Tools

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | jq .tools[].name
```

Expected output:

```
"search_hotels"
"recommend_hotel"
"find_hotels"
"get_hotel_detail"
```

---

## 6. Testing Each MCP Tool

### 6a. `search_hotels`

Searches by city, brand, or free-text query. Returns a hotel list with prices.

**Request:**

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 10,
    "method": "tools/call",
    "params": {
      "name": "search_hotels",
      "arguments": {
        "query": "Bali",
        "city": "Bali",
        "brand": ""
      }
    }
  }' | jq .
```

**Sample response:**

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Found 12 hotels in Bali.\n\n1. Aston Sunset Beach Resort - Gili Islands\n   Brand: Aston\n   City: Gili Islands, Bali\n   Starting from: Rp 850.000/night\n   ..."
      }
    ]
  }
}
```

**Variations to test:**

```bash
# By brand only
"arguments": {"query": "", "city": "", "brand": "favehotels"}

# By city + brand
"arguments": {"query": "", "city": "Jakarta", "brand": "neo"}

# Free-text query
"arguments": {"query": "beachfront pool", "city": "", "brand": ""}
```

---

### 6b. `recommend_hotel`

Ranks hotels by vibe, budget, or purpose.

**Request:**

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 11,
    "method": "tools/call",
    "params": {
      "name": "recommend_hotel",
      "arguments": {
        "purpose": "business",
        "budget": "mid-range",
        "city": "Surabaya"
      }
    }
  }' | jq .
```

**Sample response:**

```json
{
  "jsonrpc": "2.0",
  "id": 11,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Top recommendations for business travel in Surabaya (mid-range budget):\n\n1. Harper Pecenongan by ASTON\n   Score: 92/100\n   Why: Central location, business facilities, fast Wi-Fi\n   Rate: Rp 650.000/night\n   ..."
      }
    ]
  }
}
```

**Variations to test:**

```bash
# Leisure / luxury
"arguments": {"purpose": "leisure", "budget": "luxury", "city": "Lombok"}

# Family stay, budget
"arguments": {"purpose": "family", "budget": "budget", "city": "Yogyakarta"}
```

---

### 6c. `find_hotels`

Browses all hotels with optional city or brand filter. Used for listing / booking flows.

**Request — no filter:**

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 12,
    "method": "tools/call",
    "params": {
      "name": "find_hotels",
      "arguments": {}
    }
  }' | jq .
```

**Request — city filter:**

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 13,
    "method": "tools/call",
    "params": {
      "name": "find_hotels",
      "arguments": {"city": "Bandung"}
    }
  }' | jq .
```

**Sample response:**

```json
{
  "jsonrpc": "2.0",
  "id": 13,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Hotels in Bandung (8 results):\n\n- Quest Hotel Bandung\n- favehotel Braga Bandung\n- Hotel Neo Dipatiukur\n..."
      }
    ]
  }
}
```

---

### 6d. `get_hotel_detail` (app-only)

Returns full hotel detail including room types. Visibility is `app`; it is intended to be called by the embedded UI, not Claude directly.

> **Note:** Claude Desktop and most MCP clients will not surface this tool in the tool list because it is marked `visibility: app`. Use MCP Inspector or a direct JSON-RPC call to test it.

**Request:**

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 20,
    "method": "tools/call",
    "params": {
      "name": "get_hotel_detail",
      "arguments": {
        "hotel_id": "42"
      }
    }
  }' | jq .
```

**Sample response:**

```json
{
  "jsonrpc": "2.0",
  "id": 20,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"hotel_id\":42,\"name\":\"Aston Sunset Beach Resort - Gili Islands\",\"brand\":\"Aston\",\"city\":\"Gili Islands\",\"address\":\"Jl. Raya Gili Trawangan...\",\"rooms\":[{\"room_type\":\"Deluxe Room\",\"rate\":850000,\"currency\":\"IDR\"},{\"room_type\":\"Suite\",\"rate\":1500000,\"currency\":\"IDR\"}]}"
      }
    ]
  }
}
```

To find a valid `hotel_id`, run `find_hotels` or `search_hotels` first and note an ID from the results.

---

## 7. Verifying the UI Resource

The embedded TypeScript UI is exposed as an MCP resource.

### 7a. List resources

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":30,"method":"resources/list","params":{}}' | jq .
```

Expected: a resource entry with `uri: "ui://hotel-dashboard"` and `mimeType: "text/html;profile=mcp-app"`.

### 7b. Read the resource

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 31,
    "method": "resources/read",
    "params": {"uri": "ui://hotel-dashboard"}
  }' | jq '.result.contents[0].mimeType'
```

Expected output: `"text/html;profile=mcp-app"`

In MCP Inspector, click **Resources**, select `ui://hotel-dashboard`, and the HTML content should render in the preview panel. Clients that support the `ext-apps` protocol (e.g. Claude.ai desktop with MCP Apps enabled) will render it as an interactive panel.

---

## 8. Checking Image CDN Rewriting

The server rewrites thumbnail URLs through `url_image_resizer` to keep responses CSP-safe — no external fetches occur server-side.

### Verify the default CDN base

```bash
# Default base is https://images.archipelagohotels.com/
# Check a hotel thumbnail URL in search_hotels output:
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","id":40,"method":"tools/call",
    "params":{"name":"search_hotels","arguments":{"query":"Aston","city":"","brand":""}}
  }' | jq '.result.content[0].text' | grep -o 'https://images.archipelagohotels.com[^"]*' | head -5
```

All thumbnail URLs in the response should begin with `https://images.archipelagohotels.com/`.

### Override the CDN base

```bash
url_image_resizer=https://cdn.myproxy.example.com/ \
  go run ./cmd/archipelago-hotels-mcp --http &

curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","id":41,"method":"tools/call",
    "params":{"name":"search_hotels","arguments":{"query":"Harper","city":"","brand":""}}
  }' | jq '.result.content[0].text' | grep -o 'https://cdn.myproxy.example.com[^"]*' | head -3
```

URLs should now use the overridden base.

---

## 9. HTTP REST Endpoint Tests (curl)

These endpoints are available in HTTP mode independently of MCP.

### GET /api/hotels

```bash
# All hotels
curl -s "http://localhost:9011/api/hotels" | jq 'length'

# Filter by city
curl -s "http://localhost:9011/api/hotels?city=Jakarta" | jq '.[0]'

# Filter by brand
curl -s "http://localhost:9011/api/hotels?brand=fave" | jq '[.[].brand] | unique'
```

### GET /api/brands

```bash
curl -s "http://localhost:9011/api/brands" | jq .
```

Expected: an array of brand names such as `["Aston","favehotels","Harper","Hotel Neo",...]`.

### GET /api/regions

```bash
curl -s "http://localhost:9011/api/regions" | jq .
```

Expected: an array of region strings.

### GET /dashboard

```bash
curl -s "http://localhost:9011/dashboard" | head -5
```

Expected: an HTML document (the standalone dashboard page).

---

## 10. Rate Fallback Verification

The rate service tries three sources in order: SimpleBooking XML API → stored `room_rate` → `hotel_starting_price`. You can observe which path was taken by enabling debug logging.

```bash
DEBUG=1 go run ./cmd/archipelago-hotels-mcp --http 2>&1 | grep -i "rate\|simplebooking\|fallback"
```

Then trigger a rate lookup:

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","id":50,"method":"tools/call",
    "params":{"name":"search_hotels","arguments":{"query":"","city":"Bali","brand":""}}
  }' > /dev/null
```

Debug output will show which fallback tier was used for each hotel.

---

## 11. Environment Variable Reference for Testing

| Variable | Test value | Effect |
|---|---|---|
| `DEBUG` | `1` | Verbose SQL and rate-fetch logs |
| `MYSQL_HOST` | `127.0.0.1` | Point at local DB |
| `MYSQL_PORT` | `3306` | Default MySQL port |
| `MYSQL_USER` | `root` | DB user |
| `MYSQL_PASS` | *(empty)* | No password for local dev |
| `MYSQL_DB` | `db_archipelagowebsite` | Central catalog DB |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | CDN proxy base |

Set them inline for a single test run:

```bash
DEBUG=1 MYSQL_PASS=secret go run ./cmd/archipelago-hotels-mcp --http
```

---

## 12. Quick-Reference Checklist

```
[ ] go vet ./... passes with no output
[ ] go build ./... (or make build) succeeds
[ ] GET /health returns {"status":"ok","db":"connected"}
[ ] tools/list returns all 4 tools
[ ] resources/list returns ui://hotel-dashboard
[ ] search_hotels returns hotel results with prices
[ ] recommend_hotel ranks results with rationale
[ ] find_hotels returns full listing (optionally filtered)
[ ] get_hotel_detail returns rooms array for a valid hotel_id
[ ] Thumbnail URLs begin with images.archipelagohotels.com
[ ] GET /api/hotels, /api/brands, /api/regions all return JSON arrays
[ ] GET /dashboard returns HTML
[ ] MCP Inspector shows all tools + resource without errors
```
