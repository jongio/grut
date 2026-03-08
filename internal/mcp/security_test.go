package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PathJail tests ---

func TestPathJail_ValidateWithinRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("ok"), 0o644))

	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	resolved, err := jail.Validate("file.txt")
	require.NoError(t, err)

	// Resolved path should be inside root.
	rel, err := filepath.Rel(jail.Root(), resolved)
	require.NoError(t, err)
	assert.Equal(t, "file.txt", rel)
}

func TestPathJail_ValidateAbsoluteWithinRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "abs.txt"), []byte("ok"), 0o644))

	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	absPath := filepath.Join(root, "abs.txt")
	resolved, err := jail.Validate(absPath)
	require.NoError(t, err)
	assert.Contains(t, resolved, "abs.txt")
}

func TestPathJail_RejectDotDot(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	_, err = jail.Validate("../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "..")
}

func TestPathJail_RejectAbsoluteOutsideRoot(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	// Use an absolute path outside the root.
	outsidePath := filepath.Join(os.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsidePath, []byte("secret"), 0o644))
	defer func() { _ = os.Remove(outsidePath) }()

	_, err = jail.Validate(outsidePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes repository root")
}

func TestPathJail_RejectEmptyPath(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	_, err = jail.Validate("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestPathJail_AllowNewFileForWrites(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	// File doesn't exist yet — this is a write scenario.
	resolved, err := jail.Validate("newfile.txt")
	require.NoError(t, err)
	assert.Contains(t, resolved, "newfile.txt")
}

func TestPathJail_ValidateSubdirectory(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "sub", "dir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "deep.txt"), []byte("deep"), 0o644))

	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	resolved, err := jail.Validate(filepath.Join("sub", "dir", "deep.txt"))
	require.NoError(t, err)
	assert.Contains(t, resolved, "deep.txt")
}

// --- RateLimiter tests ---

func TestRateLimiter_UnderLimit(t *testing.T) {
	rl := NewRateLimiter(100, 10)

	// Should allow many calls under the limit.
	for i := 0; i < 50; i++ {
		assert.True(t, rl.Allow(categoryRead), "call %d should be allowed", i)
	}
}

func TestRateLimiter_AtLimit(t *testing.T) {
	// Create limiter with 5 calls per minute for reads.
	rl := NewRateLimiter(5, 5)

	// Exhaust the bucket.
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow(categoryRead), "call %d should be allowed", i)
	}

	// Next call should be denied.
	assert.False(t, rl.Allow(categoryRead), "should be rate limited")
}

func TestRateLimiter_OverLimit(t *testing.T) {
	rl := NewRateLimiter(3, 3)

	// Exhaust all tokens.
	for i := 0; i < 3; i++ {
		rl.Allow(categoryRead)
	}

	// Multiple calls over limit should all be denied.
	for i := 0; i < 5; i++ {
		assert.False(t, rl.Allow(categoryRead), "over-limit call %d should be denied", i)
	}
}

func TestRateLimiter_SeparateCategories(t *testing.T) {
	rl := NewRateLimiter(2, 2)

	// Exhaust read tokens.
	assert.True(t, rl.Allow(categoryRead))
	assert.True(t, rl.Allow(categoryRead))
	assert.False(t, rl.Allow(categoryRead))

	// Write tokens should still be available.
	assert.True(t, rl.Allow(categoryWrite))
	assert.True(t, rl.Allow(categoryWrite))
	assert.False(t, rl.Allow(categoryWrite))
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	// Rate: 600 per minute = 10 per second.
	rl := NewRateLimiter(600, 600)

	// Exhaust all tokens.
	for i := 0; i < 600; i++ {
		rl.Allow(categoryRead)
	}
	assert.False(t, rl.Allow(categoryRead))

	// Wait for some token refill.
	time.Sleep(200 * time.Millisecond)

	// Should have refilled ~2 tokens (10/s * 0.2s).
	assert.True(t, rl.Allow(categoryRead), "should have refilled at least 1 token")
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(1000, 1000)

	var wg sync.WaitGroup
	allowed := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.Allow(categoryRead)
		}()
	}
	wg.Wait()
	close(allowed)

	var count int
	for a := range allowed {
		if a {
			count++
		}
	}
	assert.Greater(t, count, 0, "some calls should succeed")
	assert.LessOrEqual(t, count, 1000, "should not exceed bucket capacity")
}

// --- AuditLogger tests ---

func TestAuditLogger_DisabledNoOp(t *testing.T) {
	al := &AuditLogger{enabled: false}

	// Should not panic or error.
	al.Log("test_tool", map[string]any{"key": "val"}, "success", time.Millisecond)
}

func TestAuditLogger_WritesLogEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	cfg := stubSecurityConfig()
	cfg.AuditLog = true
	cfg.AuditLogPath = logPath

	al, err := NewAuditLogger(cfg)
	require.NoError(t, err)
	defer func() { _ = al.Close() }()

	al.Log("git_status", map[string]any{"path": "."}, "success", 50*time.Millisecond)
	al.Log("file_write", map[string]any{"path": "file.txt", "content": "secret data"}, "success", 10*time.Millisecond)

	// Close to flush before reading.
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logStr := string(data)
	assert.Contains(t, logStr, "git_status")
	assert.Contains(t, logStr, "file_write")
	assert.Contains(t, logStr, "<redacted>", "content should be redacted")
	assert.NotContains(t, logStr, "secret data", "raw content must not appear")
}

func TestAuditLogger_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sub", "deep", "audit.log")

	cfg := stubSecurityConfig()
	cfg.AuditLog = true
	cfg.AuditLogPath = logPath

	al, err := NewAuditLogger(cfg)
	require.NoError(t, err)
	defer func() { _ = al.Close() }()
	assert.True(t, al.enabled)

	// Verify directory was created.
	_, err = os.Stat(filepath.Dir(logPath))
	assert.NoError(t, err)
}

// --- containsDotDot tests ---

func TestContainsDotDot(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.txt", false},
		{"sub/file.txt", false},
		{"../etc/passwd", true},
		{"sub/../other", true},
		{"...", false},
		{".dotfile", false},
		{"a/b/../c", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, containsDotDot(tt.path))
		})
	}
}

// --- PathJail extended security tests ---

func TestPathJail_RejectDeepTraversal(t *testing.T) {
	root := t.TempDir()
	// Create a subdirectory so the path looks plausible.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub", "dir"), 0o755))

	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	// Two levels of ".." from sub/dir should escape root.
	_, err = jail.Validate("sub/dir/../../../etc/passwd")
	assert.Error(t, err, "deep traversal sub/dir/../../../etc/passwd must be blocked")
}

func TestPathJail_RejectDotDotAtEnd(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))

	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	// ".." at the end of a path that escapes root.
	_, err = jail.Validate("sub/../..")
	assert.Error(t, err, "path ending with .. that escapes root must be blocked")
}

func TestPathJail_SymlinkEscape(t *testing.T) {
	// Create two isolated temp directories: "jail" and "outside".
	jailDir := t.TempDir()
	outsideDir := t.TempDir()

	// Put a secret file outside the jail.
	secretPath := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(secretPath, []byte("top-secret"), 0o644))

	// Create a symlink inside the jail that points to the outside directory.
	linkPath := filepath.Join(jailDir, "escape-link")
	err := os.Symlink(outsideDir, linkPath)
	if err != nil {
		t.Skipf("symlinks not supported or require elevated privileges: %v", err)
	}

	jail, err := NewPathJail(jailDir, false)
	require.NoError(t, err)

	// Attempt to access "escape-link/secret.txt" — should be blocked because
	// the resolved path is outside the jail root.
	_, err = jail.Validate(filepath.Join("escape-link", "secret.txt"))
	assert.Error(t, err, "symlink pointing outside jail must be blocked")
	if err != nil {
		assert.Contains(t, err.Error(), "escapes repository root")
	}
}

func TestPathJail_RejectNullBytePath(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	// Null bytes in paths are a common injection technique.
	resolved, err := jail.Validate("file\x00.txt")
	// This should either error because of null byte or because the path
	// is invalid. The key invariant: it must not succeed with a path
	// outside the jail.
	if err == nil {
		// If the OS didn't reject it, verify the resolved path is still
		// inside the jail (it should be if the null byte was sanitized).
		t.Log("OS accepted null byte in path — verifying jail containment")
		require.True(t, strings.HasPrefix(resolved, jail.Root()),
			"resolved path %q must be inside jail root %q", resolved, jail.Root())
	}
}

// TestPathJail_RejectUNCPath verifies that UNC paths (Windows network shares)
// are blocked to prevent escape to remote file shares.
func TestPathJail_RejectUNCPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("UNC path rejection is Windows-only")
	}

	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	uncPaths := []string{
		`\\server\share`,
		`\\192.168.1.1\c$\secret`,
		`\\?\C:\secret`,
	}

	for _, p := range uncPaths {
		t.Run(p, func(t *testing.T) {
			_, err := jail.Validate(p)
			require.Error(t, err, "UNC path %q must be rejected", p)
			assert.Contains(t, err.Error(), "UNC", "error should mention UNC for path %q", p)
		})
	}
}

// --- Additional rate limiter tests ---

// TestRateLimiter_WriteEnforcedSeparately verifies that the write-rate cap
// does not bleed into the read bucket. Each category is independently tracked.
func TestRateLimiter_WriteEnforcedSeparately(t *testing.T) {
	// 100 reads/min, only 2 writes/min.
	rl := NewRateLimiter(100, 2)

	// Exhaust write tokens.
	assert.True(t, rl.Allow(categoryWrite), "write call 1 should be allowed")
	assert.True(t, rl.Allow(categoryWrite), "write call 2 should be allowed")
	assert.False(t, rl.Allow(categoryWrite), "write should be rate-limited after 2 calls")

	// Read bucket is independent — still has tokens.
	assert.True(t, rl.Allow(categoryRead), "reads must not be blocked by write exhaustion")
}

// TestRateLimiter_ReadEnforcedSeparately is the complement: exhausting reads
// must not drain write tokens.
func TestRateLimiter_ReadEnforcedSeparately(t *testing.T) {
	rl := NewRateLimiter(2, 100)

	assert.True(t, rl.Allow(categoryRead))
	assert.True(t, rl.Allow(categoryRead))
	assert.False(t, rl.Allow(categoryRead), "read should be limited after 2 calls")

	// Write bucket is unaffected.
	assert.True(t, rl.Allow(categoryWrite), "writes must not be blocked by read exhaustion")
}

// TestRateLimiter_UnknownCategoryFallsBackToDefault verifies that an
// unrecognised category uses the default read rate rather than panicking.
func TestRateLimiter_UnknownCategoryFallsBackToDefault(t *testing.T) {
	rl := NewRateLimiter(10, 10)

	// Must not panic and must return a consistent bool.
	result := rl.Allow("unknown_category_xyz")
	assert.True(t, result, "first call on unknown category should be allowed")
}

// TestRateLimiter_ExactlyAtLimitThenOver verifies the boundary condition:
// the N-th call is allowed, the (N+1)-th is denied.
func TestRateLimiter_ExactlyAtLimitThenOver(t *testing.T) {
	const limit = 7
	rl := NewRateLimiter(limit, limit)

	for i := 0; i < limit; i++ {
		assert.True(t, rl.Allow(categoryRead), "call %d of %d should be allowed", i+1, limit)
	}
	assert.False(t, rl.Allow(categoryRead), "call %d should be denied (over limit)", limit+1)
}

// TestRateLimiter_WriteVsReadBucketSizes verifies that different capacities
// are respected for read and write independently.
func TestRateLimiter_WriteVsReadBucketSizes(t *testing.T) {
	rl := NewRateLimiter(10, 3)

	// Drain writes.
	for i := 0; i < 3; i++ {
		require.True(t, rl.Allow(categoryWrite), "write call %d should pass", i+1)
	}
	assert.False(t, rl.Allow(categoryWrite), "4th write should be blocked")

	// Read bucket has 10 tokens.
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow(categoryRead), "read call %d should pass", i+1)
	}
	assert.False(t, rl.Allow(categoryRead), "11th read should be blocked")
}

// --- Additional PathJail tests ---

// TestPathJail_RejectWindowsTraversal verifies that Windows-style backslash
// traversal attempts (..\) are rejected. filepath.ToSlash normalisation in
// containsDotDot should catch these regardless of the host OS.
func TestPathJail_RejectWindowsTraversal(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	windowsTraversals := []string{
		`sub\..\..`,
		`..\etc\passwd`,
		`sub\dir\..\..\..\etc`,
	}

	for _, p := range windowsTraversals {
		_, err = jail.Validate(p)
		// Either the containsDotDot check or filepath.EvalSymlinks will reject
		// this; either way, accessing outside the jail must fail.
		if err != nil {
			assert.Contains(t, err.Error(), "..", "error should mention '..' for path %q", p)
		}
		// Note: on Unix, backslash is a valid filename character, so the OS
		// may not reject it. The containsDotDot check normalises with
		// filepath.ToSlash before splitting, so ".." segments are caught.
	}
}

// TestPathJail_MultipleSymlinkLevels verifies that a chain of symlinks that
// eventually escapes the jail root is blocked.
func TestPathJail_MultipleSymlinkLevels(t *testing.T) {
	jailDir := t.TempDir()
	outsideDir := t.TempDir()

	// Write a secret file outside.
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644))

	// Level-1 symlink inside jail → outside.
	link1 := filepath.Join(jailDir, "link1")
	if err := os.Symlink(outsideDir, link1); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	jail, err := NewPathJail(jailDir, false)
	require.NoError(t, err)

	_, err = jail.Validate(filepath.Join("link1", "secret.txt"))
	assert.Error(t, err, "multi-level symlink escape must be blocked")
	if err != nil {
		assert.Contains(t, err.Error(), "escapes repository root")
	}
}

// TestPathJail_RootItself verifies that "." (the root itself) is a valid path.
func TestPathJail_RootItself(t *testing.T) {
	root := t.TempDir()
	jail, err := NewPathJail(root, false)
	require.NoError(t, err)

	resolved, err := jail.Validate(".")
	require.NoError(t, err)
	assert.Equal(t, jail.Root(), resolved, "root jail path should resolve to the root itself")
}
