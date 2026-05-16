package agents

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock tracker
// ---------------------------------------------------------------------------

type mockTracker struct {
	agents        []mcp.AgentInfo
	killErr       error
	killedPID     int
	killAllCalled bool
	stdout        []string
	stderr        []string
}

var _ Tracker = (*mockTracker)(nil)

func (m *mockTracker) List() []mcp.AgentInfo {
	return m.agents
}

func (m *mockTracker) Kill(pid int) error {
	m.killedPID = pid
	return m.killErr
}

func (m *mockTracker) KillAll() {
	m.killAllCalled = true
}

func (m *mockTracker) Output(pid int) ([]string, []string) {
	return m.stdout, m.stderr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func newTestPanel(t *testing.T, mock *mockTracker) *Agents {
	t.Helper()
	p := New(mock, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return a command")
	// Execute the load command synchronously.
	msg := cmd()
	p.Update(msg)
	return p
}

// ---------------------------------------------------------------------------
// Tests: Panel creation and interface compliance
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)
	assert.Equal(t, "agents", p.Title())
	assert.NotNil(t, p.expanded)
	assert.NotNil(t, p.outputCache)
}

func TestInterfaceCompliance(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)

	// Verify Panel interface.
	var _ panels.Panel = p

	// Verify Closer interface.
	var _ panels.Closer = p
}

func TestInit_ReturnsLoadCmd(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning},
		},
	}
	p := New(mock, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Rendering
// ---------------------------------------------------------------------------

func TestView_ZeroDimensions(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(0, 10))
}

func TestView_NoAgents(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	view := p.View(80, 24)
	assert.Contains(t, view, "No agents tracked")
}

func TestView_Loading(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)
	p.loading = true
	view := p.View(80, 24)
	assert.Contains(t, view, "Loading agents")
}

func TestView_OneAgent_Running(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{
				PID:       1234,
				Command:   "python",
				Args:      []string{"script.py"},
				Status:    mcp.AgentRunning,
				StartedAt: time.Now().Add(-5 * time.Second),
				Duration:  5 * time.Second,
			},
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "1234")
	assert.Contains(t, view, "python")
	assert.Contains(t, view, "●")
}

func TestView_OneAgent_Exited(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{
				PID:      2345,
				Command:  "bash",
				Status:   mcp.AgentExited,
				ExitCode: 0,
				Duration: 10 * time.Second,
			},
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "2345")
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "exit=0")
}

func TestView_OneAgent_Failed(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{
				PID:      3456,
				Command:  "node",
				Status:   mcp.AgentFailed,
				ExitCode: 1,
				Duration: 2 * time.Second,
			},
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "3456")
	assert.Contains(t, view, "✗")
	assert.Contains(t, view, "exit=1")
}

func TestView_MultipleAgents(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "agent1", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "agent2", Status: mcp.AgentExited, ExitCode: 0, Duration: 5 * time.Second},
			{PID: 300, Command: "agent3", Status: mcp.AgentFailed, ExitCode: 2, Duration: 3 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "100")
	assert.Contains(t, view, "200")
	assert.Contains(t, view, "300")
	assert.Contains(t, view, "agent1")
	assert.Contains(t, view, "agent2")
	assert.Contains(t, view, "agent3")
}

// ---------------------------------------------------------------------------
// Tests: Navigation
// ---------------------------------------------------------------------------

func TestNavigation_JK(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
			{PID: 300, Command: "c", Status: mcp.AgentRunning, Duration: 3 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	assert.Equal(t, 0, p.cursor)

	// Move down.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// At bottom, should not go further.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// Move up.
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursor)

	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)

	// At top, should not go further.
	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)
}

func TestNavigation_ArrowKeys(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Down arrow key sends a specific tea.KeyPressMsg.
	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Tests: Expand/collapse output
// ---------------------------------------------------------------------------

func TestExpand_ToggleOutput(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentExited, Duration: 1 * time.Second},
		},
		stdout: []string{"output line 1", "output line 2"},
		stderr: []string{"error line 1"},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(120, 24)

	// Press Enter to expand.
	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd, "expand should return an output load command")
	assert.True(t, p.expanded[100])

	// Execute the output load command.
	msg := cmd()
	p.Update(msg)

	// Verify output is cached.
	out, ok := p.outputCache[100]
	assert.True(t, ok)
	assert.Equal(t, []string{"output line 1", "output line 2"}, out.stdout)
	assert.Equal(t, []string{"error line 1"}, out.stderr)

	// View should include output.
	view := p.View(120, 24)
	assert.Contains(t, view, "output line 1")
	assert.Contains(t, view, "error line 1")

	// Press Enter again to collapse.
	p.Update(keyMsg('\r'))
	assert.False(t, p.expanded[100])
}

// ---------------------------------------------------------------------------
// Tests: Kill agent
// ---------------------------------------------------------------------------

func TestKillAgent_Running(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press K to kill.
	_, cmd := p.Update(keyMsg('K'))
	require.NotNil(t, cmd, "kill should return a command")

	msg := cmd()
	require.IsType(t, agentKilledMsg{}, msg)
	assert.Equal(t, 100, mock.killedPID)
}

func TestKillAgent_NotRunning(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentExited, ExitCode: 0, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press K — should not produce a command for non-running agent.
	_, cmd := p.Update(keyMsg('K'))
	assert.Nil(t, cmd, "kill should not return a command for exited agent")
}

func TestKillAgent_EmptyList(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('K'))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Refresh
// ---------------------------------------------------------------------------

func TestRefresh(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('R'))
	require.NotNil(t, cmd)
	assert.True(t, p.loading)
}

// ---------------------------------------------------------------------------
// Tests: Close (Closer interface)
// ---------------------------------------------------------------------------

func TestClose_KillsAll(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := New(mock, nil)
	p.Close()
	assert.True(t, mock.killAllCalled)
}

// ---------------------------------------------------------------------------
// Tests: KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify specific bindings exist.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/↓"])
	assert.True(t, keys["k/↑"])
	assert.True(t, keys["enter"])
	assert.True(t, keys["R"])
}

// ---------------------------------------------------------------------------
// Tests: Focus/Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	mock := &mockTracker{}
	p := New(mock, nil)

	assert.False(t, p.Focused)
	p.Focus()
	assert.True(t, p.Focused)
	p.Blur()
	assert.False(t, p.Focused)
}

// ---------------------------------------------------------------------------
// Tests: Unfocused key handling
// ---------------------------------------------------------------------------

func TestUnfocused_IgnoresKeys(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	// Don't focus the panel.
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('j'))
	assert.Nil(t, cmd)
	assert.Equal(t, 0, p.cursor) // cursor should not move
}

// ---------------------------------------------------------------------------
// Tests: Tick auto-refresh
// ---------------------------------------------------------------------------

func TestTick_WhenFocused(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()

	_, cmd := p.Update(tickMsg{time: time.Now()})
	assert.NotNil(t, cmd, "tick while focused should return batch command")
}

func TestTick_WhenBlurred(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	p.Blur()

	_, cmd := p.Update(tickMsg{time: time.Now()})
	assert.Nil(t, cmd, "tick while blurred should return nil")
}

// ---------------------------------------------------------------------------
// Tests: Cursor clamping
// ---------------------------------------------------------------------------

func TestCursorClamp_EmptyList(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	assert.Equal(t, 0, p.cursor)
}

func TestCursorClamp_AfterRemoval(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Move to last item.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	// Simulate agents list shrinking.
	mock.agents = []mcp.AgentInfo{
		{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
	}
	msg := p.loadAgentsCmd()()
	p.Update(msg)

	// Cursor should be clamped.
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Tests: Format duration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{30 * time.Second, "30.0s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h0m"},
		{5400 * time.Second, "1h30m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.d)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: Error display
// ---------------------------------------------------------------------------

func TestView_Error(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)
	p.err = assert.AnError

	view := p.View(80, 24)
	assert.Contains(t, view, "Could not load agents")
}

// ---------------------------------------------------------------------------
// Tests: AgentKilledMsg with error
// ---------------------------------------------------------------------------

func TestAgentKilledMsg_WithError(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(agentKilledMsg{pid: 100, err: assert.AnError})
	assert.Nil(t, cmd)
	assert.NotNil(t, p.err)
}

func TestAgentKilledMsg_Success(t *testing.T) {
	mock := &mockTracker{agents: []mcp.AgentInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(agentKilledMsg{pid: 100, err: nil})
	assert.NotNil(t, cmd, "successful kill should trigger a refresh")
	assert.True(t, p.loading)
}

// ---------------------------------------------------------------------------
// Tests: Mouse click
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsAgent(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
			{PID: 300, Command: "c", Status: mcp.AgentRunning, Duration: 3 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	assert.Equal(t, 0, p.cursor)

	// Click on row 1 → selects second agent.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)

	// Click on row 2 → selects third agent.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 24)

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "out-of-bounds click should not move cursor")
}

// ---------------------------------------------------------------------------
// Tests: Mouse wheel
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollDown(t *testing.T) {
	agents := make([]mcp.AgentInfo, 10)
	for i := range agents {
		agents[i] = mcp.AgentInfo{PID: 100 + i, Command: "agent", Status: mcp.AgentRunning, Duration: 1 * time.Second}
	}
	mock := &mockTracker{agents: agents}
	p := newTestPanel(t, mock)
	p.SetSize(80, 3) // Small viewport to allow scrolling.

	assert.Equal(t, 0, p.offset)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 3, p.offset, "should scroll down by delta of 3")
}

func TestMouseWheel_ScrollUp(t *testing.T) {
	agents := make([]mcp.AgentInfo, 10)
	for i := range agents {
		agents[i] = mcp.AgentInfo{PID: 100 + i, Command: "agent", Status: mcp.AgentRunning, Duration: 1 * time.Second}
	}
	mock := &mockTracker{agents: agents}
	p := newTestPanel(t, mock)
	p.SetSize(80, 3)

	// Scroll down first.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Greater(t, p.offset, 0)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should scroll back to top")
}

func TestMouseWheel_ScrollUpClampsToZero(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: 1 * time.Second},
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 24)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should not scroll below 0")
}
