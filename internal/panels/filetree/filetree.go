// Package filetree implements the file explorer panel for grut.
// It provides a navigable tree view of the filesystem with lazy directory
// loading, symlink safety, icon support, and cursor-based navigation.
package filetree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
)

// Messages emitted by the filetree panel.
// DirChangedMsg is sent when a directory is expanded.
type DirChangedMsg struct{ Path string }

type panelColors struct {
	Directory  string
	Default    string
	Executable string
	Symlink    string
	CursorBg   string
	SelectedBg string
	Dim        string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Directory:  "#7A9EBF",
		Default:    "#999999",
		Executable: "#6B9E56",
		Symlink:    "#C9A227",
		CursorBg:   "#2A2A2A",
		SelectedBg: "#222222",
		Dim:        "#555555",
	}
	if th != nil {
		c.Directory = th.Colors.FileDirectory
		c.Default = th.Colors.FileDefault
		c.Executable = th.Colors.FileExecutable
		c.Symlink = th.Colors.FileSymlink
		c.CursorBg = th.Colors.SelectionBg
		c.SelectedBg = th.Colors.CursorLine
		c.Dim = th.Colors.BrightBlack
	}
	return c
}

// keyEsc is the key name for the Escape key, used in mode-exit checks.
const keyEsc = "esc"

// changedFiles holds the set of changed file paths and the derived set of
// directories that transitively contain those files. A nil *changedFiles
// means "no data loaded yet" (different from an empty set).
type changedFiles struct {
	paths map[string]bool
	dirs  map[string]bool
}

// hasPath reports whether p is in the changed-file set.
func (cf *changedFiles) hasPath(p string) bool {
	return cf != nil && cf.paths[p]
}

// hasDir reports whether d is in the changed-directory set.
func (cf *changedFiles) hasDir(d string) bool {
	return cf != nil && cf.dirs[d]
}

// loaded reports whether changed-file data has been populated.
func (cf *changedFiles) loaded() bool {
	return cf != nil && cf.paths != nil
}

// newChangedFiles builds a changedFiles from a set of absolute paths,
// deriving the transitive directory set up to (and including) rootPath.
func newChangedFiles(paths map[string]bool, rootPath string) *changedFiles {
	dirs := make(map[string]bool)
	for p := range paths {
		dir := filepath.Dir(p)
		for dir != "" && dir != "." && dir != rootPath {
			if dirs[dir] {
				break
			}
			dirs[dir] = true
			dir = filepath.Dir(dir)
		}
		dirs[rootPath] = true
	}
	return &changedFiles{paths: paths, dirs: dirs}
}

// node represents a single entry (file or directory) in the file tree.
type node struct {
	loadErr       error // non-nil if directory loading failed (F06)
	name          string
	path          string
	symlinkTarget string // raw readlink value for display
	children      []*node
	depth         int
	isDir         bool
	isSymlink     bool
	isExecutable  bool
	loaded        bool // true after first loadChildren call
	expanded      bool
}

// viewportState holds cursor position and viewport dimensions used together
// for scroll/viewport calculations.
type viewportState struct {
	cursor int // index into visible
	offset int // viewport scroll offset
	width  int
	height int
}

// filterState holds all git/commit/PR/branch filter mode fields. These
// control which subset of files the tree displays.
type filterState struct {
	commitChanged *changedFiles // commit-changed files + dirs
	prChanged     *changedFiles // PR-changed files + dirs
	branchChanged *changedFiles // branch-changed files + dirs
	commitHash    string        // short hash for display
	commitLabel   string        // e.g. "abc1234 Fix auth bug"
	prLabel       string
	branchName    string   // selected branch name
	branchLabel   string   // e.g. "branch: feature/auth"
	branchBaseRef string   // base ref for branch comparison (e.g., "main")
	baseBranch    string   // configured default/base branch for "b" toggle
	commitFiles   []string // relative paths from diff-tree
	branchFiles   []string // relative paths from branch diff
	prFiles       []panels.PRFile
	prNumber      int
	// Git-aware filtering state.
	gitFilter bool // when true, only show git-changed files
	// Commit-files mode: shows files changed by a specific commit.
	commitFilesMode bool // when true, view shows commit-changed files
	// PR-files mode: shows files changed in a pull request.
	prFilesMode bool
	// Branch-files mode: shows files changed on a selected branch.
	branchFilesMode bool
	// Branch-diff filter: user toggled "b" to see origination diff.
	branchDiffFilter bool
}

// FileTree is the file explorer panel. It implements [panels.Panel].
type FileTree struct {
	viewport viewportState
	filter   filterState
	// Right-click action configuration.
	actionsCfg config.ActionsConfig
	gitClient  git.StatusReader // git client for fetching status (nil = no git)
	// Git-ignored paths (from .gitignore) keyed by absolute path.
	ignoreChecker git.IgnoreChecker
	ctx           context.Context // stored from Init for watcher lifecycle
	root          *node           // root directory node (not rendered; depth = -1)
	selected      map[string]bool // multi-select state keyed by path
	gitChanged    *changedFiles   // git-status changed files + dirs
	// Git file status indicators (e.g. M, A, ?, D) per absolute path.
	gitFileStatus   map[string]string
	gitIgnoredPaths map[string]bool
	dirSizeCache    map[string]int64
	// Per-mode expand/collapse state preservation (Change 3).
	explorerExpanded map[string]bool   // saved expand state for explorer mode
	gitModeExpanded  map[string]bool   // saved expand state for git mode
	pending          *pendingOperation // operation awaiting modal confirmation
	watcher          *watcher          // filesystem watcher
	rootPath         string
	// Cursor path saved across async boundaries (e.g. toggleGitFilter → GitChangedFilesMsg).
	savedCursorPath string
	visible         []*node // flattened list of currently visible nodes
	cfg             Config
	colors          panelColors
	theme           *theme.Theme
	// File operation state.
	clip              clipboard // cut/copy clipboard
	focused           bool
	showHidden        bool
	listMode          bool // true = flat list with relative paths, false = tree view
	statusLoadPending bool // prevents redundant loadGitFileStatus dispatches
	// Self-contained double-click detection: the engine's detection relies on
	// two consecutive MouseClickMsg events at the same position, but on Windows
	// terminals the second press may not be delivered as a separate event.
	// We detect double-clicks ourselves in handleMouseClick by comparing timing.
	lastClickTime time.Time
	lastClickIdx  int // absolute index in visible slice (not viewport-relative row)
}

// Compile-time interface check.
var _ panels.Panel = (*FileTree)(nil)

// Compile-time check that FileTree implements panels.Closer (F29).
var _ panels.Closer = (*FileTree)(nil)

// New creates a new FileTree panel rooted at rootPath.
func New(cfg Config, rootPath string, th *theme.Theme) *FileTree {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		absRoot = rootPath
	}
	ft := &FileTree{
		cfg:      cfg,
		rootPath: filepath.Clean(absRoot),
		root: &node{
			name:  filepath.Base(absRoot),
			path:  filepath.Clean(absRoot),
			isDir: true,
			depth: -1,
		},
		colors:     initColors(th),
		theme:      th,
		selected:   make(map[string]bool),
		showHidden: cfg.GetShowHidden(),
	}
	// No directory I/O here — deferred to Init() (F05).
	return ft
}

// SetGitClient configures the filetree with a git client for git-aware
// filtering. When set, the user can press "g" to toggle between all files
// and only git-changed files.
func (ft *FileTree) SetGitClient(gc git.StatusReader) {
	ft.gitClient = gc
	if ic, ok := gc.(git.IgnoreChecker); ok {
		ft.ignoreChecker = ic
	}
}

// SetBaseBranch configures the default base branch for the "b" toggle
// (branch diff filter). Typically set from config.GitConfig.DefaultBranch.
func (ft *FileTree) SetBaseBranch(branch string) {
	ft.filter.baseBranch = branch
}

// handleRepoChanged replaces the git client so git-aware features (file
// status icons, git filter mode, ignored-file detection) use the new repo.
func (ft *FileTree) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		ft.gitClient = nil
		ft.ignoreChecker = nil
	} else {
		ft.SetGitClient(client)
	}
	ft.filter.gitFilter = false
	ft.gitChanged = nil
	ft.gitFileStatus = nil
	ft.gitIgnoredPaths = nil
	ft.gitModeExpanded = nil
	var cmds []tea.Cmd
	if ft.gitClient != nil {
		cmds = append(cmds, ft.loadGitFileStatus(), ft.loadGitIgnored())
	}
	return ft, tea.Batch(cmds...)
}

// rootLoadedMsg is sent when the initial directory load completes (F05).
type rootLoadedMsg struct {
	root *node
}

// commitFilesLoadedMsg carries the result of an async DiffTreeFiles call.
type commitFilesLoadedMsg struct {
	err   error
	hash  string
	label string
	files []string
}

// branchFilesLoadedMsg carries the result of an async DiffFileNames call.
type branchFilesLoadedMsg struct {
	err    error
	branch string
	files  []string
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (ft *FileTree) Init(ctx context.Context) tea.Cmd {
	ft.ctx = ctx
	ft.watcher = newWatcher(defaultDebounce, defaultPollInterval)
	ft.watcher.addDir(ft.rootPath)
	// Load root children asynchronously (F05).
	rootPath := ft.rootPath
	cfg := ft.cfg
	return tea.Batch(
		ft.watcher.start(ctx),
		func() tea.Msg {
			root := &node{
				name:  filepath.Base(rootPath),
				path:  rootPath,
				isDir: true,
				depth: -1,
			}
			loadChildrenStatic(root, cfg)
			return rootLoadedMsg{root: root}
		},
		ft.loadGitFileStatus(),
		ft.loadGitIgnored(),
	)
}

// Update implements panels.Panel.
func (ft *FileTree) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case rootLoadedMsg:
		ft.root = msg.root
		if ft.filter.gitFilter && ft.gitChanged.loaded() {
			// Git status arrived before root loaded — apply filter
			// but keep all folders collapsed on startup.
			ft.rebuildVisible()
			return ft, nil
		}
		ft.rebuildVisible()
		if ft.filter.gitFilter && !ft.gitChanged.loaded() && ft.gitClient != nil {
			return ft, ft.loadGitChangedFiles()
		}
		return ft, nil
	case gitFileStatusMsg:
		ft.gitFileStatus = msg.status
		ft.statusLoadPending = false
		return ft, nil
	case gitIgnoredMsg:
		ft.gitIgnoredPaths = msg.paths
		return ft, nil
	case dirSizeResultMsg:
		return ft.handleDirSizeResult(msg)
	case pasteResultMsg:
		if msg.wasCut {
			ft.clip = clipboard{}
			ft.selected = make(map[string]bool)
		}
		ft.reloadTree()
		if len(msg.errs) > 0 {
			errMsg := strings.Join(msg.errs, "; ")
			return ft, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: "Paste error: " + errMsg,
					Level:   notify.Error,
				}
			}
		}
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("%s %d item(s)", msg.action, msg.count),
				Level:   notify.Success,
			}
		}
	case deleteResultMsg:
		for _, p := range msg.paths {
			delete(ft.selected, p)
		}
		ft.reloadTree()
		if len(msg.errs) > 0 {
			errMsg := strings.Join(msg.errs, "; ")
			return ft, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Delete error: " + errMsg, Level: notify.Error}
			}
		}
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Deleted %d item(s)", msg.count),
				Level:   notify.Success,
			}
		}
	case renameResultMsg:
		ft.reloadTree()
		if msg.err != "" {
			errMsg := msg.err
			return ft, func() tea.Msg {
				return notify.ShowToastMsg{Message: errMsg, Level: notify.Error}
			}
		}
		if ft.selected[msg.oldPath] {
			delete(ft.selected, msg.oldPath)
			ft.selected[filepath.Join(filepath.Dir(msg.oldPath), msg.newName)] = true
		}
		newName := msg.newName
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Renamed to " + newName, Level: notify.Success}
		}
	case tea.KeyPressMsg:
		return ft.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return ft.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return ft.handleMouseDoubleClick(msg)
	case panels.PanelMouseRightClickMsg:
		return ft.handleMouseRightClick(msg)
	case tea.MouseWheelMsg:
		return ft.handleMouseWheel(msg)
	case notify.ModalResultMsg:
		return ft.handleModalResult(msg)
	case panels.CommandSelectedMsg:
		if !ft.focused || msg.Action != actionDirSize {
			return ft, nil
		}
		return ft.requestDirSizeScan()
	case RefreshMsg:
		return ft.handleRefresh()
	case panels.NavigateToPathMsg:
		return ft.navigateToPath(msg.Path)
	case panels.RepoChangedMsg:
		return ft.handleRepoChanged(msg)
	case panels.BookmarkAddMsg:
		return ft.bookmarkPath(msg.Path)
	case panels.TabActivatedMsg:
		return ft.handleTabActivated(msg)
	case panels.GitStatusChangedMsg:
		// Refresh per-file status indicators after stage/unstage so the
		// title dirty indicator (*) stays in sync.
		cmds := []tea.Cmd{ft.loadGitFileStatus()}
		ft.statusLoadPending = true
		// If in git filter mode, also reload the changed-files list so
		// discarded/unstaged files disappear from the filtered view.
		if ft.filter.gitFilter && ft.gitClient != nil {
			ft.savedCursorPath = ft.CursorPath()
			cmds = append(cmds, ft.loadGitChangedFiles())
		}
		return ft, tea.Batch(cmds...)
	case panels.RefreshGitChangedFilesMsg:
		// Direct request to reload git-changed files (e.g. after discard/unstage).
		if ft.filter.gitFilter && ft.gitClient != nil {
			ft.savedCursorPath = ft.CursorPath()
			return ft, ft.loadGitChangedFiles()
		}
		return ft, nil
	case panels.GitChangedFilesMsg:
		ft.gitChanged = newChangedFiles(msg.Paths, ft.rootPath)
		if ft.root.loaded {
			// Auto-expand every directory that contains git-changed
			// files so the user sees the changed files immediately.
			ft.expandGitChangedDirs()
			ft.rebuildVisible()
			ft.restoreCursorToPath(ft.savedCursorPath)
			ft.savedCursorPath = ""
		}
		// Skip redundant loadGitFileStatus if already dispatched by the
		// GitStatusChangedMsg handler in the same cascade.
		cmds := []tea.Cmd{ft.loadGitIgnored(), ft.emitCursorFileSelected()}
		if !ft.statusLoadPending {
			cmds = append(cmds, ft.loadGitFileStatus())
		}
		ft.statusLoadPending = false
		return ft, tea.Batch(cmds...)
	case panels.RevealFileMsg:
		ft.revealFile(msg.Path)
		return ft, ft.emitCursorFileSelectedAtLine(msg.Line)
	case panels.CommitSelectedMsg:
		return ft.handleCommitSelected(msg)
	case panels.CommitDeselectedMsg:
		ft.exitCommitFilesMode()
		return ft, ft.emitCursorFileSelected()
	case commitFilesLoadedMsg:
		return ft.handleCommitFilesLoaded(msg)
	case panels.PRFilesLoadedMsg:
		return ft.handlePRFilesLoaded(msg)
	case panels.PRDeselectedMsg:
		ft.exitPRFilesMode()
		return ft, ft.emitCursorFileSelected()
	case panels.BranchSelectedMsg:
		return ft.handleBranchSelected(msg)
	case panels.BranchDeselectedMsg:
		ft.exitBranchFilesMode()
		return ft, ft.emitCursorFileSelected()
	case branchFilesLoadedMsg:
		return ft.handleBranchFilesLoaded(msg)
	// CRUD actions dispatched via keymap.
	case panels.ItemCreateMsg:
		if !ft.focused {
			return ft, nil
		}
		return ft.requestNewFile()
	case panels.ItemDeleteMsg:
		if !ft.focused {
			return ft, nil
		}
		return ft.requestDelete()
	case panels.ItemEditMsg:
		if !ft.focused {
			return ft, nil
		}
		return ft.requestRename()
	case panels.ItemOpenMsg:
		if !ft.focused {
			return ft, nil
		}
		return ft.openInEditor()
	case panels.ItemCopyMsg:
		if !ft.focused {
			return ft, nil
		}
		return ft.copyPath()
	}
	return ft, nil
}

// View implements panels.Panel.
func (ft *FileTree) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if len(ft.visible) == 0 {
		label := "Empty"
		if !ft.root.loaded {
			label = "Loading..."
		} else if ft.filter.gitFilter {
			label = "No changed files\n\nPress g to show all files"
		}
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(ft.colors.Dim)).
			Render(label)
	}
	lines := make([]string, 0, height)
	end := ft.viewport.offset + height
	if end > len(ft.visible) {
		end = len(ft.visible)
	}
	for i := ft.viewport.offset; i < end; i++ {
		lines = append(lines, ft.renderLine(ft.visible[i], width, i == ft.viewport.cursor))
	}
	// Pad remaining height with blank lines.
	emptyLine := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	return strings.Join(lines, "\n")
}

// Focus implements panels.Panel.
func (ft *FileTree) Focus() { ft.focused = true }

// Blur implements panels.Panel.
func (ft *FileTree) Blur() { ft.focused = false }

// SetSize implements panels.Panel.
func (ft *FileTree) SetSize(width, height int) {
	ft.viewport.width = width
	ft.viewport.height = height
}

// Title implements panels.Panel.
func (ft *FileTree) Title() string {
	if ft.filter.branchFilesMode {
		return "Files: " + ft.filter.branchLabel
	}
	if ft.filter.commitFilesMode {
		return "Files: " + ft.filter.commitLabel
	}
	if ft.filter.prFilesMode {
		return "Files: " + ft.filter.prLabel
	}
	if ft.filter.gitFilter {
		return "Files (git changed)"
	}
	if len(ft.gitFileStatus) > 0 {
		return "Files*"
	}
	return "Files"
}

// KeyBindings implements panels.Panel.
func (ft *FileTree) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "enter/l/→", Description: "Expand dir / select file", Action: "expand"},
		{Key: "h/←", Description: "Collapse dir / go to parent", Action: "collapse"},
		{Key: "H", Description: "Collapse all directories", Action: "collapse_all"},
		{Key: "L", Description: "Expand all directories", Action: "expand_all"},
		{Key: ".", Description: "Toggle hidden files", Action: "toggle_hidden"},
		{Key: "G", Description: "Go to bottom", Action: "go_bottom"},
		{Key: "g", Description: "Go to top", Action: "go_top"},
		{Key: "f", Description: "Cycle filter: all → git changed → branch diff", Action: "cycle_file_filter"},
		{Key: "D", Description: "Scan directory size", Action: actionDirSize},
		{Key: "space", Description: "Toggle selection", Action: "toggle_select"},
		{Key: "n", Description: "Create new file", Action: "item_create"},
		{Key: "d", Description: "Delete file(s)", Action: "item_delete"},
		{Key: "e/F2", Description: "Rename file", Action: "item_edit"},
		{Key: "o", Description: "Open in editor", Action: "item_open"},
		{Key: "M", Description: "Reveal in file manager", Action: "reveal_in_file_manager"},
		{Key: "B", Description: "Open on GitHub", Action: "open_on_github"},
		{Key: "y", Description: "Copy file path", Action: "item_copy"},
		{Key: "c", Description: "Copy selected to clipboard", Action: "copy"},
		{Key: "x", Description: "Cut selected to clipboard", Action: "cut"},
		{Key: "p", Description: "Paste from clipboard", Action: "paste"},
		{Key: "I", Description: "Add to .gitignore", Action: "gitignore_add"},
		{Key: "a", Description: "Create new file (alt)", Action: "new_file"},
		{Key: "A", Description: "Create new directory", Action: "new_dir"},
		{Key: "R", Description: "Rename file (alt)", Action: "rename"},
		{Key: "m", Description: "Bookmark current directory", Action: "bookmark"},
		{Key: "C", Description: "Add file to context", Action: "add_to_context"},
		{Key: "v", Description: "Toggle tree/list view", Action: "toggle_view"},
	}
}

// Close implements panels.Closer. It stops the filesystem watcher and
// releases associated resources (F29).
func (ft *FileTree) Close() {
	if ft.watcher != nil {
		ft.watcher.stop()
	}
}

// ---------------------------------------------------------------------------
// Commit-files mode
// ---------------------------------------------------------------------------
// handleCommitSelected starts loading files changed by the selected commit.
func (ft *FileTree) handleCommitSelected(msg panels.CommitSelectedMsg) (panels.Panel, tea.Cmd) {
	if ft.gitClient == nil {
		return ft, nil
	}
	gc := ft.gitClient
	ctx := ft.ctx
	hash := msg.Hash
	subject := msg.Subject
	// Build a short label: "abc1234 Fix auth bug"
	shortHash := hash
	if len(shortHash) > git.ShortHashLen {
		shortHash = shortHash[:git.ShortHashLen]
	}
	label := shortHash
	if subject != "" {
		label += " " + subject
	}
	return ft, func() tea.Msg {
		files, err := gc.DiffTreeFiles(ctx, hash)
		return commitFilesLoadedMsg{files: files, hash: shortHash, label: label, err: err}
	}
}

// handleCommitFilesLoaded enters commit-files mode with the loaded file list.
func (ft *FileTree) handleCommitFilesLoaded(msg commitFilesLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "diff-tree: " + errMsg, Level: notify.Error}
		}
	}
	ft.filter.commitFilesMode = true
	ft.filter.commitFiles = msg.files
	ft.filter.commitHash = msg.hash
	ft.filter.commitLabel = msg.label
	// Exit branch-files mode if active.
	if ft.filter.branchFilesMode {
		ft.filter.branchFilesMode = false
		ft.filter.branchDiffFilter = false
		ft.filter.branchFiles = nil
		ft.filter.branchName = ""
		ft.filter.branchLabel = ""
		ft.filter.branchBaseRef = ""
		ft.filter.branchChanged = nil
	}
	// Save cursor position so we can restore it on exit.
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		ft.savedCursorPath = ft.visible[ft.viewport.cursor].path
	}
	// Build filter sets from commit file paths (analogous to gitFilter approach).
	paths := make(map[string]bool, len(msg.files))
	for _, f := range msg.files {
		abs := filepath.Clean(filepath.Join(ft.rootPath, f))
		paths[abs] = true
	}
	ft.filter.commitChanged = newChangedFiles(paths, ft.rootPath)
	// Expand directories containing commit-changed files so the tree
	// shows the full hierarchy immediately.
	ft.expandDirsInSet(ft.root, ft.filter.commitChanged.dirs)
	ft.rebuildVisible()
	ft.viewport.cursor = 0
	ft.viewport.offset = 0
	return ft, ft.emitCursorFileSelected()
}

// exitCommitFilesMode restores the normal file tree view.
func (ft *FileTree) exitCommitFilesMode() {
	ft.filter.commitFilesMode = false
	ft.filter.commitFiles = nil
	ft.filter.commitHash = ""
	ft.filter.commitLabel = ""
	ft.filter.commitChanged = nil
	ft.rebuildVisible()
	ft.restoreCursorToPath(ft.savedCursorPath)
	ft.savedCursorPath = ""
}

// ---------------------------------------------------------------------------
// PR-files mode
// ---------------------------------------------------------------------------
// handlePRFilesLoaded enters PR-files mode with the loaded file list.
func (ft *FileTree) handlePRFilesLoaded(msg panels.PRFilesLoadedMsg) (panels.Panel, tea.Cmd) {
	ft.filter.prFilesMode = true
	ft.filter.prFiles = msg.Files
	ft.filter.prNumber = msg.Number
	ft.filter.prLabel = fmt.Sprintf("PR #%d", msg.Number)
	// Exit commit-files mode if active.
	if ft.filter.commitFilesMode {
		ft.filter.commitFilesMode = false
		ft.filter.commitFiles = nil
		ft.filter.commitChanged = nil
	}
	// Exit branch-files mode if active.
	if ft.filter.branchFilesMode {
		ft.filter.branchFilesMode = false
		ft.filter.branchDiffFilter = false
		ft.filter.branchFiles = nil
		ft.filter.branchName = ""
		ft.filter.branchLabel = ""
		ft.filter.branchBaseRef = ""
		ft.filter.branchChanged = nil
	}
	// Save cursor position so we can restore it on exit.
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		ft.savedCursorPath = ft.visible[ft.viewport.cursor].path
	}
	// Build filter sets from PR file paths (analogous to gitFilter approach).
	paths := make(map[string]bool, len(msg.Files))
	for _, f := range msg.Files {
		abs := filepath.Clean(filepath.Join(ft.rootPath, f.Filename))
		paths[abs] = true
	}
	ft.filter.prChanged = newChangedFiles(paths, ft.rootPath)
	// Expand directories containing PR-changed files so the tree
	// shows the full hierarchy immediately.
	ft.expandDirsInSet(ft.root, ft.filter.prChanged.dirs)
	ft.rebuildVisible()
	ft.viewport.cursor = 0
	ft.viewport.offset = 0
	return ft, ft.emitCursorFileSelected()
}

// exitPRFilesMode restores the normal file tree view.
func (ft *FileTree) exitPRFilesMode() {
	ft.filter.prFilesMode = false
	ft.filter.prFiles = nil
	ft.filter.prNumber = 0
	ft.filter.prLabel = ""
	ft.filter.prChanged = nil
	ft.rebuildVisible()
	ft.restoreCursorToPath(ft.savedCursorPath)
	ft.savedCursorPath = ""
}

// ---------------------------------------------------------------------------
// Branch-files mode
// ---------------------------------------------------------------------------

// handleBranchSelected starts loading files changed on the selected branch.
func (ft *FileTree) handleBranchSelected(msg panels.BranchSelectedMsg) (panels.Panel, tea.Cmd) {
	// Toggle off if same branch selected again or empty name.
	if msg.Name == "" || msg.Name == ft.filter.branchName {
		ft.exitBranchFilesMode()
		return ft, ft.emitCursorFileSelected()
	}
	if ft.gitClient == nil {
		return ft, nil
	}
	gc := ft.gitClient
	ctx := ft.ctx
	name := msg.Name
	return ft, func() tea.Msg {
		files, err := gc.DiffFileNames(ctx, name, "HEAD")
		return branchFilesLoadedMsg{files: files, branch: name, err: err}
	}
}

// handleBranchFilesLoaded enters branch-files mode with the loaded file list.
func (ft *FileTree) handleBranchFilesLoaded(msg branchFilesLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "branch diff: " + errMsg, Level: notify.Error}
		}
	}
	ft.filter.branchFilesMode = true
	ft.filter.branchFiles = msg.files
	ft.filter.branchName = msg.branch
	ft.filter.branchBaseRef = msg.branch
	if ft.filter.branchDiffFilter {
		ft.filter.branchLabel = "diff vs " + msg.branch
	} else {
		ft.filter.branchLabel = "branch: " + msg.branch
	}
	// Exit commit-files mode if active.
	if ft.filter.commitFilesMode {
		ft.filter.commitFilesMode = false
		ft.filter.commitFiles = nil
		ft.filter.commitChanged = nil
	}
	// Exit PR-files mode if active.
	if ft.filter.prFilesMode {
		ft.filter.prFilesMode = false
		ft.filter.prFiles = nil
		ft.filter.prChanged = nil
	}
	// Save cursor position so we can restore it on exit.
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		ft.savedCursorPath = ft.visible[ft.viewport.cursor].path
	}
	// Build filter sets from branch file paths.
	paths := make(map[string]bool, len(msg.files))
	for _, f := range msg.files {
		abs := filepath.Clean(filepath.Join(ft.rootPath, f))
		paths[abs] = true
	}
	ft.filter.branchChanged = newChangedFiles(paths, ft.rootPath)
	// Expand directories containing branch-changed files so the tree
	// shows the full hierarchy immediately.
	ft.expandDirsInSet(ft.root, ft.filter.branchChanged.dirs)
	ft.rebuildVisible()
	ft.viewport.cursor = 0
	ft.viewport.offset = 0
	cmds := []tea.Cmd{ft.emitCursorFileSelected()}
	if ft.filter.branchDiffFilter {
		baseBranch := ft.filter.branchBaseRef
		cmds = append(cmds, func() tea.Msg {
			return panels.BranchDiffFilterActiveMsg{Active: true, BaseBranch: baseBranch}
		})
	}
	return ft, tea.Batch(cmds...)
}

// exitBranchFilesMode restores the normal file tree view.
func (ft *FileTree) exitBranchFilesMode() {
	ft.filter.branchFilesMode = false
	ft.filter.branchDiffFilter = false
	ft.filter.branchFiles = nil
	ft.filter.branchName = ""
	ft.filter.branchLabel = ""
	ft.filter.branchBaseRef = ""
	ft.filter.branchChanged = nil
	ft.rebuildVisible()
	ft.restoreCursorToPath(ft.savedCursorPath)
	ft.savedCursorPath = ""
}

// activateBranchDiffFilter enters branch-diff mode: shows only files that
// differ from the configured base branch (e.g., main). Uses three-dot diff
// to show changes introduced on the current branch since it diverged.
func (ft *FileTree) activateBranchDiffFilter() (panels.Panel, tea.Cmd) {
	base := ft.filter.baseBranch
	if base == "" {
		base = "main"
	}
	ft.filter.branchDiffFilter = true
	gc := ft.gitClient
	ctx := ft.ctx
	return ft, func() tea.Msg {
		files, err := gc.DiffFileNames(ctx, base, "HEAD")
		return branchFilesLoadedMsg{files: files, branch: base, err: err}
	}
}

// cycleFileFilter cycles through file filter modes:
//
//	all files → git changed → branch diff → all files
//
// If a non-cycle filter mode is active (e.g., branch selected from gitinfo
// panel), pressing the cycle key exits to "all files" first.
func (ft *FileTree) cycleFileFilter() (panels.Panel, tea.Cmd) {
	if ft.gitClient == nil {
		return ft, nil
	}
	// If in a panel-driven branch mode (not from our cycle), exit first.
	if ft.filter.branchFilesMode && !ft.filter.branchDiffFilter {
		return ft.exitBranchDiffWithCmd()
	}
	// Cycle: branchDiff → all
	if ft.filter.branchDiffFilter {
		return ft.exitBranchDiffWithCmd()
	}
	// Cycle: gitFilter → branchDiff
	if ft.filter.gitFilter {
		cursorPath := ft.CursorPath()
		ft.filter.gitFilter = false
		ft.gitChanged = nil
		ft.rebuildVisible()
		ft.restoreCursorToPath(cursorPath)
		cmds := []tea.Cmd{
			func() tea.Msg { return panels.GitFilterActiveMsg{Active: false} },
		}
		_, branchCmd := ft.activateBranchDiffFilter()
		if branchCmd != nil {
			cmds = append(cmds, branchCmd)
		}
		return ft, tea.Batch(cmds...)
	}
	// Cycle: all → gitFilter (reuse existing logic)
	return ft.toggleGitFilter()
}

// exitBranchDiffWithCmd exits branch-files mode and emits the appropriate
// deactivation messages.
func (ft *FileTree) exitBranchDiffWithCmd() (panels.Panel, tea.Cmd) {
	wasDiffFilter := ft.filter.branchDiffFilter
	ft.exitBranchFilesMode()
	var cmds []tea.Cmd
	cmds = append(cmds, ft.emitCursorFileSelected())
	if wasDiffFilter {
		cmds = append(cmds, func() tea.Msg {
			return panels.BranchDiffFilterActiveMsg{Active: false}
		})
	}
	return ft, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (ft *FileTree) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !ft.focused {
		return ft, nil
	}
	// In commit-files mode, Escape returns to normal tree view.
	if ft.filter.commitFilesMode && msg.String() == keyEsc {
		ft.exitCommitFilesMode()
		return ft, ft.emitCursorFileSelected()
	}
	// In PR-files mode, Escape returns to normal tree view.
	if ft.filter.prFilesMode && msg.String() == keyEsc {
		ft.exitPRFilesMode()
		return ft, ft.emitCursorFileSelected()
	}
	// In branch-files mode, Escape returns to normal tree view.
	if ft.filter.branchFilesMode && msg.String() == keyEsc {
		return ft.exitBranchDiffWithCmd()
	}
	switch msg.String() {
	case "j", "down":
		ft.moveCursorDown()
		return ft, ft.emitCursorFileSelected()
	case "k", "up":
		ft.moveCursorUp()
		return ft, ft.emitCursorFileSelected()
	case "enter", "l", "right":
		return ft.selectOrExpand()
	case "h", "left":
		return ft.collapseOrParent()
	case "H":
		ft.collapseAllDirs()
	case "L":
		ft.expandAllDirs()
	case ".":
		ft.toggleHidden()
	case "f":
		return ft.cycleFileFilter()
	case "D":
		return ft.requestDirSizeScan()
	case "g":
		ft.goToTop()
	case "G":
		ft.goToBottom()
	case "pgdown":
		ft.pageDown()
		return ft, ft.emitCursorFileSelected()
	case "pgup":
		ft.pageUp()
		return ft, ft.emitCursorFileSelected()
	case " ", "space":
		ft.toggleSelection()
	case "n":
		return ft.requestNewFile()
	case "N":
		return ft.requestNewDir()
	case "x":
		return ft.requestDelete()
	case "e", "F2":
		return ft.requestRename()
	case "o":
		return ft.openInEditor()
	case "O":
		return ft.openInDefaultApp()
	case "M":
		return ft.revealInFileManager()
	case "B":
		return ft.openOnGitHub()
	case "y":
		return ft.copyPath()
	case "c":
		return ft.copyToClipboard()
	case "p":
		return ft.pasteFromClipboard()
	case "I":
		return ft.addToGitignore()
	case "v":
		ft.listMode = !ft.listMode
		ft.rebuildVisible()
	case "J":
		return ft, func() tea.Msg { return panels.PreviewScrollMsg{Delta: 1} }
	case "K":
		return ft, func() tea.Msg { return panels.PreviewScrollMsg{Delta: -1} }
	}
	return ft, nil
}

// handleMouseClick processes a left-click in the filetree panel.
// ContentRow is the row within the content area (0-based), which maps
// directly to ft.viewport.offset + row in the visible node slice.
func (ft *FileTree) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := ft.viewport.offset + msg.ContentRow
	if idx < 0 || idx >= len(ft.visible) {
		return ft, nil
	}
	prevCursor := ft.viewport.cursor
	ft.viewport.cursor = idx
	// Double-click detection: two clicks on the same absolute index within 500ms.
	now := time.Now()
	hadPriorClick := !ft.lastClickTime.IsZero()
	isDouble := ft.lastClickIdx == idx &&
		now.Sub(ft.lastClickTime) <= 500*time.Millisecond
	ft.lastClickTime = now
	ft.lastClickIdx = idx
	if isDouble {
		ft.lastClickTime = time.Time{} // reset so triple-click isn't also double
		return ft.executeDoubleClick()
	}
	// If clicking the same file that was already under the cursor (placed
	// there by a prior mouse click), treat as an "open" action. This handles
	// the case where Windows Terminal does not deliver a second MouseClickMsg
	// rapidly enough for timing-based detection.
	n := ft.visible[idx]
	if !n.isDir && prevCursor == idx && hadPriorClick {
		return ft.executeDoubleClick()
	}
	if n.isDir {
		return ft.selectOrExpand()
	}
	return ft, ft.emitCursorFileSelected()
}

// handleMouseDoubleClick processes a double-click in the filetree panel
// when routed by the engine's own double-click detection. Delegates to
// executeDoubleClick for the actual action.
func (ft *FileTree) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := ft.viewport.offset + msg.ContentRow
	if idx < 0 || idx >= len(ft.visible) {
		return ft, nil
	}
	ft.viewport.cursor = idx
	return ft.executeDoubleClick()
}

// executeDoubleClick performs the double-click action on the node at the
// current cursor. For files it opens in the default app; for directories
// it shows the first-use picker or executes the configured action.
func (ft *FileTree) executeDoubleClick() (panels.Panel, tea.Cmd) {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return ft, nil
	}
	n := ft.visible[ft.viewport.cursor]
	if !n.isDir {
		itemType := actions.ItemFile
		if !ft.actionsCfg.IsConfirmed(string(itemType)) {
			config.SaveDoubleClickChoice(&ft.actionsCfg, string(itemType), string(actions.DefaultAction(itemType)))
		}
		action := actions.ActionID(ft.actionsCfg.GetDoubleClickAction(string(itemType)))
		return ft.executeRightClickAction(action)
	}
	itemType := actions.ItemDirectory
	if !ft.actionsCfg.IsConfirmed(string(itemType)) {
		ft.pending = &pendingOperation{kind: opFirstUseConfirm, name: string(itemType)}
		return ft, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(ft.actionsCfg.GetDoubleClickAction(string(itemType)))
	return ft.executeRightClickAction(action)
}

// SetActionsCfg stores the actions configuration for right-click menus.
func (ft *FileTree) SetActionsCfg(cfg config.ActionsConfig) { ft.actionsCfg = cfg }

// handleMouseRightClick processes a right-click in the filetree panel.
func (ft *FileTree) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := ft.viewport.offset + msg.ContentRow
	if idx < 0 || idx >= len(ft.visible) {
		return ft, nil
	}
	ft.viewport.cursor = idx
	ft.ensureCursorVisible()
	n := ft.visible[idx]
	itemType := actions.ItemFile
	if n.isDir {
		itemType = actions.ItemDirectory
	}
	cmd, directAction := rightclick.Cmd(ft.actionsCfg, itemType, n.name)
	if cmd != nil {
		ft.pending = &pendingOperation{kind: opRightClickPick}
		return ft, cmd
	}
	if directAction != "" {
		return ft.executeRightClickAction(directAction)
	}
	return ft, nil
}

// handleMouseWheel scrolls the filetree viewport.
func (ft *FileTree) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		ft.viewport.offset -= panels.ScrollDelta
		if ft.viewport.offset < 0 {
			ft.viewport.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(ft.visible) - ft.viewport.height
		if maxOffset < 0 {
			maxOffset = 0
		}
		ft.viewport.offset += panels.ScrollDelta
		if ft.viewport.offset > maxOffset {
			ft.viewport.offset = maxOffset
		}
	}
	// Keep cursor within the visible viewport so that background events
	// (filesystem watcher refresh, git status updates) calling
	// ensureCursorVisible() don't snap the viewport back to the old
	// cursor position.
	cursorMoved := false
	if ft.viewport.cursor < ft.viewport.offset {
		ft.viewport.cursor = ft.viewport.offset
		cursorMoved = true
	}
	if ft.viewport.height > 0 && ft.viewport.cursor >= ft.viewport.offset+ft.viewport.height {
		ft.viewport.cursor = ft.viewport.offset + ft.viewport.height - 1
		cursorMoved = true
	}
	if cursorMoved {
		return ft, ft.emitCursorFileSelected()
	}
	return ft, nil
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (ft *FileTree) moveCursorDown() {
	if ft.viewport.cursor < len(ft.visible)-1 {
		ft.viewport.cursor++
		ft.ensureCursorVisible()
	}
}

func (ft *FileTree) moveCursorUp() {
	if ft.viewport.cursor > 0 {
		ft.viewport.cursor--
		ft.ensureCursorVisible()
	}
}

// emitCursorFileSelected returns a cmd that sends FileSelectedMsg for the
// currently focused node, so the preview panel updates on every cursor move.
// In branch-files mode, it emits ShowDiffMsg with ref comparison context so
// the diff panel shows the branch comparison instead of working tree diff.
func (ft *FileTree) emitCursorFileSelected() tea.Cmd {
	return ft.emitCursorFileSelectedAtLine(0)
}

// emitCursorFileSelectedAtLine emits a FileSelectedMsg for the file under the
// cursor, carrying an optional 1-based line for the preview to scroll to.
// A line of 0 leaves the preview at the top, matching normal selection.
func (ft *FileTree) emitCursorFileSelectedAtLine(line int) tea.Cmd {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return nil
	}
	n := ft.visible[ft.viewport.cursor]
	if n.isDir {
		path := n.path
		return func() tea.Msg { return panels.FolderSelectedMsg{Path: path} }
	}
	path := n.path
	// Build the DiffContext based on the filetree's current mode.
	var dc *panels.DiffContext
	switch {
	case ft.filter.commitFilesMode && ft.filter.commitHash != "":
		dc = &panels.DiffContext{
			Type:    panels.DiffContextCommit,
			CommitA: ft.filter.commitHash + "~1",
			CommitB: ft.filter.commitHash,
		}
	case ft.filter.prFilesMode:
		dc = &panels.DiffContext{
			Type: panels.DiffContextPR,
		}
	case ft.filter.branchFilesMode && ft.filter.branchBaseRef != "":
		dc = &panels.DiffContext{
			Type:     panels.DiffContextBranch,
			CommitA:  ft.filter.branchBaseRef,
			CommitB:  "HEAD",
			ThreeDot: true,
		}
	case ft.filter.gitFilter:
		dc = &panels.DiffContext{
			Type: panels.DiffContextWorking,
		}
	}

	// In branch-files mode, also emit ShowDiffMsg for the gitdiff panel.
	if ft.filter.branchFilesMode && ft.filter.branchBaseRef != "" {
		relPath, err := filepath.Rel(ft.rootPath, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)
		baseRef := ft.filter.branchBaseRef
		return tea.Batch(
			func() tea.Msg { return panels.FileSelectedMsg{Path: path, DiffContext: dc, Line: line} },
			func() tea.Msg {
				return panels.ShowDiffMsg{
					Path:     relPath,
					CommitA:  baseRef,
					CommitB:  "HEAD",
					ThreeDot: true,
				}
			},
		)
	}
	return func() tea.Msg { return panels.FileSelectedMsg{Path: path, DiffContext: dc, Line: line} }
}

func (ft *FileTree) goToTop() {
	ft.viewport.cursor = 0
	ft.ensureCursorVisible()
}

func (ft *FileTree) goToBottom() {
	if n := len(ft.visible); n > 0 {
		ft.viewport.cursor = n - 1
		ft.ensureCursorVisible()
	}
}

// pageDown moves the cursor down by one page (viewport height).
func (ft *FileTree) pageDown() {
	if ft.viewport.height <= 0 {
		return
	}
	n := len(ft.visible)
	ft.viewport.cursor += ft.viewport.height
	if ft.viewport.cursor >= n {
		ft.viewport.cursor = n - 1
	}
	if ft.viewport.cursor < 0 {
		ft.viewport.cursor = 0
	}
	ft.ensureCursorVisible()
}

// pageUp moves the cursor up by one page (viewport height).
func (ft *FileTree) pageUp() {
	if ft.viewport.height <= 0 {
		return
	}
	ft.viewport.cursor -= ft.viewport.height
	if ft.viewport.cursor < 0 {
		ft.viewport.cursor = 0
	}
	ft.ensureCursorVisible()
}

func (ft *FileTree) ensureCursorVisible() {
	ft.viewport.offset = panels.EnsureCursorVisible(ft.viewport.cursor, ft.viewport.offset, ft.viewport.height)
}

// ---------------------------------------------------------------------------
// Selection & expand/collapse
// ---------------------------------------------------------------------------
func (ft *FileTree) selectOrExpand() (panels.Panel, tea.Cmd) {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return ft, nil
	}
	n := ft.visible[ft.viewport.cursor]
	if n.isDir {
		// Guard symlink expansion.
		if n.isSymlink {
			if !ft.cfg.GetFollowSymlinks() {
				return ft, nil
			}
			if !ft.isPathSafe(n.path) || ft.isSymlinkLoop(n.path) {
				return ft, nil
			}
		}
		// Toggle expand/collapse.
		n.expanded = !n.expanded
		if n.expanded {
			ft.loadChildren(n)
		}
		ft.rebuildVisible()
		if n.expanded {
			return ft, func() tea.Msg { return DirChangedMsg{Path: n.path} }
		}
		return ft, nil
	}
	// File: emit selection message.
	path := n.path
	return ft, func() tea.Msg { return panels.FileSelectedMsg{Path: path} }
}

func (ft *FileTree) collapseOrParent() (panels.Panel, tea.Cmd) {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return ft, nil
	}
	n := ft.visible[ft.viewport.cursor]
	// Collapse if the node is an expanded directory.
	if n.isDir && n.expanded {
		n.expanded = false
		ft.rebuildVisible()
		return ft, nil
	}
	// Otherwise navigate to the parent directory.
	targetDepth := n.depth - 1
	for i := ft.viewport.cursor - 1; i >= 0; i-- {
		if ft.visible[i].depth == targetDepth && ft.visible[i].isDir {
			ft.viewport.cursor = i
			ft.ensureCursorVisible()
			break
		}
	}
	return ft, nil
}

func (ft *FileTree) toggleSelection() {
	if ft.viewport.cursor < 0 || ft.viewport.cursor >= len(ft.visible) {
		return
	}
	p := ft.visible[ft.viewport.cursor].path
	if ft.selected[p] {
		delete(ft.selected, p)
	} else {
		ft.selected[p] = true
	}
}

func (ft *FileTree) toggleHidden() {
	ft.showHidden = !ft.showHidden
	ft.rebuildVisible()
}

// toggleGitFilter switches between showing all files and only git-changed files.
func (ft *FileTree) toggleGitFilter() (panels.Panel, tea.Cmd) {
	if ft.gitClient == nil {
		return ft, nil
	}
	// Save cursor path before switching modes.
	cursorPath := ft.CursorPath()
	ft.filter.gitFilter = !ft.filter.gitFilter
	if ft.filter.gitFilter {
		// Save cursor path across the async boundary so
		// GitChangedFilesMsg handler can restore it.
		ft.savedCursorPath = cursorPath
		// Rebuild visible immediately — while gitChanged is nil the
		// filter check is skipped and all files remain shown, avoiding a
		// blank tree before the async result arrives.
		ft.rebuildVisible()
		// Notify preview to show diff-only, fetch git status, and update
		// the preview to reflect whatever file the cursor now points at.
		return ft, tea.Batch(
			func() tea.Msg { return panels.GitFilterActiveMsg{Active: true} },
			ft.loadGitChangedFiles(),
			ft.emitCursorFileSelected(),
		)
	}
	// Unfilter: rebuild visible with all files
	ft.gitChanged = nil
	ft.rebuildVisible()
	ft.restoreCursorToPath(cursorPath)
	return ft, tea.Batch(
		func() tea.Msg { return panels.GitFilterActiveMsg{Active: false} },
		ft.emitCursorFileSelected(),
	)
}

// loadGitChangedFiles issues an async command to fetch git-changed file paths.
func (ft *FileTree) loadGitChangedFiles() tea.Cmd {
	client := ft.gitClient
	ctx := ft.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	rootPath := ft.rootPath // capture for volume normalization
	return func() tea.Msg {
		files, err := client.Status(ctx)
		if err != nil {
			return panels.GitChangedFilesMsg{Paths: nil}
		}
		// git status --porcelain=v2 returns paths relative to the
		// directory where git runs (client.repoDir == rootPath), so
		// join with rootPath — NOT repoRoot — to build absolute paths
		// that match the filetree node paths.
		paths := make(map[string]bool, len(files))
		for _, f := range files {
			abs := filepath.Join(rootPath, f.Path)
			paths[filepath.Clean(abs)] = true
		}
		return panels.GitChangedFilesMsg{Paths: paths}
	}
}

// normalizeVolume replaces the volume prefix of p with the volume prefix of
// ref so that paths built from p are comparable to paths built from ref.
// On Windows this fixes drive-letter case mismatches (e.g. "d:" vs "D:")
// between git rev-parse output and os.Getwd(). On Unix this is a no-op.
func normalizeVolume(p, ref string) string {
	pVol := filepath.VolumeName(p)
	refVol := filepath.VolumeName(ref)
	if pVol == "" || refVol == "" || pVol == refVol {
		return p
	}
	if strings.EqualFold(pVol, refVol) {
		return refVol + p[len(pVol):]
	}
	return p
}

// handleTabActivated reacts to tab switches by auto-enabling/disabling
// git filter and preserving per-mode expand/collapse state.
func (ft *FileTree) handleTabActivated(msg panels.TabActivatedMsg) (panels.Panel, tea.Cmd) {
	// Save cursor path before any mode switch so it can be restored.
	cursorPath := ft.CursorPath()
	if msg.PresetName == presetGit {
		if !ft.filter.gitFilter {
			// Save explorer expand state.
			ft.explorerExpanded = ft.collectExpanded(ft.root)
			ft.filter.gitFilter = true
			// Save cursor across the async boundary.
			ft.savedCursorPath = cursorPath
			// If we have a saved git-mode state, restore it.
			if ft.gitModeExpanded != nil {
				ft.collapseAll(ft.root)
				ft.restoreExpanded(ft.root, ft.gitModeExpanded)
			}
			// Rebuild visible immediately so the tree shows all files
			// while the async git status is loading (gitChanged
			// is nil, so the filter is skipped and all files show).
			ft.rebuildVisible()
			ft.restoreCursorToPath(cursorPath)
			// Load git-changed files; GitChangedFilesMsg will auto-expand dirs.
			return ft, tea.Batch(
				func() tea.Msg { return panels.GitFilterActiveMsg{Active: true} },
				ft.loadGitChangedFiles(),
			)
		}
		return ft, nil
	}
	// Switching away from git mode.
	if ft.filter.gitFilter {
		// Save git-mode expand state.
		ft.gitModeExpanded = ft.collectExpanded(ft.root)
		ft.filter.gitFilter = false
		ft.gitChanged = nil
		// Restore explorer expand state.
		ft.collapseAll(ft.root)
		if ft.explorerExpanded != nil {
			ft.restoreExpanded(ft.root, ft.explorerExpanded)
		}
		ft.rebuildVisible()
		ft.restoreCursorToPath(cursorPath)
	}
	return ft, func() tea.Msg { return panels.GitFilterActiveMsg{Active: false} }
}

// expandGitChangedDirs walks the tree and expands every directory whose
// path is in ft.gitChanged.dirs, loading children as needed.
func (ft *FileTree) expandGitChangedDirs() {
	if ft.gitChanged == nil || len(ft.gitChanged.dirs) == 0 {
		return
	}
	ft.expandGitChangedDirsWalk(ft.root)
}

func (ft *FileTree) expandGitChangedDirsWalk(n *node) {
	for _, child := range n.children {
		if child.isDir && ft.gitChanged.hasDir(child.path) {
			ft.loadChildren(child)
			child.expanded = true
			ft.expandGitChangedDirsWalk(child)
		}
	}
}

// expandDirsInSet walks the tree and expands every directory whose path is
// in the given set, loading children as needed. Used by commit/PR/branch modes to auto-expand directories containing changed files.
func (ft *FileTree) expandDirsInSet(n *node, dirs map[string]bool) {
	for _, child := range n.children {
		if child.isDir && dirs[child.path] {
			ft.loadChildren(child)
			child.expanded = true
			ft.expandDirsInSet(child, dirs)
		}
	}
}

// collapseAll recursively collapses every directory under n.
func (ft *FileTree) collapseAll(n *node) {
	for _, child := range n.children {
		if child.isDir {
			child.expanded = false
			ft.collapseAll(child)
		}
	}
}

// expandAll recursively expands and loads every directory under n. Symlinked
// directories are only expanded when following symlinks is enabled and the
// target is safe, matching the guards in selectOrExpand.
func (ft *FileTree) expandAll(n *node) {
	for _, child := range n.children {
		if !child.isDir {
			continue
		}
		if child.isSymlink {
			if !ft.cfg.GetFollowSymlinks() || !ft.isPathSafe(child.path) || ft.isSymlinkLoop(child.path) {
				continue
			}
		}
		ft.loadChildren(child)
		child.expanded = true
		ft.expandAll(child)
	}
}

// collapseAllDirs collapses every directory in the tree and keeps the cursor on
// the nearest still-visible ancestor of its previous position.
func (ft *FileTree) collapseAllDirs() {
	prev := ft.CursorPath()
	ft.collapseAll(ft.root)
	ft.rebuildVisible()
	ft.cursorToPathOrAncestor(prev)
}

// expandAllDirs expands and loads every directory in the tree, keeping the
// cursor on its previous node.
func (ft *FileTree) expandAllDirs() {
	prev := ft.CursorPath()
	ft.expandAll(ft.root)
	ft.rebuildVisible()
	ft.restoreCursorToPath(prev)
}

// cursorToPathOrAncestor positions the cursor on the node matching path, or on
// the nearest visible ancestor directory when the exact node is no longer
// visible (for example after collapsing the whole tree). Falls back to the top.
func (ft *FileTree) cursorToPathOrAncestor(path string) {
	if path == "" {
		ft.goToTop()
		return
	}
	best := -1
	bestLen := -1
	for i, n := range ft.visible {
		if n.path == path {
			ft.viewport.cursor = i
			ft.ensureCursorVisible()
			return
		}
		if len(n.path) > bestLen && strings.HasPrefix(path, n.path+string(filepath.Separator)) {
			best = i
			bestLen = len(n.path)
		}
	}
	if best >= 0 {
		ft.viewport.cursor = best
		ft.ensureCursorVisible()
		return
	}
	ft.goToTop()
}

// ---------------------------------------------------------------------------
// Git file status indicators
// ---------------------------------------------------------------------------
// gitFileStatusMsg carries per-file git status indicators loaded
// asynchronously.
type gitFileStatusMsg struct {
	status map[string]string
}

// loadGitFileStatus issues an async command to populate per-file status
// indicators (M, A, D, ?, R, U) for display in the tree.
func (ft *FileTree) loadGitFileStatus() tea.Cmd {
	if ft.gitClient == nil {
		return nil
	}
	client := ft.gitClient
	ctx := ft.ctx
	return func() tea.Msg {
		files, err := client.Status(ctx)
		if err != nil {
			return gitFileStatusMsg{status: nil}
		}
		repoRoot, err := client.RepoRoot(ctx)
		if err != nil {
			return gitFileStatusMsg{status: nil}
		}
		status := make(map[string]string, len(files))
		for _, f := range files {
			indicator := fileStatusIndicator(f)
			if indicator == "" {
				continue
			}
			absPath := filepath.Clean(filepath.Join(repoRoot, f.Path))
			status[absPath] = indicator
		}
		return gitFileStatusMsg{status: status}
	}
}

// fileStatusIndicator returns a single-character indicator for a file's
// git status. Conflicts take priority, then staged status, then worktree.
func fileStatusIndicator(f git.FileStatus) string {
	switch {
	case f.StagedStatus == git.StatusConflict || f.WorktreeStatus == git.StatusConflict:
		return "U"
	case f.StagedStatus != git.StatusUnmodified && f.StagedStatus != git.StatusUntracked:
		return string(f.StagedStatus)
	case f.WorktreeStatus != git.StatusUnmodified:
		return string(f.WorktreeStatus)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Git-ignored path detection
// ---------------------------------------------------------------------------
// gitIgnoredMsg carries paths ignored by .gitignore loaded asynchronously.
type gitIgnoredMsg struct {
	paths map[string]bool
}

// loadGitIgnored issues an async command to populate the set of git-ignored
// paths for dimming in the tree.
func (ft *FileTree) loadGitIgnored() tea.Cmd {
	if ft.ignoreChecker == nil || ft.gitClient == nil {
		return nil
	}
	checker := ft.ignoreChecker
	client := ft.gitClient
	ctx := ft.ctx
	return func() tea.Msg {
		ignored, err := checker.IgnoredPaths(ctx)
		if err != nil {
			return gitIgnoredMsg{paths: nil}
		}
		repoRoot, err := client.RepoRoot(ctx)
		if err != nil {
			return gitIgnoredMsg{paths: nil}
		}
		paths := make(map[string]bool, len(ignored))
		for _, p := range ignored {
			// Strip trailing slash from directories.
			p = strings.TrimRight(p, "/\\")
			abs := filepath.Clean(filepath.Join(repoRoot, p))
			paths[abs] = true
		}
		inferIgnoredDirs(paths)
		return gitIgnoredMsg{paths: paths}
	}
}

// inferIgnoredDirs promotes parent directories to ignored when every child
// entry (as reported by os.ReadDir) is already in the ignored set. This
// handles the case where a subdirectory has its own .gitignore (e.g. `*`)
// that ignores all of its contents — git reports each file individually but
// never the directory itself.
func inferIgnoredDirs(paths map[string]bool) {
	for {
		parents := map[string]struct{}{}
		for p := range paths {
			dir := filepath.Dir(p)
			if dir != p && !paths[dir] {
				parents[dir] = struct{}{}
			}
		}
		promoted := false
		for dir := range parents {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			allIgnored := len(entries) > 0
			for _, e := range entries {
				child := filepath.Join(dir, e.Name())
				if !paths[child] {
					allIgnored = false
					break
				}
			}
			if allIgnored {
				paths[dir] = true
				promoted = true
			}
		}
		if !promoted {
			break
		}
	}
}

// isPathIgnored returns true if the given absolute path (or any of its
// ancestor directories) is in the git-ignored set.
func (ft *FileTree) isPathIgnored(absPath string) bool {
	if len(ft.gitIgnoredPaths) == 0 {
		return false
	}
	if ft.gitIgnoredPaths[absPath] {
		return true
	}
	// Walk up the directory tree to check parent directories.
	dir := filepath.Dir(absPath)
	for len(dir) >= len(ft.rootPath) {
		if ft.gitIgnoredPaths[dir] {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}
