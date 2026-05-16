// Package agents implements the Agent Monitor panel for grut.
// It displays spawned agent processes with their status, output,
// and provides controls to kill agents and refresh the list.
package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
)

// ---------------------------------------------------------------------------
// Internal message types (async result messages)
// ---------------------------------------------------------------------------
// agentsLoadedMsg carries the result of an async agent list refresh.
type agentsLoadedMsg struct {
	agents []mcp.AgentInfo
}

// agentKilledMsg carries the result of a kill operation.
type agentKilledMsg struct {
	err error
	pid int
}

// agentOutputLoadedMsg carries captured output for an expanded agent.
type agentOutputLoadedMsg struct {
	stdout []string
	stderr []string
	pid    int
}

// tickMsg drives the auto-refresh timer while the panel is focused.
type tickMsg struct {
	time time.Time
}

// ---------------------------------------------------------------------------
// Tracker interface (for testability)
// ---------------------------------------------------------------------------
// Tracker is the subset of mcp.AgentTracker used by this panel.
type Tracker interface {
	List() []mcp.AgentInfo
	Kill(pid int) error
	KillAll()
	Output(pid int) (stdout, stderr []string)
}

// ---------------------------------------------------------------------------
// Agents panel
// ---------------------------------------------------------------------------
const opRightClickPick = "right_click_pick"

// Agents is the agent monitor panel. It implements [panels.Panel] and
// [panels.Closer].
type Agents struct {
	actionsCfg config.ActionsConfig
	tracker    Tracker
	theme      *theme.Theme
	ctx        context.Context
	err        error        // last error
	expanded   map[int]bool // PIDs with output expanded
	// Cached output for expanded agents.
	outputCache map[int]agentOutput
	pendingOp   string
	agents      []mcp.AgentInfo // latest snapshot
	panels.BasePanel
	cursor  int  // index into agents list
	offset  int  // viewport scroll offset
	loading bool // true while a refresh is in flight
}
type agentOutput struct {
	stdout []string
	stderr []string
}

// Compile-time interface checks.
var (
	_ panels.Panel  = (*Agents)(nil)
	_ panels.Closer = (*Agents)(nil)
)

// New creates a new Agents panel with the given tracker.
func New(tracker Tracker, th *theme.Theme) *Agents {
	return &Agents{
		BasePanel:   panels.BasePanel{PanelTitle: "agents"},
		tracker:     tracker,
		expanded:    make(map[int]bool),
		outputCache: make(map[int]agentOutput),
		theme:       th,
	}
}

func (p *Agents) themeColors() theme.Colors {
	if p.theme != nil {
		return p.theme.Colors
	}
	return theme.Colors{}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Agents) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	p.loading = true
	return p.loadAgentsCmd()
}

// Update implements panels.Panel.
func (p *Agents) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case agentsLoadedMsg:
		p.loading = false
		p.agents = msg.agents
		p.clampCursor()
		return p, nil
	case agentKilledMsg:
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.err = nil
		p.loading = true
		return p, p.loadAgentsCmd()
	case agentOutputLoadedMsg:
		p.outputCache[msg.pid] = agentOutput{
			stdout: msg.stdout,
			stderr: msg.stderr,
		}
		return p, nil
	case tickMsg:
		if p.Focused {
			return p, tea.Batch(p.loadAgentsCmd(), p.tickCmd())
		}
		return p, nil
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseRightClickMsg:
		return p.handleMouseRightClick(msg)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Agents) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if p.loading && len(p.agents) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#555555")).
			Render("Loading agents...")
	}
	if p.err != nil && len(p.agents) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().NormalRed, "#C44B4B")).
			Render(fmt.Sprintf("Error: %v", p.err))
	}
	if len(p.agents) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#555555")).
			Render("No agents tracked")
	}
	// Build visible rows.
	rows := p.buildVisibleRows(width)
	// Apply viewport offset.
	end := p.offset + height
	if end > len(rows) {
		end = len(rows)
	}
	start := p.offset
	if start > len(rows) {
		start = len(rows)
	}
	lines := make([]string, 0, height)
	for i := start; i < end; i++ {
		lines = append(lines, rows[i])
	}
	// Pad remaining height with blank lines.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (p *Agents) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "enter", Description: "Expand/collapse output", Action: "expand"},
		{Key: "k (shift)", Description: "Kill selected agent", Action: "kill"},
		{Key: "R", Description: "Refresh agent list", Action: "refresh"},
	}
}

// SetActionsCfg stores the actions configuration for right-click menus.
func (p *Agents) SetActionsCfg(cfg config.ActionsConfig) {
	p.actionsCfg = cfg
}

// Close implements panels.Closer. Kills all running agents on shutdown.
func (p *Agents) Close() {
	if p.tracker != nil {
		p.tracker.KillAll()
	}
}

// ---------------------------------------------------------------------------
// Async commands
// ---------------------------------------------------------------------------
func (p *Agents) loadAgentsCmd() tea.Cmd {
	tracker := p.tracker
	return func() tea.Msg {
		return agentsLoadedMsg{agents: tracker.List()}
	}
}

func (p *Agents) killAgentCmd(pid int) tea.Cmd {
	tracker := p.tracker
	return func() tea.Msg {
		err := tracker.Kill(pid)
		return agentKilledMsg{pid: pid, err: err}
	}
}

func (p *Agents) loadOutputCmd(pid int) tea.Cmd {
	tracker := p.tracker
	return func() tea.Msg {
		stdout, stderr := tracker.Output(pid)
		return agentOutputLoadedMsg{pid: pid, stdout: stdout, stderr: stderr}
	}
}

func (p *Agents) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{time: t}
	})
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Agents) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}
	key := msg.String()
	switch key {
	case "j", "down":
		p.moveCursorDown()
	case "k", "up":
		p.moveCursorUp()
	case "enter":
		return p.toggleExpand()
	case "K":
		return p.killSelected()
	case "R":
		p.loading = true
		p.err = nil
		return p, p.loadAgentsCmd()
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the agent at the clicked row.
func (p *Agents) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.agents) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	return p, nil
}

// handleMouseRightClick shows a context menu for the agent at the clicked row.
func (p *Agents) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	idx := p.offset + msg.ContentRow
	if idx < 0 || idx >= len(p.agents) {
		return p, nil
	}
	p.cursor = idx
	p.ensureCursorVisible()
	label := fmt.Sprintf("PID:%d %s", p.agents[p.cursor].PID, p.agents[p.cursor].Command)
	cmd, directAction := rightclick.Cmd(p.actionsCfg, actions.ItemAgent, label)
	if cmd != nil {
		p.pendingOp = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// handleModalResult processes the result from the action picker modal.
func (p *Agents) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pendingOp
	p.pendingOp = ""
	if !msg.Accept {
		return p, nil
	}
	if op == opRightClickPick {
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	}
	return p, nil
}

// executeRightClickAction dispatches a right-click action to the appropriate method.
func (p *Agents) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	if action == actions.ActionToggleOutput {
		return p.toggleExpand()
	}
	return p, nil
}

// handleMouseWheel scrolls the agent list viewport.
func (p *Agents) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		p.offset -= panels.ScrollDelta
		if p.offset < 0 {
			p.offset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(p.agents) - p.Height
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
// Navigation
// ---------------------------------------------------------------------------
func (p *Agents) moveCursorDown() {
	if p.cursor < len(p.agents)-1 {
		p.cursor++
		p.ensureCursorVisible()
	}
}

func (p *Agents) moveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureCursorVisible()
	}
}

func (p *Agents) clampCursor() {
	p.cursor = panels.ClampCursor(p.cursor, len(p.agents))
}

func (p *Agents) ensureCursorVisible() {
	p.offset = panels.EnsureCursorVisible(p.cursor, p.offset, p.Height)
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
func (p *Agents) toggleExpand() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.agents) {
		return p, nil
	}
	pid := p.agents[p.cursor].PID
	if p.expanded[pid] {
		delete(p.expanded, pid)
		delete(p.outputCache, pid)
		return p, nil
	}
	p.expanded[pid] = true
	return p, p.loadOutputCmd(pid)
}

func (p *Agents) killSelected() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.agents) {
		return p, nil
	}
	agent := p.agents[p.cursor]
	if agent.Status != mcp.AgentRunning {
		return p, nil // nothing to kill
	}
	return p, p.killAgentCmd(agent.PID)
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (p *Agents) buildVisibleRows(width int) []string {
	var rows []string
	for i, agent := range p.agents {
		isCursor := i == p.cursor
		rows = append(rows, p.renderAgentRow(agent, width, isCursor))
		// If expanded, append output lines.
		if p.expanded[agent.PID] {
			if out, ok := p.outputCache[agent.PID]; ok {
				rows = append(rows, p.renderOutputLines(out, width)...)
			}
		}
	}
	return rows
}

func (p *Agents) renderAgentRow(agent mcp.AgentInfo, width int, isCursor bool) string {
	// Status icon.
	tc := p.themeColors()
	var statusIcon string
	var statusColor string
	switch agent.Status {
	case mcp.AgentRunning:
		statusIcon = "●"
		statusColor = panels.OrDefault(tc.NormalGreen, "#6B9E56")
	case mcp.AgentExited:
		statusIcon = "✓"
		statusColor = panels.OrDefault(tc.BrightBlue, "#7A9EBF")
	case mcp.AgentFailed:
		statusIcon = "✗"
		statusColor = panels.OrDefault(tc.NormalRed, "#C44B4B")
	}
	// Duration.
	dur := formatDuration(agent.Duration)
	// Exit code (only shown for non-running).
	exitStr := ""
	if agent.Status != mcp.AgentRunning {
		exitStr = fmt.Sprintf(" exit=%d", agent.ExitCode)
	}
	// Build the command string (truncate if needed).
	cmdStr := agent.Command
	if len(agent.Args) > 0 {
		cmdStr += " " + strings.Join(agent.Args, " ")
	}
	// Compose the line.
	line := fmt.Sprintf(" %s PID:%-6d %s  %s%s",
		statusIcon, agent.PID, dur, cmdStr, exitStr)
	// Truncate to width.
	if len(line) > width && width > 3 {
		line = line[:width-3] + "..."
	}
	style := lipgloss.NewStyle().Width(width)
	if isCursor && p.Focused {
		style = style.
			Background(panels.ColorOf(tc.SelectionBg, "#2A2A2A")).
			Foreground(panels.ColorOf(tc.Foreground, "#D4D4D4"))
	} else {
		style = style.Foreground(lipgloss.Color(statusColor))
	}
	return style.Render(line)
}

func (p *Agents) renderOutputLines(out agentOutput, width int) []string {
	var rows []string
	indent := "    "
	maxLines := 10 // cap visible output lines per stream
	outputStyle := lipgloss.NewStyle().
		Width(width).
		Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#555555")) // muted
	stderrStyle := lipgloss.NewStyle().
		Width(width).
		Foreground(panels.ColorOf(p.themeColors().NormalYellow, "#C9A227")) // pink for stderr
	if len(out.stdout) > 0 {
		rows = append(rows, outputStyle.Render(indent+"── stdout ──"))
		lines := out.stdout
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
			rows = append(rows, outputStyle.Render(
				fmt.Sprintf(indent+"  ... (%d lines omitted)", len(out.stdout)-maxLines),
			))
		}
		for _, line := range lines {
			text := indent + "  " + line
			if len(text) > width && width > 3 {
				text = text[:width-3] + "..."
			}
			rows = append(rows, outputStyle.Render(text))
		}
	}
	if len(out.stderr) > 0 {
		rows = append(rows, stderrStyle.Render(indent+"── stderr ──"))
		lines := out.stderr
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
			rows = append(rows, stderrStyle.Render(
				fmt.Sprintf(indent+"  ... (%d lines omitted)", len(out.stderr)-maxLines),
			))
		}
		for _, line := range lines {
			text := indent + "  " + line
			if len(text) > width && width > 3 {
				text = text[:width-3] + "..."
			}
			rows = append(rows, stderrStyle.Render(text))
		}
	}
	if len(out.stdout) == 0 && len(out.stderr) == 0 {
		rows = append(rows, outputStyle.Render(indent+"(no output)"))
	}
	return rows
}

// formatDuration formats a duration as a compact human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}
