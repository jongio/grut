package fuzzyfinder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTodoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestTodoSource_ScansMarkers(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n" + // line 1
		"// TODO: wire up the thing\n" + // line 2
		"func main() {}\n" + // line 3
		"// FIXME broken on windows\n" + // line 4
		"// HACK temporary shim\n" + // line 5
		"// BUG off-by-one here\n" + // line 6
		"// XXX revisit\n" // line 7
	writeTodoFile(t, dir, "main.go", content)

	items := NewTodoSource(dir).Items()
	require.Len(t, items, 5)

	byLine := make(map[int]Item)
	for _, it := range items {
		assert.Equal(t, categoryTodo, it.Category)
		byLine[it.Line] = it
	}
	assert.Equal(t, "TODO", byLine[2].Description)
	assert.Contains(t, byLine[2].Text, "wire up the thing")
	assert.Equal(t, "FIXME", byLine[4].Description)
	assert.Equal(t, "HACK", byLine[5].Description)
	assert.Equal(t, "BUG", byLine[6].Description)
	assert.Equal(t, "XXX", byLine[7].Description)
}

func TestTodoSource_ValuePointsAtAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := writeTodoFile(t, dir, "notes.go", "// TODO fix\n")

	items := NewTodoSource(dir).Items()
	require.Len(t, items, 1)
	assert.Equal(t, path, items[0].Value)
	assert.Equal(t, 1, items[0].Line)
}

func TestTodoSource_IgnoresBinary(t *testing.T) {
	dir := t.TempDir()
	// A NUL byte in the first chunk marks the file as binary.
	writeTodoFile(t, dir, "blob.bin", "\x00// TODO should be ignored\n")

	items := NewTodoSource(dir).Items()
	assert.Empty(t, items)
}

func TestTodoSource_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	writeTodoFile(t, dir, ".gitignore", "ignored.go\n")
	writeTodoFile(t, dir, "ignored.go", "// TODO hidden by gitignore\n")
	writeTodoFile(t, dir, "kept.go", "// TODO visible\n")

	items := NewTodoSource(dir).Items()
	require.Len(t, items, 1)
	assert.Contains(t, items[0].Text, "visible")
}

func TestTodoSource_SkipsNonNavigableDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "node_modules", "pkg")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeTodoFile(t, nested, "index.js", "// TODO in dependency\n")
	writeTodoFile(t, dir, "app.go", "// TODO in source\n")

	items := NewTodoSource(dir).Items()
	require.Len(t, items, 1)
	assert.Contains(t, items[0].Text, "in source")
}

func TestTodoSource_NilAndEmptyRoot(t *testing.T) {
	var ts *TodoSource
	assert.Nil(t, ts.Items())
	assert.Nil(t, NewTodoSource("").Items())
}

func TestMatchTodoLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantMarker  string
		wantMessage string
		wantOK      bool
	}{
		{"todo with colon", "// TODO: do the work", "TODO", "do the work", true},
		{"fixme no colon", "  # FIXME broken", "FIXME", "broken", true},
		{"marker at end", "value // XXX", "XXX", "", true},
		{"embedded in word not matched", "autoTODOlist := 1", "", "", false},
		{"lowercase not matched", "// todo lowercase", "", "", false},
		{"no marker", "just a normal line", "", "", false},
		{"hack marker", "//HACK:patch", "HACK", "patch", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marker, message, ok := matchTodoLine(tc.line)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantMarker, marker)
			assert.Equal(t, tc.wantMessage, message)
		})
	}
}

func TestIsBinary(t *testing.T) {
	assert.True(t, isBinary([]byte{'a', 0, 'b'}))
	assert.False(t, isBinary([]byte("plain text content")))
	assert.False(t, isBinary(nil))
}
