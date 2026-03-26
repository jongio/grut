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
	p := New(&mockGitOps{}, testGitConfig(), "/repo")
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"worktree": "switch"},
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
			p := New(&mockGitOps{}, testGitConfig(), "/repo")
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

func TestExecuteRightClickAction_Switch(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionSwitch)
	assert.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(panels.SwitchWorktreeMsg)
	assert.True(t, ok, "expected SwitchWorktreeMsg")
}

func TestExecuteRightClickAction_CopyPath(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.executeRightClickAction(actions.ActionCopyPath)
	// cmd invokes clipboard; just ensure it's non-nil
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_OpenTerminal(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.executeRightClickAction(actions.ActionOpenTerminal)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_UnknownAction(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	_, cmd := p.executeRightClickAction(actions.ActionID("unknown"))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// copyPath
// ---------------------------------------------------------------------------

func TestCopyPath_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = -1

	_, cmd := p.copyPath()
	assert.Nil(t, cmd)
}

func TestCopyPath_ValidCursor(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 0

	_, cmd := p.copyPath()
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// openTerminal
// ---------------------------------------------------------------------------

func TestOpenTerminal_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 99

	_, cmd := p.openTerminal()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// requestSwitch — additional branches
// ---------------------------------------------------------------------------

func TestRequestSwitch_MissingPath(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.items[1].isMissing = true
	p.cursor = 1

	_, cmd := p.requestSwitch()
	assert.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok)
	assert.Contains(t, toast.Message, "Cannot switch")
}

func TestRequestSwitch_NewTerminalMode(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cfg.WorktreeOpenMode = "new_terminal"
	p.cursor = 1

	_, cmd := p.requestSwitch()
	assert.NotNil(t, cmd, "should emit terminal open command")
}

func TestRequestSwitch_NoItems(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: []git.Worktree{}}, alwaysExists)
	_, cmd := p.requestSwitch()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleModalResult — additional branches
// ---------------------------------------------------------------------------

func TestHandleModalResult_RightClickPick(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.pending = opRightClickPick
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionSwitch),
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
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionSwitch),
		Remember: true,
	})
	assert.NotNil(t, cmd)
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
// changeDirectory
// ---------------------------------------------------------------------------

func TestChangeDirectory_EmitsChangeDirectoryMsg(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 1

	_, cmd := p.changeDirectory()
	require.NotNil(t, cmd)
	msg := cmd()
	cdMsg, ok := msg.(panels.ChangeDirectoryMsg)
	require.True(t, ok, "expected ChangeDirectoryMsg")
	assert.Equal(t, "/home/user/grut-feat", cdMsg.Path)
}

func TestChangeDirectory_MissingPath(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.items[1].isMissing = true
	p.cursor = 1

	_, cmd := p.changeDirectory()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "missing")
}

func TestChangeDirectory_NoItems(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: []git.Worktree{}}, alwaysExists)
	_, cmd := p.changeDirectory()
	assert.Nil(t, cmd)
}

func TestExecuteRightClickAction_ChangeDirectory(t *testing.T) {
	p := newTestPanel(t, &mockGitOps{worktrees: sampleWorktrees()}, alwaysExists)
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionChangeDirectory)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(panels.ChangeDirectoryMsg)
	assert.True(t, ok, "expected ChangeDirectoryMsg")
}
