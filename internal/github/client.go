package github

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	gh "github.com/google/go-github/v68/github"
)

// clientImpl implements the Client interface using the google/go-github SDK.
type clientImpl struct {
	gh    *gh.Client
	cache *cache
}

// maxPaginationPages caps the number of GitHub API pages fetched in a single
// list call to prevent unbounded memory growth on very large repositories.
const maxPaginationPages = 100 // ~10,000 items at 100 per page

// Compile-time interface assertion.
var _ Client = (*clientImpl)(nil)

// ---------------------------------------------------------------------------
// IssueReader
// ---------------------------------------------------------------------------

func (c *clientImpl) ListIssues(ctx context.Context, owner, repo string, opts *gh.IssueListByRepoOptions) ([]*gh.Issue, error) {
	// Work on a local copy so we never mutate the caller's pointer.
	// Normalize Page=0 so the cache key is stable across calls.
	local := gh.IssueListByRepoOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("issues:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		issues, ok := v.([]*gh.Issue)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for issues")
		}
		return issues, nil
	}

	local.Page = 1
	var allIssues []*gh.Issue
	for page := 0; page < maxPaginationPages; page++ {
		issues, resp, err := c.gh.Issues.ListByRepo(ctx, owner, repo, &local)
		if err != nil {
			return nil, fmt.Errorf("list issues: %w", err)
		}
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		local.Page = resp.NextPage
	}

	c.cache.Set(key, allIssues)
	return allIssues, nil
}

// ---------------------------------------------------------------------------
// Paged cache-entry types
// ---------------------------------------------------------------------------

type issuePage struct {
	issues []*gh.Issue
	page   PageResult
}

type prPage struct {
	prs  []*gh.PullRequest
	page PageResult
}

type workflowRunPage struct {
	runs []*gh.WorkflowRun
	page PageResult
}

type workflowPage struct {
	workflows []*gh.Workflow
	page      PageResult
}

type releasePage struct {
	releases []*gh.RepositoryRelease
	page     PageResult
}

func (c *clientImpl) ListIssuesPage(ctx context.Context, owner, repo string, opts *gh.IssueListByRepoOptions) ([]*gh.Issue, PageResult, error) {
	local := gh.IssueListByRepoOptions{}
	if opts != nil {
		local = *opts
	}
	if local.Page == 0 {
		local.Page = 1
	}
	key := fmt.Sprintf("issues-page:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(issuePage)
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for issues page")
		}
		return entry.issues, entry.page, nil
	}

	issues, resp, err := c.gh.Issues.ListByRepo(ctx, owner, repo, &local)
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("list issues page: %w", err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: -1}
	c.cache.Set(key, issuePage{issues: issues, page: pr})
	return issues, pr, nil
}

func (c *clientImpl) GetIssue(ctx context.Context, owner, repo string, number int) (*gh.Issue, error) {
	key := fmt.Sprintf("issue:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		issue, ok := v.(*gh.Issue)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for issue")
		}
		return issue, nil
	}

	issue, _, err := c.gh.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
	}

	c.cache.Set(key, issue)
	return issue, nil
}

func (c *clientImpl) GetIssueComments(ctx context.Context, owner, repo string, number int) ([]*gh.IssueComment, error) {
	key := fmt.Sprintf("issue-comments:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		comments, ok := v.([]*gh.IssueComment)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for issue comments")
		}
		return comments, nil
	}

	var allComments []*gh.IssueComment
	opts := &gh.IssueListCommentsOptions{
		ListOptions: gh.ListOptions{Page: 1, PerPage: 100},
	}
	for page := 0; page < maxPaginationPages; page++ {
		comments, resp, err := c.gh.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("get issue #%d comments: %w", number, err)
		}
		allComments = append(allComments, comments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(key, allComments)
	return allComments, nil
}

// ---------------------------------------------------------------------------
// IssueWriter
// ---------------------------------------------------------------------------

func (c *clientImpl) CreateIssue(ctx context.Context, owner, repo string, req *gh.IssueRequest) (*gh.Issue, error) {
	issue, _, err := c.gh.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	c.cache.InvalidatePrefix(fmt.Sprintf("issues:%s/%s:", owner, repo))
	return issue, nil
}

func (c *clientImpl) EditIssue(ctx context.Context, owner, repo string, number int, req *gh.IssueRequest) error {
	_, _, err := c.gh.Issues.Edit(ctx, owner, repo, number, req)
	if err != nil {
		return fmt.Errorf("edit issue #%d: %w", number, err)
	}
	c.cache.Invalidate(fmt.Sprintf("issue:%s/%s:%d", owner, repo, number))
	return nil
}

func (c *clientImpl) CommentOnIssue(ctx context.Context, owner, repo string, number int, body string) error {
	comment := &gh.IssueComment{Body: gh.Ptr(body)}
	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, comment)
	if err != nil {
		return fmt.Errorf("comment on issue #%d: %w", number, err)
	}
	c.cache.Invalidate(fmt.Sprintf("issue-comments:%s/%s:%d", owner, repo, number))
	return nil
}

func (c *clientImpl) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	state := "closed"
	req := &gh.IssueRequest{State: &state}
	return c.EditIssue(ctx, owner, repo, number, req)
}

func (c *clientImpl) ReopenIssue(ctx context.Context, owner, repo string, number int) error {
	state := "open"
	req := &gh.IssueRequest{State: &state}
	return c.EditIssue(ctx, owner, repo, number, req)
}

// ---------------------------------------------------------------------------
// PRReader
// ---------------------------------------------------------------------------

func (c *clientImpl) ListPRs(ctx context.Context, owner, repo string, opts *gh.PullRequestListOptions) ([]*gh.PullRequest, error) {
	local := gh.PullRequestListOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("prs:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		prs, ok := v.([]*gh.PullRequest)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PRs")
		}
		return prs, nil
	}

	local.Page = 1
	var allPRs []*gh.PullRequest
	for page := 0; page < maxPaginationPages; page++ {
		prs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, &local)
		if err != nil {
			return nil, fmt.Errorf("list PRs: %w", err)
		}
		allPRs = append(allPRs, prs...)
		if resp.NextPage == 0 {
			break
		}
		local.Page = resp.NextPage
	}

	c.cache.Set(key, allPRs)
	return allPRs, nil
}

func (c *clientImpl) ListPRsPage(ctx context.Context, owner, repo string, opts *gh.PullRequestListOptions) ([]*gh.PullRequest, PageResult, error) {
	local := gh.PullRequestListOptions{}
	if opts != nil {
		local = *opts
	}
	if local.Page == 0 {
		local.Page = 1
	}
	key := fmt.Sprintf("prs-page:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(prPage)
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for PRs page")
		}
		return entry.prs, entry.page, nil
	}

	prs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, &local)
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("list PRs page: %w", err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: -1}
	c.cache.Set(key, prPage{prs: prs, page: pr})
	return prs, pr, nil
}

func (c *clientImpl) GetPR(ctx context.Context, owner, repo string, number int) (*gh.PullRequest, error) {
	key := fmt.Sprintf("pr:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		pr, ok := v.(*gh.PullRequest)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PR")
		}
		return pr, nil
	}

	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}

	c.cache.Set(key, pr)
	return pr, nil
}

func (c *clientImpl) GetPRFiles(ctx context.Context, owner, repo string, number int) ([]*gh.CommitFile, error) {
	key := fmt.Sprintf("pr-files:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		files, ok := v.([]*gh.CommitFile)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PR files")
		}
		return files, nil
	}

	var allFiles []*gh.CommitFile
	opts := &gh.ListOptions{Page: 1, PerPage: 100}
	for page := 0; page < maxPaginationPages; page++ {
		files, resp, err := c.gh.PullRequests.ListFiles(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("get PR #%d files: %w", number, err)
		}
		allFiles = append(allFiles, files...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(key, allFiles)
	return allFiles, nil
}

func (c *clientImpl) GetPRComments(ctx context.Context, owner, repo string, number int) ([]*gh.PullRequestComment, error) {
	key := fmt.Sprintf("pr-comments:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		comments, ok := v.([]*gh.PullRequestComment)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PR comments")
		}
		return comments, nil
	}

	var allComments []*gh.PullRequestComment
	opts := &gh.PullRequestListCommentsOptions{
		ListOptions: gh.ListOptions{Page: 1, PerPage: 100},
	}
	for page := 0; page < maxPaginationPages; page++ {
		comments, resp, err := c.gh.PullRequests.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("get PR #%d comments: %w", number, err)
		}
		allComments = append(allComments, comments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(key, allComments)
	return allComments, nil
}

func (c *clientImpl) GetPRReviews(ctx context.Context, owner, repo string, number int) ([]*gh.PullRequestReview, error) {
	key := fmt.Sprintf("pr-reviews:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		reviews, ok := v.([]*gh.PullRequestReview)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PR reviews")
		}
		return reviews, nil
	}

	var allReviews []*gh.PullRequestReview
	opts := &gh.ListOptions{Page: 1, PerPage: 100}
	for page := 0; page < maxPaginationPages; page++ {
		reviews, resp, err := c.gh.PullRequests.ListReviews(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("get PR #%d reviews: %w", number, err)
		}
		allReviews = append(allReviews, reviews...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(key, allReviews)
	return allReviews, nil
}

func (c *clientImpl) GetPRDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	key := fmt.Sprintf("pr-diff:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		diff, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("unexpected cache type for PR diff")
		}
		return diff, nil
	}

	diff, _, err := c.gh.PullRequests.GetRaw(ctx, owner, repo, number, gh.RawOptions{Type: gh.Diff})
	if err != nil {
		return "", fmt.Errorf("get PR #%d diff: %w", number, err)
	}

	c.cache.Set(key, diff)
	return diff, nil
}

func (c *clientImpl) GetPRCommits(ctx context.Context, owner, repo string, number int) ([]*gh.RepositoryCommit, error) {
	key := fmt.Sprintf("pr-commits:%s/%s:%d", owner, repo, number)
	if v, ok := c.cache.Get(key); ok {
		commits, ok := v.([]*gh.RepositoryCommit)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for PR commits")
		}
		return commits, nil
	}

	var allCommits []*gh.RepositoryCommit
	opts := &gh.ListOptions{Page: 1, PerPage: 100}
	for page := 0; page < maxPaginationPages; page++ {
		commits, resp, err := c.gh.PullRequests.ListCommits(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("get PR #%d commits: %w", number, err)
		}
		allCommits = append(allCommits, commits...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.cache.Set(key, allCommits)
	return allCommits, nil
}

// ---------------------------------------------------------------------------
// PRWriter
// ---------------------------------------------------------------------------

func (c *clientImpl) CreatePR(ctx context.Context, owner, repo string, req *gh.NewPullRequest) (*gh.PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}
	c.cache.InvalidatePrefix(fmt.Sprintf("prs:%s/%s:", owner, repo))
	return pr, nil
}

func (c *clientImpl) MergePR(ctx context.Context, owner, repo string, number int, msg string, opts *gh.PullRequestOptions) error {
	_, _, err := c.gh.PullRequests.Merge(ctx, owner, repo, number, msg, opts)
	if err != nil {
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	c.cache.Invalidate(fmt.Sprintf("pr:%s/%s:%d", owner, repo, number))
	c.cache.InvalidatePrefix(fmt.Sprintf("prs:%s/%s:", owner, repo))
	return nil
}

func (c *clientImpl) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	_, err := c.gh.Git.DeleteRef(ctx, owner, repo, "heads/"+branch)
	if err != nil {
		return fmt.Errorf("delete branch %q: %w", branch, err)
	}
	return nil
}

func (c *clientImpl) CommentOnPR(ctx context.Context, owner, repo string, number int, body string, path string, line int) error {
	comment := &gh.PullRequestComment{
		Body: gh.Ptr(body),
		Path: gh.Ptr(path),
		Line: gh.Ptr(line),
	}
	_, _, err := c.gh.PullRequests.CreateComment(ctx, owner, repo, number, comment)
	if err != nil {
		return fmt.Errorf("comment on PR #%d: %w", number, err)
	}
	c.cache.Invalidate(fmt.Sprintf("pr-comments:%s/%s:%d", owner, repo, number))
	return nil
}

func (c *clientImpl) SubmitReview(ctx context.Context, owner, repo string, number int, review *gh.PullRequestReviewRequest) error {
	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review)
	if err != nil {
		return fmt.Errorf("submit review on PR #%d: %w", number, err)
	}
	c.cache.Invalidate(fmt.Sprintf("pr-reviews:%s/%s:%d", owner, repo, number))
	return nil
}

func (c *clientImpl) RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error {
	req := gh.ReviewersRequest{Reviewers: reviewers}
	_, _, err := c.gh.PullRequests.RequestReviewers(ctx, owner, repo, number, req)
	if err != nil {
		return fmt.Errorf("request reviewers on PR #%d: %w", number, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ActionReader
// ---------------------------------------------------------------------------

func (c *clientImpl) ListWorkflowRuns(ctx context.Context, owner, repo string, opts *gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRun, error) {
	local := gh.ListWorkflowRunsOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("runs:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		runs, ok := v.([]*gh.WorkflowRun)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for workflow runs")
		}
		return runs, nil
	}

	result, _, err := c.gh.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &local)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}

	runs := result.WorkflowRuns
	c.cache.Set(key, runs)
	return runs, nil
}

func (c *clientImpl) ListWorkflowRunsPage(ctx context.Context, owner, repo string, opts *gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRun, PageResult, error) {
	local := gh.ListWorkflowRunsOptions{}
	if opts != nil {
		local = *opts
	}
	if local.Page == 0 {
		local.Page = 1
	}
	key := fmt.Sprintf("runs-page:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(workflowRunPage)
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for workflow runs page")
		}
		return entry.runs, entry.page, nil
	}

	result, resp, err := c.gh.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &local)
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("list workflow runs page: %w", err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: result.GetTotalCount()}
	c.cache.Set(key, workflowRunPage{runs: result.WorkflowRuns, page: pr})
	return result.WorkflowRuns, pr, nil
}

func (c *clientImpl) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*gh.WorkflowRun, error) {
	key := fmt.Sprintf("run:%s/%s:%d", owner, repo, runID)
	if v, ok := c.cache.Get(key); ok {
		run, ok := v.(*gh.WorkflowRun)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for workflow run")
		}
		return run, nil
	}

	run, _, err := c.gh.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run %d: %w", runID, err)
	}

	c.cache.Set(key, run)
	return run, nil
}

func (c *clientImpl) ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]*gh.WorkflowJob, error) {
	key := fmt.Sprintf("jobs:%s/%s:%d", owner, repo, runID)
	if v, ok := c.cache.Get(key); ok {
		jobs, ok := v.([]*gh.WorkflowJob)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for workflow jobs")
		}
		return jobs, nil
	}

	result, _, err := c.gh.Actions.ListWorkflowJobs(ctx, owner, repo, runID, &gh.ListWorkflowJobsOptions{
		Filter: "latest",
	})
	if err != nil {
		return nil, fmt.Errorf("list workflow jobs for run %d: %w", runID, err)
	}

	jobs := result.Jobs
	c.cache.Set(key, jobs)
	return jobs, nil
}

func (c *clientImpl) GetJobLogs(ctx context.Context, owner, repo string, jobID int64) (string, error) {
	key := fmt.Sprintf("job-logs:%s/%s:%d", owner, repo, jobID)
	if v, ok := c.cache.Get(key); ok {
		logs, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("unexpected cache type for job logs")
		}
		return logs, nil
	}

	url, _, err := c.gh.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 2)
	if err != nil {
		return "", fmt.Errorf("get job %d logs URL: %w", jobID, err)
	}
	if url == nil {
		return "", fmt.Errorf("get job %d logs: API returned nil URL", jobID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil) //nolint:gosec // URL comes from GitHub API
	if err != nil {
		return "", fmt.Errorf("build job %d logs request: %w", jobID, err)
	}
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-HTTPS URL: %s", r.URL)
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch job %d logs: %w", jobID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	const maxLogSize = 10 << 20 // 10 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLogSize))
	if err != nil {
		return "", fmt.Errorf("read job %d logs: %w", jobID, err)
	}

	logs := string(body)
	c.cache.Set(key, logs)
	return logs, nil
}

// ---------------------------------------------------------------------------
// ActionWriter
// ---------------------------------------------------------------------------

func (c *clientImpl) RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error {
	_, err := c.gh.Actions.RerunFailedJobsByID(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("rerun failed jobs for run %d: %w", runID, err)
	}
	c.cache.Invalidate(fmt.Sprintf("run:%s/%s:%d", owner, repo, runID))
	c.cache.Invalidate(fmt.Sprintf("jobs:%s/%s:%d", owner, repo, runID))
	return nil
}

func (c *clientImpl) CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error {
	_, err := c.gh.Actions.CancelWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("cancel workflow run %d: %w", runID, err)
	}
	c.cache.Invalidate(fmt.Sprintf("run:%s/%s:%d", owner, repo, runID))
	return nil
}

func (c *clientImpl) RerunWorkflow(ctx context.Context, owner, repo string, runID int64) error {
	_, err := c.gh.Actions.RerunWorkflowByID(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("rerun workflow run %d: %w", runID, err)
	}
	c.cache.Invalidate(fmt.Sprintf("run:%s/%s:%d", owner, repo, runID))
	c.cache.Invalidate(fmt.Sprintf("jobs:%s/%s:%d", owner, repo, runID))
	return nil
}

func (c *clientImpl) DispatchWorkflow(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]any) error {
	event := gh.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: inputs,
	}
	resp, err := c.gh.Actions.CreateWorkflowDispatchEventByID(ctx, owner, repo, workflowID, event)
	if err != nil {
		// Surface specific auth scope issues clearly.
		if resp != nil && resp.StatusCode == 403 {
			slog.Error("workflow dispatch forbidden — token may lack 'workflow' scope",
				"workflow", workflowID, "ref", ref, "status", resp.StatusCode)
			return fmt.Errorf("dispatch workflow %d: forbidden (ensure token has 'workflow' scope): %w", workflowID, err)
		}
		slog.Error("workflow dispatch failed",
			"workflow", workflowID, "ref", ref, "err", err)
		return fmt.Errorf("dispatch workflow %d: %w", workflowID, err)
	}
	slog.Debug("workflow dispatched", "workflow", workflowID, "ref", ref,
		"status", resp.StatusCode)
	// Invalidate runs cache so the new run appears on next fetch.
	c.cache.InvalidatePrefix(fmt.Sprintf("runs:%s/%s:", owner, repo))
	return nil
}

func (c *clientImpl) ListWorkflows(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.Workflow, error) {
	local := gh.ListOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("workflows:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		workflows, ok := v.([]*gh.Workflow)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for workflows")
		}
		return workflows, nil
	}

	local.Page = 1
	var allWorkflows []*gh.Workflow
	for {
		result, resp, err := c.gh.Actions.ListWorkflows(ctx, owner, repo, &local)
		if err != nil {
			return nil, fmt.Errorf("list workflows: %w", err)
		}
		allWorkflows = append(allWorkflows, result.Workflows...)
		if resp.NextPage == 0 {
			break
		}
		local.Page = resp.NextPage
	}

	c.cache.Set(key, allWorkflows)
	return allWorkflows, nil
}

func (c *clientImpl) ListWorkflowsPage(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.Workflow, PageResult, error) {
	local := gh.ListOptions{}
	if opts != nil {
		local = *opts
	}
	if local.Page == 0 {
		local.Page = 1
	}
	key := fmt.Sprintf("workflows-page:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(workflowPage)
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for workflows page")
		}
		return entry.workflows, entry.page, nil
	}

	result, resp, err := c.gh.Actions.ListWorkflows(ctx, owner, repo, &local)
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("list workflows page: %w", err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: -1}
	c.cache.Set(key, workflowPage{workflows: result.Workflows, page: pr})
	return result.Workflows, pr, nil
}

// ---------------------------------------------------------------------------
// ReleaseReader
// ---------------------------------------------------------------------------

func (c *clientImpl) ListReleases(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.RepositoryRelease, error) {
	local := gh.ListOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("releases:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		releases, ok := v.([]*gh.RepositoryRelease)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for releases")
		}
		return releases, nil
	}

	local.Page = 1
	var allReleases []*gh.RepositoryRelease
	for {
		releases, resp, err := c.gh.Repositories.ListReleases(ctx, owner, repo, &local)
		if err != nil {
			return nil, fmt.Errorf("list releases: %w", err)
		}
		allReleases = append(allReleases, releases...)
		if resp.NextPage == 0 {
			break
		}
		local.Page = resp.NextPage
	}

	c.cache.Set(key, allReleases)
	return allReleases, nil
}

func (c *clientImpl) ListReleasesPage(ctx context.Context, owner, repo string, opts *gh.ListOptions) ([]*gh.RepositoryRelease, PageResult, error) {
	local := gh.ListOptions{}
	if opts != nil {
		local = *opts
	}
	if local.Page == 0 {
		local.Page = 1
	}
	key := fmt.Sprintf("releases-page:%s/%s:%+v", owner, repo, local)
	if v, ok := c.cache.Get(key); ok {
		entry, ok := v.(releasePage)
		if !ok {
			return nil, PageResult{}, fmt.Errorf("unexpected cache type for releases page")
		}
		return entry.releases, entry.page, nil
	}

	releases, resp, err := c.gh.Repositories.ListReleases(ctx, owner, repo, &local)
	if err != nil {
		return nil, PageResult{}, fmt.Errorf("list releases page: %w", err)
	}

	pr := PageResult{NextPage: resp.NextPage, TotalCount: -1}
	c.cache.Set(key, releasePage{releases: releases, page: pr})
	return releases, pr, nil
}

func (c *clientImpl) GetRelease(ctx context.Context, owner, repo string, id int64) (*gh.RepositoryRelease, error) {
	key := fmt.Sprintf("release:%s/%s:%d", owner, repo, id)
	if v, ok := c.cache.Get(key); ok {
		release, ok := v.(*gh.RepositoryRelease)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for release")
		}
		return release, nil
	}

	release, _, err := c.gh.Repositories.GetRelease(ctx, owner, repo, id)
	if err != nil {
		return nil, fmt.Errorf("get release %d: %w", id, err)
	}

	c.cache.Set(key, release)
	return release, nil
}

func (c *clientImpl) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*gh.RepositoryRelease, error) {
	key := fmt.Sprintf("release-tag:%s/%s:%s", owner, repo, tag)
	if v, ok := c.cache.Get(key); ok {
		release, ok := v.(*gh.RepositoryRelease)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for release by tag")
		}
		return release, nil
	}

	release, _, err := c.gh.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("get release by tag %s: %w", tag, err)
	}

	c.cache.Set(key, release)
	return release, nil
}

// ---------------------------------------------------------------------------
// NotificationReader
// ---------------------------------------------------------------------------

func (c *clientImpl) ListNotifications(ctx context.Context, opts *gh.NotificationListOptions) ([]*gh.Notification, error) {
	local := gh.NotificationListOptions{}
	if opts != nil {
		local = *opts
	}
	local.Page = 0
	key := fmt.Sprintf("notifications:%+v", local)
	if v, ok := c.cache.Get(key); ok {
		notifications, ok := v.([]*gh.Notification)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for notifications")
		}
		return notifications, nil
	}

	local.Page = 1
	var allNotifications []*gh.Notification
	for {
		notifications, resp, err := c.gh.Activity.ListNotifications(ctx, &local)
		if err != nil {
			return nil, fmt.Errorf("list notifications: %w", err)
		}
		allNotifications = append(allNotifications, notifications...)
		if resp.NextPage == 0 {
			break
		}
		local.Page = resp.NextPage
	}

	c.cache.Set(key, allNotifications)
	return allNotifications, nil
}

func (c *clientImpl) MarkRead(ctx context.Context, threadID string) error {
	_, err := c.gh.Activity.MarkThreadRead(ctx, threadID)
	if err != nil {
		return fmt.Errorf("mark thread %s read: %w", threadID, err)
	}
	c.cache.InvalidateAll() // Notification list changes on mark-read
	return nil
}

// ---------------------------------------------------------------------------
// Standalone methods
// ---------------------------------------------------------------------------

func (c *clientImpl) RepoInfo(ctx context.Context, owner, repo string) (*gh.Repository, error) {
	key := fmt.Sprintf("repo:%s/%s", owner, repo)
	if v, ok := c.cache.Get(key); ok {
		repository, ok := v.(*gh.Repository)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for repository")
		}
		return repository, nil
	}

	repository, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo %s/%s: %w", owner, repo, err)
	}

	c.cache.Set(key, repository)
	return repository, nil
}

func (c *clientImpl) CurrentUser(ctx context.Context) (*gh.User, error) {
	const key = "current-user"
	if v, ok := c.cache.Get(key); ok {
		user, ok := v.(*gh.User)
		if !ok {
			return nil, fmt.Errorf("unexpected cache type for user")
		}
		return user, nil
	}

	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	c.cache.Set(key, user)
	return user, nil
}
