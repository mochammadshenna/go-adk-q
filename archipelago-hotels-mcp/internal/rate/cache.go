package rate

import (
	"sync"
	"time"
)

// cacheEntry holds cached room rates with an expiry time.
// ponytail: lazy expiry on read — no background goroutine, no ticker, no leak.
type cacheEntry struct {
	rates     []RoomRate
	expiresAt time.Time
}

// rateCache is a TTL-based rate cache safe for concurrent use.
type rateCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
	ttl   time.Duration
}

func newRateCache(ttl time.Duration) *rateCache {
	return &rateCache{
		items: make(map[string]cacheEntry),
		ttl:   ttl,
	}
}

// cacheKey builds a unique key from dbPrefix and apiHotelID.
func cacheKey(prefix string, hotelID int) string {
	return prefix + ":" + itoa(hotelID)
}

// Get returns cached rates if present and not expired.
func (c *rateCache) Get(key string) ([]RoomRate, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.rates, true
}

// Set stores rates in the cache.
func (c *rateCache) Set(key string, rates []RoomRate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheEntry{
		rates:     rates,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Clear empties the cache. Safe for testing.
func (c *rateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]cacheEntry)
}

// itoa is a fast int-to-string for cache keys.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
