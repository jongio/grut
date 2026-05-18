package preview

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// highlightLine
// ---------------------------------------------------------------------------

func TestHighlightLine_GoCode(t *testing.T) {
	line := `func main() { fmt.Println("hello") }`
	result := highlightLine(line, "main.go", "dracula")
	// Should contain ANSI escape sequences for syntax highlighting.
	assert.NotEmpty(t, result)
	// Stripped output should match the original text.
	assert.Equal(t, line, ansi.Strip(result))
}

func TestHighlightLine_EmptyString(t *testing.T) {
	result := highlightLine("", "main.go", "dracula")
	assert.Equal(t, "", result)
}

func TestHighlightLine_UnknownFileType(t *testing.T) {
	line := "some random text"
	result := highlightLine(line, "file.unknown_ext_xyz", "dracula")
	// No lexer matched — should return raw text.
	assert.Equal(t, line, result)
}

func TestHighlightLine_NilThemeFallback(t *testing.T) {
	line := "package main"
	result := highlightLine(line, "main.go", "nonexistent_theme")
	// Should still produce output (falls back to Fallback style).
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "package")
}

// ---------------------------------------------------------------------------
// renderCursorOnLine
// ---------------------------------------------------------------------------

func TestRenderCursorOnLine_AtStart(t *testing.T) {
	raw := "hello"
	highlighted := raw // no ANSI — plain text
	cursorStyle := lipgloss.NewStyle().Reverse(true)

	result := renderCursorOnLine(highlighted, raw, 0, cursorStyle)
	stripped := ansi.Strip(result)
	// The cursor character 'h' should still be present in stripped output.
	assert.Contains(t, stripped, "h")
	// The full stripped text should equal the original.
	assert.Equal(t, raw, stripped)
}

func TestRenderCursorOnLine_AtMiddle(t *testing.T) {
	raw := "hello"
	highlighted := raw
	cursorStyle := lipgloss.NewStyle().Reverse(true)

	result := renderCursorOnLine(highlighted, raw, 2, cursorStyle)
	stripped := ansi.Strip(result)
	assert.Equal(t, raw, stripped)
}

func TestRenderCursorOnLine_AtEnd(t *testing.T) {
	raw := "hello"
	highlighted := raw
	cursorStyle := lipgloss.NewStyle().Reverse(true)

	result := renderCursorOnLine(highlighted, raw, 5, cursorStyle)
	stripped := ansi.Strip(result)
	// Cursor past end of line — should append a block cursor space.
	assert.True(t, strings.HasSuffix(stripped, " "))
	assert.True(t, strings.HasPrefix(stripped, "hello"))
}

func TestRenderCursorOnLine_EmptyLine(t *testing.T) {
	raw := ""
	highlighted := ""
	cursorStyle := lipgloss.NewStyle().Reverse(true)

	result := renderCursorOnLine(highlighted, raw, 0, cursorStyle)
	stripped := ansi.Strip(result)
	assert.Equal(t, " ", stripped)
}

func TestRenderCursorOnLine_WithANSI(t *testing.T) {
	raw := "ab"
	// Simulate ANSI: \x1b[31m a \x1b[0m b
	highlighted := "\x1b[31ma\x1b[0mb"
	cursorStyle := lipgloss.NewStyle().Reverse(true)

	result := renderCursorOnLine(highlighted, raw, 0, cursorStyle)
	stripped := ansi.Strip(result)
	assert.Equal(t, "ab", stripped)
}

// ---------------------------------------------------------------------------
// renderEditStatusBar
// ---------------------------------------------------------------------------

func TestRenderEditStatusBar_Basic(t *testing.T) {
	p := &Preview{
		cursorLine: 4,
		cursorCol:  10,
		editCfg:    config.EditorConfig{TabSize: 4},
	}

	bar := renderEditStatusBar(p, 80, 100)
	stripped := ansi.Strip(bar)
	assert.Contains(t, stripped, "Ln 5")
	assert.Contains(t, stripped, "Col 11")
	assert.Contains(t, stripped, "Tab: 4")
	assert.Contains(t, stripped, "100 lines")
}

func TestRenderEditStatusBar_DirtyIndicator(t *testing.T) {
	buf := NewTextBuffer([]string{"original"})
	buf.InsertRune(0, 0, 'x')

	p := &Preview{
		cursorLine: 0,
		cursorCol:  0,
		editBuf:    buf,
		editCfg:    config.EditorConfig{TabSize: 2},
	}

	bar := renderEditStatusBar(p, 80, 10)
	stripped := ansi.Strip(bar)
	assert.Contains(t, stripped, "[+]")
}

func TestRenderEditStatusBar_CleanBuffer(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})

	p := &Preview{
		cursorLine: 0,
		cursorCol:  0,
		editBuf:    buf,
		editCfg:    config.EditorConfig{TabSize: 4},
	}

	bar := renderEditStatusBar(p, 80, 1)
	stripped := ansi.Strip(bar)
	assert.NotContains(t, stripped, "[+]")
}

func TestRenderEditStatusBar_ZeroTabSize(t *testing.T) {
	p := &Preview{
		editCfg: config.EditorConfig{TabSize: 0},
	}
	bar := renderEditStatusBar(p, 60, 5)
	stripped := ansi.Strip(bar)
	// Should default to 4 when TabSize is 0.
	assert.Contains(t, stripped, "Tab: 4")
}

// ---------------------------------------------------------------------------
// renderEditContent (smoke test)
// ---------------------------------------------------------------------------

func TestRenderEditContent_NilBuffer(t *testing.T) {
	p := &Preview{editMode: true, editBuf: nil}
	result := renderEditContent(p, 80, 24)
	assert.Equal(t, "", result)
}

func TestRenderEditContent_BasicSmoke(t *testing.T) {
	buf := NewTextBuffer([]string{"line one", "line two", "line three"})

	p := &Preview{
		editMode:   true,
		editBuf:    buf,
		cursorLine: 0,
		cursorCol:  0,
		cfg:        defaultCfg(),
		editCfg:    config.EditorConfig{TabSize: 4},
		filePath:   "test.go",
	}

	result := renderEditContent(p, 60, 10)
	require.NotEmpty(t, result)

	stripped := ansi.Strip(result)
	// Should contain line numbers.
	assert.Contains(t, stripped, "1")
	assert.Contains(t, stripped, "2")
	assert.Contains(t, stripped, "3")
	// Should contain the text content.
	assert.Contains(t, stripped, "line one")
	assert.Contains(t, stripped, "line two")
	// Should contain a status bar with cursor info.
	assert.Contains(t, stripped, "Ln 1")
	assert.Contains(t, stripped, "Col 1")
}

func TestRenderEditContent_CursorOnLastLine(t *testing.T) {
	buf := NewTextBuffer([]string{"a", "b", "c"})

	p := &Preview{
		editMode:   true,
		editBuf:    buf,
		cursorLine: 2,
		cursorCol:  1,
		cfg:        defaultCfg(),
		editCfg:    config.EditorConfig{TabSize: 4},
		filePath:   "test.txt",
	}

	result := renderEditContent(p, 40, 10)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Ln 3")
	assert.Contains(t, stripped, "Col 2")
}

func TestRenderEditContent_SmallHeight(t *testing.T) {
	buf := NewTextBuffer([]string{"line 1", "line 2", "line 3", "line 4", "line 5"})

	p := &Preview{
		editMode:   true,
		editBuf:    buf,
		cursorLine: 0,
		cursorCol:  0,
		cfg:        defaultCfg(),
		editCfg:    config.EditorConfig{TabSize: 4},
	}

	// height=2 means 1 line for content, 1 for status bar.
	result := renderEditContent(p, 40, 2)
	require.NotEmpty(t, result)
	lines := strings.Split(result, "\n")
	// Should have exactly 2 lines: 1 content + 1 status bar.
	assert.Equal(t, 2, len(lines))
}

// ---------------------------------------------------------------------------
// clampEditScroll
// ---------------------------------------------------------------------------

func TestClampEditScroll_NoOverflow(t *testing.T) {
	p := &Preview{scrollY: 0}
	p.clampEditScroll(100, 20)
	assert.Equal(t, 0, p.scrollY)
}

func TestClampEditScroll_ClampsMax(t *testing.T) {
	p := &Preview{scrollY: 90}
	p.clampEditScroll(100, 20)
	assert.Equal(t, 80, p.scrollY)
}

func TestClampEditScroll_ClampsNegative(t *testing.T) {
	p := &Preview{scrollY: -5}
	p.clampEditScroll(100, 20)
	assert.Equal(t, 0, p.scrollY)
}

func TestClampEditScroll_ContentTallerThanLines(t *testing.T) {
	p := &Preview{scrollY: 5}
	// Only 3 lines but viewport can show 10.
	p.clampEditScroll(3, 10)
	assert.Equal(t, 0, p.scrollY)
}

// ---------------------------------------------------------------------------
// Title edit-mode indicators
// ---------------------------------------------------------------------------

func TestTitle_EditMode(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "/tmp/test.go"
	p.editMode = true

	assert.Equal(t, "test.go [edit]", p.Title())
}

func TestTitle_EditModeDirty(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})
	buf.InsertRune(0, 0, 'x')

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "/tmp/test.go"
	p.editMode = true
	p.editBuf = buf

	assert.Equal(t, "test.go [+]", p.Title())
}

func TestTitle_EditModeClean(t *testing.T) {
	buf := NewTextBuffer([]string{"hello"})

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "/tmp/test.go"
	p.editMode = true
	p.editBuf = buf

	assert.Equal(t, "test.go [edit]", p.Title())
}

// ---------------------------------------------------------------------------
// KeyBindings edit-mode vs read-mode
// ---------------------------------------------------------------------------

func TestKeyBindings_ReadMode(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	bindings := p.KeyBindings()

	// Read mode should include "e" for edit and standard navigation.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["e"], "read mode should have 'e' binding")
	assert.True(t, keys["j/↓"], "read mode should have scroll bindings")
}

func TestKeyBindings_EditMode(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.editMode = true
	bindings := p.KeyBindings()

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["Esc"], "edit mode should have Esc binding")
	assert.True(t, keys["Ctrl+S"], "edit mode should have save binding")
	assert.True(t, keys["Ctrl+Z"], "edit mode should have undo binding")
	assert.False(t, keys["j/↓"], "edit mode should not have read-mode scroll bindings")
}

// ---------------------------------------------------------------------------
// renderSelectionOnLine
// ---------------------------------------------------------------------------

func TestRenderSelectionOnLine_FullLine(t *testing.T) {
	raw := "hello"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 0}
	end := &selPoint{Line: 0, Col: 5}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "hello", stripped)
	// Should contain styling (result differs from plain highlighted).
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_PartialMiddle(t *testing.T) {
	raw := "hello world"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 2}
	end := &selPoint{Line: 0, Col: 7}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "hello world", stripped)
	// Before = "he", selected = "llo w", after = "orld"
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_StartColEqualsEnd_NoChange(t *testing.T) {
	raw := "abc"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#FF0000"))
	start := &selPoint{Line: 0, Col: 2}
	end := &selPoint{Line: 0, Col: 2}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	assert.Equal(t, highlighted, result, "no selection when start==end")
}

func TestRenderSelectionOnLine_StartBeyondLineLen(t *testing.T) {
	raw := "abc"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#FF0000"))
	start := &selPoint{Line: 0, Col: 10}
	end := &selPoint{Line: 0, Col: 12}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	assert.Equal(t, highlighted, result, "selection beyond line returns unchanged")
}

func TestRenderSelectionOnLine_MultiLineFirstLine(t *testing.T) {
	// Selection spans lines 0-2; on line 0 it should select from col 3 to end.
	raw := "hello world"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 3}
	end := &selPoint{Line: 2, Col: 5}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "hello world", stripped)
	// startCol=3, endCol=lineLen (since lineIdx != selEnd.Line)
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_MultiLineMiddleLine(t *testing.T) {
	// Middle line: entire line selected (startCol=0, endCol=lineLen).
	raw := "middle line"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 3}
	end := &selPoint{Line: 2, Col: 5}

	result := renderSelectionOnLine(highlighted, raw, 1, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "middle line", stripped)
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_MultiLineLastLine(t *testing.T) {
	// Last line: from col 0 to selEnd.Col.
	raw := "last line"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 3}
	end := &selPoint{Line: 2, Col: 4}

	result := renderSelectionOnLine(highlighted, raw, 2, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "last line", stripped)
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_WithANSIEscapes(t *testing.T) {
	raw := "hello"
	// Simulate syntax-highlighted text with ANSI codes.
	highlighted := "\x1b[32mhel\x1b[0mlo"
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 1}
	end := &selPoint{Line: 0, Col: 4}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "hello", stripped)
	// ANSI before selection "h" = before segment, selected "ell" rendered through selStyle.
	assert.NotEqual(t, highlighted, result)
}

func TestRenderSelectionOnLine_EmptyLine(t *testing.T) {
	raw := ""
	highlighted := ""
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#FF0000"))
	start := &selPoint{Line: 0, Col: 0}
	end := &selPoint{Line: 0, Col: 0}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	assert.Equal(t, "", result)
}

func TestRenderSelectionOnLine_EndExceedsLineClamped(t *testing.T) {
	raw := "abc"
	highlighted := raw
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#0000FF"))
	start := &selPoint{Line: 0, Col: 1}
	end := &selPoint{Line: 0, Col: 99}

	result := renderSelectionOnLine(highlighted, raw, 0, start, end, selStyle)

	stripped := ansi.Strip(result)
	assert.Equal(t, "abc", stripped)
	// Should select from col 1 to end (clamped to 3).
	assert.NotEqual(t, highlighted, result)
	assert.True(t, strings.Contains(result, "a"), "before segment has 'a'")
}
