# Tutorial: Adding a New MCP Tool

This tutorial walks through adding a brand-new MCP tool to the archipelago-hotels-mcp server from scratch. The concrete example is `search_by_rating` — a tool that returns Archipelago hotels with a guest rating at or above a caller-supplied minimum.

---

## GateGuard: Facts Required Before You Edit

Read every item below before touching a source file. Editing without this understanding is the most common source of regressions.

| # | Fact | Why it matters |
|---|------|----------------|
| 1 | **Module path is `github.com/msw/archipelago-hotels-mcp`** | Every import must use this prefix, not the directory name. |
| 2 | **MCP SDK is `github.com/modelcontextprotocol/go-sdk v1.6.1`** | `mcp.AddTool` is generic — the handler signature must match the `args` struct exactly. Do not use a different version. |
| 3 | **`hotelSummary` lives in `internal/tools/search.go`** | All tool handlers in the `tools` package share this type. Do not redeclare it. |
| 4 | **`resources.ResourceURI` is the MCP App resource URI** | Every tool that should render the hotel card UI must include the `"ui"` meta block verbatim. |
| 5 | **`deriveTags` is unexported but package-local** | Your handler can call `deriveTags(h)` because it is in the same `tools` package. |
| 6 | **`pool.SearchHotels` always returns `hotel_status = 1` rows only** | No need to re-filter for active hotels. |
| 7 | **`HotelRow.Rating` is already a `float64` from the DB** | The central DB column is `hotel_rating`. No string-to-float conversion required. |
| 8 | **`rateSvc.BatchMinRates` is bounded to 5 concurrent SB XML calls with a 5-minute TTL cache** | Call it once per handler invocation; never call it in a loop. |
| 9 | **Register the tool in `internal/server/server.go` inside `New()`** | The function is called once at startup. Order within `New()` is cosmetic. |
| 10 | **HTTP transport (`make dev-http`) and stdio transport (`make dev`) share the same `*mcp.Server`** | A tool registered in `New()` is available in both transports automatically. |

---

## Step 1 — Create the Handler File

Create `internal/tools/rating.go`. This file contains the registration function and the handler closure. Keep it self-contained.

```go
// internal/tools/rating.go
package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/archipelago-hotels-mcp/internal/rate"
	"github.com/msw/archipelago-hotels-mcp/internal/repository"
	"github.com/msw/archipelago-hotels-mcp/internal/resources"
)

// RegisterSearchByRating registers the search_by_rating MCP tool.
// The tool returns hotels whose guest rating is >= minRating, sorted
// highest-rated first.
func RegisterSearchByRating(s *mcp.Server, pool *repository.Pool, rateSvc *rate.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "search_by_rating",
		Description: "Find Archipelago hotels with a guest rating at or above a minimum score. " +
			"Useful when the caller explicitly wants high-rated properties: " +
			"'show me highly rated hotels in Bali', 'hotels above 8.5 stars in Jakarta', " +
			"'best reviewed Aston hotels'. Returns hotels sorted highest-rated first with live pricing.",
		// The "ui" meta block is required for any tool that should render
		// hotel cards in the MCP App UI. Keep the resourceDomains list
		// consistent with all other tools — it controls which image hostnames
		// the CSP allows.
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri":     resources.ResourceURI,
				"resourceDomains": []string{"images.archipelagohotels.com"},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{
					"type":        "string",
					"description": "City or region to search in (e.g. Bali, Jakarta, Yogyakarta).",
				},
				"brand": map[string]any{
					"type":        "string",
					"description": "Brand filter (e.g. Aston, Harper, NEO, FAVE, Kamuela).",
				},
				"min_rating": map[string]any{
					"type":        "number",
					"description": "Minimum guest rating on a 0–10 scale. Defaults to 8.0 if omitted.",
					"minimum":     0,
					"maximum":     10,
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of hotels to return (1–50, default 20).",
					"minimum":     1,
					"maximum":     50,
				},
			},
		},
	}, ratingHandler(pool, rateSvc))
}

// ratingArgs is the typed input the MCP SDK deserialises from the caller's
// JSON arguments. Field names must match the InputSchema property keys exactly.
type ratingArgs struct {
	City      string  `json:"city,omitempty"`
	Brand     string  `json:"brand,omitempty"`
	MinRating float64 `json:"min_rating,omitempty"`
	Limit     int     `json:"limit,omitempty"`
}

// ratingResult is the structured output serialised into the tool result.
// It reuses hotelSummary (defined in search.go) so the UI renders identical
// hotel cards regardless of which tool produced the data.
type ratingResult struct {
	Hotels    []hotelSummary `json:"hotels"`
	Total     int            `json:"total"`   // hotels returned after rating filter
	Scanned   int            `json:"scanned"` // hotels examined before filter
	MinRating float64        `json:"minRating"`
	City      string         `json:"city"`
}

// ratingHandler returns the closure that the MCP SDK calls on each invocation.
//
// Handler signature rules (enforced by mcp.AddTool generics):
//   - First return value must be *mcp.CallToolResult (nil → SDK builds it from the second value).
//   - Second return value is the structured result (any JSON-serialisable type).
//   - Third return value is error (non-nil causes the SDK to return a tool error to the caller).
func ratingHandler(pool *repository.Pool, rateSvc *rate.Service) func(
	context.Context, *mcp.CallToolRequest, ratingArgs,
) (*mcp.CallToolResult, ratingResult, error) {

	return func(ctx context.Context, _ *mcp.CallToolRequest, args ratingArgs) (
		_ *mcp.CallToolResult, out ratingResult, err error,
	) {
		// Recover from any unexpected panics so the server stays alive.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("search_by_rating panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()

		// Apply defaults.
		minRating := args.MinRating
		if minRating <= 0 {
			minRating = 8.0
		}

		limit := args.Limit
		if limit <= 0 || limit > 50 {
			limit = 20
		}

		// Fetch candidates from the central DB.
		// Fetch more than needed (up to 200) so the in-process rating filter
		// has a reasonable candidate pool even for niche city+brand combos.
		candidates, _, err := pool.SearchHotels(ctx, repository.SearchParams{
			City:  args.City,
			Brand: args.Brand,
			Limit: 200,
		})
		if err != nil {
			return nil, ratingResult{}, fmt.Errorf("search failed: %w", err)
		}

		// Filter by rating in process — no SQL round-trip needed because
		// hotel_rating is already loaded in HotelRow.
		var filtered []repository.HotelRow
		for _, h := range candidates {
			if h.Rating >= minRating {
				filtered = append(filtered, h)
			}
		}

		if len(filtered) == 0 {
			city := args.City
			if city == "" {
				city = "all cities"
			}
			return nil, ratingResult{
				Hotels:    nil,
				Total:     0,
				Scanned:   len(candidates),
				MinRating: minRating,
				City:      city,
			}, fmt.Errorf(
				"no hotels found with rating >= %.1f in %s (checked %d hotels)",
				minRating, city, len(candidates),
			)
		}

		// Sort by rating descending so the top result is always the best-rated.
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Rating > filtered[j].Rating
		})

		// Cap to the caller's requested limit.
		if len(filtered) > limit {
			filtered = filtered[:limit]
		}

		// Fetch live rates and thumbnails in parallel for the shortlisted hotels.
		// BatchMinRates is bounded and cached — always call it once per handler.
		rateMap := rateSvc.BatchMinRates(ctx, filtered)
		thumbMap := pool.GetThumbnails(ctx, filtered)

		summaries := make([]hotelSummary, 0, len(filtered))
		for _, h := range filtered {
			priceFrom := h.StartingPrice
			if m, ok := rateMap[h.HotelID]; ok && m > 0 {
				priceFrom = m
			}
			summaries = append(summaries, hotelSummary{
				ID:         fmt.Sprintf("%d", h.HotelID),
				Name:       h.Name,
				Brand:      h.BrandName,
				City:       h.RegionName,
				Country:    "Indonesia",
				Rating:     h.Rating,
				Stars:      h.Stars,
				PriceFrom:  priceFrom,
				Currency:   h.Currency,
				ImageStyle: h.ImageStyle,
				BrandColor: h.BrandColor,
				Thumbnail:  thumbMap[h.HotelID],
				Tags:       deriveTags(h), // package-local helper in search.go
			})
		}

		city := args.City
		if city == "" {
			city = "all cities"
		}

		return nil, ratingResult{
			Hotels:    summaries,
			Total:     len(summaries),
			Scanned:   len(candidates),
			MinRating: minRating,
			City:      city,
		}, nil
	}
}
```

### What each section does

| Section | Purpose |
|---------|---------|
| `RegisterSearchByRating` | Declares the tool to the SDK once at startup. The `InputSchema` map is JSON Schema — property names here must match the `ratingArgs` json tags exactly. |
| `ratingArgs` | The SDK deserialises the caller's JSON `arguments` object into this struct. Omit a field and its zero value applies. |
| `ratingResult` | Serialised into the tool result. Using the shared `hotelSummary` type means the UI renders hotel cards without any frontend change. |
| `ratingHandler` | The closed-over handler. Fetches candidates, filters in process (avoids a second DB query), sorts, caps, then enriches with live rates and thumbnails. |

---

## Step 2 — Register the Tool in `internal/server/server.go`

Open `internal/server/server.go` and add one line inside `New()`, alongside the existing registration calls.

**Before:**

```go
	tools.RegisterSearch(s, svc.DB, svc.RateSvc)
	tools.RegisterDetail(s, svc.DB, svc.RateSvc)
	tools.RegisterRecommend(s, svc.DB, svc.RateSvc)
	tools.RegisterDashboardTool(s, svc.DB, svc.RateSvc)
```

**After:**

```go
	tools.RegisterSearch(s, svc.DB, svc.RateSvc)
	tools.RegisterDetail(s, svc.DB, svc.RateSvc)
	tools.RegisterRecommend(s, svc.DB, svc.RateSvc)
	tools.RegisterDashboardTool(s, svc.DB, svc.RateSvc)
	tools.RegisterSearchByRating(s, svc.DB, svc.RateSvc)  // ← add this line
```

Also update the `Instructions` string in `mcp.NewServer` so the LLM's system prompt knows the tool exists:

```go
		Instructions: `# Archipelago Hotels MCP Server
...
5. search_by_rating: Find hotels at or above a minimum guest rating
`,
```

No other changes are required in `server.go`. The tool is automatically available in both stdio and HTTP transports because they share the same `*mcp.Server` instance.

---

## Step 3 — Link to the UI Resource

The `"ui"` meta block in `RegisterSearchByRating` is what causes the MCP App UI to launch and render hotel cards when the tool is called from Claude Desktop.

```go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     resources.ResourceURI,
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

**`resourceUri`** — the MCP resource identifier (`ui://hotel-dashboard`) declared in `internal/resources/dashboard.go`. The client fetches this URI to get the HTML/JS app.

**`resourceDomains`** — the CSP allowlist. The browser blocks requests to any domain not listed here. The image CDN proxy (`images.archipelagohotels.com`) must be present for thumbnails to render. Do not add external domains unless you own them and understand the CSP implications.

The UI already handles any tool that returns a `hotels` array inside its result object — no frontend changes are needed for `search_by_rating` because `ratingResult.Hotels` is `[]hotelSummary`, the same shape `search_hotels` produces.

---

## Step 4 — Update `mcp-app.ts` (Only If You Add New Input Fields)

The TypeScript frontend in `ui/src/mcp-app.ts` defines a `ToolInputParams` discriminated union that maps tool names to their argument types. You only need to touch this file if you add an input that the UI will call programmatically (for example, a filter button that triggers `search_by_rating` with a `min_rating` slider value).

For `search_by_rating`, since the tool is invoked by the LLM from natural language and the UI already handles the `hotels` result array, **no change to `mcp-app.ts` is required** unless you want to add a dedicated UI panel for it.

If you do add a UI control in the future, the pattern to follow is:

```typescript
// Inside the ToolInputParams type union in mcp-app.ts:
| { tool: 'search_by_rating'; city?: string; brand?: string; min_rating?: number; limit?: number }
```

Then rebuild the UI:

```bash
make build-ui
```

The embedded HTML is regenerated automatically — `go build` embeds the new file via `//go:embed`.

---

## Step 5 — Build and Test

### Build

```bash
# From the project root
make build
```

This runs `make build-ui` (Vite) then `make build-go`. The binary is written to `bin/archipelago-hotels-mcp`.

If you see a compile error like `undefined: hotelSummary`, check that your file is in `package tools` and that you have not redeclared the type.

### Run in HTTP mode (recommended for manual testing)

```bash
make dev-http
# Server listens on :9011
```

### Test prompts

Send these prompts to Claude Desktop (or any MCP client pointed at the server):

```
Show me highly rated hotels in Bali
```

```
Find Aston hotels with rating above 9
```

```
Best reviewed hotels in Jakarta, limit 5
```

```
Hotels with rating above 8.5 in Yogyakarta for a family trip
```

Expected response: hotel cards rendered in the MCP App UI, sorted highest-rated first, each showing name, rating, price, and thumbnail.

### Test the error path

```
Highly rated hotels in Timbuktu
```

Expected: The tool returns an error message ("no hotels found with rating >= 8.0 in Timbuktu") and Claude surfaces it gracefully. The server must not crash.

### Verify tool listing

```bash
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | \
  python3 -m json.tool | grep '"name"'
```

`search_by_rating` must appear in the output alongside the existing four tools.

---

## Step 6 — Checklist Before Opening a PR

Go through every item before pushing.

### Code quality

- [ ] File is in `package tools` and imports use the full module path `github.com/msw/archipelago-hotels-mcp/...`
- [ ] `ratingArgs` json tags match `InputSchema` property keys exactly (case-sensitive)
- [ ] `defer recover()` is present in the handler to prevent panics from crashing the server
- [ ] `BatchMinRates` and `GetThumbnails` are each called exactly once per handler invocation
- [ ] No new type declarations that duplicate `hotelSummary` or `deriveTags`
- [ ] `RegisterSearchByRating` is called inside `server.New()` — not in `main.go` or anywhere else

### Tool definition

- [ ] `Name` is snake_case, globally unique among all registered tools
- [ ] `Description` starts with a trigger sentence the LLM can match against user intent
- [ ] `InputSchema` specifies `"minimum"` / `"maximum"` constraints for numeric fields
- [ ] `Meta` `"ui"` block is present and `resourceDomains` includes `"images.archipelagohotels.com"`
- [ ] Server `Instructions` string updated to list the new tool

### Build

- [ ] `make build` exits 0 with no warnings
- [ ] `go vet ./...` exits 0
- [ ] Binary size increase is reasonable (no accidental large dependency added)

### Runtime

- [ ] Tool appears in `tools/list` response
- [ ] Happy path returns hotel cards in the MCP App UI, sorted by rating descending
- [ ] `min_rating` default (0 → 8.0) works when the argument is omitted
- [ ] `limit` default (0 → 20) and cap (> 50 → 20) both work
- [ ] Error path (no matching hotels) returns a clear message and does not panic
- [ ] Server remains healthy (`GET /health`) after running all test prompts

### Documentation

- [ ] This tutorial reviewed for any steps that no longer apply after your change
- [ ] `SESSION_HANDOFF.md` or equivalent updated if you changed shared types or the DB query pattern

---

## Reference: Anatomy of a Tool File

```
internal/tools/rating.go
│
├── RegisterXxx(s, pool, rateSvc)   ← called once in server.New()
│     ├── mcp.AddTool(s, &mcp.Tool{...}, handler)
│     │     ├── Name          snake_case, unique
│     │     ├── Description   LLM-readable trigger sentence first
│     │     ├── Meta          "ui" block → resourceUri + resourceDomains
│     │     └── InputSchema   JSON Schema, keys match Args struct tags
│     └── handler closure
│
├── type xxxArgs struct            ← SDK deserialises caller JSON into this
│     └── json tags must match InputSchema property names
│
├── type xxxResult struct          ← SDK serialises this into the tool result
│     └── reuse hotelSummary for hotel data; UI renders it without changes
│
└── func xxxHandler(pool, rateSvc) ← returns the closure
      ├── defer recover()          ← required in every handler
      ├── fetch candidates         ← pool.SearchHotels
      ├── filter / score / sort    ← in-process logic
      ├── enrich                   ← rateSvc.BatchMinRates + pool.GetThumbnails
      └── return nil, result, nil  ← first return nil → SDK builds CallToolResult
```

The first return value (`*mcp.CallToolResult`) is `nil` in all existing handlers because the SDK builds the result automatically from the second return value. Only return a non-nil `*mcp.CallToolResult` if you need to override the raw MCP response (rare).
