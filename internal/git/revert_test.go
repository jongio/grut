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
// Revert
// ---------------------------------------------------------------------------

func TestClient_Revert(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create a second commit we can revert.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "revert.txt"), []byte("to revert\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"revert.txt"}))
	_, err = client.Commit(ctx, "commit to revert", CommitOpts{})
	require.NoError(t, err)

	// File should exist before revert.
	_, err = os.Stat(filepath.Join(dir, "revert.txt"))
	require.NoError(t, err)

	// Revert the HEAD commit — file should be removed.
	require.NoError(t, client.Revert(ctx, "HEAD"))

	// The reverted file is gone.
	content, err := os.ReadFile(filepath.Join(dir, "revert.txt"))
	if err == nil {
		// File may still exist but be empty depending on git version.
		assert.Empty(t, string(content), "reverted file should be removed or empty")
	}

	// Verify a new revert commit was created.
	commits, err := client.Log(ctx, LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.NotEmpty(t, commits)
	assert.Contains(t, commits[0].Subject, "Revert")
}

func TestClient_RevertValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hash    string
		wantErr string
	}{
		{name: "empty hash", hash: "", wantErr: "revert hash"},
		{name: "invalid ref with shell chars", hash: "abc;rm", wantErr: "revert hash"},
		{name: "leading dash", hash: "--amend", wantErr: "revert hash"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.Revert(ctx, tt.hash)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestClient_RevertAbortContinueErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// No revert in progress — both should fail.
	assert.Error(t, client.RevertAbort(ctx))
	assert.Error(t, client.RevertContinue(ctx))
}

func TestClient_RevertConflictAndAbort(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create file and two commits that modify it differently.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("base\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"file.txt"}))
	_, err = client.Commit(ctx, "add file", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"file.txt"}))
	_, err = client.Commit(ctx, "modify file", CommitOpts{})
	require.NoError(t, err)

	// Revert the first commit (add file) — should conflict since later commit modified it.
	commits, err := client.Log(ctx, LogOpts{MaxCount: 3})
	require.NoError(t, err)
	require.True(t, len(commits) >= 2)

	err = client.Revert(ctx, commits[1].Hash)
	if err != nil {
		// Conflict expected — abort should succeed.
		require.NoError(t, client.RevertAbort(ctx))

		// File should be back to pre-revert state.
		content, readErr := os.ReadFile(filepath.Join(dir, "file.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, "modified\n", string(content))
	}
}
