package gitinfo

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// parseReviewerLogins — pure input parsing
// ---------------------------------------------------------------------------

func TestParseReviewerLogins(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple", "octocat, hubot", []string{"octocat", "hubot"}},
		{"trims whitespace", "  octocat ,  hubot  ", []string{"octocat", "hubot"}},
		{"strips at prefix", "@octocat, @hubot", []string{"octocat", "hubot"}},
		{"dedupes case-insensitively", "octocat, OCTOCAT, octocat", []string{"octocat"}},
		{"drops empty entries", "octocat,,hubot,", []string{"octocat", "hubot"}},
		{"single login", "octocat", []string{"octocat"}},
		{"empty", "", nil},
		{"only separators and spaces", " , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseReviewerLogins(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// doRequestReviewers — guard rails
// ---------------------------------------------------------------------------

func TestDoRequestReviewers_OpenPR(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Add auth", State: prStateOpen}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.doRequestReviewers()
	assert.NotNil(t, cmd, "open PR should open the reviewer input")
	assert.Equal(t, opPRRequestReviewers, p.pending)
	assert.Equal(t, "42", p.pendingName)
}

func TestDoRequestReviewers_NonOpenPR(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 7, Title: "Old", State: prStateMerged}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.doRequestReviewers()
	assert.NotNil(t, cmd, "should return a warning toast for a non-open PR")
	assert.Equal(t, opNone, p.pending, "pending should not be set for a non-open PR")
}

func TestDoRequestReviewers_WrongTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "main"}},
	}
	p.tabCursor[tabBranches] = 0

	_, cmd := p.doRequestReviewers()
	assert.Nil(t, cmd)
}

func TestDoRequestReviewers_EmptyCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{}

	_, cmd := p.doRequestReviewers()
	assert.Nil(t, cmd)
}

func TestDoRequestReviewers_NoClient(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Add auth", State: prStateOpen}},
	}
	p.tabCursor[tabPRs] = 0
	// ghClient is nil.

	_, cmd := p.doRequestReviewers()
	assert.Nil(t, cmd)
	assert.Equal(t, opNone, p.pending)
}

// ---------------------------------------------------------------------------
// handleModalResult — opPRRequestReviewers
// ---------------------------------------------------------------------------

func TestHandleModalResult_PRRequestReviewers_RequestThenRefresh(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Add auth", State: prStateOpen}},
	}
	p.tabCursor[tabPRs] = 0
	p.pending = opPRRequestReviewers
	p.pendingName = "42"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "octocat, @hubot, OCTOCAT"})
	require.NotNil(t, cmd, "submitting logins should trigger the request")

	// Executing the async command calls the client with parsed logins.
	msg := cmd()
	res, ok := msg.(prRequestReviewersResultMsg)
	require.True(t, ok)
	require.NoError(t, res.err)
	assert.Equal(t, 42, res.number)
	assert.Equal(t, []string{"octocat", "hubot"}, res.reviewers)
	assert.Equal(t, 1, ghMock.requestReviewersCalls)
	assert.Equal(t, []string{"octocat", "hubot"}, ghMock.requestedReviewers)

	// Feeding the result back emits a toast plus a PR-detail refresh.
	_, refreshCmd := p.handlePRRequestReviewersResult(res)
	assert.NotNil(t, refreshCmd, "success should emit toast + PR detail refresh")
}

func TestHandleModalResult_PRRequestReviewers_EmptyCancels(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.pending = opPRRequestReviewers
	p.pendingName = "42"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "   ,  , "})
	assert.Nil(t, cmd, "empty/whitespace-only logins should cancel")
}

func TestHandleModalResult_PRRequestReviewers_BadNumber(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.pending = opPRRequestReviewers
	p.pendingName = "not-a-number"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "octocat"})
	assert.Nil(t, cmd)
}

func TestHandleModalResult_PRRequestReviewers_Cancel(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRRequestReviewers
	p.pendingName = "42"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancel should produce a nil cmd")
	assert.Equal(t, opNone, p.pending)
}

// ---------------------------------------------------------------------------
// requestReviewersCmd / handlePRRequestReviewersResult
// ---------------------------------------------------------------------------

func TestRequestReviewersCmd_Error(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{reviewersErr: errors.New("unknown login: nope")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	cmd := p.requestReviewersCmd(42, []string{"nope"})
	require.NotNil(t, cmd)
	msg := cmd()
	res, ok := msg.(prRequestReviewersResultMsg)
	require.True(t, ok)
	require.Error(t, res.err)
	assert.Equal(t, []string{"nope"}, ghMock.requestedReviewers)
}

func TestHandlePRRequestReviewersResult_Success(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	_, cmd := p.handlePRRequestReviewersResult(prRequestReviewersResultMsg{
		number:    42,
		reviewers: []string{"octocat", "hubot"},
	})
	assert.NotNil(t, cmd, "success should produce a toast + refresh")
}

func TestHandlePRRequestReviewersResult_Error(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRRequestReviewersResult(prRequestReviewersResultMsg{
		number:    42,
		reviewers: []string{"nope"},
		err:       errors.New("unknown login"),
	})
	assert.NotNil(t, cmd, "error should produce an error toast")
}

// ---------------------------------------------------------------------------
// Key binding — 'R' triggers request-reviewers on the PRs tab
// ---------------------------------------------------------------------------

func TestHandleKey_R_PRsTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Test", State: prStateOpen}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: -1, Text: "R"})
	assert.NotNil(t, cmd, "pressing 'R' on PRs tab should open the reviewer input")
	assert.Equal(t, opPRRequestReviewers, p.pending)
}

func TestHandleKey_R_NotPRsTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabBranches

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: -1, Text: "R"})
	assert.Nil(t, cmd, "pressing 'R' outside the PRs tab should be a no-op")
}

func TestHandleKey_R_NoGHClient(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabPRs

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: -1, Text: "R"})
	assert.Nil(t, cmd, "pressing 'R' without a ghClient should be a no-op")
}
