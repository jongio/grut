package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	categoryRead  = "read"
	categoryWrite = "write"
)

// Server wraps an MCP protocol server with grut git and file tools,
// rate limiting, path jailing, and audit logging.
type Server struct {
	git      git.GitClient
	mcp      *mcpserver.MCPServer
	cfg      *config.Config
	jail     *PathJail
	limiter  *RateLimiter
	audit    *AuditLogger
	repoRoot string
	tools    []ToolInfo
}

// Default rate limits (calls per minute) when not configured.
const (
	defaultReadRatePerMin  = 1000
	defaultWriteRatePerMin = 100
)

// NewServer creates a fully configured MCP server with all git and file
// tools registered. The server is ready to serve over stdin/stdout.
func NewServer(gitClient git.GitClient, repoRoot string, cfg *config.Config) (*Server, error) {
	jail, err := NewPathJail(repoRoot, cfg.MCP.Security.FollowSymlinks)
	if err != nil {
		return nil, fmt.Errorf("create path jail: %w", err)
	}
	readRate := cfg.MCP.Security.RateLimitRead
	if readRate <= 0 {
		readRate = defaultReadRatePerMin
	}
	writeRate := cfg.MCP.Security.RateLimitWrite
	if writeRate <= 0 {
		writeRate = defaultWriteRatePerMin
	}
	limiter := NewRateLimiter(readRate, writeRate)
	audit, err := NewAuditLogger(cfg.MCP.Security)
	if err != nil {
		return nil, fmt.Errorf("create audit logger: %w", err)
	}
	mcpSrv := mcpserver.NewMCPServer(
		"grut",
		config.AppVersion,
		mcpserver.WithToolCapabilities(false),
	)
	s := &Server{
		mcp:      mcpSrv,
		git:      gitClient,
		repoRoot: repoRoot,
		cfg:      cfg,
		jail:     jail,
		limiter:  limiter,
		audit:    audit,
	}
	registerGitReadTools(s)
	registerGitWriteTools(s)
	registerGitOpsTools(s)
	registerFileTools(s)
	return s, nil
}

// Serve starts the MCP server on stdin/stdout (headless mode).
// It blocks until the context is cancelled or the transport closes.
func (s *Server) Serve(ctx context.Context) error {
	return mcpserver.ServeStdio(s.mcp)
}

// MCPServer returns the underlying MCPServer for testing.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcp
}

// addTool registers a tool with security middleware wrapping.
func (s *Server) addTool(name string, category string, tool mcplib.Tool, handler mcpserver.ToolHandlerFunc) {
	s.tools = append(s.tools, ToolInfo{
		Name:        name,
		Category:    category,
		Description: tool.Description,
	})
	s.mcp.AddTool(tool, s.wrapHandler(name, category, handler))
}

// wrapHandler wraps a tool handler with security checks: allowed-command
// filtering, write-confirmation gating, rate limiting, and audit logging.
func (s *Server) wrapHandler(name string, category string, handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		start := time.Now()

		// 1. Allowed-commands allowlist: if configured, reject tools not in the list.
		if !s.isCommandAllowed(name) {
			s.audit.Log(name, req.GetArguments(), "command_blocked", time.Since(start))
			return mcplib.NewToolResultErrorf("tool %q is not in the allowed commands list", name), nil
		}

		// 2. Write-confirmation gate: write tools require explicit confirmation.
		if category == categoryWrite && s.cfg.MCP.Security.RequireConfirmation {
			args := req.GetArguments()
			confirmed, _ := args["_confirmed"].(bool)
			if !confirmed {
				s.audit.Log(name, args, "confirmation_required", time.Since(start))
				return mcplib.NewToolResultErrorf(
					"tool %q is a write operation and requires confirmation: "+
						"re-invoke with \"_confirmed\": true to proceed", name,
				), nil
			}
		}

		// 3. Rate limiting.
		if !s.limiter.Allow(category) {
			s.audit.Log(name, req.GetArguments(), "rate_limited", time.Since(start))
			return mcplib.NewToolResultError("rate limit exceeded"), nil
		}

		result, err := handler(ctx, req)
		status := "success"
		if err != nil {
			status = "error"
		} else if result != nil && result.IsError {
			status = "tool_error"
		}
		s.audit.Log(name, req.GetArguments(), status, time.Since(start))
		return result, err
	}
}

// isCommandAllowed reports whether the named tool is permitted by the
// AllowedCommands allowlist. An empty list means all commands are allowed.
func (s *Server) isCommandAllowed(name string) bool {
	allowed := s.cfg.MCP.Security.AllowedCommands
	if len(allowed) == 0 {
		return true
	}
	for _, cmd := range allowed {
		if cmd == name {
			return true
		}
	}
	return false
}

// writePathAllowed enforces the optional mcp.security.allowed_write_paths
// narrowing policy for filesystem writes. The repository jail is always the
// outer boundary; this is an additional restriction that confines writes to an
// explicit subset of the repository. An empty list means no additional
// restriction (any in-jail path is writable). When non-empty, resolvedAbs (a
// jail-validated, symlink-resolved absolute path) must lie within at least one
// configured entry. Entries are interpreted relative to the repository root
// unless absolute and have already been validated to reject traversal and UNC
// paths (see config.Validate).
func (s *Server) writePathAllowed(resolvedAbs string) error {
	allowed := s.cfg.MCP.Security.AllowedWritePaths
	if len(allowed) == 0 {
		return nil
	}
	for _, entry := range allowed {
		base := entry
		if !filepath.IsAbs(base) {
			base = filepath.Join(s.jail.Root(), base)
		}
		base = filepath.Clean(base)
		// Resolve symlinks so the comparison happens in the same canonical
		// namespace as resolvedAbs. Non-existent entries fall back to their
		// cleaned form.
		if canonical, err := filepath.EvalSymlinks(base); err == nil {
			base = canonical
		}
		if pathWithin(base, resolvedAbs) {
			return nil
		}
	}
	return fmt.Errorf("path is not within any allowed_write_paths entry")
}

// pathWithin reports whether target is root itself or nested inside root.
// Both paths are expected to be absolute and canonical.
func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// jsonResult marshals data to JSON and returns it as a text tool result.
func jsonResult(data any) (*mcplib.CallToolResult, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return mcplib.NewToolResultErrorf("marshal result: %v", err), nil
	}
	return mcplib.NewToolResultText(string(b)), nil
}

// toolError returns an MCP error result wrapping an error.
func toolError(msg string, err error) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultErrorf("%s: %v", msg, err), nil
}

func validateGitPath(path string) error {
	return git.ValidateRepoRelativePath(path)
}

func validateGitPaths(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	for _, path := range paths {
		if err := validateGitPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) validateGitPathInJail(path string) (string, error) {
	if err := validateGitPath(path); err != nil {
		return "", err
	}
	resolved, err := s.jail.Validate(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.jail.Root(), resolved)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		slog.Debug("mcp: git path escapes root", "path", path)
		return "", fmt.Errorf("path escapes repository root")
	}
	return filepath.ToSlash(rel), nil
}

func (s *Server) validateGitPathsInJail(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}
	resolved := make([]string, 0, len(paths))
	for _, p := range paths {
		r, err := s.validateGitPathInJail(p)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, r)
	}
	return resolved, nil
}
