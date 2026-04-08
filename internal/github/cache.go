package github

import (
	"reflect"
	"sync"
	"time"
)

const (
	defaultTTL     = 30 * time.Second
	maxCacheEntries = 500
)

type cacheEntry struct {
	value     any
	expiresAt time.Time
}
type cache struct {
	entries  map[string]cacheEntry
	order    []string // insertion order for LRU eviction
	ttl      time.Duration
	mu       sync.RWMutex
}

func newCache() *cache {
	return &cache{
		entries: make(map[string]cacheEntry),
		order:   make([]string, 0, maxCacheEntries),
		ttl:     defaultTTL,
	}
}

// Get retrieves a cached value by key. Returns (nil, false) if the key
// is missing or expired. Expired entries are lazily removed.
func (c *cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return cloneCacheValue(entry.value), true
}

// Set stores a value in the cache with the default TTL.
// If the cache exceeds maxCacheEntries, the oldest entry is evicted.
func (c *cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// If key already exists, update in-place without growing order slice.
	if _, exists := c.entries[key]; !exists {
		// Evict oldest entries until we're under the limit.
		for len(c.entries) >= maxCacheEntries && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = cacheEntry{value: cloneCacheValue(value), expiresAt: time.Now().Add(c.ttl)}
}

func cloneCacheValue(value any) any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return value
	}
	clone := reflect.New(rv.Elem().Type())
	clone.Elem().Set(rv.Elem())
	return clone.Interface()
}

// Invalidate removes a single key from the cache.
func (c *cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// InvalidatePrefix removes all cache entries whose keys start with prefix.
func (c *cache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.entries, k)
		}
	}
}

// InvalidateAll clears all cached entries.
func (c *cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
	c.order = c.order[:0]
}
