package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiff_SingleFile(t *testing.T) {
	input := `diff --git a/file.go b/file.go
index abc123..def456 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
 
+import "fmt"
 func main() {
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)

	d := diffs[0]
	assert.Equal(t, "file.go", d.Path)
	assert.False(t, d.IsBinary)
	require.Len(t, d.Hunks, 1)

	h := d.Hunks[0]
	assert.Equal(t, 1, h.OldStart)
	assert.Equal(t, 3, h.OldLines)
	assert.Equal(t, 1, h.NewStart)
	assert.Equal(t, 4, h.NewLines)

	// Check lines: context, context, added, context
	require.Len(t, h.Lines, 4)
	assert.Equal(t, DiffLineContext, h.Lines[0].Type)
	assert.Equal(t, "package main", h.Lines[0].Content)
	assert.Equal(t, DiffLineContext, h.Lines[1].Type)
	assert.Equal(t, DiffLineAdded, h.Lines[2].Type)
	assert.Equal(t, `import "fmt"`, h.Lines[2].Content)
	assert.Equal(t, DiffLineContext, h.Lines[3].Type)
}

func TestParseDiff_Rename(t *testing.T) {
	input := `diff --git a/old.go b/new.go
similarity index 95%
rename from old.go
rename to new.go
index abc123..def456 100644
--- a/old.go
+++ b/new.go
@@ -1,3 +1,3 @@
 package main
 
-func oldName() {}
+func newName() {}
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)

	assert.Equal(t, "new.go", diffs[0].Path)
	assert.Equal(t, "old.go", diffs[0].OldPath)
	require.Len(t, diffs[0].Hunks, 1)

	h := diffs[0].Hunks[0]
	// 4 lines: context ("package main"), context (empty line), removed, added.
	require.Len(t, h.Lines, 4)
	assert.Equal(t, DiffLineContext, h.Lines[0].Type)
	assert.Equal(t, "package main", h.Lines[0].Content)
	assert.Equal(t, DiffLineContext, h.Lines[1].Type)
	assert.Equal(t, DiffLineRemoved, h.Lines[2].Type)
	assert.Equal(t, "func oldName() {}", h.Lines[2].Content)
	assert.Equal(t, DiffLineAdded, h.Lines[3].Type)
	assert.Equal(t, "func newName() {}", h.Lines[3].Content)
}

func TestParseDiff_Binary(t *testing.T) {
	input := `diff --git a/image.png b/image.png
new file mode 100644
index 0000000..abc1234
Binary files differ
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)

	assert.True(t, diffs[0].IsBinary)
	assert.Empty(t, diffs[0].Hunks)
}

func TestParseDiff_MultipleFiles(t *testing.T) {
	input := `diff --git a/a.go b/a.go
index abc123..def456 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 package a
+// added
 
diff --git a/b.go b/b.go
index abc123..def456 100644
--- a/b.go
+++ b/b.go
@@ -1,2 +1,2 @@
 package b
-// old
+// new
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 2)

	assert.Equal(t, "a.go", diffs[0].Path)
	assert.Equal(t, "b.go", diffs[1].Path)
}

func TestParseDiff_Empty(t *testing.T) {
	diffs, err := parseDiff("")
	require.NoError(t, err)
	assert.Empty(t, diffs)
}

func TestParseDiff_MultipleHunks(t *testing.T) {
	input := `diff --git a/file.go b/file.go
index abc..def 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+// comment 1
 
 func a() {}
@@ -10,3 +11,4 @@
 func b() {}
 
+// comment 2
 func c() {}
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)
	require.Len(t, diffs[0].Hunks, 2)

	assert.Equal(t, 1, diffs[0].Hunks[0].OldStart)
	assert.Equal(t, 10, diffs[0].Hunks[1].OldStart)
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		oldStart int
		oldLines int
		newStart int
		newLines int
	}{
		{
			name:     "standard",
			header:   "@@ -1,3 +1,4 @@ func main",
			oldStart: 1, oldLines: 3, newStart: 1, newLines: 4,
		},
		{
			name:     "single line old",
			header:   "@@ -5 +5,2 @@",
			oldStart: 5, oldLines: 1, newStart: 5, newLines: 2,
		},
		{
			name:     "large range",
			header:   "@@ -100,50 +110,60 @@",
			oldStart: 100, oldLines: 50, newStart: 110, newLines: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os, ol, ns, nl := parseHunkHeader(tt.header)
			assert.Equal(t, tt.oldStart, os, "oldStart")
			assert.Equal(t, tt.oldLines, ol, "oldLines")
			assert.Equal(t, tt.newStart, ns, "newStart")
			assert.Equal(t, tt.newLines, nl, "newLines")
		})
	}
}

func TestParseDiffHeader(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		path    string
		oldPath string
	}{
		{
			name:    "simple",
			line:    "diff --git a/file.go b/file.go",
			path:    "file.go",
			oldPath: "file.go",
		},
		{
			name:    "nested",
			line:    "diff --git a/internal/git/client.go b/internal/git/client.go",
			path:    "internal/git/client.go",
			oldPath: "internal/git/client.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, oldPath := parseDiffHeader(tt.line)
			assert.Equal(t, tt.path, path)
			assert.Equal(t, tt.oldPath, oldPath)
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input string
		start int
		count int
	}{
		{"1,3", 1, 3},
		{"5", 5, 1},
		{"100,0", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			s, c := parseRange(tt.input)
			assert.Equal(t, tt.start, s)
			assert.Equal(t, tt.count, c)
		})
	}
}

func TestParseDiff_LineNumbers(t *testing.T) {
	input := `diff --git a/file.go b/file.go
index abc..def 100644
--- a/file.go
+++ b/file.go
@@ -5,4 +5,5 @@
 context line
-removed line
+added line 1
+added line 2
 another context
`
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)

	h := diffs[0].Hunks[0]
	require.Len(t, h.Lines, 5)

	// Context line: old=5, new=5
	assert.Equal(t, 5, h.Lines[0].OldLine)
	assert.Equal(t, 5, h.Lines[0].NewLine)

	// Removed line: old=6, new=0
	assert.Equal(t, 6, h.Lines[1].OldLine)
	assert.Equal(t, 0, h.Lines[1].NewLine)

	// Added line 1: old=0, new=6
	assert.Equal(t, 0, h.Lines[2].OldLine)
	assert.Equal(t, 6, h.Lines[2].NewLine)

	// Added line 2: old=0, new=7
	assert.Equal(t, 0, h.Lines[3].OldLine)
	assert.Equal(t, 7, h.Lines[3].NewLine)

	// Context line: old=7, new=8
	assert.Equal(t, 7, h.Lines[4].OldLine)
	assert.Equal(t, 8, h.Lines[4].NewLine)
}
