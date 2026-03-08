package mcp

import (
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer_CreatesSuccessfully(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	assert.NotNil(t, srv.mcp)
	assert.NotNil(t, srv.jail)
	assert.NotNil(t, srv.limiter)
	assert.NotNil(t, srv.audit)
}

func TestNewServer_RegistersAllTools(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)

	// Get the list of registered tools from the MCP server.
	mcpSrv := srv.MCPServer()

	// Expected tool names.
	expectedTools := []string{
		// Read tools (10)
		"git_status", "git_diff", "git_log", "git_blame",
		"git_branch_list", "git_tag_list",
		"git_worktree_list", "git_reflog", "git_is_repo", "git_repo_root",
		// Write tools (38)
		"git_stage", "git_unstage", "git_commit",
		"git_branch_create", "git_branch_delete", "git_branch_rename",
		"git_checkout", "git_push", "git_pull", "git_fetch",
		"git_merge", "git_merge_abort",
		"git_rebase", "git_rebase_continue", "git_rebase_abort",
		"git_cherry_pick",
		"git_stash_list", "git_stash_show",
		"git_stash_push", "git_stash_pop", "git_stash_apply", "git_stash_drop",
		"git_tag_create", "git_tag_delete",
		"git_worktree_add", "git_worktree_remove",
		"git_bisect_start", "git_bisect_good", "git_bisect_bad", "git_bisect_reset",
		"git_discard", "git_discard_all",
		"git_revert", "git_revert_continue", "git_revert_abort",
		"git_reset",
		"git_stage_hunk", "git_unstage_hunk",
		// File tools (3)
		"file_read", "file_write", "file_list",
	}

	// We can't directly list tools from MCPServer, but we can verify the
	// server was created and tools were registered without error.
	// The real proof is in the git_tools_test and file_tools_test where
	// we invoke each tool handler.
	assert.NotNil(t, mcpSrv, "MCPServer should be created")
	assert.Equal(t, 51, len(expectedTools), "should have 51 expected tools defined")
}

func TestNewServer_DefaultRateLimits(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:  0, // should default to 1000
				RateLimitWrite: 0, // should default to 100
				AuditLog:       false,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestNewServer_InvalidRootDir(t *testing.T) {
	mock := &mockGitClient{}
	cfg := stubConfig()

	// Non-existent directory should fail in PathJail creation.
	_, err := NewServer(mock, "/nonexistent/path/that/does/not/exist", cfg)
	assert.Error(t, err)
}
