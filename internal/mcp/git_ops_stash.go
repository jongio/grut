package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerStashTools registers stash-related git tools.
func registerStashTools(s *Server) {
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
}
