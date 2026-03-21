// Package terminal implements the embedded terminal panel for grut.
// It provides a scrollable view of shell output with two modes:
// Normal mode for scrolling and Insert mode for typing commands.
package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	term "github.com/jongio/grut/internal/terminal"
)

// mode represents the terminal panel's input mode.
type mode int

const (
	modeNormal mode = iota
	modeInsert
)

// tickMsg drives the render-throttle timer (~30fps).
type tickMsg struct {
	time time.Time
}

// Panel is the embedded terminal panel. It implements [panels.Panel] and
// [panels.Closer]. The panel wraps a [term.Runner] and provides two modes:
//
//   - Normal mode: j/k to scroll output, i/Enter to switch to insert mode.
//   - Insert mode: keystrokes are collected and sent to the shell on Enter.
//     The configured prefix key (default ctrl+b) exits insert mode.
type Panel struct {
	runner term.Runner
	ctx    context.Context
	cfg    config.TerminalConfig
	shell  string   // display name for status bar
	input  []rune   // input buffer in insert mode
	lines  []string // latest snapshot of output lines
	panels.BasePanel
	mode    mode
	offset  int  // scroll offset from bottom (0 = latest)
	ticking bool // whether the tick timer is active
}

// Compile-time interface checks.
var (
	_ panels.Panel  = (*Panel)(nil)
	_ panels.Closer = (*Panel)(nil)
)

// New creates a new terminal panel with the given config and backend runner.
// If runner is nil, the panel displays a placeholder until a runner is set.
func New(cfg config.TerminalConfig, runner term.Runner, shell string) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "terminal"},
		runner:    runner,
		cfg:       cfg,
		shell:     shell,
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	if p.runner == nil {
		return nil
	}
	return p.scheduleTick()
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return p.handleTick()
	case tea.KeyPressMsg:
		if !p.Focused {
			return p, nil
		}
		return p.handleKey(msg)
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.runner == nil {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No terminal")
	}
	// Reserve space for status bar and (optionally) input prompt.
	statusHeight := 1
	inputHeight := 0
	if p.mode == modeInsert {
		inputHeight = 1
	}
	contentHeight := height - statusHeight - inputHeight
	if contentHeight < 0 {
		contentHeight = 0
	}
	var sections []string
	// --- Output content ---
	sections = append(sections, p.renderContent(width, contentHeight))
	// --- Input prompt (insert mode only) ---
	if inputHeight > 0 {
		sections = append(sections, p.renderInput(width))
	}
	// --- Status bar ---
	sections = append(sections, p.renderStatus(width))
	return strings.Join(sections, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	prefixKey := p.cfg.PrefixKey
	if prefixKey == "" {
		prefixKey = "ctrl+b"
	}
	return []panels.KeyBinding{
		{Key: "i/enter", Description: "Enter insert mode", Action: "insert_mode"},
		{Key: prefixKey, Description: "Exit insert mode", Action: "normal_mode"},
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
		{Key: "G", Description: "Scroll to bottom", Action: "scroll_bottom"},
		{Key: "g", Description: "Scroll to top", Action: "scroll_top"},
	}
}

// Close implements panels.Closer. Kills the shell process on shutdown.
func (p *Panel) Close() {
	if p.runner != nil {
		_ = p.runner.Close()
	}
}

// Mode returns the current input mode. Exported for testing.
func (p *Panel) Mode() mode {
	return p.mode
}

// Input returns the current input buffer contents. Exported for testing.
func (p *Panel) Input() string {
	return string(p.input)
}

// Offset returns the current scroll offset. Exported for testing.
func (p *Panel) Offset() int {
	return p.offset
}

// ---------------------------------------------------------------------------
// Tick handling
// ---------------------------------------------------------------------------
func (p *Panel) scheduleTick() tea.Cmd {
	p.ticking = true
	fps := p.cfg.RenderFPS
	if fps <= 0 {
		fps = 30
	}
	interval := time.Second / time.Duration(fps)
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg{time: t}
	})
}

func (p *Panel) handleTick() (panels.Panel, tea.Cmd) {
	if p.runner == nil {
		p.ticking = false
		return p, nil
	}
	// Refresh the line snapshot from the runner.
	p.lines = p.runner.Lines()
	// Check if the process has exited.
	select {
	case <-p.runner.Done():
		p.ticking = false
		exitCode := p.runner.ExitCode()
		return p, func() tea.Msg {
			return panels.TerminalExitedMsg{ExitCode: exitCode}
		}
	default:
	}
	// Schedule next tick.
	return p, p.scheduleTick()
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch p.mode {
	case modeNormal:
		return p.handleNormalKey(msg)
	case modeInsert:
		return p.handleInsertKey(msg)
	}
	return p, nil
}

func (p *Panel) handleNormalKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "i", "enter":
		p.mode = modeInsert
		p.offset = 0 // scroll to bottom when entering insert mode
	case "j", "down":
		p.scrollDown()
	case "k", "up":
		p.scrollUp()
	case "G":
		p.offset = 0 // scroll to bottom
	case "g":
		p.scrollToTop()
	}
	return p, nil
}

func (p *Panel) handleInsertKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	key := msg.String()
	// Check prefix key to exit insert mode.
	prefixKey := p.cfg.PrefixKey
	if prefixKey == "" {
		prefixKey = "ctrl+b"
	}
	if key == prefixKey {
		p.mode = modeNormal
		return p, nil
	}
	if p.runner == nil {
		return p, nil
	}
	switch key {
	case "enter":
		// Send the accumulated input + newline to the shell.
		line := string(p.input) + "\n"
		p.input = nil
		if err := p.runner.Write([]byte(line)); err != nil {
			// Write failed (process likely exited); next tick will detect exit.
			return p, nil
		}
	case "backspace":
		if len(p.input) > 0 {
			p.input = p.input[:len(p.input)-1]
		}
	case "tab":
		p.input = append(p.input, '\t')
	case "space":
		p.input = append(p.input, ' ')
	default:
		// Append printable characters to the input buffer.
		runes := []rune(key)
		if len(runes) == 1 && runes[0] >= 32 {
			p.input = append(p.input, runes[0])
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Scroll logic
// ---------------------------------------------------------------------------
func (p *Panel) scrollUp() {
	maxOffset := p.maxOffset()
	if p.offset < maxOffset {
		p.offset++
	}
}

func (p *Panel) scrollDown() {
	if p.offset > 0 {
		p.offset--
	}
}

func (p *Panel) scrollToTop() {
	p.offset = p.maxOffset()
}

func (p *Panel) maxOffset() int {
	contentHeight := p.contentHeight()
	if contentHeight <= 0 || len(p.lines) <= contentHeight {
		return 0
	}
	return len(p.lines) - contentHeight
}

func (p *Panel) contentHeight() int {
	h := p.Height
	statusHeight := 1
	inputHeight := 0
	if p.mode == modeInsert {
		inputHeight = 1
	}
	ch := h - statusHeight - inputHeight
	if ch < 0 {
		return 0
	}
	return ch
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (p *Panel) renderContent(width, contentHeight int) string {
	if contentHeight <= 0 {
		return ""
	}
	totalLines := len(p.lines)
	if totalLines == 0 {
		// Show empty area.
		emptyLine := lipgloss.NewStyle().Width(width).Render("")
		empty := make([]string, contentHeight)
		for i := range empty {
			empty[i] = emptyLine
		}
		return strings.Join(empty, "\n")
	}
	// Calculate the visible window. offset=0 means latest lines visible.
	end := totalLines - p.offset
	if end < 0 {
		end = 0
	}
	start := end - contentHeight
	if start < 0 {
		start = 0
	}
	visible := p.lines[start:end]
	lineStyle := lipgloss.NewStyle().Width(width)
	rendered := make([]string, 0, contentHeight)
	for _, line := range visible {
		// Truncate long lines.
		if len(line) > width && width > 3 {
			line = line[:width-3] + "..."
		}
		rendered = append(rendered, lineStyle.Render(line))
	}
	// Pad with empty lines if fewer lines than height.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(rendered) < contentHeight {
		rendered = append([]string{emptyLine}, rendered...)
	}
	return strings.Join(rendered, "\n")
}

func (p *Panel) renderInput(width int) string {
	prompt := "> " + string(p.input) + "█"
	if len(prompt) > width && width > 3 {
		prompt = prompt[:width-3] + "..."
	}
	return lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("#F8F8F2")).
		Render(prompt)
}

func (p *Panel) renderStatus(width int) string {
	// Mode indicator.
	modeStr := "NORMAL"
	if p.mode == modeInsert {
		modeStr = "INSERT"
	}
	// Shell name.
	shellName := p.shell
	if shellName == "" {
		shellName = "shell"
	}
	// Line count.
	lineCount := len(p.lines)
	// Process status.
	processStatus := ""
	if p.runner != nil {
		select {
		case <-p.runner.Done():
			processStatus = fmt.Sprintf(" exit=%d", p.runner.ExitCode())
		default:
		}
	}
	status := fmt.Sprintf(" %s │ %s │ %d lines%s", shellName, modeStr, lineCount, processStatus)
	// Truncate if needed.
	if len(status) > width && width > 3 {
		status = status[:width-3] + "..."
	}
	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color("#44475A")).
		Foreground(lipgloss.Color("#F8F8F2")).
		Render(status)
}
