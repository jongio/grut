package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		maxAge time.Duration
	}{
		{name: "sets max age", maxAge: 25 * time.Millisecond},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewCache(tt.maxAge)
			require.NotNil(t, cache)

			assert.Equal(t, tt.maxAge, cache.maxAge)
		})
	}
}

func TestCacheStatus(t *testing.T) {
	t.Parallel()

	baseStatus := []FileStatus{
		{
			Path:           "file.txt",
			StagedStatus:   StatusModified,
			WorktreeStatus: StatusAdded,
			OrigPath:       "old-file.txt",
		},
	}

	tests := []struct {
		name func() string
		run  func(t *testing.T)
	}{
		{
			name: func() string { return "get status returns false on empty cache" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)

				status, fresh := cache.GetStatus()

				assert.Nil(t, status)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "set status then get status returns data and fresh true" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetStatus(baseStatus)

				status, fresh := cache.GetStatus()
				require.NotNil(t, status)

				assert.Equal(t, baseStatus, status)
				assert.True(t, fresh)
			},
		},
		{
			name: func() string { return "set status copies input" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				input := []FileStatus{{Path: "tracked.go", StagedStatus: StatusAdded, WorktreeStatus: StatusUnmodified}}
				cache.SetStatus(input)
				input[0].Path = "changed.go"
				input[0].StagedStatus = StatusDeleted

				status, fresh := cache.GetStatus()
				require.NotNil(t, status)

				assert.True(t, fresh)
				assert.Equal(t, "tracked.go", status[0].Path)
				assert.Equal(t, StatusAdded, status[0].StagedStatus)
			},
		},
		{
			name: func() string { return "get status returns copy" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetStatus(baseStatus)

				status, fresh := cache.GetStatus()
				require.NotNil(t, status)
				require.True(t, fresh)
				status[0].Path = "mutated.txt"

				again, againFresh := cache.GetStatus()
				require.NotNil(t, again)

				assert.True(t, againFresh)
				assert.Equal(t, "file.txt", again[0].Path)
			},
		},
		{
			name: func() string { return "invalidate clears status" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetStatus(baseStatus)
				cache.Invalidate()

				status, fresh := cache.GetStatus()

				assert.Nil(t, status)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "stale status returns fresh false" },
			run: func(t *testing.T) {
				cache := NewCache(time.Millisecond)
				cache.SetStatus(baseStatus)
				time.Sleep(5 * time.Millisecond)

				status, fresh := cache.GetStatus()
				require.NotNil(t, status)

				assert.Equal(t, baseStatus, status)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "setting empty status slice keeps it fresh and empty" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetStatus([]FileStatus{})

				status, fresh := cache.GetStatus()
				require.NotNil(t, status)

				assert.Empty(t, status)
				assert.True(t, fresh)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name(), func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestCacheBranches(t *testing.T) {
	t.Parallel()

	baseBranches := []Branch{
		{
			Name:      "main",
			IsRemote:  false,
			IsCurrent: true,
			Upstream:  "origin/main",
			Ahead:     2,
			Behind:    1,
			Hash:      "abc123",
		},
	}

	tests := []struct {
		name func() string
		run  func(t *testing.T)
	}{
		{
			name: func() string { return "get branches returns false on empty cache" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)

				branches, fresh := cache.GetBranches()

				assert.Nil(t, branches)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "set branches then get branches returns data and fresh true" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetBranches(baseBranches)

				branches, fresh := cache.GetBranches()
				require.NotNil(t, branches)

				assert.Equal(t, baseBranches, branches)
				assert.True(t, fresh)
			},
		},
		{
			name: func() string { return "set branches copies input" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				input := []Branch{{Name: "feature", Hash: "def456", Ahead: 1}}
				cache.SetBranches(input)
				input[0].Name = "mutated"
				input[0].Ahead = 99

				branches, fresh := cache.GetBranches()
				require.NotNil(t, branches)

				assert.True(t, fresh)
				assert.Equal(t, "feature", branches[0].Name)
				assert.Equal(t, 1, branches[0].Ahead)
			},
		},
		{
			name: func() string { return "get branches returns copy" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetBranches(baseBranches)

				branches, fresh := cache.GetBranches()
				require.NotNil(t, branches)
				require.True(t, fresh)
				branches[0].Name = "changed"

				again, againFresh := cache.GetBranches()
				require.NotNil(t, again)

				assert.True(t, againFresh)
				assert.Equal(t, "main", again[0].Name)
			},
		},
		{
			name: func() string { return "invalidate clears branches" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetBranches(baseBranches)
				cache.Invalidate()

				branches, fresh := cache.GetBranches()

				assert.Nil(t, branches)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "stale branches returns fresh false" },
			run: func(t *testing.T) {
				cache := NewCache(time.Millisecond)
				cache.SetBranches(baseBranches)
				time.Sleep(5 * time.Millisecond)

				branches, fresh := cache.GetBranches()
				require.NotNil(t, branches)

				assert.Equal(t, baseBranches, branches)
				assert.False(t, fresh)
			},
		},
		{
			name: func() string { return "setting empty branches slice keeps it fresh and empty" },
			run: func(t *testing.T) {
				cache := NewCache(time.Second)
				cache.SetBranches([]Branch{})

				branches, fresh := cache.GetBranches()
				require.NotNil(t, branches)

				assert.Empty(t, branches)
				assert.True(t, fresh)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name(), func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestCacheInvalidateClearsAllData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "invalidate clears status and branches"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewCache(time.Second)
			cache.SetStatus([]FileStatus{{Path: "file.txt", StagedStatus: StatusModified}})
			cache.SetBranches([]Branch{{Name: "main", Hash: "abc123"}})
			cache.Invalidate()

			status, statusFresh := cache.GetStatus()
			branches, branchFresh := cache.GetBranches()

			assert.Nil(t, status)
			assert.False(t, statusFresh)
			assert.Nil(t, branches)
			assert.False(t, branchFresh)
		})
	}
}
