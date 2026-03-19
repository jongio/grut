// Package welcome implements the first-run welcome overlay panel for grut.
// It displays a curated introduction with the grut banner, essential keyboard
// shortcuts, and OK / Don't Show Again controls.
package welcome

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/panels"
)

// DismissMsg is sent when the user dismisses the welcome screen.
// DontShowAgain indicates whether to persist the choice in config.
type DismissMsg struct {
	DontShowAgain bool
}

// colors used by the welcome panel, matching the Dracula palette.
var colors = struct {
	Banner    string
	Heading   string
	Key       string
	Desc      string
	Accent    string
	Dim       string
	Separator string
	Star      string
}{
	Banner:    "#BD93F9",
	Heading:   "#FFB86C",
	Key:       "#50FA7B",
	Desc:      "#F8F8F2",
	Accent:    "#FF79C6",
	Dim:       "#666666",
	Separator: "#44475A",
	Star:      "#F1FA8C",
}

// Panel is the welcome overlay. It implements [panels.Panel].
type Panel struct {
	panels.BasePanel
	lines  []string // pre-rendered content lines
	offset int      // scroll offset
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new welcome overlay panel.
func New() *Panel {
	p := &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "welcome"},
	}
	p.buildLines()
	return p
}

// buildLines pre-computes the content lines for the welcome overlay.
func (p *Panel) buildLines() {
	var lines []string

	// Banner.
	lines = append(lines, "")
	lines = append(lines, "banner:   ┏━━━┓ ┏━━━┓ ┏━┓ ┏━┓ ┏━━━━┓")
	lines = append(lines, "banner:   ┃┏━┓┃ ┃┏━┓┃ ┃ ┃ ┃ ┃ ┃┏━━━┛")
	lines = append(lines, "banner:   ┃┃ ┗┛ ┃┗━┛┃ ┃ ┃ ┃ ┃ ┃┗━━━┓")
	lines = append(lines, "banner:   ┃┃┏━┓ ┃┏┓┏┛ ┃ ┃ ┃ ┃ ┃┏━━━┛")
	lines = append(lines, "banner:   ┃┗┻━┃ ┃┃┃┗┓ ┃ ┗━┛ ┃ ┃┃    ")
	lines = append(lines, "banner:   ┗━━━┛ ┗┛┗━┛ ┗━━━━━┛ ┗┛    ")
	lines = append(lines, "")
	lines = append(lines, "subtitle:Git Review Utility for Terminals")
	lines = append(lines, "")

	// Key feature callout.
	lines = append(lines, "section:★ Key Feature")
	lines = append(lines, "sep:"+strings.Repeat("─", 13))
	lines = append(lines, "accent:  g            Filter file tree to git-changed files")
	lines = append(lines, "accent:               Your fastest path to what matters")
	lines = append(lines, "")

	// Navigation.
	lines = append(lines, "section:Navigation")
	lines = append(lines, "sep:"+strings.Repeat("─", 10))
	lines = append(lines, "bind:Tab/S-Tab\tSwitch panels")
	lines = append(lines, "bind:j/k or ↑/↓\tNavigate items")
	lines = append(lines, "bind:Enter\tOpen / expand")
	lines = append(lines, "bind:?\tFull keyboard reference")
	lines = append(lines, "bind:W\tShow this welcome screen")
	lines = append(lines, "")

	// File tree.
	lines = append(lines, "section:File Tree")
	lines = append(lines, "sep:"+strings.Repeat("─", 9))
	lines = append(lines, "bind:space\tStage / unstage file")
	lines = append(lines, "bind:.\tToggle hidden files")
	lines = append(lines, "bind:o\tOpen in external editor")
	lines = append(lines, "")

	// Git.
	lines = append(lines, "section:Git")
	lines = append(lines, "sep:"+strings.Repeat("─", 3))
	lines = append(lines, "bind:P\tPush to remote")
	lines = append(lines, "bind:F\tFetch all remotes")
	lines = append(lines, "bind:ctrl+z\tUndo last git action")
	lines = append(lines, "")

	// Footer.
	lines = append(lines, "footer:[Enter] OK    [d] Don't Show Again    [?] Full Help    [W] Reopen Anytime")

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

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bannerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Banner)).
		Bold(true)
	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Desc)).
		Italic(true)
	headingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Heading)).
		Bold(true)
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Key)).
		Bold(true)
	accentKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Accent)).
		Bold(true)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Desc))
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Separator))
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Dim))
	starStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Star)).
		Bold(true)
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
		case strings.HasPrefix(line, "banner:"):
			text := strings.TrimPrefix(line, "banner:")
			styled = bannerStyle.Render(text)

		case strings.HasPrefix(line, "subtitle:"):
			text := strings.TrimPrefix(line, "subtitle:")
			// Center the subtitle.
			pad := (width - len(text)) / 2
			if pad < 0 {
				pad = 0
			}
			styled = strings.Repeat(" ", pad) + subtitleStyle.Render(text)

		case strings.HasPrefix(line, "section:"):
			title := strings.TrimPrefix(line, "section:")
			// Render star separately if title starts with ★.
			if strings.HasPrefix(title, "★") {
				styled = "  " + starStyle.Render("★") + headingStyle.Render(title[len("★"):])
			} else {
				styled = "  " + headingStyle.Render(title)
			}

		case strings.HasPrefix(line, "sep:"):
			sep := strings.TrimPrefix(line, "sep:")
			styled = "  " + sepStyle.Render(sep)

		case strings.HasPrefix(line, "accent:"):
			// Special highlighted binding for the key feature.
			text := strings.TrimPrefix(line, "accent:")
			if len(text) > 2 {
				// First 14 chars are the key column (with padding).
				keyEnd := 14
				if keyEnd > len(text) {
					keyEnd = len(text)
				}
				styled = "  " + accentKeyStyle.Render(text[:keyEnd]) + descStyle.Render(text[keyEnd:])
			} else {
				styled = "  " + accentKeyStyle.Render(text)
			}

		case strings.HasPrefix(line, "bind:"):
			parts := strings.SplitN(strings.TrimPrefix(line, "bind:"), "\t", 2)
			key := parts[0]
			desc := ""
			if len(parts) > 1 {
				desc = parts[1]
			}
			padded := key + strings.Repeat(" ", 14-len(key))
			if len(key) >= 14 {
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
		{Key: "Enter", Description: "OK (dismiss)", Action: "dismiss"},
		{Key: "d", Description: "Don't show again", Action: "dont_show_again"},
		{Key: "Esc", Description: "Close", Action: "dismiss"},
		{Key: "?", Description: "Open full help", Action: "open_help"},
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return p, func() tea.Msg { return DismissMsg{DontShowAgain: false} }
	case "d":
		return p, func() tea.Msg { return DismissMsg{DontShowAgain: true} }
	case "escape", "esc":
		return p, func() tea.Msg { return DismissMsg{DontShowAgain: false} }
	case "?":
		// Dismiss welcome and open help.
		return p, tea.Batch(
			func() tea.Msg { return DismissMsg{DontShowAgain: false} },
			func() tea.Msg { return panels.ToggleHelpMsg{} },
		)
	case "j", "down":
		p.scrollDown()
	case "k", "up":
		p.scrollUp()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------

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
