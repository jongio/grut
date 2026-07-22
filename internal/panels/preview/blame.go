package preview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/git"
)

// blameHashLen is the truncated hash length for blame annotations.
const blameHashLen = 7

// blameAuthorMaxLen is the max display width for author names.
const blameAuthorMaxLen = 10

// blameAnnotationWidth is the total width of the blame prefix.
// Format: "abc1234 AuthorName 2024-01 │ "
//
//	7 + 1 + 10 + 1 + 7 + 3 = 29
const blameAnnotationWidth = 29

// renderBlameContent renders file content with inline blame annotations.
// Each line is prefixed with: hash author date │ content
// Annotations are color-coded by commit recency (older = dimmer).
func (p *Preview) renderBlameContent(width, height int) string {
	totalLines := len(p.blameLines)
	if totalLines == 0 {
		return p.renderEmptyState(width, height)
	}

	p.clampScroll()

	contentHeight := height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	start := p.scrollY
	end := start + contentHeight
	if end > totalLines {
		end = totalLines
	}

	visible := p.blameLines[start:end]
	now := time.Now()

	contentWidth := width - blameAnnotationWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	rendered := make([]string, 0, len(visible))
	for i, bl := range visible {
		annotation := formatBlameAnnotation(bl)
		color := blameRecencyColor(bl.Date, now)
		styledAnnotation := lipgloss.NewStyle().
			Foreground(lipgloss.Color(color)).
			Render(annotation)

		line := bl.Content
		line = strings.ReplaceAll(line, "\t", "    ")
		line = p.applySearchHighlight(line, start+i)
		if p.wordWrap && contentWidth > 0 {
			line = lipgloss.NewStyle().Width(contentWidth).Render(line)
		} else {
			line = ansi.Truncate(line, contentWidth, "")
		}

		rendered = append(rendered, styledAnnotation+line)
	}

	// Pad with empty lines if needed
	for len(rendered) < contentHeight {
		rendered = append(rendered, "")
	}

	content := strings.Join(rendered, "\n")

	// Add scroll indicator
	scrollInfo := p.scrollIndicator(totalLines, height)
	scrollInfo = p.searchFooterInfo(scrollInfo)
	content += "\n" + p.newDimStyle().Render(scrollInfo)

	return content
}

// formatBlameAnnotation formats the blame prefix for a single line.
// Format: "abc1234 AuthorName 2024-01 │ "
func formatBlameAnnotation(bl git.BlameLine) string {
	hash := bl.Hash
	if len(hash) > blameHashLen {
		hash = hash[:blameHashLen]
	}

	author := bl.Author
	if len(author) > blameAuthorMaxLen {
		author = author[:blameAuthorMaxLen]
	}

	date := bl.Date.Format("2006-01")

	return fmt.Sprintf("%-7s %-*s %s │ ", hash, blameAuthorMaxLen, author, date)
}

// blameRecencyColor returns a hex color string based on commit age.
// More recent commits are brighter; older commits are dimmer.
func blameRecencyColor(commitDate, now time.Time) string {
	age := now.Sub(commitDate)
	switch {
	case age < 7*24*time.Hour:
		// Less than 1 week: bright white
		return colorWhite
	case age < 30*24*time.Hour:
		// Less than 1 month: light gray
		return "#CCCCCC"
	case age < 180*24*time.Hour:
		// Less than 6 months: medium gray
		return "#999999"
	case age < 365*24*time.Hour:
		// Less than 1 year: dim gray
		return "#777777"
	default:
		// More than 1 year: very dim
		return "#555555"
	}
}
