package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Push — validation paths (no actual remote)
// ---------------------------------------------------------------------------

func TestPushValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    PushOpts
		wantErr string
	}{
		{
			name:    "invalid remote",
			opts:    PushOpts{Remote: "; rm -rf /"},
			wantErr: "push remote",
		},
		{
			name:    "invalid branch",
			opts:    PushOpts{Branch: "--delete"},
			wantErr: "push branch",
		},
		{
			name:    "leading dash remote",
			opts:    PushOpts{Remote: "--option-injection"},
			wantErr: "push remote",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.Push(ctx, tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPushArgConstruction(t *testing.T) {
	t.Parallel()

	// We can't actually push without a remote, but we can verify validation
	// passes for valid args (the push itself fails because no remote is configured).
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Push with all valid options — will fail at git execution, not validation.
	err = client.Push(ctx, PushOpts{
		Force:       true,
		SetUpstream: true,
		Tags:        true,
		Remote:      "origin",
		Branch:      "main",
	})
	// Fails because no remote configured, but validation passed.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "push:")

	// ForceWith (force-with-lease) path.
	err = client.Push(ctx, PushOpts{
		ForceWith: true,
		Remote:    "origin",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "push:")
}

// ---------------------------------------------------------------------------
// Pull — validation paths
// ---------------------------------------------------------------------------

func TestPullValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    PullOpts
		wantErr string
	}{
		{
			name:    "invalid remote",
			opts:    PullOpts{Remote: "$(evil)"},
			wantErr: "pull remote",
		},
		{
			name:    "invalid branch",
			opts:    PullOpts{Branch: "; drop tables"},
			wantErr: "pull branch",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.Pull(ctx, tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ---------------------------------------------------------------------------
// Fetch — validation paths
// ---------------------------------------------------------------------------

func TestFetchValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	err = client.Fetch(ctx, FetchOpts{Remote: "--evil"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch remote")

	err = client.Fetch(ctx, FetchOpts{Remote: "; rm -rf /"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch remote")

	err = client.Fetch(ctx, FetchOpts{Remote: "origin", Refspec: "pull/1/head:bad;branch"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch refspec")
}

func TestFetchArgConstruction(t *testing.T) {
	GlobalCommandLog().Clear()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Fetch with all options — will fail because no remote configured.
	err = client.Fetch(ctx, FetchOpts{
		Prune:   true,
		Tags:    true,
		All:     true,
		Remote:  "origin",
		Refspec: "pull/7/head:pr-7",
	})
	// Fails at git execution but validation passes.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch:")
	entries := GlobalCommandLog().Entries()
	require.NotEmpty(t, entries)
	assert.Equal(t, []string{"fetch", "--prune", "--tags", "--all", "origin", "pull/7/head:pr-7"}, entries[len(entries)-1].Args)
}

// ---------------------------------------------------------------------------
// IgnoredPaths
// ---------------------------------------------------------------------------

func TestClient_IgnoredPaths(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Without a .gitignore, no paths should be ignored.
	paths, err := client.IgnoredPaths(ctx)
	require.NoError(t, err)
	assert.Empty(t, paths)

	// Create .gitignore and an ignored file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.log"), []byte("log content"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "build"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build", "out.bin"), []byte("binary"), 0o644))

	paths, err = client.IgnoredPaths(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, paths)
	// At least test.log should appear.
	found := false
	for _, p := range paths {
		if p == "test.log" {
			found = true
		}
	}
	assert.True(t, found, "expected test.log in ignored paths, got: %v", paths)
}
