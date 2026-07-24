package config

import (
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validCfg returns a Config initialised from embedded defaults (local helper
// to avoid coupling to config_test.go's validConfig which takes *testing.T).
func validCfg(t *testing.T) *Config {
	t.Helper()
	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(defaultsTOML, cfg))
	return cfg
}

// ---------------------------------------------------------------------------
// Hard upper-bound limits
// ---------------------------------------------------------------------------

func TestValidate_HardLimits(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		// file_tree
		{
			name:    "max_depth too high",
			mutate:  func(c *Config) { c.FileTree.MaxDepth = maxMaxDepth + 1 },
			wantErr: "file_tree.max_depth",
		},
		// preview
		{
			name:    "max_file_size too high",
			mutate:  func(c *Config) { c.Preview.MaxFileSize = maxMaxFileSize*1024*1024 + 1 },
			wantErr: "preview.max_file_size",
		},
		// terminal
		{
			name:    "scrollback too high",
			mutate:  func(c *Config) { c.Terminal.Scrollback = maxScrollback + 1 },
			wantErr: "terminal.scrollback",
		},
		{
			name:    "render_fps too high",
			mutate:  func(c *Config) { c.Terminal.RenderFPS = maxRenderFPS + 1 },
			wantErr: "terminal.render_fps",
		},
		// git
		{
			name:    "max_log_entries too high",
			mutate:  func(c *Config) { c.Git.MaxLogEntries = maxMaxLogEntries + 1 },
			wantErr: "git.max_log_entries",
		},
		// github
		{
			name:    "poll_interval too high",
			mutate:  func(c *Config) { c.GitHub.PollInterval = maxPollInterval + 1 },
			wantErr: "github.poll_interval",
		},
		{
			name:    "review_diff_context_lines too high",
			mutate:  func(c *Config) { c.GitHub.ReviewDiffContextLines = maxDiffContext + 1 },
			wantErr: "github.review_diff_context_lines",
		},
		// ai
		{
			name:    "max_context_files too high",
			mutate:  func(c *Config) { c.AI.MaxContextFiles = maxContextFiles + 1 },
			wantErr: "ai.max_context_files",
		},
		{
			name:    "max_context_tokens too high",
			mutate:  func(c *Config) { c.AI.MaxContextTokens = maxContextTokens + 1 },
			wantErr: "ai.max_context_tokens",
		},
		// ai.claude
		{
			name:    "claude.max_tokens too high",
			mutate:  func(c *Config) { c.AI.Claude.MaxTokens = maxClaudeTokens + 1 },
			wantErr: "ai.claude.max_tokens",
		},
		// ai.chat
		{
			name:    "chat.collapsed_height too high",
			mutate:  func(c *Config) { c.AI.Chat.CollapsedHeight = maxChatHeight + 1 },
			wantErr: "ai.chat.collapsed_height",
		},
		{
			name:    "chat.expanded_height too high",
			mutate:  func(c *Config) { c.AI.Chat.ExpandedHeight = maxChatHeight + 1 },
			wantErr: "ai.chat.expanded_height",
		},
		// mcp.security
		{
			name:    "rate_limit_read too high",
			mutate:  func(c *Config) { c.MCP.Security.RateLimitRead = maxRateLimitRead + 1 },
			wantErr: "mcp.security.rate_limit_read",
		},
		{
			name:    "rate_limit_write too high",
			mutate:  func(c *Config) { c.MCP.Security.RateLimitWrite = maxRateLimitWrite + 1 },
			wantErr: "mcp.security.rate_limit_write",
		},
		{
			name:    "max_agent_processes too high",
			mutate:  func(c *Config) { c.MCP.Security.MaxAgentProcesses = maxAgentProcesses + 1 },
			wantErr: "mcp.security.max_agent_processes",
		},
		{
			name:    "agent_timeout too high",
			mutate:  func(c *Config) { c.MCP.Security.AgentTimeout = maxAgentTimeout + 1 },
			wantErr: "mcp.security.agent_timeout",
		},
		// extensions
		{
			name:    "lua_timeout_ms too high",
			mutate:  func(c *Config) { c.Extensions.LuaTimeoutMs = maxLuaTimeout + 1 },
			wantErr: "extensions.lua_timeout_ms",
		},
		{
			name:    "wasm_memory_limit too high",
			mutate:  func(c *Config) { c.Extensions.WasmMemoryLimit = maxWasmMemoryLimit + 1 },
			wantErr: "extensions.wasm_memory_limit",
		},
		// logging
		{
			name:    "max_size_mb too high",
			mutate:  func(c *Config) { c.Logging.MaxSizeMB = maxLogSizeMB + 1 },
			wantErr: "logging.max_size_mb",
		},
		{
			name:    "max_backups too high",
			mutate:  func(c *Config) { c.Logging.MaxBackups = maxLogBackups + 1 },
			wantErr: "logging.max_backups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCfg(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err, "expected validation error for %s", tt.name)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestValidate_HardLimitsAtBoundary verifies that values exactly at the hard
// limit are accepted (limits are inclusive).
func TestValidate_HardLimitsAtBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
	}{
		{"max_depth at limit", func(c *Config) { c.FileTree.MaxDepth = maxMaxDepth }},
		{"scrollback at limit", func(c *Config) { c.Terminal.Scrollback = maxScrollback }},
		{"render_fps at limit", func(c *Config) { c.Terminal.RenderFPS = maxRenderFPS }},
		{"max_log_entries at limit", func(c *Config) { c.Git.MaxLogEntries = maxMaxLogEntries }},
		{"poll_interval at limit", func(c *Config) { c.GitHub.PollInterval = maxPollInterval }},
		{"review_diff_context at limit", func(c *Config) { c.GitHub.ReviewDiffContextLines = maxDiffContext }},
		{"max_context_files at limit", func(c *Config) { c.AI.MaxContextFiles = maxContextFiles }},
		{"max_context_tokens at limit", func(c *Config) { c.AI.MaxContextTokens = maxContextTokens }},
		{"claude.max_tokens at limit", func(c *Config) { c.AI.Claude.MaxTokens = maxClaudeTokens }},
		{"collapsed_height at limit", func(c *Config) { c.AI.Chat.CollapsedHeight = maxChatHeight }},
		{"expanded_height at limit", func(c *Config) { c.AI.Chat.ExpandedHeight = maxChatHeight }},
		{"rate_limit_read at limit", func(c *Config) { c.MCP.Security.RateLimitRead = maxRateLimitRead }},
		{"rate_limit_write at limit", func(c *Config) { c.MCP.Security.RateLimitWrite = maxRateLimitWrite }},
		{"max_agent_processes at limit", func(c *Config) { c.MCP.Security.MaxAgentProcesses = maxAgentProcesses }},
		{"agent_timeout at limit", func(c *Config) { c.MCP.Security.AgentTimeout = maxAgentTimeout }},
		{"lua_timeout_ms at limit", func(c *Config) { c.Extensions.LuaTimeoutMs = maxLuaTimeout }},
		{"wasm_memory_limit at limit", func(c *Config) { c.Extensions.WasmMemoryLimit = maxWasmMemoryLimit }},
		{"max_size_mb at limit", func(c *Config) { c.Logging.MaxSizeMB = maxLogSizeMB }},
		{"max_backups at limit", func(c *Config) { c.Logging.MaxBackups = maxLogBackups }},
		{"max_file_size at limit", func(c *Config) { c.Preview.MaxFileSize = maxMaxFileSize * 1024 * 1024 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCfg(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			assert.NoError(t, err, "value at exact limit should be accepted")
		})
	}
}

// ---------------------------------------------------------------------------
// Array size limits
// ---------------------------------------------------------------------------

func TestValidate_ArraySizeLimits(t *testing.T) {
	t.Run("redact_patterns over limit", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.AI.RedactPatterns = make([]string, maxRedactPatterns+1)
		for i := range cfg.AI.RedactPatterns {
			cfg.AI.RedactPatterns[i] = "*.log"
		}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ai.redact_patterns")
		assert.Contains(t, err.Error(), "too many entries")
	})

	t.Run("review.categories over limit", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.AI.Review.Categories = make([]string, maxReviewCategories+1)
		for i := range cfg.AI.Review.Categories {
			cfg.AI.Review.Categories[i] = "cat"
		}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ai.review.categories")
	})

	t.Run("bookmarks.paths over limit", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.Bookmarks.Paths = make([]string, maxBookmarkPaths+1)
		for i := range cfg.Bookmarks.Paths {
			cfg.Bookmarks.Paths[i] = "/some/path"
		}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bookmarks.paths")
	})

	t.Run("shortcuts.custom over limit", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.Shortcuts.Custom = make([]CustomShortcut, maxCustomShortcuts+1)
		for i := range cfg.Shortcuts.Custom {
			cfg.Shortcuts.Custom[i] = CustomShortcut{Name: "sc", Steps: []string{"echo hi"}}
		}
		err := Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shortcuts.custom")
	})

	t.Run("arrays at exact limit are accepted", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.AI.RedactPatterns = make([]string, maxRedactPatterns)
		for i := range cfg.AI.RedactPatterns {
			cfg.AI.RedactPatterns[i] = "*.log"
		}
		err := Validate(cfg)
		assert.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// UNC path rejection (path security)
// ---------------------------------------------------------------------------

func TestValidate_RejectUNCPaths(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{
			name:    "logging.file UNC path",
			mutate:  func(c *Config) { c.Logging.File = `\\evil-server\share\log.txt` },
			wantErr: "logging.file",
		},
		{
			name:    "mcp.security.audit_log_path UNC path",
			mutate:  func(c *Config) { c.MCP.Security.AuditLogPath = `\\attacker\smb\audit.log` },
			wantErr: "mcp.security.audit_log_path",
		},
		{
			name:    "extensions.install_dir UNC path",
			mutate:  func(c *Config) { c.Extensions.InstallDir = `\\remote\extensions` },
			wantErr: "extensions.install_dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCfg(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "UNC paths")
		})
	}

	t.Run("normal local paths are accepted", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.Logging.File = "/var/log/grut.log"
		cfg.MCP.Security.AuditLogPath = "C:\\Users\\me\\audit.log"
		cfg.Extensions.InstallDir = "/home/user/.grut/extensions"
		err := Validate(cfg)
		assert.NoError(t, err)
	})

	t.Run("empty paths skip UNC check", func(t *testing.T) {
		cfg := validCfg(t)
		cfg.Logging.File = ""
		cfg.MCP.Security.AuditLogPath = ""
		cfg.Extensions.InstallDir = ""
		err := Validate(cfg)
		assert.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// rejectUNCPath unit test
// ---------------------------------------------------------------------------

func TestValidate_EnumErrorRedactsSecretValue(t *testing.T) {
	cfg := validCfg(t)
	cfg.AI.Provider = "sk-abcdefghijklmnopqrstuvwxyz012345"
	err := Validate(cfg)
	require.Error(t, err)
	// The mistyped secret must never be echoed back in the validation error,
	// which `grut doctor`/`grut config` print to the terminal.
	assert.NotContains(t, err.Error(), "sk-abcdefghijklmnopqrstuvwxyz012345")
	assert.Contains(t, err.Error(), "[redacted]")
}

func TestRejectUNCPath(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"UNC path", `\\server\share`, true},
		{"UNC path with extra", `\\192.168.1.1\c$`, true},
		{"forward-slash UNC", "//evil/share/log.txt", true},
		{"forward-slash UNC short", "//server/x", true},
		{"normal Unix path", "/var/log/test.log", false},
		{"normal Windows path", `C:\Users\test\log.txt`, false},
		{"relative path", "logs/app.log", false},
		{"empty string", "", false},
		{"single backslash", `\temp`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := rejectUNCPath(nil, "test.field", tt.value)
			if tt.wantErr {
				require.Len(t, errs, 1)
				assert.Contains(t, errs[0].Error(), "UNC paths")
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Multiple hard-limit violations in a single config
// ---------------------------------------------------------------------------

func TestValidate_MultipleHardLimitViolations(t *testing.T) {
	cfg := validCfg(t)
	cfg.Terminal.Scrollback = maxScrollback + 1
	cfg.Terminal.RenderFPS = maxRenderFPS + 1
	cfg.Logging.MaxSizeMB = maxLogSizeMB + 1

	err := Validate(cfg)
	require.Error(t, err)
	msg := err.Error()

	// All three violations should be reported (errors.Join collects them all).
	assert.True(t, strings.Contains(msg, "terminal.scrollback"), "should report scrollback error")
	assert.True(t, strings.Contains(msg, "terminal.render_fps"), "should report render_fps error")
	assert.True(t, strings.Contains(msg, "logging.max_size_mb"), "should report max_size_mb error")
}

// ---------------------------------------------------------------------------
// Hard limit constants are documented and sane
// ---------------------------------------------------------------------------

func TestHardLimitConstants_Positive(t *testing.T) {
	// All hard limits must be positive (safety check against typos).
	limits := []struct {
		name  string
		value int
	}{
		{"maxMaxDepth", maxMaxDepth},
		{"maxPreviewWidth", maxPreviewWidth},
		{"maxMaxFileSize", maxMaxFileSize},
		{"maxScrollback", maxScrollback},
		{"maxRenderFPS", maxRenderFPS},
		{"maxMaxLogEntries", maxMaxLogEntries},
		{"maxPollInterval", maxPollInterval},
		{"maxDiffContext", maxDiffContext},
		{"maxContextFiles", maxContextFiles},
		{"maxContextTokens", maxContextTokens},
		{"maxClaudeTokens", maxClaudeTokens},
		{"maxChatHeight", maxChatHeight},
		{"maxRateLimitRead", maxRateLimitRead},
		{"maxRateLimitWrite", maxRateLimitWrite},
		{"maxAgentProcesses", maxAgentProcesses},
		{"maxAgentTimeout", maxAgentTimeout},
		{"maxLuaTimeout", maxLuaTimeout},
		{"maxWasmMemoryLimit", maxWasmMemoryLimit},
		{"maxRedactPatterns", maxRedactPatterns},
		{"maxReviewCategories", maxReviewCategories},
		{"maxCustomShortcuts", maxCustomShortcuts},
		{"maxBookmarkPaths", maxBookmarkPaths},
		{"maxLogSizeMB", maxLogSizeMB},
		{"maxLogBackups", maxLogBackups},
	}
	for _, l := range limits {
		assert.Positive(t, l.value, "%s must be positive", l.name)
	}
}

// ---------------------------------------------------------------------------
// AllowedWritePaths security validation
// ---------------------------------------------------------------------------

func TestValidate_AllowedWritePathsTraversal(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantErr string
	}{
		{
			name:    "dotdot forward slash",
			paths:   []string{"~/../../etc"},
			wantErr: "must not contain '..'",
		},
		{
			name:    "dotdot backslash",
			paths:   []string{`~\..\..\etc`},
			wantErr: "must not contain '..'",
		},
		{
			name:    "dotdot middle",
			paths:   []string{"/home/user/../../../etc/passwd"},
			wantErr: "must not contain '..'",
		},
		{
			name:  "valid absolute path",
			paths: []string{"/home/user/repos"},
		},
		{
			name:  "valid tilde path",
			paths: []string{"~/repos"},
		},
		{
			name:  "empty list ok",
			paths: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validCfg(t)
			cfg.MCP.Security.AllowedWritePaths = tc.paths
			err := Validate(cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidate_AllowedWritePathsUNC(t *testing.T) {
	cfg := validCfg(t)
	cfg.MCP.Security.AllowedWritePaths = []string{`\\evil-server\share`}
	err := Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNC paths")
}

func TestRejectTraversalPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "clean path", path: "/home/user/repo", wantErr: false},
		{name: "dotdot unix", path: "/home/../etc", wantErr: true},
		{name: "dotdot windows", path: `C:\Users\..\Admin`, wantErr: true},
		{name: "dotdot start", path: "../escape", wantErr: true},
		{name: "dotdot end", path: "/tmp/dir/..", wantErr: true},
		{name: "single dot ok", path: "/tmp/./dir", wantErr: false},
		{name: "dots in name ok", path: "/tmp/my..file.txt", wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := rejectTraversalPath(nil, "test.field", tc.path)
			if tc.wantErr {
				assert.Len(t, errs, 1)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}
