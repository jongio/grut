// Package context implements the context builder panel for grut.
// It displays selected files with token counts and supports export
// for AI chat workflows.
package context

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	ctxbuilder "github.com/jongio/grut/internal/context"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
)

type panelColors struct {
	Path     string
	Tokens   string
	Header   string
	CursorBg string
	Dim      string
	Accent   string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Path:     "#7A9EBF",
		Tokens:   "#C9875A",
		Header:   "#D4D4D4",
		CursorBg: "#2A2A2A",
		Dim:      "#555555",
		Accent:   "#6B9E56",
	}
	if th != nil {
		c.Path = th.Colors.BrightBlue
		c.Tokens = th.Colors.NormalMagenta
		c.Header = th.Colors.Foreground
		c.CursorBg = th.Colors.SelectionBg
		c.Dim = th.Colors.BrightBlack
		c.Accent = th.Colors.NormalGreen
	}
	return c
}

// Pending operation identifiers for modal result dispatch.
const (
	opRightClickPick  = "right_click_pick"
	opFirstUseConfirm = "first_use_confirm"
)

// Panel is the context builder panel. It implements [panels.Panel].
type Panel struct {
	actionsCfg  config.ActionsConfig
	builder     *ctxbuilder.Builder
	pendingOp   string
	pendingName string
	panels.BasePanel
	colors panelColors
	theme  *theme.Theme
	cursor int
	offset int
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new context builder panel backed by the given builder.
func New(builder *ctxbuilder.Builder, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "context"},
		builder:   builder,
		colors:    initColors(th),
		theme:     th,
	}
}

// Init implements panels.Panel.
func (p *Panel) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.AddToContextMsg:
		return p.addFile(msg.Path)
	case panels.RemoveFromContextMsg:
		return p.removeFile(msg.Path)
	case panels.ClearContextMsg:
		return p.clearAll()
	case panels.ExportContextMsg:
		return p.export()
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
	files := p.builder.Files()
	// Reserve 1 line for the status bar at the bottom.
	listHeight := height - 1
	if listHeight < 0 {
		listHeight = 0
	}
	if len(files) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Width(width).Height(listHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.colors.Dim)).
			Render("No files in context\n\nPress C in filetree to add files")
		statusBar := p.renderStatusBar(width, 0, 0)
		return emptyMsg + "\n" + statusBar
	}
	// Render file list.
	lines := make([]string, 0, listHeight)
	end := p.offset + listHeight
	if end > len(files) {
		end = len(files)
	}
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.renderLine(files[i], width, i == p.cursor))
	}
	// Pad remaining height.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < listHeight {
		lines = append(lines, emptyLine)
	}
	statusBar := p.renderStatusBar(width, len(files), p.builder.TotalTokens())
	lines = append(lines, statusBar)
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "d/del", Description: "Remove file from context", Action: "remove"},
		{Key: "c", Description: "Clear all files", Action: "clear"},
		{Key: "e", Description: "Export context", Action: "export"},
		{Key: "enter", Description: "Preview file", Action: "preview"},
	}
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
	case "G":
		p.goToBottom()
	case "g":
		p.goToTop()
	case "d", "delete":
		return p.removeCurrent()
	case "c":
		return p.clearAll()
	case "e":
		return p.export()
	case "enter":
		return p.previewCurrent()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (p *Panel) moveCursorDown() {
	files := p.builder.Files()
	if p.cursor < len(files)-1 {
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
	files := p.builder.Files()
	if n := len(files); n > 0 {
		p.cursor = n - 1
		p.ensureCursorVisible()
	}
}

func (p *Panel) ensureCursorVisible() {
	viewHeight := p.Height - 1 // subtract status bar
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, viewHeight)
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the context file at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(files) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick previews the file at the double-clicked row.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(files) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	itemType := actions.ItemContextFile
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.clearPending()
		p.pendingOp = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseWheel scrolls the context file list viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	viewHeight := p.Height - 1 // subtract status bar
	if viewHeight <= 0 {
		viewHeight = 1
	}
	files := p.builder.Files()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(files) - viewHeight
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

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// handleMouseRightClick shows a context menu for the file at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(files) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	label := files[idx].Path
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemContextFile, label)
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
	case actions.ActionPreview:
		return p.previewCurrent()
	case actions.ActionRemove:
		return p.removeCurrent()
	case actions.ActionCopyPath:
		return p.copyPath()
	}
	return p, nil
}

// copyPath copies the file path at the cursor to the OS clipboard.
func (p *Panel) copyPath() (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	if p.cursor < 0 || p.cursor >= len(files) {
		return p, nil
	}
	path := files[p.cursor].Path
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

// ---------------------------------------------------------------------------
func (p *Panel) addFile(path string) (panels.Panel, tea.Cmd) {
	if err := p.builder.Add(path); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Context: " + errMsg, Level: notify.Error}
		}
	}
	files := p.builder.Files()
	totalTokens := p.builder.TotalTokens()
	fileCount := len(files)
	return p, func() tea.Msg {
		return panels.ContextUpdatedMsg{FileCount: fileCount, TokenCount: totalTokens}
	}
}

func (p *Panel) removeFile(path string) (panels.Panel, tea.Cmd) {
	p.builder.Remove(path)
	p.clampCursor()
	files := p.builder.Files()
	totalTokens := p.builder.TotalTokens()
	fileCount := len(files)
	return p, func() tea.Msg {
		return panels.ContextUpdatedMsg{FileCount: fileCount, TokenCount: totalTokens}
	}
}

func (p *Panel) removeCurrent() (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	if p.cursor < 0 || p.cursor >= len(files) {
		return p, nil
	}
	path := files[p.cursor].Path
	p.builder.Remove(path)
	p.clampCursor()
	files = p.builder.Files()
	totalTokens := p.builder.TotalTokens()
	fileCount := len(files)
	return p, func() tea.Msg {
		return panels.ContextUpdatedMsg{FileCount: fileCount, TokenCount: totalTokens}
	}
}

func (p *Panel) clearAll() (panels.Panel, tea.Cmd) {
	p.builder.Clear()
	p.cursor = 0
	p.offset = 0
	return p, func() tea.Msg {
		return panels.ContextUpdatedMsg{FileCount: 0, TokenCount: 0}
	}
}

func (p *Panel) export() (panels.Panel, tea.Cmd) {
	exported := p.builder.Export()
	if exported == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Nothing to export", Level: notify.Warn}
		}
	}
	// Print to stdout as fallback (clipboard integration can be added later).
	fmt.Print(exported)
	files := p.builder.Files()
	tokenCount := p.builder.TotalTokens()
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: fmt.Sprintf("Exported %d files (%d tokens)", len(files), tokenCount),
			Level:   notify.Success,
		}
	}
}

func (p *Panel) previewCurrent() (panels.Panel, tea.Cmd) {
	files := p.builder.Files()
	if p.cursor < 0 || p.cursor >= len(files) {
		return p, nil
	}
	path := files[p.cursor].Path
	return p, func() tea.Msg {
		return panels.FileSelectedMsg{Path: path}
	}
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------
func (p *Panel) clampCursor() {
	files := p.builder.Files()
	p.cursor = panels.ClampCursor(p.cursor, len(files))
}

func (p *Panel) renderLine(f ctxbuilder.ContextFile, width int, isCursor bool) string {
	var b strings.Builder
	b.WriteString("  ")
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Path))
	b.WriteString(pathStyle.Render(f.Path))
	b.WriteString("  ")
	tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Tokens))
	b.WriteString(tokenStyle.Render(fmt.Sprintf("%d tokens", f.Tokens)))
	content := b.String()
	style := lipgloss.NewStyle().
		Width(width).
		MaxWidth(width)
	if isCursor && p.Focused {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(content)
}

func (p *Panel) renderStatusBar(width, fileCount, tokenCount int) string {
	var text string
	if fileCount == 0 {
		text = " 0 files │ 0 tokens"
	} else {
		text = fmt.Sprintf(" %d files │ %d tokens", fileCount, tokenCount)
	}
	style := lipgloss.NewStyle().
		Width(width).
		MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.Accent)).
		Bold(true)
	return style.Render(text)
}

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package)
// ---------------------------------------------------------------------------
func (p *Panel) cursorIndex() int { return p.cursor }
func (p *Panel) fileCount() int   { return len(p.builder.Files()) }
