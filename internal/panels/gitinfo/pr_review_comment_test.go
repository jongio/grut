package gitinfo

import (
	"testing"

	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostPRReviewCommentCreatesLineAnchoredReviewComment(t *testing.T) {
	t.Parallel()

	ghMock := &mockGHClientFull{
		pr: &gh.PullRequest{Head: &gh.PullRequestBranch{SHA: gh.Ptr("abc123")}},
	}
	p := newPRReviewCommentTestPanel(t, ghMock)

	cmd := p.postPRReviewCommentCmd(panels.PostPRReviewCommentMsg{
		Number:  42,
		Path:    "src/main.go",
		Line:    11,
		HasLine: true,
		Body:    "Looks good",
	})
	require.NotNil(t, cmd)
	result, ok := cmd().(prReviewCommentResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	assert.False(t, result.fallback)
	assert.Equal(t, 1, ghMock.getPRCalls)
	assert.Equal(t, 1, ghMock.reviewCommentCalls)
	assert.Equal(t, 42, ghMock.reviewCommentPR)
	assert.Equal(t, "abc123", ghMock.reviewCommentCommitID)
	assert.Equal(t, "src/main.go", ghMock.reviewCommentPath)
	assert.Equal(t, 11, ghMock.reviewCommentLine)
	assert.Equal(t, "Looks good", ghMock.reviewCommentBody)
	assert.Zero(t, ghMock.commentCalls)
}

func TestPostPRReviewCommentFallsBackToPlainPRCommentWithoutLine(t *testing.T) {
	t.Parallel()

	ghMock := &mockGHClientFull{}
	p := newPRReviewCommentTestPanel(t, ghMock)

	cmd := p.postPRReviewCommentCmd(panels.PostPRReviewCommentMsg{
		Number: 42,
		Path:   "src/main.go",
		Body:   "Question",
	})
	require.NotNil(t, cmd)
	result, ok := cmd().(prReviewCommentResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	assert.True(t, result.fallback)
	assert.Zero(t, ghMock.getPRCalls)
	assert.Zero(t, ghMock.reviewCommentCalls)
	assert.Equal(t, 1, ghMock.commentCalls)
	assert.Equal(t, 42, ghMock.commentNumber)
	assert.Equal(t, "Question", ghMock.commentBody)
}

func newPRReviewCommentTestPanel(t *testing.T, ghMock *mockGHClientFull) *Panel {
	t.Helper()
	p := &Panel{}
	p.ctx = t.Context()
	p.gh.client = ghMock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	return p
}
