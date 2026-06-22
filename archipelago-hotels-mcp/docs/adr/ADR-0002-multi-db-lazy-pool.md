# ADR-0002: Multi-Database Pool with Lazy Connection Strategy

**File(s):** `internal/repository/repository.go`
**Decision date:** 2026-06-22
**Status:** Accepted

---

## Context

Archipelago Hotels & Resorts grew by acquiring independent brands. Each brand operated its own
MySQL installation before the central catalog (`db_archipelagowebsite`) was introduced. Today the
system has:

- 1 central database (`db_archipelagowebsite`) — canonical hotel catalog, brands, regions
- 8 per-brand databases (`db_astonwebsite`, `db_neowebsite`, `db_favewebsite`, `db_alanawebsite`,
  `db_harperwebsite`, `db_kamuelawebsite`, `db_questwebsite`, `db_pba`) — room types, rates,
  booking credentials, thumbnails

Not all brand databases are reachable at the same time. During maintenance windows, a brand DBA
may take their database offline without affecting the central catalog. A startup strategy that
opens all connections eagerly would fail-fast the entire server whenever any single brand DB is
unreachable, which is not acceptable for a 24/7 hotel search service.

Additionally, brand databases were not built against a shared schema. Columns such as
`thumbnail_desktop`, `sentec_booking_id`, and room-table names (`tb_hrooms` vs `tb_hroom`) vary
across brands. There is no central registry of which brand has which columns.

---

## Decision

Use a `Pool` struct with:

1. **One always-on central connection** — server refuses to start if the central DB is unreachable.
2. **Per-brand lazy connections** — brand DBs are connected on first use, with the result (success
   or `nil`) cached for the server's lifetime.
3. **INFORMATION_SCHEMA introspection on first connect** — column presence for the three key tables
   is queried once per brand DB and cached in `brandCols`.

### Implementation

```go
// Pool — connection manager (repository.go)
type Pool struct {
    central   *sql.DB
    config    Config
    brandDBs  map[string]*sql.DB
    brandCols map[string]map[string]map[string]bool // prefix → table → column → true
    brands    map[int]BrandRow
    mu        sync.RWMutex
}

// BrandDB — double-checked locking pattern
func (p *Pool) BrandDB(ctx context.Context, prefix string) *sql.DB {
    p.mu.RLock()
    db, ok := p.brandDBs[prefix]
    p.mu.RUnlock()
    if ok {
        return db // nil means "permanently failed, do not retry"
    }

    // Slow path: connect outside lock; multiple goroutines may enter here
    // for different prefixes simultaneously — that is intentional.
    db = p.connectBrand(ctx, prefix)

    p.mu.Lock()
    defer p.mu.Unlock()
    if existing, ok := p.brandDBs[prefix]; ok {
        if db != nil && db != existing {
            db.Close() // another goroutine won the race; discard our handle
        }
        return existing
    }
    p.brandDBs[prefix] = db
    if db != nil {
        p.scanColumns(prefix, db)
    }
    return db
}

// HasColumn — guards queries against schema divergence
func (p *Pool) HasColumn(prefix, table, column string) bool {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.brandCols[prefix][table][column]
}
```

```go
// Per-brand connection pool limits (connectBrand)
conn.SetMaxOpenConns(3)
conn.SetMaxIdleConns(1)
conn.SetConnMaxLifetime(5 * time.Minute)
```

### Column introspection via INFORMATION_SCHEMA

On first successful connect, `scanColumns` runs a single query against
`INFORMATION_SCHEMA.COLUMNS` scoped to the connected database and the three key tables
(`tb_hotels`, `tb_hrooms`, `tb_hroom`). Results are stored in a nested bool map.

```go
func (p *Pool) scanColumns(prefix string, db *sql.DB) {
    rows, _ := db.Query(`
        SELECT TABLE_NAME, COLUMN_NAME
        FROM INFORMATION_SCHEMA.COLUMNS
        WHERE TABLE_SCHEMA = (SELECT DATABASE())
          AND TABLE_NAME IN ('tb_hotels','tb_hrooms','tb_hroom')`)
    // ... populate p.brandCols[prefix]
}
```

Call sites guard optional columns before building queries:

```go
if p.HasColumn(prefix, "tb_hrooms", "thumbnail_desktop") {
    // include thumbnail_desktop in SELECT
}
```

---

## Consequences

### Benefits

- **Resilience to missing brand DBs.** An unreachable brand DB logs a debug-level warning and
  returns `nil`; the calling code falls back to the central catalog's `hotel_starting_price`
  field. The server keeps serving other brands.
- **No startup blast.** All 8 brand connections opening simultaneously at startup would hit the
  DB server with a burst of TCP handshakes and auth round-trips. Lazy connection distributes
  this load across first requests.
- **Schema divergence handled at runtime.** `HasColumn` prevents SQL errors on brands that have
  not yet added a column or use a non-standard table name.

### Trade-offs

| Concern | Detail |
|---------|--------|
| First-request latency | The first request to a brand pays a ~3 s Ping timeout if the DB is down, or ~50–200 ms if it is up. Subsequent requests pay nothing. |
| No automatic retry | A brand DB that was down at first use stays `nil` until the server restarts. This is deliberate — a failed DB tends to stay failed until DBA action, and retrying on every request would add latency for all callers. |
| INFORMATION_SCHEMA cost | One extra query per brand DB on first connect. `INFORMATION_SCHEMA.COLUMNS` with `TABLE_SCHEMA = DATABASE()` is indexed in MySQL 8.0+ and typically sub-millisecond. |
| Lock contention | `BrandDB` takes a write lock only when storing a new connection. The double-checked pattern keeps the read path lock-free in the steady state. |
| Connection pool sizing | `MaxOpenConns=3` per brand DB is conservative. Under high load with many concurrent room-type queries per brand, a caller may queue for a connection. This is preferable to opening an unbounded number of connections to a shared DB server. |

---

## Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Connect all brand DBs at startup | One unreachable brand DB prevents the entire server from starting; unacceptable for a 24/7 service. |
| Static schema map per brand | Brands add and remove columns independently of the MCP server release cycle; a hard-coded map becomes a maintenance liability and a source of silent data loss. |
| Single database with brand views | Migration cost is prohibitive; brands operate on separate MySQL hosts with separate credentials. |
| Retry on brand DB failure per request | Retrying up to the 3-second timeout on every request doubles worst-case query latency for all callers sharing that brand path. A failed DB tends to need DBA intervention, not repeated pings. |
| Periodic background reconnect | Adds a goroutine and complexity; the current server process is typically restarted by systemd/Docker on config changes, which clears stale `nil` entries naturally. |

---

## Key Files

| File | Role |
|------|------|
| `internal/repository/repository.go` | `Pool`, `BrandDB`, `connectBrand`, `HasColumn`, `scanColumns` |
| `internal/repository/hotel.go` | `GetThumbnails` — guards `thumbnail_desktop` via `HasColumn` |
| `internal/repository/room.go` | `GetRooms` — handles `tb_hroom` vs `tb_hrooms` via `HasColumn`; PBA special case |
| `internal/repository/room.go` | `GetCredentials` — reads `sentec_booking_id` only when `HasColumn` confirms it exists |
