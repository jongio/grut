package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/mcp"
)

// ToolExecutor maps AI tool calls to actual git and file operations.
type ToolExecutor struct {
	client   git.GitClient
	jail     *mcp.PathJail
	limiter  *mcp.RateLimiter
	registry *ToolRegistry
}

// maxListEntries caps the number of entries returned by file listing and
// search operations to prevent excessive memory use. The same limit is
// enforced in the MCP file_list tool (internal/mcp/file_tools.go) as
// maxFileListEntries — keep both values in sync.
const maxListEntries = 10000

// ToolResult holds the outcome of executing a tool call.
type ToolResult struct {
	ToolID  string // Matches the ToolCall.ID
	Content string // Result content for the AI to consume
	Error   string // Error message if execution failed
}

// NewToolExecutor creates a ToolExecutor wired to the given git client,
// path jail, rate limiter, and tool registry.
func NewToolExecutor(client git.GitClient, jail *mcp.PathJail, limiter *mcp.RateLimiter, registry *ToolRegistry) *ToolExecutor {
	return &ToolExecutor{
		client:   client,
		jail:     jail,
		limiter:  limiter,
		registry: registry,
	}
}

// Execute runs a tool call and returns the result.
// It does NOT check safety classification — caller must handle confirmation
// first.
func (e *ToolExecutor) Execute(ctx context.Context, call ai.ToolCall) ToolResult {
	// Verify the tool exists in the registry.
	_, ok := e.registry.Get(call.Name)
	if !ok {
		return ToolResult{
			ToolID: call.ID,
			Error:  fmt.Sprintf("unknown tool: %s", call.Name),
		}
	}
	// Rate-limit based on tool category.
	category := e.rateCategory(call.Name)
	if !e.limiter.Allow(category) {
		return ToolResult{
			ToolID: call.ID,
			Error:  fmt.Sprintf("rate limit exceeded for %s operations", category),
		}
	}
	content, err := e.dispatch(ctx, call.Name, call.Arguments)
	if err != nil {
		return ToolResult{
			ToolID: call.ID,
			Error:  err.Error(),
		}
	}
	return ToolResult{
		ToolID:  call.ID,
		Content: content,
	}
}

// rateCategory returns "read" or "write" depending on the tool name.
func (e *ToolExecutor) rateCategory(name string) string {
	info, ok := e.registry.Get(name)
	if !ok {
		return "read"
	}
	if info.Safety == Destructive {
		return "write"
	}
	// Read-only tools and safe write tools (stage, commit) use the read
	// bucket since they are frequent, low-risk operations.
	switch name {
	case "file_write", "file_delete", "file_rename", "file_mkdir", //nolint:goconst // inline tool names are easier to scan here
		"git_push", "git_branch_delete", "git_rebase", "git_reset", //nolint:goconst // inline tool names are easier to scan here
		"git_tag_delete", "git_discard", //nolint:goconst // inline tool names are easier to scan here
		"bulk_delete", "bulk_rename": //nolint:goconst // inline tool names are easier to scan here
		return "write"
	default:
		return "read"
	}
}

// dispatch routes a tool call to the appropriate handler.
func (e *ToolExecutor) dispatch(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	// ── File operations ──────────────────────────────────────────────
	case "file_read":
		return e.fileRead(ctx, args)
	case "file_write":
		return e.fileWrite(ctx, args)
	case "file_delete":
		return e.fileDelete(ctx, args)
	case "file_rename":
		return e.fileRename(ctx, args)
	case "file_list":
		return e.fileList(ctx, args)
	case "file_mkdir":
		return e.fileMkdir(ctx, args)
	// ── Git read operations ──────────────────────────────────────────
	case "git_status":
		return e.gitStatus(ctx)
	case "git_diff":
		return e.gitDiff(ctx, args)
	case "git_log":
		return e.gitLog(ctx, args)
	case "git_blame":
		return e.gitBlame(ctx, args)
	case "git_branch_list":
		return e.gitBranchList(ctx)
	case "git_stash_list":
		return e.gitStashList(ctx)
	case "git_worktree_list":
		return e.gitWorktreeList(ctx)
	// ── Git write operations ─────────────────────────────────────────
	case "git_stage":
		return e.gitStage(ctx, args)
	case "git_unstage":
		return e.gitUnstage(ctx, args)
	case "git_commit":
		return e.gitCommit(ctx, args)
	case "git_push":
		return e.gitPush(ctx, args)
	case "git_pull":
		return e.gitPull(ctx, args)
	case "git_fetch":
		return e.gitFetch(ctx, args)
	case "git_checkout":
		return e.gitCheckout(ctx, args)
	case "git_branch_create":
		return e.gitBranchCreate(ctx, args)
	case "git_branch_delete":
		return e.gitBranchDelete(ctx, args)
	case "git_merge":
		return e.gitMerge(ctx, args)
	case "git_rebase":
		return e.gitRebase(ctx, args)
	case "git_stash_push":
		return e.gitStashPush(ctx, args)
	case "git_stash_pop":
		return e.gitStashPop(ctx, args)
	case "git_reset":
		return e.gitReset(ctx, args)
	case "git_tag_create":
		return e.gitTagCreate(ctx, args)
	case "git_tag_delete": //nolint:goconst // inline tool names are easier to scan here
		return e.gitTagDelete(ctx, args)
	case "git_discard":
		return e.gitDiscard(ctx, args)
	// ── Navigation & search ──────────────────────────────────────────
	case "navigate_to":
		return e.navigateTo(args)
	case "search_files":
		return e.searchFiles(ctx, args)
	case "search_content":
		return e.searchContent(ctx, args)
	case "explain":
		return e.explain(args)
	// ── Bulk operations ──────────────────────────────────────────────
	case "bulk_stage":
		return e.bulkStage(ctx, args)
	case "bulk_delete":
		return e.bulkDelete(ctx, args)
	case "bulk_rename":
		return e.bulkRename(ctx, args)
	// ── GitHub operations ────────────────────────────────────────────
	case "gh_issues":
		return e.ghIssues(ctx, args)
	case "gh_issue_view":
		return e.ghIssueView(ctx, args)
	case "gh_prs":
		return e.ghPRs(ctx, args)
	case "gh_pr_view":
		return e.ghPRView(ctx, args)
	case "gh_pr_diff":
		return e.ghPRDiff(ctx, args)
	case "gh_actions":
		return e.ghActions(ctx, args)
	case "gh_actions_logs":
		return e.ghActionsLogs(ctx, args)
	case "gh_comment":
		return e.ghComment(ctx, args)
	case "gh_pr_review":
		return e.ghPRReview(ctx, args)
	case "gh_actions_rerun":
		return e.ghActionsRerun(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// ---------------------------------------------------------------------------
// File operation handlers
// ---------------------------------------------------------------------------
func (e *ToolExecutor) fileRead(_ context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved, err := e.jail.Validate(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	if err := mcp.IsSensitivePath(path); err != nil {
		return "", fmt.Errorf("path blocked: %w", err)
	}
	// CR-008: Check resolved path against sensitive patterns to catch
	// symlinks inside jail that point to sensitive files.
	if err := mcp.IsSensitivePath(resolved); err != nil {
		return "", fmt.Errorf("path blocked (resolved): %w", err)
	}
	// CR-004: Open the file once and stat+read from the same fd to
	// avoid TOCTOU races (CWE-367).
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	const maxFileReadSize = 10 * 1024 * 1024 // 10 MiB — keep in sync with internal/mcp/file_tools.go
	if info.Size() > maxFileReadSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileReadSize)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileReadSize))
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (e *ToolExecutor) fileWrite(_ context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	content := getString(args, "content")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	// CR-003: Enforce write size limit matching MCP file_write.
	const maxFileWriteSize = 10 * 1024 * 1024 // 10 MiB — keep in sync with internal/mcp/file_tools.go
	if len(content) > maxFileWriteSize {
		return "", fmt.Errorf("content too large: %d bytes (max %d)", len(content), maxFileWriteSize)
	}
	resolved, err := e.jail.Validate(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	if err := mcp.IsSensitivePath(path); err != nil {
		return "", fmt.Errorf("path blocked: %w", err)
	}
	// Ensure parent directory exists.
	if mkErr := os.MkdirAll(filepath.Dir(resolved), 0o755); mkErr != nil {
		return "", fmt.Errorf("create directory: %w", mkErr)
	}
	// CR-002: Use open-stat-write-on-fd pattern to prevent TOCTOU races
	// (CWE-367). CR-005: Use 0o600 instead of 0o644.
	f, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("open file for write: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat opened file: %w", err)
	}
	if fi.Mode()&os.ModeType != 0 {
		return "", fmt.Errorf("refusing to write: not a regular file")
	}
	if _, err := f.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("written %d bytes to %s", len(content), path), nil
}

func (e *ToolExecutor) fileDelete(_ context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := mcp.IsSensitivePath(path); err != nil {
		return "", err
	}
	resolved, err := e.jail.Validate(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	if err := os.Remove(resolved); err != nil {
		return "", fmt.Errorf("delete file: %w", err)
	}
	return fmt.Sprintf("deleted %s", path), nil
}

func (e *ToolExecutor) fileRename(_ context.Context, args map[string]any) (string, error) {
	oldPath := getString(args, "old_path")
	newPath := getString(args, "new_path")
	if oldPath == "" || newPath == "" {
		return "", fmt.Errorf("old_path and new_path are required")
	}
	if err := mcp.IsSensitivePath(oldPath); err != nil {
		return "", err
	}
	if err := mcp.IsSensitivePath(newPath); err != nil {
		return "", err
	}
	resolvedOld, err := e.jail.Validate(oldPath)
	if err != nil {
		return "", fmt.Errorf("invalid old path: %s", oldPath)
	}
	resolvedNew, err := e.jail.Validate(newPath)
	if err != nil {
		return "", fmt.Errorf("invalid new path: %s", newPath)
	}
	// Ensure parent directory for target exists.
	if mkErr := os.MkdirAll(filepath.Dir(resolvedNew), 0o755); mkErr != nil {
		return "", fmt.Errorf("create directory: %w", mkErr)
	}
	// Re-validate parent directory after MkdirAll to close TOCTOU gap
	// (CWE-367): a symlink could have been swapped in between initial
	// validation and directory creation.
	if _, err := e.jail.Validate(filepath.Dir(resolvedNew)); err != nil {
		return "", fmt.Errorf("parent directory escaped jail after mkdir: %w", err)
	}
	if err := os.Rename(resolvedOld, resolvedNew); err != nil {
		return "", fmt.Errorf("rename file: %w", err)
	}
	return fmt.Sprintf("renamed %s to %s", oldPath, newPath), nil
}

func (e *ToolExecutor) fileList(_ context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		path = "."
	}
	recursive := getBool(args, "recursive")
	resolved, err := e.jail.Validate(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	var entries []string
	if recursive {
		err = filepath.WalkDir(resolved, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // error returned as tool result
			}
			if d.IsDir() && d.Name() == ".git" { //nolint:goconst // inline string is more readable here
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(e.jail.Root(), p)
			if relErr != nil {
				return nil //nolint:nilerr // error returned as tool result
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			if len(entries) >= maxListEntries {
				return fmt.Errorf("listing capped at %d entries", maxListEntries)
			}
			suffix := ""
			if d.IsDir() {
				suffix = "/"
			}
			entries = append(entries, rel+suffix)
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walk directory: %w", err)
		}
	} else {
		dirEntries, readErr := os.ReadDir(resolved)
		if readErr != nil {
			return "", fmt.Errorf("list directory: %w", readErr)
		}
		for _, d := range dirEntries {
			if d.Name() == ".git" {
				continue
			}
			suffix := ""
			if d.IsDir() {
				suffix = "/"
			}
			entries = append(entries, d.Name()+suffix)
		}
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(entries, "\n"), nil
}

func (e *ToolExecutor) fileMkdir(_ context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := mcp.IsSensitivePath(path); err != nil {
		return "", err
	}
	resolved, err := e.jail.Validate(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return fmt.Sprintf("created %s", path), nil
}

// ---------------------------------------------------------------------------
// Git read handlers
// ---------------------------------------------------------------------------
func (e *ToolExecutor) gitStatus(ctx context.Context) (string, error) {
	statuses, err := e.client.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	if len(statuses) == 0 {
		return "nothing to commit, working tree clean", nil
	}
	return toJSON(statuses)
}

func (e *ToolExecutor) gitDiff(ctx context.Context, args map[string]any) (string, error) {
	opts := git.DiffOpts{
		Staged: getBool(args, "staged"),
		Path:   getString(args, "path"),
	}
	diffs, err := e.client.Diff(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	if len(diffs) == 0 {
		return "no differences", nil
	}
	return toJSON(diffs)
}

func (e *ToolExecutor) gitLog(ctx context.Context, args map[string]any) (string, error) {
	count := getInt(args, "count")
	if count <= 0 {
		count = 10
	}
	opts := git.LogOpts{
		MaxCount: count,
		Path:     getString(args, "path"),
	}
	commits, err := e.client.Log(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	if len(commits) == 0 {
		return "no commits", nil
	}
	return toJSON(commits)
}

func (e *ToolExecutor) gitBlame(ctx context.Context, args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	lines, err := e.client.Blame(ctx, path)
	if err != nil {
		return "", fmt.Errorf("git blame: %w", err)
	}
	return toJSON(lines)
}

func (e *ToolExecutor) gitBranchList(ctx context.Context) (string, error) {
	branches, err := e.client.BranchList(ctx)
	if err != nil {
		return "", fmt.Errorf("git branch list: %w", err)
	}
	return toJSON(branches)
}

func (e *ToolExecutor) gitStashList(ctx context.Context) (string, error) {
	entries, err := e.client.StashList(ctx)
	if err != nil {
		return "", fmt.Errorf("git stash list: %w", err)
	}
	if len(entries) == 0 {
		return "no stash entries", nil
	}
	return toJSON(entries)
}

func (e *ToolExecutor) gitWorktreeList(ctx context.Context) (string, error) {
	worktrees, err := e.client.WorktreeList(ctx)
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}
	return toJSON(worktrees)
}

// ---------------------------------------------------------------------------
// Git write handlers
// ---------------------------------------------------------------------------
func (e *ToolExecutor) gitStage(ctx context.Context, args map[string]any) (string, error) {
	paths := getStringSlice(args, "paths")
	if len(paths) == 0 {
		return "", fmt.Errorf("paths is required")
	}
	if err := e.client.Stage(ctx, paths); err != nil {
		return "", fmt.Errorf("git stage: %w", err)
	}
	return fmt.Sprintf("staged %d file(s)", len(paths)), nil
}

func (e *ToolExecutor) gitUnstage(ctx context.Context, args map[string]any) (string, error) {
	paths := getStringSlice(args, "paths")
	if len(paths) == 0 {
		return "", fmt.Errorf("paths is required")
	}
	if err := e.client.Unstage(ctx, paths); err != nil {
		return "", fmt.Errorf("git unstage: %w", err)
	}
	return fmt.Sprintf("unstaged %d file(s)", len(paths)), nil
}

func (e *ToolExecutor) gitCommit(ctx context.Context, args map[string]any) (string, error) {
	message := getString(args, "message")
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	hash, err := e.client.Commit(ctx, message, git.CommitOpts{})
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	return fmt.Sprintf("committed: %s", hash), nil
}

func (e *ToolExecutor) gitPush(ctx context.Context, args map[string]any) (string, error) {
	opts := git.PushOpts{
		Remote: getString(args, "remote"),
		Force:  getBool(args, "force"),
	}
	if opts.Remote == "" {
		opts.Remote = "origin" //nolint:goconst // default remote name is clearer inline
	}
	if err := e.client.Push(ctx, opts); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}
	return "pushed successfully", nil
}

func (e *ToolExecutor) gitPull(ctx context.Context, args map[string]any) (string, error) {
	opts := git.PullOpts{
		Remote: getString(args, "remote"),
	}
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if err := e.client.Pull(ctx, opts); err != nil {
		return "", fmt.Errorf("git pull: %w", err)
	}
	return "pulled successfully", nil
}

func (e *ToolExecutor) gitFetch(ctx context.Context, args map[string]any) (string, error) {
	opts := git.FetchOpts{
		Remote: getString(args, "remote"),
	}
	if err := e.client.Fetch(ctx, opts); err != nil {
		return "", fmt.Errorf("git fetch: %w", err)
	}
	return "fetched successfully", nil
}

func (e *ToolExecutor) gitCheckout(ctx context.Context, args map[string]any) (string, error) {
	ref := getString(args, "ref")
	if ref == "" {
		return "", fmt.Errorf("ref is required")
	}
	if err := e.client.Checkout(ctx, ref); err != nil {
		return "", fmt.Errorf("git checkout: %w", err)
	}
	return fmt.Sprintf("checked out %s", ref), nil
}

func (e *ToolExecutor) gitBranchCreate(ctx context.Context, args map[string]any) (string, error) {
	name := getString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	startPoint := getString(args, "start_point")
	if err := e.client.BranchCreate(ctx, name, startPoint); err != nil {
		return "", fmt.Errorf("git branch create: %w", err)
	}
	return fmt.Sprintf("created branch %s", name), nil
}

func (e *ToolExecutor) gitBranchDelete(ctx context.Context, args map[string]any) (string, error) {
	name := getString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	force := getBool(args, "force")
	if err := e.client.BranchDelete(ctx, name, force); err != nil {
		return "", fmt.Errorf("git branch delete: %w", err)
	}
	return fmt.Sprintf("deleted branch %s", name), nil
}

func (e *ToolExecutor) gitMerge(ctx context.Context, args map[string]any) (string, error) {
	branch := getString(args, "branch")
	if branch == "" {
		return "", fmt.Errorf("branch is required")
	}
	if err := e.client.Merge(ctx, branch, git.MergeOpts{}); err != nil {
		return "", fmt.Errorf("git merge: %w", err)
	}
	return fmt.Sprintf("merged %s", branch), nil
}

func (e *ToolExecutor) gitRebase(ctx context.Context, args map[string]any) (string, error) {
	onto := getString(args, "onto")
	if onto == "" {
		return "", fmt.Errorf("onto is required")
	}
	if err := e.client.Rebase(ctx, onto, git.RebaseOpts{}); err != nil {
		return "", fmt.Errorf("git rebase: %w", err)
	}
	return fmt.Sprintf("rebased onto %s", onto), nil
}

func (e *ToolExecutor) gitStashPush(ctx context.Context, args map[string]any) (string, error) {
	opts := git.StashOpts{
		Message: getString(args, "message"),
	}
	if err := e.client.StashPush(ctx, opts); err != nil {
		return "", fmt.Errorf("git stash push: %w", err)
	}
	return "stashed changes", nil
}

func (e *ToolExecutor) gitStashPop(ctx context.Context, args map[string]any) (string, error) {
	index := getInt(args, "index")
	if err := e.client.StashPop(ctx, index); err != nil {
		return "", fmt.Errorf("git stash pop: %w", err)
	}
	return "applied and dropped stash", nil
}

func (e *ToolExecutor) gitReset(_ context.Context, args map[string]any) (string, error) {
	ref := getString(args, "ref")
	if ref == "" {
		return "", fmt.Errorf("ref is required")
	}
	// git_reset and git_discard are registered as chat tools but the
	// GitClient interface does not yet expose Reset/Discard methods.
	// Return a clear message so the AI can relay it to the user.
	return "", fmt.Errorf("git reset is not yet supported via the chat interface")
}

func (e *ToolExecutor) gitTagCreate(ctx context.Context, args map[string]any) (string, error) {
	name := getString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	ref := getString(args, "ref")
	message := getString(args, "message")
	if err := e.client.TagCreate(ctx, name, ref, message); err != nil {
		return "", fmt.Errorf("git tag create: %w", err)
	}
	return fmt.Sprintf("created tag %s", name), nil
}

func (e *ToolExecutor) gitTagDelete(ctx context.Context, args map[string]any) (string, error) {
	name := getString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := e.client.TagDelete(ctx, name); err != nil {
		return "", fmt.Errorf("git tag delete: %w", err)
	}
	return fmt.Sprintf("deleted tag %s", name), nil
}

func (e *ToolExecutor) gitDiscard(_ context.Context, args map[string]any) (string, error) {
	paths := getStringSlice(args, "paths")
	if len(paths) == 0 {
		return "", fmt.Errorf("paths is required")
	}
	// git_discard is registered as a chat tool but the GitClient interface
	// does not yet expose a Discard method.
	return "", fmt.Errorf("git discard is not yet supported via the chat interface")
}

// ---------------------------------------------------------------------------
// Navigation & search handlers
// ---------------------------------------------------------------------------
func (e *ToolExecutor) navigateTo(args map[string]any) (string, error) {
	path := getString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	// Validate the path is inside the repo, but navigation itself is a
	// message for the TUI — we just return the validated relative path.
	if _, err := e.jail.Validate(path); err != nil {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	return fmt.Sprintf("navigate:%s", path), nil
}

func (e *ToolExecutor) searchFiles(ctx context.Context, args map[string]any) (string, error) {
	pattern := getString(args, "pattern")
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	searchPath := getString(args, "path")
	if searchPath == "" {
		searchPath = "."
	}
	resolved, err := e.jail.Validate(searchPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", searchPath)
	}
	var matches []string
	walkErr := filepath.WalkDir(resolved, func(p string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return nil //nolint:nilerr // error returned as tool result
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		matched, matchErr := filepath.Match(pattern, d.Name())
		if matchErr != nil {
			return nil //nolint:nilerr // error returned as tool result
		}
		if matched {
			if len(matches) >= maxListEntries {
				return fmt.Errorf("search capped at %d entries", maxListEntries)
			}
			rel, relErr := filepath.Rel(e.jail.Root(), p)
			if relErr != nil {
				return nil //nolint:nilerr // error returned as tool result
			}
			matches = append(matches, filepath.ToSlash(rel))
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("search files: %w", walkErr)
	}
	if len(matches) == 0 {
		return "no files matched", nil
	}
	return strings.Join(matches, "\n"), nil
}

func (e *ToolExecutor) searchContent(ctx context.Context, args map[string]any) (string, error) {
	pattern := getString(args, "pattern")
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	// CR-001: Limit regex pattern length to prevent memory exhaustion
	// during compilation. Go's RE2 engine has no backtracking, but very
	// large patterns still consume significant memory to compile.
	const maxPatternLen = 1000
	if len(pattern) > maxPatternLen {
		return "", fmt.Errorf("pattern too long: %d bytes (max %d)", len(pattern), maxPatternLen)
	}
	searchPath := getString(args, "path")
	if searchPath == "" {
		searchPath = "."
	}
	resolved, err := e.jail.Validate(searchPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %s", searchPath)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}
	type contentMatch struct {
		File string `json:"file"`
		Text string `json:"text"`
		Line int    `json:"line"`
	}
	var matches []contentMatch
	const maxMatches = 100
	walkErr := filepath.WalkDir(resolved, func(p string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil //nolint:nilerr // error returned as tool result
		}
		if len(matches) >= maxMatches {
			return filepath.SkipAll
		}
		// CR-019: Compute relative path early and skip sensitive files
		// to prevent .env/SSH key contents from appearing in search results.
		rel, relErr := filepath.Rel(e.jail.Root(), p)
		if relErr != nil {
			return nil //nolint:nilerr // skip files with unresolvable paths
		}
		rel = filepath.ToSlash(rel)
		if err := mcp.IsSensitivePath(rel); err != nil {
			return nil //nolint:nilerr // skip sensitive files
		}
		// Skip binary-looking or very large files.
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > 1<<20 { // 1 MiB limit
			return nil //nolint:nilerr // error returned as tool result
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil //nolint:nilerr // error returned as tool result
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, contentMatch{
					File: rel,
					Line: i + 1,
					Text: truncate(strings.TrimSpace(line), 200),
				})
				if len(matches) >= maxMatches {
					break
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("search content: %w", walkErr)
	}
	if len(matches) == 0 {
		return "no matches found", nil
	}
	return toJSON(matches)
}

func (e *ToolExecutor) explain(args map[string]any) (string, error) {
	topic := getString(args, "topic")
	if topic == "" {
		return "", fmt.Errorf("topic is required")
	}
	// explain is a pass-through tool — the AI generates the explanation
	// in its response, so we just echo the topic back as acknowledgment.
	return fmt.Sprintf("explain: %s", topic), nil
}

// ---------------------------------------------------------------------------
// Bulk operation handlers
// ---------------------------------------------------------------------------
func (e *ToolExecutor) bulkStage(ctx context.Context, args map[string]any) (string, error) {
	patterns := getStringSlice(args, "patterns")
	if len(patterns) == 0 {
		return "", fmt.Errorf("patterns is required")
	}
	// Expand glob patterns to matching file paths.
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(e.jail.Root(), pattern))
		if err != nil {
			return "", fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		for _, m := range matches {
			rel, relErr := filepath.Rel(e.jail.Root(), m)
			if relErr != nil {
				continue
			}
			paths = append(paths, rel)
		}
	}
	if len(paths) == 0 {
		return "no files matched the patterns", nil
	}
	if err := e.client.Stage(ctx, paths); err != nil {
		return "", fmt.Errorf("git stage: %w", err)
	}
	return fmt.Sprintf("staged %d file(s)", len(paths)), nil
}

func (e *ToolExecutor) bulkDelete(_ context.Context, args map[string]any) (string, error) {
	paths := getStringSlice(args, "paths")
	if len(paths) == 0 {
		return "", fmt.Errorf("paths is required")
	}
	var deleted int
	var errs []string
	for _, p := range paths {
		if err := mcp.IsSensitivePath(p); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		resolved, err := e.jail.Validate(p)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid path", p))
			continue
		}
		if err := os.Remove(resolved); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		deleted++
	}
	result := fmt.Sprintf("deleted %d file(s)", deleted)
	if len(errs) > 0 {
		result += "\nerrors:\n" + strings.Join(errs, "\n")
	}
	return result, nil
}

func (e *ToolExecutor) bulkRename(_ context.Context, args map[string]any) (string, error) {
	renames, ok := args["renames"]
	if !ok {
		return "", fmt.Errorf("renames is required")
	}
	renameSlice, ok := renames.([]any)
	if !ok {
		return "", fmt.Errorf("renames must be an array")
	}
	var renamed int
	var errs []string
	for _, item := range renameSlice {
		m, ok := item.(map[string]any)
		if !ok {
			errs = append(errs, "invalid rename entry")
			continue
		}
		oldPath := getString(m, "old")
		newPath := getString(m, "new")
		if oldPath == "" || newPath == "" {
			errs = append(errs, "rename entry missing old or new path")
			continue
		}
		if err := mcp.IsSensitivePath(oldPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", oldPath, err))
			continue
		}
		if err := mcp.IsSensitivePath(newPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", newPath, err))
			continue
		}
		resolvedOld, err := e.jail.Validate(oldPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid path", oldPath))
			continue
		}
		resolvedNew, err := e.jail.Validate(newPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid path", newPath))
			continue
		}
		if mkErr := os.MkdirAll(filepath.Dir(resolvedNew), 0o755); mkErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", newPath, mkErr))
			continue
		}
		if err := os.Rename(resolvedOld, resolvedNew); err != nil {
			errs = append(errs, fmt.Sprintf("%s → %s: %v", oldPath, newPath, err))
			continue
		}
		renamed++
	}
	result := fmt.Sprintf("renamed %d file(s)", renamed)
	if len(errs) > 0 {
		result += "\nerrors:\n" + strings.Join(errs, "\n")
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Argument extraction helpers
// ---------------------------------------------------------------------------
// getString safely extracts a string value from the argument map.
func getString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// getBool safely extracts a boolean value from the argument map.
func getBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// getInt safely extracts an integer value from the argument map.
// JSON numbers arrive as float64, so we handle that conversion.
func getInt(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// getStringSlice safely extracts a []string from the argument map.
// JSON arrays arrive as []any, so we convert element-wise.
func getStringSlice(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------
// toJSON marshals v to indented JSON for AI-readable output.
func toJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(data), nil
}

// truncate returns s shortened to maxLen characters with a trailing "…" if
// it was truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
