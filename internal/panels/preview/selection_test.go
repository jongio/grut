package preview

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPreview creates a Preview with test lines loaded.
func newTestPreview(lines []string) *Preview {
	p := New(config.PreviewConfig{
		Enabled:     true,
		MaxFileSize: 1048576,
	}, defaultEditorCfg(), nil)
	p.lines = lines
	p.width = 80
	p.height = 24
	p.focused = true
	return p
}

// ---------------------------------------------------------------------------
// selPoint and selRange
// ---------------------------------------------------------------------------

func TestSelRange_NilAnchors(t *testing.T) {
	p := newTestPreview(nil)
	s, e := p.selRange()
	assert.Nil(t, s)
	assert.Nil(t, e)
}

func TestSelRange_Normalized(t *testing.T) {
	p := newTestPreview([]string{"hello", "world"})
	// Anchor after end → should be swapped.
	p.selAnchor = &selPoint{Line: 1, Col: 3}
	p.selEnd = &selPoint{Line: 0, Col: 1}
	s, e := p.selRange()
	require.NotNil(t, s)
	assert.Equal(t, 0, s.Line)
	assert.Equal(t, 1, s.Col)
	assert.Equal(t, 1, e.Line)
	assert.Equal(t, 3, e.Col)
}

func TestSelRange_SameLine(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 4}
	p.selEnd = &selPoint{Line: 0, Col: 1}
	s, e := p.selRange()
	require.NotNil(t, s)
	assert.Equal(t, 1, s.Col)
	assert.Equal(t, 4, e.Col)
}

// ---------------------------------------------------------------------------
// hasSelection / clearSelection
// ---------------------------------------------------------------------------

func TestHasSelection_Empty(t *testing.T) {
	p := newTestPreview(nil)
	assert.False(t, p.hasSelection())
}

func TestHasSelection_SamePoint(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 2}
	p.selEnd = &selPoint{Line: 0, Col: 2}
	assert.False(t, p.hasSelection(), "same anchor and end = no selection")
}

func TestHasSelection_Valid(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 3}
	assert.True(t, p.hasSelection())
}

func TestClearSelection(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 3}
	p.selecting = true
	p.clearSelection()
	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
	assert.False(t, p.selecting)
}

// ---------------------------------------------------------------------------
// selectedText
// ---------------------------------------------------------------------------

func TestSelectedText_SingleLine(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	assert.Equal(t, "hello", p.selectedText())
}

func TestSelectedText_MultiLine(t *testing.T) {
	p := newTestPreview([]string{"line one", "line two", "line three"})
	p.selAnchor = &selPoint{Line: 0, Col: 5}
	p.selEnd = &selPoint{Line: 2, Col: 4}
	got := p.selectedText()
	assert.Equal(t, "one\nline two\nline", got)
}

func TestSelectedText_WithANSI(t *testing.T) {
	p := newTestPreview([]string{"\x1b[31mhello\x1b[0m world"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	// selectedText strips ANSI via ansi.Strip internally
	assert.Equal(t, "hello", got)
}

func TestSelectedText_Empty(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	assert.Equal(t, "", p.selectedText())
}

func TestSelectedText_ColBeyondLine(t *testing.T) {
	p := newTestPreview([]string{"hi"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 100}
	assert.Equal(t, "hi", p.selectedText())
}

func TestSelectedText_TabExpansion(t *testing.T) {
	p := newTestPreview([]string{"a\tb"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 6}
	got := p.selectedText()
	// Tab is expanded to 4 spaces: "a\tb" → "a    b" (6 chars).
	// Selecting cols 0..6 gets all 6 chars.
	assert.Equal(t, "a    b", got)
}

// ---------------------------------------------------------------------------
// contentRowColToAbs
// ---------------------------------------------------------------------------

func TestContentRowColToAbs_Basic(t *testing.T) {
	p := newTestPreview([]string{"hello", "world", "test"})
	p.scrollY = 0
	pt := p.contentRowColToAbs(1, 3)
	assert.Equal(t, 1, pt.Line)
	assert.Equal(t, 3, pt.Col)
}

func TestContentRowColToAbs_WithScroll(t *testing.T) {
	p := newTestPreview([]string{"a", "b", "c", "d", "e"})
	p.scrollY = 2
	pt := p.contentRowColToAbs(0, 0)
	assert.Equal(t, 2, pt.Line)
}

func TestContentRowColToAbs_WithLineNumbers(t *testing.T) {
	p := newTestPreview([]string{"hello", "world"})
	p.lineNumbers = true
	pt := p.contentRowColToAbs(0, 0)
	// With gutter, col 0 maps to rune 0 (gutter is subtracted, clamped to 0).
	assert.Equal(t, 0, pt.Col)
}

func TestContentRowColToAbs_ColClamped(t *testing.T) {
	p := newTestPreview([]string{"hi"})
	pt := p.contentRowColToAbs(0, 100)
	assert.Equal(t, 2, pt.Col, "col should be clamped to line rune count")
}

func TestContentRowColToAbs_NegativeRow(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	pt := p.contentRowColToAbs(-5, 0)
	assert.Equal(t, 0, pt.Line)
}

// ---------------------------------------------------------------------------
// Mouse event handlers
// ---------------------------------------------------------------------------

func TestHandleMouseClick_StartsSelection(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 3})

	assert.True(t, p.selecting)
	require.NotNil(t, p.selAnchor)
	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 3, p.selAnchor.Col)
}

func TestHandleMouseClick_BinarySkipped(t *testing.T) {
	p := newTestPreview(nil)
	p.isBinary = true
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 0})
	assert.Nil(t, p.selAnchor)
}

func TestHandleMouseMotion_ExtendsSelection(t *testing.T) {
	p := newTestPreview([]string{"hello world", "second line"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 0}
	p.selecting = true

	p.Update(panels.PanelMouseMotionMsg{ContentRow: 1, ContentCol: 5})

	require.NotNil(t, p.selEnd)
	assert.Equal(t, 1, p.selEnd.Line)
	assert.Equal(t, 5, p.selEnd.Col)
}

func TestHandleMouseMotion_IgnoredWhenNotSelecting(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selecting = false
	p.Update(panels.PanelMouseMotionMsg{ContentRow: 0, ContentCol: 3})
	assert.Nil(t, p.selEnd)
}

func TestHandleMouseRelease_FinalizesSelection(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 3}
	p.selecting = true

	p.Update(panels.PanelMouseReleaseMsg{ContentRow: 0, ContentCol: 3})

	assert.False(t, p.selecting)
	assert.NotNil(t, p.selAnchor, "anchor preserved after release")
	assert.NotNil(t, p.selEnd, "end preserved after release")
}

// ---------------------------------------------------------------------------
// Double-click word selection
// ---------------------------------------------------------------------------

func TestDoubleClick_SelectsWord(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 2})

	require.NotNil(t, p.selAnchor)
	require.NotNil(t, p.selEnd)
	s, e := p.selRange()
	assert.Equal(t, 0, s.Col)
	assert.Equal(t, 5, e.Col) // "hello" = cols 0..5
	assert.Equal(t, "hello", p.selectedText())
}

func TestDoubleClick_SelectsSecondWord(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 7})

	assert.Equal(t, "world", p.selectedText())
}

func TestDoubleClick_NonWordChar(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})

	// Space is a non-word char → select just the space.
	assert.Equal(t, " ", p.selectedText())
}

func TestDoubleClick_BinarySkipped(t *testing.T) {
	p := newTestPreview(nil)
	p.isBinary = true
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 0})
	assert.Nil(t, p.selAnchor)
}

// ---------------------------------------------------------------------------
// Keyboard: y (copy), escape (clear)
// ---------------------------------------------------------------------------

func TestKeyY_WithSelection_Clears(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	_, cmd := p.Update(keyMsg("y"))
	assert.NotNil(t, cmd, "should return copy command")
	// Selection should be cleared after copy.
	assert.Nil(t, p.selAnchor)
}

func TestKeyY_NoSelection_Noop(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	_, cmd := p.Update(keyMsg("y"))
	assert.Nil(t, cmd)
}

func TestKeyY_NoSelection_CopiesFilePath(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.filePath = `C:\repo\main.go`

	_, cmd := p.Update(keyMsg("y"))

	assert.NotNil(t, cmd, "should copy the file path when no text is selected")
}

func TestKeyY_GitHubModeDoesNotCopyFilePath(t *testing.T) {
	p := newTestPreview([]string{"issue"})
	p.filePath = `C:\repo\main.go`
	p.ghMode = true

	_, cmd := p.Update(keyMsg("y"))

	assert.Nil(t, cmd)
}

func TestKeyEscape_ClearsSelection(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 3}

	p.Update(keyMsg("escape"))
	assert.Nil(t, p.selAnchor)
}

// ---------------------------------------------------------------------------
// Selection cleared on new file load
// ---------------------------------------------------------------------------

func TestFileSelectedMsg_ClearsSelection(t *testing.T) {
	p := newTestPreview([]string{"old content"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	p.Update(panels.FileSelectedMsg{Path: "/tmp/test.txt"})

	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
}

// ---------------------------------------------------------------------------
// applySelectionHighlight
// ---------------------------------------------------------------------------

// _selP is a zero-value Preview used to call applySelectionHighlight in tests.
var _selP = &Preview{}

func TestApplySelectionHighlight_NoSelection(t *testing.T) {
	line := "hello world"
	got := _selP.applySelectionHighlight(line, 0, nil, nil)
	assert.Equal(t, line, got)
}

func TestApplySelectionHighlight_OutOfRange(t *testing.T) {
	line := "hello world"
	sel := &selPoint{Line: 5, Col: 0}
	selE := &selPoint{Line: 5, Col: 5}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	assert.Equal(t, line, got)
}

func TestApplySelectionHighlight_FullLine(t *testing.T) {
	line := "hello"
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 0, Col: 5}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	// The entire content should be highlighted.
	stripped := ansi.Strip(got)
	assert.Equal(t, "hello", stripped)
	assert.NotEqual(t, line, got, "should contain ANSI highlight codes")
}

func TestApplySelectionHighlight_PartialLine(t *testing.T) {
	line := "hello world"
	sel := &selPoint{Line: 0, Col: 6}
	selE := &selPoint{Line: 0, Col: 11}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	stripped := ansi.Strip(got)
	assert.Equal(t, "hello world", stripped)
	// "world" portion should have highlight codes.
	assert.Contains(t, got, "world")
	assert.NotEqual(t, line, got)
}

func TestApplySelectionHighlight_MultiLineMiddle(t *testing.T) {
	// For a middle line in a multi-line selection, the entire line is highlighted.
	line := "middle line"
	sel := &selPoint{Line: 0, Col: 3}
	selE := &selPoint{Line: 2, Col: 5}
	got := _selP.applySelectionHighlight(line, 1, sel, selE) // line 1 is fully within range
	stripped := ansi.Strip(got)
	assert.Equal(t, "middle line", stripped)
	assert.NotEqual(t, line, got, "should be highlighted")
}

func TestApplySelectionHighlight_WithANSI(t *testing.T) {
	line := "\x1b[31mhello\x1b[0m world"
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 0, Col: 5}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	stripped := ansi.Strip(got)
	assert.Equal(t, "hello world", stripped)
}

// ---------------------------------------------------------------------------
// numDigits helper
// ---------------------------------------------------------------------------

func TestNumDigits(t *testing.T) {
	assert.Equal(t, 1, numDigits(0))
	assert.Equal(t, 1, numDigits(1))
	assert.Equal(t, 1, numDigits(9))
	assert.Equal(t, 2, numDigits(10))
	assert.Equal(t, 3, numDigits(100))
	assert.Equal(t, 4, numDigits(1000))
}

// ---------------------------------------------------------------------------
// pluralize helper
// ---------------------------------------------------------------------------

func TestPluralize(t *testing.T) {
	assert.Equal(t, "1 char", pluralize(1, "char"))
	assert.Equal(t, "5 chars", pluralize(5, "char"))
	assert.Equal(t, "1 line", pluralize(1, "line"))
	assert.Equal(t, "12 lines", pluralize(12, "line"))
}

// ---------------------------------------------------------------------------
// Rendering with selection
// ---------------------------------------------------------------------------

func TestRenderContent_WithSelection(t *testing.T) {
	p := newTestPreview([]string{"hello world", "second line"})
	p.filePath = "/tmp/test.txt" // must be set so renderContent is used
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	out := p.View(80, 10)
	assert.NotEmpty(t, out)
	stripped := ansi.Strip(out)
	assert.Contains(t, stripped, "hello")
}

// ===========================================================================
// Security Test Suite — MQ Wave 1
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. Input boundary tests
// ---------------------------------------------------------------------------

func TestSecurity_NegativeCol(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	pt := p.contentRowColToAbs(0, -10)
	assert.GreaterOrEqual(t, pt.Col, 0, "negative col must be clamped to >= 0")
}

func TestSecurity_NegativeRowAndCol(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	pt := p.contentRowColToAbs(-100, -100)
	assert.Equal(t, 0, pt.Line)
	assert.GreaterOrEqual(t, pt.Col, 0)
}

func TestSecurity_VeryLargeRow(t *testing.T) {
	p := newTestPreview([]string{"a", "b", "c"})
	pt := p.contentRowColToAbs(999999, 0)
	assert.LessOrEqual(t, pt.Line, 2, "row must be clamped to last valid index")
}

func TestSecurity_VeryLargeCol(t *testing.T) {
	p := newTestPreview([]string{"hi"})
	pt := p.contentRowColToAbs(0, 999999)
	assert.Equal(t, 2, pt.Col, "col must be clamped to rune count")
}

func TestSecurity_MaxIntCoordinates(t *testing.T) {
	p := newTestPreview([]string{"x"})
	pt := p.contentRowColToAbs(1<<31-1, 1<<31-1)
	assert.GreaterOrEqual(t, pt.Line, 0)
	assert.LessOrEqual(t, pt.Line, 0)
	assert.GreaterOrEqual(t, pt.Col, 0)
}

func TestSecurity_EmptyLinesSlice(t *testing.T) {
	p := newTestPreview([]string{})
	// Must not panic on any operation.
	assert.False(t, p.hasSelection())
	assert.Equal(t, "", p.selectedText())
	pt := p.contentRowColToAbs(0, 0)
	assert.GreaterOrEqual(t, pt.Line, 0)
}

func TestSecurity_NilLines(t *testing.T) {
	p := newTestPreview(nil)
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	// Must not panic; selectedText on nil lines returns empty.
	assert.Equal(t, "", p.selectedText())
}

func TestSecurity_SelectedText_LineBeyondContent(t *testing.T) {
	p := newTestPreview([]string{"only one line"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 100, Col: 5}
	// Must not panic; extracts available content gracefully.
	got := p.selectedText()
	assert.Contains(t, got, "only one line")
}

func TestSecurity_SelectionStartBeyondContent(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selAnchor = &selPoint{Line: 50, Col: 0}
	p.selEnd = &selPoint{Line: 100, Col: 5}
	// Both beyond content - should return empty, not panic.
	got := p.selectedText()
	assert.Equal(t, "", got)
}

func TestSecurity_EmptyLineInContent(t *testing.T) {
	p := newTestPreview([]string{"first", "", "third"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 2, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "first\n\nthird", got)
}

// ---------------------------------------------------------------------------
// 2. ANSI robustness
// ---------------------------------------------------------------------------

func TestSecurity_ANSI_MalformedSequence_LoneESC(t *testing.T) {
	// Lone ESC byte with no bracket or terminator.
	p := newTestPreview([]string{"abc\x1bdef"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 6}
	got := p.selectedText()
	// ansi.Strip should handle this; must not panic.
	assert.NotEmpty(t, got)
}

func TestSecurity_ANSI_MalformedSequence_UnterminatedCSI(t *testing.T) {
	// ESC[ with no terminating byte.
	p := newTestPreview([]string{"abc\x1b[999def"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 10}
	got := p.selectedText()
	assert.NotEmpty(t, got)
}

func TestSecurity_ANSI_NestedSequences(t *testing.T) {
	// Multiple stacked ANSI codes (bold + red + underline).
	line := "\x1b[1m\x1b[31m\x1b[4mhello\x1b[0m world"
	p := newTestPreview([]string{line})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "hello", got, "nested ANSI must be fully stripped")
}

func TestSecurity_ANSI_VeryLongSequence(t *testing.T) {
	// ANSI sequence with extremely long parameter list.
	longParams := ""
	for i := 0; i < 1000; i++ {
		if i > 0 {
			longParams += ";"
		}
		longParams += "38"
	}
	line := "\x1b[" + longParams + "mhello\x1b[0m"
	p := newTestPreview([]string{line})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "hello", got)
}

func TestSecurity_ANSI_OSCSequence(t *testing.T) {
	// OSC (Operating System Command) sequence terminated by BEL.
	line := "\x1b]0;Window Title\x07hello world"
	p := newTestPreview([]string{line})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "hello", got)
}

func TestSecurity_ANSI_Highlight_MalformedSequence(t *testing.T) {
	// applySelectionHighlight must handle malformed ANSI gracefully.
	line := "abc\x1bdef\x1b[ghi"
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 0, Col: 6}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	// Must not panic; result must contain the visible text.
	stripped := ansi.Strip(got)
	assert.NotEmpty(t, stripped)
}

func TestSecurity_ANSI_Highlight_NestedSequences(t *testing.T) {
	line := "\x1b[1m\x1b[31mhello\x1b[0m"
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 0, Col: 5}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	stripped := ansi.Strip(got)
	assert.Equal(t, "hello", stripped)
}

// ---------------------------------------------------------------------------
// 3. DoS prevention — large inputs
// ---------------------------------------------------------------------------

func TestSecurity_VeryLongLine(t *testing.T) {
	long := strings.Repeat("A", 100_000)
	p := newTestPreview([]string{long})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 50}
	got := p.selectedText()
	assert.Equal(t, 50, len(got))
}

func TestSecurity_VeryLargeSelection(t *testing.T) {
	lines := make([]string, 5000)
	for i := range lines {
		lines[i] = "line content here"
	}
	p := newTestPreview(lines)
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 4999, Col: 17}
	got := p.selectedText()
	// Must not panic or hang; should contain all lines.
	lineCount := strings.Count(got, "\n") + 1
	assert.Equal(t, 5000, lineCount)
}

func TestSecurity_HighlightVeryLongLine(t *testing.T) {
	long := strings.Repeat("X", 100_000)
	sel := &selPoint{Line: 0, Col: 10}
	selE := &selPoint{Line: 0, Col: 100}
	got := _selP.applySelectionHighlight(long, 0, sel, selE)
	// Must not panic; highlighted output must contain the original visible chars.
	stripped := ansi.Strip(got)
	assert.Equal(t, 100_000, len(stripped))
}

func TestSecurity_ThousandsOfLinesWithANSI(t *testing.T) {
	lines := make([]string, 2000)
	for i := range lines {
		lines[i] = "\x1b[31mred\x1b[0m normal"
	}
	p := newTestPreview(lines)
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 1999, Col: 10}
	got := p.selectedText()
	assert.NotEmpty(t, got)
	// Verify ANSI was stripped from selected text.
	assert.NotContains(t, got, "\x1b")
}

// ---------------------------------------------------------------------------
// 4. Data handling — Unicode edge cases
// ---------------------------------------------------------------------------

func TestSecurity_Unicode_MultiByte_CJK(t *testing.T) {
	p := newTestPreview([]string{"Hello\u4e16\u754c"}) // 世界
	p.selAnchor = &selPoint{Line: 0, Col: 5}
	p.selEnd = &selPoint{Line: 0, Col: 7}
	got := p.selectedText()
	assert.Equal(t, "\u4e16\u754c", got)
}

func TestSecurity_Unicode_Emoji(t *testing.T) {
	p := newTestPreview([]string{"Go \U0001f680 rocks"})
	p.selAnchor = &selPoint{Line: 0, Col: 3}
	p.selEnd = &selPoint{Line: 0, Col: 4}
	got := p.selectedText()
	assert.Equal(t, "\U0001f680", got, "should select single emoji codepoint")
}

func TestSecurity_Unicode_CombiningChars(t *testing.T) {
	// e + combining acute accent = e followed by \u0301
	p := newTestPreview([]string{"caf\u0065\u0301"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	// With Go's rune model, combining char is a separate rune.
	assert.Equal(t, "caf\u0065\u0301", got)
}

func TestSecurity_Unicode_ZeroWidthChars(t *testing.T) {
	// Zero-width space and zero-width joiner.
	p := newTestPreview([]string{"a\u200bb\u200dc"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "a\u200bb\u200dc", got)
}

func TestSecurity_Unicode_RTL(t *testing.T) {
	// Right-to-left text should be selected correctly.
	p := newTestPreview([]string{"\u0645\u0631\u062d\u0628\u0627"}) // مرحبا
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}
	got := p.selectedText()
	assert.Equal(t, "\u0645\u0631\u062d\u0628\u0627", got)
}

func TestSecurity_BinaryContentInLine(t *testing.T) {
	// Lines with null bytes and control chars (not flagged as binary mode).
	p := newTestPreview([]string{"abc\x00\x01\x02def"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 9}
	got := p.selectedText()
	// Must not panic; should contain all runes.
	assert.Equal(t, 9, len([]rune(got)))
}

func TestSecurity_Highlight_Unicode_MultiByte(t *testing.T) {
	line := "Hello\u4e16\u754c!"
	sel := &selPoint{Line: 0, Col: 5}
	selE := &selPoint{Line: 0, Col: 7}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	stripped := ansi.Strip(got)
	assert.Equal(t, "Hello\u4e16\u754c!", stripped)
	assert.NotEqual(t, line, got, "should have highlight ANSI codes")
}

func TestSecurity_DoubleClick_Unicode(t *testing.T) {
	p := newTestPreview([]string{"hello \u4e16\u754c foo"})
	// Click on the CJK character at rune position 6.
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 6})
	// CJK chars are not letter/digit/underscore in Go's unicode.IsLetter
	// Actually, CJK ARE letters in Go. So "世界" should be selected as a word.
	require.NotNil(t, p.selAnchor)
	got := p.selectedText()
	assert.NotEmpty(t, got)
}

// ---------------------------------------------------------------------------
// 5. State machine — invalid transitions
// ---------------------------------------------------------------------------

func TestSecurity_ReleaseWithoutClick(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	// Release event with no prior click — must not panic.
	p.Update(panels.PanelMouseReleaseMsg{ContentRow: 0, ContentCol: 3})
	assert.False(t, p.selecting)
	assert.Nil(t, p.selAnchor)
}

func TestSecurity_MotionWithoutAnchor(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.selecting = true
	p.selAnchor = nil // inconsistent state: selecting but no anchor
	p.Update(panels.PanelMouseMotionMsg{ContentRow: 0, ContentCol: 3})
	// handleMouseMotion guards on selAnchor == nil.
	assert.Nil(t, p.selEnd)
}

func TestSecurity_DoubleClickBeyondContent(t *testing.T) {
	p := newTestPreview([]string{"short"})
	// Double-click at a column well beyond the line length.
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 100})
	// Code checks col >= len(runes) and returns early.
	assert.Nil(t, p.selAnchor)
}

func TestSecurity_DoubleClickOnEmptyLine(t *testing.T) {
	p := newTestPreview([]string{""})
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 0})
	// Empty line has 0 runes; col >= len(runes) returns early.
	assert.Nil(t, p.selAnchor)
}

func TestSecurity_DoubleClickRowBeyondContent(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 50, ContentCol: 0})
	assert.Nil(t, p.selAnchor)
}

func TestSecurity_ClickThenCopyThenMotion(t *testing.T) {
	// After copy clears selection, motion events must be no-ops.
	p := newTestPreview([]string{"hello world"})
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 0})
	p.Update(panels.PanelMouseMotionMsg{ContentRow: 0, ContentCol: 5})
	p.Update(panels.PanelMouseReleaseMsg{ContentRow: 0, ContentCol: 5})

	// Now copy (clears selection).
	p.Update(keyMsg("y"))
	assert.Nil(t, p.selAnchor)
	assert.False(t, p.selecting)

	// Motion after copy must not crash or create partial state.
	p.Update(panels.PanelMouseMotionMsg{ContentRow: 0, ContentCol: 3})
	assert.Nil(t, p.selEnd)
}

func TestSecurity_MultipleClicksOverwrite(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	// First click.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 0})
	require.NotNil(t, p.selAnchor)
	assert.Equal(t, 0, p.selAnchor.Col)
	// Second click without release — new anchor must overwrite.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 5, p.selAnchor.Col)
}

func TestSecurity_ClickOnLargeMode(t *testing.T) {
	p := newTestPreview(nil)
	p.isLarge = true
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 0})
	assert.Nil(t, p.selAnchor, "large file mode must block selection")
}

func TestSecurity_DoubleClickOnLargeMode(t *testing.T) {
	p := newTestPreview(nil)
	p.isLarge = true
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 0})
	assert.Nil(t, p.selAnchor, "large file mode must block double-click selection")
}

func TestSecurity_BlameMode_ClickBlocked(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "abc1234", Author: "test", Date: time.Now(), Content: "hello", LineNo: 1},
	}
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 0})
	assert.Nil(t, p.selAnchor, "blame mode with blameLines must block click selection")
}

// ---------------------------------------------------------------------------
// 6. Edge cases in applySelectionHighlight
// ---------------------------------------------------------------------------

func TestSecurity_Highlight_EmptyLine(t *testing.T) {
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 2, Col: 5}
	// Middle line in selection is empty.
	got := _selP.applySelectionHighlight("", 1, sel, selE)
	// Empty string highlighted — should not panic.
	stripped := ansi.Strip(got)
	assert.Equal(t, "", stripped)
}

func TestSecurity_Highlight_StartColEqualsEndCol(t *testing.T) {
	line := "hello"
	sel := &selPoint{Line: 0, Col: 3}
	selE := &selPoint{Line: 0, Col: 3}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	// Zero-width selection — no highlight applied.
	assert.Equal(t, line, got)
}

func TestSecurity_Highlight_StartBeyondLine(t *testing.T) {
	line := "hi"
	sel := &selPoint{Line: 0, Col: 100}
	selE := &selPoint{Line: 0, Col: 200}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	// startCol >= runeCount — returns unchanged line.
	assert.Equal(t, line, got)
}

func TestSecurity_Highlight_OnlyANSI(t *testing.T) {
	// Line that is entirely ANSI sequences with no visible text.
	line := "\x1b[31m\x1b[0m"
	sel := &selPoint{Line: 0, Col: 0}
	selE := &selPoint{Line: 0, Col: 5}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	// runeCount = 0, so startCol >= runeCount. Returns unchanged.
	assert.Equal(t, line, got)
}

// ---------------------------------------------------------------------------
// 7. displayLines integration — diff/combined mode
// ---------------------------------------------------------------------------

func TestSecurity_SelectedText_DiffMode(t *testing.T) {
	p := newTestPreview([]string{"file content"})
	p.diffLines = []string{"+added line", "-removed line"}
	p.diffMode = true
	// In diff mode, displayLines returns diffLines only.
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 11}
	got := p.selectedText()
	assert.Equal(t, "+added line", got)
}

func TestSecurity_SelectedText_FileMode(t *testing.T) {
	p := newTestPreview([]string{"file content"})
	p.diffLines = []string{"+added"}
	p.diffMode = false
	// File mode: displayLines returns file lines only.
	dl := p.displayLines()
	assert.Equal(t, []string{"file content"}, dl)
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 12}
	got := p.selectedText()
	assert.Equal(t, "file content", got)
}

// ---------------------------------------------------------------------------
// 8. Helper function edge cases
// ---------------------------------------------------------------------------

func TestSecurity_NumDigits_NegativeInput(t *testing.T) {
	// numDigits with negative input — code returns 1 for n <= 0.
	assert.Equal(t, 1, numDigits(-1))
	assert.Equal(t, 1, numDigits(-1000))
}

func TestSecurity_Pluralize_Zero(t *testing.T) {
	assert.Equal(t, "0 chars", pluralize(0, "char"))
}

func TestSecurity_Pluralize_NegativeCount(t *testing.T) {
	got := pluralize(-1, "char")
	assert.Equal(t, "-1 chars", got)
}

func TestWordWrapBlocksSelection(t *testing.T) {
	p := &Preview{
		lines:    []string{"hello world this is a long line"},
		filePath: "test.txt",
		wordWrap: true,
	}
	msg := panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5}
	p.handleMouseClick(msg)
	assert.False(t, p.hasSelection(), "selection should be blocked when word wrap is active")
	assert.Nil(t, p.selAnchor)
}

func TestWordWrapBlocksDoubleClick(t *testing.T) {
	p := &Preview{
		lines:    []string{"hello world"},
		filePath: "test.txt",
		wordWrap: true,
	}
	msg := panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 2}
	p.handleDoubleClick(msg)
	assert.False(t, p.hasSelection(), "double-click should be blocked when word wrap is active")
	assert.Nil(t, p.selAnchor)
}

func TestSelectionDisabled(t *testing.T) {
	tests := []struct {
		name string
		p    *Preview
		want bool
	}{
		{"normal", &Preview{}, false},
		{"binary", &Preview{isBinary: true}, true},
		{"large", &Preview{isLarge: true}, true},
		{"wordWrap", &Preview{wordWrap: true}, true},
		{"blame", &Preview{blameMode: true, blameLines: []git.BlameLine{{}}}, true},
		{"blame_no_lines", &Preview{blameMode: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.p.selectionDisabled())
		})
	}
}

func TestContentRowColToAbs_BlameMode_NilDisplayLines(t *testing.T) {
	p := &Preview{
		blameMode:  true,
		blameLines: []git.BlameLine{{}},
		lines:      []string{"hello"},
		filePath:   "test.txt",
	}
	// displayLines() returns nil in blame mode with blameLines set.
	// When dl is nil, runeCol clamping is skipped so col passes through.
	pt := p.contentRowColToAbs(0, 5)
	assert.Equal(t, 0, pt.Line)
	assert.Equal(t, 5, pt.Col)
}

func TestSelectedText_SelectionEndBeyondFile(t *testing.T) {
	p := newTestPreview([]string{"line one", "line two"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 100, Col: 50} // way beyond content
	got := p.selectedText()
	// Loop iterates through both lines; no trailing newline when selection
	// extends past the end of the content.
	assert.Equal(t, "line one\nline two", got)
}

func TestCopySelection_EmptyStringWithValidAnchors(t *testing.T) {
	p := newTestPreview([]string{"hi"})
	p.selAnchor = &selPoint{Line: 0, Col: 5} // beyond line
	p.selEnd = &selPoint{Line: 0, Col: 5}    // same point = empty
	_, cmd := p.copySelection()
	assert.Nil(t, cmd, "copy with empty text should return nil cmd")
	// selAnchor is NOT cleared because copySelection returns early on empty text.
	assert.NotNil(t, p.selAnchor)
}

func TestApplySelectionHighlight_BothGuardConditions(t *testing.T) {
	line := "hi"
	// startCol > endCol AND startCol > runeCount
	sel := &selPoint{Line: 0, Col: 200}
	selE := &selPoint{Line: 0, Col: 100}
	got := _selP.applySelectionHighlight(line, 0, sel, selE)
	assert.Equal(t, line, got, "should return original line when start > end and > runeCount")
}

func TestSelectedText_DiffMode_MultipleLines(t *testing.T) {
	p := newTestPreview([]string{"file line"})
	p.diffLines = []string{"+diff line 1", "+diff line 2", "-removed"}
	p.diffMode = true

	dl := p.displayLines()
	require.Len(t, dl, 3)

	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 2, Col: 4}
	got := p.selectedText()
	assert.Contains(t, got, "+diff line 1")
	assert.Contains(t, got, "-rem")
}

// ---------------------------------------------------------------------------
// SelectionCopier interface
// ---------------------------------------------------------------------------

func TestPreviewImplementsSelectionCopier(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	var sc panels.SelectionCopier = p // compile-time interface check
	assert.NotNil(t, sc)
}

func TestHasSelection_ExportedDelegatesToInternal(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	assert.False(t, p.HasSelection())

	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 3}
	assert.True(t, p.HasSelection())
}

func TestCopySelection_ExportedDelegatesToInternal(t *testing.T) {
	p := newTestPreview([]string{"hello world"})
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	panel, cmd := p.CopySelection()
	assert.Equal(t, p, panel)
	// copySelection clears the selection after copying.
	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
	// cmd is non-nil (clipboard operation).
	assert.NotNil(t, cmd)
}

func TestCopySelection_ExportedNoSelection(t *testing.T) {
	p := newTestPreview([]string{"hello"})
	panel, cmd := p.CopySelection()
	assert.Equal(t, p, panel)
	assert.Nil(t, cmd)
}
