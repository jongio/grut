// Package settings implements the settings overlay panel for grut.
// It displays a navigable list of configurable options (preview position,
// theme, double-click actions) and emits messages when the user changes a value.
package settings

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// Message type aliases — the canonical definitions now live in the panels
// package so that the root TUI model can type-switch without importing
// this package directly. The aliases preserve backwards compatibility
// for any external callers and internal tests.
type (
	SetPreviewPositionMsg = panels.SetPreviewPositionMsg
	SetThemeMsg           = panels.SetThemeMsg
	SetDoubleClickActionMsg = panels.SetDoubleClickActionMsg
	SetRightClickActionMsg  = panels.SetRightClickActionMsg
	ResetActionPromptsMsg   = panels.ResetActionPromptsMsg
	ToggleSettingsMsg       = panels.ToggleSettingsMsg
)

// settingField identifies which setting row the cursor is on.
type settingField int

const (
	fieldPreviewPosition settingField = iota
	fieldTheme
	fieldActionsStart // sentinel: double-click action fields start here
	// fieldResetPrompts is computed dynamically after action fields.
)

// previewOptions defines the ordered list of preview positions.
var previewOptions = []layout.PreviewPosition{
	layout.PreviewRight,
	layout.PreviewBottom,
	layout.PreviewLeft,
	layout.PreviewTop,
}

// separatorWidth is the number of ─ characters drawn under section headings.
const separatorWidth = 20

type panelColors struct {
	Heading   string
	Selected  string
	Active    string
	Dim       string
	Separator string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Heading:   "#C9875A",
		Selected:  "#6B9E56",
		Active:    "#C9A227",
		Dim:       "#555555",
		Separator: "#2A2A2A",
	}
	if th != nil {
		c.Heading = th.Colors.BrightBlue
		c.Selected = th.Colors.NormalGreen
		c.Active = th.Colors.BorderFocused
		c.Dim = th.Colors.BrightBlack
		c.Separator = th.Colors.SelectionBg
	}
	return c
}

// Panel is the settings overlay. It implements [panels.Panel].
type Panel struct {
	actionsCfg          config.ActionsConfig // persisted action overrides
	actionOverrides     map[string]string    // in-memory copy of current double-click overrides
	rightClickOverrides map[string]string    // in-memory copy of current right-click overrides
	themeName           string               // active theme name
	themeNames          []string             // available theme names
	configurableItems   []actions.ItemType   // cached from actions.ConfigurableItems()
	panels.BasePanel
	colors     panelColors
	theme      *theme.Theme
	cursor     settingField           // which setting row is highlighted
	offset     int                    // scroll offset for viewport
	totalLines int                    // total rendered content lines (updated each View)
	previewPos layout.PreviewPosition // active preview position
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new settings overlay panel.
func New(currentPos layout.PreviewPosition, currentTheme string, themeNames []string, actionsCfg config.ActionsConfig, th *theme.Theme) *Panel {
	items := actions.ConfigurableItems()
	// Seed in-memory overrides from persisted config.
	overrides := make(map[string]string, len(items))
	for _, it := range items {
		if a := actionsCfg.GetDoubleClickAction(string(it)); a != "" {
			overrides[string(it)] = a
		}
	}
	rcOverrides := make(map[string]string, len(items))
	if actionsCfg.RightClick != nil {
		for _, it := range items {
			if a, ok := actionsCfg.RightClick[string(it)]; ok && a != "" {
				rcOverrides[string(it)] = a
			}
		}
	}
	return &Panel{
		BasePanel:           panels.BasePanel{PanelTitle: "settings"},
		cursor:              fieldPreviewPosition,
		previewPos:          currentPos,
		themeName:           currentTheme,
		themeNames:          themeNames,
		actionsCfg:          actionsCfg,
		configurableItems:   items,
		actionOverrides:     overrides,
		rightClickOverrides: rcOverrides,
		colors:              initColors(th),
		theme:               th,
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	}
	return p, nil
}

// View implements panels.Panel. It renders the settings menu within the
// given dimensions.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	styles := p.viewStyles()
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	row := func(s string) string {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(s)
	}
	previewLabel := positionLabel(p.previewPos)
	themeLabel := p.themeName
	if themeLabel == "" {
		themeLabel = labelDefault
	}
	rendered := []string{
		emptyLine,
	}
	rendered = append(rendered, p.renderSection(row, styles, "Preview Position", previewLabel, fieldPreviewPosition)...)
	rendered = append(rendered, emptyLine)
	rendered = append(rendered, p.renderSection(row, styles, "Theme", themeLabel, fieldTheme)...)
	// Double-click actions section.
	if len(p.configurableItems) > 0 {
		rendered = append(rendered, emptyLine)
		rendered = append(rendered, row("  "+styles.heading.Render("Double-Click Actions")))
		rendered = append(rendered, row("  "+styles.sep.Render(strings.Repeat("\u2500", separatorWidth))))
		for i, it := range p.configurableItems {
			field := fieldActionsStart + settingField(i)
			currentAction := p.actionOverrides[string(it)]
			if currentAction == "" {
				currentAction = string(actions.DefaultAction(it))
			}
			label := actions.ItemLabel(it)
			actionLabel := actions.ActionLabel(actions.ActionID(currentAction))
			line := "  "
			if p.cursor == field {
				line += styles.selected.Render("\u25b8 " + label + "  \u25c2 " + actionLabel + " \u25b8")
			} else {
				line += styles.active.Render("  "+label) + "  " + styles.dim.Render(actionLabel)
			}
			rendered = append(rendered, row(line))
		}
	}
	// Right-click actions section.
	if len(p.configurableItems) > 0 {
		rendered = append(rendered, emptyLine)
		rendered = append(rendered, row("  "+styles.heading.Render("Right-Click Actions")))
		rendered = append(rendered, row("  "+styles.sep.Render(strings.Repeat("\u2500", separatorWidth))))
		for i, it := range p.configurableItems {
			field := p.fieldRightClickStart() + settingField(i)
			currentAction := p.rightClickOverrides[string(it)]
			if currentAction == "" {
				currentAction = string(actions.ActionShowContextMenu)
			}
			label := actions.ItemLabel(it)
			actionLabel := actions.ActionLabel(actions.ActionID(currentAction))
			line := "  "
			if p.cursor == field {
				line += styles.selected.Render("\u25b8 " + label + "  \u25c2 " + actionLabel + " \u25b8")
			} else {
				line += styles.active.Render("  "+label) + "  " + styles.dim.Render(actionLabel)
			}
			rendered = append(rendered, row(line))
		}
	}
	// Confirmations section.
	rendered = append(rendered, emptyLine)
	rendered = append(rendered, row("  "+styles.heading.Render("Confirmations")))
	rendered = append(rendered, row("  "+styles.sep.Render(strings.Repeat("\u2500", separatorWidth))))
	resetLine := "  "
	if p.cursor == p.fieldResetPrompts() {
		resetLine += styles.selected.Render("\u25b8 Reset all prompts  \u23ce")
	} else {
		resetLine += styles.active.Render("  Reset all prompts")
	}
	rendered = append(rendered, row(resetLine))
	rendered = append(rendered, emptyLine)
	rendered = append(rendered, row("  "+styles.dim.Render("j/k navigate \u00b7 Enter cycle \u00b7 Esc close")))
	// Track total content height for mouse scroll bounds.
	p.totalLines = len(rendered)
	// Apply scroll offset.
	if p.offset > 0 && p.offset < len(rendered) {
		rendered = rendered[p.offset:]
	}
	// Pad/truncate to exact height.
	for len(rendered) < height {
		rendered = append(rendered, emptyLine)
	}
	if len(rendered) > height {
		rendered = rendered[:height]
	}
	return strings.Join(rendered, "\n")
}

// viewStyles holds pre-built lipgloss styles used during rendering.
type viewStyles struct {
	heading  lipgloss.Style
	selected lipgloss.Style
	active   lipgloss.Style
	dim      lipgloss.Style
	sep      lipgloss.Style
}

// viewStyles creates the style set for the current render pass.
func (p *Panel) viewStyles() viewStyles {
	return viewStyles{
		heading:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Heading)).Bold(true),
		selected: lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Selected)).Bold(true),
		active:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Active)),
		dim:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim)),
		sep:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Separator)),
	}
}

// renderSection renders a single settings section: heading, separator, and
// the current value with cursor indicator.
func (p *Panel) renderSection(row func(string) string, s viewStyles, heading, value string, field settingField) []string {
	lines := []string{ //nolint:prealloc // composite literal initialization
		row("  " + s.heading.Render(heading)),
		row("  " + s.sep.Render(strings.Repeat("─", separatorWidth))),
	}
	line := "  "
	if p.cursor == field {
		line += s.selected.Render("▸ " + value + "  ⏎ cycle")
	} else {
		line += s.active.Render("  " + value)
	}
	lines = append(lines, row(line))
	return lines
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move up", Action: "cursor_up"},
		{Key: "Enter", Description: "Cycle value", Action: "select"},
		{Key: "Esc", Description: "Close settings", Action: "close"},
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
// fieldCount returns the total number of navigable fields.
func (p *Panel) fieldCount() settingField {
	// preview + theme + N double-click items + N right-click items + reset prompts
	return fieldActionsStart + 2*settingField(len(p.configurableItems)) + 1
}

// fieldRightClickStart returns the field index where right-click action
// items begin (immediately after double-click items).
func (p *Panel) fieldRightClickStart() settingField {
	return fieldActionsStart + settingField(len(p.configurableItems))
}

// fieldResetPrompts returns the field index for the "Reset all prompts" row.
func (p *Panel) fieldResetPrompts() settingField {
	return fieldActionsStart + 2*settingField(len(p.configurableItems))
}

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if p.cursor < p.fieldCount()-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		switch {
		case p.cursor == fieldPreviewPosition:
			p.previewPos = cyclePreviewPosition(p.previewPos)
			return p, func() tea.Msg {
				return SetPreviewPositionMsg{Position: int(p.previewPos)}
			}
		case p.cursor == fieldTheme:
			p.themeName = p.cycleTheme(p.themeName)
			return p, func() tea.Msg {
				return SetThemeMsg{Name: p.themeName}
			}
		case p.cursor == p.fieldResetPrompts():
			return p, p.resetPrompts()
		case p.cursor >= fieldActionsStart && p.cursor < p.fieldRightClickStart():
			idx := int(p.cursor - fieldActionsStart)
			if idx < len(p.configurableItems) {
				return p, p.cycleAction(idx)
			}
		case p.cursor >= p.fieldRightClickStart() && p.cursor < p.fieldResetPrompts():
			idx := int(p.cursor - p.fieldRightClickStart())
			if idx < len(p.configurableItems) {
				return p, p.cycleRightClickAction(idx)
			}
		}
	case "escape", "esc":
		return p, func() tea.Msg { return ToggleSettingsMsg{} }
	}
	return p, nil
}

func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := p.totalLines - p.Height
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
// Helpers
// ---------------------------------------------------------------------------
// cycleAction cycles the double-click action for the item at the given index
// and returns a command that emits SetDoubleClickActionMsg.
func (p *Panel) cycleAction(idx int) tea.Cmd {
	it := p.configurableItems[idx]
	allActs := actions.AllActions(it)
	if len(allActs) == 0 {
		return nil
	}
	current := p.actionOverrides[string(it)]
	if current == "" {
		current = string(actions.DefaultAction(it))
	}
	// Find next action in cycle.
	nextIdx := 0
	for i, a := range allActs {
		if string(a) == current {
			nextIdx = (i + 1) % len(allActs)
			break
		}
	}
	next := string(allActs[nextIdx])
	p.actionOverrides[string(it)] = next
	return func() tea.Msg {
		return SetDoubleClickActionMsg{
			ItemType: string(it),
			Action:   next,
		}
	}
}

// cycleRightClickAction cycles the right-click action for the item at the
// given index and returns a command that emits SetRightClickActionMsg.
func (p *Panel) cycleRightClickAction(idx int) tea.Cmd {
	it := p.configurableItems[idx]
	allActs := actions.AllRightClickActions(it)
	if len(allActs) == 0 {
		return nil
	}
	current := p.rightClickOverrides[string(it)]
	if current == "" {
		current = string(actions.ActionShowContextMenu)
	}
	// Find next action in cycle.
	nextIdx := 0
	for i, a := range allActs {
		if string(a) == current {
			nextIdx = (i + 1) % len(allActs)
			break
		}
	}
	next := string(allActs[nextIdx])
	p.rightClickOverrides[string(it)] = next
	return func() tea.Msg {
		return SetRightClickActionMsg{
			ItemType: string(it),
			Action:   next,
		}
	}
}

// resetPrompts returns a command that emits ResetActionPromptsMsg.
func (p *Panel) resetPrompts() tea.Cmd {
	return func() tea.Msg {
		return ResetActionPromptsMsg{}
	}
}

// cyclePreviewPosition returns the next preview position in the cycle.
func cyclePreviewPosition(current layout.PreviewPosition) layout.PreviewPosition {
	for i, pos := range previewOptions {
		if pos == current {
			return previewOptions[(i+1)%len(previewOptions)]
		}
	}
	return previewOptions[0]
}

// positionLabel returns a display label for a preview position.
func positionLabel(pos layout.PreviewPosition) string {
	switch pos {
	case layout.PreviewRight:
		return labelRight
	case layout.PreviewBottom:
		return "Bottom"
	case layout.PreviewLeft:
		return "Left"
	case layout.PreviewTop:
		return "Top"
	default:
		return labelRight
	}
}

// cycleTheme returns the next theme name in the cycle.
func (p *Panel) cycleTheme(current string) string {
	if len(p.themeNames) == 0 {
		return current
	}
	if current == "" {
		current = labelDefault
	}
	for i, name := range p.themeNames {
		if name == current {
			return p.themeNames[(i+1)%len(p.themeNames)]
		}
	}
	return p.themeNames[0]
}

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package)
// ---------------------------------------------------------------------------
// cursorIndex returns the current cursor position.
func (p *Panel) cursorIndex() settingField { return p.cursor }

// currentPosition returns the marked-as-active preview position.
func (p *Panel) currentPosition() layout.PreviewPosition { return p.previewPos }

// currentThemeName returns the marked-as-active theme name.
func (p *Panel) currentThemeName() string { return p.themeName }

// currentActionOverride returns the in-memory action override for an item type.
func (p *Panel) currentActionOverride(itemType string) string {
	return p.actionOverrides[itemType]
}

// currentRightClickOverride returns the in-memory right-click action override
// for an item type.
func (p *Panel) currentRightClickOverride(itemType string) string {
	return p.rightClickOverrides[itemType]
}
