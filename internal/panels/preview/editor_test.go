package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPreview returns a Preview with edit-related fields ready for testing.
func testPreview() *Preview {
	edCfg := config.EditorConfig{
		TabSize:    4,
		AutoIndent: true,
	}
	p := New(defaultCfg(), edCfg)
	p.width = 80
	p.height = 24
	p.focused = true
	return p
}

// testPreviewWithFile writes content to a temp file, sets it as the preview's
// current file, and returns the preview and file path.
func testPreviewWithFile(t *testing.T, content string) (*Preview, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	p := testPreview()
	p.filePath = path
	return p, path
}

// ---------------------------------------------------------------------------
// enterEditMode
// ---------------------------------------------------------------------------

func TestEnterEditMode_Success(t *testing.T) {
	content := "line one\nline two\nline three"
	p, path := testPreviewWithFile(t, content)
	p.scrollY = 1

	cmd := enterEditMode(p)

	assert.True(t, p.editMode, "editMode should be true")
	assert.NotNil(t, p.editBuf, "editBuf should be created")
	assert.Equal(t, 3, p.editBuf.LineCount(), "buffer should have 3 lines")
	assert.Equal(t, "line one", p.editBuf.Line(0))
	assert.Equal(t, "line two", p.editBuf.Line(1))
	assert.Equal(t, "line three", p.editBuf.Line(2))
	assert.Equal(t, 1, p.cursorLine, "cursor should be at scrollY")
	assert.Equal(t, 0, p.cursorCol, "cursor col should be 0")

	// The cmd should produce an EditModeEnteredMsg.
	require.NotNil(t, cmd, "cmd should not be nil")
	msg := cmd()
	entered, ok := msg.(panels.EditModeEnteredMsg)
	require.True(t, ok, "should produce EditModeEnteredMsg")
	assert.Equal(t, path, entered.Path)
}

func TestEnterEditMode_BlockedBinary(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	p.isBinary = true

	cmd := enterEditMode(p)
	assert.Nil(t, cmd)
	assert.False(t, p.editMode)
}

func TestEnterEditMode_BlockedLarge(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	p.isLarge = true

	cmd := enterEditMode(p)
	assert.Nil(t, cmd)
	assert.False(t, p.editMode)
}

func TestEnterEditMode_BlockedGHMode(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	p.ghMode = true

	cmd := enterEditMode(p)
	assert.Nil(t, cmd)
	assert.False(t, p.editMode)
}

func TestEnterEditMode_BlockedEmptyPath(t *testing.T) {
	p := testPreview()
	p.filePath = ""

	cmd := enterEditMode(p)
	assert.Nil(t, cmd)
	assert.False(t, p.editMode)
}

func TestEnterEditMode_FileReadError(t *testing.T) {
	p := testPreview()
	p.filePath = filepath.Join(t.TempDir(), "nonexistent.txt")

	cmd := enterEditMode(p)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "should produce a toast for read error")
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Cannot edit")
}

// ---------------------------------------------------------------------------
// exitEditMode
// ---------------------------------------------------------------------------

func TestExitEditMode_Clean(t *testing.T) {
	content := "hello\nworld"
	p, path := testPreviewWithFile(t, content)
	enterEditMode(p)
	require.True(t, p.editMode)

	p.cursorLine = 1
	cmd := exitEditMode(p, true)

	assert.False(t, p.editMode, "editMode should be false")
	assert.Nil(t, p.editBuf, "editBuf should be nil")
	assert.Equal(t, 1, p.scrollY, "scrollY should preserve cursor position")
	require.NotNil(t, cmd)

	// Run the batch — it should include EditModeExitedMsg and loadFileCmd.
	msg := cmd()
	// The batch produces a tea.BatchMsg with sub-commands.
	batchMsg, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "should produce tea.BatchMsg, got %T", msg)
	// Find the EditModeExitedMsg in the batch results.
	foundExited := false
	for _, subCmd := range batchMsg {
		if subCmd == nil {
			continue
		}
		subMsg := subCmd()
		if exited, ok := subMsg.(panels.EditModeExitedMsg); ok {
			assert.Equal(t, path, exited.Path)
			foundExited = true
		}
	}
	assert.True(t, foundExited, "batch should contain EditModeExitedMsg")
}

func TestExitEditMode_NotInEditMode(t *testing.T) {
	p := testPreview()
	p.editMode = false

	cmd := exitEditMode(p, false)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleEditKeyPress — character insert
// ---------------------------------------------------------------------------

func TestHandleEditKeyPress_CharInsert(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorLine = 0
	p.cursorCol = 0

	msg := tea.KeyPressMsg{Text: "X", Code: 'X'}
	handleEditKeyPress(p, msg)

	assert.Equal(t, "Xabc", p.editBuf.Line(0))
	assert.Equal(t, 1, p.cursorCol)
}

func TestHandleEditKeyPress_MultiRuneInsert(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorCol = 3

	msg := tea.KeyPressMsg{Text: "XY"}
	handleEditKeyPress(p, msg)

	assert.Equal(t, "abcXY", p.editBuf.Line(0))
	assert.Equal(t, 5, p.cursorCol)
}

// ---------------------------------------------------------------------------
// handleEditKeyPress — backspace
// ---------------------------------------------------------------------------

func TestHandleEditKeyPress_Backspace(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorCol = 2

	msg := tea.KeyPressMsg{Code: tea.KeyBackspace}
	handleEditKeyPress(p, msg)

	assert.Equal(t, "ac", p.editBuf.Line(0))
	assert.Equal(t, 1, p.cursorCol)
}

// ---------------------------------------------------------------------------
// handleEditKeyPress — enter
// ---------------------------------------------------------------------------

func TestHandleEditKeyPress_Enter(t *testing.T) {
	p, _ := testPreviewWithFile(t, "hello world")
	enterEditMode(p)
	p.editCfg.AutoIndent = false
	p.cursorCol = 5

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	handleEditKeyPress(p, msg)

	assert.Equal(t, 2, p.editBuf.LineCount())
	assert.Equal(t, "hello", p.editBuf.Line(0))
	assert.Equal(t, " world", p.editBuf.Line(1))
	assert.Equal(t, 1, p.cursorLine)
	assert.Equal(t, 0, p.cursorCol)
}

func TestHandleEditKeyPress_EnterAutoIndent(t *testing.T) {
	p, _ := testPreviewWithFile(t, "    indented line")
	enterEditMode(p)
	p.editCfg.AutoIndent = true
	p.cursorCol = 17 // end of line

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	handleEditKeyPress(p, msg)

	assert.Equal(t, 2, p.editBuf.LineCount())
	assert.Equal(t, 1, p.cursorLine)
	// The new line should have auto-indentation; cursor at indent position.
	assert.Equal(t, countLeadingSpaces(p.editBuf.Line(1)), p.cursorCol)
}

// ---------------------------------------------------------------------------
// handleEditKeyPress — arrows / movement
// ---------------------------------------------------------------------------

func TestHandleEditKeyPress_ArrowKeys(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc\ndef\nghi")
	enterEditMode(p)
	p.cursorLine = 1
	p.cursorCol = 1

	// down
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, p.cursorLine)

	// up twice (should clamp at 0)
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyUp})
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, p.cursorLine)

	// right to end of line, then one more wraps
	p.cursorCol = 3
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyRight})
	assert.Equal(t, 1, p.cursorLine)
	assert.Equal(t, 0, p.cursorCol)

	// left wraps to end of previous line
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 3, p.cursorCol)
}

func TestHandleEditKeyPress_HomeEnd(t *testing.T) {
	p, _ := testPreviewWithFile(t, "hello world")
	enterEditMode(p)
	p.cursorCol = 5

	// home
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyHome})
	assert.Equal(t, 0, p.cursorCol)

	// end
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyEnd})
	assert.Equal(t, 11, p.cursorCol) // len("hello world")
}

func TestHandleEditKeyPress_CtrlHomeEnd(t *testing.T) {
	p, _ := testPreviewWithFile(t, "line1\nline2\nline3")
	enterEditMode(p)
	p.cursorLine = 1

	// ctrl+end goes to last line, end of line
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	assert.Equal(t, 2, p.cursorLine)
	assert.Equal(t, 5, p.cursorCol) // len("line3")

	// ctrl+home goes to 0,0
	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 0, p.cursorCol)
}

func TestHandleEditKeyPress_Escape_Clean(t *testing.T) {
	p, _ := testPreviewWithFile(t, "clean buffer")
	enterEditMode(p)
	require.True(t, p.editMode)

	_, cmd := handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyEscape})
	// Since buffer is clean, should produce an exit command.
	assert.NotNil(t, cmd)
}

func TestHandleEditKeyPress_CtrlS(t *testing.T) {
	p, _ := testPreviewWithFile(t, "save me")
	enterEditMode(p)

	_, cmd := handleEditKeyPress(p, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd, "ctrl+s should produce a save command")
}

// ---------------------------------------------------------------------------
// moveCursor boundary tests
// ---------------------------------------------------------------------------

func TestMoveCursorLeft_AtOrigin(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorLine = 0
	p.cursorCol = 0

	moveCursorLeft(p)
	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 0, p.cursorCol, "should not go negative")
}

func TestMoveCursorLeft_WrapToPreviousLine(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc\ndef")
	enterEditMode(p)
	p.cursorLine = 1
	p.cursorCol = 0

	moveCursorLeft(p)
	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 3, p.cursorCol, "should wrap to end of previous line")
}

func TestMoveCursorRight_AtEnd(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorLine = 0
	p.cursorCol = 3

	moveCursorRight(p)
	// Single line, no next line to wrap to.
	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 3, p.cursorCol)
}

func TestMoveCursorRight_WrapToNextLine(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc\ndef")
	enterEditMode(p)
	p.cursorLine = 0
	p.cursorCol = 3

	moveCursorRight(p)
	assert.Equal(t, 1, p.cursorLine)
	assert.Equal(t, 0, p.cursorCol)
}

func TestMoveCursorUp_AtTop(t *testing.T) {
	p, _ := testPreviewWithFile(t, "only line")
	enterEditMode(p)
	p.cursorLine = 0

	moveCursorUp(p)
	assert.Equal(t, 0, p.cursorLine, "should stay at line 0")
}

func TestMoveCursorDown_AtBottom(t *testing.T) {
	p, _ := testPreviewWithFile(t, "only line")
	enterEditMode(p)
	p.cursorLine = 0

	moveCursorDown(p)
	assert.Equal(t, 0, p.cursorLine, "should stay at last line")
}

func TestMoveCursorDown_ClampCol(t *testing.T) {
	p, _ := testPreviewWithFile(t, "long line here\nhi")
	enterEditMode(p)
	p.cursorLine = 0
	p.cursorCol = 14 // end of "long line here"

	moveCursorDown(p)
	assert.Equal(t, 1, p.cursorLine)
	assert.Equal(t, 2, p.cursorCol, "should clamp to length of shorter line")
}

// ---------------------------------------------------------------------------
// ensureCursorVisible
// ---------------------------------------------------------------------------

func TestEnsureCursorVisible_ScrollsDown(t *testing.T) {
	p := testPreview()
	p.height = 10 // viewportHeight = 9
	p.scrollY = 0
	p.cursorLine = 15

	ensureCursorVisible(p)
	// cursorLine should be within [scrollY, scrollY+vh)
	assert.LessOrEqual(t, p.scrollY, p.cursorLine)
	assert.Greater(t, p.scrollY+p.viewportHeight(), p.cursorLine)
}

func TestEnsureCursorVisible_ScrollsUp(t *testing.T) {
	p := testPreview()
	p.height = 10
	p.scrollY = 20
	p.cursorLine = 5

	ensureCursorVisible(p)
	assert.Equal(t, 5, p.scrollY, "scrollY should match cursorLine when scrolling up")
}

func TestEnsureCursorVisible_NoChangeWhenVisible(t *testing.T) {
	p := testPreview()
	p.height = 10
	p.scrollY = 5
	p.cursorLine = 8

	ensureCursorVisible(p)
	assert.Equal(t, 5, p.scrollY, "scrollY should not change when cursor is visible")
}

func TestEnsureCursorVisible_NegativeScrollY(t *testing.T) {
	p := testPreview()
	p.height = 10
	p.scrollY = -5
	p.cursorLine = 0

	ensureCursorVisible(p)
	assert.Equal(t, 0, p.scrollY, "scrollY should be clamped to 0")
}

// ---------------------------------------------------------------------------
// saveFile
// ---------------------------------------------------------------------------

func TestSaveFile_WritesToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saveable.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	p := testPreview()
	p.filePath = path
	p.editBuf = NewTextBuffer([]string{"modified", "content"})
	p.editMode = true

	cmd := saveFile(p)
	require.NotNil(t, cmd)

	msg := cmd()
	saved, ok := msg.(fileSavedMsg)
	require.True(t, ok, "should produce fileSavedMsg, got %T", msg)
	assert.Equal(t, path, saved.path)

	// Verify disk content.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "modified\ncontent", string(data))
}

func TestSaveFile_NilBuffer(t *testing.T) {
	p := testPreview()
	p.editBuf = nil

	cmd := saveFile(p)
	assert.Nil(t, cmd)
}

func TestSaveFile_EmptyPath(t *testing.T) {
	p := testPreview()
	p.filePath = ""
	p.editBuf = NewTextBuffer([]string{"data"})

	cmd := saveFile(p)
	assert.Nil(t, cmd)
}

func TestSaveFile_InvalidDir(t *testing.T) {
	p := testPreview()
	p.filePath = filepath.Join(t.TempDir(), "nonexistent", "deep", "file.txt")
	p.editBuf = NewTextBuffer([]string{"data"})
	p.editMode = true

	cmd := saveFile(p)
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "should produce error toast, got %T", msg)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Save failed")
}

// ---------------------------------------------------------------------------
// handleFileSaved
// ---------------------------------------------------------------------------

func TestHandleFileSaved_MarksClean(t *testing.T) {
	p, path := testPreviewWithFile(t, "content")
	enterEditMode(p)
	// Simulate modification to make buffer dirty.
	p.editBuf.InsertRune(0, 0, 'X')
	require.True(t, p.editBuf.Dirty())

	_, cmd := handleFileSaved(p, fileSavedMsg{path: path})
	assert.False(t, p.editBuf.Dirty(), "buffer should be marked clean")
	assert.NotNil(t, cmd, "should produce batch cmd for toast + FileModifiedMsg")
}

// ---------------------------------------------------------------------------
// handleModalResult
// ---------------------------------------------------------------------------

func TestHandleModalResult_Cancel(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	enterEditMode(p)

	// Esc / cancel
	_, cmd := handleModalResult(p, notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancel should produce no command")
	assert.True(t, p.editMode, "should still be in edit mode")
}

func TestHandleModalResult_CancelAction(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	enterEditMode(p)

	_, cmd := handleModalResult(p, notify.ModalResultMsg{Accept: true, Value: "cancel"})
	assert.Nil(t, cmd)
	assert.True(t, p.editMode)
}

func TestHandleModalResult_Discard(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	enterEditMode(p)

	_, cmd := handleModalResult(p, notify.ModalResultMsg{Accept: true, Value: "discard_exit"})
	// Should trigger exit (editMode cleared, buffer nil'd).
	assert.NotNil(t, cmd)
}

func TestHandleModalResult_SaveAndExit(t *testing.T) {
	p, _ := testPreviewWithFile(t, "data")
	enterEditMode(p)

	_, cmd := handleModalResult(p, notify.ModalResultMsg{Accept: true, Value: "save_exit"})
	assert.NotNil(t, cmd, "save+exit should produce a batch cmd")
}

// ---------------------------------------------------------------------------
// dirtyGuardCmd
// ---------------------------------------------------------------------------

func TestDirtyGuardCmd_ProducesModal(t *testing.T) {
	p, _ := testPreviewWithFile(t, "some content")
	enterEditMode(p)

	cmd := dirtyGuardCmd(p, "exit")
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "should produce ShowModalMsg, got %T", msg)
	assert.Equal(t, "Unsaved Changes", modal.Title)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
	require.Len(t, modal.Actions, 3)
	assert.Equal(t, "save_exit", modal.Actions[0].ID)
	assert.Equal(t, "discard_exit", modal.Actions[1].ID)
	assert.Equal(t, "cancel", modal.Actions[2].ID)
}

// ---------------------------------------------------------------------------
// countLeadingSpaces
// ---------------------------------------------------------------------------

func TestCountLeadingSpaces(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"no indent", "hello", 0},
		{"spaces", "    hello", 4},
		{"tab", "\thello", 4},
		{"mixed", "  \thello", 6},
		{"empty", "", 0},
		{"only spaces", "   ", 3},
		{"only tabs", "\t\t", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countLeadingSpaces(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// clampCursorCol
// ---------------------------------------------------------------------------

func TestClampCursorCol(t *testing.T) {
	p, _ := testPreviewWithFile(t, "hi")
	enterEditMode(p)
	p.cursorCol = 100

	clampCursorCol(p)
	assert.Equal(t, 2, p.cursorCol, "should clamp to line length")
}

func TestClampCursorCol_NilBuffer(t *testing.T) {
	p := testPreview()
	p.editBuf = nil
	p.cursorCol = 10

	clampCursorCol(p) // should not panic
	assert.Equal(t, 10, p.cursorCol, "should be unchanged with nil buffer")
}

// ---------------------------------------------------------------------------
// handleEditKeyPress — tab / shift+tab
// ---------------------------------------------------------------------------

func TestHandleEditKeyPress_Tab(t *testing.T) {
	p, _ := testPreviewWithFile(t, "hello")
	enterEditMode(p)
	p.cursorCol = 0
	p.editCfg.TabSize = 4

	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyTab})

	line := p.editBuf.Line(0)
	assert.True(t, strings.HasPrefix(line, "    "), "should insert 4 spaces, got: %q", line)
	assert.Equal(t, 4, p.cursorCol)
}

func TestHandleEditKeyPress_Delete(t *testing.T) {
	p, _ := testPreviewWithFile(t, "abc")
	enterEditMode(p)
	p.cursorCol = 1

	handleEditKeyPress(p, tea.KeyPressMsg{Code: tea.KeyDelete})

	assert.Equal(t, "ac", p.editBuf.Line(0))
	assert.Equal(t, 1, p.cursorCol, "cursor should stay in place after delete")
}

func TestHandleEditKeyPress_CtrlD_DuplicateLine(t *testing.T) {
	p, _ := testPreviewWithFile(t, "duplicate me")
	enterEditMode(p)
	p.cursorLine = 0

	handleEditKeyPress(p, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	assert.Equal(t, 2, p.editBuf.LineCount())
	assert.Equal(t, "duplicate me", p.editBuf.Line(0))
	assert.Equal(t, "duplicate me", p.editBuf.Line(1))
	assert.Equal(t, 1, p.cursorLine, "cursor should move to duplicated line")
}

// ---------------------------------------------------------------------------
// Integration: multiple edits
// ---------------------------------------------------------------------------

func TestEditSequence_TypeAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "editable.txt")
	require.NoError(t, os.WriteFile(path, []byte("start"), 0o644))

	p := testPreview()
	p.filePath = path
	p.width = 80
	p.height = 24
	p.editCfg = config.EditorConfig{TabSize: 4, AutoIndent: true}

	// Enter edit mode.
	cmd := enterEditMode(p)
	require.NotNil(t, cmd)
	require.True(t, p.editMode)

	// Type at end of first line.
	p.cursorCol = 5
	handleEditKeyPress(p, tea.KeyPressMsg{Text: "!", Code: '!'})
	assert.Equal(t, "start!", p.editBuf.Line(0))

	// Save.
	saveCmd := saveFile(p)
	require.NotNil(t, saveCmd)
	msg := saveCmd()
	_, ok := msg.(fileSavedMsg)
	require.True(t, ok)

	// Verify on disk.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "start!", string(data))
}
