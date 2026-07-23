package gitinfo

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitinfoCompareBaseMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	msgs := make([]tea.Msg, 0, len(batch))
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		msgs = append(msgs, batchCmd())
	}
	return msgs
}

func TestCompareBaseKeySetsRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tab  tabID
		item listItem
		want string
	}{
		{
			name: "local branch",
			tab:  tabBranches,
			item: listItem{kind: kindLocalBranch, branch: git.Branch{Name: "feature"}},
			want: "feature",
		},
		{
			name: "remote branch",
			tab:  tabBranches,
			item: listItem{kind: kindRemoteBranch, branch: git.Branch{Name: "origin/main", IsRemote: true}},
			want: "origin/main",
		},
		{
			name: "tag",
			tab:  tabTags,
			item: listItem{kind: kindTag, tag: git.Tag{Name: "v1.0.0"}},
			want: "v1.0.0",
		},
		{
			name: "remote tag",
			tab:  tabTags,
			item: listItem{kind: kindRemoteTag, tag: git.Tag{Name: "v2.0.0"}},
			want: "v2.0.0",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			p.Focused = true
			p.activeTab = tt.tab
			p.tabItems[tt.tab] = []listItem{tt.item}

			_, cmd := p.handleKey(tea.KeyPressMsg{Code: '='})
			msgs := gitinfoCompareBaseMessages(t, cmd)

			require.Len(t, msgs, 2)
			setMsg, ok := msgs[0].(panels.SetCompareBaseMsg)
			require.True(t, ok)
			assert.Equal(t, tt.want, setMsg.Ref)
			toast, ok := msgs[1].(notify.ShowToastMsg)
			require.True(t, ok)
			assert.Equal(t, "Compare base: "+tt.want, toast.Message)
		})
	}
}

func TestCompareBaseKeyClearsPinnedRef(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabBranches
	p.compareBase = "feature"
	p.tabItems[tabBranches] = []listItem{{kind: kindLocalBranch, branch: git.Branch{Name: "feature"}}}

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: '='})
	msgs := gitinfoCompareBaseMessages(t, cmd)

	require.Len(t, msgs, 2)
	_, ok := msgs[0].(panels.ClearCompareBaseMsg)
	require.True(t, ok)
	toast, ok := msgs[1].(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, "Compare base cleared", toast.Message)
}

func TestCompareBaseKeyIgnoresInvalidContexts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode PanelMode
		tab  tabID
		item listItem
	}{
		{
			name: "bare remote",
			mode: ModeGit,
			tab:  tabRemotes,
			item: listItem{kind: kindRemote, remote: git.Remote{Name: "origin"}},
		},
		{
			name: "github mode",
			mode: ModeGitHub,
			tab:  tabBranches,
			item: listItem{kind: kindRemoteBranch, branch: git.Branch{Name: "origin/main", IsRemote: true}},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			p.Focused = true
			p.mode = tt.mode
			p.activeTab = tt.tab
			p.tabItems[tt.tab] = []listItem{tt.item}

			_, cmd := p.handleKey(tea.KeyPressMsg{Code: '='})

			assert.Nil(t, cmd)
		})
	}
}
