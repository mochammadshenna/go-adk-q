# ADR-0002: Multi-DB Pool with Lazy Per-Brand Connections

**File(s):** `internal/repository/repository.go`
**Decision date:** 2026-06-22

---

## Decision

Database connectivity uses a `Pool` struct that holds one always-on central database connection (`db_archipelagowebsite`) and a map of lazily-connected per-brand databases. Brand databases are connected on first use with a 3-second timeout and their connection handles cached indefinitely (including `nil` on failure). Column presence is introspected from `INFORMATION_SCHEMA` on first connect and cached in `brandCols`.

### Implementation

```go
// repository.go — Pool struct
type Pool struct {
    central   *sql.DB
    brandDBs  map[string]*sql.DB             // nil entry = permanently failed
    brandCols map[string]map[string]map[string]bool // [prefix][table][col] = exists
    brands    map[int]BrandRow
    mu        sync.RWMutex
}

// BrandDB — double-checked locking pattern
func (p *Pool) BrandDB(ctx context.Context, prefix string) *sql.DB {
    p.mu.RLock()
    db, seen := p.brandDBs[prefix]
    p.mu.RUnlock()
    if seen {
        return db // nil means "failed, don't retry"
    }
    // slow path: connect outside lock, then store under write lock
    db = p.connectBrand(ctx, prefix)
    p.mu.Lock()
    if _, alreadyDone := p.brandDBs[prefix]; !alreadyDone {
        p.brandDBs[prefix] = db
        if db != nil {
            p.scanColumns(prefix, db)
        }
    }
    p.mu.Unlock()
    return p.brandDBs[prefix]
}

// HasColumn — guards queries against schema diversity across brands
func (p *Pool) HasColumn(prefix, table, column string) bool {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.brandCols[prefix][table][column]
}
```

```go
// Per-brand connection settings
db.SetMaxOpenConns(3)
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(10 * time.Minute)
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Central DB | Always-connected at startup; server refuses to start without it | `repository.go:NewPool` |
| Brand DB connect | `sql.Open()` + `Ping()` with 3-second context timeout | `repository.go:connectBrand` |
| Failure handling | `nil` stored in `brandDBs`; no retry until server restart | `repository.go:BrandDB` |
| Column introspection | `INFORMATION_SCHEMA.COLUMNS` query on first connect | `repository.go:scanColumns` |
| Schema diversity | `GetThumbnails` guards `thumbnail_desktop` with `HasColumn()` | `hotel.go:GetThumbnails` |
| PBA schema | Uses `tb_hroom` (singular); `GetRooms` falls back via `HasColumn` | `room.go:GetRooms` |
| Connection limits | `MaxOpenConns=3`, `MaxIdleConns=1` per brand DB | `repository.go:connectBrand` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Connect all brand DBs at startup | One unreachable brand DB would prevent the entire server from starting |
| Static schema map per brand | Brands add/remove columns independently; a static map becomes a maintenance liability |
| Single database with brand views | Migration cost is prohibitive; brands share no common DB server |
| Retry on brand DB failure | Retrying on every request adds latency; a failed DB tends to stay failed until the DBA intervenes |

### Background

Archipelago Hotels & Resorts grew by acquiring independent brands. Each brand operated its own MySQL installation before the central catalog (`db_archipelagowebsite`) existed. The central DB holds the canonical hotel list; brand DBs hold operational data (room types, rates, thumbnails) that was never migrated to the central catalog. The two-database join is the only practical bridge, linked by `central.tb_hotels.api_hotel_id = brand.tb_hotels.hotel_id`.
