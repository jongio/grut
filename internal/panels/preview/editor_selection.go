// editor_selection.go provides text selection support for edit mode.
// It manages selection anchors, range extraction, and selected-text
// retrieval using buffer coordinates (not display-line coordinates).
//
// In edit mode the existing selAnchor / selEnd fields on Preview are
// reused (read-mode mouse selection is inactive while editing).
package preview

import (
	"strings"
	"unicode"
)

// isWordRune reports whether r is a "word" character (letter, digit, or
// underscore). This mirrors the local closure in handleDoubleClick but
// is package-level so that both selection.go and editor_selection.go
// can reference it.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// ---------------------------------------------------------------------------
// Selection range helpers
// ---------------------------------------------------------------------------

// editSelRange returns the edit-mode selection normalised so that
// start <= end. Returns nil, nil when no selection exists.
func editSelRange(p *Preview) (*selPoint, *selPoint) {
	if p.selAnchor == nil || p.selEnd == nil {
		return nil, nil
	}
	a, b := *p.selAnchor, *p.selEnd
	if a.Line > b.Line || (a.Line == b.Line && a.Col > b.Col) {
		a, b = b, a
	}
	return &a, &b
}

// hasEditSelection reports whether a non-empty edit selection exists
// (i.e. start and end differ).
func hasEditSelection(p *Preview) bool {
	s, e := editSelRange(p)
	if s == nil {
		return false
	}
	return s.Line != e.Line || s.Col != e.Col
}

// clearEditSelection removes any active edit selection.
func clearEditSelection(p *Preview) {
	p.selAnchor = nil
	p.selEnd = nil
}

// ---------------------------------------------------------------------------
// Text extraction
// ---------------------------------------------------------------------------

// editSelectedText returns the plain text covered by the current edit
// selection, reading from the edit buffer. Returns "" if there is no
// selection or no buffer.
func editSelectedText(p *Preview) string {
	s, e := editSelRange(p)
	if s == nil {
		return ""
	}
	buf := p.editBuf
	if buf == nil {
		return ""
	}

	// Single-line selection.
	if s.Line == e.Line {
		runes := []rune(buf.Line(s.Line))
		sc := clampInt(s.Col, 0, len(runes))
		ec := clampInt(e.Col, 0, len(runes))
		return string(runes[sc:ec])
	}

	// Multi-line selection.
	var sb strings.Builder
	for lineIdx := s.Line; lineIdx <= e.Line && lineIdx < buf.LineCount(); lineIdx++ {
		runes := []rune(buf.Line(lineIdx))

		startCol := 0
		endCol := len(runes)
		if lineIdx == s.Line {
			startCol = clampInt(s.Col, 0, len(runes))
		}
		if lineIdx == e.Line {
			endCol = clampInt(e.Col, 0, len(runes))
		}

		sb.WriteString(string(runes[startCol:endCol]))
		if lineIdx < e.Line {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// clampInt returns v clamped to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// Selection manipulation
// ---------------------------------------------------------------------------

// extendEditSelection extends (or starts) the edit selection so that the
// end point moves to (newLine, newCol). If no anchor exists yet the
// anchor is placed at the current cursor position first - this is how
// Shift+Arrow works: the anchor stays where Shift was first pressed and
// the end moves with the cursor.
func extendEditSelection(p *Preview, newLine, newCol int) {
	if p.selAnchor == nil {
		p.selAnchor = &selPoint{Line: p.cursorLine, Col: p.cursorCol}
	}
	p.selEnd = &selPoint{Line: newLine, Col: newCol}
}

// selectAll selects the entire edit buffer content.
func selectAll(p *Preview) {
	buf := p.editBuf
	if buf == nil || buf.LineCount() == 0 {
		return
	}
	p.selAnchor = &selPoint{Line: 0, Col: 0}

	lastLine := buf.LineCount() - 1
	lastCol := len([]rune(buf.Line(lastLine)))
	p.selEnd = &selPoint{Line: lastLine, Col: lastCol}
}

// ---------------------------------------------------------------------------
// Word boundary helpers
// ---------------------------------------------------------------------------

// findWordBoundaryLeft returns the column of the start of the previous
// word when scanning leftward from col in line. It first skips any
// non-word characters, then skips word characters. Returns 0 when
// already at the start.
func findWordBoundaryLeft(line string, col int) int {
	runes := []rune(line)
	if col <= 0 {
		return 0
	}
	if col > len(runes) {
		col = len(runes)
	}

	i := col
	// Skip non-word characters going left.
	for i > 0 && !isWordRune(runes[i-1]) {
		i--
	}
	// Skip word characters going left.
	for i > 0 && isWordRune(runes[i-1]) {
		i--
	}
	return i
}

// findWordBoundaryRight returns the column of the start of the next word
// when scanning rightward from col in line. It first skips word
// characters, then skips non-word characters. Returns len(runes) when
// already at the end.
func findWordBoundaryRight(line string, col int) int {
	runes := []rune(line)
	n := len(runes)
	if col >= n {
		return n
	}
	if col < 0 {
		col = 0
	}

	i := col
	// Skip word characters going right.
	for i < n && isWordRune(runes[i]) {
		i++
	}
	// Skip non-word characters going right.
	for i < n && !isWordRune(runes[i]) {
		i++
	}
	return i
}
