package mcp

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerBisectTools registers bisect-related git tools.
func registerBisectTools(s *Server) {
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
}
