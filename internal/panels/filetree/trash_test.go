package filetree

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withTestDataHome(t *testing.T) string {
	t.Helper()

	orig := xdg.DataHome
	xdg.DataHome = t.TempDir()
	t.Cleanup(func() { xdg.DataHome = orig })
	return xdg.DataHome
}

func TestTrashDeleteFileMovesToTrashAndManifest(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte("safe"), 0o644))

	err := deleteFile(context.Background(), root, path, false)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	entries, err := readTrashManifest(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, path, entries[0].OriginalPath)
	assert.False(t, entries[0].IsDir)
	assert.True(t, strings.HasPrefix(entries[0].TrashedPath, repoTrashDir(root)))
	got, err := os.ReadFile(entries[0].TrashedPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("safe"), got)
}

func TestTrashDeleteDirectoryMovesWholeDirectoryToTrash(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "guide.md"), []byte("guide"), 0o644))

	err := deleteFile(context.Background(), root, dir, false)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	entries, err := readTrashManifest(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].IsDir)
	got, err := os.ReadFile(filepath.Join(entries[0].TrashedPath, "nested", "guide.md"))
	require.NoError(t, err)
	assert.Equal(t, []byte("guide"), got)
}

func TestRestoreLatestTrashedRestoresAndRemovesManifestEntry(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "restore.txt")
	require.NoError(t, os.WriteFile(path, []byte("back"), 0o644))
	require.NoError(t, deleteFile(context.Background(), root, path, false))

	entry, err := restoreLatestTrashed(context.Background(), root)
	require.NoError(t, err)

	assert.Equal(t, path, entry.OriginalPath)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("back"), got)
	entries, err := readTrashManifest(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
	_, err = os.Stat(entry.TrashedPath)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestRestoreLatestTrashedConflictKeepsManifestEntry(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "conflict.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(t, deleteFile(context.Background(), root, path, false))
	require.NoError(t, os.WriteFile(path, []byte("new"), 0o644))

	_, err := restoreLatestTrashed(context.Background(), root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "restore target already exists")

	entries, err := readTrashManifest(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	_, err = os.Stat(entries[0].TrashedPath)
	assert.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)
}

func TestDeleteFilePermanentDeleteSkipsTrash(t *testing.T) {
	dataHome := withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "gone.txt")
	require.NoError(t, os.WriteFile(path, []byte("gone"), 0o644))

	err := deleteFile(context.Background(), root, path, true)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	_, err = os.Stat(filepath.Join(dataHome, "grut", trashDirName))
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestDeleteConfirmMessageShowsTrashAndTracking(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "new.txt")
	ft := New(defaultCfg(), root, nil)
	ft.gitFileStatus = map[string]string{path: "?"}

	msg := ft.deleteConfirmMessage(path)

	assert.Contains(t, msg, "Original: "+path)
	assert.Contains(t, msg, "Git: untracked")
	assert.Contains(t, msg, "Trash: "+repoTrashDir(root))
}

func TestUndoDeleteRequestReturnsToastResult(t *testing.T) {
	withTestDataHome(t)
	root := t.TempDir()
	path := filepath.Join(root, "undo.txt")
	require.NoError(t, os.WriteFile(path, []byte("undo"), 0o644))
	require.NoError(t, deleteFile(context.Background(), root, path, false))
	ft := New(defaultCfg(), root, nil)

	_, cmd := ft.requestUndoDelete()
	require.NotNil(t, cmd)
	result, ok := cmd().(undoDeleteResultMsg)
	require.True(t, ok)
	panel, toastCmd := ft.handleUndoDeleteResult(result)
	require.IsType(t, ft, panel)
	require.NotNil(t, toastCmd)
	toast, ok := runCmd(t, ft, toastCmd).(notify.ShowToastMsg)
	require.True(t, ok)

	assert.Equal(t, notify.Success, toast.Level)
	assert.Contains(t, toast.Message, "Restored undo.txt")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("undo"), got)
}
