// Package worktrees implements the worktree management panel for grut.
// It displays all git worktrees with path, branch, and hash information,
// supports create, remove, and switch operations, and detects external
// deletion of worktree directories.
package worktrees

import (
	"context"
	"fmt"
	"os"
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

// GitOps defines the git operations required by the worktree panel.
// This narrow interface is satisfied by *git.Client and makes the
// panel easy to mock in tests.
type GitOps interface {
	WorktreeList(ctx context.Context) ([]git.Worktree, error)
	WorktreeAdd(ctx context.Context, path, branch string) error
	WorktreeRemove(ctx context.Context, path string, force bool) error
}

// PathChecker abstracts filesystem existence checks for testability.
// The default implementation uses os.Stat.
type PathChecker func(path string) bool

// defaultPathChecker returns true if the path exists on disk.
func defaultPathChecker(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------
// worktreeItem represents a single row in the worktree list.
type worktreeItem struct {
	worktree  git.Worktree
	isMain    bool // first worktree returned by git is the main worktree
	isMissing bool // path no longer exists on disk
}

// pendingOp identifies which operation is awaiting modal input.
type pendingOp int

const (
	opNone            pendingOp = iota
	opCreate                    // awaiting new branch name for worktree
	opDelete                    // awaiting delete confirmation
	opPrune                     // awaiting prune confirmation
	opRightClickPick            // awaiting right-click action picker result
	opFirstUseConfirm           // awaiting first-use double-click confirmation
)

// ---------------------------------------------------------------------------
// Internal messages (async result messages)
// ---------------------------------------------------------------------------
// worktreesLoadedMsg carries the result of an async worktree-list call.
type worktreesLoadedMsg struct {
	err       error
	worktrees []git.Worktree
}

// worktreeOpResultMsg carries the result of a worktree operation.
type worktreeOpResultMsg struct {
	err  error
	op   string // "created", "removed", "pruned"
	name string // branch or path involved
}

type panelColors struct {
	Current  string
	Normal   string
	Hash     string
	Branch   string
	Missing  string
	CursorBg string
	Dim      string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Current:  "#6B9E56",
		Normal:   "#D4D4D4",
		Hash:     "#555555",
		Branch:   "#C9A227",
		Missing:  "#C44B4B",
		CursorBg: "#2A2A2A",
		Dim:      "#555555",
	}
	if th != nil {
		c.Current = th.Colors.NormalGreen
		c.Normal = th.Colors.Foreground
		c.Hash = th.Colors.BrightBlack
		c.Branch = th.Colors.GitBranch
		c.Missing = th.Colors.NormalRed
		c.CursorBg = th.Colors.SelectionBg
		c.Dim = th.Colors.BrightBlack
	}
	return c
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------
// Panel is the worktree management panel. It implements [panels.Panel].
type Panel struct {
	actionsCfg  config.ActionsConfig
	git         GitOps
	ctx         context.Context
	pathCheck   PathChecker
	repoRoot    string
	pendingPath string         // worktree path for pending delete
	pendingName string         // item type name for first-use confirm
	items       []worktreeItem // flat display list
	panels.BasePanel
	colors  panelColors
	theme   *theme.Theme
	cfg     config.GitConfig
	cursor  int       // index into items
	offset  int       // viewport scroll offset
	pending pendingOp // operation awaiting modal result
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new worktree panel.
func New(gitOps GitOps, cfg config.GitConfig, repoRoot string, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "worktrees"},
		git:       gitOps,
		cfg:       cfg,
		pathCheck: defaultPathChecker,
		repoRoot:  repoRoot,
		colors:    initColors(th),
		theme:     th,
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return p.loadWorktrees()
}

// handleRepoChanged replaces the git client and reloads worktrees for the
// new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
		p.items = nil
		p.cursor = 0
		p.offset = 0
		return p, nil
	}
	p.git = client
	p.repoRoot = msg.Path
	p.items = nil
	p.cursor = 0
	p.offset = 0
	return p, p.loadWorktrees()
}

// loadWorktrees returns a tea.Cmd that loads the worktree list asynchronously.
func (p *Panel) loadWorktrees() tea.Cmd {
	g := p.git
	ctx := p.ctx
	return func() tea.Msg {
		worktrees, err := g.WorktreeList(ctx)
		return worktreesLoadedMsg{worktrees: worktrees, err: err}
	}
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case worktreesLoadedMsg:
		return p.handleWorktreesLoaded(msg)
	case worktreeOpResultMsg:
		return p.handleOpResult(msg)
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleMouseDoubleClick(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case panels.WorktreeChangedMsg:
		return p, p.loadWorktrees()
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	// CRUD actions dispatched via keymap.
	case panels.ItemDeleteMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestDelete()
	case panels.ItemCreateMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestCreate()
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if len(p.items) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("No worktrees")
	}
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > len(p.items) {
		end = len(p.items)
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderLine(p.items[i], width, i == p.cursor))
	}
	// Pad remaining height with blank lines.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "enter", Description: "Switch to worktree", Action: "switch"},
		{Key: "n", Description: "New worktree", Action: "create"},
		{Key: "d/x", Description: "Remove worktree", Action: "item_delete"},
		{Key: "R", Description: "Refresh", Action: "refresh"},
		{Key: "p", Description: "Prune missing", Action: "prune"},
	}
}

// ---------------------------------------------------------------------------
// Message handlers
// ---------------------------------------------------------------------------
func (p *Panel) handleWorktreesLoaded(msg worktreesLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Worktree load error: " + errMsg, Level: notify.Error}
		}
	}
	p.buildItems(msg.worktrees)
	return p, nil
}

func (p *Panel) handleOpResult(msg worktreeOpResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errText := fmt.Sprintf("%s error: %s", msg.op, msg.err)
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: errText, Level: notify.Error}
		}
	}
	op := msg.op
	name := msg.name
	cmds := []tea.Cmd{p.loadWorktrees()}
	switch op {
	case "created":
		cmds = append(cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree created for " + name, Level: notify.Success}
			},
		)
	case "removed":
		cmds = append(cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree removed: " + name, Level: notify.Success}
			},
		)
	default:
		successMsg := fmt.Sprintf("Worktree %s: %s", op, name)
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: successMsg, Level: notify.Success}
		})
	}
	return p, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}
	switch msg.String() {
	case "j", "down":
		p.moveCursorDown()
		return p, p.worktreeSelectedCmd()
	case "k", "up":
		p.moveCursorUp()
		return p, p.worktreeSelectedCmd()
	case "enter":
		return p.requestSwitch("")
	case "n":
		return p.requestCreate()
	case "d", "x":
		return p.requestDelete()
	case "R":
		return p, p.loadWorktrees()
	case "p":
		return p.requestPrune()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the worktree at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, p.worktreeSelectedCmd()
}

// handleMouseDoubleClick switches to the worktree at the clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	itemType := actions.ItemWorktree
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pending = opFirstUseConfirm
		p.pendingName = string(itemType)
		p.pendingPath = p.items[idx].worktree.Path
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action, "")
}

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// executeRightClickAction dispatches a right-click action to the appropriate
// method. When pathOverride is non-empty it is used directly instead of
// looking up the item via p.cursor, which may have become stale during an
// async modal delay.
func (p *Panel) executeRightClickAction(action actions.ActionID, pathOverride string) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionSwitch:
		return p.requestSwitch(pathOverride)
	case actions.ActionChangeDirectory:
		return p.changeDirectory(pathOverride)
	case actions.ActionOpenTerminal:
		return p.openTerminal(pathOverride)
	case actions.ActionCopyPath:
		return p.copyPath(pathOverride)
	}
	return p, nil
}

// openTerminal opens a terminal at the worktree's path.
func (p *Panel) openTerminal(pathOverride string) (panels.Panel, tea.Cmd) {
	var wtPath string
	if pathOverride != "" {
		wtPath = pathOverride
	} else {
		if p.cursor < 0 || p.cursor >= len(p.items) {
			return p, nil
		}
		wtPath = p.items[p.cursor].worktree.Path
	}
	return p, func() tea.Msg {
		if err := panels.OpenInTerminal(wtPath); err != nil {
			return notify.ShowToastMsg{Message: "Terminal failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened terminal", Level: notify.Info}
	}
}

// copyPath copies the worktree path to the clipboard.
func (p *Panel) copyPath(pathOverride string) (panels.Panel, tea.Cmd) {
	var wtPath string
	if pathOverride != "" {
		wtPath = pathOverride
	} else {
		if p.cursor < 0 || p.cursor >= len(p.items) {
			return p, nil
		}
		wtPath = p.items[p.cursor].worktree.Path
	}
	return p, func() tea.Msg {
		if err := panels.CopyToClipboard(p.ctx, wtPath); err != nil {
			return notify.ShowToastMsg{Message: "Copy failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Copied path", Level: notify.Info}
	}
}

// changeDirectory emits a ChangeDirectoryMsg so the app re-roots into
// the selected worktree.
func (p *Panel) changeDirectory(pathOverride string) (panels.Panel, tea.Cmd) {
	var path string
	if pathOverride != "" {
		path = pathOverride
	} else {
		item := p.selectedWorktree()
		if item == nil {
			return p, nil
		}
		if item.isMissing {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Cannot cd: path missing", Level: notify.Warn}
			}
		}
		path = item.worktree.Path
	}
	return p, func() tea.Msg {
		return panels.ChangeDirectoryMsg{Path: path}
	}
}

// handleMouseWheel scrolls the worktree list viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.items) - p.Height
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
// Navigation
// ---------------------------------------------------------------------------
func (p *Panel) moveCursorDown() {
	if p.cursor < len(p.items)-1 {
		p.cursor++
		p.ensureCursorVisible()
	}
}

func (p *Panel) moveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureCursorVisible()
	}
}

func (p *Panel) ensureCursorVisible() {
	if p.Height <= 0 {
		return
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.Height {
		p.offset = p.cursor - p.Height + 1
	}
}

// selectedWorktree returns the worktree item at the cursor, or nil if
// the cursor is out of bounds.
func (p *Panel) selectedWorktree() *worktreeItem {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}
	item := p.items[p.cursor]
	return &item
}

// worktreeSelectedCmd returns a Cmd that emits WorktreeSelectedMsg for the
// worktree under the cursor so other panels (e.g. commits) can react.
func (p *Panel) worktreeSelectedCmd() tea.Cmd {
	item := p.selectedWorktree()
	if item == nil {
		return nil
	}
	path := item.worktree.Path
	branch := item.worktree.Branch
	return func() tea.Msg {
		return panels.WorktreeSelectedMsg{Path: path, Branch: branch}
	}
}

// ---------------------------------------------------------------------------
// Worktree operations
// ---------------------------------------------------------------------------
func (p *Panel) requestSwitch(pathOverride string) (panels.Panel, tea.Cmd) {
	var path string
	if pathOverride != "" {
		path = pathOverride
	} else {
		item := p.selectedWorktree()
		if item == nil {
			return p, nil
		}
		if item.isMissing {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Cannot switch: path missing", Level: notify.Warn}
			}
		}
		path = item.worktree.Path
	}
	if p.cfg.WorktreeOpenMode == "new_terminal" {
		return p, func() tea.Msg {
			if err := panels.OpenInTerminal(path); err != nil {
				errMsg := err.Error()
				return notify.ShowToastMsg{Message: "Terminal error: " + errMsg, Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened terminal at " + path, Level: notify.Success}
		}
	}
	return p, func() tea.Msg {
		return panels.SwitchWorktreeMsg{Path: path}
	}
}

func (p *Panel) requestCreate() (panels.Panel, tea.Cmd) {
	p.pending = opCreate
	return p, notify.ShowInput("New Worktree", "branch-name")
}

func (p *Panel) requestDelete() (panels.Panel, tea.Cmd) {
	item := p.selectedWorktree()
	if item == nil {
		return p, nil
	}
	if item.isMain {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot remove main worktree", Level: notify.Warn}
		}
	}
	p.pending = opDelete
	p.pendingPath = item.worktree.Path
	displayName := filepath.Base(item.worktree.Path)
	return p, notify.ShowConfirm("Remove Worktree", fmt.Sprintf("Remove worktree %q?", displayName))
}

func (p *Panel) requestPrune() (panels.Panel, tea.Cmd) {
	// Check if any worktrees are missing.
	hasMissing := false
	for _, item := range p.items {
		if item.isMissing {
			hasMissing = true
			break
		}
	}
	if !hasMissing {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No missing worktrees to prune", Level: notify.Info}
		}
	}
	p.pending = opPrune
	return p, notify.ShowConfirm("Prune Worktrees", "Force-remove all missing worktrees?")
}

// ---------------------------------------------------------------------------
// Modal result handling
// ---------------------------------------------------------------------------
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	pendingPath := p.pendingPath
	pendingName := p.pendingName
	p.pending = opNone
	p.pendingPath = ""
	p.pendingName = ""
	if !msg.Accept {
		return p, nil
	}
	g := p.git
	ctx := p.ctx
	switch op { //nolint:exhaustive // only relevant cases handled
	case opCreate:
		branch := strings.TrimSpace(msg.Value)
		if branch == "" {
			return p, nil
		}
		wtPath := worktreePath(p.repoRoot, branch)
		return p, func() tea.Msg {
			err := g.WorktreeAdd(ctx, wtPath, branch)
			return worktreeOpResultMsg{op: "created", name: branch, err: err}
		}
	case opDelete:
		return p, func() tea.Msg {
			err := g.WorktreeRemove(ctx, pendingPath, false)
			return worktreeOpResultMsg{op: "removed", name: filepath.Base(pendingPath), err: err}
		}
	case opPrune:
		return p.pruneAllMissing()
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, pendingName, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value), pendingPath)
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value), pendingPath)
	}
	return p, nil
}

// pruneAllMissing force-removes all worktrees whose path is missing.
func (p *Panel) pruneAllMissing() (panels.Panel, tea.Cmd) {
	var missingPaths []string
	for _, item := range p.items {
		if item.isMissing {
			missingPaths = append(missingPaths, item.worktree.Path)
		}
	}
	if len(missingPaths) == 0 {
		return p, nil
	}
	g := p.git
	ctx := p.ctx
	paths := missingPaths
	return p, func() tea.Msg {
		var lastErr error
		for _, path := range paths {
			if err := g.WorktreeRemove(ctx, path, true); err != nil {
				lastErr = err
			}
		}
		count := len(paths)
		name := fmt.Sprintf("%d worktree(s)", count)
		return worktreeOpResultMsg{op: "pruned", name: name, err: lastErr}
	}
}

// ---------------------------------------------------------------------------
// Item list building
// ---------------------------------------------------------------------------
// buildItems constructs the flat display list from worktree data.
// The first worktree in git's output is always the main worktree.
func (p *Panel) buildItems(worktrees []git.Worktree) {
	p.items = nil
	checker := p.pathCheck
	if checker == nil {
		checker = defaultPathChecker
	}
	for i, wt := range worktrees {
		item := worktreeItem{
			worktree:  wt,
			isMain:    i == 0,
			isMissing: !wt.Bare && !checker(wt.Path),
		}
		p.items = append(p.items, item)
	}
	// Clamp cursor.
	if p.cursor >= len(p.items) {
		p.cursor = len(p.items) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.offset = 0
	p.ensureCursorVisible()
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
// renderLine renders a single worktree item.
func (p *Panel) renderLine(item worktreeItem, width int, isCursor bool) string {
	wt := item.worktree
	// Prefix: main worktree marker.
	prefix := "  "
	if item.isMain {
		prefix = "* "
	}
	// Branch name (or "(bare)" for bare worktrees).
	branch := wt.Branch
	if branch == "" {
		if wt.Bare {
			branch = "(bare)"
		} else {
			branch = "(detached)"
		}
	}
	// Short hash (first 7 chars).
	hash := wt.Head
	if len(hash) > git.ShortHashLen {
		hash = hash[:git.ShortHashLen]
	}
	// Missing tag.
	missingTag := ""
	if item.isMissing {
		missingTag = "  [MISSING]"
	}
	// Build left side: "  /path/to/worktree"
	leftSide := prefix + wt.Path
	// Build right side: "branch  hash  [MISSING]"
	rightSide := "  " + branch
	if hash != "" {
		rightSide += "  " + hash
	}
	rightSide += missingTag
	// Compute gap for right-alignment.
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	// Apply styles.
	style := lipgloss.NewStyle().Width(width)
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	if item.isMissing {
		style = style.Foreground(lipgloss.Color(p.colors.Missing))
	} else if item.isMain {
		style = style.Foreground(lipgloss.Color(p.colors.Current)).Bold(true)
	} else {
		style = style.Foreground(lipgloss.Color(p.colors.Normal))
	}
	return style.Render(line)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
// worktreePath is an alias to the canonical implementation in the git package.
// See git.WorktreePath for the convention details.
func worktreePath(repoRoot, branch string) string {
	return git.WorktreePath(repoRoot, branch)
}
