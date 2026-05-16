package mcp

import (
	"context"
	"fmt"
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
// cherry-pick, stash, tag, worktree, bisect, discard, revert, reset, and
// hunk staging operations.
func registerGitOpsTools(s *Server) {
	registerMergeTools(s)
	registerRebaseTools(s)
	registerCherryPickTools(s)
	registerStashTools(s)
	registerTagTools(s)
	registerWorktreeTools(s)
	registerBisectTools(s)
	registerDiscardTools(s)
	registerRevertTools(s)
	registerResetTools(s)
	registerHunkTools(s)
}

// registerDiscardTools registers discard-related git tools.
func registerDiscardTools(s *Server) {
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
}

// registerRevertTools registers revert-related git tools.
func registerRevertTools(s *Server) {
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
}

// registerResetTools registers reset-related git tools.
func registerResetTools(s *Server) {
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
}

// registerHunkTools registers hunk-level staging/unstaging git tools.
func registerHunkTools(s *Server) {
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
