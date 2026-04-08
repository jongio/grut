package markdown

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCacheEvictsAtMaxWidths verifies that the renderer cache never
// exceeds maxCachedWidths entries. When the limit is reached, all
// entries are evicted before adding the new one.
func TestCacheEvictsAtMaxWidths(t *testing.T) {
	// Reset global cache state.
	rendererMu.Lock()
	clear(rendererCache)
	rendererMu.Unlock()

	// Fill the cache to exactly maxCachedWidths.
	for i := range maxCachedWidths {
		_, err := getRenderer(80 + i)
		require.NoError(t, err)
	}

	rendererMu.Lock()
	assert.Len(t, rendererCache, maxCachedWidths, "cache should be at max capacity")
	rendererMu.Unlock()

	// Adding one more width should trigger eviction of all prior entries.
	_, err := getRenderer(80 + maxCachedWidths)
	require.NoError(t, err)

	rendererMu.Lock()
	assert.Len(t, rendererCache, 1, "cache should contain only the new entry after eviction")
	rendererMu.Unlock()
}

// TestConcurrentGetRenderer verifies that concurrent calls to
// getRenderer do not race. Run with -race to detect issues.
func TestConcurrentGetRenderer(t *testing.T) {
	rendererMu.Lock()
	clear(rendererCache)
	rendererMu.Unlock()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(width int) {
			defer wg.Done()
			// Vary widths across maxCachedWidths boundary to exercise eviction.
			r, err := getRenderer(80 + (width % (maxCachedWidths + 2)))
			assert.NoError(t, err)
			assert.NotNil(t, r)
		}(i)
	}
	wg.Wait()

	// Verify cache is in a valid state.
	rendererMu.Lock()
	assert.LessOrEqual(t, len(rendererCache), maxCachedWidths)
	rendererMu.Unlock()
}
