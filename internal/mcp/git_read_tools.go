package mcp

import (
	"context"

	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerGitReadTools registers all read-only git tools on the server.
func registerGitReadTools(s *Server) {
	// git_status
	s.addTool("git_status", categoryRead,
		mcplib.NewTool("git_status",
			mcplib.WithDescription("Returns the list of changed files with their git status codes"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			statuses, err := s.git.Status(ctx)
			if err != nil {
				return toolError("git status", err)
			}
			return jsonResult(statuses)
		},
	)

	// git_diff
	s.addTool("git_diff", categoryRead,
		mcplib.NewTool("git_diff",
			mcplib.WithDescription("Returns diff output for changed files"),
			mcplib.WithBoolean("staged", mcplib.Description("Compare staged changes against HEAD")),
			mcplib.WithString("path", mcplib.Description("Limit diff to a specific file path")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			opts := git.DiffOpts{
				Staged: req.GetBool("staged", false),
				Path:   req.GetString("path", ""),
			}
			if opts.Path != "" {
				resolvedPath, err := s.validateGitPathInJail(opts.Path)
				if err != nil {
					return mcplib.NewToolResultErrorf("path validation: %v", err), nil
				}
				opts.Path = resolvedPath
			}
			diffs, err := s.git.Diff(ctx, opts)
			if err != nil {
				return toolError("git diff", err)
			}
			// Cap serialized output to prevent unbounded memory/transfer.
			const maxDiffBytes = 5 * 1024 * 1024 // 5 MiB
			result, err := jsonResult(diffs)
			if err != nil {
				return result, err
			}
			if len(result.Content) > 0 {
				if tc, ok := result.Content[0].(mcplib.TextContent); ok && len(tc.Text) > maxDiffBytes {
					return mcplib.NewToolResultErrorf("diff output exceeds %d bytes limit; narrow the query with a path filter", maxDiffBytes), nil
				}
			}
			return result, nil
		},
	)

	// git_log
	s.addTool("git_log", categoryRead,
		mcplib.NewTool("git_log",
			mcplib.WithDescription("Returns the commit log"),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number of commits to return (max 10000)")),
			mcplib.WithString("path", mcplib.Description("Filter commits by file path")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			limit := req.GetInt("limit", 20)
			if limit <= 0 {
				limit = 20
			}
			const maxLogLimit = 10_000
			if limit > maxLogLimit {
				limit = maxLogLimit
			}
			opts := git.LogOpts{
				MaxCount: limit,
				Path:     req.GetString("path", ""),
			}
			if opts.Path != "" {
				resolvedPath, err := s.validateGitPathInJail(opts.Path)
				if err != nil {
					return mcplib.NewToolResultErrorf("path validation: %v", err), nil
				}
				opts.Path = resolvedPath
			}
			commits, err := s.git.Log(ctx, opts)
			if err != nil {
				return toolError("git log", err)
			}
			return jsonResult(commits)
		},
	)

	// git_blame
	s.addTool("git_blame", categoryRead,
		mcplib.NewTool("git_blame",
			mcplib.WithDescription("Returns per-line blame annotation for a file"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path to blame")),
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
			lines, err := s.git.Blame(ctx, resolvedPath)
			if err != nil {
				return toolError("git blame", err)
			}
			return jsonResult(lines)
		},
	)

	// git_branch_list
	s.addTool("git_branch_list", categoryRead,
		mcplib.NewTool("git_branch_list",
			mcplib.WithDescription("Returns the list of local and remote branches"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			branches, err := s.git.BranchList(ctx)
			if err != nil {
				return toolError("git branch list", err)
			}
			return jsonResult(branches)
		},
	)

	// git_tag_list
	s.addTool("git_tag_list", categoryRead,
		mcplib.NewTool("git_tag_list",
			mcplib.WithDescription("Returns the list of tags"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			tags, err := s.git.TagList(ctx)
			if err != nil {
				return toolError("git tag list", err)
			}
			return jsonResult(tags)
		},
	)

	// git_worktree_list
	s.addTool("git_worktree_list", categoryRead,
		mcplib.NewTool("git_worktree_list",
			mcplib.WithDescription("Returns the list of git worktrees"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			worktrees, err := s.git.WorktreeList(ctx)
			if err != nil {
				return toolError("git worktree list", err)
			}
			return jsonResult(worktrees)
		},
	)

	// git_reflog
	s.addTool("git_reflog", categoryRead,
		mcplib.NewTool("git_reflog",
			mcplib.WithDescription("Returns reflog entries for a ref"),
			mcplib.WithString("ref", mcplib.Description("Git ref to show reflog for (default HEAD)")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number of entries to return")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			ref := req.GetString("ref", "HEAD")
			limit := req.GetInt("limit", 20)
			entries, err := s.git.Reflog(ctx, ref, limit)
			if err != nil {
				return toolError("git reflog", err)
			}
			return jsonResult(entries)
		},
	)

	// git_is_repo
	s.addTool("git_is_repo", categoryRead,
		mcplib.NewTool("git_is_repo",
			mcplib.WithDescription("Returns whether the current directory is inside a git repository"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			isRepo, err := s.git.IsRepo(ctx)
			if err != nil {
				return toolError("git is_repo", err)
			}
			return jsonResult(map[string]bool{"is_repo": isRepo})
		},
	)

	// git_repo_root
	s.addTool("git_repo_root", categoryRead,
		mcplib.NewTool("git_repo_root",
			mcplib.WithDescription("Returns the absolute path to the repository root"),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			root, err := s.git.RepoRoot(ctx)
			if err != nil {
				return toolError("git repo_root", err)
			}
			return jsonResult(map[string]string{"root": root})
		},
	)
}
