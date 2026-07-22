package gitinfo

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findMsg returns the first message of type T in msgs, failing the test if
// none is present. It is used to assert on the messages a batched command
// emits during the multi-step workflow-dispatch flow.
func findMsg[T tea.Msg](t *testing.T, msgs []tea.Msg) T {
	t.Helper()
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			return v
		}
	}
	var zero T
	require.Failf(t, "message not found", "expected a %T among %d messages", zero, len(msgs))
	return zero
}

// TestHandleWorkflowInputsFetched_UsesMultilineModal verifies the inputs step
// uses a multi-line composer (issue #361 AC #4): the content is one key=value
// per line, which a single-line input can neither hold nor let the user extend.
func TestHandleWorkflowInputsFetched_UsesMultilineModal(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   7,
		workflowName: "Deploy",
		ref:          "main",
		inputs: []ghclient.WorkflowInput{
			{Name: "env", Default: "prod"},
			{Name: "version", Default: "1.0"},
		},
	})
	require.NotNil(t, cmd)

	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalMultilineInput, modal.Kind,
		"multi-line key=value content needs a multi-line composer")
	assert.Equal(t, "env=prod\nversion=1.0", modal.Value)
}

// TestHandleWorkflowDispatchInputs_NilClientNoPanic verifies a nil GitHub
// client surfaces as a failure toast rather than a nil dereference that crashes
// the TUI (issue #361 AC #3).
func TestHandleWorkflowDispatchInputs_NilClientNoPanic(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = nil
	p.pending = opWorkflowDispatchInputs
	p.pendingName = "9:Deploy:main"

	var result workflowDispatchResultMsg
	assert.NotPanics(t, func() {
		_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "env=prod"})
		require.NotNil(t, cmd)
		var ok bool
		result, ok = cmd().(workflowDispatchResultMsg)
		require.True(t, ok)
	})
	require.Error(t, result.err, "nil client must yield an error result")

	// The result handler renders that error as a failure toast.
	_, toastCmd := p.handleWorkflowDispatchResult(result)
	toast := findMsg[notify.ShowToastMsg](t, collectCmdMsgs(toastCmd))
	assert.Equal(t, notify.Error, toast.Level)
}

// TestWorkflowDispatch_FullFlow_NoPanicSuccessToast drives the entire dispatch
// flow (D → ref → inputs → submit → result) through the panel's Update and
// asserts it dispatches exactly once and ends with a success toast, without
// panicking (issue #361 AC #2, #5).
func TestWorkflowDispatch_FullFlow_NoPanicSuccessToast(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{
		workflowInputs: []ghclient.WorkflowInput{{Name: "env", Default: "prod"}},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 42, Name: "Deploy", Path: ".github/workflows/deploy.yml"}},
	}
	p.tabCursor[tabWorkflows] = 0

	assert.NotPanics(t, func() {
		// Step 1: trigger dispatch (ref prompt).
		_, cmd := p.doWorkflowDispatch()
		require.NotNil(t, cmd)
		require.Equal(t, opWorkflowDispatch, p.pending)

		// Step 2: submit ref → fetch workflow inputs.
		_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "main"})
		fetched := findMsg[workflowInputsFetchedMsg](t, collectCmdMsgs(cmd))
		require.Equal(t, "main", fetched.ref)

		// Step 3: inputs modal appears as a multi-line composer.
		_, cmd = p.Update(fetched)
		modal := findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))
		require.Equal(t, notify.ModalMultilineInput, modal.Kind)
		require.Equal(t, opWorkflowDispatchInputs, p.pending)

		// Step 4: submit inputs → dispatch fires.
		_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "env=prod"})
		result := findMsg[workflowDispatchResultMsg](t, collectCmdMsgs(cmd))
		require.NoError(t, result.err)

		// Step 5: result handled → success toast + data reload, no panic.
		_, cmd = p.Update(result)
		toast := findMsg[notify.ShowToastMsg](t, collectCmdMsgs(cmd))
		assert.Equal(t, notify.Info, toast.Level)
		assert.Contains(t, toast.Message, "Dispatched Deploy")
	})

	assert.Equal(t, 1, ghMock.dispatchCalls, "dispatch should fire exactly once")
	assert.Equal(t, int64(42), ghMock.dispatchID)
	assert.Equal(t, "main", ghMock.dispatchRef)
	require.NotNil(t, ghMock.dispatchInputs)
	assert.Equal(t, "prod", ghMock.dispatchInputs["env"])
}

// TestWorkflowDispatch_RenderAfterReload_NoPanic renders the panel across a
// range of sizes after a dispatch result and data reload, guarding the
// post-dispatch render path against panics (issue #361 AC #2).
func TestWorkflowDispatch_RenderAfterReload_NoPanic(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.activeTab = tabWorkflows
	p.tabItems[tabWorkflows] = []listItem{
		{kind: kindWorkflow, workflow: ghWorkflowItem{ID: 42, Name: "Deploy", Path: ".github/workflows/deploy.yml", State: "active"}},
	}
	p.tabCursor[tabWorkflows] = 0

	assert.NotPanics(t, func() {
		_, cmd := p.Update(workflowDispatchResultMsg{workflowName: "Deploy"})
		for _, m := range collectCmdMsgs(cmd) {
			if m != nil {
				p.Update(m)
			}
		}
		for _, sz := range [][2]int{{120, 40}, {80, 24}, {40, 10}, {20, 6}, {10, 3}} {
			_ = p.View(sz[0], sz[1])
		}
	})
}
