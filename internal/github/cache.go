package github

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v89/github"
)

const (
	defaultTTL      = 30 * time.Second
	maxCacheEntries = 500
)

type cacheEntry struct {
	value     any
	expiresAt time.Time
}
type cache struct {
	entries map[string]cacheEntry
	order   []string // insertion order for LRU eviction
	ttl     time.Duration
	mu      sync.Mutex
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
		c.order = slices.DeleteFunc(c.order, func(k string) bool { return k == key })
		return nil, false
	}
	return cloneCacheValue(entry.value), true
}

// Set stores a value in the cache with the default TTL.
// If the cache exceeds maxCacheEntries, the oldest entry is evicted.
//
// Cached values are shallow-copied on Get/Set via cloneCacheValue.
// Callers must not mutate nested pointer/slice/map fields of returned
// values; treat them as read-only. For this TUI application the
// GitHub API structs are consumed for display only, so shallow copy
// is sufficient.
func (c *cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// If key already exists, update in-place without growing order slice.
	if _, exists := c.entries[key]; !exists {
		// Evict oldest entries until we're under the limit.
		// Skip stale order entries that were already removed by
		// Invalidate/InvalidatePrefix (their map entry is gone).
		for len(c.entries) >= maxCacheEntries && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = cacheEntry{value: cloneCacheValue(value), expiresAt: time.Now().Add(c.ttl)}
}

// clonePtr returns a shallow copy of the pointed-to value.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// cloneCacheValue returns a shallow clone of pointer types stored in the
// cache. Known GitHub types use the generic clonePtr fast path; unknown
// pointer types fall back to reflect-based cloning. Non-pointer values
// are returned as-is.
func cloneCacheValue(value any) any {
	switch v := value.(type) {
	case *gh.Issue:
		return clonePtr(v)
	case *gh.PullRequest:
		return clonePtr(v)
	case *gh.Repository:
		return clonePtr(v)
	case *gh.RepositoryRelease:
		return clonePtr(v)
	case *gh.WorkflowRun:
		return clonePtr(v)
	case *gh.Notification:
		return clonePtr(v)
	default:
		// Fallback: clone unknown pointer types via reflect.
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
			clone := reflect.New(rv.Elem().Type())
			clone.Elem().Set(rv.Elem())
			return clone.Interface()
		}
		return value
	}
}

// Invalidate removes a single key from the cache.
func (c *cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	c.order = slices.DeleteFunc(c.order, func(k string) bool { return k == key })
}

// InvalidatePrefix removes all cache entries whose keys start with prefix.
func (c *cache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	c.order = slices.DeleteFunc(c.order, func(k string) bool { return strings.HasPrefix(k, prefix) })
}

// InvalidateAll clears all cached entries.
func (c *cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
	c.order = c.order[:0]
}
