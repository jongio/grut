package gitinfo

import (
	"testing"

	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseCompareTagsUsesNextReleaseAsPrevious(t *testing.T) {
	t.Parallel()

	items := []listItem{
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.2.0"}},
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.1.0"}},
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0.0"}},
	}

	base, head, ok := releaseCompareTags(items, 0)

	require.True(t, ok)
	assert.Equal(t, "v1.1.0", base)
	assert.Equal(t, "v1.2.0", head)
}

func TestCompareReleaseMissingPreviousShowsToast(t *testing.T) {
	t.Parallel()

	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabReleases
	p.tabCursor[tabReleases] = 0
	p.tabItems[tabReleases] = []listItem{
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0.0"}},
	}

	_, cmd := p.compareRelease()
	require.NotNil(t, cmd)

	toast := findMsg[notify.ShowToastMsg](t, collectCmdMsgs(cmd))
	assert.Equal(t, "No previous release to compare", toast.Message)
	assert.Equal(t, notify.Info, toast.Level)
}

func TestBuildReleaseCompareLoadedMsgAggregatesSummary(t *testing.T) {
	t.Parallel()

	comparison := &gh.CommitsComparison{
		TotalCommits: gh.Ptr(2),
		Files: []*gh.CommitFile{
			{Filename: gh.Ptr("README.md"), Status: gh.Ptr("modified"), Additions: gh.Ptr(7), Deletions: gh.Ptr(2)},
			{Filename: gh.Ptr("cmd/root.go"), Status: gh.Ptr("added"), Additions: gh.Ptr(11), Deletions: gh.Ptr(0)},
		},
		Commits: []*gh.RepositoryCommit{
			{SHA: gh.Ptr("abcdef123456"), Commit: &gh.Commit{Message: gh.Ptr("first change\n\nbody")}},
			{SHA: gh.Ptr("123456abcdef"), Commit: &gh.Commit{Message: gh.Ptr("second change")}},
		},
	}

	msg := buildReleaseCompareLoadedMsg("v1.0.0", "v1.1.0", comparison)

	assert.Equal(t, 2, msg.fileCount)
	assert.Equal(t, 18, msg.additions)
	assert.Equal(t, 2, msg.deletions)
	assert.Len(t, msg.files, 2)
	assert.Len(t, msg.commits, 2)
	assert.Contains(t, msg.summary, "Commits: 2")
	assert.Contains(t, msg.summary, "Changed files: 2")
	assert.Contains(t, msg.summary, "Additions: 18")
	assert.Contains(t, msg.summary, "Deletions: 2")
	assert.Contains(t, msg.summary, "abcdef1 first change")
}

func TestCompareReleaseCommandEmitsPanelMessages(t *testing.T) {
	t.Parallel()

	ghMock := &mockGHClientFull{
		comparison: &gh.CommitsComparison{
			TotalCommits: gh.Ptr(1),
			Files: []*gh.CommitFile{
				{Filename: gh.Ptr("README.md"), Status: gh.Ptr("modified"), Additions: gh.Ptr(3), Deletions: gh.Ptr(1)},
			},
			Commits: []*gh.RepositoryCommit{
				{SHA: gh.Ptr("abcdef123456"), Commit: &gh.Commit{Message: gh.Ptr("release note")}},
			},
		},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabReleases
	p.tabCursor[tabReleases] = 0
	p.tabItems[tabReleases] = []listItem{
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.1.0"}},
		{kind: kindRelease, release: ghReleaseItem{TagName: "v1.0.0"}},
	}

	_, cmd := p.compareRelease()
	require.NotNil(t, cmd)
	loaded := cmd().(releaseCompareLoadedMsg)
	assert.Equal(t, "owner", ghMock.compareOwner)
	assert.Equal(t, "repo", ghMock.compareRepo)
	assert.Equal(t, "v1.0.0", ghMock.compareBase)
	assert.Equal(t, "v1.1.0", ghMock.compareHead)

	_, outCmd := p.handleReleaseCompareLoaded(loaded)
	msgs := collectCmdMsgs(outCmd)

	preview := findMsg[panels.ReleaseComparePreviewMsg](t, msgs)
	files := findMsg[panels.ReleaseCompareFilesLoadedMsg](t, msgs)
	commits := findMsg[panels.ReleaseCompareCommitsLoadedMsg](t, msgs)
	assert.Contains(t, preview.Content, "Commits: 1")
	assert.Len(t, files.Files, 1)
	assert.Len(t, commits.Commits, 1)
}
