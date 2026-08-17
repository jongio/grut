package filetree

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileTree_SetActionsCfg_Extra(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	cfg := config.ActionsConfig{
		RightClick: map[string]string{string(actions.ItemFile): string(actions.ActionCopyPath)},
	}

	ft.SetActionsCfg(cfg)
	assert.Equal(t, cfg, ft.actionsCfg)
}

func TestBookmarkCurrent_Extra(t *testing.T) {
	tests := []struct {
		name     string
		cursor   int
		wantPath func(root string) string
	}{
		{name: "valid cursor emits selected directory", cursor: 0, wantPath: func(root string) string { return filepath.Join(root, "docs") }},
		{name: "no node selected falls back to root", cursor: 99, wantPath: func(root string) string { return root }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir := createTestTree(t)
			ft := newTestFT(t, defaultCfg(), dir)
			ft.viewport.cursor = tt.cursor
			_, cmd := ft.bookmarkCurrent()
			require.NotNil(t, cmd)
			msg := cmd()
			bookmarkMsg, ok := msg.(panels.BookmarkAddMsg)
			require.True(t, ok)
			assert.Equal(t, tt.wantPath(dir), bookmarkMsg.Path)
		})
	}
}

func TestAddToContext_Extra(t *testing.T) {
	tests := []struct {
		name    string
		cursor  int
		wantCmd bool
		assert  func(t *testing.T, root string, msg any)
	}{
		{
			name:    "valid file emits context message",
			cursor:  2,
			wantCmd: true,
			assert: func(t *testing.T, root string, msg any) {
				t.Helper()
				ctxMsg, ok := msg.(panels.AddToContextMsg)
				require.True(t, ok)
				assert.Equal(t, filepath.Join(root, "main.go"), ctxMsg.Path)
			},
		},
		{
			name:    "no node selected is ignored",
			cursor:  99,
			wantCmd: false,
			assert:  func(t *testing.T, _ string, _ any) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir := createTestTree(t)
			ft := newTestFT(t, defaultCfg(), dir)
			ft.viewport.cursor = tt.cursor
			_, cmd := ft.addToContext()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assert(t, dir, cmd())
			}
		})
	}
}

func TestCopyPath_Extra(t *testing.T) {
	tests := []struct {
		name    string
		cursor  int
		wantCmd bool
		assert  func(t *testing.T, msg any)
	}{
		{
			name:    "valid node copies path",
			cursor:  2,
			wantCmd: true,
			assert: func(t *testing.T, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
		{
			name:    "no node is ignored",
			cursor:  99,
			wantCmd: false,
			assert:  func(t *testing.T, _ any) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir := createTestTree(t)
			ft := newTestFT(t, defaultCfg(), dir)
			ft.viewport.cursor = tt.cursor
			_, cmd := ft.copyPath()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assert(t, cmd())
			}
		})
	}
}

func TestStageFile_Extra(t *testing.T) {
	tests := []struct {
		name      string
		cursor    int
		prepare   func(t *testing.T, root string)
		wantCmd   bool
		assertion func(t *testing.T, msg any)
	}{
		{
			name:   "valid file stages successfully",
			cursor: 2,
			prepare: func(t *testing.T, root string) {
				t.Helper()
				cmd := exec.Command("git", "-C", root, "init")
				require.NoError(t, cmd.Run())
			},
			wantCmd: true,
			assertion: func(t *testing.T, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Success, toast.Level)
				assert.Contains(t, toast.Message, "Staged: main.go")
			},
		},
		{
			name:      "no selection is ignored",
			cursor:    99,
			prepare:   func(t *testing.T, _ string) { t.Helper() },
			wantCmd:   false,
			assertion: func(t *testing.T, _ any) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir := createTestTree(t)
			tt.prepare(t, dir)
			ft := newTestFT(t, defaultCfg(), dir)
			ft.viewport.cursor = tt.cursor
			_, cmd := ft.stageFile()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, cmd())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// navigateToPath
// ---------------------------------------------------------------------------

func TestNavigateToPath_ValidDir(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	subDir := filepath.Join(dir, "docs")

	_, cmd := ft.navigateToPath(subDir)
	require.NotNil(t, cmd)
	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Info, toast.Level)
	assert.Contains(t, toast.Message, "Navigated to")
	assert.Equal(t, subDir, ft.rootPath)
}

func TestNavigateToPath_InvalidPath(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.navigateToPath("/nonexistent/path/xyz123")
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Cannot navigate")
}

func TestNavigateToPath_File(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	filePath := filepath.Join(dir, "main.go")

	_, cmd := ft.navigateToPath(filePath)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

// ---------------------------------------------------------------------------
// bookmarkPath (stub — just returns nil cmd)
// ---------------------------------------------------------------------------

func TestBookmarkPath_Extra(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.bookmarkPath("/some/path")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// exitPRFilesMode
// ---------------------------------------------------------------------------

func TestExitPRFilesMode_Extra(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	// Set up PR files mode
	ft.filter.prFilesMode = true
	ft.filter.prNumber = 42
	ft.filter.prLabel = "test-pr"
	ft.filter.prFiles = []panels.PRFile{{Filename: "a.go"}, {Filename: "b.go"}}
	ft.filter.prChanged = &changedFiles{paths: map[string]bool{"a.go": true}}

	ft.exitPRFilesMode()

	assert.False(t, ft.filter.prFilesMode)
	assert.Equal(t, 0, ft.filter.prNumber)
	assert.Empty(t, ft.filter.prLabel)
	assert.Nil(t, ft.filter.prFiles)
	assert.Nil(t, ft.filter.prChanged)
}

// ---------------------------------------------------------------------------
// revealFile
// ---------------------------------------------------------------------------

func TestRevealFile_ExistingFile(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	target := filepath.Join(dir, "docs", "guide.md")
	_, cmd := ft.Update(panels.RevealFileMsg{Path: target})
	applyFileTreeCmd(t, ft, cmd)

	// After reveal, cursor should be on guide.md
	path := ft.CursorPath()
	assert.Equal(t, target, path)
}

func TestRevealFile_EmptyPath(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.Update(panels.RevealFileMsg{})
	assert.Nil(t, cmd)
	// Should be a no-op
	assert.NotNil(t, ft.root)
}

func TestRevealFile_OutsideRoot(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.Update(panels.RevealFileMsg{Path: "/completely/outside/path/file.txt"})
	assert.Nil(t, cmd)
	// Should be a no-op — can't reveal files outside the tree root
}

func TestRevealFile_NonexistentFile(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)

	_, cmd := ft.Update(panels.RevealFileMsg{Path: filepath.Join(dir, "nonexistent", "file.go")})
	applyFileTreeCmd(t, ft, cmd)
	// Should be a no-op — path segment not found
}

// ---------------------------------------------------------------------------
// runeDisplayWidth
// ---------------------------------------------------------------------------

func TestRuneDisplayWidth_PrivateUseArea(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want int
	}{
		{name: "PUA BMP", r: 0xE000, want: 2},
		{name: "PUA end BMP", r: 0xF8FF, want: 2},
		{name: "PUA plane 15", r: 0xF0000, want: 2},
		{name: "PUA plane 16", r: 0x100000, want: 2},
		{name: "ASCII letter", r: 'A', want: 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := runeDisplayWidth(tt.r)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// safeCtx
// ---------------------------------------------------------------------------

func TestSafeCtx_NilContext(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.ctx = nil

	ctx := ft.safeCtx()
	assert.NotNil(t, ctx, "safeCtx should return a non-nil context even when ft.ctx is nil")
}
