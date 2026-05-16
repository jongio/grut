// Package commitrender provides a shared commit-line renderer used by both the
// commits panel (flat list) and the gitlog panel (graph + progressive columns).
// Extracting this logic avoids duplicating the same layout, truncation, and
// highlight behaviour across panels.
package commitrender

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/mattn/go-runewidth"
)

// AuthorColMaxWidth is the maximum rune-width used for the author column
// in the log list view. Wider names are truncated to keep the layout compact.
const AuthorColMaxWidth = 14

// columnGap is the separator placed between columns.
const columnGap = "  "

// minSubjectWidth is the lower bound for the subject column so it is never
// squeezed into an unreadable sliver.
const minSubjectWidth = 10

// Styles caches lipgloss.Style objects so RenderLine does not allocate new
// styles on every call (~7,200 allocations/sec at 20 visible commits × 60 fps).
type Styles struct {
	Hash    lipgloss.Style
	Date    lipgloss.Style
	Author  lipgloss.Style
	Subject lipgloss.Style
	Ref     lipgloss.Style
	Graph   lipgloss.Style
	Cursor  lipgloss.Style
}

// Params configures a single call to [RenderLine].
type Params struct {
	Commit git.Commit
	Width  int

	// IsCursor is true when this row is the cursor row.
	IsCursor bool

	// GraphPrefix is the pre-rendered graph characters for this line.
	// Pass an empty string when graph rendering is not used.
	GraphPrefix string

	Styles Styles

	// ShowRefs enables inline ref display (branch/tag names after the subject).
	ShowRefs bool

	// ShowAuthor makes the author column eligible for display if width allows.
	ShowAuthor bool

	// ShowDate makes the date column eligible for display if width allows.
	ShowDate bool

	// IsSelected highlights the row with a subtler background when true and
	// IsCursor is false (used by the commits panel for the "selected commit"
	// whose files are currently shown).
	IsSelected bool

	// SelectedBg is the background color used when IsSelected is true but
	// IsCursor is false. Ignored when empty.
	SelectedBg string
}

// RenderLine formats a single commit as a fixed-width terminal line.
//
// Layout (left to right, progressive):
//
//	[graph]  subject [(refs)]  [date]  [author]  hash
//
// Columns in square brackets appear only when the corresponding Show* flag is
// true AND the available width is large enough.
func RenderLine(p Params) string {
	c := p.Commit
	s := p.Styles

	// ---- right-side fixed columns ----
	hashCol := panels.StripANSI(c.ShortHash)
	hashW := len(hashCol)

	authorCol := panels.StripANSI(c.Author)
	if runewidth.StringWidth(authorCol) > AuthorColMaxWidth {
		authorCol = runewidth.Truncate(authorCol, AuthorColMaxWidth, "")
	}
	dateCol := c.Date.Format("2006-01-02")

	graphW := lipgloss.Width(p.GraphPrefix)
	if graphW > 0 {
		graphW += len(columnGap) // gap after graph
	}

	// Progressive column visibility: always subject+hash; medium adds
	// author; wide adds date. A disabled column is never shown regardless
	// of width.
	baseUsed := graphW + minSubjectWidth + len(columnGap) + hashW
	showAuthor := p.ShowAuthor && baseUsed+len(columnGap)+len(authorCol) <= p.Width
	showDate := p.ShowDate && showAuthor && baseUsed+len(columnGap)+len(authorCol)+len(columnGap)+len(dateCol) <= p.Width

	// Build the right-side string (everything after subject).
	var rb strings.Builder
	if showDate {
		rb.WriteString(s.Date.Render(dateCol))
		rb.WriteString(columnGap)
	}
	if showAuthor {
		rb.WriteString(s.Author.Render(authorCol))
		rb.WriteString(columnGap)
	}
	rb.WriteString(s.Hash.Render(hashCol))
	rightSide := rb.String()
	rightW := lipgloss.Width(rightSide)

	// ---- subject + refs ----
	subjectSpace := p.Width - graphW - len(columnGap) - rightW
	if subjectSpace < minSubjectWidth {
		subjectSpace = minSubjectWidth
	}

	// Sanitise untrusted git data to prevent ANSI escape-sequence injection
	// (CWE-150).
	safeSubject := panels.StripANSI(c.Subject)
	var safeRefs []string
	if p.ShowRefs && len(c.Refs) > 0 {
		safeRefs = make([]string, len(c.Refs))
		for i, r := range c.Refs {
			safeRefs[i] = panels.StripANSI(r)
		}
	}

	subjectText := safeSubject
	if len(safeRefs) > 0 {
		subjectText += " (" + strings.Join(safeRefs, ", ") + ")"
	}

	// Truncate or pad subject to fill its allotted space.
	subjectVisW := runewidth.StringWidth(subjectText)
	var styledSubject string
	if subjectVisW > subjectSpace {
		subjectText = runewidth.Truncate(subjectText, subjectSpace, "")
		if len(safeRefs) > 0 && strings.Contains(subjectText, "(") {
			styledSubject = styleSubjectWithRefs(subjectText, s.Subject, s.Ref)
		} else {
			styledSubject = s.Subject.Render(subjectText)
		}
	} else {
		if len(safeRefs) > 0 {
			styledSubject = s.Subject.Render(safeSubject) + " " + s.Ref.Render("("+strings.Join(safeRefs, ", ")+")")
			styledVisW := lipgloss.Width(styledSubject)
			if styledVisW < subjectSpace {
				styledSubject += strings.Repeat(" ", subjectSpace-styledVisW)
			}
		} else {
			styledSubject = s.Subject.Render(subjectText)
			if subjectVisW < subjectSpace {
				styledSubject += strings.Repeat(" ", subjectSpace-subjectVisW)
			}
		}
	}

	// ---- assemble line ----
	var lb strings.Builder
	if graphW > 0 {
		lb.WriteString(s.Graph.Render(p.GraphPrefix))
		lb.WriteString(columnGap)
	}
	lb.WriteString(styledSubject)
	lb.WriteString(columnGap)
	lb.WriteString(rightSide)
	line := lb.String()

	// ---- cursor / selection highlight ----
	if p.IsCursor {
		line = s.Cursor.Width(p.Width).Render(line)
	} else if p.IsSelected && p.SelectedBg != "" {
		selStyle := lipgloss.NewStyle().Background(lipgloss.Color(p.SelectedBg))
		line = selStyle.Width(p.Width).Render(line)
	}

	return TruncateOrPad(line, p.Width)
}

// TruncateOrPad ensures a rendered string fits exactly the given width.
func TruncateOrPad(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		return lipgloss.NewStyle().MaxWidth(width).Render(s)
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// styleSubjectWithRefs handles the case where subject text includes a
// truncated refs portion — style everything before "(" as subject and
// everything from "(" onward as ref.
func styleSubjectWithRefs(text string, subjectStyle, refStyle lipgloss.Style) string {
	idx := strings.Index(text, "(")
	if idx < 0 {
		return subjectStyle.Render(text)
	}
	return subjectStyle.Render(text[:idx]) + refStyle.Render(text[idx:])
}
