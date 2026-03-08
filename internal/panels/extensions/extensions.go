// Package extensions implements the extension management panel for grut.
// It lists installed extensions with their status, runtime type, and
// provides controls to enable/disable, install, remove, and refresh.
package extensions

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/extension"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
)

// ---------------------------------------------------------------------------
// Internal message types (async result messages)
// ---------------------------------------------------------------------------

// extensionsLoadedMsg carries the result of an async extension list refresh.
type extensionsLoadedMsg struct {
	extensions []extension.ExtensionInfo
}

// extensionToggleResultMsg carries the result of an enable/disable operation.
type extensionToggleResultMsg struct {
	name string
	err  error
}

// extensionRemoveResultMsg carries the result of a remove operation.
type extensionRemoveResultMsg struct {
	name string
	err  error
}

// extensionInstallResultMsg carries the result of an install operation.
type extensionInstallResultMsg struct {
	source string
	err    error
}

// ---------------------------------------------------------------------------
// Narrow interface for testability
// ---------------------------------------------------------------------------

// extManager defines the subset of extension.Manager used by this panel.
// It is satisfied by *extension.Manager and makes the panel easy to mock
// in tests.
type extManager interface {
	List() []extension.ExtensionInfo
	Enable(name string) error
	Disable(name string) error
	Remove(name string) error
	Install(ctx context.Context, source string) error
}

// ---------------------------------------------------------------------------
// Pending operation tracking
// ---------------------------------------------------------------------------

type pendingOp int

const (
	opNone pendingOp = iota
	opRemove
	opInstall
	opRightClickPick
	opFirstUseConfirm
)

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------

// Panel is the extension management panel. It implements [panels.Panel].
type Panel struct {
	panels.BasePanel

	mgr extManager
	ctx context.Context

	extensions []extension.ExtensionInfo // latest snapshot, sorted by name
	cursor     int                       // index into extensions list
	offset     int                       // viewport scroll offset
	expanded   map[string]bool           // extension names with details expanded

	actionsCfg config.ActionsConfig // right-click action overrides

	pending     pendingOp // operation awaiting modal result
	pendingName string    // extension name for pending remove

	loading bool // true while a refresh is in flight
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new extension management panel with the given manager.
func New(mgr extManager) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "extensions"},
		mgr:       mgr,
		expanded:  make(map[string]bool),
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------

// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	p.loading = true
	return p.loadExtensionsCmd()
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case extensionsLoadedMsg:
		p.loading = false
		p.extensions = msg.extensions
		p.clampCursor()
		return p, nil

	case extensionToggleResultMsg:
		if msg.err != nil {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: fmt.Sprintf("Toggle failed: %v", msg.err), Level: notify.Error}
			}
		}
		return p, tea.Batch(p.loadExtensionsCmd(), func() tea.Msg {
			return panels.ExtensionChangedMsg{}
		})

	case extensionRemoveResultMsg:
		if msg.err != nil {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: fmt.Sprintf("Remove failed: %v", msg.err), Level: notify.Error}
			}
		}
		return p, tea.Batch(p.loadExtensionsCmd(), func() tea.Msg {
			return panels.ExtensionChangedMsg{}
		})

	case extensionInstallResultMsg:
		if msg.err != nil {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: fmt.Sprintf("Install failed: %v", msg.err), Level: notify.Error}
			}
		}
		return p, tea.Batch(p.loadExtensionsCmd(), func() tea.Msg {
			return panels.ExtensionChangedMsg{}
		})

	case notify.ModalResultMsg:
		return p.handleModalResult(msg)

	case panels.ExtensionChangedMsg:
		return p, p.loadExtensionsCmd()

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
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	if p.loading && len(p.extensions) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("Loading extensions...")
	}

	if len(p.extensions) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No extensions installed")
	}

	// Build visible rows.
	rows := p.buildVisibleRows(width)

	// Apply viewport offset.
	end := p.offset + height
	if end > len(rows) {
		end = len(rows)
	}
	start := p.offset
	if start > len(rows) {
		start = len(rows)
	}

	lines := make([]string, 0, height)
	for i := start; i < end; i++ {
		lines = append(lines, rows[i])
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
		{Key: "enter", Description: "Expand/collapse details", Action: "expand"},
		{Key: "e", Description: "Toggle enable/disable", Action: "toggle"},
		{Key: "d", Description: "Remove extension", Action: "remove"},
		{Key: "i", Description: "Install extension", Action: "install"},
		{Key: "R", Description: "Refresh list", Action: "refresh"},
	}
}

// ---------------------------------------------------------------------------
// Async commands
// ---------------------------------------------------------------------------

func (p *Panel) loadExtensionsCmd() tea.Cmd {
	mgr := p.mgr
	return func() tea.Msg {
		exts := mgr.List()
		slices.SortFunc(exts, func(a, b extension.ExtensionInfo) int {
			return strings.Compare(a.Manifest.Name, b.Manifest.Name)
		})
		return extensionsLoadedMsg{extensions: exts}
	}
}

func (p *Panel) toggleExtensionCmd(name string, enable bool) tea.Cmd {
	mgr := p.mgr
	return func() tea.Msg {
		var err error
		if enable {
			err = mgr.Enable(name)
		} else {
			err = mgr.Disable(name)
		}
		return extensionToggleResultMsg{name: name, err: err}
	}
}

func (p *Panel) removeExtensionCmd(name string) tea.Cmd {
	mgr := p.mgr
	return func() tea.Msg {
		err := mgr.Remove(name)
		return extensionRemoveResultMsg{name: name, err: err}
	}
}

func (p *Panel) installExtensionCmd(source string) tea.Cmd {
	mgr := p.mgr
	ctx := p.ctx
	return func() tea.Msg {
		err := mgr.Install(ctx, source)
		return extensionInstallResultMsg{source: source, err: err}
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}

	key := msg.String()
	switch key {
	case "j", "down":
		p.moveCursorDown()
	case "k", "up":
		p.moveCursorUp()
	case "enter":
		return p.toggleExpand()
	case "e":
		return p.toggleEnable()
	case "d":
		return p.requestRemove()
	case "i":
		return p.requestInstall()
	case "R":
		p.loading = true
		return p, p.loadExtensionsCmd()
	}

	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------

// handleMouseClick selects the extension at the clicked row.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.extensions) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseDoubleClick toggles expand/collapse on the double-clicked extension.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.extensions) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()

	itemType := actions.ItemExtension
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pending = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// handleMouseWheel scrolls the extension list viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.extensions) - p.Height
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
	if p.cursor < len(p.extensions)-1 {
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

func (p *Panel) clampCursor() {
	if p.cursor >= len(p.extensions) {
		p.cursor = len(p.extensions) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
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

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (p *Panel) toggleExpand() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.extensions) {
		return p, nil
	}

	name := p.extensions[p.cursor].Manifest.Name
	if p.expanded[name] {
		delete(p.expanded, name)
	} else {
		p.expanded[name] = true
	}
	return p, nil
}

func (p *Panel) toggleEnable() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.extensions) {
		return p, nil
	}

	ext := p.extensions[p.cursor]
	return p, p.toggleExtensionCmd(ext.Manifest.Name, !ext.Enabled)
}

func (p *Panel) requestRemove() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.extensions) {
		return p, nil
	}

	name := p.extensions[p.cursor].Manifest.Name
	p.pending = opRemove
	p.pendingName = name
	return p, notify.ShowConfirm("Remove Extension",
		fmt.Sprintf("Remove extension %q? This will delete its files.", name))
}

func (p *Panel) requestInstall() (panels.Panel, tea.Cmd) {
	p.pending = opInstall
	return p, notify.ShowInput("Install Extension", "https://github.com/user/extension")
}

func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	name := p.pendingName
	p.pending = opNone
	p.pendingName = ""

	if !msg.Accept {
		return p, nil
	}

	switch op { //nolint:exhaustive // only relevant cases handled
	case opRemove:
		return p, p.removeExtensionCmd(name)
	case opInstall:
		source := msg.Value
		if source == "" {
			return p, nil
		}
		return p, p.installExtensionCmd(source)
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, name, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	}

	return p, nil
}

// ---------------------------------------------------------------------------
// Right-click
// ---------------------------------------------------------------------------

// SetActionsCfg injects the actions configuration for right-click menus.
func (p *Panel) SetActionsCfg(cfg config.ActionsConfig) { p.actionsCfg = cfg }

// handleMouseRightClick shows a context menu for the extension at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.extensions) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()

	ext := p.extensions[p.cursor]
	label := ext.Manifest.Name

	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemExtension, label)
	if cmd != nil {
		p.pending = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// executeRightClickAction dispatches a right-click action.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	switch action { //nolint:exhaustive // only relevant cases handled
	case actions.ActionToggleDetails:
		return p.toggleExpand()
	case actions.ActionEnableDisable:
		return p.toggleEnable()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// runtimeIcon returns a display icon for the extension runtime type.
func runtimeIcon(runtime string) string {
	switch runtime {
	case "lua":
		return "λ"
	case "wasm":
		return "◈"
	case "mcp":
		return "⟡"
	default:
		return "○"
	}
}

func (p *Panel) buildVisibleRows(width int) []string {
	var rows []string

	for i, ext := range p.extensions {
		isCursor := i == p.cursor
		rows = append(rows, p.renderExtensionRow(ext, width, isCursor))

		// If expanded, append detail lines.
		if p.expanded[ext.Manifest.Name] {
			rows = append(rows, p.renderDetailLines(ext, width)...)
		}
	}

	return rows
}

func (p *Panel) renderExtensionRow(ext extension.ExtensionInfo, width int, isCursor bool) string {
	// Enabled/disabled indicator.
	var statusIcon string
	if ext.Enabled {
		statusIcon = "✓"
	} else {
		statusIcon = "✗"
	}

	icon := runtimeIcon(ext.Manifest.Runtime)

	// Compose the line.
	line := fmt.Sprintf(" %s %s %s v%s",
		statusIcon, icon, ext.Manifest.Name, ext.Manifest.Version)

	if ext.Manifest.Author != "" {
		line += fmt.Sprintf("  by %s", ext.Manifest.Author)
	}

	// Truncate to width.
	if len(line) > width && width > 3 {
		line = line[:width-3] + "..."
	}

	style := lipgloss.NewStyle().Width(width)
	if isCursor && p.Focused {
		style = style.
			Background(lipgloss.Color("#44475A")).
			Foreground(lipgloss.Color("#F8F8F2"))
	} else if ext.Enabled {
		style = style.Foreground(lipgloss.Color("#50FA7B")) // green
	} else {
		style = style.Foreground(lipgloss.Color("#6272A4")) // muted
	}

	return style.Render(line)
}

func (p *Panel) renderDetailLines(ext extension.ExtensionInfo, width int) []string {
	indent := "    "
	detailStyle := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("#8BE9FD")) // cyan

	var rows []string

	if ext.Manifest.Description != "" {
		rows = append(rows, detailStyle.Render(indent+ext.Manifest.Description))
	}

	rows = append(rows, detailStyle.Render(
		fmt.Sprintf("%sRuntime: %s  Entry: %s", indent, ext.Manifest.Runtime, ext.Manifest.EntryPoint)))

	if len(ext.Manifest.Permissions) > 0 {
		rows = append(rows, detailStyle.Render(
			fmt.Sprintf("%sPermissions: %s", indent, strings.Join(ext.Manifest.Permissions, ", "))))
	} else {
		rows = append(rows, detailStyle.Render(indent+"Permissions: none"))
	}

	return rows
}
