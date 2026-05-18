package preview

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// Pre-allocated styles for editor rendering. These are created once to
// avoid per-frame lipgloss.NewStyle allocations in the render loop.
var (
	editorDimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	editorCursorStyle     = lipgloss.NewStyle().Reverse(true)
	editorCurLineNumStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C9A027"))
	editorCurLineBg       = lipgloss.Color("#1A1A1A")
	editorCurLineStyle    = lipgloss.NewStyle().Background(editorCurLineBg)
	editorSelectionStyle = lipgloss.NewStyle().Background(lipgloss.Color("#264F78"))
	editorStatusStyle    = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Background(lipgloss.Color("#1A1A1A"))
)

// renderEditContent renders the buffer contents with syntax highlighting,
// a cursor block, current-line highlight, line-number gutter, and a
// status bar at the bottom.
func renderEditContent(p *Preview, width, height int) string {
	if p.editBuf == nil {
		return ""
	}
	lines := p.editBuf.Lines()
	totalLines := len(lines)

	// Reserve 1 line for the status bar.
	contentHeight := height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Calculate visible range based on scroll position.
	p.clampEditScroll(totalLines, contentHeight)
	start := p.scrollY
	end := start + contentHeight
	if end > totalLines {
		end = totalLines
	}

	// Line number gutter sizing: "NNN │ " where NNN is right-aligned.
	numWidth := len(strconv.Itoa(totalLines))
	if numWidth < 3 {
		numWidth = 3
	}
	gutterWidth := numWidth + 3 // digits + " │ "
	contentWidth := width - gutterWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	rendered := make([]string, 0, contentHeight)
	for i := start; i < end; i++ {
		lineNum := i + 1
		rawLine := lines[i]

		// Expand tabs for display.
		tabSize := p.editCfg.TabSize
		if tabSize < 1 {
			tabSize = 4
		}
		displayLine := strings.ReplaceAll(rawLine, "\t", strings.Repeat(" ", tabSize))

		// Apply syntax highlighting to this single line.
		highlighted := highlightLine(displayLine, p.filePath, p.cfg.GetTheme())

		// Apply selection highlighting if this line intersects the selection.
		selStart, selEnd := editSelRange(p)
		if selStart != nil && selEnd != nil && i >= selStart.Line && i <= selEnd.Line {
			highlighted = renderSelectionOnLine(highlighted, displayLine, i, selStart, selEnd, editorSelectionStyle)
		}

		isCursorLine := i == p.cursorLine

		// Render cursor on the cursor line.
		if isCursorLine {
			highlighted = renderCursorOnLine(highlighted, displayLine, p.cursorCol, editorCursorStyle)
		}

		// Truncate content to fit the available width.
		highlighted = ansi.Truncate(highlighted, contentWidth, "")

		// Apply current line background highlight and build gutter.
		// Build gutter string without fmt.Sprintf to avoid per-line allocation.
		digits := strconv.Itoa(lineNum)
		numStr := strings.Repeat(" ", numWidth-len(digits)) + digits + " │ "
		if isCursorLine {
			highlighted = editorCurLineStyle.Width(contentWidth).Render(highlighted)
			highlighted = editorCurLineNumStyle.Render(numStr) + highlighted
		} else {
			highlighted = editorDimStyle.Render(numStr) + highlighted
		}

		// Hard truncate to panel width.
		highlighted = ansi.Truncate(highlighted, width, "")
		rendered = append(rendered, highlighted)
	}

	// Pad remaining lines with empty gutter space.
	for len(rendered) < contentHeight {
		numStr := strings.Repeat(" ", gutterWidth)
		rendered = append(rendered, editorDimStyle.Render(numStr))
	}

	content := strings.Join(rendered, "\n")

	// Status bar at the bottom.
	statusBar := renderEditStatusBar(p, width, totalLines)
	content += "\n" + statusBar

	return content
}

// highlightLine applies Chroma syntax highlighting to a single line of
// text, using the filename for lexer matching and the given theme name.
func highlightLine(line, filename, theme string) string {
	if line == "" {
		return ""
	}
	lexer := lexers.Match(filename)
	if lexer == nil {
		return line
	}
	lexer = chroma.Coalesce(lexer)
	style := styles.Get(theme)
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		return line
	}
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return line
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return line
	}
	// Remove any trailing newline added by Chroma.
	return strings.TrimRight(buf.String(), "\n")
}

// renderCursorOnLine renders the cursor as an inverse-video block at the
// cursor column position within an ANSI-highlighted line. It walks the
// highlighted string counting visible runes (skipping escape sequences)
// to locate the cursor character.
func renderCursorOnLine(highlighted, rawLine string, cursorCol int, cursorStyle lipgloss.Style) string {
	runes := []rune(rawLine)
	if cursorCol >= len(runes) {
		// Cursor at end of line — render a thin block cursor.
		return highlighted + cursorStyle.Render("▏")
	}

	// Walk through the highlighted string, counting visible runes
	// (skipping ANSI escape sequences) to split at the cursor position.
	runeIdx := 0
	i := 0
	var before, cursor, after strings.Builder
	for i < len(highlighted) {
		if highlighted[i] == '\x1b' {
			// ANSI escape — copy verbatim to the current segment.
			seqEnd := i + 1
			if seqEnd < len(highlighted) && highlighted[seqEnd] == '[' {
				seqEnd++
				for seqEnd < len(highlighted) && !isCSITerminator(highlighted[seqEnd]) {
					seqEnd++
				}
				if seqEnd < len(highlighted) {
					seqEnd++
				}
			}
			seq := highlighted[i:seqEnd]
			switch {
			case runeIdx < cursorCol:
				before.WriteString(seq)
			case runeIdx == cursorCol:
				cursor.WriteString(seq)
			default:
				after.WriteString(seq)
			}
			i = seqEnd
			continue
		}

		// Normal rune.
		_, size := utf8.DecodeRuneInString(highlighted[i:])
		ch := highlighted[i : i+size]
		switch {
		case runeIdx < cursorCol:
			before.WriteString(ch)
		case runeIdx == cursorCol:
			cursor.WriteString(ch)
		default:
			after.WriteString(ch)
		}
		runeIdx++
		i += size
	}

	if cursor.Len() == 0 {
		return highlighted + cursorStyle.Render("▏")
	}

	// Strip ANSI from cursor char to get a clean character, then
	// apply the inverse-video cursor style.
	cursorChar := ansi.Strip(cursor.String())
	return before.String() + cursorStyle.Render(cursorChar) + after.String()
}

// renderSelectionOnLine applies selection highlight to the specified range
// of a syntax-highlighted line. It walks the ANSI string, finds the selected
// rune range for this line, and wraps selected characters with selStyle.
func renderSelectionOnLine(highlighted, rawLine string, lineIdx int, selStart, selEnd *selPoint, selStyle lipgloss.Style) string {
	runes := []rune(rawLine)
	lineLen := len(runes)

	startCol := 0
	endCol := lineLen

	if lineIdx == selStart.Line {
		startCol = selStart.Col
	}
	if lineIdx == selEnd.Line {
		endCol = selEnd.Col
	}

	if startCol >= endCol {
		return highlighted
	}
	if startCol >= lineLen {
		return highlighted
	}
	if endCol > lineLen {
		endCol = lineLen
	}

	// Walk the highlighted string splitting into before/selected/after segments.
	// This is the same ANSI-aware walk pattern as renderCursorOnLine.
	runeIdx := 0
	i := 0
	var before, selected, after strings.Builder
	for i < len(highlighted) {
		if highlighted[i] == '\x1b' {
			// ANSI escape — copy verbatim to the current segment.
			seqEnd := i + 1
			if seqEnd < len(highlighted) && highlighted[seqEnd] == '[' {
				seqEnd++
				for seqEnd < len(highlighted) && !isCSITerminator(highlighted[seqEnd]) {
					seqEnd++
				}
				if seqEnd < len(highlighted) {
					seqEnd++
				}
			}
			seq := highlighted[i:seqEnd]
			switch {
			case runeIdx < startCol:
				before.WriteString(seq)
			case runeIdx >= startCol && runeIdx < endCol:
				selected.WriteString(seq)
			default:
				after.WriteString(seq)
			}
			i = seqEnd
			continue
		}

		// Normal rune.
		_, size := utf8.DecodeRuneInString(highlighted[i:])
		ch := highlighted[i : i+size]
		switch {
		case runeIdx < startCol:
			before.WriteString(ch)
		case runeIdx >= startCol && runeIdx < endCol:
			selected.WriteString(ch)
		default:
			after.WriteString(ch)
		}
		runeIdx++
		i += size
	}

	if selected.Len() == 0 {
		return highlighted
	}

	return before.String() + selStyle.Render(ansi.Strip(selected.String())) + after.String()
}

// renderEditStatusBar renders the bottom status bar showing cursor
// position, tab settings, dirty indicator, and total line count.
func renderEditStatusBar(p *Preview, width, totalLines int) string {
	left := fmt.Sprintf(" Ln %d, Col %d", p.cursorLine+1, p.cursorCol+1)

	tabSize := p.editCfg.TabSize
	if tabSize < 1 {
		tabSize = 4
	}
	right := fmt.Sprintf("Tab: %d │ %d lines ", tabSize, totalLines)
	if p.editBuf != nil && p.editBuf.Dirty() {
		right = "[+] " + right
	}

	// Pad middle with spaces so left and right are flush.
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	bar := left + strings.Repeat(" ", padding) + right

	return editorStatusStyle.Width(width).Render(bar)
}

// clampEditScroll ensures the scroll offset keeps the cursor visible and
// doesn't exceed the buffer bounds.
func (p *Preview) clampEditScroll(totalLines, contentHeight int) {
	maxScroll := totalLines - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if p.scrollY > maxScroll {
		p.scrollY = maxScroll
	}
	if p.scrollY < 0 {
		p.scrollY = 0
	}
}
