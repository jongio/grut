// Package middleware provides AIGitClient, a transparent wrapper around
// git.GitClient that intercepts operations with AI enhancements while
// delegating all other methods to the inner client.
package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/ai/ops"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
)

// AIGitClient wraps a git.GitClient to transparently intercept operations
// with AI enhancements. Unintercepted methods delegate directly to the
// inner client.
type AIGitClient struct {
	inner    git.GitClient
	registry *ai.Registry
	builder  *ai.Builder
	audit    *ai.AuditLogger
	cfg      config.AIConfig
}

// Compile-time interface check.
var _ git.GitClient = (*AIGitClient)(nil)

// NewAIGitClient creates an AI-enhanced git client wrapper.
func NewAIGitClient(
	inner git.GitClient,
	registry *ai.Registry,
	builder *ai.Builder,
	audit *ai.AuditLogger,
	cfg config.AIConfig,
) *AIGitClient {
	return &AIGitClient{
		inner:    inner,
		registry: registry,
		builder:  builder,
		audit:    audit,
		cfg:      cfg,
	}
}

// ---------------------------------------------------------------------------
// StatusReader — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Status(ctx context.Context) ([]git.FileStatus, error) {
	return c.inner.Status(ctx)
}

func (c *AIGitClient) Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
	return c.inner.Diff(ctx, opts)
}

func (c *AIGitClient) Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
	return c.inner.Log(ctx, opts)
}

func (c *AIGitClient) Blame(ctx context.Context, path string) ([]git.BlameLine, error) {
	return c.inner.Blame(ctx, path)
}

func (c *AIGitClient) RepoRoot(ctx context.Context) (string, error) {
	return c.inner.RepoRoot(ctx)
}

func (c *AIGitClient) IsRepo(ctx context.Context) (bool, error) {
	return c.inner.IsRepo(ctx)
}

func (c *AIGitClient) DiffTreeFiles(ctx context.Context, hash string) ([]string, error) {
	return c.inner.DiffTreeFiles(ctx, hash)
}

// ---------------------------------------------------------------------------
// IndexMutator — Commit is intercepted; Stage/Unstage pass through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Stage(ctx context.Context, paths []string) error {
	return c.inner.Stage(ctx, paths)
}

func (c *AIGitClient) Unstage(ctx context.Context, paths []string) error {
	return c.inner.Unstage(ctx, paths)
}

func (c *AIGitClient) StageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	return c.inner.StageHunk(ctx, path, hunk)
}

func (c *AIGitClient) UnstageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	return c.inner.UnstageHunk(ctx, path, hunk)
}

func (c *AIGitClient) StageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	return c.inner.StageLine(ctx, path, hunk, lineIdx)
}

func (c *AIGitClient) UnstageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	return c.inner.UnstageLine(ctx, path, hunk, lineIdx)
}

// Commit intercepts the commit operation. When cfg.AutoCommitMsg is true and
// msg is empty, it generates an AI commit message before delegating to the
// inner client. AI failures never block the git operation.
func (c *AIGitClient) Commit(ctx context.Context, msg string, opts git.CommitOpts) (string, error) {
	if c.cfg.AutoCommitMsg && strings.TrimSpace(msg) == "" {
		suggestion, err := c.generateCommitMsg(ctx)
		if err != nil {
			c.logAudit("commit_message", "error", err)
		} else if suggestion != nil {
			msg = suggestion.String()
			c.logAudit("commit_message", "accepted", nil)
		}
	}
	return c.inner.Commit(ctx, msg, opts)
}

// ---------------------------------------------------------------------------
// BranchManager — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) BranchList(ctx context.Context) ([]git.Branch, error) {
	return c.inner.BranchList(ctx)
}

func (c *AIGitClient) BranchCreate(ctx context.Context, name string, base string) error {
	return c.inner.BranchCreate(ctx, name, base)
}

func (c *AIGitClient) BranchDelete(ctx context.Context, name string, force bool) error {
	return c.inner.BranchDelete(ctx, name, force)
}

func (c *AIGitClient) BranchRename(ctx context.Context, oldName, newName string) error {
	return c.inner.BranchRename(ctx, oldName, newName)
}

func (c *AIGitClient) Checkout(ctx context.Context, ref string) error {
	return c.inner.Checkout(ctx, ref)
}

// ---------------------------------------------------------------------------
// RemoteOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Push(ctx context.Context, opts git.PushOpts) error {
	return c.inner.Push(ctx, opts)
}

func (c *AIGitClient) Pull(ctx context.Context, opts git.PullOpts) error {
	return c.inner.Pull(ctx, opts)
}

func (c *AIGitClient) Fetch(ctx context.Context, opts git.FetchOpts) error {
	return c.inner.Fetch(ctx, opts)
}

// ---------------------------------------------------------------------------
// RemoteListOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) RemoteList(ctx context.Context) ([]git.Remote, error) {
	return c.inner.RemoteList(ctx)
}

func (c *AIGitClient) RemoteAdd(ctx context.Context, name, url string) error {
	return c.inner.RemoteAdd(ctx, name, url)
}

func (c *AIGitClient) RemoteRemove(ctx context.Context, name string) error {
	return c.inner.RemoteRemove(ctx, name)
}

// ---------------------------------------------------------------------------
// WorktreeOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) WorktreeList(ctx context.Context) ([]git.Worktree, error) {
	return c.inner.WorktreeList(ctx)
}

func (c *AIGitClient) WorktreeAdd(ctx context.Context, path, branch string) error {
	return c.inner.WorktreeAdd(ctx, path, branch)
}

func (c *AIGitClient) WorktreeRemove(ctx context.Context, path string, force bool) error {
	return c.inner.WorktreeRemove(ctx, path, force)
}

// ---------------------------------------------------------------------------
// StashOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) StashList(ctx context.Context) ([]git.StashEntry, error) {
	return c.inner.StashList(ctx)
}

func (c *AIGitClient) StashShow(ctx context.Context, index int) (string, error) {
	return c.inner.StashShow(ctx, index)
}

func (c *AIGitClient) StashPush(ctx context.Context, opts git.StashOpts) error {
	return c.inner.StashPush(ctx, opts)
}

func (c *AIGitClient) StashPop(ctx context.Context, index int) error {
	return c.inner.StashPop(ctx, index)
}

func (c *AIGitClient) StashApply(ctx context.Context, index int) error {
	return c.inner.StashApply(ctx, index)
}

func (c *AIGitClient) StashDrop(ctx context.Context, index int) error {
	return c.inner.StashDrop(ctx, index)
}

// ---------------------------------------------------------------------------
// TagOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) TagList(ctx context.Context) ([]git.Tag, error) {
	return c.inner.TagList(ctx)
}

func (c *AIGitClient) TagCreate(ctx context.Context, name, ref, message string) error {
	return c.inner.TagCreate(ctx, name, ref, message)
}

func (c *AIGitClient) TagDelete(ctx context.Context, name string) error {
	return c.inner.TagDelete(ctx, name)
}

func (c *AIGitClient) TagListRemote(ctx context.Context, remote string) ([]git.Tag, error) {
	return c.inner.TagListRemote(ctx, remote)
}

func (c *AIGitClient) TagPush(ctx context.Context, remote, name string) error {
	return c.inner.TagPush(ctx, remote, name)
}

func (c *AIGitClient) TagPushAll(ctx context.Context, remote string) error {
	return c.inner.TagPushAll(ctx, remote)
}

// ---------------------------------------------------------------------------
// MergeRebaseOps — Merge and Rebase are intercepted
// ---------------------------------------------------------------------------

// Merge delegates to the inner client and, on conflict, triggers AI conflict
// resolution. AI failures never block the git operation — the original merge
// error is returned so the caller can handle it.
func (c *AIGitClient) Merge(ctx context.Context, branch string, opts git.MergeOpts) error {
	err := c.inner.Merge(ctx, branch, opts)
	if err != nil && c.hasConflicts(ctx) {
		c.tryResolveConflicts(ctx)
	}
	return err
}

func (c *AIGitClient) MergeAbort(ctx context.Context) error {
	return c.inner.MergeAbort(ctx)
}

// Rebase delegates to the inner client and, on conflict, triggers AI conflict
// resolution. AI failures never block the git operation.
func (c *AIGitClient) Rebase(ctx context.Context, onto string, opts git.RebaseOpts) error {
	err := c.inner.Rebase(ctx, onto, opts)
	if err != nil && c.hasConflicts(ctx) {
		c.tryResolveConflicts(ctx)
	}
	return err
}

func (c *AIGitClient) RebaseContinue(ctx context.Context) error {
	return c.inner.RebaseContinue(ctx)
}

func (c *AIGitClient) RebaseAbort(ctx context.Context) error {
	return c.inner.RebaseAbort(ctx)
}

func (c *AIGitClient) CherryPick(ctx context.Context, commitHash string) error {
	return c.inner.CherryPick(ctx, commitHash)
}

// ---------------------------------------------------------------------------
// BisectOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) BisectStart(ctx context.Context, bad, good string) error {
	return c.inner.BisectStart(ctx, bad, good)
}

func (c *AIGitClient) BisectGood(ctx context.Context) (string, error) {
	return c.inner.BisectGood(ctx)
}

func (c *AIGitClient) BisectBad(ctx context.Context) (string, error) {
	return c.inner.BisectBad(ctx)
}

func (c *AIGitClient) BisectReset(ctx context.Context) error {
	return c.inner.BisectReset(ctx)
}

// ---------------------------------------------------------------------------
// ReflogOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Reflog(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
	return c.inner.Reflog(ctx, ref, limit)
}

// ---------------------------------------------------------------------------
// DiscardOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) DiscardFile(ctx context.Context, path string) error {
	return c.inner.DiscardFile(ctx, path)
}

func (c *AIGitClient) DiscardAllUnstaged(ctx context.Context) error {
	return c.inner.DiscardAllUnstaged(ctx)
}

// ---------------------------------------------------------------------------
// RevertOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Revert(ctx context.Context, hash string) error {
	return c.inner.Revert(ctx, hash)
}

func (c *AIGitClient) RevertContinue(ctx context.Context) error {
	return c.inner.RevertContinue(ctx)
}

func (c *AIGitClient) RevertAbort(ctx context.Context) error {
	return c.inner.RevertAbort(ctx)
}

// ---------------------------------------------------------------------------
// ResetOps — pass-through
// ---------------------------------------------------------------------------

func (c *AIGitClient) Reset(ctx context.Context, ref string, mode git.ResetMode) error {
	return c.inner.Reset(ctx, ref, mode)
}

// ---------------------------------------------------------------------------
// Additional AI query methods (not part of git.GitClient)
// ---------------------------------------------------------------------------

// GenerateCommitMessage generates an AI commit message without committing.
func (c *AIGitClient) GenerateCommitMessage(ctx context.Context) (*ops.CommitSuggestion, error) {
	return c.generateCommitMsg(ctx)
}

// ReviewDiff performs an AI code review on the given diff.
func (c *AIGitClient) ReviewDiff(ctx context.Context, opts git.DiffOpts) ([]ops.ReviewFinding, error) {
	reviewer := ops.NewReviewer(c.registry, c.builder)
	findings, err := reviewer.Review(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("AI review: %w", err)
	}
	c.logAudit("code_review", "accepted", nil)
	return findings, nil
}

// GeneratePRDescription generates an AI PR description.
func (c *AIGitClient) GeneratePRDescription(ctx context.Context, targetBranch string) (*ops.PRDescription, error) {
	gen := ops.NewPRDescriptionGenerator(c.registry, c.builder)
	desc, err := gen.Generate(ctx, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("AI PR description: %w", err)
	}
	c.logAudit("pr_description", "accepted", nil)
	return desc, nil
}

// SuggestRebase provides AI-powered rebase suggestions.
func (c *AIGitClient) SuggestRebase(ctx context.Context, onto string) (*ops.RebaseSuggestion, error) {
	assistant := ops.NewRebaseAssistant(c.registry, c.builder)
	suggestion, err := assistant.Suggest(ctx, onto)
	if err != nil {
		return nil, fmt.Errorf("AI rebase suggestion: %w", err)
	}
	c.logAudit("rebase_suggest", "accepted", nil)
	return suggestion, nil
}

// AnalyzeBranches recommends branch cleanup actions.
func (c *AIGitClient) AnalyzeBranches(ctx context.Context) ([]ops.BranchRecommendation, error) {
	analyzer := ops.NewBranchAnalyzer(c.registry, c.builder, c.inner)
	recs, err := analyzer.Analyze(ctx)
	if err != nil {
		return nil, fmt.Errorf("AI branch analysis: %w", err)
	}
	c.logAudit("branch_analyze", "accepted", nil)
	return recs, nil
}

// AnalyzeBisect provides AI-enhanced bisect analysis.
func (c *AIGitClient) AnalyzeBisect(ctx context.Context, good, bad string) (*ops.BisectAnalysis, error) {
	analyzer := ops.NewBisectAnalyzer(c.registry, c.builder)
	analysis, err := analyzer.Analyze(ctx, good, bad)
	if err != nil {
		return nil, fmt.Errorf("AI bisect analysis: %w", err)
	}
	c.logAudit("bisect_analyze", "accepted", nil)
	return analysis, nil
}

// GenerateChangelog creates an AI-powered changelog.
func (c *AIGitClient) GenerateChangelog(ctx context.Context, fromRef, toRef string) ([]ops.ChangelogEntry, error) {
	gen := ops.NewChangelogGenerator(c.registry, c.builder)
	entries, err := gen.Generate(ctx, fromRef, toRef)
	if err != nil {
		return nil, fmt.Errorf("AI changelog: %w", err)
	}
	c.logAudit("changelog", "accepted", nil)
	return entries, nil
}

// SuggestSplit suggests how to split a large commit.
func (c *AIGitClient) SuggestSplit(ctx context.Context, commitHash string) (*ops.SplitPlan, error) {
	splitter := ops.NewCommitSplitter(c.registry, c.builder)
	plan, err := splitter.Suggest(ctx, commitHash)
	if err != nil {
		return nil, fmt.Errorf("AI split suggestion: %w", err)
	}
	c.logAudit("split", "accepted", nil)
	return plan, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// generateCommitMsg uses the CommitGenerator to produce an AI-generated
// commit message from the currently staged changes.
func (c *AIGitClient) generateCommitMsg(ctx context.Context) (*ops.CommitSuggestion, error) {
	gen := ops.NewCommitGenerator(c.registry, c.builder)
	return gen.Generate(ctx)
}

// hasConflicts checks the working tree for unmerged (conflict) entries.
func (c *AIGitClient) hasConflicts(ctx context.Context) bool {
	statuses, err := c.inner.Status(ctx)
	if err != nil {
		return false
	}
	for _, s := range statuses {
		if s.StagedStatus == git.StatusConflict || s.WorktreeStatus == git.StatusConflict {
			return true
		}
	}
	return false
}

// conflictFiles returns the paths of all files with conflict status.
func (c *AIGitClient) conflictFiles(ctx context.Context) []string {
	statuses, err := c.inner.Status(ctx)
	if err != nil {
		return nil
	}
	var files []string
	for _, s := range statuses {
		if s.StagedStatus == git.StatusConflict || s.WorktreeStatus == git.StatusConflict {
			files = append(files, s.Path)
		}
	}
	return files
}

// tryResolveConflicts attempts AI conflict resolution. Failures are logged
// but never propagated — the caller retains the original merge/rebase error.
func (c *AIGitClient) tryResolveConflicts(ctx context.Context) {
	files := c.conflictFiles(ctx)
	if len(files) == 0 {
		return
	}

	resolver := ops.NewConflictResolver(c.registry, c.builder)
	_, err := resolver.Resolve(ctx, files)
	if err != nil {
		c.logAudit("conflict_resolve", "error", err)
		return
	}
	c.logAudit("conflict_resolve", "accepted", nil)
}

// logAudit writes an audit entry. If the audit logger is nil or logging
// fails, the error is silently ignored — audit must never block operations.
func (c *AIGitClient) logAudit(operation, result string, opErr error) {
	if c.audit == nil {
		return
	}
	entry := ai.AuditEntry{
		Timestamp: time.Now(),
		Operation: operation,
		Result:    result,
	}
	if opErr != nil {
		// Redact error messages before storing in audit logs — API errors
		// may contain secrets, tokens, or internal server details.
		errMsg := opErr.Error()
		if c.builder != nil {
			redactor := ai.NewRedactor(nil)
			errMsg, _ = redactor.RedactContent(errMsg)
		}
		entry.Error = errMsg
	}
	_ = c.audit.Log(entry)
}
