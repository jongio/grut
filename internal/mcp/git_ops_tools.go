package mcp

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// maxGitMessageLen is the maximum allowed length for freeform git messages
// (stash, tag, merge commit messages) to prevent abuse.
const maxGitMessageLen = 10_000

// validateGitMessage validates a freeform git message (stash, tag, commit).
// It rejects null bytes, control characters (except \n, \r, \t), and
// messages exceeding the maximum length.
func validateGitMessage(msg string) error {
	if len(msg) > maxGitMessageLen {
		return fmt.Errorf("message exceeds maximum length of %d characters", maxGitMessageLen)
	}
	if !utf8.ValidString(msg) {
		return fmt.Errorf("message contains invalid UTF-8")
	}
	for i, r := range msg {
		if r == 0 {
			return fmt.Errorf("message contains null byte at position %d", i)
		}
		// Allow common whitespace: \n (10), \r (13), \t (9).
		if r != '\n' && r != '\r' && r != '\t' && r < 0x20 {
			return fmt.Errorf("message contains control character at position %d", i)
		}
	}
	return nil
}

// registerGitOpsTools registers write/mutating git tools for merge, rebase,
// cherry-pick, stash, tag, worktree, and bisect operations.
func registerGitOpsTools(s *Server) {
	// git_merge
	s.addTool(
		"git_merge", categoryWrite,
		mcplib.NewTool(
			"git_merge",
			mcplib.WithDescription("Merge a branch into the current branch"),
			mcplib.WithString("branch", mcplib.Required(), mcplib.Description("Branch to merge")),
			mcplib.WithBoolean("no_ff", mcplib.Description("Create a merge commit even for fast-forward merges")),
			mcplib.WithBoolean("squash", mcplib.Description("Squash commits into a single commit")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			branch, err := req.RequireString("branch")
			if err != nil || strings.TrimSpace(branch) == "" {
				return mcplib.NewToolResultError("branch is required and must not be empty"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := git.ValidateRef(branch); err != nil {
				return mcplib.NewToolResultErrorf("invalid branch ref: %v", err), nil
			}
			opts := git.MergeOpts{
				NoFF:   req.GetBool("no_ff", false),
				Squash: req.GetBool("squash", false),
			}
			if err := s.git.Merge(ctx, branch, opts); err != nil {
				return toolError("git merge", err)
			}
			return mcplib.NewToolResultText("merged: " + branch), nil
		},
	)

	// git_merge_abort
	s.addTool(
		"git_merge_abort", categoryWrite,
		mcplib.NewTool(
			"git_merge_abort",
			mcplib.WithDescription("Abort an in-progress merge"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.MergeAbort(ctx); err != nil {
				return toolError("git merge abort", err)
			}
			return mcplib.NewToolResultText("merge aborted"), nil
		},
	)

	// git_rebase
	s.addTool(
		"git_rebase", categoryWrite,
		mcplib.NewTool(
			"git_rebase",
			mcplib.WithDescription("Rebase the current branch onto another ref"),
			mcplib.WithString("onto", mcplib.Required(), mcplib.Description("Ref to rebase onto")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			onto, err := req.RequireString("onto")
			if err != nil {
				return mcplib.NewToolResultError("onto is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := git.ValidateRef(onto); err != nil {
				return mcplib.NewToolResultErrorf("invalid ref: %v", err), nil
			}
			if err := s.git.Rebase(ctx, onto, git.RebaseOpts{}); err != nil {
				return toolError("git rebase", err)
			}
			return mcplib.NewToolResultText("rebased onto: " + onto), nil
		},
	)

	// git_rebase_continue
	s.addTool(
		"git_rebase_continue", categoryWrite,
		mcplib.NewTool(
			"git_rebase_continue",
			mcplib.WithDescription("Continue an in-progress rebase after resolving conflicts"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.RebaseContinue(ctx); err != nil {
				return toolError("git rebase continue", err)
			}
			return mcplib.NewToolResultText("rebase continued"), nil
		},
	)

	// git_rebase_abort
	s.addTool(
		"git_rebase_abort", categoryWrite,
		mcplib.NewTool(
			"git_rebase_abort",
			mcplib.WithDescription("Abort an in-progress rebase"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.RebaseAbort(ctx); err != nil {
				return toolError("git rebase abort", err)
			}
			return mcplib.NewToolResultText("rebase aborted"), nil
		},
	)

	// git_cherry_pick
	s.addTool(
		"git_cherry_pick", categoryWrite,
		mcplib.NewTool(
			"git_cherry_pick",
			mcplib.WithDescription("Apply a commit by its hash to the current branch"),
			mcplib.WithString("commit", mcplib.Required(), mcplib.Description("Commit hash to cherry-pick")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			commit, err := req.RequireString("commit")
			if err != nil {
				return mcplib.NewToolResultError("commit is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := git.ValidateRef(commit); err != nil {
				return mcplib.NewToolResultErrorf("invalid commit ref: %v", err), nil
			}
			if err := s.git.CherryPick(ctx, commit); err != nil {
				return toolError("git cherry-pick", err)
			}
			return mcplib.NewToolResultText("cherry-picked: " + commit), nil
		},
	)

	// git_stash_list
	s.addTool(
		"git_stash_list", categoryRead,
		mcplib.NewTool(
			"git_stash_list",
			mcplib.WithDescription("List all stash entries"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			entries, err := s.git.StashList(ctx)
			if err != nil {
				return toolError("git stash list", err)
			}
			if len(entries) == 0 {
				return mcplib.NewToolResultText("no stash entries"), nil
			}
			var sb strings.Builder
			for _, e := range entries {
				hash := e.Hash
				if len(hash) > git.ShortHashLen {
					hash = hash[:git.ShortHashLen]
				}
				fmt.Fprintf(&sb, "stash@{%d}: %s (%s)\n", e.Index, e.Message, hash)
			}
			return mcplib.NewToolResultText(sb.String()), nil
		},
	)

	// git_stash_show
	s.addTool(
		"git_stash_show", categoryRead,
		mcplib.NewTool(
			"git_stash_show",
			mcplib.WithDescription("Show the diff of a stash entry"),
			mcplib.WithNumber("index", mcplib.Description("Stash index to show (default 0)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			index := req.GetInt("index", 0)
			if index < 0 {
				return mcplib.NewToolResultError("stash index must not be negative"), nil
			}
			diff, err := s.git.StashShow(ctx, index)
			if err != nil {
				return toolError("git stash show", err)
			}
			if diff == "" {
				return mcplib.NewToolResultText("empty stash diff"), nil
			}
			return mcplib.NewToolResultText(diff), nil
		},
	)

	// git_stash_push
	s.addTool(
		"git_stash_push", categoryWrite,
		mcplib.NewTool(
			"git_stash_push",
			mcplib.WithDescription("Stash the current working directory changes"),
			mcplib.WithString(fieldMessage, mcplib.Description("Stash message")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			msg := req.GetString(fieldMessage, "")
			if msg != "" {
				if err := validateGitMessage(msg); err != nil {
					return mcplib.NewToolResultErrorf("invalid stash message: %v", err), nil
				}
			}
			opts := git.StashOpts{
				Message: msg,
			}
			if err := s.git.StashPush(ctx, opts); err != nil {
				return toolError("git stash push", err)
			}
			return mcplib.NewToolResultText("stashed successfully"), nil
		},
	)

	// git_stash_pop
	s.addTool(
		"git_stash_pop", categoryWrite,
		mcplib.NewTool(
			"git_stash_pop",
			mcplib.WithDescription("Apply and remove the top stash entry"),
			mcplib.WithNumber("index", mcplib.Description("Stash index to pop (default 0)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			index := req.GetInt("index", 0)
			if index < 0 {
				return mcplib.NewToolResultError("stash index must not be negative"), nil
			}
			if err := s.git.StashPop(ctx, index); err != nil {
				return toolError("git stash pop", err)
			}
			return mcplib.NewToolResultText("stash popped"), nil
		},
	)

	// git_stash_apply
	s.addTool(
		"git_stash_apply", categoryWrite,
		mcplib.NewTool(
			"git_stash_apply",
			mcplib.WithDescription("Apply a stash entry without removing it"),
			mcplib.WithNumber("index", mcplib.Description("Stash index to apply (default 0)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			index := req.GetInt("index", 0)
			if index < 0 {
				return mcplib.NewToolResultError("stash index must not be negative"), nil
			}
			if err := s.git.StashApply(ctx, index); err != nil {
				return toolError("git stash apply", err)
			}
			return mcplib.NewToolResultText("stash applied"), nil
		},
	)

	// git_stash_drop
	s.addTool(
		"git_stash_drop", categoryWrite,
		mcplib.NewTool(
			"git_stash_drop",
			mcplib.WithDescription("Remove a stash entry without applying it"),
			mcplib.WithNumber("index", mcplib.Description("Stash index to drop (default 0)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			index := req.GetInt("index", 0)
			if index < 0 {
				return mcplib.NewToolResultError("stash index must not be negative"), nil
			}
			if err := s.git.StashDrop(ctx, index); err != nil {
				return toolError("git stash drop", err)
			}
			return mcplib.NewToolResultText("stash dropped"), nil
		},
	)

	// git_tag_create
	s.addTool(
		"git_tag_create", categoryWrite,
		mcplib.NewTool(
			"git_tag_create",
			mcplib.WithDescription("Create a new tag"),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Tag name")),
			mcplib.WithString("ref", mcplib.Description("Ref to tag (default HEAD)")),
			mcplib.WithString(fieldMessage, mcplib.Description("Annotation message (creates annotated tag if provided)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcplib.NewToolResultError("name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			ref := req.GetString("ref", "")
			message := req.GetString(fieldMessage, "")
			if message != "" {
				if err := validateGitMessage(message); err != nil {
					return mcplib.NewToolResultErrorf("invalid tag message: %v", err), nil
				}
			}
			if err := s.git.TagCreate(ctx, name, ref, message); err != nil {
				return toolError("git tag create", err)
			}
			return mcplib.NewToolResultText("tag created: " + name), nil
		},
	)

	// git_tag_delete
	s.addTool(
		"git_tag_delete", categoryWrite,
		mcplib.NewTool(
			"git_tag_delete",
			mcplib.WithDescription("Delete a tag"),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Tag name to delete")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcplib.NewToolResultError("name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := s.git.TagDelete(ctx, name); err != nil {
				return toolError("git tag delete", err)
			}
			return mcplib.NewToolResultText("tag deleted: " + name), nil
		},
	)

	// git_worktree_add
	s.addTool(
		"git_worktree_add", categoryWrite,
		mcplib.NewTool(
			"git_worktree_add",
			mcplib.WithDescription("Add a new worktree"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("Path for the new worktree")),
			mcplib.WithString("branch", mcplib.Description("Branch to checkout in the worktree")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPath, err := s.validateGitPathInJail(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			branch := req.GetString("branch", "")
			if err := s.git.WorktreeAdd(ctx, resolvedPath, branch); err != nil {
				return toolError("git worktree add", err)
			}
			return mcplib.NewToolResultText("worktree added: " + resolvedPath), nil
		},
	)

	// git_worktree_remove
	s.addTool(
		"git_worktree_remove", categoryWrite,
		mcplib.NewTool(
			"git_worktree_remove",
			mcplib.WithDescription("Remove a worktree"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("Worktree path to remove")),
			mcplib.WithBoolean("force", mcplib.Description("Force removal even with changes")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPath, err := s.validateGitPathInJail(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			force := req.GetBool("force", false)
			if err := s.git.WorktreeRemove(ctx, resolvedPath, force); err != nil {
				return toolError("git worktree remove", err)
			}
			return mcplib.NewToolResultText("worktree removed: " + resolvedPath), nil
		},
	)

	// git_bisect_start
	s.addTool(
		"git_bisect_start", categoryWrite,
		mcplib.NewTool(
			"git_bisect_start",
			mcplib.WithDescription("Start a bisect session"),
			mcplib.WithString("bad", mcplib.Required(), mcplib.Description("Known bad commit")),
			mcplib.WithString("good", mcplib.Required(), mcplib.Description("Known good commit")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			bad, err := req.RequireString("bad")
			if err != nil {
				return mcplib.NewToolResultError("bad is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			good, err := req.RequireString("good")
			if err != nil {
				return mcplib.NewToolResultError("good is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := s.git.BisectStart(ctx, bad, good); err != nil {
				return toolError("git bisect start", err)
			}
			return mcplib.NewToolResultText("bisect started"), nil
		},
	)

	// git_bisect_good
	s.addTool(
		"git_bisect_good", categoryWrite,
		mcplib.NewTool(
			"git_bisect_good",
			mcplib.WithDescription("Mark the current revision as good during bisect"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			result, err := s.git.BisectGood(ctx)
			if err != nil {
				return toolError("git bisect good", err)
			}
			return mcplib.NewToolResultText(result), nil
		},
	)

	// git_bisect_bad
	s.addTool(
		"git_bisect_bad", categoryWrite,
		mcplib.NewTool(
			"git_bisect_bad",
			mcplib.WithDescription("Mark the current revision as bad during bisect"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			result, err := s.git.BisectBad(ctx)
			if err != nil {
				return toolError("git bisect bad", err)
			}
			return mcplib.NewToolResultText(result), nil
		},
	)

	// git_bisect_reset
	s.addTool(
		"git_bisect_reset", categoryWrite,
		mcplib.NewTool(
			"git_bisect_reset",
			mcplib.WithDescription("End the bisect session and return to the original HEAD"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.BisectReset(ctx); err != nil {
				return toolError("git bisect reset", err)
			}
			return mcplib.NewToolResultText("bisect reset"), nil
		},
	)

	// git_discard
	s.addTool(
		"git_discard", categoryWrite,
		mcplib.NewTool(
			"git_discard",
			mcplib.WithDescription("Discard unstaged changes for a file, restoring it to the index state"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path to discard changes for")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPath, err := s.validateGitPathInJail(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			if err := s.git.DiscardFile(ctx, resolvedPath); err != nil {
				return toolError("git discard", err)
			}
			return mcplib.NewToolResultText("discarded changes: " + resolvedPath), nil
		},
	)

	// git_discard_all
	s.addTool(
		"git_discard_all", categoryWrite,
		mcplib.NewTool(
			"git_discard_all",
			mcplib.WithDescription("Discard all unstaged changes in the working tree"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.DiscardAllUnstaged(ctx); err != nil {
				return toolError("git discard all", err)
			}
			return mcplib.NewToolResultText("discarded all unstaged changes"), nil
		},
	)

	// git_revert
	s.addTool(
		"git_revert", categoryWrite,
		mcplib.NewTool(
			"git_revert",
			mcplib.WithDescription("Create a new commit that undoes the changes from a given commit"),
			mcplib.WithString(paramHash, mcplib.Required(), mcplib.Description("Commit hash to revert")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			hash, err := req.RequireString(paramHash)
			if err != nil {
				return mcplib.NewToolResultError("hash is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := s.git.Revert(ctx, hash); err != nil {
				return toolError("git revert", err)
			}
			return mcplib.NewToolResultText("reverted: " + hash), nil
		},
	)

	// git_revert_continue
	s.addTool(
		"git_revert_continue", categoryWrite,
		mcplib.NewTool(
			"git_revert_continue",
			mcplib.WithDescription("Continue a revert after resolving conflicts"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.RevertContinue(ctx); err != nil {
				return toolError("git revert continue", err)
			}
			return mcplib.NewToolResultText("revert continued"), nil
		},
	)

	// git_revert_abort
	s.addTool(
		"git_revert_abort", categoryWrite,
		mcplib.NewTool(
			"git_revert_abort",
			mcplib.WithDescription("Abort an in-progress revert and restore the previous state"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if err := s.git.RevertAbort(ctx); err != nil {
				return toolError("git revert abort", err)
			}
			return mcplib.NewToolResultText("revert aborted"), nil
		},
	)

	// git_reset
	s.addTool(
		"git_reset", categoryWrite,
		mcplib.NewTool(
			"git_reset",
			mcplib.WithDescription("Reset the current branch to a ref. Modes: soft (HEAD only), mixed (HEAD + index), hard (HEAD + index + working tree)"),
			mcplib.WithString("ref", mcplib.Required(), mcplib.Description("Target ref (commit hash, branch, tag, HEAD~N, etc.)")),
			mcplib.WithString("mode", mcplib.Required(), mcplib.Description("Reset mode: soft, mixed, or hard")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			ref, err := req.RequireString("ref")
			if err != nil {
				return mcplib.NewToolResultError("ref is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			modeStr, err := req.RequireString("mode")
			if err != nil {
				return mcplib.NewToolResultError("mode is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			mode := git.ResetMode(modeStr)
			if err := s.git.Reset(ctx, ref, mode); err != nil {
				return toolError("git reset", err)
			}
			return mcplib.NewToolResultText(fmt.Sprintf("reset --%s to %s", mode, ref)), nil
		},
	)

	// git_stage_hunk
	s.addTool(
		"git_stage_hunk", categoryWrite,
		mcplib.NewTool(
			"git_stage_hunk",
			mcplib.WithDescription("Stage a single hunk from an unstaged file's diff. Get the hunk index from git_diff output (0-based)."),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path with unstaged changes")),
			mcplib.WithNumber("hunk_index", mcplib.Required(), mcplib.Description("Zero-based index of the hunk to stage")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPath, err := s.validateGitPathInJail(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			hunkIdx := req.GetInt("hunk_index", -1)
			if hunkIdx < 0 {
				return mcplib.NewToolResultError("hunk_index is required and must be >= 0"), nil
			}
			diffs, err := s.git.Diff(ctx, git.DiffOpts{Path: resolvedPath})
			if err != nil {
				return toolError("git diff for stage_hunk", err)
			}
			if len(diffs) == 0 || hunkIdx >= len(diffs[0].Hunks) {
				return mcplib.NewToolResultError(fmt.Sprintf("hunk_index %d out of range", hunkIdx)), nil
			}
			if err := s.git.StageHunk(ctx, resolvedPath, diffs[0].Hunks[hunkIdx]); err != nil {
				return toolError("git stage hunk", err)
			}
			return mcplib.NewToolResultText(fmt.Sprintf("staged hunk %d of %s", hunkIdx, resolvedPath)), nil
		},
	)

	// git_unstage_hunk
	s.addTool(
		"git_unstage_hunk", categoryWrite,
		mcplib.NewTool(
			"git_unstage_hunk",
			mcplib.WithDescription("Unstage a single hunk from a staged file's diff. Get the hunk index from git_diff --cached output (0-based)."),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path with staged changes")),
			mcplib.WithNumber("hunk_index", mcplib.Required(), mcplib.Description("Zero-based index of the hunk to unstage")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPath, err := s.validateGitPathInJail(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			hunkIdx := req.GetInt("hunk_index", -1)
			if hunkIdx < 0 {
				return mcplib.NewToolResultError("hunk_index is required and must be >= 0"), nil
			}
			diffs, err := s.git.Diff(ctx, git.DiffOpts{Staged: true, Path: resolvedPath})
			if err != nil {
				return toolError("git diff for unstage_hunk", err)
			}
			if len(diffs) == 0 || hunkIdx >= len(diffs[0].Hunks) {
				return mcplib.NewToolResultError(fmt.Sprintf("hunk_index %d out of range", hunkIdx)), nil
			}
			if err := s.git.UnstageHunk(ctx, resolvedPath, diffs[0].Hunks[hunkIdx]); err != nil {
				return toolError("git unstage hunk", err)
			}
			return mcplib.NewToolResultText(fmt.Sprintf("unstaged hunk %d of %s", hunkIdx, resolvedPath)), nil
		},
	)
}
