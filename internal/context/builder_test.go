package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBuilder creates a Builder rooted at a temp directory.
func newTestBuilder(t *testing.T) (*Builder, string) {
	t.Helper()
	root := t.TempDir()
	b, err := NewBuilder(root)
	require.NoError(t, err)
	return b, root
}

// writeFile is a helper that creates a file with the given content.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestBuilder_AddAndFiles(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "hello.go", "package main\n")

	require.NoError(t, b.Add("hello.go"))

	files := b.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "hello.go", files[0].Path)
	assert.Equal(t, "package main\n", files[0].Content)
	assert.Greater(t, files[0].Tokens, 0)
}

func TestBuilder_AddAbsolutePath(t *testing.T) {
	b, root := newTestBuilder(t)
	abs := writeFile(t, root, "main.go", "package main\n")

	require.NoError(t, b.Add(abs))

	files := b.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "main.go", files[0].Path)
}

func TestBuilder_AddSubdirectory(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, filepath.Join("src", "lib.go"), "package src\n")

	require.NoError(t, b.Add(filepath.Join("src", "lib.go")))

	files := b.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "src/lib.go", files[0].Path)
}

func TestBuilder_AddDuplicate(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "dup.go", "package dup\n")

	require.NoError(t, b.Add("dup.go"))
	require.NoError(t, b.Add("dup.go")) // no-op

	files := b.Files()
	assert.Len(t, files, 1)
}

func TestBuilder_AddNonexistent(t *testing.T) {
	b, _ := newTestBuilder(t)
	err := b.Add("does_not_exist.go")
	assert.Error(t, err)
}

func TestBuilder_AddEmptyPath(t *testing.T) {
	b, _ := newTestBuilder(t)
	err := b.Add("")
	assert.Error(t, err)
}

func TestBuilder_PathJail_DotDot(t *testing.T) {
	b, _ := newTestBuilder(t)
	err := b.Add("../escape.go")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes repository root")
}

func TestBuilder_PathJail_AbsoluteOutside(t *testing.T) {
	b, _ := newTestBuilder(t)
	// Create a file outside the repo root.
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", "top secret\n")

	err := b.Add(filepath.Join(outside, "secret.txt"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes repository root")
}

func TestBuilder_Remove(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "b.go", "package b\n")

	require.NoError(t, b.Add("a.go"))
	require.NoError(t, b.Add("b.go"))
	assert.Len(t, b.Files(), 2)

	b.Remove("a.go")
	files := b.Files()
	require.Len(t, files, 1)
	assert.Equal(t, "b.go", files[0].Path)
}

func TestBuilder_RemoveNonexistent(t *testing.T) {
	b, _ := newTestBuilder(t)
	// Should not panic or error.
	b.Remove("nope.go")
	assert.Empty(t, b.Files())
}

func TestBuilder_RemoveMiddle(t *testing.T) {
	b, root := newTestBuilder(t)
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		writeFile(t, root, name, "package "+name+"\n")
		require.NoError(t, b.Add(name))
	}

	b.Remove("b.go")
	files := b.Files()
	require.Len(t, files, 2)
	assert.Equal(t, "a.go", files[0].Path)
	assert.Equal(t, "c.go", files[1].Path)

	// Verify we can still add and remove after index rebuild.
	require.NoError(t, b.Add("b.go"))
	assert.Len(t, b.Files(), 3)
}

func TestBuilder_Clear(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "b.go", "package b\n")
	require.NoError(t, b.Add("a.go"))
	require.NoError(t, b.Add("b.go"))

	b.Clear()
	assert.Empty(t, b.Files())
	assert.Equal(t, 0, b.TotalTokens())
}

func TestBuilder_TotalTokens(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "a.go", "one two three\n")
	writeFile(t, root, "b.go", "four five\n")
	require.NoError(t, b.Add("a.go"))
	require.NoError(t, b.Add("b.go"))

	total := b.TotalTokens()
	// Should be sum of individual token counts.
	files := b.Files()
	expected := files[0].Tokens + files[1].Tokens
	assert.Equal(t, expected, total)
	assert.Greater(t, total, 0)
}

func TestBuilder_Export_Empty(t *testing.T) {
	b, _ := newTestBuilder(t)
	assert.Equal(t, "", b.Export())
}

func TestBuilder_Export_Format(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "main.go", "package main\n")
	writeFile(t, root, "README.md", "# Hello\n")
	require.NoError(t, b.Add("main.go"))
	require.NoError(t, b.Add("README.md"))

	export := b.Export()

	// Header line.
	assert.True(t, strings.HasPrefix(export, "# Context (2 files,"))
	assert.Contains(t, export, "tokens)")

	// File sections.
	assert.Contains(t, export, "## main.go")
	assert.Contains(t, export, "```go\n")
	assert.Contains(t, export, "package main\n")

	assert.Contains(t, export, "## README.md")
	assert.Contains(t, export, "```markdown\n")
	assert.Contains(t, export, "# Hello\n")
}

func TestBuilder_Export_NoTrailingNewline(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "no_nl.txt", "no trailing newline")
	require.NoError(t, b.Add("no_nl.txt"))

	export := b.Export()
	// Should have closing fence on its own line.
	assert.Contains(t, export, "no trailing newline\n```\n")
}

func TestBuilder_FilesReturnsCopy(t *testing.T) {
	b, root := newTestBuilder(t)
	writeFile(t, root, "a.go", "package a\n")
	require.NoError(t, b.Add("a.go"))

	files := b.Files()
	files[0].Path = "modified"

	// Original should be unaffected.
	assert.Equal(t, "a.go", b.Files()[0].Path)
}

func TestLangFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".py", "python"},
		{".rs", "rust"},
		{".json", "json"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".md", "markdown"},
		{".unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			assert.Equal(t, tt.want, langFromExt(tt.ext))
		})
	}
}
