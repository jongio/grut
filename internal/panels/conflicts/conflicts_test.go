package conflicts

import (
	"context"
	"errors"
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
	statusFiles []git.FileStatus
	statusErr   error
	stageErr    error
	mergeErr    error
	mergeAbErr  error
	rebaseErr   error
	rebaseCErr  error
	rebaseAbErr error

	// Recorded calls.
	mergedBranch      string
	rebasedOnto       string
	stagedPaths       []string
	mergeCalled       bool
	rebaseCalled      bool
	continueCalled    bool
	abortMergeCalled  bool
	abortRebaseCalled bool
}

func (m *mockGit) Status(_ context.Context) ([]git.FileStatus, error) {
	return m.statusFiles, m.statusErr
}

func (m *mockGit) Stage(_ context.Context, paths []string) error {
	m.stagedPaths = paths
	return m.stageErr
}

func (m *mockGit) Merge(_ context.Context, branch string, _ git.MergeOpts) error {
	m.mergeCalled = true
	m.mergedBranch = branch
	return m.mergeErr
}

func (m *mockGit) MergeAbort(_ context.Context) error {
	m.abortMergeCalled = true
	return m.mergeAbErr
}

func (m *mockGit) Rebase(_ context.Context, onto string, _ git.RebaseOpts) error {
	m.rebaseCalled = true
	m.rebasedOnto = onto
	return m.rebaseErr
}

func (m *mockGit) RebaseContinue(_ context.Context) error {
	m.continueCalled = true
	return m.rebaseCErr
}

func (m *mockGit) RebaseAbort(_ context.Context) error {
	m.abortRebaseCalled = true
	return m.rebaseAbErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// conflictFiles returns mock status entries with conflict markers.
func conflictFiles() []git.FileStatus {
	return []git.FileStatus{
		{Path: "file1.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		{Path: "file2.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		{Path: "file3.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}
}

// newTestPanel creates a Panel pre-loaded with conflict files for testing.
func newTestPanel(t *testing.T, mg *mockGit) *Panel {
	t.Helper()
	p := New(mg)
	p.SetSize(80, 24)
	return p
}

// runCmd executes a tea.Cmd and returns the resulting message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// executeBatch runs a tea.Cmd that may be a tea.Batch, collecting all
// produced messages.
func executeBatch(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			if c != nil {
				if m := c(); m != nil {
					msgs = append(msgs, m)
				}
			}
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// ---------------------------------------------------------------------------
// Compile-time checks
// ---------------------------------------------------------------------------

func TestPanelImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

// ---------------------------------------------------------------------------
// Construction and Init
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mg := &mockGit{}
	p := New(mg)
	assert.Equal(t, "conflicts", p.Title())
	assert.NotNil(t, p.KeyBindings())
	assert.Empty(t, p.files)
	assert.NotNil(t, p.resolved)
}

func TestInit(t *testing.T) {
	mg := &mockGit{}
	p := New(mg)
	cmd := p.Init(context.Background())
	// Init returns nil — no initial load needed until a merge/rebase starts.
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestViewNoConflicts(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	view := p.View(80, 24)
	assert.Contains(t, view, "No conflicts")
}

func TestViewZeroSize(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	assert.Empty(t, p.View(0, 24))
	assert.Empty(t, p.View(80, 0))
	assert.Empty(t, p.View(-1, 10))
}

func TestViewWithConflicts(t *testing.T) {
	mg := &mockGit{statusFiles: conflictFiles()}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go", "file3.go"}

	view := p.View(80, 24)
	assert.Contains(t, view, "file1.go")
	assert.Contains(t, view, "file2.go")
	assert.Contains(t, view, "file3.go")
	assert.Contains(t, view, "MERGING")
	assert.Contains(t, view, "3/3 conflicts remaining")
}

func TestViewResolvedMarker(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go"}
	p.resolved["file1.go"] = true

	view := p.View(80, 24)
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "✗")
	assert.Contains(t, view, "1/2 conflicts remaining")
}

func TestViewRebaseMode(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase
	p.files = []string{"file1.go"}

	view := p.View(80, 24)
	assert.Contains(t, view, "REBASING")
}

// ---------------------------------------------------------------------------
// Keyboard navigation
// ---------------------------------------------------------------------------

func TestNavigateDown(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.files = []string{"a.go", "b.go", "c.go"}

	assert.Equal(t, 0, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.cursor)

	// Should not go past the end.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.cursor)
}

func TestNavigateUp(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.files = []string{"a.go", "b.go", "c.go"}
	p.cursor = 2

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.cursor)

	// Should not go past the beginning.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.cursor)
}

func TestNavigateUnfocused(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.files = []string{"a.go", "b.go"}
	// Panel is not focused — keys should be ignored.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 0, p.cursor)
}

func TestNavigateEmptyList(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	// No files — navigation should be a no-op.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Merge request
// ---------------------------------------------------------------------------

func TestMergeClean(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.MergeRequestMsg{Branch: "feature/foo"})
	require.NotNil(t, cmd)
	assert.Equal(t, opMerge, p.mode)

	msg := runCmd(t, cmd)
	result, ok := msg.(mergeResultMsg)
	require.True(t, ok, "expected mergeResultMsg")
	assert.Equal(t, "feature/foo", result.branch)
	assert.Empty(t, result.conflicts)
	assert.NoError(t, result.err)
	assert.True(t, mg.mergeCalled)
}

func TestMergeCleanResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge

	_, cmd := p.Update(mergeResultMsg{branch: "feature/foo"})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.mode) // reset after clean merge

	msgs := executeBatch(t, cmd)
	var foundToast, foundRefresh bool
	for _, m := range msgs {
		switch m := m.(type) {
		case notify.ShowToastMsg:
			foundToast = true
			assert.Equal(t, notify.Success, m.Level)
			assert.Contains(t, m.Message, "feature/foo")
		case panels.RefreshGitStatusMsg:
			foundRefresh = true
		}
	}
	assert.True(t, foundToast, "expected success toast")
	assert.True(t, foundRefresh, "expected status refresh")
}

func TestMergeWithConflicts(t *testing.T) {
	mg := &mockGit{
		mergeErr:    errors.New("merge conflict"),
		statusFiles: conflictFiles(),
	}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.MergeRequestMsg{Branch: "feature/bar"})
	msg := runCmd(t, cmd)
	result, ok := msg.(mergeResultMsg)
	require.True(t, ok)
	assert.Equal(t, "feature/bar", result.branch)
	assert.Len(t, result.conflicts, 3)
	assert.NoError(t, result.err) // err is nil when conflicts detected
}

func TestMergeConflictResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge

	_, cmd := p.Update(mergeResultMsg{
		branch:    "feature/bar",
		conflicts: []string{"a.go", "b.go"},
	})
	require.NotNil(t, cmd)
	assert.Equal(t, opMerge, p.mode) // stays in merge mode
	assert.Len(t, p.files, 2)

	msg := runCmd(t, cmd)
	detected, ok := msg.(panels.ConflictDetectedMsg)
	require.True(t, ok, "expected ConflictDetectedMsg")
	assert.Len(t, detected.Files, 2)
}

func TestMergeError(t *testing.T) {
	mg := &mockGit{
		mergeErr: errors.New("not a git repo"),
		// No conflict files — so it's a real error, not conflicts.
	}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.MergeRequestMsg{Branch: "bad"})
	msg := runCmd(t, cmd)
	result, ok := msg.(mergeResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)
	assert.Empty(t, result.conflicts)
}

func TestMergeErrorResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge

	_, cmd := p.Update(mergeResultMsg{branch: "bad", err: errors.New("oops")})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.mode) // reset on error

	msg := runCmd(t, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "oops")
}

// ---------------------------------------------------------------------------
// Rebase request
// ---------------------------------------------------------------------------

func TestRebaseClean(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.RebaseRequestMsg{Onto: "main"})
	require.NotNil(t, cmd)
	assert.Equal(t, opRebase, p.mode)

	msg := runCmd(t, cmd)
	result, ok := msg.(rebaseResultMsg)
	require.True(t, ok)
	assert.Equal(t, "main", result.onto)
	assert.Empty(t, result.conflicts)
	assert.NoError(t, result.err)
}

func TestRebaseCleanResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase

	_, cmd := p.Update(rebaseResultMsg{onto: "main"})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.mode)

	msgs := executeBatch(t, cmd)
	var foundToast bool
	for _, m := range msgs {
		if toast, ok := m.(notify.ShowToastMsg); ok {
			foundToast = true
			assert.Equal(t, notify.Success, toast.Level)
			assert.Contains(t, toast.Message, "main")
		}
	}
	assert.True(t, foundToast)
}

func TestRebaseWithConflicts(t *testing.T) {
	mg := &mockGit{
		rebaseErr:   errors.New("rebase conflict"),
		statusFiles: conflictFiles(),
	}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.RebaseRequestMsg{Onto: "main"})
	msg := runCmd(t, cmd)
	result, ok := msg.(rebaseResultMsg)
	require.True(t, ok)
	assert.Len(t, result.conflicts, 3)
}

func TestRebaseConflictResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase

	_, cmd := p.Update(rebaseResultMsg{
		onto:      "main",
		conflicts: []string{"x.go"},
	})
	require.NotNil(t, cmd)
	assert.Equal(t, opRebase, p.mode)
	assert.Len(t, p.files, 1)

	msg := runCmd(t, cmd)
	_, ok := msg.(panels.ConflictDetectedMsg)
	assert.True(t, ok)
}

func TestRebaseError(t *testing.T) {
	mg := &mockGit{rebaseErr: errors.New("fatal")}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.RebaseRequestMsg{Onto: "main"})
	msg := runCmd(t, cmd)
	result, ok := msg.(rebaseResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)
}

func TestRebaseErrorResult(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase

	_, cmd := p.Update(rebaseResultMsg{onto: "main", err: errors.New("fatal")})
	msg := runCmd(t, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

// ---------------------------------------------------------------------------
// Mark as resolved
// ---------------------------------------------------------------------------

func TestMarkResolved(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go"}

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	result, ok := msg.(resolveResultMsg)
	require.True(t, ok)
	assert.Equal(t, "file1.go", result.path)
	assert.NoError(t, result.err)
	assert.Equal(t, []string{"file1.go"}, mg.stagedPaths)
}

func TestMarkResolvedAlreadyResolved(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go"}
	p.resolved["file1.go"] = true

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	assert.Nil(t, cmd, "already resolved — should be no-op")
}

func TestMarkResolvedEmpty(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	assert.Nil(t, cmd)
}

func TestMarkResolvedError(t *testing.T) {
	mg := &mockGit{stageErr: errors.New("permission denied")}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	msg := runCmd(t, cmd)
	result := msg.(resolveResultMsg)
	assert.Error(t, result.err)

	// Feed resolve error back into Update.
	_, cmd2 := p.Update(result)
	require.NotNil(t, cmd2)
	toast := runCmd(t, cmd2).(notify.ShowToastMsg)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "permission denied")
}

func TestResolveResultUpdatesState(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go"}

	p.Update(resolveResultMsg{path: "file1.go"})
	assert.True(t, p.resolved["file1.go"])
	assert.False(t, p.resolved["file2.go"])
}

func TestResolveAllEmitsConflictResolvedMsg(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(resolveResultMsg{path: "file1.go"})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	_, ok := msg.(panels.ConflictResolvedMsg)
	assert.True(t, ok, "expected ConflictResolvedMsg when all resolved")
}

// ---------------------------------------------------------------------------
// Continue operation
// ---------------------------------------------------------------------------

func TestContinueMerge(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go"}
	p.resolved["file1.go"] = true // all resolved

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.Equal(t, "continued", result.action)
	assert.NoError(t, result.err)
}

func TestContinueRebase(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opRebase
	p.files = []string{"file1.go"}
	p.resolved["file1.go"] = true

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.NoError(t, result.err)
	assert.True(t, mg.continueCalled)
}

func TestContinueWithUnresolved(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go"}
	p.resolved["file1.go"] = true
	// file2.go is NOT resolved.

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "1 conflict(s) remaining")
}

func TestContinueNoMode(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opNone

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c'})
	assert.Nil(t, cmd)
}

func TestContinueRebaseError(t *testing.T) {
	mg := &mockGit{rebaseCErr: errors.New("conflict")}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opRebase
	p.files = []string{"file1.go"}
	p.resolved["file1.go"] = true

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'c'})
	msg := runCmd(t, cmd)
	result := msg.(continueResultMsg)
	assert.Error(t, result.err)
}

func TestContinueResultSuccess(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(continueResultMsg{action: "continued"})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.mode) // reset
	assert.Empty(t, p.files)

	msgs := executeBatch(t, cmd)
	var foundToast bool
	for _, m := range msgs {
		if toast, ok := m.(notify.ShowToastMsg); ok {
			foundToast = true
			assert.Equal(t, notify.Success, toast.Level)
			assert.Contains(t, toast.Message, "Merge continued")
		}
	}
	assert.True(t, foundToast)
}

func TestContinueResultError(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase

	_, cmd := p.Update(continueResultMsg{action: "continued", err: errors.New("bad")})
	msg := runCmd(t, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "bad")
}

// ---------------------------------------------------------------------------
// Abort operation
// ---------------------------------------------------------------------------

func TestAbortMerge(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.Equal(t, "aborted", result.action)
	assert.NoError(t, result.err)
	assert.True(t, mg.abortMergeCalled)
}

func TestAbortRebase(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opRebase
	p.files = []string{"file1.go"}

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.Equal(t, "aborted", result.action)
	assert.True(t, mg.abortRebaseCalled)
}

func TestAbortNoMode(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opNone

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	assert.Nil(t, cmd)
}

func TestAbortError(t *testing.T) {
	mg := &mockGit{mergeAbErr: errors.New("cannot abort")}
	p := newTestPanel(t, mg)
	p.Focus()
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	msg := runCmd(t, cmd)
	result := msg.(continueResultMsg)
	assert.Error(t, result.err)
}

func TestAbortViaMessage(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(panels.MergeAbortMsg{})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.Equal(t, "aborted", result.action)
	assert.True(t, mg.abortMergeCalled)
}

func TestAbortRebaseViaMessage(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase
	p.files = []string{"file1.go"}

	_, cmd := p.Update(panels.RebaseAbortMsg{})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	result, ok := msg.(continueResultMsg)
	require.True(t, ok)
	assert.Equal(t, "aborted", result.action)
	assert.True(t, mg.abortRebaseCalled)
}

func TestAbortResultSuccess(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opRebase
	p.files = []string{"file1.go"}

	_, cmd := p.Update(continueResultMsg{action: "aborted"})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.mode)
	assert.Empty(t, p.files)

	msgs := executeBatch(t, cmd)
	var foundToast bool
	for _, m := range msgs {
		if toast, ok := m.(notify.ShowToastMsg); ok {
			foundToast = true
			assert.Contains(t, toast.Message, "Rebase aborted")
		}
	}
	assert.True(t, foundToast)
}

// ---------------------------------------------------------------------------
// Open file (enter key)
// ---------------------------------------------------------------------------

func TestOpenFile(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()
	p.files = []string{"file1.go", "file2.go"}
	p.cursor = 1

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	diff, ok := msg.(panels.ShowDiffMsg)
	require.True(t, ok, "expected ShowDiffMsg")
	assert.Equal(t, "file2.go", diff.Path)
}

func TestOpenFileEmpty(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// ConflictsLoadedMsg handling
// ---------------------------------------------------------------------------

func TestConflictsLoaded(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	p.Update(conflictsLoadedMsg{files: []string{"a.go", "b.go"}})
	assert.Len(t, p.files, 2)
	assert.False(t, p.loading)
}

func TestConflictsLoadedError(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.loading = true

	_, cmd := p.Update(conflictsLoadedMsg{err: errors.New("fail")})
	require.NotNil(t, cmd)
	assert.False(t, p.loading)

	toast := runCmd(t, cmd).(notify.ShowToastMsg)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestConflictsLoadedClampsCursor(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.cursor = 10

	p.Update(conflictsLoadedMsg{files: []string{"a.go"}})
	assert.Equal(t, 0, p.cursor) // clamped
}

// ---------------------------------------------------------------------------
// Empty conflicts (clean merge)
// ---------------------------------------------------------------------------

func TestCleanMergeNoConflicts(t *testing.T) {
	// Merge succeeds with no error — no conflicts at all.
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.MergeRequestMsg{Branch: "clean-branch"})
	msg := runCmd(t, cmd)
	result := msg.(mergeResultMsg)
	assert.Empty(t, result.conflicts)
	assert.NoError(t, result.err)
}

// ---------------------------------------------------------------------------
// Focus / Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	mg := &mockGit{}
	p := New(mg)
	assert.False(t, p.Focused)

	p.Focus()
	assert.True(t, p.Focused)

	p.Blur()
	assert.False(t, p.Focused)
}

// ---------------------------------------------------------------------------
// allResolved / remainingCount helpers
// ---------------------------------------------------------------------------

func TestAllResolvedEmpty(t *testing.T) {
	p := &Panel{resolved: make(map[string]bool)}
	assert.True(t, p.allResolved())
}

func TestAllResolvedPartial(t *testing.T) {
	p := &Panel{
		files:    []string{"a.go", "b.go"},
		resolved: map[string]bool{"a.go": true},
	}
	assert.False(t, p.allResolved())
	assert.Equal(t, 1, p.remainingCount())
}

func TestAllResolvedComplete(t *testing.T) {
	p := &Panel{
		files:    []string{"a.go", "b.go"},
		resolved: map[string]bool{"a.go": true, "b.go": true},
	}
	assert.True(t, p.allResolved())
	assert.Equal(t, 0, p.remainingCount())
}

// ---------------------------------------------------------------------------
// scanConflicts helper
// ---------------------------------------------------------------------------

func TestScanConflicts(t *testing.T) {
	mg := &mockGit{statusFiles: []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		{Path: "untracked.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
	}}
	conflicts := scanConflicts(context.Background(), mg)
	assert.Equal(t, []string{"conflict.go"}, conflicts)
}

func TestScanConflictsEmpty(t *testing.T) {
	mg := &mockGit{statusFiles: []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
	}}
	conflicts := scanConflicts(context.Background(), mg)
	assert.Empty(t, conflicts)
}

func TestScanConflictsError(t *testing.T) {
	mg := &mockGit{statusErr: errors.New("fail")}
	conflicts := scanConflicts(context.Background(), mg)
	assert.Nil(t, conflicts)
}

// ---------------------------------------------------------------------------
// opMode String
// ---------------------------------------------------------------------------

func TestOpModeString(t *testing.T) {
	assert.Equal(t, "MERGING", opMerge.String())
	assert.Equal(t, "REBASING", opRebase.String())
	assert.Equal(t, "", opNone.String())
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	p := New(&mockGit{})
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify all expected keys are present.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Action] = true
	}
	assert.True(t, keys["cursor_down"])
	assert.True(t, keys["cursor_up"])
	assert.True(t, keys["resolve"])
	assert.True(t, keys["continue"])
	assert.True(t, keys["abort"])
	assert.True(t, keys["open"])
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsFile(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go", "file3.go"}
	p.Focus()

	assert.Equal(t, 0, p.cursor)

	// Click on row 1 (second file).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)

	// Click on row 2 (third file).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go"}

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "out-of-bounds click should not move cursor")
}

func TestMouseDoubleClick_OpensFile(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go", "file2.go", "file3.go"}
	p.Focus()
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemConflictFile): true}

	// Double-click on row 1 (file2.go).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)
	require.NotNil(t, cmd, "double-click should trigger open file command")

	msg := runCmd(t, cmd)
	showMsg, ok := msg.(panels.ShowDiffMsg)
	require.True(t, ok, "expected ShowDiffMsg, got %T", msg)
	assert.Equal(t, "file2.go", showMsg.Path)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.mode = opMerge
	p.files = []string{"file1.go"}

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)
	assert.Nil(t, cmd)
}
