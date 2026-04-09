package worktrees

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockGitOps{}, testGitConfig(), "/repo", nil)
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"worktree": "change_directory"},
		Confirmed:  map[string]bool{"worktree": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to max offset", func(t *testing.T) {
		p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
		p.Height = 1

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 2, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
		p.Height = 1
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestEnsureCursorVisible(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height leaves offset unchanged", height: 0, cursor: 0, offset: 2, wantOffset: 2},
		{name: "cursor above offset moves viewport up", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport moves viewport down", height: 3, cursor: 5, offset: 1, wantOffset: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&mockGitOps{}, testGitConfig(), "/repo", nil)
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset

			p.ensureCursorVisible()

			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

// ---------------------------------------------------------------------------
// executeRightClickAction
// ---------------------------------------------------------------------------

func TestExecuteRightClickAction_ChangeDirectory_Default(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionChangeDirectory, "")
	assert.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(panels.ChangeDirectoryMsg)
	assert.True(t, ok, "expected ChangeDirectoryMsg")
}

func TestExecuteRightClickAction_CopyPath(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.executeRightClickAction(actions.ActionCopyPath, "")
	// cmd invokes clipboard; just ensure it's non-nil
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_OpenTerminal(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.executeRightClickAction(actions.ActionOpenTerminal, "")
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_UnknownAction(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	_, cmd := p.executeRightClickAction(actions.ActionID("unknown"), "")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// copyPath
// ---------------------------------------------------------------------------

func TestCopyPath_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = -1

	_, cmd := p.copyPath("")
	assert.Nil(t, cmd)
}

func TestCopyPath_ValidCursor(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.copyPath("")
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// openTerminal
// ---------------------------------------------------------------------------

func TestOpenTerminal_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 99

	_, cmd := p.openTerminal("")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// changeDirectory — additional branches
// ---------------------------------------------------------------------------

func TestChangeDirectory_MissingPath(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.items[1].isMissing = true
	p.cursor = 1

	_, cmd := p.changeDirectory("")
	assert.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok)
	assert.Contains(t, toast.Message, "Cannot cd")
}

func TestChangeDirectory_NewTerminalMode(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cfg.WorktreeOpenMode = "new_terminal"
	p.cursor = 1

	_, cmd := p.changeDirectory("")
	assert.NotNil(t, cmd, "should emit terminal open command")
}

func TestChangeDirectory_NoItems(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: []git.Worktree{}}, alwaysExists)
	_, cmd := p.changeDirectory("")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleModalResult — additional branches
// ---------------------------------------------------------------------------

func TestHandleModalResult_RightClickPick(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opRightClickPick
	p.pendingPath = "/home/user/grut-feat"
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionChangeDirectory),
	})
	assert.NotNil(t, cmd)
}

func TestHandleModalResult_Rejected(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opRightClickPick

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
}

func TestHandleModalResult_FirstUseConfirm(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opFirstUseConfirm
	p.pendingName = "worktree"
	p.pendingPath = "/home/user/grut-feat"
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionChangeDirectory),
		Remember: true,
	})
	assert.NotNil(t, cmd)
}

func TestHandleModalResult_FirstUseConfirm_UsesPendingPath(t *testing.T) {
	// Regression test: after double-click triggers first-use confirmation,
	// the cursor may become stale before the modal result arrives.
	// The fix stores pendingPath at double-click time and threads it through
	// to changeDirectory, so even if the cursor is invalidated the correct
	// worktree path is used.
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opFirstUseConfirm
	p.pendingName = "worktree"
	p.pendingPath = "/home/user/grut-feat"

	// Simulate cursor becoming stale (pointing beyond items).
	p.cursor = 999

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionChangeDirectory),
		Remember: false,
	})
	require.NotNil(t, cmd, "should produce ChangeDirectoryMsg even with stale cursor")
	msg := cmd()
	cdMsg, ok := msg.(panels.ChangeDirectoryMsg)
	require.True(t, ok, "expected ChangeDirectoryMsg, got %T", msg)
	assert.Equal(t, "/home/user/grut-feat", cdMsg.Path)
}

func TestHandleModalResult_FirstUseConfirm_ChangeDirectory_UsesPendingPath(t *testing.T) {
	// Same regression test but for the change-directory action.
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opFirstUseConfirm
	p.pendingName = "worktree"
	p.pendingPath = "/home/user/grut-fix"

	// Stale cursor.
	p.cursor = 999

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionChangeDirectory),
		Remember: false,
	})
	require.NotNil(t, cmd, "should produce ChangeDirectoryMsg even with stale cursor")
	msg := cmd()
	cdMsg, ok := msg.(panels.ChangeDirectoryMsg)
	require.True(t, ok, "expected ChangeDirectoryMsg, got %T", msg)
	assert.Equal(t, "/home/user/grut-fix", cdMsg.Path)
}

func TestMouseDoubleClick_FirstUse_StoresPendingPath(t *testing.T) {
	// Verify that handleMouseDoubleClick stores pendingPath when showing
	// the first-use confirmation dialog.
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)
	p.Focus()

	// Ensure IsConfirmed returns false so the first-use flow is triggered.
	p.actionsCfg = config.ActionsConfig{}

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})

	// The first-use flow should have stored the pendingPath.
	assert.Equal(t, opFirstUseConfirm, p.pending)
	assert.Equal(t, "/home/user/grut-feat", p.pendingPath)
	assert.NotNil(t, cmd, "should emit FirstUseCmd")
}

// ---------------------------------------------------------------------------
// defaultPathChecker
// ---------------------------------------------------------------------------

func TestDefaultPathChecker(t *testing.T) {
	t.Run("existing path returns true", func(t *testing.T) {
		assert.True(t, defaultPathChecker(t.TempDir()))
	})
	t.Run("missing path returns false", func(t *testing.T) {
		assert.False(t, defaultPathChecker("/nonexistent/path/abc123"))
	})
}

// ---------------------------------------------------------------------------
// changeDirectory — basic emit (covered by TestChangeDirectory_* above
// and TestExecuteRightClickAction_ChangeDirectory_Default)
// ---------------------------------------------------------------------------
