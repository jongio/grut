package filetree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func buildDetachedTree(ctx context.Context, request treeLoadRequest) (*node, error) {
	root := &node{
		name:      request.name,
		path:      request.path,
		depth:     request.depth,
		isDir:     true,
		isSymlink: request.isSymlink,
		expanded:  request.expanded[request.path],
	}
	if request.isSymlink {
		if !request.config.followSymlinks {
			err := fmt.Errorf("symbolic link traversal is disabled")
			root.loadErr = err
			root.loaded = true
			return root, err
		}
		if !safeSymlinkTarget(request.rootPath, request.path) {
			err := fmt.Errorf("symbolic link target is outside the file tree or forms a loop")
			root.loadErr = err
			root.loaded = true
			return root, err
		}
	}
	err := loadDetachedChildren(ctx, root, request)
	return root, err
}

func loadDetachedChildren(ctx context.Context, parent *node, request treeLoadRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if parent.depth+1 >= request.config.maxDepth {
		parent.loaded = true
		return nil
	}
	readDir := request.config.readDir
	if readDir == nil {
		readDir = os.ReadDir
	}
	entries, err := readDir(parent.path)
	if err != nil {
		parent.loadErr = err
		parent.loaded = true
		return err
	}

	children := make([]*node, 0, len(entries))
	var loadErrors []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child, childErr := nodeFromDirEntry(parent, entry)
		if childErr != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: %w", filepath.Join(parent.path, entry.Name()), childErr))
			continue
		}
		children = append(children, child)
	}
	sortChildrenStatic(children, request.config.sortDirectoriesFirst)
	parent.children = children
	parent.loaded = true
	parent.loadErr = errors.Join(loadErrors...)
	for _, child := range children {
		if !child.isDir || (!request.expandAll && !request.expanded[child.path]) {
			continue
		}
		if child.isSymlink {
			if !request.config.followSymlinks || !safeSymlinkTarget(request.rootPath, child.path) {
				continue
			}
		}
		child.expanded = true
		if childErr := loadDetachedChildren(ctx, child, request); childErr != nil {
			if errors.Is(childErr, context.Canceled) {
				return childErr
			}
			loadErrors = append(loadErrors, childErr)
		}
	}
	return errors.Join(loadErrors...)
}

func nodeFromDirEntry(parent *node, entry os.DirEntry) (*node, error) {
	childPath := filepath.Join(parent.path, entry.Name())
	child := &node{
		name:  entry.Name(),
		path:  childPath,
		depth: parent.depth + 1,
	}
	entryType := entry.Type()
	switch {
	case entryType&os.ModeSymlink != 0:
		populateSymlinkNode(child)
		return child, nil
	case entryType.IsDir():
		child.isDir = true
		return child, nil
	}

	info, err := entry.Info()
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		populateSymlinkNode(child)
		return child, nil
	}
	child.isDir = info.IsDir()
	if !child.isDir {
		child.isExecutable = info.Mode()&0o111 != 0
	}
	return child, nil
}

func populateSymlinkNode(child *node) {
	child.isSymlink = true
	if target, err := os.Readlink(child.path); err == nil {
		child.symlinkTarget = target
	}
	if targetInfo, err := os.Stat(child.path); err == nil {
		child.isDir = targetInfo.IsDir()
		if !child.isDir {
			child.isExecutable = targetInfo.Mode()&0o111 != 0
		}
	}
}

func safeSymlinkTarget(rootPath, symlinkPath string) bool {
	return pathResolvesWithinRoot(rootPath, symlinkPath) && !symlinkCreatesLoop(symlinkPath)
}

func pathResolvesWithinRoot(rootPath, path string) bool {
	rootResolved, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		rootResolved = filepath.Clean(rootPath)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(rootResolved, resolved)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func symlinkCreatesLoop(symlinkPath string) bool {
	target, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		return true
	}
	target = filepath.Clean(target)
	symlinkDir, err := filepath.EvalSymlinks(filepath.Dir(symlinkPath))
	if err != nil {
		return true
	}
	loopRel, err := filepath.Rel(target, filepath.Clean(symlinkDir))
	if err != nil {
		return true
	}
	return loopRel != ".." && !strings.HasPrefix(loopRel, ".."+string(filepath.Separator))
}
