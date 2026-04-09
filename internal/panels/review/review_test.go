package review

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGit struct {
	diffs      []git.FileDiff
	diffErr    error
	stageErr   error
	unstageErr error

	// Recorded calls for verification.
	stagePaths   []string
	unstagePaths []string
}

func (m *mockGit) Status(_ context.Context) ([]git.FileStatus, error) {
	return nil, nil
}

func (m *mockGit) Diff(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
	return m.diffs, m.diffErr
}

func (m *mockGit) Stage(_ context.Context, paths []string) error {
	m.stagePaths = paths
	return m.stageErr
}

func (m *mockGit) Unstage(_ context.Context, paths []string) error {
	m.unstagePaths = paths
	return m.unstageErr
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

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
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "escape":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		return tea.KeyPressMsg{Text: key}
	}
}

// sampleDiffs returns two FileDiffs for testing review workflows.
func sampleDiffs() []git.FileDiff {
	return []git.FileDiff{
		{
			Path: "file1.go",
			Hunks: []git.Hunk{
				{
					OldStart: 1, OldLines: 3,
					NewStart: 1, NewLines: 4,
					Header: "@@ -1,3 +1,4 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineContext, Content: "package main", OldLine: 1, NewLine: 1},
						{Type: git.DiffLineRemoved, Content: "old line", OldLine: 2},
						{Type: git.DiffLineAdded, Content: "new line", NewLine: 2},
						{Type: git.DiffLineAdded, Content: "another new", NewLine: 3},
						{Type: git.DiffLineContext, Content: "end", OldLine: 3, NewLine: 4},
					},
				},
				{
					OldStart: 10, OldLines: 2,
					NewStart: 11, NewLines: 3,
					Header: "@@ -10,2 +11,3 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineContext, Content: "func main() {", OldLine: 10, NewLine: 11},
						{Type: git.DiffLineAdded, Content: "    fmt.Println(\"hello\")", NewLine: 12},
					},
				},
			},
		},
		{
			Path: "file2.go",
			Hunks: []git.Hunk{
				{
					OldStart: 5, OldLines: 2,
					NewStart: 5, NewLines: 2,
					Header: "@@ -5,2 +5,2 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineRemoved, Content: "old value", OldLine: 5},
						{Type: git.DiffLineAdded, Content: "new value", NewLine: 5},
					},
				},
			},
		},
	}
}

// sampleReviewFiles builds ReviewFile entries from sampleDiffs.
func sampleReviewFiles() []ReviewFile {
	diffs := sampleDiffs()
	files := make([]ReviewFile, len(diffs))
	for i, d := range diffs {
		files[i] = ReviewFile{
			Path:       d.Path,
			Diff:       d,
			HunkStates: make([]HunkState, len(d.Hunks)),
		}
	}
	return files
}

// newTestPanel creates a review panel initialised for testing.
func newTestPanel(gc gitOps) *Panel {
	p := New(gc, nil)
	p.Init(context.Background())
	return p
}

// ---------------------------------------------------------------------------
// Construction and interface compliance
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	p := New(nil, nil)
	assert.NotNil(t, p)
	assert.Equal(t, "review", p.Title())
}

func TestImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

func TestInitReturnsNil(t *testing.T) {
	p := New(nil, nil)
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// File list rendering
// ---------------------------------------------------------------------------

func TestFileListPendingIndicator(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())

	view := p.View(80, 24)
	assert.Contains(t, view, "○")
	assert.Contains(t, view, "file1.go")
	assert.Contains(t, view, "file2.go")
}

func TestFileListApprovedIndicator(t *testing.T) {
	p := newTestPanel(nil)
	files := sampleReviewFiles()
	for i := range files[0].HunkStates {
		files[0].HunkStates[i] = HunkApproved
	}
	p.SetFiles(files)

	view := p.View(80, 24)
	assert.Contains(t, view, "✓")
}

func TestFileListRejectedIndicator(t *testing.T) {
	p := newTestPanel(nil)
	files := sampleReviewFiles()
	for i := range files[0].HunkStates {
		files[0].HunkStates[i] = HunkRejected
	}
	p.SetFiles(files)

	view := p.View(80, 24)
	assert.Contains(t, view, "✗")
}

func TestFileListMixedIndicator(t *testing.T) {
	p := newTestPanel(nil)
	files := sampleReviewFiles()
	files[0].HunkStates[0] = HunkApproved // first approved
	// second stays pending
	p.SetFiles(files)

	view := p.View(80, 24)
	assert.Contains(t, view, "◐")
}

// ---------------------------------------------------------------------------
// Navigation between files
// ---------------------------------------------------------------------------

func TestFileNavDown(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	assert.Equal(t, 0, p.fileCursor)
	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.fileCursor)
}

func TestFileNavUp(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.fileCursor = 1
	p.rebuildLines()

	p.Update(keyMsg("k"))
	assert.Equal(t, 0, p.fileCursor)
}

func TestFileNavClampsAtBounds(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	// Can't go below 0
	p.Update(keyMsg("k"))
	assert.Equal(t, 0, p.fileCursor)

	// Go to last
	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.fileCursor)

	// Can't go past last
	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.fileCursor)
}

func TestFileNavWithArrowKeys(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("down"))
	assert.Equal(t, 1, p.fileCursor)

	p.Update(keyMsg("up"))
	assert.Equal(t, 0, p.fileCursor)
}

// ---------------------------------------------------------------------------
// Next/previous file (n/N keys)
// ---------------------------------------------------------------------------

func TestNextFileInFileListMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("n"))
	assert.Equal(t, 1, p.fileCursor)
}

func TestPrevFileInFileListMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.fileCursor = 1
	p.rebuildLines()

	p.Update(keyMsg("N"))
	assert.Equal(t, 0, p.fileCursor)
}

// ---------------------------------------------------------------------------
// Expand file (enter diff mode)
// ---------------------------------------------------------------------------

func TestEnterExpandsFile(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	assert.Equal(t, modeFileList, p.mode)

	p.Update(keyMsg("enter"))
	assert.Equal(t, modeDiff, p.mode)
	assert.Equal(t, 0, p.hunkCursor)
}

func TestDiffViewShowsHunks(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 40)
	p.Focus()

	p.Update(keyMsg("enter"))

	view := p.View(80, 40)
	assert.Contains(t, view, "file1.go")
	assert.Contains(t, view, "Hunk 1/2")
	assert.Contains(t, view, "Hunk 2/2")
}

// ---------------------------------------------------------------------------
// Hunk navigation
// ---------------------------------------------------------------------------

func TestHunkNavDown(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	assert.Equal(t, 0, p.hunkCursor)

	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.hunkCursor)
}

func TestHunkNavUp(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))
	p.Update(keyMsg("j")) // go to hunk 1
	assert.Equal(t, 1, p.hunkCursor)

	p.Update(keyMsg("k")) // back to hunk 0
	assert.Equal(t, 0, p.hunkCursor)
}

func TestHunkNavBrackets(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))

	p.Update(keyMsg("]"))
	assert.Equal(t, 1, p.hunkCursor)

	p.Update(keyMsg("["))
	assert.Equal(t, 0, p.hunkCursor)
}

func TestHunkNavClampsAtBounds(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))

	// Can't go below 0
	p.Update(keyMsg("["))
	assert.Equal(t, 0, p.hunkCursor)

	// Go to last hunk
	p.Update(keyMsg("]"))
	assert.Equal(t, 1, p.hunkCursor)

	// Can't go past last
	p.Update(keyMsg("]"))
	assert.Equal(t, 1, p.hunkCursor)
}

// ---------------------------------------------------------------------------
// Approve hunk
// ---------------------------------------------------------------------------

func TestApproveHunk(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	p.Update(keyMsg("a"))     // approve hunk 0

	assert.Equal(t, HunkApproved, p.files[0].HunkStates[0])
	assert.Equal(t, HunkPending, p.files[0].HunkStates[1])
}

func TestApproveUpdatesViewIndicator(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 40)
	p.Focus()

	p.Update(keyMsg("enter"))
	p.Update(keyMsg("a"))

	view := p.View(80, 40)
	assert.Contains(t, view, "✓")
}

// ---------------------------------------------------------------------------
// Reject hunk
// ---------------------------------------------------------------------------

func TestRejectHunk(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	p.Update(keyMsg("x"))     // reject hunk 0

	assert.Equal(t, HunkRejected, p.files[0].HunkStates[0])
	assert.Equal(t, HunkPending, p.files[0].HunkStates[1])
}

func TestRejectUpdatesViewIndicator(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 40)
	p.Focus()

	p.Update(keyMsg("enter"))
	p.Update(keyMsg("x"))

	view := p.View(80, 40)
	assert.Contains(t, view, "✗")
}

// ---------------------------------------------------------------------------
// Approve all hunks in file
// ---------------------------------------------------------------------------

func TestApproveAll(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	p.Update(keyMsg("A"))     // approve all

	for _, s := range p.files[0].HunkStates {
		assert.Equal(t, HunkApproved, s)
	}
}

func TestApproveAllStagesFile(t *testing.T) {
	mock := &mockGit{}
	p := newTestPanel(mock)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))
	_, cmd := p.Update(keyMsg("A"))

	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(stageResultMsg)
	assert.True(t, ok, "should return stageResultMsg")
	assert.NoError(t, result.err)
	assert.Equal(t, []string{"file1.go"}, mock.stagePaths)
}

// ---------------------------------------------------------------------------
// Reject all hunks in file
// ---------------------------------------------------------------------------

func TestRejectAll(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	p.Update(keyMsg("X"))     // reject all

	for _, s := range p.files[0].HunkStates {
		assert.Equal(t, HunkRejected, s)
	}
}

func TestRejectAllUnstagesFile(t *testing.T) {
	mock := &mockGit{}
	p := newTestPanel(mock)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))
	_, cmd := p.Update(keyMsg("X"))

	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(stageResultMsg)
	assert.True(t, ok, "should return stageResultMsg")
	assert.NoError(t, result.err)
	assert.Equal(t, []string{"file1.go"}, mock.unstagePaths)
}

// ---------------------------------------------------------------------------
// Stage on all hunks individually approved
// ---------------------------------------------------------------------------

func TestStageOnAllApproved(t *testing.T) {
	mock := &mockGit{}
	p := newTestPanel(mock)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter"))

	// Approve first hunk — not all approved yet, no stage command.
	_, cmd1 := p.Update(keyMsg("a"))
	assert.Nil(t, cmd1)
	assert.Equal(t, HunkApproved, p.files[0].HunkStates[0])

	// Move to second hunk and approve — now all approved.
	p.Update(keyMsg("j"))
	_, cmd2 := p.Update(keyMsg("a"))
	require.NotNil(t, cmd2)

	msg := cmd2()
	result, ok := msg.(stageResultMsg)
	assert.True(t, ok)
	assert.NoError(t, result.err)
	assert.Equal(t, []string{"file1.go"}, mock.stagePaths)
}

// ---------------------------------------------------------------------------
// Next/previous file in diff mode
// ---------------------------------------------------------------------------

func TestNextFileInDiffMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // diff mode on file 0

	p.Update(keyMsg("n")) // next file
	assert.Equal(t, 1, p.fileCursor)
	assert.Equal(t, 0, p.hunkCursor)  // reset hunk cursor
	assert.Equal(t, modeDiff, p.mode) // stay in diff mode
}

func TestPrevFileInDiffMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	// Navigate to file 1 and enter diff
	p.Update(keyMsg("j"))
	p.Update(keyMsg("enter"))

	p.Update(keyMsg("N")) // prev file
	assert.Equal(t, 0, p.fileCursor)
}

// ---------------------------------------------------------------------------
// Summary generation
// ---------------------------------------------------------------------------

func TestSummaryAllApproved(t *testing.T) {
	files := sampleReviewFiles()
	for i := range files {
		for j := range files[i].HunkStates {
			files[i].HunkStates[j] = HunkApproved
		}
	}

	summary := GenerateSummary(files)
	assert.Contains(t, summary, "# Review Summary")
	assert.Contains(t, summary, "## Approved")
	assert.Contains(t, summary, "file1.go")
	assert.Contains(t, summary, "file2.go")
	assert.Contains(t, summary, "hunks approved")
}

func TestSummaryAllRejected(t *testing.T) {
	files := sampleReviewFiles()
	for i := range files {
		for j := range files[i].HunkStates {
			files[i].HunkStates[j] = HunkRejected
		}
	}

	summary := GenerateSummary(files)
	assert.Contains(t, summary, "## Rejected")
	assert.Contains(t, summary, "hunks rejected")
}

func TestSummaryMixed(t *testing.T) {
	files := sampleReviewFiles()
	files[0].HunkStates[0] = HunkApproved
	files[0].HunkStates[1] = HunkRejected

	summary := GenerateSummary(files)
	assert.Contains(t, summary, "## Approved")
	assert.Contains(t, summary, "## Rejected")
	assert.Contains(t, summary, "## Pending")
}

func TestSummaryEmpty(t *testing.T) {
	summary := GenerateSummary(nil)
	assert.Contains(t, summary, "No changes to review")
}

func TestSummaryAllPending(t *testing.T) {
	files := sampleReviewFiles()

	summary := GenerateSummary(files)
	assert.Contains(t, summary, "## Pending")
	assert.Contains(t, summary, "unreviewed")
}

func TestSummaryRejectedShowsLineRange(t *testing.T) {
	files := sampleReviewFiles()
	files[0].HunkStates[1] = HunkRejected // reject second hunk

	summary := GenerateSummary(files)
	assert.Contains(t, summary, "hunk 2: lines 11-13")
}

// ---------------------------------------------------------------------------
// Show / dismiss summary overlay
// ---------------------------------------------------------------------------

func TestShowSummary(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("s"))
	assert.True(t, p.showSummary)

	view := p.View(80, 24)
	assert.Contains(t, view, "Review Summary")
}

func TestDismissSummaryWithQ(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("s"))
	assert.True(t, p.showSummary)

	p.Update(keyMsg("q"))
	assert.False(t, p.showSummary)
}

func TestDismissSummaryWithEscape(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("s"))
	assert.True(t, p.showSummary)

	p.Update(keyMsg("escape"))
	assert.False(t, p.showSummary)
}

func TestDismissSummaryWithEnter(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("s"))
	assert.True(t, p.showSummary)

	p.Update(keyMsg("enter"))
	assert.False(t, p.showSummary)
}

// ---------------------------------------------------------------------------
// Empty review
// ---------------------------------------------------------------------------

func TestEmptyReview(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(nil)

	view := p.View(80, 24)
	assert.Contains(t, view, "No changes to review")
}

func TestEmptyReviewEmptySlice(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles([]ReviewFile{})

	view := p.View(80, 24)
	assert.Contains(t, view, "No changes to review")
}

// ---------------------------------------------------------------------------
// Zero dimensions
// ---------------------------------------------------------------------------

func TestViewZeroDimensions(t *testing.T) {
	p := newTestPanel(nil)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(80, 0))
	assert.Empty(t, p.View(0, 24))
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	p := New(nil, nil)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/k"])
	assert.True(t, keys["enter"])
	assert.True(t, keys["a"])
	assert.True(t, keys["x"])
	assert.True(t, keys["A"])
	assert.True(t, keys["X"])
	assert.True(t, keys["n/N"])
	assert.True(t, keys["[/]"])
	assert.True(t, keys["s"])
	assert.True(t, keys["q"])
}

// ---------------------------------------------------------------------------
// Unfocused key ignore
// ---------------------------------------------------------------------------

func TestUnfocusedKeyIgnore(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	// Not focused — keys should be ignored.

	assert.Equal(t, 0, p.fileCursor)
	p.Update(keyMsg("j"))
	assert.Equal(t, 0, p.fileCursor, "should ignore keys when not focused")
}

// ---------------------------------------------------------------------------
// q exits review mode
// ---------------------------------------------------------------------------

func TestQExitsReview(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	_, cmd := p.Update(keyMsg("q"))
	require.NotNil(t, cmd)

	msg := cmd()
	complete, ok := msg.(panels.ReviewCompleteMsg)
	assert.True(t, ok, "should emit ReviewCompleteMsg")
	assert.Contains(t, complete.Summary, "Review Summary")
}

// ---------------------------------------------------------------------------
// Escape goes back from diff to file list
// ---------------------------------------------------------------------------

func TestEscapeGoesBackToFileList(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	assert.Equal(t, modeDiff, p.mode)

	p.Update(keyMsg("escape"))
	assert.Equal(t, modeFileList, p.mode)
}

func TestQInDiffGoesBackToFileList(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode
	assert.Equal(t, modeDiff, p.mode)

	p.Update(keyMsg("q"))
	assert.Equal(t, modeFileList, p.mode)
}

// ---------------------------------------------------------------------------
// Loading state
// ---------------------------------------------------------------------------

func TestViewLoadingState(t *testing.T) {
	p := newTestPanel(nil)
	p.loading = true

	view := p.View(80, 24)
	assert.Contains(t, view, "Loading changes...")
}

func TestViewErrorState(t *testing.T) {
	p := newTestPanel(nil)
	p.err = assert.AnError

	view := p.View(80, 24)
	assert.Contains(t, view, "Error:")
}

// ---------------------------------------------------------------------------
// Async load via StartReviewMsg
// ---------------------------------------------------------------------------

func TestStartReviewMsg(t *testing.T) {
	mock := &mockGit{diffs: sampleDiffs()}
	p := newTestPanel(mock)
	p.Focus()

	// Trigger review start
	_, cmd := p.Update(panels.StartReviewMsg{})
	require.NotNil(t, cmd)
	assert.True(t, p.loading)

	// Execute the load command synchronously
	msg := cmd()
	p.Update(msg)
	assert.False(t, p.loading)
	assert.Len(t, p.files, 2)
}

func TestStartReviewMsgNilClient(t *testing.T) {
	p := newTestPanel(nil) // no git client
	p.Focus()

	_, cmd := p.Update(panels.StartReviewMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	p.Update(msg)
	assert.NotNil(t, p.err)
}

// ---------------------------------------------------------------------------
// Binary file handling in diff view
// ---------------------------------------------------------------------------

func TestBinaryFileDiffView(t *testing.T) {
	p := newTestPanel(nil)
	files := []ReviewFile{
		{
			Path: "image.png",
			Diff: git.FileDiff{Path: "image.png", IsBinary: true},
		},
	}
	p.SetFiles(files)
	p.SetSize(80, 24)
	p.Focus()

	p.Update(keyMsg("enter")) // enter diff mode

	view := p.View(80, 24)
	assert.Contains(t, view, "Binary file differs")
}

// ---------------------------------------------------------------------------
// File status helpers
// ---------------------------------------------------------------------------

func TestFileStatusIconPending(t *testing.T) {
	f := ReviewFile{
		Path:       "test.go",
		HunkStates: []HunkState{HunkPending, HunkPending},
	}
	assert.Equal(t, "○", fileStatusIcon(f))
}

func TestFileStatusIconApproved(t *testing.T) {
	f := ReviewFile{
		Path:       "test.go",
		HunkStates: []HunkState{HunkApproved, HunkApproved},
	}
	assert.Equal(t, "✓", fileStatusIcon(f))
}

func TestFileStatusIconRejected(t *testing.T) {
	f := ReviewFile{
		Path:       "test.go",
		HunkStates: []HunkState{HunkRejected, HunkRejected},
	}
	assert.Equal(t, "✗", fileStatusIcon(f))
}

func TestFileStatusIconMixed(t *testing.T) {
	f := ReviewFile{
		Path:       "test.go",
		HunkStates: []HunkState{HunkApproved, HunkPending},
	}
	assert.Equal(t, "◐", fileStatusIcon(f))
}

func TestFileStatusIconEmpty(t *testing.T) {
	f := ReviewFile{
		Path:       "test.go",
		HunkStates: nil,
	}
	assert.Equal(t, "○", fileStatusIcon(f))
}

func TestFileStatusTextPending(t *testing.T) {
	f := ReviewFile{
		HunkStates: []HunkState{HunkPending, HunkPending},
	}
	assert.Equal(t, "(2 hunks)", fileStatusText(f))
}

func TestFileStatusTextPartial(t *testing.T) {
	f := ReviewFile{
		HunkStates: []HunkState{HunkApproved, HunkPending},
	}
	assert.Equal(t, "(1/2 reviewed)", fileStatusText(f))
}

func TestFileStatusTextEmpty(t *testing.T) {
	f := ReviewFile{HunkStates: nil}
	assert.Equal(t, "", fileStatusText(f))
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsFileInFileList(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 24)
	p.Focus()

	assert.Equal(t, 0, p.fileCursor)

	// File list lines: [0]=header, [1]=blank, [2]=file0, [3]=file1
	// Click on row 3 (scrollY=0, line 3 = file index 1).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 3, ContentCol: 5})
	assert.Equal(t, 1, p.fileCursor)
}

func TestMouseClick_IgnoresHeaderRows(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 24)

	// Click on row 0 (header line) — should not change cursor.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 0, p.fileCursor, "clicking header should not move cursor")

	// Click on row 1 (blank line) — should not change cursor.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.fileCursor, "clicking blank row should not move cursor")
}

func TestMouseDoubleClick_EntersDiffMode(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 24)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemReviewFile): true}

	// Double-click on row 3 (file index 1).
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 3, ContentCol: 5})
	assert.Equal(t, 1, p.fileCursor)
	assert.Equal(t, modeDiff, p.mode, "double-click should switch to diff mode")
}

func TestMouseDoubleClick_FirstUseShowsConfirm(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 24)
	// No confirmations — should prompt first.

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 3, ContentCol: 5})
	require.NotNil(t, cmd, "first-use double-click should produce a command")
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPickerWithCheckbox, modal.Kind)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	p := newTestPanel(nil)
	p.SetFiles(sampleReviewFiles())
	p.SetSize(80, 24)

	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.fileCursor, "out-of-bounds double-click should not change cursor")
	assert.Equal(t, modeFileList, p.mode, "should remain in file list mode")
}
