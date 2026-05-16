// Package review implements the diff review panel for grut.
// It displays changed files with their diffs and supports hunk-level
// approve/reject decisions for structured code review workflows.
package review

import (
	"context"
	"fmt"

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

// gitOps defines the git operations needed by the review panel.
// Using a narrow interface enables straightforward testing with mocks.
type gitOps interface {
	Status(ctx context.Context) ([]git.FileStatus, error)
	Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	Stage(ctx context.Context, paths []string) error
	Unstage(ctx context.Context, paths []string) error
}

// HunkState represents the review decision for a single diff hunk.
type HunkState int

const (
	HunkPending  HunkState = iota // not yet reviewed
	HunkApproved                  // approved (to be staged)
	HunkRejected                  // rejected (to be unstaged/skipped)
)

// ReviewFile tracks review state for a single changed file.
type ReviewFile struct {
	Path       string
	HunkStates []HunkState // per-hunk decision state
	Diff       git.FileDiff
}

// viewMode tracks which view the panel is currently displaying.
type viewMode int

const (
	modeFileList viewMode = iota // navigable list of changed files
	modeDiff                     // inline diff with hunk controls
)

// ---------------------------------------------------------------------------
// Internal message types (async result messages)
// ---------------------------------------------------------------------------
// filesLoadedMsg carries the result of an async diff-all load.
type filesLoadedMsg struct {
	err   error
	files []git.FileDiff
}

// stageResultMsg carries the result of a stage/unstage operation.
type stageResultMsg struct {
	err error
}

// Compile-time check that *Panel implements panels.Panel.
var _ panels.Panel = (*Panel)(nil)

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
)

// Panel is the diff review panel. It displays changed files and their diffs,
// supporting hunk-level approve/reject decisions.
type Panel struct {
	// Right-click context menu
	actionsCfg config.ActionsConfig
	// Dependencies
	git         gitOps
	theme       *theme.Theme
	ctx         context.Context
	err         error  // last error from load
	summary     string // cached summary text
	pendingOp   string
	pendingName string
	// Review state
	files []ReviewFile // all changed files with review state
	// Pre-rendered content (rebuilt on state/mode/cursor change)
	lines          []string // rendered lines for current view
	hunkLineStarts []int    // line indices where hunks begin (diff mode)
	panels.BasePanel
	fileCursor int      // selected file index
	hunkCursor int      // selected hunk index (in diff mode)
	mode       viewMode // file list or diff view
	// Display state
	scrollY     int  // viewport scroll offset
	loading     bool // true during async load
	showSummary bool // true when summary overlay is visible
}

// New creates a new review panel with the given git client.
// gc may be nil; the panel will show an error until a client is available.
func New(gc gitOps, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "review"},
		git:       gc,
		theme:     th,
	}
}

func (p *Panel) themeColors() theme.Colors {
	if p.theme != nil {
		return p.theme.Colors
	}
	return theme.Colors{}
}

// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return nil
}

// handleRepoChanged replaces the git client and clears review state for
// the new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
	} else {
		p.git = client
	}
	p.files = nil
	p.fileCursor = 0
	p.hunkCursor = 0
	p.mode = 0
	p.scrollY = 0
	p.loading = false
	p.err = nil
	p.showSummary = false
	p.summary = ""
	p.lines = nil
	p.hunkLineStarts = nil
	return p, nil
}

// Update implements panels.Panel. It handles review-related messages and
// keyboard events for navigation, approve/reject, and summary display.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.StartReviewMsg:
		return p, p.loadFiles()
	case filesLoadedMsg:
		p.loading = false
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.buildReviewFiles(msg.files)
		p.rebuildLines()
		return p, nil
	case stageResultMsg:
		// Stage/unstage completed — could surface errors via toast.
		return p, nil
	case tea.KeyPressMsg:
		if !p.Focused {
			return p, nil
		}
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
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	}
	return p, nil
}

// View implements panels.Panel. It renders the review panel content into
// the given width×height area.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.loading {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#666666")).
			Render("Loading changes...")
	}
	if p.err != nil {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().NormalRed, "#C44B4B")).
			Render(fmt.Sprintf("Error: %s", p.err))
	}
	if len(p.files) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#666666")).
			Render("No changes to review")
	}
	if p.showSummary {
		return p.renderSummaryView(width, height)
	}
	return p.renderViewport(width, height)
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/k", Description: "Move between files/hunks", Action: "navigate"},
		{Key: keyEnter, Description: "Expand file diff", Action: "expand"},
		{Key: "a", Description: "Approve hunk", Action: "approve"},
		{Key: "x", Description: "Reject hunk", Action: "reject"},
		{Key: "A", Description: "Approve all hunks", Action: "approve_all"},
		{Key: "X", Description: "Reject all hunks", Action: "reject_all"},
		{Key: "n/N", Description: "Next/previous file", Action: "file_nav"},
		{Key: "[/]", Description: "Previous/next hunk", Action: "hunk_nav"},
		{Key: "s", Description: "Show review summary", Action: "summary"},
		{Key: "q", Description: "Exit review", Action: "quit"},
	}
}

// SetActionsCfg stores the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// SetFiles directly injects review file data for testing.
// It resets cursor positions and rebuilds rendered lines.
func (p *Panel) SetFiles(files []ReviewFile) {
	p.files = files
	p.fileCursor = 0
	p.hunkCursor = 0
	p.scrollY = 0
	p.loading = false
	p.err = nil
	p.mode = modeFileList
	p.showSummary = false
	p.rebuildLines()
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if p.showSummary {
		switch msg.String() {
		case "q", "escape", "esc", keyEnter, "s": //nolint:goconst // inline string is more readable here
			p.showSummary = false
			p.rebuildLines()
		}
		return p, nil
	}
	switch p.mode {
	case modeFileList:
		return p.handleFileListKey(msg)
	case modeDiff:
		return p.handleDiffKey(msg)
	}
	return p, nil
}

func (p *Panel) handleFileListKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "j", "down": //nolint:goconst // key name, not a magic string
		if p.fileCursor < len(p.files)-1 {
			p.fileCursor++
			p.rebuildLines()
			p.ensureFileVisible()
		}
	case "k", "up":
		if p.fileCursor > 0 {
			p.fileCursor--
			p.rebuildLines()
			p.ensureFileVisible()
		}
	case keyEnter:
		if len(p.files) > 0 {
			p.mode = modeDiff
			p.hunkCursor = 0
			p.scrollY = 0
			p.rebuildLines()
		}
	case "n":
		if p.fileCursor < len(p.files)-1 {
			p.fileCursor++
			p.rebuildLines()
			p.ensureFileVisible()
		}
	case "N":
		if p.fileCursor > 0 {
			p.fileCursor--
			p.rebuildLines()
			p.ensureFileVisible()
		}
	case "s":
		p.summary = GenerateSummary(p.files)
		p.showSummary = true
	case "q":
		summary := GenerateSummary(p.files)
		return p, func() tea.Msg {
			return panels.ReviewCompleteMsg{Summary: summary}
		}
	}
	return p, nil
}

func (p *Panel) handleDiffKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if p.fileCursor >= len(p.files) {
		return p, nil
	}
	f := &p.files[p.fileCursor]
	numHunks := len(f.Diff.Hunks)
	switch msg.String() {
	case "j", "down", "]": //nolint:goconst // key name, not a magic string
		if p.hunkCursor < numHunks-1 {
			p.hunkCursor++
			p.rebuildLines()
			p.ensureHunkVisible()
		}
	case "k", "up", "[":
		if p.hunkCursor > 0 {
			p.hunkCursor--
			p.rebuildLines()
			p.ensureHunkVisible()
		}
	case "a":
		if numHunks > 0 && p.hunkCursor < numHunks {
			f.HunkStates[p.hunkCursor] = HunkApproved
			p.rebuildLines()
			return p, p.afterHunkDecision(f)
		}
	case "x":
		if numHunks > 0 && p.hunkCursor < numHunks {
			f.HunkStates[p.hunkCursor] = HunkRejected
			p.rebuildLines()
			return p, p.afterHunkDecision(f)
		}
	case "A":
		if numHunks > 0 {
			for i := range f.HunkStates {
				f.HunkStates[i] = HunkApproved
			}
			p.rebuildLines()
			return p, p.stageFile(f.Path)
		}
	case "X":
		if numHunks > 0 {
			for i := range f.HunkStates {
				f.HunkStates[i] = HunkRejected
			}
			p.rebuildLines()
			return p, p.unstageFile(f.Path)
		}
	case "n":
		if p.fileCursor < len(p.files)-1 {
			p.fileCursor++
			p.hunkCursor = 0
			p.scrollY = 0
			p.rebuildLines()
		}
	case "N":
		if p.fileCursor > 0 {
			p.fileCursor--
			p.hunkCursor = 0
			p.scrollY = 0
			p.rebuildLines()
		}
	case "s":
		p.summary = GenerateSummary(p.files)
		p.showSummary = true
	case "q", "escape", "esc":
		p.mode = modeFileList
		p.scrollY = 0
		p.ensureFileVisible()
		p.rebuildLines()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the file or hunk at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	if p.showSummary || len(p.files) == 0 {
		return p, nil
	}
	switch p.mode {
	case modeFileList:
		// File list lines: [0]=header, [1]=blank, [2+]=files
		lineIdx := p.scrollY + msg.ContentRow
		fileIdx := lineIdx - 2
		if fileIdx < 0 || fileIdx >= len(p.files) {
			return p, nil
		}
		p.fileCursor = fileIdx
		p.rebuildLines()
		p.ensureFileVisible()
	case modeDiff:
		// In diff mode, scroll the viewport to the clicked position.
		p.scrollY += msg.ContentRow
		if p.scrollY < 0 {
			p.scrollY = 0
		}
	}
	return p, nil
}

// handleMouseDoubleClick triggers the primary action at the clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	if p.showSummary || len(p.files) == 0 {
		return p, nil
	}
	if p.mode == modeFileList {
		lineIdx := p.scrollY + msg.ContentRow
		fileIdx := lineIdx - 2
		if fileIdx < 0 || fileIdx >= len(p.files) {
			return p, nil
		}
		p.fileCursor = fileIdx
		itemType := actions.ItemReviewFile
		if !p.actionsCfg.IsConfirmed(string(itemType)) {
			p.clearPending()
			p.pendingOp = opFirstUseConfirm
			p.pendingName = string(itemType)
			return p, rightclick.FirstUseCmd(itemType)
		}
		action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
		return p.executeRightClickAction(action)
	}
	return p, nil
}

// handleMouseWheel scrolls the review viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	if p.showSummary {
		return p, nil
	}
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.scrollY -= panels.ScrollDelta
		if p.scrollY < 0 {
			p.scrollY = 0
		}
	case tea.MouseWheelDown:
		maxScroll := len(p.lines) - p.Height
		if maxScroll < 0 {
			maxScroll = 0
		}
		p.scrollY += panels.ScrollDelta
		if p.scrollY > maxScroll {
			p.scrollY = maxScroll
		}
	}
	return p, nil
}

// handleMouseRightClick opens the context menu for the file at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	if p.showSummary || len(p.files) == 0 {
		return p, nil
	}
	if p.mode == modeFileList {
		lineIdx := p.scrollY + msg.ContentRow
		fileIdx := lineIdx - 2
		if fileIdx < 0 || fileIdx >= len(p.files) {
			return p, nil
		}
		p.fileCursor = fileIdx
		p.rebuildLines()
		label := p.files[fileIdx].Path
		cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemReviewFile, label)
		if cmd != nil {
			p.clearPending()
			p.pendingOp = opRightClickPick
			return p, cmd
		}
		if directAction != "" {
			return p.executeRightClickAction(directAction)
		}
	}
	return p, nil
}

// clearPending resets all pending-operation state so that no stale values
// leak across interactions. Call this before setting new pending state.
func (p *Panel) clearPending() {
	p.pendingOp = ""
	p.pendingName = ""
}

// handleModalResult dispatches the result of an action-picker modal.
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pendingOp
	name := p.pendingName
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
	}
	return p, nil
}

// executeRightClickAction runs the selected right-click action.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionExpandDiff:
		if p.fileCursor >= 0 && p.fileCursor < len(p.files) {
			p.mode = modeDiff
			p.hunkCursor = 0
			p.scrollY = 0
			p.rebuildLines()
		}
	case actions.ActionApprove:
		return p.approveFile()
	case actions.ActionCopyPath:
		return p.copyPath()
	}
	return p, nil
}

// approveFile marks all hunks in the current file as approved and stages it.
func (p *Panel) approveFile() (panels.Panel, tea.Cmd) {
	if p.fileCursor < 0 || p.fileCursor >= len(p.files) {
		return p, nil
	}
	f := &p.files[p.fileCursor]
	if len(f.Diff.Hunks) == 0 {
		return p, nil
	}
	for i := range f.HunkStates {
		f.HunkStates[i] = HunkApproved
	}
	p.rebuildLines()
	return p, p.stageFile(f.Path)
}

// copyPath copies the file path at the cursor to the OS clipboard.
func (p *Panel) copyPath() (panels.Panel, tea.Cmd) {
	if p.fileCursor < 0 || p.fileCursor >= len(p.files) {
		return p, nil
	}
	path := p.files[p.fileCursor].Path
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

// ---------------------------------------------------------------------------
// Async loading
// ---------------------------------------------------------------------------
func (p *Panel) loadFiles() tea.Cmd {
	p.loading = true
	p.files = nil
	p.fileCursor = 0
	p.hunkCursor = 0
	p.scrollY = 0
	p.err = nil
	p.mode = modeFileList
	p.showSummary = false
	p.lines = nil
	gc := p.git
	ctx := p.ctx
	return func() tea.Msg {
		if gc == nil {
			return filesLoadedMsg{err: fmt.Errorf("no git client configured")}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		diffs, err := gc.Diff(ctx, git.DiffOpts{})
		return filesLoadedMsg{files: diffs, err: err}
	}
}

func (p *Panel) buildReviewFiles(diffs []git.FileDiff) {
	p.files = make([]ReviewFile, len(diffs))
	for i, d := range diffs {
		p.files[i] = ReviewFile{
			Path:       d.Path,
			Diff:       d,
			HunkStates: make([]HunkState, len(d.Hunks)),
		}
	}
	p.fileCursor = 0
	p.hunkCursor = 0
}

// ---------------------------------------------------------------------------
// Stage / unstage
// ---------------------------------------------------------------------------
// afterHunkDecision checks if all hunks in a file are uniformly decided
// and triggers the appropriate git operation.
func (p *Panel) afterHunkDecision(f *ReviewFile) tea.Cmd {
	if len(f.HunkStates) == 0 {
		return nil
	}
	allApproved := true
	allRejected := true
	for _, s := range f.HunkStates {
		if s != HunkApproved {
			allApproved = false
		}
		if s != HunkRejected {
			allRejected = false
		}
	}
	if allApproved {
		return p.stageFile(f.Path)
	}
	if allRejected {
		return p.unstageFile(f.Path)
	}
	return nil
}

func (p *Panel) stageFile(path string) tea.Cmd {
	gc := p.git
	ctx := p.ctx
	return func() tea.Msg {
		if gc == nil {
			return stageResultMsg{err: fmt.Errorf("no git client")}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		return stageResultMsg{err: gc.Stage(ctx, []string{path})}
	}
}

func (p *Panel) unstageFile(path string) tea.Cmd {
	gc := p.git
	ctx := p.ctx
	return func() tea.Msg {
		if gc == nil {
			return stageResultMsg{err: fmt.Errorf("no git client")}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		return stageResultMsg{err: gc.Unstage(ctx, []string{path})}
	}
}
