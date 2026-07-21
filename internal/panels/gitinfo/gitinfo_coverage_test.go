package gitinfo

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// loadGitHubData — partial API failures (graceful degradation)
// ---------------------------------------------------------------------------

func TestLoadGitHubData_IssuesFailOtherSucceed(t *testing.T) {
	t.Parallel()
	login := "user1"
	prNum := 1
	prTitle := "fix"
	prState := "open"
	headRef := "fix-branch"

	ghMock := &mockGHClientFull{
		user:      ghUser(login),
		issuesErr: assert.AnError, // issues fail
		prs: []*gh.PullRequest{
			{Number: &prNum, Title: &prTitle, State: &prState, Head: &gh.PullRequestBranch{Ref: &headRef}, User: ghUser("u")},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result, ok := msg.(ghDataLoadedMsg)
	require.True(t, ok)

	assert.Equal(t, login, result.user)
	assert.Empty(t, result.issues, "issues should be empty on error")
	assert.Len(t, result.prs, 1, "PRs should still load")
	assert.Equal(t, "fix", result.prs[0].Title)
}

func TestLoadGitHubData_PRsFailOtherSucceed(t *testing.T) {
	t.Parallel()
	login := "user2"
	issNum := 10
	issTitle := "bug"
	issState := "open"

	ghMock := &mockGHClientFull{
		user:   ghUser(login),
		prsErr: assert.AnError,
		issues: []*gh.Issue{
			{Number: &issNum, Title: &issTitle, State: &issState},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)

	assert.Len(t, result.issues, 1, "issues should still load")
	assert.Empty(t, result.prs, "PRs should be empty on error")
}

func TestLoadGitHubData_RunsFailOtherSucceed(t *testing.T) {
	t.Parallel()
	prNum := 5
	prTitle := "feat"
	prState := "open"
	headRef := "feat-branch"

	ghMock := &mockGHClientFull{
		user:    ghUser("u"),
		runsErr: assert.AnError,
		prs: []*gh.PullRequest{
			{Number: &prNum, Title: &prTitle, State: &prState, Head: &gh.PullRequestBranch{Ref: &headRef}},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)

	assert.Len(t, result.prs, 1, "PRs should still load")
	assert.Empty(t, result.actions, "actions should be empty on error")
}

func TestLoadGitHubData_UserFailOtherSucceed(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{
		userErr: assert.AnError,
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)

	assert.Empty(t, result.user, "user should be empty on error")
	assert.Empty(t, result.issues, "no data, but no panic")
}

// ---------------------------------------------------------------------------
// handleRepoChanged — verify all GitHub state is reset
// ---------------------------------------------------------------------------

func TestHandleRepoChanged_ResetsGitHubState(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	// Pre-populate GitHub state to verify it gets cleared.
	p.gh.owner = "old-owner"
	p.gh.repo = "old-repo"
	p.gh.user = "old-user"
	p.gh.client = &mockGHClientFull{}
	p.gh.repoPrivate = true
	p.gh.allIssues = []ghIssueItem{{Number: 1, Title: "stale"}}
	p.gh.allPRs = []ghPRItem{{Number: 2, Title: "stale"}}
	p.actionsWatching = true
	p.actionsWatchFrame = 5

	// Populate some tab data.
	p.tabItems[tabIssues] = []listItem{{kind: kindIssue, issue: ghIssueItem{Number: 1}}}
	p.tabItems[tabPRs] = []listItem{{kind: kindPR, pr: ghPRItem{Number: 2}}}
	p.tabItems[tabActions] = []listItem{{kind: kindActionRun, actionRun: ghActionItem{RunID: 100}}}
	p.tabCursor[tabIssues] = 5
	p.tabOffset[tabPRs] = 3
	p.tabPaging[tabIssues] = tabPagination{nextPage: 3, allLoaded: false}

	// Trigger repo change (path won't resolve to a real git repo, that's fine for this test).
	p.handleRepoChanged(panels.RepoChangedMsg{Path: "/nonexistent/path"})

	// Verify GitHub state is reset.
	assert.Empty(t, p.gh.user, "user should be cleared")
	assert.Nil(t, p.gh.allIssues, "allIssues should be cleared")
	assert.Nil(t, p.gh.allPRs, "allPRs should be cleared")
	assert.False(t, p.actionsWatching, "actionsWatching should be cleared")
	assert.Equal(t, 0, p.actionsWatchFrame, "actionsWatchFrame should be reset")
	assert.False(t, p.gh.repoPrivate, "repoPrivate should be reset")

	// Verify tab data is cleared.
	assert.Nil(t, p.tabItems[tabIssues], "issues tab items should be cleared")
	assert.Nil(t, p.tabItems[tabPRs], "PRs tab items should be cleared")
	assert.Nil(t, p.tabItems[tabActions], "actions tab items should be cleared")
	assert.Equal(t, 0, p.tabCursor[tabIssues], "issues cursor should be reset")
	assert.Equal(t, 0, p.tabOffset[tabPRs], "PRs offset should be reset")

	// Verify pagination is reset.
	assert.Equal(t, tabPagination{}, p.tabPaging[tabIssues], "issues pagination should be reset")
}

func TestHandleRepoChanged_ClearsGitData(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	// Verify pre-populated git data exists.
	require.NotNil(t, p.gitData.lastBranches)

	p.handleRepoChanged(panels.RepoChangedMsg{Path: "/nonexistent"})

	assert.Nil(t, p.gitData.lastBranches, "branches should be cleared")
	assert.Nil(t, p.gitData.lastWorktrees, "worktrees should be cleared")
	assert.Nil(t, p.gitData.lastRemotes, "remotes should be cleared")
	assert.Nil(t, p.gitData.lastStashes, "stashes should be cleared")
	assert.Nil(t, p.gitData.lastTags, "tags should be cleared")
	assert.Nil(t, p.gitData.lastReflog, "reflog should be cleared")
}

// ---------------------------------------------------------------------------
// loadMoreIfNeeded — cursor-based pagination triggers
// ---------------------------------------------------------------------------

func TestLoadMoreIfNeeded_GitTab_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "git tabs don't paginate")
}

func TestLoadMoreIfNeeded_NoPaging_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabPaging[tabIssues] = tabPagination{allLoaded: true} // all loaded
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "should not load more when all loaded")
}

func TestLoadMoreIfNeeded_Loading_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabPaging[tabIssues] = tabPagination{loading: true, nextPage: 2}
	p.tabItems[tabIssues] = make([]listItem, 10)
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "should not load more when already loading")
}

func TestLoadMoreIfNeeded_EmptyItems_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2}
	p.tabItems[tabIssues] = nil
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "should not load more with empty items")
}

func TestLoadMoreIfNeeded_NextPageZero_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabPaging[tabIssues] = tabPagination{nextPage: 0}
	p.tabItems[tabIssues] = make([]listItem, 10)
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "should not load more when nextPage is 0")
}

func TestLoadMoreIfNeeded_CursorFarFromEnd_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.Height = 10
	// 20 items, cursor at 0, well away from triggerIdx (20-5=15).
	items := make([]listItem, 20)
	for i := range items {
		items[i] = listItem{kind: kindIssue, issue: ghIssueItem{Number: i}}
	}
	p.tabItems[tabIssues] = items
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 0
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}
	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "cursor far from end should not trigger load")
}

func TestLoadMoreIfNeeded_CursorNearEnd_TriggersLoad(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabIssues
	p.Height = 10

	// 10 items, cursor at 8 (>= triggerIdx = 10-5=5).
	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindIssue, issue: ghIssueItem{Number: i}}
	}
	p.tabItems[tabIssues] = items
	p.tabCursor[tabIssues] = 8
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "cursor near end should trigger load")
	assert.True(t, p.tabPaging[tabIssues].loading, "should set loading flag")
}

func TestLoadMoreIfNeeded_Debounce_ReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabIssues
	p.Height = 10

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindIssue}
	}
	p.tabItems[tabIssues] = items
	p.tabCursor[tabIssues] = 8
	// Last load was just now — debounce should prevent another load.
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2, lastLoadAt: time.Now()}

	cmd := p.loadMoreIfNeeded()
	assert.Nil(t, cmd, "debounce should prevent rapid loading")
}

func TestLoadMoreIfNeeded_PRsTab(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabPRs
	p.Height = 10

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindPR}
	}
	p.tabItems[tabPRs] = items
	p.tabCursor[tabPRs] = 9
	p.tabPaging[tabPRs] = tabPagination{nextPage: 3, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "PRs tab should trigger pagination")
}

func TestLoadMoreIfNeeded_ActionsTab(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabActions
	p.Height = 10

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindActionRun}
	}
	p.tabItems[tabActions] = items
	p.tabCursor[tabActions] = 9
	p.tabPaging[tabActions] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "Actions tab should trigger pagination")
}

func TestLoadMoreIfNeeded_WorkflowsTab(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabWorkflows
	p.Height = 10

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindWorkflow}
	}
	p.tabItems[tabWorkflows] = items
	p.tabCursor[tabWorkflows] = 9
	p.tabPaging[tabWorkflows] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "Workflows tab should trigger pagination")
}

func TestLoadMoreIfNeeded_ReleasesTab(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabReleases
	p.Height = 10

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindRelease}
	}
	p.tabItems[tabReleases] = items
	p.tabCursor[tabReleases] = 9
	p.tabPaging[tabReleases] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "Releases tab should trigger pagination")
}

func TestLoadMoreIfNeeded_ViewEndNearEnd_TriggersLoad(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabIssues
	p.Height = 20

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindIssue}
	}
	p.tabItems[tabIssues] = items
	// Cursor is low (0), but viewport end is near the end (offset=4, viewH=19, viewEnd=23 > triggerIdx=5).
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 4
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	cmd := p.loadMoreIfNeeded()
	assert.NotNil(t, cmd, "viewport near end should trigger load even when cursor is low")
}

// ---------------------------------------------------------------------------
// handleMouseWheel — scroll offset changes
// ---------------------------------------------------------------------------

func TestHandleMouseWheel_ScrollDown_IncreasesOffset(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 5)
	p.activeTab = tabBranches
	p.tabOffset[tabBranches] = 0

	// Add enough items so scrolling has room.
	items := make([]listItem, 20)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.tabItems[tabBranches] = items

	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, p.tabOffset[tabBranches], 0, "scroll down should increase offset")
}

func TestHandleMouseWheel_ScrollUp_DecreasesOffset(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 10)
	p.activeTab = tabBranches
	p.tabOffset[tabBranches] = 5

	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Less(t, p.tabOffset[tabBranches], 5, "scroll up should decrease offset")
}

func TestHandleMouseWheel_ScrollUp_ClampsToZero(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 10)
	p.activeTab = tabBranches
	p.tabOffset[tabBranches] = 1 // less than ScrollDelta (3)

	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.tabOffset[tabBranches], "should clamp to 0")
}

func TestHandleMouseWheel_ScrollDown_ClampsToMaxOffset(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 10)
	p.activeTab = tabBranches

	// Only 3 items, height 10 → maxOffset = 3-9 < 0 → 0
	p.tabOffset[tabBranches] = 0
	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 0, p.tabOffset[tabBranches], "should clamp to maxOffset=0 when items fit")
}

func TestHandleMouseWheel_ScrollDown_ManyItems(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 5) // small viewport
	p.activeTab = tabBranches

	items := make([]listItem, 30)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.tabItems[tabBranches] = items
	p.tabOffset[tabBranches] = 0

	// Scroll down multiple times.
	for i := 0; i < 5; i++ {
		p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	// Offset should be positive and within bounds.
	tbh := p.tabBarHeight()
	maxOffset := 30 - (p.Height - tbh)
	if maxOffset < 0 {
		maxOffset = 0
	}
	assert.LessOrEqual(t, p.tabOffset[tabBranches], maxOffset, "offset should not exceed max")
	assert.GreaterOrEqual(t, p.tabOffset[tabBranches], 0, "offset should be non-negative")
}

func TestHandleMouseWheel_ReturnsLoadMoreCmd(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.SetSize(80, 5)
	p.activeTab = tabIssues

	items := make([]listItem, 10)
	for i := range items {
		items[i] = listItem{kind: kindIssue}
	}
	p.tabItems[tabIssues] = items
	p.tabCursor[tabIssues] = 8
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2, lastLoadAt: time.Time{}}

	_, cmd := p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	// Should call loadMoreIfNeeded which returns a cmd.
	assert.NotNil(t, cmd, "mouse wheel near end of paginated tab should trigger load")
}

// ---------------------------------------------------------------------------
// crossRefPRsActions — PR/action run cross-referencing
// ---------------------------------------------------------------------------

func TestCrossRefPRsActions_MatchingBranch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	p.gh.allPRs = []ghPRItem{
		{Number: 1, HeadBranch: "feature-a"},
		{Number: 2, HeadBranch: "feature-b"},
		{Number: 3, HeadBranch: "no-action"},
	}
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "feature-a", Status: "completed", Conclusion: "success"}},
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "feature-b", Status: "completed", Conclusion: "failure"}},
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "feature-a", Status: "in_progress", Conclusion: ""}}, // older run, should not override
	}

	p.crossRefPRsActions()

	assert.Equal(t, "completed", p.gh.allPRs[0].ActionStatus, "PR 1 should get first matching action's status")
	assert.Equal(t, "success", p.gh.allPRs[0].ActionConclusion, "PR 1 should get first matching action's conclusion")
	assert.Equal(t, "completed", p.gh.allPRs[1].ActionStatus, "PR 2 should match feature-b")
	assert.Equal(t, "failure", p.gh.allPRs[1].ActionConclusion, "PR 2 conclusion")
	assert.Empty(t, p.gh.allPRs[2].ActionStatus, "PR 3 should have no action match")
}

func TestCrossRefPRsActions_EmptyPRs(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = nil
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "main"}},
	}

	// Should not panic.
	p.crossRefPRsActions()
}

func TestCrossRefPRsActions_EmptyActions(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{{Number: 1, HeadBranch: "main"}}
	p.tabItems[tabActions] = nil

	// Should not panic.
	p.crossRefPRsActions()
}

func TestCrossRefPRsActions_SkipsNonActionRunItems(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{
		{Number: 1, HeadBranch: "main"},
	}
	p.tabItems[tabActions] = []listItem{
		{kind: kindTag}, // wrong kind, should be skipped
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "main", Status: "completed", Conclusion: "success"}},
	}

	p.crossRefPRsActions()
	assert.Equal(t, "completed", p.gh.allPRs[0].ActionStatus)
}

func TestCrossRefPRsActions_NoMatchingBranches(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{
		{Number: 1, HeadBranch: "feature-x"},
	}
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Branch: "main", Status: "completed", Conclusion: "success"}},
	}

	p.crossRefPRsActions()
	assert.Empty(t, p.gh.allPRs[0].ActionStatus, "no matching branch should leave action status empty")
}

// ---------------------------------------------------------------------------
// renderTabBar — narrow widths (no panic, no corruption)
// ---------------------------------------------------------------------------

func TestRenderTabBar_NarrowWidth_10(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	result := p.renderTabBar(10)
	assert.NotEmpty(t, result, "should render something at width 10")
}

func TestRenderTabBar_NarrowWidth_15(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	result := p.renderTabBar(15)
	assert.NotEmpty(t, result, "should render something at width 15")
}

func TestRenderTabBar_NarrowWidth_20(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	result := p.renderTabBar(20)
	assert.NotEmpty(t, result, "should render something at width 20")
}

func TestRenderTabBar_Width_1(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// Extremely narrow — must not panic.
	result := p.renderTabBar(1)
	assert.NotEmpty(t, result)
}

func TestRenderTabBar_Width_0(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// Zero width — must not panic.
	result := p.renderTabBar(0)
	_ = result // just verify no panic
}

func TestRenderTabBar_ModeGitHub_NarrowWidth(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	result := p.renderTabBar(15)
	assert.NotEmpty(t, result, "GitHub mode should render at narrow width")
}

func TestRenderTabBar_ModeAll_NarrowWidth(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.gh.client = &mockGHClientFull{} // non-nil → two rows
	p.activeTab = tabBranches

	result := p.renderTabBar(10)
	assert.NotEmpty(t, result, "ModeAll should render two rows at narrow width")
}

func TestRenderTabBar_ModeAll_NoGHClient(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.gh.client = nil

	result := p.renderTabBar(20)
	assert.NotEmpty(t, result, "ModeAll without GH client should render git row only")
}

func TestRenderTabBar_ModeGit(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.mode = ModeGit
	result := p.renderTabBar(80)
	assert.NotEmpty(t, result)
}

func TestRenderTabBar_ActiveGitHubTab_ModeAll(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := New(mock, config.GitConfig{}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.mode = ModeAll
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabIssues // GitHub tab active

	result := p.renderTabBar(80)
	assert.NotEmpty(t, result)
}

func TestRenderTabBar_WithPagingIndicators(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.tabItems[tabIssues] = []listItem{{kind: kindIssue}}
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2} // not allLoaded → shows "+"
	p.activeTab = tabIssues

	result := p.renderTabBar(80)
	assert.NotEmpty(t, result)
}

func TestRenderTabBar_WithFilters(t *testing.T) {
	t.Parallel()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.issueFilter = issueFilterAssigned
	p.gh.prFilter = prFilterMine
	p.activeTab = tabIssues

	result := p.renderTabBar(80)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// pageUp / pageDown — boundary conditions
// ---------------------------------------------------------------------------

func TestPageDown_EmptyTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10
	p.activeTab = tabStash
	p.tabItems[tabStash] = nil
	p.tabCursor[tabStash] = 0

	// Should not panic on empty items.
	p.pageDown()
	assert.Equal(t, 0, p.tabCursor[tabStash])
}

func TestPageUp_EmptyTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10
	p.activeTab = tabStash
	p.tabItems[tabStash] = nil
	p.tabCursor[tabStash] = 0

	p.pageUp()
	assert.Equal(t, 0, p.tabCursor[tabStash])
}

func TestPageDown_LargeList(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10

	items := make([]listItem, 100)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = items
	p.tabCursor[tabBranches] = 0

	p.pageDown()
	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	assert.Equal(t, viewH, p.tabCursor[tabBranches], "should advance by viewH")
}

func TestPageDown_NearEnd(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10

	items := make([]listItem, 15)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = items
	p.tabCursor[tabBranches] = 12

	p.pageDown()
	assert.Equal(t, 14, p.tabCursor[tabBranches], "should clamp to last item")
}

func TestPageUp_FromMiddle(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10

	items := make([]listItem, 50)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = items

	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	p.tabCursor[tabBranches] = 20

	p.pageUp()
	assert.Equal(t, 20-viewH, p.tabCursor[tabBranches], "should go back by viewH")
}

func TestPageUp_ClampsToZero(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 10
	p.activeTab = tabBranches

	items := make([]listItem, 20)
	for i := range items {
		items[i] = listItem{kind: kindLocalBranch}
	}
	p.tabItems[tabBranches] = items
	p.tabCursor[tabBranches] = 3 // less than viewH

	p.pageUp()
	assert.Equal(t, 0, p.tabCursor[tabBranches], "should clamp to 0")
}

func TestPageDown_NegativeViewH(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 1 // With tab bar, viewH will be <= 0.
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 1

	p.pageDown()
	assert.Equal(t, 1, p.tabCursor[tabBranches], "should not move when viewH<=0")
}

func TestPageUp_NegativeViewH(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Height = 1
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 2

	p.pageUp()
	assert.Equal(t, 2, p.tabCursor[tabBranches], "should not move when viewH<=0")
}

// ---------------------------------------------------------------------------
// ghTabCountStr — pagination indicator
// ---------------------------------------------------------------------------

func TestGhTabCountStr_AllLoaded(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[tabIssues] = []listItem{{kind: kindIssue}, {kind: kindIssue}, {kind: kindIssue}}
	p.tabPaging[tabIssues] = tabPagination{allLoaded: true}

	assert.Equal(t, "3", p.ghTabCountStr(tabIssues))
}

func TestGhTabCountStr_MorePages(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[tabIssues] = []listItem{{kind: kindIssue}, {kind: kindIssue}}
	p.tabPaging[tabIssues] = tabPagination{nextPage: 2}

	assert.Equal(t, "2+", p.ghTabCountStr(tabIssues))
}

func TestGhTabCountStr_GitTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// Git tabs (tabBranches < tabIssues) should just return count.
	assert.Equal(t, fmt.Sprintf("%d", len(p.tabItems[tabBranches])), p.ghTabCountStr(tabBranches))
}
