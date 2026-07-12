// Package panels defines shared message types used for inter-panel communication.
package panels

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
)

// TargetedPanelMsg wraps a message that should only be delivered to a
// specific panel by name, avoiding unnecessary broadcast to all panels.
// This prevents periodic tick messages (e.g. animation frames) from
// triggering full-UI re-renders when only one panel needs the update.
type TargetedPanelMsg struct {
	Inner  tea.Msg // the actual message to deliver
	Target string  // panel name (e.g. "gitinfo")
}

// DiffContextType enumerates the kinds of diff context the preview can show.
type DiffContextType int

const (
	// DiffContextWorking shows the working tree diff (unstaged, fallback staged).
	DiffContextWorking DiffContextType = iota
	// DiffContextStaged shows the staged diff (index vs HEAD).
	DiffContextStaged
	// DiffContextCommit shows a single commit's diff (parent..commit).
	DiffContextCommit
	// DiffContextBranch shows a branch comparison diff (base...HEAD).
	DiffContextBranch
	// DiffContextPR shows a pull request diff (base..head).
	DiffContextPR
)

// DiffContext describes the diff to show for a file selection. The preview
// panel uses this to load the correct diff based on the filetree's current
// navigation mode.
type DiffContext struct {
	Type     DiffContextType
	CommitA  string // base ref (e.g., "abc123~1", "main")
	CommitB  string // head ref (e.g., "abc123", "HEAD")
	ThreeDot bool   // use merge-base comparison (A...B)
}

// FileSelectedMsg is sent when the user selects (opens) a file.
// Both filetree and preview panels use this shared type so that
// cross-panel message routing works correctly.
type FileSelectedMsg struct {
	Path        string
	DiffContext *DiffContext // nil = working tree diff (backward compatible)
	Line        int          // 1-based line to scroll the preview to (0 = none)
}

// RevealFileMsg requests the filetree to expand parent directories and
// move the cursor to the specified file path. Emitted by the fuzzy finder
// when the user selects a file, so the file tree highlights it.
type RevealFileMsg struct {
	Path string
	Line int // 1-based line to scroll the preview to after selecting (0 = none)
}

// ToggleFuzzyFinderMsg is sent to show or hide the fuzzy finder overlay.
// When the fuzzy finder is visible, this message closes it; when hidden,
// this message has no effect (opening is handled by specific actions).
type ToggleFuzzyFinderMsg struct{}

// CommandSelectedMsg is sent when the user selects a command from the
// command palette. The Action field holds the action identifier to execute
// (e.g. "quit", "focus_next").
type CommandSelectedMsg struct {
	Action string
}

// ToggleBookmarksMsg requests showing or hiding the bookmarks overlay panel.
type ToggleBookmarksMsg struct{}

// ToggleHelpMsg requests showing or hiding the help overlay panel.
type ToggleHelpMsg struct{}

// FirstRunMsg is sent when the application detects it is the user's first run.
// The root model shows the help overlay in response.
type FirstRunMsg struct{}

// NavigateToPathMsg requests the filetree to change its root directory.
type NavigateToPathMsg struct {
	Path string
}

// ChangeDirectoryMsg requests changing the application's working directory.
// Unlike NavigateToPathMsg (which only changes the filetree root), this
// changes the process CWD, reinitializes git, and refreshes all panels.
type ChangeDirectoryMsg struct {
	Path string
}

// RepoChangedMsg is broadcast after the working directory has changed to a
// new repository. All panels that hold a git client reference should replace
// it with a fresh client for the new path and reload their data. Path is
// the absolute path of the new working directory.
type RepoChangedMsg struct {
	Path string
}

// BookmarkAddMsg requests adding the given directory path as a bookmark.
type BookmarkAddMsg struct {
	Path string
}

// BookmarkRemoveMsg requests removing a bookmark by path.
type BookmarkRemoveMsg struct {
	Path string
}

// BranchChangedMsg is sent after a successful branch checkout so that other
// panels can react to the new HEAD (e.g. refresh git status, file tree).
type BranchChangedMsg struct {
	Name string
}

// BranchSelectedMsg is sent when the user selects (clicks or navigates to)
// a branch in the gitinfo panel. Other panels (e.g. gitlog) can react by
// showing data for that branch without switching HEAD.
type BranchSelectedMsg struct {
	Name string
}

// BranchDeselectedMsg is sent when the user deselects a branch (e.g. Escape
// or clicking the same branch again in the gitinfo panel).
type BranchDeselectedMsg struct{}

// RefreshBranchesMsg is received by the branches panel to trigger a re-fetch
// of the branch list.
type RefreshBranchesMsg struct{}

// StashChangedMsg is emitted after stash operations (push, pop, apply, drop)
// so that other panels can refresh their stash-dependent state.
type StashChangedMsg struct{}

// CherryPickMsg requests cherry-picking a commit by hash. Typically sent
// from the git log panel to the stash panel for execution.
type CherryPickMsg struct {
	Hash string
}

// ShowDiffMsg requests showing a diff for the specified file path.
// Staged indicates whether to show the staged (index vs HEAD) diff.
// When CommitA and CommitB are set, shows a ref comparison diff instead.
type ShowDiffMsg struct {
	Path     string
	CommitA  string // base ref for comparison (e.g., "main")
	CommitB  string // head ref for comparison (e.g., "HEAD")
	Staged   bool
	ThreeDot bool // use three-dot (merge-base) comparison
}

// UndoMsg is sent when the user requests an undo operation (ctrl+z).
// The root model intercepts this and delegates to the UndoManager.
type UndoMsg struct{}

// RedoMsg is sent when the user requests a redo operation (ctrl+y).
// The root model intercepts this and delegates to the UndoManager.
type RedoMsg struct{}

// UndoResultMsg is sent after an undo or redo operation completes.
// It carries a human-readable description or an error for toast display.
type UndoResultMsg struct {
	Err         error
	Description string
}

// GitStatusChangedMsg is emitted by the gitstatus panel after a stage/unstage
// operation so that other panels (e.g. filetree) can update file markers.
type GitStatusChangedMsg struct {
	Files []git.FileStatus
}

// RefreshGitStatusMsg is received by the gitstatus panel to trigger a re-fetch
// of git status (e.g. after an external commit or file change).
type RefreshGitStatusMsg struct{}

// RefreshPreviewMsg tells the preview panel to re-render whatever it is
// currently showing (file, GitHub issue, PR, CI run, etc.) without changing
// the displayed content type.
type RefreshPreviewMsg struct{}

// RefreshGitChangedFilesMsg tells the filetree to reload its git-changed
// file list. Sent after operations (discard, unstage) that change which
// files appear in the git filter view.
type RefreshGitChangedFilesMsg struct{}

// GitChangedFilesMsg delivers the set of git-changed file paths to
// panels that need to filter by git status (e.g. filetree).
type GitChangedFilesMsg struct {
	Paths map[string]bool // absolute paths of changed files
}

// GitFilterActiveMsg notifies panels whether git-filter mode is active.
// When active, the preview panel shows diff-only instead of file content.
type GitFilterActiveMsg struct {
	Active bool
}

// BranchDiffFilterActiveMsg notifies panels whether branch-diff filter mode
// is active (the "b" toggle). When active, the filetree shows only files
// that differ from the base branch.
type BranchDiffFilterActiveMsg struct {
	Active     bool
	BaseBranch string // e.g., "main"
}

// PreviewScrollMsg requests the preview panel to scroll by Delta lines.
// Positive = down, negative = up. Emitted by filetree so the user can
// scroll the preview without Tab-focusing it (e.g. J/K keys).
type PreviewScrollMsg struct {
	Delta int
}

// PanelMouseClickMsg is sent by the layout engine when a mouse click
// lands inside a panel. Coordinates are relative to the panel's content
// area (inside borders), so ContentRow 0 is the first visible line.
type PanelMouseClickMsg struct {
	ContentRow int // row within the panel content area (0-based)
	ContentCol int // column within the panel content area (0-based)
}

// PanelMouseDoubleClickMsg is sent by the layout engine when two clicks
// arrive within 300ms at the same panel position. Used for action triggers
// such as checkout, worktree switch, file open, etc.
type PanelMouseDoubleClickMsg struct {
	ContentRow int // row within the panel content area (0-based)
	ContentCol int // column within the panel content area (0-based)
}

// PanelHeaderDoubleClickMsg is sent by the layout engine when a
// double-click lands on the panel header / border title area (the row
// above the panel content). Panels can use this to trigger a
// header-specific action, e.g. opening a repo URL in a browser.
type PanelHeaderDoubleClickMsg struct {
	ContentCol int // approximate column within the panel area
}

// PanelMouseRightClickMsg is sent by the layout engine when a right-click
// lands inside a panel. Coordinates are relative to the panel's content
// area (inside borders), so ContentRow 0 is the first visible line.
type PanelMouseRightClickMsg struct {
	ContentRow int // row within the panel content area (0-based)
	ContentCol int // column within the panel content area (0-based)
}

// PanelMouseMotionMsg is sent by the layout engine when the mouse moves
// within a panel while a button is held. Coordinates are relative to the
// panel's content area (inside borders). Used for drag-based text
// selection in the preview panel.
type PanelMouseMotionMsg struct {
	ContentRow int // row within the panel content area (0-based)
	ContentCol int // column within the panel content area (0-based)
}

// PanelMouseReleaseMsg is sent by the layout engine when a mouse button
// is released inside a panel. Coordinates are relative to the panel's
// content area (inside borders). Used to finalize text selection.
type PanelMouseReleaseMsg struct {
	ContentRow int // row within the panel content area (0-based)
	ContentCol int // column within the panel content area (0-based)
}

// ToggleBlameMsg requests toggling blame annotations for the given file.
// Emitted by the preview panel when the user presses B; the root model
// catches this to start an async git-blame load.
type ToggleBlameMsg struct {
	Path string
}

// BlameLoadedMsg delivers blame annotation data to the preview panel.
// Produced by the root model after running git blame asynchronously.
type BlameLoadedMsg struct {
	Err   error
	Lines []git.BlameLine
}

// BisectStatusMsg reports the current bisect session status.
// Emitted after bisect operations so the status bar can display progress.
type BisectStatusMsg struct {
	Current        string
	StepsRemaining int
	Active         bool
}

// ---------------------------------------------------------------------------
// Git operation messages (app-level commit/push/pull/fetch)
// ---------------------------------------------------------------------------
// CommitRequestMsg is sent to trigger the commit dialog from any panel.
type CommitRequestMsg struct{}

// PushRequestMsg is sent to trigger a git push from any panel.
type PushRequestMsg struct{}

// PullRequestMsg is sent to trigger a git pull from any panel.
type PullRequestMsg struct{}

// FetchRequestMsg is sent to trigger a git fetch from any panel.
type FetchRequestMsg struct{}

// AmendRequestMsg is sent to trigger an amend of the last commit.
// The panel opens a commit message modal; the app handles the actual amend.
type AmendRequestMsg struct{}

// RewordRequestMsg is sent to reword the last commit's message.
// OldMessage carries the current commit message so the modal can pre-fill it.
type RewordRequestMsg struct {
	OldMessage string
}

// AsyncOpStartMsg is emitted when an async git operation begins. The root
// model uses it to display a loading indicator in the status bar.
type AsyncOpStartMsg struct {
	Description string
}

// AsyncOpDoneMsg is emitted when an async git operation completes. The root
// model clears the loading indicator and shows a toast with the result.
type AsyncOpDoneMsg struct {
	Err         error
	Description string
}

// AutoFetchTickMsg drives the recurring auto-fetch timer. The root model
// handles this by performing a background fetch if configured.
type AutoFetchTickMsg struct {
	Time time.Time
}

// WorktreeChangedMsg is emitted after worktree create/remove so that other
// panels can react (e.g. refresh worktree-dependent state).
type WorktreeChangedMsg struct{}

// RemoteChangedMsg is emitted after remote add/remove so that other
// panels can react (e.g. refresh remote-dependent state).
type RemoteChangedMsg struct{}

// TagChangedMsg is emitted after tag operations (create, delete) so that
// other panels can refresh their tag-dependent state.
type TagChangedMsg struct{}

// MergeRequestMsg requests merging a branch into the current HEAD.
// Typically sent from the branches panel when the user presses m.
type MergeRequestMsg struct {
	Branch string
}

// RebaseRequestMsg requests rebasing the current branch onto a target ref.
// Typically sent from the branches panel when the user presses r.
type RebaseRequestMsg struct {
	Onto string
}

// ConflictDetectedMsg is emitted when a merge or rebase encounters conflicts.
// Files lists the paths of conflicted files.
type ConflictDetectedMsg struct {
	Files []string
}

// ConflictResolvedMsg is emitted when all conflicts have been resolved
// and the merge/rebase can continue.
type ConflictResolvedMsg struct{}

// MergeAbortMsg requests aborting the current merge operation.
type MergeAbortMsg struct{}

// RebaseAbortMsg requests aborting the current rebase operation.
type RebaseAbortMsg struct{}

// ---------------------------------------------------------------------------
// Review messages (diff review mode)
// ---------------------------------------------------------------------------
// StartReviewMsg enters the diff review mode, loading all changed files
// for hunk-level approve/reject decisions.
type StartReviewMsg struct{}

// ReviewCompleteMsg is emitted when the user exits review mode. Summary
// contains a plain-text export of all review decisions.
type ReviewCompleteMsg struct {
	Summary string
}

// HunkApprovedMsg is emitted when a single diff hunk is approved during
// review. Other panels can react (e.g. refresh git status).
type HunkApprovedMsg struct {
	Path      string
	HunkIndex int
}

// HunkRejectedMsg is emitted when a single diff hunk is rejected during
// review. Other panels can react (e.g. refresh git status).
type HunkRejectedMsg struct {
	Path      string
	HunkIndex int
}

// ---------------------------------------------------------------------------
// Context builder messages
// ---------------------------------------------------------------------------
// AddToContextMsg requests adding a file to the AI context builder.
type AddToContextMsg struct {
	Path string
}

// RemoveFromContextMsg requests removing a file from the AI context builder.
type RemoveFromContextMsg struct {
	Path string
}

// ClearContextMsg requests clearing all files from the context builder.
type ClearContextMsg struct{}

// ExportContextMsg requests exporting the context as structured text.
type ExportContextMsg struct{}

// ContextUpdatedMsg notifies other panels that the context has changed.
type ContextUpdatedMsg struct {
	FileCount  int
	TokenCount int
}

// ---------------------------------------------------------------------------
// Agent monitor messages
// ---------------------------------------------------------------------------
// AgentSpawnedMsg is emitted when a new agent process is spawned.
type AgentSpawnedMsg struct {
	Command string
	PID     int
}

// AgentExitedMsg is emitted when an agent process exits.
type AgentExitedMsg struct {
	PID      int
	ExitCode int
}

// AgentOutputMsg is emitted when an agent produces a line of output.
type AgentOutputMsg struct {
	Line string
	PID  int
}

// ---------------------------------------------------------------------------
// Terminal messages
// ---------------------------------------------------------------------------
// TerminalOutputMsg notifies other panels that new terminal output is available.
// Lines contains the latest snapshot of all output lines.
type TerminalOutputMsg struct {
	Lines []string
}

// TerminalExitedMsg is emitted when the embedded terminal's shell process exits.
type TerminalExitedMsg struct {
	ExitCode int
}

// ---------------------------------------------------------------------------
// Tab management messages
// ---------------------------------------------------------------------------
// NewTabMsg requests creating a new tab with the given layout preset.
// An empty Preset string defaults to "explorer".
type NewTabMsg struct {
	Preset string
}

// CloseTabMsg requests closing the currently active tab.
type CloseTabMsg struct{}

// TabActivatedMsg is emitted after a tab switch completes so that panels
// can react to the active preset (e.g. auto-filter filetree in git mode).
type TabActivatedMsg struct {
	PresetName string
}

// SwitchTabMsg requests switching to the tab at the given index.
type SwitchTabMsg struct {
	Index int
}

// NextTabMsg requests switching to the next tab (wrapping).
type NextTabMsg struct{}

// PrevTabMsg requests switching to the previous tab (wrapping).
type PrevTabMsg struct{}

// ---------------------------------------------------------------------------
// Extension messages
// ---------------------------------------------------------------------------
// ToggleExtensionsMsg requests showing or hiding the extensions overlay panel.
type ToggleExtensionsMsg struct{}

// ExtensionChangedMsg is emitted after an extension install, remove, or
// enable/disable so that other panels can refresh extension-dependent state.
type ExtensionChangedMsg struct{}

// ---------------------------------------------------------------------------
// Split / panel messages
// ---------------------------------------------------------------------------
// SplitVerticalMsg requests splitting the focused panel vertically,
// placing a new panel of PanelType to the right.
type SplitVerticalMsg struct {
	PanelType string
}

// SplitHorizontalMsg requests splitting the focused panel horizontally,
// placing a new panel of PanelType below.
type SplitHorizontalMsg struct {
	PanelType string
}

// ClosePanelMsg requests closing the currently focused panel.
type ClosePanelMsg struct{}

// ResizePanelMsg requests resizing the focused panel boundary.
// Direction is "left", "right", "up", or "down".
type ResizePanelMsg struct {
	Direction string
	Delta     int
}

// ---------------------------------------------------------------------------
// AI and Chat messages
// ---------------------------------------------------------------------------
// ChatFocusMsg requests the chat footer to take or release focus.
type ChatFocusMsg struct{}

// ChatNavigateMsg asks the UI to navigate to a file path, triggered
// by the AI chat's navigate_to tool.
type ChatNavigateMsg struct {
	Path string
}

// ChatRefreshMsg signals that the chat performed an operation that
// may have changed repository state, so panels should refresh.
type ChatRefreshMsg struct{}

// AIConflictResolvedMsg indicates that AI has resolved a conflict
// in the given file.
type AIConflictResolvedMsg struct {
	Path string
}

// AIReviewReadyMsg carries AI-generated code review findings for
// display in the diff panel.
type AIReviewReadyMsg struct {
	Findings []AIReviewFinding
}

// AIReviewFinding is a single code review annotation from the AI.
type AIReviewFinding struct {
	File       string
	Severity   string // "error", "warning", "info", "hint"
	Category   string // "security", "bug", "performance", "style", "test"
	Message    string
	Suggestion string
	Line       int
}

// AICommitSuggestionMsg carries an AI-generated commit message
// suggestion for the commit input.
type AICommitSuggestionMsg struct {
	Subject string
	Body    string
	Type    string // "feat", "fix", "docs", etc.
	Scope   string
}

// ---------------------------------------------------------------------------
// Commit selection messages
// ---------------------------------------------------------------------------
// CommitSelectedMsg is sent when the user selects a commit in the commits
// panel. Other panels can react to display commit details, diffs, etc.
type CommitSelectedMsg struct {
	Hash    string
	Subject string
}

// CommitDeselectedMsg is sent when the user deselects a commit (via Escape).
// File tree exits commit-files mode when receiving this.
type CommitDeselectedMsg struct{}

// FolderSelectedMsg is sent when a directory is selected in the file tree.
type FolderSelectedMsg struct {
	Path string
}

// WorktreeSelectedMsg is sent when a worktree is selected (without switching).
type WorktreeSelectedMsg struct {
	Path   string
	Branch string
}

// RemoteSelectedMsg is sent when a remote is selected in the gitinfo panel.
type RemoteSelectedMsg struct {
	Name string
}

// StashSelectedMsg is sent when a stash entry is selected.
type StashSelectedMsg struct {
	Hash  string
	Index int
}

// ---------------------------------------------------------------------------
// GitHub Integration Messages
// ---------------------------------------------------------------------------
// GitHubContextMsg provides the owner/repo context detected from git remote.
// Emitted on startup after parsing the git remote URL.
type GitHubContextMsg struct {
	Owner string
	Repo  string
}

// GitHubUserMsg provides the authenticated GitHub user's login.
// Emitted after successful GitHub authentication.
type GitHubUserMsg struct {
	Login string
}

// IssueSelectedMsg is sent when the user selects an issue in the GitHub Issues tab.
// Preview pane should render the issue body as markdown.
type IssueSelectedMsg struct {
	Title  string
	Body   string
	State  string
	Number int
}

// IssueDeselectedMsg is sent when the user deselects an issue (via Escape).
// Preview pane should restore to file preview.
type IssueDeselectedMsg struct{}

// IssueCommentAddedMsg is sent after a comment is added to an issue.
type IssueCommentAddedMsg struct {
	Body   string
	Number int
}

// IssueStateChangedMsg is sent when an issue is opened/closed.
type IssueStateChangedMsg struct {
	State  string
	Number int
}

// PRSelectedMsg is sent when the user selects a PR in the GitHub PRs tab.
// Files pane enters PR-files mode, commits pane shows PR commits, preview shows diff.
type PRSelectedMsg struct {
	Title      string
	State      string
	HeadBranch string
	Number     int
}

// PRDeselectedMsg is sent when the user deselects a PR (via Escape).
// Files and commits panes restore to normal mode.
type PRDeselectedMsg struct{}

// PRReviewSubmittedMsg is sent after a review is submitted on a PR.
type PRReviewSubmittedMsg struct {
	ReviewState string
	Number      int
}

// PRCommentAddedMsg is sent after a comment is added to a PR.
type PRCommentAddedMsg struct {
	Body     string
	Number   int
	ThreadID int64
}

// PRThreadResolvedMsg is sent when a review thread is resolved/unresolved.
type PRThreadResolvedMsg struct {
	Number   int
	ThreadID int64
}

// PRMergedMsg is sent after a PR is merged.
type PRMergedMsg struct {
	Strategy string
	Number   int
}

// PRMergeRequestedMsg triggers the merge strategy picker for a pull request.
type PRMergeRequestedMsg struct {
	Number     int
	Title      string
	HeadBranch string
}

// PRMergeFailedMsg is sent when a PR merge fails.
type PRMergeFailedMsg struct {
	Number int
	Err    error
}

// PRFilesLoadedMsg carries the list of files changed in a PR.
// Sent to the files pane to enter PR-files mode.
type PRFilesLoadedMsg struct {
	Files  []PRFile
	Number int
}

// PRFile represents a file changed in a pull request.
type PRFile struct {
	Filename  string
	Status    string // "added", "removed", "modified", "renamed"
	Patch     string
	Additions int
	Deletions int
}

// PRCommitsLoadedMsg carries the list of commits in a PR.
// Sent to the commits pane to show PR-specific commits.
type PRCommitsLoadedMsg struct {
	Commits []PRCommit
	Number  int
}

// PRCommit represents a commit in a pull request.
type PRCommit struct {
	SHA     string
	Message string
	Author  string
	Date    string
}

// ActionRunSelectedMsg is sent when the user selects a workflow run in the Actions tab.
// Preview pane should show job steps and logs.
type ActionRunSelectedMsg struct {
	WorkflowName string
	Status       string
	RunID        int64
}

// ActionRunDeselectedMsg is sent when the user deselects a workflow run (via Escape).
type ActionRunDeselectedMsg struct{}

// WorkflowSelectedMsg is sent when the user selects a workflow definition in the
// Workflows tab. Preview pane should show the workflow file contents.
type WorkflowSelectedMsg struct {
	Name string // e.g. "CI"
	Path string // e.g. ".github/workflows/ci.yml"
}

// ActionJobsLoadedMsg carries the list of jobs for a workflow run.
type ActionJobsLoadedMsg struct {
	Jobs  []ActionJob
	RunID int64
}

// ActionStep represents a single step within a workflow job.
type ActionStep struct {
	Name       string
	Status     string
	Conclusion string
	Number     int64
}

// ActionJob represents a job in a workflow run.
type ActionJob struct {
	Name        string
	Status      string
	Conclusion  string
	StartedAt   string
	CompletedAt string
	Steps       []ActionStep
	ID          int64
}

// ActionLogMsg carries log output for a specific job.
type ActionLogMsg struct {
	Log   string
	RunID int64
	JobID int64
}

// GitHubFilterChangedMsg is sent when the user cycles through quick filters.
type GitHubFilterChangedMsg struct {
	Tab    string // "issues", "prs", "actions"
	Filter string // filter value (e.g., "all", "assigned", "needs_review")
}

// ---------------------------------------------------------------------------
// CRUD action messages
//
// These are dispatched by the root model when the keymap resolves a CRUD
// action (item_create, item_delete, etc.). Each focused panel interprets
// the message according to its own context (e.g. "create" means "new branch"
// in the branches panel, "new file" in the filetree).
// ---------------------------------------------------------------------------
// ItemCreateMsg requests the focused panel to create a new item.
type ItemCreateMsg struct{}

// ItemDeleteMsg requests the focused panel to delete the selected item.
type ItemDeleteMsg struct{}

// ItemEditMsg requests the focused panel to edit/rename the selected item.
type ItemEditMsg struct{}

// ItemOpenMsg requests the focused panel to open the selected item
// externally (e.g. in a browser for GitHub resources, in an editor for files).
type ItemOpenMsg struct{}

// ItemCopyMsg requests the focused panel to copy the selected item's
// identifier (hash, URL, path, name) to the clipboard.
type ItemCopyMsg struct{}

// ---------------------------------------------------------------------------
// Inline editor messages
// ---------------------------------------------------------------------------

// FileModifiedMsg is broadcast after a file is written to disk by the
// inline editor. Panels that display file content or git status should
// refresh their state for the affected path.
type FileModifiedMsg struct {
	Path string
}

// EditModeEnteredMsg notifies other panels that the preview panel
// entered inline edit mode for a file.
type EditModeEnteredMsg struct {
	Path string
}

// EditModeExitedMsg notifies other panels that the preview panel
// exited inline edit mode.
type EditModeExitedMsg struct {
	Path string
}

// PreviewInputStartedMsg notifies the app that the preview panel opened an
// inline text prompt (for example, the go-to-line prompt). While a prompt is
// open the app routes all key presses directly to the preview so digits and
// Enter/Esc are not intercepted by global bindings.
type PreviewInputStartedMsg struct{}

// PreviewInputEndedMsg notifies the app that the preview panel closed its
// inline text prompt, so normal key routing should resume.
type PreviewInputEndedMsg struct{}

// ---------------------------------------------------------------------------
// Overlay panel messages
// ---------------------------------------------------------------------------
// These message types were extracted from the concrete overlay panel packages
// (settings, welcome) so that the root TUI model can type-switch on them
// without importing those packages directly.

// ToggleSettingsMsg requests showing or hiding the settings overlay.
type ToggleSettingsMsg struct{}

// SetPreviewPositionMsg is emitted when the user selects a preview position.
// Position is stored as int to avoid a circular dependency between the panels
// and layout packages.
type SetPreviewPositionMsg struct {
	Position int
}

// SetThemeMsg is emitted when the user selects a theme.
type SetThemeMsg struct {
	Name string
}

// SetDoubleClickActionMsg is emitted when the user changes a double-click action.
type SetDoubleClickActionMsg struct {
	ItemType string
	Action   string
}

// SetRightClickActionMsg is emitted when the user changes a right-click action.
type SetRightClickActionMsg struct {
	ItemType string
	Action   string
}

// ResetActionPromptsMsg is emitted when the user resets all action confirmations.
type ResetActionPromptsMsg struct{}

// WelcomeDismissMsg is sent when the user dismisses the welcome screen.
// The first-run marker is always persisted; users can re-show with W.
type WelcomeDismissMsg struct{}

// WelcomeAnimTickMsg advances the welcome banner animation by one frame.
type WelcomeAnimTickMsg time.Time
