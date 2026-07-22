package filetree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

const (
	actionUndoDelete = "undo_delete"
	trashDirName     = "trash"
	manifestFileName = "manifest.json"
	repoHashLength   = 16
)

type trashEntry struct {
	DeletedAt    time.Time `json:"deleted_at"`
	OriginalPath string    `json:"original_path"`
	TrashedPath  string    `json:"trashed_path"`
	IsDir        bool      `json:"is_dir"`
}

type undoDeleteResultMsg struct {
	entry trashEntry
	err   string
}

func trashRoot() string {
	return filepath.Join(config.DataDir(), trashDirName)
}

func repoTrashDir(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(trashRoot(), hex.EncodeToString(sum[:])[:repoHashLength])
}

func manifestPath(root string) string {
	return filepath.Join(repoTrashDir(root), manifestFileName)
}

func trashDestinationPreview(root, path string) string {
	return filepath.Join(repoTrashDir(root), fmt.Sprintf("<timestamp>-%s", filepath.Base(path)))
}

func trashFile(ctx context.Context, root, path string) (trashEntry, error) {
	if err := ctx.Err(); err != nil {
		return trashEntry{}, err
	}
	if !isWithinRoot(root, path) {
		return trashEntry{}, fmt.Errorf("path %q is outside root", path)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return trashEntry{}, nil
	}
	if err != nil {
		return trashEntry{}, fmt.Errorf("stat %q: %w", path, err)
	}
	dir := repoTrashDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return trashEntry{}, fmt.Errorf("create trash %q: %w", dir, err)
	}
	deletedAt := time.Now()
	dst := uniqueTrashPath(dir, deletedAt.UnixNano(), filepath.Base(path))
	if err := movePath(path, dst, info); err != nil {
		return trashEntry{}, fmt.Errorf("trash %q → %q: %w", path, dst, err)
	}
	entry := trashEntry{
		OriginalPath: path,
		TrashedPath:  dst,
		DeletedAt:    deletedAt,
		IsDir:        info.IsDir(),
	}
	entries, err := readTrashManifest(root)
	if err != nil {
		return trashEntry{}, err
	}
	entries = append(entries, entry)
	if err := writeTrashManifest(root, entries); err != nil {
		return trashEntry{}, err
	}
	return entry, nil
}

func uniqueTrashPath(dir string, unixNano int64, base string) string {
	candidate := filepath.Join(dir, fmt.Sprintf("%d-%s", unixNano, base))
	for suffix := 1; ; suffix++ {
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%d-%d-%s", unixNano, suffix, base))
	}
}

func restoreLatestTrashed(ctx context.Context, root string) (trashEntry, error) {
	if err := ctx.Err(); err != nil {
		return trashEntry{}, err
	}
	entries, err := readTrashManifest(root)
	if err != nil {
		return trashEntry{}, err
	}
	if len(entries) == 0 {
		return trashEntry{}, errors.New("trash is empty")
	}
	entry := entries[len(entries)-1]
	if !isWithinRoot(root, entry.OriginalPath) {
		return trashEntry{}, fmt.Errorf("restore target %q is outside root", entry.OriginalPath)
	}
	if _, err := os.Stat(entry.OriginalPath); err == nil {
		return trashEntry{}, fmt.Errorf("restore target already exists: %s", entry.OriginalPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return trashEntry{}, fmt.Errorf("stat restore target %q: %w", entry.OriginalPath, err)
	}
	parent := filepath.Dir(entry.OriginalPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return trashEntry{}, fmt.Errorf("create restore parent %q: %w", parent, err)
	}
	info, err := os.Lstat(entry.TrashedPath)
	if err != nil {
		return trashEntry{}, fmt.Errorf("stat trashed item %q: %w", entry.TrashedPath, err)
	}
	if err := movePath(entry.TrashedPath, entry.OriginalPath, info); err != nil {
		return trashEntry{}, fmt.Errorf("restore %q → %q: %w", entry.TrashedPath, entry.OriginalPath, err)
	}
	if err := writeTrashManifest(root, entries[:len(entries)-1]); err != nil {
		return trashEntry{}, err
	}
	return entry, nil
}

func readTrashManifest(root string) ([]trashEntry, error) {
	data, err := os.ReadFile(manifestPath(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trash manifest: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var entries []trashEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse trash manifest: %w", err)
	}
	return entries, nil
}

func writeTrashManifest(root string, entries []trashEntry) error {
	dir := repoTrashDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create trash %q: %w", dir, err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trash manifest: %w", err)
	}
	if len(data) > 0 {
		data = append(data, '\n')
	}
	if err := os.WriteFile(manifestPath(root), data, 0o644); err != nil {
		return fmt.Errorf("write trash manifest: %w", err)
	}
	return nil
}

func movePath(src, dst string, info os.FileInfo) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if info.IsDir() {
		if err := copyDirRaw(src, dst, info.Mode()); err != nil {
			return err
		}
	} else if err := copyFileRaw(src, dst, info.Mode()); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyFileRaw(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = out.Close()
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	closed = true
	return out.Close()
}

func copyDirRaw(src, dst string, mode fs.FileMode) error {
	if err := os.Mkdir(dst, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcChild := filepath.Join(src, entry.Name())
		dstChild := filepath.Join(dst, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyDirRaw(srcChild, dstChild, info.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := copyFileRaw(srcChild, dstChild, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func (ft *FileTree) requestUndoDelete() (panels.Panel, tea.Cmd) {
	rootPath := ft.rootPath
	ctx := ft.safeCtx()
	return ft, func() tea.Msg {
		entry, err := restoreLatestTrashed(ctx, rootPath)
		if err != nil {
			return undoDeleteResultMsg{err: err.Error()}
		}
		return undoDeleteResultMsg{entry: entry}
	}
}

func (ft *FileTree) handleUndoDeleteResult(msg undoDeleteResultMsg) (panels.Panel, tea.Cmd) {
	ft.reloadTree()
	if msg.err != "" {
		errMsg := msg.err
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot restore: " + errMsg, Level: notify.Error}
		}
	}
	name := filepath.Base(msg.entry.OriginalPath)
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Restored " + name, Level: notify.Success}
	}
}
