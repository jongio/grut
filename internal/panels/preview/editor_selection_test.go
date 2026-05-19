package preview

import (
	"testing"

	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// editSelRange
// ---------------------------------------------------------------------------

func TestEditSelRange_Normalized(t *testing.T) {
	p := testPreview()
	// Set anchor AFTER end so the function must swap them.
	p.selAnchor = &selPoint{Line: 5, Col: 10}
	p.selEnd = &selPoint{Line: 2, Col: 3}

	s, e := editSelRange(p)

	assert.NotNil(t, s)
	assert.NotNil(t, e)
	assert.Equal(t, 2, s.Line)
	assert.Equal(t, 3, s.Col)
	assert.Equal(t, 5, e.Line)
	assert.Equal(t, 10, e.Col)
}

func TestEditSelRange_Normalized_SameLine(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 3, Col: 8}
	p.selEnd = &selPoint{Line: 3, Col: 2}

	s, e := editSelRange(p)

	assert.Equal(t, 2, s.Col)
	assert.Equal(t, 8, e.Col)
}

func TestEditSelRange_Nil(t *testing.T) {
	p := testPreview()
	// No selection set.
	s, e := editSelRange(p)
	assert.Nil(t, s)
	assert.Nil(t, e)
}

// ---------------------------------------------------------------------------
// hasEditSelection
// ---------------------------------------------------------------------------

func TestHasEditSelection_True(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	assert.True(t, hasEditSelection(p))
}

func TestHasEditSelection_True_DifferentLines(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 1, Col: 0}

	assert.True(t, hasEditSelection(p))
}

func TestHasEditSelection_False_Same(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 2, Col: 4}
	p.selEnd = &selPoint{Line: 2, Col: 4}

	assert.False(t, hasEditSelection(p))
}

func TestHasEditSelection_False_Nil(t *testing.T) {
	p := testPreview()
	assert.False(t, hasEditSelection(p))
}

// ---------------------------------------------------------------------------
// clearEditSelection
// ---------------------------------------------------------------------------

func TestClearEditSelection(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 1, Col: 2}
	p.selEnd = &selPoint{Line: 3, Col: 4}

	clearEditSelection(p)

	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
}

// ---------------------------------------------------------------------------
// editSelectedText
// ---------------------------------------------------------------------------

func TestEditSelectedText_SingleLine(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz"})
	p.editMode = true
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	got := editSelectedText(p)
	assert.Equal(t, "hello", got)
}

func TestEditSelectedText_SingleLine_Middle(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world"})
	p.editMode = true
	p.selAnchor = &selPoint{Line: 0, Col: 6}
	p.selEnd = &selPoint{Line: 0, Col: 11}

	got := editSelectedText(p)
	assert.Equal(t, "world", got)
}

func TestEditSelectedText_MultiLine(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz", "end"})
	p.editMode = true
	p.selAnchor = &selPoint{Line: 0, Col: 6}
	p.selEnd = &selPoint{Line: 2, Col: 3}

	got := editSelectedText(p)
	assert.Equal(t, "world\nfoo bar baz\nend", got)
}

func TestEditSelectedText_MultiLine_Partial(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz", "end line"})
	p.editMode = true
	p.selAnchor = &selPoint{Line: 0, Col: 6}
	p.selEnd = &selPoint{Line: 1, Col: 3}

	got := editSelectedText(p)
	assert.Equal(t, "world\nfoo", got)
}

func TestEditSelectedText_Empty(t *testing.T) {
	p := testPreview()
	// No selection, no buffer.
	got := editSelectedText(p)
	assert.Equal(t, "", got)
}

func TestEditSelectedText_NoBuffer(t *testing.T) {
	p := testPreview()
	p.selAnchor = &selPoint{Line: 0, Col: 0}
	p.selEnd = &selPoint{Line: 0, Col: 5}

	got := editSelectedText(p)
	assert.Equal(t, "", got)
}

func TestEditSelectedText_ReversedAnchors(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"abcdefghij"})
	p.editMode = true
	// Anchor after end - editSelRange normalises.
	p.selAnchor = &selPoint{Line: 0, Col: 7}
	p.selEnd = &selPoint{Line: 0, Col: 2}

	got := editSelectedText(p)
	assert.Equal(t, "cdefg", got)
}

// ---------------------------------------------------------------------------
// extendEditSelection
// ---------------------------------------------------------------------------

func TestExtendEditSelection_FirstPress(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello", "world"})
	p.editMode = true
	p.cursorLine = 0
	p.cursorCol = 3

	// No existing anchor - extend should set anchor at cursor.
	extendEditSelection(p, 1, 2)

	assert.NotNil(t, p.selAnchor)
	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 3, p.selAnchor.Col)

	assert.NotNil(t, p.selEnd)
	assert.Equal(t, 1, p.selEnd.Line)
	assert.Equal(t, 2, p.selEnd.Col)
}

func TestExtendEditSelection_Subsequent(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello", "world"})
	p.editMode = true
	p.cursorLine = 0
	p.cursorCol = 3

	// First press sets anchor.
	extendEditSelection(p, 0, 5)

	// Second press only moves end; anchor stays.
	extendEditSelection(p, 1, 4)

	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 3, p.selAnchor.Col)
	assert.Equal(t, 1, p.selEnd.Line)
	assert.Equal(t, 4, p.selEnd.Col)
}

// ---------------------------------------------------------------------------
// selectAll
// ---------------------------------------------------------------------------

func TestSelectAll(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz", "end"})
	p.editMode = true

	selectAll(p)

	assert.NotNil(t, p.selAnchor)
	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 0, p.selAnchor.Col)

	assert.NotNil(t, p.selEnd)
	assert.Equal(t, 2, p.selEnd.Line)
	assert.Equal(t, 3, p.selEnd.Col) // len("end") == 3
}

func TestSelectAll_SingleLine(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello"})
	p.editMode = true

	selectAll(p)

	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 0, p.selAnchor.Col)
	assert.Equal(t, 0, p.selEnd.Line)
	assert.Equal(t, 5, p.selEnd.Col)
}

func TestSelectAll_NilBuffer(t *testing.T) {
	p := testPreview()
	selectAll(p)
	// Should not panic; no selection set.
	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
}

// ---------------------------------------------------------------------------
// findWordBoundaryLeft
// ---------------------------------------------------------------------------

func TestFindWordBoundaryLeft(t *testing.T) {
	tests := []struct {
		name string
		line string
		col  int
		want int
	}{
		{"middle of word", "hello world", 8, 6},
		{"start of second word", "hello world", 6, 0},
		{"end of line", "hello world", 11, 6},
		{"at start", "hello world", 0, 0},
		{"between words spaces", "foo   bar", 6, 0},
		{"at start of first word", "hello", 3, 0},
		{"after punctuation", "foo.bar", 7, 4},
		{"empty line", "", 0, 0},
		{"col beyond end", "abc", 10, 0},
		{"underscored word", "foo_bar baz", 8, 8}, // at 'b' of baz, skip non-word (space), skip word -> 8? No.
	}
	// Fix: "underscored word" - col 8 is 'b' in "baz". Left: skip non-word=' '->7, skip word 'foo_bar'->0.
	tests[len(tests)-1].want = 0

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findWordBoundaryLeft(tt.line, tt.col)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindWordBoundaryLeft_OnlySpaces(t *testing.T) {
	got := findWordBoundaryLeft("     ", 3)
	assert.Equal(t, 0, got)
}

// ---------------------------------------------------------------------------
// findWordBoundaryRight
// ---------------------------------------------------------------------------

func TestFindWordBoundaryRight(t *testing.T) {
	tests := []struct {
		name string
		line string
		col  int
		want int
	}{
		{"middle of word", "hello world", 2, 6},
		{"start of word", "hello world", 0, 6},
		{"at space", "hello world", 5, 6},
		{"end of line", "hello world", 11, 11},
		{"between words spaces", "foo   bar", 3, 6},
		{"after last word", "hello", 5, 5},
		{"with punctuation", "foo.bar", 0, 4},
		{"empty line", "", 0, 0},
		{"col beyond end", "abc", 10, 3},
		{"underscored word", "foo_bar baz", 0, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findWordBoundaryRight(tt.line, tt.col)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindWordBoundaryRight_OnlySpaces(t *testing.T) {
	got := findWordBoundaryRight("     ", 0)
	assert.Equal(t, 5, got)
}

// ---------------------------------------------------------------------------
// isWordRune
// ---------------------------------------------------------------------------

func TestIsWordRune(t *testing.T) {
	// True cases.
	assert.True(t, isWordRune('a'))
	assert.True(t, isWordRune('Z'))
	assert.True(t, isWordRune('0'))
	assert.True(t, isWordRune('9'))
	assert.True(t, isWordRune('_'))

	// False cases.
	assert.False(t, isWordRune(' '))
	assert.False(t, isWordRune('.'))
	assert.False(t, isWordRune('-'))
	assert.False(t, isWordRune('!'))
	assert.False(t, isWordRune('\t'))
	assert.False(t, isWordRune('\n'))
}

// ---------------------------------------------------------------------------
// Edit-mode mouse handlers
// ---------------------------------------------------------------------------

func TestHandleEditMouseClick_PositionsCursor(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz"})
	p.editMode = true
	p.lineNumbers = true

	// Click on row 0, col = gutterWidth + 5 → cursorCol should be 5.
	gw := p.editGutterWidth()
	msg := panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: gw + 5}
	p.handleEditMouseClick(msg)

	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 5, p.cursorCol)
	// Selection anchor is set (for potential drag).
	assert.NotNil(t, p.selAnchor)
	assert.True(t, p.selecting)
}

func TestHandleEditMouseClick_SecondLine(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello", "world", "test"})
	p.editMode = true
	p.lineNumbers = true

	gw := p.editGutterWidth()
	msg := panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: gw + 3}
	p.handleEditMouseClick(msg)

	assert.Equal(t, 1, p.cursorLine)
	assert.Equal(t, 3, p.cursorCol)
}

func TestHandleEditMouseDrag_CreatesSelection(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world", "foo bar baz"})
	p.editMode = true
	p.lineNumbers = true
	p.height = 20

	gw := p.editGutterWidth()

	// Click at (0, 2).
	clickMsg := panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: gw + 2}
	p.handleEditMouseClick(clickMsg)

	// Drag to (0, 7).
	motionMsg := panels.PanelMouseMotionMsg{ContentRow: 0, ContentCol: gw + 7}
	p.handleEditMouseMotion(motionMsg)

	assert.Equal(t, 0, p.cursorLine)
	assert.Equal(t, 7, p.cursorCol)
	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 2, p.selAnchor.Col)
	assert.Equal(t, 0, p.selEnd.Line)
	assert.Equal(t, 7, p.selEnd.Col)

	// Release — selection stays because anchor != end.
	releaseMsg := panels.PanelMouseReleaseMsg{ContentRow: 0, ContentCol: gw + 7}
	p.handleEditMouseRelease(releaseMsg)
	assert.False(t, p.selecting)
	assert.NotNil(t, p.selAnchor) // selection persists
}

func TestHandleEditMouseRelease_ClearsOnClick(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world"})
	p.editMode = true
	p.lineNumbers = true

	gw := p.editGutterWidth()

	// Click without drag.
	clickMsg := panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: gw + 3}
	p.handleEditMouseClick(clickMsg)

	// Release at same position — should clear selection.
	releaseMsg := panels.PanelMouseReleaseMsg{ContentRow: 0, ContentCol: gw + 3}
	p.handleEditMouseRelease(releaseMsg)

	assert.Nil(t, p.selAnchor)
	assert.Nil(t, p.selEnd)
}

func TestHandleEditDoubleClick_SelectsWord(t *testing.T) {
	p := testPreview()
	p.editBuf = NewTextBuffer([]string{"hello world"})
	p.editMode = true
	p.lineNumbers = true

	gw := p.editGutterWidth()

	// Double-click on "world" (col 6).
	msg := panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: gw + 6}
	p.handleEditDoubleClick(msg)

	assert.Equal(t, 0, p.selAnchor.Line)
	assert.Equal(t, 6, p.selAnchor.Col) // start of "world"
	assert.Equal(t, 0, p.selEnd.Line)
	assert.Equal(t, 11, p.selEnd.Col) // end of "world"
	assert.Equal(t, 11, p.cursorCol)
}
