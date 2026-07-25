package gitinfo

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
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

// TestHandleWorkflowInputsFetched_KnownInputsUsePicker verifies that a declared
// choice input is presented as an action picker preselected on its default, so
// the user can pick a valid value (issue: parameter picker).
func TestHandleWorkflowInputsFetched_KnownInputsUsePicker(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   7,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs: []ghclient.WorkflowInput{
			{Name: "env", Type: "choice", Options: []string{"dev", "staging", "prod"}, Default: "staging"},
		},
	})
	require.NotNil(t, cmd)

	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind, "choice input should use a picker")
	assert.Equal(t, "staging", modal.SelectedID, "picker should preselect the declared default")
	require.Len(t, modal.Actions, 3)
	assert.Equal(t, "dev", modal.Actions[0].ID)
	assert.Equal(t, opWorkflowDispatchInputs, p.pending)
}

// TestHandleWorkflowInputsFetched_FreeFormInputUsesTextField verifies a string
// input is a text field pre-filled with its default, so the user can accept it
// or type a custom value.
func TestHandleWorkflowInputsFetched_FreeFormInputUsesTextField(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   7,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs:       []ghclient.WorkflowInput{{Name: "tag", Default: "v1.0"}},
	})
	require.NotNil(t, cmd)

	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalInput, modal.Kind, "free-form input should use a text field")
	assert.Equal(t, "v1.0", modal.Value, "text field should pre-fill the declared default")
}

// TestHandleWorkflowInputsFetched_UnknownInputsUseRawComposer verifies the
// free-form key=value composer is offered when the workflow inputs could not be
// read (fetch/parse failed).
func TestHandleWorkflowInputsFetched_UnknownInputsUseRawComposer(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   7,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  false,
	})
	require.NotNil(t, cmd)

	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalMultilineInput, modal.Kind, "unreadable inputs fall back to the composer")
	assert.Equal(t, opWorkflowDispatchRaw, p.pending)
}

// TestHandleWorkflowInputsFetched_NoInputsDispatchesDirectly verifies a
// workflow with no declared inputs dispatches immediately without prompting.
func TestHandleWorkflowInputsFetched_NoInputsDispatchesDirectly(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   42,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs:       nil,
	})
	require.NotNil(t, cmd)
	result := findMsg[workflowDispatchResultMsg](t, collectCmdMsgs(cmd))
	require.NoError(t, result.err)
	assert.Equal(t, 1, ghMock.dispatchCalls, "no-input workflow should dispatch directly")
	assert.Nil(t, ghMock.dispatchInputs, "no inputs collected")
}

// TestFireWorkflowDispatch_NilClientNoPanic verifies the per-input dispatch
// path surfaces a nil client as a failure toast rather than crashing the TUI.
func TestFireWorkflowDispatch_NilClientNoPanic(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = nil
	p.wfDispatch = workflowDispatchDraft{workflowID: 9, workflowName: "Deploy", ref: "main"}

	var result workflowDispatchResultMsg
	assert.NotPanics(t, func() {
		_, cmd := p.fireWorkflowDispatch()
		require.NotNil(t, cmd)
		var ok bool
		result, ok = cmd().(workflowDispatchResultMsg)
		require.True(t, ok)
	})
	require.Error(t, result.err, "nil client must yield an error result")
	assert.Empty(t, p.wfDispatch.workflowName, "draft should be reset after firing")
}

// TestHandleWorkflowDispatchRaw_NilClientNoPanic verifies the free-form
// fallback path guards a nil client instead of dereferencing it (issue #361).
func TestHandleWorkflowDispatchRaw_NilClientNoPanic(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = nil
	p.pending = opWorkflowDispatchRaw
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
// flow (D → ref → per-input prompt → submit → result) through the panel's
// Update and asserts it dispatches exactly once with the collected input and
// ends with a success toast, without panicking (issue #361 AC #2, #5).
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
		require.True(t, fetched.inputsKnown)

		// Step 3: the single declared input appears as a text field.
		_, cmd = p.Update(fetched)
		modal := findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))
		require.Equal(t, notify.ModalInput, modal.Kind)
		require.Equal(t, "prod", modal.Value)
		require.Equal(t, opWorkflowDispatchInputs, p.pending)

		// Step 4: submit the input value → dispatch fires.
		_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "prod"})
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

// TestHandleWorkflowInputsFetched_BooleanInputUsesPicker verifies a boolean
// input is offered as a true/false picker preselected on the declared default.
func TestHandleWorkflowInputsFetched_BooleanInputUsesPicker(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   7,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs:       []ghclient.WorkflowInput{{Name: "dry_run", Type: "boolean", Default: "true"}},
	})
	modal := findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))
	require.Equal(t, notify.ModalActionPicker, modal.Kind)
	assert.Equal(t, "true", modal.SelectedID)
	require.Len(t, modal.Actions, 2)
	assert.Equal(t, "true", modal.Actions[0].ID)
	assert.Equal(t, "false", modal.Actions[1].ID)
}

// TestWorkflowDispatch_MultiInput_CollectsChoiceAndText drives a two-input
// dispatch (a choice then a free-form string), asserting each value is
// collected and passed to the dispatch.
func TestWorkflowDispatch_MultiInput_CollectsChoiceAndText(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   42,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs: []ghclient.WorkflowInput{
			{Name: "env", Type: "choice", Options: []string{"dev", "prod"}, Default: "prod"},
			{Name: "tag", Default: "v1.0"},
		},
	})
	// First prompt: choice picker preselected on "prod".
	modal := findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))
	require.Equal(t, notify.ModalActionPicker, modal.Kind)
	require.Equal(t, "prod", modal.SelectedID)

	// User picks "dev" → second prompt is a text field pre-filled with default.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "dev"})
	modal = findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))
	require.Equal(t, notify.ModalInput, modal.Kind)
	require.Equal(t, "v1.0", modal.Value)

	// User enters a custom tag → dispatch fires with both values.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "custom"})
	result := findMsg[workflowDispatchResultMsg](t, collectCmdMsgs(cmd))
	require.NoError(t, result.err)

	require.Equal(t, 1, ghMock.dispatchCalls)
	require.NotNil(t, ghMock.dispatchInputs)
	assert.Equal(t, "dev", ghMock.dispatchInputs["env"])
	assert.Equal(t, "custom", ghMock.dispatchInputs["tag"])
}

// TestWorkflowDispatch_EmptyValueKeepsDefault verifies an input left blank is
// omitted, so GitHub applies the workflow's declared default.
func TestWorkflowDispatch_EmptyValueKeepsDefault(t *testing.T) {
	t.Parallel()
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	_, cmd := p.handleWorkflowInputsFetched(workflowInputsFetchedMsg{
		workflowID:   42,
		workflowName: "Deploy",
		ref:          "main",
		inputsKnown:  true,
		inputs:       []ghclient.WorkflowInput{{Name: "tag", Default: "v1.0"}},
	})
	_ = findMsg[notify.ShowModalMsg](t, collectCmdMsgs(cmd))

	// Submit an empty value → omitted from inputs.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	result := findMsg[workflowDispatchResultMsg](t, collectCmdMsgs(cmd))
	require.NoError(t, result.err)
	assert.Equal(t, 1, ghMock.dispatchCalls)
	assert.Nil(t, ghMock.dispatchInputs, "blank input should be omitted, keeping the default")
}

// TestGuardedGitHubCmd_RecoversPanicToPanicMsg verifies a panic inside a GitHub
// command goroutine is captured to a crash report and converted to a
// ghCmdPanicMsg (handled on the main goroutine) instead of killing the TUI
// (issue #361 durable fix).
func TestGuardedGitHubCmd_RecoversPanicToPanicMsg(t *testing.T) {
	// Redirect crash reports to a temp dir so the real data dir stays clean.
	orig := xdg.DataHome
	xdg.DataHome = t.TempDir()
	t.Cleanup(func() { xdg.DataHome = orig })

	cmd := guardedGitHubCmd("test.panic", func() tea.Msg {
		panic("kaboom")
	})
	var msg tea.Msg
	assert.NotPanics(t, func() { msg = cmd() })
	_, ok := msg.(ghCmdPanicMsg)
	require.True(t, ok, "panic should be converted to ghCmdPanicMsg, got %T", msg)
}

// TestHandleGHCmdPanic_ClearsLoadingAndToasts verifies the panic message clears
// stranded loading flags and surfaces an error toast.
func TestHandleGHCmdPanic_ClearsLoadingAndToasts(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	for i := range p.tabPaging {
		p.tabPaging[i].loading = true
	}
	p.gh.notifLoading = true

	_, cmd := p.handleGHCmdPanic(ghCmdPanicMsg{label: "gitinfo.loadIssuesPage"})
	for i := range p.tabPaging {
		assert.False(t, p.tabPaging[i].loading, "loading flag %d should be cleared", i)
	}
	assert.False(t, p.gh.notifLoading, "notifLoading should be cleared")
	toast := findMsg[notify.ShowToastMsg](t, collectCmdMsgs(cmd))
	assert.Equal(t, notify.Error, toast.Level)
}

// TestGuardedGitHubCmd_PassesThroughNormalMsg verifies the wrapper is
// transparent when the command does not panic.
func TestGuardedGitHubCmd_PassesThroughNormalMsg(t *testing.T) {
	t.Parallel()
	cmd := guardedGitHubCmd("test.ok", func() tea.Msg {
		return workflowDispatchResultMsg{workflowName: "X"}
	})
	result, ok := cmd().(workflowDispatchResultMsg)
	require.True(t, ok)
	assert.Equal(t, "X", result.workflowName)
}

// TestGuardedCommand_NilClientYieldsToast verifies a GitHub action command
// (representative: cancelWorkflowRunCmd) surfaces a nil client as an error
// toast via ghClientUnavailableCmd rather than dereferencing it.
func TestGuardedCommand_NilClientYieldsToast(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = nil // no GitHub client configured

	cmd := p.cancelWorkflowRunCmd(1)
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok, "nil client should yield a toast, got %T", cmd())
	assert.Equal(t, notify.Error, toast.Level)
	assert.Equal(t, errGitHubClientUnavailable.Error(), toast.Message)
}

// TestWorkflowInputPrompt covers the prompt-label construction branches:
// description vs. name fallback, name prefixing, and the required marker.
func TestWorkflowInputPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   ghclient.WorkflowInput
		want string
	}{
		{"name only", ghclient.WorkflowInput{Name: "env"}, "env"},
		{"description prefixed with name", ghclient.WorkflowInput{Name: "env", Description: "Target environment"}, "env: Target environment"},
		{"required flag", ghclient.WorkflowInput{Name: "env", Required: true}, "env (required)"},
		{"description + required", ghclient.WorkflowInput{Name: "env", Description: "Target", Required: true}, "env: Target (required)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, workflowInputPrompt(tc.in))
		})
	}
}
