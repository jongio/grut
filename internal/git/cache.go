package git

import (
	"sync"
	"time"
)

// Cache provides a single-writer broadcast-reader cache for git data.
// Readers use RLock; the writer uses Lock. Data is considered fresh
// if its age is within maxAge.
type Cache struct {
	mu        sync.RWMutex
	status    []FileStatus
	branches  []Branch
	statusAge time.Time
	branchAge time.Time
	maxAge    time.Duration
}

// NewCache creates a Cache with the given maximum age for cached data.
func NewCache(maxAge time.Duration) *Cache {
	return &Cache{maxAge: maxAge}
}

// GetStatus returns cached status data and whether it is still fresh.
func (c *Cache) GetStatus() ([]FileStatus, bool) {
	if c.maxAge == 0 {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.status == nil {
		return nil, false
	}
	fresh := time.Since(c.statusAge) < c.maxAge
	out := make([]FileStatus, len(c.status))
	copy(out, c.status)
	return out, fresh
}

// SetStatus replaces the cached status data and resets the age.
func (c *Cache) SetStatus(status []FileStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = make([]FileStatus, len(status))
	copy(c.status, status)
	c.statusAge = time.Now()
}

// GetBranches returns cached branch data and whether it is still fresh.
func (c *Cache) GetBranches() ([]Branch, bool) {
	if c.maxAge == 0 {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.branches == nil {
		return nil, false
	}
	fresh := time.Since(c.branchAge) < c.maxAge
	out := make([]Branch, len(c.branches))
	copy(out, c.branches)
	return out, fresh
}

// SetBranches replaces the cached branch data and resets the age.
func (c *Cache) SetBranches(branches []Branch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.branches = make([]Branch, len(branches))
	copy(c.branches, branches)
	c.branchAge = time.Now()
}

// Invalidate clears all cached data.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = nil
	c.branches = nil
	c.statusAge = time.Time{}
	c.branchAge = time.Time{}
}
