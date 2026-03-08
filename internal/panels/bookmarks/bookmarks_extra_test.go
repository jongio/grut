package bookmarks

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanel_SetActionsCfg_Extra(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)
	cfg := config.ActionsConfig{
		DoubleClick: map[string]string{string(actions.ItemBookmark): string(actions.ActionCopyPath)},
		Confirmed:   map[string]bool{string(actions.ItemBookmark): true},
	}

	p.SetActionsCfg(cfg)
	assert.Equal(t, cfg, p.actionsCfg)
}

func TestPanel_HandleModalResult_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Panel
		msg       notify.ModalResultMsg
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, cmd func() any)
	}{
		{
			name: "rejected clears pending state",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				p := newTestPanel(t, m)
				p.pendingOp = opRightClickPick
				p.pendingName = string(actions.ItemBookmark)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: false},
			wantCmd: false,
			assertion: func(t *testing.T, p *Panel, _ func() any) {
				t.Helper()
				assert.Empty(t, p.pendingOp)
				assert.Empty(t, p.pendingName)
			},
		},
		{
			name: "right click pick deletes bookmark",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				p := newTestPanel(t, m)
				p.pendingOp = opRightClickPick
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionDelete)},
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, runCmd func() any) {
				t.Helper()
				msg := runCmd()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Success, toast.Level)
				assert.Equal(t, 0, p.itemCount())
			},
		},
		{
			name: "first use confirm jumps to bookmark",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				p := newTestPanel(t, m)
				p.pendingOp = opFirstUseConfirm
				p.pendingName = string(actions.ItemBookmark)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionJump)},
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, runCmd func() any) {
				t.Helper()
				msg := runCmd()
				navMsg, ok := msg.(panels.NavigateToPathMsg)
				require.True(t, ok)
				assert.NotEmpty(t, navMsg.Path)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			_, cmd := p.handleModalResult(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if !tt.wantCmd {
				tt.assertion(t, p, nil)
				return
			}
			tt.assertion(t, p, func() any { return cmd() })
		})
	}
}

func TestPanel_CopyPath_Extra(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *Panel
		wantCmd bool
		assert  func(t *testing.T, cmd any)
	}{
		{
			name: "valid cursor copies bookmark path",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m)
			},
			wantCmd: true,
			assert: func(t *testing.T, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
		{
			name: "out of bounds cursor is no op",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				p := newTestPanel(t, m)
				p.cursor = 99
				return p
			},
			wantCmd: false,
			assert:  func(t *testing.T, _ any) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			_, cmd := p.copyPath()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assert(t, cmd())
			}
		})
	}
}

func TestPanel_ExecuteRightClickAction_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Panel
		action    actions.ActionID
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, msg any)
	}{
		{
			name: "jump navigates to bookmark",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m)
			},
			action:  actions.ActionJump,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, msg any) {
				t.Helper()
				navMsg, ok := msg.(panels.NavigateToPathMsg)
				require.True(t, ok)
				assert.NotEmpty(t, navMsg.Path)
			},
		},
		{
			name: "delete removes bookmark",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m)
			},
			action:  actions.ActionDelete,
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Success, toast.Level)
				assert.Equal(t, 0, p.itemCount())
			},
		},
		{
			name: "copy path returns success toast",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m)
			},
			action:  actions.ActionCopyPath,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
		{
			name: "unknown action is ignored",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m)
			},
			action:    actions.ActionID("unknown"),
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ any) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			_, cmd := p.executeRightClickAction(tt.action)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, cmd())
			}
		})
	}
}

func TestPanel_EnsureCursorVisible_Extra(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height does nothing", height: 0, cursor: 4, offset: 2, wantOffset: 2},
		{name: "cursor above viewport", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport", height: 3, cursor: 6, offset: 1, wantOffset: 4},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(t)
			p := newTestPanel(t, m)
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset
			p.ensureCursorVisible()
			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

func TestPanel_DeleteBookmark_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (*Panel, string)
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, path string, msg any)
	}{
		{
			name: "cursor out of bounds is ignored",
			setup: func(t *testing.T) (*Panel, string) {
				t.Helper()
				m := newTestManager(t)
				p := newTestPanel(t, m)
				p.cursor = 42
				return p, ""
			},
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ string, _ any) {},
		},
		{
			name: "valid delete removes item",
			setup: func(t *testing.T) (*Panel, string) {
				t.Helper()
				m := newTestManager(t)
				dir := t.TempDir()
				require.NoError(t, m.Add(dir))
				return newTestPanel(t, m), filepath.Clean(dir)
			},
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, path string, msg any) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Success, toast.Level)
				assert.Equal(t, 0, p.itemCount())
				assert.False(t, p.manager.Has(path))
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p, path := tt.setup(t)
			_, cmd := p.deleteBookmark()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, path, cmd())
			}
		})
	}
}
