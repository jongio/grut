package gitinfo

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPRCheckoutPanel(t *testing.T, mock *mockGitOps, pr ghPRItem) *Panel {
	t.Helper()
	p := newTestGitHubPanel(t, mock)
	p.Focused = true
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{{kind: kindPR, pr: pr}}
	p.tabCursor[tabPRs] = 0
	return p
}

func TestPRCheckoutKeyShowsPicker(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	pr := ghPRItem{Number: 42, Title: "Fix bug", HeadBranch: "fix-bug"}
	p := testPRCheckoutPanel(t, mock, pr)

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: 'c'})
	require.NotNil(t, cmd)
	assert.Equal(t, opPRCheckout, p.pending)
	assert.Equal(t, pr.Number, p.pendingPRCheckout.Number)
	assert.Empty(t, mock.fetches)
	assert.Empty(t, mock.checkouts)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, "Check out PR #42", modal.Title)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
}

func TestPRCheckoutCurrentSameRepoFetchesAndChecksOutHeadBranch(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	pr := ghPRItem{Number: 12, Title: "Feature", HeadBranch: "feature-branch"}
	p := testPRCheckoutPanel(t, mock, pr)
	p.pending = opPRCheckout
	p.pendingPRCheckout = pr

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: actionPRCheckoutCurrent})
	require.NotNil(t, cmd)
	result, ok := cmd().(opResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	assert.Equal(t, eventPRCheckout, result.op)
	assert.Equal(t, "feature-branch", result.name)
	require.Len(t, mock.fetches, 1)
	assert.Equal(t, git.FetchOpts{Remote: remoteOrigin, Refspec: "pull/12/head:feature-branch"}, mock.fetches[0])
	assert.Equal(t, []string{"feature-branch"}, mock.checkouts)

	_, cmd = p.handleOpResult(result)
	msgs := collectCmdMsgs(cmd)
	assert.Contains(t, msgs, panels.BranchChangedMsg{Name: "feature-branch"})
}

func TestPRCheckoutCurrentForkUsesPullRefPRBranch(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	pr := ghPRItem{Number: 34, Title: "Fork fix", HeadBranch: "contrib/fix", IsFork: true}
	p := testPRCheckoutPanel(t, mock, pr)
	p.pending = opPRCheckout
	p.pendingPRCheckout = pr

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: actionPRCheckoutCurrent})
	require.NotNil(t, cmd)
	result, ok := cmd().(opResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	require.Len(t, mock.fetches, 1)
	assert.Equal(t, "pull/34/head:pr-34", mock.fetches[0].Refspec)
	assert.Equal(t, []string{"pr-34"}, mock.checkouts)
}

func TestPRCheckoutDirtyCurrentRefusedSiblingAllowed(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	mock.status = []git.FileStatus{{Path: "changed.go"}}
	pr := ghPRItem{Number: 56, Title: "Dirty ok", HeadBranch: "dirty-branch"}
	p := testPRCheckoutPanel(t, mock, pr)
	p.pending = opPRCheckout
	p.pendingPRCheckout = pr

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: actionPRCheckoutCurrent})
	require.NotNil(t, cmd)
	result, ok := cmd().(opResultMsg)
	require.True(t, ok)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "dirty worktree")
	assert.Empty(t, mock.fetches)
	assert.Empty(t, mock.checkouts)

	p.pending = opPRCheckout
	p.pendingPRCheckout = pr
	_, cmd = p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: actionPRCheckoutWorktree})
	require.NotNil(t, cmd)
	result, ok = cmd().(opResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	require.Len(t, mock.fetches, 1)
	require.Len(t, mock.worktreeAdds, 1)
	assert.Equal(t, "pull/56/head:dirty-branch", mock.fetches[0].Refspec)
	assert.Equal(t, "dirty-branch", mock.worktreeAdds[0].branch)
	assert.Equal(t, prWorktreePath(p.repoRoot, 56), mock.worktreeAdds[0].path)

	_, cmd = p.handleOpResult(result)
	msgs := collectCmdMsgs(cmd)
	assert.Contains(t, msgs, panels.ChangeDirectoryMsg{Path: filepath.Clean(prWorktreePath(p.repoRoot, 56))})
}
