// Package commits implements the commits panel for grut.
// It displays commit history for a specific branch or worktree,
// driven by branch/worktree selection messages from other panels.
// Unlike gitlog (which always follows HEAD with graph rendering),
// this panel is selection-driven and shows a flat commit list.
package commits

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/panels/commitrender"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
)

// defaultPageSize is the number of commits to load per page.
const defaultPageSize = 100

// loadMoreThreshold triggers a new page load when the cursor is within
// this many items from the bottom of the loaded data.
const loadMoreThreshold = 10

// filterKind describes the active contextual filter source.
type filterKind int

const (
	filterNone     filterKind = iota
	filterFile                // commits touching a specific file
	filterFolder              // commits touching files under a directory
	filterBranch              // commits on a specific branch
	filterWorktree            // commits on a worktree's branch
	filterRemote              // commits on a remote
	filterStash               // commits for a stash entry
)

// gitOps defines the git operations required by the commits panel.
// This narrow interface is satisfied by *git.Client and makes the
// panel easy to mock in tests.
type gitOps interface {
	Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	FormatPatch(ctx context.Context, hash string) (string, error)
	RepoRoot(ctx context.Context) (string, error)
}

type panelColors struct {
	Hash       string
	Date       string
	Author     string
	Subject    string
	Dim        string
	CursorBg   string
	Refs       string
	SearchBg   string
	SearchFg   string
	SigGood    string
	SigBad     string
	SigCaution string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Hash:       "#D4B84A",
		Date:       "#555555",
		Author:     "#6B9E56",
		Subject:    "#999999",
		Dim:        "#555555",
		CursorBg:   "#2A2A2A",
		Refs:       "#7A9EBF",
		SearchBg:   "#2A2A2A",
		SearchFg:   "#D4D4D4",
		SigGood:    "#6B9E56",
		SigBad:     "#C05B5B",
		SigCaution: "#D4B84A",
	}
	if th != nil {
		c.Hash = th.Colors.NormalYellow
		c.Date = th.Colors.BrightBlack
		c.Author = th.Colors.NormalGreen
		c.Subject = th.Colors.NormalWhite
		c.Dim = th.Colors.BrightBlack
		c.CursorBg = th.Colors.SelectionBg
		c.Refs = th.Colors.BrightBlue
		c.SearchBg = th.Colors.SelectionBg
		c.SearchFg = th.Colors.SelectionFg
		c.SigGood = th.Colors.NormalGreen
		c.SigBad = th.Colors.NormalRed
		c.SigCaution = th.Colors.NormalYellow
	}
	return c
}

func newCommitLineStyles(c panelColors) commitrender.Styles {
	return commitrender.Styles{
		Hash:       lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hash)),
		Subject:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.Subject)),
		Cursor:     lipgloss.NewStyle().Background(lipgloss.Color(c.CursorBg)),
		SigGood:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.SigGood)),
		SigBad:     lipgloss.NewStyle().Foreground(lipgloss.Color(c.SigBad)),
		SigCaution: lipgloss.NewStyle().Foreground(lipgloss.Color(c.SigCaution)),
	}
}

// Panel is the commits panel. It implements [panels.Panel].
type Panel struct {
	actionsCfg   config.ActionsConfig
	gitClient    gitOps
	ctx          context.Context
	ref          string // current branch/ref to show commits for
	refLabel     string // display label for the current ref
	filterPath   string // path filter for file/folder kinds
	filterLabel  string // display label for filter
	searchQuery  string
	authorFilter string // active author filter (empty means no filter)
	// Commit selection state (drives file-tree filtering).
	selectedHash    string
	selectedSubject string
	prLabel         string
	pendingOp       string // operation awaiting modal result
	pendingName     string // item type name for first-use confirm
	commits         []git.Commit
	filteredIdx     []int
	detailLines     []string
	prCommits       []panels.PRCommit
	cursor          int
	offset          int
	// Contextual filter state.
	filter       filterKind // active filter source
	pageSize     int
	detailOffset int
	prNumber     int
	width        int
	height       int
	loading      bool
	allLoaded    bool
	// Search state.
	searchMode bool
	// Pickaxe content-search state (server-side git -S / -G). pickaxeTerm
	// doubles as the editable input buffer and the applied filter.
	pickaxeMode  bool
	pickaxeTerm  string
	pickaxeRegex bool
	// Message grep-search state (server-side git --grep). grepTerm doubles
	// as the editable input buffer and the applied filter.
	grepMode bool
	grepTerm string
	// Detail view state.
	detailMode bool
	// PR-commits mode: shows commits in a pull request.
	prCommitsMode bool
	focused       bool
	colors        panelColors
	clStyles      commitrender.Styles
	theme         *theme.Theme
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new commits panel.
func New(client gitOps, th *theme.Theme) *Panel {
	c := initColors(th)
	return &Panel{
		gitClient: client,
		pageSize:  defaultPageSize,
		colors:    c,
		clStyles:  newCommitLineStyles(c),
		theme:     th,
	}
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------
// commitsLoadedMsg carries loaded commits from the async git log command.
type commitsLoadedMsg struct {
	commits []git.Commit
	append  bool // true for pagination loads
}

// loadCommitsCmd returns a tea.Cmd that loads commits asynchronously.
func (p *Panel) loadCommitsCmd(skip int, appendMode bool) tea.Cmd {
	client := p.gitClient
	pageSize := p.pageSize
	ref := p.ref
	ctx := p.ctx
	// Pass path filter for file/folder filter kinds.
	var pathFilter string
	if (p.filter == filterFile || p.filter == filterFolder) && p.filterPath != "" {
		pathFilter = p.filterPath
	}
	pickaxe := p.pickaxeTerm
	pickaxeRegex := p.pickaxeRegex
	grep := p.grepTerm
	return func() tea.Msg {
		commits, err := client.Log(ctx, git.LogOpts{
			Ref:          ref,
			MaxCount:     pageSize,
			Skip:         skip,
			Path:         pathFilter,
			Pickaxe:      pickaxe,
			PickaxeRegex: pickaxeRegex,
			Grep:         grep,
		})
		if err != nil {
			return notify.ShowToastMsg{
				Message: "commits: " + err.Error(),
				Level:   notify.Error,
			}
		}
		return commitsLoadedMsg{commits: commits, append: appendMode}
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	p.loading = true
	return p.loadCommitsCmd(0, false)
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case commitsLoadedMsg:
		return p.handleCommitsLoaded(msg)
	case panels.BranchChangedMsg:
		return p.handleBranchChanged(msg)
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	case panels.BranchSelectedMsg:
		return p.handleBranchSelected(msg)
	case panels.FileSelectedMsg:
		return p.handleFileSelected(msg)
	case panels.FolderSelectedMsg:
		return p.handleFolderSelected(msg)
	case panels.WorktreeSelectedMsg:
		return p.handleWorktreeSelected(msg)
	case panels.RemoteSelectedMsg:
		return p.handleRemoteSelected(msg)
	case panels.StashSelectedMsg:
		return p.handleStashSelected(msg)
	case panels.PRCommitsLoadedMsg:
		return p.handlePRCommitsLoaded(msg)
	case panels.PRDeselectedMsg:
		return p.exitPRCommitsMode()
	case panels.ChangeDirectoryMsg:
		return p.handleSwitchWorktree(msg)
	case panels.WorktreeChangedMsg:
		return p.handleWorktreeChanged()
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
		switch msg.Button {
		case tea.MouseWheelUp:
			p.moveCursorUp()
		case tea.MouseWheelDown:
			return p.moveCursorDown()
		}
		return p, nil
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.detailMode {
		return p.renderDetail(width, height)
	}
	if p.loading && len(p.commits) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("Loading commits...")
	}
	if len(p.commits) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("No commits")
	}
	return p.renderList(width, height)
}

// Focus implements panels.Panel.
func (p *Panel) Focus() { p.focused = true }

// Blur implements panels.Panel.
func (p *Panel) Blur() { p.focused = false }

// SetSize implements panels.Panel.
func (p *Panel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Title implements panels.Panel.
func (p *Panel) Title() string {
	const title = "Commits"
	base := title
	switch {
	case p.prCommitsMode:
		base = title + ": " + p.prLabel
	case p.selectedHash != "":
		short := p.selectedHash
		if len(short) > git.ShortHashLen {
			short = short[:git.ShortHashLen]
		}
		base = title + ": " + short
	case p.filterLabel != "":
		base = title + ": " + p.filterLabel
	case p.refLabel != "":
		base = title + ": " + p.refLabel
	case p.ref != "":
		base = title + ": " + p.ref
	}
	if p.authorFilter != "" {
		base += " [@" + p.authorFilter + "]"
	}
	return base
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: keyEnter, Description: "Show commit details", Action: "detail"},
		{Key: "PgDn", Description: "Page down", Action: "page_down"},
		{Key: "PgUp", Description: "Page up", Action: "page_up"},
		{Key: "g", Description: "Go to top", Action: "go_top"},
		{Key: "G", Description: "Go to bottom", Action: "go_bottom"},
		{Key: "y", Description: "Copy commit hash", Action: "copy_hash"},
		{Key: "x", Description: "Export commit as a .patch file", Action: "export_patch"},
		{Key: "/", Description: "Search commits", Action: "search"},
		{Key: "S", Description: "Search commit content (pickaxe)", Action: "pickaxe"},
		{Key: "M", Description: "Search commit messages (grep)", Action: "grep"},
		{Key: "a", Description: "Filter by commit author", Action: "author_filter"},
		{Key: "A", Description: "Amend last commit", Action: "amend"},
		{Key: "r", Description: "Reword last commit", Action: "reword"},
	}
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
func (p *Panel) handleCommitsLoaded(msg commitsLoadedMsg) (panels.Panel, tea.Cmd) {
	p.loading = false
	if len(msg.commits) < p.pageSize {
		p.allLoaded = true
	}
	if msg.append {
		p.commits = append(p.commits, msg.commits...)
	} else {
		p.commits = msg.commits
		p.cursor = 0
		p.offset = 0
	}
	if p.searchMode && p.searchQuery != "" {
		p.applyFilters()
	} else if p.authorFilter != "" {
		p.applyFilters()
	}
	return p, nil
}

func (p *Panel) handleBranchChanged(msg panels.BranchChangedMsg) (panels.Panel, tea.Cmd) {
	p.ref = msg.Name
	p.refLabel = msg.Name
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.gitClient = nil
		p.ref = ""
		p.refLabel = ""
		p.filter = filterNone
		p.filterPath = ""
		p.filterLabel = ""
		p.resetState()
		p.loading = false
		return p, nil
	}
	p.gitClient = client
	p.ref = ""
	p.refLabel = ""
	p.filter = filterNone
	p.filterPath = ""
	p.filterLabel = ""
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleSwitchWorktree(msg panels.ChangeDirectoryMsg) (panels.Panel, tea.Cmd) {
	// When switching worktrees, reload from HEAD. A subsequent
	// BranchChangedMsg will set the specific ref if needed.
	p.ref = ""
	p.refLabel = msg.Path
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleWorktreeChanged() (panels.Panel, tea.Cmd) {
	p.loading = true
	p.allLoaded = false
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleBranchSelected(msg panels.BranchSelectedMsg) (panels.Panel, tea.Cmd) {
	p.filter = filterBranch
	p.filterPath = ""
	p.filterLabel = msg.Name
	p.ref = msg.Name
	p.refLabel = msg.Name
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleFileSelected(msg panels.FileSelectedMsg) (panels.Panel, tea.Cmd) {
	// When a commit is selected, the filetree enters commit-files mode and
	// emits FileSelectedMsg as a side effect. Ignore it so the commit list
	// doesn't flicker with a reload.
	if p.selectedHash != "" {
		return p, nil
	}
	p.filter = filterFile
	p.filterPath = msg.Path
	p.filterLabel = filepath.Base(msg.Path)
	p.ref = ""
	p.refLabel = ""
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleFolderSelected(msg panels.FolderSelectedMsg) (panels.Panel, tea.Cmd) {
	if p.selectedHash != "" {
		return p, nil
	}
	// Use the last two path components for a short label (e.g. "internal/git").
	label := filepath.Base(msg.Path)
	parent := filepath.Base(filepath.Dir(msg.Path))
	if parent != "." && parent != string(filepath.Separator) {
		label = parent + "/" + label
	}
	p.filter = filterFolder
	p.filterPath = msg.Path
	p.filterLabel = label
	p.ref = ""
	p.refLabel = ""
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleWorktreeSelected(msg panels.WorktreeSelectedMsg) (panels.Panel, tea.Cmd) {
	p.filter = filterWorktree
	p.filterPath = ""
	p.filterLabel = msg.Branch
	p.ref = msg.Branch
	p.refLabel = msg.Branch
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleRemoteSelected(msg panels.RemoteSelectedMsg) (panels.Panel, tea.Cmd) {
	p.filter = filterRemote
	p.filterPath = ""
	p.filterLabel = msg.Name
	p.ref = ""
	p.refLabel = msg.Name
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleStashSelected(msg panels.StashSelectedMsg) (panels.Panel, tea.Cmd) {
	label := "stash@{" + strconv.Itoa(msg.Index) + "}"
	p.filter = filterStash
	p.filterPath = ""
	p.filterLabel = label
	p.ref = msg.Hash
	p.refLabel = label
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

// resetState clears transient state when the ref changes.
func (p *Panel) resetState() {
	p.loading = true
	p.allLoaded = false
	p.commits = nil
	p.cursor = 0
	p.offset = 0
	p.searchMode = false
	p.searchQuery = ""
	p.pickaxeMode = false
	p.pickaxeTerm = ""
	p.grepMode = false
	p.grepTerm = ""
	p.authorFilter = ""
	p.filteredIdx = nil
	p.detailMode = false
	p.detailLines = nil
	p.detailOffset = 0
}

// clearFilter resets all contextual filter state back to defaults.
func (p *Panel) clearFilter() {
	p.filter = filterNone
	p.filterPath = ""
	p.filterLabel = ""
	p.ref = ""
	p.refLabel = ""
}

// ---------------------------------------------------------------------------
// PR-commits mode
// ---------------------------------------------------------------------------
// handlePRCommitsLoaded switches the panel into PR-commits mode,
// converting PRCommit values into git.Commit for rendering reuse.
func (p *Panel) handlePRCommitsLoaded(msg panels.PRCommitsLoadedMsg) (panels.Panel, tea.Cmd) {
	p.prCommitsMode = true
	p.prCommits = msg.Commits
	p.prNumber = msg.Number
	p.prLabel = fmt.Sprintf("PR #%d", msg.Number)
	// Convert PR commits to git.Commit format so the existing list
	// renderer can display them without any changes.
	p.commits = nil
	for _, c := range msg.Commits {
		subject := c.Message
		if idx := strings.Index(subject, "\n"); idx > 0 {
			subject = subject[:idx]
		}
		short := c.SHA
		if len(short) > git.ShortHashLen {
			short = short[:git.ShortHashLen]
		}
		// Best-effort date parse; fall back to zero time.
		var dt time.Time
		if c.Date != "" {
			if t, err := time.Parse(time.RFC3339, c.Date); err == nil {
				dt = t
			}
		}
		p.commits = append(p.commits, git.Commit{
			Hash:      c.SHA,
			ShortHash: short,
			Author:    c.Author,
			Date:      dt,
			Subject:   subject,
		})
	}
	p.cursor = 0
	p.offset = 0
	p.selectedHash = ""
	p.selectedSubject = ""
	p.searchMode = false
	p.detailMode = false
	p.loading = false
	p.allLoaded = true
	return p, nil
}

// exitPRCommitsMode restores the panel to normal branch-commit mode.
func (p *Panel) exitPRCommitsMode() (panels.Panel, tea.Cmd) {
	if !p.prCommitsMode {
		return p, nil
	}
	p.prCommitsMode = false
	p.prCommits = nil
	p.prNumber = 0
	p.prLabel = ""
	p.selectedHash = ""
	p.selectedSubject = ""
	// Reload branch commits.
	p.resetState()
	return p, p.loadCommitsCmd(0, false)
}

// loadMore triggers pagination if near the bottom and not already loading.
func (p *Panel) loadMore() tea.Cmd {
	if p.loading || p.allLoaded {
		return nil
	}
	p.loading = true
	return p.loadCommitsCmd(len(p.commits), true)
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (p *Panel) renderList(width, height int) string {
	total := p.activeLen()
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > total {
		end = total
	}
	for i := p.offset; i < end; i++ {
		c := p.commitAt(i)
		isSelected := p.selectedHash != "" && c.Hash == p.selectedHash
		styles := p.clStyles
		if isSelected {
			styles.Subject = styles.Subject.Bold(true)
		}
		lines = append(lines, commitrender.RenderLine(commitrender.Params{
			Commit:        c,
			Width:         width,
			IsCursor:      i == p.cursor,
			Styles:        styles,
			IsSelected:    isSelected,
			SelectedBg:    "#3B3F52", // subtler highlight for selected-but-not-cursor
			ShowSignature: true,
		}))
	}
	// Loading indicator.
	if p.loading && len(lines) < height {
		loadingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim))
		lines = append(lines, commitrender.TruncateOrPad(loadingStyle.Render("  Loading more commits..."), width))
	}
	// Bottom status bar: pickaxe content-search takes precedence over the
	// client-side subject search when both are relevant.
	var barLine string
	switch {
	case p.grepMode || p.grepTerm != "":
		barLine = p.renderGrepBar(width)
	case p.pickaxeMode || p.pickaxeTerm != "":
		barLine = p.renderPickaxeBar(width)
	case p.searchMode:
		barLine = p.renderSearchBar(width)
	}
	if barLine != "" {
		if len(lines) >= height {
			lines[height-1] = barLine
		} else {
			for len(lines) < height-1 {
				lines = append(lines, strings.Repeat(" ", width))
			}
			lines = append(lines, barLine)
		}
	}
	// Pad remaining height.
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (p *Panel) renderSearchBar(width int) string {
	searchStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(p.colors.SearchBg)).
		Foreground(lipgloss.Color(p.colors.SearchFg))
	prompt := " /" + p.searchQuery
	matchCount := len(p.filteredIdx)
	if p.searchQuery != "" {
		prompt += fmt.Sprintf("  [%d matches]", matchCount)
	}
	return searchStyle.Width(width).Render(prompt)
}

func (p *Panel) renderPickaxeBar(width int) string {
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(p.colors.SearchBg)).
		Foreground(lipgloss.Color(p.colors.SearchFg))
	mode := "text"
	if p.pickaxeRegex {
		mode = "regex"
	}
	prompt := " pickaxe(" + mode + "): " + p.pickaxeTerm
	switch {
	case p.pickaxeMode:
		prompt += "  (Tab: regex, Enter: search, Esc: clear)"
	case p.pickaxeTerm != "":
		prompt += fmt.Sprintf("  [%d results]", len(p.commits))
	}
	return barStyle.Width(width).Render(prompt)
}

func (p *Panel) renderGrepBar(width int) string {
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(p.colors.SearchBg)).
		Foreground(lipgloss.Color(p.colors.SearchFg))
	prompt := " grep: " + p.grepTerm
	switch {
	case p.grepMode:
		prompt += "  (Enter: search, Esc: clear)"
	case p.grepTerm != "":
		prompt += fmt.Sprintf("  [%d results]", len(p.commits))
	}
	return barStyle.Width(width).Render(prompt)
}

func (p *Panel) renderDetail(width, height int) string {
	if len(p.detailLines) == 0 {
		return ""
	}
	lines := make([]string, 0, height)
	end := p.detailOffset + height
	if end > len(p.detailLines) {
		end = len(p.detailLines)
	}
	for i := p.detailOffset; i < end; i++ {
		lines = append(lines, commitrender.TruncateOrPad(p.detailLines[i], width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= p.activeLen() {
		return p, nil
	}
	p.cursor = idx
	return p.selectCommit()
}

// handleMouseDoubleClick selects the commit at the double-clicked row,
// triggering the same action as pressing Enter (commit selection).
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= p.activeLen() {
		return p, nil
	}
	p.cursor = idx
	itemType := actions.ItemCommit
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
)

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// handleMouseRightClick shows a context menu for the commit at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= p.activeLen() {
		return p, nil
	}
	p.cursor = idx
	c := p.commitAt(p.cursor)
	label := c.ShortHash + " " + panels.StripANSI(c.Subject)
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemCommit, label)
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
func (p *Panel) clearPending() {
	p.pendingOp = ""
	p.pendingName = ""
}

// handleModalResult processes the result from the action picker modal.
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

// executeRightClickAction dispatches a right-click action to the appropriate method.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionShowDetail:
		return p.selectCommit()
	case actions.ActionCopyHash:
		return p.copyHash()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.focused {
		return p, nil
	}
	if p.pickaxeMode {
		return p.handlePickaxeKey(msg)
	}
	if p.grepMode {
		return p.handleGrepKey(msg)
	}
	if p.searchMode {
		return p.handleSearchKey(msg)
	}
	if p.detailMode {
		return p.handleDetailKey(msg)
	}
	switch msg.String() {
	case "j", "down":
		return p.moveCursorDown()
	case "k", "up":
		p.moveCursorUp()
	case keyEnter:
		return p.selectCommit()
	case "pgdown":
		p.pageDown()
	case "pgup":
		p.pageUp()
	case "g":
		p.goToTop()
	case "G":
		p.goToBottom()
	case "y":
		return p.copyHash()
	case "x":
		return p.exportPatch()
	case "/":
		p.searchMode = true
		p.searchQuery = ""
	case "S":
		p.pickaxeMode = true
	case "M":
		p.grepMode = true
	case "a":
		return p.toggleAuthorFilter()
	case "A":
		return p, func() tea.Msg { return panels.AmendRequestMsg{} }
	case "r":
		if len(p.commits) > 0 && p.cursor == 0 {
			c := p.commits[0]
			msg := c.Subject
			if c.Body != "" {
				msg = c.Subject + "\n\n" + c.Body
			}
			return p, func() tea.Msg { return panels.RewordRequestMsg{OldMessage: msg} }
		}
	case "esc": //nolint:goconst // inline string is more readable here
		// Progressive reset: pickaxe → PR-commits mode → selected commit → filter → nothing.
		if p.pickaxeTerm != "" {
			p.pickaxeTerm = ""
			p.resetState()
			return p, p.loadCommitsCmd(0, false)
		}
		if p.grepTerm != "" {
			p.grepTerm = ""
			p.resetState()
			return p, p.loadCommitsCmd(0, false)
		}
		if p.prCommitsMode {
			return p.exitPRCommitsMode()
		}
		if p.selectedHash != "" {
			return p.deselectCommit()
		}
		if p.authorFilter != "" {
			p.authorFilter = ""
			p.applyFilters()
			return p, nil
		}
		if p.filter != filterNone {
			p.clearFilter()
			p.resetState()
			return p, p.loadCommitsCmd(0, false)
		}
	}
	return p, nil
}

func (p *Panel) handleSearchKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		p.searchMode = false
	case "esc":
		p.searchMode = false
		p.searchQuery = ""
		p.applyFilters()
	case keyBackspace:
		if len(p.searchQuery) > 0 {
			p.searchQuery = p.searchQuery[:len(p.searchQuery)-1]
			p.applyFilters()
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= ' ' {
			p.searchQuery += ch
			p.applyFilters()
		}
	}
	return p, nil
}

func (p *Panel) handlePickaxeKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		// Run the content search server-side. Empty term clears the filter.
		p.pickaxeMode = false
		p.cursor = 0
		p.offset = 0
		p.allLoaded = false
		return p, p.loadCommitsCmd(0, false)
	case "esc":
		hadTerm := p.pickaxeTerm != ""
		p.pickaxeMode = false
		p.pickaxeTerm = ""
		if hadTerm {
			p.cursor = 0
			p.offset = 0
			p.allLoaded = false
			return p, p.loadCommitsCmd(0, false)
		}
	case "tab":
		p.pickaxeRegex = !p.pickaxeRegex
	case keyBackspace:
		if len(p.pickaxeTerm) > 0 {
			p.pickaxeTerm = p.pickaxeTerm[:len(p.pickaxeTerm)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= ' ' {
			p.pickaxeTerm += ch
		}
	}
	return p, nil
}

func (p *Panel) handleGrepKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		// Run the message search server-side. Empty term clears the filter.
		p.grepMode = false
		p.cursor = 0
		p.offset = 0
		p.allLoaded = false
		return p, p.loadCommitsCmd(0, false)
	case "esc":
		hadTerm := p.grepTerm != ""
		p.grepMode = false
		p.grepTerm = ""
		if hadTerm {
			p.cursor = 0
			p.offset = 0
			p.allLoaded = false
			return p, p.loadCommitsCmd(0, false)
		}
	case keyBackspace:
		if len(p.grepTerm) > 0 {
			p.grepTerm = p.grepTerm[:len(p.grepTerm)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= ' ' {
			p.grepTerm += ch
		}
	}
	return p, nil
}

func (p *Panel) handleDetailKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "esc", keyEnter, "q":
		p.detailMode = false
		p.detailLines = nil
		p.detailOffset = 0
	case "j", "down":
		if p.detailOffset < len(p.detailLines)-1 {
			p.detailOffset++
		}
	case "k", "up":
		if p.detailOffset > 0 {
			p.detailOffset--
		}
	case "pgdown":
		p.detailOffset += p.height / 2
		if p.detailOffset > len(p.detailLines)-1 {
			p.detailOffset = max(0, len(p.detailLines)-1)
		}
	case "pgup":
		p.detailOffset -= p.height / 2
		if p.detailOffset < 0 {
			p.detailOffset = 0
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (p *Panel) maxCursor() int {
	return max(0, p.activeLen()-1)
}

func (p *Panel) moveCursorDown() (panels.Panel, tea.Cmd) {
	if p.cursor < p.maxCursor() {
		p.cursor++
		p.ensureCursorVisible()
	}
	// Trigger pagination when near the bottom.
	if !p.allLoaded && p.filteredIdx == nil && p.cursor >= len(p.commits)-loadMoreThreshold {
		return p, p.loadMore()
	}
	return p, nil
}

func (p *Panel) moveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureCursorVisible()
	}
}

func (p *Panel) pageDown() {
	p.cursor += p.height / 2
	if p.cursor > p.maxCursor() {
		p.cursor = p.maxCursor()
	}
	p.ensureCursorVisible()
}

func (p *Panel) pageUp() {
	p.cursor -= p.height / 2
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureCursorVisible()
}

func (p *Panel) goToTop() {
	p.cursor = 0
	p.ensureCursorVisible()
}

func (p *Panel) goToBottom() {
	p.cursor = p.maxCursor()
	p.ensureCursorVisible()
}

func (p *Panel) ensureCursorVisible() {
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, p.height)
}

// ---------------------------------------------------------------------------
// Commit selection (filters file tree without entering detail view)
// ---------------------------------------------------------------------------
func (p *Panel) selectCommit() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= p.activeLen() {
		return p, nil
	}
	c := p.commitAt(p.cursor)
	p.selectedHash = c.Hash
	p.selectedSubject = panels.StripANSI(c.Subject)
	hash := c.Hash
	subject := panels.StripANSI(c.Subject)
	return p, func() tea.Msg {
		return panels.CommitSelectedMsg{Hash: hash, Subject: subject}
	}
}

func (p *Panel) deselectCommit() (panels.Panel, tea.Cmd) {
	p.selectedHash = ""
	p.selectedSubject = ""
	return p, func() tea.Msg {
		return panels.CommitDeselectedMsg{}
	}
}

// ---------------------------------------------------------------------------
// Detail view
// ---------------------------------------------------------------------------
// signatureDetail renders a human-readable, colored description of a commit's
// signature verification status for the detail view.
func (p *Panel) signatureDetail(c git.Commit) string {
	var label, color string
	switch c.Signature {
	case git.SigGood:
		label, color = "verified", p.colors.SigGood
	case git.SigBad:
		label, color = "bad signature", p.colors.SigBad
	case git.SigUnknown:
		label, color = "unknown validity", p.colors.SigCaution
	case git.SigExpired:
		label, color = "expired", p.colors.SigCaution
	case git.SigRevoked:
		label, color = "revoked key", p.colors.SigCaution
	case git.SigError:
		label, color = "unverifiable", p.colors.SigCaution
	default:
		return ""
	}
	if c.Signer != "" {
		label += " (" + panels.StripANSI(c.Signer) + ")"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(label)
}

func (p *Panel) showDetail() tea.Cmd {
	if p.cursor < 0 || p.cursor >= p.activeLen() {
		return nil
	}
	c := p.commitAt(p.cursor)
	headerStyle := lipgloss.NewStyle().Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim))
	hashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Hash))
	authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Author))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Date))
	refStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Refs))
	var lines []string
	lines = append(lines, headerStyle.Render("Commit Details"))
	lines = append(lines, dimStyle.Render(strings.Repeat("─", 40)))
	lines = append(lines, "")
	lines = append(lines, "Commit:  "+hashStyle.Render(c.Hash))
	lines = append(lines, "Author:  "+authorStyle.Render(panels.StripANSI(c.Author)+" <"+panels.StripANSI(c.AuthorEmail)+">"))
	lines = append(lines, "Date:    "+dateStyle.Render(c.Date.Format("2006-01-02 15:04:05 -0700")))
	if len(c.Refs) > 0 {
		sanitizedRefs := make([]string, len(c.Refs))
		for i, r := range c.Refs {
			sanitizedRefs[i] = panels.StripANSI(r)
		}
		lines = append(lines, "Refs:    "+refStyle.Render(strings.Join(sanitizedRefs, ", ")))
	}
	if len(c.Parents) > 0 {
		parentHash := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Hash))
		lines = append(lines, "Parents: "+parentHash.Render(strings.Join(c.Parents, " ")))
	}
	if c.Signature.Present() {
		lines = append(lines, "Signed:  "+p.signatureDetail(c))
	}
	lines = append(lines, "")
	lines = append(lines, "    "+panels.StripANSI(c.Subject))
	if c.Body != "" {
		lines = append(lines, "")
		for _, bodyLine := range strings.Split(c.Body, "\n") {
			lines = append(lines, "    "+panels.StripANSI(bodyLine))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Press Enter, Escape, or q to return"))
	p.detailMode = true
	p.detailLines = lines
	p.detailOffset = 0
	// Emit CommitSelectedMsg so other panels can react.
	hash := c.Hash
	subject := panels.StripANSI(c.Subject)
	return func() tea.Msg {
		return panels.CommitSelectedMsg{Hash: hash, Subject: subject}
	}
}

// ---------------------------------------------------------------------------
// Clipboard
func (p *Panel) copyHash() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= p.activeLen() {
		return p, nil
	}
	c := p.commitAt(p.cursor)
	hash := c.ShortHash
	if err := panels.CopyToClipboard(p.ctx, hash); err != nil {
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
			Message: "Copied: " + hash,
			Level:   notify.Success,
		}
	}
}

// ---------------------------------------------------------------------------
// Patch export
// ---------------------------------------------------------------------------

// exportPatch writes the commit under the cursor to a single-commit patch
// file at the repository root, named after the commit subject. The written
// path is shown as a toast; any failure surfaces as an error toast.
func (p *Panel) exportPatch() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= p.activeLen() {
		return p, nil
	}
	c := p.commitAt(p.cursor)
	if c.Hash == "" {
		return p, nil
	}

	patch, err := p.gitClient.FormatPatch(p.ctx, c.Hash)
	if err != nil {
		return p, patchErrorToast(err)
	}
	root, err := p.gitClient.RepoRoot(p.ctx)
	if err != nil {
		return p, patchErrorToast(err)
	}

	dest := filepath.Join(root, patchFileName(c.Subject))
	if err := os.WriteFile(dest, []byte(patch), 0o600); err != nil {
		return p, patchErrorToast(err)
	}

	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Wrote " + dest,
			Level:   notify.Success,
		}
	}
}

func patchErrorToast(err error) tea.Cmd {
	msg := err.Error()
	return func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Export failed: " + msg,
			Level:   notify.Error,
		}
	}
}

// patchFileName builds a filesystem-safe patch file name from a commit
// subject, mirroring the "NNNN-slug.patch" form that git format-patch uses.
// A single commit is always sequence 0001.
func patchFileName(subject string) string {
	slug := slugify(subject)
	if slug == "" {
		slug = "patch"
	}
	const maxSlug = 50
	if len(slug) > maxSlug {
		slug = strings.Trim(slug[:maxSlug], "-")
	}
	return "0001-" + slug + ".patch"
}

// slugify lowercases s and replaces every run of non-alphanumeric characters
// with a single dash, trimming leading and trailing dashes. The result is
// safe to use as a file name on any platform.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------
// applyFilters recomputes filteredIdx from the active search query and author
// filter. When neither is set, filteredIdx is cleared so the full commit list
// shows. A commit must match both the search query and the author filter to be
// included.
func (p *Panel) applyFilters() {
	if p.searchQuery == "" && p.authorFilter == "" {
		p.filteredIdx = nil
		p.cursor = 0
		p.offset = 0
		return
	}
	query := strings.ToLower(p.searchQuery)
	p.filteredIdx = p.filteredIdx[:0]
	for i, c := range p.commits {
		if p.authorFilter != "" && !strings.EqualFold(c.Author, p.authorFilter) {
			continue
		}
		if query != "" &&
			!containsFold(c.Subject, query) &&
			!containsFold(c.Author, query) &&
			!containsFold(c.ShortHash, query) {
			continue
		}
		p.filteredIdx = append(p.filteredIdx, i)
	}
	p.cursor = 0
	p.offset = 0
}

// toggleAuthorFilter filters the commit list to the author of the commit under
// the cursor. Pressing it again on the same author clears the filter; pressing
// it on a different author switches to that author.
func (p *Panel) toggleAuthorFilter() (panels.Panel, tea.Cmd) {
	if p.activeLen() == 0 {
		return p, nil
	}
	author := p.commitAt(p.cursor).Author
	if author == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No author to filter by", Level: notify.Info}
		}
	}
	if strings.EqualFold(p.authorFilter, author) {
		p.authorFilter = ""
		p.applyFilters()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Author filter cleared", Level: notify.Info}
		}
	}
	p.authorFilter = author
	p.applyFilters()
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Filtering by author: " + author, Level: notify.Success}
	}
}

// containsFold reports whether s contains the already-lowered substr
// using case-insensitive comparison without allocating new strings.
func containsFold(s, lowerSubstr string) bool {
	n := len(lowerSubstr)
	if n == 0 {
		return true
	}
	if n > len(s) {
		return false
	}
	for i := 0; i <= len(s)-n; i++ {
		if strings.EqualFold(s[i:i+n], lowerSubstr) {
			return true
		}
	}
	return false
}

// activeLen returns the number of items in the active list.
func (p *Panel) activeLen() int {
	if p.filteredIdx != nil {
		return len(p.filteredIdx)
	}
	return len(p.commits)
}

// commitAt returns the commit at the given index in the active list.
func (p *Panel) commitAt(idx int) git.Commit {
	if p.filteredIdx != nil {
		if idx < 0 || idx >= len(p.filteredIdx) {
			return git.Commit{}
		}
		mapped := p.filteredIdx[idx]
		if mapped < 0 || mapped >= len(p.commits) {
			return git.Commit{}
		}
		return p.commits[mapped]
	}
	if idx < 0 || idx >= len(p.commits) {
		return git.Commit{}
	}
	return p.commits[idx]
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
// relativeDate formats a time.Time as a human-readable relative date.
func relativeDate(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return labelJustNow
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(d.Hours() / 24 / 365)
		if years <= 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}
