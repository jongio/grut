package mcp

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerWorktreeTools registers worktree-related git tools.
func registerWorktreeTools(s *Server) {
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
}
