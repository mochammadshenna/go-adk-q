# ADR-0003: Three-Level Rate Fallback Chain

**File(s):** `internal/rate/rate.go`, `internal/rate/simplebooking.go`, `internal/rate/cache.go`
**Decision date:** 2026-06-22

---

## Decision

Room prices are resolved through a three-level fallback chain: (1) SimpleBooking XML API live rates, (2) stored rates from `tb_hrooms` in the brand DB, (3) `hotel_starting_price` from the central catalog. A circuit breaker prevents repeated calls to a failing API. Results are cached with a 5-minute TTL. A bounded worker pool (5 concurrent goroutines) caps outbound API load.

### Implementation

```go
// rate.go — GetRates fallback chain
func (s *Service) GetRates(ctx context.Context, prefix string, apiID int, ...) ([]RoomRate, error) {
    key := cacheKey(prefix, apiID)

    // Level 1: in-process TTL cache (5-minute window)
    if cached, ok := s.cache.Get(key); ok {
        return cached, nil
    }

    // Level 2: SimpleBooking XML API (live, circuit-breaker protected)
    if rates, err := s.sb.GetRates(ctx, prefix, apiID, checkIn, checkOut); err == nil && len(rates) > 0 {
        s.cache.Set(key, rates)
        return rates, nil
    }

    // Level 3: stored rates from brand DB tb_hrooms
    if rooms, err := s.pool.GetRooms(ctx, prefix, apiID); err == nil && len(rooms) > 0 {
        rates := roomsToRates(rooms)
        s.cache.Set(key, rates)
        return rates, nil
    }

    // Cache nil to prevent re-fetch for hotels with no rate data
    s.cache.Set(key, nil)
    return nil, nil
}

// BatchMinRates — level 4 fallback lives HERE, not in GetRates
// If GetRates returns nil, use hotel_starting_price from central DB
for _, r := range hotels {
    go func(r hotelRef) {
        sem <- struct{}{}; defer func() { <-sem }()
        rates, _ := svc.GetRates(ctx, r.prefix, r.apiID, ...)
        if len(rates) == 0 && r.startFrom > 0 {
            results <- result{r.hotelID, r.startFrom} // central DB fallback
        } else if len(rates) > 0 {
            results <- result{r.hotelID, minRate(rates)}
        }
    }(r)
}
```

```go
// simplebooking.go — circuit breaker
type circuitBreaker struct {
    failures  int
    lastFail  time.Time
    threshold int           // 5
    cooldown  time.Duration // 120s
    mu        sync.Mutex
}

func (cb *circuitBreaker) Allow() bool {
    cb.mu.Lock(); defer cb.mu.Unlock()
    if cb.failures >= cb.threshold {
        if time.Since(cb.lastFail) < cb.cooldown {
            return false // circuit open
        }
        cb.failures = 0 // auto-reset after cooldown
    }
    return true
}
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Cache TTL | 5 minutes, lazy expiry (no background goroutine) | `cache.go` |
| Cache nil storage | Prevents re-fetch for zero-rate hotels | `rate.go:GetRates` |
| Circuit breaker threshold | 5 consecutive failures → open | `simplebooking.go:circuitBreaker` |
| Circuit breaker cooldown | 120 seconds before auto-reset | `simplebooking.go:circuitBreaker` |
| Worker pool size | `maxWorkers = 5` semaphore channel | `rate.go:BatchMinRates` |
| Batch size limit | Up to 50 hotels per `BatchMinRates` call | `search.go:Limit=50` |
| Starting price fallback | Handled in `BatchMinRates`, not `GetRates` | `rate.go:BatchMinRates` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Live rates only | SimpleBooking is occasionally unreachable; zero prices break the UI |
| Starting price only | Too coarse — one price for all room types; defeats the purpose of a booking tool |
| Redis cache | Adds an external dependency for a single-process server; in-memory is sufficient for the query volume |
| Retry with backoff | Does not prevent request avalanche during a sustained outage; circuit breaker is safer |
| Unlimited concurrency to SimpleBooking | SimpleBooking's XML API has undocumented rate limits; burst of 50 parallel requests risks 429 errors |
