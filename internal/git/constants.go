package git

// Git ref constants.
const refHEAD = "HEAD"

// Git command/subcommand constants.
const (
	cmdShow     = "show"
	cmdCheckout = "checkout"
	cmdReset    = "reset"
	cmdRevert   = "revert"
)

// Git object type constants.
const (
	objCommit = "commit"
	objBranch = "branch"
)

// Undo action type constants.
const (
	actionStage   = "stage"
	actionUnstage = "unstage"
	actionAmend   = "amend"
)
