package mcp

import (
	"context"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git/gittest"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// mockGitClient is a package-local alias for gittest.MockClient so existing
// test code in the mcp package doesn't need renaming.
type mockGitClient = gittest.MockClient

// stubConfig returns a minimal config suitable for tests.
func stubConfig() *config.Config {
	return &config.Config{
		MCP: config.MCPConfig{
			Security: stubSecurityConfig(),
		},
	}
}

// stubSecurityConfig returns a security config with audit logging disabled
// and generous rate limits for tests.
func stubSecurityConfig() config.MCPSecurityConfig {
	return config.MCPSecurityConfig{
		RateLimitRead:  10000,
		RateLimitWrite: 10000,
		AuditLog:       false,
	}
}

// newTestServer creates an MCP Server with a mock git client for testing.
// The repo root is set to the given directory.
func newTestServer(t *testing.T, mock *mockGitClient, root string) *Server {
	t.Helper()
	cfg := stubConfig()
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)
	return srv
}

// callTool is a test helper that directly invokes a registered tool handler.
// It builds a CallToolRequest with the given arguments and calls the handler
// returned by the server's MCPServer.
func callTool(t *testing.T, srv *Server, toolName string, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	ctx := context.Background()

	st := srv.mcp.GetTool(toolName)
	require.NotNil(t, st, "tool %q should be registered", toolName)

	req := mcplib.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	result, err := st.Handler(ctx, req)
	require.NoError(t, err, "tool handler should not return protocol error")
	require.NotNil(t, result, "result should not be nil")
	return result
}

// resultText extracts the text content from a CallToolResult.
func resultText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "result should have content")
	for _, c := range result.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content in result")
	return ""
}
