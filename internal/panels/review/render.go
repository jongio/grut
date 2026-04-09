package review

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/git"
)

// ---------------------------------------------------------------------------
// Line rendering
// ---------------------------------------------------------------------------

func (p *Panel) rebuildLines() {
	p.lines = nil
	p.hunkLineStarts = nil

	switch p.mode {
	case modeFileList:
		p.buildFileListLines()
	case modeDiff:
		p.buildDiffLines()
	}
}

func (p *Panel) buildFileListLines() {
	lines := make([]string, 0, 2+len(p.files))
	lines = append(lines, p.headerStyle().Render("Review Changes"))
	lines = append(lines, "")

	for i, f := range p.files {
		icon := fileStatusIcon(f)
		status := fileStatusText(f)

		cursor := "  "
		if i == p.fileCursor {
			cursor = "▸ "
		}

		style := p.dimStyle()
		if i == p.fileCursor {
			style = p.selectedStyle()
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, icon, f.Path, status)
		lines = append(lines, style.Render(line))
	}

	p.lines = lines
}

func (p *Panel) buildDiffLines() {
	if p.fileCursor >= len(p.files) {
		return
	}

	f := p.files[p.fileCursor]
	var lines []string
	var hunkStarts []int

	// File header
	header := fmt.Sprintf("── %s ──", f.Path)
	lines = append(lines, p.headerStyle().Render(header))
	lines = append(lines, "")

	if f.Diff.IsBinary {
		lines = append(lines, p.dimStyle().Render("Binary file differs"))
		p.lines = lines
		p.hunkLineStarts = nil
		return
	}

	if len(f.Diff.Hunks) == 0 {
		lines = append(lines, p.dimStyle().Render("No changes"))
		p.lines = lines
		p.hunkLineStarts = nil
		return
	}

	for i, hunk := range f.Diff.Hunks {
		hunkStarts = append(hunkStarts, len(lines))

		// Hunk header with state indicator and cursor
		stateIcon := "○"         //nolint:goconst // inline string is more readable here
		switch f.HunkStates[i] { //nolint:exhaustive // only relevant cases handled
		case HunkApproved:
			stateIcon = "✓"
		case HunkRejected:
			stateIcon = "✗"
		}

		cursor := "  "
		if i == p.hunkCursor {
			cursor = "▸ "
		}

		hunkHeader := fmt.Sprintf("%sHunk %d/%d [%s] %s",
			cursor, i+1, len(f.Diff.Hunks), stateIcon, hunk.Header)

		style := p.headerStyle()
		if i == p.hunkCursor {
			style = p.selectedStyle()
		}
		lines = append(lines, style.Render(hunkHeader))

		// Diff lines within hunk
		for _, dl := range hunk.Lines {
			var rendered string
			switch dl.Type {
			case git.DiffLineAdded:
				rendered = p.addedStyle().Render("+ " + dl.Content)
			case git.DiffLineRemoved:
				rendered = p.removedStyle().Render("- " + dl.Content)
			default:
				rendered = p.contextStyle().Render("  " + dl.Content)
			}
			lines = append(lines, rendered)
		}

		lines = append(lines, "") // blank separator between hunks
	}

	p.lines = lines
	p.hunkLineStarts = hunkStarts
}

// ---------------------------------------------------------------------------
// Viewport
// ---------------------------------------------------------------------------

func (p *Panel) renderViewport(_ int, height int) string {
	if len(p.lines) == 0 {
		return ""
	}

	contentHeight := height
	if contentHeight < 1 {
		contentHeight = 1
	}

	p.clampScroll(contentHeight)

	start := p.scrollY
	end := start + contentHeight
	if end > len(p.lines) {
		end = len(p.lines)
	}

	visible := append(make([]string, 0, contentHeight), p.lines[start:end]...)

	for len(visible) < contentHeight {
		visible = append(visible, "")
	}

	return strings.Join(visible, "\n")
}

func (p *Panel) renderSummaryView(_ int, height int) string {
	if p.summary == "" {
		p.summary = GenerateSummary(p.files)
	}

	summaryLines := strings.Split(p.summary, "\n")
	contentHeight := height
	if contentHeight < 1 {
		contentHeight = 1
	}

	visible := summaryLines
	if len(visible) > contentHeight {
		visible = visible[:contentHeight]
	}

	for len(visible) < contentHeight {
		visible = append(visible, "")
	}

	return strings.Join(visible, "\n")
}

// ---------------------------------------------------------------------------
// Scroll helpers
// ---------------------------------------------------------------------------

func (p *Panel) ensureFileVisible() {
	if p.Height <= 0 {
		return
	}
	// File list lines: header (0), blank (1), then files at index 2+.
	cursorLine := p.fileCursor + 2
	contentHeight := p.Height

	if cursorLine < p.scrollY {
		p.scrollY = cursorLine
	}
	if cursorLine >= p.scrollY+contentHeight {
		p.scrollY = cursorLine - contentHeight + 1
	}
}

func (p *Panel) ensureHunkVisible() {
	if len(p.hunkLineStarts) == 0 || p.hunkCursor >= len(p.hunkLineStarts) {
		return
	}
	p.scrollY = p.hunkLineStarts[p.hunkCursor]
}

func (p *Panel) clampScroll(viewportHeight int) {
	maxScroll := len(p.lines) - viewportHeight
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

// ---------------------------------------------------------------------------
// Styles (fallback colors matching gitdiff panel defaults)
// ---------------------------------------------------------------------------

func (p *Panel) headerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(colorOf(p.themeColors().BrightBlue, "#7A9EBF"))
}

func (p *Panel) selectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(colorOf(p.themeColors().Foreground, "#D4D4D4"))
}

func (p *Panel) addedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorOf(p.themeColors().DiffAdded, "#6B9E56"))
}

func (p *Panel) removedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorOf(p.themeColors().DiffRemoved, "#C44B4B"))
}

func (p *Panel) contextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorOf(p.themeColors().DiffContext, "#999999"))
}

func (p *Panel) dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorOf(p.themeColors().BrightBlack, "#666666"))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fileStatusIcon returns the review status indicator for a file.
func fileStatusIcon(f ReviewFile) string {
	if len(f.HunkStates) == 0 {
		return "○"
	}
	allApproved := true
	allRejected := true
	hasReviewed := false
	for _, s := range f.HunkStates {
		if s != HunkApproved {
			allApproved = false
		}
		if s != HunkRejected {
			allRejected = false
		}
		if s != HunkPending {
			hasReviewed = true
		}
	}
	if allApproved {
		return "✓"
	}
	if allRejected {
		return "✗"
	}
	if hasReviewed {
		return "◐"
	}
	return "○"
}

// fileStatusText returns a human-readable review progress string.
func fileStatusText(f ReviewFile) string {
	total := len(f.HunkStates)
	if total == 0 {
		return ""
	}
	reviewed := 0
	for _, s := range f.HunkStates {
		if s != HunkPending {
			reviewed++
		}
	}
	if reviewed == 0 {
		return fmt.Sprintf("(%d hunks)", total)
	}
	return fmt.Sprintf("(%d/%d reviewed)", reviewed, total)
}
