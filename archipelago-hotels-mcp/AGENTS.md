# AGENTS.md — AI Agent Guide for archipelago-hotels-mcp

Reference document for AI coding assistants (Claude Code, GitHub Copilot, Cursor, Windsurf, etc.). Read this before making any changes.

---

## 1. Project Overview

`archipelago-hotels-mcp` is a Go MCP (Model Context Protocol) server — a Sentec Tech product — that gives AI assistants structured access to Archipelago Hotels & Resorts inventory: 279+ properties across 13 brands. It supports two transports: **stdio** (Claude Desktop, launched as a child process) and **Streamable HTTP** via Gin on `:9011` (web-based MCP clients). Four MCP tools expose search, recommendation, browsing, and hotel detail. An embedded Vite-built TypeScript UI is registered as an MCP App resource (`ui://hotel-dashboard`) and rendered inside Claude Desktop. Pricing comes from a three-tier fallback chain: SimpleBooking XML API → per-brand stored rates → central starting price. Hotel data lives in a central MySQL database (`db_archipelagowebsite`) plus 8 per-brand databases that are lazily connected.

**Module**: `github.com/msw/archipelago-hotels-mcp`
**Go**: 1.25 | **go-sdk**: `modelcontextprotocol/go-sdk v1.6.1` | **HTTP**: `gin v1.11.0` | **DB**: `go-sql-driver/mysql v1.10`

---

## 2. Build Commands

| Command | Effect | Use when |
|---|---|---|
| `make build` | `build-ui` then `build-go` — full rebuild | Any change to UI or Go source |
| `make build-ui` | `npm install` + Vite compile → `internal/resources/mcp-app.html` | UI-only changes |
| `make build-go` | Go binary → `bin/archipelago-hotels-mcp` | Go-only changes, UI already built |
| `make build-fast` | Go binary only, skips UI (alias for `build-go`) | Fast iteration on Go when UI is unchanged |
| `make lint` | `go vet ./...` | Before committing |
| `make dev-http` | `build-fast` then run HTTP mode with debug on `:9011` | Local HTTP testing |
| `make run-stdio` | `build-fast` then run stdio transport | Local stdio testing |
| `make install` | `go install ./cmd/archipelago-hotels-mcp` | Install to `$GOPATH/bin` |
| `make clean` | Remove `bin/`, `ui/dist/`, `ui/node_modules/`, Go cache | Clean slate |

**UI change workflow:** edit `ui/src/mcp-app.ts` → `make build` (rebuilds both Vite and Go binary).

**UI embed pipeline:**
```
ui/src/mcp-app.ts
    ↓  vite build (npm run build)
ui/dist/index.html          (self-contained: all JS/CSS inlined)
    ↓  cp (Makefile)
internal/resources/mcp-app.html
    ↓  //go:embed (resources/dashboard.go)
bin/archipelago-hotels-mcp  (embedded at compile time)
```

`internal/resources/mcp-app.html` must exist before `go build`. Never delete it without running `make build-ui` first.

---

## 3. Key Architecture Facts Agents Must Know

### Mandatory Hooks (never bypass)

**GateGuard — before editing any Go file, present these facts in your response:**
1. Which other files import the file you are about to modify.
2. Which functions you are changing and what calls them.
3. The user's instruction verbatim (quote it exactly).

**Read-before-edit — call `Read` on any file before calling `Edit` or `Write` on it.** Never edit blind.

### Two Separate Builds

UI and Go binary are independent artifacts.
- Editing `mcp-app.ts` without running `make build-ui` leaves the embedded HTML stale.
- Running `make build-fast` after a UI edit silently ships the old UI.
- Always run `make build` (or `make build-ui && make build-go`) after any UI change.

### Pool.brandDBs — Lazy Connect, Nil-Safe

- `Pool.central` connects to `db_archipelagowebsite` (catalog, brands, regions) at startup.
- `Pool.BrandDB(prefix)` lazy-connects per brand on first call. A nil return means the brand database is offline or doesn't exist — this is non-fatal.
- **Never assume a brand DB is available.** Always nil-check before querying. Fall back to `HotelRow.StartingPrice` on nil.
- Column sets vary per brand. `Pool.HasColumn(prefix, table, column)` reflects the result of `INFORMATION_SCHEMA` introspection performed on connect.
- PBA special case: table is `tb_hroom` (singular, not `tb_hrooms`) and has a different status column.

### Package Responsibilities (stay in your lane)

| Package | Role | Must NOT contain |
|---|---|---|
| `internal/repository/` | SQL queries, row mapping, image URL rewriting | MCP SDK calls, rate service calls |
| `internal/tools/` | MCP tool handlers: parse args, call repository + rate, return MCP content | Raw SQL or direct DB access |
| `internal/rate/` | Pricing: SB API, cache, circuit breaker, fallback chain | Hotel metadata |
| `internal/resources/` | Register MCP resources (UI) | Business logic |
| `internal/server/` | Wire tools + resources onto `mcp.Server`; own Gin routes | Tool business logic |

### Rate Fallback Chain (do not reorder)

1. SimpleBooking live XML API — 5-worker bounded pool, 5-min TTL cache, circuit breaker (5 failures → 120 s cooldown).
2. `tb_hrooms.room_rate` — per-brand DB stored rate.
3. `hotel_starting_price` — central DB, last resort.

### MCP Tool Visibility

| Tool | Visibility | Caller |
|---|---|---|
| `search_hotels` | public | Any MCP client |
| `recommend_hotel` | public | Any MCP client |
| `find_hotels` | public | Any MCP client |
| `get_hotel_detail` | `app` only | Embedded UI (`mcp-app.ts`) only |

`get_hotel_detail` visibility must never be promoted to `public`.

### Central DB Partition Filtering

The central database has tables partitioned by `hotel_id`. Any query that omits an explicit `hotel_id` or `region_id` filter performs a full scan across all partitions. Always include explicit filters on catalog tables.

### resourceDomains Is Per Tool Call

Every tool that opens the hotel dashboard must declare `"resourceDomains": []string{"images.archipelagohotels.com"}` in its `mcp.Meta["ui"]` map. The MCP App sandbox re-evaluates domain permissions per tool call; the resource registration alone is insufficient.

---

## 4. Code Style

- **Idiomatic Go.** `gofmt`-formatted, standard receiver naming, error wrapping with `%w`, table-driven tests.
- **No global state.** All dependencies (DB pool, rate service, config) are injected at tool/resource registration time. Do not introduce package-level vars for runtime state.
- **Dependencies injected, not discovered.** Write the function signature and body first; add imports only after the function is complete. Do not add imports speculatively.
- **Errors returned, not panicked.** Panics are only acceptable where the Go runtime forces them. Every tool handler must have a `defer recover()` that converts panics to error returns and logs via `slog.Error`.
- **`log/slog` everywhere.** Never use `fmt.Println` or `log.Printf` in handlers or packages.
- **Context propagated.** `ctx` from the tool handler flows to every DB and HTTP call. Never use `context.Background()` inside a handler.
- **No TODOs for incomplete functionality** in `internal/tools`, `internal/repository`, `internal/rate`, or `internal/server`. Use `// ponytail:` comments for intentional deferrals with an upgrade rationale.

### Handler Function Pattern

```go
func myHandler(pool *repository.Pool, rateSvc *rate.Service) func(
    context.Context, *mcp.CallToolRequest, MyArgs,
) (*mcp.CallToolResult, MyResult, error) {
    return func(ctx context.Context, _ *mcp.CallToolRequest, args MyArgs) (
        res *mcp.CallToolResult, out MyResult, err error,
    ) {
        defer func() {
            if r := recover(); r != nil {
                slog.Error("handler panic", "recover", r)
                err = fmt.Errorf("internal error: %v", r)
            }
        }()
        // normal logic
        return nil, out, nil
    }
}
```

Returning `(nil, result, nil)` is the normal success path. Returning `(nil, zeroValue, err)` signals a tool error.

---

## 5. What NOT To Do

- **Do not hardcode currency symbols.** Pass `hotel_currency` from the DB through to the JSON response. The UI formats display via `fmtPrice(v, currency)` using `toLocaleString('id-ID')` for IDR. Hardcoding breaks multi-currency properties.
- **Do not make HTTP requests to DB-sourced image URLs.** `resizeImageURL()` rewrites CDN URLs for CSP compliance. Never fetch or base64-proxy image bytes server-side — it will exceed MCP response size limits.
- **Do not add imports before writing functions.** Write function bodies first; add imports after the compiler identifies them. Speculative imports cause merge conflicts.
- **Do not assume brand DB schema.** Use the introspected column map (`Pool.HasColumn`). PBA uses `tb_hroom` not `tb_hrooms`.
- **Do not bypass the rate fallback chain.** All rate resolution goes through `internal/rate.Service`. Never call the SB API directly from a tool handler.
- **Do not promote `get_hotel_detail` to public visibility.** It is intentionally gated to the embedded UI.
- **Do not edit `internal/resources/mcp-app.html` by hand.** It is a Vite build artifact; any edit is overwritten by the next `make build-ui`.
- **Do not remove `resourceUri` from the three public tool Meta maps.** Doing so degrades the experience to plain text with no hotel card UI.
- **Do not hardcode `"Indonesia"` as country for new code.** Several legacy handlers do this incorrectly. New code must derive country from the hotel record.

---

## 6. Testing and Verification

There is no automated test suite. Minimum verification before committing:

```bash
# Lint
make lint               # go vet ./...

# Compilation check (fast)
go build ./...

# Full build (authoritative)
make build              # UI + Go — if this passes, the build is valid

# Health check (HTTP mode)
make dev-http &
curl http://localhost:9011/health
# Expected: {"db":true,"status":"ok","version":"dev"}

# Tool list smoke test
curl -s -X POST http://localhost:9011/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | jq '.result.tools[].name'
# Expected: ["search_hotels","get_hotel_detail","recommend_hotel","find_hotels"]

# Degraded mode (no DB) — must start without panic
MYSQL_PASS=wrong ./bin/archipelago-hotels-mcp stdio
```

When adding tests, place them as `_test.go` files in the same package (`package tools`, `package repository`, etc.).

---

## 7. UI Changes

1. Edit `ui/src/mcp-app.ts` — single TypeScript entry point (~1200 lines).
2. Run `make build` — rebuilds Vite output and recompiles the Go binary with the new embed.
3. Verify: open `http://localhost:9011/dashboard` in a browser and check hotel cards render correctly.

**Critical UI contracts:**
- `hotelSummary` JSON fields are read by name in the UI. Renaming or removing a field in Go breaks the UI silently (no JS error, missing data).
- All CSS and JS must be inlined by Vite. No CDN imports — the MCP App sandbox blocks external scripts.
- `fmtPrice(amount, currencyCode)` receives the raw currency code from the DB. The UI handles display formatting.
- `resizeImageURL()` produces `images.archipelagohotels.com` URLs. The UI loads them via `<img src>` — no `fetch()`.

---

## 8. Documentation

The `docs/` directory follows the [Diátaxis](https://diataxis.fr/) framework:

| Quadrant | Location | Content type |
|---|---|---|
| Tutorials | `docs/tutorials/` | Learning-oriented, step-by-step walkthroughs |
| How-to guides | `docs/how-to/` | Task-oriented, goal-focused recipes |
| Reference | `docs/reference/` | Precise technical information |
| Explanation | `docs/explanation/` | Conceptual background and rationale |

Architecture decision history is in `DESIGN.md` as numbered ADR entries. When you make an architectural choice, add an ADR entry — do not embed rationale in code comments.

---

## Key Files Quick Reference

| File | Role |
|---|---|
| `cmd/archipelago-hotels-mcp/main.go` | Entrypoint, transport dispatch (stdio / http) |
| `internal/server/server.go` | MCP server wiring + Gin HTTP routes |
| `internal/repository/repository.go` | Pool, Config, HotelRow, BrandRow, RoomRow types |
| `internal/repository/hotel.go` | SearchHotels, GetHotelByID, GetThumbnails, resizeImageURL |
| `internal/repository/room.go` | GetRooms (schema-adaptive), GetCredentials |
| `internal/rate/rate.go` | Service, BatchMinRates, circuitBreaker, SBClient |
| `internal/rate/cache.go` | rateCache (TTL, lazy expiry, no goroutine leak) |
| `internal/rate/simplebooking.go` | SimpleBooking XML request builder + response parser |
| `internal/rate/sentec.go` | Sentec REST client (reserved, 0 hotels use it) |
| `internal/tools/search.go` | `search_hotels` handler; defines shared `hotelSummary` type |
| `internal/tools/recommend.go` | `recommend_hotel` handler |
| `internal/tools/dashboard.go` | `find_hotels` handler |
| `internal/tools/detail.go` | `get_hotel_detail` handler (app-only visibility) |
| `internal/resources/dashboard.go` | MCP resource registration; `//go:embed mcp-app.html` |
| `internal/resources/mcp-app.html` | **Generated — do not edit by hand** |
| `ui/src/mcp-app.ts` | TypeScript frontend (single file, ~1200 lines) |
| `Makefile` | All build, lint, run, and clean targets |

---

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `MYSQL_HOST` | `127.0.0.1` | Central DB host |
| `MYSQL_PORT` | `3306` | Central DB port |
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | *(empty)* | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central database name |
| `DEBUG` | `0` | Set to `1` for debug logging |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

Per-brand databases follow the naming convention `db_{prefix}website` (e.g. `db_astonwebsite`). Exceptions mapped in `repository.brandDBName`: `favehotel → db_favewebsite`, `pba → db_pba`. SimpleBooking credentials are stored per-hotel in the per-brand databases and fetched at runtime by `repository.GetCredentials`.
