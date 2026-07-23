// Package tui implements the terminal user interface for grut
// using Bubble Tea v2, Lip Gloss v2, and Bubbles v2.
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/chat"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/crashlog"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/session"
	"github.com/jongio/grut/internal/theme"
)

// Modal / overlay sizing constants.
// These govern the chat overlay and bookmark/help/settings dialogs.
const (
	chatModalWidthPct  = 70 // percentage of terminal width used for the chat dialog
	chatModalHeightPct = 60 // percentage of panel-area height used for the chat dialog
	chatModalMinWidth  = 40 // minimum character width for the chat modal
	chatModalMinHeight = 10 // minimum row height for the chat modal
	chatInnerMinWidth  = 10 // minimum inner width after subtracting borders
	chatInnerMinHeight = 5  // minimum inner height after subtracting borders
)

// Model is the top-level TUI model for grut. It composes panels via
// the layout engine, routes keyboard events, and renders the final view.
type Model struct {
	lastStatusBarClick time.Time     // for double-click detection on the status bar
	gitClient          git.GitClient // git client for app-level operations (nil = no git)
	ctx                context.Context
	engine             layout.PanelManager
	theme              *theme.Theme
	keys               *keymap.Keymap
	nav                navHistory
	notify             *notify.Manager               // F27: integrated notification manager
	bookmarkMgr        *bm.Manager                   // bookmark persistence
	overlays           OverlayCreator                // factory for overlay panels
	bookmarkPanel      panels.Panel                  // overlay panel (nil = hidden)
	fuzzyFinder        panels.Panel                  // overlay fuzzy finder (nil = hidden)
	commandLogPanel    panels.Panel                  // overlay git command log panel (nil = hidden)
	commandLogShown    bool                          // whether git command log overlay is visible
	helpPanel          panels.Panel                  // overlay help panel (nil = hidden)
	helpShown          bool                          // whether help overlay is visible
	welcomePanel       panels.Panel                  // overlay welcome panel (nil = hidden)
	welcomeShown       bool                          // whether welcome overlay is visible
	settingsPanel      panels.Panel                  // overlay settings panel (nil = hidden)
	undoMgr            *git.UndoManager              // undo/redo manager (nil = disabled)
	cfg                *config.Config                // app config (nil = defaults)
	sessionMgr         *session.Manager              // session persistence (nil = disabled)
	chat               *chat.Model                   // AI chat footer (nil if AI disabled)
	asyncCancel        context.CancelFunc            // cancel the running async operation
	aiCommitSuggestion *panels.AICommitSuggestionMsg // AI-generated commit message suggestion
	cancel             context.CancelFunc
	asyncOp            string // current async operation label ("pushing...", etc.)
	pendingAction      string // action waiting for modal input ("commit")
	pendingDiscardPath string // file path for pending discard confirmation
	pendingRevertHash  string // full commit hash for pending revert confirmation
	currentBranch      string // cached git branch name for status bar
	compareBase        string // pinned branch-diff compare base for status bar
	cwdEditValue       string // editable path text
	initialFile        string // file to open + focus in preview at startup (empty = none)
	initialFocusPanel  string // panel to focus after startup file reveal (empty = default)
	branchAhead        int    // commits ahead of upstream (needs push)
	branchBehind       int    // commits behind upstream (needs pull)
	width              int
	height             int
	initialLine        int    // 1-based line to scroll the preview to at startup (0 = none)
	cwdEditCursor      int    // cursor position (rune index) within cwdEditValue
	branchInfoGen      uint64 // generation counter - invalidates stale branchLoadedMsg
	bookmarksShown     bool   // whether bookmark overlay is visible
	settingsShown      bool   // whether settings overlay is visible
	gitDirty           bool   // true when working tree has uncommitted changes
	ready              bool   // true after first WindowSizeMsg
	cwdEditing         bool   // true when status bar CWD is in inline-edit mode
	previewEditing     bool   // true when preview panel is in edit mode
	previewInput       bool   // true when preview panel has an inline prompt open (e.g. go-to-line)
}

// New creates a new TUI model with the given panel manager, theme, keymap,
// and bookmark manager. The panel manager is typically a *layout.Engine but
// can be any implementation of layout.PanelManager.
func New(engine layout.PanelManager, th *theme.Theme, km *keymap.Keymap, bmMgr *bm.Manager, overlays OverlayCreator) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		engine:      engine,
		theme:       th,
		keys:        km,
		notify:      notify.NewManager(),
		bookmarkMgr: bmMgr,
		overlays:    overlays,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// WithUndoManager returns a copy of the model with the given UndoManager
// configured. When set, ctrl+z and ctrl+y trigger undo/redo operations.
func (m Model) WithUndoManager(um *git.UndoManager) Model {
	m.undoMgr = um
	return m
}

// WithGitClient returns a copy of the model with the given git client
// configured for app-level operations (commit, push, pull, fetch).
func (m Model) WithGitClient(gc git.GitClient) Model {
	m.gitClient = gc
	return m
}

// WithConfig returns a copy of the model with the given configuration.
// Used for auto-fetch interval and sign-commits settings.
func (m Model) WithConfig(cfg *config.Config) Model {
	m.cfg = cfg
	return m
}

// WithSessionManager returns a copy of the model with the given session
// manager configured. When set, the current session state is saved to
// disk on quit and restored on the next launch.
func (m Model) WithSessionManager(mgr *session.Manager) Model {
	m.sessionMgr = mgr
	return m
}

// WithChat returns a copy of the model with the given chat model attached
// as a footer. When non-nil, the chat footer is rendered below the panel
// area and can receive keyboard focus via ctrl+space.
func (m Model) WithChat(c *chat.Model) Model {
	m.chat = c
	return m
}

// WithInitialFile returns a copy of the model configured to open the given
// file in the preview panel at startup, focusing the preview and scrolling
// to the given 1-based line (0 = top). Used when grut is launched with a
// file path argument, e.g. "grut main.go:42". An empty path is a no-op.
func (m Model) WithInitialFile(path string, line int) Model {
	m.initialFile = path
	m.initialLine = line
	return m
}

// WithInitialFocusPanel returns a copy of the model configured to focus the
// named panel after initial startup messages are queued.
func (m Model) WithInitialFocusPanel(name string) Model {
	m.initialFocusPanel = name
	return m
}

// branchLoadedMsg carries the initial branch name and tracking info for the status bar.
type branchLoadedMsg struct {
	Name       string
	Ahead      int
	Behind     int
	generation uint64 // matches branchInfoGen at the time loadBranchInfo was called
}

// gitDirtyMsg reports whether the working tree has uncommitted changes.
type gitDirtyMsg struct{ dirty bool }

// Init implements tea.Model. Initializes all panels and starts the
// auto-fetch timer if configured.
func (m Model) Init() tea.Cmd {
	defer crashlog.GuardTUI("tui.Init")
	cmds := []tea.Cmd{m.engine.Init(m.ctx)}
	// Open a file passed on the command line (e.g. "grut main.go:42"):
	// focus the preview panel and ask the filetree to reveal + select the
	// file, carrying the optional line so the preview scrolls to it once the
	// content loads. engine.Init has already focused the default panel above,
	// so this FocusByName call takes precedence for the initial view.
	if m.initialFile != "" {
		m.engine.FocusByName("preview")
		file, line := m.initialFile, m.initialLine
		cmds = append(cmds, func() tea.Msg {
			return panels.RevealFileMsg{Path: file, Line: line}
		})
	}
	if m.initialFocusPanel != "" {
		m.engine.FocusByName(m.initialFocusPanel)
	}
	if tick := m.autoFetchTickCmd(); tick != nil {
		cmds = append(cmds, tick)
	}
	// Check for first run — show help overlay automatically.
	if m.cfg == nil || m.cfg.General.ShowFirstRunHelp {
		if session.IsFirstRun() {
			cmds = append(cmds, func() tea.Msg { return panels.FirstRunMsg{} })
		}
	}
	// Load current branch name for status bar.
	if cmd := m.loadBranchInfo(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Initialize the chat footer if present.
	if m.chat != nil {
		if cmd := m.chat.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// checkGitDirty returns a command that checks if the git working tree is dirty.
func (m Model) checkGitDirty() tea.Cmd {
	gc := m.gitClient
	if gc == nil {
		return nil
	}
	ctx := m.ctx
	return func() tea.Msg {
		files, err := gc.Status(ctx)
		if err != nil {
			return gitDirtyMsg{dirty: false}
		}
		return gitDirtyMsg{dirty: len(files) > 0}
	}
}

// loadBranchInfo returns a command that reloads the current branch's
// ahead/behind tracking counts for the status bar.
func (m Model) loadBranchInfo() tea.Cmd {
	gc := m.gitClient
	if gc == nil {
		return nil
	}
	ctx := m.ctx
	generation := m.branchInfoGen
	return func() tea.Msg {
		b, err := gc.CurrentBranch(ctx)
		if err != nil {
			return branchLoadedMsg{generation: generation}
		}
		return branchLoadedMsg{Name: b.Name, Ahead: b.Ahead, Behind: b.Behind, generation: generation}
	}
}

// Update implements tea.Model. Routes messages to the layout engine
// and handles global key bindings.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer crashlog.GuardTUI("tui.Update")
	if entry, ok := navEntryFromMsg(msg); ok {
		m.nav.record(entry)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg), nil

	// Branch / git status.
	case branchLoadedMsg, gitDirtyMsg, panels.BranchChangedMsg:
		return m.handleBranchMsg(msg)
	case panels.SetCompareBaseMsg, panels.ClearCompareBaseMsg:
		return m.handleCompareBaseMsg(msg)

	// Notifications.
	case notify.ShowToastMsg, notify.ShowModalMsg,
		notify.ToastExpiredMsg, notify.ModalResultMsg:
		return m.handleNotifyMsg(msg)

	// Git operations.
	case panels.CommitRequestMsg, panels.AmendRequestMsg, panels.RewordRequestMsg,
		panels.AICommitSuggestionMsg, panels.PushRequestMsg, panels.PullRequestMsg,
		panels.FetchRequestMsg, panels.AsyncOpDoneMsg, discardFileDoneMsg,
		unstageFileDoneMsg, panels.AutoFetchTickMsg, panels.RevertRequestMsg,
		revertDoneMsg:
		return m.handleGitOpMsg(msg)
	case panels.ToggleBlameMsg:
		return m.handleToggleBlame(msg)
	case panels.ShowCommitDetailMsg:
		m.engine.FocusByName(panelCommits)
		return m, m.engine.Update(msg)

	// Undo / redo.
	case panels.UndoMsg, panels.RedoMsg, panels.UndoResultMsg:
		return m.handleUndoRedoMsg(msg)

	// Bookmarks & navigation.
	case panels.ToggleBookmarksMsg, panels.NavigateToPathMsg,
		panels.ChangeDirectoryMsg, panels.BookmarkAddMsg:
		return m.handleBookmarkNavMsg(msg)

	// Overlays & settings.
	case panels.ToggleHelpMsg, panels.ToggleCommandLogMsg, panels.FirstRunMsg,
		panels.WelcomeAnimTickMsg, panels.WelcomeDismissMsg,
		panels.ToggleSettingsMsg, panels.SetPreviewPositionMsg,
		panels.SetThemeMsg, panels.SetDoubleClickActionMsg,
		panels.SetRightClickActionMsg, panels.ResetActionPromptsMsg:
		return m.handleOverlayMsg(msg)

	// Fuzzy finder.
	case panels.ToggleFuzzyFinderMsg, panels.CommandSelectedMsg, panels.FileSelectedMsg:
		return m.handleFuzzyFinderMsg(msg)

	// Chat.
	case panels.ChatFocusMsg, panels.ChatRefreshMsg, panels.ChatNavigateMsg:
		return m.handleChatMsg(msg)
	case panels.AIExplainMsg:
		return m.handleAIExplainMsg(msg)
	case chat.StreamChunkMsg, chat.ToolCallMsg, chat.ToolResultMsg,
		chat.SendMessageCmd:
		if m.chat != nil {
			return m.handleChatStreamMsg(msg)
		}

	// Tab management (v1: disabled).
	case panels.NewTabMsg, panels.CloseTabMsg, panels.NextTabMsg,
		panels.PrevTabMsg, panels.SwitchTabMsg:
		return m, nil

	// Panel layout.
	case panels.SplitVerticalMsg, panels.SplitHorizontalMsg,
		panels.ClosePanelMsg, panels.GitStatusChangedMsg:
		return m.handlePanelLayoutMsg(msg)

	// Edit mode tracking — the preview panel broadcasts these when entering
	// or leaving inline edit mode so we can skip global key bindings.
	case panels.EditModeEnteredMsg:
		m.previewEditing = true
		return m.handleDefaultMsg(msg)
	case panels.EditModeExitedMsg:
		m.previewEditing = false
		return m.handleDefaultMsg(msg)

	// Inline preview prompt tracking — the preview panel broadcasts these
	// when it opens or closes an inline text prompt (e.g. go-to-line) so we
	// route all keys to it and skip global bindings while it is open.
	case panels.PreviewInputStartedMsg:
		m.previewInput = true
		return m, nil
	case panels.PreviewInputEndedMsg:
		m.previewInput = false
		return m, nil

	// Mouse events — may fall through to default broadcast.
	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
		if mdl, cmd, handled := m.handleMouseMsg(msg); handled {
			return mdl, cmd
		}

	// Keyboard input.
	case tea.KeyPressMsg:
		return m.handleKeyPressMsg(msg)
	}

	// Default: broadcast to all panels and chat.
	return m.handleDefaultMsg(msg)
}

// handleAction maps a dispatched action string to concrete model operations.
// Global actions (quit, focus, zoom, resize) are handled directly.
// Panel-level actions are forwarded to the focused panel.
func (m Model) handleAction(action string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch action {
	case "quit":
		// In edit mode, ctrl+c should be handled by the editor (copy),
		// not quit the application.
		if m.previewEditing {
			cmd := m.engine.Update(msg)
			return m, cmd
		}
		// If the focused panel has an active text selection, Ctrl+C copies
		// the selection instead of quitting (standard OS copy behavior).
		if sc, ok := m.engine.FocusedPanel().(panels.SelectionCopier); ok && sc.HasSelection() {
			_, cmd := sc.CopySelection()
			return m, cmd
		}
		m.saveSession()
		m.closePanels()
		m.cancel()
		return m, tea.Quit
	case "focus_next":
		m.engine.FocusNext()
		return m, nil
	case "focus_prev":
		m.engine.FocusPrev()
		return m, nil
	case "focus_left":
		m.engine.FocusPrev()
		return m, nil
	case "focus_right":
		m.engine.FocusNext()
		return m, nil
	case actionNavBack:
		entry, ok := m.nav.back()
		if !ok {
			return m, nil
		}
		return m.restoreNavigation(entry)
	case actionNavForward:
		entry, ok := m.nav.forward()
		if !ok {
			return m, nil
		}
		return m.restoreNavigation(entry)
	case "zoom_toggle":
		m.engine.ToggleZoom()
		return m, nil
	case "resize_left", "resize_right", "resize_up", "resize_down":
		// In edit mode, ctrl+left/right are word navigation keys — route to panel.
		if m.previewEditing {
			cmd := m.engine.Update(msg)
			return m, cmd
		}
		switch action {
		case "resize_left", "resize_up":
			m.engine.ResizeShrink()
		default:
			m.engine.ResizeGrow()
		}
		return m, nil
	case "exit_input":
		if m.keys != nil {
			m.keys.SetMode(keymap.ModePanel)
		}
		return m, nil
	case "fuzzy_finder":
		return m.openFuzzyFinder("files"), nil
	case "command_palette":
		return m.openFuzzyFinder("commands"), nil
	case "change_directory":
		return m.openFuzzyFinder("directories"), nil
	case "todo_finder":
		return m.openFuzzyFinder("todos"), nil
	case "help":
		return m.toggleHelp()
	case "command_log":
		return m.toggleCommandLog()
	case "welcome":
		return m.toggleWelcome()
	case "settings":
		return m.toggleSettings()
	case "chat_focus":
		return m.toggleChatFocus()
	case pendingActionCommit:
		return m.handleCommit()
	case actionPush:
		return m.handlePush()
	case "pull":
		return m.handlePull()
	case actionFetch:
		return m.handleFetch()
	// Direct panel focus (1-5 number keys).
	case "focus_panel_1":
		m.engine.FocusByName(panelFileTree)
		return m, nil
	case "focus_panel_2":
		m.engine.FocusByName("gitinfo")
		return m, nil
	case "focus_panel_3":
		m.engine.FocusByName(panelGitHub)
		return m, nil
	case "focus_panel_4":
		m.engine.FocusByName(panelCommits)
		return m, nil
	case "focus_panel_5":
		m.engine.FocusByName("preview")
		return m, nil
	// Tab actions — disabled for v1 single-tab mode.
	case "tab_new":
		return m, nil // v1: no-op
	case "tab_close":
		return m, nil // v1: no-op
	case "tab_next":
		return m, nil // v1: no-op
	case "tab_prev":
		return m, nil // v1: no-op
	// Split actions
	case "split_horizontal":
		return m.handleSplitHorizontal("preview")
	case "split_vertical":
		return m.handleSplitVertical("preview")
	case "close_panel":
		return m.handleClosePanel()
	case "preview_position":
		m.engine.RotatePreviewPosition()
		pos := m.engine.CurrentPreviewPosition()
		if err := config.SaveUserSetting("preview.position", pos.String()); err != nil {
			slog.Warn("failed to persist preview position", "err", err)
		}
		return m, nil
	// Preset tab switching — disabled for v1 single-tab mode.
	case "tab_explorer", "tab_git", "tab_review", "tab_agent", "tab_full":
		return m, nil // v1: no-op
	// CRUD item actions — dispatched to all panels; focused panel acts.
	case "item_create":
		cmd := m.engine.Update(panels.ItemCreateMsg{})
		return m, cmd
	case "item_delete":
		cmd := m.engine.Update(panels.ItemDeleteMsg{})
		return m, cmd
	case "item_edit":
		cmd := m.engine.Update(panels.ItemEditMsg{})
		return m, cmd
	case "item_open":
		cmd := m.engine.Update(panels.ItemOpenMsg{})
		return m, cmd
	case "item_copy":
		cmd := m.engine.Update(panels.ItemCopyMsg{})
		return m, cmd
	case "discard_file":
		return m.handleDiscardFile()
	case "unstage_file":
		return m.handleUnstageFile()
	default:
		// Panel-level actions (cursor_down, stage, etc.): route the
		// original key event to the focused panel which handles its
		// own key logic.
		cmd := m.engine.Update(msg)
		return m, cmd
	}
}

// closePanels calls Close() on every panel that implements panels.Closer,
// releasing resources (watchers, goroutines) before shutdown (F29).
func (m Model) closePanels() {
	for _, p := range m.engine.Panels() {
		if closer, ok := p.(panels.Closer); ok {
			closer.Close()
		}
	}
	if m.chat != nil {
		m.chat.Close()
	}
}

// broadcastActionsCfg pushes the current ActionsConfig to every panel that
// implements SetActionsCfg. Called whenever the user changes a double-click,
// right-click, or confirmation setting so panels pick up the change immediately.
func (m Model) broadcastActionsCfg() {
	for _, p := range m.engine.Panels() {
		if ac, ok := p.(interface{ SetActionsCfg(config.ActionsConfig) }); ok {
			ac.SetActionsCfg(m.cfg.Actions)
		}
	}
}

// toggleChatFocus toggles keyboard focus on the chat footer.
// When the chat gains focus, the keymap switches to input mode so that
// letter keys are not intercepted as panel shortcuts. The engine size
// is not recalculated because panels keep their full dimensions — the
// chat modal is overlaid on top without displacing them.
func (m Model) toggleChatFocus() (tea.Model, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	if m.chat.Focused() {
		m.chat.Blur()
		if m.keys != nil {
			m.keys.SetMode(keymap.ModePanel)
		}
	} else {
		m.chat.Focus()
		if m.keys != nil {
			m.keys.SetMode(keymap.ModeInput)
		}
	}
	return m, nil
}

func (m Model) handleAIExplainMsg(msg panels.AIExplainMsg) (tea.Model, tea.Cmd) {
	if m.chat == nil {
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "AI chat is disabled (--no-ai)", Level: notify.Info}
		}
	}
	if !m.chat.Focused() {
		m.chat.Focus()
		if m.keys != nil {
			m.keys.SetMode(keymap.ModeInput)
		}
	}
	return m.handleChatStreamMsg(chat.SendMessageCmd{Content: msg.Content})
}

// handleGlobalRefresh refreshes all data sources and forces preview to re-render.
func (m Model) handleGlobalRefresh() (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 3)
	// Refresh git data in all gitinfo/github panels.
	cmds = append(cmds, m.engine.Update(panels.RefreshBranchesMsg{}))
	cmds = append(cmds, m.engine.Update(panels.RefreshGitStatusMsg{}))
	// Refresh preview — re-renders whatever it is currently showing
	// (file, GitHub issue, PR, CI run, etc.) without switching content.
	cmds = append(cmds, m.engine.Update(panels.RefreshPreviewMsg{}))
	return m, tea.Batch(cmds...)
}

// saveSession persists the current tab layout and focus state to disk.
// Errors are logged but do not prevent shutdown.
func (m Model) saveSession() {
	if m.sessionMgr == nil || m.cfg == nil || !m.cfg.Session.Enabled {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.Warn("session save: cannot determine working directory", "err", err)
		return
	}
	tabs := m.engine.TabManager().Tabs()
	tabStates := make([]session.TabState, len(tabs))
	for i, tab := range tabs {
		tabStates[i] = session.TabState{
			Name:         tab.Name,
			Preset:       tab.Name, // tab name matches preset name
			FocusedPanel: m.engine.FocusedName(),
		}
	}
	state := session.SessionState{
		WorkDir:   cwd,
		ActiveTab: m.engine.TabManager().ActiveIndex(),
		Tabs:      tabStates,
	}
	if err := m.sessionMgr.Save(state); err != nil {
		slog.Warn("session save failed", "err", err)
	}
}

// toggleBookmarks shows or hides the bookmarks overlay panel.
func (m Model) toggleBookmarks() (tea.Model, tea.Cmd) {
	if m.bookmarksShown {
		m.bookmarksShown = false
		m.bookmarkPanel = nil
		return m, nil
	}
	m.bookmarksShown = true
	m.bookmarkPanel = m.overlays.NewBookmarkPanel()
	m.bookmarkPanel.Focus()
	m.bookmarkPanel.SetSize(m.bookmarkOverlayDims())
	m.bookmarkPanel.Init(m.ctx)
	return m, nil
}

// toggleHelp shows or hides the help overlay panel.
func (m Model) toggleHelp() (tea.Model, tea.Cmd) {
	if m.helpShown {
		m.helpShown = false
		m.helpPanel = nil
		return m, nil
	}
	m.helpShown = true
	m.helpPanel = m.overlays.NewHelpPanel()
	m.helpPanel.Focus()
	w, h := m.helpOverlayDims()
	m.helpPanel.SetSize(w, h)
	m.helpPanel.Init(m.ctx)
	// Mark first run as done so the overlay won't auto-show next time.
	if err := session.MarkFirstRunDone(); err != nil {
		slog.Warn("failed to mark first run done", "err", err)
	}
	return m, nil
}

// toggleCommandLog shows or hides the git command log overlay panel.
func (m Model) toggleCommandLog() (tea.Model, tea.Cmd) {
	if m.commandLogShown {
		m.commandLogShown = false
		m.commandLogPanel = nil
		return m, nil
	}
	m.commandLogShown = true
	m.commandLogPanel = m.overlays.NewCommandLogPanel()
	m.commandLogPanel.Focus()
	w, h := m.commandLogOverlayDims()
	m.commandLogPanel.SetSize(w, h)
	m.commandLogPanel.Init(m.ctx)
	return m, nil
}

// overlayDims computes clamped overlay dimensions using percentage-based
// sizing with minimum bounds. It applies the standard pattern:
//
//	raw = terminal * 3 / den
//	clamped to [min, terminal-4, terminal]
func (m Model) overlayDims(wDen, minW, hNum, hDen, minH int) (int, int) {
	w := m.width * 3 / wDen
	if w < minW {
		w = minW
	}
	if w > m.width-4 {
		w = m.width - 4
	}
	if w > m.width {
		w = m.width
	}
	h := m.height * hNum / hDen
	if h < minH {
		h = minH
	}
	if h > m.height-4 {
		h = m.height - 4
	}
	if h > m.height {
		h = m.height
	}
	return w, h
}

// helpOverlayDims returns the content dimensions for the help overlay.
func (m Model) helpOverlayDims() (int, int) {
	return m.overlayDims(5, 40, 3, 4, 10)
}

// commandLogOverlayDims returns the content dimensions for the git command log overlay.
func (m Model) commandLogOverlayDims() (int, int) {
	return m.overlayDims(5, 60, 3, 4, 10)
}

// toggleWelcome shows or hides the welcome overlay panel.
func (m Model) toggleWelcome() (tea.Model, tea.Cmd) {
	if m.welcomeShown {
		m.welcomeShown = false
		m.welcomePanel = nil
		return m, nil
	}

	m.welcomeShown = true
	m.welcomePanel = m.overlays.NewWelcomePanel()
	m.welcomePanel.Focus()
	w, h := m.welcomeOverlayDims()
	m.welcomePanel.SetSize(w, h)
	cmd := m.welcomePanel.Init(m.ctx)

	// Mark first run as done so the overlay won't auto-show next time.
	if err := session.MarkFirstRunDone(); err != nil {
		slog.Warn("failed to mark first run done", "err", err)
	}
	return m, cmd
}

// dismissWelcome handles the welcome panel dismiss message.
func (m Model) dismissWelcome(_ panels.WelcomeDismissMsg) (tea.Model, tea.Cmd) {
	m.welcomeShown = false
	m.welcomePanel = nil

	if err := config.SaveUserSettingBool("general.show_first_run_help", false); err != nil {
		slog.Warn("failed to persist show_first_run_help", "err", err)
	}

	return m, nil
}

// welcomeOverlayDims returns the content dimensions for the welcome overlay.
func (m Model) welcomeOverlayDims() (int, int) {
	return m.overlayDims(5, 44, 4, 5, 20)
}

// toggleSettings shows or hides the settings overlay panel.
func (m Model) toggleSettings() (tea.Model, tea.Cmd) {
	if m.settingsShown {
		m.settingsShown = false
		m.settingsPanel = nil
		return m, nil
	}
	currentTheme := "default"
	if m.theme != nil {
		currentTheme = m.theme.Name
	}
	m.settingsShown = true
	var actionsCfg config.ActionsConfig
	if m.cfg != nil {
		actionsCfg = m.cfg.Actions
	}
	m.settingsPanel = m.overlays.NewSettingsPanel(
		m.engine.CurrentPreviewPosition(),
		currentTheme,
		theme.ListThemes(),
		actionsCfg,
	)
	m.settingsPanel.Focus()
	w, h := m.settingsOverlayDims()
	m.settingsPanel.SetSize(w-2, h-2) // subtract border
	m.settingsPanel.Init(m.ctx)
	return m, nil
}

// settingsOverlayDims returns the outer dimensions for the settings overlay.
func (m Model) settingsOverlayDims() (int, int) {
	w := 44
	if w > m.width-4 {
		w = m.width - 4
	}
	if w < 20 {
		w = 20
	}
	h := m.height - 4
	if h > 40 {
		h = 40
	}
	if h < 6 {
		h = 6
	}
	// Clamp to terminal bounds for tiny terminals.
	if w > m.width {
		w = m.width
	}
	if h > m.height {
		h = m.height
	}
	return w, h
}

// overlayBorderColor is the fallback border color for overlay panels when
// the theme does not define BorderFocused.
const overlayBorderColor = "#BD93F9"

// overlayTitleColor is the fallback foreground color for overlay title text
// when the theme does not define NormalYellow.
const overlayTitleColor = "#FFB86C"

// overlayBorderCol returns the border color for overlay dialogs.
// It uses the theme's BorderFocused color so that all overlays (fuzzy
// finder, chat, bookmarks, settings, help) share the same border
// appearance. Falls back to overlayBorderColor when no theme is loaded.
func (m Model) overlayBorderCol() string {
	if m.theme != nil && m.theme.Colors.BorderFocused != "" {
		return m.theme.Colors.BorderFocused
	}
	return overlayBorderColor
}

// overlayTitleCol returns the title color for overlay dialogs.
// It uses the theme's NormalYellow color for consistency with the chat
// dialog title. Falls back to overlayTitleColor when no theme is loaded.
func (m Model) overlayTitleCol() string {
	if m.theme != nil && m.theme.Colors.NormalYellow != "" {
		return m.theme.Colors.NormalYellow
	}
	return overlayTitleColor
}

// injectBorderTitle reconstructs the top border line of a rendered lipgloss
// box, inserting a styled title label after the first horizontal border
// character. It rebuilds the line from the border definition to avoid
// corrupting ANSI escape sequences that wrap border characters.
func injectBorderTitle(rendered, title, titleColor, borderColor string, border lipgloss.Border) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(titleColor))
	label := "  " + titleStyle.Render(title) + "  "
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	topWidth := lipgloss.Width(lines[0])
	labelWidth := lipgloss.Width(label)
	if labelWidth+4 > topWidth {
		return rendered
	}
	bStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))
	// Reconstruct: ┌─ label ─────────┐
	// topWidth includes the two corner characters (1 cell each).
	innerWidth := topWidth - 2
	trailingDashes := innerWidth - 1 - labelWidth // 1 dash before label
	if trailingDashes < 0 {
		trailingDashes = 0
	}
	lines[0] = bStyle.Render(border.TopLeft+border.Top) +
		label +
		bStyle.Render(strings.Repeat(border.Top, trailingDashes)+border.TopRight)
	return strings.Join(lines, "\n")
}

// overlayViewer is the minimal interface for overlay panels that can be
// sized and rendered.
type overlayViewer interface {
	SetSize(width, height int)
	View(width, height int) string
}

// renderOverlayBox renders an overlay panel centered in the terminal with the
// given dimensions, border style, and optional title. If title is empty,
// injectBorderTitle is skipped.
func (m Model) renderOverlayBox(panel overlayViewer, w, h int, border lipgloss.Border, title string) string {
	contentW := w - 4 // subtract border + padding
	contentH := h - 2 // subtract border
	if contentW < 1 {
		contentW = 1
	}
	if contentH < 1 {
		contentH = 1
	}
	panel.SetSize(contentW, contentH)
	panelContent := panel.View(contentW, contentH)
	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(m.overlayBorderCol())).
		Padding(0, 1).
		Width(contentW).
		Height(contentH)
	rendered := style.Render(panelContent)
	if title != "" {
		rendered = injectBorderTitle(rendered, title, m.overlayTitleCol(), m.overlayBorderCol(), border)
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, rendered)
}

// addBookmark adds a directory to bookmarks and shows a toast.
func (m Model) addBookmark(path string) (tea.Model, tea.Cmd) {
	if m.bookmarkMgr == nil {
		return m, nil
	}
	if err := m.bookmarkMgr.Add(path); err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Warn}
		}
	}
	if err := m.bookmarkMgr.Save(); err != nil {
		errMsg := "Bookmark added but save failed: " + err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Warn}
		}
	}
	name := path
	if len(name) > 30 {
		name = "..." + name[len(name)-27:]
	}
	return m, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Bookmarked: " + name,
			Level:   notify.Success,
		}
	}
}

// handleUndo performs an undo operation and shows a toast with the result.
func (m Model) handleUndo() (tea.Model, tea.Cmd) {
	if m.undoMgr == nil || !m.undoMgr.CanUndo() {
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Nothing to undo", Level: notify.Info}
		}
	}
	desc, err := m.undoMgr.Undo(m.ctx)
	if err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Undo failed: " + errMsg, Level: notify.Warn}
		}
	}
	return m, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Undone: " + desc, Level: notify.Success}
	}
}

// handleRedo re-applies the most recently undone operation and shows a toast.
func (m Model) handleRedo() (tea.Model, tea.Cmd) {
	if m.undoMgr == nil || !m.undoMgr.CanRedo() {
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Nothing to redo", Level: notify.Info}
		}
	}
	desc, err := m.undoMgr.Redo(m.ctx)
	if err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Redo failed: " + errMsg, Level: notify.Warn}
		}
	}
	return m, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Redone: " + desc, Level: notify.Success}
	}
}

// ---------------------------------------------------------------------------
// Tab and split handlers
// ---------------------------------------------------------------------------
// handleNewTab creates a new tab from the given preset name.
// An empty preset defaults to "explorer".
func (m Model) handleNewTab(presetName string) (tea.Model, tea.Cmd) {
	if presetName == "" {
		presetName = "explorer"
	}
	preset, ok := layout.Presets()[presetName]
	if !ok {
		preset = layout.ExplorerPreset()
	}
	cmd, err := m.engine.AddTab(preset)
	if err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "New tab failed: " + errMsg, Level: notify.Warn}
		}
	}
	pn := presetName
	return m, tea.Batch(cmd, func() tea.Msg { return panels.TabActivatedMsg{PresetName: pn} })
}

// handleSwitchOrCreateTab switches to an existing tab with the given preset
// name, or creates a new tab with that preset if none exists.
func (m Model) handleSwitchOrCreateTab(presetName string) (tea.Model, tea.Cmd) {
	tm := m.engine.TabManager()
	tabs := tm.Tabs()
	// Check if a tab with this preset already exists; switch to it.
	for i, tab := range tabs {
		if tab.Name == presetName {
			if err := m.engine.SwitchTab(i); err == nil {
				pn := presetName
				return m, func() tea.Msg { return panels.TabActivatedMsg{PresetName: pn} }
			}
		}
	}
	// Create new tab with this preset.
	return m.handleNewTab(presetName)
}

// handleCloseTab closes the currently active tab.
func (m Model) handleCloseTab() (tea.Model, tea.Cmd) {
	if err := m.engine.CloseActiveTab(); err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Warn}
		}
	}
	return m, nil
}

// handleSplitVertical splits the focused panel vertically (new panel right).
func (m Model) handleSplitVertical(panelType string) (tea.Model, tea.Cmd) {
	if panelType == "" {
		panelType = "preview" //nolint:goconst // inline string is more readable here
	}
	cmd, err := m.engine.SplitFocusedVertical(panelType)
	if err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Split failed: " + errMsg, Level: notify.Warn}
		}
	}
	return m, cmd
}

// handleSplitHorizontal splits the focused panel horizontally (new panel below).
func (m Model) handleSplitHorizontal(panelType string) (tea.Model, tea.Cmd) {
	if panelType == "" {
		panelType = "preview"
	}
	cmd, err := m.engine.SplitFocusedHorizontal(panelType)
	if err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Split failed: " + errMsg, Level: notify.Warn}
		}
	}
	return m, cmd
}

// handleClosePanel closes the currently focused panel.
func (m Model) handleClosePanel() (tea.Model, tea.Cmd) {
	if err := m.engine.CloseFocusedPanel(); err != nil {
		errMsg := err.Error()
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Warn}
		}
	}
	return m, nil
}

// bookmarkOverlayDims returns the width and height for the bookmarks overlay.
func (m Model) bookmarkOverlayDims() (int, int) {
	return m.overlayDims(4, 40, 1, 2, 10)
}

// openFuzzyFinder creates and shows the fuzzy finder overlay with the
// appropriate source based on mode ("files" or "commands").
func (m Model) openFuzzyFinder(mode string) Model {
	var bindings []keymap.Binding
	if m.keys != nil {
		bindings = m.keys.Bindings()
	}
	ff := m.overlays.NewFuzzyFinder(mode, bindings)
	ff.Focus()
	w, h := m.fuzzyFinderDims()
	ff.SetSize(w, h)
	m.fuzzyFinder = ff
	return m
}

// fuzzyFinderDims returns the content dimensions for the fuzzy finder overlay.
func (m Model) fuzzyFinderDims() (int, int) {
	return m.overlayDims(5, 40, 3, 5, 10)
}

// View implements tea.Model. Composes all panels and the status bar
// into the final view. F27: overlay notifications on top of the panel layout.
func (m Model) View() tea.View {
	defer crashlog.GuardTUI("tui.View")
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if !m.ready {
		v.SetContent("Loading...")
		return v
	}
	content := m.renderLayout()
	// F27: Overlay toast notifications on top of the rendered layout.
	toastOverlay := m.notify.View(m.width)
	if toastOverlay != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, toastOverlay)
	}
	// F27: Overlay modal on top of everything.
	modalOverlay := m.notify.ModalView(m.width, m.height)
	if modalOverlay != "" {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modalOverlay)
	}
	// Fuzzy finder overlay.
	if m.fuzzyFinder != nil {
		w, h := m.fuzzyFinderDims()
		content = m.renderOverlayBox(m.fuzzyFinder, w, h, lipgloss.NormalBorder(), m.fuzzyFinder.Title())
	}
	// Bookmarks overlay.
	if m.bookmarksShown && m.bookmarkPanel != nil {
		w, h := m.bookmarkOverlayDims()
		content = m.renderOverlayBox(m.bookmarkPanel, w, h, lipgloss.RoundedBorder(), "Bookmarks")
	}
	// Settings overlay.
	if m.settingsShown && m.settingsPanel != nil {
		w, h := m.settingsOverlayDims()
		content = m.renderOverlayBox(m.settingsPanel, w, h, lipgloss.RoundedBorder(), "Settings")
	}

	// Welcome overlay.
	if m.welcomeShown && m.welcomePanel != nil {
		w, h := m.welcomeOverlayDims()
		contentW := w - 4 // subtract border + padding
		contentH := h - 2 // subtract border
		if contentW < 1 {
			contentW = 1
		}
		if contentH < 1 {
			contentH = 1
		}

		m.welcomePanel.SetSize(contentW, contentH)
		welcomeContent := m.welcomePanel.View(contentW, contentH)

		welcomeOverlayStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.overlayBorderCol())).
			Padding(0, 1).
			Height(contentH)

		welcomeRendered := welcomeOverlayStyle.Render(welcomeContent)

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, welcomeRendered)
	}

	// Help overlay.
	if m.helpShown && m.helpPanel != nil {
		w, h := m.helpOverlayDims()
		content = m.renderOverlayBox(m.helpPanel, w, h, lipgloss.RoundedBorder(), "grut \u2014 Terminal File Explorer")
	}
	// Git command log overlay.
	if m.commandLogShown && m.commandLogPanel != nil {
		w, h := m.commandLogOverlayDims()
		content = m.renderOverlayBox(m.commandLogPanel, w, h, lipgloss.RoundedBorder(), "Git Command Log")
	}
	v.SetContent(content)
	return v
}

// renderLayout composes all panels with borders and the status bar.
// When multiple tabs are open, a tab bar is rendered above the panel area.
// When the chat is focused, a floating modal dialog is overlaid on the
// panel area so panels remain visible (dimmed) behind it.
func (m Model) renderLayout() string {
	rects := m.engine.PanelRects()
	if len(rects) == 0 {
		return ""
	}
	focusedName := m.engine.FocusedName()
	allPanels := m.engine.Panels()
	// Determine the outer border color: use the focused panel's border color.
	borderColorStr := m.theme.Colors.BorderFocused
	var innerContent string
	if m.engine.IsZoomed() {
		// Zoomed: render only the focused panel
		rect := rects[focusedName]
		innerContent = m.renderPanel(focusedName, allPanels[focusedName], rect, true)
	} else {
		// Normal: render the full layout tree with separators
		tab := m.engine.TabManager().ActiveTab()
		innerContent = m.renderNode(tab.Tree, rects, allPanels, focusedName, borderColorStr)
	}
	// Wrap the composed inner content in a single outer rounded border.
	// We collect junction positions from the layout tree so that the
	// top/bottom border lines use ┬/┴ where vertical separators meet
	// them, and left/right borders use ├/┤ where horizontal separators
	// meet them.
	innerArea := m.engine.InnerArea()
	var tree layout.Node
	if !m.engine.IsZoomed() {
		tree = m.engine.TabManager().ActiveTab().Tree
	}
	// Compute top border titles from panels at Y=0 (topmost panels).
	topTitles := m.computeTopBorderTitles(rects, allPanels, focusedName)
	panelArea := m.buildOuterBorder(innerContent, innerArea.Width, innerArea.Height, borderColorStr, tree, topTitles)
	// Render status bar
	statusBar := m.renderStatusBar()
	// Render context-sensitive keybinding hints
	hintsBar := m.renderHintsBar()
	// Tab bar — empty string in single-tab mode.
	tabBar := m.renderTabBar()
	// Build the vertical component list, omitting the tab bar when empty.
	var components []string
	if tabBar != "" {
		components = append(components, tabBar)
	}
	if m.chat != nil {
		if m.chat.Focused() {
			// Floating modal overlay: panels render normally with a
			// centered chat dialog on top.
			panelAreaHeight := lipgloss.Height(panelArea)
			panelAreaWidth := m.width
			modalWidth := m.width * chatModalWidthPct / 100
			modalHeight := panelAreaHeight * chatModalHeightPct / 100
			if modalWidth < chatModalMinWidth {
				modalWidth = chatModalMinWidth
			}
			if modalHeight < chatModalMinHeight {
				modalHeight = chatModalMinHeight
			}
			if modalHeight > panelAreaHeight {
				modalHeight = panelAreaHeight
			}
			// Inner dimensions after subtracting the border (1 char each side).
			innerWidth := modalWidth - 2
			innerHeight := modalHeight - 2
			if innerWidth < chatInnerMinWidth {
				innerWidth = chatInnerMinWidth
			}
			if innerHeight < chatInnerMinHeight {
				innerHeight = chatInnerMinHeight
			}
			chatContent := m.chat.RenderModalContent(innerWidth, innerHeight)
			// Use the same overlay colors as other dialogs.
			borderColor := m.overlayBorderCol()
			titleColor := m.overlayTitleCol()
			// Build the modal border entirely from scratch to avoid
			// alignment issues with lipgloss's Border() + line replacement.
			bdr := lipgloss.RoundedBorder()
			bdrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))
			ttlStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(titleColor))
			// Size the content to exact dimensions without a border.
			contentBlock := lipgloss.NewStyle().
				Width(innerWidth).
				MaxWidth(innerWidth).
				Height(innerHeight).
				MaxHeight(innerHeight).
				Render(chatContent)
			// Top line with title: ╭── Chat ──────────╮
			titleText := ttlStyle.Render("Chat")
			titleVisualW := lipgloss.Width(titleText)
			prefixDashes := 2
			suffixDashes := innerWidth - prefixDashes - 1 - titleVisualW - 1
			if suffixDashes < 0 {
				suffixDashes = 0
			}
			topLine := bdrStyle.Render(bdr.TopLeft+strings.Repeat(bdr.Top, prefixDashes)) +
				bdrStyle.Render(" ") + titleText + bdrStyle.Render(" ") +
				bdrStyle.Render(strings.Repeat(bdr.Top, suffixDashes)+bdr.TopRight)
			// Wrap each content line with left/right borders.
			contentLines := strings.Split(contentBlock, "\n")
			for i, cl := range contentLines {
				contentLines[i] = bdrStyle.Render(bdr.Left) + cl + bdrStyle.Render(bdr.Right)
			}
			// Bottom line: ╰───────────────────╯
			bottomLine := bdrStyle.Render(bdr.BottomLeft + strings.Repeat(bdr.Bottom, innerWidth) + bdr.BottomRight)
			modalBox := topLine + "\n" + strings.Join(contentLines, "\n") + "\n" + bottomLine
			// Center the modal on top of the panel area.
			overlaidPanelArea := lipgloss.Place(
				panelAreaWidth, panelAreaHeight,
				lipgloss.Center, lipgloss.Center,
				modalBox,
			)
			components = append(components, overlaidPanelArea, hintsBar, statusBar)
			return lipgloss.JoinVertical(lipgloss.Left, components...)
		}
		// Not focused: render panels + chat footer normally.
		chatView := m.chat.View()
		components = append(components, panelArea, chatView, hintsBar, statusBar)
		return lipgloss.JoinVertical(lipgloss.Left, components...)
	}
	components = append(components, panelArea, hintsBar, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, components...)
}

// renderTabBar renders a styled tab bar with theme colors.
// Active tab is highlighted with bold and the active tab background color;
// inactive tabs use the status bar background. Unopened preset hints are
// shown dimmed to the right.
//
// v1: hidden — returns "" so the tab bar takes no vertical space.
func (m Model) renderTabBar() string {
	if layout.SingleTabMode {
		return "" // v1: single-tab mode, tab bar hidden
	}
	tm := m.engine.TabManager()
	tabs := tm.Tabs()
	// Determine the active tab name.
	activeName := ""
	if at := tm.ActiveTab(); at != nil {
		activeName = at.Name
	}
	type presetInfo struct {
		key  string
		name string
	}
	allPresets := []presetInfo{
		{"1", "explorer"},
		{"2", "git"},
		{"3", "review"},
		{"4", "agent"},
		{"5", "full"},
	}
	openNames := make(map[string]bool)
	for _, tab := range tabs {
		openNames[tab.Name] = true
	}
	activeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.TabActiveBg)).
		Foreground(lipgloss.Color(m.theme.Colors.TabActiveFg)).
		Bold(true).
		PaddingLeft(1).
		PaddingRight(1)
	inactiveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Foreground(lipgloss.Color(m.theme.Colors.TabInactiveFg)).
		PaddingLeft(1).
		PaddingRight(1)
	hintStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack))
	var parts []string
	// Render all presets in canonical 1-5 order: opened tabs are
	// active/inactive, unopened tabs are dimmed hints.
	for _, p := range allPresets {
		label := p.key + ":" + p.name
		if openNames[p.name] {
			if p.name == activeName {
				parts = append(parts, activeStyle.Render("▸ "+strings.ToUpper(label)))
			} else {
				parts = append(parts, inactiveStyle.Render(label))
			}
		} else {
			parts = append(parts, hintStyle.Render(" "+label))
		}
	}
	tabContent := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	// Pad to full width with the status bar background.
	contentWidth := lipgloss.Width(tabContent)
	if contentWidth < m.width {
		filler := lipgloss.NewStyle().
			Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
			Render(strings.Repeat(" ", m.width-contentWidth))
		tabContent += filler
	}
	return tabContent
}

// renderNode recursively renders the layout tree into a composed string.
// Panels are rendered without individual borders; vertical separators (│)
// are inserted between horizontally-split panels and horizontal separators
// (─) between vertically-split panels.
func (m Model) renderNode(
	node layout.Node,
	rects map[string]layout.Rect,
	allPanels map[string]panels.Panel,
	focusedName string,
	borderColorStr string,
) string {
	switch n := node.(type) {
	case *layout.LeafNode:
		rect, ok := rects[n.Panel]
		if !ok {
			return ""
		}
		return m.renderPanel(n.Panel, allPanels[n.Panel], rect, n.Panel == focusedName)
	case *layout.SplitNode:
		firstContent := m.renderNode(n.First, rects, allPanels, focusedName, borderColorStr)
		secondContent := m.renderNode(n.Second, rects, allPanels, focusedName, borderColorStr)
		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColorStr))
		if n.Direction == layout.Horizontal {
			// Build a vertical separator column (│) matching the content height.
			h := lipgloss.Height(firstContent)
			if sh := lipgloss.Height(secondContent); sh > h {
				h = sh
			}
			// Render the separator character once and reuse for all rows
			// to avoid per-row lipgloss.Render overhead.
			styledSep := sepStyle.Render("│")
			sepLines := make([]string, h)
			for i := range sepLines {
				sepLines[i] = styledSep
			}
			sep := strings.Join(sepLines, "\n")
			return lipgloss.JoinHorizontal(lipgloss.Top, firstContent, sep, secondContent)
		}
		// Vertical split: horizontal separator row (─) matching width.
		w := lipgloss.Width(firstContent)
		if sw := lipgloss.Width(secondContent); sw > w {
			w = sw
		}
		// Inject the title of the panel below the separator.
		bottomPanelName := layout.FirstPanelOf(n.Second)
		sep := m.renderSeparatorWithTitle(w, bottomPanelName, allPanels, focusedName, sepStyle)
		return lipgloss.JoinVertical(lipgloss.Left, firstContent, sep, secondContent)
	}
	return ""
}

// borderTitle holds the title and position information for a panel
// that should have its title rendered in the outer border.
type borderTitle struct {
	title    string
	startCol int
	endCol   int
	focused  bool
}

// renderSeparatorWithTitle renders a horizontal separator line (─) with the
// given panel's title injected. The focused panel's title is styled with
// TitleFocused (bold+cyan), unfocused with Title (normal foreground).
func (m Model) renderSeparatorWithTitle(
	width int,
	panelName string,
	allPanels map[string]panels.Panel,
	focusedName string,
	sepStyle lipgloss.Style,
) string {
	plain := sepStyle.Render(strings.Repeat("─", width))
	p, ok := allPanels[panelName]
	if !ok || width < 6 {
		return plain
	}
	title := p.Title()
	if title == "" {
		return plain
	}
	titleStyle := m.theme.Styles.Title
	if panelName == focusedName {
		titleStyle = m.theme.Styles.TitleFocused
	}
	// Truncate title if needed.
	maxLen := width - 6
	if maxLen < 3 {
		return plain
	}
	titleRunes := []rune(title)
	if len(titleRunes) > maxLen {
		title = string(titleRunes[:maxLen-1]) + "…"
	}
	label := " " + titleStyle.Render(title) + " "
	labelWidth := lipgloss.Width(label)
	remaining := width - 1 - labelWidth
	if remaining < 0 {
		return plain
	}
	return sepStyle.Render("─") + label + sepStyle.Render(strings.Repeat("─", remaining))
}

// renderPanel renders a single panel's content at the specified rect size.
// No individual border is drawn; the single outer border is applied by
// renderLayout after all panels are composed together.
func (m Model) renderPanel(
	_ string,
	p panels.Panel,
	rect layout.Rect,
	_ bool,
) string {
	if p == nil {
		return ""
	}
	// The rect IS the content area (no per-panel border to subtract).
	contentW := rect.Width
	contentH := rect.Height
	if contentW < 1 {
		contentW = 1
	}
	if contentH < 1 {
		contentH = 1
	}
	// Reserve horizontal padding inside the panel so content doesn't
	// press against the border characters.
	pad := layout.PanelPadH
	innerW := contentW - 2*pad
	if innerW < 1 {
		innerW = 1
	}
	leftPad := strings.Repeat(" ", pad)
	rightPad := strings.Repeat(" ", pad)
	content := p.View(innerW, contentH)
	// Normalize every content line to exactly innerW, then wrap with
	// horizontal padding so the final line width equals contentW.
	{
		lines := strings.Split(content, "\n")
		// Truncate to contentH lines so panels never overflow.
		if len(lines) > contentH {
			lines = lines[:contentH]
		}
		// Ensure we have exactly contentH lines.
		for len(lines) < contentH {
			lines = append(lines, strings.Repeat(" ", innerW))
		}
		// strings.Builder: avoid per-iteration string concat in render loop.
		// Reusing a single Builder across iterations amortises the buffer
		// allocation; writing padding bytes directly avoids strings.Repeat.
		var b strings.Builder
		for i, line := range lines {
			b.Reset()
			w := lipgloss.Width(line)
			if w > innerW {
				line = ansi.Truncate(line, innerW, "")
			}
			b.WriteString(leftPad)
			b.WriteString(line)
			if w < innerW {
				for j := 0; j < innerW-w; j++ {
					b.WriteByte(' ')
				}
			}
			b.WriteString(rightPad)
			lines[i] = b.String()
		}
		content = strings.Join(lines, "\n")
	}
	return content
}

// buildOuterBorder wraps the composed inner panel content in a single
// rounded border. Junction characters (┬/┴/├/┤) are placed where internal
// separators meet the outer border for a clean, connected appearance.
// Panel titles are injected into the top border for the topmost panels.
func (m Model) buildOuterBorder(content string, contentWidth, contentHeight int, borderColorStr string, tree layout.Node, topTitles []borderTitle) string {
	border := lipgloss.RoundedBorder()
	bdr := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColorStr))
	// Collect junction positions from the layout tree.
	topJ := map[int]bool{}
	bottomJ := map[int]bool{}
	leftJ := map[int]bool{}
	rightJ := map[int]bool{}
	if tree != nil {
		area := layout.Rect{X: 0, Y: 0, Width: contentWidth, Height: contentHeight}
		collectJunctions(tree, area, contentWidth, contentHeight, topJ, bottomJ, leftJ, rightJ)
	}
	// Find junction columns in the top border (sorted).
	var junctionCols []int
	for col := 0; col < contentWidth; col++ {
		if topJ[col] {
			junctionCols = append(junctionCols, col)
		}
	}
	// Build top border line with panel titles.
	topLine := m.buildTopBorderWithTitles(contentWidth, junctionCols, topTitles, bdr, border)
	// Build content lines with left/right border characters.
	contentLines := strings.Split(content, "\n")
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, strings.Repeat(" ", contentWidth))
	}
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	// strings.Builder: pre-render border chars and avoid per-line
	// lipgloss.Render + string concat in render loop.
	renderedLeft := bdr.Render(border.Left)
	renderedRight := bdr.Render(border.Right)
	renderedLeftJ := bdr.Render("├")
	renderedRightJ := bdr.Render("┤")
	var borderB strings.Builder
	for i, line := range contentLines {
		borderB.Reset()
		if leftJ[i] {
			borderB.WriteString(renderedLeftJ)
		} else {
			borderB.WriteString(renderedLeft)
		}
		borderB.WriteString(line)
		if rightJ[i] {
			borderB.WriteString(renderedRightJ)
		} else {
			borderB.WriteString(renderedRight)
		}
		contentLines[i] = borderB.String()
	}
	// Build bottom border line with junctions. Group consecutive non-junction
	// characters into single Render calls to avoid per-character ANSI overhead.
	var bottomParts []string
	bottomParts = append(bottomParts, bdr.Render(border.BottomLeft))
	runStart := 0
	for col := 0; col <= contentWidth; col++ {
		if col == contentWidth || bottomJ[col] {
			if run := col - runStart; run > 0 {
				bottomParts = append(bottomParts, bdr.Render(strings.Repeat(border.Bottom, run)))
			}
			if col < contentWidth {
				bottomParts = append(bottomParts, bdr.Render("┴"))
			}
			runStart = col + 1
		}
	}
	bottomParts = append(bottomParts, bdr.Render(border.BottomRight))
	bottomLine := strings.Join(bottomParts, "")
	// Assemble: top + content + bottom.
	result := topLine + "\n" + strings.Join(contentLines, "\n") + "\n" + bottomLine
	return result
}

// buildTopBorderWithTitles constructs the top border line, injecting panel
// titles into the sections between corner/junction characters. Each section
// corresponds to a topmost panel's column range.
func (m Model) buildTopBorderWithTitles(
	contentWidth int,
	junctionCols []int,
	topTitles []borderTitle,
	bdr lipgloss.Style,
	border lipgloss.Border,
) string {
	var parts []string
	parts = append(parts, bdr.Render(border.TopLeft))
	// Build section boundaries from junction columns.
	// Sections: [0, j0), [j0+1, j1), ..., [jN+1, contentWidth)
	type section struct {
		start, end int // column range [start, end)
	}
	sections := make([]section, 0, len(junctionCols)+1)
	prev := 0
	for _, j := range junctionCols {
		sections = append(sections, section{prev, j})
		prev = j + 1
	}
	sections = append(sections, section{prev, contentWidth})
	for si, sec := range sections {
		width := sec.end - sec.start
		// Find the title that belongs to this section.
		var title *borderTitle
		for i := range topTitles {
			if topTitles[i].startCol >= sec.start && topTitles[i].startCol < sec.end {
				title = &topTitles[i]
				break
			}
		}
		if title != nil && width > 4 {
			titleStyle := m.theme.Styles.Title
			if title.focused {
				titleStyle = m.theme.Styles.TitleFocused
			}
			t := title.title
			maxLen := width - 5
			tRunes := []rune(t)
			if len(tRunes) > maxLen {
				t = string(tRunes[:maxLen-1]) + "…"
			}
			label := " " + titleStyle.Render(t) + " "
			labelWidth := lipgloss.Width(label)
			remaining := width - 1 - labelWidth
			if remaining >= 0 {
				parts = append(parts, bdr.Render("─")+label+bdr.Render(strings.Repeat("─", remaining)))
			} else {
				// Title too long, fill with plain border.
				parts = append(parts, bdr.Render(strings.Repeat(border.Top, width)))
			}
		} else {
			parts = append(parts, bdr.Render(strings.Repeat(border.Top, width)))
		}
		// Add junction after this section (if not the last).
		if si < len(junctionCols) {
			parts = append(parts, bdr.Render("┬"))
		}
	}
	parts = append(parts, bdr.Render(border.TopRight))
	return strings.Join(parts, "")
}

// computeTopBorderTitles finds all panels at Y=0 (topmost) and returns
// their title, column range, and focus state for top border injection.
func (m Model) computeTopBorderTitles(
	rects map[string]layout.Rect,
	allPanels map[string]panels.Panel,
	focusedName string,
) []borderTitle {
	var titles []borderTitle
	for name, rect := range rects {
		if rect.Y != 0 {
			continue
		}
		p := allPanels[name]
		if p == nil {
			continue
		}
		titles = append(titles, borderTitle{
			startCol: rect.X,
			endCol:   rect.X + rect.Width - 1,
			title:    p.Title(),
			focused:  name == focusedName,
		})
	}
	// Sort by startCol for left-to-right processing.
	for i := 1; i < len(titles); i++ {
		for j := i; j > 0 && titles[j].startCol < titles[j-1].startCol; j-- {
			titles[j], titles[j-1] = titles[j-1], titles[j]
		}
	}
	return titles
}

// collectJunctions walks the layout tree and records where separators
// meet the outer border edges. topJ/bottomJ map column indices;
// leftJ/rightJ map row indices.
func collectJunctions(
	node layout.Node,
	area layout.Rect,
	totalW, totalH int,
	topJ, bottomJ map[int]bool,
	leftJ, rightJ map[int]bool,
) {
	split, ok := node.(*layout.SplitNode)
	if !ok {
		return
	}
	firstArea, secondArea := layout.SplitRect(area, split.Direction, split.Ratio)
	switch split.Direction {
	case layout.Horizontal:
		// The vertical separator column is at firstArea.X + firstArea.Width.
		sepCol := firstArea.X + firstArea.Width
		if area.Y == 0 {
			topJ[sepCol] = true
		}
		if area.Y+area.Height == totalH {
			bottomJ[sepCol] = true
		}
	case layout.Vertical:
		// The horizontal separator row is at firstArea.Y + firstArea.Height.
		sepRow := firstArea.Y + firstArea.Height
		if area.X == 0 {
			leftJ[sepRow] = true
		}
		if area.X+area.Width == totalW {
			rightJ[sepRow] = true
		}
	}
	collectJunctions(split.First, firstArea, totalW, totalH, topJ, bottomJ, leftJ, rightJ)
	collectJunctions(split.Second, secondArea, totalW, totalH, topJ, bottomJ, leftJ, rightJ)
}

// renderHintsBar renders a single-line context-sensitive keybinding hints
// bar. The hints change based on the currently focused panel, showing the
// most relevant keybindings for that context.
func (m Model) renderHintsBar() string {
	// When the chat footer is focused, show chat-specific hints.
	if m.chat != nil && m.chat.Focused() {
		hints := []string{"enter:send", "ctrl+e:expand", "ctrl+l:clear", "esc:blur", "ctrl+space:close"}
		return m.renderHintLine(hints)
	}
	focusedName := m.engine.FocusedName()
	// Panel-specific hints.
	var hints []string
	switch focusedName {
	case panelFileTree:
		hints = []string{"h/l:collapse/expand", hintFind, hintHelp}
	case "gitstatus":
		hints = []string{"s:stage", "u:unstage", "d:discard", "c:commit", "P:push", "p:pull", "F:fetch", hintHelp}
	case "preview":
		hints = []string{hintScroll, hintTabFocus, hintFind, hintHelp}
	case panelBranches:
		hints = []string{"enter:checkout", "n:new branch", "d:delete", hintHelp}
	case "gitlog":
		hints = []string{"enter:details", hintScroll, "/:search", hintHelp}
	case "gitdiff":
		hints = []string{hintScroll, hintTabFocus, hintHelp}
	case "terminal":
		hints = []string{"i:insert mode", "ctrl+b:normal mode", hintHelp}
	case "agents":
		hints = []string{hintScroll, "enter:select", hintHelp}
	case "extensions":
		hints = []string{"enter:toggle", "i:install", hintHelp}
	default:
		hints = []string{hintTabFocus, hintHelp, hintFind, "1-5:tabs"}
	}
	// Append chat hint when chat is available but not focused.
	if m.chat != nil {
		hints = append(hints, "ctrl+space:chat")
	}
	return m.renderHintLine(hints)
}

// renderHintLine renders a single-line styled hint bar from the given
// key:description pairs.
func (m Model) renderHintLine(hints []string) string {
	bg := lipgloss.Color(m.theme.Colors.StatusBarBg)
	// Style each hint: key part bold/colored, description dim.
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.Colors.NormalCyan)).
		Background(bg)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack)).
		Background(bg)
	fill := lipgloss.NewStyle().Background(bg)
	var parts []string
	for _, h := range hints {
		idx := strings.Index(h, ":")
		if idx > 0 {
			key := h[:idx]
			desc := h[idx+1:]
			parts = append(parts, keyStyle.Render(key)+fill.Render(" ")+descStyle.Render(desc))
		}
	}
	hintsText := fill.Render(" ") + strings.Join(parts, fill.Render("  "))
	// Fill remaining width with background-colored spaces (same
	// approach as renderStatusBar) so there are no unstyled gaps.
	textW := lipgloss.Width(hintsText)
	gap := m.width - textW
	if gap < 0 {
		gap = 0
	}
	return hintsText + fill.Render(strings.Repeat(" ", gap))
}

// handleCWDEditKey processes key events while the status bar CWD is being
// edited inline. Enter confirms the directory change, Escape cancels.
func (m Model) handleCWDEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	runes := []rune(m.cwdEditValue)
	switch msg.String() {
	case "enter":
		m.cwdEditing = false
		path := strings.TrimSpace(m.cwdEditValue)
		if path == "" {
			return m, nil
		}
		return m.Update(panels.ChangeDirectoryMsg{Path: path})
	case "esc":
		m.cwdEditing = false
		return m, nil
	case "backspace":
		if m.cwdEditCursor > 0 {
			m.cwdEditValue = string(runes[:m.cwdEditCursor-1]) + string(runes[m.cwdEditCursor:])
			m.cwdEditCursor--
		}
	case "delete":
		if m.cwdEditCursor < len(runes) {
			m.cwdEditValue = string(runes[:m.cwdEditCursor]) + string(runes[m.cwdEditCursor+1:])
		}
	case "left":
		if m.cwdEditCursor > 0 {
			m.cwdEditCursor--
		}
	case "right":
		if m.cwdEditCursor < len(runes) {
			m.cwdEditCursor++
		}
	case "home", "ctrl+a":
		m.cwdEditCursor = 0
	case "end", "ctrl+e":
		m.cwdEditCursor = len(runes)
	case "ctrl+u":
		m.cwdEditValue = ""
		m.cwdEditCursor = 0
	default:
		// Insert printable characters at cursor position.
		text := msg.Text
		if text != "" {
			runes = []rune(m.cwdEditValue)
			newRunes := make([]rune, 0, len(runes)+len([]rune(text)))
			newRunes = append(newRunes, runes[:m.cwdEditCursor]...)
			newRunes = append(newRunes, []rune(text)...)
			newRunes = append(newRunes, runes[m.cwdEditCursor:]...)
			m.cwdEditValue = string(newRunes)
			m.cwdEditCursor += len([]rune(text))
		}
	}
	return m, nil
}

// renderStatusBar renders the bottom status bar. When an async git
// operation is in progress, its description is shown alongside the
// working directory and git branch.
func (m Model) renderStatusBar() string {
	// If CWD is being edited, render the editable input instead.
	if m.cwdEditing {
		return m.renderStatusBarEditing()
	}
	// Left side: full cwd + git branch + async op.
	cwd, _ := os.Getwd()
	cwdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack)).
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
	leftParts := []string{cwdStyle.Render(cwd)}
	if m.currentBranch != "" {
		branchText := "⎇ " + ansi.Strip(m.currentBranch)
		if m.gitDirty {
			branchText += "*"
		}
		branchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.theme.Colors.GitBranch)).
			Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
			Bold(true)
		leftParts = append(leftParts, branchStyle.Render(branchText))
		// Ahead/behind indicators.
		if m.branchAhead > 0 || m.branchBehind > 0 {
			var abParts []string
			abStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.theme.Colors.NormalYellow)).
				Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
			if m.branchAhead > 0 {
				abParts = append(abParts, fmt.Sprintf("↑%d", m.branchAhead))
			}
			if m.branchBehind > 0 {
				abParts = append(abParts, fmt.Sprintf("↓%d", m.branchBehind))
			}
			leftParts = append(leftParts, abStyle.Render(strings.Join(abParts, " ")))
		}
	}
	if m.asyncOp != "" {
		asyncStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.theme.Colors.NormalYellow)).
			Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
		leftParts = append(leftParts, asyncStyle.Render("⟳ "+m.asyncOp))
	}
	if m.compareBase != "" {
		baseStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.theme.Colors.NormalCyan)).
			Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
		leftParts = append(leftParts, baseStyle.Render("⇄ base: "+ansi.Strip(m.compareBase)))
	}
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack)).
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Render(" │ ")
	leftText := strings.Join(leftParts, sep)
	leftPad := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Render(" ")
	left := leftPad + leftText
	// Right side: version + project/folder name in brackets.
	// Every segment must carry an explicit background to avoid
	// body-background bleed-through in the ANSI→HTML pipeline.
	sbBg := lipgloss.Color(m.theme.Colors.StatusBarBg)
	folderName := filepath.Base(func() string { d, _ := os.Getwd(); return d }())
	versionText := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack)).
		Background(sbBg).
		Render("v" + config.AppVersion)
	bracketText := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BorderFocused)).
		Background(sbBg).
		Bold(true).
		Render(" [" + folderName + "]")
	rightPad := lipgloss.NewStyle().Background(sbBg).Render(" ")
	brandText := versionText + bracketText + rightPad
	// Fill the gap between left and right.
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(brandText)
	gap := m.width - leftW - rightW
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Render(strings.Repeat(" ", gap))
	return left + filler + brandText
}

// renderStatusBarEditing renders the status bar with an inline text input
// for editing the current working directory path.
func (m Model) renderStatusBarEditing() string {
	runes := []rune(m.cwdEditValue)
	// Build the path text with a visible cursor.
	var before, cursor, after string
	if m.cwdEditCursor < len(runes) {
		before = string(runes[:m.cwdEditCursor])
		cursor = string(runes[m.cwdEditCursor : m.cwdEditCursor+1])
		after = string(runes[m.cwdEditCursor+1:])
	} else {
		before = m.cwdEditValue
		cursor = " "
		after = ""
	}
	editStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.NormalWhite)).
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Background(lipgloss.Color(m.theme.Colors.NormalWhite))
	pathText := editStyle.Render(before) + cursorStyle.Render(cursor) + editStyle.Render(after)
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Colors.BrightBlack)).
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg))
	hint := hintStyle.Render(" enter=confirm  esc=cancel")
	left := m.theme.Styles.StatusBar.
		PaddingLeft(1).
		Render(pathText + hint)
	leftW := lipgloss.Width(left)
	gap := m.width - leftW
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Colors.StatusBarBg)).
		Render(strings.Repeat(" ", gap))
	return left + filler
}
