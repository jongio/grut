package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// asyncOpPushing is the status label shown during git push operations.
const asyncOpPushing = "pushing..."

// toastMsgMaxLen is the maximum number of characters kept from a commit
// message when it is displayed in a toast notification.
const toastMsgMaxLen = 40

// pendingActionCommit identifies the commit pending action.
const pendingActionCommit = "commit"

// pendingActionAmend identifies the amend-commit pending action.
const pendingActionAmend = "amend"

// pendingActionReword identifies the reword-commit pending action.
const pendingActionReword = "reword"

// asyncOpFetching is the status label shown during fetch operations.
const asyncOpFetching = "fetching..."

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

// handleCommit opens the commit message input modal. If an AI commit
// suggestion is available, the input is pre-filled with the formatted
// suggestion. The actual commit is executed when the user confirms the
// modal (see handlePendingAction).
func (m Model) handleCommit() (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" || m.notify.HasModal() {
		return m, nil
	}
	m.pendingAction = pendingActionCommit

	// Pre-fill with AI suggestion if available.
	if s := m.aiCommitSuggestion; s != nil {
		prefill := formatCommitSuggestion(s)
		m.aiCommitSuggestion = nil // consume the suggestion
		return m, notify.ShowInputWithValue("Commit", "Enter commit message...", prefill)
	}

	return m, notify.ShowInput("Commit", "Enter commit message...")
}

// executeCommit runs git commit asynchronously with the provided message.
func (m Model) executeCommit(commitMsg string) (tea.Model, tea.Cmd) {
	if commitMsg == "" {
		return m, showWarnToast("Empty commit message")
	}

	m.asyncOp = "committing..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel

	gc := m.gitClient
	sign := m.cfg != nil && m.cfg.Git.SignCommits
	undoMgr := m.undoMgr

	return m, func() tea.Msg {
		hash, err := gc.Commit(ctx, commitMsg, git.CommitOpts{Sign: sign})
		if err != nil {
			return panels.AsyncOpDoneMsg{Description: pendingActionCommit, Err: err}
		}

		// Record for undo so the user can revert the commit.
		if undoMgr != nil {
			undoMgr.RecordAction(git.UndoAction{
				Type:      pendingActionCommit,
				RefBefore: hash,
				Metadata: map[string]string{
					"message": commitMsg,
				},
			})
		}

		desc := "commit: " + truncateForToast(commitMsg, toastMsgMaxLen)
		return panels.AsyncOpDoneMsg{Description: desc, Err: nil}
	}
}

// ---------------------------------------------------------------------------
// Amend & Reword
// ---------------------------------------------------------------------------

// handleAmend opens a commit message modal for amending the last commit.
// If an AI suggestion is available it pre-fills; otherwise the modal is empty.
func (m Model) handleAmend() (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" || m.notify.HasModal() {
		return m, nil
	}
	m.pendingAction = pendingActionAmend

	if s := m.aiCommitSuggestion; s != nil {
		prefill := formatCommitSuggestion(s)
		m.aiCommitSuggestion = nil
		return m, notify.ShowInputWithValue("Amend Commit", "Enter new commit message...", prefill)
	}

	return m, notify.ShowInput("Amend Commit", "Enter new commit message...")
}

// handleReword opens a commit message modal pre-filled with the current
// commit message so the user can edit it.
func (m Model) handleReword(oldMessage string) (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" || m.notify.HasModal() {
		return m, nil
	}
	m.pendingAction = pendingActionReword
	return m, notify.ShowInputWithValue("Reword Commit", "Edit commit message...", oldMessage)
}

// executeAmend amends the last commit asynchronously with the provided message.
func (m Model) executeAmend(commitMsg string) (tea.Model, tea.Cmd) {
	if commitMsg == "" {
		return m, showWarnToast("Empty commit message")
	}

	m.asyncOp = "amending..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel

	gc := m.gitClient
	sign := m.cfg != nil && m.cfg.Git.SignCommits
	undoMgr := m.undoMgr

	return m, func() tea.Msg {
		hash, err := gc.Commit(ctx, commitMsg, git.CommitOpts{Amend: true, Sign: sign})
		if err != nil {
			return panels.AsyncOpDoneMsg{Description: pendingActionAmend, Err: err}
		}

		if undoMgr != nil {
			undoMgr.RecordAction(git.UndoAction{
				Type:      pendingActionAmend,
				RefBefore: hash,
				Metadata: map[string]string{
					"message": commitMsg,
				},
			})
		}

		desc := pendingActionAmend + ": " + truncateForToast(commitMsg, toastMsgMaxLen)
		return panels.AsyncOpDoneMsg{Description: desc, Err: nil}
	}
}

// executeReword changes only the commit message without including staged changes.
func (m Model) executeReword(commitMsg string) (tea.Model, tea.Cmd) {
	if commitMsg == "" {
		return m, showWarnToast("Empty commit message")
	}

	m.asyncOp = "rewording..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel

	gc := m.gitClient
	sign := m.cfg != nil && m.cfg.Git.SignCommits
	undoMgr := m.undoMgr

	return m, func() tea.Msg {
		hash, err := gc.Commit(ctx, commitMsg, git.CommitOpts{
			Amend:      true,
			RewordOnly: true,
			Sign:       sign,
		})
		if err != nil {
			return panels.AsyncOpDoneMsg{Description: pendingActionReword, Err: err}
		}

		if undoMgr != nil {
			undoMgr.RecordAction(git.UndoAction{
				Type:      pendingActionReword,
				RefBefore: hash,
				Metadata: map[string]string{
					"message": commitMsg,
				},
			})
		}

		desc := pendingActionReword + ": " + truncateForToast(commitMsg, toastMsgMaxLen)
		return panels.AsyncOpDoneMsg{Description: desc, Err: nil}
	}
}

// ---------------------------------------------------------------------------
// Push / Pull / Fetch
// ---------------------------------------------------------------------------

// handlePush runs git push asynchronously.
func (m Model) handlePush() (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" {
		return m, showWarnToast("Operation in progress: " + m.asyncOp)
	}

	m.asyncOp = asyncOpPushing
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel
	gc := m.gitClient

	return m, func() tea.Msg {
		err := gc.Push(ctx, git.PushOpts{})
		return panels.AsyncOpDoneMsg{Description: "push", Err: err}
	}
}

// handlePull runs git pull asynchronously.
func (m Model) handlePull() (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" {
		return m, showWarnToast("Operation in progress: " + m.asyncOp)
	}

	m.asyncOp = "pulling..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel
	gc := m.gitClient

	return m, func() tea.Msg {
		err := gc.Pull(ctx, git.PullOpts{})
		return panels.AsyncOpDoneMsg{Description: "pull", Err: err}
	}
}

// handleFetch runs git fetch --all --prune asynchronously.
func (m Model) handleFetch() (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, showWarnToast("Git not available")
	}
	if m.asyncOp != "" {
		return m, showWarnToast("Operation in progress: " + m.asyncOp)
	}

	m.asyncOp = asyncOpFetching
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel
	gc := m.gitClient

	return m, func() tea.Msg {
		err := gc.Fetch(ctx, git.FetchOpts{All: true, Prune: true})
		return panels.AsyncOpDoneMsg{Description: "fetch", Err: err}
	}
}

// ---------------------------------------------------------------------------
// Async operation completion
// ---------------------------------------------------------------------------

// handleAsyncOpDone clears the loading state and shows a toast with the
// result. On success it also emits RefreshGitStatusMsg so panels update.
func (m Model) handleAsyncOpDone(msg panels.AsyncOpDoneMsg) (tea.Model, tea.Cmd) {
	m.asyncOp = ""
	m.asyncCancel = nil

	if msg.Err != nil {
		if errors.Is(msg.Err, context.Canceled) {
			return m, showInfoToast(msg.Description + " cancelled")
		}
		errMsg := msg.Err.Error()
		desc := msg.Description
		return m, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: desc + " failed: " + errMsg,
				Level:   notify.Error,
			}
		}
	}

	desc := msg.Description
	return m, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: desc + " succeeded",
				Level:   notify.Success,
			}
		},
		func() tea.Msg {
			return panels.RefreshGitStatusMsg{}
		},
		m.loadBranchInfo(),
	)
}

// ---------------------------------------------------------------------------
// Pending action (modal result dispatch)
// ---------------------------------------------------------------------------

// handlePendingAction processes a ModalResultMsg for an app-level action
// (currently only "commit"). Clears pendingAction regardless of outcome.
func (m Model) handlePendingAction(msg notify.ModalResultMsg) (tea.Model, tea.Cmd) {
	action := m.pendingAction
	m.pendingAction = ""

	if !msg.Accept {
		return m, nil
	}

	switch action {
	case pendingActionCommit:
		return m.executeCommit(msg.Value)
	case pendingActionAmend:
		return m.executeAmend(msg.Value)
	case pendingActionReword:
		return m.executeReword(msg.Value)
	default:
		return m, nil
	}
}

// ---------------------------------------------------------------------------
// Cancel async operation
// ---------------------------------------------------------------------------

// cancelAsyncOp cancels the running async git operation and shows a toast.
func (m Model) cancelAsyncOp() (tea.Model, tea.Cmd) {
	if m.asyncCancel != nil {
		m.asyncCancel()
	}
	desc := m.asyncOp
	m.asyncOp = ""
	m.asyncCancel = nil
	return m, showInfoToast(desc + " cancelled")
}

// ---------------------------------------------------------------------------
// Auto-fetch
// ---------------------------------------------------------------------------

// handleAutoFetchTick runs a background fetch and schedules the next tick.
// If an async operation is already in progress the fetch is skipped.
func (m Model) handleAutoFetchTick() (tea.Model, tea.Cmd) {
	nextTick := m.autoFetchTickCmd()

	if m.gitClient == nil || m.asyncOp != "" {
		return m, nextTick
	}

	gc := m.gitClient
	ctx := m.ctx

	return m, tea.Batch(
		func() tea.Msg {
			err := gc.Fetch(ctx, git.FetchOpts{All: true, Prune: true})
			if err != nil {
				// Auto-fetch shows toast only on error (silent success).
				errMsg := err.Error()
				return notify.ShowToastMsg{
					Message: "Auto-fetch failed: " + errMsg,
					Level:   notify.Warn,
				}
			}
			return panels.RefreshGitStatusMsg{}
		},
		nextTick,
	)
}

// autoFetchTickCmd returns a tea.Tick command for the next auto-fetch cycle.
// Returns nil if auto-fetch is not configured.
func (m Model) autoFetchTickCmd() tea.Cmd {
	if m.cfg == nil || m.cfg.Git.AutoFetchInterval.Duration <= 0 {
		return nil
	}
	d := m.cfg.Git.AutoFetchInterval.Duration
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return panels.AutoFetchTickMsg{Time: t}
	})
}

// ---------------------------------------------------------------------------
// Toast helpers
// ---------------------------------------------------------------------------

func showWarnToast(msg string) tea.Cmd {
	return func() tea.Msg {
		return notify.ShowToastMsg{Message: msg, Level: notify.Warn}
	}
}

func showInfoToast(msg string) tea.Cmd {
	return func() tea.Msg {
		return notify.ShowToastMsg{Message: msg, Level: notify.Info}
	}
}

// truncateForToast shortens a string for display in a toast notification.
func truncateForToast(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatCommitSuggestion formats an AI commit suggestion as a conventional
// commit string: "type(scope): subject". If type or scope is empty, the
// corresponding part is omitted.
func formatCommitSuggestion(s *panels.AICommitSuggestionMsg) string {
	if s == nil || s.Subject == "" {
		return ""
	}
	prefix := s.Type
	if prefix != "" && s.Scope != "" {
		prefix += "(" + s.Scope + ")"
	}
	if prefix != "" {
		return prefix + ": " + s.Subject
	}
	return s.Subject
}
