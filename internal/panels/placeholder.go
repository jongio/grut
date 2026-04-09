package panels

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/theme"
)

func colorFallback(th *theme.Theme, get func(theme.Colors) string, fallback string) color.Color {
	if th != nil {
		if v := get(th.Colors); v != "" {
			return lipgloss.Color(v)
		}
	}
	return lipgloss.Color(fallback)
}

// Placeholder is a stub panel that displays its name centered. It implements
// the Panel interface and is used as a stand-in until real panel
// implementations (filetree, preview, etc.) are built. It embeds BasePanel
// for default Focus/Blur/SetSize/Title/KeyBindings (F07).
type Placeholder struct {
	BasePanel
	theme *theme.Theme
}

// NewPlaceholder creates a new placeholder panel with the given name.
func NewPlaceholder(name string, th *theme.Theme) *Placeholder {
	return &Placeholder{
		BasePanel: BasePanel{PanelTitle: name},
		theme:     th,
	}
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

	unfocused := colorFallback(p.theme, func(c theme.Colors) string { return c.BrightBlack }, "#555555")
	focused := colorFallback(p.theme, func(c theme.Colors) string { return c.FileDefault }, "#888888")

	label := fmt.Sprintf("[ %s ]", p.PanelTitle)

	color := unfocused
	if p.Focused {
		color = focused
	}

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(color)

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
