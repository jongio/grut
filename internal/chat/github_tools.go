package chat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// maxGitHubOutputChars caps the truncation length for GitHub CLI output
// included in AI tool responses.
const maxGitHubOutputChars = 10000

// ---------------------------------------------------------------------------
// GitHub tool registrations
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerGitHubTools() {
	// ── Read-only (Safe) ────────────────────────────────────────────

	r.register("gh_issues",
		"List or search GitHub issues in this repository",
		Safe,
		objectSchema(map[string]any{
			"query": stringProp("Search query to filter issues"),
			"state": stringProp("Issue state: open, closed, or all (default open)"),
			"limit": intProp("Maximum number of issues to return (default 20)"),
		}, nil),
	)

	r.register("gh_issue_view",
		"View a GitHub issue with its body and comments",
		Safe,
		objectSchema(map[string]any{
			"number": intProp("Issue number"),
		}, []string{"number"}),
	)

	r.register("gh_prs",
		"List or search GitHub pull requests in this repository",
		Safe,
		objectSchema(map[string]any{
			"query": stringProp("Search query to filter pull requests"),
			"state": stringProp("PR state: open, closed, merged, or all (default open)"),
			"limit": intProp("Maximum number of PRs to return (default 20)"),
		}, nil),
	)

	r.register("gh_pr_view",
		"View a GitHub pull request with reviews, comments, and files",
		Safe,
		objectSchema(map[string]any{
			"number": intProp("Pull request number"),
		}, []string{"number"}),
	)

	r.register("gh_pr_diff",
		"Get the diff of a GitHub pull request",
		Safe,
		objectSchema(map[string]any{
			"number": intProp("Pull request number"),
		}, []string{"number"}),
	)

	r.register("gh_actions",
		"List recent GitHub Actions workflow runs",
		Safe,
		objectSchema(map[string]any{
			"branch": stringProp("Filter runs by branch name"),
			"status": stringProp("Filter by status: completed, in_progress, or queued"),
			"limit":  intProp("Maximum number of runs to return (default 10)"),
		}, nil),
	)

	r.register("gh_actions_logs",
		"Get failed job logs for a GitHub Actions workflow run",
		Safe,
		objectSchema(map[string]any{
			"run_id": intProp("Workflow run ID"),
		}, []string{"run_id"}),
	)

	// ── Write (Destructive) ─────────────────────────────────────────

	r.register("gh_comment",
		"Post a comment on a GitHub issue or pull request",
		Destructive,
		objectSchema(map[string]any{
			"number": intProp("Issue or PR number"),
			"body":   stringProp("Comment body text"),
		}, []string{"number", "body"}),
	)

	r.register("gh_pr_review",
		"Submit a review on a GitHub pull request",
		Destructive,
		objectSchema(map[string]any{
			"number": intProp("Pull request number"),
			"body":   stringProp("Review body text"),
			"action": stringProp("Review action: approve, request-changes, or comment"),
		}, []string{"number", "body", "action"}),
	)

	r.register("gh_actions_rerun",
		"Rerun failed jobs in a GitHub Actions workflow run",
		Destructive,
		objectSchema(map[string]any{
			"run_id": intProp("Workflow run ID"),
		}, []string{"run_id"}),
	)
}

// ---------------------------------------------------------------------------
// GitHub CLI helper
// ---------------------------------------------------------------------------

// ghExec runs a gh CLI command in the repository root and returns its output.
// The child process inherits a filtered environment that excludes known secret
// variables unrelated to GitHub CLI operation.
func (e *ToolExecutor) ghExec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = e.jail.Root()
	cmd.Env = filterEnvForGH()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ---------------------------------------------------------------------------
// GitHub tool handlers — read-only
// ---------------------------------------------------------------------------

func (e *ToolExecutor) ghIssues(ctx context.Context, args map[string]any) (string, error) {
	state := getString(args, "state")
	if state == "" {
		state = "open"
	}
	limit := getInt(args, "limit")
	if limit <= 0 {
		limit = 20
	}
	query := getString(args, "query")

	ghArgs := []string{
		"issue", "list",
		"--state", state,
		"--limit", strconv.Itoa(limit),
		"--json", "number,title,state,labels,assignees,author,createdAt,updatedAt",
	}
	if query != "" {
		ghArgs = append(ghArgs, "--search", query)
	}
	return e.ghExec(ctx, ghArgs...)
}

func (e *ToolExecutor) ghIssueView(ctx context.Context, args map[string]any) (string, error) {
	number := getInt(args, "number")
	if number == 0 {
		return "", fmt.Errorf("number is required")
	}
	out, err := e.ghExec(ctx,
		"issue", "view", strconv.Itoa(number),
		"--json", "number,title,body,state,labels,assignees,author,comments,createdAt,updatedAt",
	)
	if err != nil {
		return "", err
	}
	return ai.SanitizeExternalContent(out), nil
}

func (e *ToolExecutor) ghPRs(ctx context.Context, args map[string]any) (string, error) {
	state := getString(args, "state")
	if state == "" {
		state = "open"
	}
	limit := getInt(args, "limit")
	if limit <= 0 {
		limit = 20
	}
	query := getString(args, "query")

	ghArgs := []string{
		"pr", "list",
		"--state", state,
		"--limit", strconv.Itoa(limit),
		"--json", "number,title,state,author,labels,reviewDecision,isDraft,headRefName,createdAt,updatedAt",
	}
	if query != "" {
		ghArgs = append(ghArgs, "--search", query)
	}
	return e.ghExec(ctx, ghArgs...)
}

func (e *ToolExecutor) ghPRView(ctx context.Context, args map[string]any) (string, error) {
	number := getInt(args, "number")
	if number == 0 {
		return "", fmt.Errorf("number is required")
	}
	out, err := e.ghExec(ctx,
		"pr", "view", strconv.Itoa(number),
		"--json", "number,title,body,state,author,labels,reviewDecision,isDraft,headRefName,reviews,comments,files,commits,createdAt,updatedAt",
	)
	if err != nil {
		return "", err
	}
	return ai.SanitizeExternalContent(out), nil
}

func (e *ToolExecutor) ghPRDiff(ctx context.Context, args map[string]any) (string, error) {
	number := getInt(args, "number")
	if number == 0 {
		return "", fmt.Errorf("number is required")
	}
	out, err := e.ghExec(ctx, "pr", "diff", strconv.Itoa(number))
	if err != nil {
		return "", err
	}
	return ai.SanitizeExternalContent(truncate(out, maxGitHubOutputChars)), nil
}

func (e *ToolExecutor) ghActions(ctx context.Context, args map[string]any) (string, error) {
	limit := getInt(args, "limit")
	if limit <= 0 {
		limit = 10
	}
	branch := getString(args, "branch")
	status := getString(args, "status")

	ghArgs := []string{
		"run", "list",
		"--limit", strconv.Itoa(limit),
		"--json", "databaseId,displayTitle,status,conclusion,workflowName,headBranch,createdAt,updatedAt",
	}
	if branch != "" {
		ghArgs = append(ghArgs, "--branch", branch)
	}
	if status != "" {
		ghArgs = append(ghArgs, "--status", status)
	}
	return e.ghExec(ctx, ghArgs...)
}

func (e *ToolExecutor) ghActionsLogs(ctx context.Context, args map[string]any) (string, error) {
	runID := getInt(args, "run_id")
	if runID == 0 {
		return "", fmt.Errorf("run_id is required")
	}
	out, err := e.ghExec(ctx, "run", "view", strconv.Itoa(runID), "--log-failed")
	if err != nil {
		return "", err
	}
	return ai.SanitizeExternalContent(truncate(out, maxGitHubOutputChars)), nil
}

// ---------------------------------------------------------------------------
// GitHub tool handlers — write (Destructive)
// ---------------------------------------------------------------------------

func (e *ToolExecutor) ghComment(ctx context.Context, args map[string]any) (string, error) {
	number := getInt(args, "number")
	if number == 0 {
		return "", fmt.Errorf("number is required")
	}
	body := getString(args, "body")
	if body == "" {
		return "", fmt.Errorf("body is required")
	}
	_, err := e.ghExec(ctx, "issue", "comment", strconv.Itoa(number), "--body", body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("commented on #%d", number), nil
}

func (e *ToolExecutor) ghPRReview(ctx context.Context, args map[string]any) (string, error) {
	number := getInt(args, "number")
	if number == 0 {
		return "", fmt.Errorf("number is required")
	}
	body := getString(args, "body")
	if body == "" {
		return "", fmt.Errorf("body is required")
	}
	action := getString(args, "action")
	if action == "" {
		return "", fmt.Errorf("action is required")
	}
	switch action {
	case "approve", "request-changes", "comment":
		// valid
	default:
		return "", fmt.Errorf("action must be approve, request-changes, or comment")
	}
	_, err := e.ghExec(ctx, "pr", "review", strconv.Itoa(number), "--"+action, "--body", body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("submitted %s review on PR #%d", action, number), nil
}

func (e *ToolExecutor) ghActionsRerun(ctx context.Context, args map[string]any) (string, error) {
	runID := getInt(args, "run_id")
	if runID == 0 {
		return "", fmt.Errorf("run_id is required")
	}
	_, err := e.ghExec(ctx, "run", "rerun", strconv.Itoa(runID), "--failed")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("rerun triggered for failed jobs in run %d", runID), nil
}

// blockedEnvPrefixes lists environment variable prefixes that must never be
// forwarded to the gh CLI child process. These are non-GitHub secrets that
// could be exfiltrated via a crafted gh alias or extension.
var blockedEnvPrefixes = []string{
	"ANTHROPIC_",
	"AWS_SECRET_",
	"AWS_SESSION_TOKEN",
	"AZURE_",
}

// blockedEnvSuffixes lists suffixes that indicate secret values. Variables
// matching these are stripped UNLESS they start with "GITHUB_" (gh needs them).
var blockedEnvSuffixes = []string{
	"_SECRET",
	"_SECRET_KEY",
	"_API_KEY",
}

// blockedEnvExact lists specific variable names to block.
var blockedEnvExact = map[string]bool{
	"ANTHROPIC_API_KEY":     true,
	"AWS_SECRET_ACCESS_KEY": true,
	"AWS_SESSION_TOKEN":     true,
	"OPENAI_API_KEY":        true,
}

// filterEnvForGH returns the current process environment with known non-GitHub
// secrets removed. The gh CLI needs GITHUB_TOKEN, GH_TOKEN, PATH, HOME, and
// most standard variables to function, so we use a blocklist rather than an
// allowlist.
func filterEnvForGH() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if isBlockedEnvVar(key) {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

// isBlockedEnvVar reports whether the given variable name should be excluded
// from the gh CLI environment.
func isBlockedEnvVar(key string) bool {
	upper := strings.ToUpper(key)

	// Exact match blocklist.
	if blockedEnvExact[upper] {
		return true
	}

	// Prefix match.
	for _, prefix := range blockedEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			// Allow AZURE_* only if it's not a key/secret.
			if strings.HasPrefix(upper, "AZURE_") {
				return strings.HasSuffix(upper, "_KEY") || strings.HasSuffix(upper, "_SECRET")
			}
			return true
		}
	}

	// Suffix match — but never block GITHUB_* variables.
	if strings.HasPrefix(upper, "GITHUB_") || strings.HasPrefix(upper, "GH_") {
		return false
	}
	for _, suffix := range blockedEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}

	return false
}
