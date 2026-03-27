package layout

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/panels"
)

const (
	// resizeStep is the ratio increment/decrement when resizing splits.
	resizeStep = 0.05
	// minRatio is the minimum split ratio to prevent panels from disappearing.
	minRatio = 0.1
	// maxRatio is the maximum split ratio to prevent panels from disappearing.
	maxRatio = 0.9
	// statusBarHeight is the number of rows reserved for the status bar.
	statusBarHeight = 1
	// hintsBarHeight is the number of rows reserved for the keybinding hints bar.
	hintsBarHeight = 1
	// tabBarHeight is the number of rows reserved for the tab bar when
	// multiple tabs are open.
	tabBarReservedHeight = 1
	// borderSize is the number of characters a border occupies on each side.
	borderSize = 1
	// PanelPadH is the horizontal padding (in characters) added inside each
	// panel's left and right edges to keep content away from the border.
	PanelPadH = 1
)

// Engine manages the layout state: the active layout tree, instantiated
// panels, focus tracking, and zoom state.
type Engine struct {
	// Double-click detection: tracks the last click for double-click detection.
	lastClickTime   time.Time       // timestamp of the last click
	ctx             context.Context // stored from Init for late panel creation
	registry        *Registry
	tabs            *TabManager
	panels          map[string]panels.Panel
	dragSplit       *SplitNode // the split being resized
	lastClickPanel  string     // panel name that received the last click
	panelOrder      []string   // ordered list of panel names for focus cycling
	dragArea        Rect       // the area the split occupies (for ratio computation)
	focusIdx        int        // index into panelOrder
	width           int
	height          int
	nextID          int       // counter for generating unique panel names
	dragDir         Direction // direction of the split being dragged
	lastClickRow    int       // content-relative row of the last click
	lastClickCol    int       // content-relative column of the last click
	lastClickHeader bool      // true when the last click was on the header/border title
	zoomed          bool      // true when focused panel is full-screen
	// Drag-resize state: tracks an in-progress mouse drag on a split border.
	dragging bool // true while dragging a split border
}

// NewEngine creates a layout engine with the given registry and initial preset.
func NewEngine(reg *Registry, preset Preset) (*Engine, error) {
	e := &Engine{
		registry: reg,
		tabs:     NewTabManager(preset.Name, preset.Tree.Clone()),
		panels:   make(map[string]panels.Panel),
	}
	if err := e.instantiatePanels(preset.Panels); err != nil {
		return nil, err
	}
	e.panelOrder = preset.Panels
	return e, nil
}

// instantiatePanels creates panel instances from the registry for all
// panel names not already instantiated.
func (e *Engine) instantiatePanels(names []string) error {
	for _, name := range names {
		if _, exists := e.panels[name]; exists {
			continue
		}
		p, err := e.registry.Create(name)
		if err != nil {
			return fmt.Errorf("instantiate panel %q: %w", name, err)
		}
		e.panels[name] = p
	}
	return nil
}

// Init initializes all panels. Must be called after NewEngine.
func (e *Engine) Init(ctx context.Context) tea.Cmd {
	e.ctx = ctx
	var cmds []tea.Cmd
	for _, name := range e.panelOrder {
		p := e.panels[name]
		if cmd := p.Init(ctx); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Focus the first panel
	if len(e.panelOrder) > 0 {
		e.panels[e.panelOrder[0]].Focus()
	}
	// Emit TabActivatedMsg for the initial tab so panels can react to
	// the starting preset (e.g. filetree auto-enables git filter on
	// the git tab).
	tab := e.tabs.ActiveTab()
	if tab != nil {
		activateMsg := panels.TabActivatedMsg{PresetName: tab.Name}
		cmds = append(cmds, func() tea.Msg { return activateMsg })
	}
	return tea.Batch(cmds...)
}

// SetSize updates the terminal dimensions and recalculates panel sizes.
func (e *Engine) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.recalcPanelSizes()
}

// recalcPanelSizes resolves the layout tree and informs each panel of its size.
func (e *Engine) recalcPanelSizes() {
	if e.width <= 0 || e.height <= 0 {
		return
	}
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return
	}
	// Reserve space for status bar, hints bar, and (optionally) tab bar
	panelHeight := e.height - statusBarHeight - hintsBarHeight - e.tabBarHeight()
	if panelHeight < 1 {
		panelHeight = 1
	}
	// The outer border consumes 2 columns (left+right) and 2 rows
	// (top+bottom). Panels fill the remaining inner area.
	innerW := e.width - 2*borderSize
	innerH := panelHeight - 2*borderSize
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	area := Rect{X: 0, Y: 0, Width: innerW, Height: innerH}
	rects := Resolve(tab.Tree, area)
	for name, rect := range rects {
		if p, ok := e.panels[name]; ok {
			// Panels no longer have individual borders; the rect IS
			// the content area within the single outer border.
			p.SetSize(rect.Width, rect.Height)
		}
	}
}

// FocusedPanel returns the currently focused panel, or nil if none.
func (e *Engine) FocusedPanel() panels.Panel {
	if len(e.panelOrder) == 0 {
		return nil
	}
	return e.panels[e.panelOrder[e.focusIdx]]
}

// FocusedName returns the name of the currently focused panel.
func (e *Engine) FocusedName() string {
	if len(e.panelOrder) == 0 {
		return ""
	}
	return e.panelOrder[e.focusIdx]
}

// FocusNext cycles focus to the next panel in order.
func (e *Engine) FocusNext() {
	if len(e.panelOrder) <= 1 {
		return
	}
	e.panels[e.panelOrder[e.focusIdx]].Blur()
	e.focusIdx = (e.focusIdx + 1) % len(e.panelOrder)
	e.panels[e.panelOrder[e.focusIdx]].Focus()
}

// FocusPrev cycles focus to the previous panel in order.
func (e *Engine) FocusPrev() {
	if len(e.panelOrder) <= 1 {
		return
	}
	e.panels[e.panelOrder[e.focusIdx]].Blur()
	e.focusIdx = (e.focusIdx - 1 + len(e.panelOrder)) % len(e.panelOrder)
	e.panels[e.panelOrder[e.focusIdx]].Focus()
}

// FocusByName focuses the panel with the given name. Returns false if not found.
func (e *Engine) FocusByName(name string) bool {
	for i, n := range e.panelOrder {
		if n == name {
			if i != e.focusIdx {
				e.panels[e.panelOrder[e.focusIdx]].Blur()
				e.focusIdx = i
				e.panels[e.panelOrder[e.focusIdx]].Focus()
			}
			return true
		}
	}
	return false
}

// Update routes the given message to the appropriate panel(s).
// Key and mouse events go only to the focused panel (we don't want
// unfocused panels reacting to keyboard input or mouse clicks).
// All other messages (async results, cross-panel notifications, etc.)
// are broadcast to ALL panels in the active tab, following the standard
// Bubble Tea v2 composite model pattern where all sub-models see all
// non-input messages.
func (e *Engine) Update(msg tea.Msg) tea.Cmd {
	// Mouse wheel events route to the panel under the cursor so the
	// user can scroll whichever panel the mouse is hovering over.
	// Mouse click events route to the panel under the cursor, focus it,
	// and send a PanelMouseClickMsg with content-relative coordinates.
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return e.updatePanelAtMouse(msg)
	case tea.MouseClickMsg:
		if e.handleDragStart(msg) {
			return nil
		}
		return e.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		if e.dragging {
			return e.handleDragMotion(msg)
		}
		return e.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		if e.dragging {
			e.handleDragEnd()
			return nil
		}
		return e.handleMouseRelease(msg)
	case tea.KeyMsg, tea.MouseMsg:
		return e.updateFocused(msg)
	case panels.TargetedPanelMsg:
		if p, ok := e.panels[msg.Target]; ok {
			updated, cmd := p.Update(msg.Inner)
			e.panels[msg.Target] = updated
			return cmd
		}
		return nil
	}
	// All other messages broadcast to all panels in the active tab.
	var cmds []tea.Cmd
	for _, name := range e.panelOrder {
		if p, ok := e.panels[name]; ok {
			updated, cmd := p.Update(msg)
			e.panels[name] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return tea.Batch(cmds...)
}

// updatePanelAtMouse routes a mouse message to the panel whose rectangle
// contains the mouse coordinates. Falls back to the focused panel.
func (e *Engine) updatePanelAtMouse(msg tea.Msg) tea.Cmd {
	mm, ok := msg.(tea.MouseMsg)
	if !ok {
		return e.updateFocused(msg)
	}
	m := mm.Mouse()
	// Convert terminal coordinates to inner-content coordinates by
	// subtracting the outer border (1 left, 1 top) and tab bar offset.
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	rects := e.PanelRects()
	for name, r := range rects {
		if innerX >= r.X && innerX < r.X+r.Width && innerY >= r.Y && innerY < r.Y+r.Height {
			if p, ok := e.panels[name]; ok {
				updated, cmd := p.Update(msg)
				e.panels[name] = updated
				return cmd
			}
		}
	}
	return e.updateFocused(msg)
}

// doubleClickThreshold is the maximum time between two clicks at the same
// position to be treated as a double-click. 500ms matches the standard OS
// default (Windows, macOS, most Linux desktops).
const doubleClickThreshold = 500 * time.Millisecond

// handleMouseClick routes a left-click to the panel under the mouse,
// focuses that panel, and sends a PanelMouseClickMsg or
// PanelMouseDoubleClickMsg with coordinates relative to the panel's
// content area. Two clicks at the same position within 300ms are
// treated as a double-click.
func (e *Engine) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	m := msg.Mouse()
	// Handle right-click: route to panel without double-click detection.
	if m.Button == tea.MouseRight {
		return e.handleMouseRightClick(m)
	}
	if m.Button != tea.MouseLeft {
		return nil
	}
	// Convert terminal coordinates to inner-content coordinates.
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	rects := e.PanelRects()
	// Find the panel directly under the click, or the nearest panel if
	// the click landed on a separator or border title.
	hitName := ""
	for name, r := range rects {
		if innerX >= r.X && innerX < r.X+r.Width && innerY >= r.Y && innerY < r.Y+r.Height {
			hitName = name
			break
		}
	}
	// Fallback: if the click landed on a separator or border, find the
	// nearest panel by minimum distance to the panel rect.
	if hitName == "" {
		bestDist := int(^uint(0) >> 1) // max int
		for name, r := range rects {
			dx, dy := 0, 0
			if innerX < r.X {
				dx = r.X - innerX
			} else if innerX >= r.X+r.Width {
				dx = innerX - (r.X + r.Width - 1)
			}
			if innerY < r.Y {
				dy = r.Y - innerY
			} else if innerY >= r.Y+r.Height {
				dy = innerY - (r.Y + r.Height - 1)
			}
			dist := dx + dy
			if dist < bestDist && dist <= 1 {
				bestDist = dist
				hitName = name
			}
		}
	}
	if hitName == "" {
		return nil
	}
	r := rects[hitName]
	// Focus the clicked panel.
	for i, pn := range e.panelOrder {
		if pn == hitName && i != e.focusIdx {
			e.panels[e.panelOrder[e.focusIdx]].Blur()
			e.focusIdx = i
			e.panels[hitName].Focus()
			break
		}
	}
	// Content-relative coordinates (no per-panel border now).
	contentRow := innerY - r.Y
	contentCol := innerX - r.X - PanelPadH
	// Detect whether the click landed on the panel header / border title
	// area (the row above the panel content). This happens when innerY is
	// above the panel rect — the nearest-panel fallback still resolves
	// the correct panel, but the raw contentRow is negative.
	isHeaderClick := contentRow < 0
	if contentRow < 0 {
		contentRow = 0
	}
	if contentCol < 0 {
		contentCol = 0
	}
	// Detect double-click: same panel, same row, same zone (header vs
	// content), within threshold.  The zone check prevents a header click
	// (clamped to row 0) followed by a content-row-0 click from being
	// treated as a double-click.
	now := time.Now()
	isDouble := e.lastClickPanel == hitName &&
		e.lastClickRow == contentRow &&
		e.lastClickHeader == isHeaderClick &&
		now.Sub(e.lastClickTime) <= doubleClickThreshold
	// Record this click for next comparison.
	e.lastClickTime = now
	e.lastClickRow = contentRow
	e.lastClickCol = contentCol
	e.lastClickHeader = isHeaderClick
	e.lastClickPanel = hitName
	if p, ok := e.panels[hitName]; ok {
		var clickMsg tea.Msg
		if isDouble {
			// Reset tracking so a third click isn't also a double.
			e.lastClickTime = time.Time{}
			if isHeaderClick {
				clickMsg = panels.PanelHeaderDoubleClickMsg{
					ContentCol: contentCol,
				}
			} else {
				clickMsg = panels.PanelMouseDoubleClickMsg{
					ContentRow: contentRow,
					ContentCol: contentCol,
				}
			}
		} else {
			clickMsg = panels.PanelMouseClickMsg{
				ContentRow: contentRow,
				ContentCol: contentCol,
			}
		}
		updated, cmd := p.Update(clickMsg)
		e.panels[hitName] = updated
		return cmd
	}
	return nil
}

// handleMouseRightClick routes a right-click to the panel under the mouse,
// focuses that panel, and sends a PanelMouseRightClickMsg with coordinates
// relative to the panel's content area. No double-click detection is
// performed for right-clicks.
func (e *Engine) handleMouseRightClick(m tea.Mouse) tea.Cmd {
	// Convert terminal coordinates to inner-content coordinates.
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	rects := e.PanelRects()
	hitName := ""
	for name, r := range rects {
		if innerX >= r.X && innerX < r.X+r.Width && innerY >= r.Y && innerY < r.Y+r.Height {
			hitName = name
			break
		}
	}
	// Fallback: find nearest panel within 1 cell (separator/border).
	if hitName == "" {
		bestDist := int(^uint(0) >> 1)
		for name, r := range rects {
			dx, dy := 0, 0
			if innerX < r.X {
				dx = r.X - innerX
			} else if innerX >= r.X+r.Width {
				dx = innerX - (r.X + r.Width - 1)
			}
			if innerY < r.Y {
				dy = r.Y - innerY
			} else if innerY >= r.Y+r.Height {
				dy = innerY - (r.Y + r.Height - 1)
			}
			dist := dx + dy
			if dist < bestDist && dist <= 1 {
				bestDist = dist
				hitName = name
			}
		}
	}
	if hitName == "" {
		return nil
	}
	r := rects[hitName]
	// Focus the clicked panel.
	for i, pn := range e.panelOrder {
		if pn == hitName && i != e.focusIdx {
			e.panels[e.panelOrder[e.focusIdx]].Blur()
			e.focusIdx = i
			e.panels[hitName].Focus()
			break
		}
	}
	// Content-relative coordinates (no per-panel border now).
	contentRow := innerY - r.Y
	contentCol := innerX - r.X - PanelPadH
	if contentRow < 0 {
		contentRow = 0
	}
	if contentCol < 0 {
		contentCol = 0
	}
	if p, ok := e.panels[hitName]; ok {
		updated, cmd := p.Update(panels.PanelMouseRightClickMsg{
			ContentRow: contentRow,
			ContentCol: contentCol,
		})
		e.panels[hitName] = updated
		return cmd
	}
	return nil
}

// updateFocused sends a message to only the focused panel.
func (e *Engine) updateFocused(msg tea.Msg) tea.Cmd {
	if len(e.panelOrder) == 0 {
		return nil
	}
	idx := e.focusIdx
	if idx < 0 || idx >= len(e.panelOrder) {
		return nil
	}
	name := e.panelOrder[idx]
	if p, ok := e.panels[name]; ok {
		updated, cmd := p.Update(msg)
		e.panels[name] = updated
		return cmd
	}
	return nil
}

// handleMouseMotion translates raw terminal coordinates into
// panel-relative content coordinates and forwards a
// PanelMouseMotionMsg to the focused panel.
func (e *Engine) handleMouseMotion(msg tea.MouseMotionMsg) tea.Cmd {
	return e.forwardMouseToFocused(msg.Mouse(), func(row, col int) tea.Msg {
		return panels.PanelMouseMotionMsg{ContentRow: row, ContentCol: col}
	})
}

// handleMouseRelease translates raw terminal coordinates into
// panel-relative content coordinates and forwards a
// PanelMouseReleaseMsg to the focused panel.
func (e *Engine) handleMouseRelease(msg tea.MouseReleaseMsg) tea.Cmd {
	return e.forwardMouseToFocused(msg.Mouse(), func(row, col int) tea.Msg {
		return panels.PanelMouseReleaseMsg{ContentRow: row, ContentCol: col}
	})
}

// forwardMouseToFocused converts raw terminal mouse coordinates into
// panel-relative content coordinates and sends the result of buildMsg
// to the focused panel. If the focused panel cannot be resolved or the
// mouse is outside the panel area, it falls back to updateFocused.
func (e *Engine) forwardMouseToFocused(m tea.Mouse, buildMsg func(row, col int) tea.Msg) tea.Cmd {
	if len(e.panelOrder) == 0 {
		return nil
	}
	idx := e.focusIdx
	if idx < 0 || idx >= len(e.panelOrder) {
		return nil
	}
	name := e.panelOrder[idx]
	rects := e.PanelRects()
	r, ok := rects[name]
	if !ok {
		return nil
	}
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	contentRow := innerY - r.Y
	contentCol := innerX - r.X - PanelPadH
	if contentRow < 0 {
		contentRow = 0
	}
	if contentCol < 0 {
		contentCol = 0
	}
	p, ok := e.panels[name]
	if !ok {
		return nil
	}
	updated, cmd := p.Update(buildMsg(contentRow, contentCol))
	e.panels[name] = updated
	return cmd
}

// PanelRects returns the resolved rectangles for each panel in the
// current layout, excluding the status bar and tab bar area.
func (e *Engine) PanelRects() map[string]Rect {
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return nil
	}
	panelHeight := e.height - statusBarHeight - hintsBarHeight - e.tabBarHeight()
	if panelHeight < 1 {
		panelHeight = 1
	}
	// Inner area inside the single outer border.
	innerW := e.width - 2*borderSize
	innerH := panelHeight - 2*borderSize
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	area := Rect{X: 0, Y: 0, Width: innerW, Height: innerH}
	if e.zoomed {
		// Zoomed: only the focused panel fills the entire area
		return map[string]Rect{
			e.FocusedName(): area,
		}
	}
	return Resolve(tab.Tree, area)
}

// ToggleZoom toggles the zoom state of the focused panel.
func (e *Engine) ToggleZoom() {
	e.zoomed = !e.zoomed
	e.recalcPanelSizes()
}

// IsZoomed returns whether the focused panel is in zoom mode.
func (e *Engine) IsZoomed() bool {
	return e.zoomed
}

// ResizeGrow increases the space allocated to the focused panel by
// adjusting the nearest split ratio.
func (e *Engine) ResizeGrow() {
	e.adjustFocusedRatio(resizeStep)
}

// ResizeShrink decreases the space allocated to the focused panel by
// adjusting the nearest split ratio.
func (e *Engine) ResizeShrink() {
	e.adjustFocusedRatio(-resizeStep)
}

func (e *Engine) adjustFocusedRatio(delta float64) {
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return
	}
	focusedName := e.FocusedName()
	split, side := FindSplitContaining(tab.Tree, focusedName)
	if split == nil {
		return
	}
	switch side {
	case "first":
		split.Ratio += delta
	case "second":
		split.Ratio -= delta
	}
	// Clamp
	if split.Ratio < minRatio {
		split.Ratio = minRatio
	}
	if split.Ratio > maxRatio {
		split.Ratio = maxRatio
	}
	e.recalcPanelSizes()
}

// IsDragging returns whether a mouse drag resize is in progress.
func (e *Engine) IsDragging() bool {
	return e.dragging
}

// ---------------------------------------------------------------------------
// Mouse drag resize
// ---------------------------------------------------------------------------
// handleDragStart checks whether a left-click is on a split border.
// If so, it initiates a drag-resize operation and returns true.
func (e *Engine) handleDragStart(msg tea.MouseClickMsg) bool {
	m := msg.Mouse()
	if m.Button != tea.MouseLeft {
		return false
	}
	tab := e.tabs.ActiveTab()
	if tab == nil || e.zoomed {
		return false
	}
	// Convert terminal coordinates to inner-content coordinates.
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	panelHeight := e.height - statusBarHeight - hintsBarHeight - e.tabBarHeight()
	if panelHeight < 1 {
		panelHeight = 1
	}
	innerW := e.width - 2*borderSize
	innerH := panelHeight - 2*borderSize
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	area := Rect{X: 0, Y: 0, Width: innerW, Height: innerH}
	split, dir, splitArea := FindSplitAtBorder(tab.Tree, innerX, innerY, area, 0)
	if split == nil {
		return false
	}
	e.dragging = true
	e.dragSplit = split
	e.dragDir = dir
	e.dragArea = splitArea
	return true
}

// handleDragMotion updates the split ratio during a drag resize.
func (e *Engine) handleDragMotion(msg tea.MouseMotionMsg) tea.Cmd {
	m := msg.Mouse()
	// Convert terminal coordinates to inner-content coordinates.
	innerX := m.X - borderSize
	innerY := m.Y - e.tabBarHeight() - borderSize
	var ratio float64
	switch e.dragDir {
	case Horizontal:
		if e.dragArea.Width <= 0 {
			return nil
		}
		ratio = float64(innerX-e.dragArea.X) / float64(e.dragArea.Width)
	case Vertical:
		if e.dragArea.Height <= 0 {
			return nil
		}
		ratio = float64(innerY-e.dragArea.Y) / float64(e.dragArea.Height)
	default:
		return nil
	}
	if ratio < minRatio {
		ratio = minRatio
	}
	if ratio > maxRatio {
		ratio = maxRatio
	}
	e.dragSplit.Ratio = ratio
	e.recalcPanelSizes()
	return nil
}

// handleDragEnd finishes a drag resize, clearing all drag state.
func (e *Engine) handleDragEnd() {
	e.dragging = false
	e.dragSplit = nil
	e.dragArea = Rect{}
}

// Panels returns the map of panel name to panel instance.
func (e *Engine) Panels() map[string]panels.Panel {
	return e.panels
}

// PanelOrder returns the ordered list of panel names.
func (e *Engine) PanelOrder() []string {
	return e.panelOrder
}

// Width returns the current total width.
func (e *Engine) Width() int {
	return e.width
}

// Height returns the current total height.
func (e *Engine) Height() int {
	return e.height
}

// TabManager returns the underlying tab manager.
func (e *Engine) TabManager() *TabManager {
	return e.tabs
}

// StatusBarHeight returns the height reserved for the status bar.
func (e *Engine) StatusBarHeight() int {
	return statusBarHeight
}

// tabBarHeight returns the height reserved for the tab bar.
// In single-tab mode no space is reserved; otherwise one row is kept
// so the tab bar stays visible for discoverability.
func (e *Engine) tabBarHeight() int {
	if SingleTabMode {
		return 0
	}
	return tabBarReservedHeight
}

// TabBarHeight returns the exported tab bar height for rendering.
func (e *Engine) TabBarHeight() int {
	return e.tabBarHeight()
}

// BorderSize returns the width of one side of the outer border.
func (e *Engine) BorderSize() int {
	return borderSize
}

// InnerArea returns the inner content area dimensions (after subtracting
// the single outer border) used to resolve panel rects.
func (e *Engine) InnerArea() Rect {
	panelHeight := e.height - statusBarHeight - hintsBarHeight - e.tabBarHeight()
	if panelHeight < 1 {
		panelHeight = 1
	}
	innerW := e.width - 2*borderSize
	innerH := panelHeight - 2*borderSize
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	return Rect{X: 0, Y: 0, Width: innerW, Height: innerH}
}

// ---------------------------------------------------------------------------
// Tab operations
// ---------------------------------------------------------------------------
// AddTab creates a new tab from a preset, instantiating any panels not
// already present. Returns a tea.Cmd from initializing new panels.
func (e *Engine) AddTab(preset Preset) (tea.Cmd, error) {
	// Blur the current panel before switching.
	e.blurCurrent()
	// Track which panels are new so we can Init them.
	var newPanels []string
	for _, name := range preset.Panels {
		if _, exists := e.panels[name]; !exists {
			newPanels = append(newPanels, name)
		}
	}
	if err := e.instantiatePanels(preset.Panels); err != nil {
		return nil, err
	}
	e.tabs.Add(preset.Name, preset.Tree.Clone())
	e.panelOrder = preset.Panels
	e.focusIdx = 0
	e.zoomed = false
	// Init new panels.
	var cmds []tea.Cmd
	for _, name := range newPanels {
		if p, ok := e.panels[name]; ok && e.ctx != nil {
			if cmd := p.Init(e.ctx); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	if len(e.panelOrder) > 0 {
		e.panels[e.panelOrder[0]].Focus()
	}
	e.recalcPanelSizes()
	return tea.Batch(cmds...), nil
}

// CloseActiveTab closes the currently active tab. Returns an error if
// it is the last remaining tab.
func (e *Engine) CloseActiveTab() error {
	e.blurCurrent()
	if err := e.tabs.Close(e.tabs.ActiveIndex()); err != nil {
		return err
	}
	e.syncToActiveTab()
	return nil
}

// NextTab cycles to the next tab.
func (e *Engine) NextTab() {
	if e.tabs.Count() <= 1 {
		return
	}
	e.blurCurrent()
	e.tabs.NextTab()
	e.syncToActiveTab()
}

// PrevTab cycles to the previous tab.
func (e *Engine) PrevTab() {
	if e.tabs.Count() <= 1 {
		return
	}
	e.blurCurrent()
	e.tabs.PrevTab()
	e.syncToActiveTab()
}

// SwitchTab activates the tab at the given index.
func (e *Engine) SwitchTab(idx int) error {
	e.blurCurrent()
	if err := e.tabs.Select(idx); err != nil {
		return err
	}
	e.syncToActiveTab()
	return nil
}

// RenameActiveTab renames the currently active tab.
func (e *Engine) RenameActiveTab(name string) error {
	return e.tabs.Rename(e.tabs.ActiveIndex(), name)
}

// MoveTabLeft swaps the active tab with its left neighbor.
func (e *Engine) MoveTabLeft() {
	e.tabs.MoveLeft()
}

// MoveTabRight swaps the active tab with its right neighbor.
func (e *Engine) MoveTabRight() {
	e.tabs.MoveRight()
}

// ---------------------------------------------------------------------------
// Split / panel operations
// ---------------------------------------------------------------------------
// SplitFocusedHorizontal splits the focused panel with a horizontal divider,
// placing a new panel of the given type below.
func (e *Engine) SplitFocusedHorizontal(newPanelType string) (tea.Cmd, error) {
	return e.splitFocused(Vertical, newPanelType)
}

// SplitFocusedVertical splits the focused panel with a vertical divider,
// placing a new panel of the given type to the right.
func (e *Engine) SplitFocusedVertical(newPanelType string) (tea.Cmd, error) {
	return e.splitFocused(Horizontal, newPanelType)
}

// splitFocused replaces the focused panel's leaf node with a SplitNode
// containing the original panel and a newly created panel.
func (e *Engine) splitFocused(dir Direction, newPanelType string) (tea.Cmd, error) {
	focusedName := e.FocusedName()
	if focusedName == "" {
		return nil, fmt.Errorf("no focused panel to split")
	}
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return nil, fmt.Errorf("no active tab")
	}
	// Generate a unique name for the new panel instance.
	newName := e.uniqueName(newPanelType)
	// Create the new panel via the registry (using the base type).
	p, err := e.registry.Create(newPanelType)
	if err != nil {
		return nil, fmt.Errorf("create panel %q: %w", newPanelType, err)
	}
	e.panels[newName] = p
	// Init the panel if we have a context.
	var cmd tea.Cmd
	if e.ctx != nil {
		cmd = p.Init(e.ctx)
	}
	// Modify the tree.
	tab.Tree = SplitLeaf(tab.Tree, focusedName, dir, newName)
	// Rebuild panel order from the tree.
	e.panelOrder = tab.Tree.PanelNames()
	// Keep focus on the original panel.
	for i, name := range e.panelOrder {
		if name == focusedName {
			e.focusIdx = i
			break
		}
	}
	e.recalcPanelSizes()
	return cmd, nil
}

// CloseFocusedPanel removes the focused panel from the current tab's tree.
// The sibling takes the parent split's place. Returns an error if it is the
// last panel in the tab.
func (e *Engine) CloseFocusedPanel() error {
	focusedName := e.FocusedName()
	if focusedName == "" {
		return fmt.Errorf("no focused panel to close")
	}
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return fmt.Errorf("no active tab")
	}
	names := tab.Tree.PanelNames()
	if len(names) <= 1 {
		return fmt.Errorf("cannot close the last panel")
	}
	e.panels[focusedName].Blur()
	newTree, found := RemoveLeaf(tab.Tree, focusedName)
	if !found {
		return fmt.Errorf("panel %q not found in tree", focusedName)
	}
	tab.Tree = newTree
	// Note: we keep the panel instance in e.panels because another tab
	// may still reference it. Only uniquely-named split panels (e.g.
	// "preview:2") are safe to remove.
	if isUniqueInstanceName(focusedName) {
		delete(e.panels, focusedName)
	}
	e.panelOrder = tab.Tree.PanelNames()
	if e.focusIdx >= len(e.panelOrder) {
		e.focusIdx = len(e.panelOrder) - 1
	}
	if e.focusIdx < 0 {
		e.focusIdx = 0
	}
	if len(e.panelOrder) > 0 {
		e.panels[e.panelOrder[e.focusIdx]].Focus()
	}
	e.recalcPanelSizes()
	return nil
}

// ---------------------------------------------------------------------------
// Preview position cycling
// ---------------------------------------------------------------------------
// PreviewPosition represents where the preview panel sits relative to
// the filetree.
type PreviewPosition int

const (
	PreviewRight  PreviewPosition = iota // default: filetree | preview
	PreviewBottom                        // filetree on top, preview on bottom
	PreviewLeft                          // preview | filetree
	PreviewTop                           // preview on top, filetree on bottom
)

// String returns the string representation of a PreviewPosition.
func (p PreviewPosition) String() string {
	switch p {
	case PreviewRight:
		return "right" //nolint:goconst // inline string is more readable here
	case PreviewBottom:
		return "bottom"
	case PreviewLeft:
		return "left"
	case PreviewTop:
		return "top"
	default:
		return "right"
	}
}

// PreviewPositionFromString converts a string to a PreviewPosition.
// Unrecognised values default to PreviewRight.
func PreviewPositionFromString(s string) PreviewPosition {
	switch s {
	case "right":
		return PreviewRight
	case "bottom":
		return PreviewBottom
	case "left":
		return PreviewLeft
	case "top":
		return PreviewTop
	default:
		return PreviewRight
	}
}

// RotatePreviewPosition cycles the preview panel position in the active
// tab through right → bottom → left → top → right.
func (e *Engine) RotatePreviewPosition() {
	cur := e.CurrentPreviewPosition()
	next := (cur + 1) % 4
	e.SetPreviewPosition(next)
}

// CurrentPreviewPosition returns the current preview position of the active
// tab's layout. It checks whether "preview" is a direct child of the root
// split and infers position from the split direction and child order.
// If preview is not found at the root level, returns PreviewRight as default.
func (e *Engine) CurrentPreviewPosition() PreviewPosition {
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return PreviewRight
	}
	split, ok := tab.Tree.(*SplitNode)
	if !ok {
		return PreviewRight
	}
	previewIsFirst := isPreviewLeaf(split.First)
	previewIsSecond := isPreviewLeaf(split.Second)
	if !previewIsFirst && !previewIsSecond {
		return PreviewRight
	}
	if split.Direction == Horizontal {
		if previewIsFirst {
			return PreviewLeft
		}
		return PreviewRight
	}
	// Vertical
	if previewIsFirst {
		return PreviewTop
	}
	return PreviewBottom
}

// SetPreviewPosition sets the preview panel position across all tabs whose
// tree is a simple two-leaf split of "filetree" and "preview". This ensures
// that switching tabs preserves the chosen position. If the active tab's
// position already matches, this is a no-op.
func (e *Engine) SetPreviewPosition(pos PreviewPosition) {
	// Apply to every tab so switching tabs never reverts the position.
	modified := false
	for i := range e.tabs.tabs {
		if applyPreviewPositionToTree(e.tabs.tabs[i].Tree, pos) {
			modified = true
		}
	}
	if !modified {
		return
	}
	// Sync engine state (panelOrder, focusIdx, sizes) to the active tab.
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return
	}
	e.panelOrder = tab.Tree.PanelNames()
	focused := e.FocusedName()
	for i, n := range e.panelOrder {
		if n == focused {
			e.focusIdx = i
			break
		}
	}
	e.recalcPanelSizes()
}

// applyPreviewPositionToTree modifies a single tab's layout tree to place the
// preview panel at the given position. It works on any tree where "preview" is
// a direct child of the root split. Returns true if the tree was modified.
func applyPreviewPositionToTree(tree Node, pos PreviewPosition) bool {
	split, ok := tree.(*SplitNode)
	if !ok {
		return false
	}
	// Find preview as a direct child of the root split.
	var preview *LeafNode
	var rest Node
	if isPreviewLeaf(split.First) {
		leaf, ok := split.First.(*LeafNode)
		if !ok {
			return false
		}
		preview = leaf
		rest = split.Second
	} else if isPreviewLeaf(split.Second) {
		leaf, ok := split.Second.(*LeafNode)
		if !ok {
			return false
		}
		preview = leaf
		rest = split.First
	}
	if preview == nil {
		return false
	}
	// For complex trees (rest is a subtree with multiple panels), vertical
	// splits give more space to the rest since it contains stacked panels.
	_, simpleRest := rest.(*LeafNode)
	switch pos {
	case PreviewRight:
		split.Direction = Horizontal
		split.Ratio = 0.3
		split.First = rest
		split.Second = preview
	case PreviewBottom:
		split.Direction = Vertical
		if simpleRest {
			split.Ratio = 0.3 // rest 30%, preview 70%
		} else {
			split.Ratio = 0.7 // rest 70%, preview 30%
		}
		split.First = rest
		split.Second = preview
	case PreviewLeft:
		split.Direction = Horizontal
		split.Ratio = 0.7
		split.First = preview
		split.Second = rest
	case PreviewTop:
		split.Direction = Vertical
		if simpleRest {
			split.Ratio = 0.7 // preview 70%, rest 30%
		} else {
			split.Ratio = 0.3 // preview 30%, rest 70%
		}
		split.First = preview
		split.Second = rest
	}
	return true
}

// isPreviewLeaf returns true if the node is a LeafNode named "preview".
func isPreviewLeaf(n Node) bool {
	leaf, ok := n.(*LeafNode)
	return ok && leaf.Panel == "preview"
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------
// blurCurrent blurs the currently focused panel, if any.
func (e *Engine) blurCurrent() {
	if fp := e.FocusedPanel(); fp != nil {
		fp.Blur()
	}
}

// syncToActiveTab updates panelOrder, focus, and sizing to match the
// currently active tab. Called after tab switching or closing.
func (e *Engine) syncToActiveTab() {
	tab := e.tabs.ActiveTab()
	if tab == nil {
		return
	}
	// Instantiate any panels referenced by the new tab that haven't
	// been created yet (possible if presets reference different panels).
	newNames := tab.Tree.PanelNames()
	_ = e.instantiatePanels(newNames)
	e.panelOrder = newNames
	e.focusIdx = 0
	e.zoomed = false
	if len(e.panelOrder) > 0 {
		e.panels[e.panelOrder[0]].Focus()
	}
	e.recalcPanelSizes()
}

// uniqueName returns a name for a new panel of the given base type that
// does not collide with any existing panel in e.panels.
func (e *Engine) uniqueName(baseType string) string {
	if _, exists := e.panels[baseType]; !exists {
		return baseType
	}
	for {
		e.nextID++
		name := fmt.Sprintf("%s:%d", baseType, e.nextID)
		if _, exists := e.panels[name]; !exists {
			return name
		}
	}
}

// isUniqueInstanceName returns true if the panel name is a generated
// unique instance (contains a ":" separator), indicating it was created
// by a split operation and is not shared across tabs.
func isUniqueInstanceName(name string) bool {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == ':' {
			return true
		}
	}
	return false
}
