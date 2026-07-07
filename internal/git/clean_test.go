package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCleanDryRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single file", "Would remove junk.txt\n", []string{"junk.txt"}},
		{
			"file and dir",
			"Would remove junk.txt\nWould remove build/\n",
			[]string{"junk.txt", "build/"},
		},
		{
			"skips nested repository lines",
			"Would remove a.txt\nWould skip repository nested/\nWould remove b.txt\n",
			[]string{"a.txt", "b.txt"},
		},
		{
			"crlf line endings",
			"Would remove a.txt\r\nWould remove b.txt\r\n",
			[]string{"a.txt", "b.txt"},
		},
		{"ignores unrelated lines", "removing nothing\nblah\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseCleanDryRun(tt.in))
		})
	}
}

func TestClient_CleanPreview(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Ignore *.log via the repo-local exclude file so .gitignore itself
	// does not show up as an untracked candidate.
	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	require.NoError(t, os.WriteFile(excludePath, []byte("*.log\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "debug.log"), []byte("y"), 0o644))

	// Without ignored files: only the plain untracked file appears.
	got, err := client.CleanPreview(ctx, CleanOpts{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "junk.txt", got[0].Path)
	assert.False(t, got[0].Ignored)

	// With ignored files: both appear, and the ignored one is flagged.
	got, err = client.CleanPreview(ctx, CleanOpts{IncludeIgnored: true})
	require.NoError(t, err)
	paths := map[string]bool{}
	ignored := map[string]bool{}
	for _, c := range got {
		paths[c.Path] = true
		ignored[c.Path] = c.Ignored
	}
	assert.True(t, paths["junk.txt"])
	assert.True(t, paths["debug.log"])
	assert.False(t, ignored["junk.txt"])
	assert.True(t, ignored["debug.log"])
}

func TestClient_Clean_RemovesSelectedOnly(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("y"), 0o644))

	require.NoError(t, client.Clean(ctx, CleanOpts{Paths: []string{"junk.txt"}}))

	_, err = os.Stat(filepath.Join(dir, "junk.txt"))
	assert.True(t, os.IsNotExist(err), "selected file should be removed")
	_, err = os.Stat(filepath.Join(dir, "keep.txt"))
	assert.NoError(t, err, "unselected file must be kept")
}

func TestClient_Clean_EmptyPathsNoop(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))

	require.NoError(t, client.Clean(ctx, CleanOpts{}))
	_, err = os.Stat(filepath.Join(dir, "junk.txt"))
	assert.NoError(t, err, "empty path list must not remove anything")
}

func TestClient_Clean_RejectsTraversal(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	err = client.Clean(ctx, CleanOpts{Paths: []string{"../escape.txt"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clean path")
}
