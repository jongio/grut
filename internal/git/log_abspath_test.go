package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Log_AbsolutePathFilter(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create a second file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"other.txt"}))
	_, err = client.Commit(ctx, "Add other file", CommitOpts{})
	require.NoError(t, err)

	// Verify we have 2 commits total
	allCommits, err := client.Log(ctx, LogOpts{MaxCount: 10})
	require.NoError(t, err)
	require.Len(t, allCommits, 2)
	t.Logf("All commits: %d", len(allCommits))

	// Test with ABSOLUTE path (like filetree sends)
	absPath := filepath.Join(dir, "README.md")
	t.Logf("Testing absolute path: %s", absPath)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 10, Path: absPath})
	t.Logf("Result: %d commits, err=%v", len(commits), err)
	if err != nil {
		t.Fatalf("Log with absolute path failed: %v", err)
	}
	assert.Len(t, commits, 1, "absolute path should filter to 1 commit")
	assert.Equal(t, "Initial commit", commits[0].Subject)

	// Test with relative path
	commits2, err := client.Log(ctx, LogOpts{MaxCount: 10, Path: "README.md"})
	require.NoError(t, err)
	assert.Len(t, commits2, 1, "relative path should filter to 1 commit")

	// Test with other file absolute path
	otherAbs := filepath.Join(dir, "other.txt")
	commits3, err := client.Log(ctx, LogOpts{MaxCount: 10, Path: otherAbs})
	require.NoError(t, err)
	assert.Len(t, commits3, 1, "should find 1 commit for other.txt")
	assert.Equal(t, "Add other file", commits3[0].Subject)
}
