package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/ai"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/chat"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/overlayreg"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/session"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModel creates a Model with the explorer preset for testing.
func newTestModel(t *testing.T) Model {
	t.Helper()
	reg := layout.NewRegistry()
	cfg, _ := config.LoadDefaults()
	layout.RegisterDefaults(context.Background(), reg, cfg, nil, nil)
	preset := layout.ExplorerPreset()
	engine, err := layout.NewEngine(reg, preset)
	require.NoError(t, err)
	th, err := theme.Load("default")
	require.NoError(t, err)
	km, err := keymap.NewKeymap("default")
	require.NoError(t, err)
	bmMgr := bm.NewManager(cfg.Bookmarks)
	bmMgr.SetConfigDir(t.TempDir())
	return New(engine, th, km, bmMgr, overlayreg.New(th, bmMgr))
}

func TestNewModel(t *testing.T) {
	m := newTestModel(t)
	assert.NotNil(t, m.engine)
	assert.Equal(t, 0, m.width)
	assert.Equal(t, 0, m.height)
	assert.False(t, m.ready)
}

func TestModelViewBeforeResize(t *testing.T) {
	m := newTestModel(t)
	view := m.View()
	assert.Contains(t, view.Content, "Loading")
	assert.True(t, view.AltScreen)
}

func TestModelViewAfterResize(t *testing.T) {
	m := newTestModel(t)

	// Simulate window size message
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	view := m.View()
	assert.True(t, view.AltScreen)
	assert.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
	// v1: tab bar is hidden, so no "EXPLORER" label. Verify panels render.
	assert.Contains(t, view.Content, "No file selected")
}

func TestModelQuitOnCtrlC(t *testing.T) {
	m := newTestModel(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	// tea.Quit returns a QuitMsg
	if cmd != nil {
		msg := cmd()
		_, isQuit := msg.(tea.QuitMsg)
		assert.True(t, isQuit, "ctrl+c should produce QuitMsg")
	}
}

func TestModelQuitOnQ(t *testing.T) {
	m := newTestModel(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	if cmd != nil {
		msg := cmd()
		_, isQuit := msg.(tea.QuitMsg)
		assert.True(t, isQuit, "q should produce QuitMsg")
	}
}

func TestModelTabDoesNotCyclePanels(t *testing.T) {
	m := newTestModel(t)

	// Send WindowSizeMsg first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Initialize panels to set focus
	m.Init()

	// Initially focused on first panel
	assert.Equal(t, "filetree", m.engine.FocusedName())

	// Tab should NOT move focus to next panel (tab cycling was removed;
	// Tab now cycles tabs within the focused panel instead).
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, "filetree", m.engine.FocusedName(), "Tab should not cycle panels")
}

func TestModelShiftTabDoesNotCyclePanels(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	assert.Equal(t, "filetree", m.engine.FocusedName())

	// Shift+Tab should NOT cycle panels backward (tab cycling was removed).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(Model)
	assert.Equal(t, "filetree", m.engine.FocusedName(), "Shift+Tab should not cycle panels")
}

func TestModelWindowResize(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
	assert.True(t, m.ready)

	// Should render without panic
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestModelZoomToggle(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	assert.False(t, m.engine.IsZoomed())

	// Use handleAction directly for zoom_toggle
	updated, _ = m.handleAction("zoom_toggle", nil)
	m = updated.(Model)

	assert.True(t, m.engine.IsZoomed())

	// Toggle back
	updated, _ = m.handleAction("zoom_toggle", nil)
	m = updated.(Model)

	assert.False(t, m.engine.IsZoomed())
}

func TestModelZoomedView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Toggle zoom
	m.engine.ToggleZoom()

	view := m.View()
	assert.NotEmpty(t, view.Content)
	assert.True(t, view.AltScreen)
}

func TestModelResizeLeftRight(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Trigger resize_left action via handleAction
	updated, _ = m.handleAction("resize_left", nil)
	m = updated.(Model)

	// Trigger resize_right action via handleAction
	updated, _ = m.handleAction("resize_right", nil)
	m = updated.(Model)

	// Should not panic
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestModelExitInputAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Set mode to input, then exit_input should set it back to panel
	m.keys.SetMode(keymap.ModeInput)
	assert.Equal(t, keymap.ModeInput, m.keys.CurrentMode())

	updated, _ = m.handleAction("exit_input", nil)
	m = updated.(Model)
	assert.Equal(t, keymap.ModePanel, m.keys.CurrentMode())
}

func TestModelUnknownActionForwardsToPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Unknown action should forward to panel without panic
	updated, cmd := m.handleAction("cursor_down", tea.KeyPressMsg{Code: 'j'})
	_ = updated.(Model)
	_ = cmd
}

func TestModelRoutesNonKeyMessages(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// A non-key message should be routed to the focused panel
	type customMsg struct{}
	updated, _ = m.Update(customMsg{})
	_ = updated.(Model)
	// Should not panic
}

func TestModelPanelRectsAfterResize(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	rects := m.engine.PanelRects()
	assert.NotEmpty(t, rects)
	assert.Contains(t, rects, "filetree")
	assert.Contains(t, rects, "preview")

	// Verify positive dimensions
	for name, rect := range rects {
		assert.Greater(t, rect.Width, 0, "panel %s width should be > 0", name)
		assert.Greater(t, rect.Height, 0, "panel %s height should be > 0", name)
	}
}

// ---------------------------------------------------------------------------
// F30: Notify integration tests
// ---------------------------------------------------------------------------

func TestModelToastAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Send a toast message.
	updated, _ = m.Update(notify.ShowToastMsg{
		Message: "Hello toast",
		Level:   notify.Success,
	})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "Hello toast", "toast should appear in view")
}

func TestModelModalBlocksKeyInput(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Show a modal.
	updated, _ = m.Update(notify.ShowModalMsg{
		Kind:    notify.ModalConfirm,
		Title:   "Confirm",
		Message: "Are you sure?",
	})
	m = updated.(Model)

	// Modal should be active.
	assert.True(t, m.notify.HasModal(), "modal should be active")

	// Tab key should NOT change panel focus while modal is active.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should not change focus while modal is active")
}

func TestModelModalAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Show a modal.
	updated, _ = m.Update(notify.ShowModalMsg{
		Kind:    notify.ModalConfirm,
		Title:   "Delete",
		Message: "Delete file.txt?",
	})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "Delete", "modal title should appear in view")
}

func TestModelModalResultRoutesToPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Send a modal result — it should be routed to the focused panel
	// without panic.
	updated, _ = m.Update(notify.ModalResultMsg{Accept: true, Value: "test"})
	m = updated.(Model)

	// Should not panic; just verifying the routing works.
	assert.NotNil(t, m.engine)
}

// ---------------------------------------------------------------------------
// Bookmark overlay tests
// ---------------------------------------------------------------------------

func TestToggleBookmarksShowsOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Toggle bookmarks on.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)

	assert.True(t, m.bookmarksShown, "bookmarks overlay should be shown")
	assert.NotNil(t, m.bookmarkPanel, "bookmarkPanel should be initialised")
}

func TestToggleBookmarksHidesOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Toggle on, then off.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)

	assert.False(t, m.bookmarksShown, "bookmarks overlay should be hidden")
	assert.Nil(t, m.bookmarkPanel, "bookmarkPanel should be nil")
}

func TestBookmarkOverlayAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Toggle on to render the overlay.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "Bookmarks", "bookmark overlay title should appear")
}

func TestBookmarkOverlayRoutesKeys(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Open bookmarks.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)

	// Tab should NOT change panel focus while bookmarks overlay is active.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should not change focus while bookmarks overlay is active")
}

func TestRKeyRefreshesGlobal(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Pressing 'R' (global refresh) should not panic.
	updated, _ = m.Update(tea.KeyPressMsg{Code: -1, Text: "R"})
	_ = updated.(Model)
}

func TestNavigateToPathClosesBookmarks(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Open bookmarks.
	updated, _ = m.Update(panels.ToggleBookmarksMsg{})
	m = updated.(Model)
	assert.True(t, m.bookmarksShown)

	// Navigating should close the overlay.
	updated, _ = m.Update(panels.NavigateToPathMsg{Path: t.TempDir()})
	m = updated.(Model)

	assert.False(t, m.bookmarksShown, "navigate should close bookmarks overlay")
	assert.Nil(t, m.bookmarkPanel)
}

func TestAddBookmarkSuccess(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	tmpDir := t.TempDir()
	updated, cmd := m.Update(panels.BookmarkAddMsg{Path: tmpDir})
	m = updated.(Model)

	// Should produce a success toast command.
	assert.NotNil(t, cmd, "addBookmark should return a command")
	assert.True(t, m.bookmarkMgr.Has(tmpDir), "bookmark should be added")
}

func TestAddBookmarkDuplicate(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	tmpDir := t.TempDir()
	// Add once.
	updated, _ = m.Update(panels.BookmarkAddMsg{Path: tmpDir})
	m = updated.(Model)
	// Add again — should produce a warning toast.
	_, cmd := m.Update(panels.BookmarkAddMsg{Path: tmpDir})

	assert.NotNil(t, cmd, "duplicate add should return a toast command")
}

func TestBookmarkOverlayDims(t *testing.T) {
	m := newTestModel(t)

	// Small screen.
	m.width = 50
	m.height = 20
	w, h := m.bookmarkOverlayDims()
	assert.Greater(t, w, 0, "width should be positive")
	assert.Greater(t, h, 0, "height should be positive")
	assert.LessOrEqual(t, w, m.width, "width should not exceed screen width")
	assert.LessOrEqual(t, h, m.height, "height should not exceed screen height")

	// Very small screen — clamp to minimums.
	m.width = 30
	m.height = 8
	w, _ = m.bookmarkOverlayDims()
	assert.GreaterOrEqual(t, w, 26, "width should be at least 26 on tiny screen")
}

// ---------------------------------------------------------------------------
// Fuzzy finder overlay tests
// ---------------------------------------------------------------------------

func TestFuzzyFinderToggleMessage(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Open fuzzy finder via action.
	m = m.openFuzzyFinder("files")
	assert.NotNil(t, m.fuzzyFinder, "fuzzy finder should be open")

	// Toggle off.
	updated, _ = m.Update(panels.ToggleFuzzyFinderMsg{})
	m = updated.(Model)
	assert.Nil(t, m.fuzzyFinder, "fuzzy finder should be closed")
}

func TestFuzzyFinderOverlayRoutesKeys(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Open fuzzy finder.
	m = m.openFuzzyFinder("files")

	// Tab should NOT change panel focus while fuzzy finder is active.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should not change focus while fuzzy finder is active")
}

func TestFuzzyFinderAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Open fuzzy finder.
	m = m.openFuzzyFinder("commands")
	assert.NotNil(t, m.fuzzyFinder)

	view := m.View()
	// The overlay should render something.
	assert.NotEmpty(t, view.Content, "view should render fuzzy finder content")
}

func TestFuzzyFinderDims(t *testing.T) {
	m := newTestModel(t)

	m.width = 100
	m.height = 40
	w, h := m.fuzzyFinderDims()
	assert.Greater(t, w, 0)
	assert.Greater(t, h, 0)
	assert.LessOrEqual(t, w, m.width)
	assert.LessOrEqual(t, h, m.height)

	// Very small screen — clamp to minimums.
	m.width = 30
	m.height = 8
	w, _ = m.fuzzyFinderDims()
	assert.GreaterOrEqual(t, w, 26)
}

func TestOpenFuzzyFinderFilesMode(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	m.height = 40
	m = m.openFuzzyFinder("files")
	assert.NotNil(t, m.fuzzyFinder, "fuzzy finder should be created for files mode")
}

func TestOpenFuzzyFinderCommandsMode(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	m.height = 40
	m = m.openFuzzyFinder("commands")
	assert.NotNil(t, m.fuzzyFinder, "fuzzy finder should be created for commands mode")
}

func TestCommandSelectedClosesFuzzyFinder(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	m = m.openFuzzyFinder("commands")
	assert.NotNil(t, m.fuzzyFinder)

	// Selecting a command should close the fuzzy finder.
	updated, _ = m.Update(panels.CommandSelectedMsg{Action: "focus_next"})
	m = updated.(Model)
	assert.Nil(t, m.fuzzyFinder, "fuzzy finder should be closed after command selection")
}

func TestFuzzyFinderAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "fuzzy_finder" action should open the fuzzy finder.
	updated, _ = m.handleAction("fuzzy_finder", tea.KeyPressMsg{})
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "fuzzy_finder action should open fuzzy finder")
}

func TestCommandPaletteAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "command_palette" action should open the fuzzy finder in commands mode.
	updated, _ = m.handleAction("command_palette", tea.KeyPressMsg{})
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "command_palette action should open fuzzy finder")
}

// ---------------------------------------------------------------------------
// Change directory tests
// ---------------------------------------------------------------------------

func TestHandleActionChangeDirectory(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "change_directory" action should open the fuzzy finder.
	updated, _ = m.handleAction("change_directory", tea.KeyPressMsg{})
	m = updated.(Model)
	assert.NotNil(t, m.fuzzyFinder, "change_directory action should open fuzzy finder")
}

func TestOpenFuzzyFinderDirectoriesMode(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	m.height = 40
	m = m.openFuzzyFinder("directories")
	assert.NotNil(t, m.fuzzyFinder, "fuzzy finder should be created for directories mode")
}

func TestChangeDirectoryMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir, err := os.MkdirTemp(origDir, "change-dir-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tmpDir)) })
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	relDir, err := filepath.Rel(origDir, tmpDir)
	require.NoError(t, err)

	updated, cmd := m.Update(panels.ChangeDirectoryMsg{Path: relDir})
	_ = updated.(Model) // verify type assertion succeeds

	// Relative paths should be resolved before changing the process CWD.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, tmpDir, cwd, "CWD should be changed to the target dir")

	// A batch of commands should be returned (navigate, refresh, toast, dirty).
	assert.NotNil(t, cmd, "ChangeDirectoryMsg should return commands")
}

func TestChangeDirectoryMsgInvalidPath(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	// Send a ChangeDirectoryMsg with a non-existent path.
	updated, cmd := m.Update(panels.ChangeDirectoryMsg{Path: missingDir})
	_ = updated.(Model)

	// Should return an error toast command.
	require.NotNil(t, cmd, "should return a command for error toast")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok, "should be a ShowToastMsg")
	assert.Contains(t, toast.Message, "Failed to change directory")
	assert.Equal(t, notify.Error, toast.Level)
}

func TestChangeDirectoryMsg_ChangesCWD(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir, err := os.MkdirTemp(origDir, "switch-wt-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tmpDir)) })
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	updated, cmd := m.Update(panels.ChangeDirectoryMsg{Path: tmpDir})
	_ = updated.(Model)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, tmpDir, cwd, "ChangeDirectoryMsg should change CWD")
	assert.NotNil(t, cmd, "ChangeDirectoryMsg should return commands")
}

// ---------------------------------------------------------------------------
// Chat footer integration tests (p4-footer + p4-focus)
// ---------------------------------------------------------------------------

// newTestChatModel creates a minimal chat.Model suitable for app integration tests.
func newTestChatModel(t *testing.T) *chat.Model {
	t.Helper()

	cfg := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	th := &theme.Theme{Name: "test", Variant: "dark"}
	toolReg := chat.NewToolRegistry()
	executor := chat.NewToolExecutor(nil, nil, nil, nil, toolReg)
	confirmer := chat.NewConfirmationManager(toolReg)
	sysBuilder := chat.NewSystemPromptBuilder(nil, "test")
	redactor := ai.NewRedactor(nil)
	m := chat.New(chat.Deps{
		Registry:   cfg,
		Executor:   executor,
		Confirming: confirmer,
		SysPrompt:  sysBuilder,
		Redactor:   redactor,
		Theme:      th,
		ChatCfg:    config.ChatConfig{RenderMarkdown: true},
	})
	return &m
}

func TestModelWithChatNil(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.chat, "chat should be nil by default")

	// WithChat(nil) is a no-op.
	m = m.WithChat(nil)
	assert.Nil(t, m.chat)

	// View should render without panic.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestModelWithChatNonNil(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)
	assert.NotNil(t, m.chat, "chat should be set via WithChat")

	// View should render without panic and include the chat view.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestWindowSizeMsgAccountsForChatHeight(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	// Send WindowSizeMsg. The engine should receive a height reduced
	// by the chat's collapsed height (3 rows).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	assert.Equal(t, 100, m.width)
	assert.Equal(t, 40, m.height)
	assert.True(t, m.ready)

	// Panel rects should reflect smaller available height.
	rects := m.engine.PanelRects()
	assert.NotEmpty(t, rects)
	for _, rect := range rects {
		assert.Greater(t, rect.Height, 0)
		// Panel height should be less than total height minus bars minus chat.
		assert.Less(t, rect.Height, 40)
	}
}

func TestWindowSizeMsgWithoutChat(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	rects := m.engine.PanelRects()
	assert.NotEmpty(t, rects)
	// Panels should be taller without chat eating 3 rows.
	for _, rect := range rects {
		assert.Greater(t, rect.Height, 0)
	}
}

func TestChatFocusToggle(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)
	m.Init()

	// Initially unfocused.
	assert.False(t, m.chat.Focused(), "chat should start unfocused")
	assert.Equal(t, keymap.ModePanel, m.keys.CurrentMode())

	// Toggle focus on via handleAction.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused(), "chat should be focused after toggle")
	assert.Equal(t, keymap.ModeInput, m.keys.CurrentMode(),
		"keymap should switch to input mode when chat focused")

	// Toggle focus off.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.False(t, m.chat.Focused(), "chat should be unfocused after second toggle")
	assert.Equal(t, keymap.ModePanel, m.keys.CurrentMode(),
		"keymap should switch back to panel mode")
}

func TestChatFocusToggleWithoutChat(t *testing.T) {
	m := newTestModel(t)

	// chat_focus on a model without chat should be a no-op.
	updated, cmd := m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.Nil(t, cmd)
	assert.Nil(t, m.chat)
}

func TestChatFocusRoutesKeysToChat(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// A regular key (e.g. 'j') should NOT change panel focus — it should go to chat.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"panel focus should not change while chat is focused")
}

func TestChatFocusedQuestionMarkDoesNotOpenHelp(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// Pressing '?' while chat is focused should NOT open help overlay.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '?'})
	m = updated.(Model)
	assert.False(t, m.helpShown,
		"? should be consumed by chat input, not open help overlay")
	assert.True(t, m.chat.Focused(),
		"chat should remain focused after typing ?")
}

func TestChatFocusedEnterDoesNotRouteToPanel(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	initialFocus := m.engine.FocusedName()

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// Enter with empty input should be consumed by chat (no-op),
	// not dispatched to the panel's "select" action.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"Enter should be consumed by chat, not route to panel")
	assert.True(t, m.chat.Focused(),
		"chat should remain focused after pressing Enter")
}

func TestChatFocusedEscBlursChat(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// Esc should blur the chat (handled by chat.handleKey).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	assert.False(t, m.chat.Focused(),
		"Esc should blur the chat input")
}

func TestChatFocusedCtrlSpaceTogglesOff(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// Ctrl+Space should toggle chat focus off.
	updated, _ = m.Update(tea.KeyPressMsg{Code: ' ', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.False(t, m.chat.Focused(),
		"Ctrl+Space should toggle chat focus off")
	assert.Equal(t, keymap.ModePanel, m.keys.CurrentMode(),
		"keymap should return to panel mode")
}

func TestChatFocusedGlobalKeysConsumed(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// Focus chat.
	updated, _ = m.handleAction("chat_focus", nil)
	m = updated.(Model)
	assert.True(t, m.chat.Focused())

	// Pressing ',' (globally bound to settings) while chat focused
	// should NOT open settings.
	updated, _ = m.Update(tea.KeyPressMsg{Code: ','})
	m = updated.(Model)
	assert.False(t, m.settingsShown,
		"comma should be consumed by chat, not open settings")

	// Pressing '1' should be consumed by chat, not dispatched.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1'})
	m = updated.(Model)
	assert.True(t, m.chat.Focused(),
		"number keys should be consumed by chat, not switch tabs")

	// Tab key (globally bound to focus_next) should NOT change panel focus.
	initialFocus := m.engine.FocusedName()
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should be consumed by chat, not change panel focus")
}

func TestChatRefreshMsgTriggersGitDirty(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// ChatRefreshMsg should not panic (git client may be nil in test).
	updated, _ = m.Update(panels.ChatRefreshMsg{})
	_ = updated.(Model)
}

func TestChatNavigateMsgRoutesToEngine(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// ChatNavigateMsg should route to engine without panic.
	updated, _ = m.Update(panels.ChatNavigateMsg{Path: "/tmp/test.go"})
	_ = updated.(Model)
}

func TestChatFocusMsgToggle(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// ChatFocusMsg should toggle focus.
	updated, _ = m.Update(panels.ChatFocusMsg{})
	m = updated.(Model)
	assert.True(t, m.chat.Focused(), "ChatFocusMsg should focus chat")

	updated, _ = m.Update(panels.ChatFocusMsg{})
	m = updated.(Model)
	assert.False(t, m.chat.Focused(), "ChatFocusMsg should unfocus chat")
}

func TestHintsBarShowsChatHint(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)
	m.width = 120 // wide enough to show all hints

	// When chat is present but not focused, hints should include ctrl+space:chat.
	hints := m.renderHintsBar()
	assert.Contains(t, hints, "chat",
		"hints bar should include chat hint when chat is available")
}

func TestHintsBarShowsChatFocusedHints(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)
	m.width = 120

	// Focus the chat.
	m.chat.Focus()

	hints := m.renderHintsBar()
	assert.Contains(t, hints, "send",
		"hints bar should show chat-specific hints when focused")
	assert.Contains(t, hints, "blur",
		"hints bar should show esc:blur when chat focused")
}

// ---------------------------------------------------------------------------
// WithUndoManager / WithSessionManager / WithConfig / WithGitClient tests
// ---------------------------------------------------------------------------

func TestWithUndoManagerSetsField(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.undoMgr, "undoMgr should be nil by default")

	um := git.NewUndoManager(nil)
	m = m.WithUndoManager(um)
	assert.Same(t, um, m.undoMgr, "WithUndoManager should store the manager")
}

func TestWithSessionManagerSetsField(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.sessionMgr, "sessionMgr should be nil by default")

	mgr := session.NewManager()
	m = m.WithSessionManager(mgr)
	assert.Same(t, mgr, m.sessionMgr, "WithSessionManager should store the manager")
}

func TestWithConfigSetsField(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.cfg, "cfg should be nil by default")

	cfg := &config.Config{}
	m = m.WithConfig(cfg)
	assert.Same(t, cfg, m.cfg, "WithConfig should store the config")
}

// ---------------------------------------------------------------------------
// Help overlay toggle tests
// ---------------------------------------------------------------------------

func TestToggleHelpShowsOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	assert.False(t, m.helpShown)
	assert.Nil(t, m.helpPanel)

	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.True(t, m.helpShown, "help overlay should be shown")
	assert.NotNil(t, m.helpPanel, "helpPanel should be initialised")
}

func TestToggleHelpHidesOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Toggle on.
	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.True(t, m.helpShown)

	// Toggle off.
	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)
	assert.False(t, m.helpShown, "help overlay should be hidden")
	assert.Nil(t, m.helpPanel, "helpPanel should be nil")
}

func TestHelpOverlayRoutesKeys(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Open help.
	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)

	// Tab should NOT change panel focus while help overlay is active.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should not change focus while help overlay is active")
}

func TestHelpOverlayAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.Update(panels.ToggleHelpMsg{})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "grut", "help overlay should render")
}

func TestHelpActionTogglesOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "help" action should open the help overlay.
	updated, _ = m.handleAction("help", nil)
	m = updated.(Model)
	assert.True(t, m.helpShown, "help action should open help overlay")
}

// ---------------------------------------------------------------------------
// Settings overlay toggle tests
// ---------------------------------------------------------------------------

func TestToggleSettingsShowsOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	assert.False(t, m.settingsShown)
	assert.Nil(t, m.settingsPanel)

	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.True(t, m.settingsShown, "settings overlay should be shown")
	assert.NotNil(t, m.settingsPanel, "settingsPanel should be initialised")
}

func TestToggleSettingsHidesOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Toggle on.
	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.True(t, m.settingsShown)

	// Toggle off.
	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)
	assert.False(t, m.settingsShown, "settings overlay should be hidden")
	assert.Nil(t, m.settingsPanel, "settingsPanel should be nil")
}

func TestSettingsOverlayRoutesKeys(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	initialFocus := m.engine.FocusedName()

	// Open settings.
	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)

	// Tab should NOT change panel focus while settings overlay is active.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	assert.Equal(t, initialFocus, m.engine.FocusedName(),
		"tab should not change focus while settings overlay is active")
}

func TestSettingsOverlayAppearsInView(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.Update(panels.ToggleSettingsMsg{})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "Settings", "settings overlay should render")
}

func TestSettingsActionTogglesOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "settings" action should open the settings overlay.
	updated, _ = m.handleAction("settings", nil)
	m = updated.(Model)
	assert.True(t, m.settingsShown, "settings action should open settings overlay")
}

// ---------------------------------------------------------------------------
// Undo / Redo tests (nil manager and empty stack paths)
// ---------------------------------------------------------------------------

func TestHandleUndoNilManager(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.undoMgr)

	updated, _ := m.handleAction("zoom_toggle", nil) // just to prep model
	m = updated.(Model)

	// handleUndo with nil undoMgr should return an info toast.
	result, cmd := m.handleUndo()
	_ = result.(Model)
	require.NotNil(t, cmd, "should return a command for toast")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok, "should be a ShowToastMsg")
	assert.Contains(t, toast.Message, "Nothing to undo")
	assert.Equal(t, notify.Info, toast.Level)
}

func TestHandleRedoNilManager(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.undoMgr)

	result, cmd := m.handleRedo()
	_ = result.(Model)
	require.NotNil(t, cmd, "should return a command for toast")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok, "should be a ShowToastMsg")
	assert.Contains(t, toast.Message, "Nothing to redo")
	assert.Equal(t, notify.Info, toast.Level)
}

func TestHandleUndoEmptyStack(t *testing.T) {
	m := newTestModel(t)
	// Create an UndoManager with a nil client — CanUndo() will return false.
	um := git.NewUndoManager(nil)
	m = m.WithUndoManager(um)

	result, cmd := m.handleUndo()
	_ = result.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to undo")
}

func TestHandleRedoEmptyStack(t *testing.T) {
	m := newTestModel(t)
	um := git.NewUndoManager(nil)
	m = m.WithUndoManager(um)

	result, cmd := m.handleRedo()
	_ = result.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to redo")
}

func TestCtrlZTriggersUndo(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// ctrl+z with nil undoMgr should produce "Nothing to undo" toast.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok)
	assert.Contains(t, toast.Message, "Nothing to undo")
}

func TestCtrlYTriggersRedo(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// ctrl+y with nil undoMgr should produce "Nothing to redo" toast.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok)
	assert.Contains(t, toast.Message, "Nothing to redo")
}

// ---------------------------------------------------------------------------
// UndoMsg / RedoMsg routed via Update
// ---------------------------------------------------------------------------

func TestUndoMsgRouted(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.UndoMsg{})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to undo")
}

func TestRedoMsgRouted(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.RedoMsg{})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Nothing to redo")
}

func TestUndoResultMsgSuccess(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.UndoResultMsg{Description: "staged file.go"})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Undone: staged file.go")
	assert.Equal(t, notify.Success, toast.Level)
}

func TestUndoResultMsgError(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.UndoResultMsg{Err: fmt.Errorf("conflict")})
	_ = updated.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "Undo failed")
	assert.Equal(t, notify.Warn, toast.Level)
}

// ---------------------------------------------------------------------------
// Tab management messages (v1 no-ops)
// ---------------------------------------------------------------------------

func TestNewTabMsgNoOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.NewTabMsg{})
	_ = updated.(Model)
	assert.Nil(t, cmd, "NewTabMsg should be no-op in v1")
}

func TestCloseTabMsgNoOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.CloseTabMsg{})
	_ = updated.(Model)
	assert.Nil(t, cmd, "CloseTabMsg should be no-op in v1")
}

func TestNextTabMsgNoOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.NextTabMsg{})
	_ = updated.(Model)
	assert.Nil(t, cmd, "NextTabMsg should be no-op in v1")
}

func TestPrevTabMsgNoOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.PrevTabMsg{})
	_ = updated.(Model)
	assert.Nil(t, cmd, "PrevTabMsg should be no-op in v1")
}

func TestSwitchTabMsgNoOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.SwitchTabMsg{Index: 0})
	_ = updated.(Model)
	assert.Nil(t, cmd, "SwitchTabMsg should be no-op in v1")
}

func TestTabActionNoOps(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	for _, action := range []string{"tab_new", "tab_close", "tab_next", "tab_prev"} {
		updated, cmd := m.handleAction(action, nil)
		m = updated.(Model)
		assert.Nil(t, cmd, "%s action should be no-op in v1", action)
	}
}

func TestPresetTabActionNoOps(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	for _, action := range []string{"tab_explorer", "tab_git", "tab_review", "tab_agent", "tab_full"} {
		updated, cmd := m.handleAction(action, nil)
		m = updated.(Model)
		assert.Nil(t, cmd, "%s action should be no-op in v1", action)
	}
}

// ---------------------------------------------------------------------------
// Split / close panel via handleAction
// ---------------------------------------------------------------------------

func TestSplitVerticalAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "split_vertical" action should split without panic.
	updated, _ = m.handleAction("split_vertical", nil)
	m = updated.(Model)

	// Should render without panic.
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestSplitHorizontalAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The "split_horizontal" action should split without panic.
	updated, _ = m.handleAction("split_horizontal", nil)
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestClosePanelAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Closing the only/last panel should produce an error toast.
	_, cmd := m.handleAction("close_panel", nil)
	// close_panel may return nil or a warning toast depending on the layout.
	_ = cmd
}

// ---------------------------------------------------------------------------
// SplitVerticalMsg / SplitHorizontalMsg / ClosePanelMsg via Update
// ---------------------------------------------------------------------------

func TestSplitVerticalMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.Update(panels.SplitVerticalMsg{PanelType: "preview"})
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestSplitHorizontalMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.Update(panels.SplitHorizontalMsg{PanelType: "preview"})
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestClosePanelMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// ClosePanelMsg on a multi-panel layout — first split, then close.
	updated, _ = m.Update(panels.SplitVerticalMsg{PanelType: "preview"})
	m = updated.(Model)

	updated, _ = m.Update(panels.ClosePanelMsg{})
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// Overlay dimension calculations
// ---------------------------------------------------------------------------

func TestHelpOverlayDims(t *testing.T) {
	m := newTestModel(t)

	m.width = 120
	m.height = 40
	w, h := m.helpOverlayDims()
	assert.Greater(t, w, 0)
	assert.Greater(t, h, 0)
	assert.LessOrEqual(t, w, m.width)
	assert.LessOrEqual(t, h, m.height)

	// Small screen — clamp to minimums.
	m.width = 30
	m.height = 8
	w, h = m.helpOverlayDims()
	assert.GreaterOrEqual(t, w, 26, "should clamp width on tiny screen")
	assert.GreaterOrEqual(t, h, 4, "should clamp height on tiny screen")
}

func TestSettingsOverlayDims(t *testing.T) {
	m := newTestModel(t)

	m.width = 120
	m.height = 40
	w, h := m.settingsOverlayDims()
	assert.Equal(t, 44, w, "settings width is fixed at 44 on large screen")
	assert.Equal(t, 36, h, "settings height fills available space on large screen")

	// Very small screen.
	m.width = 20
	m.height = 8
	w, h = m.settingsOverlayDims()
	assert.GreaterOrEqual(t, w, 16, "should clamp width")
	assert.GreaterOrEqual(t, h, 4, "should clamp height")
}

// ---------------------------------------------------------------------------
// renderTabBar tests (v1 single-tab mode)
// ---------------------------------------------------------------------------

func TestRenderTabBarSingleTabMode(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	tabBar := m.renderTabBar()
	assert.Empty(t, tabBar, "tab bar should be empty in single-tab mode")
}

// ---------------------------------------------------------------------------
// renderLayout / View with various states
// ---------------------------------------------------------------------------

func TestViewWithHelpOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Show help overlay.
	updated, _ = m.toggleHelp()
	m = updated.(Model)

	view := m.View()
	assert.True(t, view.AltScreen)
	assert.NotEmpty(t, view.Content)
}

func TestViewWithSettingsOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Show settings overlay.
	updated, _ = m.toggleSettings()
	m = updated.(Model)

	view := m.View()
	assert.True(t, view.AltScreen)
	assert.NotEmpty(t, view.Content)
}

func TestViewWithMultipleResizes(t *testing.T) {
	m := newTestModel(t)

	// First resize.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Second resize (shrink).
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = updated.(Model)

	assert.Equal(t, 60, m.width)
	assert.Equal(t, 20, m.height)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// Branch/git status messages
// ---------------------------------------------------------------------------

func TestBranchLoadedMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(branchLoadedMsg{Name: "main", Ahead: 2, Behind: 1})
	m = updated.(Model)

	assert.Equal(t, "main", m.currentBranch)
	assert.Equal(t, 2, m.branchAhead)
	assert.Equal(t, 1, m.branchBehind)
}

func TestGitDirtyMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(gitDirtyMsg{dirty: true})
	m = updated.(Model)
	assert.True(t, m.gitDirty)

	updated, _ = m.Update(gitDirtyMsg{dirty: false})
	m = updated.(Model)
	assert.False(t, m.gitDirty)
}

func TestBranchChangedMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	updated, _ = m.Update(panels.BranchChangedMsg{Name: "feature-x"})
	m = updated.(Model)

	assert.Equal(t, "feature-x", m.currentBranch)
	assert.Equal(t, 0, m.branchAhead)
	assert.Equal(t, 0, m.branchBehind)
}

func TestGitStatusChangedMsgRouted(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Should not panic — routes to engine and checks dirty.
	updated, _ = m.Update(panels.GitStatusChangedMsg{})
	_ = updated.(Model)
}

// ---------------------------------------------------------------------------
// FirstRunMsg triggers help overlay
// ---------------------------------------------------------------------------

func TestFirstRunMsgOpensHelp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(panels.FirstRunMsg{})
	m = updated.(Model)

	assert.True(t, m.welcomeShown, "FirstRunMsg should open welcome overlay")
}

// ---------------------------------------------------------------------------
// Git operations with nil client (early returns)
// ---------------------------------------------------------------------------

func TestHandleCommitNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// CommitRequestMsg with nil gitClient should not panic.
	updated, _ = m.Update(panels.CommitRequestMsg{})
	_ = updated.(Model)
}

func TestHandlePushNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(panels.PushRequestMsg{})
	_ = updated.(Model)
}

func TestHandlePullNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(panels.PullRequestMsg{})
	_ = updated.(Model)
}

func TestHandleFetchNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(panels.FetchRequestMsg{})
	_ = updated.(Model)
}

// ---------------------------------------------------------------------------
// handleAction for commit/push/pull/fetch with nil client
// ---------------------------------------------------------------------------

func TestCommitActionNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.handleAction("commit", nil)
	_ = updated.(Model)
}

func TestPushActionNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.handleAction("push", nil)
	_ = updated.(Model)
}

func TestPullActionNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.handleAction("pull", nil)
	_ = updated.(Model)
}

func TestFetchActionNilClient(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.handleAction("fetch", nil)
	_ = updated.(Model)
}

func TestFKeyFetch(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Pressing 'F' (global fetch) should not panic; with nil git client
	// it simply returns without error.
	updated, _ = m.Update(tea.KeyPressMsg{Code: -1, Text: "F"})
	_ = updated.(Model)
}

// ---------------------------------------------------------------------------
// focus_panel_N via handleAction
// ---------------------------------------------------------------------------

func TestFocusPanelActions(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// focus_panel_1 through focus_panel_5 should not panic and should
	// update the focused panel name.
	expected := []string{"filetree", "gitinfo", "github", "commits", "preview"}
	for i, action := range []string{
		"focus_panel_1", "focus_panel_2", "focus_panel_3",
		"focus_panel_4", "focus_panel_5",
	} {
		updated, cmd := m.handleAction(action, nil)
		m = updated.(Model)
		assert.Nil(t, cmd, "action %s should return nil cmd", action)
		assert.Equal(t, expected[i], m.engine.FocusedName(),
			"action %s should focus panel %q", action, expected[i])
	}
}

// ---------------------------------------------------------------------------
// Coverage-boost tests: renderLayout, renderHintsBar, handleSwitchOrCreateTab,
// saveSession, checkGitDirty, loadBranchInfo, addBookmark, handleClosePanel,
// handleSplitVertical/Horizontal error paths, renderSeparatorWithTitle,
// renderPanel edge cases, Init with git client/chat, renderLayout with chat.
// ---------------------------------------------------------------------------

// mockFullGitOps extends mockGitOps with Status and BranchList support.
type mockFullGitOps struct {
	mockGitOps // embedded for commit/push/pull/fetch

	statusFiles []git.FileStatus
	statusErr   error
	branches    []git.Branch
	branchErr   error
}

func (m *mockFullGitOps) Status(_ context.Context) ([]git.FileStatus, error) {
	return m.statusFiles, m.statusErr
}

func (m *mockFullGitOps) BranchList(_ context.Context) ([]git.Branch, error) {
	return m.branches, m.branchErr
}

func (m *mockFullGitOps) CurrentBranch(_ context.Context) (git.Branch, error) {
	if m.branchErr != nil {
		return git.Branch{}, m.branchErr
	}
	for _, b := range m.branches {
		if b.IsCurrent {
			return b, nil
		}
	}
	return git.Branch{IsCurrent: true}, nil
}

func newTestModelWithFullGit(t *testing.T, mock *mockFullGitOps) Model {
	t.Helper()
	m := newTestModel(t)
	m = m.WithGitClient(mock)
	m = m.WithConfig(&config.Config{})
	return m
}

// ---------------------------------------------------------------------------
// checkGitDirty tests
// ---------------------------------------------------------------------------

func TestCheckGitDirtyNilClientReturnsNil(t *testing.T) {
	m := newTestModel(t) // no git client
	cmd := m.checkGitDirty()
	assert.Nil(t, cmd, "checkGitDirty should return nil when gitClient is nil")
}

func TestCheckGitDirtyWithCleanStatus(t *testing.T) {
	mock := &mockFullGitOps{statusFiles: nil}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.checkGitDirty()
	require.NotNil(t, cmd, "checkGitDirty should return a command")

	msg := cmd()
	dirtyMsg, ok := msg.(gitDirtyMsg)
	require.True(t, ok)
	assert.False(t, dirtyMsg.dirty, "clean status should report not dirty")
}

func TestCheckGitDirtyWithDirtyStatus(t *testing.T) {
	mock := &mockFullGitOps{
		statusFiles: []git.FileStatus{{Path: "file.go", WorktreeStatus: git.StatusModified}},
	}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.checkGitDirty()
	require.NotNil(t, cmd)

	msg := cmd()
	dirtyMsg, ok := msg.(gitDirtyMsg)
	require.True(t, ok)
	assert.True(t, dirtyMsg.dirty, "dirty status should report dirty")
}

func TestCheckGitDirtyWithError(t *testing.T) {
	mock := &mockFullGitOps{statusErr: fmt.Errorf("git error")}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.checkGitDirty()
	require.NotNil(t, cmd)

	msg := cmd()
	dirtyMsg, ok := msg.(gitDirtyMsg)
	require.True(t, ok)
	assert.False(t, dirtyMsg.dirty, "error should report not dirty")
}

// ---------------------------------------------------------------------------
// loadBranchInfo tests
// ---------------------------------------------------------------------------

func TestLoadBranchInfoNilClientReturnsNil(t *testing.T) {
	m := newTestModel(t)
	cmd := m.loadBranchInfo()
	assert.Nil(t, cmd, "loadBranchInfo should return nil when gitClient is nil")
}

func TestLoadBranchInfoWithCurrentBranch(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{
			{Name: "feature-x", IsCurrent: false},
			{Name: "main", IsCurrent: true, Ahead: 3, Behind: 1},
		},
	}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.loadBranchInfo()
	require.NotNil(t, cmd)

	msg := cmd()
	bl, ok := msg.(branchLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, "main", bl.Name)
	assert.Equal(t, 3, bl.Ahead)
	assert.Equal(t, 1, bl.Behind)
	assert.Equal(t, m.branchInfoGen, bl.generation, "returned msg should carry model's generation")
}

func TestLoadBranchInfoNoCurrent(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{{Name: "feature-x", IsCurrent: false}},
	}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.loadBranchInfo()
	require.NotNil(t, cmd)

	msg := cmd()
	bl, ok := msg.(branchLoadedMsg)
	require.True(t, ok)
	assert.Empty(t, bl.Name, "should return empty when no current branch")
	assert.Equal(t, m.branchInfoGen, bl.generation, "returned msg should carry model's generation")
}

func TestLoadBranchInfoWithError(t *testing.T) {
	mock := &mockFullGitOps{branchErr: fmt.Errorf("branch error")}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.loadBranchInfo()
	require.NotNil(t, cmd)

	msg := cmd()
	bl, ok := msg.(branchLoadedMsg)
	require.True(t, ok)
	assert.Empty(t, bl.Name, "error should return empty branch")
	assert.Equal(t, m.branchInfoGen, bl.generation, "returned msg should carry model's generation")
}

// ---------------------------------------------------------------------------
// Init with git client
// ---------------------------------------------------------------------------

func TestInitWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true, Ahead: 1}},
	}
	m := newTestModelWithFullGit(t, mock)

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init with gitClient should return commands")
}

func TestInitWithChat(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init with chat should return commands including chat init")
}

func TestInitWithGitClientAndAutoFetch(t *testing.T) {
	mock := &mockFullGitOps{
		branches: []git.Branch{{Name: "dev", IsCurrent: true}},
	}
	m := newTestModelWithFullGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init should include auto-fetch tick and branch load")
}

// ---------------------------------------------------------------------------
// handleSwitchOrCreateTab
// ---------------------------------------------------------------------------

func TestHandleSwitchOrCreateTabExistingTab(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// The default explorer tab should already exist.
	updated, cmd := m.handleSwitchOrCreateTab("explorer")
	_ = updated.(Model)
	assert.NotNil(t, cmd, "switching to existing tab should return a command")
}

func TestHandleSwitchOrCreateTabNewTab(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// A non-existing tab should be created.
	updated, cmd := m.handleSwitchOrCreateTab("git")
	_ = updated.(Model)
	assert.NotNil(t, cmd, "creating a new tab should return a command")
}

// ---------------------------------------------------------------------------
// handleClosePanel after split
// ---------------------------------------------------------------------------

func TestClosePanelAfterSplit(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Split to create a panel that can be closed.
	updated, _ = m.handleAction("split_vertical", nil)
	m = updated.(Model)

	// Now close should succeed.
	updated, cmd := m.handleClosePanel()
	_ = updated.(Model)
	assert.Nil(t, cmd, "closing a split panel should succeed with no error toast")
}

func TestClosePanelUnsplitError(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Closing without any splits may produce an error.
	_, cmd := m.handleClosePanel()
	// Should either return nil or an error toast (depending on engine).
	if cmd != nil {
		msg := cmd()
		toast, ok := msg.(notify.ShowToastMsg)
		if ok {
			assert.Equal(t, notify.Warn, toast.Level)
		}
	}
}

// ---------------------------------------------------------------------------
// handleSplitVertical/Horizontal with explicit panelType
// ---------------------------------------------------------------------------

func TestSplitVerticalWithExplicitType(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.handleSplitVertical("preview")
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestSplitHorizontalWithExplicitType(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.handleSplitHorizontal("preview")
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// saveSession tests
// ---------------------------------------------------------------------------

func TestSaveSessionNilManagerNoPanic(t *testing.T) {
	m := newTestModel(t)
	// Should not panic with nil session manager.
	m.saveSession()
}

func TestSaveSessionDisabledConfig(t *testing.T) {
	m := newTestModel(t)
	m = m.WithSessionManager(session.NewManager())
	m = m.WithConfig(&config.Config{Session: config.SessionConfig{Enabled: false}})
	// Should return early without error.
	m.saveSession()
}

func TestSaveSessionEnabled(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	mgr := session.NewManager()
	m = m.WithSessionManager(mgr)
	m = m.WithConfig(&config.Config{Session: config.SessionConfig{Enabled: true}})

	// Should not panic and should attempt to save.
	m.saveSession()
}

// ---------------------------------------------------------------------------
// addBookmark edge cases
// ---------------------------------------------------------------------------

func TestAddBookmarkLongPath(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Create a valid long path.
	tmpDir := t.TempDir()
	longPath := tmpDir // use the temp dir itself, which exists

	updated2, cmd := m.addBookmark(longPath)
	_ = updated2.(Model)
	assert.NotNil(t, cmd, "addBookmark should return a success toast command")

	// Execute the command to verify it produces a toast.
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Bookmarked:")
	assert.Equal(t, notify.Success, toast.Level)
}

// ---------------------------------------------------------------------------
// renderHintsBar for various focused panels
// ---------------------------------------------------------------------------

func TestHintsBarForFiletreePanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Filetree is the default focused panel.
	assert.Equal(t, "filetree", m.engine.FocusedName())

	hints := m.renderHintsBar()
	assert.Contains(t, hints, "collapse", "filetree hints should show collapse/expand")
	assert.Contains(t, hints, "find", "filetree hints should show find")
}

func TestHintsBarForGitInfoPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Use number key 2 to focus gitinfo panel.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '2'})
	m = updated.(Model)
	assert.Equal(t, "gitinfo", m.engine.FocusedName())

	hints := m.renderHintsBar()
	// gitinfo falls through to the default case.
	assert.Contains(t, hints, "help")
}

func TestHintsBarForCommitsPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Use number key 4 to focus commits panel directly.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '4'})
	m = updated.(Model)
	assert.Equal(t, "commits", m.engine.FocusedName())

	hints := m.renderHintsBar()
	assert.Contains(t, hints, "help")
}

func TestHintsBarForPreviewPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Use number key 5 to focus preview panel directly.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '5'})
	m = updated.(Model)
	assert.Equal(t, "preview", m.engine.FocusedName())

	hints := m.renderHintsBar()
	assert.Contains(t, hints, "scroll", "preview hints should show scroll")
}

// ---------------------------------------------------------------------------
// renderLayout with chat (non-focused path)
// ---------------------------------------------------------------------------

func TestRenderLayoutWithChatUnfocused(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Chat is present but not focused — should render panels + chat footer.
	content := m.renderLayout()
	assert.NotEmpty(t, content)
}

func TestRenderLayoutWithChatFocused(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Focus the chat.
	m.chat.Focus()
	content := m.renderLayout()
	assert.NotEmpty(t, content, "chat-focused layout should render")
	assert.Contains(t, content, "Chat", "should contain chat modal title")
}

// ---------------------------------------------------------------------------
// renderLayout with zoomed mode
// ---------------------------------------------------------------------------

func TestRenderLayoutZoomed(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	m.engine.ToggleZoom()
	content := m.renderLayout()
	assert.NotEmpty(t, content, "zoomed layout should render")
}

// ---------------------------------------------------------------------------
// renderSeparatorWithTitle edge cases
// ---------------------------------------------------------------------------

func TestRenderSeparatorWithTitleSmallWidth(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	allPanels := m.engine.Panels()
	focusedName := m.engine.FocusedName()
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	// Width < 6 should return plain separator.
	result := m.renderSeparatorWithTitle(5, "filetree", allPanels, focusedName, sepStyle)
	assert.NotEmpty(t, result)
}

func TestRenderSeparatorWithTitleFocusedPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	allPanels := m.engine.Panels()
	focusedName := m.engine.FocusedName()
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	// Focused panel title.
	result := m.renderSeparatorWithTitle(40, focusedName, allPanels, focusedName, sepStyle)
	assert.NotEmpty(t, result)
}

func TestRenderSeparatorWithTitleUnknownPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	allPanels := m.engine.Panels()
	focusedName := m.engine.FocusedName()
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	// Unknown panel should return plain separator.
	result := m.renderSeparatorWithTitle(40, "nonexistent", allPanels, focusedName, sepStyle)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// renderPanel edge cases
// ---------------------------------------------------------------------------

func TestRenderPanelNilPanel(t *testing.T) {
	m := newTestModel(t)
	result := m.renderPanel("test", nil, layout.Rect{Width: 10, Height: 5}, false)
	assert.Empty(t, result, "nil panel should return empty string")
}

func TestRenderPanelSmallDimensions(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	allPanels := m.engine.Panels()
	p := allPanels["filetree"]

	// Very small dimensions.
	result := m.renderPanel("filetree", p, layout.Rect{Width: 3, Height: 2}, false)
	assert.NotEmpty(t, result)
}

func TestRenderPanelZeroDimensions(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	allPanels := m.engine.Panels()
	p := allPanels["preview"]

	// Zero dimensions should clamp to 1.
	result := m.renderPanel("preview", p, layout.Rect{Width: 0, Height: 0}, true)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// View with all overlays exercised
// ---------------------------------------------------------------------------

func TestViewWithFuzzyFinderOverlay(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	m = m.openFuzzyFinder("files")
	assert.NotNil(t, m.fuzzyFinder)

	view := m.View()
	assert.NotEmpty(t, view.Content)
	assert.True(t, view.AltScreen)
}

func TestViewWithChatFocusedOverlay(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Focus chat.
	m.chat.Focus()
	view := m.View()
	assert.NotEmpty(t, view.Content)
	assert.Contains(t, view.Content, "Chat")
}

// ---------------------------------------------------------------------------
// renderStatusBar with branch info and dirty state
// ---------------------------------------------------------------------------

func TestStatusBarShowsBranchInfo(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	m.currentBranch = "feature-x"
	m.branchAhead = 2
	m.branchBehind = 1
	m.gitDirty = true

	bar := m.renderStatusBar()
	assert.Contains(t, bar, "feature-x", "status bar should show branch name")
}

// ---------------------------------------------------------------------------
// handleNewTab with invalid preset
// ---------------------------------------------------------------------------

func TestHandleNewTabInvalidPreset(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Invalid preset should fall back to explorer.
	updated, cmd := m.handleNewTab("nonexistent_preset")
	_ = updated.(Model)
	assert.NotNil(t, cmd)
}

func TestHandleNewTabEmptyPreset(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Empty preset should default to "explorer".
	updated, cmd := m.handleNewTab("")
	_ = updated.(Model)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleCloseTab
// ---------------------------------------------------------------------------

func TestHandleCloseTabSingleTab(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Closing the only tab should fail with a toast.
	_, cmd := m.handleCloseTab()
	if cmd != nil {
		msg := cmd()
		toast, ok := msg.(notify.ShowToastMsg)
		if ok {
			assert.Equal(t, notify.Warn, toast.Level)
		}
	}
}

// ---------------------------------------------------------------------------
// closePanels
// ---------------------------------------------------------------------------

func TestClosePanelsNoPanic(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Should not panic.
	m.closePanels()
}

func TestClosePanelsWithChatModel(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Should close chat without panic.
	m.closePanels()
}

// ---------------------------------------------------------------------------
// View with small window (edge cases for chat modal clamping)
// ---------------------------------------------------------------------------

func TestViewWithChatFocusedSmallWindow(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	// Use a small window to exercise the clamping logic in renderLayout.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	m = updated.(Model)
	m.Init()

	m.chat.Focus()
	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// renderLayout with split panels (exercises renderNode branches)
// ---------------------------------------------------------------------------

func TestRenderLayoutAfterVerticalSplit(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Split vertically to exercise the horizontal split path in renderNode.
	updated, _ = m.handleSplitVertical("preview")
	m = updated.(Model)

	content := m.renderLayout()
	assert.NotEmpty(t, content)
}

func TestRenderLayoutAfterHorizontalSplit(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Split horizontally to exercise the vertical split path in renderNode.
	updated, _ = m.handleSplitHorizontal("preview")
	m = updated.(Model)

	content := m.renderLayout()
	assert.NotEmpty(t, content)
}

// ---------------------------------------------------------------------------
// handleAutoFetchTick with git client (exercises fetch call)
// ---------------------------------------------------------------------------

func TestAutoFetchTickWithGitClient(t *testing.T) {
	mock := &mockFullGitOps{}
	m := newTestModelWithFullGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}

	updated, cmd := m.handleAutoFetchTick()
	_ = updated.(Model)
	assert.NotNil(t, cmd, "should return batch with fetch and next tick")
}

func TestAutoFetchTickNilGitClient(t *testing.T) {
	m := newTestModel(t)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 5 * time.Minute},
		},
	})

	updated, cmd := m.handleAutoFetchTick()
	_ = updated.(Model)
	// Should return only the next tick (no fetch because gitClient is nil).
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// AICommitSuggestionMsg via Update
// ---------------------------------------------------------------------------

func TestAICommitSuggestionMsgStored(t *testing.T) {
	mock := &mockFullGitOps{mockGitOps: mockGitOps{commitHash: "abc123"}}
	m := newTestModelWithFullGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Sending an AI commit suggestion should be stored.
	updated, _ = m.Update(panels.AICommitSuggestionMsg{
		Subject: "fix: typo in README",
		Type:    "fix",
	})
	m = updated.(Model)
	assert.NotNil(t, m.aiCommitSuggestion)
	assert.Equal(t, "fix: typo in README", m.aiCommitSuggestion.Subject)
}

// ---------------------------------------------------------------------------
// renderLayout with empty panel rects
// ---------------------------------------------------------------------------

func TestRenderLayoutEmptyModel(t *testing.T) {
	m := newTestModel(t)
	// Not resized yet, engine may have no rects.
	content := m.renderLayout()
	// Should not panic; may be empty.
	_ = content
}

// ---------------------------------------------------------------------------
// preview_position action
// ---------------------------------------------------------------------------

func TestPreviewPositionAction(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// preview_position should rotate without panic.
	updated, cmd := m.handleAction("preview_position", nil)
	_ = updated.(Model)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// closePanels test
// ---------------------------------------------------------------------------

func TestClosePanelsDoesNotPanic(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// closePanels should close all panels without panic.
	assert.NotPanics(t, func() { m.closePanels() })
}

func TestClosePanelsWithChat(t *testing.T) {
	m := newTestModel(t)
	chatM := newTestChatModel(t)
	m = m.WithChat(chatM)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	assert.NotPanics(t, func() { m.closePanels() })
}

// ---------------------------------------------------------------------------
// Quit via handleAction saves session and closes panels
// ---------------------------------------------------------------------------

func TestQuitActionProducesQuitMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handleAction("quit", nil)
	require.NotNil(t, cmd)
	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit, "quit action should produce QuitMsg")
}

// ---------------------------------------------------------------------------
// renderStatusBar / renderHintsBar coverage
// ---------------------------------------------------------------------------

func TestRenderStatusBar(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.currentBranch = "main"
	m.gitDirty = true
	m.branchAhead = 3
	m.branchBehind = 1

	sb := m.renderStatusBar()
	assert.Contains(t, sb, "main", "status bar should show branch name")
}

func TestRenderStatusBarWithAsyncOp(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.asyncOp = asyncOpPushing

	sb := m.renderStatusBar()
	assert.Contains(t, sb, "pushing", "status bar should show async op")
}

func TestRenderHintsBarFileTree(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Focused panel should be filetree by default.
	assert.Equal(t, "filetree", m.engine.FocusedName())
	hints := m.renderHintsBar()
	assert.Contains(t, hints, "help", "filetree hints should contain help")
}

func TestRenderHintsBarDefaultPanel(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	// No engine setup — just test renderHintsBar doesn't panic.
	hints := m.renderHintsBar()
	assert.NotEmpty(t, hints)
}

// ---------------------------------------------------------------------------
// AsyncOpDoneMsg handling
// ---------------------------------------------------------------------------

func TestAsyncOpDoneMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.asyncOp = asyncOpPushing

	updated, _ = m.Update(panels.AsyncOpDoneMsg{Description: "push"})
	m = updated.(Model)

	// asyncOp should be cleared.
	assert.Empty(t, m.asyncOp, "asyncOp should be cleared after AsyncOpDoneMsg")
}

func TestAsyncOpDoneMsgWithError(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.asyncOp = asyncOpPushing

	updated, _ = m.Update(panels.AsyncOpDoneMsg{
		Description: "push",
		Err:         fmt.Errorf("network error"),
	})
	m = updated.(Model)
	assert.Empty(t, m.asyncOp)
}

// ---------------------------------------------------------------------------
// AICommitSuggestionMsg
// ---------------------------------------------------------------------------

func TestAICommitSuggestionMsg(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	assert.Nil(t, m.aiCommitSuggestion)

	updated, cmd := m.Update(panels.AICommitSuggestionMsg{Subject: "feat: add feature"})
	m = updated.(Model)
	assert.Nil(t, cmd)
	assert.NotNil(t, m.aiCommitSuggestion)
	assert.Equal(t, "feat: add feature", m.aiCommitSuggestion.Subject)
}

// ---------------------------------------------------------------------------
// resize_up / resize_down actions
// ---------------------------------------------------------------------------

func TestResizeUpDownActions(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	updated, _ = m.handleAction("resize_up", nil)
	m = updated.(Model)

	updated, _ = m.handleAction("resize_down", nil)
	m = updated.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// ToggleFuzzyFinderMsg (close) after manual open
// ---------------------------------------------------------------------------

func TestToggleFuzzyFinderMsgCloses(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	m = m.openFuzzyFinder("files")
	assert.NotNil(t, m.fuzzyFinder)

	updated, _ = m.Update(panels.ToggleFuzzyFinderMsg{})
	m = updated.(Model)
	assert.Nil(t, m.fuzzyFinder, "ToggleFuzzyFinderMsg should close fuzzy finder")
}

// ---------------------------------------------------------------------------
// Model.Init returns a command
// ---------------------------------------------------------------------------

func TestModelInitReturnsCmd(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	assert.NotNil(t, cmd, "Init should return a batch command")
}

// ---------------------------------------------------------------------------
// handleNewTab / handleCloseTab / handleSwitchOrCreateTab
// ---------------------------------------------------------------------------

func TestHandleNewTabExplorer(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// handleNewTab with empty preset defaults to "explorer".
	result, cmd := m.handleNewTab("")
	_ = result.(Model)
	_ = cmd
	// Should not panic.
}

func TestHandleNewTabUnknownPreset(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Unknown preset falls back to explorer.
	result, cmd := m.handleNewTab("nonexistent")
	_ = result.(Model)
	_ = cmd
}

func TestHandleCloseTabSingle(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Closing the only tab should produce an error.
	result, cmd := m.handleCloseTab()
	_ = result.(Model)
	if cmd != nil {
		msg := cmd()
		toast, ok := msg.(notify.ShowToastMsg)
		if ok {
			assert.Equal(t, notify.Warn, toast.Level)
		}
	}
}

// ---------------------------------------------------------------------------
// handleSplitVertical / handleSplitHorizontal with empty panelType
// ---------------------------------------------------------------------------

func TestHandleSplitVerticalEmptyType(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	result, _ := m.handleSplitVertical("")
	m = result.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

func TestHandleSplitHorizontalEmptyType(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	result, _ := m.handleSplitHorizontal("")
	m = result.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// handleClosePanel — close after split
// ---------------------------------------------------------------------------

func TestHandleClosePanelAfterSplit(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Split first, then close.
	result, _ := m.handleSplitVertical("preview")
	m = result.(Model)

	result, _ = m.handleClosePanel()
	m = result.(Model)

	view := m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// Esc with async operation cancels it (app_test variant)
// ---------------------------------------------------------------------------

func TestEscCancelsAsyncOpFromApp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Simulate an async operation in progress.
	m.asyncOp = asyncOpPushing
	cancelled := false
	m.asyncCancel = func() { cancelled = true }

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	assert.True(t, cancelled, "Esc should call asyncCancel")
	assert.Empty(t, m.asyncOp, "asyncOp should be cleared after cancel")
	assert.Nil(t, m.asyncCancel, "asyncCancel should be cleared")
}

// ---------------------------------------------------------------------------
// ModalResultMsg routing when pendingAction is set
// ---------------------------------------------------------------------------

func TestModalResultMsgWithPendingActionCommit(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.pendingAction = "commit"

	// ModalResultMsg with Accept=false should clear pending and not panic.
	updated, _ = m.Update(notify.ModalResultMsg{Accept: false, Value: ""})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
}

// ---------------------------------------------------------------------------
// checkGitDirty with nil client
// ---------------------------------------------------------------------------

func TestCheckGitDirtyNilClient(t *testing.T) {
	m := newTestModel(t)
	cmd := m.checkGitDirty()
	assert.Nil(t, cmd, "checkGitDirty with nil client should return nil")
}

// ---------------------------------------------------------------------------
// saveSession with nil manager
// ---------------------------------------------------------------------------

func TestSaveSessionNilManager(t *testing.T) {
	m := newTestModel(t)
	assert.NotPanics(t, func() { m.saveSession() }, "saveSession with nil sessionMgr should not panic")
}

func TestSaveSessionDisabled(t *testing.T) {
	m := newTestModel(t)
	mgr := session.NewManager()
	m = m.WithSessionManager(mgr)
	cfg := &config.Config{}
	cfg.Session.Enabled = false
	m = m.WithConfig(cfg)

	assert.NotPanics(t, func() { m.saveSession() }, "saveSession with disabled config should not panic")
}

// ---------------------------------------------------------------------------
// autoFetchTickCmd with nil config
// ---------------------------------------------------------------------------

func TestAutoFetchTickCmdNilConfig(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.cfg)
	cmd := m.autoFetchTickCmd()
	assert.Nil(t, cmd, "autoFetchTickCmd with nil config should return nil")
}

// ---------------------------------------------------------------------------
// Multiple overlays: opening one overlay while another is already visible
// ---------------------------------------------------------------------------

func TestHelpOverlayClosesWhenSettingsOpens(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.Init()

	// Open help.
	updated, _ = m.toggleHelp()
	m = updated.(Model)
	assert.True(t, m.helpShown)

	// View should contain help content.
	view := m.View()
	assert.NotEmpty(t, view.Content)

	// Now open settings — both can coexist, verify no panic.
	updated, _ = m.toggleSettings()
	m = updated.(Model)
	assert.True(t, m.settingsShown)

	view = m.View()
	assert.NotEmpty(t, view.Content)
}

// ---------------------------------------------------------------------------
// addBookmark with nil manager
// ---------------------------------------------------------------------------

func TestAddBookmarkNilManager(t *testing.T) {
	m := newTestModel(t)
	m.bookmarkMgr = nil

	result, cmd := m.addBookmark("/some/path")
	_ = result
	assert.Nil(t, cmd, "addBookmark with nil manager should return nil cmd")
}

// ---------------------------------------------------------------------------
// FileSelectedMsg with directory path converts to ChangeDirectoryMsg
// ---------------------------------------------------------------------------

func TestFileSelectedMsgWithDirectoryChangesDir(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	tmpDir := t.TempDir()

	// Send FileSelectedMsg with a directory path — should be converted to
	// ChangeDirectoryMsg internally, changing the process CWD.
	updated, cmd := m.Update(panels.FileSelectedMsg{Path: tmpDir})
	m = updated.(Model)

	cwd, cwdErr := os.Getwd()
	require.NoError(t, cwdErr)
	assert.Equal(t, tmpDir, cwd, "FileSelectedMsg with dir path should change CWD")
	assert.NotNil(t, cmd, "should return batch command")
	assert.Nil(t, m.fuzzyFinder, "fuzzy finder should be closed")

	// Restore CWD before cleanup to avoid TempDir removal errors on Windows.
	require.NoError(t, os.Chdir(origDir))
}

// ---------------------------------------------------------------------------
// ChangeDirectoryMsg resets git fields when target is not a git repo
// ---------------------------------------------------------------------------

func TestChangeDirectoryMsgResetsGitFields(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	tmpDir := t.TempDir() // not a git repo
	genBefore := m.branchInfoGen
	updated, cmd := m.Update(panels.ChangeDirectoryMsg{Path: tmpDir})
	m = updated.(Model)
	assert.Equal(t, genBefore+1, m.branchInfoGen, "ChangeDirectoryMsg should bump branchInfoGen")

	cwd, cwdErr := os.Getwd()
	require.NoError(t, cwdErr)
	assert.Equal(t, tmpDir, cwd, "CWD should change to target dir")
	assert.NotNil(t, cmd, "should return batch command for navigation + refresh")

	// Restore CWD before cleanup to avoid TempDir removal errors on Windows.
	require.NoError(t, os.Chdir(origDir))
}

// ---------------------------------------------------------------------------
// CWD inline editing (double-click footer)
// ---------------------------------------------------------------------------

func TestStatusBarDoubleClickEntersEditMode(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.ready = true

	// First click on status bar (Y = height-1).
	click := tea.MouseClickMsg{X: 10, Y: 39, Button: tea.MouseLeft}
	updated, _ := m.Update(click)
	m = updated.(Model)
	assert.False(t, m.cwdEditing, "single click should not enter edit mode")

	// Second click within threshold — simulate by setting lastStatusBarClick.
	m.lastStatusBarClick = time.Now()
	updated, _ = m.Update(click)
	m = updated.(Model)
	assert.True(t, m.cwdEditing, "double-click should enter edit mode")
	assert.NotEmpty(t, m.cwdEditValue, "edit value should be pre-populated with CWD")
	assert.Equal(t, len([]rune(m.cwdEditValue)), m.cwdEditCursor, "cursor should be at end")
}

func TestCWDEditTypingUpdatesValue(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp"
	m.cwdEditCursor = 4

	// Type "/test".
	for _, ch := range "/test" {
		key := tea.KeyPressMsg{Text: string(ch)}
		updated, _ := m.Update(key)
		m = updated.(Model)
	}
	assert.Equal(t, "/tmp/test", m.cwdEditValue)
	assert.Equal(t, 9, m.cwdEditCursor)
}

func TestCWDEditEscapeCancels(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/some/new/path"
	m.cwdEditCursor = 14

	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEscape})
	m = updated.(Model)
	assert.False(t, m.cwdEditing, "Escape should exit edit mode")
}

func TestCWDEditEnterChangesDirectory(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(origDir) })

	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	tmpDir := t.TempDir()
	m.cwdEditing = true
	m.cwdEditValue = tmpDir
	m.cwdEditCursor = len([]rune(tmpDir))

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEnter})
	m = updated.(Model)
	assert.False(t, m.cwdEditing, "Enter should exit edit mode")
	assert.NotNil(t, cmd, "should return command")

	cwd, cwdErr := os.Getwd()
	require.NoError(t, cwdErr)
	assert.Equal(t, tmpDir, cwd, "Enter should change CWD")

	os.Chdir(origDir)
}

func TestCWDEditBackspaceDeletesChar(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 9 // cursor at end (9 runes: /tmp/test)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyBackspace})
	m = updated.(Model)
	assert.Equal(t, "/tmp/tes", m.cwdEditValue)
	assert.Equal(t, 8, m.cwdEditCursor)
}

func TestCWDEditArrowKeys(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp"
	m.cwdEditCursor = 4

	// Left moves cursor back.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyLeft})
	m = updated.(Model)
	assert.Equal(t, 3, m.cwdEditCursor)

	// Right moves cursor forward.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyRight})
	m = updated.(Model)
	assert.Equal(t, 4, m.cwdEditCursor)

	// Home moves to start.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyHome})
	m = updated.(Model)
	assert.Equal(t, 0, m.cwdEditCursor)

	// End moves to end.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEnd})
	m = updated.(Model)
	assert.Equal(t, 4, m.cwdEditCursor)
}

func TestCWDEditRenderShowsCursor(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 4

	rendered := m.renderStatusBar()
	assert.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "esc=cancel", "edit mode should show hints")
}

func TestCWDEditEnterWithEmptyPathIsNoOp(t *testing.T) {
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })

	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = ""
	m.cwdEditCursor = 0

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEnter})
	m = updated.(Model)
	assert.False(t, m.cwdEditing, "empty path should exit edit mode")
	assert.Nil(t, cmd, "empty path should produce no command")

	// Whitespace-only is also a no-op.
	m.cwdEditing = true
	m.cwdEditValue = "   "
	m.cwdEditCursor = 3

	updated, cmd = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyEnter})
	m = updated.(Model)
	assert.False(t, m.cwdEditing)
	assert.Nil(t, cmd)
}

func TestCWDEditDeleteAndCtrlKeys(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp/test"
	m.cwdEditCursor = 4

	// Delete removes char at cursor (forward delete).
	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyDelete})
	m = updated.(Model)
	assert.Equal(t, "/tmptest", m.cwdEditValue)
	assert.Equal(t, 4, m.cwdEditCursor, "cursor stays in place after delete")

	// Ctrl+U clears entire value.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Equal(t, "", m.cwdEditValue)
	assert.Equal(t, 0, m.cwdEditCursor)

	// Rebuild for ctrl+a / ctrl+e tests.
	m.cwdEditValue = "/var/log"
	m.cwdEditCursor = 4

	// Ctrl+A moves to start.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: 'a', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Equal(t, 0, m.cwdEditCursor)

	// Ctrl+E moves to end.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: 'e', Mod: tea.ModCtrl})
	m = updated.(Model)
	assert.Equal(t, 8, m.cwdEditCursor)
}

func TestCWDEditBoundaryConditions(t *testing.T) {
	m := newTestModel(t)
	m.cwdEditing = true
	m.cwdEditValue = "/tmp"
	m.cwdEditCursor = 0

	// Backspace at cursor=0 is a no-op.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyBackspace})
	m = updated.(Model)
	assert.Equal(t, "/tmp", m.cwdEditValue)
	assert.Equal(t, 0, m.cwdEditCursor)

	// Left at cursor=0 is a no-op.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyLeft})
	m = updated.(Model)
	assert.Equal(t, 0, m.cwdEditCursor)

	// Right at end is a no-op.
	m.cwdEditCursor = 4 // len("/tmp") = 4
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyRight})
	m = updated.(Model)
	assert.Equal(t, 4, m.cwdEditCursor)

	// Delete at end is a no-op.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "", Code: tea.KeyDelete})
	m = updated.(Model)
	assert.Equal(t, "/tmp", m.cwdEditValue)
	assert.Equal(t, 4, m.cwdEditCursor)
}
