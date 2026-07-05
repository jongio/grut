// Package gitstatus implements the Git Status panel for grut.
// It displays files grouped by stage status (Staged, Unstaged, Untracked)
// with support for file-level staging/unstaging, inline diff expansion,
// hunk-level and line-level staging, and cursor-based navigation.
package gitstatus

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

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

// ---------------------------------------------------------------------------
// Internal message types (async result messages)
// ---------------------------------------------------------------------------
// statusLoadedMsg carries the result of an async git-status call.
type statusLoadedMsg struct {
	err        error
	files      []git.FileStatus
	generation uint64 // monotonic counter to discard stale results
}

// diffLoadedMsg carries the result of an async diff call for a file.
type diffLoadedMsg struct {
	err        error
	path       string
	hunks      []git.Hunk
	generation uint64
}

// stageResultMsg carries the result of a stage/unstage operation.
type stageResultMsg struct {
	err error
}

// discardResultMsg carries the result of a discard operation.
type discardResultMsg struct {
	err error
}

// ---------------------------------------------------------------------------
// View mode
// ---------------------------------------------------------------------------
// viewMode tracks the current interaction granularity.
type viewMode int

const (
	modeFile viewMode = iota // navigate files
	modeHunk                 // navigate hunks within an expanded file
	modeLine                 // navigate lines within a hunk
)

// maxDiffCacheEntries caps the number of cached inline diffs to prevent
// unbounded memory growth during long sessions.
const maxDiffCacheEntries = 50

// ---------------------------------------------------------------------------
// Section groups
// ---------------------------------------------------------------------------
// section is a logical group header in the status list.
type section int

const (
	sectionStaged    section = iota
	sectionUnstaged  section = iota
	sectionUntracked section = iota
)

func (s section) String() string {
	switch s {
	case sectionStaged:
		return "Staged"
	case sectionUnstaged:
		return "Unstaged"
	case sectionUntracked:
		return "Untracked"
	default:
		return "Unknown"
	}
}

// ---------------------------------------------------------------------------
// Visible row — each row is either a section header, a file entry,
// a hunk header, or a diff line.
// ---------------------------------------------------------------------------
type rowKind int

const (
	rowSection  rowKind = iota // section header
	rowFile     rowKind = iota // file entry
	rowHunk     rowKind = iota // hunk header within expanded diff
	rowDiffLine rowKind = iota // single diff line within a hunk
)

type row struct {
	file      *git.FileStatus // non-nil for rowFile, rowHunk, rowDiffLine
	diffLine  *git.DiffLine   // pointer to the actual diff line (for rowDiffLine)
	hunkEntry *git.Hunk       // pointer to the actual hunk (for rowHunk)
	hunks     []git.Hunk      // cached diff hunks for expanded files
	kind      rowKind
	section   section // which section this row belongs to
	hunkIdx   int     // hunk index (for rowHunk, rowDiffLine)
	lineIdx   int     // line index within hunk (for rowDiffLine)
	expanded  bool    // file expanded to show diff
	selected  bool    // marked for bulk ops
}

// ---------------------------------------------------------------------------
// GitStatus panel
// ---------------------------------------------------------------------------
// GitClient is the subset of git.GitClient used by this panel.
// It's intentionally narrow — only the methods the panel actually needs —
// making it easy to mock in tests without implementing unused methods.
type GitClient interface {
	Status(ctx context.Context) ([]git.FileStatus, error)
	Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	Stage(ctx context.Context, paths []string) error
	Unstage(ctx context.Context, paths []string) error
	StageHunk(ctx context.Context, path string, hunk git.Hunk) error
	UnstageHunk(ctx context.Context, path string, hunk git.Hunk) error
	StageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	UnstageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	DiscardFile(ctx context.Context, path string) error
	CleanPreview(ctx context.Context, opts git.CleanOpts) ([]git.CleanCandidate, error)
	Clean(ctx context.Context, opts git.CleanOpts) error
}

// GitStatus is the git status panel. It implements [panels.Panel].
type GitStatus struct {
	actionsCfg config.ActionsConfig
	git        GitClient
	ctx        context.Context
	err        error           // last error from loading status
	selected   map[string]bool // file paths selected for bulk ops
	// Per-file diff expansion and cache.
	expandedFiles  map[string]bool       // paths of expanded files
	diffCache      map[string][]git.Hunk // cached diffs keyed by path
	diffCacheOrder []string              // insertion-order keys for LRU eviction
	activeFile     string                // path of the file in hunk/line mode
	pendingOp      string
	pendingName    string
	pendingPath    string           // file path for pending destructive ops (e.g. discard)
	files          []git.FileStatus // latest status from git
	rows           []row            // flattened visible rows
	panels.BasePanel
	cursor     int      // index into rows
	offset     int      // viewport scroll offset
	mode       viewMode // current interaction mode
	hunkCursor int      // active hunk index within expanded file
	lineCursor int      // active line index within active hunk
	// Generation counters to discard stale async results (CWE-362).
	statusGen uint64 // incremented on each status load request
	diffGen   uint64 // incremented on each diff load request
	loading   bool   // true while an async status load is in flight
	rowsDirty bool   // true when rows need rebuilding before next render
	theme     *theme.Theme
	colors    panelColors
	// Cached lipgloss styles for hot-path rendering (avoid per-row allocations).
	styleSectionHeader lipgloss.Style
	styleHunkHeader    lipgloss.Style
	styleAdded         lipgloss.Style
	styleRemoved       lipgloss.Style
	styleDim           lipgloss.Style
	styleStaged        lipgloss.Style
	styleUnstaged      lipgloss.Style
	styleUntracked     lipgloss.Style
	styleDefault       lipgloss.Style
	// Clean overlay state: preview of untracked files and their selection.
	cleanCandidates     []git.CleanCandidate
	cleanSelected       map[string]bool
	cleanCursor         int
	cleanOffset         int
	cleanActive         bool
	cleanLoading        bool
	cleanIncludeIgnored bool
}

// Compile-time interface check.
var _ panels.Panel = (*GitStatus)(nil)

// New creates a new GitStatus panel.
func New(client GitClient, th *theme.Theme) *GitStatus {
	colors := initColors(th)
	p := &GitStatus{
		BasePanel:     panels.BasePanel{PanelTitle: "gitstatus"},
		git:           client,
		selected:      make(map[string]bool),
		expandedFiles: make(map[string]bool),
		diffCache:     make(map[string][]git.Hunk),
		cleanSelected: make(map[string]bool),
		theme:         th,
		colors:        colors,
	}
	p.rebuildContentStyles()
	return p
}

// rebuildContentStyles initializes or re-creates the cached foreground styles
// used in the render hot path. Call once at construction and again if the theme
// changes.
func (p *GitStatus) rebuildContentStyles() {
	p.styleSectionHeader = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.SectionHeader)).Bold(true)
	p.styleHunkHeader = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.HunkHeader))
	p.styleAdded = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Added))
	p.styleRemoved = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Removed))
	p.styleDim = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim))
	p.styleStaged = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Staged))
	p.styleUnstaged = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Unstaged))
	p.styleUntracked = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Untracked))
	p.styleDefault = lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Default))
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *GitStatus) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	p.loading = true
	return p.loadStatusCmd()
}

// handleRepoChanged replaces the git client and reloads status for the new
// repository after a directory change.
func (p *GitStatus) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
		p.files = nil
		p.rows = nil
		p.err = nil
		p.cursor = 0
		p.offset = 0
		p.selected = make(map[string]bool)
		p.expandedFiles = make(map[string]bool)
		p.diffCache = make(map[string][]git.Hunk)
		p.loading = false
		p.mode = 0
		p.activeFile = ""
		return p, nil
	}
	p.git = client
	p.files = nil
	p.rows = nil
	p.err = nil
	p.cursor = 0
	p.offset = 0
	p.selected = make(map[string]bool)
	p.expandedFiles = make(map[string]bool)
	p.diffCache = make(map[string][]git.Hunk)
	p.mode = 0
	p.activeFile = ""
	p.loading = true
	return p, p.loadStatusCmd()
}

// Update implements panels.Panel.
func (p *GitStatus) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		// Discard stale results from older load requests.
		if msg.generation != p.statusGen {
			return p, nil
		}
		p.loading = false
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.files = msg.files
		p.rebuildRows()
		p.rowsDirty = false
		return p, p.emitStatusChanged()
	case diffLoadedMsg:
		// Discard stale diff results.
		if msg.generation != p.diffGen {
			return p, nil
		}
		if msg.err != nil {
			return p, nil
		}
		p.diffCache[msg.path] = msg.hunks
		p.diffCacheOrder = append(p.diffCacheOrder, msg.path)
		// Evict oldest half of entries if cache exceeds limit (LRU).
		if len(p.diffCache) > maxDiffCacheEntries {
			evictCount := len(p.diffCacheOrder) / 2
			for _, key := range p.diffCacheOrder[:evictCount] {
				delete(p.diffCache, key)
			}
			p.diffCacheOrder = p.diffCacheOrder[evictCount:]
		}
		p.rebuildRows()
		p.rowsDirty = false
		return p, nil
	case stageResultMsg:
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		// Invalidate diff caches for the affected file so stale hunks
		// are not reused after partial staging/unstaging.
		p.invalidateDiffCaches()
		// Refresh status after staging/unstaging.
		p.loading = true
		return p, p.loadStatusCmd()
	case discardResultMsg:
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.invalidateDiffCaches()
		p.loading = true
		return p, p.loadStatusCmd()
	case cleanPreviewLoadedMsg:
		return p.handleCleanPreviewLoaded(msg)
	case cleanResultMsg:
		return p.handleCleanResult(msg)
	case panels.RefreshGitStatusMsg:
		p.loading = true
		return p, p.loadStatusCmd()
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleMouseDoubleClick(msg)
	case panels.PanelMouseRightClickMsg:
		return p.handleMouseRightClick(msg)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	}
	return p, nil
}

// View implements panels.Panel.
func (p *GitStatus) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.cleanActive {
		return p.renderCleanOverlay(width, height)
	}
	// Rebuild rows only when the underlying data has changed.
	if p.rowsDirty {
		p.rebuildRows()
		p.rowsDirty = false
	}
	if p.loading && len(p.rows) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("Loading git status...")
	}
	if p.err != nil && len(p.rows) == 0 {
		slog.Warn("git status panel error", "err", p.err)
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Removed)).
			Render("Could not load git status")
	}
	if len(p.rows) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Staged)).
			Render("Working tree clean")
	}
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > len(p.rows) {
		end = len(p.rows)
	}
	// Pre-compute row background styles once per frame instead of per row.
	rs := rowStyles{
		cursor:   lipgloss.NewStyle().Width(width).Background(lipgloss.Color(p.colors.CursorBg)),
		selected: lipgloss.NewStyle().Width(width).Background(lipgloss.Color(p.colors.SelectedBg)),
		normal:   lipgloss.NewStyle().Width(width),
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderRow(&p.rows[i], width, i == p.cursor, &rs))
	}
	// Pad remaining height with blank lines.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (p *GitStatus) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "s", Description: "Stage file/hunk/line", Action: "stage"},
		{Key: "u", Description: "Unstage file/hunk/line", Action: "unstage"},
		{Key: "enter/l", Description: "Expand file diff", Action: "expand"},
		{Key: "h", Description: "Enter hunk mode", Action: "hunk_mode"},
		{Key: "d", Description: "Discard unstaged changes", Action: "discard"},
		{Key: "y", Description: "Copy hunk (or file path) to clipboard", Action: "copy"},
		{Key: "space", Description: "Toggle select for bulk", Action: "toggle_select"},
		{Key: "a", Description: "Stage all", Action: "stage_all"},
		{Key: "U", Description: "Unstage all", Action: "unstage_all"},
		{Key: "R", Description: "Refresh status", Action: "refresh"},
		{Key: "X", Description: "Clean untracked files (preview and select)", Action: "clean"},
		{Key: "Esc", Description: "Exit hunk/line mode", Action: "escape"},
	}
}

// ---------------------------------------------------------------------------
// Async commands (never call git in Update synchronously)
// ---------------------------------------------------------------------------
func (p *GitStatus) loadStatusCmd() tea.Cmd {
	p.statusGen++
	gen := p.statusGen
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		files, err := client.Status(ctx)
		return statusLoadedMsg{files: files, err: err, generation: gen}
	}
}

func (p *GitStatus) loadDiffCmd(diffKey string, path string, staged bool) tea.Cmd {
	p.diffGen++
	gen := p.diffGen
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		diffs, err := client.Diff(ctx, git.DiffOpts{
			Staged: staged,
			Path:   path,
		})
		var hunks []git.Hunk
		if err == nil && len(diffs) > 0 {
			hunks = diffs[0].Hunks
		}
		return diffLoadedMsg{path: diffKey, hunks: hunks, err: err, generation: gen}
	}
}

func (p *GitStatus) stageCmd(paths []string) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.Stage(ctx, paths)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) unstageCmd(paths []string) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.Unstage(ctx, paths)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) stageHunkCmd(path string, hunk git.Hunk) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.StageHunk(ctx, path, hunk)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) unstageHunkCmd(path string, hunk git.Hunk) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.UnstageHunk(ctx, path, hunk)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) stageLineCmd(path string, hunk git.Hunk, lineIdx int) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.StageLine(ctx, path, hunk, lineIdx)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) unstageLineCmd(path string, hunk git.Hunk, lineIdx int) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.UnstageLine(ctx, path, hunk, lineIdx)
		return stageResultMsg{err: err}
	}
}

func (p *GitStatus) emitStatusChanged() tea.Cmd {
	files := p.files
	return func() tea.Msg {
		return panels.GitStatusChangedMsg{Files: files}
	}
}

// ---------------------------------------------------------------------------
// Classification helpers
// ---------------------------------------------------------------------------
// classifyFiles groups files into staged, unstaged, and untracked lists.
func classifyFiles(files []git.FileStatus) (staged, unstaged, untracked []git.FileStatus) {
	for i := range files {
		f := &files[i]
		switch f.StagedStatus {
		case git.StatusUntracked:
			untracked = append(untracked, *f)
		default:
			if f.StagedStatus != git.StatusUnmodified {
				staged = append(staged, *f)
			}
			if f.WorktreeStatus != git.StatusUnmodified &&
				f.StagedStatus != git.StatusUntracked {
				unstaged = append(unstaged, *f)
			}
		}
	}
	return staged, unstaged, untracked
}

// ---------------------------------------------------------------------------
// Row rebuild
// ---------------------------------------------------------------------------
func (p *GitStatus) rebuildRows() {
	staged, unstaged, untracked := classifyFiles(p.files)
	var rows []row
	if len(staged) > 0 {
		rows = append(rows, row{kind: rowSection, section: sectionStaged})
		for i := range staged {
			fr := row{
				kind:     rowFile,
				section:  sectionStaged,
				file:     &staged[i],
				expanded: p.expandedFiles[staged[i].Path+":staged"],
				selected: p.selected[staged[i].Path+":staged"],
			}
			if fr.expanded {
				fr.hunks = p.diffCache[staged[i].Path+":staged"]
			}
			rows = append(rows, fr)
			if fr.expanded {
				rows = append(rows, p.buildDiffRows(fr.hunks, sectionStaged, &staged[i])...)
			}
		}
	}
	if len(unstaged) > 0 {
		rows = append(rows, row{kind: rowSection, section: sectionUnstaged})
		for i := range unstaged {
			fr := row{
				kind:     rowFile,
				section:  sectionUnstaged,
				file:     &unstaged[i],
				expanded: p.expandedFiles[unstaged[i].Path+":unstaged"],
				selected: p.selected[unstaged[i].Path+":unstaged"],
			}
			if fr.expanded {
				fr.hunks = p.diffCache[unstaged[i].Path+":unstaged"]
			}
			rows = append(rows, fr)
			if fr.expanded {
				rows = append(rows, p.buildDiffRows(fr.hunks, sectionUnstaged, &unstaged[i])...)
			}
		}
	}
	if len(untracked) > 0 {
		rows = append(rows, row{kind: rowSection, section: sectionUntracked})
		for i := range untracked {
			fr := row{
				kind:     rowFile,
				section:  sectionUntracked,
				file:     &untracked[i],
				selected: p.selected[untracked[i].Path],
			}
			rows = append(rows, fr)
		}
	}
	p.rows = rows
	// Clamp cursor.
	if p.cursor >= len(p.rows) {
		p.cursor = len(p.rows) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	// Skip section headers so the cursor always lands on an actionable row.
	if p.cursor < len(p.rows) && p.rows[p.cursor].kind == rowSection {
		if p.cursor < len(p.rows)-1 {
			p.cursor++
		}
	}
}

func (p *GitStatus) buildDiffRows(hunks []git.Hunk, sec section, file *git.FileStatus) []row {
	var rows []row
	for hi := range hunks {
		h := &hunks[hi]
		rows = append(rows, row{
			kind:      rowHunk,
			section:   sec,
			file:      file,
			hunkIdx:   hi,
			hunkEntry: h,
		})
		for li := range h.Lines {
			dl := &h.Lines[li]
			rows = append(rows, row{
				kind:     rowDiffLine,
				section:  sec,
				file:     file,
				hunkIdx:  hi,
				lineIdx:  li,
				diffLine: dl,
			})
		}
	}
	return rows
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the item at the clicked row, skipping section headers.
func (p *GitStatus) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.rows) {
		return p, nil
	}
	// Skip section headers — they are not selectable.
	if p.rows[idx].kind == rowSection {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick stages or unstages the file at the double-clicked row.
func (p *GitStatus) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.rows) {
		return p, nil
	}
	if p.rows[idx].kind == rowSection {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	itemType := actions.ItemStatusFile
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.clearPending()
		p.pendingOp = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
	opDiscard         = "discard"
	opClean           = "clean"
)

// SetActionsCfg stores the actions configuration for right-click menus.
func (p *GitStatus) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// handleMouseRightClick opens the context menu for the file at the clicked row.
func (p *GitStatus) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.rows) {
		return p, nil
	}
	if p.rows[idx].kind == rowSection {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	r := &p.rows[idx]
	if r.file == nil {
		return p, nil
	}
	label := r.file.Path
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemStatusFile, label)
	if cmd != nil {
		p.clearPending()
		p.pendingOp = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// clearPending resets all pending-operation state so that no stale values
// leak across interactions. Call this before setting new pending state.
func (p *GitStatus) clearPending() {
	p.pendingOp = ""
	p.pendingName = ""
	p.pendingPath = ""
}

// handleModalResult dispatches the result of an action-picker modal.
func (p *GitStatus) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pendingOp
	name := p.pendingName
	path := p.pendingPath
	p.clearPending()
	if !msg.Accept {
		return p, nil
	}
	switch op {
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, name, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opDiscard:
		return p, p.discardCmd(path)
	case opClean:
		return p, p.cleanSelectedCmd()
	}
	return p, nil
}

// executeRightClickAction runs the selected right-click action.
func (p *GitStatus) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionExpandDiff:
		return p.expandOrEnter()
	case actions.ActionStageUnstage:
		if p.cursor >= 0 && p.cursor < len(p.rows) {
			if p.rows[p.cursor].section == sectionStaged {
				return p.unstageAtCursor()
			}
		}
		return p.stageAtCursor()
	case actions.ActionCopyPath:
		return p.copyPath()
	}
	return p, nil
}

// copyPath copies the file path at the cursor to the OS clipboard.
func (p *GitStatus) copyPath() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	if r.file == nil {
		return p, nil
	}
	path := r.file.Path
	if err := panels.CopyToClipboard(p.ctx, path); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Copy failed: " + errMsg,
				Level:   notify.Error,
			}
		}
	}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Copied: " + path,
			Level:   notify.Success,
		}
	}
}

// copyAtCursor copies the hunk under the cursor when the cursor is on a hunk
// or diff line, and otherwise falls back to copying the file path. This keeps
// "y" consistent with the copy-path binding used elsewhere while adding hunk
// copying inside an expanded diff.
func (p *GitStatus) copyAtCursor() (panels.Panel, tea.Cmd) {
	if hunk, ok := p.hunkAtCursor(); ok {
		return p.copyHunk(hunk)
	}
	return p.copyPath()
}

// hunkAtCursor returns the hunk the cursor is currently on, whether the cursor
// sits on the hunk header row or one of its diff lines.
func (p *GitStatus) hunkAtCursor() (git.Hunk, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return git.Hunk{}, false
	}
	r := &p.rows[p.cursor]
	switch r.kind {
	case rowHunk:
		if r.hunkEntry != nil {
			return *r.hunkEntry, true
		}
	case rowDiffLine:
		hunks := p.diffCache[p.fileKey(r)]
		if r.hunkIdx >= 0 && r.hunkIdx < len(hunks) {
			return hunks[r.hunkIdx], true
		}
	case rowSection, rowFile:
		return git.Hunk{}, false
	}
	return git.Hunk{}, false
}

// copyHunk writes the hunk as a unified diff to the OS clipboard.
func (p *GitStatus) copyHunk(hunk git.Hunk) (panels.Panel, tea.Cmd) {
	text := hunkText(hunk)
	if err := panels.CopyToClipboard(p.ctx, text); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	n := len(hunk.Lines)
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: fmt.Sprintf("Copied hunk (%d lines)", n),
			Level:   notify.Success,
		}
	}
}

// hunkText renders a hunk back into unified diff text, restoring the leading
// space, "+" or "-" that identifies each line. When the parsed header is
// missing it is rebuilt from the hunk's line ranges.
func hunkText(h git.Hunk) string {
	header := h.Header
	if header == "" {
		header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	for _, line := range h.Lines {
		switch line.Type {
		case git.DiffLineContext:
			b.WriteString(" ")
		case git.DiffLineAdded:
			b.WriteString("+")
		case git.DiffLineRemoved:
			b.WriteString("-")
		}
		b.WriteString(line.Content)
		b.WriteString("\n")
	}
	if h.NoNewlineEOF {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}

// handleMouseWheel scrolls the git status viewport.
func (p *GitStatus) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.rows) - p.Height
		if maxOffset < 0 {
			maxOffset = 0
		}
		p.offset += panels.ScrollDelta
		if p.offset > maxOffset {
			p.offset = maxOffset
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *GitStatus) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}
	key := msg.String()
	if p.cleanActive {
		return p.handleCleanKey(msg)
	}
	// Escape from hunk/line mode back to file mode.
	if key == "esc" || key == "escape" {
		if p.mode != modeFile {
			p.mode = modeFile
			p.activeFile = ""
			return p, nil
		}
		return p, nil
	}
	switch key {
	case "j", "down":
		p.moveCursorDown()
	case "k", "up":
		p.moveCursorUp()
	case "G":
		p.goToBottom()
	case "g":
		p.goToTop()
	case "enter", "l":
		return p.expandOrEnter()
	case "h":
		return p.enterHunkMode()
	case "s":
		return p.stageAtCursor()
	case "u":
		return p.unstageAtCursor()
	case " ", "space":
		p.toggleSelection()
	case "a":
		return p.stageAll()
	case "U":
		return p.unstageAll()
	case "d":
		return p.discardAtCursor()
	case "y":
		return p.copyAtCursor()
	case "R":
		p.loading = true
		p.expandedFiles = make(map[string]bool)
		p.diffCache = make(map[string][]git.Hunk)
		p.mode = modeFile
		return p, p.loadStatusCmd()
	case "X":
		return p.openCleanOverlay()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (p *GitStatus) moveCursorDown() {
	if p.cursor < len(p.rows)-1 {
		p.cursor++
		// Skip section headers.
		if p.cursor < len(p.rows) && p.rows[p.cursor].kind == rowSection {
			if p.cursor < len(p.rows)-1 {
				p.cursor++
			}
		}
		p.ensureCursorVisible()
	}
}

func (p *GitStatus) moveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
		// Skip section headers.
		if p.cursor >= 0 && p.rows[p.cursor].kind == rowSection {
			if p.cursor > 0 {
				p.cursor--
			}
		}
		p.ensureCursorVisible()
	}
}

func (p *GitStatus) goToTop() {
	p.cursor = 0
	if len(p.rows) > 0 && p.rows[0].kind == rowSection {
		if len(p.rows) > 1 {
			p.cursor = 1
		}
	}
	p.ensureCursorVisible()
}

func (p *GitStatus) goToBottom() {
	if n := len(p.rows); n > 0 {
		p.cursor = n - 1
		p.ensureCursorVisible()
	}
}

func (p *GitStatus) ensureCursorVisible() {
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, p.Height)
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
func (p *GitStatus) expandOrEnter() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	if r.kind != rowFile {
		// In hunk/line mode, entering line mode from hunk is via 'l'.
		return p, nil
	}
	// Untracked files cannot be expanded (no diff).
	if r.section == sectionUntracked {
		return p, nil
	}
	key := p.fileKey(r)
	if p.expandedFiles[key] {
		// Collapse.
		delete(p.expandedFiles, key)
		delete(p.diffCache, key)
		p.rebuildRows()
		p.rowsDirty = false
		return p, nil
	}
	// Expand — load diff.
	p.expandedFiles[key] = true
	staged := r.section == sectionStaged
	path := r.file.Path
	diffKey := key
	// If we already have cached hunks, just rebuild.
	if _, ok := p.diffCache[diffKey]; ok {
		p.rebuildRows()
		p.rowsDirty = false
		return p, nil
	}
	return p, p.loadDiffCmd(diffKey, path, staged)
}

func (p *GitStatus) enterHunkMode() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	switch r.kind { //nolint:exhaustive // only relevant cases handled
	case rowFile:
		// Only enter hunk mode if file is expanded and has hunks.
		key := p.fileKey(r)
		if !p.expandedFiles[key] {
			return p, nil
		}
		hunks := p.diffCache[key]
		if len(hunks) == 0 {
			return p, nil
		}
		p.mode = modeHunk
		p.activeFile = key
		p.hunkCursor = 0
		// Move cursor to first hunk row.
		for i := p.cursor + 1; i < len(p.rows); i++ {
			if p.rows[i].kind == rowHunk {
				p.cursor = i
				p.ensureCursorVisible()
				break
			}
		}
		return p, nil
	case rowHunk:
		// Enter line mode from hunk.
		p.mode = modeLine
		p.activeFile = p.fileKey(r)
		p.hunkCursor = r.hunkIdx
		p.lineCursor = 0
		// Move cursor to first non-context diff line if possible.
		for i := p.cursor + 1; i < len(p.rows); i++ {
			if p.rows[i].kind != rowDiffLine {
				break // hit a non-diff row (next hunk or file)
			}
			if p.rows[i].diffLine != nil && p.rows[i].diffLine.Type != git.DiffLineContext {
				p.cursor = i
				p.ensureCursorVisible()
				break
			}
		}
		return p, nil
	}
	return p, nil
}

func (p *GitStatus) stageAtCursor() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	switch r.kind { //nolint:exhaustive // only relevant cases handled
	case rowSection:
		return p.stageSectionFiles(r.section)
	case rowFile:
		if r.file == nil {
			return p, nil
		}
		return p, p.stageCmd([]string{r.file.Path})
	case rowHunk:
		if r.file == nil || r.hunkEntry == nil {
			return p, nil
		}
		if r.section == sectionUnstaged {
			return p, p.stageHunkCmd(r.file.Path, *r.hunkEntry)
		}
		return p, p.stageCmd([]string{r.file.Path})
	case rowDiffLine:
		if r.file == nil || r.diffLine == nil {
			return p, nil
		}
		if r.section == sectionUnstaged && r.diffLine.Type != git.DiffLineContext {
			key := p.fileKey(r)
			hunks := p.diffCache[key]
			if hunks == nil {
				// Cache miss — cannot stage individual line without cached hunks.
				return p, nil
			}
			if r.hunkIdx >= 0 && r.hunkIdx < len(hunks) {
				return p, p.stageLineCmd(r.file.Path, hunks[r.hunkIdx], r.lineIdx)
			}
			return p, nil
		}
		return p, p.stageCmd([]string{r.file.Path})
	}
	return p, nil
}

func (p *GitStatus) unstageAtCursor() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	switch r.kind { //nolint:exhaustive // only relevant cases handled
	case rowSection:
		return p.unstageSectionFiles(r.section)
	case rowFile:
		if r.file == nil {
			return p, nil
		}
		return p, p.unstageCmd([]string{r.file.Path})
	case rowHunk:
		if r.file == nil || r.hunkEntry == nil {
			return p, nil
		}
		if r.section == sectionStaged {
			return p, p.unstageHunkCmd(r.file.Path, *r.hunkEntry)
		}
		return p, p.unstageCmd([]string{r.file.Path})
	case rowDiffLine:
		if r.file == nil || r.diffLine == nil {
			return p, nil
		}
		if r.section == sectionStaged && r.diffLine.Type != git.DiffLineContext {
			key := p.fileKey(r)
			hunks := p.diffCache[key]
			if r.hunkIdx >= 0 && r.hunkIdx < len(hunks) {
				return p, p.unstageLineCmd(r.file.Path, hunks[r.hunkIdx], r.lineIdx)
			}
		}
		return p, p.unstageCmd([]string{r.file.Path})
	}
	return p, nil
}

func (p *GitStatus) stageAll() (panels.Panel, tea.Cmd) {
	if len(p.files) == 0 {
		return p, nil
	}
	var paths []string
	seen := make(map[string]bool)
	for i := range p.files {
		path := p.files[i].Path
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return p, nil
	}
	return p, p.stageCmd(paths)
}

// stageSectionFiles stages all files belonging to the given section.
func (p *GitStatus) stageSectionFiles(sec section) (panels.Panel, tea.Cmd) {
	staged, unstaged, untracked := classifyFiles(p.files)
	var targets []git.FileStatus
	switch sec {
	case sectionStaged:
		targets = staged
	case sectionUnstaged:
		targets = unstaged
	case sectionUntracked:
		targets = untracked
	}
	if len(targets) == 0 {
		return p, nil
	}
	var paths []string
	for i := range targets {
		paths = append(paths, targets[i].Path)
	}
	return p, p.stageCmd(paths)
}

// unstageSectionFiles unstages all files belonging to the given section.
func (p *GitStatus) unstageSectionFiles(sec section) (panels.Panel, tea.Cmd) {
	staged, unstaged, untracked := classifyFiles(p.files)
	var targets []git.FileStatus
	switch sec {
	case sectionStaged:
		targets = staged
	case sectionUnstaged:
		targets = unstaged
	case sectionUntracked:
		targets = untracked
	}
	if len(targets) == 0 {
		return p, nil
	}
	var paths []string
	for i := range targets {
		paths = append(paths, targets[i].Path)
	}
	return p, p.unstageCmd(paths)
}

// unstageAll unstages all currently staged files.
func (p *GitStatus) unstageAll() (panels.Panel, tea.Cmd) {
	staged, _, _ := classifyFiles(p.files)
	if len(staged) == 0 {
		return p, nil
	}
	var paths []string
	for i := range staged {
		paths = append(paths, staged[i].Path)
	}
	return p, p.unstageCmd(paths)
}

// discardAtCursor discards unstaged changes for the file at the cursor,
// after showing a confirmation dialog (destructive operation).
func (p *GitStatus) discardAtCursor() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return p, nil
	}
	r := &p.rows[p.cursor]
	if r.file == nil {
		return p, nil
	}
	if r.section != sectionUnstaged {
		return p, nil
	}
	p.clearPending()
	p.pendingOp = opDiscard
	p.pendingPath = r.file.Path
	return p, notify.ShowConfirm("Discard Changes",
		fmt.Sprintf("Discard unstaged changes to %q?", filepath.Base(r.file.Path)))
}

// discardCmd returns a tea.Cmd that discards unstaged changes for the given path.
func (p *GitStatus) discardCmd(path string) tea.Cmd {
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.DiscardFile(ctx, path)
		return discardResultMsg{err: err}
	}
}

func (p *GitStatus) toggleSelection() {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return
	}
	r := &p.rows[p.cursor]
	if r.kind != rowFile || r.file == nil {
		return
	}
	key := p.fileKey(r)
	if p.selected[key] {
		delete(p.selected, key)
		r.selected = false
	} else {
		p.selected[key] = true
		r.selected = true
	}
}

func (p *GitStatus) fileKey(r *row) string {
	if r.file == nil {
		return ""
	}
	switch r.section {
	case sectionStaged:
		return r.file.Path + ":staged"
	case sectionUnstaged:
		return r.file.Path + ":unstaged"
	default:
		return r.file.Path
	}
}

// invalidateDiffCaches clears all cached diffs and collapses expanded files.
// Called after stage/unstage to prevent stale hunk data from being reused.
func (p *GitStatus) invalidateDiffCaches() {
	for k := range p.diffCache {
		delete(p.diffCache, k)
	}
	p.diffCacheOrder = p.diffCacheOrder[:0]
	for k := range p.expandedFiles {
		delete(p.expandedFiles, k)
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
// panelColors holds resolved color strings for the git status panel.
type panelColors struct {
	SectionHeader string
	Staged        string
	Unstaged      string
	Untracked     string
	CursorBg      string
	SelectedBg    string
	Dim           string
	Default       string
	Added         string
	Removed       string
	HunkHeader    string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		SectionHeader: "#C9A227",
		Staged:        "#6B9E56",
		Unstaged:      "#C9875A",
		Untracked:     "#7A9EBF",
		CursorBg:      "#2A2A2A",
		SelectedBg:    "#222222",
		Dim:           "#555555",
		Default:       "#999999",
		Added:         "#6B9E56",
		Removed:       "#C44B4B",
		HunkHeader:    "#555555",
	}
	if th == nil {
		return c
	}
	tc := th.Colors
	if tc.NormalYellow != "" {
		c.SectionHeader = tc.NormalYellow
	}
	if tc.GitStaged != "" {
		c.Staged = tc.GitStaged
	}
	if tc.GitUnstaged != "" {
		c.Unstaged = tc.GitUnstaged
	}
	if tc.GitUntracked != "" {
		c.Untracked = tc.GitUntracked
	}
	if tc.SelectionBg != "" {
		c.CursorBg = tc.SelectionBg
	}
	if tc.CursorLine != "" {
		c.SelectedBg = tc.CursorLine
	}
	if tc.BrightBlack != "" {
		c.Dim = tc.BrightBlack
	}
	if tc.FileDefault != "" {
		c.Default = tc.FileDefault
	}
	if tc.DiffAdded != "" {
		c.Added = tc.DiffAdded
	}
	if tc.DiffRemoved != "" {
		c.Removed = tc.DiffRemoved
	}
	if tc.DiffHunk != "" {
		c.HunkHeader = tc.DiffHunk
	}
	return c
}

// rowStyles holds pre-computed styles for the three possible row backgrounds.
// Created once per View() call instead of per row.
type rowStyles struct {
	cursor   lipgloss.Style
	selected lipgloss.Style
	normal   lipgloss.Style
}

func (p *GitStatus) renderRow(r *row, width int, isCursor bool, rs *rowStyles) string {
	var content string
	switch r.kind {
	case rowSection:
		content = p.renderSectionHeader(r, width)
	case rowFile:
		content = p.renderFileRow(r, width)
	case rowHunk:
		content = p.renderHunkRow(r, width)
	case rowDiffLine:
		content = p.renderDiffLineRow(r, width)
	}
	if isCursor {
		return rs.cursor.Render(content)
	}
	if r.selected {
		return rs.selected.Render(content)
	}
	return rs.normal.Render(content)
}

func (p *GitStatus) renderSectionHeader(r *row, width int) string {
	label := fmt.Sprintf("── %s ──", r.section.String())
	return p.styleSectionHeader.Width(width).Render(label)
}

func (p *GitStatus) renderFileRow(r *row, width int) string {
	if r.file == nil {
		return ""
	}
	var b strings.Builder
	// Selection marker.
	if r.selected {
		b.WriteString("● ")
	} else {
		b.WriteString("  ")
	}
	// Status indicator.
	b.WriteString(statusIndicator(r.file, r.section))
	b.WriteByte(' ')
	// File name (basename for compact display).
	name := filepath.Base(r.file.Path)
	dirPart := filepath.Dir(r.file.Path)
	if dirPart != "." {
		b.WriteString(name)
		b.WriteByte(' ')
		dirContent := b.String()
		dirLabel := dirPart
		remaining := width - len(dirContent) - len(dirLabel) - 1
		if remaining > 0 {
			var out strings.Builder
			out.WriteString(p.fileStyle(r.section).Render(dirContent))
			out.WriteString(p.styleDim.Render(dirLabel))
			return out.String()
		}
	} else {
		b.WriteString(name)
	}
	fgStyle := p.fileStyle(r.section)
	// Expand indicator.
	if r.expanded {
		var out strings.Builder
		out.WriteString(fgStyle.Render(b.String()))
		out.WriteString(p.styleDim.Render(" ▼"))
		return out.String()
	}
	return fgStyle.Render(b.String())
}

func (p *GitStatus) renderHunkRow(r *row, _ int) string {
	var b strings.Builder
	b.WriteString("  ")
	if r.hunkEntry != nil {
		b.WriteString(r.hunkEntry.Header)
	}
	return p.styleHunkHeader.Render(b.String())
}

func (p *GitStatus) renderDiffLineRow(r *row, _ int) string {
	if r.diffLine == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("    ")
	var style lipgloss.Style
	switch r.diffLine.Type {
	case git.DiffLineAdded:
		b.WriteByte('+')
		style = p.styleAdded
	case git.DiffLineRemoved:
		b.WriteByte('-')
		style = p.styleRemoved
	default:
		b.WriteByte(' ')
		style = p.styleDim
	}
	b.WriteString(r.diffLine.Content)
	return style.Render(b.String())
}

func statusIndicator(f *git.FileStatus, sec section) string {
	switch sec {
	case sectionStaged:
		return statusCodeLabel(f.StagedStatus)
	case sectionUnstaged:
		return statusCodeLabel(f.WorktreeStatus)
	case sectionUntracked:
		return "?"
	default:
		return " "
	}
}

func statusCodeLabel(sc git.StatusCode) string {
	switch sc {
	case git.StatusModified:
		return "M"
	case git.StatusAdded:
		return "A"
	case git.StatusDeleted:
		return "D"
	case git.StatusRenamed:
		return "R"
	case git.StatusCopied:
		return "C"
	case git.StatusUntracked:
		return "?"
	case git.StatusConflict:
		return "U"
	default:
		return " "
	}
}

func (p *GitStatus) fileColor(sec section) string {
	switch sec {
	case sectionStaged:
		return p.colors.Staged
	case sectionUnstaged:
		return p.colors.Unstaged
	case sectionUntracked:
		return p.colors.Untracked
	default:
		return p.colors.Default
	}
}

// fileStyle returns the cached style for the given section's file color.
func (p *GitStatus) fileStyle(sec section) lipgloss.Style {
	switch sec {
	case sectionStaged:
		return p.styleStaged
	case sectionUnstaged:
		return p.styleUnstaged
	case sectionUntracked:
		return p.styleUntracked
	default:
		return p.styleDefault
	}
}
