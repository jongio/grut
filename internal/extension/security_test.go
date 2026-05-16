package extension

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────────────────────────────────────────────────
// Extension install — path traversal via manifest name (CR-010)
// ──────────────────────────────────────────────────────────────────────────

func TestInstall_RejectsTraversalManifestName(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	traversalNames := []struct {
		name    string
		errFrag string
	}{
		{"../../../tmp/evil", "invalid"},
		{"../sibling", "invalid"},
		{"./current", "invalid"},
		{"ext/../../../etc", "invalid"},
	}
	for _, tc := range traversalNames {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srcDir := filepath.Join(t.TempDir(), "src")
			require.NoError(t, os.MkdirAll(srcDir, 0o755))
			content := `name = "` + tc.name + `"
version = "1.0.0"
runtime = "lua"
permissions = ["file_read"]
`
			require.NoError(t, os.WriteFile(
				filepath.Join(srcDir, "extension.toml"),
				[]byte(content), 0o644,
			))

			err := mgr.Install(context.Background(), srcDir)
			require.Error(t, err, "manifest name %q must be rejected", tc.name)
			assert.Contains(t, err.Error(), tc.errFrag)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Extension install — URL protocol edge cases
// ──────────────────────────────────────────────────────────────────────────

func TestInstall_RejectsFileProtocol(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	fileURLs := []string{
		"file:///etc/passwd",
		"file://localhost/etc/passwd",
		"ftp://evil.com/repo.git",
	}
	for _, url := range fileURLs {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			err := mgr.Install(context.Background(), url)
			require.Error(t, err, "URL %q must be rejected", url)
			assert.Contains(t, err.Error(), "only https://")
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Extension name validation — shell metachar / injection vectors
// ──────────────────────────────────────────────────────────────────────────

func TestIsValidExtensionName_RejectsInjection(t *testing.T) {
	t.Parallel()
	injectionNames := []string{
		"ext;rm -rf /",
		"ext|cat /etc/passwd",
		"ext&evil",
		"ext$(whoami)",
		"ext`id`",
		"ext\nnewline",
		"ext\x00null",
	}
	for _, name := range injectionNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, isValidExtensionName(name),
				"extension name %q must be rejected", name)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Registry allowlist — validateSourceURL
// ──────────────────────────────────────────────────────────────────────────

func TestValidateSourceURL_AllowedHosts(t *testing.T) {
	t.Parallel()
	allowed := []string{"github.com", "gitlab.com"}

	tests := []struct {
		name    string
		url     string
		wantErr bool
		errFrag string
	}{
		{"github ok", "https://github.com/user/repo", false, ""},
		{"github with .git", "https://github.com/user/repo.git", false, ""},
		{"gitlab ok", "https://gitlab.com/org/project", false, ""},
		{"case insensitive", "https://GitHub.COM/user/repo", false, ""},
		{"blocked host", "https://evil.com/user/repo", true, "not in the trusted registry"},
		{"blocked host with path", "https://malicious.io/org/ext", true, "not in the trusted registry"},
		{"bitbucket blocked", "https://bitbucket.org/user/repo", true, "not in the trusted registry"},
		{"localhost blocked", "https://localhost/user/repo", true, "not in the trusted registry"},
		{"ip address blocked", "https://192.168.1.1/user/repo", true, "not in the trusted registry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSourceURL(tt.url, allowed)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errFrag)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateSourceURL_MalformedURLs(t *testing.T) {
	t.Parallel()
	allowed := []string{"github.com"}

	tests := []struct {
		name    string
		url     string
		errFrag string
	}{
		{"path traversal", "https://github.com/user/../etc/passwd", "path traversal"},
		{"embedded credentials", "https://token:secret@github.com/user/repo", "credentials"},
		{"no path segments", "https://github.com/", "owner/repo"},
		{"single path segment", "https://github.com/user", "owner/repo"},
		{"http scheme", "http://github.com/user/repo", "only https://"},
		{"empty host", "https:///user/repo", "no hostname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSourceURL(tt.url, allowed)
			require.Error(t, err, "URL %q must be rejected", tt.url)
			assert.Contains(t, err.Error(), tt.errFrag)
		})
	}
}

func TestValidateSourceURL_EmptyAllowlist(t *testing.T) {
	t.Parallel()
	err := validateSourceURL("https://github.com/user/repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the trusted registry")
}

// ──────────────────────────────────────────────────────────────────────────
// Install — registry allowlist integration
// ──────────────────────────────────────────────────────────────────────────

func TestInstall_RejectsNonAllowlistedHost(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir) // default: github.com only

	err := mgr.Install(context.Background(), "https://evil.example.com/user/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the trusted registry")
}

func TestInstall_CustomAllowlistRejectsDefault(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	// Only allow gitlab.com — github.com should be rejected.
	mgr := NewManagerWithHosts(extDir, []string{"gitlab.com"})

	err := mgr.Install(context.Background(), "https://github.com/user/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the trusted registry")
}

// ──────────────────────────────────────────────────────────────────────────
// Commit hash recording & integrity
// ──────────────────────────────────────────────────────────────────────────

func TestGitHeadHash_ValidRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a minimal git repo with one commit.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	hash, err := gitHeadHash(context.Background(), dir)
	require.NoError(t, err)
	assert.Len(t, hash, 40, "SHA-1 hash should be 40 hex chars")
	for _, c := range hash {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hash char %c must be hex", c)
	}
}

func TestGitHeadHash_NotARepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := gitHeadHash(context.Background(), dir)
	require.Error(t, err)
}

func TestInstall_LocalPath_NoCommitHash(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "local-ext")
	writeManifest(t, srcDir, "local-ext", "1.0.0", "lua")

	require.NoError(t, mgr.Install(context.Background(), srcDir))

	info, err := mgr.Get("local-ext")
	require.NoError(t, err)
	assert.Empty(t, info.CommitHash, "local installs should not have a commit hash")
	assert.Empty(t, info.SourceURL, "local installs should not have a source URL")
}

func TestInstall_ReinstallDetectsDuplicate(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "dup-test")
	writeManifest(t, srcDir, "dup-test", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	// Try installing again from a different local path with same name.
	srcDir2 := filepath.Join(t.TempDir(), "dup-test-2")
	writeManifest(t, srcDir2, "dup-test", "1.0.0", "lua")
	err := mgr.Install(context.Background(), srcDir2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

// ──────────────────────────────────────────────────────────────────────────
// VerifyIntegrity
// ──────────────────────────────────────────────────────────────────────────

func TestVerifyIntegrity_MatchingHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a git repo to act as the installed extension directory.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "test")
	writeManifest(t, dir, "verify-ext", "1.0.0", "lua")
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	hash, err := gitHeadHash(context.Background(), dir)
	require.NoError(t, err)

	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Manually register the extension with the correct hash.
	mgr.mu.Lock()
	mgr.installed["verify-ext"] = &ExtensionInfo{
		Dir:        dir,
		CommitHash: hash,
		SourceURL:  "https://github.com/test/verify-ext",
		Enabled:    true,
	}
	mgr.mu.Unlock()

	err = mgr.VerifyIntegrity(context.Background(), "verify-ext")
	require.NoError(t, err)
}

func TestVerifyIntegrity_MismatchedHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "test")
	writeManifest(t, dir, "tampered-ext", "1.0.0", "lua")
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Register with a WRONG hash — simulates tampered/force-pushed upstream.
	mgr.mu.Lock()
	mgr.installed["tampered-ext"] = &ExtensionInfo{
		Dir:        dir,
		CommitHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SourceURL:  "https://github.com/test/tampered-ext",
		Enabled:    true,
	}
	mgr.mu.Unlock()

	err := mgr.VerifyIntegrity(context.Background(), "tampered-ext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity check failed")
	assert.Contains(t, err.Error(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
}

func TestVerifyIntegrity_LocalInstall_AlwaysPasses(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "local-verify")
	writeManifest(t, srcDir, "local-verify", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	// Local installs have no commit hash, so integrity check always passes.
	err := mgr.VerifyIntegrity(context.Background(), "local-verify")
	require.NoError(t, err)
}

func TestVerifyIntegrity_NotFound(t *testing.T) {
	t.Parallel()
	mgr := NewManager(filepath.Join(t.TempDir(), "extensions"))
	err := mgr.VerifyIntegrity(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ──────────────────────────────────────────────────────────────────────────
// State persistence of SourceURL and CommitHash
// ──────────────────────────────────────────────────────────────────────────

func TestStatePersistence_SourceURLAndCommitHash(t *testing.T) {
	t.Parallel()
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "persist-ext")
	writeManifest(t, srcDir, "persist-ext", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	// Manually set URL and hash as if it was a remote install, then save.
	mgr.mu.Lock()
	mgr.installed["persist-ext"].SourceURL = "https://github.com/test/persist-ext"
	mgr.installed["persist-ext"].CommitHash = "cccccccccccccccccccccccccccccccccccccccc"
	require.NoError(t, mgr.saveStateLocked())
	mgr.mu.Unlock()

	// Load from a fresh manager — URL and hash should survive.
	mgr2 := NewManager(extDir)
	require.NoError(t, mgr2.LoadAll())

	info, err := mgr2.Get("persist-ext")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/test/persist-ext", info.SourceURL)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccc", info.CommitHash)
}