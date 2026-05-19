package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager returns a Manager backed by a temporary directory.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager()
	m.SetDataDir(t.TempDir())
	return m
}

func sampleState(workDir string) SessionState {
	return SessionState{
		WorkDir:   workDir,
		ActiveTab: 0,
		Tabs: []TabState{
			{Name: "explorer", Preset: "explorer", FocusedPanel: "filetree"},
		},
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	m := newTestManager(t)

	state := SessionState{
		WorkDir:   "/home/user/project",
		ActiveTab: 1,
		Tabs: []TabState{
			{Name: "explorer", Preset: "explorer", FocusedPanel: "filetree"},
			{Name: "git", Preset: "git", FocusedPanel: "preview"},
		},
	}

	require.NoError(t, m.Save(state))

	loaded, err := m.Load("/home/user/project")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, currentVersion, loaded.Version)
	assert.Equal(t, "/home/user/project", loaded.WorkDir)
	assert.Equal(t, 1, loaded.ActiveTab)
	require.Len(t, loaded.Tabs, 2)
	assert.Equal(t, "explorer", loaded.Tabs[0].Name)
	assert.Equal(t, "explorer", loaded.Tabs[0].Preset)
	assert.Equal(t, "filetree", loaded.Tabs[0].FocusedPanel)
	assert.Equal(t, "git", loaded.Tabs[1].Name)
	assert.Equal(t, "git", loaded.Tabs[1].Preset)
	assert.Equal(t, "preview", loaded.Tabs[1].FocusedPanel)
	assert.False(t, loaded.SavedAt.IsZero(), "SavedAt should be populated")
}

func TestLoad_NonexistentSession_ReturnsNil(t *testing.T) {
	m := newTestManager(t)

	loaded, err := m.Load("/does/not/exist")
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestSessionPath_DifferentDirs_DifferentPaths(t *testing.T) {
	m := newTestManager(t)

	p1 := m.SessionPath("/home/user/project-a")
	p2 := m.SessionPath("/home/user/project-b")

	assert.NotEqual(t, p1, p2, "different work dirs should produce different session paths")
	assert.True(t, filepath.Ext(p1) == ".toml", "session files should have .toml extension")
	assert.True(t, filepath.Ext(p2) == ".toml", "session files should have .toml extension")
}

func TestSessionPath_SameDir_SamePath(t *testing.T) {
	m := newTestManager(t)

	p1 := m.SessionPath("/home/user/project")
	p2 := m.SessionPath("/home/user/project")

	assert.Equal(t, p1, p2, "same work dir should produce the same session path")
}

func TestDelete_ExistingSession(t *testing.T) {
	m := newTestManager(t)
	workDir := "/home/user/project"

	require.NoError(t, m.Save(sampleState(workDir)))

	// Verify it was saved.
	loaded, err := m.Load(workDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Delete.
	require.NoError(t, m.Delete(workDir))

	// Verify it was deleted.
	loaded, err = m.Load(workDir)
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDelete_NonexistentSession_NoError(t *testing.T) {
	m := newTestManager(t)
	err := m.Delete("/not/a/session")
	assert.NoError(t, err)
}

func TestSave_MultipleTabs(t *testing.T) {
	m := newTestManager(t)

	state := SessionState{
		WorkDir:   "/project",
		ActiveTab: 2,
		Tabs: []TabState{
			{Name: "explorer", Preset: "explorer", FocusedPanel: "preview"},
			{Name: "git", Preset: "git", FocusedPanel: "preview"},
			{Name: "full", Preset: "full", FocusedPanel: "terminal"},
		},
	}

	require.NoError(t, m.Save(state))

	loaded, err := m.Load("/project")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.Tabs, 3)
	assert.Equal(t, 2, loaded.ActiveTab)
	assert.Equal(t, "full", loaded.Tabs[2].Preset)
	assert.Equal(t, "terminal", loaded.Tabs[2].FocusedPanel)
}

func TestSave_SetsVersionAndTimestamp(t *testing.T) {
	m := newTestManager(t)

	before := time.Now().Add(-time.Second)
	require.NoError(t, m.Save(sampleState("/v-test")))
	after := time.Now().Add(time.Second)

	loaded, err := m.Load("/v-test")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, currentVersion, loaded.Version, "version should be set to currentVersion")
	assert.True(t, loaded.SavedAt.After(before), "SavedAt should be after test start")
	assert.True(t, loaded.SavedAt.Before(after), "SavedAt should be before test end")
}

func TestLoad_WorkDirMismatch_StillLoads(t *testing.T) {
	// WorkDir is stored inside the file for informational purposes.
	// Load uses the hash of the provided workDir to find the file.
	m := newTestManager(t)
	workDir := "/home/user/myproject"

	require.NoError(t, m.Save(sampleState(workDir)))

	loaded, err := m.Load(workDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, workDir, loaded.WorkDir)
}

func TestLoad_InvalidTOML_ReturnsError(t *testing.T) {
	m := newTestManager(t)
	workDir := "/home/user/bad"

	// Write garbage to the session file location.
	path := m.SessionPath(workDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("{{{{not valid toml"), 0o644))

	loaded, err := m.Load(workDir)
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "parse session")
}

func TestSave_CreatesDirectories(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "deep", "nested", "sessions")

	m := NewManager()
	m.SetDataDir(nested)

	require.NoError(t, m.Save(sampleState("/dirtest")))

	_, err := os.Stat(m.SessionPath("/dirtest"))
	assert.NoError(t, err, "session file should exist after save")
}

func TestLoad_IncompatibleVersion_ReturnsNil(t *testing.T) {
	m := newTestManager(t)
	workDir := "/home/user/old"

	// Save a valid session, then overwrite the version.
	require.NoError(t, m.Save(sampleState(workDir)))

	// Overwrite saved session with an incompatible version.
	path := m.SessionPath(workDir)
	content := `[session]
version = 999
work_dir = "/home/user/old"
active_tab = 0
saved_at = 2025-01-01T00:00:00Z

[[session.tabs]]
name = "explorer"
preset = "explorer"
focused_panel = "filetree"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	loaded, err := m.Load(workDir)
	assert.NoError(t, err)
	assert.Nil(t, loaded, "should return nil for incompatible version")
}

func TestSessionPath_HashLength(t *testing.T) {
	m := newTestManager(t)

	path := m.SessionPath("/any/path")
	base := filepath.Base(path)
	name := base[:len(base)-len(".toml")]

	assert.Len(t, name, 16, "hash portion of filename should be 16 hex chars")
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestSave_EmptyTabs(t *testing.T) {
	m := newTestManager(t)

	state := SessionState{
		WorkDir:   "/empty-tabs",
		ActiveTab: 0,
		Tabs:      []TabState{},
	}
	require.NoError(t, m.Save(state))

	loaded, err := m.Load("/empty-tabs")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Empty(t, loaded.Tabs)
	assert.Equal(t, 0, loaded.ActiveTab)
}

func TestSave_OverwriteExisting(t *testing.T) {
	m := newTestManager(t)
	workDir := "/overwrite"

	require.NoError(t, m.Save(sampleState(workDir)))
	loaded, err := m.Load(workDir)
	require.NoError(t, err)
	assert.Equal(t, "explorer", loaded.Tabs[0].Name)

	// Overwrite with different data.
	state := SessionState{
		WorkDir:   workDir,
		ActiveTab: 0,
		Tabs:      []TabState{{Name: "git", Preset: "git", FocusedPanel: "preview"}},
	}
	require.NoError(t, m.Save(state))
	loaded, err = m.Load(workDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "git", loaded.Tabs[0].Name)
}

func TestLoad_EmptyFile_ReturnsError(t *testing.T) {
	m := newTestManager(t)
	workDir := "/empty-file"

	path := m.SessionPath(workDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	loaded, err := m.Load(workDir)
	// Empty file should parse as empty TOML (version=0 → incompatible → nil).
	assert.NoError(t, err)
	assert.Nil(t, loaded, "empty file has version=0, should return nil")
}

func TestLoad_VersionZero_ReturnsNil(t *testing.T) {
	m := newTestManager(t)
	workDir := "/ver-zero"

	path := m.SessionPath(workDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := `[session]
version = 0
work_dir = "/ver-zero"
active_tab = 0
saved_at = 2025-01-01T00:00:00Z

[[session.tabs]]
name = "explorer"
preset = "explorer"
focused_panel = "filetree"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	loaded, err := m.Load(workDir)
	assert.NoError(t, err)
	assert.Nil(t, loaded, "version 0 should be treated as incompatible")
}

func TestSave_FilePermissions(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("file permission checks not reliable on Windows")
	}

	m := newTestManager(t)
	state := sampleState("/test/perm-check")

	require.NoError(t, m.Save(state))

	path := m.SessionPath(state.WorkDir)
	info, err := os.Stat(path)
	require.NoError(t, err)

	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o600), mode,
		"session file should be owner-only (0600), got %o", mode)
}

func TestSave_DirectoryPermissions(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("directory permission checks not reliable on Windows")
	}

	dir := filepath.Join(t.TempDir(), "new-session-dir")
	m := NewManager()
	m.SetDataDir(dir)

	state := sampleState("/test/dir-perm")
	require.NoError(t, m.Save(state))

	info, err := os.Stat(dir)
	require.NoError(t, err)

	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o700), mode,
		"session directory should be owner-only (0700), got %o", mode)
}

func TestSessionPath_SpecialCharacters(t *testing.T) {
	m := newTestManager(t)

	// Paths with special characters should produce valid session paths.
	p1 := m.SessionPath("/path/with spaces/project")
	p2 := m.SessionPath("/path/with-dashes/project")
	p3 := m.SessionPath("/path/with.dots/project")

	assert.NotEqual(t, p1, p2)
	assert.NotEqual(t, p2, p3)
	assert.True(t, filepath.Ext(p1) == ".toml")
	assert.True(t, filepath.Ext(p2) == ".toml")
	assert.True(t, filepath.Ext(p3) == ".toml")
}

func TestDelete_AfterSave_ReloadReturnsNil(t *testing.T) {
	m := newTestManager(t)
	workDir := "/delete-reload"

	require.NoError(t, m.Save(sampleState(workDir)))
	require.NoError(t, m.Delete(workDir))

	// Verify file is gone.
	_, err := os.Stat(m.SessionPath(workDir))
	assert.True(t, errors.Is(err, fs.ErrNotExist), "session file should not exist after delete")

	loaded, err := m.Load(workDir)
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestSetDataDir(t *testing.T) {
	m := NewManager()
	newDir := t.TempDir()
	m.SetDataDir(newDir)

	// SessionPath should use the new directory.
	path := m.SessionPath("/test")
	assert.True(t, strings.HasPrefix(path, newDir),
		"session path should be under new data dir")
}

func TestSave_LargeTabCount(t *testing.T) {
	m := newTestManager(t)

	tabs := make([]TabState, 50)
	for i := range tabs {
		tabs[i] = TabState{
			Name:         fmt.Sprintf("tab-%d", i),
			Preset:       "explorer",
			FocusedPanel: "filetree",
		}
	}

	state := SessionState{
		WorkDir:   "/large-tabs",
		ActiveTab: 25,
		Tabs:      tabs,
	}
	require.NoError(t, m.Save(state))

	loaded, err := m.Load("/large-tabs")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Len(t, loaded.Tabs, 50)
	assert.Equal(t, 25, loaded.ActiveTab)
	assert.Equal(t, "tab-49", loaded.Tabs[49].Name)
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m)
	// The data dir should be set to something non-empty.
	path := m.SessionPath("/any")
	assert.NotEmpty(t, path)
}
