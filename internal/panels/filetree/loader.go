package filetree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

type treeLoadPurpose uint8

const (
	treeLoadInitial treeLoadPurpose = iota
	treeLoadExpand
	treeLoadRefresh
	treeLoadReload
	treeLoadRecursive
	treeLoadFilter
	treeLoadReveal
	treeLoadNavigate
	treeLoadModeRestore
)

type treeLoadConfig struct {
	maxDepth             int
	sortDirectoriesFirst bool
	followSymlinks       bool
	readDir              func(string) ([]os.DirEntry, error)
}

type treeLoadRequest struct {
	config     treeLoadConfig
	expanded   map[string]bool
	name       string
	path       string
	rootPath   string
	cursorPath string
	revealPath string
	generation uint64
	depth      int
	revealLine int
	purpose    treeLoadPurpose
	expandAll  bool
	cursorTop  bool
	isSymlink  bool
}

type treeLoadedMsg struct {
	root    *node
	request treeLoadRequest
	err     error
}

type treeBuilderFunc func(context.Context, treeLoadRequest) (*node, error)

func snapshotTreeLoadConfig(cfg Config) treeLoadConfig {
	return treeLoadConfig{
		maxDepth:             cfg.GetMaxDepth(),
		sortDirectoriesFirst: cfg.GetSortDirectoriesFirst(),
		followSymlinks:       cfg.GetFollowSymlinks(),
	}
}

func clonePathSet(paths map[string]bool) map[string]bool {
	if len(paths) == 0 {
		return nil
	}
	cloned := make(map[string]bool, len(paths))
	for path, included := range paths {
		if included {
			cloned[path] = true
		}
	}
	return cloned
}

func mergePathSets(base, extra map[string]bool) map[string]bool {
	merged := clonePathSet(base)
	if merged == nil && len(extra) > 0 {
		merged = make(map[string]bool, len(extra))
	}
	for path, included := range extra {
		if included {
			merged[path] = true
		}
	}
	return merged
}

func (ft *FileTree) currentTreeLoadContext() (uint64, context.Context) {
	if ft.treeLoadContext != nil {
		return ft.treeLoadGeneration, ft.treeLoadContext
	}
	base := ft.safeCtx()
	ft.treeLoadGeneration++
	ft.treeLoadContext, ft.treeLoadCancel = context.WithCancel(base)
	return ft.treeLoadGeneration, ft.treeLoadContext
}

func (ft *FileTree) nextTreeLoadGeneration() (uint64, context.Context) {
	if ft.treeLoadCancel != nil {
		ft.treeLoadCancel()
	}
	base := ft.safeCtx()
	ft.treeLoadGeneration++
	ft.treeLoadContext, ft.treeLoadCancel = context.WithCancel(base)
	return ft.treeLoadGeneration, ft.treeLoadContext
}

func (ft *FileTree) invalidateTreeLoads() {
	if ft.treeLoadCancel != nil {
		ft.treeLoadCancel()
	}
	ft.treeLoadGeneration++
	ft.treeLoadContext = nil
	ft.treeLoadCancel = nil
	clearLoadingState(ft.root)
}

func clearLoadingState(root *node) {
	if root == nil {
		return
	}
	root.loading = false
	for _, child := range root.children {
		clearLoadingState(child)
	}
}

func (ft *FileTree) treeLoadCmd(ctx context.Context, request treeLoadRequest) tea.Cmd {
	builder := ft.treeBuilder
	if builder == nil {
		builder = buildDetachedTree
	}
	return func() tea.Msg {
		root, err := builder(ctx, request)
		return treeLoadedMsg{root: root, request: request, err: err}
	}
}

func (ft *FileTree) requestChildLoad(n *node) tea.Cmd {
	if n == nil || !n.isDir || n.loaded || n.loading {
		return nil
	}
	generation, ctx := ft.currentTreeLoadContext()
	n.loading = true
	request := treeLoadRequest{
		config:     snapshotTreeLoadConfig(ft.cfg),
		expanded:   ft.collectExpanded(n),
		name:       n.name,
		path:       n.path,
		rootPath:   ft.rootPath,
		generation: generation,
		depth:      n.depth,
		purpose:    treeLoadExpand,
		isSymlink:  n.isSymlink,
	}
	return ft.treeLoadCmd(ctx, request)
}

func (ft *FileTree) requestRootLoad(
	purpose treeLoadPurpose,
	expanded map[string]bool,
	expandAll bool,
	cursorPath string,
	cursorTop bool,
	revealPath string,
	revealLine int,
) tea.Cmd {
	generation, ctx := ft.nextTreeLoadGeneration()
	request := treeLoadRequest{
		config:     snapshotTreeLoadConfig(ft.cfg),
		expanded:   clonePathSet(expanded),
		name:       filepath.Base(ft.rootPath),
		path:       ft.rootPath,
		rootPath:   ft.rootPath,
		cursorPath: cursorPath,
		revealPath: revealPath,
		generation: generation,
		depth:      -1,
		revealLine: revealLine,
		purpose:    purpose,
		expandAll:  expandAll,
		cursorTop:  cursorTop,
	}
	return ft.treeLoadCmd(ctx, request)
}

func (ft *FileTree) requestTreeReload(purpose treeLoadPurpose) tea.Cmd {
	ft.clearDirSizeCache()
	return ft.requestRootLoad(
		purpose,
		ft.collectExpanded(ft.root),
		false,
		ft.CursorPath(),
		false,
		"",
		0,
	)
}

func (ft *FileTree) requestExpandedTreeLoad(
	extraExpanded map[string]bool,
	cursorPath string,
	cursorTop bool,
) tea.Cmd {
	expanded := mergePathSets(ft.collectExpanded(ft.root), extraExpanded)
	if ft.canApplyExpandedState(expanded) {
		ft.invalidateTreeLoads()
		applyExpandedState(ft.root, expanded)
		ft.rebuildVisible()
		if cursorTop {
			ft.viewport.cursor = 0
			ft.viewport.offset = 0
		} else {
			ft.restoreCursorToPath(cursorPath)
		}
		return ft.emitCursorFileSelected()
	}
	return ft.requestRootLoad(treeLoadFilter, expanded, false, cursorPath, cursorTop, "", 0)
}

func (ft *FileTree) requestExactExpandedTreeLoad(
	purpose treeLoadPurpose,
	expanded map[string]bool,
	cursorPath string,
) tea.Cmd {
	if ft.canApplyExpandedState(expanded) {
		ft.invalidateTreeLoads()
		applyExpandedState(ft.root, expanded)
		ft.rebuildVisible()
		ft.restoreCursorToPath(cursorPath)
		return ft.emitCursorFileSelected()
	}
	return ft.requestRootLoad(purpose, expanded, false, cursorPath, false, "", 0)
}

func (ft *FileTree) requestExpandAll() tea.Cmd {
	if ft.allExpandableDirectoriesLoaded(ft.root) {
		ft.invalidateTreeLoads()
		cursorPath := ft.CursorPath()
		ft.expandLoadedDirectories(ft.root)
		ft.rebuildVisible()
		ft.restoreCursorToPath(cursorPath)
		return nil
	}
	return ft.requestRootLoad(
		treeLoadRecursive,
		nil,
		true,
		ft.CursorPath(),
		false,
		"",
		0,
	)
}

func (ft *FileTree) canApplyExpandedState(expanded map[string]bool) bool {
	for path := range expanded {
		if path == ft.rootPath {
			continue
		}
		n := findNodeByPath(ft.root, path)
		if n == nil || !n.isDir || !n.loaded {
			return false
		}
	}
	return true
}

func applyExpandedState(root *node, expanded map[string]bool) {
	if root == nil {
		return
	}
	for _, child := range root.children {
		if !child.isDir {
			continue
		}
		child.expanded = expanded[child.path]
		applyExpandedState(child, expanded)
	}
}

func (ft *FileTree) allExpandableDirectoriesLoaded(root *node) bool {
	if root == nil {
		return true
	}
	for _, child := range root.children {
		if !child.isDir {
			continue
		}
		if child.isSymlink && !ft.cfg.GetFollowSymlinks() {
			continue
		}
		if !child.loaded {
			return false
		}
		if !ft.allExpandableDirectoriesLoaded(child) {
			return false
		}
	}
	return true
}

func (ft *FileTree) expandLoadedDirectories(root *node) {
	if root == nil {
		return
	}
	for _, child := range root.children {
		if !child.isDir {
			continue
		}
		if child.isSymlink && (!ft.cfg.GetFollowSymlinks() || child.loadErr != nil) {
			continue
		}
		child.expanded = true
		ft.expandLoadedDirectories(child)
	}
}

func (ft *FileTree) requestReveal(path string, line int) tea.Cmd {
	if path == "" || ft.root == nil {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(ft.rootPath, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	expanded := ft.collectExpanded(ft.root)
	if expanded == nil {
		expanded = make(map[string]bool)
	}
	parent := filepath.Dir(absPath)
	for parent != ft.rootPath {
		expanded[parent] = true
		next := filepath.Dir(parent)
		if next == parent {
			return nil
		}
		parent = next
	}
	return ft.requestRootLoad(
		treeLoadReveal,
		expanded,
		false,
		ft.CursorPath(),
		false,
		absPath,
		line,
	)
}

func (ft *FileTree) handleTreeLoaded(msg treeLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.request.generation != ft.treeLoadGeneration {
		return ft, nil
	}
	if errors.Is(msg.err, context.Canceled) {
		return ft, nil
	}

	var cmds []tea.Cmd
	if msg.request.purpose == treeLoadExpand {
		target := findNodeByPath(ft.root, msg.request.path)
		if target == nil {
			return ft, nil
		}
		target.loading = false
		if msg.root != nil {
			target.children = msg.root.children
			target.loaded = msg.root.loaded
			target.loadErr = msg.root.loadErr
		}
		ft.rebuildVisible()
		if msg.err == nil && target.expanded {
			path := target.path
			cmds = append(cmds, func() tea.Msg { return DirChangedMsg{Path: path} })
		}
	} else if msg.root != nil {
		ft.root = msg.root
		ft.rebuildVisible()
		switch {
		case msg.request.revealPath != "":
			ft.restoreCursorToPath(msg.request.revealPath)
		case msg.request.cursorTop:
			ft.viewport.cursor = 0
			ft.viewport.offset = 0
		default:
			ft.restoreCursorToPath(msg.request.cursorPath)
		}

		switch msg.request.purpose {
		case treeLoadInitial:
			if ft.filter.gitFilter && ft.gitChanged.loaded() {
				cmds = append(cmds, ft.requestExpandedTreeLoad(
					ft.gitChanged.dirs,
					ft.savedCursorPath,
					false,
				))
			} else if ft.filter.gitFilter && ft.gitClient != nil {
				cmds = append(cmds, ft.loadGitChangedFiles())
			}
		case treeLoadReload, treeLoadFilter, treeLoadReveal, treeLoadNavigate, treeLoadModeRestore:
			if msg.request.purpose == treeLoadReveal {
				cmds = append(cmds, ft.emitCursorFileSelectedAtLine(msg.request.revealLine))
			} else {
				cmds = append(cmds, ft.emitCursorFileSelected())
			}
		case treeLoadRefresh, treeLoadRecursive, treeLoadExpand:
		}
	}

	if msg.err != nil {
		errText := msg.err.Error()
		path := msg.request.path
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Cannot load %s: %s", filepath.Base(path), errText),
				Level:   notify.Error,
			}
		})
	}
	return ft, tea.Batch(cmds...)
}

func findNodeByPath(root *node, path string) *node {
	if root == nil {
		return nil
	}
	if root.path == path {
		return root
	}
	for _, child := range root.children {
		if found := findNodeByPath(child, path); found != nil {
			return found
		}
	}
	return nil
}
