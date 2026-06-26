package preview

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"

	tea "charm.land/bubbletea/v2"
)

// selPoint represents an absolute position within the preview content.
type selPoint struct {
	Line int // absolute index into the display lines
	Col  int // rune offset within the stripped (ANSI-free) line
}

// selRange returns the selection range in normalized order (start ≤ end).
// Returns nil, nil if no valid selection exists.
func (p *Preview) selRange() (*selPoint, *selPoint) {
	if p.selAnchor == nil || p.selEnd == nil {
		return nil, nil
	}
	a, b := *p.selAnchor, *p.selEnd
	if a.Line > b.Line || (a.Line == b.Line && a.Col > b.Col) {
		a, b = b, a
	}
	return &a, &b
}

// hasSelection returns true if a non-empty text selection exists.
func (p *Preview) hasSelection() bool {
	s, e := p.selRange()
	if s == nil {
		return false
	}
	return s.Line != e.Line || s.Col != e.Col
}

// clearSelection removes the current selection.
func (p *Preview) clearSelection() {
	p.selAnchor = nil
	p.selEnd = nil
	p.selecting = false
}

// displayLines returns the current set of lines being rendered,
// matching the logic in renderContent.
func (p *Preview) displayLines() []string {
	if p.blameMode && len(p.blameLines) > 0 {
		return nil // selection not supported in blame mode
	}
	dl := p.lines
	if p.diffMode && len(p.diffLines) > 0 {
		dl = p.diffLines
	}
	return dl
}

// gutterWidth returns the number of characters used by the line number
// gutter (including the " │ " separator). Returns 0 when line numbers
// are disabled.
func (p *Preview) gutterWidth() int {
	if !p.lineNumbers {
		return 0
	}
	dl := p.displayLines()
	if len(dl) == 0 {
		return 0
	}
	w := numDigits(len(dl))
	if w < 3 {
		w = 3
	}
	return w + 3 // "NNN │ "
}

// numDigits returns the number of decimal digits needed to represent n.
func numDigits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// contentRowColToAbs converts content-area coordinates (row, col) into
// an absolute selPoint. Row is offset by scrollY; col accounts for the
// gutter and maps from display column to rune offset in the stripped line.
func (p *Preview) contentRowColToAbs(row, col int) selPoint {
	absLine := p.scrollY + row
	dl := p.displayLines()
	if absLine < 0 {
		absLine = 0
	}
	if dl != nil && absLine >= len(dl) {
		absLine = len(dl) - 1
	}
	if absLine < 0 {
		return selPoint{Line: 0, Col: 0}
	}
	// Subtract gutter offset from column.
	runeCol := col - p.gutterWidth()
	if runeCol < 0 {
		runeCol = 0
	}
	// Map display column to rune offset in the stripped line.
	if dl != nil && absLine < len(dl) {
		raw := dl[absLine]
		raw = strings.ReplaceAll(raw, "\t", "    ")
		stripped := ansi.Strip(raw)
		runeCount := utf8.RuneCountInString(stripped)
		if runeCol > runeCount {
			runeCol = runeCount
		}
	}
	return selPoint{Line: absLine, Col: runeCol}
}

// selectionDisabled returns true when text selection should not be active.
func (p *Preview) selectionDisabled() bool {
	return p.isBinary || p.isLarge || p.wordWrap || (p.blameMode && len(p.blameLines) > 0)
}

// handleMouseClick starts a new selection at the clicked position.
func (p *Preview) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	if p.editMode && p.editBuf != nil {
		return p.handleEditMouseClick(msg)
	}
	if p.selectionDisabled() {
		return p, nil
	}
	pt := p.contentRowColToAbs(msg.ContentRow, msg.ContentCol)
	p.selAnchor = &pt
	p.selEnd = &pt
	p.selecting = true
	return p, nil
}

// handleMouseMotion extends the selection while dragging.
func (p *Preview) handleMouseMotion(msg panels.PanelMouseMotionMsg) (panels.Panel, tea.Cmd) {
	if p.editMode && p.editBuf != nil {
		return p.handleEditMouseMotion(msg)
	}
	if !p.selecting || p.selAnchor == nil {
		return p, nil
	}

	// Auto-scroll when dragging past viewport edges (before computing
	// coordinates so selEnd reflects the updated scroll position).
	vh := p.viewportHeight()
	if msg.ContentRow < 0 {
		p.scrollUp(1)
	} else if msg.ContentRow >= vh-1 {
		p.scrollDown(1)
	}

	pt := p.contentRowColToAbs(msg.ContentRow, msg.ContentCol)
	p.selEnd = &pt
	return p, nil
}

// handleMouseRelease finalizes the selection.
func (p *Preview) handleMouseRelease(msg panels.PanelMouseReleaseMsg) (panels.Panel, tea.Cmd) {
	if p.editMode && p.editBuf != nil {
		return p.handleEditMouseRelease(msg)
	}
	p.selecting = false
	return p, nil
}

// ---------------------------------------------------------------------------
// Edit-mode mouse handlers
// ---------------------------------------------------------------------------

// editGutterWidth computes the gutter width for edit mode rendering.
// This must match editor_render.go's gutter calculation.
func (p *Preview) editGutterWidth() int {
	totalLines := p.editBuf.LineCount()
	numWidth := len(strconv.Itoa(totalLines))
	if numWidth < 3 {
		numWidth = 3
	}
	return numWidth + 3 // digits + " │ "
}

// mouseToBufferPos converts mouse content-area coordinates to buffer
// line and column. Returns clamped values safe to use as cursor position.
func (p *Preview) mouseToBufferPos(contentRow, contentCol int) (line, col int) {
	line = p.scrollY + contentRow
	if line < 0 {
		line = 0
	}
	if line >= p.editBuf.LineCount() {
		line = p.editBuf.LineCount() - 1
	}
	if line < 0 {
		return 0, 0
	}

	col = contentCol - p.editGutterWidth()
	if col < 0 {
		col = 0
	}

	// Account for tab expansion: the displayed column may be wider
	// than the rune offset because tabs are expanded to spaces.
	tabSize := p.editCfg.TabSize
	if tabSize < 1 {
		tabSize = 4
	}
	runes := []rune(p.editBuf.Line(line))

	// Walk runes, tracking display column to find the rune offset
	// that corresponds to the clicked display column.
	displayCol := 0
	for i, r := range runes {
		if displayCol >= col {
			return line, i
		}
		if r == '\t' {
			displayCol += tabSize
		} else {
			displayCol++
		}
	}
	// Clicked past end of line — clamp to end.
	return line, len(runes)
}

// handleEditMouseClick positions the cursor and starts a potential drag selection.
func (p *Preview) handleEditMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	line, col := p.mouseToBufferPos(msg.ContentRow, msg.ContentCol)
	p.cursorLine = line
	p.cursorCol = col
	clearEditSelection(p)
	// Set anchor for potential drag.
	pt := selPoint{Line: line, Col: col}
	p.selAnchor = &pt
	p.selEnd = &pt
	p.selecting = true
	return p, nil
}

// handleEditMouseMotion extends the selection while dragging in edit mode.
func (p *Preview) handleEditMouseMotion(msg panels.PanelMouseMotionMsg) (panels.Panel, tea.Cmd) {
	if !p.selecting || p.selAnchor == nil {
		return p, nil
	}

	// Auto-scroll when dragging past viewport edges.
	vh := p.viewportHeight()
	if msg.ContentRow < 0 {
		p.scrollUp(1)
	} else if msg.ContentRow >= vh-1 {
		p.scrollDown(1)
	}

	line, col := p.mouseToBufferPos(msg.ContentRow, msg.ContentCol)
	p.cursorLine = line
	p.cursorCol = col
	p.selEnd = &selPoint{Line: line, Col: col}
	return p, nil
}

// handleEditMouseRelease finalizes mouse selection in edit mode.
func (p *Preview) handleEditMouseRelease(_ panels.PanelMouseReleaseMsg) (panels.Panel, tea.Cmd) {
	p.selecting = false
	// If anchor == end, it was just a click (no drag) — clear selection.
	if p.selAnchor != nil && p.selEnd != nil &&
		p.selAnchor.Line == p.selEnd.Line && p.selAnchor.Col == p.selEnd.Col {
		clearEditSelection(p)
	}
	return p, nil
}

// handleEditDoubleClick selects the word under the cursor in edit mode.
func (p *Preview) handleEditDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	line, col := p.mouseToBufferPos(msg.ContentRow, msg.ContentCol)
	p.cursorLine = line
	p.cursorCol = col

	runes := []rune(p.editBuf.Line(line))
	if col >= len(runes) {
		return p, nil
	}

	if !isWordRune(runes[col]) {
		// Non-word char: select just that character.
		p.selAnchor = &selPoint{Line: line, Col: col}
		p.selEnd = &selPoint{Line: line, Col: col + 1}
		p.cursorCol = col + 1
		p.selecting = false
		return p, nil
	}

	start, end := col, col
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	for end < len(runes)-1 && isWordRune(runes[end+1]) {
		end++
	}
	p.selAnchor = &selPoint{Line: line, Col: start}
	p.selEnd = &selPoint{Line: line, Col: end + 1}
	p.cursorCol = end + 1
	p.selecting = false
	return p, nil
}

// handleDoubleClick selects the word under the cursor.
func (p *Preview) handleDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	if p.editMode && p.editBuf != nil {
		return p.handleEditDoubleClick(msg)
	}
	if p.selectionDisabled() {
		return p, nil
	}
	dl := p.displayLines()
	if dl == nil {
		return p, nil
	}
	absLine := p.scrollY + msg.ContentRow
	if absLine < 0 || absLine >= len(dl) {
		return p, nil
	}
	raw := dl[absLine]
	raw = strings.ReplaceAll(raw, "\t", "    ")
	stripped := ansi.Strip(raw)

	col := msg.ContentCol - p.gutterWidth()
	if col < 0 {
		col = 0
	}
	runes := []rune(stripped)
	if col >= len(runes) {
		return p, nil
	}

	// Find word boundaries.
	start, end := col, col
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	if !isWord(runes[col]) {
		// Click on non-word char: select just that character.
		p.selAnchor = &selPoint{Line: absLine, Col: col}
		p.selEnd = &selPoint{Line: absLine, Col: col + 1}
		p.selecting = false
		return p, nil
	}
	for start > 0 && isWord(runes[start-1]) {
		start--
	}
	for end < len(runes)-1 && isWord(runes[end+1]) {
		end++
	}
	p.selAnchor = &selPoint{Line: absLine, Col: start}
	p.selEnd = &selPoint{Line: absLine, Col: end + 1}
	p.selecting = false
	return p, nil
}

// selectedText extracts the selected text as a plain string.
func (p *Preview) selectedText() string {
	s, e := p.selRange()
	if s == nil {
		return ""
	}
	dl := p.displayLines()
	if len(dl) == 0 {
		return ""
	}
	var sb strings.Builder
	for lineIdx := s.Line; lineIdx <= e.Line && lineIdx < len(dl); lineIdx++ {
		raw := dl[lineIdx]
		raw = strings.ReplaceAll(raw, "\t", "    ")
		stripped := ansi.Strip(raw)
		runes := []rune(stripped)

		startCol := 0
		endCol := len(runes)
		if lineIdx == s.Line {
			startCol = s.Col
		}
		if lineIdx == e.Line {
			endCol = e.Col
		}
		if startCol > len(runes) {
			startCol = len(runes)
		}
		if endCol > len(runes) {
			endCol = len(runes)
		}
		if startCol > endCol {
			startCol = endCol
		}
		sb.WriteString(string(runes[startCol:endCol]))
		if lineIdx < e.Line && lineIdx+1 < len(dl) {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// HasSelection implements panels.SelectionCopier.
func (p *Preview) HasSelection() bool {
	return p.hasSelection()
}

// CopySelection implements panels.SelectionCopier.
func (p *Preview) CopySelection() (panels.Panel, tea.Cmd) {
	return p.copySelection()
}

// copySelection copies the selected text to the clipboard.
func (p *Preview) copySelection() (panels.Panel, tea.Cmd) {
	text := p.selectedText()
	if text == "" {
		return p, nil
	}
	p.clearSelection()
	return p, func() tea.Msg {
		if err := panels.CopyToClipboard(context.Background(), text); err != nil {
			return notify.ShowToastMsg{Message: "Copy failed: " + err.Error(), Level: notify.Error}
		}
		lineCount := strings.Count(text, "\n") + 1
		runeCount := utf8.RuneCountInString(text)
		if lineCount == 1 {
			return notify.ShowToastMsg{
				Message: "Copied " + strings.TrimSpace(pluralize(runeCount, "char")),
				Level:   notify.Info,
			}
		}
		return notify.ShowToastMsg{
			Message: "Copied " + pluralize(lineCount, "line"),
			Level:   notify.Info,
		}
	}
}

func (p *Preview) copyFilePath() (panels.Panel, tea.Cmd) {
	if p.filePath == "" || p.ghMode {
		return p, nil
	}
	path := p.filePath
	return p, func() tea.Msg {
		if err := panels.CopyToClipboard(context.Background(), path); err != nil {
			return notify.ShowToastMsg{Message: "Copy failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Copied path", Level: notify.Info}
	}
}

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
