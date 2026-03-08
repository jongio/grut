package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_Log_PathFilter(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create a second file and commit it
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"other.txt"}))
	_, err = client.Commit(ctx, "Add other", CommitOpts{})
	require.NoError(t, err)

	// Test 1: Log with relative path - should return only commits touching README.md
	commits, err := client.Log(ctx, LogOpts{MaxCount: 10, Path: "README.md"})
	require.NoError(t, err)
	t.Logf("Relative path 'README.md' returned %d commits", len(commits))
	for _, c := range commits {
		t.Logf("  %s %s", c.ShortHash, c.Subject)
	}
	require.Len(t, commits, 1, "relative path should find 1 commit for README.md")

	// Test 2: Log with absolute path
	absPath := filepath.Join(dir, "README.md")
	t.Logf("Testing absolute path: %s", absPath)
	commits2, err := client.Log(ctx, LogOpts{MaxCount: 10, Path: absPath})
	require.NoError(t, err)
	t.Logf("Absolute path returned %d commits", len(commits2))
	for _, c := range commits2 {
		t.Logf("  %s %s", c.ShortHash, c.Subject)
	}
	require.Len(t, commits2, 1, "absolute path should find 1 commit for README.md")

	// Test 3: Log without path filter - should return all commits
	allCommits, err := client.Log(ctx, LogOpts{MaxCount: 10})
	require.NoError(t, err)
	t.Logf("No path filter returned %d commits", len(allCommits))
	require.Len(t, allCommits, 2)
}
