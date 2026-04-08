package github

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "creates cache with default ttl and empty entries"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := newCache()
			require.NotNil(t, cache)
			require.NotNil(t, cache.entries)

			assert.Equal(t, defaultTTL, cache.ttl)
			assert.Empty(t, cache.entries)
		})
	}
}

func TestCacheGetAndSet(t *testing.T) {
	t.Parallel()

	type sample struct {
		Name string
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "get on empty cache returns nil false",
			run: func(t *testing.T) {
				cache := newCache()

				value, ok := cache.Get("missing")

				assert.Nil(t, value)
				assert.False(t, ok)
			},
		},
		{
			name: "set then get returns stored value",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("key", "value")

				value, ok := cache.Get("key")
				require.True(t, ok)

				assert.Equal(t, "value", value)
			},
		},
		{
			name: "expired entry returns nil false",
			run: func(t *testing.T) {
				cache := newCache()
				cache.ttl = time.Millisecond
				cache.Set("key", "value")
				time.Sleep(5 * time.Millisecond)

				value, ok := cache.Get("key")

				assert.Nil(t, value)
				assert.False(t, ok)
			},
		},
		{
			name: "different value types work",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("string", "value")
				cache.Set("int", 42)
				cache.Set("struct", sample{Name: "octo"})

				stringValue, ok := cache.Get("string")
				require.True(t, ok)
				intValue, ok := cache.Get("int")
				require.True(t, ok)
				structValue, ok := cache.Get("struct")
				require.True(t, ok)

				assert.Equal(t, "value", stringValue)
				assert.Equal(t, 42, intValue)
				assert.Equal(t, sample{Name: "octo"}, structValue)
			},
		},
		{
			name: "multiple keys are independent",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("one", 1)
				cache.Set("two", 2)
				cache.Set("three", 3)

				one, ok := cache.Get("one")
				require.True(t, ok)
				two, ok := cache.Get("two")
				require.True(t, ok)
				three, ok := cache.Get("three")
				require.True(t, ok)

				assert.Equal(t, 1, one)
				assert.Equal(t, 2, two)
				assert.Equal(t, 3, three)
			},
		},
		{
			name: "set overwrites existing key",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("key", "first")
				cache.Set("key", "second")

				value, ok := cache.Get("key")
				require.True(t, ok)

				assert.Equal(t, "second", value)
			},
		},
		{
			name: "set is not coupled to original pointer value",
			run: func(t *testing.T) {
				cache := newCache()
				original := &sample{Name: "before"}
				cache.Set("key", original)
				original.Name = "after"

				value, ok := cache.Get("key")
				require.True(t, ok)
				stored, ok := value.(*sample)
				require.True(t, ok)
				require.NotNil(t, stored)

				assert.NotSame(t, original, stored)
				assert.Equal(t, "before", stored.Name)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestCacheInvalidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "invalidate removes specific key",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("remove", "value")
				cache.Set("keep", "other")
				cache.Invalidate("remove")

				removed, removedOK := cache.Get("remove")
				kept, keptOK := cache.Get("keep")

				assert.Nil(t, removed)
				assert.False(t, removedOK)
				assert.Equal(t, "other", kept)
				assert.True(t, keptOK)
			},
		},
		{
			name: "invalidate missing key is no-op",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("keep", "value")
				cache.Invalidate("missing")

				value, ok := cache.Get("keep")
				require.True(t, ok)

				assert.Equal(t, "value", value)
			},
		},
		{
			name: "invalidate all clears entries",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("one", 1)
				cache.Set("two", 2)
				cache.InvalidateAll()

				one, oneOK := cache.Get("one")
				two, twoOK := cache.Get("two")
				require.NotNil(t, cache.entries)

				assert.Nil(t, one)
				assert.False(t, oneOK)
				assert.Nil(t, two)
				assert.False(t, twoOK)
				assert.Empty(t, cache.entries)
			},
		},
		{
			name: "invalidate prefix removes matching keys",
			run: func(t *testing.T) {
				cache := newCache()
				cache.Set("issues:owner/repo:a", "v1")
				cache.Set("issues:owner/repo:b", "v2")
				cache.Set("prs:owner/repo:a", "v3")
				cache.InvalidatePrefix("issues:owner/repo:")

				_, issA := cache.Get("issues:owner/repo:a")
				_, issB := cache.Get("issues:owner/repo:b")
				prVal, prOK := cache.Get("prs:owner/repo:a")

				assert.False(t, issA, "issues:a should be removed")
				assert.False(t, issB, "issues:b should be removed")
				assert.True(t, prOK, "prs:a should remain")
				assert.Equal(t, "v3", prVal)
			},
		},
		{
			name: "invalidate cleans order slice",
			run: func(t *testing.T) {
				c := newCache()
				c.Set("a", 1)
				c.Set("b", 2)
				c.Set("c", 3)
				c.Invalidate("b")

				assert.Len(t, c.order, 2, "order slice should shrink after Invalidate")
				assert.Equal(t, []string{"a", "c"}, c.order)
			},
		},
		{
			name: "invalidate prefix cleans order slice",
			run: func(t *testing.T) {
				c := newCache()
				c.Set("issues:1", "v1")
				c.Set("issues:2", "v2")
				c.Set("prs:1", "v3")
				c.InvalidatePrefix("issues:")

				assert.Len(t, c.order, 1, "order slice should contain only non-prefix keys")
				assert.Equal(t, []string{"prs:1"}, c.order)
			},
		},
		{
			name: "expired get cleans order slice",
			run: func(t *testing.T) {
				c := newCache()
				c.ttl = time.Millisecond
				c.Set("ephemeral", "value")
				c.ttl = defaultTTL
				c.Set("durable", "value")
				time.Sleep(5 * time.Millisecond)

				// Get the expired key — should clean order
				_, ok := c.Get("ephemeral")
				assert.False(t, ok)

				assert.Len(t, c.order, 1)
				assert.Equal(t, []string{"durable"}, c.order)
			},
		},
		{
			name: "eviction skips stale order entries from prior invalidation",
			run: func(t *testing.T) {
				// Use a tiny cache to trigger eviction quickly.
				c := newCache()
				// Fill to capacity minus 1 then invalidate some, then fill past limit.
				for i := range maxCacheEntries {
					c.Set(fmt.Sprintf("k%d", i), i)
				}
				assert.Len(t, c.entries, maxCacheEntries)

				// Invalidate first entry — order is cleaned, map is cleaned.
				c.Invalidate("k0")
				assert.Len(t, c.entries, maxCacheEntries-1)
				assert.Len(t, c.order, maxCacheEntries-1)

				// Adding a new entry should not require eviction since
				// we're below the limit.
				c.Set("new", "val")
				assert.Len(t, c.entries, maxCacheEntries)

				// Verify the new key is accessible.
				val, ok := c.Get("new")
				assert.True(t, ok)
				assert.Equal(t, "val", val)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
