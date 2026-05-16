package mcp

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerTagTools registers tag-related git tools.
func registerTagTools(s *Server) {
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
}
