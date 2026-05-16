package shortcuts

// Parameter and placeholder names used in shortcut step definitions.
const (
	paramAll         = "all"
	paramAIMessage   = "ai_message"
	paramAmend       = "amend"
	paramBase        = "base"
	paramBranch      = "branch"
	paramFixup       = "fixup"
	paramForce       = "force"
	paramMerged      = "merged"
	paramMessage     = "message"
	paramMode        = "mode"
	paramName        = "name"
	paramNewName     = "new_name"
	paramNoFF        = "no_ff"
	paramOnto        = "onto"
	paramPaths       = "paths"
	paramPrune       = "prune"
	paramRebase      = "rebase"
	paramRef         = "ref"
	paramRemote      = "remote"
	paramSetUpstream = "set_upstream"
	paramSquash      = "squash"
	paramTarget      = "target"
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

// Boolean string value used in Params maps.
const valTrue = "true"

// Default branch name.
const branchMain = "main"

// Protected branch names excluded from bulk deletion.
const (
	protectedDevelop = "develop"
	protectedMaster  = "master"
)
