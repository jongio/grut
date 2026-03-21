// Package shortcuts provides AI-powered git workflow shortcut definitions
// and an execution engine. Shortcuts are concise aliases for multi-step git
// operations that execute through the AI git client middleware.
package shortcuts

// Shortcut defines a named multi-step git workflow.
type Shortcut struct {
	Name        string
	Description string
	Steps       []Step
	Args        []Arg
	Builtin     bool
	Confirm     bool // default true
}

// Step represents a single operation within a shortcut workflow.
type Step struct {
	Op       string            // git operation name (e.g. "stage", "commit", "push")
	Params   map[string]string // operation parameters
	OnFail   string            // "stop" | "continue" | "ask" (default "stop")
	AIAssist bool              // use AI assistance for this step (default true)
}

// Arg defines a user-supplied argument for a shortcut.
type Arg struct {
	Name     string
	Default  string
	Prompt   string
	Required bool
}

// StepResult captures the outcome of executing a single step.
type StepResult struct {
	Err     error
	Output  string
	Step    Step
	Skipped bool
}

// ExecutionResult captures the outcome of a full shortcut execution.
type ExecutionResult struct {
	Err         error
	StepResults []StepResult
	Shortcut    Shortcut
}

// OnFail policy constants.
const (
	OnFailStop     = "stop"
	OnFailContinue = "continue"
	OnFailAsk      = "ask"
)

// Operation name constants used in Step.Op.
const (
	OpStage        = "stage"
	OpUnstage      = "unstage"
	OpCommit       = "commit"
	OpPush         = "push"
	OpPull         = "pull"
	OpFetch        = "fetch"
	OpRebase       = "rebase"
	OpMerge        = "merge"
	OpCheckout     = "checkout"
	OpBranch       = "branch"
	OpReset        = "reset"
	OpDelete       = "delete_branch"
	OpStash        = "stash"
	OpStashPop     = "stash_pop"
	OpBranchRename = "branch_rename"
)
