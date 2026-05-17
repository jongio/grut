package github

import (
	"fmt"

	gh "github.com/google/go-github/v68/github"
)

// pageEntry is a generic cache wrapper for a single page of results.
type pageEntry[T any] struct {
	items []T
	page  PageResult
}

// listPage is a generic helper that eliminates boilerplate across the
// ListXXXPage methods. It handles cache lookup, API invocation, PageResult
// construction, and cache storage.
//
// Parameters:
//   - c:         the client (for cache access)
//   - key:       fully-built cache key (caller normalises opts first)
//   - errPrefix: human-readable label used in error messages
//   - fetch:     closure that calls the GitHub API and returns
//     (items, response, totalCount, error). Pass -1 for totalCount
//     when the API does not provide one.
func listPage[T any](
	c *clientImpl,
	key string,
	errPrefix string,
	fetch func() ([]T, *gh.Response, int, error),
) ([]T, PageResult, error) {
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(pageEntry[T])
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for %s", errPrefix)
		}
		return entry.items, entry.page, nil
	}

	items, resp, totalCount, err := fetch()
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("%s: %w", errPrefix, err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: totalCount}
	c.cache.Set(key, pageEntry[T]{items: items, page: pr})
	return items, pr, nil
}
