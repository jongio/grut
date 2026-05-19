package preview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withFakeTime overrides nowNano with a controllable clock for the duration
// of a test. The returned function advances the fake clock.
func withFakeTime(t *testing.T) func(int64) {
	t.Helper()
	var fakeNano int64
	orig := nowNano
	nowNano = func() int64 { return fakeNano }
	t.Cleanup(func() { nowNano = orig })
	return func(ns int64) { fakeNano = ns }
}

// ---------------------------------------------------------------------------
// NewTextBuffer
// ---------------------------------------------------------------------------

func TestNewTextBuffer_Nil(t *testing.T) {
	buf := NewTextBuffer(nil)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
	assert.False(t, buf.Dirty())
}

func TestNewTextBuffer_Empty(t *testing.T) {
	buf := NewTextBuffer([]string{})
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
}

func TestNewTextBuffer_SingleLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "hello", buf.Line(0))
}

func TestNewTextBuffer_MultiLine(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	assert.Equal(t, 3, buf.LineCount())
	assert.Equal(t, []string{"a", "b", "c"}, buf.Lines())
}

func TestNewTextBuffer_DeepCopy(t *testing.T) {
	orig := []string{"hello", "world"}
	buf := NewTextBuffer(orig)
	orig[0] = "mutated"
	assert.Equal(t, "hello", buf.Line(0), "constructor must deep-copy input")
}

// ---------------------------------------------------------------------------
// Line bounds
// ---------------------------------------------------------------------------

func TestLine_OutOfRange(t *testing.T) {
	buf := NewTextBuffer([]string{"only"})
	assert.Equal(t, "", buf.Line(-1))
	assert.Equal(t, "", buf.Line(1))
	assert.Equal(t, "", buf.Line(999))
}

// ---------------------------------------------------------------------------
// InsertRune
// ---------------------------------------------------------------------------

func TestInsertRune_Start(t *testing.T) {
	buf := NewTextBuffer([]string{"ello"})
	buf.InsertRune(0, 0, 'h')
	assert.Equal(t, "hello", buf.Line(0))
}

func TestInsertRune_Middle(t *testing.T) {
	buf := NewTextBuffer([]string{"hllo"})
	buf.InsertRune(0, 1, 'e')
	assert.Equal(t, "hello", buf.Line(0))
}

func TestInsertRune_End(t *testing.T) {
	buf := NewTextBuffer([]string{"hell"})
	buf.InsertRune(0, 4, 'o')
	assert.Equal(t, "hello", buf.Line(0))
}

func TestInsertRune_Emoji(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 5, '🌍')
	assert.Equal(t, "hello🌍", buf.Line(0))
}

func TestInsertRune_CJK(t *testing.T) {
	buf := NewTextBuffer([]string{"你好"})
	buf.InsertRune(0, 1, '世')
	assert.Equal(t, "你世好", buf.Line(0))
}

func TestInsertRune_ClampsOutOfBounds(t *testing.T) {
	buf := NewTextBuffer([]string{"ab"})
	buf.InsertRune(0, 99, 'c') // col clamped to 2
	assert.Equal(t, "abc", buf.Line(0))

	buf.InsertRune(99, 0, 'x') // line clamped to 0
	assert.Equal(t, "xabc", buf.Line(0))
}

// ---------------------------------------------------------------------------
// DeleteRune (backspace)
// ---------------------------------------------------------------------------

func TestDeleteRune_Middle(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.DeleteRune(0, 3) // delete 'l' at index 2
	assert.Equal(t, 0, nl)
	assert.Equal(t, 2, nc)
	assert.Equal(t, "helo", buf.Line(0))
}

func TestDeleteRune_JoinLines(t *testing.T) {
	buf := NewTextBuffer([]string{"hello", "world"})
	nl, nc := buf.DeleteRune(1, 0) // backspace at start of line 1
	assert.Equal(t, 0, nl)
	assert.Equal(t, 5, nc)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "helloworld", buf.Line(0))
}

func TestDeleteRune_FirstLineCol0_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.DeleteRune(0, 0)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
	assert.Equal(t, "hello", buf.Line(0))
	assert.False(t, buf.Dirty(), "no-op should not set dirty")
}

func TestDeleteRune_SingleCharLine(t *testing.T) {
	buf := NewTextBuffer([]string{"x"})
	nl, nc := buf.DeleteRune(0, 1)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
	assert.Equal(t, "", buf.Line(0))
}

// ---------------------------------------------------------------------------
// DeleteForward (delete key)
// ---------------------------------------------------------------------------

func TestDeleteForward_Middle(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteForward(0, 2) // delete 'l' at index 2
	assert.Equal(t, "helo", buf.Line(0))
}

func TestDeleteForward_JoinLines(t *testing.T) {
	buf := NewTextBuffer([]string{"hello", "world"})
	buf.DeleteForward(0, 5) // at EOL — join
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "helloworld", buf.Line(0))
}

func TestDeleteForward_LastLineEOL_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteForward(0, 5)
	assert.Equal(t, "hello", buf.Line(0))
	assert.False(t, buf.Dirty(), "no-op should not set dirty")
}

func TestDeleteForward_EmptyLine(t *testing.T) {
	buf := NewTextBuffer([]string{"", "world"})
	buf.DeleteForward(0, 0) // empty line, join with next
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "world", buf.Line(0))
}

// ---------------------------------------------------------------------------
// SplitLine
// ---------------------------------------------------------------------------

func TestSplitLine_Middle(t *testing.T) {
	buf := NewTextBuffer([]string{"helloworld"})
	buf.SplitLine(0, 5, false)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "hello", buf.Line(0))
	assert.Equal(t, "world", buf.Line(1))
}

func TestSplitLine_Start(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.SplitLine(0, 0, false)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
	assert.Equal(t, "hello", buf.Line(1))
}

func TestSplitLine_End(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.SplitLine(0, 5, false)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "hello", buf.Line(0))
	assert.Equal(t, "", buf.Line(1))
}

func TestSplitLine_AutoIndent(t *testing.T) {
	buf := NewTextBuffer([]string{"    hello"})
	buf.SplitLine(0, 9, true) // split after all content
	assert.Equal(t, "    hello", buf.Line(0))
	assert.Equal(t, "    ", buf.Line(1))
}

func TestSplitLine_AutoIndentMiddle(t *testing.T) {
	buf := NewTextBuffer([]string{"  helloworld"})
	buf.SplitLine(0, 7, true) // "  hello" | "world"
	assert.Equal(t, "  hello", buf.Line(0))
	assert.Equal(t, "  world", buf.Line(1))
}

func TestSplitLine_AutoIndentTabsAndSpaces(t *testing.T) {
	buf := NewTextBuffer([]string{"\t  code"})
	buf.SplitLine(0, 5, true) // "\t  co" | "de"
	assert.Equal(t, "\t  co", buf.Line(0))
	assert.Equal(t, "\t  de", buf.Line(1))
}

// ---------------------------------------------------------------------------
// InsertTab
// ---------------------------------------------------------------------------

func TestInsertTab_AtCol0(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nc := buf.InsertTab(0, 0, 4)
	assert.Equal(t, 4, nc)
	assert.Equal(t, "    hello", buf.Line(0))
}

func TestInsertTab_Alignment(t *testing.T) {
	buf := NewTextBuffer([]string{"  hello"})
	nc := buf.InsertTab(0, 2, 4) // 4 - (2%4) = 2 spaces
	assert.Equal(t, 4, nc)
	assert.Equal(t, "    hello", buf.Line(0))
}

func TestInsertTab_AlreadyAligned(t *testing.T) {
	buf := NewTextBuffer([]string{"    hello"})
	nc := buf.InsertTab(0, 4, 4) // 4 - (4%4) = 4 spaces (full tab)
	assert.Equal(t, 8, nc)
	assert.Equal(t, "        hello", buf.Line(0))
}

func TestInsertTab_TabSize2(t *testing.T) {
	buf := NewTextBuffer([]string{"x"})
	nc := buf.InsertTab(0, 1, 2) // 2 - (1%2) = 1 space
	assert.Equal(t, 2, nc)
	assert.Equal(t, "x ", buf.Line(0))
}

// ---------------------------------------------------------------------------
// Dedent
// ---------------------------------------------------------------------------

func TestDedent_FullTabWidth(t *testing.T) {
	buf := NewTextBuffer([]string{"    hello"})
	buf.Dedent(0, 4)
	assert.Equal(t, "hello", buf.Line(0))
}

func TestDedent_Partial(t *testing.T) {
	buf := NewTextBuffer([]string{"  hello"})
	buf.Dedent(0, 4) // only 2 spaces available
	assert.Equal(t, "hello", buf.Line(0))
}

func TestDedent_NoWhitespace(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.Dedent(0, 4)
	assert.Equal(t, "hello", buf.Line(0))
	assert.False(t, buf.Dirty(), "no-op dedent should not set dirty")
}

func TestDedent_TabsNotRemoved(t *testing.T) {
	buf := NewTextBuffer([]string{"\thello"})
	buf.Dedent(0, 4) // tab is not a space — nothing removed
	assert.Equal(t, "\thello", buf.Line(0))
	assert.False(t, buf.Dirty())
}

// ---------------------------------------------------------------------------
// DuplicateLine
// ---------------------------------------------------------------------------

func TestDuplicateLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello", "world"})
	buf.DuplicateLine(0)
	assert.Equal(t, 3, buf.LineCount())
	assert.Equal(t, "hello", buf.Line(0))
	assert.Equal(t, "hello", buf.Line(1))
	assert.Equal(t, "world", buf.Line(2))
}

func TestDuplicateLine_LastLine(t *testing.T) {
	buf := NewTextBuffer([]string{"only"})
	buf.DuplicateLine(0)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "only", buf.Line(0))
	assert.Equal(t, "only", buf.Line(1))
}

// ---------------------------------------------------------------------------
// Undo / Redo
// ---------------------------------------------------------------------------

func TestUndo_RestoresState(t *testing.T) {
	setTime := withFakeTime(t)
	setTime(1_000_000_000)

	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 5, '!')
	assert.Equal(t, "hello!", buf.Line(0))

	nl, nc, ok := buf.Undo(0, 6)
	require.True(t, ok)
	assert.Equal(t, "hello", buf.Line(0))
	assert.Equal(t, 0, nl)
	assert.Equal(t, 5, nc)
}

func TestUndo_EmptyStack(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc, ok := buf.Undo(0, 0)
	assert.False(t, ok)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
}

func TestRedo_RestoresState(t *testing.T) {
	setTime := withFakeTime(t)
	setTime(1_000_000_000)

	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 5, '!')
	buf.Undo(0, 6)
	assert.Equal(t, "hello", buf.Line(0))

	_, _, ok := buf.Redo(0, 5)
	require.True(t, ok)
	assert.Equal(t, "hello!", buf.Line(0))
}

func TestRedo_EmptyStack(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	_, _, ok := buf.Redo(0, 0)
	assert.False(t, ok)
}

func TestRedo_ClearedByNewEdit(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{"hello"})
	setTime(1_000_000_000)
	buf.InsertRune(0, 5, '!')
	buf.Undo(0, 6)

	// New edit should clear redo stack.
	setTime(2_000_000_000)
	buf.InsertRune(0, 5, '?')
	_, _, ok := buf.Redo(0, 6)
	assert.False(t, ok, "redo should be empty after new edit")
}

func TestUndoRedo_MultipleSteps(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{""})
	setTime(1_000_000_000)
	buf.InsertRune(0, 0, 'a')
	setTime(2_000_000_000) // beyond 500ms — separate undo unit
	buf.InsertRune(0, 1, 'b')
	assert.Equal(t, "ab", buf.Line(0))

	// Undo 'b'
	_, _, ok := buf.Undo(0, 2)
	require.True(t, ok)
	assert.Equal(t, "a", buf.Line(0))

	// Undo 'a'
	_, _, ok = buf.Undo(0, 1)
	require.True(t, ok)
	assert.Equal(t, "", buf.Line(0))

	// No more undo
	_, _, ok = buf.Undo(0, 0)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Undo coalescing
// ---------------------------------------------------------------------------

func TestUndoCoalescing_RapidInsertsAreOneUnit(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{""})
	setTime(1_000_000_000)
	buf.InsertRune(0, 0, 'a')
	setTime(1_100_000_000) // +100ms
	buf.InsertRune(0, 1, 'b')
	setTime(1_200_000_000) // +100ms
	buf.InsertRune(0, 2, 'c')

	assert.Equal(t, "abc", buf.Line(0))

	// Single undo reverts all three
	_, _, ok := buf.Undo(0, 3)
	require.True(t, ok)
	assert.Equal(t, "", buf.Line(0))

	// Nothing more to undo
	_, _, ok = buf.Undo(0, 0)
	assert.False(t, ok)
}

func TestUndoCoalescing_PauseBreaksCoalescing(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{""})
	setTime(1_000_000_000)
	buf.InsertRune(0, 0, 'a')
	setTime(1_100_000_000) // +100ms — coalesced
	buf.InsertRune(0, 1, 'b')

	setTime(2_000_000_000) // +900ms — beyond 500ms window
	buf.InsertRune(0, 2, 'c')

	assert.Equal(t, "abc", buf.Line(0))

	// Undo 'c' (separate unit)
	_, _, ok := buf.Undo(0, 3)
	require.True(t, ok)
	assert.Equal(t, "ab", buf.Line(0))

	// Undo 'a'+'b' (coalesced unit)
	_, _, ok = buf.Undo(0, 2)
	require.True(t, ok)
	assert.Equal(t, "", buf.Line(0))
}

func TestUndoCoalescing_SplitLineBreaks(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{"ab"})
	setTime(1_000_000_000)
	buf.InsertRune(0, 2, 'c')
	setTime(1_100_000_000) // +100ms — would coalesce for InsertRune
	buf.SplitLine(0, 2, false)

	// SplitLine forces undo break — should be separate from 'c' insert
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "ab", buf.Line(0))
	assert.Equal(t, "c", buf.Line(1))

	// Undo SplitLine
	_, _, ok := buf.Undo(0, 0)
	require.True(t, ok)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "abc", buf.Line(0))

	// Undo InsertRune('c')
	_, _, ok = buf.Undo(0, 0)
	require.True(t, ok)
	assert.Equal(t, "ab", buf.Line(0))
}

// ---------------------------------------------------------------------------
// Dirty flag
// ---------------------------------------------------------------------------

func TestDirty_InitiallyClean(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	assert.False(t, buf.Dirty())
}

func TestDirty_SetByEdit(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 0, 'x')
	assert.True(t, buf.Dirty())
}

func TestDirty_ClearedByMarkClean(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 0, 'x')
	buf.MarkClean()
	assert.False(t, buf.Dirty())
}

func TestDirty_SetBySetLines(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.SetLines([]string{"new"})
	assert.True(t, buf.Dirty())
}

func TestDirty_SetByDeleteRune(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteRune(0, 3)
	assert.True(t, buf.Dirty())
}

func TestDirty_SetByDeleteForward(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteForward(0, 2)
	assert.True(t, buf.Dirty())
}

func TestDirty_SetBySplitLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.SplitLine(0, 3, false)
	assert.True(t, buf.Dirty())
}

func TestDirty_SetByDuplicateLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DuplicateLine(0)
	assert.True(t, buf.Dirty())
}

// ---------------------------------------------------------------------------
// SetLines
// ---------------------------------------------------------------------------

func TestSetLines_ReplacesContent(t *testing.T) {
	buf := NewTextBuffer([]string{"old"})
	buf.SetLines([]string{"new", "content"})
	assert.Equal(t, []string{"new", "content"}, buf.Lines())
}

func TestSetLines_ClearsUndo(t *testing.T) {
	setTime := withFakeTime(t)
	setTime(1_000_000_000)

	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 5, '!')
	buf.SetLines([]string{"replaced"})

	_, _, ok := buf.Undo(0, 0)
	assert.False(t, ok, "undo should be empty after SetLines")
}

func TestSetLines_EmptyProducesSingleLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.SetLines(nil)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
}

// ---------------------------------------------------------------------------
// DeleteRange
// ---------------------------------------------------------------------------

func TestDeleteRange_SingleLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hello world"})
	nl, nc := buf.DeleteRange(0, 5, 0, 11)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 5, nc)
	assert.Equal(t, "hello", buf.Line(0))
}

func TestDeleteRange_MultiLine(t *testing.T) {
	buf := NewTextBuffer([]string{"aaa", "bbb", "ccc"})
	nl, nc := buf.DeleteRange(0, 1, 2, 2)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 1, nc)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "ac", buf.Line(0))
}

func TestDeleteRange_FullLine(t *testing.T) {
	buf := NewTextBuffer([]string{"first", "second", "third"})
	// Delete the entirety of lines 0-1.
	nl, nc := buf.DeleteRange(0, 0, 1, 6)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
	assert.Equal(t, "third", buf.Line(1))
}

func TestDeleteRange_EmptyRange_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.DeleteRange(0, 2, 0, 2)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 2, nc)
	assert.Equal(t, "hello", buf.Line(0))
	assert.False(t, buf.Dirty(), "empty range should not set dirty")
}

func TestDeleteRange_ClampedInputs(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.DeleteRange(0, 0, 99, 99)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
	assert.Equal(t, "", buf.Line(0))
}

func TestDeleteRange_CursorPosition(t *testing.T) {
	buf := NewTextBuffer([]string{"abcdef"})
	nl, nc := buf.DeleteRange(0, 2, 0, 4)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 2, nc)
	assert.Equal(t, "abef", buf.Line(0))
}

func TestDeleteRange_MarksDirty(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteRange(0, 0, 0, 3)
	assert.True(t, buf.Dirty())
}

func TestDeleteRange_Undo(t *testing.T) {
	buf := NewTextBuffer([]string{"aaa", "bbb", "ccc"})
	buf.DeleteRange(0, 1, 2, 2)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "ac", buf.Line(0))

	nl, nc, ok := buf.Undo(0, 1)
	require.True(t, ok)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 1, nc)
	assert.Equal(t, 3, buf.LineCount())
	assert.Equal(t, []string{"aaa", "bbb", "ccc"}, buf.Lines())
}

func TestDeleteRange_ReversedInputs(t *testing.T) {
	// If start > end, they should be swapped.
	buf := NewTextBuffer([]string{"hello world"})
	nl, nc := buf.DeleteRange(0, 11, 0, 5)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 5, nc)
	assert.Equal(t, "hello", buf.Line(0))
}

// ---------------------------------------------------------------------------
// InsertText
// ---------------------------------------------------------------------------

func TestInsertText_SingleLine(t *testing.T) {
	buf := NewTextBuffer([]string{"hd"})
	nl, nc := buf.InsertText(0, 1, "ello worl")
	assert.Equal(t, 0, nl)
	assert.Equal(t, 10, nc)
	assert.Equal(t, "hello world", buf.Line(0))
}

func TestInsertText_MultiLine_TwoLines(t *testing.T) {
	buf := NewTextBuffer([]string{"ac"})
	nl, nc := buf.InsertText(0, 1, "b\nd")
	assert.Equal(t, 1, nl)
	assert.Equal(t, 1, nc)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "ab", buf.Line(0))
	assert.Equal(t, "dc", buf.Line(1))
}

func TestInsertText_MultiLine_ThreeLines(t *testing.T) {
	buf := NewTextBuffer([]string{"XY"})
	nl, nc := buf.InsertText(0, 1, "a\nb\nc")
	assert.Equal(t, 2, nl)
	assert.Equal(t, 1, nc)
	assert.Equal(t, 3, buf.LineCount())
	assert.Equal(t, "Xa", buf.Line(0))
	assert.Equal(t, "b", buf.Line(1))
	assert.Equal(t, "cY", buf.Line(2))
}

func TestInsertText_EmptyString_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.InsertText(0, 2, "")
	assert.Equal(t, 0, nl)
	assert.Equal(t, 2, nc)
	assert.Equal(t, "hello", buf.Line(0))
	assert.False(t, buf.Dirty(), "empty text should not set dirty")
}

func TestInsertText_AtLineStart(t *testing.T) {
	buf := NewTextBuffer([]string{"world"})
	nl, nc := buf.InsertText(0, 0, "hello ")
	assert.Equal(t, 0, nl)
	assert.Equal(t, 6, nc)
	assert.Equal(t, "hello world", buf.Line(0))
}

func TestInsertText_AtLineEnd(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	nl, nc := buf.InsertText(0, 5, " world")
	assert.Equal(t, 0, nl)
	assert.Equal(t, 11, nc)
	assert.Equal(t, "hello world", buf.Line(0))
}

func TestInsertText_AtMiddle(t *testing.T) {
	buf := NewTextBuffer([]string{"hd"})
	nl, nc := buf.InsertText(0, 1, "ello worl")
	assert.Equal(t, 0, nl)
	assert.Equal(t, 10, nc)
	assert.Equal(t, "hello world", buf.Line(0))
}

func TestInsertText_CursorPosition_MultiLine(t *testing.T) {
	buf := NewTextBuffer([]string{""})
	nl, nc := buf.InsertText(0, 0, "line1\nline2\nline3")
	assert.Equal(t, 2, nl)
	assert.Equal(t, 5, nc)
	assert.Equal(t, 3, buf.LineCount())
}

func TestInsertText_MarksDirty(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.InsertText(0, 0, "x")
	assert.True(t, buf.Dirty())
}

func TestInsertText_Undo(t *testing.T) {
	buf := NewTextBuffer([]string{"ac"})
	buf.InsertText(0, 1, "b\nd")
	assert.Equal(t, 2, buf.LineCount())

	nl, nc, ok := buf.Undo(1, 1)
	require.True(t, ok)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 1, nc)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "ac", buf.Line(0))
}

// ---------------------------------------------------------------------------
// DeleteLine
// ---------------------------------------------------------------------------

func TestDeleteLine_MiddleLine(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	buf.DeleteLine(1)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, []string{"a", "c"}, buf.Lines())
}

func TestDeleteLine_FirstLine(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	buf.DeleteLine(0)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, []string{"b", "c"}, buf.Lines())
}

func TestDeleteLine_LastLine(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	buf.DeleteLine(2)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, []string{"a", "b"}, buf.Lines())
}

func TestDeleteLine_OnlyLine_ClearsToEmpty(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.DeleteLine(0)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
}

func TestDeleteLine_MarksDirty(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b"})
	buf.DeleteLine(0)
	assert.True(t, buf.Dirty())
}

func TestDeleteLine_Undo(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	buf.DeleteLine(1)
	assert.Equal(t, []string{"a", "c"}, buf.Lines())

	_, _, ok := buf.Undo(1, 0)
	require.True(t, ok)
	assert.Equal(t, 3, buf.LineCount())
	assert.Equal(t, []string{"a", "b", "c"}, buf.Lines())
}

// ---------------------------------------------------------------------------
// MoveLine
// ---------------------------------------------------------------------------

func TestMoveLine_Down(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	ok := buf.MoveLine(0, 1)
	assert.True(t, ok)
	assert.Equal(t, []string{"b", "a", "c"}, buf.Lines())
}

func TestMoveLine_Up(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	ok := buf.MoveLine(2, -1)
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "c", "b"}, buf.Lines())
}

func TestMoveLine_TopBoundary_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	ok := buf.MoveLine(0, -1)
	assert.False(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, buf.Lines())
	assert.False(t, buf.Dirty(), "no-op move should not set dirty")
}

func TestMoveLine_BottomBoundary_Noop(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	ok := buf.MoveLine(2, 1)
	assert.False(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, buf.Lines())
	assert.False(t, buf.Dirty(), "no-op move should not set dirty")
}

func TestMoveLine_MarksDirty(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b"})
	buf.MoveLine(0, 1)
	assert.True(t, buf.Dirty())
}

func TestMoveLine_Undo(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})
	buf.MoveLine(0, 1)
	assert.Equal(t, []string{"b", "a", "c"}, buf.Lines())

	_, _, ok := buf.Undo(1, 0)
	require.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, buf.Lines())
}

// ---------------------------------------------------------------------------
// Edge cases: empty buffer
// ---------------------------------------------------------------------------

func TestEmptyBuffer_InsertAndDelete(t *testing.T) {
	buf := NewTextBuffer(nil)
	assert.Equal(t, 1, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))

	buf.InsertRune(0, 0, 'x')
	assert.Equal(t, "x", buf.Line(0))

	nl, nc := buf.DeleteRune(0, 1)
	assert.Equal(t, 0, nl)
	assert.Equal(t, 0, nc)
	assert.Equal(t, "", buf.Line(0))
}

func TestEmptyBuffer_SplitLine(t *testing.T) {
	buf := NewTextBuffer(nil)
	buf.SplitLine(0, 0, false)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
	assert.Equal(t, "", buf.Line(1))
}

func TestEmptyBuffer_DuplicateLine(t *testing.T) {
	buf := NewTextBuffer(nil)
	buf.DuplicateLine(0)
	assert.Equal(t, 2, buf.LineCount())
	assert.Equal(t, "", buf.Line(0))
	assert.Equal(t, "", buf.Line(1))
}

// ---------------------------------------------------------------------------
// Max undo depth
// ---------------------------------------------------------------------------

func TestMaxUndoDepth(t *testing.T) {
	setTime := withFakeTime(t)

	buf := NewTextBuffer([]string{""})
	// Push 105 snapshots (exceeding maxUndoDepth of 100).
	for i := 0; i < 105; i++ {
		setTime(int64(i+1) * 1_000_000_000) // each 1s apart — no coalescing
		buf.InsertRune(0, i, rune('a'+(i%26)))
	}

	// Should only be able to undo 100 times.
	undone := 0
	for {
		_, _, ok := buf.Undo(0, 0)
		if !ok {
			break
		}
		undone++
	}
	assert.Equal(t, maxUndoDepth, undone)
}
