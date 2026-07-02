package gitstatus

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHunkText(t *testing.T) {
	h := git.Hunk{
		Header: "@@ -1,3 +1,4 @@ func main()",
		Lines: []git.DiffLine{
			{Type: git.DiffLineContext, Content: "ctx"},
			{Type: git.DiffLineRemoved, Content: "old"},
			{Type: git.DiffLineAdded, Content: "new1"},
			{Type: git.DiffLineAdded, Content: "new2"},
		},
	}
	want := "@@ -1,3 +1,4 @@ func main()\n ctx\n-old\n+new1\n+new2\n"
	assert.Equal(t, want, hunkText(h))
}

func TestHunkTextRebuildsMissingHeader(t *testing.T) {
	h := git.Hunk{
		OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 2,
		Lines: []git.DiffLine{{Type: git.DiffLineAdded, Content: "x"}},
	}
	got := hunkText(h)
	assert.True(t, strings.HasPrefix(got, "@@ -2,1 +2,2 @@\n"), "got %q", got)
}

func TestHunkTextNoNewlineEOF(t *testing.T) {
	h := git.Hunk{
		Header:       "@@ -1 +1 @@",
		NoNewlineEOF: true,
		Lines:        []git.DiffLine{{Type: git.DiffLineAdded, Content: "a"}},
	}
	assert.Contains(t, hunkText(h), "\\ No newline at end of file\n")
}

func TestHunkAtCursor(t *testing.T) {
	p := New(&mockGitClient{}, nil)
	file := &git.FileStatus{Path: "main.go"}
	hunks := []git.Hunk{{
		Header: "@@ -1,1 +1,2 @@",
		Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: "a"}},
	}}
	p.diffCache["main.go:unstaged"] = hunks
	p.rows = []row{
		{kind: rowSection, section: sectionUnstaged},
		{kind: rowFile, section: sectionUnstaged, file: file},
		{kind: rowHunk, section: sectionUnstaged, file: file, hunkIdx: 0, hunkEntry: &hunks[0]},
		{kind: rowDiffLine, section: sectionUnstaged, file: file, hunkIdx: 0, lineIdx: 0},
	}

	p.cursor = 2 // hunk header row
	h, ok := p.hunkAtCursor()
	require.True(t, ok)
	assert.Equal(t, "@@ -1,1 +1,2 @@", h.Header)

	p.cursor = 3 // diff line row resolves to its parent hunk
	h, ok = p.hunkAtCursor()
	require.True(t, ok)
	assert.Equal(t, "@@ -1,1 +1,2 @@", h.Header)

	p.cursor = 1 // file row has no single hunk
	_, ok = p.hunkAtCursor()
	assert.False(t, ok)

	p.cursor = 0 // section header row has no hunk
	_, ok = p.hunkAtCursor()
	assert.False(t, ok)
}

func TestCopyHunkKey(t *testing.T) {
	p := New(&mockGitClient{}, nil)
	p.ctx = context.Background()
	p.Focus()
	file := &git.FileStatus{Path: "main.go"}
	hunks := []git.Hunk{{
		Header: "@@ -1,1 +1,2 @@",
		Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: "a"}},
	}}
	p.diffCache["main.go:unstaged"] = hunks
	p.rows = []row{
		{kind: rowHunk, section: sectionUnstaged, file: file, hunkIdx: 0, hunkEntry: &hunks[0]},
	}
	p.cursor = 0

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'y'})
	require.NotNil(t, cmd, "expected a command from y on a hunk row")
}

func TestCopyAtCursorFallsBackToPath(t *testing.T) {
	p := New(&mockGitClient{}, nil)
	p.ctx = context.Background()
	p.Focus()
	file := &git.FileStatus{Path: "main.go"}
	p.rows = []row{
		{kind: rowFile, section: sectionUnstaged, file: file},
	}
	p.cursor = 0

	// On a file row there is no hunk, so copyAtCursor should still return a
	// command (the copy-path path).
	_, cmd := p.copyAtCursor()
	require.NotNil(t, cmd)
}
