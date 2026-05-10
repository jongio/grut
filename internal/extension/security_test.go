package extension

import (
	"context"
	"os"
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
