// Package config handles application configuration for grut.
// It loads embedded defaults, merges user overrides from the XDG config path,
// validates all values, and expands tildes in path fields.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/adrg/xdg"
	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults.toml
var defaultsTOML []byte

// Config is the top-level configuration for grut.
type Config struct {
	Actions    ActionsConfig    `toml:"actions"`
	Terminal   TerminalConfig   `toml:"terminal"`
	Theme      ThemeConfig      `toml:"theme"`
	AI         AIConfig         `toml:"ai"`
	General    GeneralConfig    `toml:"general"`
	Shortcuts  ShortcutsConfig  `toml:"shortcuts"`
	MCP        MCPConfig        `toml:"mcp"`
	GitHub     GitHubConfig     `toml:"github"`
	Preview    PreviewConfig    `toml:"preview"`
	Editor     EditorConfig     `toml:"editor"`
	Logging    LoggingConfig    `toml:"logging"`
	FileTree   FileTreeConfig   `toml:"file_tree"`
	Bookmarks  BookmarksConfig  `toml:"bookmarks"`
	Extensions ExtensionsConfig `toml:"extensions"`
	Git        GitConfig        `toml:"git"`
	Session    SessionConfig    `toml:"session"`
}

// GeneralConfig holds top-level UI and session preferences.
type GeneralConfig struct {
	KeybindingScheme string `toml:"keybinding_scheme"`
	DefaultLayout    string `toml:"default_layout"`
	AutoSaveSession  bool   `toml:"auto_save_session"`
	ShowFirstRunHelp bool   `toml:"show_first_run_help"`
}

// FileTreeConfig controls the file explorer pane.
type FileTreeConfig struct {
	IconMode             string `toml:"icon_mode"`
	MaxDepth             int    `toml:"max_depth"`
	ShowHidden           bool   `toml:"show_hidden"`
	ShowIcons            bool   `toml:"show_icons"`
	SortDirectoriesFirst bool   `toml:"sort_directories_first"`
	GitStatusMarkers     bool   `toml:"git_status_markers"`
	FollowSymlinks       bool   `toml:"follow_symlinks"`
}

// PreviewConfig controls the file preview pane.
type PreviewConfig struct {
	Position           string `toml:"position"`
	Theme              string `toml:"theme"`
	Width              int    `toml:"width"`
	MaxFileSize        int    `toml:"max_file_size"`
	Enabled            bool   `toml:"enabled"`
	SyntaxHighlighting bool   `toml:"syntax_highlighting"`
	LineNumbers        bool   `toml:"line_numbers"`
	WordWrap           bool   `toml:"word_wrap"`
	RenderMarkdown     bool   `toml:"render_markdown"`
}

// EditorConfig controls the inline file editor in the preview panel.
type EditorConfig struct {
	TabSize    int  `toml:"tab_size"`
	InsertTabs bool `toml:"insert_tabs"` // true = hard tabs, false = spaces
	AutoIndent bool `toml:"auto_indent"`
	AutoSave   bool `toml:"auto_save"`
}

// GitConfig holds git integration settings.
type GitConfig struct {
	RefreshMethod           string   `toml:"refresh_method"`
	DefaultBranch           string   `toml:"default_branch"`
	WorktreeMergeMethod     string   `toml:"worktree_merge_method"`
	WorktreeOpenMode        string   `toml:"worktree_open_mode"`
	RefreshFallbackInterval Duration `toml:"refresh_fallback_interval"`
	AutoFetchInterval       Duration `toml:"auto_fetch_interval"`
	MaxLogEntries           int      `toml:"max_log_entries"`
	WorktreeFirst           bool     `toml:"worktree_first"`
	ShowCommitGraph         bool     `toml:"show_commit_graph"`
	SignCommits             bool     `toml:"sign_commits"`
}

// GitHubConfig holds GitHub integration settings.
type GitHubConfig struct {
	Owner                  string `toml:"owner"`
	Repo                   string `toml:"repo"`
	DefaultIssueFilter     string `toml:"default_issue_filter"`
	DefaultPRFilter        string `toml:"default_pr_filter"`
	PollInterval           int    `toml:"poll_interval"`
	ReviewDiffContextLines int    `toml:"review_diff_context_lines"`
	AutoCheckoutPRBranch   bool   `toml:"auto_checkout_pr_branch"`
	PageSize               int    `toml:"page_size"`
}

// TerminalConfig holds embedded terminal settings.
type TerminalConfig struct {
	Shell      string `toml:"shell"`
	PrefixKey  string `toml:"prefix_key"`
	Scrollback int    `toml:"scrollback"`
	RenderFPS  int    `toml:"render_fps"`
}

// AIConfig holds AI/LLM integration settings.
type AIConfig struct {
	ContextMode      string      `toml:"context_mode"`
	TokenModel       string      `toml:"token_model"`
	MCP              AIMCPConfig `toml:"mcp"`
	Provider         string      `toml:"provider"`          // "copilot" | "claude" | "none"
	FallbackProvider string      `toml:"fallback_provider"` // same enum or ""
	// Sub-feature configuration.
	Copilot          CopilotConfig     `toml:"copilot"`
	Review           ReviewConfig      `toml:"review"`
	Chat             ChatConfig        `toml:"chat"`
	Changelog        ChangelogConfig   `toml:"changelog"`
	RedactPatterns   []string          `toml:"redact_patterns"`
	Claude           ClaudeConfig      `toml:"claude"`
	Conflict         ConflictConfig    `toml:"conflict"`
	Temperature      float64           `toml:"temperature"`
	MaxContextFiles  int               `toml:"max_context_files"`
	MaxContextTokens int               `toml:"max_context_tokens"`
	CommitSplit      CommitSplitConfig `toml:"commit_split"`
	// Existing fields.
	AutoInstallDeps bool `toml:"auto_install_deps"`
	// Feature flags and provider selection.
	Enabled        bool `toml:"enabled"`
	AutoCommitMsg  bool `toml:"auto_commit_message"`
	AutoReviewDiff bool `toml:"auto_review_diff"`
}

// CopilotConfig holds GitHub Copilot model settings.
type CopilotConfig struct {
	Model string `toml:"model"` // optional override
}

// ClaudeConfig holds Anthropic Claude model settings.
type ClaudeConfig struct {
	Model     string `toml:"model"`
	MaxTokens int    `toml:"max_tokens"`
}

// ReviewConfig controls AI-powered code review behaviour.
type ReviewConfig struct {
	SeverityThreshold string   `toml:"severity_threshold"`
	Categories        []string `toml:"categories"`
	AutoReviewOnPush  bool     `toml:"auto_review_on_push"`
}

// ConflictConfig controls AI-assisted merge-conflict resolution.
type ConflictConfig struct {
	DefaultMode               string `toml:"default_mode"` // "auto" | "interactive"
	IncludeSurroundingContext int    `toml:"include_surrounding_context"`
	AutoAcceptHighConfidence  bool   `toml:"auto_accept_high_confidence"`
}

// ChangelogConfig controls AI-generated changelogs.
type ChangelogConfig struct {
	Format     string   `toml:"format"` // "keepachangelog"
	Categories []string `toml:"categories"`
}

// CommitSplitConfig controls automatic commit splitting.
type CommitSplitConfig struct {
	Threshold int `toml:"threshold"`
}

// ChatConfig controls the embedded AI chat panel.
type ChatConfig struct {
	SystemPrompt    string `toml:"system_prompt"` // custom override
	CollapsedHeight int    `toml:"collapsed_height"`
	ExpandedHeight  int    `toml:"expanded_height"`
	Enabled         bool   `toml:"enabled"`
	RenderMarkdown  bool   `toml:"render_markdown"`
}

// AIMCPConfig holds AI-specific MCP socket settings.
type AIMCPConfig struct {
	SocketPath string `toml:"socket_path"`
}

// MCPConfig holds MCP server security settings.
type MCPConfig struct {
	Security MCPSecurityConfig `toml:"security"`
}

// MCPSecurityConfig controls MCP server security policies.
type MCPSecurityConfig struct {
	AuditLogPath        string   `toml:"audit_log_path"`
	AllowedCommands     []string `toml:"allowed_commands"`
	AllowedWritePaths   []string `toml:"allowed_write_paths"`
	RateLimitRead       int      `toml:"rate_limit_read"`
	RateLimitWrite      int      `toml:"rate_limit_write"`
	MaxAgentProcesses   int      `toml:"max_agent_processes"`
	AgentTimeout        int      `toml:"agent_timeout"`
	RequireConfirmation bool     `toml:"require_confirmation"`
	// TODO(#174): socket_auth is declared but not enforced — no code in internal/mcp/
	// reads this field. Users setting socket_auth=true get no authentication on the
	// MCP socket. Wire up enforcement or remove this field in a future release.
	SocketAuth     bool `toml:"socket_auth"`
	FollowSymlinks bool `toml:"follow_symlinks"`
	AuditLog       bool `toml:"audit_log"`
}

// ExtensionsConfig controls the extension/plugin system.
type ExtensionsConfig struct {
	InstallDir      string `toml:"install_dir"`
	RegistryURL     string `toml:"registry_url"`
	LuaTimeoutMs    int    `toml:"lua_timeout_ms"`
	WasmMemoryLimit int    `toml:"wasm_memory_limit"`
	Enabled         bool   `toml:"enabled"`
	AutoUpdate      bool   `toml:"auto_update"`
}

// LoggingConfig controls log output.
type LoggingConfig struct {
	Level      string `toml:"level"`
	File       string `toml:"file"`
	MaxSizeMB  int    `toml:"max_size_mb"`
	MaxBackups int    `toml:"max_backups"`
}

// BookmarksConfig holds user-defined bookmark paths.
type BookmarksConfig struct {
	Paths         []string `toml:"paths"`
	ShowInSidebar bool     `toml:"show_in_sidebar"`
}

// SessionConfig controls session save/restore behaviour.
type SessionConfig struct {
	Enabled bool `toml:"enabled"`
	MaxAge  int  `toml:"max_age"` // days to keep sessions
}

// ThemeConfig holds UI theme selection.
type ThemeConfig struct {
	Name string `toml:"name"`
}

// ShortcutsConfig controls AI-powered git workflow shortcuts.
type ShortcutsConfig struct {
	Overrides          map[string]bool  `toml:"overrides"`
	Custom             []CustomShortcut `toml:"custom"`
	Enabled            bool             `toml:"enabled"`
	AutoExecute        bool             `toml:"auto_execute"`
	InteractivePrompts bool             `toml:"interactive_prompts"`
}

// CustomShortcut defines a user-created shortcut in the config file.
type CustomShortcut struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Steps       []string `toml:"steps"`
}

// ActionsConfig controls double-click action overrides and first-use
// confirmations. Users can override the default double-click action for
// any item type and mark item types as "always confirmed" to skip the
// first-use prompt.
type ActionsConfig struct {
	DoubleClick map[string]string `toml:"double_click"`
	RightClick  map[string]string `toml:"right_click"`
	Confirmed   map[string]bool   `toml:"confirmed"`
}

// Duration wraps time.Duration with TOML string unmarshalling.
type Duration struct {
	time.Duration
}

// UnmarshalText parses a duration string like "2s" or "5m".
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	d.Duration = parsed
	return nil
}

// MarshalText serialises the duration back to a string.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// ConfigDir returns the XDG config directory for grut.
func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, AppName)
}

// DataDir returns the XDG data directory for grut.
func DataDir() string {
	return filepath.Join(xdg.DataHome, AppName)
}

// configFilePath returns the full path to the user's config file.
func configFilePath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// LoadDefaults returns a Config built solely from the embedded defaults
// (defaults.toml), without reading any user config file. This is safe to
// call from multiple goroutines and does not touch the filesystem, making
// it ideal for test helpers that need a valid baseline config.
func LoadDefaults() (*Config, error) {
	cfg := &Config{}
	if err := toml.Unmarshal(defaultsTOML, cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded defaults: %w", err)
	}
	expandPaths(cfg)
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	cfg.FileTree.IconMode = ResolveIconMode(cfg.FileTree.IconMode)
	return cfg, nil
}

// Load reads the embedded defaults, overlays the user config file (if it
// exists), validates the result, and returns the final Config.
func Load() (*Config, error) {
	cfg := &Config{}
	// 1. Parse embedded defaults.
	if err := toml.Unmarshal(defaultsTOML, cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded defaults: %w", err)
	}
	// 2. Overlay user config file (if present).
	cfgPath := configFilePath()
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		// Warn if config file is world-readable (Unix only).
		warnIfWorldReadable(cfgPath)
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing user config %s: %w", cfgPath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading config file %s: %w", cfgPath, err)
	}
	// 3. Expand tildes in all path fields.
	expandPaths(cfg)
	// 4. Validate.
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	// 5. Resolve "auto" icon mode to "nerd" or "ascii".
	cfg.FileTree.IconMode = ResolveIconMode(cfg.FileTree.IconMode)
	return cfg, nil
}

// expandTilde replaces a leading "~" with the user's home directory.
func expandTilde(path string) string {
	if path == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// expandPaths applies tilde expansion to every path-valued field in Config.
func expandPaths(cfg *Config) {
	cfg.Terminal.Shell = expandTilde(cfg.Terminal.Shell)
	cfg.AI.MCP.SocketPath = expandTilde(cfg.AI.MCP.SocketPath)
	cfg.MCP.Security.AuditLogPath = expandTilde(cfg.MCP.Security.AuditLogPath)
	cfg.Extensions.InstallDir = expandTilde(cfg.Extensions.InstallDir)
	cfg.Logging.File = expandTilde(cfg.Logging.File)
	for i, p := range cfg.Bookmarks.Paths {
		cfg.Bookmarks.Paths[i] = expandTilde(p)
	}
	for i, p := range cfg.MCP.Security.AllowedWritePaths {
		cfg.MCP.Security.AllowedWritePaths[i] = expandTilde(p)
	}
}

// warnIfWorldReadable logs a warning when the config file's permissions
// allow "other" users to read it. Only meaningful on Unix systems.
func warnIfWorldReadable(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o004 != 0 {
		slog.Warn(
			"config file is world-readable; consider chmod 600",
			"path", path,
			"mode", info.Mode().Perm().String(),
		)
	}
}
