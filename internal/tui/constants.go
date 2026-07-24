package tui

// Action name constants used in handleAction dispatching and AsyncOpDoneMsg descriptions.
const (
	actionPush       = "push"
	actionFetch      = "fetch"
	actionNavBack    = "nav_back"
	actionNavForward = "nav_forward"
)

const pendingActionCustom = "custom_action"

const (
	panelFileTree = "filetree"
	panelBranches = "branches"
	panelCommits  = "commits"
	panelGitHub   = "github"
)

const (
	navKindFile     = "file"
	navKindCommit   = pendingActionCommit
	navKindBranch   = "branch"
	navKindIssue    = "issue"
	navKindPR       = "pr"
	navKindWorkflow = "workflow"
)

// Keyboard hint strings shown in the status bar.
const (
	hintFind     = "/:find"
	hintTabFocus = "Tab:focus"
	hintHelp     = "?:help"
	hintScroll   = "j/k:scroll"
)

// metaKeyMessage is the metadata key used when recording undo actions.
const metaKeyMessage = "message"

// metaKeyHash is the metadata key used to record the reverted commit hash so
// the revert can be undone or redone.
const metaKeyHash = "hash"
