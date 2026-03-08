package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileTool_ReadWithinJail(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", map[string]any{"path": "hello.txt"})
	assert.False(t, result.IsError)
	assert.Equal(t, "hello world", resultText(t, result))
}

func TestFileTool_ReadSubdirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("nested content"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", map[string]any{"path": "sub/nested.txt"})
	assert.False(t, result.IsError)
	assert.Equal(t, "nested content", resultText(t, result))
}

func TestFileTool_ReadPathTraversal(t *testing.T) {
	root := t.TempDir()

	// Create a file outside the root.
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	// Try to read with path traversal.
	result := callTool(t, srv, "file_read", map[string]any{"path": "../" + filepath.Base(outsideDir) + "/secret.txt"})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "..")
}

func TestFileTool_ReadAbsoluteOutsideJail(t *testing.T) {
	root := t.TempDir()

	// Create a file outside the root.
	outsidePath := filepath.Join(os.TempDir(), "outside_test_mcp.txt")
	require.NoError(t, os.WriteFile(outsidePath, []byte("should not read"), 0o644))
	defer func() { _ = os.Remove(outsidePath) }()

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", map[string]any{"path": outsidePath})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "escapes repository root")
}

func TestFileTool_ReadNonExistent(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", map[string]any{"path": "doesnotexist.txt"})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "open file")
}

func TestFileTool_ReadMissingPathParam(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", nil)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "required")
}

func TestFileTool_WriteWithinJail(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_write", map[string]any{
		"path":    "output.txt",
		"content": "written by MCP",
	})
	assert.False(t, result.IsError)

	// Verify file was created.
	data, err := os.ReadFile(filepath.Join(root, "output.txt"))
	require.NoError(t, err)
	assert.Equal(t, "written by MCP", string(data))
}

func TestFileTool_WriteCreatesSubdirectory(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_write", map[string]any{
		"path":    "new/sub/dir/file.txt",
		"content": "deep write",
	})
	assert.False(t, result.IsError)

	data, err := os.ReadFile(filepath.Join(root, "new", "sub", "dir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep write", string(data))
}

func TestFileTool_WriteOutsideJail(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_write", map[string]any{
		"path":    "../escape.txt",
		"content": "should not write",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "..")
}

func TestFileTool_WriteAbsoluteOutsideJail(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	outsidePath := filepath.Join(os.TempDir(), "mcp_write_escape.txt")
	result := callTool(t, srv, "file_write", map[string]any{
		"path":    outsidePath,
		"content": "should not write",
	})
	assert.True(t, result.IsError)
}

func TestFileTool_WriteMissingParams(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	// Missing path.
	result := callTool(t, srv, "file_write", map[string]any{"content": "data"})
	assert.True(t, result.IsError)

	// Missing content.
	result = callTool(t, srv, "file_write", map[string]any{"path": "file.txt"})
	assert.True(t, result.IsError)
}

func TestFileTool_ListRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "subdir"), 0o755))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_list", nil)
	assert.False(t, result.IsError)

	type entry struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}
	var entries []entry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, result)), &entries))

	paths := make(map[string]bool)
	for _, e := range entries {
		paths[e.Path] = true
	}
	assert.True(t, paths["a.txt"])
	assert.True(t, paths["b.txt"])
	assert.True(t, paths["subdir"])
}

func TestFileTool_ListRecursive(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub", "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("top"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "mid.txt"), []byte("mid"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "deep", "bottom.txt"), []byte("bottom"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_list", map[string]any{"recursive": true})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "top.txt")
	assert.Contains(t, text, "sub/mid.txt")
	assert.Contains(t, text, "sub/deep/bottom.txt")
}

func TestFileTool_ListSkipsGitDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real.txt"), []byte("real"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_list", map[string]any{"recursive": true})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "real.txt")
	assert.NotContains(t, text, ".git/HEAD")
	assert.NotContains(t, text, ".git/objects")
}

func TestFileTool_ListOutsideJail(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_list", map[string]any{"path": "../.."})
	assert.True(t, result.IsError)
}

func TestFileTool_ListSubdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "util.go"), []byte("package pkg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "util_test.go"), []byte("package pkg"), 0o644))

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_list", map[string]any{"path": "pkg"})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "util.go")
	assert.Contains(t, text, "util_test.go")
}

func TestFileTool_ReadFileTooLarge(t *testing.T) {
	root := t.TempDir()

	// Create a sparse file just over 10 MiB.
	largePath := filepath.Join(root, "large.bin")
	f, err := os.Create(largePath)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(10*1024*1024+1))
	require.NoError(t, f.Close())

	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	result := callTool(t, srv, "file_read", map[string]any{"path": "large.bin"})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "too large")
}

func TestFileTool_WriteContentTooLarge(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	bigContent := strings.Repeat("x", 10*1024*1024+1)
	result := callTool(t, srv, "file_write", map[string]any{
		"path":    "big.bin",
		"content": bigContent,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "too large")
}
