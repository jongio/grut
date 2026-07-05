package gitstatus

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// cleanPreviewLoadedMsg carries the result of an async CleanPreview call.
type cleanPreviewLoadedMsg struct {
	err        error
	candidates []git.CleanCandidate
}

// cleanResultMsg carries the result of a Clean removal.
type cleanResultMsg struct {
	err   error
	count int
}

// openCleanOverlay activates the cleanup overlay and kicks off a dry-run
// preview of the files git clean would remove.
func (p *GitStatus) openCleanOverlay() (panels.Panel, tea.Cmd) {
	p.cleanActive = true
	p.cleanLoading = true
	p.cleanCursor = 0
	p.cleanOffset = 0
	p.cleanCandidates = nil
	p.cleanSelected = make(map[string]bool)
	return p, p.loadCleanPreviewCmd()
}

// closeCleanOverlay tears down the overlay without removing anything.
func (p *GitStatus) closeCleanOverlay() {
	p.cleanActive = false
	p.cleanLoading = false
	p.cleanCandidates = nil
	p.cleanSelected = make(map[string]bool)
	p.cleanCursor = 0
	p.cleanOffset = 0
}

// loadCleanPreviewCmd fetches the dry-run clean candidates asynchronously.
func (p *GitStatus) loadCleanPreviewCmd() tea.Cmd {
	ctx := p.ctx
	client := p.git
	includeIgnored := p.cleanIncludeIgnored
	return func() tea.Msg {
		candidates, err := client.CleanPreview(ctx, git.CleanOpts{IncludeIgnored: includeIgnored})
		return cleanPreviewLoadedMsg{candidates: candidates, err: err}
	}
}

// handleCleanPreviewLoaded stores preview results, preserving any selections
// for paths that are still present.
func (p *GitStatus) handleCleanPreviewLoaded(msg cleanPreviewLoadedMsg) (panels.Panel, tea.Cmd) {
	p.cleanLoading = false
	if msg.err != nil {
		p.closeCleanOverlay()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Clean preview failed: " + msg.err.Error(), Level: notify.Error}
		}
	}
	p.cleanCandidates = msg.candidates
	// Drop selections whose paths no longer appear in the candidate set.
	present := make(map[string]bool, len(msg.candidates))
	for _, c := range msg.candidates {
		present[c.Path] = true
	}
	for path := range p.cleanSelected {
		if !present[path] {
			delete(p.cleanSelected, path)
		}
	}
	if p.cleanCursor >= len(p.cleanCandidates) {
		p.cleanCursor = max(0, len(p.cleanCandidates)-1)
	}
	return p, nil
}

// handleCleanResult reacts to a completed removal: on success it closes the
// overlay and refreshes status, on failure it surfaces a toast.
func (p *GitStatus) handleCleanResult(msg cleanResultMsg) (panels.Panel, tea.Cmd) {
	p.closeCleanOverlay()
	if msg.err != nil {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Clean failed: " + msg.err.Error(), Level: notify.Error}
		}
	}
	p.invalidateDiffCaches()
	p.loading = true
	toast := func() tea.Msg {
		return notify.ShowToastMsg{
			Message: fmt.Sprintf("Removed %d untracked file(s)", msg.count),
			Level:   notify.Success,
		}
	}
	return p, tea.Batch(p.loadStatusCmd(), toast)
}

// handleCleanKey processes keys while the cleanup overlay is open.
func (p *GitStatus) handleCleanKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape", "q":
		p.closeCleanOverlay()
		return p, nil
	case "j", "down":
		if p.cleanCursor < len(p.cleanCandidates)-1 {
			p.cleanCursor++
			p.ensureCleanCursorVisible()
		}
	case "k", "up":
		if p.cleanCursor > 0 {
			p.cleanCursor--
			p.ensureCleanCursorVisible()
		}
	case " ", "space":
		p.toggleCleanSelection()
	case "a":
		p.toggleSelectAllClean()
	case "i":
		p.cleanIncludeIgnored = !p.cleanIncludeIgnored
		p.cleanLoading = true
		return p, p.loadCleanPreviewCmd()
	case "enter":
		return p.confirmClean()
	}
	return p, nil
}

func (p *GitStatus) ensureCleanCursorVisible() {
	p.cleanOffset = panels.EnsureCursorVisible(p.cleanCursor, p.cleanOffset, p.Height)
}

func (p *GitStatus) toggleCleanSelection() {
	if p.cleanCursor < 0 || p.cleanCursor >= len(p.cleanCandidates) {
		return
	}
	path := p.cleanCandidates[p.cleanCursor].Path
	if p.cleanSelected[path] {
		delete(p.cleanSelected, path)
	} else {
		p.cleanSelected[path] = true
	}
}

// toggleSelectAllClean selects every candidate, or clears the selection if
// everything is already selected.
func (p *GitStatus) toggleSelectAllClean() {
	if len(p.cleanCandidates) == 0 {
		return
	}
	if len(p.cleanSelected) == len(p.cleanCandidates) {
		p.cleanSelected = make(map[string]bool)
		return
	}
	p.cleanSelected = make(map[string]bool, len(p.cleanCandidates))
	for _, c := range p.cleanCandidates {
		p.cleanSelected[c.Path] = true
	}
}

// confirmClean asks for confirmation before removing the selected paths.
func (p *GitStatus) confirmClean() (panels.Panel, tea.Cmd) {
	n := len(p.cleanSelected)
	if n == 0 {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No files selected to clean", Level: notify.Info}
		}
	}
	p.clearPending()
	p.pendingOp = opClean
	return p, notify.ShowConfirm("Clean Untracked Files",
		fmt.Sprintf("Permanently remove %d selected file(s)? This cannot be undone.", n))
}

// cleanSelectedCmd removes the currently selected paths. Ignored files are
// only removed when at least one selected path is ignored, so git receives -x
// exactly when it is needed.
func (p *GitStatus) cleanSelectedCmd() tea.Cmd {
	paths := make([]string, 0, len(p.cleanSelected))
	includeIgnored := false
	for _, c := range p.cleanCandidates {
		if p.cleanSelected[c.Path] {
			paths = append(paths, c.Path)
			if c.Ignored {
				includeIgnored = true
			}
		}
	}
	ctx := p.ctx
	client := p.git
	return func() tea.Msg {
		err := client.Clean(ctx, git.CleanOpts{Paths: paths, IncludeIgnored: includeIgnored})
		return cleanResultMsg{err: err, count: len(paths)}
	}
}

// renderCleanOverlay draws the cleanup overlay: a header with counts, the
// selectable candidate list (untracked first, ignored separated), and a hint
// footer.
func (p *GitStatus) renderCleanOverlay(width, height int) string {
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.SectionHeader)).Bold(true).
		Render("Clean untracked files")

	if p.cleanLoading {
		return p.padOverlay([]string{title, "", p.styleDim.Render("Scanning working tree...")}, width, height)
	}
	if len(p.cleanCandidates) == 0 {
		return p.padOverlay([]string{title, "", p.styleStaged.Render("Nothing to clean")}, width, height)
	}

	untracked, ignored := 0, 0
	for _, c := range p.cleanCandidates {
		if c.Ignored {
			ignored++
		} else {
			untracked++
		}
	}
	ignoredHint := "off"
	if p.cleanIncludeIgnored {
		ignoredHint = "on"
	}
	counts := p.styleDim.Render(fmt.Sprintf(
		"Untracked: %d   Ignored: %d (%s, press i)   Selected: %d",
		untracked, ignored, ignoredHint, len(p.cleanSelected),
	))

	lines := []string{title, counts, ""}

	// Reserve the header (3 lines) and footer (2 lines) from the list window.
	listHeight := height - len(lines) - 2
	if listHeight < 1 {
		listHeight = 1
	}
	end := p.cleanOffset + listHeight
	if end > len(p.cleanCandidates) {
		end = len(p.cleanCandidates)
	}
	sepShown := false
	for i := p.cleanOffset; i < end; i++ {
		c := p.cleanCandidates[i]
		if c.Ignored && !sepShown {
			lines = append(lines, p.styleDim.Render("Ignored files:"))
			sepShown = true
		}
		lines = append(lines, p.renderCleanRow(c, i == p.cleanCursor, width))
	}

	footer := p.styleDim.Render("space select · a all · i ignored · enter remove · esc cancel")
	lines = append(lines, "", footer)
	return p.padOverlay(lines, width, height)
}

// renderCleanRow renders a single candidate row with a checkbox and cursor.
func (p *GitStatus) renderCleanRow(c git.CleanCandidate, atCursor bool, width int) string {
	cursor := "  "
	if atCursor {
		cursor = "▸ "
	}
	check := "○"
	if p.cleanSelected[c.Path] {
		check = "●"
	}
	style := p.styleUntracked
	if c.Ignored {
		style = p.styleDim
	}
	line := cursor + check + " " + c.Path
	rendered := style.Render(line)
	if atCursor {
		return lipgloss.NewStyle().Width(width).Background(lipgloss.Color(p.colors.CursorBg)).Render(rendered)
	}
	return lipgloss.NewStyle().Width(width).Render(rendered)
}

// padOverlay joins overlay lines and pads to the panel height.
func (p *GitStatus) padOverlay(lines []string, width, height int) string {
	empty := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < height {
		lines = append(lines, empty)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
