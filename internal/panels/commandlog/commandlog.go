// Package commandlog implements the read-only git command log overlay.
package commandlog

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

const (
	successSymbol = "✓"
	failureSymbol = "✗"
)

type panelColors struct {
	Success string
	Failure string
	Text    string
	Dim     string
}

// Panel renders the global git command log as a scrollable read-only overlay.
type Panel struct {
	panels.BasePanel
	log     *git.CommandLog
	colors  panelColors
	entries []git.CommandEntry
	offset  int
}

var _ panels.Panel = (*Panel)(nil)

// New creates a command log overlay panel backed by the global git command log.
func New(th *theme.Theme) *Panel {
	return NewWithLog(git.GlobalCommandLog(), th)
}

// NewWithLog creates a command log overlay panel backed by log.
func NewWithLog(log *git.CommandLog, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "git command log"},
		log:       log,
		colors:    initColors(th),
	}
}

func initColors(th *theme.Theme) panelColors {
	colors := panelColors{
		Success: "#50FA7B",
		Failure: "#FF5555",
		Text:    "#F8F8F2",
		Dim:     "#6272A4",
	}
	if th != nil {
		colors.Success = th.Colors.NormalGreen
		colors.Failure = th.Colors.NormalRed
		colors.Text = th.Colors.Foreground
		colors.Dim = th.Colors.BrightBlack
	}
	return colors
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
	case tea.MouseWheelMsg:
		p.handleMouseWheel(msg)
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	p.refresh()
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Text))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Success)).Bold(true)
	failureStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Failure)).Bold(true)
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	if len(p.entries) == 0 {
		return p.renderLines(width, height, []string{
			"",
			textStyle.Render("  No git commands recorded yet"),
			"",
			dimStyle.Render("  Run a git action in grut to populate this log."),
		})
	}

	lines := make([]string, 0, len(p.entries)*2)
	for _, entry := range p.entries {
		statusStyle := successStyle
		status := successSymbol
		if !entry.Success {
			statusStyle = failureStyle
			status = failureSymbol
		}
		lines = append(lines, formatEntry(entry, statusStyle.Render(status), textStyle, dimStyle))
		if !entry.Success && entry.ErrSummary != "" {
			lines = append(lines, "  "+failureStyle.Render(entry.ErrSummary))
		}
	}
	p.clampOffset(height, len(lines))
	end := p.offset + height
	if end > len(lines) {
		end = len(lines)
	}
	rendered := make([]string, 0, height)
	for i := p.offset; i < end; i++ {
		rendered = append(rendered, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(lines[i]))
	}
	for len(rendered) < height {
		rendered = append(rendered, emptyLine)
	}
	return strings.Join(rendered, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
		{Key: "pgdn", Description: "Page down", Action: "page_down"},
		{Key: "pgup", Description: "Page up", Action: "page_up"},
		{Key: "home", Description: "Jump to top", Action: "top"},
		{Key: "end", Description: "Jump to bottom", Action: "bottom"},
		{Key: "esc", Description: "Close command log", Action: "close"},
	}
}

func formatEntry(entry git.CommandEntry, status string, textStyle, dimStyle lipgloss.Style) string {
	timestamp := entry.Timestamp.Format("15:04:05")
	command := "git " + strings.Join(entry.Args, " ")
	duration := formatDuration(entry.Duration)
	return fmt.Sprintf(
		"  %s %s %s %s %s",
		dimStyle.Render(timestamp),
		status,
		textStyle.Render(command),
		dimStyle.Render("("+duration+")"),
		dimStyle.Render(entry.Dir),
	)
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return d.String()
	}
	return d.Round(time.Millisecond).String()
}

func (p *Panel) renderLines(width, height int, lines []string) string {
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	rendered := make([]string, 0, height)
	for i := 0; i < len(lines) && i < height; i++ {
		rendered = append(rendered, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(lines[i]))
	}
	for len(rendered) < height {
		rendered = append(rendered, emptyLine)
	}
	return strings.Join(rendered, "\n")
}

func (p *Panel) refresh() {
	if p.log == nil {
		p.entries = nil
		return
	}
	p.entries = p.log.Entries()
}

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		p.scroll(1)
	case "k", "up":
		p.scroll(-1)
	case "pgdown", "pagedown":
		p.scroll(p.Height)
	case "pgup", "pageup":
		p.scroll(-p.Height)
	case "home":
		p.offset = 0
	case "end":
		p.offset = p.maxOffset()
	case "esc", "escape":
		return p, func() tea.Msg { return panels.ToggleCommandLogMsg{} }
	}
	return p, nil
}

func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		p.scroll(-panels.ScrollDelta)
	case tea.MouseWheelDown:
		p.scroll(panels.ScrollDelta)
	}
}

func (p *Panel) scroll(delta int) {
	p.offset += delta
	p.clampOffset(p.Height, p.lineCount())
}

func (p *Panel) clampOffset(height, lineCount int) {
	maxOffset := lineCount - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset < 0 {
		p.offset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}

func (p *Panel) maxOffset() int {
	maxOffset := p.lineCount() - p.Height
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (p *Panel) lineCount() int {
	count := len(p.entries)
	for _, entry := range p.entries {
		if !entry.Success && entry.ErrSummary != "" {
			count++
		}
	}
	return count
}

func (p *Panel) scrollOffset() int {
	return p.offset
}
