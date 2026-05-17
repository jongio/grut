package filetree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// safeCtx returns the FileTree's context, falling back to context.Background()
// if Init() has not been called yet (e.g. during tests).
func (ft *FileTree) safeCtx() context.Context {
	if ft.ctx != nil {
		return ft.ctx
	}
	return context.Background()
}

// ---------------------------------------------------------------------------
// Cursor & selection helpers
// ---------------------------------------------------------------------------
// selectedPaths returns paths of all selected items, or the cursor item if
// nothing is explicitly selected.
func (ft *FileTree) selectedPaths() []string {
	if len(ft.selected) > 0 {
		paths := make([]string, 0, len(ft.selected))
		for p := range ft.selected {
			paths = append(paths, p)
		}
		return paths
	}
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		return []string{ft.visible[ft.viewport.cursor].path}
	}
	return nil
}

// cursorNode returns the node under the cursor, or nil.
func (ft *FileTree) cursorNode() *node {
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		return ft.visible[ft.viewport.cursor]
	}
	return nil
}

// cursorDir returns the directory context for the current cursor position.
// If the cursor is on a directory, it returns that directory's path.
// If the cursor is on a file, it returns the parent directory's path.
func (ft *FileTree) cursorDir() string {
	n := ft.cursorNode()
	if n == nil {
		return ft.rootPath
	}
	if n.isDir {
		return n.path
	}
	return filepath.Dir(n.path)
}

// ---------------------------------------------------------------------------
// Request operations (show modals)
// ---------------------------------------------------------------------------
// requestDelete initiates a delete operation with confirmation modal.
func (ft *FileTree) requestDelete() (panels.Panel, tea.Cmd) {
	paths := ft.selectedPaths()
	if len(paths) == 0 {
		return ft, nil
	}
	ft.pending = &pendingOperation{
		kind:  opDelete,
		paths: paths,
	}
	var msg string
	if len(paths) == 1 {
		msg = fmt.Sprintf("Delete %s?", filepath.Base(paths[0]))
	} else {
		msg = fmt.Sprintf("Delete %d items?", len(paths))
	}
	return ft, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:    notify.ModalConfirm,
			Title:   "Delete",
			Message: msg,
		}
	}
}

// requestRename initiates a rename operation with input modal.
func (ft *FileTree) requestRename() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil {
		return ft, nil
	}
	ft.pending = &pendingOperation{
		kind:  opRename,
		paths: []string{n.path},
	}
	return ft, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:        notify.ModalInput,
			Title:       "Rename",
			Message:     fmt.Sprintf("Rename %s:", filepath.Base(n.path)),
			Placeholder: filepath.Base(n.path),
		}
	}
}

// requestNewFile shows an input modal for creating a new file.
func (ft *FileTree) requestNewFile() (panels.Panel, tea.Cmd) {
	ft.pending = &pendingOperation{
		kind:    opNewFile,
		destDir: ft.cursorDir(),
	}
	return ft, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:        notify.ModalInput,
			Title:       "New File",
			Message:     "Enter file name:",
			Placeholder: "filename.txt",
		}
	}
}

// requestNewDir shows an input modal for creating a new directory.
func (ft *FileTree) requestNewDir() (panels.Panel, tea.Cmd) {
	ft.pending = &pendingOperation{
		kind:    opNewDir,
		destDir: ft.cursorDir(),
	}
	return ft, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:        notify.ModalInput,
			Title:       "New Directory",
			Message:     "Enter directory name:",
			Placeholder: "dirname",
		}
	}
}

// ---------------------------------------------------------------------------
// Clipboard operations
// ---------------------------------------------------------------------------
// copyToClipboard copies selected paths to the internal clipboard.
func (ft *FileTree) copyToClipboard() (panels.Panel, tea.Cmd) {
	paths := ft.selectedPaths()
	if len(paths) == 0 {
		return ft, nil
	}
	ft.clip = clipboard{paths: paths, cut: false}
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: fmt.Sprintf("Copied %d item(s)", len(paths)),
			Level:   notify.Info,
		}
	}
}

// pasteResultMsg is sent when an async paste operation completes (F13).
type pasteResultMsg struct {
	action string // "Copied" or "Moved"
	errs   []string
	count  int
	wasCut bool
}

// pasteFromClipboard executes a copy or move from the clipboard to the
// current cursor directory.
func (ft *FileTree) pasteFromClipboard() (panels.Panel, tea.Cmd) {
	if len(ft.clip.paths) == 0 {
		return ft, nil
	}
	// Capture operation details for the async goroutine (F13).
	isCut := ft.clip.cut
	paths := make([]string, len(ft.clip.paths))
	copy(paths, ft.clip.paths)
	destDir := ft.cursorDir()
	rootPath := ft.rootPath
	ctx := ft.safeCtx()
	return ft, func() tea.Msg {
		var errs []string
		for _, src := range paths {
			dst := filepath.Join(destDir, filepath.Base(src))
			var err error
			if isCut {
				err = moveFile(ctx, rootPath, src, dst)
			} else {
				err = copyFile(ctx, rootPath, src, dst)
			}
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
		action := "Copied"
		if isCut {
			action = "Moved"
		}
		return pasteResultMsg{action: action, count: len(paths), errs: errs, wasCut: isCut}
	}
}

// ---------------------------------------------------------------------------
// Bookmark operations
// ---------------------------------------------------------------------------
// bookmarkCurrent emits a BookmarkAddMsg for the current cursor directory.
func (ft *FileTree) bookmarkCurrent() (panels.Panel, tea.Cmd) {
	path := ft.cursorDir()
	return ft, func() tea.Msg {
		return panels.BookmarkAddMsg{Path: path}
	}
}

// addToContext emits an AddToContextMsg for the file under the cursor.
// Only regular files are added; directories are silently ignored.
func (ft *FileTree) addToContext() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil || n.isDir {
		return ft, nil
	}
	path := n.path
	return ft, func() tea.Msg {
		return panels.AddToContextMsg{Path: path}
	}
}

// bookmarkPath is a no-op handler for BookmarkAddMsg within filetree.
// The actual bookmark add is handled by the app model which has access
// to the bookmark manager.
func (ft *FileTree) bookmarkPath(path string) (panels.Panel, tea.Cmd) {
	return ft, nil
}

// navigateToPath changes the filetree root to the given path.
func (ft *FileTree) navigateToPath(path string) (panels.Panel, tea.Cmd) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath = filepath.Clean(absPath)
	info, statErr := os.Stat(absPath)
	if statErr != nil || !info.IsDir() {
		errMsg := fmt.Sprintf("Cannot navigate to %s", filepath.Base(absPath))
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Error}
		}
	}
	ft.rootPath = absPath
	ft.root = &node{
		name:  filepath.Base(absPath),
		path:  absPath,
		isDir: true,
		depth: -1,
	}
	ft.loadChildren(ft.root)
	ft.rebuildVisible()
	// Update watcher to watch new root.
	if ft.watcher != nil {
		ft.watcher.addDir(absPath)
	}
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Navigated to " + filepath.Base(absPath),
			Level:   notify.Info,
		}
	}
}

// ---------------------------------------------------------------------------
// Modal result handler
// ---------------------------------------------------------------------------
// handleModalResult processes the result from a confirmation or input modal.
func (ft *FileTree) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := ft.pending
	ft.pending = nil
	if op == nil || !msg.Accept {
		return ft, nil
	}
	switch op.kind { //nolint:exhaustive // only relevant cases handled
	case opDelete:
		return ft.executeDelete(op.paths)
	case opRename:
		return ft.executeRename(op.paths[0], msg.Value)
	case opNewFile:
		return ft.executeNewFile(op.destDir, msg.Value)
	case opNewDir:
		return ft.executeNewDir(op.destDir, msg.Value)
	case opRightClickPick:
		return ft.executeRightClickAction(actions.ActionID(msg.Value))
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&ft.actionsCfg, op.name, msg.Value)
		}
		return ft.executeRightClickAction(actions.ActionID(msg.Value))
	}
	return ft, nil
}

// ---------------------------------------------------------------------------
// Execute operations (async I/O in tea.Cmd — F13)
// ---------------------------------------------------------------------------
// deleteResultMsg is sent when an async delete operation completes (F13).
type deleteResultMsg struct {
	errs  []string
	paths []string // deleted paths, for selection cleanup
	count int
}

// renameResultMsg is sent when an async rename operation completes (F13).
type renameResultMsg struct {
	oldPath string
	newName string
	err     string
}

// executeDelete deletes the given paths asynchronously (F13).
func (ft *FileTree) executeDelete(paths []string) (panels.Panel, tea.Cmd) {
	rootPath := ft.rootPath
	pathsCopy := make([]string, len(paths))
	copy(pathsCopy, paths)
	ctx := ft.safeCtx()
	return ft, func() tea.Msg {
		var errs []string
		for _, p := range pathsCopy {
			if err := deleteFile(ctx, rootPath, p); err != nil {
				errs = append(errs, err.Error())
			}
		}
		return deleteResultMsg{count: len(pathsCopy), errs: errs, paths: pathsCopy}
	}
}

// executeRename renames the file at oldPath to newName asynchronously (F13).
func (ft *FileTree) executeRename(oldPath, newName string) (panels.Panel, tea.Cmd) {
	if newName == "" {
		return ft, nil
	}
	rootPath := ft.rootPath
	ctx := ft.safeCtx()
	return ft, func() tea.Msg {
		if err := renameFile(ctx, rootPath, oldPath, newName); err != nil {
			return renameResultMsg{oldPath: oldPath, newName: newName, err: err.Error()}
		}
		return renameResultMsg{oldPath: oldPath, newName: newName}
	}
}

// executeNewFile creates a new file with the given name.
func (ft *FileTree) executeNewFile(dir, name string) (panels.Panel, tea.Cmd) {
	if name == "" {
		return ft, nil
	}
	path := filepath.Join(dir, name)
	ctx := ft.safeCtx()
	if err := createFile(ctx, ft.rootPath, path); err != nil {
		errMsg := err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Error}
		}
	}
	ft.reloadTree()
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Created " + name,
			Level:   notify.Success,
		}
	}
}

// executeNewDir creates a new directory with the given name.
func (ft *FileTree) executeNewDir(dir, name string) (panels.Panel, tea.Cmd) {
	if name == "" {
		return ft, nil
	}
	path := filepath.Join(dir, name)
	ctx := ft.safeCtx()
	if err := createDir(ctx, ft.rootPath, path); err != nil {
		errMsg := err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Error}
		}
	}
	ft.reloadTree()
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Created " + name + "/",
			Level:   notify.Success,
		}
	}
}

// ---------------------------------------------------------------------------
// Right-click action execution
// ---------------------------------------------------------------------------
// executeRightClickAction dispatches a right-click action to the appropriate method.
func (ft *FileTree) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionOpenInEditor:
		return ft.openInEditor()
	case actions.ActionExpandCollapse:
		return ft.selectOrExpand()
	case actions.ActionCopyPath:
		return ft.copyPath()
	case actions.ActionStage:
		return ft.stageFile()
	case actions.ActionPreview:
		return ft, ft.emitCursorFileSelected()
	}
	return ft, nil
}

// openInEditor opens the file at the cursor in the user's external editor.
func (ft *FileTree) openInEditor() (panels.Panel, tea.Cmd) {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return ft, nil
	}
	n := ft.visible[ft.viewport.cursor]
	if n.isDir {
		return ft.selectOrExpand()
	}
	filePath := n.path
	fileName := n.name
	return ft, func() tea.Msg {
		if err := panels.OpenInEditor(ft.safeCtx(), filePath); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened " + fileName, Level: notify.Info}
	}
}

// copyPath copies the path of the item under the cursor to the OS clipboard.
func (ft *FileTree) copyPath() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil {
		return ft, nil
	}
	path := n.path
	if err := panels.CopyToClipboard(ft.ctx, path); err != nil {
		errMsg := err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Copy failed: " + errMsg,
				Level:   notify.Error,
			}
		}
	}
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Copied: " + path,
			Level:   notify.Success,
		}
	}
}

// stageFile stages the file under the cursor using git add.
func (ft *FileTree) stageFile() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil {
		return ft, nil
	}
	path := n.path
	root := ft.rootPath
	return ft, func() tea.Msg {
		if err := git.ValidatePath(path); err != nil {
			return notify.ShowToastMsg{
				Message: "Stage blocked: invalid path",
				Level:   notify.Error,
			}
		}
		cmd := exec.CommandContext(ft.safeCtx(), "git", "-C", root, "add", "--", path)
		if err := cmd.Run(); err != nil {
			return notify.ShowToastMsg{
				Message: "Stage failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		return notify.ShowToastMsg{
			Message: "Staged: " + filepath.Base(path),
			Level:   notify.Success,
		}
	}
}
