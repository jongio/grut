package chat

// ---------------------------------------------------------------------------
// String constants extracted to satisfy goconst. Grouped by category.
// ---------------------------------------------------------------------------

// Chat roles used in ai.ChatMessage.Role fields.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Status labels displayed in the chat footer.
const (
	StatusReady = "Ready"
)

// Tool names — file operations.
const (
	ToolFileRead   = "file_read"
	ToolFileWrite  = "file_write"
	ToolFileDelete = "file_delete"
	ToolFileRename = "file_rename"
	ToolFileList   = "file_list"
	ToolFileMkdir  = "file_mkdir"
)

// Tool names — git read operations.
const (
	ToolGitStatus       = "git_status"
	ToolGitDiff         = "git_diff"
	ToolGitLog          = "git_log"
	ToolGitBlame        = "git_blame"
	ToolGitBranchList   = "git_branch_list"
	ToolGitStashList    = "git_stash_list"
	ToolGitWorktreeList = "git_worktree_list"
)

// Tool names — git write operations.
const (
	ToolGitStage        = "git_stage"
	ToolGitUnstage      = "git_unstage"
	ToolGitCommit       = "git_commit"
	ToolGitPush         = "git_push"
	ToolGitPull         = "git_pull"
	ToolGitFetch        = "git_fetch"
	ToolGitCheckout     = "git_checkout"
	ToolGitBranchCreate = "git_branch_create"
	ToolGitBranchDelete = "git_branch_delete"
	ToolGitMerge        = "git_merge"
	ToolGitRebase       = "git_rebase"
	ToolGitStashPush    = "git_stash_push"
	ToolGitStashPop     = "git_stash_pop"
	ToolGitReset        = "git_reset"
	ToolGitTagCreate    = "git_tag_create"
	ToolGitTagDelete    = "git_tag_delete"
	ToolGitDiscard      = "git_discard"
)

// Tool names — navigation & search.
const (
	ToolNavigateTo    = "navigate_to"
	ToolSearchFiles   = "search_files"
	ToolSearchContent = "search_content"
	ToolExplain       = "explain"
)

// Tool names — bulk operations.
const (
	ToolBulkStage  = "bulk_stage"
	ToolBulkDelete = "bulk_delete"
	ToolBulkRename = "bulk_rename"
)

// Tool names — GitHub operations.
const (
	ToolGHIssues       = "gh_issues"
	ToolGHIssueView    = "gh_issue_view"
	ToolGHPRs          = "gh_prs"
	ToolGHPRView       = "gh_pr_view"
	ToolGHPRDiff       = "gh_pr_diff"
	ToolGHActions      = "gh_actions"
	ToolGHActionsLogs  = "gh_actions_logs"
	ToolGHComment      = "gh_comment"
	ToolGHPRReview     = "gh_pr_review"
	ToolGHActionsRerun = "gh_actions_rerun"
)

// JSON Schema property keys used in tool parameter definitions.
const (
	PropPath       = "path"
	PropContent    = "content"
	PropOldPath    = "old_path"
	PropNewPath    = "new_path"
	PropPaths      = "paths"
	PropMessage    = "message"
	PropRemote     = "remote"
	PropForce      = "force"
	PropRef        = "ref"
	PropName       = "name"
	PropBranch     = "branch"
	PropOnto       = "onto"
	PropPattern    = "pattern"
	PropTopic      = "topic"
	PropPatterns   = "patterns"
	PropRenames    = "renames"
	PropOld        = "old"
	PropNew        = "new"
	PropLimit      = "limit"
	PropNumber     = "number"
	PropRunID      = "run_id"
	PropBody       = "body"
	PropAction     = "action"
	PropQuery      = "query"
	PropState      = "state"
	PropStatus     = "status"
	PropStartPoint = "start_point"
	PropRecursive  = "recursive"
	PropStagged    = "staged"
	PropCount      = "count"
	PropHard       = "hard"
	PropIndex      = "index"
)

// JSON Schema type and meta-property constants.
const (
	SchemaType        = "type"
	SchemaObject      = "object"
	SchemaString      = "string"
	SchemaProperties  = "properties"
	SchemaDescription = "description"
	SchemaItems       = "items"
)

// GitHub CLI flag constants.
const (
	GHFlagLimit = "--limit"
	GHFlagJSON  = "--json"
	GHSubcmdList = "list"
)

// Review action constants.
const (
	ReviewApprove        = "approve"
	ReviewRequestChanges = "request-changes"
	ReviewComment        = "comment"
)

// Environment variable constants.
const (
	EnvAWSSessionToken = "AWS_SESSION_TOKEN"
)
