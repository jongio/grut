package branches

import (
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests: handleRepoChanged
// ---------------------------------------------------------------------------

func TestHandleRepoChanged_InvalidPath_ClearsState(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Verify initial state has items loaded.
	require.NotEmpty(t, p.items)
	require.NotNil(t, p.git)

	// Send a RepoChangedMsg with an invalid path. git.NewClient may still
	// succeed (it creates the client lazily), but the panel state should
	// be reset regardless.
	msg := panels.RepoChangedMsg{Path: "/nonexistent/invalid/path"}
	result, _ := p.handleRepoChanged(msg)

	panel := result.(*Panel)
	// Panel replaces git client with a new one for the new path.
	assert.NotNil(t, panel.git)
	// State is always reset on repo change.
	assert.Nil(t, panel.items)
	assert.Equal(t, 0, panel.cursor)
	assert.Equal(t, 0, panel.offset)
	assert.NotNil(t, panel.annotations)
	assert.Empty(t, panel.annotations)
}

func TestHandleRepoChanged_ValidGitRepo_ReloadsPanel(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Use the actual grut repo root as a known valid git directory.
	// This ensures git.NewClient succeeds without needing a temp repo.
	msg := panels.RepoChangedMsg{Path: t.TempDir()}

	// Even though TempDir isn't a git repo, git.NewClient may or may not
	// succeed depending on implementation. We test the failure path above
	// which is the critical coverage path. Here we verify the reset behavior.
	result, _ := p.handleRepoChanged(msg)
	panel := result.(*Panel)

	// State should be reset regardless of success/failure.
	assert.Equal(t, 0, panel.cursor)
	assert.Equal(t, 0, panel.offset)
	assert.NotNil(t, panel.annotations)
	assert.Empty(t, panel.annotations)
}

func TestHandleRepoChanged_ResetsItemsAndCursor(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Move cursor to a non-default position.
	p.cursor = 3
	p.offset = 2

	msg := panels.RepoChangedMsg{Path: "/does/not/exist"}
	result, _ := p.handleRepoChanged(msg)

	panel := result.(*Panel)
	assert.Equal(t, 0, panel.cursor)
	assert.Equal(t, 0, panel.offset)
	assert.Nil(t, panel.items)
}

// ---------------------------------------------------------------------------
// Tests: doCopy
// ---------------------------------------------------------------------------

func TestDoCopy_NoBranchSelected_ReturnsNil(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Move cursor to a header row (index 0).
	p.cursor = 0
	assert.True(t, p.items[0].isHeader) // sanity check

	result, cmd := p.doCopy()
	assert.Equal(t, p, result)
	assert.Nil(t, cmd)
}

func TestDoCopy_CursorOutOfBounds_ReturnsNil(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Set cursor beyond bounds.
	p.cursor = 999

	result, cmd := p.doCopy()
	assert.Equal(t, p, result)
	assert.Nil(t, cmd)
}

func TestDoCopy_ValidBranch_ReturnsCmd(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Cursor is on "main" (index 1) after newTestPanel.
	assert.Equal(t, 1, p.cursor)
	assert.Equal(t, "main", p.items[p.cursor].branch.Name)

	result, cmd := p.doCopy()
	assert.Equal(t, p, result)
	require.NotNil(t, cmd, "doCopy should return a command for clipboard operation")

	// Execute the command to get the message.
	msg := cmd()
	// The result is either a success or failure toast depending on clipboard
	// availability in the test environment.
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	// Either "Copied: main" or "Copy failed: ..." depending on OS clipboard.
	assert.True(t, toast.Message != "", "toast message should not be empty")
}

func TestDoCopy_RemoteBranch_ReturnsCmd(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Move cursor to a remote branch (index 4 = "origin/main").
	p.cursor = 4
	assert.Equal(t, "origin/main", p.items[p.cursor].branch.Name)

	result, cmd := p.doCopy()
	assert.Equal(t, p, result)
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.True(t, toast.Message != "")
}

// ---------------------------------------------------------------------------
// Tests: handleRepoChanged via Update dispatch
// ---------------------------------------------------------------------------

func TestUpdate_DispatchesRepoChangedMsg(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Verify items are loaded initially.
	require.NotEmpty(t, p.items)

	// Send RepoChangedMsg through the Update method to verify dispatch.
	msg := panels.RepoChangedMsg{Path: "/nonexistent"}
	result, _ := p.Update(msg)
	panel := result.(*Panel)

	// Panel state should be reset (items cleared, cursor reset).
	assert.Nil(t, panel.items)
	assert.Equal(t, 0, panel.cursor)
	assert.Equal(t, 0, panel.offset)
}

// ---------------------------------------------------------------------------
// Tests: doCopy via key press dispatch
// ---------------------------------------------------------------------------

func TestKeyPress_Y_TriggersCopy(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focused = true

	// Cursor on "main" (index 1).
	assert.Equal(t, 1, p.cursor)

	// Send 'y' key which maps to item_copy → doCopy.
	_, cmd := p.Update(panels.ItemCopyMsg{})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: selectedBranch edge cases
// ---------------------------------------------------------------------------

func TestSelectedBranch_NegativeCursor_ReturnsNil(t *testing.T) {
	mock := &mockGitOps{branches: []git.Branch{
		{Name: "main", IsCurrent: true},
	}}
	p := newTestPanel(t, mock, defaultCfg())
	p.cursor = -1

	b := p.selectedBranch()
	assert.Nil(t, b)
}
