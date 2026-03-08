package github

import (
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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
