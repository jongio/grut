// Package help implements the help overlay panel for grut.
// It displays a scrollable keybinding cheatsheet rendered as a floating
// overlay on top of the main layout by the root model.
package help

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/keybindings"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

type panelColors struct {
	Heading   string
	Key       string
	Desc      string
	Dim       string
	Separator string
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Heading:   "#C9875A",
		Key:       "#6B9E56",
		Desc:      "#D4D4D4",
		Dim:       "#555555",
		Separator: "#2A2A2A",
	}
	if th != nil {
		c.Heading = th.Colors.BrightBlue
		c.Key = th.Colors.NormalGreen
		c.Desc = th.Colors.Foreground
		c.Dim = th.Colors.BrightBlack
		c.Separator = th.Colors.SelectionBg
	}
	return c
}

// Panel is the help overlay. It implements [panels.Panel].
type Panel struct {
	lines []string // pre-rendered content lines (unstyled text)
	panels.BasePanel
	colors      panelColors
	theme       *theme.Theme
	filterMode  bool
	filterQuery string
	offset      int // scroll offset
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new help overlay panel.
func New(th *theme.Theme) *Panel {
	p := &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "help"},
		colors:    initColors(th),
		theme:     th,
	}
	p.buildLines()
	return p
}

// buildLines pre-computes the content lines for the help overlay.
// Lines are stored as plain text; styling is applied during rendering.
func (p *Panel) buildLines() {
	secs := keybindings.Sections()
	query := strings.ToLower(strings.TrimSpace(p.filterQuery))
	var lines []string
	lines = append(lines, "") // top padding

	if p.filterMode || query != "" {
		display := p.filterQuery
		if display == "" {
			display = "type to filter"
		}
		lines = append(lines, "filter:Filter: "+display)
		lines = append(lines, "")
	}

	matchedSections := 0
	for i, sec := range secs {
		sectionMatches := query == "" || strings.Contains(strings.ToLower(sec.Title), query)
		matchingBindings := sec.Bindings
		if query != "" && !sectionMatches {
			matchingBindings = nil
			for _, b := range sec.Bindings {
				haystack := strings.ToLower(b.Key + " " + b.Action)
				if strings.Contains(haystack, query) {
					matchingBindings = append(matchingBindings, b)
				}
			}
		}
		if query != "" && !sectionMatches && len(matchingBindings) == 0 {
			continue
		}
		matchedSections++
		// Section title.
		lines = append(lines, "section:"+sec.Title)
		// Separator under title.
		lines = append(lines, "sep:"+strings.Repeat("─", len(sec.Title)))
		// Bindings.
		for _, b := range matchingBindings {
			lines = append(lines, "bind:"+b.Key+"\t"+b.Action)
		}
		// Blank line between sections (except after the last).
		if i < len(secs)-1 {
			lines = append(lines, "")
		}
	}
	if matchedSections == 0 {
		lines = append(lines, "text:No matching keybindings")
	}
	lines = append(lines, "") // bottom padding
	if p.filterMode || query != "" {
		lines = append(lines, "footer:Enter keeps filter, Esc clears, ? closes")
	} else {
		lines = append(lines, "footer:Press / to filter, ? or Esc to close")
	}
	p.lines = lines
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

// View implements panels.Panel. It renders the scrollable help content
// within the given dimensions.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	headingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Heading)).
		Bold(true)
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Key)).
		Bold(true)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Desc))
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Separator))
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Dim))
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	end := p.offset + height
	if end > len(p.lines) {
		end = len(p.lines)
	}
	rendered := make([]string, 0, height)
	for i := p.offset; i < end; i++ {
		line := p.lines[i]
		var styled string
		switch {
		case strings.HasPrefix(line, "section:"):
			title := strings.TrimPrefix(line, "section:")
			styled = "  " + headingStyle.Render(title)
		case strings.HasPrefix(line, "sep:"):
			sep := strings.TrimPrefix(line, "sep:")
			styled = "  " + sepStyle.Render(sep)
		case strings.HasPrefix(line, "bind:"):
			parts := strings.SplitN(strings.TrimPrefix(line, "bind:"), "\t", 2)
			key := parts[0]
			desc := ""
			if len(parts) > 1 {
				desc = parts[1]
			}
			// Right-pad key to 12 chars for alignment.
			var padded string
			keyWidth := lipgloss.Width(key)
			if keyWidth >= 12 {
				padded = key + " "
			} else {
				padded = key + strings.Repeat(" ", 12-keyWidth)
			}
			styled = "  " + keyStyle.Render(padded) + descStyle.Render(desc)
		case strings.HasPrefix(line, "filter:"):
			text := strings.TrimPrefix(line, "filter:")
			styled = "  " + dimStyle.Render(text)
		case strings.HasPrefix(line, "text:"):
			text := strings.TrimPrefix(line, "text:")
			styled = "  " + descStyle.Render(text)
		case strings.HasPrefix(line, "footer:"):
			text := strings.TrimPrefix(line, "footer:")
			styled = "  " + dimStyle.Render(text)
		default:
			styled = emptyLine
		}
		rendered = append(rendered, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled))
	}
	// Pad remaining height with blank lines.
	for len(rendered) < height {
		rendered = append(rendered, emptyLine)
	}
	return strings.Join(rendered, "\n")
}

const keyEscape = "escape"

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
		{Key: "/", Description: "Filter shortcuts", Action: "filter"},
		{Key: "?", Description: "Close help", Action: "close"},
		{Key: keyEscape, Description: "Close help", Action: "close"},
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if p.filterMode {
		switch msg.String() {
		case "enter":
			p.filterMode = false
			p.rebuildFromFilter()
			return p, nil
		case keyEscape, "esc":
			p.clearFilter()
			return p, nil
		case "backspace":
			if r := []rune(p.filterQuery); len(r) > 0 {
				p.filterQuery = string(r[:len(r)-1])
				p.rebuildFromFilter()
			}
			return p, nil
		default:
			if msg.Text != "" {
				p.filterQuery += msg.Text
				p.rebuildFromFilter()
			}
			return p, nil
		}
	}
	switch msg.String() {
	case "j", "down":
		p.scrollDown()
	case "k", "up":
		p.scrollUp()
	case "/":
		p.filterMode = true
		p.rebuildFromFilter()
	case keyEscape, "esc":
		if p.filterQuery != "" {
			p.clearFilter()
			return p, nil
		}
		return p, func() tea.Msg { return panels.ToggleHelpMsg{} }
	case "?":
		return p, func() tea.Msg { return panels.ToggleHelpMsg{} }
	}
	return p, nil
}

func (p *Panel) rebuildFromFilter() {
	p.offset = 0
	p.buildLines()
}

func (p *Panel) clearFilter() {
	p.filterMode = false
	p.filterQuery = ""
	p.rebuildFromFilter()
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseWheel scrolls the help content viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.lines) - p.Height
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
// Scrolling
// ---------------------------------------------------------------------------
func (p *Panel) scrollDown() {
	maxOffset := len(p.lines) - p.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset < maxOffset {
		p.offset++
	}
}

func (p *Panel) scrollUp() {
	if p.offset > 0 {
		p.offset--
	}
}

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package)
// ---------------------------------------------------------------------------
// scrollOffset returns the current scroll offset.
func (p *Panel) scrollOffset() int { return p.offset }

// lineCount returns the total number of content lines.
func (p *Panel) lineCount() int { return len(p.lines) }
