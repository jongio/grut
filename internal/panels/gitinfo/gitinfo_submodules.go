package gitinfo

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
)

func (p *Panel) selectedSubmodule() *git.Submodule {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	item := items[cursor]
	if item.kind != kindSubmodule {
		return nil
	}
	sm := item.submodule
	return &sm
}

func (p *Panel) submoduleSelectedCmd() tea.Cmd {
	if p.activeTab != tabSubmodules {
		return nil
	}
	sm := p.selectedSubmodule()
	if sm == nil {
		return nil
	}
	path := sm.Path
	commit := sm.Commit
	state := sm.State()
	describe := sm.Describe
	return func() tea.Msg {
		return panels.SubmoduleSelectedMsg{Path: path, Commit: commit, State: state, Describe: describe}
	}
}

func (p *Panel) renderSubmodule(item listItem, width int, isCursor bool) string {
	sm := item.submodule
	prefix := "  "
	shortCommit := sm.Commit
	if len(shortCommit) > git.ShortHashLen {
		shortCommit = shortCommit[:git.ShortHashLen]
	}
	icon, color := submoduleStateIcon(sm, p.colors.Hash)
	stateText := icon + " " + sm.State()
	rightSide := stateText + " " + shortCommit
	prefixWidth := lipgloss.Width(prefix)
	rightWidth := lipgloss.Width(rightSide)
	pathWidth := width - prefixWidth - rightWidth - 1
	path := truncateDisplay(panels.StripANSI(sm.Path), pathWidth)
	leftSide := prefix + path
	usedWidth := lipgloss.Width(leftSide) + rightWidth
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	stateRendered := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(stateText)
	commitRendered := lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Hash)).Render(shortCommit)
	line := leftSide + gap + stateRendered + " " + commitRendered
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.Worktree))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

func submoduleStateIcon(sm git.Submodule, fallback string) (string, string) {
	switch {
	case sm.Conflicted:
		return "⚠", colorYellow
	case !sm.Initialized:
		return "○", fallback
	case sm.Modified:
		return "●", colorOrange
	default:
		return "✓", colorGreen
	}
}

func truncateDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
