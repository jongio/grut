// Stash-related operations for the gitinfo panel.
package gitinfo

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/panels"
)

// stashSelectedCmd returns a Cmd that emits StashSelectedMsg for the
// stash entry under the cursor. Returns nil if not on stash tab or no item.
func (p *Panel) stashSelectedCmd() tea.Cmd {
	if p.activeTab != tabStash {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	if items[cursor].kind != kindStashEntry {
		return nil
	}
	s := items[cursor].stash
	idx := s.Index
	hash := s.Hash
	return func() tea.Msg {
		return panels.StashSelectedMsg{Index: idx, Hash: hash}
	}
}

func (p *Panel) renderStashEntry(item listItem, width int, isCursor bool) string {
	s := item.stash
	label := fmt.Sprintf("  stash@{%d}: %s", s.Index, panels.StripANSI(s.Message))
	// Truncate label to fit width.
	if len(label) > width {
		if width > 4 {
			label = label[:width-3] + "..."
		} else if width > 0 {
			label = label[:width]
		} else {
			label = ""
		}
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.Worktree))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(label)
}
