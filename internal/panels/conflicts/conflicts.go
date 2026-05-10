// Package conflicts implements the conflict resolution panel for grut.
// It displays files with merge/rebase conflicts and provides actions to
// mark files as resolved (by staging), continue the merge/rebase, or abort.
package conflicts

import (
	"context"
	"fmt"
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
// Narrow interface for testability
// ---------------------------------------------------------------------------
// gitOps defines the git operations used by the conflicts panel.
type gitOps interface {
	Status(ctx context.Context) ([]git.FileStatus, error)
	Stage(ctx context.Context, paths []string) error
	Merge(ctx context.Context, branch string, opts git.MergeOpts) error
	MergeAbort(ctx context.Context) error
	Rebase(ctx context.Context, onto string, opts git.RebaseOpts) error
	RebaseContinue(ctx context.Context) error
	RebaseAbort(ctx context.Context) error
}

// ---------------------------------------------------------------------------
// Operation mode — are we resolving merge or rebase conflicts?
// ---------------------------------------------------------------------------
// opMode identifies the type of in-progress operation with conflicts.
type opMode int

const (
	opNone   opMode = iota // no operation in progress
	opMerge                // merge in progress
	opRebase               // rebase in progress
)

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
)

func (m opMode) String() string {
	switch m {
	case opMerge:
		return "MERGING"
	case opRebase:
		return "REBASING"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Internal async messages
// ---------------------------------------------------------------------------
// conflictsLoadedMsg carries the result of an async conflict file scan.
type conflictsLoadedMsg struct {
	err   error
	files []string
}

// resolveResultMsg carries the result of staging (marking resolved) a file.
type resolveResultMsg struct {
	err  error
	path string
}

// continueResultMsg carries the result of a continue/abort operation.
type continueResultMsg struct {
	err    error
	action string // "continued" or "aborted"
}

// mergeResultMsg carries the result of a merge operation.
type mergeResultMsg struct {
	err       error
	branch    string
	conflicts []string // non-empty if conflicts detected
}

// rebaseResultMsg carries the result of a rebase operation.
type rebaseResultMsg struct {
	err       error
	onto      string
	conflicts []string // non-empty if conflicts detected
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------
// Panel is the conflict resolution panel. It lists conflicted files and
// provides resolve, continue, and abort actions.
type Panel struct {
	actionsCfg  config.ActionsConfig
	git         gitOps
	ctx         context.Context
	resolved    map[string]bool // files marked as resolved (staged)
	pendingOp   string
	pendingName string
	files       []string // paths of conflicted files
	theme       *theme.Theme
	panels.BasePanel
	cursor  int
	offset  int
	mode    opMode // merge or rebase
	loading bool
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new conflicts panel with the given git client.
func New(gc gitOps, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "conflicts"},
		git:       gc,
		resolved:  make(map[string]bool),
		theme:     th,
	}
}

func (p *Panel) themeColors() theme.Colors {
	if p.theme != nil {
		return p.theme.Colors
	}
	return theme.Colors{}
}

// SetActionsCfg stores the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) {
	p.actionsCfg = cfg
}

// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return nil
}

// handleRepoChanged replaces the git client and clears conflict state for
// the new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
	} else {
		p.git = client
	}
	p.files = nil
	p.resolved = make(map[string]bool)
	p.cursor = 0
	p.offset = 0
	p.mode = 0
	p.loading = false
	return p, nil
}

// safeCtx returns the panel's context, falling back to context.Background()
// if Init has not been called (e.g. during tests).
func (p *Panel) safeCtx() context.Context {
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------
// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.MergeRequestMsg:
		return p.startMerge(msg.Branch)
	case panels.RebaseRequestMsg:
		return p.startRebase(msg.Onto)
	case mergeResultMsg:
		return p.handleMergeResult(msg)
	case rebaseResultMsg:
		return p.handleRebaseResult(msg)
	case conflictsLoadedMsg:
		return p.handleConflictsLoaded(msg)
	case resolveResultMsg:
		return p.handleResolveResult(msg)
	case continueResultMsg:
		return p.handleContinueResult(msg)
	case panels.MergeAbortMsg:
		return p.abortOp()
	case panels.RebaseAbortMsg:
		return p.abortOp()
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
	case tea.KeyPressMsg:
		if p.Focused {
			return p.handleKey(msg)
		}
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.Code {
	case 'j', tea.KeyDown:
		p.moveCursor(1)
	case 'k', tea.KeyUp:
		p.moveCursor(-1)
	case 'r':
		return p.markResolved()
	case 'c':
		return p.continueOp()
	case 'a':
		return p.abortOp()
	case tea.KeyEnter:
		return p.openFile()
	}
	return p, nil
}

// moveCursor moves the cursor by delta, clamping to valid range.
func (p *Panel) moveCursor(delta int) {
	if len(p.files) == 0 {
		return
	}
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.files) {
		p.cursor = len(p.files) - 1
	}
	p.adjustOffset()
}

// adjustOffset ensures the cursor is visible within the viewport.
func (p *Panel) adjustOffset() {
	if p.Height <= 0 {
		return
	}
	// Reserve 2 lines for the status bar at the bottom.
	viewportHeight := p.Height - 2
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, viewportHeight)
}

// clampCursor ensures the cursor is within bounds after file list changes.
func (p *Panel) clampCursor() {
	if len(p.files) == 0 {
		p.cursor = 0
		p.offset = 0
		return
	}
	p.cursor = panels.ClampCursor(p.cursor, len(p.files))
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the conflict file at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.files) {
		return p, nil
	}
	p.cursor = idx
	p.adjustOffset()
	return p, nil
}

// handleMouseDoubleClick opens the diff for the double-clicked conflict file.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.files) {
		return p, nil
	}
	p.cursor = idx
	p.adjustOffset()
	itemType := actions.ItemConflictFile
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pendingOp = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseWheel scrolls the conflict list viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	viewportHeight := p.Height - 2
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.files) - viewportHeight
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

// handleMouseRightClick opens the context menu for the conflict file at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.files) {
		return p, nil
	}
	p.cursor = idx
	p.adjustOffset()
	label := p.files[idx]
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemConflictFile, label)
	if cmd != nil {
		p.pendingOp = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// handleModalResult dispatches the result of an action-picker modal.
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pendingOp
	name := p.pendingName
	p.pendingOp = ""
	p.pendingName = ""
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
	case actions.ActionOpenDiff:
		return p.openFile()
	case actions.ActionResolveOurs, actions.ActionResolveTheirs:
		return p.markResolved()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------
func (p *Panel) startMerge(branch string) (panels.Panel, tea.Cmd) {
	p.mode = opMerge
	p.files = nil
	p.resolved = make(map[string]bool)
	p.cursor = 0
	p.offset = 0
	p.loading = true
	gc := p.git
	ctx := p.safeCtx()
	return p, func() tea.Msg {
		err := gc.Merge(ctx, branch, git.MergeOpts{})
		if err != nil {
			// Check if the error is due to conflicts by scanning status.
			conflicts := scanConflicts(ctx, gc)
			if len(conflicts) > 0 {
				return mergeResultMsg{branch: branch, conflicts: conflicts}
			}
			return mergeResultMsg{branch: branch, err: err}
		}
		return mergeResultMsg{branch: branch}
	}
}

func (p *Panel) startRebase(onto string) (panels.Panel, tea.Cmd) {
	p.mode = opRebase
	p.files = nil
	p.resolved = make(map[string]bool)
	p.cursor = 0
	p.offset = 0
	p.loading = true
	gc := p.git
	ctx := p.safeCtx()
	return p, func() tea.Msg {
		err := gc.Rebase(ctx, onto, git.RebaseOpts{})
		if err != nil {
			conflicts := scanConflicts(ctx, gc)
			if len(conflicts) > 0 {
				return rebaseResultMsg{onto: onto, conflicts: conflicts}
			}
			return rebaseResultMsg{onto: onto, err: err}
		}
		return rebaseResultMsg{onto: onto}
	}
}

func (p *Panel) handleMergeResult(msg mergeResultMsg) (panels.Panel, tea.Cmd) {
	p.loading = false
	if msg.err != nil {
		p.mode = opNone
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Merge failed: " + errMsg,
				Level:   notify.Error,
			}
		}
	}
	if len(msg.conflicts) > 0 {
		p.files = msg.conflicts
		p.clampCursor()
		return p, func() tea.Msg {
			return panels.ConflictDetectedMsg{Files: msg.conflicts}
		}
	}
	// Clean merge — success.
	p.mode = opNone
	branch := msg.branch
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Merged %s successfully", branch),
				Level:   notify.Success,
			}
		},
		func() tea.Msg { return panels.RefreshGitStatusMsg{} },
		func() tea.Msg { return panels.RefreshBranchesMsg{} },
	)
}

func (p *Panel) handleRebaseResult(msg rebaseResultMsg) (panels.Panel, tea.Cmd) {
	p.loading = false
	if msg.err != nil {
		p.mode = opNone
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Rebase failed: " + errMsg,
				Level:   notify.Error,
			}
		}
	}
	if len(msg.conflicts) > 0 {
		p.files = msg.conflicts
		p.clampCursor()
		return p, func() tea.Msg {
			return panels.ConflictDetectedMsg{Files: msg.conflicts}
		}
	}
	p.mode = opNone
	onto := msg.onto
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Rebased onto %s successfully", onto),
				Level:   notify.Success,
			}
		},
		func() tea.Msg { return panels.RefreshGitStatusMsg{} },
		func() tea.Msg { return panels.RefreshBranchesMsg{} },
	)
}

func (p *Panel) handleConflictsLoaded(msg conflictsLoadedMsg) (panels.Panel, tea.Cmd) {
	p.loading = false
	if msg.err != nil {
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Failed to load conflicts: " + errMsg,
				Level:   notify.Error,
			}
		}
	}
	p.files = msg.files
	p.clampCursor()
	return p, nil
}

func (p *Panel) markResolved() (panels.Panel, tea.Cmd) {
	if len(p.files) == 0 || p.cursor >= len(p.files) {
		return p, nil
	}
	path := p.files[p.cursor]
	if p.resolved[path] {
		return p, nil // already resolved
	}
	gc := p.git
	ctx := p.safeCtx()
	return p, func() tea.Msg {
		err := gc.Stage(ctx, []string{path})
		return resolveResultMsg{path: path, err: err}
	}
}

func (p *Panel) handleResolveResult(msg resolveResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Failed to resolve %s: %s", msg.path, errMsg),
				Level:   notify.Error,
			}
		}
	}
	p.resolved[msg.path] = true
	// Check if all conflicts are resolved.
	if p.allResolved() {
		return p, func() tea.Msg {
			return panels.ConflictResolvedMsg{}
		}
	}
	return p, nil
}

func (p *Panel) continueOp() (panels.Panel, tea.Cmd) {
	if p.mode == opNone {
		return p, nil
	}
	if !p.allResolved() {
		remaining := p.remainingCount()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("%d conflict(s) remaining — resolve before continuing", remaining),
				Level:   notify.Warn,
			}
		}
	}
	gc := p.git
	ctx := p.safeCtx()
	mode := p.mode
	return p, func() tea.Msg {
		var err error
		switch mode { //nolint:exhaustive // only relevant cases handled
		case opMerge:
			// For merge, continuing is not a git command — the user commits.
			// But we emit the message so the app can handle it.
			return continueResultMsg{action: "continued"}
		case opRebase:
			err = gc.RebaseContinue(ctx)
		}
		if err != nil {
			return continueResultMsg{action: "continued", err: err}
		}
		return continueResultMsg{action: "continued"}
	}
}

func (p *Panel) abortOp() (panels.Panel, tea.Cmd) {
	if p.mode == opNone {
		return p, nil
	}
	gc := p.git
	ctx := p.safeCtx()
	mode := p.mode
	return p, func() tea.Msg {
		var err error
		switch mode { //nolint:exhaustive // only relevant cases handled
		case opMerge:
			err = gc.MergeAbort(ctx)
		case opRebase:
			err = gc.RebaseAbort(ctx)
		}
		if err != nil {
			return continueResultMsg{action: "aborted", err: err}
		}
		return continueResultMsg{action: "aborted"}
	}
}

func (p *Panel) handleContinueResult(msg continueResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		action := msg.action
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Failed to %s: %s", action, errMsg),
				Level:   notify.Error,
			}
		}
	}
	wasMode := p.mode
	p.mode = opNone
	p.files = nil
	p.resolved = make(map[string]bool)
	p.cursor = 0
	p.offset = 0
	action := msg.action
	var modeStr string
	switch wasMode { //nolint:exhaustive // only relevant cases handled
	case opMerge:
		modeStr = "Merge"
	case opRebase:
		modeStr = "Rebase"
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("%s %s", modeStr, action),
				Level:   notify.Success,
			}
		},
		func() tea.Msg { return panels.RefreshGitStatusMsg{} },
		func() tea.Msg { return panels.RefreshBranchesMsg{} },
	)
}

func (p *Panel) openFile() (panels.Panel, tea.Cmd) {
	if len(p.files) == 0 || p.cursor >= len(p.files) {
		return p, nil
	}
	path := p.files[p.cursor]
	return p, func() tea.Msg {
		return panels.ShowDiffMsg{Path: path}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
// allResolved returns true if every conflicted file has been marked resolved.
func (p *Panel) allResolved() bool {
	if len(p.files) == 0 {
		return true
	}
	for _, f := range p.files {
		if !p.resolved[f] {
			return false
		}
	}
	return true
}

// remainingCount returns the number of unresolved conflicts.
func (p *Panel) remainingCount() int {
	count := 0
	for _, f := range p.files {
		if !p.resolved[f] {
			count++
		}
	}
	return count
}

// scanConflicts queries git status and returns paths of conflicted files.
func scanConflicts(ctx context.Context, gc gitOps) []string {
	files, err := gc.Status(ctx)
	if err != nil {
		return nil
	}
	var conflicts []string
	for _, f := range files {
		if f.StagedStatus == git.StatusConflict || f.WorktreeStatus == git.StatusConflict {
			conflicts = append(conflicts, f.Path)
		}
	}
	return conflicts
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------
// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.mode == opNone && len(p.files) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().NormalGreen, "#6B9E56")).
			Render("No conflicts")
	}
	// Reserve 2 lines for the status bar.
	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}
	lines := make([]string, 0, listHeight)
	end := p.offset + listHeight
	if end > len(p.files) {
		end = len(p.files)
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderFileRow(p.files[i], width, i == p.cursor))
	}
	// Pad remaining lines.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < listHeight {
		lines = append(lines, emptyLine)
	}
	// Append status bar.
	lines = append(lines, p.renderStatusBar(width))
	return strings.Join(lines, "\n")
}

// renderFileRow renders a single conflict file entry.
func (p *Panel) renderFileRow(path string, width int, isCursor bool) string {
	marker := "✗"
	markerColor := panels.ColorOf(p.themeColors().NormalRed, "#C44B4B") // red for unresolved
	if p.resolved[path] {
		marker = "✓"
		markerColor = panels.ColorOf(p.themeColors().NormalGreen, "#6B9E56") // green for resolved
	}
	markerStyle := lipgloss.NewStyle().Foreground(markerColor)
	label := markerStyle.Render(marker) + " " + path
	// Truncate if needed. The marker is 2 visible chars + space = 4.
	maxLen := width
	if len(path)+4 > maxLen && maxLen > 7 {
		// Truncate the path portion.
		avail := maxLen - 7 // marker(2) + space(1) + "..."(3) + space(1)
		if avail > 0 {
			label = markerStyle.Render(marker) + " " + path[:avail] + "..."
		}
	}
	style := lipgloss.NewStyle().Width(width)
	if isCursor {
		style = style.
			Background(panels.ColorOf(p.themeColors().SelectionBg, "#2A2A2A")).
			Foreground(panels.ColorOf(p.themeColors().Foreground, "#D4D4D4"))
	}
	return style.Render(label)
}

// renderStatusBar renders the bottom status bar showing operation state.
func (p *Panel) renderStatusBar(width int) string {
	if p.mode == opNone {
		return lipgloss.NewStyle().Width(width).Render("")
	}
	remaining := p.remainingCount()
	total := len(p.files)
	status := fmt.Sprintf(" %s — %d/%d conflicts remaining", p.mode, remaining, total)
	style := lipgloss.NewStyle().
		Width(width).
		Background(panels.ColorOf(p.themeColors().NormalYellow, "#C9A227")).
		Foreground(panels.ColorOf(p.themeColors().Background, "#0D0D0D")).
		Bold(true)
	return style.Render(status)
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "r", Description: "Mark file as resolved (stage)", Action: "resolve"},
		{Key: "c", Description: "Continue merge/rebase", Action: "continue"},
		{Key: "a", Description: "Abort merge/rebase", Action: "abort"},
		{Key: "enter", Description: "Open file diff", Action: "open"},
	}
}
