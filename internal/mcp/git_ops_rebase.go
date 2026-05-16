package mcp

import (
	"context"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerRebaseTools registers rebase-related git tools.
func registerRebaseTools(s *Server) {
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
}
