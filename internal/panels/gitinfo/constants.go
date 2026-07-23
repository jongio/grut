package gitinfo

// ---------------------------------------------------------------------------
// Shared string constants extracted to satisfy goconst.
// ---------------------------------------------------------------------------

// Theme colors (fallback defaults before theme overrides).
const (
	colorGray   = "#555555"
	colorGreen  = "#6B9E56"
	colorOrange = "#C9875A"
	colorYellow = "#C9A227"
)

// Panel identifier.
const panelGitinfo = "gitinfo"

// Section tab labels (title-case, displayed in the tab bar).
const (
	labelBranches   = "Branches"
	labelTags       = "Tags"
	labelRemotes    = "Remotes"
	labelStash      = "Stash"
	labelWorktrees  = "Worktrees"
	labelIssues     = "Issues"
	labelPRs        = "PRs"
	labelReflog     = "Reflog"
	labelSubmodules = "Submodules"
	labelActions    = "Actions"
	labelAll        = "All"

	labelNotifications = "Notifications"
)

// Section names (lowercase, used in SetActiveTab and filter messages).
const (
	sectionBranches   = "branches"
	sectionWorktrees  = "worktrees"
	sectionRemotes    = "remotes"
	sectionStash      = "stash"
	sectionTags       = "tags"
	sectionReflog     = "reflog"
	sectionSubmodules = "submodules"
	sectionIssues     = "issues"
	sectionPRs        = "prs"
	sectionActions    = "actions"

	sectionNotifications = "notifications"
)

// Git operation result keys used in opResultMsg.op.
const (
	opCheckout = "checkout"
	opFetched  = "fetched"
)

// Git event types used in opResultMsg.op switch cases.
const (
	eventBranchCreated   = "branch_created"
	eventBranchDeleted   = "branch_deleted"
	eventBranchRenamed   = "branch_renamed"
	eventRemoteAdded     = "remote_added"
	eventRemoteRemoved   = "remote_removed"
	eventStashApplied    = "stash_applied"
	eventStashDropped    = "stash_dropped"
	eventStashPopped     = "stash_popped"
	eventTagCreated      = "tag_created"
	eventTagDeleted      = "tag_deleted"
	eventTagPushed       = "tag_pushed"
	eventTagCheckout     = "tag_checkout"
	eventWorktreeAdded   = "worktree_added"
	eventWorktreeRemoved = "worktree_removed"
	eventWorktreeSwitch  = "worktree_switch"
)

// Merge strategy IDs used in the PR merge action picker.
const (
	strategyMerge  = "merge"
	strategySquash = "squash"
	strategyRebase = "rebase"
)

// Merge commit display label.
const mergeCommitLabel = "merge commit"

// PR / issue state strings.
const (
	stateDraft   = "draft"
	stateDirty   = "dirty"
	stateUnknown = "unknown"
	stateClosed  = "closed"
)

// Assignable item kind labels used in assign-to-me toasts and result messages.
const (
	assignKindIssue = "issue"
	assignKindPR    = "PR"
)

// Workflow state strings.
const (
	stateActive             = "active"
	stateDisabledInactivity = "disabled_inactivity"
	stateDisabledManually   = "disabled_manually"
)

// Stash action names (user input in modal).
const (
	actionApply = "apply"
	actionDrop  = "drop"
	actionPop   = "pop"
)

// Navigation action names (keybinding actions).
const (
	actionFirst = "first"
	actionLast  = "last"
)

// Default branch fallback.
const branchMain = "main"

// Worktree open mode.
const openModeNewTerminal = "new_terminal"

// Short tab labels (abbreviated, displayed when space is limited).
const (
	shortPRs = "PRs"
	shortAct = "Act"
)

// PR filter display label.
const labelDraft = "Draft"
