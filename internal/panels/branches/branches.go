// Package branches implements the branch management panel for grut.
// It displays local and remote branches with tracking information and
// supports checkout, create, delete, rename, and fetch operations.
package branches

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
)

// GitOps defines the git operations required by the branch panel.
// This narrow interface is satisfied by *git.Client and makes the
// panel easy to mock in tests.
type GitOps interface {
	BranchList(ctx context.Context) ([]git.Branch, error)
	BranchCreate(ctx context.Context, name string, base string) error
	BranchDelete(ctx context.Context, name string, force bool) error
	BranchRename(ctx context.Context, oldName, newName string) error
	Checkout(ctx context.Context, ref string) error
	WorktreeAdd(ctx context.Context, path, branch string) error
	Fetch(ctx context.Context, opts git.FetchOpts) error
	RemoteList(ctx context.Context) ([]git.Remote, error)
}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------
// listItem represents a single row in the branch list display.
type listItem struct {
	branch   git.Branch // branch data (valid when !isHeader)
	header   string     // section header text (valid when isHeader)
	isHeader bool       // true for section headers ("Local Branches", etc.)
}

// pendingOp identifies which operation is awaiting modal input.
type pendingOp int

const (
	opNone            pendingOp = iota
	opCreate                    // awaiting new branch name input
	opDelete                    // awaiting delete confirmation
	opRename                    // awaiting new name input
	opFirstUseConfirm           // awaiting first-use double-click confirmation
	opRightClickPick            // awaiting right-click action picker result
)

// ---------------------------------------------------------------------------
// Internal messages (async result messages)
// ---------------------------------------------------------------------------
// branchesLoadedMsg carries the result of an async branch-list call.
type branchesLoadedMsg struct {
	err      error
	branches []git.Branch
}

// branchOpResultMsg carries the result of a branch operation.
type branchOpResultMsg struct {
	err  error
	op   string // "checkout", "worktree", "created", "deleted", "renamed", "fetched"
	name string // branch name involved
}

// ---------------------------------------------------------------------------
// Default colors (Dracula-inspired)
// ---------------------------------------------------------------------------
var defaultColors = struct {
	Current  string
	Local    string
	Remote   string
	Header   string
	Hash     string
	Tracking string
	CursorBg string
	Dim      string
}{
	Current:  "#50FA7B",
	Local:    "#F8F8F2",
	Remote:   "#BD93F9",
	Header:   "#8BE9FD",
	Hash:     "#6272A4",
	Tracking: "#FFB86C",
	CursorBg: "#44475A",
	Dim:      "#666666",
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------
// Panel is the branch management panel. It implements [panels.Panel].
type Panel struct {
	actionsCfg    config.ActionsConfig
	git           GitOps
	ctx           context.Context
	annotations   map[string]string // branch name → annotation text (e.g. "[merged]")
	repoRoot      string
	pendingBranch string     // branch name for pending delete/rename
	pendingName   string     // item type name for first-use confirm
	items         []listItem // flat display list (headers + branches)
	panels.BasePanel
	cfg             config.GitConfig
	cursor          int       // index into items (skips headers)
	offset          int       // viewport scroll offset
	pending         pendingOp // operation awaiting modal result
	showAnnotations bool      // whether to display annotations next to branch names
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new branch panel.
func New(gitOps GitOps, cfg config.GitConfig, repoRoot string) *Panel {
	return &Panel{
		BasePanel:       panels.BasePanel{PanelTitle: "branches"},
		git:             gitOps,
		cfg:             cfg,
		repoRoot:        repoRoot,
		annotations:     make(map[string]string),
		showAnnotations: true,
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return p.loadBranches()
}

// handleRepoChanged replaces the git client and reloads branches for the
// new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
		p.items = nil
		p.cursor = 0
		p.offset = 0
		p.annotations = make(map[string]string)
		return p, nil
	}
	p.git = client
	p.repoRoot = msg.Path
	p.items = nil
	p.cursor = 0
	p.offset = 0
	p.annotations = make(map[string]string)
	return p, p.loadBranches()
}

// loadBranches returns a tea.Cmd that loads the branch list asynchronously.
func (p *Panel) loadBranches() tea.Cmd {
	g := p.git
	ctx := p.ctx
	return func() tea.Msg {
		branches, err := g.BranchList(ctx)
		if err != nil {
			return branchesLoadedMsg{err: err}
		}
		return branchesLoadedMsg{branches: branches}
	}
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case branchesLoadedMsg:
		return p.handleBranchesLoaded(msg)
	case branchOpResultMsg:
		return p.handleOpResult(msg)
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleMouseDoubleClick(msg)
	case panels.PanelMouseRightClickMsg:
		return p.handleMouseRightClick(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case panels.RefreshBranchesMsg:
		return p, p.loadBranches()
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	// CRUD actions dispatched via keymap.
	case panels.ItemCreateMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestCreate()
	case panels.ItemDeleteMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestDelete()
	case panels.ItemEditMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestRename()
	case panels.ItemOpenMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doOpenInBrowser()
	case panels.ItemCopyMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doCopy()
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
			Foreground(lipgloss.Color(defaultColors.Dim)).
			Render("No branches")
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
		{Key: "enter", Description: "Checkout branch", Action: "checkout"},
		{Key: "n", Description: "Create new branch", Action: "item_create"},
		{Key: "d", Description: "Delete branch", Action: "item_delete"},
		{Key: "e/F2", Description: "Rename branch", Action: "item_edit"},
		{Key: "o", Description: "Open in browser", Action: "item_open"},
		{Key: "y", Description: "Copy branch name", Action: "item_copy"},
		{Key: "f", Description: "Fetch remotes", Action: "fetch"},
		{Key: "R", Description: "Refresh", Action: "refresh"},
		{Key: "a", Description: "Toggle annotations", Action: "toggle_annotations"},
	}
}

// ---------------------------------------------------------------------------
// Message handlers
// ---------------------------------------------------------------------------
func (p *Panel) handleBranchesLoaded(msg branchesLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch load error: " + errMsg, Level: notify.Error}
		}
	}
	p.buildItems(msg.branches)
	return p, nil
}

func (p *Panel) handleOpResult(msg branchOpResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errText := fmt.Sprintf("%s error: %s", msg.op, msg.err)
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: errText, Level: notify.Error}
		}
	}
	// Refresh branches after a successful operation.
	op := msg.op
	name := msg.name
	cmds := []tea.Cmd{p.loadBranches()}
	switch op {
	case "checkout":
		cmds = append(cmds,
			func() tea.Msg { return panels.BranchChangedMsg{Name: name} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Switched to " + name, Level: notify.Success}
			},
		)
	case "worktree":
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Worktree created for " + name, Level: notify.Success}
		})
	default:
		successMsg := fmt.Sprintf("Branch %s: %s", op, name)
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
	case "k", "up":
		p.moveCursorUp()
	case "enter":
		return p.requestCheckout()
	case "n":
		return p.requestCreate()
	case "d":
		return p.requestDelete()
	case "e", "F2":
		return p.requestRename()
	case "o":
		return p.doOpenInBrowser()
	case "y":
		return p.doCopy()
	case "f":
		return p.requestFetch()
	case "R":
		return p, p.loadBranches()
	case "a":
		p.showAnnotations = !p.showAnnotations
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the item at the clicked row, skipping headers.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	// Skip headers — land on the nearest non-header item.
	if p.items[idx].isHeader {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick performs checkout on the double-clicked branch.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	if p.items[idx].isHeader {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	itemType := p.currentItemType()
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pending = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// handleMouseRightClick shows a context menu for the branch at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	if p.items[idx].isHeader {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	b := p.items[idx].branch
	itemType := p.currentItemType()
	cmd, directAction := rightclick.Cmd(p.actionsCfg, itemType, b.Name)
	if cmd != nil {
		p.pending = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// currentItemType returns the action item type for the branch at the cursor.
func (p *Panel) currentItemType() actions.ItemType {
	if p.cursor >= 0 && p.cursor < len(p.items) && p.items[p.cursor].branch.IsRemote {
		return actions.ItemRemoteBranch
	}
	return actions.ItemLocalBranch
}

// executeRightClickAction dispatches a right-click action to the appropriate method.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.items) || p.items[p.cursor].isHeader {
		return p, nil
	}
	b := p.items[p.cursor].branch
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionCheckout:
		return p.requestCheckout()
	case actions.ActionCopyName:
		if err := panels.CopyToClipboard(p.ctx, b.Name); err != nil {
			errMsg := err.Error()
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
			}
		}
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copied: " + b.Name, Level: notify.Success}
		}
	case actions.ActionOpenInBrowser:
		return p.doOpenInBrowser()
	}
	return p, nil
}

// handleMouseWheel scrolls the branch list viewport.
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
	for i := p.cursor + 1; i < len(p.items); i++ {
		if !p.items[i].isHeader {
			p.cursor = i
			p.ensureCursorVisible()
			return
		}
	}
}

func (p *Panel) moveCursorUp() {
	for i := p.cursor - 1; i >= 0; i-- {
		if !p.items[i].isHeader {
			p.cursor = i
			p.ensureCursorVisible()
			return
		}
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

// selectedBranch returns the branch at the cursor position, or nil if the
// cursor is on a header or out of bounds.
func (p *Panel) selectedBranch() *git.Branch {
	if p.cursor < 0 || p.cursor >= len(p.items) || p.items[p.cursor].isHeader {
		return nil
	}
	b := p.items[p.cursor].branch
	return &b
}

// ---------------------------------------------------------------------------
// Branch operations
// ---------------------------------------------------------------------------
func (p *Panel) requestCheckout() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsCurrent {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Already on " + b.Name, Level: notify.Info}
		}
	}
	ref := b.Name
	if b.IsRemote {
		// Strip remote prefix (e.g., "origin/feature" → "feature") so
		// git creates a local tracking branch automatically.
		if idx := strings.IndexByte(ref, '/'); idx >= 0 {
			ref = ref[idx+1:]
		}
	}
	g := p.git
	ctx := p.ctx
	if p.cfg.WorktreeFirst {
		wtPath := worktreePath(p.repoRoot, ref)
		name := ref
		return p, func() tea.Msg {
			err := g.WorktreeAdd(ctx, wtPath, name)
			return branchOpResultMsg{op: "worktree", name: name, err: err}
		}
	}
	name := ref
	return p, func() tea.Msg {
		err := g.Checkout(ctx, name)
		return branchOpResultMsg{op: "checkout", name: name, err: err}
	}
}

func (p *Panel) requestCreate() (panels.Panel, tea.Cmd) {
	p.pending = opCreate
	return p, notify.ShowInput("New Branch", "branch-name")
}

func (p *Panel) requestDelete() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsCurrent {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete current branch", Level: notify.Warn}
		}
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote branch locally", Level: notify.Warn}
		}
	}
	p.pending = opDelete
	p.pendingBranch = b.Name
	return p, notify.ShowConfirm("Delete Branch", fmt.Sprintf("Delete branch %q?", b.Name))
}

func (p *Panel) requestRename() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot rename remote branch", Level: notify.Warn}
		}
	}
	p.pending = opRename
	p.pendingBranch = b.Name
	return p, notify.ShowInput("Rename Branch", b.Name)
}

func (p *Panel) requestFetch() (panels.Panel, tea.Cmd) {
	g := p.git
	ctx := p.ctx
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{Message: "Fetching...", Level: notify.Info}
		},
		func() tea.Msg {
			err := g.Fetch(ctx, git.FetchOpts{All: true, Prune: true})
			return branchOpResultMsg{op: "fetched", name: "all remotes", err: err}
		},
	)
}

// doOpenInBrowser opens the selected branch's page on the remote hosting
// provider (e.g. GitHub) in the default browser.
func (p *Panel) doOpenInBrowser() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	name := b.Name
	if b.IsRemote {
		if idx := strings.IndexByte(name, '/'); idx >= 0 {
			name = name[idx+1:]
		}
	}
	remotes, err := p.git.RemoteList(p.ctx)
	if err != nil || len(remotes) == 0 {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No remote available", Level: notify.Warn}
		}
	}
	base := git.RemoteToHTTPS(remotes[0].FetchURL)
	if base == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot build URL from remote", Level: notify.Warn}
		}
	}
	branchURL := base + "/tree/" + name
	return p, func() tea.Msg {
		if err := panels.OpenInBrowser(branchURL); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened branch " + name, Level: notify.Info}
	}
}

// doCopy copies the selected branch name to the clipboard.
func (p *Panel) doCopy() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if err := panels.CopyToClipboard(p.ctx, b.Name); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	name := b.Name
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Copied: " + name, Level: notify.Success}
	}
}

// ---------------------------------------------------------------------------
// Modal result handling
// ---------------------------------------------------------------------------
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	branch := p.pendingBranch
	name := p.pendingName
	p.pending = opNone
	p.pendingBranch = ""
	p.pendingName = ""
	if !msg.Accept {
		return p, nil
	}
	g := p.git
	ctx := p.ctx
	switch op { //nolint:exhaustive // only relevant cases handled
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, name, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opCreate:
		name := strings.TrimSpace(msg.Value)
		if name == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			err := g.BranchCreate(ctx, name, "")
			return branchOpResultMsg{op: "created", name: name, err: err}
		}
	case opDelete:
		return p, func() tea.Msg {
			err := g.BranchDelete(ctx, branch, false)
			return branchOpResultMsg{op: "deleted", name: branch, err: err}
		}
	case opRename:
		newName := strings.TrimSpace(msg.Value)
		if newName == "" || newName == branch {
			return p, nil
		}
		return p, func() tea.Msg {
			err := g.BranchRename(ctx, branch, newName)
			return branchOpResultMsg{op: "renamed", name: newName, err: err}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Item list building
// ---------------------------------------------------------------------------
// buildItems constructs the flat display list from branch data and
// positions the cursor on the current branch (if any).
func (p *Panel) buildItems(branches []git.Branch) {
	var local, remote []git.Branch
	for _, b := range branches {
		if b.IsRemote {
			remote = append(remote, b)
		} else {
			local = append(local, b)
		}
	}
	p.items = nil
	if len(local) > 0 {
		p.items = append(p.items, listItem{isHeader: true, header: "Local Branches"})
		for _, b := range local {
			p.items = append(p.items, listItem{branch: b})
		}
	}
	if len(remote) > 0 {
		p.items = append(p.items, listItem{isHeader: true, header: "Remote Branches"})
		for _, b := range remote {
			p.items = append(p.items, listItem{branch: b})
		}
	}
	// Default cursor to first selectable item.
	p.cursor = 0
	for i, item := range p.items {
		if !item.isHeader {
			p.cursor = i
			break
		}
	}
	// Prefer placing cursor on the current branch.
	for i, item := range p.items {
		if !item.isHeader && item.branch.IsCurrent {
			p.cursor = i
			break
		}
	}
	p.offset = 0
	p.ensureCursorVisible()
	p.computeAnnotations(branches)
}

// computeAnnotations populates the annotations map based on local branch
// heuristics. Currently detects:
//   - [merged]: local branch with 0 commits ahead of its upstream (fully synced)
//
// This is a structural enhancement; AI-powered annotations can populate
// the map with richer analysis in the future.
func (p *Panel) computeAnnotations(branches []git.Branch) {
	p.annotations = make(map[string]string)
	defaultBranch := p.cfg.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	for _, b := range branches {
		if b.IsRemote || b.IsCurrent {
			continue
		}
		// A local branch with 0 commits ahead of its upstream is likely
		// fully merged and can be cleaned up.
		if b.Upstream != "" && b.Ahead == 0 && b.Name != defaultBranch {
			p.annotations[b.Name] = "[merged]"
		}
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
// renderLine renders a single item in the branch list.
func (p *Panel) renderLine(item listItem, width int, isCursor bool) string {
	if item.isHeader {
		return lipgloss.NewStyle().
			Width(width).
			Foreground(lipgloss.Color(defaultColors.Header)).
			Bold(true).
			Render("── " + item.header + " ──")
	}
	b := item.branch
	// Prefix: current branch marker.
	prefix := "  "
	if b.IsCurrent {
		prefix = "* "
	}
	// Right-side info: ahead/behind tracking + short hash.
	var rightParts []string
	if b.Ahead > 0 {
		rightParts = append(rightParts, fmt.Sprintf("↑%d", b.Ahead))
	}
	if b.Behind > 0 {
		rightParts = append(rightParts, fmt.Sprintf("↓%d", b.Behind))
	}
	rightSide := ""
	if len(rightParts) > 0 {
		rightSide = "  " + strings.Join(rightParts, " ")
	}
	if b.Hash != "" {
		rightSide += "  " + b.Hash
	}
	// Build the line with padding for right-alignment.
	leftSide := prefix + b.Name
	// Append annotation if annotations are visible.
	if p.showAnnotations {
		if ann, ok := p.annotations[b.Name]; ok {
			leftSide += " " + ann
		}
	}
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	// Apply styles.
	style := lipgloss.NewStyle().Width(width)
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	if b.IsCurrent {
		style = style.Foreground(lipgloss.Color(defaultColors.Current)).Bold(true)
	} else if b.IsRemote {
		style = style.Foreground(lipgloss.Color(defaultColors.Remote))
	} else {
		style = style.Foreground(lipgloss.Color(defaultColors.Local))
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
