package shortcuts

// Parameter and placeholder names used in shortcut step definitions.
const (
	paramAll       = "all"
	paramAIMessage = "ai_message"
	paramAmend     = "amend"
	paramBase      = "base"
	paramBranch    = "branch"
	paramFixup     = "fixup"
	paramMerged    = "merged"
	paramMessage   = "message"
	paramName      = "name"
	paramNewName   = "new_name"
	paramOnto      = "onto"
	paramPaths     = "paths"
	paramRebase    = "rebase"
	paramRemote    = "remote"
	paramSquash    = "squash"
	paramTarget    = "target"
)

// Git refs used in shortcut definitions.
const (
	refHead       = "HEAD"
	refHeadTilde1 = "HEAD~1"
	refOrigin     = "origin"
	refPrevBranch = "@{-1}"
)

// Placeholder templates for argument substitution in shortcut steps.
const (
	placeholderRemote = "{{remote}}"
	placeholderTarget = "{{target}}"
)

// Shortcut and action names referenced across multiple definitions.
const (
	actionPull    = "pull"
	actionUnstage = "unstage"
	actionWip     = "wip"
)

// Git reset modes.
const (
	resetModeHard  = "hard"
	resetModeMixed = "mixed"
	resetModeSoft  = "soft"
)

// Protected branch names excluded from bulk deletion.
const (
	protectedDevelop = "develop"
)
