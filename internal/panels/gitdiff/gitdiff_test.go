package gitdiff

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

// keyMsg creates a tea.KeyPressMsg for testing.
func keyMsg(key string) tea.KeyPressMsg {
	if len(key) == 1 {
		return tea.KeyPressMsg{Text: key, Code: rune(key[0])}
	}
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	default:
		return tea.KeyPressMsg{Text: key}
	}
}

// sampleDiff returns a FileDiff with context, added, and removed lines
// for testing.
func sampleDiff() git.FileDiff {
	return git.FileDiff{
		Path: "file.go",
		Hunks: []git.Hunk{
			{
				OldStart: 10,
				OldLines: 3,
				NewStart: 10,
				NewLines: 4,
				Header:   "@@ -10,3 +10,4 @@",
				Lines: []git.DiffLine{
					{Type: git.DiffLineContext, Content: "context line", OldLine: 10, NewLine: 10},
					{Type: git.DiffLineRemoved, Content: "removed line", OldLine: 11, NewLine: 0},
					{Type: git.DiffLineAdded, Content: "added line", OldLine: 0, NewLine: 11},
					{Type: git.DiffLineAdded, Content: "another added line", OldLine: 0, NewLine: 12},
					{Type: git.DiffLineContext, Content: "context line 2", OldLine: 12, NewLine: 13},
				},
			},
		},
	}
}

// sampleMultiFileDiff returns two FileDiffs for testing multi-file navigation.
func sampleMultiFileDiff() []git.FileDiff {
	return []git.FileDiff{
		{
			Path: "first.go",
			Hunks: []git.Hunk{
				{
					Header: "@@ -1,2 +1,3 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineContext, Content: "line 1", OldLine: 1, NewLine: 1},
						{Type: git.DiffLineAdded, Content: "new line", OldLine: 0, NewLine: 2},
						{Type: git.DiffLineContext, Content: "line 2", OldLine: 2, NewLine: 3},
					},
				},
			},
		},
		{
			Path: "second.go",
			Hunks: []git.Hunk{
				{
					Header: "@@ -5,2 +5,2 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineRemoved, Content: "old value", OldLine: 5, NewLine: 0},
						{Type: git.DiffLineAdded, Content: "new value", OldLine: 0, NewLine: 5},
					},
				},
			},
		},
	}
}

// sampleMultiHunkDiff returns a FileDiff with two hunks for testing hunk navigation.
func sampleMultiHunkDiff() git.FileDiff {
	return git.FileDiff{
		Path: "multi.go",
		Hunks: []git.Hunk{
			{
				Header: "@@ -1,3 +1,3 @@",
				Lines: []git.DiffLine{
					{Type: git.DiffLineContext, Content: "first", OldLine: 1, NewLine: 1},
					{Type: git.DiffLineRemoved, Content: "old a", OldLine: 2, NewLine: 0},
					{Type: git.DiffLineAdded, Content: "new a", OldLine: 0, NewLine: 2},
				},
			},
			{
				Header: "@@ -20,2 +20,3 @@",
				Lines: []git.DiffLine{
					{Type: git.DiffLineContext, Content: "middle", OldLine: 20, NewLine: 20},
					{Type: git.DiffLineAdded, Content: "inserted", OldLine: 0, NewLine: 21},
				},
			},
		},
	}
}

// newTestPanel creates a GitDiff panel for testing with optional theme.
func newTestPanel(th *theme.Theme) *GitDiff {
	p := New(nil, th)
	p.Init(context.Background())
	return p
}

// loadTestTheme loads the default theme for tests.
func loadTestTheme(t *testing.T) *theme.Theme {
	t.Helper()
	th, err := theme.Load("default")
	require.NoError(t, err)
	return th
}

// mockGitClient satisfies git.StatusReader for testing the async load path.
type mockGitClient = gittest.MockClient

// --- Construction tests ---

func TestNew(t *testing.T) {
	p := New(nil, nil)
	assert.NotNil(t, p)
	assert.Equal(t, "gitdiff", p.Title())
}

func TestNewWithDeps(t *testing.T) {
	th := loadTestTheme(t)
	mock := &mockGitClient{}
	p := New(mock, th)
	assert.NotNil(t, p)
	assert.Equal(t, "gitdiff", p.Title())
}

func TestInitReturnsNil(t *testing.T) {
	p := New(nil, nil)
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

// --- Compile-time interface check ---

func TestImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*GitDiff)(nil)
}

// --- View state tests ---

func TestViewEmptyState(t *testing.T) {
	p := newTestPanel(nil)
	view := p.View(80, 24)
	assert.Contains(t, view, "No file selected")
}

func TestViewLoadingState(t *testing.T) {
	p := newTestPanel(nil)
	p.loading = true
	view := p.View(80, 24)
	assert.Contains(t, view, "Loading diff...")
}

func TestViewErrorState(t *testing.T) {
	p := newTestPanel(nil)
	p.err = assert.AnError
	view := p.View(80, 24)
	assert.Contains(t, view, "Error:")
}

func TestViewZeroDimensions(t *testing.T) {
	p := newTestPanel(nil)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(80, 0))
	assert.Empty(t, p.View(0, 24))
}

func TestViewNoChanges(t *testing.T) {
	p := newTestPanel(nil)
	p.path = "file.go"
	p.diffs = []git.FileDiff{} // empty diff = no changes
	view := p.View(80, 24)
	assert.Contains(t, view, "No changes")
}

// --- Inline view rendering ---

func TestInlineViewRendering(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	view := p.View(80, 24)

	// File header
	assert.Contains(t, view, "file.go")
	// Hunk header
	assert.Contains(t, view, "@@ -10,3 +10,4 @@")
	// Line content (with prefix)
	assert.Contains(t, view, "context line")
	assert.Contains(t, view, "removed line")
	assert.Contains(t, view, "added line")
	assert.Contains(t, view, "another added line")
}

func TestInlineViewPrefixes(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Check the rendered lines directly for prefixes
	var hasAddedPrefix, hasRemovedPrefix, hasContextPrefix bool
	for _, line := range p.lines {
		if strings.Contains(line, "+ added line") || strings.Contains(line, "+ another added line") {
			hasAddedPrefix = true
		}
		if strings.Contains(line, "- removed line") {
			hasRemovedPrefix = true
		}
		if strings.Contains(line, "  context line") {
			hasContextPrefix = true
		}
	}

	assert.True(t, hasAddedPrefix, "should have + prefix for added lines")
	assert.True(t, hasRemovedPrefix, "should have - prefix for removed lines")
	assert.True(t, hasContextPrefix, "should have space prefix for context lines")
}

// --- Side-by-side view rendering ---

func TestSideBySideViewRendering(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(100, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Toggle to side-by-side
	p.Focus()
	p.Update(keyMsg("t"))

	assert.Equal(t, viewSideBySide, p.mode)

	view := p.View(100, 24)

	// File header should show old/new labels
	assert.Contains(t, view, "(old)")
	assert.Contains(t, view, "(new)")
	// Content should still be present
	assert.Contains(t, view, "context line")
}

// --- Toggle view mode ---

func TestToggleViewMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	assert.Equal(t, viewInline, p.mode, "should start in inline mode")

	// Toggle to side-by-side
	p.Update(keyMsg("t"))
	assert.Equal(t, viewSideBySide, p.mode, "should be side-by-side after toggle")

	// Toggle back to inline
	p.Update(keyMsg("t"))
	assert.Equal(t, viewInline, p.mode, "should be inline after second toggle")
}

func TestToggleRebuildLines(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	inlineLines := len(p.lines)

	p.Update(keyMsg("t"))
	sideBySideLines := len(p.lines)

	// Both modes should produce non-empty output
	assert.Greater(t, inlineLines, 0)
	assert.Greater(t, sideBySideLines, 0)
}

// --- Binary file handling ---

func TestBinaryFileHandling(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{
		{
			Path:     "image.png",
			IsBinary: true,
		},
	})

	view := p.View(80, 24)
	assert.Contains(t, view, "Binary file differs")
}

func TestBinaryFileSideBySide(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(100, 24)
	p.mode = viewSideBySide
	p.SetDiffs([]git.FileDiff{
		{
			Path:     "data.bin",
			IsBinary: true,
		},
	})

	view := p.View(100, 24)
	assert.Contains(t, view, "Binary file differs")
}

// --- Scroll navigation ---

func TestScrollDown(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5) // small viewport to force scrolling
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	assert.Equal(t, 0, p.scrollY)

	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.scrollY)

	p.Update(keyMsg("j"))
	assert.Equal(t, 2, p.scrollY)
}

func TestScrollUp(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.scrollY = 3
	p.Update(keyMsg("k"))
	assert.Equal(t, 2, p.scrollY)
}

func TestScrollUpClampsAtZero(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.Update(keyMsg("k"))
	assert.Equal(t, 0, p.scrollY, "scroll should not go below 0")
}

func TestPageDown(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.Update(keyMsg("pgdown"))
	assert.Greater(t, p.scrollY, 0, "page down should scroll forward")
}

func TestPageUp(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.scrollY = 5
	p.Update(keyMsg("pgup"))
	assert.Less(t, p.scrollY, 5, "page up should scroll backward")
}

func TestGotoTop(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.scrollY = 5
	p.Update(keyMsg("g"))
	assert.Equal(t, 0, p.scrollY, "g should go to top")
}

func TestGotoBottom(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	p.Update(keyMsg("G"))
	maxScroll := len(p.lines) - p.pageSize()
	if maxScroll < 0 {
		maxScroll = 0
	}
	assert.Equal(t, maxScroll, p.scrollY, "G should go to bottom")
}

func TestScrollClampsToMax(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	// Scroll way past the end
	p.scrollY = 10000
	p.clampScroll()
	maxScroll := len(p.lines) - p.pageSize()
	if maxScroll < 0 {
		maxScroll = 0
	}
	assert.Equal(t, maxScroll, p.scrollY)
}

// --- Hunk navigation ---

func TestNextHunk(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30) // big enough to see all
	p.SetDiffs([]git.FileDiff{sampleMultiHunkDiff()})
	p.Focus()

	require.GreaterOrEqual(t, len(p.hunkStarts), 2, "should have at least 2 hunks")

	p.scrollY = 0
	p.Update(keyMsg("n"))
	assert.Equal(t, p.hunkStarts[0], p.scrollY, "n should jump to first hunk")

	p.Update(keyMsg("n"))
	assert.Equal(t, p.hunkStarts[1], p.scrollY, "n should jump to second hunk")
}

func TestPrevHunk(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30)
	p.SetDiffs([]git.FileDiff{sampleMultiHunkDiff()})
	p.Focus()

	require.GreaterOrEqual(t, len(p.hunkStarts), 2, "should have at least 2 hunks")

	// Go to second hunk first
	p.scrollY = p.hunkStarts[1] + 1
	p.Update(keyMsg("N"))
	assert.Equal(t, p.hunkStarts[1], p.scrollY, "N should jump back to second hunk start")
}

// --- File navigation ---

func TestNextFile(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30)
	p.SetDiffs(sampleMultiFileDiff())
	p.Focus()

	assert.Equal(t, 0, p.fileIdx)

	p.Update(keyMsg("]"))
	assert.Equal(t, 1, p.fileIdx, "] should move to next file")
	assert.Equal(t, p.fileStarts[1], p.scrollY, "scroll should jump to next file start")
}

func TestPrevFile(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30)
	p.SetDiffs(sampleMultiFileDiff())
	p.Focus()

	p.fileIdx = 1
	p.Update(keyMsg("["))
	assert.Equal(t, 0, p.fileIdx, "[ should move to previous file")
	assert.Equal(t, p.fileStarts[0], p.scrollY, "scroll should jump to previous file start")
}

func TestNextFileClampsAtEnd(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30)
	p.SetDiffs(sampleMultiFileDiff())
	p.Focus()

	p.fileIdx = len(p.diffs) - 1
	p.Update(keyMsg("]"))
	assert.Equal(t, len(p.diffs)-1, p.fileIdx, "] should not go past last file")
}

func TestPrevFileClampsAtStart(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 30)
	p.SetDiffs(sampleMultiFileDiff())
	p.Focus()

	p.fileIdx = 0
	p.Update(keyMsg("["))
	assert.Equal(t, 0, p.fileIdx, "[ should not go before first file")
}

// --- Message handling ---

func TestShowDiffMsgTriggersLoad(t *testing.T) {
	diffResult := []git.FileDiff{sampleDiff()}
	var diffErr error
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return diffResult, diffErr
		},
	}
	p := New(mock, nil)
	p.Init(context.Background())
	p.SetSize(80, 24)

	// Send ShowDiffMsg
	_, cmd := p.Update(panels.ShowDiffMsg{Path: "file.go", Staged: true})
	require.NotNil(t, cmd, "ShowDiffMsg should return an async command")

	assert.True(t, p.loading)
	assert.Equal(t, "file.go", p.path)
	assert.True(t, p.staged)

	// Execute the command and feed result back
	msg := cmd()
	p.Update(msg)

	assert.False(t, p.loading, "should not be loading after result")
	assert.Len(t, p.diffs, 1)
	assert.Nil(t, p.err)
}

func TestFileSelectedMsgTriggersLoad(t *testing.T) {
	diffResult := []git.FileDiff{sampleDiff()}
	var diffErr error
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return diffResult, diffErr
		},
	}
	p := New(mock, nil)
	p.Init(context.Background())
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.FileSelectedMsg{Path: "file.go"})
	require.NotNil(t, cmd)

	assert.Equal(t, "file.go", p.path)
	assert.False(t, p.staged, "FileSelectedMsg should default to unstaged")

	// Execute and feed back
	msg := cmd()
	p.Update(msg)

	assert.Len(t, p.diffs, 1)
}

func TestNoGitClientReturnsError(t *testing.T) {
	p := New(nil, nil)
	p.Init(context.Background())
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.ShowDiffMsg{Path: "file.go"})
	require.NotNil(t, cmd)

	msg := cmd()
	p.Update(msg)

	assert.NotNil(t, p.err, "should have error with nil git client")
	assert.Contains(t, p.err.Error(), "no git client configured")
}

func TestDiffLoadedMsgIgnoresStaleResult(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.path = "current.go"

	// Send a result for a different path
	p.Update(diffLoadedMsg{
		path:  "stale.go",
		diffs: []git.FileDiff{sampleDiff()},
	})

	assert.Empty(t, p.diffs, "should ignore diff result for different path")
}

// --- Title ---

func TestTitleDefault(t *testing.T) {
	p := New(nil, nil)
	assert.Equal(t, "gitdiff", p.Title())
}

func TestTitleWithFile(t *testing.T) {
	p := New(nil, nil)
	p.path = "/path/to/file.go"
	assert.Equal(t, "file.go", p.Title())
}

func TestTitleStagedFile(t *testing.T) {
	p := New(nil, nil)
	p.path = "/path/to/file.go"
	p.staged = true
	assert.Equal(t, "file.go (staged)", p.Title())
}

// --- KeyBindings ---

func TestKeyBindings(t *testing.T) {
	p := New(nil, nil)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify essential bindings are present
	actionSet := make(map[string]bool)
	for _, b := range bindings {
		actionSet[b.Action] = true
	}
	assert.True(t, actionSet["scroll"], "should have scroll binding")
	assert.True(t, actionSet["toggle_view"], "should have toggle_view binding")
	assert.True(t, actionSet["hunk_nav"], "should have hunk_nav binding")
	assert.True(t, actionSet["file_nav"], "should have file_nav binding")
}

// --- SetSize ---

func TestSetSizeTriggersRebuild(t *testing.T) {
	p := newTestPanel(nil)
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Switch to side-by-side (which depends on width)
	p.mode = viewSideBySide

	// Set size should trigger rebuild
	p.SetSize(120, 30)
	assert.Equal(t, 120, p.Width)
	assert.Equal(t, 30, p.Height)
	assert.NotEmpty(t, p.lines, "lines should be rebuilt after SetSize")
}

// --- SetDiffs (test helper) ---

func TestSetDiffs(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)

	p.SetDiffs([]git.FileDiff{sampleDiff()})

	assert.Len(t, p.diffs, 1)
	assert.False(t, p.loading)
	assert.Nil(t, p.err)
	assert.Equal(t, 0, p.scrollY)
	assert.NotEmpty(t, p.lines)
}

// --- Focus behavior ---

func TestIgnoresKeysWhenBlurred(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 5)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	// Panel starts blurred (Focused = false)

	p.Update(keyMsg("j"))
	assert.Equal(t, 0, p.scrollY, "blurred panel should ignore key events")
}

// --- Empty diff in hunk ---

func TestEmptyHunksShowMessage(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{
		{
			Path:  "empty.go",
			Hunks: nil, // no hunks
		},
	})

	view := p.View(80, 24)
	assert.Contains(t, view, "No changes")
}

// --- Rename file header ---

func TestRenamedFileHeader(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{
		{
			Path:    "new_name.go",
			OldPath: "old_name.go",
			Hunks: []git.Hunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineContext, Content: "content", OldLine: 1, NewLine: 1},
					},
				},
			},
		},
	})

	view := p.View(80, 24)
	assert.Contains(t, view, "old_name.go")
	assert.Contains(t, view, "new_name.go")
	assert.Contains(t, view, "→")
}

// --- Theme integration ---

func TestThemeColorsApplied(t *testing.T) {
	th := loadTestTheme(t)
	p := newTestPanel(th)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Verify theme styles are used (non-nil theme)
	assert.NotEqual(t, lipgloss.Style{}, p.addedStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.removedStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.contextStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.headerStyle())
}

func TestFallbackColorsWithNilTheme(t *testing.T) {
	p := newTestPanel(nil)
	// Should not panic with nil theme
	assert.NotEqual(t, lipgloss.Style{}, p.addedStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.removedStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.contextStyle())
	assert.NotEqual(t, lipgloss.Style{}, p.headerStyle())
}

// --- Scroll indicator ---

func TestScrollIndicatorTop(t *testing.T) {
	p := newTestPanel(nil)
	p.scrollY = 0
	ind := p.scrollIndicator(100, 10)
	assert.Equal(t, "Top", ind)
}

func TestScrollIndicatorBottom(t *testing.T) {
	p := newTestPanel(nil)
	p.scrollY = 91 // 100 - (10-1) = 91
	ind := p.scrollIndicator(100, 10)
	assert.Equal(t, "Bot", ind)
}

func TestScrollIndicatorFitsInView(t *testing.T) {
	p := newTestPanel(nil)
	p.scrollY = 0
	ind := p.scrollIndicator(5, 10)
	assert.Empty(t, ind, "no indicator needed when content fits")
}

// --- pairDiffLines ---

func TestPairDiffLinesContextOnly(t *testing.T) {
	lines := []git.DiffLine{
		{Type: git.DiffLineContext, Content: "a", OldLine: 1, NewLine: 1},
		{Type: git.DiffLineContext, Content: "b", OldLine: 2, NewLine: 2},
	}
	pairs := pairDiffLines(lines)
	assert.Len(t, pairs, 2)
	// Context lines appear on both sides
	for _, pair := range pairs {
		assert.NotNil(t, pair.old)
		assert.NotNil(t, pair.new)
	}
}

func TestPairDiffLinesChangeBlock(t *testing.T) {
	lines := []git.DiffLine{
		{Type: git.DiffLineRemoved, Content: "old", OldLine: 1},
		{Type: git.DiffLineAdded, Content: "new", OldLine: 0, NewLine: 1},
	}
	pairs := pairDiffLines(lines)
	assert.Len(t, pairs, 1)
	assert.NotNil(t, pairs[0].old, "should have old side")
	assert.NotNil(t, pairs[0].new, "should have new side")
	assert.Equal(t, "old", pairs[0].old.Content)
	assert.Equal(t, "new", pairs[0].new.Content)
}

func TestPairDiffLinesUnbalancedBlock(t *testing.T) {
	lines := []git.DiffLine{
		{Type: git.DiffLineRemoved, Content: "old1", OldLine: 1},
		{Type: git.DiffLineRemoved, Content: "old2", OldLine: 2},
		{Type: git.DiffLineAdded, Content: "new1", OldLine: 0, NewLine: 1},
	}
	pairs := pairDiffLines(lines)
	assert.Len(t, pairs, 2)
	// First pair: old1 + new1
	assert.NotNil(t, pairs[0].old)
	assert.NotNil(t, pairs[0].new)
	// Second pair: old2 + nil (no corresponding new line)
	assert.NotNil(t, pairs[1].old)
	assert.Nil(t, pairs[1].new)
}

// --- Truncate/pad helpers ---

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel…", truncate("hello", 4))
	assert.Equal(t, "h", truncate("hello", 1))
}

func TestPadRight(t *testing.T) {
	assert.Equal(t, "hi   ", padRight("hi", 5))
	assert.Equal(t, "hello", padRight("hello", 5))
	assert.Equal(t, "hello!", padRight("hello!", 3)) // no truncation
}

// --- AI Review Annotation tests ---

func sampleFindings() []panels.AIReviewFinding {
	return []panels.AIReviewFinding{
		{
			File:       "file.go",
			Line:       11,
			Severity:   "warning",
			Category:   "security",
			Message:    "SQL injection risk",
			Suggestion: "use parameterized query",
		},
		{
			File:       "file.go",
			Line:       12,
			Severity:   "error",
			Category:   "bug",
			Message:    "nil pointer dereference",
			Suggestion: "add nil check",
		},
		{
			File:       "other.go",
			Line:       5,
			Severity:   "info",
			Category:   "style",
			Message:    "consider renaming",
			Suggestion: "",
		},
	}
}

func TestAIReviewReadyMsgStoresFindings(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.path = "file.go"
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	msg := panels.AIReviewReadyMsg{Findings: sampleFindings()}
	p.Update(msg)

	// Should store only findings for "file.go", not "other.go"
	assert.Len(t, p.reviewFindings, 2)
	for _, f := range p.reviewFindings {
		assert.Equal(t, "file.go", f.File)
	}
}

func TestAIReviewReadyMsgFiltersToCurrentFile(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.path = "other.go"
	p.SetDiffs([]git.FileDiff{{Path: "other.go"}})

	msg := panels.AIReviewReadyMsg{Findings: sampleFindings()}
	p.Update(msg)

	assert.Len(t, p.reviewFindings, 1)
	assert.Equal(t, "other.go", p.reviewFindings[0].File)
}

func TestToggleReviewAnnotations(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{sampleDiff()})
	p.Focus()

	assert.False(t, p.showReviewAnnotations, "annotations should be off by default")

	p.Update(keyMsg("R"))
	assert.True(t, p.showReviewAnnotations, "R should toggle annotations on")

	p.Update(keyMsg("R"))
	assert.False(t, p.showReviewAnnotations, "R should toggle annotations off")
}

func TestViewIncludesAnnotationsWhenEnabled(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 40)
	p.path = "file.go"
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Inject findings and enable annotations
	p.reviewFindings = []panels.AIReviewFinding{
		{
			File:       "file.go",
			Line:       11,
			Severity:   "warning",
			Category:   "security",
			Message:    "SQL injection risk",
			Suggestion: "use parameterized query",
		},
	}
	p.showReviewAnnotations = true
	p.rebuildLines()

	view := p.View(80, 40)

	assert.Contains(t, view, "⚠")
	assert.Contains(t, view, "[security]")
	assert.Contains(t, view, "SQL injection risk")
	assert.Contains(t, view, "use parameterized query")
}

func TestViewUnchangedWhenAnnotationsDisabled(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 40)
	p.path = "file.go"
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	// Capture baseline view without annotations
	baselineView := p.View(80, 40)
	baselineLines := len(p.lines)

	// Add findings but keep annotations disabled
	p.reviewFindings = []panels.AIReviewFinding{
		{
			File:     "file.go",
			Line:     11,
			Severity: "warning",
			Category: "security",
			Message:  "SQL injection risk",
		},
	}
	p.showReviewAnnotations = false
	p.rebuildLines()

	view := p.View(80, 40)

	assert.Equal(t, baselineLines, len(p.lines), "line count should be unchanged when annotations disabled")
	assert.Equal(t, baselineView, view, "view should be identical when annotations disabled")
	assert.NotContains(t, view, "[security]")
}

func TestAnnotationSeverityIcons(t *testing.T) {
	tests := []struct {
		severity string
		icon     string
	}{
		{"error", "✗"},
		{"warning", "⚠"},
		{"info", "ℹ"},
		{"hint", "›"},
		{"unknown", "⚠"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			assert.Equal(t, tt.icon, severityIcon(tt.severity))
		})
	}
}

func TestAnnotationsInSideBySideMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(120, 40)
	p.path = "file.go"
	p.mode = viewSideBySide
	p.reviewFindings = []panels.AIReviewFinding{
		{
			File:     "file.go",
			Line:     11,
			Severity: "error",
			Category: "bug",
			Message:  "nil deref",
		},
	}
	p.showReviewAnnotations = true
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	view := p.View(120, 40)

	assert.Contains(t, view, "✗")
	assert.Contains(t, view, "[bug]")
	assert.Contains(t, view, "nil deref")
}

func TestNoFindingsNoAnnotationLines(t *testing.T) {
	p := newTestPanel(nil)
	p.SetSize(80, 40)
	p.path = "file.go"
	p.SetDiffs([]git.FileDiff{sampleDiff()})

	linesWithoutAnnotations := len(p.lines)

	// Enable annotations with no findings
	p.showReviewAnnotations = true
	p.rebuildLines()

	assert.Equal(t, linesWithoutAnnotations, len(p.lines),
		"no annotation lines should be added when there are no findings")
}

func TestKeyBindingsIncludeToggleReview(t *testing.T) {
	p := New(nil, nil)
	bindings := p.KeyBindings()
	found := false
	for _, b := range bindings {
		if b.Action == "toggle_review" {
			found = true
			assert.Equal(t, "R", b.Key)
			break
		}
	}
	assert.True(t, found, "should have toggle_review binding")
}

func TestAnnotationWithoutSuggestion(t *testing.T) {
	p := newTestPanel(nil)
	f := panels.AIReviewFinding{
		File:     "file.go",
		Line:     11,
		Severity: "info",
		Category: "style",
		Message:  "consider renaming",
	}
	rendered := p.renderAnnotation(f)

	assert.Contains(t, rendered, "ℹ")
	assert.Contains(t, rendered, "[style]")
	assert.Contains(t, rendered, "consider renaming")
	assert.NotContains(t, rendered, "—")
}

// ---------------------------------------------------------------------------
// Mouse wheel tests
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollsDown(t *testing.T) {
	p := newTestPanel(nil)
	p.Focused = true
	p.SetSize(80, 3) // Small viewport to ensure content exceeds it.

	// Load multiple diffs to have enough content to scroll.
	p.diffs = sampleMultiFileDiff()
	p.rebuildLines()

	require.Greater(t, len(p.lines), 3, "need more lines than viewport for scroll test")
	assert.Equal(t, 0, p.scrollY)
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, p.scrollY, 0, "wheel down should increase scrollY")
}

func TestMouseWheel_ScrollsUp(t *testing.T) {
	p := newTestPanel(nil)
	p.Focused = true
	p.SetSize(80, 3)

	p.diffs = sampleMultiFileDiff()
	p.rebuildLines()

	p.scrollY = 5
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Less(t, p.scrollY, 5, "wheel up should decrease scrollY")
}

func TestMouseWheel_UpAtZero(t *testing.T) {
	p := newTestPanel(nil)
	p.Focused = true
	p.SetSize(80, 3)

	p.diffs = sampleMultiFileDiff()
	p.rebuildLines()

	p.scrollY = 0
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.scrollY, "scrollY should not go below 0")
}

// ---------------------------------------------------------------------------
// RepoChangedMsg
// ---------------------------------------------------------------------------

func TestRepoChangedMsg_ClearsState(t *testing.T) {
	p := newTestPanel(nil)
	p.path = "old/file.go"
	p.diffs = []git.FileDiff{sampleDiff()}
	p.rebuildLines()

	tmpDir := t.TempDir()
	result, cmd := p.Update(panels.RepoChangedMsg{Path: tmpDir})
	d := result.(*GitDiff)

	assert.Equal(t, "", d.path, "path should be cleared")
	assert.Nil(t, d.diffs, "diffs should be cleared")
	assert.Nil(t, d.lines, "lines should be cleared")
	assert.Equal(t, 0, d.scrollY, "scrollY should be reset")
	assert.False(t, d.loading, "loading should be false")
	assert.Nil(t, d.err, "err should be nil")
	assert.Nil(t, cmd, "no command should be returned")
}

// --- Copy hunk ---

func TestHunkToPatch(t *testing.T) {
	h := git.Hunk{
		Header: "@@ -1,3 +1,3 @@",
		Lines: []git.DiffLine{
			{Type: git.DiffLineContext, Content: "ctx"},
			{Type: git.DiffLineRemoved, Content: "old"},
			{Type: git.DiffLineAdded, Content: "new"},
		},
	}
	want := "@@ -1,3 +1,3 @@\n ctx\n-old\n+new"
	assert.Equal(t, want, hunkToPatch(h))
}

func TestCurrentHunk_MapsScrollToEnclosingHunk(t *testing.T) {
	p := newTestPanel(nil)
	p.SetDiffs([]git.FileDiff{sampleMultiHunkDiff()})
	require.Len(t, p.hunkStarts, 2, "sample has two hunks")

	// At or before the first hunk start → first hunk.
	p.scrollY = 0
	h, ok := p.currentHunk()
	require.True(t, ok)
	assert.Equal(t, "@@ -1,3 +1,3 @@", h.Header)

	// At the second hunk's start line → second hunk.
	p.scrollY = p.hunkStarts[1]
	h, ok = p.currentHunk()
	require.True(t, ok)
	assert.Equal(t, "@@ -20,2 +20,3 @@", h.Header)

	// Scrolled past the end → clamps to the last hunk.
	p.scrollY = 10000
	h, ok = p.currentHunk()
	require.True(t, ok)
	assert.Equal(t, "@@ -20,2 +20,3 @@", h.Header)
}

func TestCurrentHunk_NoDiff(t *testing.T) {
	p := newTestPanel(nil)
	_, ok := p.currentHunk()
	assert.False(t, ok, "no diff loaded → no current hunk")
}

func TestCopyCurrentHunk_NoDiff_Warns(t *testing.T) {
	p := newTestPanel(nil)
	_, cmd := p.copyCurrentHunk()
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	assert.Equal(t, notify.Warn, toast.Level)
}

func TestCopyCurrentHunk_WithDiff_ReturnsCommand(t *testing.T) {
	p := newTestPanel(nil)
	p.SetDiffs([]git.FileDiff{sampleMultiHunkDiff()})
	_, cmd := p.copyCurrentHunk()
	require.NotNil(t, cmd, "a copy command should be returned when a diff is loaded")
}
