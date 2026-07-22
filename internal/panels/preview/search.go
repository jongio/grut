package preview

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/panels"
)

type searchMatch struct {
	line, startCol, endCol int
}

func (p *Preview) openSearch() tea.Cmd {
	if p.isBinary || p.isLarge || len(p.currentSearchLines()) == 0 {
		return nil
	}
	p.searchActive = true
	p.searchInput = ""
	p.searchQuery = ""
	p.searchMatches = nil
	p.searchIdx = 0
	return func() tea.Msg { return panels.PreviewInputStartedMsg{} }
}

func (p *Preview) handleSearchKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEscape, keyEsc:
		p.clearSearch()
		return p, func() tea.Msg { return panels.PreviewInputEndedMsg{} }
	case keyEnter:
		p.searchQuery = p.searchInput
		p.searchActive = false
		if p.searchQuery == "" {
			p.clearSearch()
		} else if len(p.searchMatches) > 0 {
			p.searchIdx = 0
			p.scrollToMatch()
		}
		return p, func() tea.Msg { return panels.PreviewInputEndedMsg{} }
	case keyBackspace:
		if r := []rune(p.searchInput); len(r) > 0 {
			p.searchInput = string(r[:len(r)-1])
			p.computeSearchMatches()
		}
		return p, nil
	default:
		if s := msg.String(); utf8.RuneCountInString(s) == 1 {
			p.searchInput += s
			p.computeSearchMatches()
		}
		return p, nil
	}
}

func (p *Preview) clearSearch() {
	p.searchActive = false
	p.searchInput = ""
	p.searchQuery = ""
	p.searchMatches = nil
	p.searchIdx = 0
}

func (p *Preview) computeSearchMatches() {
	query := p.searchInput
	if query == "" {
		p.searchMatches = nil
		p.searchIdx = 0
		return
	}

	queryRunes := []rune(strings.ToLower(query))
	lines := p.currentSearchLines()
	matches := make([]searchMatch, 0)
	for lineIdx, line := range lines {
		lineRunes := []rune(strings.ToLower(ansi.Strip(line)))
		for col := 0; col+len(queryRunes) <= len(lineRunes); col++ {
			if runesEqual(lineRunes[col:col+len(queryRunes)], queryRunes) {
				matches = append(matches, searchMatch{
					line:     lineIdx,
					startCol: col,
					endCol:   col + len(queryRunes),
				})
			}
		}
	}
	p.searchMatches = matches
	if len(p.searchMatches) == 0 {
		p.searchMatches = nil
		p.searchIdx = 0
		return
	}
	if p.searchIdx >= len(p.searchMatches) {
		p.searchIdx = len(p.searchMatches) - 1
	}
	if p.searchIdx < 0 {
		p.searchIdx = 0
	}
}

func runesEqual(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for i, r := range left {
		if r != right[i] {
			return false
		}
	}
	return true
}

func (p *Preview) nextMatch() {
	if len(p.searchMatches) == 0 {
		return
	}
	p.searchIdx = (p.searchIdx + 1) % len(p.searchMatches)
	p.scrollToMatch()
}

func (p *Preview) prevMatch() {
	if len(p.searchMatches) == 0 {
		return
	}
	p.searchIdx = (p.searchIdx - 1 + len(p.searchMatches)) % len(p.searchMatches)
	p.scrollToMatch()
}

func (p *Preview) scrollToMatch() {
	if len(p.searchMatches) == 0 || p.searchIdx < 0 || p.searchIdx >= len(p.searchMatches) {
		return
	}
	target := p.searchMatches[p.searchIdx].line
	viewportHeight := p.viewportHeight()
	if target >= p.scrollY && target < p.scrollY+viewportHeight {
		return
	}
	p.scrollY = target - viewportHeight/2
	p.clampScroll()
}

func (p *Preview) currentSearchLines() []string {
	if p.blameMode && len(p.blameLines) > 0 {
		lines := make([]string, 0, len(p.blameLines))
		for _, bl := range p.blameLines {
			lines = append(lines, bl.Content)
		}
		return lines
	}
	if p.diffMode {
		return p.diffLines
	}
	return p.lines
}

func (p *Preview) applySearchHighlight(line string, absLine int) string {
	if len(p.searchMatches) == 0 {
		return line
	}

	lineMatches := make([]searchMatch, 0)
	current := searchMatch{line: -1}
	if p.searchIdx >= 0 && p.searchIdx < len(p.searchMatches) {
		current = p.searchMatches[p.searchIdx]
	}
	for _, match := range p.searchMatches {
		if match.line == absLine {
			lineMatches = append(lineMatches, match)
		}
	}
	if len(lineMatches) == 0 {
		return line
	}

	normalStyle := lipgloss.NewStyle().Background(lipgloss.Color("#4A3F1A"))
	currentStyle := lipgloss.NewStyle().Background(lipgloss.Color("#7A5C00"))
	out := strings.Builder{}
	runeIdx := 0
	matchIdx := 0
	highlight := strings.Builder{}
	inHighlight := false
	currentHighlight := false

	flushHighlight := func() {
		if highlight.Len() == 0 {
			return
		}
		if currentHighlight {
			out.WriteString(currentStyle.Render(highlight.String()))
		} else {
			out.WriteString(normalStyle.Render(highlight.String()))
		}
		highlight.Reset()
	}

	write := func(s string) {
		for matchIdx < len(lineMatches) && runeIdx >= lineMatches[matchIdx].endCol {
			matchIdx++
		}
		nextInHighlight := matchIdx < len(lineMatches) &&
			runeIdx >= lineMatches[matchIdx].startCol &&
			runeIdx < lineMatches[matchIdx].endCol
		nextCurrentHighlight := nextInHighlight &&
			lineMatches[matchIdx].line == current.line &&
			lineMatches[matchIdx].startCol == current.startCol &&
			lineMatches[matchIdx].endCol == current.endCol
		if inHighlight != nextInHighlight || currentHighlight != nextCurrentHighlight {
			flushHighlight()
			inHighlight = nextInHighlight
			currentHighlight = nextCurrentHighlight
		}
		if inHighlight {
			highlight.WriteString(s)
		} else {
			out.WriteString(s)
		}
	}

	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			seqEnd := ansiSequenceEnd(line, i)
			write(line[i:seqEnd])
			i = seqEnd
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		write(string(r))
		runeIdx++
		i += size
	}
	flushHighlight()
	return out.String()
}

func ansiSequenceEnd(line string, start int) int {
	seqEnd := start + 1
	if seqEnd < len(line) && line[seqEnd] == '[' {
		seqEnd++
		for seqEnd < len(line) && !isCSITerminator(line[seqEnd]) {
			seqEnd++
		}
		if seqEnd < len(line) {
			seqEnd++
		}
	} else if seqEnd < len(line) && line[seqEnd] == ']' {
		for seqEnd < len(line) && line[seqEnd] != '\x07' {
			if line[seqEnd] == '\x1b' && seqEnd+1 < len(line) && line[seqEnd+1] == '\\' {
				seqEnd += 2
				break
			}
			seqEnd++
		}
		if seqEnd < len(line) && line[seqEnd] == '\x07' {
			seqEnd++
		}
	} else if seqEnd < len(line) {
		seqEnd++
	}
	return seqEnd
}

func (p *Preview) searchFooterInfo(base string) string {
	switch {
	case p.searchActive && len(p.searchMatches) > 0:
		return fmt.Sprintf("Search: %s (%d/%d)", p.searchInput, p.searchIdx+1, len(p.searchMatches))
	case p.searchActive && p.searchInput != "":
		return "Search: " + p.searchInput + " (no matches)"
	case p.searchActive:
		return "Search: "
	case len(p.searchMatches) > 0 && p.searchQuery != "":
		count := fmt.Sprintf("%d/%d", p.searchIdx+1, len(p.searchMatches))
		if base == "" {
			return count
		}
		return base + " • " + count
	default:
		return base
	}
}
