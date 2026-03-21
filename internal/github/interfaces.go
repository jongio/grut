package github

import (
	"context"

	gh "github.com/google/go-github/v68/github"
)

// IssueReader provides read-only access to GitHub issues.
type IssueReader interface {
	ListIssues(ctx context.Context, owner, repo string, opts *gh.IssueListByRepoOptions) ([]*gh.Issue, error)
	GetIssue(ctx context.Context, owner, repo string, number int) (*gh.Issue, error)
	GetIssueComments(ctx context.Context, owner, repo string, number int) ([]*gh.IssueComment, error)
}

// IssueWriter provides mutation operations on GitHub issues.
type IssueWriter interface {
	CreateIssue(ctx context.Context, owner, repo string, req *gh.IssueRequest) (*gh.Issue, error)
	EditIssue(ctx context.Context, owner, repo string, number int, req *gh.IssueRequest) error
	CommentOnIssue(ctx context.Context, owner, repo string, number int, body string) error
	CloseIssue(ctx context.Context, owner, repo string, number int) error
	ReopenIssue(ctx context.Context, owner, repo string, number int) error
}

// PRReader provides read-only access to GitHub pull requests.
type PRReader interface {
	ListPRs(ctx context.Context, owner, repo string, opts *gh.PullRequestListOptions) ([]*gh.PullRequest, error)
	GetPR(ctx context.Context, owner, repo string, number int) (*gh.PullRequest, error)
	GetPRFiles(ctx context.Context, owner, repo string, number int) ([]*gh.CommitFile, error)
	GetPRComments(ctx context.Context, owner, repo string, number int) ([]*gh.PullRequestComment, error)
	GetPRReviews(ctx context.Context, owner, repo string, number int) ([]*gh.PullRequestReview, error)
	GetPRDiff(ctx context.Context, owner, repo string, number int) (string, error)
	GetPRCommits(ctx context.Context, owner, repo string, number int) ([]*gh.RepositoryCommit, error)
}

// PRWriter provides mutation operations on GitHub pull requests.
type PRWriter interface {
	CreatePR(ctx context.Context, owner, repo string, req *gh.NewPullRequest) (*gh.PullRequest, error)
	MergePR(ctx context.Context, owner, repo string, number int, msg string, opts *gh.PullRequestOptions) error
	DeleteBranch(ctx context.Context, owner, repo, branch string) error
	CommentOnPR(ctx context.Context, owner, repo string, number int, body string, path string, line int) error
	SubmitReview(ctx context.Context, owner, repo string, number int, review *gh.PullRequestReviewRequest) error
	RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error
}

// ActionReader provides read-only access to GitHub Actions workflow runs and jobs.
type ActionReader interface {
	ListWorkflows(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.Workflow, error)
	ListWorkflowRuns(ctx context.Context, owner, repo string, opts *gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRun, error)
	GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*gh.WorkflowRun, error)
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]*gh.WorkflowJob, error)
	GetJobLogs(ctx context.Context, owner, repo string, jobID int64) (string, error)
	GetWorkflowInputs(ctx context.Context, owner, repo, path, ref string) ([]WorkflowInput, error)
}

// ActionWriter provides mutation operations on GitHub Actions workflow runs.
type ActionWriter interface {
	RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error
	RerunWorkflow(ctx context.Context, owner, repo string, runID int64) error
	CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error
	DispatchWorkflow(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]any) error
}

// ReleaseReader provides read-only access to GitHub releases.
type ReleaseReader interface {
	ListReleases(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.RepositoryRelease, error)
	GetRelease(ctx context.Context, owner, repo string, id int64) (*gh.RepositoryRelease, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*gh.RepositoryRelease, error)
}

// NotificationReader provides access to GitHub notifications.
type NotificationReader interface {
	ListNotifications(ctx context.Context, opts *gh.NotificationListOptions) ([]*gh.Notification, error)
	MarkRead(ctx context.Context, threadID string) error
}

// Client composes all GitHub operation interfaces into a single client.
type Client interface {
	IssueReader
	IssueWriter
	PRReader
	PRWriter
	ActionReader
	ActionWriter
	ReleaseReader
	NotificationReader
	RepoInfo(ctx context.Context, owner, repo string) (*gh.Repository, error)
	CurrentUser(ctx context.Context) (*gh.User, error)
}
