package git

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHunkPatch(t *testing.T) {
	hunk := Hunk{
		OldStart: 10,
		OldLines: 4,
		NewStart: 10,
		NewLines: 5,
		Header:   "@@ -10,4 +10,5 @@ func example()",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "line one"},
			{Type: DiffLineRemoved, Content: "old line"},
			{Type: DiffLineAdded, Content: "new line"},
			{Type: DiffLineAdded, Content: "extra line"},
			{Type: DiffLineContext, Content: "line four"},
		},
	}

	patch := buildHunkPatch("src/main.go", hunk)

	assert.Contains(t, patch, "diff --git a/src/main.go b/src/main.go\n")
	assert.Contains(t, patch, "--- a/src/main.go\n")
	assert.Contains(t, patch, "+++ b/src/main.go\n")
	assert.Contains(t, patch, "@@ -10,4 +10,5 @@ func example()\n")
	assert.Contains(t, patch, " line one\n")
	assert.Contains(t, patch, "-old line\n")
	assert.Contains(t, patch, "+new line\n")
	assert.Contains(t, patch, "+extra line\n")
	assert.Contains(t, patch, " line four\n")
}

func TestBuildHunkPatchWindowsPath(t *testing.T) {
	hunk := Hunk{
		OldStart: 1,
		OldLines: 1,
		NewStart: 1,
		NewLines: 2,
		Header:   "@@ -1,1 +1,2 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "x"},
			{Type: DiffLineAdded, Content: "y"},
		},
	}

	patch := buildHunkPatch("src\\sub\\file.go", hunk)
	assert.Contains(t, patch, "diff --git a/src/sub/file.go b/src/sub/file.go")
}

func TestBuildHunkPatchSanitizesInjectedHeaders(t *testing.T) {
	hunk := Hunk{
		OldStart: 1,
		OldLines: 1,
		NewStart: 1,
		NewLines: 1,
		Header:   "@@ -1,1 +1,1 @@\n@@ -9,9 +9,9 @@",
		Lines: []DiffLine{
			{Type: DiffLineAdded, Content: "safe line\n@@ -99,1 +99,1 @@\n+++ b/owned.go"},
		},
	}

	patch := buildHunkPatch("..\\..\\owned.go", hunk)

	assert.Equal(t, 1, strings.Count(patch, "diff --git "))
	assert.Equal(t, 1, strings.Count(patch, "\n@@"))
	assert.NotContains(t, patch, "../")
	// The legitimate header has one +++ line; the injected one is escaped
	// and won't appear as a standalone line in the patch.
	lines := strings.Split(patch, "\n")
	pppCount := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "+++ ") {
			pppCount++
		}
	}
	assert.Equal(t, 1, pppCount, "only the legitimate +++ header should be a standalone line")
	assert.Contains(t, patch, "diff --git a/owned.go b/owned.go")
	assert.Contains(t, patch, "+safe line\\n@@ -99,1 +99,1 @@\\n+++ b/owned.go")
}

func TestBuildLinePatchStageAddedLine(t *testing.T) {
	hunk := Hunk{
		OldStart: 5,
		OldLines: 3,
		NewStart: 5,
		NewLines: 4,
		Header:   "@@ -5,3 +5,4 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "context before"},
			{Type: DiffLineRemoved, Content: "removed line"},
			{Type: DiffLineAdded, Content: "added line one"},
			{Type: DiffLineAdded, Content: "added line two"},
			{Type: DiffLineContext, Content: "context after"},
		},
	}

	// Stage only "added line one" (index 2)
	patch := buildLinePatch("file.go", hunk, 2)

	// Non-selected removed line should become context.
	// Non-selected added line should be dropped.
	assert.Contains(t, patch, " context before\n")
	assert.Contains(t, patch, " removed line\n") // converted from -
	assert.Contains(t, patch, "+added line one\n")
	assert.NotContains(t, patch, "+added line two") // dropped
	assert.NotContains(t, patch, "-removed line")   // converted to context
	assert.Contains(t, patch, " context after\n")

	// Header should have recalculated counts.
	// Old: context before, removed line (context), context after = 3
	// New: context before, removed line (context), added line one, context after = 4
	assert.Contains(t, patch, "@@ -5,3 +5,4 @@")
}

func TestBuildLinePatchStageRemovedLine(t *testing.T) {
	hunk := Hunk{
		OldStart: 1,
		OldLines: 4,
		NewStart: 1,
		NewLines: 3,
		Header:   "@@ -1,4 +1,3 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "keep"},
			{Type: DiffLineRemoved, Content: "delete me"},
			{Type: DiffLineRemoved, Content: "also delete"},
			{Type: DiffLineAdded, Content: "new stuff"},
			{Type: DiffLineContext, Content: "end"},
		},
	}

	// Stage only "delete me" (index 1)
	patch := buildLinePatch("file.go", hunk, 1)

	assert.Contains(t, patch, " keep\n")
	assert.Contains(t, patch, "-delete me\n")
	assert.Contains(t, patch, " also delete\n") // other removed → context
	assert.NotContains(t, patch, "+new stuff")  // added → dropped
	assert.Contains(t, patch, " end\n")

	// Old: keep + delete me + also delete(context) + end = 4
	// New: keep + also delete(context) + end = 3
	assert.Contains(t, patch, "@@ -1,4 +1,3 @@")
}

func TestBuildLinePatchSingleAdded(t *testing.T) {
	hunk := Hunk{
		OldStart: 1,
		OldLines: 2,
		NewStart: 1,
		NewLines: 3,
		Header:   "@@ -1,2 +1,3 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "before"},
			{Type: DiffLineAdded, Content: "inserted"},
			{Type: DiffLineContext, Content: "after"},
		},
	}

	patch := buildLinePatch("x.go", hunk, 1)
	assert.Contains(t, patch, "+inserted\n")
	// Old: before + after = 2
	// New: before + inserted + after = 3
	assert.Contains(t, patch, "@@ -1,2 +1,3 @@")
}

func TestBuildLinePatchSanitizesInjectedHeaders(t *testing.T) {
	hunk := Hunk{
		OldStart: 3,
		OldLines: 2,
		NewStart: 3,
		NewLines: 3,
		Header:   "@@ -3,2 +3,3 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "before"},
			{Type: DiffLineAdded, Content: "inserted\n--- a/owned.go\n+++ b/owned.go"},
			{Type: DiffLineContext, Content: "after"},
		},
	}

	patch := buildLinePatch("folder/../safe.go", hunk, 1)

	assert.Equal(t, 1, strings.Count(patch, "diff --git "))
	assert.Equal(t, 1, strings.Count(patch, "\n@@"))
	assert.NotContains(t, patch, "folder/../safe.go")
	assert.NotContains(t, patch, "\n--- a/owned.go\n")
	assert.Contains(t, patch, "+inserted\\n--- a/owned.go\\n+++ b/owned.go")
}

func TestBuildLinePatchRejectsContextLine(t *testing.T) {
	hunk := Hunk{
		OldStart: 1,
		OldLines: 2,
		NewStart: 1,
		NewLines: 2,
		Header:   "@@ -1,2 +1,2 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "line"},
			{Type: DiffLineContext, Content: "line2"},
		},
	}

	// StageLine should reject context lines — tested at the Client level.
	// buildLinePatch itself doesn't validate, but the Client wrapper does.
	// This test just verifies the patch output is sensible for context-only hunk.
	patch := buildLinePatch("x.go", hunk, 0)
	require.Contains(t, patch, " line\n")
}

func TestStageLineRejectsContextLine(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)

	hunk := Hunk{
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "x"},
		},
	}

	err = c.StageLine(context.Background(), "file.go", hunk, 0)
	assert.ErrorContains(t, err, "cannot stage a context line")
}

func TestUnstageLineRejectsContextLine(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)

	hunk := Hunk{
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: "x"},
		},
	}

	err = c.UnstageLine(context.Background(), "file.go", hunk, 0)
	assert.ErrorContains(t, err, "cannot unstage a context line")
}

func TestStageLineRejectsOutOfRange(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)

	hunk := Hunk{
		Lines: []DiffLine{
			{Type: DiffLineAdded, Content: "x"},
		},
	}

	err = c.StageLine(context.Background(), "file.go", hunk, 5)
	assert.ErrorContains(t, err, "out of range")
}

func TestStageHunkRejectsInvalidPath(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)

	err = c.StageHunk(context.Background(), "../escape", Hunk{})
	assert.Error(t, err)
}
