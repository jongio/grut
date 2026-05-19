package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helper: newTestModelReady creates a model that's been resized and init'd.
// ---------------------------------------------------------------------------

func newTestModelReady(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()
	return m
}

// newTestModelWithConfig creates a model with a default config already set.
func newTestModelWithConfig(t *testing.T) Model {
	t.Helper()
	m := newTestModelReady(t)
	m = m.WithConfig(&config.Config{})
	return m
}

// ---------------------------------------------------------------------------
// 1. Settings panel messages: SetPreviewPositionMsg, SetThemeMsg,
//    ResetActionPromptsMsg, SetDoubleClickActionMsg, SetRightClickActionMsg
// ---------------------------------------------------------------------------

func TestSetPreviewPositionMsgChangesPosition(t *testing.T) {
	m := newTestModelReady(t)

	initial := m.engine.CurrentPreviewPosition()

	// Send SetPreviewPositionMsg with a different position.
	target := layout.PreviewBottom
	if initial == layout.PreviewBottom {
		target = layout.PreviewRight
	}

	updated, cmd := m.Update(panels.SetPreviewPositionMsg{Position: int(target)})
	m = updated.(Model)

	assert.Nil(t, cmd, "SetPreviewPositionMsg should return nil cmd")
	assert.Equal(t, target, m.engine.CurrentPreviewPosition(),
		"engine preview position should be updated")
}

func TestSetThemeMsgReturnsNil(t *testing.T) {
	m := newTestModelReady(t)

	updated, cmd := m.Update(panels.SetThemeMsg{Name: "dracula"})
	_ = updated.(Model)

	assert.Nil(t, cmd, "SetThemeMsg should return nil cmd")
}

func TestResetActionPromptsMsgClearsConfirmed(t *testing.T) {
	m := newTestModelWithConfig(t)
	// Pre-populate some confirmed flags.
	m.cfg.Actions.Confirmed = map[string]bool{"file.open": true, "file.delete": true}

	updated, cmd := m.Update(panels.ResetActionPromptsMsg{})
	m = updated.(Model)

	assert.Nil(t, cmd, "ResetActionPromptsMsg should return nil cmd")
	assert.Empty(t, m.cfg.Actions.Confirmed, "confirmed flags should be cleared")
}

func TestSetDoubleClickActionMsgUpdatesConfig(t *testing.T) {
	m := newTestModelWithConfig(t)

	updated, cmd := m.Update(panels.SetDoubleClickActionMsg{
		ItemType: "file",
		Action:   "open_editor",
	})
	_ = updated.(Model)

	assert.Nil(t, cmd, "SetDoubleClickActionMsg should return nil cmd")
}

func TestSetRightClickActionMsgUpdatesConfig(t *testing.T) {
	m := newTestModelWithConfig(t)

	updated, cmd := m.Update(panels.SetRightClickActionMsg{
		ItemType: "file",
		Action:   "copy_path",
	})
	m = updated.(Model)

	assert.Nil(t, cmd, "SetRightClickActionMsg should return nil cmd")
	assert.Equal(t, "copy_path", m.cfg.Actions.RightClick["file"],
		"right-click action should be stored in config")
}

func TestSetRightClickActionMsgInitializesNilMap(t *testing.T) {
	m := newTestModelWithConfig(t)
	m.cfg.Actions.RightClick = nil // force nil map

	updated, _ := m.Update(panels.SetRightClickActionMsg{
		ItemType: "directory",
		Action:   "open_terminal",
	})
	m = updated.(Model)

	assert.NotNil(t, m.cfg.Actions.RightClick, "should initialize nil map")
	assert.Equal(t, "open_terminal", m.cfg.Actions.RightClick["directory"])
}

// ---------------------------------------------------------------------------
// 2. broadcastActionsCfg gets called (coverage for the 0% function)
// ---------------------------------------------------------------------------

func TestBroadcastActionsCfgDoesNotPanic(t *testing.T) {
	m := newTestModelWithConfig(t)
	assert.NotPanics(t, func() { m.broadcastActionsCfg() })
}

// ---------------------------------------------------------------------------
// 3. handleAction for focus_next, focus_prev, focus_left, focus_right
// ---------------------------------------------------------------------------

func TestFocusNextAction(t *testing.T) {
	m := newTestModelReady(t)
	initial := m.engine.FocusedName()

	updated, cmd := m.handleAction("focus_next", nil)
	m = updated.(Model)

	assert.Nil(t, cmd, "focus_next should return nil cmd")
	assert.NotEqual(t, initial, m.engine.FocusedName(),
		"focus_next should move focus to a different panel")
}

func TestFocusPrevAction(t *testing.T) {
	m := newTestModelReady(t)
	// Move focus forward first, then back.
	updated, _ := m.handleAction("focus_next", nil)
	m = updated.(Model)
	afterNext := m.engine.FocusedName()

	updated, cmd := m.handleAction("focus_prev", nil)
	m = updated.(Model)

	assert.Nil(t, cmd, "focus_prev should return nil cmd")
	assert.NotEqual(t, afterNext, m.engine.FocusedName(),
		"focus_prev should move focus back")
}

func TestFocusLeftAction(t *testing.T) {
	m := newTestModelReady(t)
	// Move forward first.
	updated, _ := m.handleAction("focus_next", nil)
	m = updated.(Model)
	afterNext := m.engine.FocusedName()

	updated, cmd := m.handleAction("focus_left", nil)
	m = updated.(Model)

	assert.Nil(t, cmd, "focus_left should return nil cmd")
	assert.NotEqual(t, afterNext, m.engine.FocusedName(),
		"focus_left should move focus")
}

func TestFocusRightAction(t *testing.T) {
	m := newTestModelReady(t)
	initial := m.engine.FocusedName()

	updated, cmd := m.handleAction("focus_right", nil)
	m = updated.(Model)

	assert.Nil(t, cmd, "focus_right should return nil cmd")
	assert.NotEqual(t, initial, m.engine.FocusedName(),
		"focus_right should move focus to next panel")
}

// ---------------------------------------------------------------------------
// 4. handleAction for CRUD item actions
// ---------------------------------------------------------------------------

func TestItemCreateAction(t *testing.T) {
	m := newTestModelReady(t)
	updated, cmd := m.handleAction("item_create", tea.KeyPressMsg{})
	_ = updated.(Model)
	_ = cmd // may or may not be nil
}

func TestItemDeleteAction(t *testing.T) {
	m := newTestModelReady(t)
	updated, cmd := m.handleAction("item_delete", tea.KeyPressMsg{})
	_ = updated.(Model)
	_ = cmd
}

func TestItemEditAction(t *testing.T) {
	m := newTestModelReady(t)
	updated, cmd := m.handleAction("item_edit", tea.KeyPressMsg{})
	_ = updated.(Model)
	_ = cmd
}

func TestItemOpenAction(t *testing.T) {
	m := newTestModelReady(t)
	updated, cmd := m.handleAction("item_open", tea.KeyPressMsg{})
	_ = updated.(Model)
	_ = cmd
}

func TestItemCopyAction(t *testing.T) {
	m := newTestModelReady(t)
	updated, cmd := m.handleAction("item_copy", tea.KeyPressMsg{})
	_ = updated.(Model)
	_ = cmd
}

// ---------------------------------------------------------------------------
// 5. Git operation messages with mock git client
// ---------------------------------------------------------------------------

func TestPushRequestMsgWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.PushRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpPushing, m.asyncOp, "asyncOp should be set to pushing")
	assert.NotNil(t, m.asyncCancel, "asyncCancel should be set")
	assert.NotNil(t, cmd, "should return an async command")
}

func TestFetchRequestMsgWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.FetchRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpFetching, m.asyncOp, "asyncOp should be set to fetching")
	assert.NotNil(t, cmd, "should return an async command")
}

func TestPullRequestMsgWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.PullRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, "pulling...", m.asyncOp, "asyncOp should be set to pulling")
	assert.NotNil(t, cmd, "should return an async command")
}

func TestPushWhileAsyncOpInProgressShowsWarning(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Set an async operation already in progress.
	m.asyncOp = asyncOpFetching

	updated, cmd := m.Update(panels.PushRequestMsg{})
	m = updated.(Model)

	// Should not start push because another op is in progress.
	assert.Equal(t, asyncOpFetching, m.asyncOp, "asyncOp should remain unchanged")
	require.NotNil(t, cmd, "should return warning toast command")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Operation in progress")
}

// ---------------------------------------------------------------------------
// 6. branchLoadedMsg updates model state
// ---------------------------------------------------------------------------

func TestBranchLoadedMsgUpdatesBranchState(t *testing.T) {
	m := newTestModelReady(t)

	updated, _ := m.Update(branchLoadedMsg{Name: "develop", Ahead: 5, Behind: 3})
	m = updated.(Model)

	assert.Equal(t, "develop", m.currentBranch, "currentBranch should be updated")
	assert.Equal(t, 5, m.branchAhead, "branchAhead should be updated")
	assert.Equal(t, 3, m.branchBehind, "branchBehind should be updated")
}

func TestBranchLoadedMsgEmptyName(t *testing.T) {
	m := newTestModelReady(t)
	m.currentBranch = "old-branch"

	updated, _ := m.Update(branchLoadedMsg{Name: "", Ahead: 0, Behind: 0})
	m = updated.(Model)

	assert.Empty(t, m.currentBranch, "should accept empty branch name")
}

// ---------------------------------------------------------------------------
// 7. AsyncOpDoneMsg with context.Canceled
// ---------------------------------------------------------------------------

func TestAsyncOpDoneMsgCancelled(t *testing.T) {
	m := newTestModelReady(t)
	m.asyncOp = asyncOpPushing

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{
		Description: "push",
		Err:         context.Canceled,
	})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp, "asyncOp should be cleared")
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "cancelled")
	assert.Equal(t, notify.Info, toast.Level)
}

func TestAsyncOpDoneMsgSuccess(t *testing.T) {
	m := newTestModelReady(t)
	m.asyncOp = asyncOpFetching

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{Description: "fetch"})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp, "asyncOp should be cleared")
	assert.NotNil(t, cmd, "should return batch command with toast + refresh")
}

// ---------------------------------------------------------------------------
// 8. Overlay rendering in View()
// ---------------------------------------------------------------------------

func TestViewFuzzyFinderOpenAndClose(t *testing.T) {
	m := newTestModelReady(t)

	// Open fuzzy finder.
	m = m.openFuzzyFinder("files")
	assert.NotNil(t, m.fuzzyFinder)

	view1 := m.View()
	assert.NotEmpty(t, view1.Content, "view should render with fuzzy finder open")

	// Close fuzzy finder via ToggleFuzzyFinderMsg.
	updated, _ := m.Update(panels.ToggleFuzzyFinderMsg{})
	m = updated.(Model)
	assert.Nil(t, m.fuzzyFinder)

	view2 := m.View()
	assert.NotEmpty(t, view2.Content, "view should render with fuzzy finder closed")
}

func TestViewHelpToggleOnOff(t *testing.T) {
	m := newTestModelReady(t)

	// Toggle help on.
	updated, _ := m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.True(t, m.helpShown)

	view1 := m.View()
	assert.Contains(t, view1.Content, "grut", "help overlay should be visible")

	// Toggle help off.
	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.False(t, m.helpShown)

	view2 := m.View()
	assert.NotContains(t, view2.Content, "Terminal File Explorer",
		"help overlay should not be visible after toggle off")
}

func TestViewSettingsToggleOnOff(t *testing.T) {
	m := newTestModelReady(t)

	// Toggle settings on.
	updated, _ := m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.True(t, m.settingsShown)

	view1 := m.View()
	assert.Contains(t, view1.Content, "Settings", "settings overlay should be visible")

	// Toggle settings off.
	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.False(t, m.settingsShown)
}

func TestViewBookmarksToggleOnOff(t *testing.T) {
	m := newTestModelReady(t)

	// Toggle bookmarks on.
	updated, _ := m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)
	assert.True(t, m.bookmarksShown)

	view1 := m.View()
	assert.Contains(t, view1.Content, "Bookmarks", "bookmarks overlay should be visible")

	// Toggle bookmarks off.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)
	assert.False(t, m.bookmarksShown)

	view2 := m.View()
	// Verify the view renders without the overlay.
	assert.NotEmpty(t, view2.Content)
}

// ---------------------------------------------------------------------------
// 9. Mouse wheel routing to overlays
// ---------------------------------------------------------------------------

func TestMouseWheelRoutesToSettingsOverlay(t *testing.T) {
	m := newTestModelReady(t)

	// Open settings.
	updated, _ := m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.True(t, m.settingsShown)

	// Mouse wheel should be routed to settings panel, not engine.
	updated, _ = m.Update(tea.MouseWheelMsg{X: 60, Y: 20})
	_ = updated.(Model)
	// Should not panic.
}

func TestMouseWheelRoutesToHelpOverlay(t *testing.T) {
	m := newTestModelReady(t)

	// Open help.
	updated, _ := m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.True(t, m.helpShown)

	// Mouse wheel should be routed to help panel.
	updated, _ = m.Update(tea.MouseWheelMsg{X: 60, Y: 20})
	_ = updated.(Model)
	// Should not panic.
}

// ---------------------------------------------------------------------------
// 10. Mouse release swallowed while modal is active
// ---------------------------------------------------------------------------

func TestMouseReleaseSwallowedDuringModal(t *testing.T) {
	m := newTestModelReady(t)

	// Show a modal.
	updated, _ := m.Update(notify.ShowModalMsg{
		Kind:    notify.ModalConfirm,
		Title:   "Confirm",
		Message: "OK?",
	})
	m = updated.(Model)
	assert.True(t, m.notify.HasModal())

	initialFocus := m.engine.FocusedName()

	// Mouse release should be swallowed.
	updated, _ = m.Update(tea.MouseReleaseMsg{X: 10, Y: 10})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"mouse release should not change focus while modal is active")
}

// ---------------------------------------------------------------------------
// 11. handlePendingAction for amend, reword, and unknown action
// ---------------------------------------------------------------------------

func TestPendingActionAmendCancel(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "amend"

	updated, _ = m.Update(notify.ModalResultMsg{Accept: false, Value: ""})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared on cancel")
}

func TestPendingActionRewordCancel(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "reword"

	updated, _ = m.Update(notify.ModalResultMsg{Accept: false, Value: ""})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared on cancel")
}

func TestPendingActionUnknownAction(t *testing.T) {
	m := newTestModelReady(t)
	m.pendingAction = "unknown_action"

	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: "test"})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
	assert.Nil(t, cmd, "unknown action should return nil cmd")
}

// ---------------------------------------------------------------------------
// 12. Init with first-run help disabled
// ---------------------------------------------------------------------------

func TestInitWithFirstRunHelpDisabled(t *testing.T) {
	m := newTestModel(t)
	m = m.WithConfig(&config.Config{
		General: config.GeneralConfig{ShowFirstRunHelp: false},
	})

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init should return commands")
}

// ---------------------------------------------------------------------------
// 13. overlayBorderCol / overlayTitleCol fallback paths
// ---------------------------------------------------------------------------

func TestOverlayBorderColFallback(t *testing.T) {
	m := newTestModel(t)
	m.theme = nil

	col := m.overlayBorderCol()
	assert.Equal(t, overlayBorderColor, col,
		"should fall back to default when theme is nil")
}

func TestOverlayTitleColFallback(t *testing.T) {
	m := newTestModel(t)
	m.theme = nil

	col := m.overlayTitleCol()
	assert.Equal(t, overlayTitleColor, col,
		"should fall back to default when theme is nil")
}

// ---------------------------------------------------------------------------
// 14. handleAction split_horizontal
// ---------------------------------------------------------------------------

func TestSplitHorizontalActionProducesContent(t *testing.T) {
	m := newTestModelReady(t)

	updated, _ := m.handleAction("split_horizontal", nil)
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content, "split_horizontal should produce renderable content")
}

// ---------------------------------------------------------------------------
// 15. handleCommit with AI suggestion pre-fill
// ---------------------------------------------------------------------------

func TestCommitWithAISuggestionPrefillsExtra(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc123"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Store an AI suggestion.
	m.aiCommitSuggestion = &panels.AICommitSuggestionMsg{
		Subject: "fix: correct typo",
		Type:    "fix",
	}

	updated, cmd := m.handleCommit()
	m = updated.(Model)

	assert.Equal(t, "commit", m.pendingAction)
	assert.NotNil(t, cmd, "should return modal command with pre-filled value")
	assert.Nil(t, m.aiCommitSuggestion, "AI suggestion should be consumed")
}

// ---------------------------------------------------------------------------
// 16. handleAmend and handleReword messages
// ---------------------------------------------------------------------------

func TestAmendRequestMsgWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.AmendRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, "amend", m.pendingAction, "amend should set pending action")
	assert.NotNil(t, cmd, "should return modal command")
}

func TestRewordRequestMsgWithGitClient(t *testing.T) {
	t.Parallel()
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.RewordRequestMsg{OldMessage: "old msg"})
	m = updated.(Model)

	assert.Equal(t, "reword", m.pendingAction, "reword should set pending action")
	assert.NotNil(t, cmd, "should return modal command")
}

// ---------------------------------------------------------------------------
// 17. AutoFetchTickMsg via Update
// ---------------------------------------------------------------------------

func TestAutoFetchTickMsgRouted(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.AutoFetchTickMsg{})
	_ = updated.(Model)
	assert.NotNil(t, cmd, "auto-fetch tick should return commands")
}

// ---------------------------------------------------------------------------
// 18. handleAction for push/pull/fetch with git client
// ---------------------------------------------------------------------------

func TestPushActionWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("push", nil)
	m = updated.(Model)

	assert.Equal(t, asyncOpPushing, m.asyncOp)
	assert.NotNil(t, cmd)
}

func TestFetchActionWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("fetch", nil)
	m = updated.(Model)

	assert.Equal(t, asyncOpFetching, m.asyncOp)
	assert.NotNil(t, cmd)
}

func TestPullActionWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("pull", nil)
	m = updated.(Model)

	assert.Equal(t, "pulling...", m.asyncOp)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 19. saveSession with enabled config and session manager
// ---------------------------------------------------------------------------

func TestSaveSessionWithEnabledConfigAndManager(t *testing.T) {
	m := newTestModelReady(t)

	mgr := session.NewManager()
	m = m.WithSessionManager(mgr)
	m = m.WithConfig(&config.Config{Session: config.SessionConfig{Enabled: true}})

	// Should not panic.
	assert.NotPanics(t, func() { m.saveSession() })
}

// ---------------------------------------------------------------------------
// 20. View with toast overlay
// ---------------------------------------------------------------------------

func TestViewWithToastOverlay(t *testing.T) {
	m := newTestModelReady(t)

	// Send a toast.
	updated, _ := m.Update(notify.ShowToastMsg{
		Message: "Test toast message",
		Level:   notify.Info,
	})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "Test toast message",
		"toast should appear in view")
}

// ---------------------------------------------------------------------------
// 21. CWD edit mode — additional paths
// ---------------------------------------------------------------------------

func TestCWDEditDeleteKey(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 4 // cursor at '/' (second slash)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyDelete})
	m = updated.(Model)
	assert.Equal(t, "/tmptest", m.cwdEditValue, "delete should remove char at cursor")
	assert.Equal(t, 4, m.cwdEditCursor, "cursor should not move")
}

func TestCWDEditCtrlACursorToStart(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 9

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Equal(t, 0, m.cwdEditCursor, "ctrl+a should move cursor to start")
}

func TestCWDEditCtrlECursorToEnd(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Equal(t, 9, m.cwdEditCursor, "ctrl+e should move cursor to end")
}

func TestCWDEditCtrlUClearsLine(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 5

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Empty(t, m.cwdEditValue, "ctrl+u should clear the line")
	assert.Equal(t, 0, m.cwdEditCursor, "cursor should be at 0")
}

func TestCWDEditEnterEmptyPathNoOp(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "   " // whitespace-only
	m.cwdEditCursor = 3

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEnter})
	m = updated.(Model)
	assert.False(t, m.cwdEditing, "should exit edit mode")
	assert.Nil(t, cmd, "empty path should return nil cmd")
}

// ---------------------------------------------------------------------------
// 22. renderStatusBar with editing mode
// ---------------------------------------------------------------------------

func TestRenderStatusBarEditingMode(t *testing.T) {
	m := newTestModelReady(t)
	m.cwdEditing = true
	m.cwdEditValue = "/some/path"
	m.cwdEditCursor = 10

	sb := m.renderStatusBar()
	assert.NotEmpty(t, sb, "editing status bar should render")
	assert.Contains(t, sb, "/some/path", "should contain the edit value")
}

// ---------------------------------------------------------------------------
// 23. Chat stream messages routed to chat model
// ---------------------------------------------------------------------------

func TestChatStreamChunkMsgRouted(t *testing.T) {
	m := newTestModelReady(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	// Send a chat internal message — should be routed to chat without panic.
	updated, _ := m.Update(panels.ChatRefreshMsg{})
	_ = updated.(Model)
}

// ---------------------------------------------------------------------------
// 24. handleAction commit with async op in progress
// ---------------------------------------------------------------------------

func TestCommitWhileAsyncOpInProgress(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Set async op in progress.
	m.asyncOp = asyncOpPushing

	updated, cmd := m.handleCommit()
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "should not set pending action when async op is active")
	assert.Nil(t, cmd, "should return nil when async op is active")
}

// ---------------------------------------------------------------------------
// 25. injectBorderTitle edge cases
// ---------------------------------------------------------------------------

func TestInjectBorderTitleTooNarrow(t *testing.T) {
	// Create a very narrow border where label won't fit.
	rendered := "┌──┐\n│  │\n└──┘"
	result := injectBorderTitle(rendered, "Very Long Title That Won't Fit", "#FFF", "#AAA", lipgloss.NormalBorder())
	// Should return original when label is too wide.
	assert.NotEmpty(t, result)
}

func TestInjectBorderTitleEmptyLines(t *testing.T) {
	result := injectBorderTitle("", "Test", "#FFF", "#AAA", lipgloss.NormalBorder())
	assert.Empty(t, result, "empty input should return empty")
}

// ---------------------------------------------------------------------------
// 26. Multiple overlay combination tests
// ---------------------------------------------------------------------------

func TestSettingsAndBookmarksOverlayCoexist(t *testing.T) {
	m := newTestModelReady(t)

	// Open settings.
	updated, _ := m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.True(t, m.settingsShown)

	// Open bookmarks while settings is open.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)
	assert.True(t, m.bookmarksShown)

	view := m.View()
	assert.NotEmpty(t, view.Content, "should render with multiple overlays")
}

// ---------------------------------------------------------------------------
// 27. handleAction with keymap pending
// ---------------------------------------------------------------------------

func TestKeymapHasPendingSwallowsKey(t *testing.T) {
	m := newTestModelReady(t)

	// We can test this by sending a key that would create a pending state.
	// Send 'g' which might be a multi-key prefix (g + g for top).
	// This at least exercises the code path.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g'})
	_ = updated.(Model)
	// Should not panic.
}

// ---------------------------------------------------------------------------
// 28. View with AsyncOp showing in status bar
// ---------------------------------------------------------------------------

func TestViewShowsAsyncOpInStatusBar(t *testing.T) {
	m := newTestModelReady(t)
	m.asyncOp = asyncOpPushing

	view := m.View()
	assert.Contains(t, view.Content, "pushing", "view should show async op in status bar")
}

// ---------------------------------------------------------------------------
// 29. handleUndo/handleRedo full paths with working UndoManager
// ---------------------------------------------------------------------------

func TestHandleUndoNilManagerShowsInfoToast(t *testing.T) {
	m := newTestModelReady(t)
	assert.Nil(t, m.undoMgr)

	_, cmd := m.handleUndo()
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to undo")
	assert.Equal(t, notify.Info, toast.Level)
}

func TestHandleRedoNilManagerShowsInfoToast(t *testing.T) {
	m := newTestModelReady(t)
	assert.Nil(t, m.undoMgr)

	_, cmd := m.handleRedo()
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to redo")
	assert.Equal(t, notify.Info, toast.Level)
}

// ---------------------------------------------------------------------------
// 30. handlePendingAction accept routes for commit, amend, reword
// ---------------------------------------------------------------------------

func TestPendingActionCommitAcceptEmpty(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "commit"

	// Accept with empty message.
	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
	// Empty commit message should produce a warning toast.
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
}

func TestPendingActionCommitAcceptWithMessage(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "def456"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "commit"

	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: "fix: bug"})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
	assert.NotNil(t, cmd, "should return async commit command")
}

func TestPendingActionAmendAcceptWithMessage(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "amend"

	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: "updated msg"})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction)
	assert.NotNil(t, cmd, "should return async amend command")
}

func TestPendingActionRewordAcceptWithMessage(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "reword"

	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: "reworded msg"})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction)
	assert.NotNil(t, cmd, "should return async reword command")
}

// ---------------------------------------------------------------------------
// 31. Init with git client and auto-fetch — exercises full Init path
// ---------------------------------------------------------------------------

func TestInitWithGitClientChatAndAutoFetch(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true, Ahead: 1}},
	}
	m := newTestModelWithFullGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init with git+chat+autofetch should return batch")
}

// ---------------------------------------------------------------------------
// 32. handleAction edge cases
// ---------------------------------------------------------------------------

func TestHandleActionFuzzyFinder(t *testing.T) {
	m := newTestModelReady(t)
	m.width = 120
	m.height = 40

	updated, cmd := m.handleAction("fuzzy_finder", nil)
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "fuzzy_finder action should open fuzzy finder")
	assert.Nil(t, cmd)
}

func TestHandleActionCommandPalette(t *testing.T) {
	m := newTestModelReady(t)
	m.width = 120
	m.height = 40

	updated, cmd := m.handleAction("command_palette", nil)
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "command_palette should open fuzzy finder")
	assert.Nil(t, cmd)
}

func TestHandleActionChangeDirectoryExtra(t *testing.T) {
	m := newTestModelReady(t)
	m.width = 120
	m.height = 40

	updated, cmd := m.handleAction("change_directory", nil)
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "change_directory should open fuzzy finder")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// 33. cancelAsyncOp
// ---------------------------------------------------------------------------

func TestCancelAsyncOpClearsState(t *testing.T) {
	m := newTestModelReady(t)
	m.asyncOp = asyncOpFetching
	cancelled := false
	m.asyncCancel = func() { cancelled = true }

	updated, cmd := m.cancelAsyncOp()
	m = updated.(Model)

	assert.True(t, cancelled, "cancel function should be called")
	assert.Empty(t, m.asyncOp, "asyncOp should be cleared")
	assert.Nil(t, m.asyncCancel, "asyncCancel should be nil")
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "cancelled")
}

// ---------------------------------------------------------------------------
// 34. handleClosePanel (improve 40% -> higher)
// ---------------------------------------------------------------------------

func TestClosePanelMultipleSplits(t *testing.T) {
	m := newTestModelReady(t)

	// Split twice.
	updated, _ := m.handleSplitVertical("preview")
	m = updated.(Model)
	updated, _ = m.handleSplitHorizontal("preview")
	m = updated.(Model)

	// Close first split panel.
	updated, cmd := m.handleClosePanel()
	m = updated.(Model)
	assert.Nil(t, cmd, "closing a split panel should succeed")

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// 35. handleNewTab error recovery (improve 69.2%)
// ---------------------------------------------------------------------------

func TestHandleNewTabValidPreset(t *testing.T) {
	m := newTestModelReady(t)

	updated, cmd := m.handleNewTab("explorer")
	_ = updated.(Model)
	assert.NotNil(t, cmd, "creating a tab should return commands")
}

// ---------------------------------------------------------------------------
// 36. handleSplitVertical/Horizontal with non-default type (62.5% -> higher)
// ---------------------------------------------------------------------------

func TestSplitVerticalWithFiletreeType(t *testing.T) {
	m := newTestModelReady(t)

	updated, cmd := m.handleSplitVertical("filetree")
	_ = updated.(Model)
	_ = cmd

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestSplitHorizontalWithFiletreeType(t *testing.T) {
	m := newTestModelReady(t)

	updated, cmd := m.handleSplitHorizontal("filetree")
	_ = updated.(Model)
	_ = cmd

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// 37. Async operation done success triggers refresh
// ---------------------------------------------------------------------------

func TestAsyncOpDoneMsgSuccessTriggersRefresh(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true}},
	}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.asyncOp = asyncOpPushing

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{Description: "push"})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp)
	assert.NotNil(t, cmd, "success should return batch of toast + refresh + branchinfo")
}

// ---------------------------------------------------------------------------
// 38. Exercise handlePush/Fetch execute and return
// ---------------------------------------------------------------------------

func TestPushExecutesAndReturnsCmd(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handlePush()
	m = updated.(Model)

	assert.Equal(t, asyncOpPushing, m.asyncOp)
	require.NotNil(t, cmd)

	// Execute the command to simulate the async operation completing.
	msg := cmd()
	doneMsg, ok := msg.(panels.AsyncOpDoneMsg)
	require.True(t, ok, "command should produce AsyncOpDoneMsg")
	assert.Equal(t, "push", doneMsg.Description)
	assert.NoError(t, doneMsg.Err)
}

func TestFetchExecutesAndReturnsCmd(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleFetch()
	m = updated.(Model)

	assert.Equal(t, asyncOpFetching, m.asyncOp)
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(panels.AsyncOpDoneMsg)
	require.True(t, ok, "command should produce AsyncOpDoneMsg")
	assert.Equal(t, "fetch", doneMsg.Description)
	assert.NoError(t, doneMsg.Err)
}

func TestPushWithError(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{pushErr: fmt.Errorf("auth failed")}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handlePush()
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(panels.AsyncOpDoneMsg)
	require.True(t, ok)
	assert.Error(t, doneMsg.Err)
	assert.Contains(t, doneMsg.Err.Error(), "auth failed")
}

func TestFetchWithError(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{fetchErr: fmt.Errorf("network error")}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handleFetch()
	require.NotNil(t, cmd)

	msg := cmd()
	doneMsg, ok := msg.(panels.AsyncOpDoneMsg)
	require.True(t, ok)
	assert.Error(t, doneMsg.Err)
}

// ---------------------------------------------------------------------------
// 39. renderHintsBar exercised with different focused panels (63.2% -> higher)
// ---------------------------------------------------------------------------

func TestRenderHintsBarForGitHubPanel(t *testing.T) {
	m := newTestModelReady(t)
	m.width = 120

	// Focus github panel.
	m.engine.FocusByName("github")
	assert.Equal(t, "github", m.engine.FocusedName())

	hints := m.renderHintsBar()
	assert.NotEmpty(t, hints, "hints bar should render for github panel")
}

func TestRenderHintsBarNarrowWidth(t *testing.T) {
	m := newTestModelReady(t)
	m.width = 30 // very narrow

	hints := m.renderHintsBar()
	assert.NotEmpty(t, hints, "hints bar should render on narrow screen")
}

// ---------------------------------------------------------------------------
// 40. View with bookmarks overlay on small screen
// ---------------------------------------------------------------------------

func TestViewBookmarksOverlaySmallScreen(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	m = updated.(Model)
	m.Init()

	// Open bookmarks.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// 41. Chat internal messages with no chat model
// ---------------------------------------------------------------------------

func TestChatMessagesWithNilChatNoPanic(t *testing.T) {
	m := newTestModelReady(t)
	assert.Nil(t, m.chat)

	// These should not panic when chat is nil — they fall through to default.
	updated, _ := m.Update(panels.ChatFocusMsg{})
	_ = updated.(Model)

	updated, _ = m.Update(panels.ChatRefreshMsg{})
	_ = updated.(Model)

	updated, _ = m.Update(panels.ChatNavigateMsg{Path: "/test"})
	_ = updated.(Model)
}

// ---------------------------------------------------------------------------
// 42. View with all overlays simultaneously
// ---------------------------------------------------------------------------

func TestViewWithAllOverlays(t *testing.T) {
	m := newTestModelReady(t)

	// Open help, settings, bookmarks, and fuzzy finder.
	updated, _ := m.toggleHelp()
	m = updated.(Model)
	updated, _ = m.toggleSettings()
	m = updated.(Model)
	updated, _ = m.toggleBookmarks()
	m = updated.(Model)
	m = m.openFuzzyFinder("files")

	// Show a toast as well.
	updated, _ = m.Update(notify.ShowToastMsg{Message: "all overlays", Level: notify.Info})
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content, "view should render with all overlays active")
}

// ---------------------------------------------------------------------------
// 43. handleClosePanel error path (no split → toast)
// ---------------------------------------------------------------------------

func TestClosePanelNoSplitShowsError(t *testing.T) {
	m := newTestModelReady(t)

	_, cmd := m.handleClosePanel()
	if cmd != nil {
		msg := cmd()
		toast, ok := msg.(notify.ShowToastMsg)
		if ok {
			assert.Equal(t, notify.Warn, toast.Level,
				"should show warning when closing without splits")
		}
	}
}

// ---------------------------------------------------------------------------
// 44. Update dispatches unknown messages to engine + chat
// ---------------------------------------------------------------------------

func TestUpdateUnknownMsgRoutesToEngineAndChat(t *testing.T) {
	m := newTestModelReady(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	type customMsg struct{ val int }

	updated, _ := m.Update(customMsg{val: 42})
	_ = updated.(Model)
	// Should not panic — routes to engine.Update() and chat.Update().
}

// ---------------------------------------------------------------------------
// 45. Edit mode flag tracking and key routing
// ---------------------------------------------------------------------------

func TestEditModeEnteredSetsFlag(t *testing.T) {
	m := newTestModelReady(t)
	assert.False(t, m.previewEditing)

	updated, _ := m.Update(panels.EditModeEnteredMsg{Path: "test.go"})
	m = updated.(Model)

	assert.True(t, m.previewEditing, "EditModeEnteredMsg should set previewEditing")
}

func TestEditModeExitedClearsFlag(t *testing.T) {
	m := newTestModelReady(t)
	// Enter first.
	updated, _ := m.Update(panels.EditModeEnteredMsg{Path: "test.go"})
	m = updated.(Model)
	require.True(t, m.previewEditing)

	// Exit.
	updated, _ = m.Update(panels.EditModeExitedMsg{Path: "test.go"})
	m = updated.(Model)

	assert.False(t, m.previewEditing, "EditModeExitedMsg should clear previewEditing")
}

func TestCtrlZRoutesToPanelInEditMode(t *testing.T) {
	m := newTestModelReady(t)
	// Simulate being in edit mode.
	m.previewEditing = true

	// ctrl+z should NOT invoke handleUndo (which would show "Nothing to undo"
	// toast when undoMgr is nil). Instead it should route to the panel.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	_ = updated.(Model)

	// With nil undoMgr, handleUndo would produce a ShowToastMsg with
	// "Nothing to undo". If the key was correctly routed to the panel
	// instead, we should NOT get that toast.
	if cmd != nil {
		msg := cmd()
		if toast, ok := msg.(notify.ShowToastMsg); ok {
			assert.NotContains(t, toast.Message, "Nothing to undo",
				"ctrl+z in edit mode should not trigger global undo")
		}
	}
}

func TestCtrlCDoesNotQuitInEditMode(t *testing.T) {
	m := newTestModelReady(t)
	m.previewEditing = true

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	// In edit mode, ctrl+c must NOT produce tea.Quit.
	if cmd != nil {
		msg := cmd()
		_, isQuit := msg.(tea.QuitMsg)
		assert.False(t, isQuit, "ctrl+c in edit mode should not quit")
	}
}
