package gitinfo

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const syncTestUpstream = "origin/feature"

func TestRenderBranch_UpstreamSyncStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		branch     git.Branch
		want       []string
		wantAbsent []string
	}{
		{
			name:       "no upstream",
			branch:     git.Branch{Name: "feature", Hash: "abc1234"},
			want:       []string{"feature", "abc1234"},
			wantAbsent: []string{"origin/feature", "↑", "↓", "⇕"},
		},
		{
			name:   "ahead only",
			branch: git.Branch{Name: "feature", Upstream: syncTestUpstream, Ahead: 2, Hash: "abc1234"},
			want:   []string{"feature", "↑2", "→ " + syncTestUpstream, "abc1234"},
		},
		{
			name:   "behind only",
			branch: git.Branch{Name: "feature", Upstream: syncTestUpstream, Behind: 3, Hash: "abc1234"},
			want:   []string{"feature", "↓3", "→ " + syncTestUpstream, "abc1234"},
		},
		{
			name:   "diverged",
			branch: git.Branch{Name: "feature", Upstream: syncTestUpstream, Ahead: 2, Behind: 3, Hash: "abc1234"},
			want:   []string{"feature", "⇕", "↑2", "↓3", "→ " + syncTestUpstream, "abc1234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			rendered := p.renderBranch(listItem{kind: kindLocalBranch, branch: tt.branch}, 80, false)
			plain := panels.StripANSI(rendered)
			for _, want := range tt.want {
				assert.Contains(t, plain, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, plain, absent)
			}
		})
	}
}

func TestRequestBranchSyncActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*Panel) (panels.Panel, tea.Cmd)
		want pendingOp
	}{
		{name: "pull", call: (*Panel).requestPull, want: opBranchPull},
		{name: "pull rebase", call: (*Panel).requestPullRebase, want: opBranchPullRebase},
		{name: "push", call: (*Panel).requestPush, want: opBranchPush},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, &mockGitOps{branches: []git.Branch{{Name: "feature", Upstream: syncTestUpstream}}})
			p.activeTab = tabBranches
			_, cmd := tt.call(p)
			require.NotNil(t, cmd)
			_, ok := cmd().(notify.ShowModalMsg)
			assert.True(t, ok)
			assert.Equal(t, tt.want, p.pending)
			assert.Equal(t, "feature", p.pendingName)
			assert.Equal(t, syncTestUpstream, p.pendingPath)
		})
	}
}

func TestRequestPush_NoUpstreamSetsPending(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, &mockGitOps{branches: []git.Branch{{Name: "feature"}}})
	p.activeTab = tabBranches
	_, cmd := p.requestPush()
	require.NotNil(t, cmd)
	_, ok := cmd().(notify.ShowModalMsg)
	assert.True(t, ok)
	assert.Equal(t, opBranchPush, p.pending)
	assert.Empty(t, p.pendingPath)
}

func TestRequestBranchSyncGuards(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{{kind: kindRemoteBranch, branch: git.Branch{Name: syncTestUpstream, IsRemote: true}}}
	p.tabCursor[tabBranches] = 0
	_, cmd := p.requestPush()
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)

	p.activeTab = tabWorktrees
	_, cmd = p.requestPush()
	assert.Nil(t, cmd)
}

func TestHandleModalResult_BranchPullPush(t *testing.T) {
	t.Parallel()

	var pullCalled bool
	var pullOpts git.PullOpts
	var pushCalled bool
	var pushOpts git.PushOpts
	mock := &mockGitOps{
		branches: []git.Branch{{Name: "feature", Upstream: syncTestUpstream}},
		pullFunc: func(_ context.Context, opts git.PullOpts) error {
			pullCalled = true
			pullOpts = opts
			return nil
		},
		pushFunc: func(_ context.Context, opts git.PushOpts) error {
			pushCalled = true
			pushOpts = opts
			return nil
		},
	}
	p := newTestPanel(t, mock)

	p.pending = opBranchPullRebase
	p.pendingName = "feature"
	p.pendingPath = syncTestUpstream
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result, ok := cmd().(opResultMsg)
	require.True(t, ok)
	assert.True(t, pullCalled)
	assert.Equal(t, eventBranchPulled, result.op)
	assert.Equal(t, git.PullOpts{Rebase: true, Remote: "origin", Branch: "feature"}, pullOpts)

	p.pending = opBranchPush
	p.pendingName = "feature"
	p.pendingPath = ""
	_, cmd = p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result, ok = cmd().(opResultMsg)
	require.True(t, ok)
	assert.True(t, pushCalled)
	assert.Equal(t, eventBranchPushed, result.op)
	assert.Equal(t, git.PushOpts{Remote: "origin", Branch: "feature", SetUpstream: true}, pushOpts)
}

func TestHandleOpResult_BranchPullPushToastsAndReloads(t *testing.T) {
	t.Parallel()

	for _, op := range []string{eventBranchPulled, eventBranchPushed} {
		op := op
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			p := New(defaultMock(), config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
			_, cmd := p.handleOpResult(opResultMsg{op: op, name: "feature"})
			msgs := collectCmdMsgs(cmd)
			var sawToast, sawReload bool
			for _, msg := range msgs {
				switch m := msg.(type) {
				case notify.ShowToastMsg:
					sawToast = strings.Contains(m.Message, "feature") && m.Level == notify.Success
				case dataLoadedMsg:
					sawReload = true
				}
			}
			assert.True(t, sawToast)
			assert.True(t, sawReload)
		})
	}
}
