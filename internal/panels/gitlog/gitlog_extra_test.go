package gitlog

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/panels/commitrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanel_SetActionsCfg_Extra(t *testing.T) {
	p := newTestPanelWithCommits(t, 3)
	cfg := config.ActionsConfig{
		RightClick: map[string]string{string(actions.ItemLogCommit): string(actions.ActionCopyHash)},
	}

	p.SetActionsCfg(cfg)
	assert.Equal(t, cfg, p.actionsCfg)
}

func TestStyleSubjectWithRefs_Extra(t *testing.T) {
	tests := []struct {
		name string
		refs []string
		want string
	}{
		{name: "renders refs suffix", refs: []string{"HEAD -> main", "origin/main"}, want: "(HEAD -> main, origin/main)"},
		{name: "renders plain subject without refs", refs: nil, want: "feat: add panel"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanelWithCommits(t, 1)
			c := git.Commit{
				ShortHash: "abc1234",
				Author:    "Dev",
				Date:      time.Now(),
				Subject:   "feat: add panel",
				Refs:      tt.refs,
			}
			line := commitrender.RenderLine(commitrender.Params{
				Commit:      c,
				Width:       200,
				Styles:      p.clStyles,
				ShowRefs:    true,
				ShowAuthor:  true,
				ShowDate:    true,
				GraphPrefix: "*",
			})
			assert.Contains(t, line, tt.want)
		})
	}
}

func TestRenderSearchBar_Extra(t *testing.T) {
	for _, tt := range []struct {
		name    string
		query   string
		matches int
		want    string
	}{{name: "shows query and match count", query: "author", matches: 2, want: "/author  [2 matches]"}} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanelWithCommits(t, 3)
			p.searchQuery = tt.query
			p.filteredIdx = []int{0, 1}
			bar := p.renderSearchBar(40)
			assert.Contains(t, bar, tt.want)
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
		{name: "out of bounds", msg: panels.PanelMouseRightClickMsg{ContentRow: 999}, wantCursor: 0, wantPending: "", wantCmd: false},
		{name: "valid commit", msg: panels.PanelMouseRightClickMsg{ContentRow: 0}, wantCursor: 0, wantPending: opRightClickPick, wantCmd: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanelWithCommits(t, 5)
			_, cmd := p.handleMouseRightClick(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			assert.Equal(t, tt.wantCursor, p.cursor)
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
		assertion func(t *testing.T, p *Panel, msg tea.Msg)
	}{
		{
			name: "rejected clears pending state",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 3)
				p.pendingOp = opRightClickPick
				p.pendingName = string(actions.ItemLogCommit)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: false},
			wantCmd: false,
			assertion: func(t *testing.T, p *Panel, _ tea.Msg) {
				t.Helper()
				assert.Empty(t, p.pendingOp)
				assert.Empty(t, p.pendingName)
			},
		},
		{
			name: "right click pick copies hash",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 3)
				p.pendingOp = opRightClickPick
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionCopyHash)},
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Info, toast.Level)
				assert.Contains(t, toast.Message, "Copied:")
			},
		},
		{
			name: "first use confirm copies hash",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 3)
				p.pendingOp = opFirstUseConfirm
				p.pendingName = string(actions.ItemLogCommit)
				return p
			},
			msg:     notify.ModalResultMsg{Accept: true, Value: string(actions.ActionCopyHash)},
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Info, toast.Level)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			_, cmd := p.handleModalResult(tt.msg)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, cmd())
			} else {
				tt.assertion(t, p, nil)
			}
		})
	}
}

func TestExecuteRightClickAction_Extra(t *testing.T) {
	tests := []struct {
		name      string
		action    actions.ActionID
		wantCmd   bool
		assertion func(t *testing.T, p *Panel, msg tea.Msg)
	}{
		{
			name:    "copy hash returns toast",
			action:  actions.ActionCopyHash,
			wantCmd: true,
			assertion: func(t *testing.T, _ *Panel, msg tea.Msg) {
				t.Helper()
				toast, ok := msg.(notify.ShowToastMsg)
				require.True(t, ok)
				assert.Equal(t, notify.Info, toast.Level)
			},
		},
		{
			name:      "copy message is unsupported",
			action:    actions.ActionID("copy_message"),
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ tea.Msg) {},
		},
		{
			name:      "show diff is unsupported",
			action:    actions.ActionID("show_diff"),
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ tea.Msg) {},
		},
		{
			name:      "unknown action is ignored",
			action:    actions.ActionID("unknown"),
			wantCmd:   false,
			assertion: func(t *testing.T, _ *Panel, _ tea.Msg) {},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanelWithCommits(t, 3)
			_, cmd := p.executeRightClickAction(tt.action)
			assert.Equal(t, tt.wantCmd, cmd != nil)
			if cmd != nil {
				tt.assertion(t, p, cmd())
			}
		})
	}
}

func TestHandleKey_Extra(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Panel
		msg       tea.KeyPressMsg
		assertion func(t *testing.T, p *Panel, cmd tea.Cmd)
	}{
		{
			name: "search mode entry",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 5)
				p.Focus()
				return p
			},
			msg: tea.KeyPressMsg{Code: '/'},
			assertion: func(t *testing.T, p *Panel, cmd tea.Cmd) {
				t.Helper()
				assert.Nil(t, cmd)
				assert.True(t, p.searchMode)
				assert.Empty(t, p.searchQuery)
			},
		},
		{
			name: "search mode exit clears filter",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 5)
				p.Focus()
				p.searchMode = true
				p.searchQuery = "author"
				p.filteredIdx = []int{0, 1}
				p.filteredDL = []displayLine{{commitIdx: 0, text: "*"}}
				p.filteredCmtY = []int{0}
				return p
			},
			msg: tea.KeyPressMsg{Code: tea.KeyEscape},
			assertion: func(t *testing.T, p *Panel, cmd tea.Cmd) {
				t.Helper()
				assert.Nil(t, cmd)
				assert.False(t, p.searchMode)
				assert.Empty(t, p.searchQuery)
				assert.Nil(t, p.filteredIdx)
			},
		},
		{
			name: "navigation moves cursor down",
			setup: func(t *testing.T) *Panel {
				t.Helper()
				p := newTestPanelWithCommits(t, 5)
				p.Focus()
				return p
			},
			msg: tea.KeyPressMsg{Code: 'j'},
			assertion: func(t *testing.T, p *Panel, cmd tea.Cmd) {
				t.Helper()
				assert.Nil(t, cmd)
				assert.Equal(t, 1, p.cursor)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			_, cmd := p.handleKey(tt.msg)
			tt.assertion(t, p, cmd)
		})
	}
}
