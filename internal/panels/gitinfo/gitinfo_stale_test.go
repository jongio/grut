package gitinfo

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubFailedRefreshRetainsPriorRowsAndMarksStale(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)
	p.gh.client = &mockGHClientFull{}
	p.mode = ModeGitHub
	p.activeTab = tabIssues
	fetchedAt := time.Now().Add(-2 * time.Minute)
	p.gh.freshness[tabIssues] = ghTabFreshness{fetchedAt: fetchedAt, owner: p.gh.owner, repo: p.gh.repo}
	p.tabCursor[tabIssues] = 1
	p.tabOffset[tabIssues] = 1

	_, cmd := p.handleIssuesPage(ghIssuesPageMsg{err: errors.New("network down"), replace: true})

	require.Len(t, p.tabItems[tabIssues], len(sampleIssues()))
	assert.Equal(t, "Feature req", p.tabItems[tabIssues][1].issue.Title)
	assert.True(t, p.gh.freshness[tabIssues].stale)
	assert.Equal(t, fetchedAt, p.gh.freshness[tabIssues].fetchedAt)
	assert.Equal(t, 1, p.tabCursor[tabIssues])
	assert.Equal(t, 1, p.tabOffset[tabIssues])
	toast := findStaleToast(t, cmd)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, panels.StripANSI(p.renderTabBar(160)), "stale")
}

func TestGitHubSuccessfulRefreshClearsStaleAndUpdatesFetchedAt(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	oldFetchedAt := time.Now().Add(-10 * time.Minute)
	p.gh.freshness[tabIssues] = ghTabFreshness{fetchedAt: oldFetchedAt, owner: "owner", repo: "repo", stale: true}

	updated := []ghIssueItem{{Number: 99, Title: "Fresh issue", Body: "fresh body", State: "open"}}
	_, cmd := p.handleIssuesPage(ghIssuesPageMsg{issues: updated, replace: true})

	assert.Nil(t, cmd)
	require.Len(t, p.tabItems[tabIssues], 1)
	assert.Equal(t, 99, p.tabItems[tabIssues][0].issue.Number)
	assert.False(t, p.gh.freshness[tabIssues].stale)
	assert.True(t, p.gh.freshness[tabIssues].fetchedAt.After(oldFetchedAt))
	assert.Equal(t, "owner", p.gh.freshness[tabIssues].owner)
	assert.Equal(t, "repo", p.gh.freshness[tabIssues].repo)
}

func TestGitHubStaleRefreshShowsOneWarningToastPerBurst(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	now := time.Now().Add(-time.Minute)
	for _, tab := range ghPagedTabs {
		p.gh.freshness[tab] = ghTabFreshness{fetchedAt: now, owner: "owner", repo: "repo"}
	}
	p.beginGitHubRefreshBurst()

	_, issuesCmd := p.handleIssuesPage(ghIssuesPageMsg{err: errors.New("issues failed"), replace: true})
	_, prsCmd := p.handlePRsPage(ghPRsPageMsg{err: errors.New("prs failed"), replace: true})
	_, actionsCmd := p.handleActionsPage(ghActionsPageMsg{err: errors.New("actions failed"), replace: true})
	_, workflowsCmd := p.handleWorkflowsPage(ghWorkflowsPageMsg{err: errors.New("workflows failed"), replace: true})
	_, releasesCmd := p.handleReleasesPage(ghReleasesPageMsg{err: errors.New("releases failed"), replace: true})

	toasts := 0
	for _, cmd := range []tea.Cmd{issuesCmd, prsCmd, actionsCmd, workflowsCmd, releasesCmd} {
		for _, msg := range collectCmdMsgs(cmd) {
			if toast, ok := msg.(notify.ShowToastMsg); ok && toast.Level == notify.Warn {
				toasts++
			}
		}
	}
	assert.Equal(t, 1, toasts)
}

func TestGitHubFirstLoadFailureLeavesEmptyRows(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleReleasesPage(ghReleasesPageMsg{err: errors.New("network down"), replace: true})

	assert.Nil(t, cmd)
	assert.Empty(t, p.tabItems[tabReleases])
	assert.True(t, p.tabPaging[tabReleases].allLoaded)
	assert.False(t, p.gh.freshness[tabReleases].stale)
}

func TestGitHubStaleIssuePreviewUsesCachedBody(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	populateGH(p, []ghIssueItem{{Number: 7, Title: "Cached", Body: "cached body", State: "open"}}, nil, nil)
	p.gh.freshness[tabIssues] = ghTabFreshness{fetchedAt: time.Now().Add(-time.Minute), owner: p.gh.owner, repo: p.gh.repo}
	p.activeTab = tabIssues

	_, _ = p.handleIssuesPage(ghIssuesPageMsg{err: errors.New("network down"), replace: true})
	cmd := p.issueSelectedCmd()

	require.NotNil(t, cmd)
	msg := cmd()
	selected, ok := msg.(panels.IssueSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "cached body", selected.Body)
}

func findStaleToast(t *testing.T, cmd tea.Cmd) notify.ShowToastMsg {
	t.Helper()
	for _, msg := range collectCmdMsgs(cmd) {
		if toast, ok := msg.(notify.ShowToastMsg); ok {
			return toast
		}
	}
	t.Fatal("expected stale toast")
	return notify.ShowToastMsg{}
}
