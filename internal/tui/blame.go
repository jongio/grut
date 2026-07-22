package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
)

func (m Model) handleToggleBlame(msg panels.ToggleBlameMsg) (tea.Model, tea.Cmd) {
	if m.gitClient == nil {
		return m, nil
	}

	gc := m.gitClient
	path := msg.Path
	ctx := m.ctx
	return m, func() tea.Msg {
		lines, err := gc.Blame(ctx, path)
		return panels.BlameLoadedMsg{Lines: lines, Err: err}
	}
}
