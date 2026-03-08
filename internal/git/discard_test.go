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
// DiscardFile
// ---------------------------------------------------------------------------

func TestClient_DiscardFile(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Modify an existing file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644))

	// Confirm it's modified.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, statuses)

	// Discard the change.
	require.NoError(t, client.DiscardFile(ctx, "README.md"))

	// File should be restored to original.
	content, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(content))
}

func TestClient_DiscardFileValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "empty path", path: "", wantErr: "discard file path"},
		{name: "absolute path", path: "C:\\Windows\\System32\\config", wantErr: "discard file path"},
		{name: "path traversal", path: "..\\..\\..\\Windows\\System32", wantErr: "discard file path"},
		{name: "null byte", path: "file\x00.txt", wantErr: "discard file path"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.DiscardFile(ctx, tt.path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ---------------------------------------------------------------------------
// DiscardAllUnstaged
// ---------------------------------------------------------------------------

func TestClient_DiscardAllUnstaged(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Modify existing file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644))

	// Create a second tracked file first, commit it, then modify it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"extra.txt"}))
	_, err = client.Commit(ctx, "add extra", CommitOpts{})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("modified extra\n"), 0o644))

	// Discard all unstaged changes.
	require.NoError(t, client.DiscardAllUnstaged(ctx))

	// Both files should be restored.
	content1, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(content1))

	content2, err := os.ReadFile(filepath.Join(dir, "extra.txt"))
	require.NoError(t, err)
	assert.Equal(t, "extra\n", string(content2))
}
