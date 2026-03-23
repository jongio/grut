package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AuditLogger — Close nil closer (66.7% → 100%)
// ---------------------------------------------------------------------------

func TestAuditLogger_Close_NilCloser(t *testing.T) {
	// A disabled AuditLogger has no closer.
	al := &AuditLogger{enabled: false, closer: nil}
	err := al.Close()
	assert.NoError(t, err)
}

func TestAuditLogger_Close_WithFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	cfg := config.MCPSecurityConfig{
		AuditLog:     true,
		AuditLogPath: logPath,
	}

	al, err := NewAuditLogger(cfg)
	require.NoError(t, err)

	// Write a log entry.
	al.Log("test_tool", map[string]any{"key": "val"}, "ok", time.Millisecond)

	// Close the file.
	err = al.Close()
	assert.NoError(t, err)

	// Verify content was written.
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_tool")
}

// ---------------------------------------------------------------------------
// isSensitiveAuditField — direct tests (previously only implicit)
// ---------------------------------------------------------------------------

func TestIsSensitiveAuditField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want bool
	}{
		{"content", true},
		{"Content", true},
		{"CONTENT", true},
		{"file_content", true},
		{"message", true},
		{"body", true},
		{"token", true},
		{"secret", true},
		{"credential", true},
		{"api_token", true},
		{"my_secret_key", true},
		{"user_credential_hash", true},
		{"requestBody", true},
		{"authToken", true},     // camelCase: "auth" + "Token"
		{"privateKey", true},    // camelCase: "private" + "Key"
		{"secretValue", true},   // camelCase: "secret" + "Value"
		{"api-token", true},     // dash-separated
		{"my.secret.key", true}, // dot-separated
		{"path", false},
		{"file", false},
		{"status", false},
		{"repo", false},
		{"", false},
		{"is_private", true},       // "is" + "private" — "private" IS sensitive
		{"primary_key", true},      // "primary" + "key" — "key" IS sensitive
		{"authorized_user", false}, // "authorized" != "auth"
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			got := isSensitiveAuditField(tt.key)
			assert.Equal(t, tt.want, got, "isSensitiveAuditField(%q)", tt.key)
		})
	}
}

// ---------------------------------------------------------------------------
// validateGitPaths — 0% coverage
// ---------------------------------------------------------------------------

func TestValidateGitPaths_Empty(t *testing.T) {
	t.Parallel()

	err := validateGitPaths(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one path is required")

	err = validateGitPaths([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one path is required")
}

func TestValidateGitPaths_ValidPaths(t *testing.T) {
	t.Parallel()

	err := validateGitPaths([]string{"file.go", "dir/file.go"})
	assert.NoError(t, err)
}

func TestValidateGitPaths_InvalidPath(t *testing.T) {
	t.Parallel()

	err := validateGitPaths([]string{"good.go", "../../bad.go"})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// jsonResult — 75% coverage (marshal error path)
// ---------------------------------------------------------------------------

func TestJsonResult_Success(t *testing.T) {
	t.Parallel()

	result, err := jsonResult(map[string]string{"key": "value"})
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestJsonResult_MarshalError(t *testing.T) {
	t.Parallel()

	// Create an unmarshalable value (channel can't be marshaled).
	ch := make(chan int)
	result, err := jsonResult(ch)
	// jsonResult returns an error *result*, not a Go error.
	require.NoError(t, err)
	require.NotNil(t, result)
	// The result should be an error result.
	assert.True(t, result.IsError)
}

// ---------------------------------------------------------------------------
// NewAuditLogger — error paths (75% → higher)
// ---------------------------------------------------------------------------

func TestNewAuditLogger_MkdirAllError(t *testing.T) {
	// Point audit log to a path where MkdirAll will fail.
	// On Windows, use an invalid path character.
	cfg := config.MCPSecurityConfig{
		AuditLog:     true,
		AuditLogPath: filepath.Join(string([]byte{0}), "audit.log"),
	}

	_, err := NewAuditLogger(cfg)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewAuditLogger — path traversal rejection
// ---------------------------------------------------------------------------

func TestNewAuditLogger_RejectsTraversalPath(t *testing.T) {
	t.Parallel()

	cfg := config.MCPSecurityConfig{
		AuditLog:     true,
		AuditLogPath: filepath.Join("..", "..", "tmp", "malicious-audit.log"),
	}

	_, err := NewAuditLogger(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'..' traversal")
}

func TestNewAuditLogger_AcceptsCleanPath(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "subdir", "audit.log")

	cfg := config.MCPSecurityConfig{
		AuditLog:     true,
		AuditLogPath: logPath,
	}

	al, err := NewAuditLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })
	assert.True(t, al.enabled)
}

// ---------------------------------------------------------------------------
// resolveNewPath — direct test for non-existent path handling
// ---------------------------------------------------------------------------

func TestResolveNewPath_ExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "exists.txt")
	require.NoError(t, os.WriteFile(file, []byte("ok"), 0o644))

	resolved, err := resolveNewPath(file)
	require.NoError(t, err)
	assert.Equal(t, file, resolved)
}

func TestResolveNewPath_NonExistentFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "new.txt")

	resolved, err := resolveNewPath(file)
	require.NoError(t, err)
	// The resolved path should be based on the real parent directory.
	assert.Contains(t, resolved, "new.txt")
}

func TestResolveNewPath_DeepNonExistent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "file.txt")

	resolved, err := resolveNewPath(deep)
	require.NoError(t, err)
	assert.Contains(t, resolved, "file.txt")
}

// ---------------------------------------------------------------------------
// AuditLogger.Log — redaction of sensitive params
// ---------------------------------------------------------------------------

func TestAuditLogger_LogRedactsSensitiveFields(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "redact-audit.log")

	cfg := config.MCPSecurityConfig{
		AuditLog:     true,
		AuditLogPath: logPath,
	}

	al, err := NewAuditLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })

	al.Log("write_file", map[string]any{
		"path":    "file.go",
		"content": "secret source code",
		"token":   "abc123",
	}, "ok", time.Millisecond)

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	// Parse the JSON log entry.
	var entry map[string]any
	err = json.Unmarshal(data, &entry)
	require.NoError(t, err)

	// Verify path is NOT redacted.
	params, ok := entry["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "file.go", params["path"])

	// Verify content and token ARE redacted.
	assert.Equal(t, "<redacted>", params["content"])
	assert.Equal(t, "<redacted>", params["token"])
}
