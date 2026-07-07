package gitinfo

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongio/grut/internal/notify"
)

// ---------------------------------------------------------------------------
// markdownLink formatting
// ---------------------------------------------------------------------------

func TestMarkdownLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		number int
		title  string
		url    string
		want   string
	}{
		{
			name:   "issue",
			number: 42,
			title:  "Fix the login bug",
			url:    "https://github.com/o/r/issues/42",
			want:   "[#42 Fix the login bug](https://github.com/o/r/issues/42)",
		},
		{
			name:   "pull request",
			number: 10,
			title:  "Add auth",
			url:    "https://github.com/o/r/pull/10",
			want:   "[#10 Add auth](https://github.com/o/r/pull/10)",
		},
		{
			name:   "title whitespace trimmed",
			number: 7,
			title:  "  spaced out  ",
			url:    "https://github.com/o/r/issues/7",
			want:   "[#7 spaced out](https://github.com/o/r/issues/7)",
		},
		{
			name:   "empty url yields empty string",
			number: 1,
			title:  "No link",
			url:    "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := markdownLink(tt.number, tt.title, tt.url); got != tt.want {
				t.Errorf("markdownLink(%d, %q, %q) = %q, want %q", tt.number, tt.title, tt.url, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// copyMarkdownLink handler
// ---------------------------------------------------------------------------

func TestCopyMarkdownLink_Issue(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 42, Title: "Fix bug", HTMLURL: "https://github.com/o/r/issues/42"}},
	}
	p.tabCursor[tabIssues] = 0

	_, cmd := p.copyMarkdownLink()
	assert.NotNil(t, cmd, "expected a command for an issue with a URL")
}

func TestCopyMarkdownLink_PR(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Add feature", HTMLURL: "https://github.com/o/r/pull/10"}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.copyMarkdownLink()
	assert.NotNil(t, cmd, "expected a command for a PR with a URL")
}

func TestCopyMarkdownLink_NoURLWarns(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 42, Title: "Fix bug"}}, // no HTMLURL
	}
	p.tabCursor[tabIssues] = 0

	_, cmd := p.copyMarkdownLink()
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast message")
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "No link available")
}

func TestCopyMarkdownLink_EmptyList(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = nil
	p.tabCursor[tabIssues] = 0

	_, cmd := p.copyMarkdownLink()
	assert.Nil(t, cmd, "expected no command when the list is empty")
}

func TestCopyMarkdownLink_WrongKind(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindLocalBranch},
	}
	p.tabCursor[tabIssues] = 0

	_, cmd := p.copyMarkdownLink()
	assert.Nil(t, cmd, "expected no command for a non issue/PR item")
}

func TestCopyMarkdownLink_KeyIgnoredOffTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focus()
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{{kind: kindLocalBranch}}
	p.tabCursor[tabBranches] = 0

	_, cmd := p.Update(tea.KeyPressMsg{Code: -1, Text: "M"})
	assert.Nil(t, cmd, "M should be a no-op outside the issues and PRs tabs")
}
