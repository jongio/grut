package preview

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/panels"
)

// tocHeading is a single markdown ATX heading discovered in the raw source.
// rawLine is the 0-based index of the heading within the file's source lines,
// which maps directly to the display line when markdown rendering is off.
type tocHeading struct {
	text    string
	level   int
	rawLine int
}

// tocMaxLevel is the deepest ATX heading level markdown recognizes (######).
const tocMaxLevel = 6

// Fenced code block markers. Headings inside a fence are skipped.
const (
	fenceBacktick = "```"
	fenceTilde    = "~~~"
)

// parseMarkdownHeadings extracts ATX headings (# ... through ###### ...) from
// markdown source. Headings inside fenced code blocks (``` or ~~~) are ignored
// so that commented shell lines or diff hunks are not mistaken for headings.
// Setext headings (underlined with = or -) are intentionally not detected;
// only the ATX form has an unambiguous single-line anchor.
func parseMarkdownHeadings(source string) []tocHeading {
	lines := strings.Split(source, "\n")
	headings := make([]tocHeading, 0, 8)
	inFence := false
	fenceMarker := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if fenceMarker != "" && strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, fenceBacktick) {
			inFence = true
			fenceMarker = fenceBacktick
			continue
		}
		if strings.HasPrefix(trimmed, fenceTilde) {
			inFence = true
			fenceMarker = fenceTilde
			continue
		}
		level, text, ok := parseHeadingLine(line)
		if !ok {
			continue
		}
		headings = append(headings, tocHeading{text: text, level: level, rawLine: i})
	}
	return headings
}

// parseHeadingLine reports whether a single line is an ATX heading and, if so,
// returns its level (1-6) and display text with the leading hashes, optional
// closing hashes, and surrounding whitespace stripped. A line like "#tag" is
// not a heading because CommonMark requires a space after the hash run.
func parseHeadingLine(line string) (level int, text string, ok bool) {
	i := 0
	// Up to three leading spaces are allowed before the hash run.
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	start := i
	for i < len(line) && line[i] == '#' {
		i++
	}
	level = i - start
	if level < 1 || level > tocMaxLevel {
		return 0, "", false
	}
	// A space or tab must separate the hashes from the heading text (or the
	// line ends right after the hashes, which is an empty heading we skip).
	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return 0, "", false
	}
	text = strings.TrimSpace(line[i:])
	text = stripClosingHashes(text)
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

// stripClosingHashes removes an optional trailing run of hashes used to close
// an ATX heading, e.g. "## Title ##" becomes "Title". A trailing hash run that
// is not preceded by whitespace (e.g. "Title#") is treated as part of the text.
func stripClosingHashes(text string) string {
	trimmed := strings.TrimRight(text, " \t")
	end := len(trimmed)
	for end > 0 && trimmed[end-1] == '#' {
		end--
	}
	if end == len(trimmed) {
		return trimmed
	}
	if end == 0 {
		// The line was only hashes after the opening run; nothing to show.
		return ""
	}
	if trimmed[end-1] != ' ' && trimmed[end-1] != '\t' {
		// The hashes were glued to a word, so keep the original text.
		return trimmed
	}
	return strings.TrimRight(trimmed[:end], " \t")
}

// openTOC activates the heading-jump overlay for the current markdown file.
// It returns a command that routes key presses to the preview until the overlay
// closes. It is a no-op (returns nil) when the view is not a plain markdown file
// with headings, mirroring openGotoLine's guards.
func (p *Preview) openTOC() tea.Cmd {
	if p.ghMode || p.blameMode || p.diffMode || p.isBinary || p.isLarge {
		return nil
	}
	if !isMarkdownExt(filepath.Ext(p.filePath)) {
		return nil
	}
	if len(p.mdHeadings) == 0 {
		return nil
	}
	p.tocTargets = p.mapHeadingDisplayLines()
	p.tocCursor = p.nearestHeadingIndex()
	p.tocActive = true
	return func() tea.Msg { return panels.PreviewInputStartedMsg{} }
}

// closeTOC deactivates the overlay and resumes normal key routing.
func (p *Preview) closeTOC() tea.Cmd {
	p.tocActive = false
	return func() tea.Msg { return panels.PreviewInputEndedMsg{} }
}

// handleTOCKey processes a key press while the heading overlay is open.
// Up/down (and j/k) move the selection, Enter jumps to the heading, and Esc
// cancels without moving the viewport.
func (p *Preview) handleTOCKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEscape, keyEsc:
		return p, p.closeTOC()
	case keyEnter:
		p.commitTOC()
		return p, p.closeTOC()
	case "j", keyDown:
		if p.tocCursor < len(p.mdHeadings)-1 {
			p.tocCursor++
		}
		return p, nil
	case "k", "up":
		if p.tocCursor > 0 {
			p.tocCursor--
		}
		return p, nil
	case "g", "home":
		p.tocCursor = 0
		return p, nil
	case "G", keyEnd:
		p.tocCursor = len(p.mdHeadings) - 1
		return p, nil
	}
	return p, nil
}

// commitTOC scrolls so the selected heading sits at the top of the viewport.
func (p *Preview) commitTOC() {
	if p.tocCursor < 0 || p.tocCursor >= len(p.tocTargets) {
		return
	}
	p.scrollY = p.tocTargets[p.tocCursor]
	p.clampScroll()
}

// nearestHeadingIndex returns the index of the heading at or just above the
// current scroll position so the overlay opens with the visible section
// preselected.
func (p *Preview) nearestHeadingIndex() int {
	idx := 0
	for i, target := range p.tocTargets {
		if target <= p.scrollY {
			idx = i
		} else {
			break
		}
	}
	return idx
}

// mapHeadingDisplayLines resolves each heading to a display-line index in the
// currently rendered content. When markdown rendering is off, the display lines
// are a 1:1 transform of the source, so the raw line index is used directly.
// When glamour rendering is on, the hash prefixes are gone, so headings are
// located by scanning the rendered lines for their text in document order.
func (p *Preview) mapHeadingDisplayLines() []int {
	targets := make([]int, len(p.mdHeadings))
	lineCount := len(p.lines)
	if !p.renderMarkdown {
		for i, h := range p.mdHeadings {
			targets[i] = clampIndex(h.rawLine, lineCount)
		}
		return targets
	}
	cursor := 0
	for i, h := range p.mdHeadings {
		target := clampIndex(cursor, lineCount)
		if found := findHeadingLine(p.lines, h.text, cursor); found >= 0 {
			target = found
			cursor = found + 1
		}
		targets[i] = target
	}
	return targets
}

// findHeadingLine scans rendered lines starting at from for the first line whose
// visible text matches the heading. It tries exact (trimmed) equality first,
// then a substring match, then a case-insensitive substring match, so that
// glamour styling (padding, background blocks, letter casing) still resolves.
// Returns -1 when no line matches.
func findHeadingLine(lines []string, text string, from int) int {
	if from < 0 {
		from = 0
	}
	lower := strings.ToLower(text)
	fallback := -1
	for j := from; j < len(lines); j++ {
		plain := strings.TrimSpace(ansi.Strip(lines[j]))
		if plain == "" {
			continue
		}
		if plain == text {
			return j
		}
		if fallback == -1 && strings.Contains(plain, text) {
			fallback = j
		}
		if fallback == -1 && strings.Contains(strings.ToLower(plain), lower) {
			fallback = j
		}
	}
	return fallback
}

// clampIndex keeps a line index within [0, count-1], returning 0 for empty
// content.
func clampIndex(idx, count int) int {
	if count == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= count {
		return count - 1
	}
	return idx
}

// renderTOC draws the heading overlay: a scrollable, indented list of headings
// with the selected entry marked. It fills exactly height lines so the outer
// container never reflows.
func (p *Preview) renderTOC(width, height int) string {
	if height < 1 {
		height = 1
	}
	title := p.newDimStyle().Render("Jump to heading")
	footerText := "\u2191/\u2193 select \u00b7 enter jump \u00b7 esc cancel"
	footer := ansi.Truncate(p.newDimStyle().Render(footerText), width, "")

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := tocWindowStart(p.tocCursor, len(p.mdHeadings), listHeight)
	end := start + listHeight
	if end > len(p.mdHeadings) {
		end = len(p.mdHeadings)
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(panels.ColorOf(p.themeColors().BrightCyan, "#8BE9FD")).
		Bold(true)

	rows := make([]string, 0, height)
	rows = append(rows, ansi.Truncate(title, width, ""))
	for i := start; i < end; i++ {
		h := p.mdHeadings[i]
		indent := strings.Repeat("  ", h.level-1)
		marker := "  "
		label := indent + h.text
		if i == p.tocCursor {
			marker = "\u25b8 "
			label = selectedStyle.Render(indent + h.text)
		}
		row := ansi.Truncate(marker+label, width, "")
		rows = append(rows, row)
	}
	for len(rows) < height-1 {
		rows = append(rows, "")
	}
	rows = append(rows, footer)
	return strings.Join(rows, "\n")
}

// tocWindowStart returns the first visible index for the heading list so that
// the cursor stays within the visible window of listHeight rows.
func tocWindowStart(cursor, total, listHeight int) int {
	if total <= listHeight {
		return 0
	}
	start := cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	if start > total-listHeight {
		start = total - listHeight
	}
	return start
}
