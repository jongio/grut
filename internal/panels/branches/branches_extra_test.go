package branches

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanel_SetActionsCfg_Extra(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	cfg := config.ActionsConfig{
		RightClick: map[string]string{string(actions.ItemLocalBranch): string(actions.ActionCopyName)},
	}

	p.SetActionsCfg(cfg)
	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseRightClick_Extra(t *testing.T) {
	tests := []struct {
		name        string
		msg         panels.PanelMouseRightClickMsg
		wantCursor  int
		wantPending pendingOp
		wantCmd     bool
	}{
		{name: "out of bounds", msg: panels.PanelMouseRightClickMsg{ContentRow: 99}, wantCursor: 1, wantPending: opNone, wantCmd: false},
		{name: "header row", msg: panels.PanelMouseRightClickMsg{ContentRow: 0}, wantCursor: 1, wantPending: opNone, wantCmd: false},
		{name: "valid branch row", msg: panels.PanelMouseRightClickMsg{ContentRow: 2}, wantCursor: 2, wantPending: opRightClickPick, wantCmd: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			p.cursor = 1
			_, cmd := p.handleMouseRightClick(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			assert.Equal(t, tt.wantCursor, p.cursor)
			assert.Equal(t, tt.wantPending, p.pending)
		})
	}
}

func TestExecuteRightClickAction_Extra(t *testing.T) {
	tests := []struct {
		name      string
		cursor    int
		action    actions.ActionID
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, mock *mockGitOps, msg tea.Msg)
	}{
		{
			name:    "checkout selected branch",
			cursor:  2,
			action:  actions.ActionCheckout,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, mock *mockGitOps, msg tea.Msg) {
				t.Helper()
				result, ok := msg.(branchOpResultMsg)
				require.True(t, ok)
				assert.Equal(t, "checkout", result.op)
				assert.Equal(t, "feature/auth", result.name)
				assert.Equal(t, "feature/auth", mock.lastCheckout)
			},
		},
		{
			name:    "copy name returns toast",
			cursor:  2,
			action:  actions.ActionCopyName,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, _ *mockGitOps, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
		{
			name:    "open in browser no remote",
			cursor:  2,
			action:  actions.ActionOpenInBrowser,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, _ *mockGitOps, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Warn, toast.Level)
				assert.Contains(t, toast.Message, "No remote available")
			},
		},
		{
			name:      "unknown action ignored",
			cursor:    2,
			action:    actions.ActionID("unknown"),
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ *mockGitOps, _ tea.Msg) {},
		},
		{
			name:      "header cursor ignored",
			cursor:    0,
			action:    actions.ActionCheckout,
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ *mockGitOps, _ tea.Msg) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			p.cursor = tt.cursor
			_, cmd := p.executeRightClickAction(tt.action)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, mock, cmd())
			}
		})
	}
}

func TestHandleMouseWheel_Extra(t *testing.T) {
	tests := []struct {
		name       string
		button     tea.MouseButton
		start      int
		height     int
		wantOffset int
	}{
		{name: "scroll down", button: tea.MouseWheelDown, start: 0, height: 2, wantOffset: 3},
		{name: "scroll up", button: tea.MouseWheelUp, start: 3, height: 2, wantOffset: 0},
		{name: "clamp at max", button: tea.MouseWheelDown, start: 3, height: 4, wantOffset: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			p.Height = tt.height
			p.offset = tt.start
			_, cmd := p.handleMouseWheel(tea.MouseWheelMsg{Button: tt.button})
			assert.Nil(t, cmd)
			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

func TestCurrentItemType_Extra(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		want   actions.ItemType
	}{
		{name: "local branch", cursor: 2, want: actions.ItemLocalBranch},
		{name: "remote branch", cursor: 4, want: actions.ItemRemoteBranch},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			p.cursor = tt.cursor
			assert.Equal(t, tt.want, p.currentItemType())
		})
	}
}

func TestEnsureCursorVisible_Extra(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height", height: 0, cursor: 4, offset: 2, wantOffset: 2},
		{name: "cursor above", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below", height: 3, cursor: 5, offset: 1, wantOffset: 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset
			p.ensureCursorVisible()
			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

func TestRequestFetch_Extra(t *testing.T) {
	for _, tt := range []struct{ name string }{{name: "fetch emits toast and result"}} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitOps{branches: sampleBranches()}
			p := newTestPanel(t, mock, defaultCfg())
			_, cmd := p.requestFetch()
			require.NotNil(t, cmd)

			msg := cmd()
			batch, ok := msg.(tea.BatchMsg)
			require.True(t, ok)
			require.Len(t, batch, 2)

			first := batch[0]()
			toast, ok := first.(notify.ShowToastMsg)
			require.True(t, ok)
			assert.Equal(t, notify.Info, toast.Level)
			assert.Contains(t, toast.Message, "Fetching")

			second := batch[1]()
			result, ok := second.(branchOpResultMsg)
			require.True(t, ok)
			assert.Equal(t, "fetched", result.op)
			assert.Equal(t, "all remotes", result.name)
			assert.True(t, mock.fetchCalled)
		})
	}
}
