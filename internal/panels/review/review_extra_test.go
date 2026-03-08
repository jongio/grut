package review

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
	p := newTestPanel(nil)
	cfg := config.ActionsConfig{
		RightClick: map[string]string{string(actions.ItemReviewFile): string(actions.ActionApprove)},
	}

	p.SetActionsCfg(cfg)
	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Extra(t *testing.T) {
	tests := []struct {
		name       string
		button     tea.MouseButton
		start      int
		height     int
		wantScroll int
	}{
		{name: "scroll down", button: tea.MouseWheelDown, start: 0, height: 1, wantScroll: 3},
		{name: "scroll up", button: tea.MouseWheelUp, start: 3, height: 1, wantScroll: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanel(nil)
			p.SetFiles(sampleReviewFiles())
			p.Height = tt.height
			p.scrollY = tt.start
			_, cmd := p.handleMouseWheel(tea.MouseWheelMsg{Button: tt.button})
			assert.Nil(t, cmd)
			assert.Equal(t, tt.wantScroll, p.scrollY)
		})
	}
}

func TestHandleMouseRightClick_Extra(t *testing.T) {
	tests := []struct {
		name        string
		msg         panels.PanelMouseRightClickMsg
		wantCursor  int
		wantPending string
		wantCmd     bool
	}{
		{name: "out of bounds", msg: panels.PanelMouseRightClickMsg{ContentRow: 99}, wantCursor: 0, wantPending: "", wantCmd: false},
		{name: "section row", msg: panels.PanelMouseRightClickMsg{ContentRow: 0}, wantCursor: 0, wantPending: "", wantCmd: false},
		{name: "valid file row", msg: panels.PanelMouseRightClickMsg{ContentRow: 2}, wantCursor: 0, wantPending: opRightClickPick, wantCmd: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanel(nil)
			p.SetFiles(sampleReviewFiles())
			_, cmd := p.handleMouseRightClick(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			assert.Equal(t, tt.wantCursor, p.fileCursor)
			assert.Equal(t, tt.wantPending, p.pendingOp)
		})
	}
}

func TestHandleModalResult_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Panel
		msg       notify.ModalResultMsg
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, mock *mockGit, msg tea.Msg)
	}{
		{
			name: "rejected clears pending state",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanel(nil)
				p.pendingOp = opRightClickPick
				p.pendingName = string(actions.ItemReviewFile)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: false},
			wantCmd: false,
			assertion: func(t *testing.T, p *Panel, _ *mockGit, _ tea.Msg) {
				t.Helper()
				assert.Empty(t, p.pendingOp)
				assert.Empty(t, p.pendingName)
			},
		},
		{
			name: "right click pick approves file",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				mock := &mockGit{}
				p := newTestPanel(mock)
				p.SetFiles(sampleReviewFiles())
				p.pendingOp = opRightClickPick
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionApprove)},
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, _ *mockGit, msg tea.Msg) {
				t.Helper()
				result, ok := msg.(stageResultMsg)
				require.True(t, ok)
				assert.NoError(t, result.err)
				assert.Equal(t, HunkApproved, p.files[0].HunkStates[0])
			},
		},
		{
			name: "first use confirm copies path",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanel(nil)
				p.SetFiles(sampleReviewFiles())
				p.pendingOp = opFirstUseConfirm
				p.pendingName = string(actions.ItemReviewFile)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionCopyPath)},
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, _ *mockGit, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			var mock *mockGit
			if mg, ok := p.git.(*mockGit); ok {
				mock = mg
			}
			_, cmd := p.handleModalResult(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, mock, cmd())
			} else {
				tt.assertion(t, p, mock, nil)
			}
		})
	}
}

func TestApproveFile_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (*Panel, *mockGit)
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, mock *mockGit, msg tea.Msg)
	}{
		{
			name: "valid cursor approves and stages file",
			setup: func(t *testing.T) (*Panel, *mockGit) {
				t.Helper()
				mock := &mockGit{}
				p := newTestPanel(mock)
				p.SetFiles(sampleReviewFiles())
				return p, mock
			},
			wantCmd: true,
			assertion: func(t *testing.T, p *Panel, mock *mockGit, msg tea.Msg) {
				t.Helper()
				result, ok := msg.(stageResultMsg)
				require.True(t, ok)
				assert.NoError(t, result.err)
				assert.Equal(t, []string{"file1.go"}, mock.stagePaths)
				for _, state := range p.files[0].HunkStates {
					assert.Equal(t, HunkApproved, state)
				}
			},
		},
		{
			name: "out of bounds cursor is ignored",
			setup: func(t *testing.T) (*Panel, *mockGit) {
				t.Helper()
				mock := &mockGit{}
				p := newTestPanel(mock)
				p.SetFiles(sampleReviewFiles())
				p.fileCursor = 99
				return p, mock
			},
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ *mockGit, _ tea.Msg) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p, mock := tt.setup(t)
			_, cmd := p.approveFile()
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, mock, cmd())
			}
		})
	}
}

func TestCopyPath_Extra(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *Panel
		wantCmd bool
		assert  func(t *testing.T, msg tea.Msg)
	}{
		{
			name: "valid cursor copies path",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanel(nil)
				p.SetFiles(sampleReviewFiles())
				return p
			},
			wantCmd: true,
			assert: func(t *testing.T, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Contains(t, []notify.Level{notify.Success, notify.Error}, toast.Level)
				assert.True(t, strings.Contains(toast.Message, "Copied:") || strings.Contains(toast.Message, "Copy failed:"))
			},
		},
		{
			name: "out of bounds cursor is ignored",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanel(nil)
				p.SetFiles(sampleReviewFiles())
				p.fileCursor = 99
				return p
			},
			wantCmd: false,
			assert:  func(t *testing.T, _ tea.Msg) {},
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

func TestEnsureHunkVisible_Extra(t *testing.T) {
	tests := []struct {
		name       string
		hunkCursor int
		wantScroll int
	}{
		{name: "moves viewport to selected hunk", hunkCursor: 1, wantScroll: 9},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanel(nil)
			p.hunkLineStarts = []int{3, 9}
			p.hunkCursor = tt.hunkCursor
			p.scrollY = 1
			p.ensureHunkVisible()
			assert.Equal(t, tt.wantScroll, p.scrollY)
		})
	}
}
