package filetree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitignorePatternFor(t *testing.T) {
	root := filepath.Join("repo", "root")
	tests := []struct {
		name    string
		target  string
		isDir   bool
		want    string
		wantErr bool
	}{
		{"file at root", filepath.Join(root, "main.go"), false, "main.go", false},
		{"nested file uses forward slashes", filepath.Join(root, "src", "app.go"), false, "src/app.go", false},
		{"directory gets trailing slash", filepath.Join(root, "build"), true, "build/", false},
		{"nested directory", filepath.Join(root, "a", "b"), true, "a/b/", false},
		{"root itself is rejected", root, true, "", true},
		{"path outside repo is rejected", filepath.Join("repo", "other.go"), false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitignorePatternFor(root, tt.target, tt.isDir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitignoreContains(t *testing.T) {
	content := "node_modules/\n*.log\n\n  build/  \n"
	assert.True(t, gitignoreContains(content, "node_modules/"))
	assert.True(t, gitignoreContains(content, "*.log"))
	assert.True(t, gitignoreContains(content, "build/"), "surrounding whitespace should be ignored")
	assert.False(t, gitignoreContains(content, "dist/"))
	assert.False(t, gitignoreContains(content, "log"), "must match a whole line, not a substring")
}

func TestAppendGitignore(t *testing.T) {
	t.Run("creates the file when missing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".gitignore")
		added, err := appendGitignore(path, "main.go")
		require.NoError(t, err)
		assert.True(t, added)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "main.go\n", string(data))
	})

	t.Run("adds a trailing newline before appending", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".gitignore")
		require.NoError(t, os.WriteFile(path, []byte("*.log"), 0o644))
		added, err := appendGitignore(path, "build/")
		require.NoError(t, err)
		assert.True(t, added)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "*.log\nbuild/\n", string(data))
	})

	t.Run("does not duplicate an existing pattern", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".gitignore")
		require.NoError(t, os.WriteFile(path, []byte("build/\n"), 0o644))
		added, err := appendGitignore(path, "build/")
		require.NoError(t, err)
		assert.False(t, added)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "build/\n", string(data))
	})
}

func TestAddToGitignore_Handler(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	// Position the cursor on main.go.
	idx := -1
	for i, n := range ft.visible {
		if n.name == "main.go" {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "main.go should be visible")
	ft.viewport.cursor = idx

	_, cmd := ft.addToGitignore()
	require.NotNil(t, cmd)
	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)

	gi := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(gi)
	require.NoError(t, err)
	assert.Contains(t, string(data), "main.go\n")

	// Adding the same entry again is a no-op reported as Info.
	_, cmd = ft.addToGitignore()
	require.NotNil(t, cmd)
	msg = runCmd(t, ft, cmd)
	toast, ok = msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Info, toast.Level)
}
