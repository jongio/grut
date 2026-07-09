package gitinfo

import (
	"errors"
	"testing"

	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// parseIssueLabels
// ---------------------------------------------------------------------------

func TestParseIssueLabels(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "bug", []string{"bug"}},
		{"trims spaces", " bug , ui ", []string{"bug", "ui"}},
		{"drops empties", "bug,,ui,", []string{"bug", "ui"}},
		{"dedupes", "bug, ui, bug", []string{"bug", "ui"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseIssueLabels(tt.in))
		})
	}
}

// ---------------------------------------------------------------------------
// doCreate on the Issues tab
// ---------------------------------------------------------------------------

func TestDoCreate_IssuesTab_StartsFlowWithClient(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabIssues

	_, cmd := p.doCreate()

	assert.Equal(t, opIssueCreateTitle, p.pending)
	require.NotNil(t, cmd, "should show the title input overlay")
}

func TestDoCreate_IssuesTab_NoClientDoesNotStartFlow(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = nil
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabIssues

	_, cmd := p.doCreate()

	assert.Equal(t, opNone, p.pending, "no in-TUI create flow without an API client")
	require.NotNil(t, cmd, "falls back to the browser new-issue page")
}

// ---------------------------------------------------------------------------
// Validation: empty title is rejected and the flow stays recoverable
// ---------------------------------------------------------------------------

func TestIssueCreate_EmptyTitleReprompts(t *testing.T) {
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabIssues
	p.pending = opIssueCreateTitle

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "   "})

	assert.Equal(t, opIssueCreateTitle, p.pending, "should re-prompt for the title")
	require.NotNil(t, cmd, "re-prompt + inline warning command expected")
	assert.Zero(t, ghMock.createIssueCalls, "no issue created on empty title")
}

func TestIssueCreate_EscapeCancelsAndClearsDraft(t *testing.T) {
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabIssues
	p.pending = opIssueCreateBody
	p.gh.issueDraftTitle = "Half-typed"
	p.gh.issueDraftBody = "partial"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})

	assert.Equal(t, opNone, p.pending)
	assert.Empty(t, p.gh.issueDraftTitle)
	assert.Empty(t, p.gh.issueDraftBody)
	assert.Nil(t, cmd)
	assert.Zero(t, ghMock.createIssueCalls)
}

// ---------------------------------------------------------------------------
// createIssueCmd builds the correct request
// ---------------------------------------------------------------------------

func TestCreateIssueCmd_BuildsRequestWithAllFields(t *testing.T) {
	ghMock := &mockGHClientFull{
		createIssueResp: &gh.Issue{Number: gh.Ptr(42), Title: gh.Ptr("My title")},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.createIssueCmd("My title", "My body", []string{"bug", "ui"})()

	res, ok := msg.(issueCreateResultMsg)
	require.True(t, ok)
	require.NoError(t, res.err)
	assert.Equal(t, 42, res.number)

	require.NotNil(t, ghMock.createIssueReq)
	assert.Equal(t, "My title", ghMock.createIssueReq.GetTitle())
	assert.Equal(t, "My body", ghMock.createIssueReq.GetBody())
	require.NotNil(t, ghMock.createIssueReq.Labels)
	assert.Equal(t, []string{"bug", "ui"}, *ghMock.createIssueReq.Labels)
}

func TestCreateIssueCmd_OmitsEmptyBodyAndLabels(t *testing.T) {
	ghMock := &mockGHClientFull{
		createIssueResp: &gh.Issue{Number: gh.Ptr(7), Title: gh.Ptr("T")},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	p.createIssueCmd("T", "", nil)()

	require.NotNil(t, ghMock.createIssueReq)
	assert.Equal(t, "T", ghMock.createIssueReq.GetTitle())
	assert.Nil(t, ghMock.createIssueReq.Body, "empty body should be omitted")
	assert.Nil(t, ghMock.createIssueReq.Labels, "empty labels should be omitted")
}

func TestCreateIssueCmd_SurfacesError(t *testing.T) {
	ghMock := &mockGHClientFull{createIssueErr: errors.New("boom")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.createIssueCmd("T", "", nil)()

	res, ok := msg.(issueCreateResultMsg)
	require.True(t, ok)
	require.Error(t, res.err)

	// The result handler surfaces a clear error notification.
	_, cmd := p.handleIssueCreateResult(res)
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Create issue failed")
	assert.Zero(t, p.gh.pendingSelectIssue, "no selection queued on failure")
}

// ---------------------------------------------------------------------------
// Full create-then-refresh-then-select flow
// ---------------------------------------------------------------------------

func TestIssueCreate_FullFlowRefreshesAndSelects(t *testing.T) {
	newNum := 99
	newTitle := "New issue"
	ghMock := &mockGHClientFull{
		createIssueResp: &gh.Issue{Number: &newNum, Title: &newTitle, State: gh.Ptr("open")},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.pageSize = 25
	p.activeTab = tabIssues

	// Press n -> title prompt.
	if _, cmd := p.doCreate(); assert.NotNil(t, cmd) {
		assert.Equal(t, opIssueCreateTitle, p.pending)
	}

	// Step 1: title.
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "New issue"})
	assert.Equal(t, opIssueCreateBody, p.pending)
	assert.Equal(t, "New issue", p.gh.issueDraftTitle)

	// Step 2: body.
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "Body text"})
	assert.Equal(t, opIssueCreateLabels, p.pending)
	assert.Equal(t, "Body text", p.gh.issueDraftBody)

	// Step 3: labels -> creates the issue.
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "bug, ui"})
	require.NotNil(t, cmd)
	res, ok := cmd().(issueCreateResultMsg)
	require.True(t, ok)
	require.NoError(t, res.err)
	assert.Equal(t, 99, res.number)
	assert.Equal(t, opNone, p.pending, "pending cleared after submit")

	// Request carried title, body, and labels.
	require.NotNil(t, ghMock.createIssueReq)
	assert.Equal(t, "New issue", ghMock.createIssueReq.GetTitle())
	assert.Equal(t, "Body text", ghMock.createIssueReq.GetBody())
	require.NotNil(t, ghMock.createIssueReq.Labels)
	assert.Equal(t, []string{"bug", "ui"}, *ghMock.createIssueReq.Labels)

	// Result queues a refresh + selection of the new issue.
	_, cmd = p.handleIssueCreateResult(res)
	require.NotNil(t, cmd)
	assert.Equal(t, 99, p.gh.pendingSelectIssue)
	assert.Equal(t, tabIssues, p.activeTab)

	// The refreshed list contains the new issue; selection lands on it.
	ghMock.issues = []*gh.Issue{
		{Number: gh.Ptr(1), Title: gh.Ptr("Existing"), State: gh.Ptr("open")},
		{Number: &newNum, Title: &newTitle, State: gh.Ptr("open")},
	}
	pageMsg, ok := p.loadIssuesPage(1, true)().(ghIssuesPageMsg)
	require.True(t, ok)
	_, selCmd := p.handleIssuesPage(pageMsg)

	assert.Zero(t, p.gh.pendingSelectIssue, "selection consumed after refresh")
	require.Len(t, p.tabItems[tabIssues], 2)
	assert.Equal(t, 1, p.tabCursor[tabIssues], "cursor on the newly created issue")
	require.NotNil(t, selCmd, "emits an issue-selected command")
}
