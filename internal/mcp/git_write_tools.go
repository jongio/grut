package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerGitWriteTools registers write/mutating git tools for staging,
// committing, branching, checkout, and remote operations.
func registerGitWriteTools(s *Server) {
	// git_stage
	s.addTool("git_stage", categoryWrite,
		mcplib.NewTool("git_stage",
			mcplib.WithDescription("Stage files for commit"),
			mcplib.WithArray("paths", mcplib.Required(), mcplib.Description("File paths to stage"),
				mcplib.Items(map[string]any{"type": "string"})),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			paths, err := req.RequireStringSlice("paths")
			if err != nil {
				return mcplib.NewToolResultError("paths is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPaths, err := s.validateGitPathsInJail(paths)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			if err := s.git.Stage(ctx, resolvedPaths); err != nil {
				return toolError("git stage", err)
			}
			return mcplib.NewToolResultText("staged successfully"), nil
		},
	)

	// git_unstage
	s.addTool("git_unstage", categoryWrite,
		mcplib.NewTool("git_unstage",
			mcplib.WithDescription("Unstage files from the index"),
			mcplib.WithArray("paths", mcplib.Required(), mcplib.Description("File paths to unstage"),
				mcplib.Items(map[string]any{"type": "string"})),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			paths, err := req.RequireStringSlice("paths")
			if err != nil {
				return mcplib.NewToolResultError("paths is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolvedPaths, err := s.validateGitPathsInJail(paths)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			if err := s.git.Unstage(ctx, resolvedPaths); err != nil {
				return toolError("git unstage", err)
			}
			return mcplib.NewToolResultText("unstaged successfully"), nil
		},
	)

	// git_commit
	s.addTool("git_commit", categoryWrite,
		mcplib.NewTool("git_commit",
			mcplib.WithDescription("Create a commit with staged changes"),
			mcplib.WithString("message", mcplib.Required(), mcplib.Description("Commit message")),
			mcplib.WithBoolean("amend", mcplib.Description("Amend the previous commit")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			msg, err := req.RequireString("message")
			if err != nil || strings.TrimSpace(msg) == "" {
				return mcplib.NewToolResultError("message is required and must not be empty"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := validateGitMessage(msg); err != nil {
				return mcplib.NewToolResultErrorf("invalid commit message: %v", err), nil
			}
			opts := git.CommitOpts{
				Amend: req.GetBool("amend", false),
			}
			hash, err := s.git.Commit(ctx, msg, opts)
			if err != nil {
				return toolError("git commit", err)
			}
			return jsonResult(map[string]string{"hash": hash})
		},
	)

	// git_branch_create
	s.addTool("git_branch_create", categoryWrite,
		mcplib.NewTool("git_branch_create",
			mcplib.WithDescription("Create a new branch"),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Branch name")),
			mcplib.WithString("base", mcplib.Description("Base ref for the new branch (default HEAD)")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcplib.NewToolResultError("name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			const maxBranchNameLen = 250
			if len(name) > maxBranchNameLen {
				return mcplib.NewToolResultErrorf("branch name exceeds maximum length of %d characters", maxBranchNameLen), nil
			}
			base := req.GetString("base", "")
			if err := s.git.BranchCreate(ctx, name, base); err != nil {
				return toolError("git branch create", err)
			}
			return mcplib.NewToolResultText("branch created: " + name), nil
		},
	)

	// git_branch_delete
	s.addTool("git_branch_delete", categoryWrite,
		mcplib.NewTool("git_branch_delete",
			mcplib.WithDescription("Delete a branch"),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Branch name to delete")),
			mcplib.WithBoolean("force", mcplib.Description("Force delete even if not fully merged")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcplib.NewToolResultError("name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			force := req.GetBool("force", false)
			if err := s.git.BranchDelete(ctx, name, force); err != nil {
				return toolError("git branch delete", err)
			}
			return mcplib.NewToolResultText("branch deleted: " + name), nil
		},
	)

	// git_branch_rename
	s.addTool("git_branch_rename", categoryWrite,
		mcplib.NewTool("git_branch_rename",
			mcplib.WithDescription("Rename a branch"),
			mcplib.WithString("old_name", mcplib.Required(), mcplib.Description("Current branch name")),
			mcplib.WithString("new_name", mcplib.Required(), mcplib.Description("New branch name")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			oldName, err := req.RequireString("old_name")
			if err != nil {
				return mcplib.NewToolResultError("old_name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			newName, err := req.RequireString("new_name")
			if err != nil {
				return mcplib.NewToolResultError("new_name is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := s.git.BranchRename(ctx, oldName, newName); err != nil {
				return toolError("git branch rename", err)
			}
			return mcplib.NewToolResultText(fmt.Sprintf("branch renamed: %s -> %s", oldName, newName)), nil
		},
	)

	// git_checkout
	s.addTool("git_checkout", categoryWrite,
		mcplib.NewTool("git_checkout",
			mcplib.WithDescription("Checkout a branch, tag, or commit"),
			mcplib.WithString("ref", mcplib.Required(), mcplib.Description("Git ref to checkout")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			ref, err := req.RequireString("ref")
			if err != nil {
				return mcplib.NewToolResultError("ref is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			if err := git.ValidateRef(ref); err != nil {
				return mcplib.NewToolResultErrorf("invalid ref: %v", err), nil
			}
			if err := s.git.Checkout(ctx, ref); err != nil {
				return toolError("git checkout", err)
			}
			return mcplib.NewToolResultText("checked out: " + ref), nil
		},
	)

	// git_push
	s.addTool("git_push", categoryWrite,
		mcplib.NewTool("git_push",
			mcplib.WithDescription("Push commits to a remote"),
			mcplib.WithString("remote", mcplib.Description("Remote name (default origin)")),
			mcplib.WithString("branch", mcplib.Description("Branch to push")),
			mcplib.WithBoolean("force", mcplib.Description("Force push")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			opts := git.PushOpts{
				Remote: req.GetString("remote", ""),
				Branch: req.GetString("branch", ""),
				Force:  req.GetBool("force", false),
			}
			if err := s.git.Push(ctx, opts); err != nil {
				return toolError("git push", err)
			}
			return mcplib.NewToolResultText("pushed successfully"), nil
		},
	)

	// git_pull
	s.addTool("git_pull", categoryWrite,
		mcplib.NewTool("git_pull",
			mcplib.WithDescription("Pull changes from a remote"),
			mcplib.WithString("remote", mcplib.Description("Remote name (default origin)")),
			mcplib.WithString("branch", mcplib.Description("Branch to pull")),
			mcplib.WithBoolean("rebase", mcplib.Description("Rebase instead of merge")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			opts := git.PullOpts{
				Remote: req.GetString("remote", ""),
				Branch: req.GetString("branch", ""),
				Rebase: req.GetBool("rebase", false),
			}
			if err := s.git.Pull(ctx, opts); err != nil {
				return toolError("git pull", err)
			}
			return mcplib.NewToolResultText("pulled successfully"), nil
		},
	)

	// git_fetch
	s.addTool("git_fetch", categoryWrite,
		mcplib.NewTool("git_fetch",
			mcplib.WithDescription("Fetch refs and objects from a remote"),
			mcplib.WithString("remote", mcplib.Description("Remote name")),
			mcplib.WithBoolean("prune", mcplib.Description("Prune deleted remote branches")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			opts := git.FetchOpts{
				Remote: req.GetString("remote", ""),
				Prune:  req.GetBool("prune", false),
			}
			if err := s.git.Fetch(ctx, opts); err != nil {
				return toolError("git fetch", err)
			}
			return mcplib.NewToolResultText("fetched successfully"), nil
		},
	)
}
