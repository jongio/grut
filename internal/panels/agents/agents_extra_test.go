package agents

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockTracker{})
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"agent": "toggle_output"},
		Confirmed:  map[string]bool{"agent": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to max offset", func(t *testing.T) {
		p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
			{PID: 300, Command: "c", Status: mcp.AgentRunning, Duration: 3 * time.Second},
			{PID: 400, Command: "d", Status: mcp.AgentRunning, Duration: 4 * time.Second},
		}})
		p.Height = 1

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*Agents)

		assert.Nil(t, cmd)
		assert.Equal(t, 3, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
			{PID: 100, Command: "a", Status: mcp.AgentRunning, Duration: 1 * time.Second},
			{PID: 200, Command: "b", Status: mcp.AgentRunning, Duration: 2 * time.Second},
			{PID: 300, Command: "c", Status: mcp.AgentRunning, Duration: 3 * time.Second},
			{PID: 400, Command: "d", Status: mcp.AgentRunning, Duration: 4 * time.Second},
		}})
		p.Height = 1
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*Agents)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestEnsureCursorVisible(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height leaves offset unchanged", height: 0, cursor: 0, offset: 2, wantOffset: 2},
		{name: "cursor above offset moves viewport up", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport moves viewport down", height: 3, cursor: 5, offset: 1, wantOffset: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&mockTracker{})
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset

			p.ensureCursorVisible()

			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

// ---------------------------------------------------------------------------
// handleMouseRightClick
// ---------------------------------------------------------------------------

func TestHandleMouseRightClick_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.Height = 10

	_, cmd := p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 99})
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_ValidRow(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
		{PID: 200, Command: "test2", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.Height = 10
	// Pre-confirm the item type so we get a direct action instead of a menu.
	p.actionsCfg = config.ActionsConfig{
		Confirmed: map[string]bool{string(actions.ItemAgent): true},
	}

	p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 1})
	// Should select cursor=1 and trigger a direct action.
	assert.Equal(t, 1, p.cursor)
	// Direct action for ItemAgent is ActionToggleOutput — cmd may or may not be nil
	// depending on toggleExpand, but cursor must be set.
}

func TestHandleMouseRightClick_NegativeIndex(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.Height = 10
	p.offset = 5 // offset makes idx negative

	_, cmd := p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: -10})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleModalResult
// ---------------------------------------------------------------------------

func TestHandleModalResult_Rejected(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.pendingOp = opRightClickPick

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, "", p.pendingOp)
}

func TestHandleModalResult_RightClickPick(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.pendingOp = opRightClickPick
	p.cursor = 0

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionToggleOutput),
	})
	// toggleExpand should have been called; the command result isn't critical,
	// but the op should be cleared.
	assert.Equal(t, "", p.pendingOp)
	_ = cmd
}

func TestHandleModalResult_UnknownOp(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.pendingOp = "something_else"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "toggle_output"})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// executeRightClickAction
// ---------------------------------------------------------------------------

func TestExecuteRightClickAction_ToggleOutput(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})
	p.cursor = 0

	_, _ = p.executeRightClickAction(actions.ActionToggleOutput)
	// toggleExpand toggles the expanded map
	assert.True(t, p.expanded[100])
}

func TestExecuteRightClickAction_UnknownAction(t *testing.T) {
	p := newTestPanel(t, &mockTracker{agents: []mcp.AgentInfo{
		{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
	}})

	_, cmd := p.executeRightClickAction(actions.ActionID("unknown"))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// tickCmd
// ---------------------------------------------------------------------------

func TestTickCmd_AlwaysReturnsTick(t *testing.T) {
	p := New(&mockTracker{})
	cmd := p.tickCmd()
	assert.NotNil(t, cmd, "tickCmd always returns a tick command")
}

// ---------------------------------------------------------------------------
// buildVisibleRows with expand
// ---------------------------------------------------------------------------

func TestBuildVisibleRows_ExpandedAgent(t *testing.T) {
	mock := &mockTracker{
		agents: []mcp.AgentInfo{
			{PID: 100, Command: "test", Status: mcp.AgentRunning, Duration: time.Second},
		},
		stdout: []string{"line 1", "line 2"},
		stderr: []string{"err 1"},
	}
	p := newTestPanel(t, mock)
	p.expanded[100] = true
	// Populate output cache — this is what View() does via loadOutput.
	stdout, stderr := mock.Output(100)
	p.outputCache[100] = agentOutput{stdout: stdout, stderr: stderr}
	p.Height = 20
	p.Width = 80

	rows := p.buildVisibleRows(p.Width)
	// Should contain the agent row + output lines
	assert.True(t, len(rows) > 1, fmt.Sprintf("expected multiple rows, got %d", len(rows)))
}
