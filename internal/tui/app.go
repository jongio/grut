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
	"github.com/jongio/grut/internal/actions"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/chat"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	bmpanel "github.com/jongio/grut/internal/panels/bookmarks"
	"github.com/jongio/grut/internal/panels/fuzzyfinder"
	helppanel "github.com/jongio/grut/internal/panels/help"
	settingspanel "github.com/jongio/grut/internal/panels/settings"
	welcomepanel "github.com/jongio/grut/internal/panels/welcome"
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
	engine             *layout.Engine
	theme              *theme.Theme
	keys               *keymap.Keymap
	notify             *notify.Manager               // F27: integrated notification manager
	bookmarkMgr        *bm.Manager                   // bookmark persistence
	bookmarkPanel      *bmpanel.Panel                // overlay panel (nil = hidden)
	bookmarksShown     bool                          // whether bookmark overlay is visible
	fuzzyFinder        *fuzzyfinder.FuzzyFinder      // overlay fuzzy finder (nil = hidden)
	helpPanel          *helppanel.Panel              // overlay help panel (nil = hidden)
	helpShown          bool                          // whether help overlay is visible
	welcomePanel       *welcomepanel.Panel           // overlay welcome panel (nil = hidden)
	welcomeShown       bool                          // whether welcome overlay is visible
	settingsPanel      *settingspanel.Panel          // overlay settings panel (nil = hidden)
	settingsShown      bool                          // whether settings overlay is visible
	undoMgr            *git.UndoManager              // undo/redo manager (nil = disabled)
	gitClient          git.GitClient                 // git client for app-level operations (nil = no git)
	cfg                *config.Config                // app config (nil = defaults)
	sessionMgr         *session.Manager              // session persistence (nil = disabled)
	chat               *chat.Model                   // AI chat footer (nil if AI disabled)
	asyncOp            string                        // current async operation label ("pushing...", etc.)
	asyncCancel        context.CancelFunc            // cancel the running async operation
	pendingAction      string                        // action waiting for modal input ("commit")
	aiCommitSuggestion *panels.AICommitSuggestionMsg // AI-generated commit message suggestion
	currentBranch      string                        // cached git branch name for status bar
	gitDirty           bool                          // true when working tree has uncommitted changes
	branchAhead        int                           // commits ahead of upstream (needs push)
	branchBehind       int                           // commits behind upstream (needs pull)
	ctx                context.Context
	cancel             context.CancelFunc
	width              int
	height             int
	ready              bool      // true after first WindowSizeMsg
	cwdEditing         bool      // true when status bar CWD is in inline-edit mode
	cwdEditValue       string    // editable path text
	cwdEditCursor      int       // cursor position (rune index) within cwdEditValue
	lastStatusBarClick time.Time // for double-click detection on the status bar
}

// New creates a new TUI model with the given layout engine, theme, keymap,
// and bookmark manager.
func New(engine *layout.Engine, th *theme.Theme, km *keymap.Keymap, bmMgr *bm.Manager) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		engine:      engine,
		theme:       th,
		keys:        km,
		notify:      notify.NewManager(),
		bookmarkMgr: bmMgr,
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

// branchLoadedMsg carries the initial branch name and tracking info for the status bar.
type branchLoadedMsg struct {
	Name   string
	Ahead  int
	Behind int
}

// gitDirtyMsg reports whether the working tree has uncommitted changes.
type gitDirtyMsg struct{ dirty bool }

// Init implements tea.Model. Initializes all panels and starts the
// auto-fetch timer if configured.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.engine.Init(m.ctx)}
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
	if m.gitClient != nil {
		gc := m.gitClient
		ctx := m.ctx
		cmds = append(cmds, func() tea.Msg {
			branches, err := gc.BranchList(ctx)
			if err != nil {
				return branchLoadedMsg{}
			}
			for _, b := range branches {
				if b.IsCurrent {
					return branchLoadedMsg{Name: b.Name, Ahead: b.Ahead, Behind: b.Behind}
				}
			}
			return branchLoadedMsg{}
		})
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
	return func() tea.Msg {
		branches, err := gc.BranchList(ctx)
		if err != nil {
			return branchLoadedMsg{}
		}
		for _, b := range branches {
			if b.IsCurrent {
				return branchLoadedMsg{Name: b.Name, Ahead: b.Ahead, Behind: b.Behind}
			}
		}
		return branchLoadedMsg{}
	}
}

// Update implements tea.Model. Routes messages to the layout engine
// and handles global key bindings.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Inform chat of full terminal dimensions so overlay mode can
		// compute its height correctly.
		if m.chat != nil {
			m.chat.SetSize(msg.Width, msg.Height)
		}
		// Inform the notification manager so mouse clicks can be mapped to
		// modal-relative coordinates.
		m.notify.SetSize(msg.Width, msg.Height)
		// Subtract chat footer height so the engine reserves the right panel space.
		chatHeight := 0
		if m.chat != nil {
			chatHeight = m.chat.Height()
		}
		// The engine internally reserves space for status bar, hints bar, and tab bar.
		m.engine.SetSize(msg.Width, msg.Height-chatHeight)
		m.ready = true
		return m, nil

	// Branch info for status bar.
	case branchLoadedMsg:
		m.currentBranch = msg.Name
		m.branchAhead = msg.Ahead
		m.branchBehind = msg.Behind
		return m, m.checkGitDirty()
	case gitDirtyMsg:
		m.gitDirty = msg.dirty
		return m, nil
	case panels.BranchChangedMsg:
		m.currentBranch = msg.Name
		m.branchAhead = 0
		m.branchBehind = 0
		cmd := m.engine.Update(msg)
		return m, tea.Batch(cmd, m.checkGitDirty(), m.loadBranchInfo())

	// F27: Route notification messages to the notify manager.
	case notify.ShowToastMsg:
		cmd := m.notify.Update(msg)
		return m, cmd
	case notify.ShowModalMsg:
		cmd := m.notify.Update(msg)
		return m, cmd
	case notify.ToastExpiredMsg:
		cmd := m.notify.Update(msg)
		return m, cmd
	case notify.ModalResultMsg:
		// If an app-level action is pending (e.g. commit message input),
		// handle it here instead of forwarding to the focused panel.
		if m.pendingAction != "" {
			return m.handlePendingAction(msg)
		}
		// Route modal results to the focused panel so it can act on confirm/cancel.
		cmd := m.engine.Update(msg)
		return m, cmd

	// Git operation messages.
	case panels.CommitRequestMsg:
		return m.handleCommit()
	case panels.AmendRequestMsg:
		return m.handleAmend()
	case panels.RewordRequestMsg:
		return m.handleReword(msg.OldMessage)
	case panels.AICommitSuggestionMsg:
		m.aiCommitSuggestion = &msg
		return m, nil
	case panels.PushRequestMsg:
		return m.handlePush()
	case panels.PullRequestMsg:
		return m.handlePull()
	case panels.FetchRequestMsg:
		return m.handleFetch()
	case panels.AsyncOpDoneMsg:
		return m.handleAsyncOpDone(msg)
	case panels.AutoFetchTickMsg:
		return m.handleAutoFetchTick()

	// Undo/redo messages.
	case panels.UndoMsg:
		return m.handleUndo()
	case panels.RedoMsg:
		return m.handleRedo()
	case panels.UndoResultMsg:
		if msg.Err != nil {
			errMsg := msg.Err.Error()
			return m, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Undo failed: " + errMsg, Level: notify.Warn}
			}
		}
		desc := msg.Description
		return m, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Undone: " + desc, Level: notify.Success}
		}

	// Bookmark messages.
	case panels.ToggleBookmarksMsg:
		return m.toggleBookmarks()
	case panels.NavigateToPathMsg:
		// Close bookmarks overlay, then route to filetree.
		m.bookmarksShown = false
		m.bookmarkPanel = nil
		cmd := m.engine.Update(msg)
		return m, cmd
	case panels.ChangeDirectoryMsg:
		targetPath, err := filepath.Abs(msg.Path)
		if err != nil {
			return m, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: "Failed to resolve directory: " + err.Error(),
					Level:   notify.Error,
				}
			}
		}
		if err := os.Chdir(targetPath); err != nil {
			return m, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: "Failed to change directory: " + err.Error(),
					Level:   notify.Error,
				}
			}
		}
		// Reinitialize git client for the new directory.
		newClient, gitErr := git.NewClient(targetPath)
		if gitErr != nil {
			m.gitClient = nil
			m.currentBranch = ""
			m.gitDirty = false
			m.branchAhead = 0
			m.branchBehind = 0
		} else {
			m.gitClient = newClient
		}
		// Update filetree root.
		navCmd := m.engine.Update(panels.NavigateToPathMsg{Path: targetPath})
		// Broadcast repo change to ALL panels so they replace their git
		// client references and reload data (commits, log, status, diff,
		// gitinfo tabs, preview, etc.).
		repoCmd := m.engine.Update(panels.RepoChangedMsg{Path: targetPath})
		toastCmd := func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Changed directory to " + filepath.Base(targetPath),
				Level:   notify.Info,
			}
		}
		return m, tea.Batch(navCmd, repoCmd, toastCmd, m.checkGitDirty(), m.loadBranchInfo())
	case panels.BookmarkAddMsg:
		return m.addBookmark(msg.Path)

	// Help overlay messages.
	case panels.ToggleHelpMsg:
		return m.toggleHelp()
	case panels.FirstRunMsg:
		return m.toggleWelcome()

	// Welcome overlay messages.
	case welcomepanel.DismissMsg:
		return m.dismissWelcome(msg)

	// Settings overlay messages.
	case settingspanel.ToggleSettingsMsg:
		return m.toggleSettings()
	case settingspanel.SetPreviewPositionMsg:
		m.engine.SetPreviewPosition(msg.Position)
		if err := config.SaveUserSetting("preview.position", msg.Position.String()); err != nil {
			slog.Warn("failed to persist preview position", "err", err)
		}
		return m, nil
	case settingspanel.SetThemeMsg:
		// Theme change is persisted and takes effect on next launch,
		// matching dispatch's behaviour.
		if err := config.SaveUserSetting("theme.name", msg.Name); err != nil {
			slog.Warn("failed to persist theme", "err", err)
		}
		return m, nil
	case settingspanel.SetDoubleClickActionMsg:
		// Persist action + confirmed flag to disk and update in-memory config.
		config.SaveDoubleClickChoice(&m.cfg.Actions, msg.ItemType, msg.Action)
		// Push updated config to all panels so double-click works immediately.
		m.broadcastActionsCfg()
		return m, nil
	case settingspanel.SetRightClickActionMsg:
		if err := config.SetRightClickAction(actions.ItemType(msg.ItemType), actions.ActionID(msg.Action)); err != nil {
			slog.Warn("failed to persist right-click action", "err", err)
		}
		// Update in-memory config and push to all panels.
		if m.cfg.Actions.RightClick == nil {
			m.cfg.Actions.RightClick = make(map[string]string)
		}
		m.cfg.Actions.RightClick[msg.ItemType] = msg.Action
		m.broadcastActionsCfg()
		return m, nil
	case settingspanel.ResetActionPromptsMsg:
		if err := config.ResetAllActionConfirmations(); err != nil {
			slog.Warn("failed to reset action confirmations", "err", err)
		}
		// Clear in-memory confirmed flags and push to all panels.
		m.cfg.Actions.Confirmed = make(map[string]bool)
		m.broadcastActionsCfg()
		return m, nil

	// Fuzzy finder messages.
	case panels.ToggleFuzzyFinderMsg:
		m.fuzzyFinder = nil // close fuzzy finder
		return m, nil
	case panels.CommandSelectedMsg:
		m.fuzzyFinder = nil // close fuzzy finder
		return m.handleAction(msg.Action, msg)
	case panels.FileSelectedMsg:
		m.fuzzyFinder = nil // close fuzzy finder
		// If the selected path is a directory (from directory fuzzy finder),
		// convert to ChangeDirectoryMsg instead of opening the file.
		if info, statErr := os.Stat(msg.Path); statErr == nil && info.IsDir() {
			return m.Update(panels.ChangeDirectoryMsg(msg))
		}
		// Normal file — broadcast to panels via engine.
		cmd := m.engine.Update(msg)
		return m, cmd

	// Chat footer messages.
	case panels.ChatFocusMsg:
		return m.toggleChatFocus()
	case panels.ChatRefreshMsg:
		// A chat tool changed repo state — refresh git status and panels.
		cmd := m.engine.Update(panels.RefreshGitStatusMsg{})
		return m, tea.Batch(cmd, m.checkGitDirty())
	case panels.ChatNavigateMsg:
		// Navigate to the file path requested by chat.
		cmd := m.engine.Update(panels.FileSelectedMsg(msg))
		return m, cmd
	// Route chat-internal messages to the chat model.
	case chat.StreamChunkMsg, chat.ToolCallMsg, chat.ToolResultMsg,
		chat.SendMessageCmd:
		if m.chat != nil {
			updated, cmd := m.chat.Update(msg)
			m.chat = &updated
			return m, cmd
		}

	// Tab management messages — disabled for v1 single-tab mode.
	case panels.NewTabMsg:
		return m, nil // v1: no-op
	case panels.CloseTabMsg:
		return m, nil // v1: no-op
	case panels.NextTabMsg:
		return m, nil // v1: no-op
	case panels.PrevTabMsg:
		return m, nil // v1: no-op
	case panels.SwitchTabMsg:
		return m, nil // v1: no-op

	// Split / panel messages.
	case panels.SplitVerticalMsg:
		return m.handleSplitVertical(msg.PanelType)
	case panels.SplitHorizontalMsg:
		return m.handleSplitHorizontal(msg.PanelType)
	case panels.ClosePanelMsg:
		return m.handleClosePanel()

	// GitStatusChangedMsg needs app-level side effect (dirty check).
	// All other cross-panel messages fall through to the default
	// engine.Update() which broadcasts to all panels automatically.
	case panels.GitStatusChangedMsg:
		cmd := m.engine.Update(msg)
		return m, tea.Batch(cmd, m.checkGitDirty())

	case tea.MouseClickMsg:
		// If a modal is active, route mouse clicks to the modal first.
		if m.notify.HasModal() {
			cmd := m.notify.Update(msg)
			return m, cmd
		}
		// Double-click on the status bar CWD area enters inline-edit mode.
		if msg.Button == tea.MouseLeft && msg.Y == m.height-1 {
			// Only trigger on the CWD area (left portion of status bar).
			cwd, _ := os.Getwd()
			cwdWidth := len([]rune(cwd)) + 4 // path + padding/icon
			if cwdWidth > m.width/2 {
				cwdWidth = m.width / 2
			}
			if msg.X <= cwdWidth {
				now := time.Now()
				if now.Sub(m.lastStatusBarClick) <= 500*time.Millisecond {
					m.cwdEditing = true
					m.cwdEditValue = cwd
					m.cwdEditCursor = len([]rune(cwd))
					m.lastStatusBarClick = time.Time{}
					return m, nil
				}
				m.lastStatusBarClick = now
			}
		}
		// Otherwise fall through to the layout engine (default case).

	case tea.MouseReleaseMsg:
		// Swallow mouse releases while a modal is active so they don't
		// leak to the layout engine and trigger panel focus changes.
		if m.notify.HasModal() {
			return m, nil
		}
		// Otherwise fall through to the layout engine (default case).

	case tea.MouseWheelMsg:
		// Route mouse wheel to overlay panels so they can scroll.
		if m.settingsShown && m.settingsPanel != nil {
			_, cmd := m.settingsPanel.Update(msg)
			return m, cmd
		}
		if m.helpShown && m.helpPanel != nil {
			_, cmd := m.helpPanel.Update(msg)
			return m, cmd
		}
		if m.welcomeShown && m.welcomePanel != nil {
			_, cmd := m.welcomePanel.Update(msg)
			return m, cmd
		}
		// Otherwise fall through to the layout engine (default case).

	case tea.KeyPressMsg:
		// If a modal is active, route keys to the modal first.
		if m.notify.HasModal() {
			cmd := m.notify.Update(msg)
			return m, cmd
		}

		// If CWD inline-edit is active, route all keys to it.
		if m.cwdEditing {
			return m.handleCWDEditKey(msg)
		}

		// If an async operation is running, Esc cancels it.
		if msg.String() == "esc" && m.asyncCancel != nil {
			return m.cancelAsyncOp()
		}

		// If help overlay is shown, route keys to it.
		if m.helpShown && m.helpPanel != nil {
			_, cmd := m.helpPanel.Update(msg)
			return m, cmd
		}

		// If welcome overlay is shown, route keys to it.
		if m.welcomeShown && m.welcomePanel != nil {
			_, cmd := m.welcomePanel.Update(msg)
			return m, cmd
		}

		// If settings overlay is shown, route keys to it.
		if m.settingsShown && m.settingsPanel != nil {
			_, cmd := m.settingsPanel.Update(msg)
			return m, cmd
		}

		// If bookmarks overlay is shown, route keys to it.
		if m.bookmarksShown && m.bookmarkPanel != nil {
			_, cmd := m.bookmarkPanel.Update(msg)
			return m, cmd
		}

		// If fuzzy finder overlay is shown, route keys to it.
		if m.fuzzyFinder != nil {
			_, cmd := m.fuzzyFinder.Update(msg)
			return m, cmd
		}

		// If chat footer is focused, route ALL keys to it so that
		// typing characters (?, !, letters, Enter, etc.) are consumed
		// by the chat's text input and do NOT leak to the parent
		// keymap. Only ctrl+space passes through to toggle focus off.
		if m.chat != nil && m.chat.Focused() {
			if msg.String() == "ctrl+space" {
				return m.toggleChatFocus()
			}
			updated, cmd := m.chat.Update(msg)
			m.chat = &updated
			return m, cmd
		}

		if m.keys != nil {
			action, handled := m.keys.Dispatch(msg.String(), m.engine.FocusedName())
			if handled {
				return m.handleAction(action, msg)
			}
			// If a multi-key prefix is pending, swallow the key.
			if m.keys.HasPending() {
				return m, nil
			}
		}

		// Global refresh: R refreshes all panels + forces preview re-render.
		if msg.String() == "R" {
			return m.handleGlobalRefresh()
		}

		// Undo/redo (global key bindings).
		if msg.String() == "ctrl+z" {
			return m.handleUndo()
		}
		if msg.String() == "ctrl+y" {
			return m.handleRedo()
		}

		// Route unhandled keys to focused panel.
		cmd := m.engine.Update(msg)
		return m, cmd
	}

	// Route all other messages to all panels via engine broadcast,
	// and forward to the chat model so internal chat messages
	// (e.g. stream-done, tool-exec-done) are handled.
	var cmds []tea.Cmd
	cmds = append(cmds, m.engine.Update(msg))
	if m.chat != nil {
		updated, chatCmd := m.chat.Update(msg)
		m.chat = &updated
		if chatCmd != nil {
			cmds = append(cmds, chatCmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// handleAction maps a dispatched action string to concrete model operations.
// Global actions (quit, focus, zoom, resize) are handled directly.
// Panel-level actions are forwarded to the focused panel.
func (m Model) handleAction(action string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch action {
	case "quit":
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
	case "zoom_toggle":
		m.engine.ToggleZoom()
		return m, nil
	case "resize_left":
		m.engine.ResizeShrink()
		return m, nil
	case "resize_right":
		m.engine.ResizeGrow()
		return m, nil
	case "resize_up":
		m.engine.ResizeShrink()
		return m, nil
	case "resize_down":
		m.engine.ResizeGrow()
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
	case "help":
		return m.toggleHelp()
	case "welcome":
		return m.toggleWelcome()
	case "settings":
		return m.toggleSettings()
	case "chat_focus":
		return m.toggleChatFocus()
	case "commit":
		return m.handleCommit()
	case "push":
		return m.handlePush()
	case "pull":
		return m.handlePull()
	case "fetch":
		return m.handleFetch()

	// Direct panel focus (1-5 number keys).
	case "focus_panel_1":
		m.engine.FocusByName("filetree")
		return m, nil
	case "focus_panel_2":
		m.engine.FocusByName("gitinfo")
		return m, nil
	case "focus_panel_3":
		m.engine.FocusByName("github")
		return m, nil
	case "focus_panel_4":
		m.engine.FocusByName("commits")
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
	m.bookmarkPanel = bmpanel.New(m.bookmarkMgr)
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
	m.helpPanel = helppanel.New()
	m.helpPanel.Focus()
	w, h := m.helpOverlayDims()
	m.helpPanel.SetSize(w, h)
	m.helpPanel.Init(m.ctx)

	// Mark first run as done so the overlay won't auto-show next time.
	_ = session.MarkFirstRunDone()
	return m, nil
}

// helpOverlayDims returns the content dimensions for the help overlay.
func (m Model) helpOverlayDims() (int, int) {
	w := m.width * 3 / 5
	if w < 40 {
		w = 40
	}
	if w > m.width-4 {
		w = m.width - 4
	}

	h := m.height * 3 / 4
	if h < 10 {
		h = 10
	}
	if h > m.height-4 {
		h = m.height - 4
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

// toggleWelcome shows or hides the welcome overlay panel.
func (m Model) toggleWelcome() (tea.Model, tea.Cmd) {
	if m.welcomeShown {
		m.welcomeShown = false
		m.welcomePanel = nil
		return m, nil
	}

	m.welcomeShown = true
	m.welcomePanel = welcomepanel.New()
	m.welcomePanel.Focus()
	w, h := m.welcomeOverlayDims()
	m.welcomePanel.SetSize(w, h)
	m.welcomePanel.Init(m.ctx)

	// Mark first run as done so the overlay won't auto-show next time.
	_ = session.MarkFirstRunDone()
	return m, nil
}

// dismissWelcome handles the welcome panel dismiss message.
func (m Model) dismissWelcome(msg welcomepanel.DismissMsg) (tea.Model, tea.Cmd) {
	m.welcomeShown = false
	m.welcomePanel = nil

	if msg.DontShowAgain {
		if err := config.SaveUserSettingBool("general.show_first_run_help", false); err != nil {
			slog.Warn("failed to persist show_first_run_help", "err", err)
		}
	}

	return m, nil
}

// welcomeOverlayDims returns the content dimensions for the welcome overlay.
func (m Model) welcomeOverlayDims() (int, int) {
	w := m.width * 3 / 5
	if w < 44 {
		w = 44
	}
	if w > m.width-4 {
		w = m.width - 4
	}

	h := m.height * 4 / 5
	if h < 20 {
		h = 20
	}
	if h > m.height-4 {
		h = m.height - 4
	}

	if w > m.width {
		w = m.width
	}
	if h > m.height {
		h = m.height
	}

	return w, h
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
	m.settingsPanel = settingspanel.New(
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
	w := m.width * 3 / 4
	if w < 40 {
		w = 40
	}
	if w > m.width-4 {
		w = m.width - 4
	}

	h := m.height / 2
	if h < 10 {
		h = 10
	}
	if h > m.height-4 {
		h = m.height - 4
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

// openFuzzyFinder creates and shows the fuzzy finder overlay with the
// appropriate source based on mode ("files" or "commands").
func (m Model) openFuzzyFinder(mode string) Model {
	var sources []fuzzyfinder.Source

	switch mode {
	case "files":
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		sources = append(sources, fuzzyfinder.NewFileSource(cwd))
	case "commands":
		if m.keys != nil {
			sources = append(sources, fuzzyfinder.NewCommandSource(m.keys.Bindings()))
		}
	case "directories":
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		sources = append(sources, fuzzyfinder.NewDirectorySource(cwd, fuzzyfinder.DefaultDirectorySourceMaxDepth))
	}

	ff := fuzzyfinder.New(sources...)
	ff.Focus()
	w, h := m.fuzzyFinderDims()
	ff.SetSize(w, h)
	m.fuzzyFinder = ff
	return m
}

// fuzzyFinderDims returns the content dimensions for the fuzzy finder overlay.
func (m Model) fuzzyFinderDims() (int, int) {
	w := m.width * 3 / 5
	if w < 40 {
		w = 40
	}
	if w > m.width-4 {
		w = m.width - 4
	}

	h := m.height * 3 / 5
	if h < 10 {
		h = 10
	}
	if h > m.height-4 {
		h = m.height - 4
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

// View implements tea.Model. Composes all panels and the status bar
// into the final view. F27: overlay notifications on top of the panel layout.
func (m Model) View() tea.View {
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
		contentW := w - 4 // subtract border + padding
		contentH := h - 2 // subtract border
		if contentW < 1 {
			contentW = 1
		}
		if contentH < 1 {
			contentH = 1
		}

		m.fuzzyFinder.SetSize(contentW, contentH)
		ffContent := m.fuzzyFinder.View(contentW, contentH)

		ffOverlayStyle := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(m.overlayBorderCol())).
			Padding(0, 1).
			Width(contentW).
			Height(contentH)

		ffRendered := ffOverlayStyle.Render(ffContent)
		ffRendered = injectBorderTitle(ffRendered, m.fuzzyFinder.Title(), m.overlayTitleCol(), m.overlayBorderCol(), lipgloss.NormalBorder())

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ffRendered)
	}

	// Bookmarks overlay.
	if m.bookmarksShown && m.bookmarkPanel != nil {
		w, h := m.bookmarkOverlayDims()
		contentW := w - 4 // subtract border + padding
		contentH := h - 2 // subtract border
		if contentW < 1 {
			contentW = 1
		}
		if contentH < 1 {
			contentH = 1
		}

		m.bookmarkPanel.SetSize(contentW, contentH)
		panelContent := m.bookmarkPanel.View(contentW, contentH)

		bmOverlayStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.overlayBorderCol())).
			Padding(0, 1).
			Width(contentW).
			Height(contentH)

		bmRendered := bmOverlayStyle.Render(panelContent)
		bmRendered = injectBorderTitle(bmRendered, "Bookmarks", m.overlayTitleCol(), m.overlayBorderCol(), lipgloss.RoundedBorder())

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, bmRendered)
	}

	// Settings overlay.
	if m.settingsShown && m.settingsPanel != nil {
		w, h := m.settingsOverlayDims()
		contentW := w - 4 // subtract border + padding
		contentH := h - 2 // subtract border
		if contentW < 1 {
			contentW = 1
		}
		if contentH < 1 {
			contentH = 1
		}

		m.settingsPanel.SetSize(contentW, contentH)
		settingsContent := m.settingsPanel.View(contentW, contentH)

		settingsOverlayStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.overlayBorderCol())).
			Padding(0, 1).
			Width(contentW).
			Height(contentH)

		settingsRendered := settingsOverlayStyle.Render(settingsContent)
		settingsRendered = injectBorderTitle(settingsRendered, "Settings", m.overlayTitleCol(), m.overlayBorderCol(), lipgloss.RoundedBorder())

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, settingsRendered)
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
			Width(contentW).
			Height(contentH)

		welcomeRendered := welcomeOverlayStyle.Render(welcomeContent)
		welcomeRendered = injectBorderTitle(welcomeRendered, "Welcome to grut", m.overlayTitleCol(), m.overlayBorderCol(), lipgloss.RoundedBorder())

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, welcomeRendered)
	}

	// Help overlay.
	if m.helpShown && m.helpPanel != nil {
		w, h := m.helpOverlayDims()
		contentW := w - 4 // subtract border + padding
		contentH := h - 2 // subtract border
		if contentW < 1 {
			contentW = 1
		}
		if contentH < 1 {
			contentH = 1
		}

		m.helpPanel.SetSize(contentW, contentH)
		helpContent := m.helpPanel.View(contentW, contentH)

		helpOverlayStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.overlayBorderCol())).
			Padding(0, 1).
			Width(contentW).
			Height(contentH)

		helpRendered := helpOverlayStyle.Render(helpContent)
		helpRendered = injectBorderTitle(helpRendered, "grut — Terminal File Explorer", m.overlayTitleCol(), m.overlayBorderCol(), lipgloss.RoundedBorder())

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpRendered)
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
			sepLines := make([]string, h)
			for i := range sepLines {
				sepLines[i] = sepStyle.Render("│")
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
	startCol int
	endCol   int
	title    string
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

	// Truncate content to contentH lines so panels never overflow their
	// allocated rectangle and push the footer off screen.
	if lines := strings.Split(content, "\n"); len(lines) > contentH {
		content = strings.Join(lines[:contentH], "\n")
	}

	// Normalize every content line to exactly innerW, then wrap with
	// horizontal padding so the final line width equals contentW.
	{
		lines := strings.Split(content, "\n")
		// Ensure we have exactly contentH lines.
		for len(lines) < contentH {
			lines = append(lines, strings.Repeat(" ", innerW))
		}
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w > innerW {
				line = ansi.Truncate(line, innerW, "")
			} else if w < innerW {
				line += strings.Repeat(" ", innerW-w)
			}
			lines[i] = leftPad + line + rightPad
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
	for i, line := range contentLines {
		leftChar := border.Left
		if leftJ[i] {
			leftChar = "├"
		}
		rightChar := border.Right
		if rightJ[i] {
			rightChar = "┤"
		}
		contentLines[i] = bdr.Render(leftChar) + line + bdr.Render(rightChar)
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
	case "filetree":
		hints = []string{"h/l:collapse/expand", "/:find", "?:help"}
	case "gitstatus":
		hints = []string{"s:stage", "u:unstage", "d:discard", "c:commit", "P:push", "p:pull", "F:fetch", "?:help"}
	case "preview":
		hints = []string{"j/k:scroll", "Tab:focus", "/:find", "?:help"}
	case "branches":
		hints = []string{"enter:checkout", "n:new branch", "d:delete", "?:help"}
	case "gitlog":
		hints = []string{"enter:details", "j/k:scroll", "/:search", "?:help"}
	case "gitdiff":
		hints = []string{"j/k:scroll", "Tab:focus", "?:help"}
	case "terminal":
		hints = []string{"i:insert mode", "ctrl+b:normal mode", "?:help"}
	case "agents":
		hints = []string{"j/k:scroll", "enter:select", "?:help"}
	case "extensions":
		hints = []string{"enter:toggle", "i:install", "?:help"}
	default:
		hints = []string{"Tab:focus", "?:help", "/:find", "1-5:tabs"}
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
		branchText := "⎇ " + m.currentBranch
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
