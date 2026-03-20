// Package welcome implements the first-run welcome overlay panel for grut.
// It displays a curated introduction with the grut banner, essential keyboard
// shortcuts, and OK / Don't Show Again controls. The banner animates in
// line-by-line with a brief accent flash on each new line.
package welcome

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/panels"
)

// DismissMsg is sent when the user dismisses the welcome screen.
// The first-run marker is always persisted; users can re-show with W.
type DismissMsg struct{}

// AnimTickMsg advances the welcome banner animation by one frame.
type AnimTickMsg time.Time

// animInterval is the delay between animation frames.
const animInterval = 50 * time.Millisecond

// keyColumnWidth is the display-width allocated for the keybinding column.
const keyColumnWidth = 14

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

// Button indices for the footer button bar.
const (
	btnOK    = 0
	btnHelp  = 1
	btnCount = 2
)

// buttonLabels are the display labels for each footer button.
var buttonLabels = [btnCount]string{"OK", "Help"}

// Panel is the welcome overlay. It implements [panels.Panel].
type Panel struct {
	panels.BasePanel
	lines       []string // pre-rendered content lines
	offset      int      // scroll offset
	animFrame   int      // current animation frame (lines revealed so far)
	animDone    bool     // true after animation completes
	headerCount int      // number of lines in the banner/subtitle header area
	focusedBtn  int      // which footer button is focused (0=OK, 1=Help)

	// Cached styles to avoid per-frame allocations.
	bannerStyle    lipgloss.Style
	flashStyle     lipgloss.Style
	subtitleStyle  lipgloss.Style
	headingStyle   lipgloss.Style
	keyStyle       lipgloss.Style
	accentKeyStyle lipgloss.Style
	descStyle      lipgloss.Style
	sepStyle       lipgloss.Style
	starStyle      lipgloss.Style
	focusedBtnSty  lipgloss.Style
	normalBtnSty   lipgloss.Style
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new welcome overlay panel.
func New() *Panel {
	p := &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "welcome"},
	}
	p.initStyles()
	p.buildLines()
	return p
}

// initStyles initializes cached lipgloss styles to avoid per-frame allocations.
func (p *Panel) initStyles() {
	p.bannerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Banner)).
		Bold(true)
	p.flashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Accent)).
		Bold(true)
	p.subtitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Desc)).
		Italic(true)
	p.headingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Heading)).
		Bold(true)
	p.keyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Key)).
		Bold(true)
	p.accentKeyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Accent)).
		Bold(true)
	p.descStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Desc))
	p.sepStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Separator))
	p.starStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Star)).
		Bold(true)
	p.focusedBtnSty = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#282A36")).
		Background(lipgloss.Color(colors.Accent)).
		Padding(0, 1)
	p.normalBtnSty = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Dim)).
		Padding(0, 1)
}

// buildLines pre-computes the content lines for the welcome overlay.
func (p *Panel) buildLines() {
	var lines []string

	// Lowercase banner with prominent umlaut dots.
	// All banner lines are padded to 16 display-columns for consistent centering.
	lines = append(lines, "")
	lines = append(lines, "banner:        ●  ●    ")
	lines = append(lines, "banner:╭──╮ ╭─ │  │ ─┬─")
	lines = append(lines, "banner:│  │ │  │  │  │ ")
	lines = append(lines, "banner:╰──┤ │  ╰──╯  ╰─")
	lines = append(lines, "banner:╰──╯            ")
	lines = append(lines, "")
	lines = append(lines, "subtitle:file explorer with git, github, and ai integration")
	lines = append(lines, "")

	// Count header lines (banner + subtitle area).
	p.headerCount = len(lines)

	// Panel Focus.
	lines = append(lines, "section:Panel Focus")
	lines = append(lines, "sep:"+strings.Repeat("─", 11))
	lines = append(lines, "bind:1\tFile Tree")
	lines = append(lines, "bind:2\tGit Info")
	lines = append(lines, "bind:3\tGitHub")
	lines = append(lines, "bind:4\tCommits")
	lines = append(lines, "bind:5\tPreview")
	lines = append(lines, "")

	// Navigation.
	lines = append(lines, "section:Navigation")
	lines = append(lines, "sep:"+strings.Repeat("─", 10))
	lines = append(lines, "bind:j/k or ↑/↓\tNavigate items")
	lines = append(lines, "bind:Enter\tOpen / expand")
	lines = append(lines, "bind:Tab/S-Tab\tCycle panel tabs")
	lines = append(lines, "accent:g\tToggle git filter (File Tree)")
	lines = append(lines, "")

	// Commands.
	lines = append(lines, "section:Commands")
	lines = append(lines, "sep:"+strings.Repeat("─", 8))
	lines = append(lines, "bind:/\tFuzzy finder")
	lines = append(lines, "bind::\tCommand palette")
	lines = append(lines, "bind:?\tFull keyboard reference")
	lines = append(lines, "")

	// Git.
	lines = append(lines, "section:Git")
	lines = append(lines, "sep:"+strings.Repeat("─", 3))
	lines = append(lines, "bind:space\tStage / unstage file")
	lines = append(lines, "bind:P\tPush to remote")
	lines = append(lines, "bind:F\tFetch all remotes")
	lines = append(lines, "bind:ctrl+z\tUndo last git action")

	p.lines = lines
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------

// Init implements panels.Panel. It starts the banner reveal animation.
func (p *Panel) Init(_ context.Context) tea.Cmd {
	return p.nextAnimTick()
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case AnimTickMsg:
		return p.handleAnimTick()
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

	// Reserve the bottom line for the button bar.
	contentHeight := height - 1
	if contentHeight <= 0 {
		contentHeight = 0
	}
	// Cap to actual content size to avoid runaway allocation.
	if maxCH := len(p.lines) + 1; contentHeight > maxCH {
		contentHeight = maxCH
	}

	emptyLine := lipgloss.NewStyle().Width(width).Render("")

	// During animation, limit how many lines are visible.
	visibleCount := len(p.lines)
	if !p.animDone && p.animFrame <= p.headerCount {
		visibleCount = p.animFrame
	}

	end := p.offset + contentHeight
	if end > len(p.lines) {
		end = len(p.lines)
	}

	rendered := make([]string, 0, height)

	for i := p.offset; i < end; i++ {
		// Lines beyond the animation reveal are blank.
		if i >= visibleCount {
			rendered = append(rendered, emptyLine)
			continue
		}

		line := p.lines[i]
		var styled string

		switch {
		case strings.HasPrefix(line, "banner:"):
			text := strings.TrimPrefix(line, "banner:")
			// Center the banner.
			textWidth := lipgloss.Width(text)
			pad := (width - textWidth) / 2
			if pad < 0 {
				pad = 0
			}
			// Flash the newest revealed line during animation.
			if !p.animDone && i == p.animFrame-1 {
				styled = strings.Repeat(" ", pad) + p.flashStyle.Render(text)
			} else {
				styled = strings.Repeat(" ", pad) + p.bannerStyle.Render(text)
			}

		case strings.HasPrefix(line, "subtitle:"):
			text := strings.TrimPrefix(line, "subtitle:")
			pad := (width - lipgloss.Width(text)) / 2
			if pad < 0 {
				pad = 0
			}
			// Flash the subtitle when it first appears.
			if !p.animDone && i == p.animFrame-1 {
				styled = strings.Repeat(" ", pad) + p.flashStyle.Render(text)
			} else {
				styled = strings.Repeat(" ", pad) + p.subtitleStyle.Render(text)
			}

		case strings.HasPrefix(line, "section:"):
			title := strings.TrimPrefix(line, "section:")
			if strings.HasPrefix(title, "★") {
				styled = "  " + p.starStyle.Render("★") + p.headingStyle.Render(title[len("★"):])
			} else {
				styled = "  " + p.headingStyle.Render(title)
			}

		case strings.HasPrefix(line, "sep:"):
			sep := strings.TrimPrefix(line, "sep:")
			styled = "  " + p.sepStyle.Render(sep)

		case strings.HasPrefix(line, "accent:"):
			text := strings.TrimPrefix(line, "accent:")
			parts := strings.SplitN(text, "\t", 2)
			if len(parts) == 2 {
				key := parts[0]
				desc := parts[1]
				displayW := lipgloss.Width(key)
				padN := keyColumnWidth - displayW
				if padN < 1 {
					padN = 1
				}
				styled = "  " + p.accentKeyStyle.Render(key+strings.Repeat(" ", padN)) + p.descStyle.Render(desc)
			} else {
				styled = "  " + p.accentKeyStyle.Render(text)
			}

		case strings.HasPrefix(line, "bind:"):
			parts := strings.SplitN(strings.TrimPrefix(line, "bind:"), "\t", 2)
			key := parts[0]
			desc := ""
			if len(parts) > 1 {
				desc = parts[1]
			}
			displayW := lipgloss.Width(key)
			padN := keyColumnWidth - displayW
			if padN < 1 {
				padN = 1
			}
			padded := key + strings.Repeat(" ", padN)
			styled = "  " + p.keyStyle.Render(padded) + p.descStyle.Render(desc)

		default:
			styled = emptyLine
		}

		rendered = append(rendered, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled))
	}

	// Pad remaining content height with blank lines.
	for len(rendered) < contentHeight {
		rendered = append(rendered, emptyLine)
	}

	// Skip button bar if there's no room.
	if contentHeight <= 0 {
		return strings.Join(rendered, "\n")
	}

	// Render the button bar (right-aligned on the last line).
	var btns []string
	for i := 0; i < btnCount; i++ {
		if i == p.focusedBtn {
			btns = append(btns, p.focusedBtnSty.Render(buttonLabels[i]))
		} else {
			btns = append(btns, p.normalBtnSty.Render(buttonLabels[i]))
		}
	}
	buttonBar := strings.Join(btns, " ")
	barWidth := lipgloss.Width(buttonBar)
	pad := width - barWidth
	if pad < 0 {
		pad = 0
	}
	buttonLine := strings.Repeat(" ", pad) + buttonBar
	rendered = append(rendered, lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(buttonLine))

	return strings.Join(rendered, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "Tab/S-Tab", Description: "Cycle buttons", Action: "cycle_buttons"},
		{Key: "Enter", Description: "Activate button", Action: "activate"},
		{Key: "Esc", Description: "Close", Action: "dismiss"},
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
	}
}

// ---------------------------------------------------------------------------
// Animation
// ---------------------------------------------------------------------------

// nextAnimTick returns a Cmd that fires an AnimTickMsg after the interval.
func (p *Panel) nextAnimTick() tea.Cmd {
	return tea.Tick(animInterval, func(t time.Time) tea.Msg {
		return AnimTickMsg(t)
	})
}

// handleAnimTick advances the animation by one frame.
func (p *Panel) handleAnimTick() (panels.Panel, tea.Cmd) {
	if p.animDone {
		return p, nil
	}
	p.animFrame++
	if p.animFrame > p.headerCount {
		p.animDone = true
		return p, nil
	}
	return p, p.nextAnimTick()
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	// Any key press during animation skips to the end.
	if !p.animDone {
		p.animDone = true
		return p, nil
	}

	switch msg.String() {
	case "tab":
		p.focusedBtn = (p.focusedBtn + 1) % btnCount
	case "shift+tab":
		p.focusedBtn = (p.focusedBtn + btnCount - 1) % btnCount
	case "enter":
		return p.activateButton()
	case "escape", "esc":
		return p, func() tea.Msg { return DismissMsg{} }
	case "?":
		return p, tea.Batch(
			func() tea.Msg { return DismissMsg{} },
			func() tea.Msg { return panels.ToggleHelpMsg{} },
		)
	case "j", "down":
		p.scrollDown()
	case "k", "up":
		p.scrollUp()
	}
	return p, nil
}

// activateButton fires the action for the currently focused button.
func (p *Panel) activateButton() (panels.Panel, tea.Cmd) {
	switch p.focusedBtn {
	case btnOK:
		return p, func() tea.Msg { return DismissMsg{} }
	case btnHelp:
		return p, tea.Batch(
			func() tea.Msg { return DismissMsg{} },
			func() tea.Msg { return panels.ToggleHelpMsg{} },
		)
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
		p.offset += panels.ScrollDelta
		p.clampOffset()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Scrolling
// ---------------------------------------------------------------------------

// SetSize overrides BasePanel to reclamp the scroll offset when the
// terminal shrinks or grows so content doesn't appear past the end.
func (p *Panel) SetSize(width, height int) {
	p.BasePanel.SetSize(width, height)
	p.clampOffset()
}

func (p *Panel) clampOffset() {
	maxOffset := len(p.lines) - p.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}

func (p *Panel) scrollDown() {
	p.offset++
	p.clampOffset()
}

func (p *Panel) scrollUp() {
	if p.offset > 0 {
		p.offset--
	}
}
