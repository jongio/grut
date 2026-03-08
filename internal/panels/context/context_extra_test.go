package context

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ────────────── SetActionsCfg ──────────────

func TestPanel_SetActionsCfg(t *testing.T) {
	p, _ := newTestPanel(t)
	cfg := config.ActionsConfig{}
	p.SetActionsCfg(cfg)
	assert.Equal(t, cfg, p.actionsCfg)
}

// ────────────── ensureCursorVisible ──────────────

func TestPanel_EnsureCursorVisible_ZeroHeight(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "a")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	// Height=1 means viewHeight=0 (subtract status bar), so early return.
	p.SetSize(80, 1)
	p.cursor = 5
	p.offset = 0
	p.ensureCursorVisible()
	// offset should not change since viewHeight <= 0
	assert.Equal(t, 0, p.offset)
}

func TestPanel_EnsureCursorVisible_CursorAboveOffset(t *testing.T) {
	p, root := newTestPanel(t)
	for i := 0; i < 10; i++ {
		writeFile(t, root, filepath.Join("sub", string(rune('a'+i))+".go"), "x")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "sub", string(rune('a'+i))+".go")})
	}

	p.SetSize(80, 6) // viewHeight = 5
	p.offset = 5
	p.cursor = 2 // cursor above offset
	p.ensureCursorVisible()
	assert.Equal(t, 2, p.offset, "offset should snap to cursor when cursor < offset")
}

func TestPanel_EnsureCursorVisible_CursorBelowViewport(t *testing.T) {
	p, root := newTestPanel(t)
	for i := 0; i < 10; i++ {
		writeFile(t, root, filepath.Join("sub", string(rune('a'+i))+".go"), "x")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "sub", string(rune('a'+i))+".go")})
	}

	p.SetSize(80, 4) // viewHeight = 3
	p.offset = 0
	p.cursor = 8 // cursor below viewport
	p.ensureCursorVisible()
	// offset should be cursor - viewHeight + 1 = 8 - 3 + 1 = 6
	assert.Equal(t, 6, p.offset)
}

// ────────────── handleMouseWheel ──────────────

func TestPanel_MouseWheelDown(t *testing.T) {
	p, root := newTestPanel(t)
	// Add enough files for scrolling.
	for i := 0; i < 20; i++ {
		name := string(rune('a'+i%26)) + ".go"
		writeFile(t, root, filepath.Join("dir", name), "x")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "dir", name)})
	}

	p.SetSize(80, 6) // viewHeight = 5
	p.offset = 0

	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)

	assert.Nil(t, cmd)
	assert.Greater(t, p.offset, 0, "offset should increase after scroll down")
}

func TestPanel_MouseWheelUp(t *testing.T) {
	p, root := newTestPanel(t)
	for i := 0; i < 20; i++ {
		name := string(rune('a'+i%26)) + ".go"
		writeFile(t, root, filepath.Join("dir", name), "x")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "dir", name)})
	}

	p.SetSize(80, 6)
	p.offset = 10

	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp})
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)

	assert.Nil(t, cmd)
	assert.Less(t, p.offset, 10, "offset should decrease after scroll up")
}

func TestPanel_MouseWheelUp_ClampAtZero(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.SetSize(80, 6)
	p.offset = 0

	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp})
	updated, _ := p.Update(msg)
	p = updated.(*Panel)
	assert.Equal(t, 0, p.offset, "offset should not go below 0")
}

func TestPanel_MouseWheelDown_ClampAtMax(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.SetSize(80, 6) // viewHeight=5, 1 file → maxOffset=0
	p.offset = 0

	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	updated, _ := p.Update(msg)
	p = updated.(*Panel)
	assert.Equal(t, 0, p.offset, "offset should clamp at maxOffset=0 for few files")
}

func TestPanel_MouseWheelZeroHeight(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.SetSize(80, 0) // viewHeight would be -1 → clamped to 1
	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	updated, _ := p.Update(msg)
	p = updated.(*Panel)
	// Should not panic and offset should be valid.
	assert.GreaterOrEqual(t, p.offset, 0)
}

// ────────────── handleMouseRightClick ──────────────

func TestPanel_MouseRightClick_OutOfBounds(t *testing.T) {
	p, _ := newTestPanel(t)
	// No files → any content row is out of bounds.
	msg := panels.PanelMouseRightClickMsg{ContentRow: 0, ContentCol: 0}
	updated, cmd := p.Update(msg)
	_ = updated.(*Panel) // verify type assertion succeeds
	assert.Nil(t, cmd)
}

func TestPanel_MouseRightClick_NegativeIndex(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	// offset=5 + row=-10 = idx=-5 → out of bounds
	p.offset = 5
	msg := panels.PanelMouseRightClickMsg{ContentRow: -10, ContentCol: 0}
	updated, cmd := p.Update(msg)
	_ = updated.(*Panel) // verify type assertion succeeds
	assert.Nil(t, cmd)
}

func TestPanel_MouseRightClick_InBounds(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	// File at index 0, offset=0, row=0.
	msg := panels.PanelMouseRightClickMsg{ContentRow: 0, ContentCol: 0}
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)
	// With no actionsCfg configured, rightclick.Cmd returns (nil, "")
	// so both cmd and directAction are nil/empty → no-op
	assert.Equal(t, 0, p.cursor)
	_ = cmd // may or may not be nil depending on rightclick config
}

// ────────────── handleModalResult ──────────────

func TestPanel_ModalResult_Reject(t *testing.T) {
	p, _ := newTestPanel(t)
	p.pendingOp = opRightClickPick
	msg := notify.ModalResultMsg{Accept: false}
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)
	assert.Nil(t, cmd)
	assert.Empty(t, p.pendingOp, "pendingOp should be cleared after modal result")
}

func TestPanel_ModalResult_RightClickPick(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.pendingOp = opRightClickPick
	msg := notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionRemove),
	}
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)
	// ActionRemove triggers removeCurrent → should produce a ContextUpdatedMsg.
	assert.Empty(t, p.pendingOp)
	require.NotNil(t, cmd)
	result := cmd()
	_, ok := result.(panels.ContextUpdatedMsg)
	assert.True(t, ok, "expected ContextUpdatedMsg after remove action")
}

func TestPanel_ModalResult_FirstUseConfirm(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.pendingOp = opFirstUseConfirm
	p.pendingName = "test_action"
	msg := notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionPreview),
		Remember: false,
	}
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)
	assert.Empty(t, p.pendingOp)
	assert.Empty(t, p.pendingName)
	// ActionPreview triggers previewCurrent → should produce FileSelectedMsg.
	require.NotNil(t, cmd)
}

func TestPanel_ModalResult_UnknownOp(t *testing.T) {
	p, _ := newTestPanel(t)
	p.pendingOp = "unknown_operation"
	msg := notify.ModalResultMsg{Accept: true, Value: "something"}
	updated, cmd := p.Update(msg)
	p = updated.(*Panel)
	assert.Nil(t, cmd, "unknown op should be a no-op")
	assert.Empty(t, p.pendingOp)
}

// ────────────── executeRightClickAction ──────────────

func TestPanel_ExecuteRightClickAction_Preview(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	result, cmd := p.executeRightClickAction(actions.ActionPreview)
	assert.NotNil(t, result)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(panels.FileSelectedMsg)
	assert.True(t, ok, "preview should emit FileSelectedMsg")
}

func TestPanel_ExecuteRightClickAction_Remove(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	assert.Equal(t, 1, p.fileCount())
	_, cmd := p.executeRightClickAction(actions.ActionRemove)
	require.NotNil(t, cmd)
	assert.Equal(t, 0, p.fileCount(), "remove action should remove the file")
}

func TestPanel_ExecuteRightClickAction_Unknown(t *testing.T) {
	p, _ := newTestPanel(t)
	result, cmd := p.executeRightClickAction(actions.ActionID("nonexistent_action"))
	assert.NotNil(t, result)
	assert.Nil(t, cmd, "unknown action should be no-op")
}

// ────────────── copyPath ──────────────

func TestPanel_CopyPath_EmptyList(t *testing.T) {
	p, _ := newTestPanel(t)
	// cursor=0 but no files → out-of-bounds guard
	result, cmd := p.copyPath()
	assert.NotNil(t, result)
	assert.Nil(t, cmd, "copyPath on empty list should be no-op")
}

func TestPanel_CopyPath_HasFile(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	_, cmd := p.copyPath()
	// On CI/Windows, clipboard may or may not work, but we can verify
	// it produces a toast message (success or error).
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	assert.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.NotEmpty(t, toast.Message)
}

func TestPanel_ExecuteRightClickAction_CopyPath(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "x")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	_, cmd := p.executeRightClickAction(actions.ActionCopyPath)
	// Exercises the ActionCopyPath branch of executeRightClickAction.
	require.NotNil(t, cmd)
}
