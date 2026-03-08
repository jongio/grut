package github

// IssueFilter defines filter options for listing issues.
type IssueFilter string

const (
	IssueFilterAll       IssueFilter = "all"
	IssueFilterAssigned  IssueFilter = "assigned"
	IssueFilterMentioned IssueFilter = "mentioned"
	IssueFilterCreated   IssueFilter = "created"
)

// PRFilter defines filter options for listing pull requests.
type PRFilter string

const (
	PRFilterAll         PRFilter = "all"
	PRFilterNeedsReview PRFilter = "needs_review"
	PRFilterMine        PRFilter = "mine"
	PRFilterDraft       PRFilter = "draft"
)

// ActionStatus represents the conclusion or status of a GitHub Actions workflow run.
type ActionStatus string

const (
	ActionStatusSuccess    ActionStatus = "success"
	ActionStatusFailure    ActionStatus = "failure"
	ActionStatusInProgress ActionStatus = "in_progress"
	ActionStatusQueued     ActionStatus = "queued"
	ActionStatusCancelled  ActionStatus = "cancelled"
)
