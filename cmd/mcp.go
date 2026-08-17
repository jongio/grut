package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/diag"
	"github.com/jongio/grut/internal/git"
	grut_mcp "github.com/jongio/grut/internal/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

type mcpCommandServer interface {
	Serve(context.Context) error
}

type mcpWatchdog interface {
	Start(context.Context) func()
}

type stdioMCPServer struct {
	server *mcpserver.StdioServer
	input  io.Reader
	output io.Writer
}

func (s *stdioMCPServer) Serve(ctx context.Context) error {
	err := s.server.Listen(ctx, s.input, s.output)
	if ctx.Err() != nil && (err == nil || errors.Is(err, ctx.Err())) {
		return nil
	}
	return err
}

type mcpCommandDeps struct {
	buildServer   func(context.Context, *cobra.Command) (mcpCommandServer, error)
	newWatchdog   func() mcpWatchdog
	notifyContext func(context.Context) (context.Context, context.CancelFunc)
}

func defaultMCPCommandDeps() mcpCommandDeps {
	return mcpCommandDeps{
		buildServer: buildMCPCommandServer,
		newWatchdog: func() mcpWatchdog {
			return diag.New()
		},
		notifyContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(parent, os.Interrupt)
		},
	}
}

func buildMCPCommandServer(ctx context.Context, cmd *cobra.Command) (mcpCommandServer, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	applyNoAIFlag(cmd, cfg)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	gc, err := git.NewClient(cwd)
	if err != nil {
		return nil, fmt.Errorf("create git client: %w", err)
	}

	repoRoot, err := gc.RepoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("find repo root: %w", err)
	}

	srv, err := grut_mcp.NewServer(gc, repoRoot, cfg)
	if err != nil {
		return nil, fmt.Errorf("create MCP server: %w", err)
	}
	return &stdioMCPServer{
		server: mcpserver.NewStdioServer(srv.MCPServer()),
		input:  os.Stdin,
		output: os.Stdout,
	}, nil
}

// newMCPCmd creates the MCP server command.
func newMCPCmd() *cobra.Command {
	return newMCPCmdWithDeps(defaultMCPCommandDeps())
}

func newMCPCmdWithDeps(deps mcpCommandDeps) *cobra.Command {
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

			ctx, stopSignals := deps.notifyContext(cmd.Context())
			defer stopSignals()

			srv, err := deps.buildServer(ctx, cmd)
			if err != nil {
				return err
			}

			stopWatchdog := deps.newWatchdog().Start(ctx)
			defer stopWatchdog()

			err = srv.Serve(ctx)
			if ctx.Err() != nil && (err == nil || errors.Is(err, ctx.Err())) {
				return nil
			}
			return err
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
	if textFilterMatches(filter) {
		return tools
	}

	out := []grut_mcp.ToolInfo{}
	for _, tool := range tools {
		if textFilterMatches(filter, tool.Name, tool.Category, tool.Description) {
			out = append(out, tool)
		}
	}
	return out
}
