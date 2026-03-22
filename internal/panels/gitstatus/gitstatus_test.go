package gitstatus

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGitClient struct {
	statusResult     []git.FileStatus
	statusErr        error
	diffResult       []git.FileDiff
	diffErr          error
	stageErr         error
	unstageErr       error
	discardErr       error
	stagedPaths      []string
	unstagePaths     []string
	discardedPaths   []string
	stageHunkCalls   []stageHunkCall
	unstageHunkCalls []stageHunkCall
	stageLineCalls   []stageLineCall
	unstageLineCalls []stageLineCall
}

type stageHunkCall struct {
	path string
	hunk git.Hunk
}

type stageLineCall struct {
	path    string
	hunk    git.Hunk
	lineIdx int
}

var _ GitClient = (*mockGitClient)(nil)

func (m *mockGitClient) Status(_ context.Context) ([]git.FileStatus, error) {
	return m.statusResult, m.statusErr
}

func (m *mockGitClient) Diff(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
	return m.diffResult, m.diffErr
}

func (m *mockGitClient) Log(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
	return nil, nil
}

func (m *mockGitClient) Blame(_ context.Context, _ string) ([]git.BlameLine, error) {
	return nil, nil
}

func (m *mockGitClient) RepoRoot(_ context.Context) (string, error) {
	return "/repo", nil
}

func (m *mockGitClient) IsRepo(_ context.Context) (bool, error) {
	return true, nil
}

func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) Stage(_ context.Context, paths []string) error {
	m.stagedPaths = append(m.stagedPaths, paths...)
	return m.stageErr
}

func (m *mockGitClient) Unstage(_ context.Context, paths []string) error {
	m.unstagePaths = append(m.unstagePaths, paths...)
	return m.unstageErr
}

func (m *mockGitClient) StageHunk(_ context.Context, path string, hunk git.Hunk) error {
	m.stageHunkCalls = append(m.stageHunkCalls, stageHunkCall{path: path, hunk: hunk})
	return m.stageErr
}

func (m *mockGitClient) UnstageHunk(_ context.Context, path string, hunk git.Hunk) error {
	m.unstageHunkCalls = append(m.unstageHunkCalls, stageHunkCall{path: path, hunk: hunk})
	return m.unstageErr
}

func (m *mockGitClient) StageLine(_ context.Context, path string, hunk git.Hunk, lineIdx int) error {
	m.stageLineCalls = append(m.stageLineCalls, stageLineCall{path: path, hunk: hunk, lineIdx: lineIdx})
	return m.stageErr
}

func (m *mockGitClient) UnstageLine(_ context.Context, path string, hunk git.Hunk, lineIdx int) error {
	m.unstageLineCalls = append(m.unstageLineCalls, stageLineCall{path: path, hunk: hunk, lineIdx: lineIdx})
	return m.unstageErr
}

func (m *mockGitClient) DiscardFile(_ context.Context, path string) error {
	m.discardedPaths = append(m.discardedPaths, path)
	return m.discardErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// keyMsg constructs a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// newTestPanel creates a GitStatus panel with a mock client and processes
// an initial status load synchronously.
func newTestPanel(t *testing.T, mock *mockGitClient) *GitStatus {
	t.Helper()
	p := New(mock)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return a command")
	// Execute the load command synchronously.
	msg := cmd()
	p.Update(msg)
	return p
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mock := &mockGitClient{}
	p := New(mock)
	assert.Equal(t, "gitstatus", p.Title())
	assert.NotNil(t, p.selected)
	assert.NotNil(t, p.expandedFiles)
	assert.NotNil(t, p.diffCache)
}

func TestInit_ReturnsLoadCmd(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "file.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := New(mock)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
	assert.True(t, p.loading)
}

func TestStatusLoaded_GroupsFiles(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
			{Path: "untracked.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
			{Path: "both.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)

	// Verify sections are present in correct order.
	assert.Greater(t, len(p.rows), 0)

	// Count section headers.
	sections := 0
	fileRows := 0
	for _, r := range p.rows {
		if r.kind == rowSection {
			sections++
		}
		if r.kind == rowFile {
			fileRows++
		}
	}
	// Both staged and unstaged sections should include "both.go".
	assert.Equal(t, 3, sections, "should have 3 section headers")
	assert.Equal(t, 5, fileRows, "should have 5 file rows (staged, unstaged, untracked, +both in staged and unstaged)")
}

func TestStatusLoaded_EmptyRepo(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{},
	}
	p := newTestPanel(t, mock)

	assert.Empty(t, p.rows)
	view := p.View(80, 24)
	assert.Contains(t, view, "Working tree clean")
}

func TestStatusLoaded_Error(t *testing.T) {
	mock := &mockGitClient{
		statusErr: assert.AnError,
	}
	p := newTestPanel(t, mock)

	view := p.View(80, 24)
	assert.Contains(t, view, "Error")
}

func TestView_Loading(t *testing.T) {
	mock := &mockGitClient{}
	p := New(mock)
	p.loading = true
	view := p.View(80, 24)
	assert.Contains(t, view, "Loading git status")
}

func TestView_ZeroDimensions(t *testing.T) {
	mock := &mockGitClient{}
	p := New(mock)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(10, 0))
}

func TestNavigation_JK(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "b.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "c.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Cursor should start at 1 (skip section header at 0).
	// rows[0] = section header, rows[1] = a.go, rows[2] = b.go, rows[3] = c.go
	assert.Equal(t, 1, p.cursor)

	// Move down — should land on first file (skip section header).
	p.Update(keyMsg('j'))
	// Cursor should now be at a file row (index 1 or later).
	assert.GreaterOrEqual(t, p.cursor, 1)

	prevCursor := p.cursor
	p.Update(keyMsg('j'))
	assert.Greater(t, p.cursor, prevCursor)

	// Move up.
	prevCursor = p.cursor
	p.Update(keyMsg('k'))
	assert.Less(t, p.cursor, prevCursor)
}

func TestNavigation_GotoTopBottom(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "b.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Go to bottom.
	p.Update(keyMsg('G'))
	assert.Equal(t, len(p.rows)-1, p.cursor)

	// Go to top (should skip section header).
	p.Update(keyMsg('g'))
	// Should be at index 1 (first file after section header).
	if len(p.rows) > 1 && p.rows[0].kind == rowSection {
		assert.Equal(t, 1, p.cursor)
	}
}

func TestStageFile_EmitsCommand(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Navigate to file row (skip section header).
	p.Update(keyMsg('j'))

	// Press 's' to stage.
	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd, "stage should return a command")

	// Execute the command.
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)

	// Verify the mock received the stage call.
	assert.Contains(t, mock.stagedPaths, "unstaged.go")
}

func TestUnstageFile_EmitsCommand(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Navigate to file row.
	p.Update(keyMsg('j'))

	// Press 'u' to unstage.
	_, cmd := p.Update(keyMsg('u'))
	require.NotNil(t, cmd, "unstage should return a command")

	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	assert.Contains(t, mock.unstagePaths, "staged.go")
}

func TestStageAll(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
			{Path: "b.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'a' to stage all.
	_, cmd := p.Update(keyMsg('a'))
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	assert.Len(t, mock.stagedPaths, 2)
}

func TestToggleSelection(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Move to file row.
	p.Update(keyMsg('j'))

	// Toggle selection with space.
	p.Update(keyMsg(' '))
	assert.Len(t, p.selected, 1)

	// Toggle again to deselect.
	p.Update(keyMsg(' '))
	assert.Len(t, p.selected, 0)
}

func TestRefresh(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'R' to refresh.
	_, cmd := p.Update(keyMsg('R'))
	require.NotNil(t, cmd)
	assert.True(t, p.loading)
}

func TestRefreshGitStatusMsg(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()

	// Receive RefreshGitStatusMsg.
	_, cmd := p.Update(panels.RefreshGitStatusMsg{})
	require.NotNil(t, cmd)
	assert.True(t, p.loading)
}

func TestExpandFile(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
		diffResult: []git.FileDiff{
			{
				Path: "a.go",
				Hunks: []git.Hunk{
					{
						Header:   "@@ -1,3 +1,4 @@",
						OldStart: 1, OldLines: 3,
						NewStart: 1, NewLines: 4,
						Lines: []git.DiffLine{
							{Type: git.DiffLineContext, Content: "package main", OldLine: 1, NewLine: 1},
							{Type: git.DiffLineAdded, Content: "// new comment", NewLine: 2},
						},
					},
				},
			},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Move to file row.
	p.Update(keyMsg('j'))

	// Press enter to expand.
	_, cmd := p.Update(keyMsg(tea.KeyEnter))
	require.NotNil(t, cmd, "expand should trigger a diff load command")

	// Verify expand is tracked.
	assert.True(t, p.expandedFiles["a.go:staged"])

	// Simulate diff loaded.
	msg := cmd()
	p.Update(msg)

	// Should have more rows now (section + file + hunk header + diff lines).
	assert.Greater(t, len(p.rows), 2)

	// Verify hunk and diff line rows exist.
	hasHunk := false
	hasDiffLine := false
	for _, r := range p.rows {
		if r.kind == rowHunk {
			hasHunk = true
		}
		if r.kind == rowDiffLine {
			hasDiffLine = true
		}
	}
	assert.True(t, hasHunk, "should have hunk rows after expand")
	assert.True(t, hasDiffLine, "should have diff line rows after expand")
}

func TestCollapseFile(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Manually expand and populate diff cache.
	p.expandedFiles["a.go:staged"] = true
	p.diffCache["a.go:staged"] = []git.Hunk{
		{Header: "@@ -1,1 +1,1 @@", Lines: []git.DiffLine{{Type: git.DiffLineAdded, Content: "new"}}},
	}
	p.rebuildRows()
	rowsBefore := len(p.rows)

	// Move to file row.
	p.cursor = 1 // file row is after section header

	// Press enter to collapse.
	p.Update(keyMsg(tea.KeyEnter))
	assert.Less(t, len(p.rows), rowsBefore)
	assert.False(t, p.expandedFiles["a.go:staged"])
}

func TestHunkMode(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Manually expand and populate diff cache.
	p.expandedFiles["a.go:staged"] = true
	p.diffCache["a.go:staged"] = []git.Hunk{
		{Header: "@@ -1,1 +1,1 @@", Lines: []git.DiffLine{
			{Type: git.DiffLineAdded, Content: "added"},
		}},
	}
	p.rebuildRows()

	// Move cursor to file row.
	p.cursor = 1

	// Press 'h' to enter hunk mode.
	p.Update(keyMsg('h'))
	assert.Equal(t, modeHunk, p.mode)

	// Press escape to exit.
	p.Update(keyMsg(tea.KeyEscape))
	assert.Equal(t, modeFile, p.mode)
}

func TestKeyBindings(t *testing.T) {
	mock := &mockGitClient{}
	p := New(mock)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Check that essential bindings are present.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["stage"])
	assert.True(t, actions["unstage"])
	assert.True(t, actions["cursor_down"])
	assert.True(t, actions["cursor_up"])
	assert.True(t, actions["refresh"])
}

func TestClassifyFiles(t *testing.T) {
	files := []git.FileStatus{
		{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		{Path: "untracked.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		{Path: "both.go", StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusModified},
	}

	staged, unstaged, untracked := classifyFiles(files)

	assert.Len(t, staged, 2, "staged.go and both.go")
	assert.Len(t, unstaged, 2, "unstaged.go and both.go")
	assert.Len(t, untracked, 1, "untracked.go")
}

func TestStatusIndicator(t *testing.T) {
	tests := []struct {
		file     git.FileStatus
		section  section
		expected string
	}{
		{
			file:     git.FileStatus{StagedStatus: git.StatusModified},
			section:  sectionStaged,
			expected: "M",
		},
		{
			file:     git.FileStatus{WorktreeStatus: git.StatusAdded},
			section:  sectionUnstaged,
			expected: "A",
		},
		{
			file:     git.FileStatus{StagedStatus: git.StatusUntracked},
			section:  sectionUntracked,
			expected: "?",
		},
	}

	for _, tt := range tests {
		result := statusIndicator(&tt.file, tt.section)
		assert.Equal(t, tt.expected, result)
	}
}

func TestUnfocusedKeyIgnored(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	// Don't focus.
	p.SetSize(80, 24)

	cursorBefore := p.cursor
	p.Update(keyMsg('j'))
	assert.Equal(t, cursorBefore, p.cursor, "unfocused panel should not process keys")
}

func TestStageOnSectionHeader_StagesSection(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Cursor at section header (index 0).
	p.cursor = 0
	_, cmd := p.Update(keyMsg('s'))
	assert.NotNil(t, cmd, "staging on section header should stage all files in section")
}

func TestExpandUntracked_NoOp(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "new.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Move to file row.
	p.Update(keyMsg('j'))

	// Try to expand — should be no-op for untracked.
	_, cmd := p.Update(keyMsg(tea.KeyEnter))
	assert.Nil(t, cmd, "expanding untracked file should be no-op")
}

func TestView_RendersContent(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "src/main.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "README.md", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	view := p.View(80, 24)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Staged")
	assert.Contains(t, view, "main.go")
	assert.Contains(t, view, "Untracked")
	assert.Contains(t, view, "README.md")
}

func TestStatusCodeLabel(t *testing.T) {
	tests := []struct {
		code     git.StatusCode
		expected string
	}{
		{git.StatusModified, "M"},
		{git.StatusAdded, "A"},
		{git.StatusDeleted, "D"},
		{git.StatusRenamed, "R"},
		{git.StatusCopied, "C"},
		{git.StatusUntracked, "?"},
		{git.StatusConflict, "U"},
		{git.StatusUnmodified, " "},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, statusCodeLabel(tt.code))
	}
}

func TestGitStatusChangedMsg_Emitted(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := New(mock)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)

	// Execute load command.
	msg := cmd()

	// Feed back into Update — should return emitStatusChanged cmd.
	_, cmd2 := p.Update(msg)
	require.NotNil(t, cmd2)

	result := cmd2()
	changedMsg, ok := result.(panels.GitStatusChangedMsg)
	require.True(t, ok, "should emit GitStatusChangedMsg")
	assert.Len(t, changedMsg.Files, 1)
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

// setupExpandedPanel returns a panel with one staged file already expanded,
// diff cached, and rows rebuilt. The rows contain section + file + hunk + difflines.
func setupExpandedPanel(t *testing.T) (*GitStatus, *mockGitClient) {
	t.Helper()
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "src/main.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Manually expand and populate diff cache with all three line types.
	p.expandedFiles["src/main.go:staged"] = true
	p.diffCache["src/main.go:staged"] = []git.Hunk{
		{
			Header:   "@@ -1,3 +1,4 @@",
			OldStart: 1, OldLines: 3,
			NewStart: 1, NewLines: 4,
			Lines: []git.DiffLine{
				{Type: git.DiffLineContext, Content: "package main", OldLine: 1, NewLine: 1},
				{Type: git.DiffLineRemoved, Content: "old line", OldLine: 2},
				{Type: git.DiffLineAdded, Content: "new line", NewLine: 2},
			},
		},
	}
	p.rebuildRows()
	return p, mock
}

func TestViewWithExpandedDiff_RendersHunkAndDiffLines(t *testing.T) {
	p, _ := setupExpandedPanel(t)

	view := p.View(80, 24)

	// Hunk header must appear.
	assert.Contains(t, view, "@@ -1,3 +1,4 @@")
	// Added line with "+" prefix.
	assert.Contains(t, view, "+new line")
	// Removed line with "-" prefix.
	assert.Contains(t, view, "-old line")
	// Context line content.
	assert.Contains(t, view, "package main")
}

func TestStageAtCursorOnHunkRow(t *testing.T) {
	p, mock := setupExpandedPanel(t)

	// Find the hunk row index.
	hunkIdx := -1
	for i, r := range p.rows {
		if r.kind == rowHunk {
			hunkIdx = i
			break
		}
	}
	require.NotEqual(t, -1, hunkIdx, "should have a hunk row")
	p.cursor = hunkIdx

	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd, "stage on hunk row should return a command")
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	assert.Contains(t, mock.stagedPaths, "src/main.go")
}

func TestStageAtCursorOnDiffLineRow(t *testing.T) {
	p, mock := setupExpandedPanel(t)

	// Find a diffLine row index.
	dlIdx := -1
	for i, r := range p.rows {
		if r.kind == rowDiffLine {
			dlIdx = i
			break
		}
	}
	require.NotEqual(t, -1, dlIdx, "should have a diff line row")
	p.cursor = dlIdx

	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd, "stage on diff line row should return a command")
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	assert.Contains(t, mock.stagedPaths, "src/main.go")
}

func TestUnstageAtCursorOnSectionHeader_UnstagesSection(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Cursor at section header (index 0).
	p.cursor = 0
	_, cmd := p.Update(keyMsg('u'))
	assert.NotNil(t, cmd, "unstage on section header should unstage all files in section")
}

func TestUnstageAtCursorOnHunkRow(t *testing.T) {
	p, mock := setupExpandedPanel(t)

	// Find the hunk row.
	for i, r := range p.rows {
		if r.kind == rowHunk {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('u'))
	require.NotNil(t, cmd, "unstage on hunk row should return a command")
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	// In the staged section, unstaging a hunk now uses UnstageHunk.
	require.Len(t, mock.unstageHunkCalls, 1)
	assert.Equal(t, "src/main.go", mock.unstageHunkCalls[0].path)
}

func TestExpandOrEnterOnSectionHeader(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Cursor at section header.
	p.cursor = 0
	_, cmd := p.Update(keyMsg(tea.KeyEnter))
	assert.Nil(t, cmd, "enter on section header should be a no-op")
}

func TestViewportScrolling(t *testing.T) {
	// Create enough files to overflow a small viewport.
	var files []git.FileStatus
	for i := 0; i < 20; i++ {
		files = append(files, git.FileStatus{
			Path:           fmt.Sprintf("file%02d.go", i),
			StagedStatus:   git.StatusModified,
			WorktreeStatus: git.StatusUnmodified,
		})
	}
	mock := &mockGitClient{statusResult: files}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 5) // Very small viewport — only 5 visible rows

	// Navigate to the bottom.
	p.Update(keyMsg('G'))
	assert.Equal(t, len(p.rows)-1, p.cursor)

	// Offset should have adjusted so cursor is visible.
	assert.Greater(t, p.offset, 0, "offset should scroll when cursor exceeds viewport")
	assert.LessOrEqual(t, p.offset, p.cursor, "offset must not exceed cursor")
	assert.GreaterOrEqual(t, p.cursor, p.offset, "cursor must be within viewport")
}

func TestEnterLineModeFromHunkRow(t *testing.T) {
	p, _ := setupExpandedPanel(t)

	// Navigate cursor to the hunk row.
	for i, r := range p.rows {
		if r.kind == rowHunk {
			p.cursor = i
			break
		}
	}

	// Press 'h' while on a hunk row → enters line mode (enterHunkMode's rowHunk case).
	p.Update(keyMsg('h'))
	assert.Equal(t, modeLine, p.mode, "should enter line mode from hunk row")
}

func TestDiffLoadedError(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	rowsBefore := len(p.rows)
	// Simulate a diffLoadedMsg with an error.
	p.Update(diffLoadedMsg{path: "a.go:staged", hunks: nil, err: assert.AnError})
	// Rows should not change on error.
	assert.Equal(t, rowsBefore, len(p.rows))
}

func TestStageResultError(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Simulate a stageResultMsg with an error.
	p.Update(stageResultMsg{err: assert.AnError})
	assert.Error(t, p.err, "should store the stage error")
}

func TestFileKeyUnstagedSection(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "b.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Navigate to file row.
	p.Update(keyMsg('j'))

	// Toggle selection to exercise fileKey for unstaged section.
	p.Update(keyMsg(' '))
	assert.True(t, p.selected["b.go:unstaged"], "fileKey should produce unstaged key")

	// Toggle back.
	p.Update(keyMsg(' '))
	assert.False(t, p.selected["b.go:unstaged"])
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsFile(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Row 0 is "Staged" header, row 1 is staged.go, row 2 is "Unstaged" header, row 3 is unstaged.go.
	// Click on the file row (row 1).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor, "click on file row should set cursor")
}

func TestMouseClick_SkipsHeader(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Row 0 is "Staged" section header.
	originalCursor := p.cursor
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "click on section header should not move cursor")
}

func TestMouseClick_OutOfBounds(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "out-of-bounds click should return nil cmd")
}

func TestMouseDoubleClick_StagesFile(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Rows: [0] "Unstaged" header, [1] unstaged.go file.
	// Double-click on the file → should trigger stageAtCursor.
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor, "double-click should set cursor")
	assert.NotNil(t, cmd, "double-click on file should trigger stage command")
}

func TestMouseDoubleClick_SkipsHeader(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Nil(t, cmd, "double-click on section header should be a no-op")
}

func TestMouseWheel_Down(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "b.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "c.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "d.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "e.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 3)

	assert.Equal(t, 0, p.offset)
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, p.offset, 0, "wheel down should increase offset")
}

func TestMouseWheel_Up(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "b.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "c.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 3)

	p.offset = 3
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "wheel up from 3 with delta 3 should reach 0")
}

func TestMouseWheel_UpAtTop(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.offset = 0
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "offset should not go below 0")
}

// ---------------------------------------------------------------------------
// Right-click tests
// ---------------------------------------------------------------------------

func TestRightClickShowsActionPicker(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Row 0 is "Unstaged" header, row 1 is the file.
	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 1, ContentCol: 5})
	require.NotNil(t, cmd, "right-click on file row should produce a command")

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
}

func TestRightClickOutOfBounds(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "right-click out of bounds should return nil cmd")
}

// ---------------------------------------------------------------------------
// Partial staging tests (hunk-level and line-level)
// ---------------------------------------------------------------------------

// setupExpandedUnstagedPanel returns a panel with one unstaged file expanded,
// diff cached, and rows rebuilt. Tests can verify hunk/line staging calls.
func setupExpandedUnstagedPanel(t *testing.T) (*GitStatus, *mockGitClient) {
	t.Helper()
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "app.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.expandedFiles["app.go:unstaged"] = true
	p.diffCache["app.go:unstaged"] = []git.Hunk{
		{
			Header:   "@@ -1,3 +1,4 @@",
			OldStart: 1, OldLines: 3,
			NewStart: 1, NewLines: 4,
			Lines: []git.DiffLine{
				{Type: git.DiffLineContext, Content: "package main", OldLine: 1, NewLine: 1},
				{Type: git.DiffLineRemoved, Content: "old line", OldLine: 2},
				{Type: git.DiffLineAdded, Content: "new line", NewLine: 2},
				{Type: git.DiffLineContext, Content: "end", OldLine: 3, NewLine: 3},
			},
		},
	}
	p.rebuildRows()
	return p, mock
}

func TestStageHunkFromUnstagedSection(t *testing.T) {
	p, mock := setupExpandedUnstagedPanel(t)

	// Find the hunk row in the unstaged section.
	for i, r := range p.rows {
		if r.kind == rowHunk && r.section == sectionUnstaged {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	require.Len(t, mock.stageHunkCalls, 1, "should call StageHunk, not Stage")
	assert.Equal(t, "app.go", mock.stageHunkCalls[0].path)
	assert.Empty(t, mock.stagedPaths, "should NOT call whole-file Stage")
}

func TestStageLineFromUnstagedSection(t *testing.T) {
	p, mock := setupExpandedUnstagedPanel(t)

	// Find the added diff line row in the unstaged section.
	for i, r := range p.rows {
		if r.kind == rowDiffLine && r.section == sectionUnstaged && r.diffLine != nil && r.diffLine.Type == git.DiffLineAdded {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	require.Len(t, mock.stageLineCalls, 1, "should call StageLine")
	assert.Equal(t, "app.go", mock.stageLineCalls[0].path)
	assert.Empty(t, mock.stagedPaths, "should NOT call whole-file Stage")
}

func TestStageContextLineFromUnstagedFallsBackToWholeFile(t *testing.T) {
	p, mock := setupExpandedUnstagedPanel(t)

	// Find a context diff line row in the unstaged section.
	for i, r := range p.rows {
		if r.kind == rowDiffLine && r.section == sectionUnstaged && r.diffLine != nil && r.diffLine.Type == git.DiffLineContext {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('s'))
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	// Context lines fall back to whole-file staging.
	assert.Contains(t, mock.stagedPaths, "app.go")
	assert.Empty(t, mock.stageLineCalls, "should NOT call StageLine for context")
}

func TestUnstageHunkFromStagedSection(t *testing.T) {
	p, mock := setupExpandedPanel(t)

	for i, r := range p.rows {
		if r.kind == rowHunk && r.section == sectionStaged {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('u'))
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	require.Len(t, mock.unstageHunkCalls, 1, "should call UnstageHunk")
	assert.Equal(t, "src/main.go", mock.unstageHunkCalls[0].path)
	assert.Empty(t, mock.unstagePaths, "should NOT call whole-file Unstage")
}

func TestUnstageLineFromStagedSection(t *testing.T) {
	p, mock := setupExpandedPanel(t)

	// Find the removed diff line in the staged section (it's a non-context line).
	for i, r := range p.rows {
		if r.kind == rowDiffLine && r.section == sectionStaged && r.diffLine != nil && r.diffLine.Type == git.DiffLineRemoved {
			p.cursor = i
			break
		}
	}

	_, cmd := p.Update(keyMsg('u'))
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, stageResultMsg{}, msg)
	require.Len(t, mock.unstageLineCalls, 1, "should call UnstageLine")
	assert.Equal(t, "src/main.go", mock.unstageLineCalls[0].path)
	assert.Empty(t, mock.unstagePaths, "should NOT call whole-file Unstage")
}

// ---------------------------------------------------------------------------
// RepoChangedMsg
// ---------------------------------------------------------------------------

func TestRepoChangedMsg_NonGitDir(t *testing.T) {
	mock := &mockGitClient{
		statusResult: []git.FileStatus{
			{Path: "file.go", WorktreeStatus: git.StatusModified},
		},
	}
	p := New(mock)
	p.Init(context.Background())

	tmpDir := t.TempDir()
	result, cmd := p.Update(panels.RepoChangedMsg{Path: tmpDir})
	gs := result.(*GitStatus)

	// git.NewClient succeeds for any valid path, so we get a new client and reload.
	assert.NotNil(t, gs.git, "git client should be set after RepoChangedMsg")
	assert.Nil(t, gs.files, "files should be cleared before reload")
	assert.Nil(t, gs.rows, "rows should be cleared before reload")
	assert.Equal(t, 0, gs.cursor, "cursor should be reset")
	assert.NotNil(t, cmd, "a reload command should be returned")
}

// ---------------------------------------------------------------------------
// Discard
// ---------------------------------------------------------------------------

func TestDiscardAtCursor_UnstagedFile_ShowsConfirm(t *testing.T) {
	file := git.FileStatus{Path: "modified.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	p.Focus()
	p.SetSize(80, 24)
	// Rows: unstaged header (0), file (1)
	p.cursor = 1

	_, cmd := p.handleKey(keyMsg('d'))
	require.NotNil(t, cmd, "discard should emit a command for confirmation")
	assert.Equal(t, opDiscard, p.pendingOp)
	assert.Equal(t, "modified.go", p.pendingPath)

	// The command should produce a ShowModalMsg.
	msg := cmd()
	_, ok := msg.(notify.ShowModalMsg)
	assert.True(t, ok, "should emit ShowModalMsg, got %T", msg)
}

func TestDiscardAtCursor_StagedFile_NoOp(t *testing.T) {
	file := git.FileStatus{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	p.Focus()
	p.SetSize(80, 24)
	// Rows: staged header (0), file (1)
	p.cursor = 1

	_, cmd := p.handleKey(keyMsg('d'))
	assert.Nil(t, cmd, "discard on staged file should be a no-op")
}

func TestDiscardAtCursor_OutOfBounds(t *testing.T) {
	p := New(&mockGitClient{})
	p.Focus()
	p.cursor = -1

	_, cmd := p.discardAtCursor()
	assert.Nil(t, cmd)
}

func TestDiscardAtCursor_NilFile(t *testing.T) {
	p := New(&mockGitClient{})
	p.Focus()
	p.rows = []row{{kind: rowSection, section: sectionUnstaged}}
	p.cursor = 0

	_, cmd := p.discardAtCursor()
	assert.Nil(t, cmd)
}

func TestDiscardModalResult_Accepted(t *testing.T) {
	mock := &mockGitClient{}
	p := New(mock)
	p.Init(context.Background())
	p.pendingOp = opDiscard
	p.pendingPath = "modified.go"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd, "accepting discard should return a command")

	// Execute the discard command.
	msg := cmd()
	result, ok := msg.(discardResultMsg)
	require.True(t, ok, "should produce discardResultMsg, got %T", msg)
	assert.NoError(t, result.err)
	assert.Equal(t, []string{"modified.go"}, mock.discardedPaths)
}

func TestDiscardModalResult_Rejected(t *testing.T) {
	p := New(&mockGitClient{})
	p.pendingOp = opDiscard
	p.pendingPath = "modified.go"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "rejecting discard should return nil")
	assert.Equal(t, "", p.pendingOp)
	assert.Equal(t, "", p.pendingPath)
}

func TestDiscardResultMsg_RefreshesStatus(t *testing.T) {
	p := New(&mockGitClient{})
	p.Init(context.Background())

	_, cmd := p.Update(discardResultMsg{err: nil})
	assert.True(t, p.loading)
	assert.NotNil(t, cmd, "should reload status after discard")
}

func TestDiscardResultMsg_Error(t *testing.T) {
	p := New(&mockGitClient{})
	p.Init(context.Background())

	_, cmd := p.Update(discardResultMsg{err: fmt.Errorf("discard failed")})
	assert.NotNil(t, p.err)
	assert.Nil(t, cmd, "should not reload on error")
}
