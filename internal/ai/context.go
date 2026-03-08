package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/jongio/grut/internal/git"
)

// estimateTokens provides a rough token count for the given string.
// Uses the common approximation of ~4 characters per token.
func estimateTokens(s string) int {
	return len(s) / 4
}

// Builder constructs GitContext objects from repository state, managing
// token budgets to stay within provider limits.
type Builder struct {
	client    git.GitClient
	redactor  *Redactor
	maxTokens int
}

// NewBuilder creates a context builder with the given git client, optional
// redactor (may be nil to skip redaction), and maximum token budget.
// A maxTokens of 0 or negative means no limit.
//
// Security: if redactor is nil, a default redactor with built-in secret
// patterns is created to ensure fail-closed behavior — file contents are
// never sent to AI providers without redaction (CWE-200).
func NewBuilder(client git.GitClient, redactor *Redactor, maxTokens int) *Builder {
	if redactor == nil {
		redactor = NewRedactor(nil)
	}
	return &Builder{
		client:    client,
		redactor:  redactor,
		maxTokens: maxTokens,
	}
}

// ---------------------------------------------------------------------------
// Token budget
// ---------------------------------------------------------------------------

// tokenBudget tracks the remaining token budget for building a GitContext.
// If maxTokens is 0 or negative, all capacity checks return true (unlimited).
type tokenBudget struct {
	max  int
	used int
}

func newTokenBudget(maxTokens int) *tokenBudget {
	return &tokenBudget{max: maxTokens}
}

// unlimited reports whether there is no token budget constraint.
func (tb *tokenBudget) unlimited() bool { return tb.max <= 0 }

// remaining returns the number of tokens still available.
func (tb *tokenBudget) remaining() int {
	if tb.unlimited() {
		return int(^uint(0) >> 1) // max int
	}
	if r := tb.max - tb.used; r > 0 {
		return r
	}
	return 0
}

// canFit reports whether the given number of tokens fits in the budget.
func (tb *tokenBudget) canFit(tokens int) bool {
	if tb.unlimited() {
		return true
	}
	return tb.used+tokens <= tb.max
}

// consume records tokens as used.
func (tb *tokenBudget) consume(tokens int) { tb.used += tokens }

// ---------------------------------------------------------------------------
// Token estimation helpers
// ---------------------------------------------------------------------------

// estimateTokensForDiffs returns the estimated token count for a slice of
// FileDiff by summing path, header, and line content lengths.
func estimateTokensForDiffs(diffs []git.FileDiff) int {
	n := 0
	for i := range diffs {
		n += len(diffs[i].Path)
		if diffs[i].OldPath != "" {
			n += len(diffs[i].OldPath)
		}
		for j := range diffs[i].Hunks {
			n += len(diffs[i].Hunks[j].Header)
			for k := range diffs[i].Hunks[j].Lines {
				n += len(diffs[i].Hunks[j].Lines[k].Content) + 1
			}
		}
	}
	return n / 4
}

// estimateTokensForCommit returns the estimated token count for a single
// Commit.
func estimateTokensForCommit(c git.Commit) int {
	return (len(c.Hash) + len(c.Author) + len(c.Subject) + len(c.Body) + 20) / 4
}

// estimateTokensForStatus returns the estimated token count for a slice of
// FileStatus.
func estimateTokensForStatus(statuses []git.FileStatus) int {
	n := 0
	for i := range statuses {
		n += len(statuses[i].Path) + 5
	}
	return n / 4
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// currentBranch returns the name of the currently checked-out branch.
func (b *Builder) currentBranch(ctx context.Context) string {
	branches, err := b.client.BranchList(ctx)
	if err != nil {
		return ""
	}
	for _, br := range branches {
		if br.IsCurrent {
			return br.Name
		}
	}
	return ""
}

// shouldExclude reports whether the redactor excludes a given file path.
func (b *Builder) shouldExclude(path string) bool {
	if b.redactor == nil {
		return false
	}
	return b.redactor.ShouldExcludeFile(path)
}

// redactString applies content redaction if a redactor is configured.
func (b *Builder) redactString(content string) string {
	if b.redactor == nil {
		return content
	}
	redacted, _ := b.redactor.RedactContent(content)
	return redacted
}

// readFileContent reads a file from the working tree, applying redaction.
// Returns empty string if the file is excluded, escapes the repo root, or
// cannot be read.
func (b *Builder) readFileContent(repoRoot, path string) string {
	if b.shouldExclude(path) {
		return ""
	}

	// Path-jail validation: resolve the final path and ensure it stays
	// inside repoRoot. This prevents directory-traversal attacks where
	// path contains "../" or follows a symlink outside the repo.
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return ""
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		resolvedRoot = filepath.Clean(absRoot)
	}
	fullPath := filepath.Join(absRoot, path)
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// File may not exist yet — try the parent to catch traversal.
		resolved = filepath.Clean(fullPath)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "" // Path escapes repository root.
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return ""
	}
	return b.redactString(string(data))
}

// filterDiffs removes excluded files from diff results.
func (b *Builder) filterDiffs(diffs []git.FileDiff) []git.FileDiff {
	if b.redactor == nil {
		return diffs
	}
	filtered := make([]git.FileDiff, 0, len(diffs))
	for i := range diffs {
		if !b.shouldExclude(diffs[i].Path) {
			filtered = append(filtered, diffs[i])
		}
	}
	return filtered
}

// trimCommitsToBudget returns as many commits as fit within the remaining
// token budget, consuming tokens as it goes.
func trimCommitsToBudget(commits []git.Commit, budget *tokenBudget) []git.Commit {
	if budget.unlimited() {
		return commits
	}
	var result []git.Commit
	for i := range commits {
		tokens := estimateTokensForCommit(commits[i])
		if !budget.canFit(tokens) {
			break
		}
		budget.consume(tokens)
		result = append(result, commits[i])
	}
	return result
}

// parseConflictMarkers extracts conflict regions from file content that
// contains standard git conflict markers (<<<<<<< / ======= / >>>>>>>).
func parseConflictMarkers(content string) []ConflictRegion {
	lines := strings.Split(content, "\n")
	var regions []ConflictRegion
	var current *ConflictRegion
	inOurs, inTheirs := false, false

	for i, line := range lines {
		lineNo := i + 1
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			current = &ConflictRegion{StartLine: lineNo}
			inOurs = true
			inTheirs = false
		case strings.HasPrefix(line, "=======") && current != nil:
			inOurs = false
			inTheirs = true
		case strings.HasPrefix(line, ">>>>>>>") && current != nil:
			current.EndLine = lineNo
			regions = append(regions, *current)
			current = nil
			inOurs = false
			inTheirs = false
		default:
			if current != nil {
				if inOurs {
					current.Ours += line + "\n"
				} else if inTheirs {
					current.Theirs += line + "\n"
				}
			}
		}
	}
	return regions
}

// ---------------------------------------------------------------------------
// Public ForX methods
// ---------------------------------------------------------------------------

// ForConflict builds context for merge conflict resolution.
// Includes conflict file contents, branch histories, and surrounding context.
func (b *Builder) ForConflict(ctx context.Context, files []string) (GitContext, error) {
	gc := GitContext{FileContents: make(map[string]string)}
	budget := newTokenBudget(b.maxTokens)

	// Metadata — always included.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)
	gc.Status, _ = b.client.Status(ctx)

	// Priority 1: Conflict regions and file contents.
	for _, path := range files {
		if b.shouldExclude(path) {
			continue
		}
		content := b.readFileContent(gc.RepoRoot, path)
		if content == "" {
			continue
		}
		markers := parseConflictMarkers(content)
		if len(markers) == 0 {
			continue
		}
		gc.Conflicts = append(gc.Conflicts, ConflictFile{
			Path:            path,
			ConflictMarkers: markers,
		})
		contentTokens := estimateTokens(content)
		if budget.canFit(contentTokens) {
			gc.FileContents[path] = content
			budget.consume(contentTokens)
		}
	}

	// Priority 2: Recent commits for branch context.
	if log, err := b.client.Log(ctx, git.LogOpts{MaxCount: 5}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForCommit builds context for commit message generation.
// Includes staged diff, recent commits (for style matching), and branch name.
func (b *Builder) ForCommit(ctx context.Context) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata — always included.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)
	gc.Status, _ = b.client.Status(ctx)

	// Priority 1: Staged diff.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{Staged: true}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	// Priority 2: Recent commits for style matching.
	if log, err := b.client.Log(ctx, git.LogOpts{MaxCount: 10}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForReview builds context for code review.
// Includes diff, affected file contents, and recent commits.
func (b *Builder) ForReview(ctx context.Context, opts git.DiffOpts) (GitContext, error) {
	gc := GitContext{FileContents: make(map[string]string)}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)
	gc.Status, _ = b.client.Status(ctx)

	// Priority 1: Diff.
	var diffs []git.FileDiff
	if d, err := b.client.Diff(ctx, opts); err == nil {
		diffs = b.filterDiffs(d)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	// Priority 2: Full file contents for affected files.
	if gc.RepoRoot != "" {
		for i := range diffs {
			content := b.readFileContent(gc.RepoRoot, diffs[i].Path)
			if content == "" {
				continue
			}
			if tokens := estimateTokens(content); budget.canFit(tokens) {
				gc.FileContents[diffs[i].Path] = content
				budget.consume(tokens)
			}
		}
	}

	// Priority 3: Recent commits.
	if log, err := b.client.Log(ctx, git.LogOpts{MaxCount: 5}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForPR builds context for PR description generation.
// Includes diff vs target branch and all branch commits.
func (b *Builder) ForPR(ctx context.Context, targetBranch string) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.TargetBranch = targetBranch
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)

	// Priority 1: Diff vs target branch.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{
		CommitA: targetBranch,
		CommitB: "HEAD",
	}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	// Priority 2: All commits on the branch since target.
	if log, err := b.client.Log(ctx, git.LogOpts{
		Ref: targetBranch + "..HEAD",
	}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForRebase builds context for rebase assistance.
// Includes commits to rebase and their diffs.
func (b *Builder) ForRebase(ctx context.Context, onto string) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.TargetBranch = onto
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)

	// Priority 1: Commits to rebase.
	if log, err := b.client.Log(ctx, git.LogOpts{
		Ref: onto + "..HEAD",
	}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	// Priority 2: Diff from onto to HEAD.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{
		CommitA: onto,
		CommitB: "HEAD",
	}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	return gc, nil
}

// ForBisect builds context for bisect analysis.
// Includes diff between good/bad and candidate commits.
func (b *Builder) ForBisect(ctx context.Context, good, bad string) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)

	// Priority 1: Diff between good and bad.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{
		CommitA: good,
		CommitB: bad,
	}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	// Priority 2: Commits between good and bad.
	if log, err := b.client.Log(ctx, git.LogOpts{
		Ref: good + ".." + bad,
	}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForChangelog builds context for changelog generation.
// Includes all commits between two refs.
func (b *Builder) ForChangelog(ctx context.Context, fromRef, toRef string) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)

	// Priority 1: All commits in range.
	if log, err := b.client.Log(ctx, git.LogOpts{
		Ref: fromRef + ".." + toRef,
	}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	// Priority 2: Diff between refs.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{
		CommitA: fromRef,
		CommitB: toRef,
	}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	return gc, nil
}

// ForSplit builds context for commit splitting.
// Includes the full diff of a large commit.
func (b *Builder) ForSplit(ctx context.Context, commitHash string) (GitContext, error) {
	gc := GitContext{}
	budget := newTokenBudget(b.maxTokens)

	// Metadata.
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)

	// Priority 1: The commit's diff.
	if diffs, err := b.client.Diff(ctx, git.DiffOpts{
		CommitA: commitHash + "^",
		CommitB: commitHash,
	}); err == nil {
		diffs = b.filterDiffs(diffs)
		if tokens := estimateTokensForDiffs(diffs); budget.canFit(tokens) {
			gc.Diffs = diffs
			budget.consume(tokens)
		}
	}

	// Priority 2: The commit itself for context.
	if log, err := b.client.Log(ctx, git.LogOpts{
		Ref:      commitHash,
		MaxCount: 1,
	}); err == nil && len(log) > 0 {
		gc.Log = trimCommitsToBudget(log, budget)
	}

	return gc, nil
}

// ForChat builds lightweight context for the chat box.
// Includes only branch name and working tree status — no diffs or file
// contents.
func (b *Builder) ForChat(ctx context.Context) (GitContext, error) {
	gc := GitContext{}
	gc.CurrentBranch = b.currentBranch(ctx)
	gc.RepoRoot, _ = b.client.RepoRoot(ctx)
	gc.Status, _ = b.client.Status(ctx)
	return gc, nil
}
