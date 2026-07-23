package gitinfo

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFilterTestPanel() *Panel {
	return &Panel{
		BasePanel: panelsBase(80, 10),
		colors:    defaultColors,
		mode:      ModeGit,
		activeTab: tabBranches,
		tabItems: [tabCount][]listItem{
			tabBranches: {
				{kind: kindLocalBranch, branch: git.Branch{Name: "main", IsCurrent: true}},
				{kind: kindLocalBranch, branch: git.Branch{Name: "Feature/Login"}},
				{kind: kindLocalBranch, branch: git.Branch{Name: "release"}},
			},
		},
	}
}

func newGitHubFilterTestPanel(tab tabID) *Panel {
	return &Panel{
		BasePanel: panelsBase(80, 10),
		colors:    defaultColors,
		mode:      ModeGitHub,
		activeTab: tab,
		tabItems: [tabCount][]listItem{
			tabIssues: {
				{kind: kindIssue, issue: ghIssueItem{Number: 245, Title: "Filter GitHub issues"}},
				{kind: kindIssue, issue: ghIssueItem{Number: 101, Title: "Crash on checkout"}},
				{kind: kindIssue, issue: ghIssueItem{Number: 77, Title: "Improve help"}},
			},
			tabPRs: {
				{kind: kindPR, pr: ghPRItem{Number: 300, Title: "Add filter support", HeadBranch: "feature/filter"}},
				{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Fix checkout crash", HeadBranch: "bugfix/checkout"}},
				{kind: kindPR, pr: ghPRItem{Number: 17, Title: "Update docs", HeadBranch: "docs"}},
			},
		},
	}
}

func panelsBase(width, height int) panels.BasePanel {
	return panels.BasePanel{Focused: true, Width: width, Height: height}
}

func TestGitInfoFilterMatchingAndClear(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()

	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{name: "case_insensitive_substring", query: "login", want: []int{1}},
		{name: "uppercase_query", query: "FEATURE", want: []int{1}},
		{name: "multiple_matches", query: "e", want: []int{1, 2}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newFilterTestPanel()
			p.filterQuery = tt.query
			p.applyFilter()

			assert.Equal(t, tt.want, p.filteredIdx)
		})
	}

	p.filterQuery = "feature"
	p.applyFilter()
	require.NotNil(t, p.filteredIdx)

	p.filterQuery = ""
	p.applyFilter()

	assert.Nil(t, p.filteredIdx)
	assert.Equal(t, len(p.tabItems[tabBranches]), p.activeItemCount())
}

func TestGitInfoFilterClampsCursorAndRendersEmptyState(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()
	p.tabCursor[tabBranches] = 2
	p.filterQuery = "main"
	p.applyFilter()

	assert.Equal(t, []int{0}, p.filteredIdx)
	assert.Equal(t, 0, p.tabCursor[tabBranches])

	p.filterMode = true
	p.filterQuery = "no-such-branch"
	p.applyFilter()

	assert.Empty(t, p.filteredIdx)
	assert.Equal(t, 0, p.tabCursor[tabBranches])
	assert.Contains(t, p.View(80, 8), "No matches")
}

func TestGitInfoFilteredSelectionTargetsUnderlyingItem(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()
	p.filterQuery = "release"
	p.applyFilter()
	require.Equal(t, []int{2}, p.filteredIdx)

	item, ok := p.currentItem()
	require.True(t, ok)
	assert.Equal(t, "release", item.branch.Name)

	_, _ = p.doDelete()
	assert.Equal(t, opBranchDelete, p.pending)
	assert.Equal(t, "release", p.pendingName)
}

func TestGitInfoFilterTabSwitchClearsFilter(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()
	p.filterMode = true
	p.filterQuery = "feature"
	p.applyFilter()
	require.NotNil(t, p.filteredIdx)

	p.SetActiveTab(sectionTags)

	assert.False(t, p.filterMode)
	assert.Empty(t, p.filterQuery)
	assert.Nil(t, p.filteredIdx)
	assert.Equal(t, tabTags, p.activeTab)
}

func TestGitInfoSlashFilterDisabledInGitHubMode(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()
	p.mode = ModeGitHub
	p.activeTab = tabBranches
	p.Focus()

	_, _ = p.handleKey(tea.KeyPressMsg{Code: '/'})

	assert.False(t, p.filterMode)
	assert.Empty(t, p.filterQuery)
	assert.Nil(t, p.filteredIdx)
}

func TestGitHubIssuesAndPRsFilterMatchingAndClear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tab   tabID
		query string
		want  []int
	}{
		{name: "issue_title", tab: tabIssues, query: "github", want: []int{0}},
		{name: "issue_number", tab: tabIssues, query: "101", want: []int{1}},
		{name: "pr_title", tab: tabPRs, query: "support", want: []int{0}},
		{name: "pr_number", tab: tabPRs, query: "42", want: []int{1}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newGitHubFilterTestPanel(tt.tab)
			p.filterQuery = tt.query
			p.applyFilter()

			assert.Equal(t, tt.want, p.filteredIdx)

			p.filterQuery = ""
			p.applyFilter()

			assert.Nil(t, p.filteredIdx)
			assert.Equal(t, len(p.tabItems[tt.tab]), p.activeItemCount())
		})
	}
}

func TestGitHubFilterClampsCursorAndRendersEmptyState(t *testing.T) {
	t.Parallel()

	p := newGitHubFilterTestPanel(tabIssues)
	p.tabCursor[tabIssues] = 2
	p.filterQuery = "github"
	p.applyFilter()

	assert.Equal(t, []int{0}, p.filteredIdx)
	assert.Equal(t, 0, p.tabCursor[tabIssues])

	p.filterMode = true
	p.filterQuery = "no-matching-issue"
	p.applyFilter()

	assert.Empty(t, p.filteredIdx)
	assert.Equal(t, 0, p.tabCursor[tabIssues])
	assert.Equal(t, 0, p.activeItemCount())
	assert.Contains(t, p.View(80, 8), "No matches")
}

func TestGitHubFilteredSelectionTargetsUnderlyingItem(t *testing.T) {
	t.Parallel()

	p := newGitHubFilterTestPanel(tabPRs)
	p.filterQuery = "42"
	p.applyFilter()
	require.Equal(t, []int{1}, p.filteredIdx)

	item, ok := p.currentItem()
	require.True(t, ok)
	assert.Equal(t, 42, item.pr.Number)

	p.selectPRByNumber(42)

	assert.Equal(t, 0, p.tabCursor[tabPRs])
}

func TestGitHubFilterReappliesAfterIssuesAndPRsRebuild(t *testing.T) {
	t.Parallel()

	issuePanel := newGitHubFilterTestPanel(tabIssues)
	issuePanel.filterQuery = "keep"
	issuePanel.applyFilter()
	_, _ = issuePanel.handleIssuesPage(ghIssuesPageMsg{
		issues:  []ghIssueItem{{Number: 246, Title: "Keep filtering issues"}},
		replace: true,
	})

	assert.Equal(t, []int{0}, issuePanel.filteredIdx)

	prPanel := newGitHubFilterTestPanel(tabPRs)
	prPanel.filterQuery = "missing"
	prPanel.applyFilter()
	_, _ = prPanel.handlePRsPage(ghPRsPageMsg{
		prs:     []ghPRItem{{Number: 301, Title: "Different pull request"}},
		replace: true,
	})

	assert.Empty(t, prPanel.filteredIdx)
	assert.Equal(t, 0, prPanel.tabCursor[tabPRs])
}

func TestGitInfoFilterEnterKeepsFilterEscapeClears(t *testing.T) {
	t.Parallel()

	p := newFilterTestPanel()
	p.filterMode = true
	p.filterQuery = "feature"
	p.applyFilter()

	_, _ = p.handleFilterKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, p.filterMode)
	assert.Equal(t, "feature", p.filterQuery)
	assert.Equal(t, []int{1}, p.filteredIdx)

	p.filterMode = true
	_, _ = p.handleFilterKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, p.filterMode)
	assert.Empty(t, p.filterQuery)
	assert.Nil(t, p.filteredIdx)
	assert.False(t, strings.Contains(p.View(80, 8), "No matches"))
}
