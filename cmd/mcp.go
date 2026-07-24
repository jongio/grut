package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	grut_mcp "github.com/jongio/grut/internal/mcp"
	"github.com/spf13/cobra"
)

// newMCPCmd creates the MCP server command.
func newMCPCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run grut as an MCP server",
		Long: `Start grut in MCP (Model Context Protocol) server mode for AI agent integration.

By default the server communicates over stdin/stdout using JSON-RPC (headless mode).
This is suitable for use as a subprocess by AI agents and IDEs.

Use --socket to serve over a Unix domain socket (future).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if socketPath != "" {
				return fmt.Errorf("socket mode is not yet implemented")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Override AI if --no-ai flag is set.
			applyNoAIFlag(cmd, cfg)

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			gc, err := git.NewClient(cwd)
			if err != nil {
				return fmt.Errorf("create git client: %w", err)
			}

			repoRoot, err := gc.RepoRoot(context.Background())
			if err != nil {
				return fmt.Errorf("find repo root: %w", err)
			}

			srv, err := grut_mcp.NewServer(gc, repoRoot, cfg)
			if err != nil {
				return fmt.Errorf("create MCP server: %w", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return srv.Serve(ctx)
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path for TUI+MCP mode (not yet implemented)")
	cmd.AddCommand(newMCPToolsCmd())

	return cmd
}

const mcpToolsUse = "tools"

func newMCPToolsCmd() *cobra.Command {
	var jsonOut bool
	var filter string

	cmd := &cobra.Command{
		Use:   mcpToolsUse,
		Short: "List grut MCP tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			tools := filterMCPTools(grut_mcp.ToolInventory(), filter)
			w := cmd.OutOrStdout()
			if jsonOut {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(tools)
			}
			fmt.Fprintf(w, "%-24s %-6s %s\n", "NAME", "TYPE", "DESCRIPTION")
			for _, tool := range tools {
				fmt.Fprintf(w, "%-24s %-6s %s\n", tool.Name, tool.Category, tool.Description)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print tools as JSON")
	cmd.Flags().StringVar(&filter, "filter", "", "Only show tools matching this text")
	return cmd
}

func filterMCPTools(tools []grut_mcp.ToolInfo, filter string) []grut_mcp.ToolInfo {
	query := strings.TrimSpace(strings.ToLower(filter))
	if query == "" {
		return tools
	}

	out := []grut_mcp.ToolInfo{}
	for _, tool := range tools {
		if strings.Contains(strings.ToLower(tool.Name), query) ||
			strings.Contains(strings.ToLower(tool.Category), query) ||
			strings.Contains(strings.ToLower(tool.Description), query) {
			out = append(out, tool)
		}
	}
	return out
}
