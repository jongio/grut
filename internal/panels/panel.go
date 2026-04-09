// Package panels defines the Panel interface that all TUI panels implement.
// Panels are the building blocks of the grut UI — each panel renders a
// rectangular region and handles its own key events when focused.
package panels

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// Panel is the interface that all grut panels implement. Each panel occupies
// a rectangular region in the layout and can receive focus for keyboard input.
// Panels return plain strings from View; the root model wraps the final
// composition into a tea.View.
type Panel interface {
	// Init is called once when the panel is first created. The context is
	// cancelled on application shutdown and should be used for cancellation
	// of any background work.
	Init(ctx context.Context) tea.Cmd

	// Update processes a message and returns the (potentially updated) panel
	// and an optional command.
	Update(msg tea.Msg) (Panel, tea.Cmd)

	// View renders the panel content into the given width×height area.
	// Returns a plain string (not tea.View) — the root model handles
	// the final tea.View composition.
	View(width, height int) string

	// Focus is called when the panel gains keyboard focus.
	Focus()

	// Blur is called when the panel loses keyboard focus.
	Blur()

	// SetSize informs the panel of its allocated dimensions. Called on
	// window resize and layout changes.
	SetSize(width, height int)

	// Title returns the display name of the panel (shown in borders/tabs).
	Title() string

	// KeyBindings returns the panel-specific key bindings for help display.
	KeyBindings() []KeyBinding
}

// KeyBinding describes a single key binding for documentation and help text.
type KeyBinding struct {
	Key         string // Key combination, e.g. "ctrl+s", "enter"
	Description string // Human-readable description, e.g. "Stage file"
	Action      string // Internal action identifier, e.g. "stage"
}

// Closer is an optional interface that panels can implement to release
// resources (watchers, goroutines, file handles) on application shutdown.
// The root model calls Close() on every panel that implements it when the
// user quits (F29).
type Closer interface {
	Close()
}

// SelectionCopier is an optional interface for panels that support text
// selection and clipboard copy. When the focused panel implements this
// interface and HasSelection returns true, Ctrl+C copies the selected
// text instead of quitting.
type SelectionCopier interface {
	HasSelection() bool
	CopySelection() (Panel, tea.Cmd)
}

// BasePanel provides default implementations for Focus, Blur, SetSize, Title,
// and KeyBindings. Panels that embed BasePanel only need to implement Init,
// Update, and View (F07).
type BasePanel struct {
	PanelTitle string // exported so embedding types can set it at construction
	Focused    bool
	Width      int
	Height     int
}

// Focus implements Panel.
func (b *BasePanel) Focus() { b.Focused = true }

// Blur implements Panel.
func (b *BasePanel) Blur() { b.Focused = false }

// SetSize implements Panel.
func (b *BasePanel) SetSize(width, height int) {
	b.Width = width
	b.Height = height
}

// Title implements Panel.
func (b *BasePanel) Title() string { return b.PanelTitle }

// KeyBindings implements Panel. Returns nil (no bindings) by default.
func (b *BasePanel) KeyBindings() []KeyBinding { return nil }

// ScrollDelta is the number of lines to scroll on each mouse-wheel tick.
// All panels use this shared constant for consistent scroll behavior.
const ScrollDelta = 3
