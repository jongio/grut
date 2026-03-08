package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_DiffTreeFiles(t *testing.T) {
	dir := initTestRepo(t) // creates repo with README.md + initial commit
	c, err := NewClient(dir)
	require.NoError(t, err)

	// Get the initial commit hash.
	commits, err := c.Log(context.Background(), LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.Len(t, commits, 1)

	files, err := c.DiffTreeFiles(context.Background(), commits[0].Hash)
	require.NoError(t, err)
	assert.Equal(t, []string{"README.md"}, files)
}

func TestClient_DiffTreeFiles_MultipleFiles(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	// Add multiple files in a second commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b\n"), 0o644))

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	run("add", ".")
	run("commit", "-m", "Add files")

	commits, err := c.Log(context.Background(), LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.Len(t, commits, 1)

	files, err := c.DiffTreeFiles(context.Background(), commits[0].Hash)
	require.NoError(t, err)
	assert.Contains(t, files, "a.txt")
	assert.Contains(t, files, "sub/b.txt")
	assert.Len(t, files, 2)
}

func TestClient_DiffTreeFiles_InvalidHash(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	_, err = c.DiffTreeFiles(context.Background(), "")
	assert.Error(t, err, "empty hash should fail validation")
}

func TestClient_DiffTreeFiles_NonexistentHash(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	_, err = c.DiffTreeFiles(context.Background(), "deadbeef1234567890abcdef1234567890abcdef")
	assert.Error(t, err, "nonexistent hash should fail")
}
