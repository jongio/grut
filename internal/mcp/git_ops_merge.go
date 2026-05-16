package mcp

import (
	"context"
	"strings"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerMergeTools registers merge-related git tools.
func registerMergeTools(s *Server) {
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
}
