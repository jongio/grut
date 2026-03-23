// Package stash implements the stash management and cherry-pick panel for grut.
// It provides a navigable list of stash entries with push, pop, apply, and drop
// operations, plus cherry-pick handling for commits received from other panels.
package stash

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
)

// opKind identifies the type of pending stash operation awaiting modal
// confirmation or input.
type opKind int

const (
	opDrop            opKind = iota // drop single stash entry (confirm)
	opDropAll                       // drop all stash entries (confirm)
	opPush                          // push new stash (input for message)
	opRightClickPick                // right-click action picker
	opFirstUseConfirm               // first-use double-click confirmation
)

// pendingOp holds state for an operation awaiting a modal result.
type pendingOp struct {
	name  string // item type name (for opFirstUseConfirm)
	kind  opKind
	index int // stash index (for opDrop)
}

// gitOps defines the git operations needed by the stash panel.
// Using a narrow interface enables straightforward testing with mocks.
type gitOps interface {
	StashList(ctx context.Context) ([]git.StashEntry, error)
	StashPush(ctx context.Context, opts git.StashOpts) error
	StashPop(ctx context.Context, index int) error
	StashApply(ctx context.Context, index int) error
	StashDrop(ctx context.Context, index int) error
	StashShow(ctx context.Context, index int) (string, error)
	CherryPick(ctx context.Context, commitHash string) error
}

// stashLoadedMsg carries the result of an asynchronous stash list load.
type stashLoadedMsg struct {
	err     error
	entries []git.StashEntry
}

// stashOpDoneMsg is sent after a stash mutation completes.
type stashOpDoneMsg struct {
	err    error
	action string // human-readable action description
}

// stashShowMsg carries the result of an asynchronous stash show.
type stashShowMsg struct {
	err  error
	diff string
}

// Panel is the stash management panel. It lists stash entries and provides
// push/pop/apply/drop operations plus cherry-pick handling.
type Panel struct {
	actionsCfg     config.ActionsConfig
	git            gitOps
	ctx            context.Context
	pending        *pendingOp
	previewContent string
	entries        []git.StashEntry
	panels.BasePanel
	cursor         int
	offset         int
	previewOffset  int
	showingPreview bool
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new stash panel with the given git client.
func New(gc gitOps) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "stash"},
		git:       gc,
	}
}

// SetActionsCfg stores the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// Init implements panels.Panel. It stores the application context and
// triggers an initial asynchronous load of stash entries.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return p.loadStash()
}

// handleRepoChanged replaces the git client and reloads stash entries for
// the new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
		p.entries = nil
		p.cursor = 0
		p.offset = 0
		p.showingPreview = false
		p.previewContent = ""
		p.previewOffset = 0
		return p, nil
	}
	p.git = client
	p.entries = nil
	p.cursor = 0
	p.offset = 0
	p.showingPreview = false
	p.previewContent = ""
	p.previewOffset = 0
	return p, p.loadStash()
}

// loadStash returns a command that fetches stash entries asynchronously.
func (p *Panel) loadStash() tea.Cmd {
	gc := p.git
	ctx := p.safeCtx()
	return func() tea.Msg {
		entries, err := gc.StashList(ctx)
		return stashLoadedMsg{entries: entries, err: err}
	}
}

// safeCtx returns the panel's context, falling back to context.Background()
// if Init has not been called (e.g. during tests).
func (p *Panel) safeCtx() context.Context {
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case stashLoadedMsg:
		if msg.err != nil {
			errMsg := msg.err.Error()
			return p, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: "Stash list: " + errMsg,
					Level:   notify.Error,
				}
			}
		}
		p.entries = msg.entries
		p.clampCursor()
		return p, nil
	case stashOpDoneMsg:
		if msg.err != nil {
			errMsg := msg.err.Error()
			return p, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: errMsg,
					Level:   notify.Error,
				}
			}
		}
		action := msg.action
		return p, tea.Batch(
			func() tea.Msg {
				return notify.ShowToastMsg{Message: action, Level: notify.Success}
			},
			func() tea.Msg { return panels.StashChangedMsg{} },
			p.loadStash(),
		)
	case stashShowMsg:
		if msg.err != nil {
			errMsg := msg.err.Error()
			return p, func() tea.Msg {
				return notify.ShowToastMsg{
					Message: "Stash show: " + errMsg,
					Level:   notify.Error,
				}
			}
		}
		p.showingPreview = true
		p.previewContent = msg.diff
		p.previewOffset = 0
		return p, nil
	case panels.CherryPickMsg:
		return p.executeCherryPick(msg.Hash)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleMouseDoubleClick(msg)
	case panels.PanelMouseRightClickMsg:
		return p.handleMouseRightClick(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		if p.Focused {
			return p.handleKey(msg)
		}
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	// CRUD actions dispatched via keymap.
	case panels.ItemCreateMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestPush()
	case panels.ItemDeleteMsg:
		if !p.Focused {
			return p, nil
		}
		return p.requestDrop()
	case panels.ItemCopyMsg:
		if !p.Focused {
			return p, nil
		}
		return p.copyStashRef()
	}
	return p, nil
}

// Title returns the panel title with stash entry count.
func (p *Panel) Title() string {
	return fmt.Sprintf("stash (%d)", len(p.entries))
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.showingPreview {
		return p.renderPreview(width, height)
	}
	if len(p.entries) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No stash entries")
	}
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > len(p.entries) {
		end = len(p.entries)
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderEntry(p.entries[i], width, i == p.cursor))
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
		{Key: "v", Description: "Preview stash diff", Action: "preview"},
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "enter/a", Description: "Apply stash entry", Action: "apply"},
		{Key: "p", Description: "Pop stash entry", Action: "pop"},
		{Key: "n/s", Description: "Push new stash", Action: "item_create"},
		{Key: "d/x", Description: "Drop stash entry", Action: "item_delete"},
		{Key: "y", Description: "Copy stash reference", Action: "item_copy"},
		{Key: "D", Description: "Drop all stash entries", Action: "drop_all"},
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
// renderEntry renders a single stash entry line.
func (p *Panel) renderEntry(e git.StashEntry, width int, isCursor bool) string {
	label := fmt.Sprintf("stash@{%d}: %s", e.Index, e.Message)
	var datePart string
	if !e.Date.IsZero() {
		datePart = timeAgo(e.Date)
	}
	// Calculate available width for the label.
	maxLabel := width
	if datePart != "" {
		maxLabel = width - len(datePart) - 2 // spacing
	}
	if maxLabel < 0 {
		maxLabel = 0
	}
	// Truncate label if needed.
	if len(label) > maxLabel {
		if maxLabel > 3 {
			label = label[:maxLabel-3] + "..."
		} else if maxLabel > 0 {
			label = label[:maxLabel]
		} else {
			label = ""
		}
	}
	// Build the full line with right-aligned date.
	var line string
	if datePart != "" && len(label)+len(datePart)+1 <= width {
		padding := width - len(label) - len(datePart)
		if padding < 1 {
			padding = 1
		}
		line = label + strings.Repeat(" ", padding) + datePart
	} else {
		line = label
	}
	style := lipgloss.NewStyle().Width(width)
	if isCursor {
		style = style.Background(lipgloss.Color("#44475A")).Bold(true)
	}
	return style.Render(line)
}

// renderPreview renders the stash diff content.
func (p *Panel) renderPreview(width, height int) string {
	lines := strings.Split(p.previewContent, "\n")
	if p.previewOffset >= len(lines) {
		p.previewOffset = len(lines) - 1
	}
	if p.previewOffset < 0 {
		p.previewOffset = 0
	}
	end := p.previewOffset + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := make([]string, 0, height)
	for i := p.previewOffset; i < end; i++ {
		line := lines[i]
		// Truncate long lines
		if len(line) > width {
			line = line[:width]
		}
		// Color diff lines
		style := lipgloss.NewStyle().Width(width)
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			style = style.Foreground(lipgloss.Color("#50FA7B"))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			style = style.Foreground(lipgloss.Color("#FF5555"))
		case strings.HasPrefix(line, "@@"):
			style = style.Foreground(lipgloss.Color("#8BE9FD"))
		case strings.HasPrefix(line, "diff "):
			style = style.Foreground(lipgloss.Color("#BD93F9")).Bold(true)
		}
		visible = append(visible, style.Render(line))
	}
	// Pad remaining height
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(visible) < height {
		visible = append(visible, emptyLine)
	}
	return strings.Join(visible, "\n")
}

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
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
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
// handleKey processes key presses when the panel is focused.
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "q":
		if p.showingPreview {
			p.showingPreview = false
			p.previewContent = ""
			p.previewOffset = 0
			return p, nil
		}
	case "v":
		if p.showingPreview {
			p.showingPreview = false
			p.previewContent = ""
			p.previewOffset = 0
			return p, nil
		}
		return p.executeShow()
	case "j", "down":
		if p.showingPreview {
			p.previewOffset++
			return p, nil
		}
		p.moveCursorDown()
	case "k", "up":
		if p.showingPreview {
			if p.previewOffset > 0 {
				p.previewOffset--
			}
			return p, nil
		}
		p.moveCursorUp()
	case "enter", "a":
		return p.executeApply()
	case "p":
		return p.executePop()
	case "d", "x":
		return p.requestDrop()
	case "n", "s":
		return p.requestPush()
	case "y":
		return p.copyStashRef()
	case "D":
		return p.requestDropAll()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the stash entry at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	if p.showingPreview {
		return p, nil
	}
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.entries) {
		return p, nil
	}
	p.cursor = idx
	p.ensureVisible()
	return p, nil
}

// handleMouseDoubleClick applies the stash entry at the double-clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	if p.showingPreview {
		return p, nil
	}
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.entries) {
		return p, nil
	}
	p.cursor = idx
	p.ensureVisible()
	itemType := actions.ItemStash
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pending = &pendingOp{kind: opFirstUseConfirm, name: string(itemType)}
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseRightClick opens the context menu for the stash entry at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	if p.showingPreview {
		return p, nil
	}
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.entries) {
		return p, nil
	}
	p.cursor = idx
	p.ensureVisible()
	label := fmt.Sprintf("stash@{%d}", p.entries[idx].Index)
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemStash, label)
	if cmd != nil {
		p.pending = &pendingOp{kind: opRightClickPick, index: p.entries[idx].Index}
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// handleMouseWheel scrolls the stash list viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	if p.showingPreview {
		return p, nil
	}
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.entries) - p.Height
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
// Cursor movement
// ---------------------------------------------------------------------------
func (p *Panel) moveCursorDown() {
	if p.cursor < len(p.entries)-1 {
		p.cursor++
		p.ensureVisible()
	}
}

func (p *Panel) moveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureVisible()
	}
}

func (p *Panel) clampCursor() {
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *Panel) ensureVisible() {
	height := p.Height
	if height <= 0 {
		height = 20
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
}

// selectedEntry returns the stash entry under the cursor, or nil.
func (p *Panel) selectedEntry() *git.StashEntry {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return nil
	}
	return &p.entries[p.cursor]
}

// ---------------------------------------------------------------------------
// Immediate operations (no modal confirmation needed)
// ---------------------------------------------------------------------------
// executeApply applies the selected stash entry without removing it.
func (p *Panel) executeApply() (panels.Panel, tea.Cmd) {
	e := p.selectedEntry()
	if e == nil {
		return p, nil
	}
	gc := p.git
	ctx := p.safeCtx()
	index := e.Index
	return p, func() tea.Msg {
		if err := gc.StashApply(ctx, index); err != nil {
			return stashOpDoneMsg{err: err}
		}
		return stashOpDoneMsg{action: fmt.Sprintf("Applied stash@{%d}", index)}
	}
}

// executePop applies and removes the selected stash entry.
func (p *Panel) executePop() (panels.Panel, tea.Cmd) {
	e := p.selectedEntry()
	if e == nil {
		return p, nil
	}
	gc := p.git
	ctx := p.safeCtx()
	index := e.Index
	return p, func() tea.Msg {
		if err := gc.StashPop(ctx, index); err != nil {
			return stashOpDoneMsg{err: err}
		}
		return stashOpDoneMsg{action: fmt.Sprintf("Popped stash@{%d}", index)}
	}
}

// executeCherryPick cherry-picks a commit by hash.
func (p *Panel) executeCherryPick(hash string) (panels.Panel, tea.Cmd) {
	gc := p.git
	ctx := p.safeCtx()
	return p, func() tea.Msg {
		if err := gc.CherryPick(ctx, hash); err != nil {
			return notify.ShowToastMsg{
				Message: "Cherry-pick failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		short := hash
		if len(hash) > git.ShortHashLen {
			short = hash[:git.ShortHashLen]
		}
		return notify.ShowToastMsg{
			Message: "Cherry-picked " + short,
			Level:   notify.Success,
		}
	}
}

// ---------------------------------------------------------------------------
// Modal-gated operations
// ---------------------------------------------------------------------------
// requestDrop shows a confirmation modal before dropping a stash entry.
func (p *Panel) requestDrop() (panels.Panel, tea.Cmd) {
	e := p.selectedEntry()
	if e == nil {
		return p, nil
	}
	p.pending = &pendingOp{kind: opDrop, index: e.Index}
	idx := e.Index
	return p, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:    notify.ModalConfirm,
			Title:   "Drop stash",
			Message: fmt.Sprintf("Drop stash@{%d}?", idx),
		}
	}
}

// requestDropAll shows a confirmation modal before dropping all stash entries.
func (p *Panel) requestDropAll() (panels.Panel, tea.Cmd) {
	if len(p.entries) == 0 {
		return p, nil
	}
	p.pending = &pendingOp{kind: opDropAll}
	count := len(p.entries)
	return p, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:    notify.ModalConfirm,
			Title:   "Drop all stashes",
			Message: fmt.Sprintf("Drop all %d stash entries?", count),
		}
	}
}

// executeShow fetches the diff of the currently selected stash entry.
func (p *Panel) executeShow() (panels.Panel, tea.Cmd) {
	if len(p.entries) == 0 || p.cursor >= len(p.entries) {
		return p, nil
	}
	entry := p.entries[p.cursor]
	gc := p.git
	ctx := p.safeCtx()
	idx := entry.Index
	return p, func() tea.Msg {
		diff, err := gc.StashShow(ctx, idx)
		return stashShowMsg{diff: diff, err: err}
	}
}

// requestPush shows an input modal to collect a stash message before pushing.
func (p *Panel) requestPush() (panels.Panel, tea.Cmd) {
	p.pending = &pendingOp{kind: opPush}
	return p, func() tea.Msg {
		return notify.ShowModalMsg{
			Kind:        notify.ModalInput,
			Title:       "Stash push",
			Message:     "Enter stash message:",
			Placeholder: "WIP",
		}
	}
}

// copyStashRef copies the selected stash entry reference to the clipboard.
func (p *Panel) copyStashRef() (panels.Panel, tea.Cmd) {
	if len(p.entries) == 0 || p.cursor >= len(p.entries) {
		return p, nil
	}
	ref := fmt.Sprintf("stash@{%d}", p.entries[p.cursor].Index)
	if err := panels.CopyToClipboard(p.ctx, ref); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Copied: " + ref, Level: notify.Success}
	}
}

// handleModalResult processes the result from a confirmation or input modal.
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	p.pending = nil
	if op == nil || !msg.Accept {
		return p, nil
	}
	gc := p.git
	ctx := p.safeCtx()
	switch op.kind {
	case opDrop:
		index := op.index
		return p, func() tea.Msg {
			if err := gc.StashDrop(ctx, index); err != nil {
				return stashOpDoneMsg{err: err}
			}
			return stashOpDoneMsg{action: fmt.Sprintf("Dropped stash@{%d}", index)}
		}
	case opDropAll:
		entries := make([]git.StashEntry, len(p.entries))
		copy(entries, p.entries)
		return p, func() tea.Msg {
			// Drop in reverse order to avoid index shifting.
			for i := len(entries) - 1; i >= 0; i-- {
				if err := gc.StashDrop(ctx, entries[i].Index); err != nil {
					return stashOpDoneMsg{err: err}
				}
			}
			return stashOpDoneMsg{
				action: fmt.Sprintf("Dropped all %d stash entries", len(entries)),
			}
		}
	case opPush:
		message := msg.Value
		return p, func() tea.Msg {
			if err := gc.StashPush(ctx, git.StashOpts{Message: message}); err != nil {
				return stashOpDoneMsg{err: err}
			}
			return stashOpDoneMsg{action: "Changes stashed"}
		}
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, op.name, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	}
	return p, nil
}

// executeRightClickAction dispatches a right-click action by ID.
func (p *Panel) executeRightClickAction(id actions.ActionID) (panels.Panel, tea.Cmd) {
	switch id {
	case actions.ActionApply:
		return p.executeApply()
	case actions.ActionPop:
		return p.executePop()
	case actions.ActionDrop:
		return p.requestDrop()
	case actions.ActionShowDetail:
		return p.executeShow()
	default:
		return p, nil
	}
}
