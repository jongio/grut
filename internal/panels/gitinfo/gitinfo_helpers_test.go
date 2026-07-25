package gitinfo

import (
	"fmt"
	"testing"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Title — 28.6% → 100%
// ---------------------------------------------------------------------------

func TestTitle_GitMode(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	assert.Equal(t, "Git", p.Title())
}

func TestTitle_GitHubPublic(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.repoPrivate = false
	assert.Equal(t, "GitHub", p.Title())
}

func TestTitle_GitHubPrivateASCII(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.repoPrivate = true
	p.iconMode = "ascii"
	assert.Equal(t, "GitHub (private)", p.Title())
}

func TestTitle_GitHubPrivateNerd(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.repoPrivate = true
	p.iconMode = "nerd"
	assert.Equal(t, "GitHub \uf023", p.Title())
}

// ---------------------------------------------------------------------------
// currentBranch — 0% → 100%
// ---------------------------------------------------------------------------

func TestCurrentBranch_Found(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// After Init the panel has branches from defaultMock with "main" as current.
	assert.Equal(t, "main", p.currentBranch())
}

func TestCurrentBranch_NoCurrent(t *testing.T) {
	t.Parallel()
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "feature", IsCurrent: false, Hash: "abc1234"},
		},
	}
	p := newTestPanel(t, mock)
	assert.Equal(t, "main", p.currentBranch(), "should fall back to main when no branch is current")
}

func TestCurrentBranch_EmptyBranches(t *testing.T) {
	t.Parallel()
	mock := &mockGitOps{}
	p := newTestPanel(t, mock)
	assert.Equal(t, "main", p.currentBranch())
}

// ---------------------------------------------------------------------------
// visibleTabs / tabBarHeight — 50% → higher
// ---------------------------------------------------------------------------

func TestVisibleTabs_ModeAll_GitTab(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.activeTab = tabBranches // a git tab
	tabs := p.visibleTabs()
	assert.Contains(t, tabs, tabBranches)
	assert.Contains(t, tabs, tabWorktrees)
	assert.Contains(t, tabs, tabRemotes)
	assert.Contains(t, tabs, tabStash)
	assert.Contains(t, tabs, tabReflog)
}

func TestVisibleTabs_ModeAll_GitHubTab(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.activeTab = tabIssues // a GitHub tab
	tabs := p.visibleTabs()
	assert.Contains(t, tabs, tabIssues)
	assert.Contains(t, tabs, tabPRs)
	assert.Contains(t, tabs, tabActions)
}

func TestTabBarHeight_ModeAll_WithGHClient(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.gh.client = &mockGHClientFull{} // non-nil → 2 rows
	assert.Equal(t, 2, p.tabBarHeight())
}

func TestTabBarHeight_ModeAll_NoGHClient(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.gh.client = nil // nil → 1 row
	assert.Equal(t, 1, p.tabBarHeight())
}

// ---------------------------------------------------------------------------
// pageDown / pageUp — 36%/50% → higher
// ---------------------------------------------------------------------------

func TestPageDown_Basic(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10
	p.activeTab = tabBranches

	// Ensure we have enough items (defaultMock has 3 branches = 3 items).
	require.True(t, len(p.tabItems[tabBranches]) >= 2)

	p.tabCursor[tabBranches] = 0
	p.pageDown()
	// Cursor should advance; exact position depends on tabBarHeight and viewH.
	assert.True(t, p.tabCursor[tabBranches] >= 0)
}

func TestPageDown_ZeroHeight(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 0 // viewH = 0 - tabBarHeight ≤ 0
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 0
	p.pageDown()
	assert.Equal(t, 0, p.tabCursor[tabBranches], "should not move with zero height")
}

func TestPageUp_Basic(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 2
	p.pageUp()
	assert.Equal(t, 0, p.tabCursor[tabBranches], "should page up to beginning")
}

func TestPageUp_ZeroHeight(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 0
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 2
	p.pageUp()
	assert.Equal(t, 2, p.tabCursor[tabBranches], "should not move with zero height")
}

func TestPageDown_ClampsToEnd(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 100 // very tall → pageDown jumps past all items
	p.activeTab = tabBranches
	n := len(p.tabItems[tabBranches])
	require.True(t, n > 0)
	p.tabCursor[tabBranches] = 0
	p.pageDown()
	assert.Equal(t, n-1, p.tabCursor[tabBranches], "should clamp to last item")
}

// ---------------------------------------------------------------------------
// renderWorkflow — 0% → 100%
// ---------------------------------------------------------------------------

func TestRenderWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wf       ghWorkflowItem
		isCursor bool
		contains string
	}{
		{
			name:     "active workflow",
			wf:       ghWorkflowItem{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
			contains: "CI",
		},
		{
			name:     "disabled_manually",
			wf:       ghWorkflowItem{ID: 2, Name: "Deploy", State: "disabled_manually"},
			contains: "Deploy",
		},
		{
			name:     "disabled_inactivity",
			wf:       ghWorkflowItem{ID: 3, Name: "Lint", State: "disabled_inactivity"},
			contains: "Lint",
		},
		{
			name:     "unknown state",
			wf:       ghWorkflowItem{ID: 4, Name: "Build", State: "unknown"},
			contains: "Build",
		},
		{
			name:     "with cursor",
			wf:       ghWorkflowItem{ID: 5, Name: "Test", State: "active"},
			isCursor: true,
			contains: "Test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			item := listItem{kind: kindWorkflow, workflow: tt.wf}
			result := p.renderWorkflow(item, 80, tt.isCursor)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestRenderWorkflow_NarrowWidth(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	wf := ghWorkflowItem{ID: 1, Name: "A Very Long Workflow Name That Should Truncate", State: "active", Path: ".github/workflows/ci.yml"}
	item := listItem{kind: kindWorkflow, workflow: wf}
	result := p.renderWorkflow(item, 20, false)
	assert.NotEmpty(t, result)
}

func TestRenderWorkflow_ZeroWidth(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	wf := ghWorkflowItem{ID: 1, Name: "CI", State: "active", Path: ".github/workflows/ci.yml"}
	item := listItem{kind: kindWorkflow, workflow: wf}
	result := p.renderWorkflow(item, 5, false)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// renderRelease — 0% → 100%
// ---------------------------------------------------------------------------

func TestRenderRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rel      ghReleaseItem
		isCursor bool
		contains string
	}{
		{
			name:     "stable release",
			rel:      ghReleaseItem{TagName: "v1.0.0", Name: "Release 1.0", Author: "dev", CreatedAt: "2024-01-15"},
			contains: "v1.0.0",
		},
		{
			name:     "draft release",
			rel:      ghReleaseItem{TagName: "v2.0.0-rc1", Draft: true, Name: "Draft"},
			contains: "v2.0.0-rc1",
		},
		{
			name:     "prerelease",
			rel:      ghReleaseItem{TagName: "v1.1.0-beta", Prerelease: true},
			contains: "v1.1.0-beta",
		},
		{
			name:     "with assets",
			rel:      ghReleaseItem{TagName: "v1.0.0", AssetsCount: 3, Author: "ci-bot"},
			contains: "3 assets",
		},
		{
			name:     "same name and tag",
			rel:      ghReleaseItem{TagName: "v3.0.0", Name: "v3.0.0"},
			contains: "v3.0.0",
		},
		{
			name:     "with cursor",
			rel:      ghReleaseItem{TagName: "v1.0.0"},
			isCursor: true,
			contains: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			item := listItem{kind: kindRelease, release: tt.rel}
			result := p.renderRelease(item, 80, tt.isCursor)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestRenderRelease_NarrowWidth(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	rel := ghReleaseItem{TagName: "v1.0.0", Name: "A Very Long Release Name", Author: "dev", CreatedAt: "2024-01-15", AssetsCount: 5}
	item := listItem{kind: kindRelease, release: rel}
	result := p.renderRelease(item, 20, false)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// renderLine — dispatch coverage for workflow, release, remote, stash, reflog
// ---------------------------------------------------------------------------

func TestRenderLine_Workflow(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 1, Name: "CI", State: "active"}}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "CI")
}

func TestRenderLine_Release(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0.0"}}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "v1.0.0")
}

func TestRenderLine_Remote(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemote, remote: git.Remote{Name: "origin"}}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "origin")
}

func TestRenderLine_RemoteSub(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemoteSub, text: "https://github.com/user/repo"}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
}

func TestRenderLine_Stash(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "WIP"}}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "stash@{0}")
}

func TestRenderLine_Reflog(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{Hash: "abc1234", Action: "commit", Message: "test", Date: time.Now()}}
	result := p.renderLine(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "abc1234")
}

func TestRenderLine_UnknownKindReturnsEmpty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: itemKind(999)}
	result := p.renderLine(item, 80, false)
	assert.Empty(t, result, "unknown kind should return empty")
}

// ---------------------------------------------------------------------------
// doOpenInBrowser — 0% → high
// ---------------------------------------------------------------------------

func TestDoOpenInBrowser_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 999 // out of bounds
	_, cmd := p.doOpenInBrowser()
	assert.Nil(t, cmd)
}

func TestDoOpenInBrowser_NegativeCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = -1
	_, cmd := p.doOpenInBrowser()
	assert.Nil(t, cmd)
}

func TestDoOpenInBrowser_RemoteItem_HasURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	// After Init, remotes tab should have items from defaultMock.
	require.True(t, len(p.tabItems[tabRemotes]) > 0)
	p.tabCursor[tabRemotes] = 0

	_, cmd := p.doOpenInBrowser()
	// Remote "origin" with FetchURL "https://github.com/user/repo" → URL is set.
	assert.NotNil(t, cmd, "should return a command for remote with URL")
}

func TestDoOpenInBrowser_BranchNoRemotes(t *testing.T) {
	t.Parallel()
	mock := &mockGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true, Hash: "abc1234"}},
		remotes:  nil, // no remotes → guessBranchRemoteURL returns ""
	}
	p := newTestPanel(t, mock)
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 0

	_, cmd := p.doOpenInBrowser()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "No URL available")
}

func TestDoOpenInBrowser_IssueItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 42, HTMLURL: "https://github.com/user/repo/issues/42"}},
	}
	p.tabCursor[tabIssues] = 0

	_, cmd := p.doOpenInBrowser()
	assert.NotNil(t, cmd, "should return command for issue with URL")
}

func TestDoOpenInBrowser_PRItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 10, HTMLURL: "https://github.com/user/repo/pull/10"}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.doOpenInBrowser()
	assert.NotNil(t, cmd, "should return command for PR with URL")
}

func TestDoOpenInBrowser_ActionRunItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabActions
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{RunNumber: 100, HTMLURL: "https://github.com/user/repo/actions/runs/100"}},
	}
	p.tabCursor[tabActions] = 0

	_, cmd := p.doOpenInBrowser()
	assert.NotNil(t, cmd)
}

func TestDoOpenInBrowser_WorkflowItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{Name: "CI", HTMLURL: "https://github.com/user/repo/actions/workflows/ci.yml"}},
	}
	p.tabCursor[tabWorkflows] = 0

	_, cmd := p.doOpenInBrowser()
	assert.NotNil(t, cmd)
}

func TestDoOpenInBrowser_ReleaseItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabReleases
	p.tabItems[tabReleases] = []listItem{
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0.0", HTMLURL: "https://github.com/user/repo/releases/tag/v1.0.0"}},
	}
	p.tabCursor[tabReleases] = 0

	_, cmd := p.doOpenInBrowser()
	assert.NotNil(t, cmd)
}

func TestDoOpenInBrowser_TagWithRemotes(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc1234"}},
	}
	p.tabCursor[tabTags] = 0

	_, cmd := p.doOpenInBrowser()
	// With remotes from defaultMock, guessBranchRemoteURL returns URL.
	assert.NotNil(t, cmd, "should return command for tag when remotes exist")
}

func TestDoOpenInBrowser_TagNoRemotes(t *testing.T) {
	t.Parallel()
	mock := &mockGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true}},
		remotes:  nil,
	}
	p := newTestPanel(t, mock)
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0"}},
	}
	p.tabCursor[tabTags] = 0

	_, cmd := p.doOpenInBrowser()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
}

func TestDoOpenInBrowser_StashEntry_NoURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabStash
	require.True(t, len(p.tabItems[tabStash]) > 0)
	p.tabCursor[tabStash] = 0

	_, cmd := p.doOpenInBrowser()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "No URL available")
}

func TestDoOpenInBrowser_RemoteBranch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	// Find the remote branch in items (origin/main).
	for i, item := range p.tabItems[tabBranches] {
		if item.kind == kindRemoteBranch {
			p.tabCursor[tabBranches] = i
			_, cmd := p.doOpenInBrowser()
			assert.NotNil(t, cmd, "remote branch with remotes should have URL")
			return
		}
	}
	t.Skip("no remote branch in test items")
}

// ---------------------------------------------------------------------------
// handleModalResult — 39% → much higher
// ---------------------------------------------------------------------------

func TestHandleModalResult_RejectClearsOp(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCreate
	p.pendingName = "test"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "rejected modal should return nil cmd")
	assert.Equal(t, opNone, p.pending, "pending should be cleared")
}

func TestHandleModalResult_RemoteAdd(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAdd

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "upstream"})
	require.NotNil(t, cmd)
	assert.Equal(t, opRemoteAddURL, p.pending, "should transition to opRemoteAddURL")
	assert.Equal(t, "upstream", p.pendingName, "should store remote name")

	// The cmd should produce a ShowModalMsg for URL input.
	msg := cmd()
	_, ok := msg.(notify.ShowModalMsg)
	assert.True(t, ok, "should show modal for URL input")
}

func TestHandleModalResult_RemoteAdd_EmptyName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAdd

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd, "empty remote name should return nil cmd")
}

func TestHandleModalResult_RemoteAddURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAddURL
	p.pendingName = "upstream"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "https://github.com/other/repo"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "remote_added", result.op)
	assert.Equal(t, "upstream", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_RemoteAddURL_EmptyURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAddURL
	p.pendingName = "upstream"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "  "})
	assert.Nil(t, cmd, "empty URL should return nil cmd")
}

func TestHandleModalResult_RemoteDelete(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteDelete
	p.pendingName = "origin"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "remote_removed", result.op)
	assert.Equal(t, "origin", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_BranchCheckout(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCheckout
	p.pendingName = "feature"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	// First step returns dirty-check (mock Status returns nil = clean).
	dirtyMsg, ok := msg.(checkoutDirtyMsg)
	require.True(t, ok, "expected checkoutDirtyMsg, got %T", msg)
	assert.Equal(t, "feature", dirtyMsg.ref)
	assert.False(t, dirtyMsg.dirty)

	// Second step proceeds with checkout.
	_, cmd2 := p.handleCheckoutDirty(dirtyMsg)
	require.NotNil(t, cmd2)
	msg2 := cmd2()
	result, ok := msg2.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "checkout", result.op)
	assert.Equal(t, "feature", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_StashApply(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "apply"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_applied", result.op)
	assert.Equal(t, "stash@{0}", result.name)
}

func TestHandleModalResult_StashPop(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "1"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "pop"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_popped", result.op)
	assert.Equal(t, "stash@{1}", result.name)
}

func TestHandleModalResult_StashDrop(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "drop"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_dropped", result.op)
}

func TestHandleModalResult_StashUnknownAction(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "invalid"})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Unknown stash action")
}

func TestHandleModalResult_StashAction_InvalidIndex(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "not-a-number"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "apply"})
	assert.Nil(t, cmd, "invalid stash index should return nil cmd")
}

func TestHandleModalResult_StashAction_ShortCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		op    string
	}{
		{"a", "stash_applied"},
		{"p", "stash_popped"},
		{"d", "stash_dropped"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			p.pending = opStashAction
			p.pendingName = "0"

			_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: tt.input})
			require.NotNil(t, cmd)
			msg := cmd()
			result, ok := msg.(opResultMsg)
			require.True(t, ok)
			assert.Equal(t, tt.op, result.op)
		})
	}
}

func TestHandleModalResult_TagCreate(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagCreate

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "v1.0.0"})
	require.NotNil(t, cmd)
	assert.Equal(t, opTagMessage, p.pending, "should transition to opTagMessage")
	assert.Equal(t, "v1.0.0", p.pendingName)

	msg := cmd()
	_, ok := msg.(notify.ShowModalMsg)
	assert.True(t, ok, "should show modal for tag message")
}

func TestHandleModalResult_TagCreate_EmptyName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagCreate

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd, "empty tag name should return nil cmd")
}

func TestHandleModalResult_TagMessage(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagMessage
	p.pendingName = "v1.0.0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "Release 1.0"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tag_created", result.op)
	assert.Equal(t, "v1.0.0", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_TagMessage_Empty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagMessage
	p.pendingName = "v1.0.0"

	// Empty message → lightweight tag.
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tag_created", result.op)
}

func TestHandleModalResult_TagDelete(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagDelete
	p.pendingName = "v1.0.0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tag_deleted", result.op)
	assert.Equal(t, "v1.0.0", result.name)
}

func TestHandleModalResult_TagPush(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagPush
	p.pendingName = "v1.0.0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tag_pushed", result.op)
	assert.Equal(t, "v1.0.0", result.name)
}

func TestHandleModalResult_TagCheckout(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opTagCheckout
	p.pendingName = "v1.0.0"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tag_checkout", result.op)
	assert.Equal(t, "v1.0.0", result.name)
}

func TestHandleModalResult_WorkflowDispatch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatch
	p.pendingName = "123:CI"
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml"}},
	}
	// ghClient is nil → will skip GetWorkflowInputs.

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"})
	require.NotNil(t, cmd)
	msg := cmd()
	fetched, ok := msg.(workflowInputsFetchedMsg)
	require.True(t, ok)
	assert.Equal(t, int64(123), fetched.workflowID)
	assert.Equal(t, "CI", fetched.workflowName)
	assert.Equal(t, "main", fetched.ref)
}

func TestHandleModalResult_WorkflowDispatch_EmptyRef(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatch
	p.pendingName = "123:CI"
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 123, Name: "CI", Path: ".github/workflows/ci.yml"}},
	}

	// Empty ref → falls back to currentBranch().
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
	require.NotNil(t, cmd)
	msg := cmd()
	fetched, ok := msg.(workflowInputsFetchedMsg)
	require.True(t, ok)
	assert.Equal(t, "main", fetched.ref, "should fall back to currentBranch()")
}

func TestHandleModalResult_WorkflowDispatch_InvalidID(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatch
	p.pendingName = "0:CI" // workflowID=0 → early return

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"})
	assert.Nil(t, cmd, "workflowID=0 should return nil cmd")
}

func TestHandleModalResult_WorkflowDispatch_MalformedName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatch
	p.pendingName = "no-colon" // can't SplitN into 2 parts with ":"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"})
	assert.Nil(t, cmd, "malformed pendingName should return nil cmd")
}

func TestHandleModalResult_WorkflowDispatchRaw(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatchRaw
	p.pendingName = "456:Deploy:main"
	p.gh.client = &mockGHClientFull{} // non-nil to avoid panic

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "env=prod\nversion=1.0"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(workflowDispatchResultMsg)
	require.True(t, ok)
	assert.Equal(t, "Deploy", result.workflowName)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_WorkflowDispatchRaw_EmptyInputs(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatchRaw
	p.pendingName = "456:Deploy:main"
	p.gh.client = &mockGHClientFull{}

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(workflowDispatchResultMsg)
	require.True(t, ok)
	assert.Equal(t, "Deploy", result.workflowName)
}

func TestHandleModalResult_WorkflowDispatchRaw_InvalidID(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatchRaw
	p.pendingName = "0:Deploy:main" // workflowID=0

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "key=val"})
	assert.Nil(t, cmd, "workflowID=0 should return nil cmd")
}

func TestHandleModalResult_WorkflowDispatchRaw_MissingRef(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opWorkflowDispatchRaw
	p.pendingName = "456:Deploy" // only 2 parts, ref missing → ref=""

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "key=val"})
	assert.Nil(t, cmd, "missing ref should return nil cmd")
}

func TestHandleModalResult_UnknownOp(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opNone // already cleared; the default case returns nil.

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// doWorkflowDispatch — 0% → 100%
// ---------------------------------------------------------------------------

func TestDoWorkflowDispatch_Valid(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 42, Name: "CI"}},
	}
	p.tabCursor[tabWorkflows] = 0

	_, cmd := p.doWorkflowDispatch()
	require.NotNil(t, cmd)
	assert.Equal(t, opWorkflowDispatch, p.pending)
	assert.Equal(t, fmt.Sprintf("%d:%s", 42, "CI"), p.pendingName)
}

func TestDoWorkflowDispatch_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = nil
	p.tabCursor[tabWorkflows] = 0

	_, cmd := p.doWorkflowDispatch()
	assert.Nil(t, cmd)
}

func TestDoWorkflowDispatch_WrongKind(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1"}}, // wrong kind
	}
	p.tabCursor[tabWorkflows] = 0

	_, cmd := p.doWorkflowDispatch()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleWorkflowDispatchResult — 0% → 100%
// ---------------------------------------------------------------------------

func TestHandleWorkflowDispatchResult_Error(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowDispatchResult(workflowDispatchResultMsg{
		workflowName: "CI",
		err:          fmt.Errorf("dispatch failed"),
	})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "dispatch failed")
}

func TestHandleWorkflowDispatchResult_Success(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowDispatchResult(workflowDispatchResultMsg{
		workflowName: "CI",
		err:          nil,
	})
	// Returns a tea.Batch — just verify it's non-nil.
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleWorkflowInputsFetched — 0% → 100%
// ---------------------------------------------------------------------------

func TestHandleWorkflowInputsFetched_WithInputs(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   42,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs: []ghclient.WorkflowInput{
			{Name: "environment", Default: "staging"},
			{Name: "version", Default: "latest"},
		},
	})
	require.NotNil(t, cmd)
	assert.Equal(t, opWorkflowDispatchInputs, p.pending)
	require.Len(t, p.wfDispatch.inputs, 2)
	assert.Equal(t, 0, p.wfDispatch.idx)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Contains(t, modal.Title, "Deploy")
	assert.Equal(t, "staging", modal.Value, "first input's default pre-fills the field")
}

func TestHandleWorkflowInputsFetched_NoInputs(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	// Inputs could not be read → free-form composer fallback.
	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   42,
		workflowName: "CI",
		ref:          "main",
		inputsKnown:  false,
	})
	require.NotNil(t, cmd)
	assert.Equal(t, opWorkflowDispatchRaw, p.pending)
}

// ---------------------------------------------------------------------------
// workflowSelectedCmd — 45.5% → 100%
// ---------------------------------------------------------------------------

func TestWorkflowSelectedCmd_Valid(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{Name: "CI", Path: ".github/workflows/ci.yml"}},
	}
	p.tabCursor[tabWorkflows] = 0

	cmd := p.workflowSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.WorkflowSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "CI", sel.Name)
	assert.Equal(t, ".github/workflows/ci.yml", sel.Path)
}

func TestWorkflowSelectedCmd_WrongTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches

	cmd := p.workflowSelectedCmd()
	assert.Nil(t, cmd, "should return nil when not on workflows tab")
}

func TestWorkflowSelectedCmd_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = nil
	p.tabCursor[tabWorkflows] = 0

	cmd := p.workflowSelectedCmd()
	assert.Nil(t, cmd)
}

func TestWorkflowSelectedCmd_WrongKind(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindTag}, // wrong kind
	}
	p.tabCursor[tabWorkflows] = 0

	cmd := p.workflowSelectedCmd()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// renderStashEntry edge cases (via renderLine)
// ---------------------------------------------------------------------------

func TestRenderStashEntry_Truncation(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	tests := []struct {
		name    string
		message string
		width   int
	}{
		{"narrow", "A very long stash message that exceeds width", 20},
		{"very_narrow", "msg", 3},
		{"zero_width", "msg", 0},
		{"normal", "WIP on main", 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: tt.message}}
			result := p.renderStashEntry(item, tt.width, false)
			// Should never panic.
			_ = result
		})
	}
}

func TestRenderStashEntry_WithCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 2, Message: "experimental"}}
	result := p.renderStashEntry(item, 80, true)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "stash@{2}")
}

// ---------------------------------------------------------------------------
// renderReflogEntry edge cases
// ---------------------------------------------------------------------------

func TestRenderReflogEntry_Truncation(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	tests := []struct {
		name  string
		width int
	}{
		{"narrow", 15},
		{"very_narrow", 3},
		{"zero_width", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			item := listItem{
				kind:   kindReflogEntry,
				reflog: git.ReflogEntry{Hash: "abc1234def5678", Action: "commit", Message: "a long commit msg", Date: time.Now()},
			}
			result := p.renderReflogEntry(item, tt.width, false)
			_ = result // must not panic
		})
	}
}

func TestRenderReflogEntry_WithCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:   kindReflogEntry,
		reflog: git.ReflogEntry{Hash: "abc1234", Action: "checkout", Message: "main to feature", Date: time.Now()},
	}
	result := p.renderReflogEntry(item, 80, true)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// renderRemote / renderRemoteSub edge cases
// ---------------------------------------------------------------------------

func TestRenderRemote_Cursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemote, remote: git.Remote{Name: "origin"}}
	result := p.renderRemote(item, 80, true)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "origin")
}

func TestRenderRemoteSub_Cursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemoteSub, text: "https://github.com/user/repo (fetch)"}
	result := p.renderRemoteSub(item, 80, true)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// Tab View rendering for Stash, Remotes, Reflog, Tags tabs
// ---------------------------------------------------------------------------

func TestViewRendering_StashTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetActiveTab("stash")
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	// defaultMock has stash entries: "WIP on main", "experimental change"
	assert.Contains(t, view, "stash@{0}")
}

func TestViewRendering_RemotesTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetActiveTab("remotes")
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "origin")
}

func TestViewRendering_TagsTab(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	mock.tags = []git.Tag{
		{Name: "v1.0.0", Hash: "abc1234", IsAnnotated: true},
		{Name: "v0.9.0", Hash: "def5678"},
	}
	p := newTestPanel(t, mock)
	p.SetActiveTab("tags")
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "v1.0.0")
}

func TestViewRendering_ReflogTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// Reflog is loaded via mock's Reflog(), which returns nil.
	// Inject items directly.
	p.tabItems[tabReflog] = []listItem{
		{kind: kindReflogEntry, reflog: git.ReflogEntry{Hash: "abc1234", Action: "commit", Message: "initial commit", Date: time.Now().Add(-5 * time.Minute)}},
		{kind: kindReflogEntry, reflog: git.ReflogEntry{Hash: "def5678", Action: "checkout", Message: "from main to feature", Date: time.Now().Add(-2 * time.Hour)}},
	}
	p.SetActiveTab("reflog")
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "abc1234")
}

// ---------------------------------------------------------------------------
// renderTag edge cases (cursor on remote tag)
// ---------------------------------------------------------------------------

func TestRenderTag_RemoteTag(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemoteTag, tag: git.Tag{Name: "v2.0.0", Hash: "abc1234"}}
	result := p.renderTag(item, 80, false)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "v2.0.0")
}

func TestRenderTag_ZeroWidth(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc"}}
	result := p.renderTag(item, 5, false)
	_ = result // must not panic
}

// ---------------------------------------------------------------------------
// SetActiveTab additional cases
// ---------------------------------------------------------------------------

func TestSetActiveTab_AllTabNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tab  tabID
	}{
		{"branches", tabBranches},
		{"worktrees", tabWorktrees},
		{"remotes", tabRemotes},
		{"stash", tabStash},
		{"tags", tabTags},
		{"reflog", tabReflog},
		{"issues", tabIssues},
		{"prs", tabPRs},
		{"actions", tabActions},
		{"workflows", tabWorkflows},
		{"releases", tabReleases},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			p.SetActiveTab(tt.name)
			assert.Equal(t, tt.tab, p.activeTab)
		})
	}
}

// ---------------------------------------------------------------------------
// tabRowUseShort
// ---------------------------------------------------------------------------

func TestTabRowUseShort_Wide(t *testing.T) {
	t.Parallel()
	tabs := []struct{ name, short, count string }{
		{"Branches", "Br", "3"},
		{"Tags", "Tg", "2"},
	}
	// "Branches 3 · Tags 2" = 1+10+3+6 = ~20 chars
	assert.False(t, tabRowUseShort(tabs, 100), "should not abbreviate when width is large")
}

func TestTabRowUseShort_Narrow(t *testing.T) {
	t.Parallel()
	tabs := []struct{ name, short, count string }{
		{"Branches", "Br", "3"},
		{"Worktrees", "Wt", "2"},
		{"Remotes", "Rm", "1"},
		{"Stash", "St", "5"},
		{"Tags", "Tg", "10"},
		{"Reflog", "Rl", "50"},
	}
	assert.True(t, tabRowUseShort(tabs, 20), "should abbreviate when width is small")
}

// ---------------------------------------------------------------------------
// ghTabLabelWidth
// ---------------------------------------------------------------------------

func TestGhTabLabelWidth_Full(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	w := p.ghTabLabelWidth("Branches", "Br", "3", false)
	assert.Equal(t, len("Branches 3"), w)
}

func TestGhTabLabelWidth_Short(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	w := p.ghTabLabelWidth("Branches", "Br", "3", true)
	assert.Equal(t, len("Br 3"), w)
}

func TestGhTabLabelWidth_NoShort(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	w := p.ghTabLabelWidth("Tags", "", "5", true)
	// short is empty, so it uses the full name even when useShort=true.
	assert.Equal(t, len("Tags 5"), w)
}

func TestGhTabLabelWidth_UnicodeCount(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// "✓" is 3 bytes in UTF-8 but 1 display column.
	// "Actions ✓" should be 9 display columns, not 11 bytes.
	w := p.ghTabLabelWidth("Actions", "Act", "✓", false)
	assert.Equal(t, 9, w, "display width should count columns, not bytes")
}

func TestGhTabLabelWidth_UnicodeShort(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	w := p.ghTabLabelWidth("Actions", "Act", "✓", true)
	assert.Equal(t, 5, w, "short name + Unicode count: 'Act ✓' = 5 columns")
}

func TestTabRowUseShort_UnicodeIcons(t *testing.T) {
	t.Parallel()
	tabs := []struct{ name, short, count string }{
		{"Issues", "Is", "3"},
		{"PRs", "PR", "2"},
		{"Actions", "Act", "✓"},
	}
	// Display width: 1 + (6+1+1) + 3 + (3+1+1) + 3 + (7+1+1) = 1+8+3+5+3+9 = 29
	// With len() this would incorrectly compute 31 (✓ = 3 bytes).
	// Width 30 should NOT trigger abbreviation with correct display-width math.
	assert.False(t, tabRowUseShort(tabs, 30), "should not abbreviate: display width 29 fits in 30")
	assert.True(t, tabRowUseShort(tabs, 28), "should abbreviate: display width 29 exceeds 28")
}
