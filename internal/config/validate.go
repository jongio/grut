package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Hard limits prevent resource exhaustion from misconfigured values.
// These are intentionally generous — they're safety rails, not policy.
const (
	maxMaxDepth         = 100       // file tree traversal depth
	maxPreviewWidth     = 100       // preview width percentage
	maxMaxFileSize      = 100       // MB, for preview
	maxScrollback       = 100_000   // terminal scrollback lines
	maxRenderFPS        = 120       // terminal render FPS
	maxMaxLogEntries    = 100_000   // git log entries
	maxPollInterval     = 3600      // seconds (1 hour)
	maxDiffContext      = 100       // review diff context lines
	maxContextFiles     = 1000      // AI context files
	maxContextTokens    = 1_000_000 // AI context token budget
	maxClaudeTokens     = 1_000_000 // Claude max tokens
	maxChatHeight       = 1000      // rows for chat panel
	maxRateLimitRead    = 10_000    // MCP reads per minute
	maxRateLimitWrite   = 10_000    // MCP writes per minute
	maxAgentProcesses   = 100       // concurrent MCP agent processes
	maxAgentTimeout     = 3600      // seconds
	maxLuaTimeout       = 300_000   // ms (5 minutes)
	maxWasmMemoryLimit  = 1 << 30   // 1 GB
	maxRedactPatterns   = 100       // AI redact pattern count
	maxReviewCategories = 50        // AI review category count
	maxCustomShortcuts  = 100       // custom shortcut definitions
	maxBookmarkPaths    = 1000      // bookmark entries
	maxEditorTabSize    = 16        // editor tab size
	maxLogSizeMB        = 1000      // log file max size
	maxLogBackups       = 100       // log file backup count
)

// Validate checks every field in cfg and returns all problems joined into a
// single error. Returns nil when the config is valid.
func Validate(cfg *Config) error {
	var errs []error

	// --- general ---
	errs = appendEnumOrPathErr(errs, "general.keybinding_scheme",
		cfg.General.KeybindingScheme, "default", "classic", "vim")
	errs = appendEnumErr(errs, "general.default_layout",
		cfg.General.DefaultLayout, "explorer", "git", "review", "agent")

	// --- file_tree ---
	errs = appendEnumErr(errs, "file_tree.icon_mode",
		cfg.FileTree.IconMode, "nerd", "ascii", "auto")
	if cfg.FileTree.MaxDepth < 1 {
		errs = append(errs, fieldErr("file_tree.max_depth", "must be >= 1, got %d", cfg.FileTree.MaxDepth))
	}
	if cfg.FileTree.MaxDepth > maxMaxDepth {
		errs = append(errs, fieldErr("file_tree.max_depth", "must be <= %d, got %d", maxMaxDepth, cfg.FileTree.MaxDepth))
	}

	// --- preview ---
	errs = appendEnumErr(errs, "preview.position",
		cfg.Preview.Position, "right", "bottom", "left", "top")
	if cfg.Preview.Width < 1 || cfg.Preview.Width > maxPreviewWidth {
		errs = append(errs, fieldErr("preview.width", "must be 1-%d, got %d", maxPreviewWidth, cfg.Preview.Width))
	}
	if cfg.Preview.MaxFileSize < 0 {
		errs = append(errs, fieldErr("preview.max_file_size", "must be >= 0, got %d", cfg.Preview.MaxFileSize))
	}
	if cfg.Preview.MaxFileSize > maxMaxFileSize*1024*1024 {
		errs = append(errs, fieldErr("preview.max_file_size", "must be <= %d bytes (%d MB), got %d", maxMaxFileSize*1024*1024, maxMaxFileSize, cfg.Preview.MaxFileSize))
	}

	// --- editor ---
	if cfg.Editor.TabSize < 1 {
		errs = append(errs, fieldErr("editor.tab_size", "must be >= 1, got %d", cfg.Editor.TabSize))
	}
	if cfg.Editor.TabSize > maxEditorTabSize {
		errs = append(errs, fieldErr("editor.tab_size", "must be <= %d, got %d", maxEditorTabSize, cfg.Editor.TabSize))
	}

	// --- git ---
	errs = appendEnumErr(errs, "git.refresh_method",
		cfg.Git.RefreshMethod, "fsnotify", "poll")
	errs = appendEnumErr(errs, "git.worktree_merge_method",
		cfg.Git.WorktreeMergeMethod, "merge", "rebase", "squash")
	errs = appendEnumErr(errs, "git.worktree_open_mode",
		cfg.Git.WorktreeOpenMode, "current", "new_terminal")
	if cfg.Git.RefreshFallbackInterval.Duration <= 0 {
		errs = append(errs, fieldErr("git.refresh_fallback_interval", "must be a positive duration"))
	}
	if cfg.Git.AutoFetchInterval.Duration <= 0 {
		errs = append(errs, fieldErr("git.auto_fetch_interval", "must be a positive duration"))
	}
	if cfg.Git.MaxLogEntries < 1 {
		errs = append(errs, fieldErr("git.max_log_entries", "must be >= 1, got %d", cfg.Git.MaxLogEntries))
	}
	if cfg.Git.MaxLogEntries > maxMaxLogEntries {
		errs = append(errs, fieldErr("git.max_log_entries", "must be <= %d, got %d", maxMaxLogEntries, cfg.Git.MaxLogEntries))
	}

	// --- github ---
	if cfg.GitHub.PollInterval < 0 {
		errs = append(errs, fieldErr("github.poll_interval", "must be >= 0, got %d", cfg.GitHub.PollInterval))
	}
	if cfg.GitHub.PollInterval > maxPollInterval {
		errs = append(errs, fieldErr("github.poll_interval", "must be <= %d, got %d", maxPollInterval, cfg.GitHub.PollInterval))
	}
	errs = appendEnumErr(errs, "github.default_issue_filter",
		cfg.GitHub.DefaultIssueFilter, "all", "assigned", "mentioned", "created")
	errs = appendEnumErr(errs, "github.default_pr_filter",
		cfg.GitHub.DefaultPRFilter, "all", "needs_review", "mine", "draft")
	if cfg.GitHub.ReviewDiffContextLines < 0 {
		errs = append(errs, fieldErr("github.review_diff_context_lines", "must be >= 0, got %d", cfg.GitHub.ReviewDiffContextLines))
	}
	if cfg.GitHub.ReviewDiffContextLines > maxDiffContext {
		errs = append(errs, fieldErr("github.review_diff_context_lines", "must be <= %d, got %d", maxDiffContext, cfg.GitHub.ReviewDiffContextLines))
	}

	// --- terminal ---
	if cfg.Terminal.Scrollback < 1 {
		errs = append(errs, fieldErr("terminal.scrollback", "must be >= 1, got %d", cfg.Terminal.Scrollback))
	}
	if cfg.Terminal.Scrollback > maxScrollback {
		errs = append(errs, fieldErr("terminal.scrollback", "must be <= %d, got %d", maxScrollback, cfg.Terminal.Scrollback))
	}
	if cfg.Terminal.RenderFPS < 1 {
		errs = append(errs, fieldErr("terminal.render_fps", "must be >= 1, got %d", cfg.Terminal.RenderFPS))
	}
	if cfg.Terminal.RenderFPS > maxRenderFPS {
		errs = append(errs, fieldErr("terminal.render_fps", "must be <= %d, got %d", maxRenderFPS, cfg.Terminal.RenderFPS))
	}

	// --- ai ---
	errs = appendEnumErr(errs, "ai.context_mode",
		cfg.AI.ContextMode, "manual", "smart")
	errs = appendEnumErr(errs, "ai.provider",
		cfg.AI.Provider, "copilot", "claude", "none")
	if cfg.AI.FallbackProvider != "" {
		errs = appendEnumErr(errs, "ai.fallback_provider",
			cfg.AI.FallbackProvider, "copilot", "claude", "none")
	}
	if cfg.AI.Temperature < 0 || cfg.AI.Temperature > 1 {
		errs = append(errs, fieldErr("ai.temperature", "must be in [0, 1], got %g", cfg.AI.Temperature))
	}
	if cfg.AI.MaxContextFiles < 1 {
		errs = append(errs, fieldErr("ai.max_context_files", "must be >= 1, got %d", cfg.AI.MaxContextFiles))
	}
	if cfg.AI.MaxContextFiles > maxContextFiles {
		errs = append(errs, fieldErr("ai.max_context_files", "must be <= %d, got %d", maxContextFiles, cfg.AI.MaxContextFiles))
	}
	if cfg.AI.MaxContextTokens < 1 {
		errs = append(errs, fieldErr("ai.max_context_tokens", "must be >= 1, got %d", cfg.AI.MaxContextTokens))
	}
	if cfg.AI.MaxContextTokens > maxContextTokens {
		errs = append(errs, fieldErr("ai.max_context_tokens", "must be <= %d, got %d", maxContextTokens, cfg.AI.MaxContextTokens))
	}

	// Reject embedded API keys in string fields.
	errs = rejectEmbeddedKeys(errs, "ai", cfg.AI)

	// --- ai.claude ---
	if cfg.AI.Claude.MaxTokens < 0 {
		errs = append(errs, fieldErr("ai.claude.max_tokens", "must be >= 0, got %d", cfg.AI.Claude.MaxTokens))
	}
	if cfg.AI.Claude.MaxTokens > maxClaudeTokens {
		errs = append(errs, fieldErr("ai.claude.max_tokens", "must be <= %d, got %d", maxClaudeTokens, cfg.AI.Claude.MaxTokens))
	}

	// --- ai.review ---
	errs = appendEnumErr(errs, "ai.review.severity_threshold",
		cfg.AI.Review.SeverityThreshold, "error", "warning", "info", "hint")

	// --- ai.conflict ---
	errs = appendEnumErr(errs, "ai.conflict.default_mode",
		cfg.AI.Conflict.DefaultMode, "auto", "interactive")
	if cfg.AI.Conflict.IncludeSurroundingContext < 0 {
		errs = append(errs, fieldErr("ai.conflict.include_surrounding_context", "must be >= 0, got %d", cfg.AI.Conflict.IncludeSurroundingContext))
	}

	// --- ai.changelog ---
	errs = appendEnumErr(errs, "ai.changelog.format",
		cfg.AI.Changelog.Format, "keepachangelog")

	// --- ai.commit_split ---
	if cfg.AI.CommitSplit.Threshold < 1 {
		errs = append(errs, fieldErr("ai.commit_split.threshold", "must be >= 1, got %d", cfg.AI.CommitSplit.Threshold))
	}

	// --- ai.chat ---
	if cfg.AI.Chat.CollapsedHeight < 1 {
		errs = append(errs, fieldErr("ai.chat.collapsed_height", "must be >= 1, got %d", cfg.AI.Chat.CollapsedHeight))
	}
	if cfg.AI.Chat.CollapsedHeight > maxChatHeight {
		errs = append(errs, fieldErr("ai.chat.collapsed_height", "must be <= %d, got %d", maxChatHeight, cfg.AI.Chat.CollapsedHeight))
	}
	if cfg.AI.Chat.ExpandedHeight < 1 {
		errs = append(errs, fieldErr("ai.chat.expanded_height", "must be >= 1, got %d", cfg.AI.Chat.ExpandedHeight))
	}
	if cfg.AI.Chat.ExpandedHeight > maxChatHeight {
		errs = append(errs, fieldErr("ai.chat.expanded_height", "must be <= %d, got %d", maxChatHeight, cfg.AI.Chat.ExpandedHeight))
	}

	// --- mcp.security ---
	if cfg.MCP.Security.RateLimitRead < 0 {
		errs = append(errs, fieldErr("mcp.security.rate_limit_read", "must be >= 0, got %d", cfg.MCP.Security.RateLimitRead))
	}
	if cfg.MCP.Security.RateLimitRead > maxRateLimitRead {
		errs = append(errs, fieldErr("mcp.security.rate_limit_read", "must be <= %d, got %d", maxRateLimitRead, cfg.MCP.Security.RateLimitRead))
	}
	if cfg.MCP.Security.RateLimitWrite < 0 {
		errs = append(errs, fieldErr("mcp.security.rate_limit_write", "must be >= 0, got %d", cfg.MCP.Security.RateLimitWrite))
	}
	if cfg.MCP.Security.RateLimitWrite > maxRateLimitWrite {
		errs = append(errs, fieldErr("mcp.security.rate_limit_write", "must be <= %d, got %d", maxRateLimitWrite, cfg.MCP.Security.RateLimitWrite))
	}
	if cfg.MCP.Security.MaxAgentProcesses < 0 {
		errs = append(errs, fieldErr("mcp.security.max_agent_processes", "must be >= 0, got %d", cfg.MCP.Security.MaxAgentProcesses))
	}
	if cfg.MCP.Security.MaxAgentProcesses > maxAgentProcesses {
		errs = append(errs, fieldErr("mcp.security.max_agent_processes", "must be <= %d, got %d", maxAgentProcesses, cfg.MCP.Security.MaxAgentProcesses))
	}
	if cfg.MCP.Security.AgentTimeout < 0 {
		errs = append(errs, fieldErr("mcp.security.agent_timeout", "must be >= 0, got %d", cfg.MCP.Security.AgentTimeout))
	}
	if cfg.MCP.Security.AgentTimeout > maxAgentTimeout {
		errs = append(errs, fieldErr("mcp.security.agent_timeout", "must be <= %d, got %d", maxAgentTimeout, cfg.MCP.Security.AgentTimeout))
	}

	// --- extensions ---
	if cfg.Extensions.LuaTimeoutMs < 0 {
		errs = append(errs, fieldErr("extensions.lua_timeout_ms", "must be >= 0, got %d", cfg.Extensions.LuaTimeoutMs))
	}
	if cfg.Extensions.LuaTimeoutMs > maxLuaTimeout {
		errs = append(errs, fieldErr("extensions.lua_timeout_ms", "must be <= %d, got %d", maxLuaTimeout, cfg.Extensions.LuaTimeoutMs))
	}
	if cfg.Extensions.WasmMemoryLimit < 0 {
		errs = append(errs, fieldErr("extensions.wasm_memory_limit", "must be >= 0, got %d", cfg.Extensions.WasmMemoryLimit))
	}
	if cfg.Extensions.WasmMemoryLimit > maxWasmMemoryLimit {
		errs = append(errs, fieldErr("extensions.wasm_memory_limit", "must be <= %d, got %d", maxWasmMemoryLimit, cfg.Extensions.WasmMemoryLimit))
	}

	// --- logging ---
	errs = appendEnumErr(errs, "logging.level",
		cfg.Logging.Level, "debug", "info", "warn", "error")
	if cfg.Logging.MaxSizeMB < 0 {
		errs = append(errs, fieldErr("logging.max_size_mb", "must be >= 0, got %d", cfg.Logging.MaxSizeMB))
	}
	if cfg.Logging.MaxSizeMB > maxLogSizeMB {
		errs = append(errs, fieldErr("logging.max_size_mb", "must be <= %d, got %d", maxLogSizeMB, cfg.Logging.MaxSizeMB))
	}
	if cfg.Logging.MaxBackups < 0 {
		errs = append(errs, fieldErr("logging.max_backups", "must be >= 0, got %d", cfg.Logging.MaxBackups))
	}
	if cfg.Logging.MaxBackups > maxLogBackups {
		errs = append(errs, fieldErr("logging.max_backups", "must be <= %d, got %d", maxLogBackups, cfg.Logging.MaxBackups))
	}

	// --- array size limits ---
	if len(cfg.AI.RedactPatterns) > maxRedactPatterns {
		errs = append(errs, fieldErr("ai.redact_patterns", "too many entries: %d (max %d)", len(cfg.AI.RedactPatterns), maxRedactPatterns))
	}
	if len(cfg.AI.Review.Categories) > maxReviewCategories {
		errs = append(errs, fieldErr("ai.review.categories", "too many entries: %d (max %d)", len(cfg.AI.Review.Categories), maxReviewCategories))
	}
	if len(cfg.Bookmarks.Paths) > maxBookmarkPaths {
		errs = append(errs, fieldErr("bookmarks.paths", "too many entries: %d (max %d)", len(cfg.Bookmarks.Paths), maxBookmarkPaths))
	}
	if len(cfg.Shortcuts.Custom) > maxCustomShortcuts {
		errs = append(errs, fieldErr("shortcuts.custom", "too many entries: %d (max %d)", len(cfg.Shortcuts.Custom), maxCustomShortcuts))
	}

	// --- path security ---
	if cfg.Logging.File != "" {
		errs = rejectUNCPath(errs, "logging.file", cfg.Logging.File)
	}
	if cfg.MCP.Security.AuditLogPath != "" {
		errs = rejectUNCPath(errs, "mcp.security.audit_log_path", cfg.MCP.Security.AuditLogPath)
	}
	if cfg.Extensions.InstallDir != "" {
		errs = rejectUNCPath(errs, "extensions.install_dir", cfg.Extensions.InstallDir)
	}
	// Defence-in-depth: reject UNC paths and directory traversal in
	// AllowedWritePaths so a malicious config cannot grant the MCP
	// server write access to arbitrary network shares or parent
	// directories (CWE-22, CWE-40).
	for i, p := range cfg.MCP.Security.AllowedWritePaths {
		field := fmt.Sprintf("mcp.security.allowed_write_paths[%d]", i)
		errs = rejectUNCPath(errs, field, p)
		errs = rejectTraversalPath(errs, field, p)
	}

	return errors.Join(errs...)
}

// rejectUNCPath appends an error if value looks like a UNC path
// (\\server\share or //server/share), which could trigger outbound SMB
// authentication on Windows.
func rejectUNCPath(errs []error, field, value string) []error {
	cleaned := filepath.Clean(value)
	if strings.HasPrefix(cleaned, `\\`) {
		return append(errs, fieldErr(field, "UNC paths (\\\\...) are not allowed for security reasons"))
	}
	// filepath.Clean normalises "//" to "/" on non-Windows platforms,
	// so check the raw value to catch forward-slash UNC paths that could
	// be interpreted as UNC when the config is used on Windows.
	if strings.HasPrefix(value, "//") {
		return append(errs, fieldErr(field, "UNC paths (//...) are not allowed for security reasons"))
	}
	return errs
}

// rejectTraversalPath appends an error if value contains ".." path
// components, which could be used for directory traversal (CWE-22).
// Both forward slashes and backslashes are checked so that
// Windows-style traversal is detected on all platforms.
func rejectTraversalPath(errs []error, field, value string) []error {
	// Normalise backslashes so Windows-style "..\etc" is caught on Unix.
	normalized := strings.ReplaceAll(value, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return append(errs, fieldErr(field, "path must not contain '..' (directory traversal)"))
		}
	}
	return errs
}

// fieldErr creates a formatted error for a config field.
func fieldErr(field string, format string, args ...any) error {
	return fmt.Errorf("config %s: %s", field, fmt.Sprintf(format, args...))
}

// appendEnumErr appends a validation error if value is not in allowed.
func appendEnumErr(errs []error, field, value string, allowed ...string) []error {
	for _, a := range allowed {
		if value == a {
			return errs
		}
	}
	return append(errs, fieldErr(field, "must be one of %v, got %q", allowed, value))
}

// appendEnumOrPathErr is like appendEnumErr but also accepts custom file paths
// (containing "/" or "\") so users can provide their own configuration files.
func appendEnumOrPathErr(errs []error, field, value string, allowed ...string) []error {
	for _, a := range allowed {
		if value == a {
			return errs
		}
	}
	if strings.ContainsAny(value, `/\`) {
		// Accept file paths but reject UNC paths.
		return rejectUNCPath(errs, field, value)
	}
	return append(errs, fieldErr(field, "must be one of %v or a file path, got %q", allowed, value))
}

// apiKeyPatterns matches common API key prefixes that must never appear in
// config string fields. Each pattern is anchored to the start of the value.
var apiKeyPatterns = regexp.MustCompile(
	`^(sk-[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16}|ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|` +
		`gat_[a-zA-Z0-9]{36}|glpat-[a-zA-Z0-9\-]{20,}|xox[bps]-[a-zA-Z0-9\-]+)`,
)

// rejectEmbeddedKeys scans every exported string field in the AIConfig struct
// for values that look like API keys and appends an error for each match.
func rejectEmbeddedKeys(errs []error, prefix string, ai AIConfig) []error { //nolint:unparam // prefix parameterized for reusability
	// Flat string fields to check.
	fields := []struct {
		name  string
		value string
	}{
		{"provider", ai.Provider},
		{"fallback_provider", ai.FallbackProvider},
		{"context_mode", ai.ContextMode},
		{"token_model", ai.TokenModel},
		{"copilot.model", ai.Copilot.Model},
		{"claude.model", ai.Claude.Model},
		{"review.severity_threshold", ai.Review.SeverityThreshold},
		{"conflict.default_mode", ai.Conflict.DefaultMode},
		{"changelog.format", ai.Changelog.Format},
		{"chat.system_prompt", ai.Chat.SystemPrompt},
		{"mcp.socket_path", ai.MCP.SocketPath},
	}
	for _, f := range fields {
		if apiKeyPatterns.MatchString(f.value) {
			errs = append(errs, fieldErr(prefix+"."+f.name, "looks like an embedded API key — use an environment variable instead"))
		}
	}
	// Also scan slice string fields.
	for i, p := range ai.RedactPatterns {
		if apiKeyPatterns.MatchString(p) {
			errs = append(errs, fieldErr(fmt.Sprintf("%s.redact_patterns[%d]", prefix, i), "looks like an embedded API key — use an environment variable instead"))
		}
	}
	for i, c := range ai.Review.Categories {
		if apiKeyPatterns.MatchString(c) {
			errs = append(errs, fieldErr(fmt.Sprintf("%s.review.categories[%d]", prefix, i), "looks like an embedded API key — use an environment variable instead"))
		}
	}
	for i, c := range ai.Changelog.Categories {
		if apiKeyPatterns.MatchString(c) {
			errs = append(errs, fieldErr(fmt.Sprintf("%s.changelog.categories[%d]", prefix, i), "looks like an embedded API key — use an environment variable instead"))
		}
	}
	return errs
}
