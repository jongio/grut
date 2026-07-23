package filetree

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/panels"
	"github.com/mattn/go-runewidth"
)

// builderPool reuses strings.Builder instances to reduce per-frame
// allocations in the render loop (one builder per visible tree node).
var builderPool = sync.Pool{
	New: func() any { return new(strings.Builder) },
}

// ---------------------------------------------------------------------------
// Tree loading
// ---------------------------------------------------------------------------

// loadChildren populates n.children from the filesystem via os.ReadDir and
// os.Lstat.  Loading is lazy — children are only loaded the first time a
// directory is expanded.
func (ft *FileTree) loadChildren(n *node) {
	if n.loaded || !n.isDir {
		return
	}

	// Max depth enforcement.
	if n.depth+1 >= ft.cfg.GetMaxDepth() {
		n.loaded = true
		return
	}

	entries, err := os.ReadDir(n.path)
	if err != nil {
		n.loadErr = err
		n.loaded = true
		return
	}
	n.loadErr = nil

	children := make([]*node, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		childPath := filepath.Join(n.path, name)

		child := &node{
			name:  name,
			path:  childPath,
			depth: n.depth + 1,
		}

		info, err := os.Lstat(childPath)
		if err != nil {
			continue // skip entries we cannot stat
		}

		if info.Mode()&os.ModeSymlink != 0 {
			child.isSymlink = true
			if target, err := os.Readlink(childPath); err == nil {
				child.symlinkTarget = target
			}
			// Follow symlink to determine directory/executable status.
			if targetInfo, err := os.Stat(childPath); err == nil {
				child.isDir = targetInfo.IsDir()
				if !targetInfo.IsDir() {
					child.isExecutable = targetInfo.Mode()&0o111 != 0
				}
			}
		} else {
			child.isDir = info.IsDir()
			if !info.IsDir() {
				child.isExecutable = info.Mode()&0o111 != 0
			}
		}

		children = append(children, child)
	}

	ft.sortChildren(children)
	n.children = children
	n.loaded = true
}

// loadChildrenStatic is a standalone version of loadChildren that uses
// an explicit config rather than the FileTree receiver. Safe for use in
// background goroutines launched by tea.Cmd (F05).
func loadChildrenStatic(n *node, cfg Config) {
	if n.loaded || !n.isDir {
		return
	}
	if n.depth+1 >= cfg.GetMaxDepth() {
		n.loaded = true
		return
	}
	entries, err := os.ReadDir(n.path)
	if err != nil {
		n.loadErr = err
		n.loaded = true
		return
	}
	n.loadErr = nil
	children := make([]*node, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		childPath := filepath.Join(n.path, name)
		child := &node{name: name, path: childPath, depth: n.depth + 1}
		info, err := os.Lstat(childPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			child.isSymlink = true
			if target, err := os.Readlink(childPath); err == nil {
				child.symlinkTarget = target
			}
			if targetInfo, err := os.Stat(childPath); err == nil {
				child.isDir = targetInfo.IsDir()
				if !targetInfo.IsDir() {
					child.isExecutable = targetInfo.Mode()&0o111 != 0
				}
			}
		} else {
			child.isDir = info.IsDir()
			if !info.IsDir() {
				child.isExecutable = info.Mode()&0o111 != 0
			}
		}
		children = append(children, child)
	}
	sortChildrenStatic(children, cfg.GetSortDirectoriesFirst())
	n.children = children
	n.loaded = true
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

// sortChildrenStatic sorts children without a FileTree receiver. Safe for
// use in background goroutines.
func sortChildrenStatic(children []*node, dirFirst bool) {
	slices.SortStableFunc(children, func(a, b *node) int {
		if dirFirst && a.isDir != b.isDir {
			if a.isDir {
				return -1
			}
			return 1
		}
		return compareFold(a.name, b.name)
	})
}

func (ft *FileTree) sortChildren(children []*node) {
	// Stable sort: directories first (when configured), then case-insensitive
	// alphabetical. Uses stdlib slices.SortStableFunc (Go 1.21+).
	dirFirst := ft.cfg.GetSortDirectoriesFirst()
	slices.SortStableFunc(children, func(a, b *node) int {
		if dirFirst && a.isDir != b.isDir {
			if a.isDir {
				return -1
			}
			return 1
		}
		return compareFold(a.name, b.name)
	})
}

// compareFold compares two strings case-insensitively without allocating,
// using rune-by-rune Unicode lower-case folding.
func compareFold(a, b string) int {
	for {
		if a == "" {
			if b == "" {
				return 0
			}
			return -1
		}
		if b == "" {
			return 1
		}
		ra, sizeA := utf8.DecodeRuneInString(a)
		rb, sizeB := utf8.DecodeRuneInString(b)
		la := unicode.ToLower(ra)
		lb := unicode.ToLower(rb)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		a = a[sizeA:]
		b = b[sizeB:]
	}
}

// ---------------------------------------------------------------------------
// Visible list
// ---------------------------------------------------------------------------

// rebuildVisible flattens the tree into ft.visible, applying the hidden-file
// filter and any active mode filters (commit-files, PR-files, git-changed).
func (ft *FileTree) rebuildVisible() {
	ft.visible = ft.visible[:0]
	ft.walkVisible(ft.root)

	// Clamp cursor.
	if len(ft.visible) == 0 {
		ft.viewport.cursor = 0
		return
	}
	if ft.viewport.cursor >= len(ft.visible) {
		ft.viewport.cursor = len(ft.visible) - 1
	}
	if ft.viewport.cursor < 0 {
		ft.viewport.cursor = 0
	}

	ft.ensureCursorVisible()
}

func (ft *FileTree) walkVisible(n *node) {
	for _, child := range n.children {
		// Always hide the .git metadata directory.
		if child.name == ".git" && child.isDir {
			continue
		}
		// In filtered modes (commit/PR/branch/git-changed) the mode's own
		// path-based filter already constrains visibility, so bypass
		// the hidden-file check.  Without this, dotfile changes
		// (e.g. .github/) are suppressed before the filter runs.
		if !ft.showHidden && isHidden(child.name) {
			inFilteredMode := (ft.filter.commitFilesMode && ft.filter.commitChanged.loaded()) ||
				(ft.filter.prFilesMode && ft.filter.prChanged.loaded()) ||
				(ft.filter.releaseCompareMode && ft.filter.releaseChanged.loaded()) ||
				(ft.filter.branchFilesMode && ft.filter.branchChanged.loaded()) ||
				(ft.filter.gitFilter && ft.gitChanged.loaded())
			if !inFilteredMode {
				continue
			}
		}
		// Commit-files filter: skip files/dirs not in the commit-changed set.
		// Takes priority over git filter since the user explicitly selected
		// a commit to inspect.
		if ft.filter.commitFilesMode && ft.filter.commitChanged.loaded() {
			if child.isDir {
				if !ft.filter.commitChanged.hasDir(child.path) {
					continue
				}
			} else {
				if !ft.filter.commitChanged.hasPath(child.path) {
					continue
				}
			}
		} else if ft.filter.prFilesMode && ft.filter.prChanged.loaded() {
			// PR-files filter: skip files/dirs not in the PR-changed set.
			if child.isDir {
				if !ft.filter.prChanged.hasDir(child.path) {
					continue
				}
			} else {
				if !ft.filter.prChanged.hasPath(child.path) {
					continue
				}
			}
		} else if ft.filter.releaseCompareMode && ft.filter.releaseChanged.loaded() {
			// Release-compare filter: skip files/dirs not in the comparison set.
			if child.isDir {
				if !ft.filter.releaseChanged.hasDir(child.path) {
					continue
				}
			} else {
				if !ft.filter.releaseChanged.hasPath(child.path) {
					continue
				}
			}
		} else if ft.filter.branchFilesMode && ft.filter.branchChanged.loaded() {
			// Branch-files filter: skip files/dirs not in the branch-changed set.
			if child.isDir {
				if !ft.filter.branchChanged.hasDir(child.path) {
					continue
				}
			} else {
				if !ft.filter.branchChanged.hasPath(child.path) {
					continue
				}
			}
		} else if ft.filter.gitFilter && ft.gitChanged.loaded() {
			// Git filter: skip files not in the changed set, and skip
			// directories that contain no changed descendants.
			if child.isDir {
				if !ft.gitChanged.hasDir(child.path) {
					continue
				}
			} else {
				if !ft.gitChanged.hasPath(child.path) {
					continue
				}
			}
		}
		ft.visible = append(ft.visible, child)
		if child.isDir && child.expanded {
			ft.walkVisible(child)
		}
	}
}

func isHidden(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

// ---------------------------------------------------------------------------
// Refresh handler
// ---------------------------------------------------------------------------

// handleRefresh reloads the tree from disk and restarts the watcher command.
func (ft *FileTree) handleRefresh() (panels.Panel, tea.Cmd) {
	ft.reloadTree()

	cmds := []tea.Cmd{ft.loadGitFileStatus(), ft.loadGitIgnored()}

	// Restart the watcher to continue receiving events.
	if ft.watcher != nil && ft.ctx != nil {
		cmds = append(cmds, ft.watcher.start(ft.ctx))
	}
	return ft, tea.Batch(cmds...)
}

// reloadTree reloads the entire tree from disk, preserving the cursor
// position as much as possible.
func (ft *FileTree) reloadTree() {
	ft.clearDirSizeCache()

	// Remember the current cursor path for restoration.
	var cursorPath string
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		cursorPath = ft.visible[ft.viewport.cursor].path
	}

	// Collect expanded directories.
	expanded := ft.collectExpanded(ft.root)

	// Rebuild root.
	ft.root = &node{
		name:  filepath.Base(ft.rootPath),
		path:  ft.rootPath,
		isDir: true,
		depth: -1,
	}
	ft.loadChildren(ft.root)

	// Re-expand previously expanded directories.
	ft.restoreExpanded(ft.root, expanded)

	ft.rebuildVisible()

	// Restore cursor position.
	if cursorPath != "" {
		for i, n := range ft.visible {
			if n.path == cursorPath {
				ft.viewport.cursor = i
				ft.ensureCursorVisible()
				return
			}
		}
	}
}

// collectExpanded gathers paths of all expanded directories.
func (ft *FileTree) collectExpanded(n *node) map[string]bool {
	expanded := make(map[string]bool)
	var walk func(*node)
	walk = func(n *node) {
		if n.isDir && n.expanded {
			expanded[n.path] = true
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(n)
	return expanded
}

// restoreExpanded re-expands directories that were expanded before a reload.
func (ft *FileTree) restoreExpanded(n *node, expanded map[string]bool) {
	for _, child := range n.children {
		if child.isDir && expanded[child.path] {
			child.expanded = true
			ft.loadChildren(child)
			ft.restoreExpanded(child, expanded)
		}
	}
}

// ---------------------------------------------------------------------------
// Path safety
// ---------------------------------------------------------------------------

// isPathSafe checks that a symlink target resolves inside the repo root.
func (ft *FileTree) isPathSafe(symlinkPath string) bool {
	rootResolved, err := filepath.EvalSymlinks(ft.rootPath)
	if err != nil {
		rootResolved = filepath.Clean(ft.rootPath)
	}
	resolved, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		return false
	}
	resolved = filepath.Clean(resolved)

	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil {
		return false
	}
	// Outside root when relative path starts with "..".
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isSymlinkLoop detects whether expanding the symlink would create a cycle.
// A loop exists when the symlink target is an ancestor of (or equal to) the
// directory that contains the symlink.
func (ft *FileTree) isSymlinkLoop(symlinkPath string) bool {
	target, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		return true // cannot resolve → play safe
	}
	target = filepath.Clean(target)

	symlinkDir := filepath.Dir(symlinkPath)
	symlinkDirReal, err := filepath.EvalSymlinks(symlinkDir)
	if err != nil {
		return true
	}
	symlinkDirReal = filepath.Clean(symlinkDirReal)

	rel, err := filepath.Rel(target, symlinkDirReal)
	if err != nil {
		return true // cannot determine relationship → play safe
	}
	// If target is an ancestor of (or equal to) symlinkDir, rel will NOT start
	// with ".." — that means it's a loop.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// displayWidth returns the visible column width of s, treating Private Use
// Area (PUA) characters as width 2.  Terminals with nerd fonts render PUA
// glyphs as double-width even though Go's runewidth and lipgloss report
// them as width 1.
//
// This uses runewidth.RuneWidth directly instead of lipgloss.Width(string(r))
// to avoid per-rune string allocation. The input is always unstyled text
// (no ANSI sequences) so ANSI-aware parsing is unnecessary.
func displayWidth(s string) int {
	// Fast path: if no PUA characters are present, use the optimized
	// batch calculation from go-runewidth (zero allocations).
	hasPUA := false
	for _, r := range s {
		if r >= 0xE000 && r <= 0xF8FF ||
			r >= 0xF0000 && r <= 0xFFFFF ||
			r >= 0x100000 && r <= 0x10FFFF {
			hasPUA = true
			break
		}
	}
	if !hasPUA {
		return runewidth.StringWidth(s)
	}
	// Slow path: PUA characters need width 2 override.
	w := 0
	for _, r := range s {
		if r >= 0xE000 && r <= 0xF8FF ||
			r >= 0xF0000 && r <= 0xFFFFF ||
			r >= 0x100000 && r <= 0x10FFFF {
			w += 2
		} else {
			w += runewidth.RuneWidth(r)
		}
	}
	return w
}

// runeDisplayWidth returns the visible column width of a single rune.
func runeDisplayWidth(r rune) int {
	if r >= 0xE000 && r <= 0xF8FF ||
		r >= 0xF0000 && r <= 0xFFFFF ||
		r >= 0x100000 && r <= 0x10FFFF {
		return 2
	}
	return runewidth.RuneWidth(r)
}

// truncateToWidth truncates s so its display width does not exceed maxW.
// If truncation is needed, the last visible character is replaced with "…".
func truncateToWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w := displayWidth(s)
	if w <= maxW {
		return s
	}
	runes := []rune(s)
	cum := 0
	for i, r := range runes {
		rw := runeDisplayWidth(r)
		if cum+rw > maxW-1 {
			return string(runes[:i]) + "…"
		}
		cum += rw
	}
	return s
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func (ft *FileTree) renderLine(n *node, width int, isCursor bool) string {
	b, _ := builderPool.Get().(*strings.Builder)
	if b == nil {
		b = new(strings.Builder)
	}
	b.Reset()
	defer builderPool.Put(b)

	if ft.listMode {
		// List mode: show relative path, no indentation or tree connectors.
		rel, err := filepath.Rel(ft.rootPath, n.path)
		if err != nil {
			rel = n.name
		}
		if ft.cfg.GetShowIcons() {
			if icon := getFileIcon(n.name, n.isDir, n.expanded, ft.cfg.GetIconMode()); icon != "" {
				b.WriteString(icon)
				b.WriteByte(' ')
			}
		}
		b.WriteString(rel)
		if n.isDir {
			b.WriteByte(filepath.Separator)
		}
	} else {
		// Tree mode: indentation + expand/collapse + icons + name.
		b.WriteString(strings.Repeat("  ", n.depth))

		if n.isDir {
			b.WriteString(getExpandIcon(n.expanded, ft.cfg.GetIconMode()))
			b.WriteByte(' ')
		} else {
			b.WriteString("  ")
		}

		if ft.cfg.GetShowIcons() {
			if icon := getFileIcon(n.name, n.isDir, n.expanded, ft.cfg.GetIconMode()); icon != "" {
				b.WriteString(icon)
				b.WriteByte(' ')
			}
		}

		b.WriteString(n.name)

		if n.isDir && n.loadErr != nil {
			b.WriteString(" [error]")
		}

		if n.isSymlink && n.symlinkTarget != "" {
			b.WriteString(" → ")
			b.WriteString(n.symlinkTarget)
		}
	}

	content := b.String()

	// Determine foreground color.
	fg := ft.colors.Default
	ignored := ft.isPathIgnored(n.path)
	switch {
	case ignored:
		fg = ft.colors.Dim
	case n.isDir:
		fg = ft.colors.Directory
	case n.isSymlink:
		fg = ft.colors.Symlink
	case n.isExecutable:
		fg = ft.colors.Executable
	}

	// Determine git status indicator for this node.
	var gitIndicator string
	var gitColor string
	gitIndicatorW := 0 // visible column width of " " + indicator
	if ft.gitClient != nil && !n.isDir && ft.gitFileStatus != nil {
		if indicator, ok := ft.gitFileStatus[n.path]; ok {
			gitColor = "#D4D4D4"
			switch indicator {
			case "M":
				gitColor = "#C9875A"
			case "A":
				gitColor = "#6B9E56"
			case "D":
				gitColor = "#C44B4B"
			case "?":
				gitColor = "#555555"
			case "R", "C":
				gitColor = "#7A9EBF"
			case "U":
				gitColor = "#C9A227"
			}
			gitIndicator = gitStatusIcon(indicator, ft.cfg.GetIconMode())
			gitIndicatorW = 1 + displayWidth(gitIndicator) // " " + icon
		}
	}
	// Show ignored indicator when no other git status is present.
	if ignored && gitIndicator == "" {
		gitIndicator = gitStatusIcon("!", ft.cfg.GetIconMode())
		gitColor = ft.colors.Dim
		gitIndicatorW = 1 + displayWidth(gitIndicator)
	}

	var sizeText string
	sizeIndicatorW := 0
	if size, ok := ft.dirSizeCache[n.path]; ok {
		sizeText = humanizeSize(size)
		sizeIndicatorW = 1 + displayWidth(sizeText)
	}

	// Truncate content, reserving space for size and git indicators when present.
	availW := width
	reservedW := sizeIndicatorW + gitIndicatorW
	if reservedW > 0 {
		availW = width - reservedW
		if availW < 1 {
			availW = 1
		}
	}
	content = truncateToWidth(content, availW)

	// Pad content with spaces to fill availW exactly (manual padding, no
	// lipgloss Width which can cause wrapping).
	visW := displayWidth(content)
	if visW < availW {
		content += strings.Repeat(" ", availW-visW)
	}

	mainContent := content
	if sizeText != "" {
		mainContent += " " + sizeText
	}
	content = mainContent
	// Append git indicator before styling.
	if gitIndicator != "" {
		content += " " + gitIndicator
	}

	// Hard-truncate the combined line to exactly width columns.
	content = truncateToWidth(content, width)

	// Apply colours.
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	if ft.selected[n.path] {
		style = style.Background(lipgloss.Color(ft.colors.SelectedBg))
	}
	if isCursor && ft.focused {
		style = style.Background(lipgloss.Color(ft.colors.CursorBg)).Bold(true)
	}

	// Render the content line. If there's a git indicator, render the main
	// part and the indicator separately so they can have different colours.
	if gitIndicator != "" {
		// Split back: main part is everything except last (1 + icon_width) chars.
		mainPartW := width - gitIndicatorW
		if mainPartW < 1 {
			mainPartW = 1
		}
		mainPart := truncateToWidth(mainContent, mainPartW)
		mainVisW := displayWidth(mainPart)
		if mainVisW < mainPartW {
			mainPart += strings.Repeat(" ", mainPartW-mainVisW)
		}
		line := style.Render(mainPart)
		line += lipgloss.NewStyle().
			Foreground(lipgloss.Color(gitColor)).
			Bold(true).
			Render(" " + gitIndicator)
		return line
	}

	return style.Render(content)
}

// ---------------------------------------------------------------------------
// Cursor path helpers
// ---------------------------------------------------------------------------

// cursorPath returns the absolute path of the node at the current cursor
// position. Returns "" when the visible list is empty or cursor is out of
// range.
func (ft *FileTree) CursorPath() string {
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		return ft.visible[ft.viewport.cursor].path
	}
	return ""
}

// CursorIsDir reports whether the node under the cursor is a directory.
func (ft *FileTree) CursorIsDir() bool {
	if ft.viewport.cursor >= 0 && ft.viewport.cursor < len(ft.visible) {
		return ft.visible[ft.viewport.cursor].isDir
	}
	return false
}

// restoreCursorToPath moves the cursor to the node matching the given path.
// If the path is not found in the current visible list, the cursor is
// clamped to valid bounds.
func (ft *FileTree) restoreCursorToPath(path string) {
	if path == "" {
		return
	}
	for i, n := range ft.visible {
		if n.path == path {
			ft.viewport.cursor = i
			ft.ensureCursorVisible()
			return
		}
	}
	// Path not in current view — clamp cursor.
	if ft.viewport.cursor >= len(ft.visible) {
		ft.viewport.cursor = len(ft.visible) - 1
	}
	if ft.viewport.cursor < 0 {
		ft.viewport.cursor = 0
	}
	ft.ensureCursorVisible()
}

// revealFile expands all parent directories of the given path and
// positions the cursor on it. Used when the fuzzy finder selects a file.
func (ft *FileTree) revealFile(path string) {
	if path == "" || ft.root == nil {
		return
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}

	// Build the list of path segments from root to the target.
	rel, err := filepath.Rel(ft.rootPath, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")

	// Walk the tree, loading and expanding each directory segment.
	current := ft.root
	for i, part := range parts {
		if !current.loaded {
			ft.loadChildren(current)
		}
		current.expanded = true

		var found *node
		for _, child := range current.children {
			if child.name == part {
				found = child
				break
			}
		}
		if found == nil {
			return // path segment not found in tree
		}

		if i < len(parts)-1 {
			// Intermediate directory — continue walking.
			current = found
		} else {
			// Final segment — rebuild visible and position cursor.
			ft.rebuildVisible()
			ft.restoreCursorToPath(found.path)
		}
	}
}

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package) (F22)
// ---------------------------------------------------------------------------

// cursor returns the current cursor index.
func (ft *FileTree) cursorIndex() int { return ft.viewport.cursor }

// visibleCount returns the number of currently visible nodes.
func (ft *FileTree) visibleCount() int { return len(ft.visible) }

// visibleName returns the name of the node at the given visible index.
func (ft *FileTree) visibleName(i int) string {
	if i < 0 || i >= len(ft.visible) {
		return ""
	}
	return ft.visible[i].name
}

// visiblePath returns the path of the node at the given visible index.
func (ft *FileTree) visiblePath(i int) string {
	if i < 0 || i >= len(ft.visible) {
		return ""
	}
	return ft.visible[i].path
}

// isSelected returns whether the path is in the multi-select set.
func (ft *FileTree) isPathSelected(path string) bool { return ft.selected[path] }

// showHiddenState returns the current hidden-file visibility state.
func (ft *FileTree) showHiddenState() bool { return ft.showHidden }
