package config

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adrg/xdg"
	"github.com/jongio/grut/internal/actions"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Embedded defaults
// ---------------------------------------------------------------------------

func TestLoadEmbeddedDefaults(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(defaultsTOML, cfg))

	// Spot-check a representative field from every section.
	assert.Equal(t, "default", cfg.General.KeybindingScheme)
	assert.Equal(t, "explorer", cfg.General.DefaultLayout)
	assert.True(t, cfg.General.AutoSaveSession)
	assert.True(t, cfg.General.ShowFirstRunHelp)

	assert.True(t, cfg.FileTree.ShowHidden)
	assert.True(t, cfg.FileTree.ShowIcons)
	assert.Equal(t, "auto", cfg.FileTree.IconMode)
	assert.Equal(t, 20, cfg.FileTree.MaxDepth)

	assert.True(t, cfg.Preview.Enabled)
	assert.Equal(t, 40, cfg.Preview.Width)
	assert.Equal(t, 1048576, cfg.Preview.MaxFileSize)

	assert.Equal(t, "fsnotify", cfg.Git.RefreshMethod)
	assert.Equal(t, 2*time.Second, cfg.Git.RefreshFallbackInterval.Duration)
	assert.Equal(t, "main", cfg.Git.DefaultBranch)
	assert.Equal(t, 5*time.Minute, cfg.Git.AutoFetchInterval.Duration)
	assert.Equal(t, 500, cfg.Git.MaxLogEntries)

	assert.Equal(t, 10000, cfg.Terminal.Scrollback)
	assert.Equal(t, 30, cfg.Terminal.RenderFPS)
	assert.Equal(t, "ctrl+b", cfg.Terminal.PrefixKey)

	assert.Equal(t, "manual", cfg.AI.ContextMode)
	assert.Equal(t, "gpt-4", cfg.AI.TokenModel)

	assert.True(t, cfg.MCP.Security.RequireConfirmation)
	assert.Equal(t, 1000, cfg.MCP.Security.RateLimitRead)
	assert.Equal(t, 100, cfg.MCP.Security.RateLimitWrite)
	assert.Equal(t, 5, cfg.MCP.Security.MaxAgentProcesses)
	assert.Equal(t, 1800, cfg.MCP.Security.AgentTimeout)

	assert.True(t, cfg.Extensions.Enabled)
	assert.Equal(t, "https://registry.grut.dev", cfg.Extensions.RegistryURL)
	assert.Equal(t, 100, cfg.Extensions.LuaTimeoutMs)
	assert.Equal(t, 67108864, cfg.Extensions.WasmMemoryLimit)

	assert.Equal(t, "warn", cfg.Logging.Level)
	assert.Equal(t, 10, cfg.Logging.MaxSizeMB)
	assert.Equal(t, 3, cfg.Logging.MaxBackups)

	assert.False(t, cfg.Bookmarks.ShowInSidebar)
	assert.Equal(t, "default", cfg.Theme.Name)
	assert.Equal(t, "auto", cfg.Theme.ColorMode)
}

func TestDefaultsTOMLReturnsCopy(t *testing.T) {
	got := DefaultsTOML()

	require.NotEmpty(t, got)
	assert.Contains(t, string(got), "[general]")
	got[0] = '#'
	assert.NotEqual(t, got[0], DefaultsTOML()[0])
}

func TestLoadEmbeddedDefaultsValidate(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(defaultsTOML, cfg))
	require.NoError(t, Validate(cfg))
}

func TestCustomActionsParseFromTOML(t *testing.T) {
	data := []byte(`
[[custom_actions]]
name = "Test"
command = "go test ./..."
cwd = "services/api"
key = "ctrl+t"
prompt = "Run tests?"
confirm = true

[[custom_actions]]
name = "Generate"
command = "go generate ./..."
`)

	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(data, cfg))

	require.Len(t, cfg.CustomActions, 2)
	assert.Equal(t, CustomAction{
		Name:    "Test",
		Command: "go test ./...",
		WorkDir: "services/api",
		Key:     "ctrl+t",
		Prompt:  "Run tests?",
		Confirm: true,
	}, cfg.CustomActions[0])
	assert.Equal(t, CustomAction{
		Name:    "Generate",
		Command: "go generate ./...",
	}, cfg.CustomActions[1])
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestValidateEnumFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		errMsg string
	}{
		{
			name:   "invalid keybinding_scheme",
			mutate: func(c *Config) { c.General.KeybindingScheme = "emacs" },
			errMsg: "general.keybinding_scheme",
		},
		{
			name:   "invalid default_layout",
			mutate: func(c *Config) { c.General.DefaultLayout = "dashboard" },
			errMsg: "general.default_layout",
		},
		{
			name:   "invalid icon_mode",
			mutate: func(c *Config) { c.FileTree.IconMode = "emoji" },
			errMsg: "file_tree.icon_mode",
		},
		{
			name:   "invalid refresh_method",
			mutate: func(c *Config) { c.Git.RefreshMethod = "inotify" },
			errMsg: "git.refresh_method",
		},
		{
			name:   "invalid worktree_merge_method",
			mutate: func(c *Config) { c.Git.WorktreeMergeMethod = "cherry-pick" },
			errMsg: "git.worktree_merge_method",
		},
		{
			name:   "invalid context_mode",
			mutate: func(c *Config) { c.AI.ContextMode = "auto" },
			errMsg: "ai.context_mode",
		},
		{
			name:   "invalid log level",
			mutate: func(c *Config) { c.Logging.Level = "trace" },
			errMsg: "logging.level",
		},
		{
			name:   "invalid theme color mode",
			mutate: func(c *Config) { c.Theme.ColorMode = "sepia" },
			errMsg: "theme.color_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestValidateCustomActions(t *testing.T) {
	tests := []struct {
		name    string
		actions []CustomAction
		errMsg  string
	}{
		{
			name:    "valid",
			actions: []CustomAction{{Name: "Test", Command: "go test ./...", Key: "ctrl+t"}},
		},
		{
			name:    "missing name",
			actions: []CustomAction{{Command: "go test ./..."}},
			errMsg:  "custom_actions[0].name",
		},
		{
			name:    "missing command",
			actions: []CustomAction{{Name: "Test"}},
			errMsg:  "custom_actions[0].command",
		},
		{
			name: "duplicate name",
			actions: []CustomAction{
				{Name: "Test", Command: "go test ./..."},
				{Name: "Test", Command: "go vet ./..."},
			},
			errMsg: "duplicates custom_actions[0].name",
		},
		{
			name: "duplicate key",
			actions: []CustomAction{
				{Name: "Test", Command: "go test ./...", Key: "ctrl+t"},
				{Name: "Vet", Command: "go vet ./...", Key: "ctrl+t"},
			},
			errMsg: "duplicates custom_actions[0].key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.CustomActions = tt.actions
			err := Validate(cfg)
			if tt.errMsg == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestValidateNumericRanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		errMsg string
	}{
		{
			name:   "preview.width too low",
			mutate: func(c *Config) { c.Preview.Width = 0 },
			errMsg: "preview.width",
		},
		{
			name:   "preview.width too high",
			mutate: func(c *Config) { c.Preview.Width = 101 },
			errMsg: "preview.width",
		},
		{
			name:   "file_tree.max_depth zero",
			mutate: func(c *Config) { c.FileTree.MaxDepth = 0 },
			errMsg: "file_tree.max_depth",
		},
		{
			name:   "terminal.scrollback zero",
			mutate: func(c *Config) { c.Terminal.Scrollback = 0 },
			errMsg: "terminal.scrollback",
		},
		{
			name:   "git.max_log_entries zero",
			mutate: func(c *Config) { c.Git.MaxLogEntries = 0 },
			errMsg: "git.max_log_entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := validConfig(t)
	cfg.General.KeybindingScheme = "bad"
	cfg.Preview.Width = 200
	cfg.Logging.Level = "nope"

	err := Validate(cfg)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "general.keybinding_scheme")
	assert.Contains(t, msg, "preview.width")
	assert.Contains(t, msg, "logging.level")
}

// ---------------------------------------------------------------------------
// User config override
// ---------------------------------------------------------------------------

func TestUserConfigOverridesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "grut")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))

	override := `
[general]
keybinding_scheme = "vim"

[preview]
width = 60

[logging]
level = "debug"
`
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(override), 0o600))

	// Load defaults manually and overlay the user file.
	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(defaultsTOML, cfg))

	data, err := os.ReadFile(filepath.Join(cfgDir, "config.toml"))
	require.NoError(t, err)
	require.NoError(t, toml.Unmarshal(data, cfg))

	// Overridden values.
	assert.Equal(t, "vim", cfg.General.KeybindingScheme)
	assert.Equal(t, 60, cfg.Preview.Width)
	assert.Equal(t, "debug", cfg.Logging.Level)

	// Defaults preserved for non-overridden values.
	assert.Equal(t, "explorer", cfg.General.DefaultLayout)
	assert.True(t, cfg.Preview.Enabled)
	assert.Equal(t, "fsnotify", cfg.Git.RefreshMethod)
}

// ---------------------------------------------------------------------------
// Tilde expansion
// ---------------------------------------------------------------------------

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
		{"~/Documents", filepath.Join(home, "Documents")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandTilde(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpandPathsApplied(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	cfg := validConfig(t)
	cfg.Logging.File = "~/grut.log"
	cfg.Extensions.InstallDir = "~/exts"
	cfg.MCP.Security.AuditLogPath = "~/audit.log"
	cfg.Bookmarks.Paths = []string{"~/projects", "/abs"}

	expandPaths(cfg)

	assert.Equal(t, filepath.Join(home, "grut.log"), cfg.Logging.File)
	assert.Equal(t, filepath.Join(home, "exts"), cfg.Extensions.InstallDir)
	assert.Equal(t, filepath.Join(home, "audit.log"), cfg.MCP.Security.AuditLogPath)
	assert.Equal(t, filepath.Join(home, "projects"), cfg.Bookmarks.Paths[0])
	assert.Equal(t, "/abs", cfg.Bookmarks.Paths[1])
}

// ---------------------------------------------------------------------------
// ConfigDir / DataDir
// ---------------------------------------------------------------------------

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	assert.True(t, strings.HasSuffix(dir, AppName), "ConfigDir should end with %q, got %q", AppName, dir)
	assert.NotEmpty(t, dir)
}

func TestDataDir(t *testing.T) {
	dir := DataDir()
	assert.True(t, strings.HasSuffix(dir, AppName), "DataDir should end with %q, got %q", AppName, dir)
	assert.NotEmpty(t, dir)
}

// ---------------------------------------------------------------------------
// Duration unmarshalling
// ---------------------------------------------------------------------------

func TestDurationUnmarshal(t *testing.T) {
	var d Duration
	require.NoError(t, d.UnmarshalText([]byte("3s")))
	assert.Equal(t, 3*time.Second, d.Duration)

	require.NoError(t, d.UnmarshalText([]byte("10m")))
	assert.Equal(t, 10*time.Minute, d.Duration)

	require.Error(t, d.UnmarshalText([]byte("not-a-duration")))
}

func TestDurationMarshal(t *testing.T) {
	d := Duration{Duration: 5 * time.Second}
	b, err := d.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "5s", string(b))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// validConfig returns a Config initialised from embedded defaults.
func validConfig(t *testing.T) *Config {
	t.Helper()
	cfg := &Config{}
	require.NoError(t, toml.Unmarshal(defaultsTOML, cfg))
	return cfg
}

// ---------------------------------------------------------------------------
// Load() integration tests
// ---------------------------------------------------------------------------

func TestLoadReturnsValidConfig(t *testing.T) {
	// Load() should succeed even without a user config file present.
	// It relies on embedded defaults and XDG paths, which are available
	// in all test environments.
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify it matches embedded defaults.
	assert.Equal(t, "default", cfg.General.KeybindingScheme)
	assert.Equal(t, "explorer", cfg.General.DefaultLayout)
	assert.True(t, cfg.Preview.Enabled)
}

func TestConfigFilePath(t *testing.T) {
	path := configFilePath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, AppName)
	assert.True(t, strings.HasSuffix(path, "config.toml"))
}

func TestUserConfigFilePath(t *testing.T) {
	assert.Equal(t, configFilePath(), UserConfigFilePath())
}

func TestWarnIfWorldReadableNoOp(t *testing.T) {
	// On Windows this is a no-op; on Unix it should not panic.
	// Just ensure it doesn't crash.
	warnIfWorldReadable(filepath.Join(t.TempDir(), "nonexistent"))
}

// ---------------------------------------------------------------------------
// Additional validation edge cases
// ---------------------------------------------------------------------------

func TestValidateNegativeFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		errMsg string
	}{
		{
			name:   "preview.max_file_size negative",
			mutate: func(c *Config) { c.Preview.MaxFileSize = -1 },
			errMsg: "preview.max_file_size",
		},
		{
			name:   "terminal.render_fps zero",
			mutate: func(c *Config) { c.Terminal.RenderFPS = 0 },
			errMsg: "terminal.render_fps",
		},
		{
			name:   "mcp.security.rate_limit_read negative",
			mutate: func(c *Config) { c.MCP.Security.RateLimitRead = -1 },
			errMsg: "mcp.security.rate_limit_read",
		},
		{
			name:   "mcp.security.rate_limit_write negative",
			mutate: func(c *Config) { c.MCP.Security.RateLimitWrite = -1 },
			errMsg: "mcp.security.rate_limit_write",
		},
		{
			name:   "mcp.security.max_agent_processes negative",
			mutate: func(c *Config) { c.MCP.Security.MaxAgentProcesses = -1 },
			errMsg: "mcp.security.max_agent_processes",
		},
		{
			name:   "mcp.security.agent_timeout negative",
			mutate: func(c *Config) { c.MCP.Security.AgentTimeout = -1 },
			errMsg: "mcp.security.agent_timeout",
		},
		{
			name:   "extensions.lua_timeout_ms negative",
			mutate: func(c *Config) { c.Extensions.LuaTimeoutMs = -1 },
			errMsg: "extensions.lua_timeout_ms",
		},
		{
			name:   "extensions.wasm_memory_limit negative",
			mutate: func(c *Config) { c.Extensions.WasmMemoryLimit = -1 },
			errMsg: "extensions.wasm_memory_limit",
		},
		{
			name:   "logging.max_size_mb negative",
			mutate: func(c *Config) { c.Logging.MaxSizeMB = -1 },
			errMsg: "logging.max_size_mb",
		},
		{
			name:   "logging.max_backups negative",
			mutate: func(c *Config) { c.Logging.MaxBackups = -1 },
			errMsg: "logging.max_backups",
		},
		{
			name:   "git.refresh_fallback_interval zero",
			mutate: func(c *Config) { c.Git.RefreshFallbackInterval = Duration{} },
			errMsg: "git.refresh_fallback_interval",
		},
		{
			name:   "git.auto_fetch_interval zero",
			mutate: func(c *Config) { c.Git.AutoFetchInterval = Duration{} },
			errMsg: "git.auto_fetch_interval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestValidateCustomKeybindingPath(t *testing.T) {
	cfg := validConfig(t)
	cfg.General.KeybindingScheme = "/custom/path/to/scheme.toml"
	// Custom paths with "/" or "\" are accepted by validation.
	err := Validate(cfg)
	require.NoError(t, err)
}

func TestValidateValidConfig(t *testing.T) {
	cfg := validConfig(t)
	require.NoError(t, Validate(cfg))
}

// ---------------------------------------------------------------------------
// AI config defaults
// ---------------------------------------------------------------------------

func TestAIConfigDefaults(t *testing.T) {
	cfg := validConfig(t)

	// Top-level AI fields.
	assert.True(t, cfg.AI.Enabled)
	assert.Equal(t, "copilot", cfg.AI.Provider)
	assert.Equal(t, "claude", cfg.AI.FallbackProvider)
	assert.Equal(t, []string{".env", "*.key", "*.pem", "*.secret"}, cfg.AI.RedactPatterns)
	assert.True(t, cfg.AI.AutoCommitMsg)
	assert.False(t, cfg.AI.AutoReviewDiff)
	assert.InDelta(t, 0.2, cfg.AI.Temperature, 1e-9)
	assert.Equal(t, 20, cfg.AI.MaxContextFiles)
	assert.Equal(t, 100000, cfg.AI.MaxContextTokens)

	// Copilot.
	assert.Equal(t, "", cfg.AI.Copilot.Model)

	// Claude.
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.AI.Claude.Model)
	assert.Equal(t, 8192, cfg.AI.Claude.MaxTokens)

	// Review.
	assert.Equal(t, "warning", cfg.AI.Review.SeverityThreshold)
	assert.False(t, cfg.AI.Review.AutoReviewOnPush)
	assert.Equal(t, []string{"security", "bug", "performance", "style", "test"}, cfg.AI.Review.Categories)

	// Conflict.
	assert.Equal(t, "interactive", cfg.AI.Conflict.DefaultMode)
	assert.Equal(t, 50, cfg.AI.Conflict.IncludeSurroundingContext)
	assert.False(t, cfg.AI.Conflict.AutoAcceptHighConfidence)

	// Changelog.
	assert.Equal(t, "keepachangelog", cfg.AI.Changelog.Format)
	assert.Equal(t, []string{"added", "changed", "fixed", "removed", "security", "deprecated"}, cfg.AI.Changelog.Categories)

	// CommitSplit.
	assert.Equal(t, 10, cfg.AI.CommitSplit.Threshold)

	// Chat.
	assert.True(t, cfg.AI.Chat.Enabled)
	assert.Equal(t, 3, cfg.AI.Chat.CollapsedHeight)
	assert.Equal(t, 8, cfg.AI.Chat.ExpandedHeight)
	assert.Equal(t, "", cfg.AI.Chat.SystemPrompt)
}

// ---------------------------------------------------------------------------
// AI config validation — new enum fields
// ---------------------------------------------------------------------------

func TestValidateAIEnumFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		errMsg string
	}{
		{
			name:   "invalid ai.provider",
			mutate: func(c *Config) { c.AI.Provider = "openai" },
			errMsg: "ai.provider",
		},
		{
			name:   "invalid ai.fallback_provider",
			mutate: func(c *Config) { c.AI.FallbackProvider = "gemini" },
			errMsg: "ai.fallback_provider",
		},
		{
			name:   "empty fallback_provider is valid",
			mutate: func(c *Config) { c.AI.FallbackProvider = "" },
		},
		{
			name:   "invalid ai.review.severity_threshold",
			mutate: func(c *Config) { c.AI.Review.SeverityThreshold = "critical" },
			errMsg: "ai.review.severity_threshold",
		},
		{
			name:   "invalid ai.conflict.default_mode",
			mutate: func(c *Config) { c.AI.Conflict.DefaultMode = "manual" },
			errMsg: "ai.conflict.default_mode",
		},
		{
			name:   "invalid ai.changelog.format",
			mutate: func(c *Config) { c.AI.Changelog.Format = "conventional" },
			errMsg: "ai.changelog.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			if tt.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AI config validation — numeric ranges
// ---------------------------------------------------------------------------

func TestValidateAINumericRanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		errMsg string
	}{
		{
			name:   "temperature below zero",
			mutate: func(c *Config) { c.AI.Temperature = -0.1 },
			errMsg: "ai.temperature",
		},
		{
			name:   "temperature above one",
			mutate: func(c *Config) { c.AI.Temperature = 1.5 },
			errMsg: "ai.temperature",
		},
		{
			name:   "temperature at zero is valid",
			mutate: func(c *Config) { c.AI.Temperature = 0 },
		},
		{
			name:   "temperature at one is valid",
			mutate: func(c *Config) { c.AI.Temperature = 1 },
		},
		{
			name:   "max_context_files zero",
			mutate: func(c *Config) { c.AI.MaxContextFiles = 0 },
			errMsg: "ai.max_context_files",
		},
		{
			name:   "max_context_tokens zero",
			mutate: func(c *Config) { c.AI.MaxContextTokens = 0 },
			errMsg: "ai.max_context_tokens",
		},
		{
			name:   "claude.max_tokens negative",
			mutate: func(c *Config) { c.AI.Claude.MaxTokens = -1 },
			errMsg: "ai.claude.max_tokens",
		},
		{
			name:   "conflict.include_surrounding_context negative",
			mutate: func(c *Config) { c.AI.Conflict.IncludeSurroundingContext = -1 },
			errMsg: "ai.conflict.include_surrounding_context",
		},
		{
			name:   "commit_split.threshold zero",
			mutate: func(c *Config) { c.AI.CommitSplit.Threshold = 0 },
			errMsg: "ai.commit_split.threshold",
		},
		{
			name:   "chat.collapsed_height zero",
			mutate: func(c *Config) { c.AI.Chat.CollapsedHeight = 0 },
			errMsg: "ai.chat.collapsed_height",
		},
		{
			name:   "chat.expanded_height zero",
			mutate: func(c *Config) { c.AI.Chat.ExpandedHeight = 0 },
			errMsg: "ai.chat.expanded_height",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			if tt.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AI config validation — embedded API key rejection
// ---------------------------------------------------------------------------

func TestValidateRejectsEmbeddedAPIKeys(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name:   "OpenAI key in provider",
			mutate: func(c *Config) { c.AI.Provider = "sk-abcdefghijklmnopqrstuvwx" },
		},
		{
			name:   "AWS key in fallback_provider",
			mutate: func(c *Config) { c.AI.FallbackProvider = "AKIA1234567890123456" },
		},
		{
			name:   "GitHub PAT in copilot.model",
			mutate: func(c *Config) { c.AI.Copilot.Model = "ghp_abcdefghijklmnopqrstuvwxyz1234567890" },
		},
		{
			name:   "GitLab PAT in claude.model",
			mutate: func(c *Config) { c.AI.Claude.Model = "glpat-abcdefghijklmnopqrstuvwx" },
		},
		{
			name:   "Slack token in chat.system_prompt",
			mutate: func(c *Config) { c.AI.Chat.SystemPrompt = "xoxb-some-slack-token-value" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "embedded API key")
		})
	}
}

// ---------------------------------------------------------------------------
// AI config validation — embedded API key rejection (extended coverage)
// ---------------------------------------------------------------------------

func TestValidateRejectsGitHubOAuthKeyInConfig(t *testing.T) {
	cfg := validConfig(t)
	// gho_ (GitHub OAuth) is in the regex but was not previously tested.
	cfg.AI.Copilot.Model = "gho_abcdefghijklmnopqrstuvwxyz1234567890"
	err := Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedded API key")
}

func TestValidateRejectsAPIKeyInRedactPatterns(t *testing.T) {
	cfg := validConfig(t)
	cfg.AI.RedactPatterns = []string{"*.log", "ghp_abcdefghijklmnopqrstuvwxyz1234567890"}
	err := Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redact_patterns")
	assert.Contains(t, err.Error(), "embedded API key")
}

func TestValidateRejectsAPIKeyInReviewCategories(t *testing.T) {
	cfg := validConfig(t)
	cfg.AI.Review.Categories = []string{"security", "sk-abcdefghijklmnopqrstuvwx"}
	err := Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "review.categories")
	assert.Contains(t, err.Error(), "embedded API key")
}

func TestValidateRejectsAPIKeyInChangelogCategories(t *testing.T) {
	cfg := validConfig(t)
	cfg.AI.Changelog.Categories = []string{"AKIA1234567890123456", "added"}
	err := Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changelog.categories")
	assert.Contains(t, err.Error(), "embedded API key")
}

func TestValidateAcceptsCleanSliceFields(t *testing.T) {
	cfg := validConfig(t)
	cfg.AI.RedactPatterns = []string{"*.log", "*.tmp"}
	cfg.AI.Review.Categories = []string{"security", "performance"}
	cfg.AI.Changelog.Categories = []string{"added", "fixed"}
	err := Validate(cfg)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// AI config — user override merging
// ---------------------------------------------------------------------------

func TestAIUserOverrideMergesCorrectly(t *testing.T) {
	cfg := validConfig(t)

	override := `
[ai]
provider = "claude"
temperature = 0.8
max_context_files = 50

[ai.claude]
model = "claude-opus-4-20250514"
max_tokens = 16384

[ai.review]
severity_threshold = "error"
auto_review_on_push = true

[ai.chat]
enabled = false
expanded_height = 15
system_prompt = "You are a helpful assistant."
`
	require.NoError(t, toml.Unmarshal([]byte(override), cfg))
	require.NoError(t, Validate(cfg))

	// Overridden values.
	assert.Equal(t, "claude", cfg.AI.Provider)
	assert.InDelta(t, 0.8, cfg.AI.Temperature, 1e-9)
	assert.Equal(t, 50, cfg.AI.MaxContextFiles)
	assert.Equal(t, "claude-opus-4-20250514", cfg.AI.Claude.Model)
	assert.Equal(t, 16384, cfg.AI.Claude.MaxTokens)
	assert.Equal(t, "error", cfg.AI.Review.SeverityThreshold)
	assert.True(t, cfg.AI.Review.AutoReviewOnPush)
	assert.False(t, cfg.AI.Chat.Enabled)
	assert.Equal(t, 15, cfg.AI.Chat.ExpandedHeight)
	assert.Equal(t, "You are a helpful assistant.", cfg.AI.Chat.SystemPrompt)

	// Non-overridden defaults preserved.
	assert.True(t, cfg.AI.Enabled)
	assert.Equal(t, "claude", cfg.AI.FallbackProvider)
	assert.Equal(t, 100000, cfg.AI.MaxContextTokens)
	assert.Equal(t, "interactive", cfg.AI.Conflict.DefaultMode)
	assert.Equal(t, 10, cfg.AI.CommitSplit.Threshold)
	assert.Equal(t, 3, cfg.AI.Chat.CollapsedHeight)
}

// ---------------------------------------------------------------------------
// setNestedKey (save.go)
// ---------------------------------------------------------------------------

func TestSetNestedKey_SingleLevel(t *testing.T) {
	m := make(map[string]any)
	setNestedKey(m, "foo", "bar")
	assert.Equal(t, "bar", m["foo"])
}

func TestSetNestedKey_TwoLevel(t *testing.T) {
	m := make(map[string]any)
	setNestedKey(m, "foo.bar", "baz")

	sub, ok := m["foo"].(map[string]any)
	require.True(t, ok, "foo should be a nested map")
	assert.Equal(t, "baz", sub["bar"])
}

func TestSetNestedKey_DeepNesting(t *testing.T) {
	m := make(map[string]any)
	setNestedKey(m, "a.b.c.d", "deep")

	a, ok := m["a"].(map[string]any)
	require.True(t, ok)
	b, ok := a["b"].(map[string]any)
	require.True(t, ok)
	c, ok := b["c"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deep", c["d"])
}

func TestSetNestedKey_OverwriteExisting(t *testing.T) {
	m := map[string]any{"foo": "old"}
	setNestedKey(m, "foo", "new")
	assert.Equal(t, "new", m["foo"])
}

func TestSetNestedKey_CreateIntermediateMaps(t *testing.T) {
	// When an intermediate key holds a non-map value, setNestedKey
	// replaces it with a new map.
	m := map[string]any{"a": "scalar"}
	setNestedKey(m, "a.b", "val")

	sub, ok := m["a"].(map[string]any)
	require.True(t, ok, "a should have been replaced with a map")
	assert.Equal(t, "val", sub["b"])
}

func TestSetNestedKey_PreserveSiblings(t *testing.T) {
	m := map[string]any{
		"keep": "me",
		"section": map[string]any{
			"existing": "value",
		},
	}
	setNestedKey(m, "section.new_key", "added")

	assert.Equal(t, "me", m["keep"], "sibling top-level key should be preserved")
	sub := m["section"].(map[string]any)
	assert.Equal(t, "value", sub["existing"], "sibling nested key should be preserved")
	assert.Equal(t, "added", sub["new_key"])
}

// ---------------------------------------------------------------------------
// expandTilde — backslash separator (Windows edge case)
// ---------------------------------------------------------------------------

func TestExpandTilde_BackslashWindows(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	got := expandTilde(`~\folder`)
	assert.Equal(t, filepath.Join(home, `\folder`), got)
}

// ---------------------------------------------------------------------------
// Validation — boundary values (exact min/max)
// ---------------------------------------------------------------------------

func TestValidate_BoundaryValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name:   "preview.width at min boundary (1)",
			mutate: func(c *Config) { c.Preview.Width = 1 },
		},
		{
			name:   "preview.width at max boundary (100)",
			mutate: func(c *Config) { c.Preview.Width = 100 },
		},
		{
			name:   "file_tree.max_depth at min boundary (1)",
			mutate: func(c *Config) { c.FileTree.MaxDepth = 1 },
		},
		{
			name:   "terminal.scrollback at min boundary (1)",
			mutate: func(c *Config) { c.Terminal.Scrollback = 1 },
		},
		{
			name:   "terminal.render_fps at min boundary (1)",
			mutate: func(c *Config) { c.Terminal.RenderFPS = 1 },
		},
		{
			name:   "git.max_log_entries at min boundary (1)",
			mutate: func(c *Config) { c.Git.MaxLogEntries = 1 },
		},
		{
			name:   "preview.max_file_size at zero boundary",
			mutate: func(c *Config) { c.Preview.MaxFileSize = 0 },
		},
		{
			name:   "mcp.security.rate_limit_read at zero boundary",
			mutate: func(c *Config) { c.MCP.Security.RateLimitRead = 0 },
		},
		{
			name:   "mcp.security.rate_limit_write at zero boundary",
			mutate: func(c *Config) { c.MCP.Security.RateLimitWrite = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			require.NoError(t, Validate(cfg), "boundary value should be valid")
		})
	}
}

// ---------------------------------------------------------------------------
// setNestedKey — table-driven (save.go)
// ---------------------------------------------------------------------------

func TestSetNestedKey(t *testing.T) {
	tests := []struct {
		name   string
		init   map[string]any
		key    string
		value  any
		verify func(t *testing.T, m map[string]any)
	}{
		{
			name:  "single level key",
			init:  map[string]any{},
			key:   "foo",
			value: "bar",
			verify: func(t *testing.T, m map[string]any) {
				assert.Equal(t, "bar", m["foo"])
			},
		},
		{
			name:  "two-level key",
			init:  map[string]any{},
			key:   "section.key",
			value: "val",
			verify: func(t *testing.T, m map[string]any) {
				sub, ok := m["section"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "val", sub["key"])
			},
		},
		{
			name:  "three-level key",
			init:  map[string]any{},
			key:   "a.b.c",
			value: "deep",
			verify: func(t *testing.T, m map[string]any) {
				a, ok := m["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "deep", b["c"])
			},
		},
		{
			name:  "overwrite existing value",
			init:  map[string]any{"foo": "old"},
			key:   "foo",
			value: "new",
			verify: func(t *testing.T, m map[string]any) {
				assert.Equal(t, "new", m["foo"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.init
			setNestedKey(m, tt.key, tt.value)
			tt.verify(t, m)
		})
	}
}

// ---------------------------------------------------------------------------
// appendEnumOrPathErr (validate.go)
// ---------------------------------------------------------------------------

func TestAppendEnumOrPathErr(t *testing.T) {
	allowed := []string{"default", "vim"}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid enum value", "default", false},
		{"invalid enum but valid path with /", "/custom/path/scheme.toml", false},
		{"invalid enum but valid path with \\", `C:\configs\scheme.toml`, false},
		{"invalid enum and not a path", "emacs", true},
		{"UNC backslash path rejected", `\\evil\share\scheme.toml`, true},
		{"UNC forward-slash path rejected", "//evil/share/scheme.toml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := appendEnumOrPathErr(nil, "test.field", tt.value, allowed...)
			if tt.wantErr {
				require.Len(t, errs, 1)
				assert.Contains(t, errs[0].Error(), "test.field")
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// rejectEmbeddedKeys (validate.go)
// ---------------------------------------------------------------------------

func TestRejectEmbeddedKeys(t *testing.T) {
	clean := func() AIConfig {
		return AIConfig{
			Provider:    "copilot",
			ContextMode: "manual",
			TokenModel:  "gpt-4",
			Temperature: 0.2,
			Copilot:     CopilotConfig{},
			Claude:      ClaudeConfig{Model: "claude-sonnet-4-20250514"},
			Review:      ReviewConfig{SeverityThreshold: "warning", Categories: []string{"security"}},
			Conflict:    ConflictConfig{DefaultMode: "interactive"},
			Changelog:   ChangelogConfig{Format: "keepachangelog", Categories: []string{"added"}},
			Chat:        ChatConfig{SystemPrompt: ""},
			MCP:         AIMCPConfig{},
		}
	}

	t.Run("OpenAI key detected", func(t *testing.T) {
		ai := clean()
		ai.Provider = "sk-xxxxxxxxxxxxxxxxxxxxxxxx"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("AWS key detected", func(t *testing.T) {
		ai := clean()
		ai.FallbackProvider = "AKIA1234567890ABCDEF"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("GitHub PAT detected", func(t *testing.T) {
		ai := clean()
		ai.Copilot.Model = "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("GitLab token detected", func(t *testing.T) {
		ai := clean()
		ai.Claude.Model = "glpat-xxxxxxxxxxxxxxxxxxxx"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("Slack token detected", func(t *testing.T) {
		ai := clean()
		ai.Chat.SystemPrompt = "xoxb-something"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("GitHub App token detected (gat_)", func(t *testing.T) {
		ai := clean()
		ai.Provider = "gat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "embedded API key")
	})

	t.Run("normal string no error", func(t *testing.T) {
		ai := clean()
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		assert.Empty(t, errs)
	})

	t.Run("key in RedactPatterns slice", func(t *testing.T) {
		ai := clean()
		ai.RedactPatterns = []string{"*.log", "sk-xxxxxxxxxxxxxxxxxxxxxxxx"}
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "redact_patterns")
	})

	t.Run("key in Review.Categories slice", func(t *testing.T) {
		ai := clean()
		ai.Review.Categories = []string{"ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "review.categories")
	})

	t.Run("key in Changelog.Categories slice", func(t *testing.T) {
		ai := clean()
		ai.Changelog.Categories = []string{"AKIA1234567890ABCDEF"}
		errs := rejectEmbeddedKeys(nil, "ai", ai)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "changelog.categories")
	})
}

// ---------------------------------------------------------------------------
// GetDoubleClickAction (actions.go)
// ---------------------------------------------------------------------------

func TestGetDoubleClickAction(t *testing.T) {
	t.Run("nil DoubleClick map returns default", func(t *testing.T) {
		ac := &ActionsConfig{DoubleClick: nil}
		got := ac.GetDoubleClickAction("file")
		assert.Equal(t, "open_in_default_app", got)
	})

	t.Run("override present returns override", func(t *testing.T) {
		ac := &ActionsConfig{DoubleClick: map[string]string{"file": "copy_path"}}
		got := ac.GetDoubleClickAction("file")
		assert.Equal(t, "copy_path", got)
	})

	t.Run("override absent returns default", func(t *testing.T) {
		ac := &ActionsConfig{DoubleClick: map[string]string{"commit": "show_detail"}}
		got := ac.GetDoubleClickAction("file")
		assert.Equal(t, "open_in_default_app", got)
	})
}

// ---------------------------------------------------------------------------
// IsConfirmed (actions.go)
// ---------------------------------------------------------------------------

func TestIsConfirmed(t *testing.T) {
	t.Run("nil Confirmed map returns false", func(t *testing.T) {
		ac := &ActionsConfig{Confirmed: nil}
		assert.False(t, ac.IsConfirmed("file"))
	})

	t.Run("confirmed true returns true", func(t *testing.T) {
		ac := &ActionsConfig{Confirmed: map[string]bool{"file": true}}
		assert.True(t, ac.IsConfirmed("file"))
	})

	t.Run("confirmed not set returns false", func(t *testing.T) {
		ac := &ActionsConfig{Confirmed: map[string]bool{"commit": true}}
		assert.False(t, ac.IsConfirmed("file"))
	})
}

// ---------------------------------------------------------------------------
// GetRightClickAction (actions.go)
// ---------------------------------------------------------------------------

func TestGetRightClickAction(t *testing.T) {
	t.Run("nil RightClick map returns default", func(t *testing.T) {
		ac := &ActionsConfig{RightClick: nil}
		got := ac.GetRightClickAction("file")
		assert.Equal(t, actions.ActionShowContextMenu, got)
	})

	t.Run("valid override returns override", func(t *testing.T) {
		ac := &ActionsConfig{RightClick: map[string]string{"file": "copy_path"}}
		got := ac.GetRightClickAction("file")
		assert.Equal(t, actions.ActionID("copy_path"), got)
	})

	t.Run("invalid override falls back to default", func(t *testing.T) {
		ac := &ActionsConfig{RightClick: map[string]string{"file": "nonexistent_action"}}
		got := ac.GetRightClickAction("file")
		assert.Equal(t, actions.ActionShowContextMenu, got)
	})

	t.Run("show_context_menu override returns it", func(t *testing.T) {
		ac := &ActionsConfig{RightClick: map[string]string{"file": "context_menu"}}
		got := ac.GetRightClickAction("file")
		assert.Equal(t, actions.ActionShowContextMenu, got)
	})
}

// ---------------------------------------------------------------------------
// SaveUserSetting / SaveUserSettingBool (save.go)
// ---------------------------------------------------------------------------

func TestSaveUserSetting(t *testing.T) {
	// Override xdg.ConfigHome to a temp dir so we don't touch real config.
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	// Save a string setting.
	require.NoError(t, SaveUserSetting("preview.position", "bottom"))

	// Read the file back and verify the value.
	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "bottom")

	// Save a boolean setting.
	require.NoError(t, SaveUserSettingBool("general.auto_save_session", false))
	data, err = os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "auto_save_session")
}

func TestSaveSettingValue_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	// File should not exist yet.
	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	_, err := os.Stat(cfgPath)
	require.True(t, errors.Is(err, fs.ErrNotExist))

	// Save creates both the directory and the file.
	require.NoError(t, SaveUserSetting("logging.level", "debug"))
	_, err = os.Stat(cfgPath)
	require.NoError(t, err)
}

func TestSaveSettingValue_PreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	// Write initial setting.
	require.NoError(t, SaveUserSetting("preview.position", "right"))

	// Write a second setting — first should be preserved.
	require.NoError(t, SaveUserSetting("logging.level", "debug"))

	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "right")
	assert.Contains(t, content, "debug")
}

// ---------------------------------------------------------------------------
// SetActionConfirmed / SetDoubleClickAction / SetRightClickAction (actions.go)
// ---------------------------------------------------------------------------

func TestSetActionConfirmed(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	require.NoError(t, SetActionConfirmed("file"))

	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "file")
}

func TestSetDoubleClickAction(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	require.NoError(t, SetDoubleClickAction("file", "copy_path"))

	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "copy_path")
}

func TestSetRightClickAction(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	require.NoError(t, SetRightClickAction(actions.ItemFile, actions.ActionCopyPath))

	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "copy_path")
}

func TestResetAllActionConfirmations(t *testing.T) {
	tmpDir := t.TempDir()
	orig := xdg.ConfigHome
	xdg.ConfigHome = tmpDir
	t.Cleanup(func() { xdg.ConfigHome = orig })

	// First confirm one, then reset all.
	require.NoError(t, SetActionConfirmed("file"))
	require.NoError(t, ResetAllActionConfirmations())

	cfgPath := filepath.Join(tmpDir, AppName, "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "file")
}

// ---------------------------------------------------------------------------
// ResolveGitHubRepo (github.go)
// ---------------------------------------------------------------------------

func TestResolveGitHubRepo_ConfigOverride(t *testing.T) {
	gh := &GitHubConfig{Owner: "myorg", Repo: "myrepo"}
	owner, repo := gh.ResolveGitHubRepo(context.Background(), "/nonexistent")
	assert.Equal(t, "myorg", owner)
	assert.Equal(t, "myrepo", repo)
}

func TestResolveGitHubRepo_AutoDetect(t *testing.T) {
	// Use the current repo as a source of truth for auto-detection.
	gh := &GitHubConfig{}
	owner, repo := gh.ResolveGitHubRepo(context.Background(), ".")
	if owner == "" && repo == "" {
		// Worktrees under WSL may not resolve git remotes; skip gracefully.
		t.Skip("git remote detection unavailable (likely worktree under WSL)")
	}
	// This repo is github.com/jongio/grut.
	assert.Equal(t, "jongio", owner)
	assert.Equal(t, "grut", repo)
}

func TestResolveGitHubRepo_InvalidPath(t *testing.T) {
	gh := &GitHubConfig{}
	owner, repo := gh.ResolveGitHubRepo(context.Background(), filepath.Join(t.TempDir(), "no-git-here"))
	assert.Empty(t, owner)
	assert.Empty(t, repo)
}

// ---------------------------------------------------------------------------
// Validate edge cases (validate.go)
// ---------------------------------------------------------------------------

func TestValidateEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "temperature exactly 0 is valid",
			mutate:  func(c *Config) { c.AI.Temperature = 0 },
			wantErr: false,
		},
		{
			name:    "temperature exactly 1 is valid",
			mutate:  func(c *Config) { c.AI.Temperature = 1 },
			wantErr: false,
		},
		{
			name:    "temperature > 1 is invalid",
			mutate:  func(c *Config) { c.AI.Temperature = 1.01 },
			wantErr: true,
			errMsg:  "ai.temperature",
		},
		{
			name:    "temperature < 0 is invalid",
			mutate:  func(c *Config) { c.AI.Temperature = -0.01 },
			wantErr: true,
			errMsg:  "ai.temperature",
		},
		{
			name:    "MaxContextFiles 0 is invalid",
			mutate:  func(c *Config) { c.AI.MaxContextFiles = 0 },
			wantErr: true,
			errMsg:  "ai.max_context_files",
		},
		{
			name:    "MaxLogEntries 0 is invalid",
			mutate:  func(c *Config) { c.Git.MaxLogEntries = 0 },
			wantErr: true,
			errMsg:  "git.max_log_entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.mutate(cfg)
			err := Validate(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
