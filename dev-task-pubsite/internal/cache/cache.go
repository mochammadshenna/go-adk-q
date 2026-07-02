package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

// Cache is a TTL key/value store. Reads after expiry return the stale value
// (Stale: true) so callers can choose to serve it while refreshing.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
}

// GetResult surfaces staleness alongside the cached value.
type GetResult struct {
	Value any
	Found bool
	Stale bool
	Age   time.Duration
}

// New creates a Cache with the given TTL.
func New(ttl time.Duration) *Cache {
	return &Cache{entries: make(map[string]entry), ttl: ttl}
}

// Get retrieves a value. Found is false when the key was never set.
// Stale is true when the value is past TTL — callers decide whether to serve
// stale data with a degraded flag or return an error.
func (c *Cache) Get(key string) GetResult {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return GetResult{}
	}
	age := time.Since(e.expiresAt.Add(-c.ttl))
	stale := time.Now().After(e.expiresAt)
	return GetResult{Value: e.value, Found: true, Stale: stale, Age: age}
}

// Set stores a value under key, resetting its TTL clock.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	c.entries[key] = entry{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
