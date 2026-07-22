package filetree

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

const (
	bytesPerKiB   = 1024
	bytesPerMiB   = bytesPerKiB * 1024
	bytesPerGiB   = bytesPerMiB * 1024
	gitDirName    = ".git"
	actionDirSize = "dir_size"
)

type dirSizeResultMsg struct {
	err   error
	root  string
	sizes map[string]int64
}

func (ft *FileTree) requestDirSizeScan() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil || !n.isDir {
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Select a directory to scan size", Level: notify.Info}
		}
	}
	if n.isSymlink {
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot scan symlinked directory", Level: notify.Info}
		}
	}

	root := n.path
	return ft, func() tea.Msg {
		sizes, err := scanDirectorySizes(root)
		return dirSizeResultMsg{root: root, sizes: sizes, err: err}
	}
}

func (ft *FileTree) handleDirSizeResult(msg dirSizeResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Size scan error: " + msg.err.Error(),
				Level:   notify.Error,
			}
		}
	}
	if ft.dirSizeCache == nil {
		ft.dirSizeCache = make(map[string]int64, len(msg.sizes))
	}
	ft.clearDirSizeCacheUnder(msg.root)
	for path, size := range msg.sizes {
		ft.dirSizeCache[path] = size
	}
	ft.rebuildVisible()
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: fmt.Sprintf("Scanned size: %s", humanizeSize(msg.sizes[msg.root])),
			Level:   notify.Success,
		}
	}
}

func (ft *FileTree) clearDirSizeCache() {
	ft.dirSizeCache = nil
}

func (ft *FileTree) clearDirSizeCacheUnder(root string) {
	if len(ft.dirSizeCache) == 0 {
		return
	}
	root = filepath.Clean(root)
	for path := range ft.dirSizeCache {
		if path == root || isDescendantPath(root, path) {
			delete(ft.dirSizeCache, path)
		}
	}
}

func isDescendantPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !startsWithDotDot(rel)
}

func startsWithDotDot(path string) bool {
	return path == ".." || len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}

// humanizeSize formats a byte count for compact right-aligned display.
func humanizeSize(size int64) string {
	if size < bytesPerKiB {
		return strconv.FormatInt(size, 10) + " B"
	}

	units := []struct {
		suffix string
		size   int64
	}{
		{suffix: "GB", size: bytesPerGiB},
		{suffix: "MB", size: bytesPerMiB},
		{suffix: "KB", size: bytesPerKiB},
	}
	for _, unit := range units {
		if size >= unit.size {
			value := float64(size) / float64(unit.size)
			return strconv.FormatFloat(value, 'f', 1, 64) + " " + unit.suffix
		}
	}
	return "0 B"
}

// scanDirectorySizes returns recursive content sizes keyed by absolute path.
// It does not follow symlinks and skips .git directories.
func scanDirectorySizes(root string) (map[string]int64, error) {
	root = filepath.Clean(root)
	sizes := make(map[string]int64)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && d.IsDir() && d.Name() == gitDirName {
			return fs.SkipDir
		}

		path = filepath.Clean(path)
		if d.IsDir() {
			sizes[path] += 0
			return nil
		}

		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		size := info.Size()
		sizes[path] = size
		addSizeToAncestors(sizes, root, path, size)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sizes[root] += 0
	return sizes, nil
}

func addSizeToAncestors(sizes map[string]int64, root, path string, size int64) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		sizes[dir] += size
		if dir == root {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
	}
}
