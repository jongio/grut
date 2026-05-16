// Remote-related operations for the gitinfo panel.
package gitinfo

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// selectedRemote returns the remote at the cursor, or nil.
func (p *Panel) selectedRemote() *git.Remote {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	if items[cursor].kind != kindRemote {
		return nil
	}
	r := items[cursor].remote
	return &r
}

// remoteSelectedCmd returns a Cmd that emits RemoteSelectedMsg for the
// remote under the cursor. Returns nil if not on remotes tab or no item.
func (p *Panel) remoteSelectedCmd() tea.Cmd {
	if p.activeTab != tabRemotes {
		return nil
	}
	r := p.selectedRemote()
	if r == nil {
		return nil
	}
	name := r.Name
	return func() tea.Msg {
		return panels.RemoteSelectedMsg{Name: name}
	}
}

func (p *Panel) doFetch() (panels.Panel, tea.Cmd) {
	r := p.selectedRemote()
	g := p.git
	ctx := p.ctx
	if r != nil {
		name := r.Name
		return p, tea.Batch(
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Fetching " + name + "...", Level: notify.Info}
			},
			func() tea.Msg {
				err := g.Fetch(ctx, git.FetchOpts{Remote: name, Prune: true})
				return opResultMsg{op: opFetched, name: name, err: err}
			},
		)
	}
	// Fetch all if not on a remote item.
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{Message: "Fetching all...", Level: notify.Info}
		},
		func() tea.Msg {
			err := g.Fetch(ctx, git.FetchOpts{All: true, Prune: true})
			return opResultMsg{op: opFetched, name: "all remotes", err: err}
		},
	)
}

func (p *Panel) renderRemote(item listItem, width int, isCursor bool) string {
	leftSide := "  " + panels.StripANSI(item.remote.Name)
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.RemoteC)).Bold(true)
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(leftSide)
}

func (p *Panel) renderRemoteSub(item listItem, width int, isCursor bool) string {
	leftSide := "    " + panels.StripANSI(item.text)
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.URL))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(leftSide)
}

// remoteToHTTPS is a package-local alias for git.RemoteToHTTPS.
func remoteToHTTPS(raw string) string { return git.RemoteToHTTPS(raw) }
