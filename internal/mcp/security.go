// Package mcp implements an MCP (Model Context Protocol) server that exposes
// grut's git and file operations as tools for AI agent integration.
package mcp

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jongio/grut/internal/config"
)

// PathJail restricts file operations to within the git repository root.
// It resolves symlinks and rejects traversal attempts before any I/O occurs.
type PathJail struct {
	root           string
	followSymlinks bool
}

// NewPathJail creates a PathJail anchored at root. The root path is resolved
// to an absolute path and symlinks are evaluated to establish the canonical
// boundary.
func NewPathJail(root string, followSymlinks bool) (*PathJail, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("eval symlinks for root: %w", err)
	}
	return &PathJail{root: resolved, followSymlinks: followSymlinks}, nil
}

// Root returns the canonical root path of the jail.
func (j *PathJail) Root() string {
	return j.root
}

// Validate checks that path is within the jail boundary. It returns the
// resolved absolute path on success or an error if the path would escape
// the repository root.
func (j *PathJail) Validate(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	// Defence-in-depth: reject embedded null bytes early. Go 1.21+ rejects
	// them at the syscall layer, but catching here gives a clearer error and
	// prevents any platform-specific bypass where a null byte truncates the
	// path string (CWE-158).
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("path contains null byte")
	}
	// Windows-specific: reject UNC paths and reserved device names.
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(path, `\\`) {
			return "", fmt.Errorf("UNC paths are not allowed")
		}
		if isWindowsReservedName(filepath.Base(path)) {
			slog.Debug("mcp: windows reserved device name rejected", "basename", filepath.Base(path))
			return "", fmt.Errorf("windows reserved device name not allowed")
		}
	}
	// Reject paths containing ".." components before any resolution.
	if containsDotDot(path) {
		slog.Debug("mcp: path contains '..' rejected", "path", path)
		return "", fmt.Errorf("path contains '..'")
	}
	// Build absolute path.
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(j.root, path))
	}
	// Resolve symlinks to get canonical path.
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// For writes, the file (and intermediate dirs) may not exist yet.
			// Walk up the directory tree to find the first existing ancestor.
			resolved, err = resolveNewPath(absPath)
			if err != nil {
				return "", fmt.Errorf("resolve new path: %w", err)
			}
		} else {
			return "", fmt.Errorf("resolve path: %w", err)
		}
	}
	// When followSymlinks is false, reject any path that resolved differently
	// from its cleaned absolute form (meaning it traversed a symlink).
	if !j.followSymlinks && resolved != absPath {
		// Check if the difference is actually due to a symlink (not just
		// case normalization on case-insensitive filesystems).
		info, lErr := os.Lstat(absPath)
		if lErr == nil && info.Mode()&os.ModeSymlink != 0 {
			slog.Debug("mcp: symlink rejected", "path", path)
			return "", fmt.Errorf("symlinks not allowed")
		}
	}
	// Verify the resolved path is within the jail root.
	// Use filepath.Rel to compute relative path from root to resolved.
	rel, err := filepath.Rel(j.root, resolved)
	if err != nil {
		if filepath.IsAbs(path) {
			slog.Debug("mcp: absolute path escapes root", "path", path)
			return "", fmt.Errorf("path escapes repository root")
		}
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		slog.Debug("mcp: relative path escapes root", "path", path)
		return "", fmt.Errorf("path escapes repository root")
	}
	return resolved, nil
}

// containsDotDot reports whether path contains a ".." component.
// Backslashes are unconditionally replaced with forward slashes so that
// Windows-style traversal (e.g. "..\etc\passwd") is detected on every OS.
// filepath.ToSlash only replaces os.PathSeparator, which is already '/' on
// Unix — so it would leave backslashes untouched.
func containsDotDot(path string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, "\\", "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// resolveNewPath resolves a path that may not exist yet by walking up the
// directory tree to find the first existing ancestor, resolving symlinks on
// that ancestor, then re-appending the remaining segments.
func resolveNewPath(absPath string) (string, error) {
	// Collect path segments that don't exist yet.
	current := absPath
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			// Found an existing ancestor. Rebuild the path.
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		tail = append(tail, filepath.Base(current))
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding an existing path.
			return "", fmt.Errorf("no existing ancestor for path: %s", absPath)
		}
		current = parent
	}
}

// RateLimiter implements per-category token bucket rate limiting.
// Categories are "read" and "write" with independently configurable
// rates expressed as calls per minute.
type RateLimiter struct {
	buckets map[string]*tokenBucket
	rates   map[string]int // calls per minute by category
	mu      sync.Mutex
}
type tokenBucket struct {
	lastRefill time.Time
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
}

// NewRateLimiter creates a rate limiter with separate read and write rates
// (calls per minute).
func NewRateLimiter(readPerMin, writePerMin int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		rates: map[string]int{
			categoryRead:  readPerMin,
			categoryWrite: writePerMin,
		},
	}
}

// Allow checks whether a call in the given category is permitted.
// Returns true if the call is allowed (and consumes a token), false
// if the rate limit has been exceeded.
func (rl *RateLimiter) Allow(category string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[category]
	if !ok {
		rate, exists := rl.rates[category]
		if !exists {
			rate = defaultReadRatePerMin
		}
		maxTokens := float64(rate)
		b = &tokenBucket{
			tokens:     maxTokens,
			maxTokens:  maxTokens,
			refillRate: float64(rate) / 60.0,
			lastRefill: time.Now(),
		}
		rl.buckets[category] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = min(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// AuditLogger records MCP tool invocations to a structured log file.
type AuditLogger struct {
	logger  *slog.Logger
	closer  io.Closer
	enabled bool
}

var sensitiveAuditFields = []string{
	"content",
	"message",
	"body",
	"token",
	"secret",
	"credential",
	"password",
	"key",
	"auth",
	"private",
}

// NewAuditLogger creates an audit logger based on the MCP security config.
// If audit logging is disabled, the returned logger is a no-op.
func NewAuditLogger(cfg config.MCPSecurityConfig) (*AuditLogger, error) {
	if !cfg.AuditLog {
		return &AuditLogger{enabled: false}, nil
	}
	logPath := cfg.AuditLogPath
	if logPath == "" {
		logPath = filepath.Join(config.DataDir(), "mcp-audit.log")
	}
	// Defence-in-depth: clean the path and reject directory traversal
	// so a malicious config value cannot write logs to arbitrary locations.
	logPath = filepath.Clean(logPath)
	if containsDotDot(logPath) {
		return nil, fmt.Errorf("audit log path contains '..' traversal")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &AuditLogger{
		logger:  slog.New(handler),
		closer:  f,
		enabled: true,
	}, nil
}

// Close releases the underlying log file, if any.
func (al *AuditLogger) Close() error {
	if al.closer != nil {
		return al.closer.Close()
	}
	return nil
}

// Log records a tool invocation. Parameters are sanitized to avoid logging
// sensitive content (e.g. file contents are redacted).
func (al *AuditLogger) Log(tool string, params map[string]any, status string, duration time.Duration) {
	if !al.enabled {
		return
	}
	sanitized := make(map[string]any, len(params))
	for k, v := range params {
		if isSensitiveAuditField(k) {
			sanitized[k] = "<redacted>"
		} else {
			sanitized[k] = v
		}
	}
	al.logger.Info(
		"mcp_tool_call",
		"tool", tool,
		"params", sanitized,
		"status", status,
		"duration_ms", duration.Milliseconds(),
	)
}

// isSensitiveAuditField reports whether key contains a sensitive word.
// It splits the key on common separators ('_', '-', '.') and camelCase
// boundaries, then checks whether any resulting segment exactly matches a
// known sensitive field name.  This avoids false positives from substring
// matching (e.g. "is_private" or "primary_key") while still catching
// camelCase keys like "requestBody".
func isSensitiveAuditField(key string) bool {
	k := strings.ToLower(key)
	// Split on common separators used in JSON/config keys.
	segments := strings.FieldsFunc(k, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	// Also split each segment on camelCase boundaries.  We do this on the
	// original key (before lowering) so we can detect transitions from
	// lowercase to uppercase.
	sepParts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	camelSegments := make([]string, 0, len(sepParts)*2)
	for _, part := range sepParts {
		camelSegments = append(camelSegments, splitCamelCase(part)...)
	}
	// Merge both segment sets for checking.
	all := append(segments, camelSegments...) //nolint:gocritic // append to different slice is intentional
	for _, seg := range all {
		lower := strings.ToLower(seg)
		for _, field := range sensitiveAuditFields {
			if lower == field {
				return true
			}
		}
	}
	return false
}

// splitCamelCase splits a camelCase or PascalCase string into words.
// e.g. "requestBody" → ["request", "Body"], "APIToken" → ["API", "Token"].
func splitCamelCase(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		// Split before an uppercase letter that follows a lowercase letter,
		// or before an uppercase letter followed by a lowercase (for runs
		// like "API" → keep together, then split at "Token").
		prev := runes[i-1]
		cur := runes[i]
		if isLowerRune(prev) && isUpperRune(cur) {
			parts = append(parts, string(runes[start:i]))
			start = i
		} else if i+1 < len(runes) && isUpperRune(prev) && isUpperRune(cur) && isLowerRune(runes[i+1]) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

func isLowerRune(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpperRune(r rune) bool { return r >= 'A' && r <= 'Z' }

// windowsReservedNames lists the device names that Windows reserves and
// that cannot safely be used as regular file names.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// isWindowsReservedName reports whether name is a Windows reserved device
// name. The check is case-insensitive and strips any extension (e.g.
// "CON.txt" → "CON").
func isWindowsReservedName(name string) bool {
	// Strip extension: "CON.txt" → "CON".
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return windowsReservedNames[strings.ToUpper(base)]
}
