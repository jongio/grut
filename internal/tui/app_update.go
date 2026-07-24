package tui

// Update message handlers extracted from the main type-switch in Update().
// Each method handles a logical group of related Bubble Tea messages,
// keeping the main Update dispatcher thin and readable.

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// handleWindowSizeMsg processes terminal resize events, propagating dimensions
// to chat, notification manager, and layout engine. It never issues a command
// (the caller supplies a nil tea.Cmd), so it returns only the updated model.
func (m Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) tea.Model {
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
	return m
}

// handleBranchMsg routes branch-info and git-dirty messages to update the
// status bar and trigger dependent refreshes.
func (m Model) handleBranchMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case branchLoadedMsg:
		// Ignore stale responses from a previous generation. A branch
		// checkout bumps branchInfoGen; any in-flight loadBranchInfo
		// from before the checkout carries the old gen and must be
		// discarded so it doesn't overwrite the freshly set branch name.
		if msg.generation != m.branchInfoGen {
			return m, nil
		}
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
		m.branchInfoGen++
		cmd := m.engine.Update(msg)
		return m, tea.Batch(
			cmd,
			m.checkGitDirty(),
			m.loadBranchInfo(),
			func() tea.Msg { return panels.RefreshGitStatusMsg{} },
			func() tea.Msg { return panels.RefreshBranchesMsg{} },
		)
	}
	return m, nil
}

// handleCompareBaseMsg updates the app-level pinned compare-base indicator
// while continuing to broadcast the message to panels.
func (m Model) handleCompareBaseMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.SetCompareBaseMsg:
		m.compareBase = msg.Ref
	case panels.ClearCompareBaseMsg:
		m.compareBase = ""
	}
	cmd := m.engine.Update(msg)
	return m, cmd
}

// handleNotifyMsg routes notification messages to the notify manager,
// or to a pending app-level action for modal results.
func (m Model) handleNotifyMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	}
	return m, nil
}

// handleGitOpMsg routes git operation request and result messages to their
// dedicated handlers.
func (m Model) handleGitOpMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	case discardFileDoneMsg:
		return m.handleDiscardFileDone(msg)
	case unstageFileDoneMsg:
		return m.handleUnstageFileDone(msg)
	case panels.AutoFetchTickMsg:
		return m.handleAutoFetchTick()
	case panels.RevertRequestMsg:
		return m.handleRevert(msg.Hash, msg.Subject)
	case revertDoneMsg:
		return m.handleRevertDone(msg)
	}
	return m, nil
}

// handleUndoRedoMsg routes undo/redo trigger and result messages.
func (m Model) handleUndoRedoMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	}
	return m, nil
}

// handleBookmarkNavMsg routes bookmark toggle, path navigation, and
// directory change messages.
func (m Model) handleBookmarkNavMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.ToggleBookmarksMsg:
		return m.toggleBookmarks()
	case panels.NavigateToPathMsg:
		// Close bookmarks overlay, then route to filetree.
		m.bookmarksShown = false
		m.bookmarkPanel = nil
		cmd := m.engine.Update(msg)
		return m, cmd
	case panels.ChangeDirectoryMsg:
		return m.handleChangeDirectoryMsg(msg)
	case panels.BookmarkAddMsg:
		return m.addBookmark(msg.Path)
	}
	return m, nil
}

// handleChangeDirectoryMsg changes the working directory and reinitializes
// the git client and panel state for the new location.
func (m Model) handleChangeDirectoryMsg(msg panels.ChangeDirectoryMsg) (tea.Model, tea.Cmd) {
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
	m.nav.reset()
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
	m.branchInfoGen++
	return m, tea.Batch(navCmd, repoCmd, toastCmd, m.checkGitDirty(), m.loadBranchInfo())
}

// handleOverlayMsg routes messages for help, welcome, and settings overlays.
func (m Model) handleOverlayMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// Help overlay messages.
	case panels.ToggleHelpMsg:
		return m.toggleHelp()
	case panels.ToggleCommandLogMsg:
		return m.toggleCommandLog()
	case panels.FirstRunMsg:
		return m.toggleWelcome()

	// Welcome overlay messages.
	case panels.WelcomeAnimTickMsg:
		if m.welcomeShown && m.welcomePanel != nil {
			_, cmd := m.welcomePanel.Update(msg)
			return m, cmd
		}
		return m, nil
	case panels.WelcomeDismissMsg:
		return m.dismissWelcome(msg)

	// Settings overlay messages.
	case panels.ToggleSettingsMsg:
		return m.toggleSettings()
	case panels.SetPreviewPositionMsg:
		m.engine.SetPreviewPosition(layout.PreviewPosition(msg.Position))
		if err := config.SaveUserSetting("preview.position", layout.PreviewPosition(msg.Position).String()); err != nil {
			slog.Warn("failed to persist preview position", "err", err)
		}
		return m, nil
	case panels.SetThemeMsg:
		// Theme change is persisted and takes effect on next launch,
		// matching dispatch's behaviour.
		if err := config.SaveUserSetting("theme.name", msg.Name); err != nil {
			slog.Warn("failed to persist theme", "err", err)
		}
		return m, nil
	case panels.SetDoubleClickActionMsg:
		// Persist action + confirmed flag to disk and update in-memory config.
		config.SaveDoubleClickChoice(&m.cfg.Actions, msg.ItemType, msg.Action)
		// Push updated config to all panels so double-click works immediately.
		m.broadcastActionsCfg()
		return m, nil
	case panels.SetRightClickActionMsg:
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
	case panels.ResetActionPromptsMsg:
		if err := config.ResetAllActionConfirmations(); err != nil {
			slog.Warn("failed to reset action confirmations", "err", err)
		}
		// Clear in-memory confirmed flags and push to all panels.
		m.cfg.Actions.Confirmed = make(map[string]bool)
		m.broadcastActionsCfg()
		return m, nil
	}
	return m, nil
}

// handleFuzzyFinderMsg routes fuzzy finder selection and dismiss messages.
func (m Model) handleFuzzyFinderMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
			return m.Update(panels.ChangeDirectoryMsg{Path: msg.Path})
		}
		// Normal file — broadcast to panels via engine.
		cmd := m.engine.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleChatMsg routes chat focus, refresh, and navigation messages.
func (m Model) handleChatMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.ChatFocusMsg:
		return m.toggleChatFocus()
	case panels.ChatRefreshMsg:
		// A chat tool changed repo state — refresh git status and panels.
		cmd := m.engine.Update(panels.RefreshGitStatusMsg{})
		return m, tea.Batch(cmd, m.checkGitDirty())
	case panels.ChatNavigateMsg:
		// Navigate to the file path requested by chat.
		cmd := m.engine.Update(panels.FileSelectedMsg{Path: msg.Path})
		return m, cmd
	}
	return m, nil
}

// handleChatStreamMsg forwards chat-internal streaming messages to the chat model.
// Caller must ensure m.chat is non-nil before calling.
func (m Model) handleChatStreamMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.chat.Update(msg)
	m.chat = &updated
	return m, cmd
}

// handlePanelLayoutMsg routes split, close, and git-status-changed messages
// to their dedicated handlers.
func (m Model) handlePanelLayoutMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	}
	return m, nil
}

// handleMouseMsg processes mouse click, release, and wheel events.
// Returns handled=false when the event should fall through to the default
// engine broadcast (e.g. clicks outside special areas).
func (m Model) handleMouseMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		// If a modal is active, route mouse clicks to the modal first.
		if m.notify.HasModal() {
			cmd := m.notify.Update(msg)
			return m, cmd, true
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
					return m, nil, true
				}
				m.lastStatusBarClick = now
			}
		}
		// Otherwise fall through to the layout engine (default case).
		return m, nil, false
	case tea.MouseReleaseMsg:
		// Swallow mouse releases while a modal is active so they don't
		// leak to the layout engine and trigger panel focus changes.
		if m.notify.HasModal() {
			return m, nil, true
		}
		// Otherwise fall through to the layout engine (default case).
		return m, nil, false
	case tea.MouseWheelMsg:
		// Route mouse wheel to overlay panels so they can scroll.
		if m.settingsShown && m.settingsPanel != nil {
			_, cmd := m.settingsPanel.Update(msg)
			return m, cmd, true
		}
		if m.helpShown && m.helpPanel != nil {
			_, cmd := m.helpPanel.Update(msg)
			return m, cmd, true
		}
		if m.commandLogShown && m.commandLogPanel != nil {
			_, cmd := m.commandLogPanel.Update(msg)
			return m, cmd, true
		}
		if m.welcomeShown && m.welcomePanel != nil {
			_, cmd := m.welcomePanel.Update(msg)
			return m, cmd, true
		}
		// Otherwise fall through to the layout engine (default case).
		return m, nil, false
	}
	return m, nil, false
}

// handleKeyPressMsg processes keyboard input, routing to modals, overlays,
// chat, keymap dispatcher, and global bindings in priority order.
func (m Model) handleKeyPressMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// If a modal is active, route keys to the modal first.
	if m.notify.HasModal() {
		cmd := m.notify.Update(msg)
		return m, cmd
	}
	// If CWD inline-edit is active, route all keys to it.
	if m.cwdEditing {
		return m.handleCWDEditKey(msg)
	}
	// If the preview panel has an inline prompt open (e.g. go-to-line),
	// route every key straight to it so digits and Enter/Esc are not
	// intercepted by global bindings.
	if m.previewInput {
		cmd := m.engine.Update(msg)
		return m, cmd
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
	if m.commandLogShown && m.commandLogPanel != nil {
		_, cmd := m.commandLogPanel.Update(msg)
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
	// If repo text search overlay is shown, route keys to it.
	if m.textSearch != nil {
		_, cmd := m.textSearch.Update(msg)
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
	// Undo/redo (global key bindings) — skip when preview is in edit mode
	// because edit mode has its own buffer-level undo/redo.
	if !m.previewEditing {
		if msg.String() == "ctrl+z" {
			return m.handleUndo()
		}
		if msg.String() == "ctrl+y" {
			return m.handleRedo()
		}
	}
	// Route unhandled keys to focused panel.
	cmd := m.engine.Update(msg)
	return m, cmd
}

// handleDefaultMsg broadcasts unhandled messages to all panels via the
// layout engine and forwards them to the chat model.
func (m Model) handleDefaultMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
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
