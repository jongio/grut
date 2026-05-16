package layout

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/panels"
)

// PanelManager is the interface consumed by the TUI orchestration layer. It
// abstracts all panel-management, focus, layout, tab, and split operations
// so that the TUI depends on a behaviour contract instead of the concrete
// Engine implementation. This enables independent testing and future
// alternative layout strategies.
type PanelManager interface {
	// Lifecycle
	Init(ctx context.Context) tea.Cmd
	SetSize(width, height int)
	Update(msg tea.Msg) tea.Cmd

	// Focus
	FocusedPanel() panels.Panel
	FocusedName() string
	FocusNext()
	FocusPrev()
	FocusByName(name string) bool

	// Zoom & resize
	ToggleZoom()
	IsZoomed() bool
	ResizeGrow()
	ResizeShrink()

	// Preview position
	RotatePreviewPosition()
	CurrentPreviewPosition() PreviewPosition
	SetPreviewPosition(pos PreviewPosition)

	// Panel queries
	Panels() map[string]panels.Panel
	PanelRects() map[string]Rect
	InnerArea() Rect

	// Tab management
	TabManager() *TabManager
	AddTab(preset Preset) (tea.Cmd, error)
	CloseActiveTab() error
	SwitchTab(idx int) error

	// Split operations
	SplitFocusedVertical(newPanelType string) (tea.Cmd, error)
	SplitFocusedHorizontal(newPanelType string) (tea.Cmd, error)
	CloseFocusedPanel() error
}

// Compile-time assertion: *Engine satisfies PanelManager.
var _ PanelManager = (*Engine)(nil)
