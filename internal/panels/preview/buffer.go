package preview

import (
	"slices"
	"time"
)

// nowNano returns the current time in nanoseconds.
// Replaced in tests for deterministic undo coalescing.
var nowNano = func() int64 { return time.Now().UnixNano() }

const (
	maxUndoDepth     = 100
	coalesceWindowNs = 500_000_000 // 500ms in nanoseconds
)

// TextBuffer is a line-oriented editable text buffer with undo/redo support.
// It stores raw text lines (no ANSI) for in-place editing within the preview panel.
type TextBuffer struct {
	lines        []string
	dirty        bool
	undoStack    []bufferSnapshot
	redoStack    []bufferSnapshot
	lastEditNano int64 // time.Now().UnixNano() of last edit
}

type bufferSnapshot struct {
	lines      []string
	cursorLine int
	cursorCol  int
}

// NewTextBuffer creates a buffer from the given lines, deep-copying the input.
// An empty or nil input produces a single empty line.
func NewTextBuffer(lines []string) *TextBuffer {
	if len(lines) == 0 {
		lines = []string{""}
	}
	return &TextBuffer{lines: copyLines(lines)}
}

// Lines returns the current lines (read-only view).
func (b *TextBuffer) Lines() []string { return b.lines }

// LineCount returns the number of lines in the buffer.
func (b *TextBuffer) LineCount() int { return len(b.lines) }

// Line returns line n. Returns "" if n is out of range.
func (b *TextBuffer) Line(n int) string {
	if n < 0 || n >= len(b.lines) {
		return ""
	}
	return b.lines[n]
}

// SetLines replaces all content, marks the buffer dirty, and clears undo/redo.
func (b *TextBuffer) SetLines(lines []string) {
	if len(lines) == 0 {
		lines = []string{""}
	}
	b.lines = copyLines(lines)
	b.dirty = true
	b.undoStack = nil
	b.redoStack = nil
	b.lastEditNano = 0
}

// Dirty reports whether the buffer has unsaved changes.
func (b *TextBuffer) Dirty() bool { return b.dirty }

// MarkClean clears the dirty flag, typically called after a successful save.
func (b *TextBuffer) MarkClean() { b.dirty = false }

// InsertRune inserts a rune at (line, col). Out-of-bounds values are clamped.
// Undo snapshots are coalesced for rapid consecutive inserts.
func (b *TextBuffer) InsertRune(line, col int, r rune) {
	line = b.clampLine(line)
	col = b.clampCol(line, col)
	b.saveSnapshot(line, col)
	runes := []rune(b.lines[line])
	runes = slices.Insert(runes, col, r)
	b.lines[line] = string(runes)
	b.dirty = true
}

// DeleteRune performs backspace at (line, col): deletes the character before
// the cursor. If col==0 and line>0, the current line joins with the previous
// one. Returns the new cursor position.
func (b *TextBuffer) DeleteRune(line, col int) (newLine, newCol int) {
	line = b.clampLine(line)
	col = b.clampCol(line, col)

	if col > 0 {
		b.saveSnapshot(line, col)
		runes := []rune(b.lines[line])
		runes = slices.Delete(runes, col-1, col)
		b.lines[line] = string(runes)
		b.dirty = true
		return line, col - 1
	}

	if line == 0 {
		return 0, 0 // first line, col 0 — nothing to delete
	}

	// col==0, line>0: join with previous line (forces undo break).
	b.saveSnapshotForce(line, col)
	prevLen := len([]rune(b.lines[line-1]))
	b.lines[line-1] += b.lines[line]
	b.lines = slices.Delete(b.lines, line, line+1)
	b.dirty = true
	return line - 1, prevLen
}

// DeleteForward performs the delete-key at (line, col): removes the character
// at the cursor position. If the cursor is at end-of-line, the next line is
// joined onto the current one.
func (b *TextBuffer) DeleteForward(line, col int) {
	line = b.clampLine(line)
	col = b.clampCol(line, col)

	runes := []rune(b.lines[line])
	if col < len(runes) {
		b.saveSnapshot(line, col)
		runes = slices.Delete(runes, col, col+1)
		b.lines[line] = string(runes)
		b.dirty = true
		return
	}

	// At end of line — join with next if one exists.
	if line >= len(b.lines)-1 {
		return // last line EOL — no-op
	}
	b.saveSnapshotForce(line, col)
	b.lines[line] += b.lines[line+1]
	b.lines = slices.Delete(b.lines, line+1, line+2)
	b.dirty = true
}

// SplitLine splits the line at col (Enter key). If autoIndent is true the new
// line inherits the leading whitespace of the current line.
func (b *TextBuffer) SplitLine(line, col int, autoIndent bool) {
	line = b.clampLine(line)
	col = b.clampCol(line, col)

	b.saveSnapshotForce(line, col)

	runes := []rune(b.lines[line])
	before := string(runes[:col])
	after := string(runes[col:])

	if autoIndent {
		after = leadingWhitespace(b.lines[line]) + after
	}

	b.lines[line] = before
	b.lines = slices.Insert(b.lines, line+1, after)
	b.dirty = true
}

// InsertTab inserts spaces to the next tab stop at (line, col) and returns
// the new column. tabSize is clamped to >= 1.
func (b *TextBuffer) InsertTab(line, col int, tabSize int) int {
	if tabSize < 1 {
		tabSize = 4
	}
	line = b.clampLine(line)
	col = b.clampCol(line, col)

	spaces := tabSize - (col % tabSize)
	b.saveSnapshot(line, col)

	runes := []rune(b.lines[line])
	pad := make([]rune, spaces)
	for i := range pad {
		pad[i] = ' '
	}
	runes = slices.Insert(runes, col, pad...)
	b.lines[line] = string(runes)
	b.dirty = true
	return col + spaces
}

// Dedent removes up to tabSize leading spaces from the line.
func (b *TextBuffer) Dedent(line int, tabSize int) {
	if tabSize < 1 {
		tabSize = 4
	}
	line = b.clampLine(line)

	runes := []rune(b.lines[line])
	remove := 0
	for remove < tabSize && remove < len(runes) && runes[remove] == ' ' {
		remove++
	}
	if remove == 0 {
		return
	}
	b.saveSnapshot(line, 0)
	b.lines[line] = string(runes[remove:])
	b.dirty = true
}

// DuplicateLine duplicates the line at the given index, inserting the copy
// below it. Always forces an undo break.
func (b *TextBuffer) DuplicateLine(line int) {
	line = b.clampLine(line)
	b.saveSnapshotForce(line, 0)
	b.lines = slices.Insert(b.lines, line+1, b.lines[line])
	b.dirty = true
}

// Undo restores the previous buffer snapshot. Returns the restored cursor
// position and true, or the original position and false if nothing to undo.
func (b *TextBuffer) Undo(cursorLine, cursorCol int) (newLine, newCol int, ok bool) {
	if len(b.undoStack) == 0 {
		return cursorLine, cursorCol, false
	}
	b.redoStack = append(b.redoStack, bufferSnapshot{
		lines:      copyLines(b.lines),
		cursorLine: cursorLine,
		cursorCol:  cursorCol,
	})
	snap := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]
	b.lines = snap.lines
	b.dirty = true
	b.lastEditNano = 0
	return snap.cursorLine, snap.cursorCol, true
}

// Redo restores the next buffer snapshot. Returns the restored cursor position
// and true, or the original position and false if nothing to redo.
func (b *TextBuffer) Redo(cursorLine, cursorCol int) (newLine, newCol int, ok bool) {
	if len(b.redoStack) == 0 {
		return cursorLine, cursorCol, false
	}
	b.undoStack = append(b.undoStack, bufferSnapshot{
		lines:      copyLines(b.lines),
		cursorLine: cursorLine,
		cursorCol:  cursorCol,
	})
	snap := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]
	b.lines = snap.lines
	b.dirty = true
	b.lastEditNano = 0
	return snap.cursorLine, snap.cursorCol, true
}

// DeleteRange deletes text from (startLine, startCol) to (endLine, endCol).
// The range is [startCol, endCol) on the respective lines. If start == end,
// this is a no-op. Always forces an undo break. Returns the cursor position
// after deletion.
func (b *TextBuffer) DeleteRange(startLine, startCol, endLine, endCol int) (newLine, newCol int) {
	startLine = b.clampLine(startLine)
	startCol = b.clampCol(startLine, startCol)
	endLine = b.clampLine(endLine)
	endCol = b.clampCol(endLine, endCol)

	// Normalize: ensure start <= end.
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}

	// No-op if positions are identical.
	if startLine == endLine && startCol == endCol {
		return startLine, startCol
	}

	b.saveSnapshotForce(startLine, startCol)

	if startLine == endLine {
		// Single-line range: remove runes [startCol, endCol).
		runes := []rune(b.lines[startLine])
		runes = slices.Delete(runes, startCol, endCol)
		b.lines[startLine] = string(runes)
	} else {
		// Multi-line range: keep [0, startCol) of startLine + [endCol, end) of endLine.
		startRunes := []rune(b.lines[startLine])
		endRunes := []rune(b.lines[endLine])
		b.lines[startLine] = string(startRunes[:startCol]) + string(endRunes[endCol:])
		b.lines = slices.Delete(b.lines, startLine+1, endLine+1)
	}

	b.dirty = true
	return startLine, startCol
}

// InsertText inserts a (possibly multi-line) text string at (line, col).
// Used for paste operations. Always forces an undo break. Returns the cursor
// position at the end of the inserted text. Empty text is a no-op.
func (b *TextBuffer) InsertText(line, col int, text string) (newLine, newCol int) {
	line = b.clampLine(line)
	col = b.clampCol(line, col)

	if text == "" {
		return line, col
	}

	b.saveSnapshotForce(line, col)

	runes := []rune(b.lines[line])
	before := string(runes[:col])
	after := string(runes[col:])

	// Split the inserted text on newlines.
	parts := splitLines(text)

	if len(parts) == 1 {
		// Single-line insert.
		b.lines[line] = before + parts[0] + after
		b.dirty = true
		return line, col + len([]rune(parts[0]))
	}

	// Multi-line insert: first part joins with before, last part joins with after.
	b.lines[line] = before + parts[0]
	// Insert middle and last lines.
	newLines := make([]string, len(parts)-1)
	for i := 1; i < len(parts)-1; i++ {
		newLines[i-1] = parts[i]
	}
	newLines[len(newLines)-1] = parts[len(parts)-1] + after
	b.lines = slices.Insert(b.lines, line+1, newLines...)

	b.dirty = true
	lastPart := []rune(parts[len(parts)-1])
	return line + len(parts) - 1, len(lastPart)
}

// DeleteLine deletes the entire line at the given index. If it is the last
// remaining line, the line is cleared to "" instead of being removed (the
// buffer must always contain at least one line). Always forces an undo break.
func (b *TextBuffer) DeleteLine(line int) {
	line = b.clampLine(line)
	b.saveSnapshotForce(line, 0)

	if len(b.lines) == 1 {
		b.lines[0] = ""
	} else {
		b.lines = slices.Delete(b.lines, line, line+1)
	}
	b.dirty = true
}

// MoveLine moves the line at the given index by delta positions (e.g. -1 for
// up, +1 for down). Returns true if the move happened, false if the line is
// already at the boundary. Always forces an undo break when a move occurs.
func (b *TextBuffer) MoveLine(line, delta int) bool {
	line = b.clampLine(line)
	target := line + delta
	if target < 0 || target >= len(b.lines) {
		return false
	}

	b.saveSnapshotForce(line, 0)
	b.lines[line], b.lines[target] = b.lines[target], b.lines[line]
	b.dirty = true
	return true
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// saveSnapshot pushes the current buffer state onto the undo stack and clears
// the redo stack. If the last snapshot was saved within 500ms the push is
// coalesced (skipped) so that rapid typing forms a single undo unit.
func (b *TextBuffer) saveSnapshot(cursorLine, cursorCol int) {
	now := nowNano()
	if len(b.undoStack) > 0 && now-b.lastEditNano < coalesceWindowNs {
		b.lastEditNano = now
		b.redoStack = nil
		return
	}
	b.pushSnapshot(cursorLine, cursorCol)
	b.lastEditNano = now
}

// saveSnapshotForce always pushes a new snapshot regardless of the coalescing
// window. Used for structural changes (SplitLine, line joins, DuplicateLine).
func (b *TextBuffer) saveSnapshotForce(cursorLine, cursorCol int) {
	b.lastEditNano = 0 // break coalescing
	b.pushSnapshot(cursorLine, cursorCol)
	b.lastEditNano = nowNano()
}

// pushSnapshot performs the actual push to the undo stack.
func (b *TextBuffer) pushSnapshot(cursorLine, cursorCol int) {
	b.undoStack = append(b.undoStack, bufferSnapshot{
		lines:      copyLines(b.lines),
		cursorLine: cursorLine,
		cursorCol:  cursorCol,
	})
	if len(b.undoStack) > maxUndoDepth {
		copy(b.undoStack, b.undoStack[1:])
		b.undoStack = b.undoStack[:len(b.undoStack)-1]
	}
	b.redoStack = nil
}

// copyLines deep-copies a string slice.
func copyLines(lines []string) []string {
	c := make([]string, len(lines))
	copy(c, lines)
	return c
}

// clampLine constrains line to [0, len(b.lines)-1].
func (b *TextBuffer) clampLine(line int) int {
	if line < 0 {
		return 0
	}
	if line >= len(b.lines) {
		return len(b.lines) - 1
	}
	return line
}

// clampCol constrains col to [0, runeCount] for the given (already-clamped) line.
func (b *TextBuffer) clampCol(line, col int) int {
	if col < 0 {
		return 0
	}
	n := len([]rune(b.lines[line]))
	if col > n {
		return n
	}
	return col
}

// splitLines splits text on "\n" boundaries. A trailing newline produces an
// extra empty element (matching strings.Split semantics).
func splitLines(text string) []string {
	result := []string{}
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			result = append(result, text[start:i])
			start = i + 1
		}
	}
	result = append(result, text[start:])
	return result
}

// leadingWhitespace returns the leading spaces and tabs from s.
func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s // entire string is whitespace
}
