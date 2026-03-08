package notify

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Level tests ---

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{Info, "INFO"},
		{Warn, "WARN"},
		{Error, "ERROR"},
		{Success, "OK"},
		{Level(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.level.String())
	}
}

// --- Manager creation ---

func TestNewManager(t *testing.T) {
	m := NewManager()
	require.NotNil(t, m)
	assert.Equal(t, 0, m.ToastCount())
	assert.Equal(t, 0, m.InlineCount())
	assert.False(t, m.HasModal())
}

// --- Toast tests ---

func TestAddToastCreatesNotification(t *testing.T) {
	m := NewManager()
	cmd := m.AddToast("hello", Info)

	assert.Equal(t, 1, m.ToastCount())
	assert.NotNil(t, cmd, "AddToast should return a tick command")
}

func TestAddToastWithDuration(t *testing.T) {
	m := NewManager()
	cmd := m.AddToastWithDuration("custom", Warn, 5*time.Second)

	assert.Equal(t, 1, m.ToastCount())
	assert.NotNil(t, cmd)

	// Verify the toast has the correct duration
	assert.Equal(t, 5*time.Second, m.toastDuration(0))
}

func TestMultipleToastsStack(t *testing.T) {
	m := NewManager()
	m.AddToast("first", Info)
	m.AddToast("second", Warn)
	m.AddToast("third", Error)

	assert.Equal(t, 3, m.ToastCount())

	// Each toast has a unique ID
	ids := m.toastIDs()
	uniqIDs := make(map[int64]bool)
	for _, id := range ids {
		uniqIDs[id] = true
	}
	assert.Len(t, uniqIDs, 3, "each toast should have a unique ID")
}

func TestToastExpiredMsg(t *testing.T) {
	m := NewManager()
	m.AddToast("will expire", Info)

	id := m.toastID(0)

	// Simulate the toast expiry message
	cmd := m.Update(ToastExpiredMsg{ID: id})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, m.ToastCount(), "expired toast should be removed")
}

func TestToastExpiredMsgUnknownID(t *testing.T) {
	m := NewManager()
	m.AddToast("stays", Info)

	// Expire with a non-existent ID — should be a no-op
	m.Update(ToastExpiredMsg{ID: 9999})
	assert.Equal(t, 1, m.ToastCount(), "toast with different ID should remain")
}

func TestMultipleToastsDismissIndependently(t *testing.T) {
	m := NewManager()
	m.AddToast("first", Info)
	m.AddToast("second", Warn)
	m.AddToast("third", Error)

	assert.Equal(t, 3, m.ToastCount())

	// Expire the middle toast
	middleID := m.toastID(1)

	m.Update(ToastExpiredMsg{ID: middleID})
	assert.Equal(t, 2, m.ToastCount())

	// Verify remaining toasts
	assert.Equal(t, "first", m.toastMessage(0))
	assert.Equal(t, "third", m.toastMessage(1))
}

// --- ShowToastMsg handling ---

func TestUpdateShowToastMsg(t *testing.T) {
	m := NewManager()
	cmd := m.Update(ShowToastMsg{
		Message: "from panel",
		Level:   Success,
	})

	assert.Equal(t, 1, m.ToastCount())
	assert.NotNil(t, cmd, "should return a tick command")
}

func TestUpdateShowToastMsgWithDuration(t *testing.T) {
	m := NewManager()
	m.Update(ShowToastMsg{
		Message:  "custom duration",
		Level:    Warn,
		Duration: 10 * time.Second,
	})

	assert.Equal(t, 10*time.Second, m.toastDuration(0))
}

func TestUpdateShowToastMsgDefaultDuration(t *testing.T) {
	m := NewManager()
	m.Update(ShowToastMsg{
		Message:  "default",
		Level:    Info,
		Duration: 0, // should use DefaultToastDuration
	})

	assert.Equal(t, DefaultToastDuration, m.toastDuration(0))
}

// --- Inline tests ---

func TestAddInline(t *testing.T) {
	m := NewManager()
	m.AddInline("git-missing", "git not found in PATH", Error)

	assert.Equal(t, 1, m.InlineCount())
}

func TestDismissInline(t *testing.T) {
	m := NewManager()
	m.AddInline("git-missing", "git not found", Error)
	assert.Equal(t, 1, m.InlineCount())

	m.DismissInline("git-missing")
	assert.Equal(t, 0, m.InlineCount())
}

func TestDismissInlineNonExistent(t *testing.T) {
	m := NewManager()
	m.AddInline("exists", "msg", Info)

	// Dismissing a non-existent ID is a no-op
	m.DismissInline("nope")
	assert.Equal(t, 1, m.InlineCount())
}

func TestAddInlineReplacesExisting(t *testing.T) {
	m := NewManager()
	m.AddInline("status", "old message", Info)
	m.AddInline("status", "new message", Warn)

	assert.Equal(t, 1, m.InlineCount())

	assert.Equal(t, "new message", m.inlineMessage("status"))
	assert.Equal(t, Warn, m.inlineLevel("status"))
}

func TestInlinePersistsAcrossUpdates(t *testing.T) {
	m := NewManager()
	m.AddInline("warning", "something wrong", Warn)

	// Process unrelated messages — inline should persist
	m.Update(ToastExpiredMsg{ID: 999})
	assert.Equal(t, 1, m.InlineCount())
}

// --- View rendering tests ---

func TestViewEmptyManager(t *testing.T) {
	m := NewManager()
	assert.Empty(t, m.View(80))
}

func TestViewZeroWidth(t *testing.T) {
	m := NewManager()
	m.AddToast("hello", Info)
	assert.Empty(t, m.View(0))
}

func TestViewWithToasts(t *testing.T) {
	m := NewManager()
	m.AddToast("Build succeeded", Success)
	m.AddToast("Lint warning", Warn)

	output := m.View(80)
	assert.Contains(t, output, "Build succeeded")
	assert.Contains(t, output, "Lint warning")
}

func TestViewWithInline(t *testing.T) {
	m := NewManager()
	m.AddInline("err", "git not found", Error)

	output := m.View(80)
	assert.Contains(t, output, "git not found")
}

func TestViewWithMixed(t *testing.T) {
	m := NewManager()
	m.AddToast("toast msg", Info)
	m.AddInline("inline-id", "inline msg", Warn)

	output := m.View(80)
	assert.Contains(t, output, "toast msg")
	assert.Contains(t, output, "inline msg")
}

func TestToastViewRendering(t *testing.T) {
	to := toast{
		id: 1,
		notification: Notification{
			Message: "Test message",
			Level:   Info,
		},
	}

	output := to.view(80)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Test message")
}

func TestToastViewNarrowWidth(t *testing.T) {
	to := toast{
		id: 1,
		notification: Notification{
			Message: "msg",
			Level:   Error,
		},
	}

	output := to.view(5)
	assert.NotEmpty(t, output)
}

func TestInlineViewRendering(t *testing.T) {
	inl := inlineNotification{
		id: "test",
		notification: Notification{
			Message: "Inline error",
			Level:   Error,
		},
	}

	output := inl.view(80)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Inline error")
}

func TestInlineViewZeroWidth(t *testing.T) {
	inl := inlineNotification{
		id: "test",
		notification: Notification{
			Message: "msg",
			Level:   Info,
		},
	}

	assert.Empty(t, inl.view(0))
}

// --- Modal tests ---

func TestShowConfirm(t *testing.T) {
	cmd := ShowConfirm("Delete?", "Are you sure?")
	require.NotNil(t, cmd)

	msg := cmd()
	smm, ok := msg.(ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete?", smm.Title)
	assert.Equal(t, "Are you sure?", smm.Message)
	assert.Equal(t, ModalConfirm, smm.Kind)
}

func TestShowInput(t *testing.T) {
	cmd := ShowInput("Branch name", "feature/...")
	require.NotNil(t, cmd)

	msg := cmd()
	smm, ok := msg.(ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Branch name", smm.Title)
	assert.Equal(t, "feature/...", smm.Placeholder)
	assert.Equal(t, ModalInput, smm.Kind)
}

func TestModalConfirmYes(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Really?",
		Kind:    ModalConfirm,
	})
	assert.True(t, m.HasModal())

	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.False(t, m.HasModal())
}

func TestModalConfirmNo(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Really?",
		Kind:    ModalConfirm,
	})

	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
	assert.False(t, m.HasModal())
}

func TestModalConfirmEscape(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
}

func TestModalConfirmEnterDefault(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Default selection is false (No), so enter should reject
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept, "default selection should be No")
}

func TestModalConfirmArrowKeys(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Select Yes with left arrow
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept, "should accept after selecting Yes")
}

func TestModalInputSubmit(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:       "Branch",
		Placeholder: "feature/...",
		Kind:        ModalInput,
	})
	assert.True(t, m.HasModal())

	// Type some text
	m.Update(tea.KeyPressMsg{Code: -1, Text: "m"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "i"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "n"})

	// Submit
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.Equal(t, "main", result.Value)
	assert.False(t, m.HasModal())
}

func TestModalInputEscape(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Input",
		Kind:  ModalInput,
	})

	m.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "b"})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
	assert.Empty(t, result.Value)
}

func TestModalInputBackspace(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Input",
		Kind:  ModalInput,
	})

	m.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "b"})
	m.Update(tea.KeyPressMsg{Code: -1, Text: "c"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result := msg.(ModalResultMsg)
	assert.Equal(t, "ab", result.Value)
}

func TestModalViewRendering(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Test Modal",
		Message: "Modal body",
		Kind:    ModalConfirm,
	})

	view := m.ModalView(80, 24)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Test Modal")
	assert.Contains(t, view, "Modal body")
}

func TestModalViewNoModal(t *testing.T) {
	m := NewManager()
	assert.Empty(t, m.ModalView(80, 24))
}

func TestModalViewZeroDimensions(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Test",
		Kind:  ModalConfirm,
	})

	assert.Empty(t, m.ModalView(0, 24))
	assert.Empty(t, m.ModalView(80, 0))
}

func TestNoModalKeyPassthrough(t *testing.T) {
	m := NewManager()

	// Keys without modal should produce nil cmd
	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	assert.Nil(t, cmd)
}

// --- Error boundary tests ---

// panicPanel is a test panel that panics on Update and/or View.
type panicPanel struct {
	panels.Placeholder
	panicOnUpdate bool
	panicOnView   bool
}

func (p *panicPanel) Update(_ tea.Msg) (panels.Panel, tea.Cmd) {
	if p.panicOnUpdate {
		panic("update exploded")
	}
	return p, nil
}

func (p *panicPanel) View(_ int, _ int) string {
	if p.panicOnView {
		panic("view exploded")
	}
	return "ok"
}

func (p *panicPanel) Title() string                  { return "panic-panel" }
func (p *panicPanel) Init(_ context.Context) tea.Cmd { return nil }

func TestSafeUpdateCatchesPanic(t *testing.T) {
	p := &panicPanel{panicOnUpdate: true}

	result, cmd := SafeUpdate(p, nil)

	assert.Equal(t, p, result, "should return original panel on panic")
	require.NotNil(t, cmd, "should return a command with error toast")

	msg := cmd()
	stm, ok := msg.(ShowToastMsg)
	require.True(t, ok, "should produce ShowToastMsg")
	assert.Equal(t, Error, stm.Level)
	assert.Contains(t, stm.Message, "Panel crashed")
}

func TestSafeUpdateNoPanic(t *testing.T) {
	p := &panicPanel{panicOnUpdate: false}

	result, cmd := SafeUpdate(p, nil)

	assert.Equal(t, p, result)
	assert.Nil(t, cmd)
}

func TestSafeViewCatchesPanic(t *testing.T) {
	p := &panicPanel{panicOnView: true}

	content := SafeView(p, 80, 24)

	assert.Contains(t, content, "Panel crashed")
	assert.Contains(t, content, "panic-panel")
}

func TestSafeViewNoPanic(t *testing.T) {
	p := &panicPanel{panicOnView: false}

	content := SafeView(p, 80, 24)
	assert.Equal(t, "ok", content)
}

func TestRenderErrorState(t *testing.T) {
	content := renderErrorState("filetree", "nil pointer", 80, 10)

	assert.Contains(t, content, "Panel crashed: filetree")
	assert.Contains(t, content, "Error: nil pointer")
	assert.Contains(t, content, "restart")
}

func TestRenderErrorStateZeroSize(t *testing.T) {
	assert.Empty(t, renderErrorState("p", "err", 0, 10))
	assert.Empty(t, renderErrorState("p", "err", 80, 0))
}

// --- levelColor / levelIcon tests ---

func TestLevelColor(t *testing.T) {
	// Just verify no panics and colors are non-nil
	for _, l := range []Level{Info, Warn, Error, Success, Level(99)} {
		c := levelColor(l)
		assert.NotNil(t, c, "levelColor should return a color for %v", l)
	}
}

func TestLevelIcon(t *testing.T) {
	for _, l := range []Level{Info, Warn, Error, Success, Level(99)} {
		icon := levelIcon(l)
		assert.NotEmpty(t, icon, "levelIcon should return an icon for %v", l)
	}
}

// --- Message type tests ---

func TestMessageTypes(t *testing.T) {
	// Verify all message types are distinct and constructible
	_ = ToastTickMsg{Time: time.Now()}
	_ = ToastExpiredMsg{ID: 42}
	_ = ModalResultMsg{Accept: true, Value: "val"}
	_ = ShowToastMsg{Message: "m", Level: Info}
	_ = ShowModalMsg{Title: "t", Kind: ModalConfirm}
}

func TestModalKindConstants(t *testing.T) {
	assert.NotEqual(t, ModalConfirm, ModalInput, "modal kinds must be distinct")
	assert.NotEqual(t, ModalConfirm, ModalConfirmWithCheckbox, "modal kinds must be distinct")
	assert.NotEqual(t, ModalInput, ModalConfirmWithCheckbox, "modal kinds must be distinct")
	assert.NotEqual(t, ModalConfirm, ModalActionPicker, "modal kinds must be distinct")
	assert.NotEqual(t, ModalInput, ModalActionPicker, "modal kinds must be distinct")
	assert.NotEqual(t, ModalConfirmWithCheckbox, ModalActionPicker, "modal kinds must be distinct")
	assert.NotEqual(t, ModalActionPicker, ModalActionPickerWithCheckbox, "modal kinds must be distinct")
}

// --- ConfirmWithCheckbox modal tests ---

func TestConfirmWithCheckbox_Accept(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Message:       "Are you sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})
	assert.True(t, m.HasModal())

	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.False(t, result.Remember, "checkbox was not toggled")
	assert.False(t, m.HasModal())
}

func TestConfirmWithCheckbox_AcceptWithRemember(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Message:       "Are you sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Toggle checkbox
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Nil(t, cmd, "space should toggle without producing a result")

	// Accept
	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "checkbox was toggled on")
	assert.False(t, m.HasModal())
}

func TestConfirmWithCheckbox_Reject(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Message:       "Are you sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
	assert.False(t, result.Remember, "Remember is always false when cancelled")
	assert.False(t, m.HasModal())
}

func TestConfirmWithCheckbox_EnterAccept(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Delete?",
		Message: "Are you sure?",
		Kind:    ModalConfirmWithCheckbox,
	})

	// Yes is selected by default for ConfirmWithCheckbox
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept, "Yes is selected by default")
	assert.False(t, result.Remember)
	assert.False(t, m.HasModal())
}

func TestConfirmWithCheckbox_EnterReject(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Delete?",
		Message: "Are you sure?",
		Kind:    ModalConfirmWithCheckbox,
	})

	// Move to No
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept, "No was selected")
	assert.False(t, result.Remember)
	assert.False(t, m.HasModal())
}

func TestConfirmWithCheckbox_SpaceToggle(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Toggle test",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Checkbox starts unchecked — toggle on
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Nil(t, cmd)

	// Verify checked: accept and check Remember
	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Remember, "should be checked after one toggle")

	// Re-open and toggle twice (on then off)
	m.Update(ShowModalMsg{
		Title:         "Toggle test 2",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // on
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // off

	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)
	result = cmd().(ModalResultMsg)
	assert.False(t, result.Remember, "should be unchecked after two toggles")
}

func TestConfirmWithCheckbox_CheckboxWithEnterAccept(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Confirm?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Don't ask again",
	})

	// Toggle checkbox on
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	// Press enter (Yes is default)
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "checkbox was checked before enter")
}

func TestConfirmWithCheckbox_EscCancels(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
	assert.False(t, result.Remember)
	assert.False(t, m.HasModal())
}

func TestShowConfirmWithCheckbox(t *testing.T) {
	cmd := ShowConfirmWithCheckbox("Delete?", "Are you sure?", "Don't ask again")
	require.NotNil(t, cmd)

	msg := cmd()
	smm, ok := msg.(ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete?", smm.Title)
	assert.Equal(t, "Are you sure?", smm.Message)
	assert.Equal(t, ModalConfirmWithCheckbox, smm.Kind)
	assert.Equal(t, "Don't ask again", smm.CheckboxLabel)
}

func TestConfirmWithCheckbox_View(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete file?",
		Message:       "This cannot be undone.",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Don't ask me again",
	})

	view := m.ModalView(80, 24)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Delete file?")
	assert.Contains(t, view, "This cannot be undone.")
	assert.Contains(t, view, "Don't ask me again")
	assert.Contains(t, view, "tab cycle")
	assert.Contains(t, view, "space toggle")
	assert.Contains(t, view, "settings")
	assert.Contains(t, view, "○") // unchecked checkbox icon
}

// --- Notification struct ---

func TestNotificationStruct(t *testing.T) {
	n := Notification{
		Message:   "test",
		Level:     Warn,
		CreatedAt: time.Now(),
		Duration:  5 * time.Second,
	}
	assert.Equal(t, "test", n.Message)
	assert.Equal(t, Warn, n.Level)
	assert.Equal(t, 5*time.Second, n.Duration)
}

// --- Integration: full toast lifecycle ---

func TestToastFullLifecycle(t *testing.T) {
	m := NewManager()

	// Add 3 toasts
	m.AddToast("first", Info)
	m.AddToast("second", Warn)
	m.AddToast("third", Error)
	assert.Equal(t, 3, m.ToastCount())

	// View should contain all
	v := m.View(80)
	assert.Contains(t, v, "first")
	assert.Contains(t, v, "second")
	assert.Contains(t, v, "third")

	// Expire them one by one
	ids := m.toastIDs()

	for i, id := range ids {
		m.Update(ToastExpiredMsg{ID: id})
		assert.Equal(t, 2-i, m.ToastCount(), fmt.Sprintf("after expiring toast %d", i))
	}

	assert.Empty(t, m.View(80))
}

// --- ActionPicker modal tests ---

func testActions() []ActionOption {
	return []ActionOption{
		{ID: "checkout", Label: "checkout"},
		{ID: "copy_name", Label: "copy name"},
		{ID: "delete", Label: "delete branch"},
	}
}

func TestActionPickerShowsActions(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Branch Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})
	assert.True(t, m.HasModal())

	view := m.ModalView(80, 24)
	assert.Contains(t, view, "Branch Actions")
	assert.Contains(t, view, "checkout")
	assert.Contains(t, view, "copy name")
	assert.Contains(t, view, "delete branch")
	assert.Contains(t, view, "navigate")
}

func TestActionPickerNavigateDown(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Move down
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Select — should return second action
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestActionPickerNavigateUp(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Move down twice, then up once → should be on second item (index 1)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestActionPickerSelectAction(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Default cursor is 0, press enter to select first action
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.True(t, result.Accept)
	assert.Equal(t, "checkout", result.Value)
	assert.False(t, m.HasModal())
}

func TestActionPickerCancel(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModalResultMsg)
	require.True(t, ok)
	assert.False(t, result.Accept)
	assert.Empty(t, result.Value)
	assert.False(t, m.HasModal())
}

func TestActionPickerCursorBounds(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Try moving up past the first item — should stay on first
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.Equal(t, "checkout", result.Value, "cursor should stay on first item")

	// Re-open and try moving past the last item
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // past last
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // still past last

	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result = cmd().(ModalResultMsg)
	assert.Equal(t, "delete", result.Value, "cursor should stay on last item")
}

func TestActionPickerCursorIndicator(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Default: cursor on first item
	view := m.ModalView(80, 24)
	assert.Contains(t, view, "▸")

	// Move down, cursor indicator should still appear
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	view = m.ModalView(80, 24)
	assert.Contains(t, view, "▸")
}

func TestActionPickerVimKeys(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Use j/k vim-style navigation
	m.Update(tea.KeyPressMsg{Code: -1, Text: "j"}) // down to index 1
	m.Update(tea.KeyPressMsg{Code: -1, Text: "j"}) // down to index 2
	m.Update(tea.KeyPressMsg{Code: -1, Text: "k"}) // up to index 1

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestShowActionPickerConstructor(t *testing.T) {
	actions := testActions()
	cmd := ShowActionPicker("Pick action", actions)
	require.NotNil(t, cmd)

	msg := cmd()
	smm, ok := msg.(ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Pick action", smm.Title)
	assert.Equal(t, ModalActionPicker, smm.Kind)
	assert.Len(t, smm.Actions, 3)
	assert.Equal(t, "checkout", smm.Actions[0].ID)
}

// --- Tab key tests ---

func TestModalConfirmTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Default selection is No (selected = false).
	// Tab should toggle to Yes.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "Tab should have toggled to Yes")
}

func TestModalConfirmTabToggle(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Tab twice should return to original (No).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept, "Two tabs should toggle back to No")
}

func TestModalConfirmWithCheckboxTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// ConfirmWithCheckbox starts with selected = true (Yes).
	// Tab should toggle to No.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept, "Tab should have toggled to No")
}

func TestModalActionPickerTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Tab should move cursor down (same as down key).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value, "Tab should move to second action")
}

func TestModalActionPickerShiftTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Move down twice, then shift+tab back up once.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value, "Shift+Tab should move cursor up")
}

// --- Mouse click tests ---

func TestModalConfirmMouseClickYes(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Are you sure?",
		Kind:    ModalConfirm,
	})

	// Click on the left half of the button row (Yes).
	// The modal is centred in 80x24. With boxWidth=50, the rendered box
	// is approximately 52 wide (50 + 2 border). padLeft = (80-52)/2 = 14.
	// Content starts at x = 14 + 3 = 17.
	// Left half of content = x < 17 + (50-4)/2 = 17 + 23 = 40.
	// Click at x=20 should hit Yes.
	//
	// For Y: we need the button row. Title(1) + blank(1) + message(1) +
	// blank(1) = 4 lines of header. Buttons at content line 4.
	// Content starts at y = padTop + 2.
	// Box height = 4(header) + 1(buttons) + 4(border+padding) = 9.
	// padTop = (24 - 9) / 2 = 7.
	// Button row y = 7 + 2 + 4 = 13.
	cmd := m.Update(tea.MouseClickMsg{X: 20, Y: 13, Button: tea.MouseLeft})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.False(t, m.HasModal())
}

func TestModalConfirmMouseClickNo(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Are you sure?",
		Kind:    ModalConfirm,
	})

	// Click on the right half of the button row (No).
	// Using same geometry: button row at y=13, right half starts at x=40.
	cmd := m.Update(tea.MouseClickMsg{X: 50, Y: 13, Button: tea.MouseLeft})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept)
	assert.False(t, m.HasModal())
}

func TestModalConfirmMouseClickOutside(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Are you sure?",
		Kind:    ModalConfirm,
	})

	// Click outside the modal (top-left corner).
	cmd := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "click outside modal should be a no-op")
	assert.True(t, m.HasModal(), "modal should remain open")
}

func TestModalConfirmMouseClickRightButton(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:   "Confirm?",
		Message: "Are you sure?",
		Kind:    ModalConfirm,
	})

	// Right-click should be ignored.
	cmd := m.Update(tea.MouseClickMsg{X: 20, Y: 13, Button: tea.MouseRight})
	assert.Nil(t, cmd, "right-click should be ignored")
	assert.True(t, m.HasModal())
}

func TestModalActionPickerMouseClick(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:   "Actions",
		Message: "Pick one",
		Kind:    ModalActionPicker,
		Actions: testActions(),
	})

	// Click on the second action item.
	// Header: title(1) + blank(1) + message(1) + blank(1) = 4 lines.
	// Actions start at content line 4. Second action at content line 5.
	// Box has 4(header) + 3(actions) + 2(blank+hint) + 4(border+padding) = 13 lines.
	// padTop = (24-13)/2 = 5.
	// Second action y = 5 + 2 + 5 = 12.
	cmd := m.Update(tea.MouseClickMsg{X: 30, Y: 12, Button: tea.MouseLeft})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
	assert.False(t, m.HasModal())
}

func TestModalCheckboxMouseClickToggle(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Confirm?",
		Message:       "Sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Click on the checkbox line.
	// Header: title(1) + blank(1) + message(1) + blank(1) = 4 lines.
	// Checkbox at content line 4.
	// Box: 4(header) + 1(checkbox) + 1(hint) + 1(blank) + 1(buttons) +
	//   4(border+padding) = 12.
	// padTop = (24-12)/2 = 6.
	// Checkbox y = 6 + 2 + 4 = 12.
	cmd := m.Update(tea.MouseClickMsg{X: 30, Y: 12, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "checkbox click should toggle without producing a result")

	// Now accept — should have Remember=true.
	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "checkbox should be checked after click")
}

func TestModalMouseClickNoModal(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)

	// Mouse click without a modal should be a no-op.
	cmd := m.Update(tea.MouseClickMsg{X: 40, Y: 12, Button: tea.MouseLeft})
	assert.Nil(t, cmd)
}

func TestModalMouseClickNoSize(t *testing.T) {
	m := NewManager()
	// Don't call SetSize — screen dimensions are zero.
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Should be a no-op because screen dimensions are not set.
	cmd := m.Update(tea.MouseClickMsg{X: 40, Y: 12, Button: tea.MouseLeft})
	assert.Nil(t, cmd)
	assert.True(t, m.HasModal(), "modal should remain open")
}

func TestSetSize(t *testing.T) {
	m := NewManager()
	m.SetSize(100, 50)

	// Verify dimensions are stored (indirectly via mouse click handling).
	m.Update(ShowModalMsg{
		Title:   "Test",
		Message: "msg",
		Kind:    ModalConfirm,
	})

	// A click within the larger viewport should work without panic.
	cmd := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "click outside modal content should be a no-op")
	assert.True(t, m.HasModal())
}

// --- Tab cycling tests for ModalConfirmWithCheckbox ---

func TestModalConfirmWithCheckboxTabCyclesToCheckbox(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Start: focusIdx=0 (Yes). Tab→No (1), Tab→Checkbox (2).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter on checkbox should toggle it (not dismiss modal).
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")
	assert.True(t, m.HasModal(), "modal should remain open")

	// Now accept — checkbox was toggled on.
	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "checkbox should be checked after Enter toggled it")
}

func TestModalConfirmWithCheckboxTabCyclesBack(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Tab 3 times: Yes(0)→No(1)→Checkbox(2)→Yes(0).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Back on Yes — Enter should accept.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "Tab should have cycled back to Yes")
}

func TestModalConfirmWithCheckboxShiftTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Delete?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Start: focusIdx=0 (Yes). Shift+Tab goes backwards: Yes(0)→Checkbox(2).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	// Enter on checkbox should toggle it.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")

	// Shift+Tab again: Checkbox(2)→No(1).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept, "Shift+Tab from checkbox should reach No")
	// Remember is always false when the user rejects (Accept=false).
	assert.False(t, result.Remember, "Remember is always false when cancelled")
}

func TestModalConfirmWithCheckboxEnterOnCheckbox(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Toggle test",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Move to checkbox (Tab twice: Yes→No→Checkbox).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter toggles checkbox on.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")
	assert.True(t, m.HasModal())

	// Enter again toggles checkbox off.
	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle again")

	// Accept: checkbox was toggled on then off, so Remember=false.
	cmd = m.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.False(t, result.Remember, "checkbox toggled on then off should be unchecked")
}

func TestModalConfirmWithCheckboxFocusIndicator(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Focus test",
		Message:       "Check focus indicator",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember this",
	})

	// Initially focusIdx=0 (Yes), no focus indicator on checkbox.
	view := m.ModalView(80, 24)
	assert.NotContains(t, view, "▸", "no focus indicator when checkbox is not focused")

	// Tab twice to reach checkbox.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	view = m.ModalView(80, 24)
	assert.Contains(t, view, "▸", "focus indicator should appear when checkbox is focused")
}

func TestModalConfirmWithCheckboxLeftRightUpdateFocusIdx(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Focus test",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Tab twice to checkbox (focusIdx=2).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Left arrow should go to Yes (focusIdx=0), and Enter should accept.
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "Enter after Left should dismiss (not toggle checkbox)")
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "Left should focus Yes")
}

func TestModalConfirmShiftTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title: "Confirm?",
		Kind:  ModalConfirm,
	})

	// Default is No. Shift+Tab should toggle to Yes (same as Tab).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "Shift+Tab should toggle to Yes")
}

func TestModalCheckboxMouseClickSetsFocusIdx(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Confirm?",
		Message:       "Sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Click on the checkbox line to set focusIdx to 2.
	// Same coordinates as TestModalCheckboxMouseClickToggle.
	cmd := m.Update(tea.MouseClickMsg{X: 30, Y: 12, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "checkbox click should toggle without producing a result")

	// The focus indicator should now be visible on the checkbox.
	view := m.ModalView(80, 24)
	assert.Contains(t, view, "▸", "clicking checkbox should set focus to checkbox")
}

// --- ActionPickerWithCheckbox modal tests ---

func TestShowActionPickerWithCheckbox(t *testing.T) {
	actions := testActions()
	cmd := ShowActionPickerWithCheckbox("Pick action", "Choose wisely", "Remember choice", actions)
	require.NotNil(t, cmd)

	msg := cmd()
	smm, ok := msg.(ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Pick action", smm.Title)
	assert.Equal(t, "Choose wisely", smm.Message)
	assert.Equal(t, ModalActionPickerWithCheckbox, smm.Kind)
	assert.Equal(t, "Remember choice", smm.CheckboxLabel)
	assert.Len(t, smm.Actions, 3)
	assert.Equal(t, "checkout", smm.Actions[0].ID)
	assert.Equal(t, "checkout", smm.Actions[0].Label)
}

func TestActionPickerWithCheckbox_SelectFirstAction(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Message:       "Pick one",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})
	assert.True(t, m.HasModal())

	// Default cursor is 0; enter selects first action.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "checkout", result.Value)
	assert.False(t, result.Remember)
	assert.False(t, m.HasModal())
}

func TestActionPickerWithCheckbox_NavigateDown(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Move down to second action.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestActionPickerWithCheckbox_NavigateUp(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Move down twice, then up once → should be on second item (index 1).
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestActionPickerWithCheckbox_CursorBounds(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Try moving up past the first item — should stay on first.
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.Equal(t, "checkout", result.Value, "cursor should stay on first item")

	// Re-open and try moving past the last item.
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // past last
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // still past last

	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result = cmd().(ModalResultMsg)
	assert.Equal(t, "delete", result.Value, "cursor should stay on last item")
}

func TestActionPickerWithCheckbox_VimKeys(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// j/k vim-style navigation.
	m.Update(tea.KeyPressMsg{Code: -1, Text: "j"}) // down to index 1
	m.Update(tea.KeyPressMsg{Code: -1, Text: "j"}) // down to index 2
	m.Update(tea.KeyPressMsg{Code: -1, Text: "k"}) // up to index 1

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
}

func TestActionPickerWithCheckbox_TabTogglesFocus(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Start: focusIdx=0 (action list). Tab → focusIdx=1 (checkbox).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter on checkbox should toggle it, not dismiss.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")
	assert.True(t, m.HasModal())

	// Tab back to action list (focusIdx=0).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter should now select action; checkbox was toggled on by Enter.
	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "checkout", result.Value)
	assert.True(t, result.Remember, "checkbox was toggled on by Enter on checkbox")
}

func TestActionPickerWithCheckbox_TabCyclesBack(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Tab twice: actions(0) → checkbox(1) → actions(0).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Back on action list — Enter should select action.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "Tab should have cycled back to actions")
	assert.Equal(t, "checkout", result.Value)
}

func TestActionPickerWithCheckbox_ShiftTab(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Shift+Tab cycles same direction as Tab (both use (focusIdx+1)%2).
	// Start: focusIdx=0. Shift+Tab → focusIdx=1 (checkbox).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	// Enter on checkbox should toggle it, not dismiss.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")
	assert.True(t, m.HasModal())
}

func TestActionPickerWithCheckbox_SpaceToggle(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Space toggles checkbox.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Nil(t, cmd)

	// Accept — should have Remember=true.
	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "space should have toggled checkbox on")

	// Re-open and toggle twice (on then off).
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // on
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // off

	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result = cmd().(ModalResultMsg)
	assert.False(t, result.Remember, "two toggles should leave checkbox off")
}

func TestActionPickerWithCheckbox_SpaceTogglesRegardlessOfFocus(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// focusIdx=0 (action list) — space still toggles checkbox.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Nil(t, cmd)

	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Remember, "space toggles checkbox even when action list is focused")
}

func TestActionPickerWithCheckbox_EnterWithCheckboxChecked(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Toggle checkbox on.
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	// Navigate to second action.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Enter confirms with checkbox state.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "copy_name", result.Value)
	assert.True(t, result.Remember, "checkbox was checked before enter")
	assert.False(t, m.HasModal())
}

func TestActionPickerWithCheckbox_EnterWithCheckboxUnchecked(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Checkbox starts unchecked — enter without toggling.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.Equal(t, "checkout", result.Value)
	assert.False(t, result.Remember, "checkbox was not toggled")
}

func TestActionPickerWithCheckbox_EscCancels(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept)
	assert.Empty(t, result.Value)
	assert.False(t, result.Remember)
	assert.False(t, m.HasModal())
}

func TestActionPickerWithCheckbox_EnterOnCheckboxToggles(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Tab to checkbox.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter on checkbox toggles it on.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle, not dismiss")
	assert.True(t, m.HasModal())

	// Enter again toggles checkbox off.
	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on checkbox should toggle again")

	// Tab back to actions, then accept.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.False(t, result.Remember, "checkbox toggled on then off should be unchecked")
}

func TestActionPickerWithCheckbox_UpDownIgnoredWhenCheckboxFocused(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Tab to checkbox (focusIdx=1).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Up/down should not change actionCursor when checkbox is focused.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Tab back to action list and select.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.Equal(t, "checkout", result.Value, "cursor should not have moved while checkbox was focused")
}

func TestActionPickerWithCheckbox_EmptyActions(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       []ActionOption{},
	})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept, "empty actions should reject on enter")
	assert.False(t, m.HasModal())
}

func TestActionPickerWithCheckbox_UnrecognizedKey(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// An unrecognized key should be a no-op.
	cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "z"})
	assert.Nil(t, cmd, "unrecognized key should be a no-op")
	assert.True(t, m.HasModal())
}

func TestActionPickerWithCheckbox_View(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Pick Action",
		Message:       "Choose an action",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Always do this",
		Actions:       testActions(),
	})

	view := m.ModalView(80, 24)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Pick Action")
	assert.Contains(t, view, "Choose an action")
	assert.Contains(t, view, "checkout")
	assert.Contains(t, view, "copy name")
	assert.Contains(t, view, "delete branch")
	assert.Contains(t, view, "Always do this")
	assert.Contains(t, view, "navigate")
	assert.Contains(t, view, "tab checkbox")
	assert.Contains(t, view, "space toggle")
	assert.Contains(t, view, "settings")
	assert.Contains(t, view, "○") // unchecked checkbox
}

func TestActionPickerWithCheckbox_ViewCheckedCheckbox(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Pick Action",
		Message:       "Choose an action",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Always do this",
		Actions:       testActions(),
	})

	// Toggle checkbox on.
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	view := m.ModalView(80, 24)
	assert.Contains(t, view, "●") // checked checkbox
}

func TestActionPickerWithCheckbox_ViewFocusIndicator(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:         "Focus test",
		Message:       "Check focus",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Initially focusIdx=0 (action list), cursor on first action.
	view := m.ModalView(80, 24)
	assert.Contains(t, view, "▸", "cursor should be on first action")

	// Tab to checkbox (focusIdx=1).
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	view = m.ModalView(80, 24)
	assert.Contains(t, view, "▸", "focus indicator should be on checkbox")
}

func TestActionPickerWithCheckbox_ViewDefaultCheckboxLabel(t *testing.T) {
	m := NewManager()
	m.Update(ShowModalMsg{
		Title:   "Pick Action",
		Kind:    ModalActionPickerWithCheckbox,
		Actions: testActions(),
		// No CheckboxLabel — uses default.
	})

	view := m.ModalView(80, 24)
	assert.Contains(t, view, "Always do this action on double click")
}

func TestActionPickerWithCheckbox_MouseClickAction(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Message:       "Pick one",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Find the Y coordinate of the second action by measuring rendered
	// dimensions the same way handleMouseClick does internally.
	//
	// Header: title(1) + blank(1) + message(1) + blank(1) = 4 lines.
	// Actions start at header line 4. Second action at relY = 5.
	//
	// Measure box height to compute padTop (how the box is centred):
	// content = 4(hdr) + 3(actions) + 1(\n) + 1(chk) + 1(\n) +
	//           hint1(may wrap) + 1(\n) + hint2(1). The rendered box
	//           height depends on lipgloss but we can get close.
	//
	// Use the rendered view to compute: render → measure → derive padTop.
	view := m.ModalView(80, 24)
	_ = view // view is rendered; we need the box height.

	// Instead of fragile coordinate math, brute-force scan Y values for
	// the second action click. This is the technique used by integration
	// tests that must be geometry-resilient.
	var cmd tea.Cmd
	for y := 0; y < 24; y++ {
		// Re-open modal each attempt (click may dismiss it).
		if !m.HasModal() {
			m.Update(ShowModalMsg{
				Title:         "Actions",
				Message:       "Pick one",
				Kind:          ModalActionPickerWithCheckbox,
				CheckboxLabel: "Remember",
				Actions:       testActions(),
			})
		}
		cmd = m.Update(tea.MouseClickMsg{X: 30, Y: y, Button: tea.MouseLeft})
		if cmd != nil {
			result := cmd().(ModalResultMsg)
			if result.Accept && result.Value == "copy_name" {
				// Found the correct Y for the second action.
				assert.False(t, m.HasModal())
				return
			}
		}
	}
	t.Fatal("could not find a Y coordinate that clicks the second action")
}

func TestActionPickerWithCheckbox_MouseClickCheckbox(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Message:       "Pick one",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Scan Y values to find the checkbox row. A checkbox click toggles
	// checked, sets focusIdx=1, and returns nil (modal stays open).
	found := false
	for y := 0; y < 24; y++ {
		cmd := m.Update(tea.MouseClickMsg{X: 30, Y: y, Button: tea.MouseLeft})
		if cmd != nil {
			// Action click dismissed the modal. Re-open.
			m.Update(ShowModalMsg{
				Title:         "Actions",
				Message:       "Pick one",
				Kind:          ModalActionPickerWithCheckbox,
				CheckboxLabel: "Remember",
				Actions:       testActions(),
			})
			continue
		}
		// Check if the checkbox was toggled (view now shows ●).
		view := m.ModalView(80, 24)
		if strings.Contains(view, "●") {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("could not find a Y coordinate that clicks the checkbox")
	}

	// The click set focusIdx=1 (checkbox). Tab back to action list so
	// Enter will confirm the selection rather than toggle the checkbox.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept)
	assert.True(t, result.Remember, "checkbox should be checked after mouse click")
}

func TestActionPickerWithCheckbox_MouseClickOutside(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Message:       "Pick one",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Click outside the modal (top-left corner).
	cmd := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "click outside modal should be a no-op")
	assert.True(t, m.HasModal(), "modal should remain open")
}

func TestActionPickerWithCheckbox_MouseClickRightButton(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Actions",
		Message:       "Pick one",
		Kind:          ModalActionPickerWithCheckbox,
		CheckboxLabel: "Remember",
		Actions:       testActions(),
	})

	// Right-click should be ignored.
	cmd := m.Update(tea.MouseClickMsg{X: 30, Y: 12, Button: tea.MouseRight})
	assert.Nil(t, cmd, "right-click should be ignored")
	assert.True(t, m.HasModal())
}

// --- clickConfirmWithCheckboxButton tests ---

func TestClickConfirmWithCheckboxButton_LeftHalf(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Confirm?",
		Message:       "Sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Toggle checkbox on first.
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	// Click on the Yes/No button row. For ConfirmWithCheckbox the button
	// row is at headerLines + 3 (checkbox + hint + blank + buttons).
	// Header: title(1) + blank(1) + message(1) + blank(1) = 4.
	// Button row relY = 4 + 3 = 7.
	// Box: 4(hdr) + 1(chk) + 1(hint) + 1(blank) + 1(btns) + 4(border+pad) = 12.
	// padTop = (24-12)/2 = 6.
	// Absolute button Y = 6 + 2 + 7 = 15.
	// Left half: x < padLeft + 3 + cw/2. padLeft = (80-52)/2 = 14.
	// Left half: x < 14 + 3 + 23 = 40. Click at x=20 → Yes.
	cmd := m.Update(tea.MouseClickMsg{X: 20, Y: 15, Button: tea.MouseLeft})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.True(t, result.Accept, "left half should be Yes")
	assert.True(t, result.Remember, "checkbox was checked")
	assert.False(t, m.HasModal())
}

func TestClickConfirmWithCheckboxButton_RightHalf(t *testing.T) {
	m := NewManager()
	m.SetSize(80, 24)
	m.Update(ShowModalMsg{
		Title:         "Confirm?",
		Message:       "Sure?",
		Kind:          ModalConfirmWithCheckbox,
		CheckboxLabel: "Remember",
	})

	// Click on the right half of the button row (No).
	// Same geometry as above: button row at Y=15, right half at x>=40.
	cmd := m.Update(tea.MouseClickMsg{X: 50, Y: 15, Button: tea.MouseLeft})
	require.NotNil(t, cmd)

	result := cmd().(ModalResultMsg)
	assert.False(t, result.Accept, "right half should be No")
	assert.False(t, m.HasModal())
}
