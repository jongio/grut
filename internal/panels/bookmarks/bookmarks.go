// Package bookmarks implements the bookmarks overlay panel for grut.
// It displays a navigable list of saved directory bookmarks and allows
// the user to jump to a bookmark or delete entries.
package bookmarks

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
)

type panelColors struct {
	Title    string
	Path     string
	Name     string
	CursorBg string
	Dim      string
	Border   string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Title:    "#C9875A",
		Path:     "#7A9EBF",
		Name:     "#D4D4D4",
		CursorBg: "#2A2A2A",
		Dim:      "#555555",
		Border:   "#C9A227",
	}
	if th != nil {
		c.Title = th.Colors.NormalMagenta
		c.Path = th.Colors.BrightBlue
		c.Name = th.Colors.Foreground
		c.CursorBg = th.Colors.SelectionBg
		c.Dim = th.Colors.BrightBlack
		c.Border = th.Colors.BorderFocused
	}
	return c
}

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
)

// Panel is the bookmarks overlay. It implements [panels.Panel].
type Panel struct {
	actionsCfg  config.ActionsConfig
	manager     *bm.Manager
	pendingOp   string
	pendingName string
	items       []bm.Bookmark
	panels.BasePanel
	colors panelColors
	theme  *theme.Theme
	cursor int
	offset int
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new bookmarks overlay panel backed by the given manager.
func New(manager *bm.Manager, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "bookmarks"},
		manager:   manager,
		colors:    initColors(th),
		theme:     th,
	}
}

// Init implements panels.Panel.
func (p *Panel) Init(_ context.Context) tea.Cmd {
	p.refresh()
	return nil
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
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
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if len(p.items) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("No bookmarks\n\nPress b in filetree to add one")
	}
	lines := make([]string, 0, height)
	end := p.offset + height
	if end > len(p.items) {
		end = len(p.items)
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderLine(p.items[i], width, i == p.cursor))
	}
	// Pad remaining height.
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
		{Key: "enter", Description: "Jump to bookmark", Action: "select"},
		{Key: "d", Description: "Delete bookmark", Action: "delete"},
		{Key: "escape", Description: "Close bookmarks", Action: "close"},
	}
}

// Refresh reloads the bookmark list from the manager.
func (p *Panel) Refresh() {
	p.refresh()
}

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

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
	case "G":
		p.goToBottom()
	case "g":
		p.goToTop()
	case "enter":
		return p.selectBookmark()
	case "d":
		return p.deleteBookmark()
	case "escape", "esc":
		return p, func() tea.Msg { return panels.ToggleBookmarksMsg{} }
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

func (p *Panel) goToTop() {
	p.cursor = 0
	p.ensureCursorVisible()
}

func (p *Panel) goToBottom() {
	if n := len(p.items); n > 0 {
		p.cursor = n - 1
		p.ensureCursorVisible()
	}
}

func (p *Panel) ensureCursorVisible() {
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, p.Height)
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the bookmark at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick jumps to the bookmark at the double-clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	itemType := actions.ItemBookmark
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pendingOp = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseRightClick shows a context menu for the bookmark at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.items) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	label := p.items[p.cursor].Path
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemBookmark, label)
	if cmd != nil {
		p.pendingOp = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// handleModalResult processes the result from the action picker modal.
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

// executeRightClickAction dispatches a right-click action to the appropriate method.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionJump:
		return p.selectBookmark()
	case actions.ActionDelete:
		return p.deleteBookmark()
	case actions.ActionCopyPath:
		return p.copyPath()
	}
	return p, nil
}

// copyPath copies the current bookmark's path to the clipboard.
func (p *Panel) copyPath() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return p, nil
	}
	path := p.items[p.cursor].Path
	if err := panels.CopyToClipboard(context.Background(), path); err != nil {
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

// handleMouseWheel scrolls the bookmarks viewport.
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
// Actions
// ---------------------------------------------------------------------------
func (p *Panel) selectBookmark() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return p, nil
	}
	path := p.items[p.cursor].Path
	return p, func() tea.Msg { return panels.NavigateToPathMsg{Path: path} }
}

func (p *Panel) deleteBookmark() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return p, nil
	}
	bk := p.items[p.cursor]
	if err := p.manager.Remove(bk.Path); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: errMsg, Level: notify.Error}
		}
	}
	// Persist and refresh.
	if err := p.manager.Save(); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "bookmark removed but save failed: " + errMsg, Level: notify.Warn}
		}
	}
	p.refresh()
	name := bk.Name
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "Removed bookmark: " + name,
			Level:   notify.Success,
		}
	}
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------
func (p *Panel) refresh() {
	p.items = p.manager.List()
	if p.cursor >= len(p.items) {
		p.cursor = len(p.items) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *Panel) renderLine(bk bm.Bookmark, width int, isCursor bool) string {
	var b strings.Builder
	// "  name  path" format.
	b.WriteString("  ")
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Name)).Bold(true)
	b.WriteString(nameStyle.Render(bk.Name))
	b.WriteString("  ")
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Path))
	b.WriteString(pathStyle.Render(bk.Path))
	content := b.String()
	style := lipgloss.NewStyle().
		Width(width).
		MaxWidth(width)
	if isCursor && p.Focused {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(content)
}

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package)
// ---------------------------------------------------------------------------
func (p *Panel) cursorIndex() int { return p.cursor }
func (p *Panel) itemCount() int   { return len(p.items) }
