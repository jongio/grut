package panels

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Placeholder is a stub panel that displays its name centered. It implements
// the Panel interface and is used as a stand-in until real panel
// implementations (filetree, preview, etc.) are built. It embeds BasePanel
// for default Focus/Blur/SetSize/Title/KeyBindings (F07).
type Placeholder struct {
	BasePanel
}

// NewPlaceholder creates a new placeholder panel with the given name.
func NewPlaceholder(name string) *Placeholder {
	return &Placeholder{BasePanel: BasePanel{PanelTitle: name}}
}

// Init implements Panel.
func (p *Placeholder) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements Panel.
func (p *Placeholder) Update(_ tea.Msg) (Panel, tea.Cmd) {
	return p, nil
}

// View implements Panel. It renders the panel name centered within the
// allocated width and height.
func (p *Placeholder) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	label := fmt.Sprintf("[ %s ]", p.PanelTitle)

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(lipgloss.Color("#666666"))

	if p.Focused {
		style = style.Foreground(lipgloss.Color("#AAAAAA"))
	}

	return style.Render(label)
}

// String implements fmt.Stringer for debugging.
func (p *Placeholder) String() string {
	var sb strings.Builder
	sb.WriteString("Placeholder(")
	sb.WriteString(p.PanelTitle)
	sb.WriteString(")")
	return sb.String()
}
