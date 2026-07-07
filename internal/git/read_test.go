package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeFile(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	want := []byte("api_key = ghp_" + "0123456789abcdefghijklmnopqrstuvwxyz\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.txt"), want, 0o644))

	got, err := client.WorktreeFile(context.Background(), "config.txt")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestWorktreeFile_RejectsTraversal(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	for _, p := range []string{"../secret", "..", "/etc/passwd"} {
		_, err := client.WorktreeFile(context.Background(), p)
		assert.Error(t, err, "path %q should be rejected", p)
	}
}

func TestWorktreeFile_MissingFile(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.WorktreeFile(context.Background(), "does-not-exist.txt")
	assert.Error(t, err)
}
