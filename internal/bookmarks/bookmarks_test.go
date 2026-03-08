package bookmarks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager creates a Manager with a temporary config directory
// and no seed bookmarks, fully isolated from the user's real config.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManagerWithDir(config.BookmarksConfig{}, t.TempDir())
}

func TestNewManager_SeedsFromConfig(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	m := NewManagerWithDir(config.BookmarksConfig{
		Paths: []string{dir1, dir2},
	}, t.TempDir())

	bookmarks := m.List()
	require.Len(t, bookmarks, 2)
	assert.Equal(t, filepath.Clean(dir1), bookmarks[0].Path)
	assert.Equal(t, filepath.Base(dir1), bookmarks[0].Name)
	assert.Equal(t, filepath.Clean(dir2), bookmarks[1].Path)
}

func TestNewManager_DeduplicatesSeedPaths(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(config.BookmarksConfig{
		Paths: []string{dir, dir, dir},
	}, t.TempDir())

	assert.Len(t, m.List(), 1)
}

func TestAdd_Success(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	err := m.Add(dir)
	require.NoError(t, err)

	bookmarks := m.List()
	require.Len(t, bookmarks, 1)
	assert.Equal(t, filepath.Clean(dir), bookmarks[0].Path)
	assert.Equal(t, filepath.Base(dir), bookmarks[0].Name)
}

func TestAdd_DuplicateReturnsError(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	require.NoError(t, m.Add(dir))
	err := m.Add(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestAdd_NonExistentPathReturnsError(t *testing.T) {
	m := newTestManager(t)
	err := m.Add(filepath.Join(t.TempDir(), "nonexistent"))
	assert.Error(t, err)
}

func TestAdd_FilePathReturnsError(t *testing.T) {
	m := newTestManager(t)
	f := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	err := m.Add(f)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestRemove_Success(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	require.NoError(t, m.Add(dir))
	require.Len(t, m.List(), 1)

	err := m.Remove(dir)
	require.NoError(t, err)
	assert.Empty(t, m.List())
}

func TestRemove_NotFoundReturnsError(t *testing.T) {
	m := newTestManager(t)
	err := m.Remove("/not/bookmarked")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHas(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	assert.False(t, m.Has(dir))
	require.NoError(t, m.Add(dir))
	assert.True(t, m.Has(dir))
}

func TestSaveAndLoad(t *testing.T) {
	cfgDir := t.TempDir()

	// Create manager, add bookmarks, save.
	m := newTestManager(t)
	m.SetConfigDir(cfgDir)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	require.NoError(t, m.Add(dir1))
	require.NoError(t, m.Add(dir2))
	require.NoError(t, m.Save())

	// Verify file was created.
	_, err := os.Stat(filepath.Join(cfgDir, "bookmarks.toml"))
	require.NoError(t, err)

	// Create new manager from same config dir — should load saved bookmarks.
	m2 := NewManagerWithDir(config.BookmarksConfig{}, cfgDir)
	// Re-load explicitly since SetConfigDir is after construction.
	saved, err := m2.load()
	require.NoError(t, err)
	require.Len(t, saved, 2)
	assert.Equal(t, filepath.Clean(dir1), saved[0].Path)
	assert.Equal(t, filepath.Clean(dir2), saved[1].Path)
}

func TestSave_CreatesConfigDir(t *testing.T) {
	base := t.TempDir()
	cfgDir := filepath.Join(base, "nested", "config")

	m := newTestManager(t)
	m.SetConfigDir(cfgDir)

	require.NoError(t, m.Add(t.TempDir()))
	require.NoError(t, m.Save())

	_, err := os.Stat(filepath.Join(cfgDir, "bookmarks.toml"))
	require.NoError(t, err)
}

func TestList_ReturnsSnapshot(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	list := m.List()
	require.Len(t, list, 1)

	// Mutating the returned slice should not affect the manager.
	list[0].Name = "mutated"
	assert.NotEqual(t, "mutated", m.List()[0].Name)
}

func TestRemove_PreservesOrder(t *testing.T) {
	m := newTestManager(t)
	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		require.NoError(t, m.Add(dirs[i]))
	}

	// Remove the middle one.
	require.NoError(t, m.Remove(dirs[1]))

	list := m.List()
	require.Len(t, list, 2)
	assert.Equal(t, filepath.Clean(dirs[0]), list[0].Path)
	assert.Equal(t, filepath.Clean(dirs[2]), list[1].Path)
}
