package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

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

	return cmd
}
