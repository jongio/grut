// Package aiconflict implements a three-way diff panel with AI-powered
// conflict resolution suggestions. It displays base/ours/theirs content
// for each conflict region alongside the AI's suggested resolution, and
// lets the user accept the AI suggestion, pick ours/theirs, or navigate
// between regions.
//
// The panel defines its own view-model types (ConflictFileData,
// ConflictRegionData) to avoid importing internal/ai, which has a
// circular dependency. The caller converts from ai types when sending
// SetConflictsMsg.
package aiconflict

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// ---------------------------------------------------------------------------
// View-model types (avoid importing internal/ai to prevent import cycle)
// ---------------------------------------------------------------------------
// ConflictFileData holds display data for one conflicted file.
// The caller converts from ai.ConflictFile + ops.ConflictResolution when
// building the SetConflictsMsg.
type ConflictFileData struct {
	// Path is the repository-relative file path.
	Path string
	// Regions contains the conflict regions within this file.
	Regions []ConflictRegionData
}

// ConflictRegionData holds display data for a single conflict region,
// including the AI resolution if available.
type ConflictRegionData struct {
	// Ours is the text from the current branch.
	Ours string
	// Theirs is the text from the incoming branch.
	Theirs string
	// Base is the text from the common ancestor (may be empty).
	Base string
	// AIResolution is the AI-suggested resolution text. Empty when no AI
	// resolution is available for this region.
	AIResolution string
	// Explanation describes why the AI chose this resolution.
	Explanation string
	// StartLine is the 1-based line where the conflict begins.
	StartLine int
	// EndLine is the 1-based line where the conflict ends (inclusive).
	EndLine int
	// Confidence is a 0.0–1.0 score for the AI's certainty.
	Confidence float64
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------
// SetConflictsMsg delivers conflict data to the panel. The root model
// converts from ai.ConflictFile / ops.ConflictResolution into these
// view-model types before sending.
type SetConflictsMsg struct {
	Files []ConflictFileData
}

// ---------------------------------------------------------------------------
// Resolution choice constants
// ---------------------------------------------------------------------------
const (
	choiceAI     = "ai"
	choiceOurs   = "ours"
	choiceTheirs = "theirs"
)

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------
// Panel is the AI conflict resolution panel. It shows a three-way diff
// (base/ours/theirs) with AI-suggested resolutions and lets the user
// accept or override each region's resolution.
type Panel struct {
	ctx   context.Context
	theme *theme.Theme
	// resolved tracks user choices: filePath -> regionIndex -> choiceXxx.
	resolved map[string]map[int]string
	files    []ConflictFileData
	panels.BasePanel
	currentFile   int // index into files
	currentRegion int // index into current file's regions
	scrollY       int // scroll offset within view
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// New creates a new AI conflict panel. The theme may be nil; the panel
// falls back to hard-coded colors when it is.
func New(th *theme.Theme) *Panel {
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "aiconflict"},
		theme:     th,
		resolved:  make(map[string]map[int]string),
	}
}

// ---------------------------------------------------------------------------
// Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	return nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------
// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case SetConflictsMsg:
		return p.handleSetConflicts(msg)
	case tea.KeyPressMsg:
		if p.Focused {
			return p.handleKey(msg)
		}
	}
	return p, nil
}

func (p *Panel) handleSetConflicts(msg SetConflictsMsg) (panels.Panel, tea.Cmd) {
	p.files = msg.Files
	p.currentFile = 0
	p.currentRegion = 0
	p.scrollY = 0
	p.resolved = make(map[string]map[int]string)
	return p, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.Code {
	case 'j', tea.KeyDown:
		p.scrollY++
	case 'k', tea.KeyUp:
		if p.scrollY > 0 {
			p.scrollY--
		}
	case 'n':
		return p.nextRegion()
	case 'p':
		return p.prevRegion()
	case 'a':
		return p.acceptAI()
	case 'o':
		return p.chooseOurs()
	case 't':
		return p.chooseTheirs()
	case 'e':
		// Edit — not yet implemented; no-op.
		return p, nil
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
// regionCount returns the number of conflict regions in the current file.
func (p *Panel) regionCount() int {
	if len(p.files) == 0 || p.currentFile >= len(p.files) {
		return 0
	}
	return len(p.files[p.currentFile].Regions)
}

func (p *Panel) nextRegion() (panels.Panel, tea.Cmd) {
	if len(p.files) == 0 {
		return p, nil
	}
	rc := p.regionCount()
	if p.currentRegion+1 < rc {
		p.currentRegion++
		p.scrollY = 0
		return p, nil
	}
	// Move to next file.
	if p.currentFile+1 < len(p.files) {
		p.currentFile++
		p.currentRegion = 0
		p.scrollY = 0
	}
	return p, nil
}

func (p *Panel) prevRegion() (panels.Panel, tea.Cmd) {
	if len(p.files) == 0 {
		return p, nil
	}
	if p.currentRegion > 0 {
		p.currentRegion--
		p.scrollY = 0
		return p, nil
	}
	// Move to previous file's last region.
	if p.currentFile > 0 {
		p.currentFile--
		rc := p.regionCount()
		if rc > 0 {
			p.currentRegion = rc - 1
		} else {
			p.currentRegion = 0
		}
		p.scrollY = 0
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Resolution choices
// ---------------------------------------------------------------------------
func (p *Panel) storeChoice(choice string) tea.Cmd {
	if len(p.files) == 0 || p.currentFile >= len(p.files) {
		return nil
	}
	path := p.files[p.currentFile].Path
	if p.resolved[path] == nil {
		p.resolved[path] = make(map[int]string)
	}
	p.resolved[path][p.currentRegion] = choice
	// Check if all regions in this file are resolved.
	rc := p.regionCount()
	if rc > 0 && len(p.resolved[path]) == rc {
		filePath := path
		return func() tea.Msg {
			return panels.AIConflictResolvedMsg{Path: filePath}
		}
	}
	return nil
}

func (p *Panel) acceptAI() (panels.Panel, tea.Cmd) {
	// Only allow if we have an AI resolution for this region.
	if !p.hasAIResolution() {
		return p, nil
	}
	return p, p.storeChoice(choiceAI)
}

func (p *Panel) chooseOurs() (panels.Panel, tea.Cmd) {
	return p, p.storeChoice(choiceOurs)
}

func (p *Panel) chooseTheirs() (panels.Panel, tea.Cmd) {
	return p, p.storeChoice(choiceTheirs)
}

// hasAIResolution returns whether the current region has an AI resolution.
func (p *Panel) hasAIResolution() bool {
	if len(p.files) == 0 || p.currentFile >= len(p.files) {
		return false
	}
	regions := p.files[p.currentFile].Regions
	if p.currentRegion >= len(regions) {
		return false
	}
	return regions[p.currentRegion].AIResolution != ""
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------
// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if len(p.files) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(p.successHex())).
			Render("No conflicts")
	}
	lines := p.buildLines(width)
	// Apply scroll offset.
	if p.scrollY >= len(lines) {
		p.scrollY = max(0, len(lines)-1)
	}
	visible := lines[p.scrollY:]
	// Reserve 1 line for key hints at bottom.
	contentHeight := height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	if len(visible) > contentHeight {
		visible = visible[:contentHeight]
	}
	// Pad to fill viewport.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(visible) < contentHeight {
		visible = append(visible, emptyLine)
	}
	// Append key hints.
	visible = append(visible, p.renderKeyHints(width))
	return strings.Join(visible, "\n")
}

// buildLines constructs the full rendered content for the current
// file/region, returning one string per line.
func (p *Panel) buildLines(width int) []string {
	var lines []string
	cf := p.files[p.currentFile]
	rc := p.regionCount()
	// Header: file path and file counter.
	fileHeader := fmt.Sprintf("  File %d/%d: %s",
		p.currentFile+1, len(p.files), cf.Path)
	lines = append(lines, lipgloss.NewStyle().
		Width(width).Bold(true).
		Foreground(lipgloss.Color(p.headerHex())).
		Render(fileHeader))
	// Region indicator.
	regionLabel := fmt.Sprintf("  Region %d of %d", p.currentRegion+1, rc)
	// Show resolution status for this region.
	if choice, ok := p.resolved[cf.Path][p.currentRegion]; ok {
		regionLabel += fmt.Sprintf("  [resolved: %s]", choice)
	}
	lines = append(lines, lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color(p.regionHex())).
		Render(regionLabel))
	lines = append(lines, "") // blank separator
	// Guard: no regions available.
	if rc == 0 || p.currentRegion >= rc {
		lines = append(lines, "  (no conflict regions)")
		return lines
	}
	region := cf.Regions[p.currentRegion]
	// --- Ours section ---
	lines = append(lines, p.renderSectionHeader(width, "OURS", p.oursHex(),
		p.resolved[cf.Path][p.currentRegion] == choiceOurs))
	lines = append(lines, p.renderCodeBlock(width, region.Ours, p.oursHex())...)
	lines = append(lines, "") // separator
	// --- Theirs section ---
	lines = append(lines, p.renderSectionHeader(width, "THEIRS", p.theirsHex(),
		p.resolved[cf.Path][p.currentRegion] == choiceTheirs))
	lines = append(lines, p.renderCodeBlock(width, region.Theirs, p.theirsHex())...)
	lines = append(lines, "") // separator
	// --- AI Suggestion section ---
	if region.AIResolution != "" {
		isChosen := p.resolved[cf.Path][p.currentRegion] == choiceAI
		lines = append(lines, p.renderSectionHeader(width, "AI SUGGESTION", p.aiHex(), isChosen))
		lines = append(lines, p.renderCodeBlock(width, region.AIResolution, p.aiHex())...)
		// Explanation and confidence.
		if region.Explanation != "" {
			explLine := fmt.Sprintf("  Explanation: %s", region.Explanation)
			lines = append(lines, lipgloss.NewStyle().
				Width(width).Italic(true).
				Foreground(lipgloss.Color(p.dimHex())).
				Render(explLine))
		}
		confLine := fmt.Sprintf("  Confidence: %.0f%%", region.Confidence*100)
		lines = append(lines, lipgloss.NewStyle().
			Width(width).
			Foreground(lipgloss.Color(p.dimHex())).
			Render(confLine))
	} else {
		lines = append(lines, lipgloss.NewStyle().
			Width(width).Italic(true).
			Foreground(lipgloss.Color(p.dimHex())).
			Render("  AI SUGGESTION — not available"))
	}
	return lines
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------
// renderSectionHeader renders a section header like "▸ OURS" or "● OURS".
func (p *Panel) renderSectionHeader(width int, title, hex string, chosen bool) string {
	prefix := "  ▸ "
	if chosen {
		prefix = "  ● "
	}
	style := lipgloss.NewStyle().Width(width).Bold(true).
		Foreground(lipgloss.Color(hex))
	if chosen {
		style = style.Underline(true)
	}
	return style.Render(prefix + title)
}

// renderCodeBlock splits content into lines and renders each with an
// indent and the given color.
func (p *Panel) renderCodeBlock(width int, content, hex string) []string {
	if content == "" {
		return []string{lipgloss.NewStyle().
			Width(width).Italic(true).
			Foreground(lipgloss.Color(p.dimHex())).
			Render("    (empty)")}
	}
	style := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color(hex))
	raw := strings.TrimRight(content, "\n")
	split := strings.Split(raw, "\n")
	out := make([]string, 0, len(split))
	for _, l := range split {
		out = append(out, style.Render("    "+l))
	}
	return out
}

// renderKeyHints renders the bottom key-hint bar.
func (p *Panel) renderKeyHints(width int) string {
	hints := "  a:AI  o:ours  t:theirs  n:next  p:prev  j/k:scroll"
	style := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color(p.hintHex())).
		Background(lipgloss.Color(p.hintBgHex()))
	return style.Render(hints)
}

// ---------------------------------------------------------------------------
// Color helpers — prefer theme, fall back to hard-coded Dracula palette
// ---------------------------------------------------------------------------
func (p *Panel) headerHex() string {
	if p.theme != nil && p.theme.Colors.DiffHeader != "" {
		return p.theme.Colors.DiffHeader
	}
	return "#C9A227"
}

func (p *Panel) regionHex() string {
	if p.theme != nil && p.theme.Colors.DiffHunk != "" {
		return p.theme.Colors.DiffHunk
	}
	return "#555555"
}

func (p *Panel) oursHex() string {
	if p.theme != nil && p.theme.Colors.DiffAdded != "" {
		return p.theme.Colors.DiffAdded
	}
	return "#6B9E56"
}

func (p *Panel) theirsHex() string {
	if p.theme != nil && p.theme.Colors.DiffRemoved != "" {
		return p.theme.Colors.DiffRemoved
	}
	return "#C44B4B"
}

func (p *Panel) aiHex() string {
	if p.theme != nil && p.theme.Colors.NormalCyan != "" {
		return p.theme.Colors.NormalCyan
	}
	return "#5E8E8B"
}

func (p *Panel) dimHex() string {
	if p.theme != nil && p.theme.Colors.BrightBlack != "" {
		return p.theme.Colors.BrightBlack
	}
	return "#555555"
}

func (p *Panel) successHex() string {
	if p.theme != nil && p.theme.Colors.NotifySuccess != "" {
		return p.theme.Colors.NotifySuccess
	}
	return "#6B9E56"
}

func (p *Panel) hintHex() string {
	if p.theme != nil && p.theme.Colors.StatusBarFg != "" {
		return p.theme.Colors.StatusBarFg
	}
	return "#D4D4D4"
}

func (p *Panel) hintBgHex() string {
	if p.theme != nil && p.theme.Colors.StatusBarBg != "" {
		return p.theme.Colors.StatusBarBg
	}
	return "#2A2A2A"
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------
// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "a", Description: "Accept AI suggestion", Action: "accept_ai"},
		{Key: "o", Description: "Use ours version", Action: "choose_ours"},
		{Key: "t", Description: "Use theirs version", Action: "choose_theirs"},
		{Key: "e", Description: "Edit resolution (not yet implemented)", Action: "edit"},
		{Key: "n", Description: "Next conflict region", Action: "next_region"},
		{Key: "p", Description: "Previous conflict region", Action: "prev_region"},
		{Key: "j/k", Description: "Scroll view", Action: "scroll"},
	}
}
