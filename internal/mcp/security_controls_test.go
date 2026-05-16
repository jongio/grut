package mcp

import (
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- AllowedCommands tests ---

func TestAllowedCommands_EmptyListAllowsAll(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:  10000,
				RateLimitWrite: 10000,
				AllowedCommands: nil, // empty = allow all
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// git_status (read tool) should be allowed.
	result := callTool(t, srv, "git_status", nil)
	assert.False(t, result.IsError, "git_status should succeed with empty allowlist")
}

func TestAllowedCommands_AllowsListedTool(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:   10000,
				RateLimitWrite:  10000,
				AllowedCommands: []string{"git_status", "git_diff"},
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	result := callTool(t, srv, "git_status", nil)
	assert.False(t, result.IsError, "git_status should be allowed when in allowlist")
}

func TestAllowedCommands_BlocksUnlistedTool(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:   10000,
				RateLimitWrite:  10000,
				AllowedCommands: []string{"git_status"},
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// git_log is NOT in the allowlist.
	result := callTool(t, srv, "git_log", map[string]any{})
	assert.True(t, result.IsError, "git_log should be blocked when not in allowlist")
	text := resultText(t, result)
	assert.Contains(t, text, "not in the allowed commands list")
}

func TestAllowedCommands_BlocksWriteToolNotListed(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:   10000,
				RateLimitWrite:  10000,
				AllowedCommands: []string{"git_status"},
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	result := callTool(t, srv, "git_commit", map[string]any{"message": "test"})
	assert.True(t, result.IsError, "git_commit should be blocked when not in allowlist")
	text := resultText(t, result)
	assert.Contains(t, text, "not in the allowed commands list")
}

// --- RequireConfirmation tests ---

func TestRequireConfirmation_BlocksWriteWithoutConfirmed(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:       10000,
				RateLimitWrite:      10000,
				RequireConfirmation: true,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// Write tool without _confirmed should be rejected.
	result := callTool(t, srv, "git_commit", map[string]any{"message": "test"})
	assert.True(t, result.IsError, "write tool should require confirmation")
	text := resultText(t, result)
	assert.Contains(t, text, "requires confirmation")
}

func TestRequireConfirmation_AllowsWriteWithConfirmed(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:       10000,
				RateLimitWrite:      10000,
				RequireConfirmation: true,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// Write tool with _confirmed: true should pass the confirmation gate.
	result := callTool(t, srv, "git_commit", map[string]any{
		"message":    "test",
		"_confirmed": true,
	})
	// The commit itself may fail due to mock but should NOT fail on confirmation.
	text := resultText(t, result)
	assert.NotContains(t, text, "requires confirmation")
}

func TestRequireConfirmation_AllowsReadToolsWithoutConfirmed(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:       10000,
				RateLimitWrite:      10000,
				RequireConfirmation: true,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// Read tool should NOT require confirmation.
	result := callTool(t, srv, "git_status", nil)
	assert.False(t, result.IsError, "read tools should not require confirmation")
}

func TestRequireConfirmation_DisabledAllowsWrites(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:       10000,
				RateLimitWrite:      10000,
				RequireConfirmation: false,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	result := callTool(t, srv, "git_commit", map[string]any{"message": "test"})
	text := resultText(t, result)
	assert.NotContains(t, text, "requires confirmation",
		"confirmation should not be required when disabled")
}

// --- isCommandAllowed unit tests ---

func TestIsCommandAllowed_EmptyList(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}
	assert.True(t, srv.isCommandAllowed("anything"))
}

func TestIsCommandAllowed_Match(t *testing.T) {
	srv := &Server{cfg: &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				AllowedCommands: []string{"git_status", "git_log"},
			},
		},
	}}
	assert.True(t, srv.isCommandAllowed("git_status"))
	assert.True(t, srv.isCommandAllowed("git_log"))
	assert.False(t, srv.isCommandAllowed("git_commit"))
	assert.False(t, srv.isCommandAllowed("file_write"))
}

// --- Combined AllowedCommands + RequireConfirmation ---

func TestAllowedCommandsAndConfirmation_BothEnforced(t *testing.T) {
	root := t.TempDir()
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:       10000,
				RateLimitWrite:      10000,
				AllowedCommands:     []string{"git_commit"},
				RequireConfirmation: true,
			},
		},
	}
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// Unlisted tool: blocked by allowlist (before confirmation check).
	result := callTool(t, srv, "git_status", nil)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "not in the allowed commands list")

	// Listed write tool without confirmation: blocked by confirmation gate.
	result = callTool(t, srv, "git_commit", map[string]any{"message": "test"})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "requires confirmation")
}
