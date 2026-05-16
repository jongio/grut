package mcp

import (
	"context"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerCherryPickTools registers cherry-pick-related git tools.
func registerCherryPickTools(s *Server) {
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
}
