// Package help implements the help overlay panel for grut.
// It displays a scrollable keybinding cheatsheet rendered as a floating
// overlay on top of the main layout by the root model.
package help

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/panels"
)

// section groups related keybindings under a heading.
type section struct {
	title    string
	bindings []binding
}

// binding is a single key→description mapping.
type binding struct {
	key  string
	desc string
}

// sections defines the static help content displayed in the overlay.
var sections = []section{
	{
		title: "Global",
		bindings: []binding{
			{key: "1-5", desc: "Focus panel by number"},
			{key: "R", desc: "Refresh all data + preview"},
			{key: "P", desc: "Push"},
			{key: "F", desc: "Fetch all remotes"},
			{key: "?", desc: "Help overlay"},
			{key: ",", desc: "Settings"},
			{key: "/", desc: "Fuzzy finder"},
			{key: ":", desc: "Command palette"},
			{key: "~", desc: "Change directory"},
			{key: "ctrl+space", desc: "Toggle AI chat"},
			{key: "ctrl+z", desc: "Undo last git action"},
			{key: "ctrl+y", desc: "Redo"},
			{key: "ctrl+c", desc: "Quit"},
		},
	},
	{
		title: "Navigation",
		bindings: []binding{
			{key: "j/k", desc: "Cursor down/up"},
			{key: "g/G", desc: "Jump to top/bottom"},
			{key: "d/u", desc: "Page down/up"},
			{key: "Enter", desc: "Select / open / expand"},
			{key: "Esc", desc: "Back / close submode"},
		},
	},
	{
		title: "File Tree",
		bindings: []binding{
			{key: "h/l", desc: "Collapse/expand directory"},
			{key: "o", desc: "Open in external editor"},
			{key: ".", desc: "Toggle hidden files"},
			{key: "g", desc: "Toggle git filter"},
			{key: "v", desc: "Toggle tree/list view"},
			{key: "n", desc: "New file"},
			{key: "N", desc: "New directory"},
			{key: "x", desc: "Delete file/directory"},
			{key: "e / F2", desc: "Rename"},
			{key: "y", desc: "Copy path to clipboard"},
			{key: "c", desc: "Copy file"},
			{key: "p", desc: "Paste file"},
			{key: "space", desc: "Toggle stage/unstage"},
			{key: "J/K", desc: "Scroll preview down/up"},
		},
	},
	{
		title: "Git Info",
		bindings: []binding{
			{key: "Tab", desc: "Next tab"},
			{key: "Shift+Tab", desc: "Previous tab"},
			{key: "b", desc: "Branches tab"},
			{key: "w", desc: "Worktrees tab"},
			{key: "r", desc: "Remotes tab"},
			{key: "s", desc: "Stash tab"},
			{key: "t", desc: "Tags tab"},
			{key: "l", desc: "Reflog tab"},
			{key: "n", desc: "Create new item"},
			{key: "x", desc: "Delete selected"},
			{key: "e / F2", desc: "Rename"},
			{key: "o", desc: "Open in browser"},
			{key: "y", desc: "Copy to clipboard"},
		},
	},
	{
		title: "GitHub",
		bindings: []binding{
			{key: "Tab", desc: "Next tab"},
			{key: "Shift+Tab", desc: "Previous tab"},
			{key: "b", desc: "Branches tab"},
			{key: "t", desc: "Tags tab"},
			{key: "i", desc: "Issues tab"},
			{key: "p", desc: "Pull Requests tab"},
			{key: "a", desc: "Actions tab"},
			{key: "w", desc: "Workflows tab"},
			{key: "l", desc: "Releases tab"},
			{key: "o", desc: "Open in browser"},
			{key: "y", desc: "Copy to clipboard"},
			{key: "r", desc: "Rerun (Actions tab)"},
			{key: "x", desc: "Cancel (Actions tab)"},
			{key: "D", desc: "Dispatch workflow"},
		},
	},
	{
		title: "Commits",
		bindings: []binding{
			{key: "Enter", desc: "View commit detail"},
			{key: "Esc", desc: "Back to list"},
			{key: "o", desc: "Open in browser"},
			{key: "y", desc: "Copy SHA"},
			{key: "A", desc: "Amend last commit"},
			{key: "r", desc: "Reword last commit"},
			{key: "/", desc: "Search commits"},
		},
	},
	{
		title: "Preview",
		bindings: []binding{
			{key: "j/k", desc: "Scroll content"},
			{key: "g/G", desc: "Jump to top/bottom"},
			{key: "d/u", desc: "Page down/up"},
		},
	},
}

// colors used by the help panel, matching the Dracula palette.
var colors = struct {
	Heading   string
	Key       string
	Desc      string
	Dim       string
	Separator string
}{
	Heading:   "#FFB86C",
	Key:       "#50FA7B",
	Desc:      "#F8F8F2",
	Dim:       "#666666",
	Separator: "#44475A",
}

// Panel is the help overlay. It implements [panels.Panel].
type Panel struct {
	panels.BasePanel
	lines  []string // pre-rendered content lines (unstyled text)
	offset int      // scroll offset
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new help overlay panel.
func New() *Panel {
	p := &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "help"},
	}
	p.buildLines()
	return p
}

// buildLines pre-computes the content lines for the help overlay.
// Lines are stored as plain text; styling is applied during rendering.
func (p *Panel) buildLines() {
	var lines []string

	lines = append(lines, "") // top padding

	for i, sec := range sections {
		// Section title.
		lines = append(lines, "section:"+sec.title)

		// Separator under title.
		lines = append(lines, "sep:"+strings.Repeat("─", len(sec.title)))

		// Bindings.
		for _, b := range sec.bindings {
			lines = append(lines, "bind:"+b.key+"\t"+b.desc)
		}

		// Blank line between sections (except after the last).
		if i < len(sections)-1 {
			lines = append(lines, "")
		}
	}

	lines = append(lines, "") // bottom padding
	lines = append(lines, "footer:Press ? or Esc to close")

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
		Foreground(lipgloss.Color(colors.Heading)).
		Bold(true)
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Key)).
		Bold(true)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Desc))
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Separator))
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Dim))
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
			padded := key + strings.Repeat(" ", 12-len(key))
			if len(key) >= 12 {
				padded = key + " "
			}
			styled = "  " + keyStyle.Render(padded) + descStyle.Render(desc)

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

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
		{Key: "?", Description: "Close help", Action: "close"},
		{Key: "escape", Description: "Close help", Action: "close"},
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		p.scrollDown()
	case "k", "up":
		p.scrollUp()
	case "escape", "esc", "?":
		return p, func() tea.Msg { return panels.ToggleHelpMsg{} }
	}
	return p, nil
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
