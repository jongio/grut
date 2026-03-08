package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeManifest is a test helper that writes a valid extension.toml into dir.
func writeManifest(t *testing.T, dir, name, version, runtime string) {
	t.Helper()
	content := `name = "` + name + `"
version = "` + version + `"
runtime = "` + runtime + `"
permissions = ["file_read"]
`
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extension.toml"), []byte(content), 0o644))
}

func TestNewManager_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "extensions")
	_ = NewManager(dir)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestInstall_LocalPath(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Create a source extension directory.
	srcDir := filepath.Join(t.TempDir(), "my-ext-src")
	writeManifest(t, srcDir, "my-ext", "1.0.0", "lua")

	err := mgr.Install(context.Background(), srcDir)
	require.NoError(t, err)

	// Verify it's installed.
	info, err := mgr.Get("my-ext")
	require.NoError(t, err)
	assert.Equal(t, "my-ext", info.Manifest.Name)
	assert.True(t, info.Enabled)
	assert.DirExists(t, filepath.Join(extDir, "my-ext"))
}

func TestInstall_ValidatesManifest(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Create source with invalid manifest (missing runtime).
	srcDir := filepath.Join(t.TempDir(), "bad-ext")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "extension.toml"),
		[]byte(`name = "bad"
version = "1.0.0"
`),
		0o644,
	))

	err := mgr.Install(context.Background(), srcDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime is required")
}

func TestInstall_DuplicateName(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "dup-ext")
	writeManifest(t, srcDir, "dup-ext", "1.0.0", "lua")

	require.NoError(t, mgr.Install(context.Background(), srcDir))

	// Second install of the same name should fail.
	srcDir2 := filepath.Join(t.TempDir(), "dup-ext-2")
	writeManifest(t, srcDir2, "dup-ext", "1.0.0", "lua")

	err := mgr.Install(context.Background(), srcDir2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestInstall_RejectsNonHTTPS(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	tests := []struct {
		name string
		url  string
	}{
		{"ssh", "git@github.com:user/repo.git"},
		{"git protocol", "git://github.com/user/repo.git"},
		{"http", "http://github.com/user/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Install(context.Background(), tt.url)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "only https://")
		})
	}
}

func TestRemove(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "rm-ext")
	writeManifest(t, srcDir, "rm-ext", "1.0.0", "mcp")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	err := mgr.Remove("rm-ext")
	require.NoError(t, err)

	_, err = mgr.Get("rm-ext")
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(extDir, "rm-ext"))
}

func TestRemove_NotFound(t *testing.T) {
	mgr := NewManager(filepath.Join(t.TempDir(), "extensions"))
	err := mgr.Remove("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEnableDisable_Persistence(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "toggle-ext")
	writeManifest(t, srcDir, "toggle-ext", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	// Starts enabled by default.
	info, err := mgr.Get("toggle-ext")
	require.NoError(t, err)
	assert.True(t, info.Enabled)

	// Disable.
	require.NoError(t, mgr.Disable("toggle-ext"))
	info, err = mgr.Get("toggle-ext")
	require.NoError(t, err)
	assert.False(t, info.Enabled)

	// Re-enable.
	require.NoError(t, mgr.Enable("toggle-ext"))
	info, err = mgr.Get("toggle-ext")
	require.NoError(t, err)
	assert.True(t, info.Enabled)

	// Verify state survives a fresh load.
	mgr2 := NewManager(extDir)
	require.NoError(t, mgr2.LoadAll())
	info2, err := mgr2.Get("toggle-ext")
	require.NoError(t, err)
	assert.True(t, info2.Enabled)
}

func TestEnableDisable_NotFound(t *testing.T) {
	mgr := NewManager(filepath.Join(t.TempDir(), "extensions"))

	err := mgr.Enable("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	err = mgr.Disable("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestList(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Empty initially.
	assert.Empty(t, mgr.List())

	// Add two extensions.
	src1 := filepath.Join(t.TempDir(), "ext-a")
	writeManifest(t, src1, "ext-a", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), src1))

	src2 := filepath.Join(t.TempDir(), "ext-b")
	writeManifest(t, src2, "ext-b", "2.0.0", "wasm")
	require.NoError(t, mgr.Install(context.Background(), src2))

	list := mgr.List()
	assert.Len(t, list, 2)

	names := make(map[string]bool)
	for _, info := range list {
		names[info.Manifest.Name] = true
	}
	assert.True(t, names["ext-a"])
	assert.True(t, names["ext-b"])
}

func TestGet_Found(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "get-ext")
	writeManifest(t, srcDir, "get-ext", "3.0.0", "mcp")
	require.NoError(t, mgr.Install(context.Background(), srcDir))

	info, err := mgr.Get("get-ext")
	require.NoError(t, err)
	assert.Equal(t, "get-ext", info.Manifest.Name)
	assert.Equal(t, "3.0.0", info.Manifest.Version)
}

func TestGet_NotFound(t *testing.T) {
	mgr := NewManager(filepath.Join(t.TempDir(), "extensions"))
	_, err := mgr.Get("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadAll(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")

	// Pre-populate extension directories manually.
	writeManifest(t, filepath.Join(extDir, "scan-a"), "scan-a", "1.0.0", "lua")
	writeManifest(t, filepath.Join(extDir, "scan-b"), "scan-b", "2.0.0", "wasm")

	// Create a directory without a manifest (should be skipped).
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "no-manifest"), 0o755))

	// Create a non-directory file (should be skipped).
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "stray-file.txt"), []byte("hi"), 0o644))

	mgr := NewManager(extDir)
	require.NoError(t, mgr.LoadAll())

	list := mgr.List()
	assert.Len(t, list, 2)

	_, err := mgr.Get("scan-a")
	require.NoError(t, err)
	_, err = mgr.Get("scan-b")
	require.NoError(t, err)
}

func TestLoadAll_RestoresState(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "state-ext")
	writeManifest(t, srcDir, "state-ext", "1.0.0", "lua")
	require.NoError(t, mgr.Install(context.Background(), srcDir))
	require.NoError(t, mgr.Disable("state-ext"))

	// Fresh manager should restore disabled state.
	mgr2 := NewManager(extDir)
	require.NoError(t, mgr2.LoadAll())

	info, err := mgr2.Get("state-ext")
	require.NoError(t, err)
	assert.False(t, info.Enabled)
}

func TestInstall_NoManifest(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	// Source directory without extension.toml.
	srcDir := filepath.Join(t.TempDir(), "empty-src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	err := mgr.Install(context.Background(), srcDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest")
}

func TestInstall_RejectsSymlinks(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	mgr := NewManager(extDir)

	srcDir := filepath.Join(t.TempDir(), "ext-with-symlink")
	writeManifest(t, srcDir, "sneaky", "1.0.0", "lua")

	// Create a target file and a symlink to it inside the extension source.
	target := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o600))

	link := filepath.Join(srcDir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported on this OS/filesystem")
	}

	err := mgr.Install(context.Background(), srcDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlinks not allowed")
}
