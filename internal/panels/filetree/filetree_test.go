package filetree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockGitClient implements git.StatusReader for testing.
type mockGitClient struct{}

func (m *mockGitClient) Log(_ context.Context, _ git.LogOpts) ([]git.Commit, error) { return nil, nil }
func (m *mockGitClient) Status(_ context.Context) ([]git.FileStatus, error)         { return nil, nil }
func (m *mockGitClient) Diff(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
	return nil, nil
}
func (m *mockGitClient) Blame(_ context.Context, _ string) ([]git.BlameLine, error)  { return nil, nil }
func (m *mockGitClient) RepoRoot(_ context.Context) (string, error)                  { return "/repo", nil }
func (m *mockGitClient) IsRepo(_ context.Context) (bool, error)                      { return true, nil }
func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) { return nil, nil }

// mockGitClientWithIgnore implements both git.StatusReader and git.IgnoreChecker.
type mockGitClientWithIgnore struct {
	mockGitClient
	ignoredPaths []string
	repoRoot     string
}

func (m *mockGitClientWithIgnore) IgnoredPaths(_ context.Context) ([]string, error) {
	return m.ignoredPaths, nil
}

func (m *mockGitClientWithIgnore) RepoRoot(_ context.Context) (string, error) {
	return m.repoRoot, nil
}

func defaultCfg() config.FileTreeConfig {
	return config.FileTreeConfig{
		ShowHidden:           false,
		ShowIcons:            true,
		IconMode:             "ascii",
		SortDirectoriesFirst: true,
		FollowSymlinks:       false,
		MaxDepth:             20,
	}
}

// createTestTree builds a temp directory with a known layout:
//
//	root/
//	  .hidden
//	  README.md
//	  main.go
//	  docs/
//	    guide.md
//	  src/
//	    app.go
//	    utils.go
func createTestTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("r"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("g"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "utils.go"), []byte("u"), 0o644))

	return dir
}

// keyMsg is a shorthand for constructing a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// specialKeyMsg constructs a KeyPressMsg for special keys (enter, up, etc.).
func specialKeyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// newTestFT creates a FileTree and synchronously loads the root children,
// simulating the Init() → rootLoadedMsg cycle introduced by F05. The actual
// watcher is NOT started because its event loop blocks.
func newTestFT(t *testing.T, cfg config.FileTreeConfig, dir string) *FileTree {
	t.Helper()
	ft := New(cfg, dir)
	ft.ctx = context.Background()
	// Synchronously load root children and rebuild the visible list.
	loadChildrenStatic(ft.root, cfg)
	ft.rebuildVisible()
	return ft
}

// runCmd executes a tea.Cmd and feeds the result back through ft.Update,
// returning the final toast message. Handles the two-stage async pattern
// (F13: file op cmd → result msg → toast msg).
func runCmd(t *testing.T, ft *FileTree, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	// Check if this is already a ShowToastMsg (no second stage).
	if _, ok := msg.(notify.ShowToastMsg); ok {
		return msg
	}
	// Feed intermediate result back through Update.
	_, cmd2 := ft.Update(msg)
	if cmd2 == nil {
		return msg
	}
	return cmd2()
}

// ---------------------------------------------------------------------------
// Compile-time interface check
// ---------------------------------------------------------------------------

func TestFileTreeImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*FileTree)(nil)
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "Files", ft.Title())
	assert.NotNil(t, ft.KeyBindings())
	assert.Greater(t, len(ft.KeyBindings()), 0)

	// Hidden files filtered by default → visible should not contain .hidden.
	// Visible: docs, src, README.md, main.go = 4
	assert.Equal(t, 4, ft.visibleCount(),
		"expected 4 visible entries (2 dirs + 2 files, .hidden filtered)")
}

func TestInit(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	cmd := ft.Init(context.Background())
	// Init now starts a filesystem watcher, so it returns a non-nil command.
	assert.NotNil(t, cmd, "Init should return a watcher command")
}

// ---------------------------------------------------------------------------
// Sort order
// ---------------------------------------------------------------------------

func TestSortDirectoriesFirst(t *testing.T) {
	dir := createTestTree(t)
	cfg := defaultCfg()
	cfg.SortDirectoriesFirst = true
	ft := newTestFT(t, cfg, dir)

	// Directories should come before files.
	assert.True(t, ft.visibleCount() >= 4)
	assert.Equal(t, "docs", ft.visibleName(0))
	assert.Equal(t, "src", ft.visibleName(1))
	assert.Equal(t, "main.go", ft.visibleName(2))
	assert.Equal(t, "README.md", ft.visibleName(3))
}

func TestSortDirectoriesFirstDisabled(t *testing.T) {
	dir := createTestTree(t)
	cfg := defaultCfg()
	cfg.SortDirectoriesFirst = false
	ft := newTestFT(t, cfg, dir)

	// Pure alphabetical (case-insensitive), no directory priority.
	names := make([]string, ft.visibleCount())
	for i := range names {
		names[i] = ft.visibleName(i)
	}
	// Expected: docs, main.go, README.md, src (case-insensitive alpha)
	assert.Equal(t, []string{"docs", "main.go", "README.md", "src"}, names)
}

// ---------------------------------------------------------------------------
// Hidden file filtering
// ---------------------------------------------------------------------------

func TestHiddenFileFiltering(t *testing.T) {
	dir := createTestTree(t)

	cfg := defaultCfg()
	cfg.ShowHidden = false
	ft := newTestFT(t, cfg, dir)

	for i := 0; i < ft.visibleCount(); i++ {
		assert.False(t, isHidden(ft.visibleName(i)),
			"hidden file %q should not be visible", ft.visibleName(i))
	}
}

func TestShowHiddenFiles(t *testing.T) {
	dir := createTestTree(t)

	cfg := defaultCfg()
	cfg.ShowHidden = true
	ft := newTestFT(t, cfg, dir)

	var found bool
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == ".hidden" {
			found = true
		}
	}
	assert.True(t, found, "expected .hidden to be visible when ShowHidden=true")
}

func TestToggleHidden(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	initial := ft.visibleCount()
	assert.False(t, ft.showHiddenState())

	// Toggle hidden on.
	ft.Update(keyMsg('.'))
	assert.True(t, ft.showHiddenState())
	assert.Greater(t, ft.visibleCount(), initial, "visible count should increase after showing hidden")

	// Toggle hidden off.
	ft.Update(keyMsg('.'))
	assert.False(t, ft.showHiddenState())
	assert.Equal(t, initial, ft.visibleCount())
}

func TestGitFilterShowsHiddenChangedFiles(t *testing.T) {
	// Create a tree with a hidden dotfile directory containing changed files.
	dir := t.TempDir()
	dotDir := filepath.Join(dir, ".github")
	require.NoError(t, os.MkdirAll(dotDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dotDir, "workflow.yml"), []byte("w"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("r"), 0o644))

	cfg := defaultCfg()
	cfg.ShowHidden = false
	ft := newTestFT(t, cfg, dir)
	ft.SetGitClient(&mockGitClient{})

	// Without git filter, .github should be hidden.
	assert.Equal(t, 1, ft.visibleCount(), "only README.md visible without git filter")

	// Simulate git filter with changes in the hidden dir.
	ft.gitFilter = true
	absWorkflow := filepath.Clean(filepath.Join(dir, ".github", "workflow.yml"))
	ft.gitChangedPaths = map[string]bool{absWorkflow: true}
	ft.buildGitChangedDirs()
	// Ensure .github is loaded and expanded so children are reachable.
	ft.expandGitChangedDirs()
	ft.rebuildVisible()

	// .github dir and workflow.yml should now be visible.
	var names []string
	for i := 0; i < ft.visibleCount(); i++ {
		names = append(names, ft.visibleName(i))
	}
	assert.Contains(t, names, ".github", "hidden dir with git changes should be visible in git filter mode")
	assert.Contains(t, names, "workflow.yml", "hidden file with git changes should be visible in git filter mode")
}

func TestGitFilterShowsUntrackedInHiddenDir(t *testing.T) {
	// Untracked files in a hidden directory must also appear in git filter mode.
	dir := t.TempDir()
	subDir := filepath.Join(dir, ".config", "settings")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "new.toml"), []byte("n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("m"), 0o644))

	cfg := defaultCfg()
	cfg.ShowHidden = false
	ft := newTestFT(t, cfg, dir)
	ft.SetGitClient(&mockGitClient{})

	// Without git filter, .config should be hidden.
	for i := 0; i < ft.visibleCount(); i++ {
		assert.NotEqual(t, ".config", ft.visibleName(i))
	}

	// Simulate git filter with an untracked file in the hidden dir.
	ft.gitFilter = true
	absNew := filepath.Clean(filepath.Join(dir, ".config", "settings", "new.toml"))
	ft.gitChangedPaths = map[string]bool{absNew: true}
	ft.buildGitChangedDirs()
	ft.expandGitChangedDirs()
	ft.rebuildVisible()

	var names []string
	for i := 0; i < ft.visibleCount(); i++ {
		names = append(names, ft.visibleName(i))
	}
	assert.Contains(t, names, ".config", "hidden dir with untracked files should be visible in git filter mode")
	assert.Contains(t, names, "settings", "nested dir with untracked files should be visible")
	assert.Contains(t, names, "new.toml", "untracked file in hidden dir should be visible in git filter mode")
}

// ---------------------------------------------------------------------------
// Cursor navigation
// ---------------------------------------------------------------------------

func TestCursorNavigation(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	assert.Equal(t, 0, ft.cursorIndex())

	// Move down with 'j'.
	ft.Update(keyMsg('j'))
	assert.Equal(t, 1, ft.cursorIndex())

	// Move down with arrow key.
	ft.Update(specialKeyMsg(tea.KeyDown))
	assert.Equal(t, 2, ft.cursorIndex())

	// Move up with 'k'.
	ft.Update(keyMsg('k'))
	assert.Equal(t, 1, ft.cursorIndex())

	// Move up with arrow key.
	ft.Update(specialKeyMsg(tea.KeyUp))
	assert.Equal(t, 0, ft.cursorIndex())
}

func TestCursorBoundaries(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Can't move above 0.
	ft.Update(keyMsg('k'))
	assert.Equal(t, 0, ft.cursorIndex())

	// Move to bottom.
	for i := 0; i < ft.visibleCount()+5; i++ {
		ft.Update(keyMsg('j'))
	}
	assert.Equal(t, ft.visibleCount()-1, ft.cursorIndex())

	// Can't go beyond bottom.
	ft.Update(keyMsg('j'))
	assert.Equal(t, ft.visibleCount()-1, ft.cursorIndex())
}

func TestGoToTopBottom(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Move cursor down a couple of times.
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	assert.Greater(t, ft.cursorIndex(), 0)

	// Go to bottom with 'G'.
	ft.Update(tea.KeyPressMsg{Code: -1, Text: "G"})
	assert.Equal(t, ft.visibleCount()-1, ft.cursorIndex())
}

// ---------------------------------------------------------------------------
// Expand / collapse
// ---------------------------------------------------------------------------

func TestExpandCollapse(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Cursor starts at "docs" (first directory, sorted dirs-first).
	assert.Equal(t, "docs", ft.visibleName(0))
	initialCount := ft.visibleCount() // 4

	// Expand "docs" with enter.
	ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Greater(t, ft.visibleCount(), initialCount,
		"visible count should increase after expanding")
	// "guide.md" should now be visible at index 1.
	assert.Equal(t, "guide.md", ft.visibleName(1))

	// Collapse "docs" with 'h'.
	ft.Update(keyMsg('h'))
	assert.Equal(t, initialCount, ft.visibleCount())
}

func TestExpandWithL(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	count := ft.visibleCount()
	ft.Update(keyMsg('l')) // expand first dir
	assert.Greater(t, ft.visibleCount(), count)
}

func TestExpandWithRight(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	count := ft.visibleCount()
	ft.Update(specialKeyMsg(tea.KeyRight))
	assert.Greater(t, ft.visibleCount(), count)
}

func TestCollapseWithLeft(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Expand first, then collapse.
	ft.Update(specialKeyMsg(tea.KeyEnter))
	expanded := ft.visibleCount()
	ft.Update(specialKeyMsg(tea.KeyLeft))
	assert.Less(t, ft.visibleCount(), expanded)
}

func TestCollapseGoToParent(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Expand "docs" then move cursor to its child "guide.md".
	ft.Update(specialKeyMsg(tea.KeyEnter)) // expand docs
	ft.Update(keyMsg('j'))                 // move to guide.md
	assert.Equal(t, "guide.md", ft.visibleName(ft.cursorIndex()))

	// Press 'h' on a file → should go to parent (docs).
	ft.Update(keyMsg('h'))
	assert.Equal(t, "docs", ft.visibleName(ft.cursorIndex()))
}

// ---------------------------------------------------------------------------
// File selection message
// ---------------------------------------------------------------------------

func TestFileSelectedMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Move cursor to a file (after dirs: index 2 is main.go, index 3 is README.md).
	ft.Update(keyMsg('j')) // → src
	ft.Update(keyMsg('j')) // → main.go
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))

	_, cmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, cmd, "selecting a file should produce a command")

	msg := cmd()
	fsm, ok := msg.(panels.FileSelectedMsg)
	require.True(t, ok, "command should produce panels.FileSelectedMsg")
	assert.Equal(t, filepath.Join(dir, "main.go"), fsm.Path)
}

func TestDirChangedMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Expand a collapsed directory → should emit DirChangedMsg.
	_, cmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, cmd)

	msg := cmd()
	dcm, ok := msg.(DirChangedMsg)
	require.True(t, ok, "expanding a dir should produce DirChangedMsg")
	assert.Equal(t, filepath.Join(dir, "docs"), dcm.Path)
}

// ---------------------------------------------------------------------------
// Multi-select
// ---------------------------------------------------------------------------

func TestToggleSelection(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	path := ft.visiblePath(0)
	assert.False(t, ft.isPathSelected(path))

	ft.Update(specialKeyMsg(tea.KeySpace))
	assert.True(t, ft.isPathSelected(path))

	ft.Update(specialKeyMsg(tea.KeySpace))
	assert.False(t, ft.isPathSelected(path))
}

// ---------------------------------------------------------------------------
// Max depth enforcement
// ---------------------------------------------------------------------------

func TestMaxDepthEnforcement(t *testing.T) {
	dir := t.TempDir()

	// Create nested dirs: dir/a/b/c/d
	nested := dir
	for _, name := range []string{"a", "b", "c", "d"} {
		nested = filepath.Join(nested, name)
		require.NoError(t, os.Mkdir(nested, 0o755))
	}
	// Create a file in the deepest dir.
	require.NoError(t, os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("d"), 0o644))

	cfg := defaultCfg()
	cfg.MaxDepth = 3 // Allow depth 0, 1, 2 only.
	ft := newTestFT(t, cfg, dir)
	ft.Focus()

	// Expand all directories.
	for i := 0; i < 10; i++ {
		if ft.cursorIndex() < ft.visibleCount() {
			n := ft.visible[ft.cursorIndex()]
			if n.isDir {
				ft.Update(specialKeyMsg(tea.KeyEnter))
			}
		}
		ft.Update(keyMsg('j'))
	}

	// "d" directory should NOT be expandable (depth would be 3 → children at depth 3).
	// deep.txt should not be visible.
	for i := 0; i < ft.visibleCount(); i++ {
		assert.NotEqual(t, "deep.txt", ft.visibleName(i),
			"file at depth 4 should not be visible with MaxDepth=3")
	}
}

// ---------------------------------------------------------------------------
// Symlink handling
// ---------------------------------------------------------------------------

func TestSymlinkDetection(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o644))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	ft := newTestFT(t, defaultCfg(), dir)

	var foundLink bool
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "link.txt" {
			foundLink = true
			n := ft.visible[i]
			assert.True(t, n.isSymlink)
			assert.NotEmpty(t, n.symlinkTarget)
		}
	}
	assert.True(t, foundLink, "symlink should be detected and visible")
}

func TestSymlinkDirNotExpandedWhenFollowDisabled(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "file.txt"), []byte("f"), 0o644))
	if err := os.Symlink(sub, filepath.Join(dir, "link_dir")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	cfg := defaultCfg()
	cfg.FollowSymlinks = false
	ft := newTestFT(t, cfg, dir)
	ft.Focus()

	// Find the symlink dir and try to expand it.
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "link_dir" {
			ft.cursor = i
			break
		}
	}

	before := ft.visibleCount()
	ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Equal(t, before, ft.visibleCount(),
		"symlink dir should not expand when FollowSymlinks is false")
}

func TestSymlinkLoopDetection(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	// link → dir (ancestor → creates a loop)
	if err := os.Symlink(dir, filepath.Join(sub, "loop")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	cfg := defaultCfg()
	cfg.FollowSymlinks = true
	ft := newTestFT(t, cfg, dir)
	ft.Focus()

	// Expand "sub".
	ft.Update(specialKeyMsg(tea.KeyEnter))

	// Find "loop" and try to expand.
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "loop" {
			ft.cursor = i
			break
		}
	}

	before := ft.visibleCount()
	ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Equal(t, before, ft.visibleCount(),
		"symlink loop should be detected and not expanded")
}

func TestPathJailing(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	cfg := defaultCfg()
	cfg.FollowSymlinks = true
	ft := newTestFT(t, cfg, root)

	assert.False(t, ft.isPathSafe(filepath.Join(root, "escape")),
		"symlink pointing outside root should not be considered safe")
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestViewContainsFileNames(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	view := ft.View(60, 20)
	assert.Contains(t, view, "docs")
	assert.Contains(t, view, "src")
	assert.Contains(t, view, "README.md")
	assert.Contains(t, view, "main.go")
	assert.NotContains(t, view, ".hidden")
}

func TestViewExpandedShowsChildren(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	ft.Update(specialKeyMsg(tea.KeyEnter)) // expand docs
	view := ft.View(60, 20)
	assert.Contains(t, view, "guide.md")
}

func TestViewEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	ft := newTestFT(t, defaultCfg(), dir)

	view := ft.View(40, 10)
	assert.Contains(t, view, "Empty")
}

func TestViewZeroSize(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	assert.Empty(t, ft.View(0, 10))
	assert.Empty(t, ft.View(10, 0))
	assert.Empty(t, ft.View(0, 0))
	assert.Empty(t, ft.View(-1, 10))
}

// ---------------------------------------------------------------------------
// Scrolling
// ---------------------------------------------------------------------------

func TestScrolling(t *testing.T) {
	dir := t.TempDir()
	// Create many files to force scrolling.
	for i := 0; i < 30; i++ {
		name := filepath.Join(dir, string(rune('a'+i%26))+string(rune('a'+i/26))+".txt")
		require.NoError(t, os.WriteFile(name, []byte("x"), 0o644))
	}

	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()
	ft.height = 5 // small viewport

	// Move cursor beyond viewport.
	for i := 0; i < 10; i++ {
		ft.Update(keyMsg('j'))
	}
	assert.Equal(t, 10, ft.cursorIndex())
	// Cursor should still be visible (offset adjusted).
	assert.LessOrEqual(t, ft.offset, ft.cursorIndex())
	assert.Greater(t, ft.offset+ft.height, ft.cursorIndex())
}

// ---------------------------------------------------------------------------
// Focus/Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	assert.False(t, ft.focused)

	ft.Focus()
	assert.True(t, ft.focused)

	ft.Blur()
	assert.False(t, ft.focused)
}

func TestKeyIgnoredWhenBlurred(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	// Not focused: key presses should be ignored.
	ft.Update(keyMsg('j'))
	assert.Equal(t, 0, ft.cursorIndex())
}

func TestSetSize(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	ft.SetSize(80, 24)
	assert.Equal(t, 80, ft.width)
	assert.Equal(t, 24, ft.height)
}

// ---------------------------------------------------------------------------
// Icon mapping
// ---------------------------------------------------------------------------

func TestIconMappingByExtension(t *testing.T) {
	tests := []struct {
		name     string
		wantIcon bool
	}{
		{"main.go", true},
		{"app.js", true},
		{"style.css", true},
		{"README.md", true},
		{"config.yaml", true},
		{"data.json", true},
		{"unknown.xyz", true}, // falls back to default file icon
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icon := getFileIcon(tt.name, false, false, "nerd")
			if tt.wantIcon {
				assert.NotEmpty(t, icon)
			}
		})
	}
}

func TestIconASCIIModeReturnsEmpty(t *testing.T) {
	icon := getFileIcon("main.go", false, false, "ascii")
	assert.Empty(t, icon, "ASCII mode should return no file icon")
}

func TestDirectoryIcons(t *testing.T) {
	collapsed := getFileIcon("src", true, false, "nerd")
	expanded := getFileIcon("src", true, true, "nerd")
	assert.NotEmpty(t, collapsed)
	assert.NotEmpty(t, expanded)
	assert.NotEqual(t, collapsed, expanded, "open and closed dir icons should differ")
}

func TestExpandIcons(t *testing.T) {
	colASCII := getExpandIcon(false, "ascii")
	expASCII := getExpandIcon(true, "ascii")
	assert.Equal(t, "▸", colASCII)
	assert.Equal(t, "▾", expASCII)

	colNerd := getExpandIcon(false, "nerd")
	expNerd := getExpandIcon(true, "nerd")
	assert.NotEmpty(t, colNerd)
	assert.NotEmpty(t, expNerd)
	assert.NotEqual(t, colNerd, expNerd)
}

func TestAutoIconModeFallsBackToASCII(t *testing.T) {
	icon := getFileIcon("main.go", false, false, "auto")
	assert.Empty(t, icon, "auto mode should behave like ascii (no file icon)")
}

// ---------------------------------------------------------------------------
// Update returns
// ---------------------------------------------------------------------------

func TestUpdateUnknownMsgPassesThrough(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	type customMsg struct{}
	p, cmd := ft.Update(customMsg{})
	assert.Equal(t, ft, p)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Symlink expansion with FollowSymlinks=true
// ---------------------------------------------------------------------------

func TestSymlinkDirExpandsWhenFollowEnabled(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "real_sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "inner.txt"), []byte("i"), 0o644))
	if err := os.Symlink(sub, filepath.Join(dir, "link_sub")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	cfg := defaultCfg()
	cfg.FollowSymlinks = true
	ft := newTestFT(t, cfg, dir)
	ft.Focus()

	// Find the symlink dir and expand it.
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "link_sub" {
			ft.cursor = i
			break
		}
	}

	before := ft.visibleCount()
	ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Greater(t, ft.visibleCount(), before,
		"symlink dir should expand when FollowSymlinks=true and path is safe")
}

// ---------------------------------------------------------------------------
// Render line with nerd icons
// ---------------------------------------------------------------------------

func TestViewWithNerdIcons(t *testing.T) {
	dir := createTestTree(t)
	cfg := defaultCfg()
	cfg.IconMode = "nerd"
	ft := newTestFT(t, cfg, dir)
	ft.Focus()

	view := ft.View(60, 20)
	assert.Contains(t, view, "docs")
	assert.Contains(t, view, "main.go")
}

// ---------------------------------------------------------------------------
// Render symlink target in view
// ---------------------------------------------------------------------------

func TestViewShowsSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o644))
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	ft := newTestFT(t, defaultCfg(), dir)
	view := ft.View(80, 20)
	assert.Contains(t, view, "→")
}

// ---------------------------------------------------------------------------
// Edge: selectOrExpand on empty list
// ---------------------------------------------------------------------------

func TestSelectOrExpandEmpty(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	ft.Focus()
	p, cmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Equal(t, ft, p)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Edge: collapseOrParent at root level
// ---------------------------------------------------------------------------

func TestCollapseAtRootLevel(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// At root level (depth 0), pressing h should do nothing (no parent).
	ft.Update(keyMsg('h'))
	assert.Equal(t, 0, ft.cursorIndex())
}

// ---------------------------------------------------------------------------
// Icon by filename matching
// ---------------------------------------------------------------------------

func TestIconByFilename(t *testing.T) {
	icon := getFileIcon("Dockerfile", false, false, "nerd")
	assert.NotEmpty(t, icon)

	icon = getFileIcon(".gitignore", false, false, "nerd")
	assert.NotEmpty(t, icon)
}

// ---------------------------------------------------------------------------
// isPathSafe with non-symlink paths
// ---------------------------------------------------------------------------

func TestIsPathSafeWithRegularPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	ft := newTestFT(t, defaultCfg(), dir)
	assert.True(t, ft.isPathSafe(sub),
		"regular subdirectory should be considered safe")
}

func TestIsPathSafeOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	ft := newTestFT(t, defaultCfg(), root)
	assert.False(t, ft.isPathSafe(outside),
		"path outside root should not be considered safe")
}

// ---------------------------------------------------------------------------
// File operation key bindings — integration tests
// ---------------------------------------------------------------------------

// initFT creates a focused FileTree with Init() called.
func initFT(t *testing.T, dir string) *FileTree {
	t.Helper()
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()
	return ft
}

func TestDeleteKey_ShowsConfirmationModal(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Move to a file (main.go at index 2).
	ft.Update(keyMsg('j')) // src
	ft.Update(keyMsg('j')) // main.go
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))

	// Press 'x' to request delete.
	_, cmd := ft.Update(keyMsg('x'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg")
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Equal(t, "Delete", modal.Title)
	assert.Contains(t, modal.Message, "main.go")
}

func TestDeleteKey_BulkShowsCount(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Select two items.
	ft.Update(specialKeyMsg(tea.KeySpace)) // select docs
	ft.Update(keyMsg('j'))                 // move to src
	ft.Update(specialKeyMsg(tea.KeySpace)) // select src

	_, cmd := ft.Update(keyMsg('x'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Contains(t, modal.Message, "2 items")
}

func TestDeleteKey_ConfirmActuallyDeletes(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Move to main.go and delete.
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))

	ft.Update(keyMsg('x'))

	// Simulate modal confirmation.
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	assert.Equal(t, notify.Success, toast.Level)

	// File should be gone.
	_, err := os.Stat(filepath.Join(dir, "main.go"))
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteKey_CancelDoesNotDelete(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('x'))

	// Cancel modal.
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)

	// File should still exist.
	_, err := os.Stat(filepath.Join(dir, "main.go"))
	assert.NoError(t, err)
}

func TestRenameKey_ShowsInputModal(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))

	_, cmd := ft.Update(keyMsg('e'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalInput, modal.Kind)
	assert.Equal(t, "Rename", modal.Title)
	assert.Equal(t, "main.go", modal.Placeholder)
}

func TestRenameKey_ConfirmRenames(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('e'))

	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "app.go"})
	require.NotNil(t, cmd)

	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)

	// New name should exist, old should not.
	_, err := os.Stat(filepath.Join(dir, "app.go"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "main.go"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyKey_CopiesToClipboard(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))

	_, cmd := ft.Update(keyMsg('c'))
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Info, toast.Level)
	assert.Contains(t, toast.Message, "Copied")

	assert.Len(t, ft.clip.paths, 1)
	assert.False(t, ft.clip.cut)
}

func TestXKey_DeletesItem(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))

	_, cmd := ft.Update(keyMsg('x'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "x should trigger delete confirmation modal")
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Equal(t, "Delete", modal.Title)
}

func TestPasteKey_CopiesFile(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Copy main.go.
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))
	ft.Update(keyMsg('c'))

	// Move cursor to docs directory and expand it.
	ft.Update(keyMsg('k'))
	ft.Update(keyMsg('k'))
	assert.Equal(t, "docs", ft.visibleName(ft.cursorIndex()))
	ft.Update(specialKeyMsg(tea.KeyEnter)) // expand docs

	// Paste into docs.
	_, cmd := ft.Update(keyMsg('p'))
	require.NotNil(t, cmd)

	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)

	// File should be copied.
	_, err := os.Stat(filepath.Join(dir, "docs", "main.go"))
	assert.NoError(t, err)

	// Original should still exist.
	_, err = os.Stat(filepath.Join(dir, "main.go"))
	assert.NoError(t, err)
}

func TestPasteKey_CutMovesFile(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Set clipboard directly to simulate a cut (no longer bound to a key).
	ft.clip = clipboard{
		paths: []string{filepath.Join(dir, "main.go")},
		cut:   true,
	}

	// Cursor starts on docs (index 0), which is a directory — good paste target.

	// Paste.
	_, cmd := ft.Update(keyMsg('p'))
	require.NotNil(t, cmd)

	// Run the async paste command and feed result back.
	runCmd(t, ft, cmd)

	// File should be moved.
	_, err := os.Stat(filepath.Join(dir, "docs", "main.go"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "main.go"))
	assert.True(t, os.IsNotExist(err))
}

func TestPasteKey_EmptyClipboard(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Paste with nothing in clipboard.
	_, cmd := ft.Update(keyMsg('p'))
	assert.Nil(t, cmd)
}

func TestNewFileKey_ShowsInputModal(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	_, cmd := ft.Update(keyMsg('n'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalInput, modal.Kind)
	assert.Equal(t, "New File", modal.Title)
}

func TestNewFileKey_ConfirmCreatesFile(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('n'))
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "brand_new.txt"})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)

	// Should be created in the cursor's directory context.
	// Cursor is on "docs" (dir), so file should be inside docs.
	_, err := os.Stat(filepath.Join(dir, "docs", "brand_new.txt"))
	assert.NoError(t, err)
}

func TestNewDirKey_ShowsInputModal(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// 'N' for new directory — need to send as a text key.
	_, cmd := ft.Update(tea.KeyPressMsg{Code: -1, Text: "N"})
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalInput, modal.Kind)
	assert.Equal(t, "New Directory", modal.Title)
}

func TestNewDirKey_ConfirmCreatesDir(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(tea.KeyPressMsg{Code: -1, Text: "N"})
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "newsubdir"})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)

	info, err := os.Stat(filepath.Join(dir, "docs", "newsubdir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDeleteKey_NoSelection(t *testing.T) {
	dir := t.TempDir()
	ft := initFT(t, dir) // empty dir

	_, cmd := ft.Update(keyMsg('x'))
	assert.Nil(t, cmd, "delete on empty tree should do nothing")
}

func TestRenameKey_EmptyValue(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('r'))

	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd, "empty rename value should be a no-op")
}

func TestNewFile_EmptyName(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('n'))
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd, "empty file name should be a no-op")
}

func TestNewDir_EmptyName(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(tea.KeyPressMsg{Code: -1, Text: "N"})
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd, "empty dir name should be a no-op")
}

func TestRefreshMsg_ReloadsTree(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	initial := ft.visibleCount()

	// Create a new file externally.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "external.txt"), []byte("x"), 0o644))

	// Send RefreshMsg.
	ft.Update(RefreshMsg{})

	assert.Greater(t, ft.visibleCount(), initial,
		"tree should show new file after refresh")
}

func TestRefreshMsg_PreservesCursor(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Move cursor to "src".
	ft.Update(keyMsg('j'))
	assert.Equal(t, "src", ft.visibleName(ft.cursorIndex()))

	ft.Update(RefreshMsg{})
	assert.Equal(t, "src", ft.visibleName(ft.cursorIndex()),
		"cursor position should be preserved after refresh")
}

func TestRefreshMsg_PreservesExpanded(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Expand docs.
	ft.Update(specialKeyMsg(tea.KeyEnter))
	assert.Contains(t, ft.View(60, 20), "guide.md")

	ft.Update(RefreshMsg{})
	assert.Contains(t, ft.View(60, 20), "guide.md",
		"expanded state should be preserved after refresh")
}

func TestModalResultMsg_NoPending(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Modal result without a pending operation should be a no-op.
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "test"})
	assert.Nil(t, cmd)
}

func TestCopyKey_WithMultiSelect(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Select two items.
	ft.Update(specialKeyMsg(tea.KeySpace)) // select docs
	ft.Update(keyMsg('j'))
	ft.Update(specialKeyMsg(tea.KeySpace)) // select src

	ft.Update(keyMsg('c'))

	assert.Len(t, ft.clip.paths, 2)
	assert.False(t, ft.clip.cut)
}

func TestSelectedPaths_UseCursorWhenNoSelection(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	paths := ft.selectedPaths()
	assert.Len(t, paths, 1)
	assert.Equal(t, ft.visiblePath(0), paths[0])
}

func TestSelectedPaths_EmptyTree(t *testing.T) {
	ft := initFT(t, t.TempDir())
	paths := ft.selectedPaths()
	assert.Nil(t, paths)
}

func TestCursorDir_OnFile(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Move to a file.
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	assert.Equal(t, "main.go", ft.visibleName(ft.cursorIndex()))

	d := ft.cursorDir()
	assert.Equal(t, dir, d)
}

func TestCursorDir_OnDir(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Cursor on docs (a directory).
	assert.Equal(t, "docs", ft.visibleName(ft.cursorIndex()))

	d := ft.cursorDir()
	assert.Equal(t, filepath.Join(dir, "docs"), d)
}

func TestCursorDir_EmptyTree(t *testing.T) {
	dir := t.TempDir()
	ft := initFT(t, dir)

	d := ft.cursorDir()
	assert.Equal(t, dir, d)
}

func TestCursorNode_EmptyTree(t *testing.T) {
	ft := initFT(t, t.TempDir())
	assert.Nil(t, ft.cursorNode())
}

func TestKeyBindingsIncludeFileOps(t *testing.T) {
	ft := newTestFT(t, defaultCfg(), t.TempDir())
	bindings := ft.KeyBindings()

	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}

	for _, expected := range []string{"item_delete", "item_edit", "copy", "cut", "paste", "new_file", "new_dir"} {
		assert.True(t, actions[expected], "expected key binding for %q", expected)
	}
}

func TestDeleteKey_ErrorToast(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('x'))

	// Delete the file before the modal confirms (simulate race).
	_ = os.Remove(filepath.Join(dir, "main.go"))

	// Confirm — os.RemoveAll on non-existent is OK, so no error.
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Success, toast.Level)
}

func TestRenameKey_InvalidName_ErrorToast(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('j'))
	ft.Update(keyMsg('e'))

	// Try to rename with path separator.
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "sub/bad"})
	require.NotNil(t, cmd)
	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestPasteKey_ErrorToast(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Set clipboard to a non-existent file (simulate it was deleted).
	ft.clip = clipboard{
		paths: []string{filepath.Join(dir, "nonexistent.txt")},
		cut:   false,
	}

	_, cmd := ft.Update(keyMsg('p'))
	require.NotNil(t, cmd)
	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestNewFile_ExistingFile_ErrorToast(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	// Request new file, cursor on docs dir.
	ft.Update(keyMsg('n'))

	// Try to create "guide.md" which already exists in docs.
	// First expand docs so the file is created in docs (cursor context).
	ft.pending.destDir = filepath.Join(dir, "docs")
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "guide.md"})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestNewDir_ExistingDir_ErrorToast(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)

	ft.Update(tea.KeyPressMsg{Code: -1, Text: "N"})
	ft.pending.destDir = dir
	_, cmd := ft.Update(notify.ModalResultMsg{Accept: true, Value: "docs"})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

// ---------------------------------------------------------------------------
// Commit-files mode and Title() tests
// ---------------------------------------------------------------------------

func TestTitle_Default(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "Files", ft.Title())
}

func TestTitle_GitFilter(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitFilter = true

	assert.Equal(t, "Files (git changed)", ft.Title())
}

func TestTitle_CommitFilesMode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.commitFilesMode = true
	ft.commitLabel = "abc1234 Fix auth bug"

	assert.Equal(t, "Files: abc1234 Fix auth bug", ft.Title())
}

func TestCommitFilesMode_EnterAndExit(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Simulate entering commit-files mode via commitFilesLoadedMsg.
	// Use files that exist in createTestTree: main.go and docs/guide.md.
	_, cmd := ft.Update(commitFilesLoadedMsg{
		files: []string{"main.go", "docs/guide.md"},
		hash:  "abc1234",
		label: "abc1234 Fix auth bug",
	})

	assert.True(t, ft.commitFilesMode)
	assert.Equal(t, "abc1234 Fix auth bug", ft.commitLabel)
	// Tree structure: docs/ (dir) > guide.md, main.go = 3 visible nodes.
	assert.Equal(t, 3, len(ft.visible))
	assert.Equal(t, "Files: abc1234 Fix auth bug", ft.Title())

	// Verify tree structure: directory node present with correct depth.
	assert.Equal(t, "docs", ft.visible[0].name)
	assert.True(t, ft.visible[0].isDir)
	assert.Equal(t, 0, ft.visible[0].depth)
	assert.Equal(t, "guide.md", ft.visible[1].name)
	assert.False(t, ft.visible[1].isDir)
	assert.Equal(t, 1, ft.visible[1].depth)
	assert.Equal(t, "main.go", ft.visible[2].name)
	assert.False(t, ft.visible[2].isDir)
	assert.Equal(t, 0, ft.visible[2].depth)

	// cmd should emit a FileSelectedMsg for the first file.
	if cmd != nil {
		msg := cmd()
		_, ok := msg.(panels.FileSelectedMsg)
		// First visible node is a directory, so expect FolderSelectedMsg.
		if !ok {
			_, ok = msg.(panels.FolderSelectedMsg)
			assert.True(t, ok, "expected FileSelectedMsg or FolderSelectedMsg from commit-files mode")
		}
	}

	// Press Escape to exit.
	ft.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, ft.commitFilesMode)
	assert.Empty(t, ft.commitLabel)
	assert.Equal(t, "Files", ft.Title())
	// Visible should be rebuilt from normal tree.
	assert.Greater(t, len(ft.visible), 0)
}

func TestCommitFilesMode_Error(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.Update(commitFilesLoadedMsg{
		err: assert.AnError,
	})

	assert.False(t, ft.commitFilesMode, "should not enter commit-files mode on error")
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestCommitFilesMode_SurvivesRefreshMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.focused = true

	// Enter commit-files mode with files that exist in createTestTree.
	ft.Update(commitFilesLoadedMsg{
		files: []string{"main.go", "docs/guide.md"},
		hash:  "abc1234",
		label: "abc1234 Fix auth bug",
	})
	require.True(t, ft.commitFilesMode)
	// Tree structure: docs/ > guide.md, main.go = 3 nodes.
	require.Equal(t, 3, len(ft.visible))

	// Send RefreshMsg (simulates filesystem watcher firing).
	ft.Update(RefreshMsg{})

	// Commit-files mode must survive: flag still set, visible list unchanged.
	assert.True(t, ft.commitFilesMode, "commitFilesMode should survive RefreshMsg")
	assert.Equal(t, 3, len(ft.visible), "visible list should still contain only commit files after RefreshMsg")
}

func TestCommitFilesMode_SurvivesGitChangedFilesMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Enter commit-files mode with files that exist in createTestTree.
	ft.Update(commitFilesLoadedMsg{
		files: []string{"main.go", "docs/guide.md"},
		hash:  "abc1234",
		label: "abc1234 Fix auth bug",
	})
	require.True(t, ft.commitFilesMode)
	// Tree structure: docs/ > guide.md, main.go = 3 nodes.
	require.Equal(t, 3, len(ft.visible))

	// Send GitChangedFilesMsg (simulates git status refresh).
	ft.Update(panels.GitChangedFilesMsg{
		Paths: map[string]bool{
			filepath.Join(dir, "README.md"): true,
			filepath.Join(dir, "main.go"):   true,
		},
	})

	// Commit-files mode must survive: flag still set, visible list unchanged
	// because commit filter takes priority over git filter.
	assert.True(t, ft.commitFilesMode, "commitFilesMode should survive GitChangedFilesMsg")
	assert.Equal(t, 3, len(ft.visible), "visible list should still contain only commit files after GitChangedFilesMsg")
}

func TestPRFilesMode_SurvivesRefreshMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.focused = true

	// Enter PR-files mode.
	ft.Update(panels.PRFilesLoadedMsg{
		Number: 42,
		Files: []panels.PRFile{
			{Filename: "src/app.go", Status: "modified"},
			{Filename: "docs/guide.md", Status: "added"},
		},
	})
	require.True(t, ft.prFilesMode)
	// Tree structure: docs/ > guide.md, src/ > app.go = 4 nodes.
	require.Equal(t, 4, len(ft.visible))

	// Send RefreshMsg.
	ft.Update(RefreshMsg{})

	// PR-files mode must survive.
	assert.True(t, ft.prFilesMode, "prFilesMode should survive RefreshMsg")
	assert.Equal(t, 4, len(ft.visible), "visible list should still contain only PR files after RefreshMsg")
}

func TestCommitFilesMode_TreeStructure(t *testing.T) {
	// Create a tree with nested directories to verify proper tree hierarchy.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("r"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "git", "log.go"), []byte("l"), 0o644))

	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Enter commit-files mode with files in nested directories.
	ft.Update(commitFilesLoadedMsg{
		files: []string{"cmd/main.go", "internal/git/log.go"},
		hash:  "abc1234",
		label: "abc1234 Fix auth bug",
	})

	require.True(t, ft.commitFilesMode)

	// Expected tree structure (with SortDirectoriesFirst):
	//   cmd/         (dir, depth 0, expanded)
	//     main.go    (file, depth 1)
	//   internal/    (dir, depth 0, expanded)
	//     git/       (dir, depth 1, expanded)
	//       log.go   (file, depth 2)
	require.Equal(t, 5, len(ft.visible), "expected 5 visible nodes (2 files + 3 dirs)")

	assert.Equal(t, "cmd", ft.visible[0].name)
	assert.True(t, ft.visible[0].isDir)
	assert.Equal(t, 0, ft.visible[0].depth)
	assert.True(t, ft.visible[0].expanded)

	assert.Equal(t, "main.go", ft.visible[1].name)
	assert.False(t, ft.visible[1].isDir)
	assert.Equal(t, 1, ft.visible[1].depth)

	assert.Equal(t, "internal", ft.visible[2].name)
	assert.True(t, ft.visible[2].isDir)
	assert.Equal(t, 0, ft.visible[2].depth)
	assert.True(t, ft.visible[2].expanded)

	assert.Equal(t, "git", ft.visible[3].name)
	assert.True(t, ft.visible[3].isDir)
	assert.Equal(t, 1, ft.visible[3].depth)
	assert.True(t, ft.visible[3].expanded)

	assert.Equal(t, "log.go", ft.visible[4].name)
	assert.False(t, ft.visible[4].isDir)
	assert.Equal(t, 2, ft.visible[4].depth)

	// README.md should NOT be visible (not in commit-changed set).
	for _, n := range ft.visible {
		assert.NotEqual(t, "README.md", n.name, "README.md should be filtered out")
	}
}

func TestFolderSelectedMsg_EmittedFromDir(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	// Find a directory in visible nodes and set cursor to it.
	dirIdx := -1
	for i, n := range ft.visible {
		if n.isDir {
			dirIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, dirIdx, 0, "should have at least one dir in visible")

	ft.cursor = dirIdx
	cmd := ft.emitCursorFileSelected()
	require.NotNil(t, cmd, "expected a command for directory selection")

	msg := cmd()
	folderMsg, ok := msg.(panels.FolderSelectedMsg)
	assert.True(t, ok, "expected FolderSelectedMsg for directory, got %T", msg)
	assert.NotEmpty(t, folderMsg.Path)
}

func TestFileSelectedMsg_EmittedFromFile(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	// Find a file in visible nodes and set cursor to it.
	fileIdx := -1
	for i, n := range ft.visible {
		if !n.isDir {
			fileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, fileIdx, 0, "should have at least one file in visible")

	ft.cursor = fileIdx
	cmd := ft.emitCursorFileSelected()
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(panels.FileSelectedMsg)
	assert.True(t, ok, "expected FileSelectedMsg for file, got %T", msg)
}

// ---------------------------------------------------------------------------
// Additional coverage: Mouse click handling
// ---------------------------------------------------------------------------

func TestMouseClick_OnFile(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Find first file index.
	fileIdx := -1
	for i, n := range ft.visible {
		if !n.isDir {
			fileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, fileIdx, 0)

	// Simulate click on that row.
	result, cmd := ft.Update(panels.PanelMouseClickMsg{ContentRow: fileIdx, ContentCol: 5})
	panel := result.(*FileTree)
	assert.Equal(t, fileIdx, panel.cursor)
	assert.NotNil(t, cmd, "clicking a file should emit FileSelectedMsg")
}

func TestMouseClick_OnDirectory(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Find first directory index.
	dirIdx := -1
	for i, n := range ft.visible {
		if n.isDir {
			dirIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, dirIdx, 0)

	beforeVisible := len(ft.visible)
	// Click on directory should expand it.
	ft.Update(panels.PanelMouseClickMsg{ContentRow: dirIdx, ContentCol: 5})
	assert.Greater(t, len(ft.visible), beforeVisible,
		"clicking directory should expand it and show children")
}

func TestMouseClick_OutOfBounds(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	originalCursor := ft.cursor

	// Click on out-of-bounds row.
	ft.Update(panels.PanelMouseClickMsg{ContentRow: 100, ContentCol: 5})
	assert.Equal(t, originalCursor, ft.cursor, "out-of-bounds click should not change cursor")
}

func TestMouseClick_NegativeRow(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	ft.Update(panels.PanelMouseClickMsg{ContentRow: -1, ContentCol: 5})
	assert.Equal(t, 0, ft.cursor)
}

func TestMouseClick_WithOffset(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 2
	ft.offset = 1

	// Click on ContentRow 0 with offset 1 should select visible[1].
	ft.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 1, ft.cursor)
}

// ---------------------------------------------------------------------------
// Additional coverage: Mouse wheel
// ---------------------------------------------------------------------------

func TestMouseWheel_Down(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 2

	assert.Equal(t, 0, ft.offset)

	ft.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, ft.offset, 0, "wheel down should increase offset")
}

func TestMouseWheel_Up(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 2

	ft.offset = 3
	ft.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, ft.offset, "wheel up should decrease offset")
}

func TestMouseWheel_UpAtTop(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 2

	ft.offset = 0
	ft.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, ft.offset, "offset should not go below 0")
}

func TestMouseWheel_CursorFollowsViewport(t *testing.T) {
	// Create a tree with enough items to scroll.
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, fmt.Sprintf("file%02d.txt", i)),
			[]byte("x"), 0o644,
		))
	}

	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 5

	assert.Equal(t, 0, ft.cursor, "cursor starts at 0")
	assert.Equal(t, 0, ft.offset, "offset starts at 0")

	// Scroll down — cursor should follow the viewport.
	ft.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, ft.offset, 0, "offset should increase")
	assert.GreaterOrEqual(t, ft.cursor, ft.offset,
		"cursor must be >= offset after scroll down")
	assert.Less(t, ft.cursor, ft.offset+ft.height,
		"cursor must be within viewport after scroll down")
}

func TestMouseWheel_RebuildDoesNotSnapBack(t *testing.T) {
	// Regression test: after scrolling down with the mouse wheel,
	// a background rebuildVisible() (e.g. from filesystem watcher)
	// must NOT snap the viewport back to the top.
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, fmt.Sprintf("file%02d.txt", i)),
			[]byte("x"), 0o644,
		))
	}

	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 5

	// Scroll down.
	ft.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	scrolledOffset := ft.offset
	assert.Greater(t, scrolledOffset, 0)

	// Simulate a background refresh triggering rebuildVisible.
	ft.rebuildVisible()

	assert.Equal(t, scrolledOffset, ft.offset,
		"rebuildVisible must not snap offset back to top after scroll")
}

// ---------------------------------------------------------------------------
// Mouse double-click tests
// ---------------------------------------------------------------------------

func TestMouseDoubleClick_OnFile(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	// Pre-confirm so the first-use prompt is skipped.
	ft.actionsCfg.Confirmed = map[string]bool{string(actions.ItemFile): true}

	// Find first file index.
	fileIdx := -1
	for i, n := range ft.visible {
		if !n.isDir {
			fileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, fileIdx, 0)

	// Simulate the first click (sets cursor), then the double-click.
	ft.Update(panels.PanelMouseClickMsg{ContentRow: fileIdx, ContentCol: 5})
	assert.Equal(t, fileIdx, ft.cursor)

	result, cmd := ft.Update(panels.PanelMouseDoubleClickMsg{ContentRow: fileIdx, ContentCol: 5})
	panel := result.(*FileTree)
	assert.Equal(t, fileIdx, panel.cursor)

	// Double-click on a file should open it in the editor, not emit
	// FileSelectedMsg. The returned command produces a toast.
	require.NotNil(t, cmd, "double-click on file should produce a command")

	// Force the error path by using a non-existent editor.
	t.Setenv("VISUAL", "this_editor_does_not_exist_67890")
	t.Setenv("EDITOR", "")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Open failed")
}

func TestMouseDoubleClick_OnFile_NotFileSelected(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	// Pre-confirm so the first-use prompt is skipped.
	ft.actionsCfg.Confirmed = map[string]bool{string(actions.ItemFile): true}

	// Find first file index.
	fileIdx := -1
	for i, n := range ft.visible {
		if !n.isDir {
			fileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, fileIdx, 0)

	// Force the error path so we can inspect the message type.
	t.Setenv("VISUAL", "this_editor_does_not_exist_67890")
	t.Setenv("EDITOR", "")

	_, cmd := ft.Update(panels.PanelMouseDoubleClickMsg{ContentRow: fileIdx, ContentCol: 5})
	require.NotNil(t, cmd)
	msg := cmd()

	// Must NOT be FileSelectedMsg — double-click opens in editor, not selects.
	_, isFileSelected := msg.(panels.FileSelectedMsg)
	assert.False(t, isFileSelected, "double-click on file should open editor, not emit FileSelectedMsg")
}

func TestMouseDoubleClick_OnDirectory(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	// Pre-confirm so the first-use prompt is skipped.
	ft.actionsCfg.Confirmed = map[string]bool{string(actions.ItemDirectory): true}

	// Find first directory index.
	dirIdx := -1
	for i, n := range ft.visible {
		if n.isDir {
			dirIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, dirIdx, 0)

	beforeVisible := len(ft.visible)
	ft.Update(panels.PanelMouseDoubleClickMsg{ContentRow: dirIdx, ContentCol: 5})
	assert.Greater(t, len(ft.visible), beforeVisible,
		"double-clicking directory should expand it")
}

func TestMouseDoubleClick_OutOfBounds(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	originalCursor := ft.cursor
	ft.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 100, ContentCol: 5})
	assert.Equal(t, originalCursor, ft.cursor, "out-of-bounds double-click should not change cursor")
}

// ---------------------------------------------------------------------------
// Additional coverage: toggleGitFilter
// ---------------------------------------------------------------------------

func TestToggleGitFilter_NoClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Without git client, toggle should be a no-op.
	assert.Nil(t, ft.gitClient)
	beforeFilter := ft.gitFilter

	ft.Update(keyMsg('g'))
	assert.Equal(t, beforeFilter, ft.gitFilter, "toggle without git client should be no-op")
}

func TestToggleGitFilter_EnableDisable(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}

	assert.False(t, ft.gitFilter)

	// Enable git filter.
	_, cmd := ft.Update(keyMsg('g'))
	assert.True(t, ft.gitFilter)
	assert.NotNil(t, cmd, "enabling git filter should return commands")

	// Disable git filter.
	ft.Update(keyMsg('g'))
	assert.False(t, ft.gitFilter)
	assert.Nil(t, ft.gitChangedPaths, "disabling should clear git paths")
	assert.Nil(t, ft.gitChangedDirs, "disabling should clear git dirs")
}

func TestToggleGitFilter_PreservesCursorPath(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}

	// Move cursor to a known file.
	ft.cursor = 2 // main.go in default sort
	originalPath := ft.cursorPath()
	assert.NotEmpty(t, originalPath)

	// Toggle on.
	ft.Update(keyMsg('g'))
	assert.True(t, ft.gitFilter)
	assert.Equal(t, originalPath, ft.savedCursorPath, "should save cursor path for async restore")
}

// ---------------------------------------------------------------------------
// Additional coverage: List mode toggle
// ---------------------------------------------------------------------------

func TestToggleListMode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	assert.False(t, ft.listMode)

	// Toggle to list mode.
	ft.Update(keyMsg('v'))
	assert.True(t, ft.listMode)

	// Toggle back to tree mode.
	ft.Update(keyMsg('v'))
	assert.False(t, ft.listMode)
}

// ---------------------------------------------------------------------------
// Additional coverage: TabActivated
// ---------------------------------------------------------------------------

func TestHandleTabActivated_GitTab(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}

	assert.False(t, ft.gitFilter)

	_, cmd := ft.Update(panels.TabActivatedMsg{PresetName: "git"})
	assert.True(t, ft.gitFilter, "switching to git tab should enable git filter")
	assert.NotNil(t, cmd, "should return commands for git filter setup")
}

func TestHandleTabActivated_ExplorerTab(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}

	// Enable git filter first.
	ft.gitFilter = true
	ft.gitChangedPaths = map[string]bool{"dummy": true}
	ft.gitChangedDirs = map[string]bool{"dummy": true}

	_, cmd := ft.Update(panels.TabActivatedMsg{PresetName: "explorer"})
	assert.False(t, ft.gitFilter, "switching to explorer tab should disable git filter")
	assert.Nil(t, ft.gitChangedPaths)
	assert.Nil(t, ft.gitChangedDirs)
	assert.NotNil(t, cmd)
}

func TestHandleTabActivated_AlreadyInGitMode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}
	ft.gitFilter = true

	// Already in git mode, receiving git tab activation should be no-op.
	_, cmd := ft.Update(panels.TabActivatedMsg{PresetName: "git"})
	assert.True(t, ft.gitFilter)
	assert.Nil(t, cmd, "already in git mode should be no-op")
}

// ---------------------------------------------------------------------------
// Additional coverage: GitChangedFilesMsg
// ---------------------------------------------------------------------------

func TestGitChangedFilesMsg_Handling(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.gitClient = &mockGitClient{}
	ft.gitFilter = true

	changedPaths := map[string]bool{
		filepath.Join(dir, "main.go"):       true,
		filepath.Join(dir, "src", "app.go"): true,
	}

	ft.Update(panels.GitChangedFilesMsg{Paths: changedPaths})

	assert.Equal(t, changedPaths, ft.gitChangedPaths)
	assert.NotNil(t, ft.gitChangedDirs)
	// Dirs containing changed files should be in gitChangedDirs.
	assert.True(t, ft.gitChangedDirs[filepath.Join(dir, "src")])
	assert.True(t, ft.gitChangedDirs[dir])
}

// ---------------------------------------------------------------------------
// Additional coverage: SetGitClient, Close, goToTop
// ---------------------------------------------------------------------------

func TestSetGitClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Nil(t, ft.gitClient)
	gc := &mockGitClient{}
	ft.SetGitClient(gc)
	assert.Equal(t, gc, ft.gitClient)
}

func TestClose_NilWatcher(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.watcher = nil
	ft.Close() // should not panic
}

func TestGoToTop(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 20

	// Move cursor down a few times first.
	ft.cursor = 3
	ft.goToTop()
	assert.Equal(t, 0, ft.cursor)
}

// ---------------------------------------------------------------------------
// Additional coverage: fileStatusIndicator
// ---------------------------------------------------------------------------

func TestFileStatusIndicator(t *testing.T) {
	tests := []struct {
		name     string
		status   git.FileStatus
		expected string
	}{
		{
			name:     "conflict in staged",
			status:   git.FileStatus{StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusUnmodified},
			expected: "U",
		},
		{
			name:     "conflict in worktree",
			status:   git.FileStatus{StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusConflict},
			expected: "U",
		},
		{
			name:     "staged modified",
			status:   git.FileStatus{StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			expected: "M",
		},
		{
			name:     "staged added",
			status:   git.FileStatus{StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusUnmodified},
			expected: "A",
		},
		{
			name:     "staged deleted",
			status:   git.FileStatus{StagedStatus: git.StatusDeleted, WorktreeStatus: git.StatusUnmodified},
			expected: "D",
		},
		{
			name:     "worktree modified (unstaged)",
			status:   git.FileStatus{StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
			expected: "M",
		},
		{
			name:     "untracked",
			status:   git.FileStatus{StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
			expected: "?",
		},
		{
			name:     "unmodified",
			status:   git.FileStatus{StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusUnmodified},
			expected: "",
		},
		{
			name:     "staged renamed",
			status:   git.FileStatus{StagedStatus: git.StatusRenamed, WorktreeStatus: git.StatusUnmodified},
			expected: "R",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, fileStatusIndicator(tc.status))
		})
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: gitStatusIcon
// ---------------------------------------------------------------------------

func TestGitStatusIcon_ASCIIMode(t *testing.T) {
	// In non-nerd mode, the status character is returned as-is.
	for _, status := range []string{"M", "A", "D", "?", "R", "C", "U"} {
		assert.Equal(t, status, gitStatusIcon(status, "ascii"))
	}
}

func TestGitStatusIcon_NerdMode(t *testing.T) {
	// In nerd mode, each status maps to a nerd font icon.
	statuses := []string{"M", "A", "D", "?", "R", "C", "U"}
	for _, status := range statuses {
		result := gitStatusIcon(status, "nerd")
		assert.NotEmpty(t, result, "nerd icon for %q should not be empty", status)
		// Nerd icons are different from the raw status char.
		assert.NotEqual(t, status, result, "nerd icon for %q should differ from raw char", status)
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: handleCommitSelected
// ---------------------------------------------------------------------------

func TestHandleCommitSelected_NoGitClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitClient = nil

	result, cmd := ft.Update(panels.CommitSelectedMsg{Hash: "abc1234567890", Subject: "Fix bug"})
	assert.Equal(t, ft, result)
	assert.Nil(t, cmd)
}

func TestHandleCommitSelected_WithGitClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.SetGitClient(&mockGitClient{})

	result, cmd := ft.Update(panels.CommitSelectedMsg{Hash: "abc1234567890", Subject: "Fix bug"})
	assert.Equal(t, ft, result)
	assert.NotNil(t, cmd, "should return async command to load commit files")
}

func TestHandleCommitSelected_ShortHash(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.SetGitClient(&mockGitClient{})

	// Short hash (<=7 chars) should be used as-is.
	_, cmd := ft.Update(panels.CommitSelectedMsg{Hash: "abc", Subject: ""})
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Additional coverage: normalizeVolume
// ---------------------------------------------------------------------------

func TestNormalizeVolume(t *testing.T) {
	// Same volume returns unchanged.
	assert.Equal(t, filepath.Join("C:", "foo"), normalizeVolume(filepath.Join("C:", "foo"), filepath.Join("C:", "bar")))
	// No volume returns unchanged.
	assert.Equal(t, "foo", normalizeVolume("foo", "bar"))
	// Empty reference volume returns unchanged.
	assert.Equal(t, filepath.Join("C:", "foo"), normalizeVolume(filepath.Join("C:", "foo"), "bar"))
}

// ---------------------------------------------------------------------------
// Additional coverage: View rendering, restoreCursorToPath, renderLine
// ---------------------------------------------------------------------------

func TestView_EmptyTree(t *testing.T) {
	dir := t.TempDir() // empty directory
	ft := newTestFT(t, defaultCfg(), dir)
	ft.height = 10

	view := ft.View(40, 10)
	assert.Contains(t, view, "Empty")
}

func TestView_ZeroSize(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "", ft.View(0, 10))
	assert.Equal(t, "", ft.View(10, 0))
	assert.Equal(t, "", ft.View(0, 0))
}

func TestView_NormalRendering(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 20

	view := ft.View(60, 20)
	assert.NotEmpty(t, view)
	// Should contain directory and file names.
	assert.Contains(t, view, "docs")
	assert.Contains(t, view, "main.go")
}

func TestRestoreCursorToPath_Empty(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	cursor := ft.cursor
	ft.restoreCursorToPath("")
	assert.Equal(t, cursor, ft.cursor, "empty path should not change cursor")
}

func TestRestoreCursorToPath_Found(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.height = 20

	// Move cursor to 0, then restore to main.go.
	ft.cursor = 0
	mainGoPath := filepath.Join(dir, "main.go")
	ft.restoreCursorToPath(mainGoPath)
	// cursor should now be at the main.go entry.
	if ft.cursor >= 0 && ft.cursor < len(ft.visible) {
		assert.Equal(t, mainGoPath, ft.visible[ft.cursor].path)
	}
}

func TestRestoreCursorToPath_NotFound(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.height = 20

	ft.cursor = 100 // intentionally out of range
	ft.restoreCursorToPath(filepath.Join(dir, "nonexistent.go"))
	// cursor should be clamped to valid range.
	assert.True(t, ft.cursor >= 0 && ft.cursor < len(ft.visible))
}

func TestRenderLine_WithGitStatus(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.SetGitClient(&mockGitClient{})

	// Inject git file status for main.go.
	mainGoPath := filepath.Join(dir, "main.go")
	ft.gitFileStatus = map[string]string{
		mainGoPath: "M",
	}

	// Find the main.go node.
	var mainNode *node
	for _, n := range ft.visible {
		if n.path == mainGoPath {
			mainNode = n
			break
		}
	}
	require.NotNil(t, mainNode, "main.go should be in visible list")

	line := ft.renderLine(mainNode, 60, true)
	assert.NotEmpty(t, line)
}

func TestRenderLine_ListMode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.listMode = true
	ft.rebuildVisible()

	if len(ft.visible) > 0 {
		line := ft.renderLine(ft.visible[0], 60, false)
		assert.NotEmpty(t, line)
	}
}

func TestRenderLine_Symlink(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Create a synthetic symlink node.
	symlinkNode := &node{
		name:          "link",
		path:          filepath.Join(dir, "link"),
		isDir:         false,
		isSymlink:     true,
		symlinkTarget: "/some/target",
	}

	line := ft.renderLine(symlinkNode, 60, false)
	assert.Contains(t, line, "→")
}

func TestRenderLine_DirWithError(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	errNode := &node{
		name:    "broken-dir",
		path:    filepath.Join(dir, "broken-dir"),
		isDir:   true,
		loadErr: os.ErrPermission,
	}

	line := ft.renderLine(errNode, 60, false)
	assert.Contains(t, line, "error")
}

// ---------------------------------------------------------------------------
// Additional coverage: gitFileStatusMsg, loadGitFileStatus
// ---------------------------------------------------------------------------

func TestGitFileStatusMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	status := map[string]string{
		filepath.Join(dir, "main.go"): "M",
	}
	ft.Update(gitFileStatusMsg{status: status})
	assert.Equal(t, status, ft.gitFileStatus)
}

func TestLoadGitFileStatus_NoClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitClient = nil

	cmd := ft.loadGitFileStatus()
	assert.Nil(t, cmd, "no git client should return nil cmd")
}

func TestLoadGitFileStatus_WithClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.SetGitClient(&mockGitClient{})

	cmd := ft.loadGitFileStatus()
	assert.NotNil(t, cmd, "with git client should return async command")
}

// ---------------------------------------------------------------------------
// Additional coverage: handleKey goToTop ('g' is toggleGitFilter,
// check 'home' for cursor-to-top if available)
// ---------------------------------------------------------------------------

func TestHandleKey_PreviewScroll(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	_, cmd := ft.Update(tea.KeyPressMsg{Code: 'J'})
	assert.NotNil(t, cmd, "J key should emit preview scroll down")

	_, cmd = ft.Update(tea.KeyPressMsg{Code: 'K'})
	assert.NotNil(t, cmd, "K key should emit preview scroll up")
}

func TestHandleKey_ToggleListMode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	assert.False(t, ft.listMode)
	ft.Update(tea.KeyPressMsg{Code: 'v'})
	assert.True(t, ft.listMode)
	ft.Update(tea.KeyPressMsg{Code: 'v'})
	assert.False(t, ft.listMode)
}

func TestHandleKey_CommitFilesMode_Escape(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.commitFilesMode = true

	ft.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, ft.commitFilesMode, "escape should exit commit files mode")
}

func TestHandleKey_Unfocused_Ignored(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = false

	cursor := ft.cursor
	ft.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, cursor, ft.cursor, "unfocused should ignore keys")
}

// ---------------------------------------------------------------------------
// Additional: visibleName, visiblePath, cursorPath edge cases
// ---------------------------------------------------------------------------

func TestVisibleName_OutOfBounds(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "", ft.visibleName(-1))
	assert.Equal(t, "", ft.visibleName(9999))
	assert.NotEmpty(t, ft.visibleName(0))
}

func TestVisiblePath_OutOfBounds(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "", ft.visiblePath(-1))
	assert.Equal(t, "", ft.visiblePath(9999))
	assert.NotEmpty(t, ft.visiblePath(0))
}

func TestCursorPath_Empty(t *testing.T) {
	dir := t.TempDir() // empty directory
	ft := newTestFT(t, defaultCfg(), dir)

	assert.Equal(t, "", ft.cursorPath(), "empty tree should return empty cursor path")
}

func TestCursorPath_Valid(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	path := ft.cursorPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, dir)
}

func TestClose_WithWatcher(t *testing.T) {
	dir := createTestTree(t)
	cfg := defaultCfg()
	ft := New(cfg, dir)
	ft.ctx = context.Background()
	ft.watcher = newWatcher(defaultDebounce, defaultPollInterval)
	ft.watcher.addDir(dir)

	ft.Close() // should stop the watcher without panic
}

func TestLoadGitChangedFiles_WithMockClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.SetGitClient(&mockGitClient{})

	cmd := ft.loadGitChangedFiles()
	require.NotNil(t, cmd)
	// Execute the command to cover the inner function.
	msg := cmd()
	assert.NotNil(t, msg)
}

func TestHandleKey_GoToBottom(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 20

	ft.Update(tea.KeyPressMsg{Code: 'G'})
	assert.Equal(t, len(ft.visible)-1, ft.cursor)
}

func TestHandleKey_ToggleHidden(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 20

	beforeCount := ft.visibleCount()
	ft.Update(tea.KeyPressMsg{Code: '.'})
	// After toggling, .hidden should appear and count should increase.
	assert.Greater(t, ft.visibleCount(), beforeCount)
}

func TestCommitDeselectedMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.commitFilesMode = true

	ft.Update(panels.CommitDeselectedMsg{})
	assert.False(t, ft.commitFilesMode, "should exit commit files mode")
}

// ---------------------------------------------------------------------------
// Git-ignored path tests
// ---------------------------------------------------------------------------

func TestSetGitClient_SetsIgnoreChecker(t *testing.T) {
	ft := New(defaultCfg(), t.TempDir())
	mock := &mockGitClientWithIgnore{repoRoot: t.TempDir()}
	ft.SetGitClient(mock)

	assert.NotNil(t, ft.ignoreChecker, "ignoreChecker should be set via type assertion")
}

func TestSetGitClient_NoIgnoreChecker(t *testing.T) {
	ft := New(defaultCfg(), t.TempDir())
	mock := &mockGitClient{}
	ft.SetGitClient(mock)

	assert.Nil(t, ft.ignoreChecker, "ignoreChecker should be nil for plain StatusReader")
}

func TestIsPathIgnored_DirectMatch(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.gitIgnoredPaths = map[string]bool{
		filepath.Join(dir, "main.go"): true,
	}

	assert.True(t, ft.isPathIgnored(filepath.Join(dir, "main.go")))
	assert.False(t, ft.isPathIgnored(filepath.Join(dir, "README.md")))
}

func TestIsPathIgnored_AncestorDir(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.gitIgnoredPaths = map[string]bool{
		filepath.Join(dir, "docs"): true,
	}

	assert.True(t, ft.isPathIgnored(filepath.Join(dir, "docs")),
		"the ignored directory itself should be ignored")
	assert.True(t, ft.isPathIgnored(filepath.Join(dir, "docs", "guide.md")),
		"file inside ignored directory should be ignored")
	assert.False(t, ft.isPathIgnored(filepath.Join(dir, "src", "app.go")),
		"file not under ignored directory should not be ignored")
}

func TestIsPathIgnored_NilMap(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.gitIgnoredPaths = nil
	assert.False(t, ft.isPathIgnored(filepath.Join(dir, "main.go")))
}

func TestIsPathIgnored_EmptyMap(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.gitIgnoredPaths = map[string]bool{}
	assert.False(t, ft.isPathIgnored(filepath.Join(dir, "main.go")))
}

func TestLoadGitIgnored_NilChecker(t *testing.T) {
	ft := New(defaultCfg(), t.TempDir())
	ft.ignoreChecker = nil

	cmd := ft.loadGitIgnored()
	assert.Nil(t, cmd, "loadGitIgnored should return nil when ignoreChecker is nil")
}

func TestLoadGitIgnored_ProducesMsg(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()

	mock := &mockGitClientWithIgnore{
		ignoredPaths: []string{"docs/", "main.go"},
		repoRoot:     dir,
	}
	ft.SetGitClient(mock)

	cmd := ft.loadGitIgnored()
	require.NotNil(t, cmd)

	msg := cmd()
	igMsg, ok := msg.(gitIgnoredMsg)
	require.True(t, ok, "expected gitIgnoredMsg, got %T", msg)
	assert.True(t, igMsg.paths[filepath.Join(dir, "docs")])
	assert.True(t, igMsg.paths[filepath.Join(dir, "main.go")])
}

func TestRenderLine_IgnoredFileDimmed(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.width = 60
	ft.height = 20
	ft.focused = true

	// Set ignored paths.
	ft.gitIgnoredPaths = map[string]bool{
		filepath.Join(dir, "main.go"): true,
	}

	// Find the main.go node.
	var mainNode *node
	for _, n := range ft.visible {
		if n.name == "main.go" {
			mainNode = n
			break
		}
	}
	require.NotNil(t, mainNode, "main.go should be visible")

	line := ft.renderLine(mainNode, 60, false)

	// The line should contain the dim color (#666666) and the ignored
	// indicator "!".
	assert.Contains(t, line, "!", "ignored file should show ! indicator")
}

func TestRenderLine_IgnoredDirDimmed(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.width = 60
	ft.height = 20
	ft.focused = true

	// Set ignored paths.
	ft.gitIgnoredPaths = map[string]bool{
		filepath.Join(dir, "docs"): true,
	}

	// Find the docs node.
	var docsNode *node
	for _, n := range ft.visible {
		if n.name == "docs" {
			docsNode = n
			break
		}
	}
	require.NotNil(t, docsNode, "docs should be visible")

	line := ft.renderLine(docsNode, 60, false)

	// Ignored dirs are dimmed (foreground color changed to Dim).
	assert.Contains(t, line, "!", "ignored dir should show ! indicator")
}

func TestGitStatusIcon_IgnoredAscii(t *testing.T) {
	assert.Equal(t, "!", gitStatusIcon("!", "ascii"))
}

func TestGitStatusIcon_IgnoredNerd(t *testing.T) {
	result := gitStatusIcon("!", "nerd")
	assert.NotEmpty(t, result)
	assert.NotEqual(t, "!", result, "nerd icon for ! should differ from raw char")
}

func TestGitIgnoredMsg_HandledInUpdate(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ignored := map[string]bool{
		filepath.Join(dir, "main.go"): true,
	}
	ft.Update(gitIgnoredMsg{paths: ignored})
	assert.Equal(t, ignored, ft.gitIgnoredPaths)
}

// ---------------------------------------------------------------------------
// Right-click tests
// ---------------------------------------------------------------------------

func TestRightClickShowsActionPicker(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	// Find first file index.
	fileIdx := -1
	for i, n := range ft.visible {
		if !n.isDir {
			fileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, fileIdx, 0)

	_, cmd := ft.Update(panels.PanelMouseRightClickMsg{ContentRow: fileIdx, ContentCol: 5})
	require.NotNil(t, cmd, "right-click on file should produce a command")

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
}

func TestRightClickOutOfBounds(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true

	_, cmd := ft.Update(panels.PanelMouseRightClickMsg{ContentRow: 100, ContentCol: 5})
	assert.Nil(t, cmd, "right-click out of bounds should return nil cmd")
}

// ---------------------------------------------------------------------------
// Title dirty indicator (*)
// ---------------------------------------------------------------------------

func TestTitle_DirtyIndicator(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitFileStatus = map[string]string{
		filepath.Join(dir, "file.go"): "M",
	}
	assert.Equal(t, "Files*", ft.Title())
}

func TestTitle_DirtyIndicatorMultipleFiles(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitFileStatus = map[string]string{
		filepath.Join(dir, "a.go"): "M",
		filepath.Join(dir, "b.go"): "A",
		filepath.Join(dir, "c.go"): "?",
	}
	title := ft.Title()
	assert.Equal(t, "Files*", title)
	assert.Equal(t, 1, strings.Count(title, "*"), "should have exactly one asterisk")
}

func TestTitle_CleanAfterDirty(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	ft.gitFileStatus = map[string]string{filepath.Join(dir, "file.go"): "M"}
	assert.Equal(t, "Files*", ft.Title())

	ft.gitFileStatus = map[string]string{}
	assert.Equal(t, "Files", ft.Title())
}

func TestTitle_NilStatusIsClean(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitFileStatus = nil

	assert.Equal(t, "Files", ft.Title())
}

func TestTitle_GitFilterTakesPrecedenceOverDirty(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.gitFilter = true
	ft.gitFileStatus = map[string]string{filepath.Join(dir, "file.go"): "M"}

	assert.Equal(t, "Files (git changed)", ft.Title())
}

func TestTitle_CommitModeTakesPrecedenceOverDirty(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.commitFilesMode = true
	ft.commitLabel = "abc1234"
	ft.gitFileStatus = map[string]string{filepath.Join(dir, "file.go"): "M"}

	assert.Equal(t, "Files: abc1234", ft.Title())
}

func TestTitle_PRModeTakesPrecedenceOverDirty(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.prFilesMode = true
	ft.prLabel = "PR #42"
	ft.gitFileStatus = map[string]string{filepath.Join(dir, "file.go"): "M"}

	assert.Equal(t, "Files: PR #42", ft.Title())
}

func TestGitStatusChangedMsg_ReloadsFileStatus(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = context.Background()
	ft.gitClient = &mockGitClient{}

	updated, cmd := ft.Update(panels.GitStatusChangedMsg{})
	assert.Equal(t, ft, updated, "panel should be returned unchanged")
	assert.NotNil(t, cmd, "handler should return async command to reload git status")
}

func TestGitStatusChangedMsg_NoClient(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	updated, cmd := ft.Update(panels.GitStatusChangedMsg{})
	assert.Equal(t, ft, updated, "panel should be returned unchanged")
	assert.Nil(t, cmd, "without git client, no command should be returned")
}

// ---------------------------------------------------------------------------
// inferIgnoredDirs tests
// ---------------------------------------------------------------------------

func TestInferIgnoredDirs_AllChildrenIgnored(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "a.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(sub, "b.txt"), nil, 0o644)

	paths := map[string]bool{
		filepath.Join(sub, "a.txt"): true,
		filepath.Join(sub, "b.txt"): true,
	}
	inferIgnoredDirs(paths)

	assert.True(t, paths[sub], "parent dir should be promoted to ignored")
}

func TestInferIgnoredDirs_PartialChildren(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "a.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(sub, "b.txt"), nil, 0o644)

	paths := map[string]bool{
		filepath.Join(sub, "a.txt"): true,
		// b.txt NOT ignored
	}
	inferIgnoredDirs(paths)

	assert.False(t, paths[sub], "parent should NOT be promoted when not all children are ignored")
}

func TestInferIgnoredDirs_Nested(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	child := filepath.Join(parent, "child")
	os.MkdirAll(child, 0o755)
	os.WriteFile(filepath.Join(child, "x.txt"), nil, 0o644)

	paths := map[string]bool{
		filepath.Join(child, "x.txt"): true,
	}
	inferIgnoredDirs(paths)

	assert.True(t, paths[child], "child dir should be promoted")
	assert.True(t, paths[parent], "parent dir should be promoted (child is its only entry)")
}

// ---------------------------------------------------------------------------
// Page navigation (d / u)
// ---------------------------------------------------------------------------

func TestPageDown(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 3 // small viewport to make page size meaningful

	assert.Equal(t, 0, ft.cursor)

	// Press 'd' to page down.
	ft.Update(keyMsg('d'))
	assert.Equal(t, 3, ft.cursor, "cursor should move down by viewport height")
}

func TestPageUp(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 3

	// Move cursor to the bottom first.
	ft.cursor = ft.visibleCount() - 1

	// Press 'u' to page up.
	ft.Update(keyMsg('u'))
	expected := ft.visibleCount() - 1 - 3
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, ft.cursor, "cursor should move up by viewport height")
}

func TestPageDown_ClampsAtEnd(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 100 // larger than total items

	ft.Update(keyMsg('d'))
	assert.Equal(t, ft.visibleCount()-1, ft.cursor,
		"page down beyond end should clamp to last item")
}

func TestPageUp_ClampsAtZero(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.focused = true
	ft.height = 3

	ft.cursor = 1 // near the top
	ft.Update(keyMsg('u'))
	assert.Equal(t, 0, ft.cursor, "page up past beginning should clamp to 0")
}
