// Package gitlog implements the git log panel for grut.
// It provides a scrollable commit history with ASCII graph rendering,
// paginated loading, search filtering, and commit detail view.
package gitlog

import (
	"context"
	"fmt"
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
	"github.com/mattn/go-runewidth"
)

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
)

// defaultPageSize is the number of commits to load per page.
const defaultPageSize = 500

// loadMoreThreshold triggers a new page load when the cursor is within
// this many lines from the bottom of the loaded data.
const loadMoreThreshold = 50

// debounceInterval prevents rapid-fire pagination requests.
const debounceInterval = 200 * time.Millisecond

// authorColMaxWidth is the maximum rune-width used for the author column
// in the log list view. Wider names are truncated to keep the layout compact.
const authorColMaxWidth = 14

type panelColors struct {
	Hash     string
	Date     string
	Author   string
	Refs     string
	Subject  string
	Dim      string
	CursorBg string
	Graph    string
	SearchBg string
	SearchFg string
}

// commitLineStyles caches lipgloss.Style objects derived from panelColors so
// that renderCommitLine does not allocate new styles on every call (~7,200
// allocations/sec at 20 visible commits * 60 fps).
type commitLineStyles struct {
	hash    lipgloss.Style
	date    lipgloss.Style
	author  lipgloss.Style
	subject lipgloss.Style
	ref     lipgloss.Style
	graph   lipgloss.Style
	cursor  lipgloss.Style
}

func newCommitLineStyles(c panelColors) commitLineStyles {
	return commitLineStyles{
		hash:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hash)),
		date:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.Date)),
		author:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.Author)),
		subject: lipgloss.NewStyle().Foreground(lipgloss.Color(c.Subject)),
		ref:     lipgloss.NewStyle().Foreground(lipgloss.Color(c.Refs)).Bold(true),
		graph:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.Graph)),
		cursor:  lipgloss.NewStyle().Background(lipgloss.Color(c.CursorBg)),
	}
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Hash:     "#D4B84A",
		Date:     colorDim,
		Author:   "#6B9E56",
		Refs:     "#7A9EBF",
		Subject:  "#999999",
		Dim:      colorDim,
		CursorBg: "#2A2A2A",
		Graph:    colorDim,
		SearchBg: "#2A2A2A",
		SearchFg: "#D4D4D4",
	}
	if th != nil {
		c.Hash = th.Colors.NormalYellow
		c.Date = th.Colors.BrightBlack
		c.Author = th.Colors.NormalGreen
		c.Refs = th.Colors.BrightBlue
		c.Subject = th.Colors.NormalWhite
		c.Dim = th.Colors.BrightBlack
		c.CursorBg = th.Colors.SelectionBg
		c.Graph = th.Colors.BrightBlack
		c.SearchBg = th.Colors.SelectionBg
		c.SearchFg = th.Colors.SelectionFg
	}
	return c
}

// displayLine represents a single rendered line in the viewport.
type displayLine struct {
	text      string // pre-rendered text
	commitIdx int    // index into commits; -1 for connector lines
}

// Panel is the git log panel. It implements [panels.Panel].
type Panel struct {
	lastLoadAt time.Time
	actionsCfg config.ActionsConfig
	gitClient  git.StatusReader
	ctx        context.Context
	// Branch filtering — when set, shows commits for this ref instead of HEAD.
	selectedRef string
	searchQuery string
	pendingOp   string
	pendingName string
	commits     []git.Commit
	// Display state.
	display      []displayLine // flattened lines (commits + connectors)
	commitY      []int         // display index for each commit (for cursor mapping)
	filteredIdx  []int         // indices into commits matching search; nil = show all
	filteredDL   []displayLine
	filteredCmtY []int
	detailLines  []string
	cfg          config.GitConfig
	colors       panelColors
	clStyles     commitLineStyles
	theme        *theme.Theme
	cursor       int // index into commits
	offset       int // viewport offset into display
	width        int
	height       int
	pageSize     int
	detailOffset int
	focused      bool
	// Pagination.
	loading   bool
	allLoaded bool
	// Search.
	searchMode bool
	// Detail view.
	detailMode bool
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new git log panel.
func New(client git.StatusReader, cfg config.GitConfig, th *theme.Theme) *Panel {
	ps := defaultPageSize
	if cfg.MaxLogEntries > 0 {
		ps = cfg.MaxLogEntries
	}
	colors := initColors(th)
	return &Panel{
		gitClient: client,
		cfg:       cfg,
		pageSize:  ps,
		colors:    colors,
		clStyles:  newCommitLineStyles(colors),
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
	// Use --all when graph is enabled, but not when filtering to a specific ref.
	showAll := p.cfg.ShowCommitGraph && p.selectedRef == ""
	ctx := p.ctx
	ref := p.selectedRef
	return func() tea.Msg {
		commits, err := client.Log(ctx, git.LogOpts{
			MaxCount: pageSize,
			Skip:     skip,
			All:      showAll,
			Ref:      ref,
		})
		if err != nil {
			return notify.ShowToastMsg{
				Message: "git log: " + err.Error(),
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
	case panels.BranchSelectedMsg:
		return p.handleBranchSelected(msg.Name)
	case panels.BranchChangedMsg:
		// After checkout, reset to HEAD.
		return p.handleBranchSelected("")
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
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
	return p.renderLog(width, height)
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
func (p *Panel) Title() string { return "Commits" }

// SetActionsCfg stores the actions configuration for right-click support.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

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
		{Key: "/", Description: "Search commits", Action: "search"},
		{Key: "A", Description: "Amend last commit", Action: "amend"},
		{Key: "r", Description: "Reword last commit", Action: "reword"},
	}
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
// handleBranchSelected reloads commits for a specific branch ref.
// An empty name resets to HEAD (used after checkout).
func (p *Panel) handleBranchSelected(name string) (panels.Panel, tea.Cmd) {
	if name == p.selectedRef {
		return p, nil
	}
	p.selectedRef = name
	p.cursor = 0
	p.offset = 0
	p.allLoaded = false
	p.loading = true
	p.searchMode = false
	p.searchQuery = ""
	p.filteredIdx = nil
	p.filteredDL = nil
	p.filteredCmtY = nil
	return p, p.loadCommitsCmd(0, false)
}

// handleRepoChanged replaces the git client and reloads commits for the
// new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.gitClient = nil
		p.commits = nil
		p.display = nil
		p.commitY = nil
		p.selectedRef = ""
		p.cursor = 0
		p.offset = 0
		p.allLoaded = true
		p.loading = false
		p.searchMode = false
		p.searchQuery = ""
		p.filteredIdx = nil
		p.filteredDL = nil
		p.filteredCmtY = nil
		p.detailMode = false
		p.detailLines = nil
		p.detailOffset = 0
		return p, nil
	}
	p.gitClient = client
	p.commits = nil
	p.display = nil
	p.commitY = nil
	p.selectedRef = ""
	p.cursor = 0
	p.offset = 0
	p.allLoaded = false
	p.loading = true
	p.searchMode = false
	p.searchQuery = ""
	p.filteredIdx = nil
	p.filteredDL = nil
	p.filteredCmtY = nil
	p.detailMode = false
	p.detailLines = nil
	p.detailOffset = 0
	return p, p.loadCommitsCmd(0, false)
}

func (p *Panel) handleCommitsLoaded(msg commitsLoadedMsg) (panels.Panel, tea.Cmd) {
	p.loading = false
	if len(msg.commits) < p.pageSize {
		p.allLoaded = true
	}
	if msg.append {
		p.commits = append(p.commits, msg.commits...)
	} else {
		p.commits = msg.commits
	}
	p.rebuildDisplay()
	if p.searchMode && p.searchQuery != "" {
		p.applySearch()
	}
	return p, nil
}

// loadMore triggers pagination if near the bottom and not already loading.
func (p *Panel) loadMore() tea.Cmd {
	if p.loading || p.allLoaded {
		return nil
	}
	now := time.Now()
	if now.Sub(p.lastLoadAt) < debounceInterval {
		return nil
	}
	p.loading = true
	p.lastLoadAt = now
	return p.loadCommitsCmd(len(p.commits), true)
}

// ---------------------------------------------------------------------------
// Display building
// ---------------------------------------------------------------------------
// rebuildDisplay flattens commits and graph connectors into displayLines.
func (p *Panel) rebuildDisplay() {
	graph := NewGraphRenderer()
	p.display = p.display[:0]
	p.commitY = p.commitY[:0]
	for i, c := range p.commits {
		entry := graph.RenderCommit(c)
		// Record the display-line index for this commit.
		p.commitY = append(p.commitY, len(p.display))
		// Commit line.
		p.display = append(p.display, displayLine{
			commitIdx: i,
			text:      entry.Prefix,
		})
		// Connector lines.
		for _, conn := range entry.Connectors {
			p.display = append(p.display, displayLine{
				commitIdx: -1,
				text:      conn,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (p *Panel) renderLog(width, height int) string {
	dl := p.activeDisplay()
	cmtY := p.activeCommitY()
	// Resolve cursor's display line.
	cursorDL := -1
	if p.cursor >= 0 && p.cursor < len(cmtY) {
		cursorDL = cmtY[p.cursor]
	}
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > len(dl) {
		end = len(dl)
	}
	for i := p.offset; i < end; i++ {
		d := dl[i]
		if d.commitIdx >= 0 && d.commitIdx < len(p.commits) {
			lines = append(lines, p.renderCommitLine(p.commits[d.commitIdx], d.text, width, i == cursorDL))
		} else {
			// Connector line — just show the graph portion.
			graphStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Graph))
			line := graphStyle.Render(d.text)
			lines = append(lines, truncateOrPad(line, width))
		}
	}
	// Loading indicator.
	if p.loading && len(lines) < height {
		loadingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim))
		lines = append(lines, truncateOrPad(loadingStyle.Render("  Loading more commits..."), width))
	}
	// Search bar at top if in search mode.
	if p.searchMode {
		searchLine := p.renderSearchBar(width)
		if len(lines) >= height {
			lines[height-1] = searchLine
		} else {
			// Pad and append search bar at the bottom.
			for len(lines) < height-1 {
				lines = append(lines, strings.Repeat(" ", width))
			}
			lines = append(lines, searchLine)
		}
	}
	// Pad remaining height.
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (p *Panel) renderCommitLine(c git.Commit, graphPrefix string, width int, isCursor bool) string {
	hashStyle := p.clStyles.hash
	dateStyle := p.clStyles.date
	authorStyle := p.clStyles.author
	subjectStyle := p.clStyles.subject
	refStyle := p.clStyles.ref
	graphStyle := p.clStyles.graph
	// Build right-side fixed columns first to determine how much space subject gets.
	// SHA is always pinned to the right. Author and date appear as width allows.
	hashCol := panels.StripANSI(c.ShortHash)
	hashW := len(hashCol)
	gap := "  " // column separator
	// Compute author/date only when they'll fit.
	authorCol := panels.StripANSI(c.Author)
	if runewidth.StringWidth(authorCol) > authorColMaxWidth {
		authorCol = runewidth.Truncate(authorCol, authorColMaxWidth, "")
	}
	dateCol := c.Date.Format("2006-01-02")
	graphW := lipgloss.Width(graphPrefix)
	if graphW > 0 {
		graphW += 2 // gap after graph
	}
	// Progressive columns: show more as width grows.
	// Always: graph + subject + gap + hash
	// Medium: + author
	// Wide: + date
	minSubjectW := 10
	baseUsed := graphW + minSubjectW + len(gap) + hashW
	showAuthor := baseUsed+len(gap)+len(authorCol) <= width
	showDate := showAuthor && baseUsed+len(gap)+len(authorCol)+len(gap)+len(dateCol) <= width
	// Compute right-side string (everything after subject).
	var rightParts []string
	if showDate {
		rightParts = append(rightParts, dateStyle.Render(dateCol))
	}
	if showAuthor {
		rightParts = append(rightParts, authorStyle.Render(authorCol))
	}
	rightParts = append(rightParts, hashStyle.Render(hashCol))
	rightSide := strings.Join(rightParts, gap)
	rightW := lipgloss.Width(rightSide)
	// Subject + refs fill the remaining space.
	subjectSpace := width - graphW - len(gap) - rightW
	if subjectSpace < minSubjectW {
		subjectSpace = minSubjectW
	}
	// Build subject text with inline refs. Sanitise untrusted git data
	// to prevent ANSI escape-sequence injection (CWE-150).
	safeSubject := panels.StripANSI(c.Subject)
	safeRefs := make([]string, len(c.Refs))
	for i, r := range c.Refs {
		safeRefs[i] = panels.StripANSI(r)
	}
	subjectText := safeSubject
	if len(safeRefs) > 0 {
		subjectText += " (" + strings.Join(safeRefs, ", ") + ")"
	}
	// Truncate or pad subject to fill its allotted space.
	subjectVisW := runewidth.StringWidth(subjectText)
	var styledSubject string
	if subjectVisW > subjectSpace {
		subjectText = runewidth.Truncate(subjectText, subjectSpace, "")
		// Check if refs portion was included in the truncated text.
		if len(safeRefs) > 0 && strings.Contains(subjectText, "(") {
			styledSubject = p.styleSubjectWithRefs(subjectText, safeSubject, subjectStyle, refStyle)
		} else {
			styledSubject = subjectStyle.Render(subjectText)
		}
	} else {
		if len(safeRefs) > 0 {
			styledSubject = subjectStyle.Render(safeSubject) + " " + refStyle.Render("("+strings.Join(safeRefs, ", ")+")")
			// Pad to fill subject space.
			styledVisW := lipgloss.Width(styledSubject)
			if styledVisW < subjectSpace {
				styledSubject += strings.Repeat(" ", subjectSpace-styledVisW)
			}
		} else {
			styledSubject = subjectStyle.Render(subjectText)
			if subjectVisW < subjectSpace {
				styledSubject += strings.Repeat(" ", subjectSpace-subjectVisW)
			}
		}
	}
	// Assemble the line: graph + subject + gap + right-side columns.
	var line string
	if graphW > 0 {
		line = graphStyle.Render(graphPrefix) + gap + styledSubject
	} else {
		line = styledSubject
	}
	line += gap + rightSide
	if isCursor {
		line = p.clStyles.cursor.Width(width).Render(line)
	}
	return truncateOrPad(line, width)
}

// styleSubjectWithRefs handles the case where subject text includes a truncated refs portion.
func (p *Panel) styleSubjectWithRefs(text string, _ string, subjectStyle, refStyle lipgloss.Style) string {
	idx := strings.Index(text, "(")
	if idx < 0 {
		return subjectStyle.Render(text)
	}
	return subjectStyle.Render(text[:idx]) + refStyle.Render(text[idx:])
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
		lines = append(lines, truncateOrPad(p.detailLines[i], width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the commit at the clicked display row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	dl := p.activeDisplay()
	displayIdx := p.offset + msg.ContentRow
	if displayIdx < 0 || displayIdx >= len(dl) {
		return p, nil
	}
	commitIdx := dl[displayIdx].commitIdx
	if commitIdx < 0 {
		return p, nil // connector line — not selectable
	}
	p.cursor = commitIdx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick shows the commit detail for the double-clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	dl := p.activeDisplay()
	displayIdx := p.offset + msg.ContentRow
	if displayIdx < 0 || displayIdx >= len(dl) {
		return p, nil
	}
	commitIdx := dl[displayIdx].commitIdx
	if commitIdx < 0 {
		return p, nil
	}
	p.cursor = commitIdx
	p.ensureCursorVisible()
	itemType := actions.ItemLogCommit
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pendingOp = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseRightClick opens a context menu for the right-clicked commit row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	dl := p.activeDisplay()
	displayIdx := p.offset + msg.ContentRow
	if displayIdx < 0 || displayIdx >= len(dl) {
		return p, nil
	}
	commitIdx := dl[displayIdx].commitIdx
	if commitIdx < 0 {
		return p, nil
	}
	p.cursor = commitIdx
	p.ensureCursorVisible()
	if p.cursor >= len(p.commits) {
		return p, nil
	}
	c := p.commits[p.cursor]
	if p.filteredIdx != nil && p.cursor < len(p.filteredIdx) {
		if fi := p.filteredIdx[p.cursor]; fi < len(p.commits) {
			c = p.commits[fi]
		}
	}
	label := panels.StripANSI(c.ShortHash) + " " + panels.StripANSI(c.Subject)
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemLogCommit, label)
	if cmd != nil {
		p.pendingOp = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// handleModalResult dispatches the result of a modal dialog.
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

// executeRightClickAction runs the given action on the currently selected commit.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionShowDetail:
		p.showDetail()
		return p, nil
	case actions.ActionCopyHash:
		return p.copyHash()
	}
	return p, nil
}

// handleMouseWheel scrolls the git log viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	dl := p.activeDisplay()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(dl) - p.height
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
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.focused {
		return p, nil
	}
	// Search mode input handling.
	if p.searchMode {
		return p.handleSearchKey(msg)
	}
	// Detail mode key handling.
	if p.detailMode {
		return p.handleDetailKey(msg)
	}
	switch msg.String() {
	case "j", "down":
		return p.moveCursorDown()
	case "k", "up":
		p.moveCursorUp()
	case keyEnter:
		p.showDetail()
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
	case "/":
		p.searchMode = true
		p.searchQuery = ""
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
	}
	return p, nil
}

func (p *Panel) handleSearchKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		p.searchMode = false
		// Keep filter active.
	case "escape", "esc":
		p.searchMode = false
		p.searchQuery = ""
		p.filteredIdx = nil
		p.filteredDL = nil
		p.filteredCmtY = nil
		p.cursor = 0
		p.offset = 0
	case "backspace":
		if len(p.searchQuery) > 0 {
			p.searchQuery = p.searchQuery[:len(p.searchQuery)-1]
			p.applySearch()
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= ' ' {
			p.searchQuery += ch
			p.applySearch()
		}
	}
	return p, nil
}

func (p *Panel) handleDetailKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "escape", "esc", keyEnter, "q":
		p.detailMode = false
		p.detailLines = nil
		p.detailOffset = 0
	case "j", "down":
		if n := len(p.detailLines); n > 0 && p.detailOffset < n-1 {
			p.detailOffset++
		}
	case "k", "up":
		if p.detailOffset > 0 {
			p.detailOffset--
		}
	case "pgdown":
		if n := len(p.detailLines); n > 0 {
			p.detailOffset += p.height / 2
			if p.detailOffset > n-1 {
				p.detailOffset = n - 1
			}
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
	if p.filteredIdx != nil {
		return max(0, len(p.filteredIdx)-1)
	}
	return max(0, len(p.commits)-1)
}

func (p *Panel) moveCursorDown() (panels.Panel, tea.Cmd) {
	if p.cursor < p.maxCursor() {
		p.cursor++
		p.ensureCursorVisible()
	}
	// Trigger pagination when near the bottom.
	if !p.allLoaded && p.cursor >= len(p.commits)-loadMoreThreshold {
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
	if p.height <= 0 {
		return
	}
	cmtY := p.activeCommitY()
	if p.cursor < 0 || p.cursor >= len(cmtY) {
		return
	}
	cursorDL := cmtY[p.cursor]
	if cursorDL < p.offset {
		p.offset = cursorDL
	}
	if cursorDL >= p.offset+p.height {
		p.offset = cursorDL - p.height + 1
	}
}

// ---------------------------------------------------------------------------
// Detail view
// ---------------------------------------------------------------------------
func (p *Panel) showDetail() {
	if p.cursor < 0 || p.cursor >= len(p.commits) {
		return
	}
	c := p.commits[p.cursor]
	if p.filteredIdx != nil && p.cursor < len(p.filteredIdx) {
		if fi := p.filteredIdx[p.cursor]; fi < len(p.commits) {
			c = p.commits[fi]
		}
	}
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
}

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------
func (p *Panel) copyHash() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.commits) {
		return p, nil
	}
	c := p.commits[p.cursor]
	if p.filteredIdx != nil && p.cursor < len(p.filteredIdx) {
		if fi := p.filteredIdx[p.cursor]; fi < len(p.commits) {
			c = p.commits[fi]
		}
	}
	hash := panels.StripANSI(c.ShortHash)
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Copied: " + hash,
			Level:   notify.Info,
		}
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------
func (p *Panel) applySearch() {
	if p.searchQuery == "" {
		p.filteredIdx = nil
		p.filteredDL = nil
		p.filteredCmtY = nil
		p.cursor = 0
		p.offset = 0
		return
	}
	query := strings.ToLower(p.searchQuery)
	p.filteredIdx = p.filteredIdx[:0]
	for i, c := range p.commits {
		if containsFold(c.Subject, query) ||
			containsFold(c.Author, query) ||
			containsFold(c.ShortHash, query) {
			p.filteredIdx = append(p.filteredIdx, i)
		}
	}
	// Build filtered display lines (flat list, no graph in filter mode).
	p.filteredDL = p.filteredDL[:0]
	p.filteredCmtY = p.filteredCmtY[:0]
	for fIdx := range p.filteredIdx {
		p.filteredCmtY = append(p.filteredCmtY, len(p.filteredDL))
		p.filteredDL = append(p.filteredDL, displayLine{
			commitIdx: p.filteredIdx[fIdx],
			text:      "*",
		})
	}
	// Reset cursor.
	p.cursor = 0
	p.offset = 0
}

// activeDisplay returns the current display lines (filtered or full).
func (p *Panel) activeDisplay() []displayLine {
	if p.filteredIdx != nil {
		return p.filteredDL
	}
	return p.display
}

// activeCommitY returns the commit-to-display mapping (filtered or full).
func (p *Panel) activeCommitY() []int {
	if p.filteredIdx != nil {
		return p.filteredCmtY
	}
	return p.commitY
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
// truncateOrPad ensures a rendered string fits exactly the given width.
func truncateOrPad(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		// Truncate: take first width chars accounting for ANSI codes.
		return lipgloss.NewStyle().MaxWidth(width).Render(s)
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
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
