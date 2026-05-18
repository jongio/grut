package gitinfo

import (
	"testing"

	ghclient "github.com/jongio/grut/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// handleMetaLoaded — sets user and repoPrivate
// ---------------------------------------------------------------------------

func TestHandleMetaLoaded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		msg         ghMetaLoadedMsg
		wantUser    string
		wantPrivate bool
	}{
		{
			name:        "sets user and private flag",
			msg:         ghMetaLoadedMsg{user: "octocat", repoPrivate: true},
			wantUser:    "octocat",
			wantPrivate: true,
		},
		{
			name:        "empty user leaves field unchanged",
			msg:         ghMetaLoadedMsg{user: "", repoPrivate: false},
			wantUser:    "pre-existing",
			wantPrivate: false,
		},
		{
			name:        "sets public repo",
			msg:         ghMetaLoadedMsg{user: "bob", repoPrivate: false},
			wantUser:    "bob",
			wantPrivate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.gh.user = "pre-existing"

			result, cmd := p.handleMetaLoaded(tt.msg)
			assert.Nil(t, cmd)
			panel := result.(*Panel)
			assert.Equal(t, tt.wantUser, panel.gh.user)
			assert.Equal(t, tt.wantPrivate, panel.gh.repoPrivate)
		})
	}
}

// ---------------------------------------------------------------------------
// handleIssuesPage — pagination, replace vs append, max cap
// ---------------------------------------------------------------------------

func TestHandleIssuesPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		existing       []ghIssueItem
		msg            ghIssuesPageMsg
		wantCount      int
		wantAllLoaded  bool
		wantNextPage   int
		wantFirstTitle string
	}{
		{
			name:     "replace mode sets items directly",
			existing: []ghIssueItem{{Number: 99, Title: "old"}},
			msg: ghIssuesPageMsg{
				issues:   []ghIssueItem{{Number: 1, Title: "new"}},
				nextPage: 2,
				replace:  true,
			},
			wantCount:      1,
			wantNextPage:   2,
			wantAllLoaded:  false,
			wantFirstTitle: "new",
		},
		{
			name:     "append mode adds to existing",
			existing: []ghIssueItem{{Number: 1, Title: "first"}},
			msg: ghIssuesPageMsg{
				issues:   []ghIssueItem{{Number: 2, Title: "second"}},
				nextPage: 3,
				replace:  false,
			},
			wantCount:      2,
			wantNextPage:   3,
			wantAllLoaded:  false,
			wantFirstTitle: "first",
		},
		{
			name:     "nextPage zero marks all loaded",
			existing: nil,
			msg: ghIssuesPageMsg{
				issues:   []ghIssueItem{{Number: 5, Title: "last"}},
				nextPage: 0,
				replace:  true,
			},
			wantCount:      1,
			wantNextPage:   0,
			wantAllLoaded:  true,
			wantFirstTitle: "last",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.gh.allIssues = tt.existing
			p.tabPaging[tabIssues] = tabPagination{loading: true}

			result, cmd := p.handleIssuesPage(tt.msg)
			assert.Nil(t, cmd)
			panel := result.(*Panel)

			assert.Equal(t, tt.wantCount, len(panel.gh.allIssues))
			assert.Equal(t, tt.wantAllLoaded, panel.tabPaging[tabIssues].allLoaded)
			assert.Equal(t, tt.wantNextPage, panel.tabPaging[tabIssues].nextPage)
			assert.False(t, panel.tabPaging[tabIssues].loading, "loading should be cleared")
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirstTitle, panel.gh.allIssues[0].Title)
			}
		})
	}
}

func TestHandleIssuesPage_MaxCapEnforced(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())

	// Pre-populate with MaxPaginationItems-1 issues.
	existing := make([]ghIssueItem, ghclient.MaxPaginationItems-1)
	for i := range existing {
		existing[i] = ghIssueItem{Number: i, Title: "existing"}
	}
	p.gh.allIssues = existing

	// Append 5 more (would exceed MaxPaginationItems).
	msg := ghIssuesPageMsg{
		issues:   []ghIssueItem{{Number: 9001}, {Number: 9002}, {Number: 9003}, {Number: 9004}, {Number: 9005}},
		nextPage: 5,
		replace:  false,
	}

	result, _ := p.handleIssuesPage(msg)
	panel := result.(*Panel)
	assert.Equal(t, ghclient.MaxPaginationItems, len(panel.gh.allIssues))
	assert.True(t, panel.tabPaging[tabIssues].allLoaded, "should mark all loaded when capped")
}

// ---------------------------------------------------------------------------
// handlePRsPage — pagination, replace vs append, max cap
// ---------------------------------------------------------------------------

func TestHandlePRsPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existing      []ghPRItem
		msg           ghPRsPageMsg
		wantCount     int
		wantAllLoaded bool
		wantNextPage  int
	}{
		{
			name:     "replace mode sets PRs directly",
			existing: []ghPRItem{{Number: 10, Title: "stale"}},
			msg: ghPRsPageMsg{
				prs:      []ghPRItem{{Number: 1, Title: "new-pr"}},
				nextPage: 2,
				replace:  true,
			},
			wantCount:     1,
			wantNextPage:  2,
			wantAllLoaded: false,
		},
		{
			name:     "append mode adds to existing PRs",
			existing: []ghPRItem{{Number: 1, Title: "first-pr"}},
			msg: ghPRsPageMsg{
				prs:      []ghPRItem{{Number: 2, Title: "second-pr"}},
				nextPage: 3,
				replace:  false,
			},
			wantCount:     2,
			wantNextPage:  3,
			wantAllLoaded: false,
		},
		{
			name:     "nextPage zero marks all loaded",
			existing: nil,
			msg: ghPRsPageMsg{
				prs:      []ghPRItem{{Number: 7, Title: "final"}},
				nextPage: 0,
				replace:  true,
			},
			wantCount:     1,
			wantNextPage:  0,
			wantAllLoaded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.gh.allPRs = tt.existing
			p.tabPaging[tabPRs] = tabPagination{loading: true}

			result, cmd := p.handlePRsPage(tt.msg)
			assert.Nil(t, cmd)
			panel := result.(*Panel)

			assert.Equal(t, tt.wantCount, len(panel.gh.allPRs))
			assert.Equal(t, tt.wantAllLoaded, panel.tabPaging[tabPRs].allLoaded)
			assert.Equal(t, tt.wantNextPage, panel.tabPaging[tabPRs].nextPage)
			assert.False(t, panel.tabPaging[tabPRs].loading)
		})
	}
}

func TestHandlePRsPage_MaxCapEnforced(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())

	existing := make([]ghPRItem, ghclient.MaxPaginationItems-1)
	for i := range existing {
		existing[i] = ghPRItem{Number: i, Title: "existing-pr"}
	}
	p.gh.allPRs = existing

	msg := ghPRsPageMsg{
		prs:      []ghPRItem{{Number: 9001}, {Number: 9002}, {Number: 9003}},
		nextPage: 4,
		replace:  false,
	}

	result, _ := p.handlePRsPage(msg)
	panel := result.(*Panel)
	assert.Equal(t, ghclient.MaxPaginationItems, len(panel.gh.allPRs))
	assert.True(t, panel.tabPaging[tabPRs].allLoaded)
}

// ---------------------------------------------------------------------------
// handleActionsPage — pagination, replace vs append, watching state
// ---------------------------------------------------------------------------

func TestHandleActionsPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		msg             ghActionsPageMsg
		wantItemCount   int
		wantNextPage    int
		wantAllLoaded   bool
		wantWatching    bool
		existingActions []listItem
	}{
		{
			name: "replace mode clears existing actions",
			msg: ghActionsPageMsg{
				actions:  []ghActionItem{{RunID: 1, WorkflowName: "CI", Status: "completed"}},
				nextPage: 2,
				replace:  true,
			},
			existingActions: []listItem{{kind: kindActionRun, actionRun: ghActionItem{RunID: 99}}},
			wantItemCount:   1,
			wantNextPage:    2,
			wantAllLoaded:   false,
			wantWatching:    false,
		},
		{
			name: "append mode adds to existing",
			msg: ghActionsPageMsg{
				actions:  []ghActionItem{{RunID: 2, WorkflowName: "Deploy", Status: "completed"}},
				nextPage: 3,
				replace:  false,
			},
			existingActions: []listItem{{kind: kindActionRun, actionRun: ghActionItem{RunID: 1}}},
			wantItemCount:   2,
			wantNextPage:    3,
			wantAllLoaded:   false,
			wantWatching:    false,
		},
		{
			name: "in-progress run enables watching",
			msg: ghActionsPageMsg{
				actions:  []ghActionItem{{RunID: 3, Status: statusInProgress}},
				nextPage: 0,
				replace:  true,
			},
			wantItemCount: 1,
			wantNextPage:  0,
			wantAllLoaded: true,
			wantWatching:  true,
		},
		{
			name: "queued run enables watching",
			msg: ghActionsPageMsg{
				actions:  []ghActionItem{{RunID: 4, Status: statusQueued}},
				nextPage: 0,
				replace:  true,
			},
			wantItemCount: 1,
			wantNextPage:  0,
			wantAllLoaded: true,
			wantWatching:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.tabItems[tabActions] = tt.existingActions
			p.tabPaging[tabActions] = tabPagination{loading: true}

			result, _ := p.handleActionsPage(tt.msg)
			panel := result.(*Panel)

			assert.Equal(t, tt.wantItemCount, len(panel.tabItems[tabActions]))
			assert.Equal(t, tt.wantNextPage, panel.tabPaging[tabActions].nextPage)
			assert.Equal(t, tt.wantAllLoaded, panel.tabPaging[tabActions].allLoaded)
			assert.False(t, panel.tabPaging[tabActions].loading)
			assert.Equal(t, tt.wantWatching, panel.actionsWatching)
		})
	}
}

// ---------------------------------------------------------------------------
// handleWorkflowsPage — pagination and replace
// ---------------------------------------------------------------------------

func TestHandleWorkflowsPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		msg           ghWorkflowsPageMsg
		existing      []listItem
		wantCount     int
		wantAllLoaded bool
	}{
		{
			name: "replace mode clears existing",
			msg: ghWorkflowsPageMsg{
				workflows: []ghWorkflowItem{{Name: "CI", ID: 1}},
				nextPage:  2,
				replace:   true,
			},
			existing:      []listItem{{kind: kindWorkflow, workflow: ghWorkflowItem{Name: "old"}}},
			wantCount:     1,
			wantAllLoaded: false,
		},
		{
			name: "append mode adds to existing",
			msg: ghWorkflowsPageMsg{
				workflows: []ghWorkflowItem{{Name: "Deploy", ID: 2}},
				nextPage:  3,
				replace:   false,
			},
			existing:      []listItem{{kind: kindWorkflow, workflow: ghWorkflowItem{Name: "CI"}}},
			wantCount:     2,
			wantAllLoaded: false,
		},
		{
			name: "nextPage zero marks all loaded",
			msg: ghWorkflowsPageMsg{
				workflows: []ghWorkflowItem{{Name: "Lint", ID: 3}},
				nextPage:  0,
				replace:   true,
			},
			wantCount:     1,
			wantAllLoaded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.tabItems[tabWorkflows] = tt.existing
			p.tabPaging[tabWorkflows] = tabPagination{loading: true}

			result, cmd := p.handleWorkflowsPage(tt.msg)
			assert.Nil(t, cmd)
			panel := result.(*Panel)

			assert.Equal(t, tt.wantCount, len(panel.tabItems[tabWorkflows]))
			assert.Equal(t, tt.wantAllLoaded, panel.tabPaging[tabWorkflows].allLoaded)
			assert.False(t, panel.tabPaging[tabWorkflows].loading)
		})
	}
}

// ---------------------------------------------------------------------------
// handleReleasesPage — pagination and replace
// ---------------------------------------------------------------------------

func TestHandleReleasesPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		msg           ghReleasesPageMsg
		existing      []listItem
		wantCount     int
		wantAllLoaded bool
	}{
		{
			name: "replace mode clears existing",
			msg: ghReleasesPageMsg{
				releases: []ghReleaseItem{{TagName: "v1.0", ID: 1}},
				nextPage: 2,
				replace:  true,
			},
			existing:      []listItem{{kind: kindRelease, release: ghReleaseItem{TagName: "v0.9"}}},
			wantCount:     1,
			wantAllLoaded: false,
		},
		{
			name: "append mode adds to existing",
			msg: ghReleasesPageMsg{
				releases: []ghReleaseItem{{TagName: "v1.1", ID: 2}},
				nextPage: 3,
				replace:  false,
			},
			existing:      []listItem{{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0"}}},
			wantCount:     2,
			wantAllLoaded: false,
		},
		{
			name: "nextPage zero marks all loaded",
			msg: ghReleasesPageMsg{
				releases: []ghReleaseItem{{TagName: "v2.0", ID: 3}},
				nextPage: 0,
				replace:  true,
			},
			wantCount:     1,
			wantAllLoaded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(defaultMock())
			p.tabItems[tabReleases] = tt.existing
			p.tabPaging[tabReleases] = tabPagination{loading: true}

			result, cmd := p.handleReleasesPage(tt.msg)
			assert.Nil(t, cmd)
			panel := result.(*Panel)

			assert.Equal(t, tt.wantCount, len(panel.tabItems[tabReleases]))
			assert.Equal(t, tt.wantAllLoaded, panel.tabPaging[tabReleases].allLoaded)
			assert.False(t, panel.tabPaging[tabReleases].loading)
		})
	}
}

// ---------------------------------------------------------------------------
// handleActionsPage — cursor/offset reset on replace
// ---------------------------------------------------------------------------

func TestHandleActionsPage_ResetsCursorOnReplace(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())
	p.tabCursor[tabActions] = 5
	p.tabOffset[tabActions] = 3

	msg := ghActionsPageMsg{
		actions: []ghActionItem{{RunID: 1, Status: "completed"}},
		replace: true,
	}
	result, _ := p.handleActionsPage(msg)
	panel := result.(*Panel)

	assert.Equal(t, 0, panel.tabCursor[tabActions])
	assert.Equal(t, 0, panel.tabOffset[tabActions])
}

func TestHandleWorkflowsPage_ResetsCursorOnReplace(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())
	p.tabCursor[tabWorkflows] = 4
	p.tabOffset[tabWorkflows] = 2

	msg := ghWorkflowsPageMsg{
		workflows: []ghWorkflowItem{{Name: "CI", ID: 1}},
		replace:   true,
	}
	result, _ := p.handleWorkflowsPage(msg)
	panel := result.(*Panel)

	assert.Equal(t, 0, panel.tabCursor[tabWorkflows])
	assert.Equal(t, 0, panel.tabOffset[tabWorkflows])
}

func TestHandleReleasesPage_ResetsCursorOnReplace(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())
	p.tabCursor[tabReleases] = 3
	p.tabOffset[tabReleases] = 1

	msg := ghReleasesPageMsg{
		releases: []ghReleaseItem{{TagName: "v1.0", ID: 1}},
		replace:  true,
	}
	result, _ := p.handleReleasesPage(msg)
	panel := result.(*Panel)

	assert.Equal(t, 0, panel.tabCursor[tabReleases])
	assert.Equal(t, 0, panel.tabOffset[tabReleases])
}

// ---------------------------------------------------------------------------
// handleIssuesPage — preserves cursor/offset on append
// ---------------------------------------------------------------------------

func TestHandleIssuesPage_PreservesCursorOnAppend(t *testing.T) {
	t.Parallel()
	p := newTestPanel(defaultMock())
	p.gh.allIssues = []ghIssueItem{{Number: 1, Title: "first"}}
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 0

	// Set cursor position before append
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 0

	msg := ghIssuesPageMsg{
		issues:   []ghIssueItem{{Number: 2, Title: "second"}},
		nextPage: 3,
		replace:  false,
	}

	result, _ := p.handleIssuesPage(msg)
	panel := result.(*Panel)
	require.Equal(t, 2, len(panel.gh.allIssues))
	assert.Equal(t, 0, panel.tabCursor[tabIssues], "cursor should be preserved on append")
}
