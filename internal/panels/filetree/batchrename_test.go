package filetree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBatchRename(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.txt"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.txt"), []byte("other"), 0o644))

	tests := []struct {
		name    string
		oldRel  []string
		newRel  []string
		wantErr string
	}{
		{
			name:    "valid",
			oldRel:  []string{"old.txt"},
			newRel:  []string{"renamed.txt"},
			wantErr: "",
		},
		{
			name:    "duplicate destination",
			oldRel:  []string{"old.txt", "other.txt"},
			newRel:  []string{"same.txt", "same.txt"},
			wantErr: "duplicate destination: same.txt",
		},
		{
			name:    "outside root absolute",
			oldRel:  []string{"old.txt"},
			newRel:  []string{filepath.Join(filepath.Dir(root), "escape.txt")},
			wantErr: "name outside repo root:",
		},
		{
			name:    "empty name",
			oldRel:  []string{"old.txt"},
			newRel:  []string{"  "},
			wantErr: "empty name on line 1",
		},
		{
			name:    "parent traversal",
			oldRel:  []string{"old.txt"},
			newRel:  []string{"dir/../renamed.txt"},
			wantErr: "name outside repo root: dir/../renamed.txt",
		},
		{
			name:    "line count mismatch",
			oldRel:  []string{"old.txt"},
			newRel:  []string{"renamed.txt", "extra.txt"},
			wantErr: "line count changed: expected 1, got 2",
		},
		{
			name:    "existing unrelated destination",
			oldRel:  []string{"old.txt"},
			newRel:  []string{"other.txt"},
			wantErr: "destination already exists: other.txt",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBatchRename(root, tt.oldRel, tt.newRel)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRequestBatchRenamePrefillsSelectedPaths(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.selected[filepath.Join(root, "main.go")] = true
	ft.selected[filepath.Join(root, "README.md")] = true

	_, cmd := ft.requestBatchRename()
	require.NotNil(t, cmd)

	assert.True(t, ft.batchRename.enabled)
	assert.Equal(t, []string{"README.md", "main.go"}, ft.batchRename.oldRel)
	assert.Equal(t, "README.md\nmain.go", ft.batchRename.editor.Value())
}

func TestRequestBatchRenamePrefillsFocusedPath(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.viewport.cursor = indexOfVisiblePath(t, ft, filepath.Join(root, "README.md"))

	_, cmd := ft.requestBatchRename()
	require.NotNil(t, cmd)

	assert.True(t, ft.batchRename.enabled)
	assert.Equal(t, []string{"README.md"}, ft.batchRename.oldRel)
	assert.Equal(t, "README.md", ft.batchRename.editor.Value())
}

func TestBatchRenameEscCancels(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.Focus()
	_, _ = ft.requestBatchRename()
	ft.batchRename.editor.SetValue("renamed.md")

	_, cmd := ft.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	require.Nil(t, cmd)
	assert.False(t, ft.batchRename.enabled)
	assert.FileExists(t, filepath.Join(root, "README.md"))
	assert.NoFileExists(t, filepath.Join(root, "renamed.md"))
}

func TestBatchRenameSuccessfulRenameRefreshesTree(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.Focus()
	_, _ = ft.requestBatchRename()
	ft.batchRename.oldRel = []string{"README.md"}
	ft.batchRename.editor.SetValue("renamed.md")

	_, cmd := ft.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)
	msg, ok := cmd().(batchRenameResultMsg)
	require.True(t, ok)
	assert.Equal(t, 1, msg.renamed)
	require.Empty(t, msg.errs)

	_, resultCmd := ft.Update(msg)
	require.NotNil(t, resultCmd)
	applyFileTreeCmd(t, ft, resultCmd)

	assert.NoFileExists(t, filepath.Join(root, "README.md"))
	assert.FileExists(t, filepath.Join(root, "renamed.md"))
	assert.NotEqual(t, -1, findVisiblePath(ft, filepath.Join(root, "renamed.md")))
}

func TestApplyBatchRenamesReportsPartialFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644))
	ft := newTestFT(t, defaultCfg(), root)

	msg := applyBatchRenames(
		context.Background(),
		root,
		[]string{"a.txt", "missing.txt"},
		[]string{"renamed.txt", "missing-renamed.txt"},
	)
	assert.Equal(t, 1, msg.renamed)
	require.Len(t, msg.errs, 1)
	assert.Contains(t, msg.errs[0], "missing.txt")
	assert.FileExists(t, filepath.Join(root, "renamed.txt"))

	_, cmd := ft.handleBatchRenameResult(msg)
	toast := findToast(t, cmd)
	require.NotNil(t, toast)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Renamed 1, 1 failed:")
	assert.Contains(t, toast.Message, "missing.txt")
}

func indexOfVisiblePath(t *testing.T, ft *FileTree, path string) int {
	t.Helper()
	idx := findVisiblePath(ft, path)
	require.NotEqual(t, -1, idx)
	return idx
}

func findVisiblePath(ft *FileTree, path string) int {
	for i, n := range ft.visible {
		if n.path == path {
			return i
		}
	}
	return -1
}

func findToast(t *testing.T, cmd tea.Cmd) *notify.ShowToastMsg {
	t.Helper()
	require.NotNil(t, cmd)

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok)
	for _, batched := range batch {
		if batched == nil {
			continue
		}
		if toast, ok := batched().(notify.ShowToastMsg); ok && strings.HasPrefix(toast.Message, "Renamed") {
			return &toast
		}
	}
	return nil
}
