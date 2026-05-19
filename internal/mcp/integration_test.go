package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test‐fixture helpers
// ---------------------------------------------------------------------------

// initGitRepo creates a temporary git repository with an initial commit.
// Returns the path to the repo root. The directory is cleaned up by t.Cleanup.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_TERMINAL_PROMPT=0",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create an initial file and commit so HEAD exists.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "initial commit")

	return dir
}

// newIntegrationServer creates an MCP Server using a real git.Client pointed
// at the given repo directory. This is the "integration" counterpart of
// newTestServer which uses a mock.
func newIntegrationServer(t *testing.T, repoDir string) *Server {
	t.Helper()
	gc, err := git.NewClient(repoDir)
	require.NoError(t, err, "create real git client")

	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:  10000,
				RateLimitWrite: 10000,
				AuditLog:       false,
			},
		},
	}
	srv, err := NewServer(gc, repoDir, cfg)
	require.NoError(t, err, "create MCP server")
	return srv
}

// newInProcessMCPClient creates an in-process MCP client connected directly
// to the given server's underlying MCPServer. The client is initialised and
// ready for ListTools / CallTool requests.
func newInProcessMCPClient(t *testing.T, srv *Server) *mcpclient.Client {
	t.Helper()
	ctx := context.Background()

	c, err := mcpclient.NewInProcessClient(srv.MCPServer())
	require.NoError(t, err, "create in-process client")

	require.NoError(t, c.Start(ctx), "start in-process client")
	t.Cleanup(func() { _ = c.Close() })

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "integration-test", Version: "0.0.1"}
	_, err = c.Initialize(ctx, initReq)
	require.NoError(t, err, "initialize client")

	return c
}

// newPipeMCPClient creates a client+server pair connected through OS pipes
// exercising the full JSON-RPC over stdio transport. The client is initialised
// and ready for use.
func newPipeMCPClient(t *testing.T, srv *Server) *mcpclient.Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create two OS-level pipes:
	//   clientOut → serverIn   (client writes requests, server reads them)
	//   serverOut → clientIn   (server writes responses, client reads them)
	clientOutR, clientOutW, err := os.Pipe()
	require.NoError(t, err, "pipe for client→server")
	serverOutR, serverOutW, err := os.Pipe()
	require.NoError(t, err, "pipe for server→client")

	t.Cleanup(func() {
		_ = clientOutW.Close()
		_ = clientOutR.Close()
		_ = serverOutW.Close()
		_ = serverOutR.Close()
	})

	// Start the server in a goroutine, reading from clientOutR, writing to serverOutW.
	stdioSrv := mcpserver.NewStdioServer(srv.MCPServer())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- stdioSrv.Listen(ctx, clientOutR, serverOutW)
	}()

	// Build a client transport that reads from serverOutR and writes to clientOutW.
	tr := transport.NewIO(serverOutR, clientOutW, nil)
	c := mcpclient.NewClient(tr)

	require.NoError(t, c.Start(ctx), "start pipe client transport")
	t.Cleanup(func() {
		_ = c.Close()
		cancel()
		<-serverDone // drain
	})

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "pipe-test", Version: "0.0.1"}
	_, err = c.Initialize(ctx, initReq)
	require.NoError(t, err, "initialize pipe client")

	return c
}

// callToolResult is a convenience wrapper for calling a tool through the MCP
// client and asserting no protocol-level error.
func callToolResult(t *testing.T, c *mcpclient.Client, tool string, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err, "CallTool %q protocol error", tool)
	require.NotNil(t, result, "CallTool %q returned nil result", tool)
	return result
}

// textFromResult extracts the text content from a CallToolResult.
func textFromResult(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	for _, c := range result.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content in result")
	return ""
}

// gitExec runs a git command in the given directory and returns stdout.
func gitExec(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return strings.TrimSpace(string(out))
}

// ===========================================================================
// 1. End-to-end MCP protocol tests (in-process transport)
// ===========================================================================

func TestIntegration_Initialize(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)

	ctx := context.Background()
	c, err := mcpclient.NewInProcessClient(srv.MCPServer())
	require.NoError(t, err)
	require.NoError(t, c.Start(ctx))
	t.Cleanup(func() { _ = c.Close() })

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "test", Version: "1.0"}

	result, err := c.Initialize(ctx, initReq)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "grut", result.ServerInfo.Name)
	assert.NotEmpty(t, result.ProtocolVersion)
	assert.NotNil(t, result.Capabilities.Tools, "server should advertise tool capabilities")
}

func TestIntegration_ListTools_ReturnsAll51(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result, err := c.ListTools(context.Background(), mcplib.ListToolsRequest{})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Collect tool names for diagnostics on failure.
	names := make([]string, len(result.Tools))
	for i, tool := range result.Tools {
		names[i] = tool.Name
	}

	assert.Len(t, result.Tools, 51, "expected 51 tools, got: %v", names)

	// Verify a representative set of tools exists with descriptions and schemas.
	mustHave := []string{
		"git_status", "git_diff", "git_log", "git_blame",
		"git_stage", "git_commit", "git_branch_create",
		"git_merge", "git_stash_push", "git_bisect_start",
		"file_read", "file_write", "file_list",
	}
	nameSet := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		nameSet[tool.Name] = true
	}
	for _, name := range mustHave {
		assert.True(t, nameSet[name], "tool %q should be registered", name)
	}

	// Every tool should have a description.
	for _, tool := range result.Tools {
		assert.NotEmpty(t, tool.Description, "tool %q should have a description", tool.Name)
	}
}

// ===========================================================================
// 2. Real git repository tests (in-process transport)
// ===========================================================================

func TestIntegration_GitStatus_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// No changes → empty status.
	result := callToolResult(t, c, "git_status", nil)
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	// Should be a JSON array — possibly empty or null.
	assert.Contains(t, text, "[", "status should be JSON array when no changes (got %q)", text)

	// Create a new untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "new.txt"), []byte("hello\n"), 0o644))

	result = callToolResult(t, c, "git_status", nil)
	text = textFromResult(t, result)
	assert.Contains(t, text, "new.txt", "status should list untracked file")
}

func TestIntegration_GitDiff_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Modify the tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("# changed\n"), 0o644))

	result := callToolResult(t, c, "git_diff", nil)
	text := textFromResult(t, result)
	assert.Contains(t, text, "changed", "diff should show the modification")
}

func TestIntegration_GitLog_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "git_log", map[string]any{"limit": float64(5)})
	text := textFromResult(t, result)
	assert.Contains(t, text, "initial commit", "log should contain the initial commit message")

	// Parse as JSON array of commits.
	var commits []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(text), &commits), "log should be valid JSON array")
	assert.GreaterOrEqual(t, len(commits), 1)
}

func TestIntegration_GitStageAndCommit_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Create and stage a file.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("stage me\n"), 0o644))

	stageResult := callToolResult(t, c, "git_stage", map[string]any{
		"paths": []any{"staged.txt"},
	})
	assert.False(t, stageResult.IsError, "stage should succeed: %s", textFromResult(t, stageResult))

	// Commit through MCP.
	commitResult := callToolResult(t, c, "git_commit", map[string]any{
		"message": "add staged.txt via MCP",
	})
	assert.False(t, commitResult.IsError, "commit should succeed")
	commitText := textFromResult(t, commitResult)
	assert.NotEmpty(t, commitText, "commit should return hash")

	// Verify commit exists in log.
	logResult := callToolResult(t, c, "git_log", map[string]any{"limit": float64(1)})
	logText := textFromResult(t, logResult)
	assert.Contains(t, logText, "add staged.txt via MCP")
}

func TestIntegration_GitBranch_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Create branch.
	createResult := callToolResult(t, c, "git_branch_create", map[string]any{
		"name": "feature-test",
	})
	assert.False(t, createResult.IsError, "branch create should succeed")

	// List branches.
	listResult := callToolResult(t, c, "git_branch_list", nil)
	text := textFromResult(t, listResult)
	assert.Contains(t, text, "feature-test", "branch list should contain the new branch")
	assert.Contains(t, text, "main", "branch list should contain main")
}

func TestIntegration_FileReadWriteList_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// file_write: create a file.
	writeResult := callToolResult(t, c, "file_write", map[string]any{
		"path":    "subdir/hello.txt",
		"content": "hello from MCP",
	})
	assert.False(t, writeResult.IsError, "file_write should succeed: %s", textFromResult(t, writeResult))

	// file_read: read it back.
	readResult := callToolResult(t, c, "file_read", map[string]any{
		"path": "subdir/hello.txt",
	})
	assert.False(t, readResult.IsError)
	assert.Equal(t, "hello from MCP", textFromResult(t, readResult))

	// file_list: verify the file appears.
	listResult := callToolResult(t, c, "file_list", map[string]any{
		"path":      "subdir",
		"recursive": true,
	})
	assert.False(t, listResult.IsError)
	assert.Contains(t, textFromResult(t, listResult), "hello.txt")
}

func TestIntegration_GitTagCreateAndList_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	createResult := callToolResult(t, c, "git_tag_create", map[string]any{
		"name":    "v0.1.0",
		"ref":     "HEAD",
		"message": "first release",
	})
	assert.False(t, createResult.IsError, "tag create should succeed")

	listResult := callToolResult(t, c, "git_tag_list", nil)
	text := textFromResult(t, listResult)
	assert.Contains(t, text, "v0.1.0", "tag list should contain v0.1.0")
}

func TestIntegration_GitReflog_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "git_reflog", map[string]any{
		"ref":   "HEAD",
		"limit": float64(5),
	})
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	// Reflog should contain at least the initial commit entry.
	assert.Contains(t, text, "commit", "reflog should mention a commit action")
}

func TestIntegration_GitIsRepo_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "git_is_repo", nil)
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	assert.Contains(t, text, "true", "should report as a git repo")
}

func TestIntegration_GitBlame_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "git_blame", map[string]any{
		"path": "README.md",
	})
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	assert.Contains(t, text, "test", "blame should show the commit content")
}

func TestIntegration_GitCheckout_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Create branch and switch to it.
	callToolResult(t, c, "git_branch_create", map[string]any{"name": "switch-target"})
	result := callToolResult(t, c, "git_checkout", map[string]any{"ref": "switch-target"})
	assert.False(t, result.IsError, "checkout should succeed")

	// Verify by checking branch list for current marker.
	branchResult := callToolResult(t, c, "git_branch_list", nil)
	text := textFromResult(t, branchResult)
	// Parse branches to verify the current one.
	var branches []json.RawMessage
	if err := json.Unmarshal([]byte(text), &branches); err == nil {
		assert.GreaterOrEqual(t, len(branches), 2)
	}
}

func TestIntegration_GitDiscard_RealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Modify tracked file, then discard.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("modified\n"), 0o644))

	result := callToolResult(t, c, "git_discard", map[string]any{"path": "README.md"})
	assert.False(t, result.IsError, "discard should succeed")

	// File should be restored (normalize CRLF for Windows).
	content, err := os.ReadFile(filepath.Join(repo, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# test\n", strings.ReplaceAll(string(content), "\r\n", "\n"))
}

// ===========================================================================
// 3. Error handling through protocol
// ===========================================================================

func TestIntegration_UnknownTool(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	req := mcplib.CallToolRequest{}
	req.Params.Name = "nonexistent_tool"
	req.Params.Arguments = map[string]any{}

	_, err := c.CallTool(context.Background(), req)
	// Unknown tool should return a protocol-level error.
	assert.Error(t, err, "calling unknown tool should produce an error")
}

func TestIntegration_MissingRequiredArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// file_read requires "path".
	result := callToolResult(t, c, "file_read", map[string]any{})
	assert.True(t, result.IsError, "file_read without path should be a tool error")
	text := textFromResult(t, result)
	assert.Contains(t, strings.ToLower(text), "required", "error should mention the required field")
}

func TestIntegration_PathTraversal_Blocked(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Attempt to read outside repo via traversal.
	traversalPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"subdir/../../..",
	}
	for _, p := range traversalPaths {
		t.Run(p, func(t *testing.T) {
			result := callToolResult(t, c, "file_read", map[string]any{"path": p})
			assert.True(t, result.IsError, "path traversal should be blocked for %q", p)
			text := textFromResult(t, result)
			assert.True(
				t,
				strings.Contains(strings.ToLower(text), "escapes") ||
					strings.Contains(strings.ToLower(text), "validation") ||
					strings.Contains(strings.ToLower(text), "outside") ||
					strings.Contains(strings.ToLower(text), "traversal") ||
					strings.Contains(strings.ToLower(text), "path"),
				"error message should indicate path security violation, got: %s", text,
			)
		})
	}
}

func TestIntegration_FileReadNonexistent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "file_read", map[string]any{"path": "does_not_exist.txt"})
	assert.True(t, result.IsError, "reading nonexistent file should be a tool error")
}

func TestIntegration_GitStage_InvalidPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Stage with path traversal.
	result := callToolResult(t, c, "git_stage", map[string]any{
		"paths": []any{"../outside.txt"},
	})
	assert.True(t, result.IsError, "staging outside-repo path should fail")
}

// ===========================================================================
// 4. Pipe-based transport (JSON-RPC over stdio)
// ===========================================================================

func TestIntegration_Pipe_Initialize(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	// newPipeMCPClient already initializes — the test passes if no error.
	c := newPipeMCPClient(t, srv)
	_ = c
}

func TestIntegration_Pipe_ListTools(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	result, err := c.ListTools(context.Background(), mcplib.ListToolsRequest{})
	require.NoError(t, err)
	assert.Len(t, result.Tools, 51, "pipe transport should return all 51 tools")
}

func TestIntegration_Pipe_CallTool_GitStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	// Create a file so status has something to report.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "pipe-test.txt"), []byte("pipe\n"), 0o644))

	result := callToolResult(t, c, "git_status", nil)
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	assert.Contains(t, text, "pipe-test.txt")
}

func TestIntegration_Pipe_CallTool_FileRoundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	writeResult := callToolResult(t, c, "file_write", map[string]any{
		"path":    "pipe-roundtrip.txt",
		"content": "roundtrip content",
	})
	assert.False(t, writeResult.IsError)

	readResult := callToolResult(t, c, "file_read", map[string]any{
		"path": "pipe-roundtrip.txt",
	})
	assert.False(t, readResult.IsError)
	assert.Equal(t, "roundtrip content", textFromResult(t, readResult))
}

func TestIntegration_Pipe_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	// Unknown tool.
	req := mcplib.CallToolRequest{}
	req.Params.Name = "does_not_exist"
	_, err := c.CallTool(context.Background(), req)
	assert.Error(t, err, "unknown tool should error over pipe transport")

	// Path traversal over pipe.
	result := callToolResult(t, c, "file_read", map[string]any{
		"path": "../../../etc/passwd",
	})
	assert.True(t, result.IsError, "path traversal should be blocked over pipe transport")
}

// ===========================================================================
// 5. Concurrent requests
// ===========================================================================

func TestIntegration_Concurrent_InProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)

	// Create some files for concurrent reading.
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("concurrent_%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(fmt.Sprintf("content %d", i)), 0o644))
	}
	gitExec(t, repo, "add", ".")
	gitExec(t, repo, "commit", "-m", "add concurrent test files")

	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	const numWorkers = 20
	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Mix of different read operations.
			switch idx % 4 {
			case 0:
				result := callToolResult(t, c, "git_status", nil)
				if result.IsError {
					errors <- fmt.Errorf("worker %d: git_status returned error", idx)
				}
			case 1:
				result := callToolResult(t, c, "git_log", map[string]any{"limit": float64(3)})
				if result.IsError {
					errors <- fmt.Errorf("worker %d: git_log returned error", idx)
				}
			case 2:
				fileIdx := idx % 10
				result := callToolResult(t, c, "file_read", map[string]any{
					"path": fmt.Sprintf("concurrent_%d.txt", fileIdx),
				})
				if result.IsError {
					errors <- fmt.Errorf("worker %d: file_read returned error", idx)
				}
			case 3:
				result := callToolResult(t, c, "git_branch_list", nil)
				if result.IsError {
					errors <- fmt.Errorf("worker %d: git_branch_list returned error", idx)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestIntegration_Concurrent_Pipe(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("pipe_concurrent_%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(fmt.Sprintf("pipe %d", i)), 0o644))
	}
	gitExec(t, repo, "add", ".")
	gitExec(t, repo, "commit", "-m", "add pipe concurrent files")

	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	const numWorkers = 10
	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			switch idx % 3 {
			case 0:
				result := callToolResult(t, c, "git_status", nil)
				if result.IsError {
					errors <- fmt.Errorf("pipe worker %d: git_status error", idx)
				}
			case 1:
				result := callToolResult(t, c, "git_log", map[string]any{"limit": float64(2)})
				if result.IsError {
					errors <- fmt.Errorf("pipe worker %d: git_log error", idx)
				}
			case 2:
				fileIdx := idx % 5
				result := callToolResult(t, c, "file_read", map[string]any{
					"path": fmt.Sprintf("pipe_concurrent_%d.txt", fileIdx),
				})
				if result.IsError {
					errors <- fmt.Errorf("pipe worker %d: file_read error", idx)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// ===========================================================================
// 6. Server lifecycle
// ===========================================================================

func TestIntegration_ServerShutdown_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)

	ctx, cancel := context.WithCancel(context.Background())

	// Set up pipes for the server.
	clientOutR, clientOutW, err := os.Pipe()
	require.NoError(t, err)
	serverOutR, serverOutW, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = clientOutR.Close()
		_ = clientOutW.Close()
		_ = serverOutR.Close()
		_ = serverOutW.Close()
	})

	stdioSrv := mcpserver.NewStdioServer(srv.MCPServer())
	done := make(chan error, 1)
	go func() {
		done <- stdioSrv.Listen(ctx, clientOutR, serverOutW)
	}()

	// Cancel context → server should exit.
	cancel()

	select {
	case err := <-done:
		// Server should exit without error (or with context.Canceled).
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled, "server should exit cleanly on cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds after context cancel")
	}
}

func TestIntegration_ServerShutdown_CloseStdin(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)

	ctx := context.Background()

	clientOutR, clientOutW, err := os.Pipe()
	require.NoError(t, err)
	_, serverOutW, err := os.Pipe()
	require.NoError(t, err)

	stdioSrv := mcpserver.NewStdioServer(srv.MCPServer())
	done := make(chan error, 1)
	go func() {
		done <- stdioSrv.Listen(ctx, clientOutR, serverOutW)
	}()

	// Close the write end of stdin → server should see EOF and exit.
	require.NoError(t, clientOutW.Close())

	select {
	case <-done:
		// Server exited — pass (error or nil is fine for stdin close).
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5 seconds after stdin close")
	}
}

// ===========================================================================
// 7. Complex multi-step git workflow
// ===========================================================================

func TestIntegration_FullWorkflow_BranchCommitMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// 1. Create a feature branch.
	r := callToolResult(t, c, "git_branch_create", map[string]any{"name": "feature-x"})
	assert.False(t, r.IsError)

	// 2. Switch to feature branch.
	r = callToolResult(t, c, "git_checkout", map[string]any{"ref": "feature-x"})
	assert.False(t, r.IsError)

	// 3. Create + stage + commit a file on the feature branch.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature work\n"), 0o644))
	r = callToolResult(t, c, "git_stage", map[string]any{"paths": []any{"feature.txt"}})
	assert.False(t, r.IsError)
	r = callToolResult(t, c, "git_commit", map[string]any{"message": "feature commit"})
	assert.False(t, r.IsError)

	// 4. Switch back to main.
	r = callToolResult(t, c, "git_checkout", map[string]any{"ref": "main"})
	assert.False(t, r.IsError)

	// 5. Merge feature branch.
	r = callToolResult(t, c, "git_merge", map[string]any{"branch": "feature-x"})
	assert.False(t, r.IsError)

	// 6. Verify merged file exists (normalize CRLF for Windows).
	readResult := callToolResult(t, c, "file_read", map[string]any{"path": "feature.txt"})
	assert.False(t, readResult.IsError)
	got := strings.ReplaceAll(textFromResult(t, readResult), "\r\n", "\n")
	assert.Equal(t, "feature work\n", got)

	// 7. Verify log has the feature commit.
	logResult := callToolResult(t, c, "git_log", map[string]any{"limit": float64(5)})
	assert.Contains(t, textFromResult(t, logResult), "feature commit")
}

func TestIntegration_StashWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Modify tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("stash me\n"), 0o644))

	// Stash changes.
	r := callToolResult(t, c, "git_stash_push", map[string]any{"message": "wip changes"})
	assert.False(t, r.IsError, "stash push should succeed")

	// File should be reverted (normalize CRLF for Windows).
	content, err := os.ReadFile(filepath.Join(repo, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# test\n", strings.ReplaceAll(string(content), "\r\n", "\n"), "file should be restored after stash")

	// List stash.
	listResult := callToolResult(t, c, "git_stash_list", nil)
	assert.False(t, listResult.IsError)
	assert.Contains(t, textFromResult(t, listResult), "wip changes", "stash list should show the stash")

	// Pop stash.
	popResult := callToolResult(t, c, "git_stash_pop", map[string]any{"index": float64(0)})
	assert.False(t, popResult.IsError, "stash pop should succeed")

	// File should be modified again (normalize CRLF for Windows).
	content, err = os.ReadFile(filepath.Join(repo, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "stash me\n", strings.ReplaceAll(string(content), "\r\n", "\n"), "stash pop should restore changes")
}

// ===========================================================================
// 8. Pipe transport: full workflow end-to-end
// ===========================================================================

func TestIntegration_Pipe_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newPipeMCPClient(t, srv)

	// Write → Stage → Commit → Log over pipe transport.
	r := callToolResult(t, c, "file_write", map[string]any{
		"path":    "pipe-workflow.txt",
		"content": "pipe workflow test",
	})
	assert.False(t, r.IsError)

	r = callToolResult(t, c, "git_stage", map[string]any{"paths": []any{"pipe-workflow.txt"}})
	assert.False(t, r.IsError)

	r = callToolResult(t, c, "git_commit", map[string]any{"message": "pipe workflow commit"})
	assert.False(t, r.IsError)

	logResult := callToolResult(t, c, "git_log", map[string]any{"limit": float64(1)})
	assert.Contains(t, textFromResult(t, logResult), "pipe workflow commit")
}

// ===========================================================================
// 9. Edge cases
// ===========================================================================

func TestIntegration_FileList_SkipsDotGit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "file_list", map[string]any{
		"recursive": true,
	})
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	assert.NotContains(t, text, ".git/", "file_list should not expose .git directory contents")
}

func TestIntegration_GitDiff_Staged(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Modify and stage.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("staged diff\n"), 0o644))
	gitExec(t, repo, "add", "README.md")

	result := callToolResult(t, c, "git_diff", map[string]any{"staged": true})
	text := textFromResult(t, result)
	assert.Contains(t, text, "staged diff", "staged diff should show the staged content")
}

func TestIntegration_GitRepoRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result := callToolResult(t, c, "git_repo_root", nil)
	assert.False(t, result.IsError)
	text := textFromResult(t, result)
	// The repo root should be a valid path.
	assert.NotEmpty(t, text)
}

func TestIntegration_EmptyArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	// Tools that work with no args should not panic.
	noArgTools := []string{
		"git_status", "git_diff", "git_log",
		"git_branch_list", "git_tag_list", "git_worktree_list",
		"git_is_repo", "git_repo_root", "git_stash_list",
	}
	for _, tool := range noArgTools {
		t.Run(tool, func(t *testing.T) {
			result := callToolResult(t, c, tool, nil)
			assert.False(t, result.IsError, "tool %q with nil args should not error", tool)
		})
	}
}

// ===========================================================================
// 10. Tool schema validation
// ===========================================================================

func TestIntegration_ToolSchemas_HaveRequiredFields(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	repo := initGitRepo(t)
	srv := newIntegrationServer(t, repo)
	c := newInProcessMCPClient(t, srv)

	result, err := c.ListTools(context.Background(), mcplib.ListToolsRequest{})
	require.NoError(t, err)

	// Verify tools with required parameters have proper schema definitions.
	toolsWithRequired := map[string][]string{
		"file_read":           {"path"},
		"file_write":          {"path", "content"},
		"git_stage":           {"paths"},
		"git_unstage":         {"paths"},
		"git_commit":          {"message"},
		"git_branch_create":   {"name"},
		"git_branch_delete":   {"name"},
		"git_branch_rename":   {"old_name", "new_name"},
		"git_checkout":        {"ref"},
		"git_blame":           {"path"},
		"git_merge":           {"branch"},
		"git_rebase":          {"onto"},
		"git_cherry_pick":     {"commit"},
		"git_tag_create":      {"name"},
		"git_tag_delete":      {"name"},
		"git_bisect_start":    {"bad", "good"},
		"git_discard":         {"path"},
		"git_revert":          {"hash"},
		"git_reset":           {"ref"},
		"git_worktree_add":    {"path"},
		"git_worktree_remove": {"path"},
	}

	toolMap := make(map[string]mcplib.Tool)
	for _, tool := range result.Tools {
		toolMap[tool.Name] = tool
	}

	for toolName, expectedRequired := range toolsWithRequired {
		t.Run(toolName, func(t *testing.T) {
			tool, ok := toolMap[toolName]
			if !ok {
				t.Fatalf("tool %q not found in registered tools", toolName)
			}

			// Verify input schema has required fields.
			schemaRequired := tool.InputSchema.Required
			for _, req := range expectedRequired {
				assert.Contains(t, schemaRequired, req,
					"tool %q schema should list %q as required", toolName, req)
			}
		})
	}
}
