// Worktree-related operations for the gitinfo panel.
package gitinfo

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// selectedWorktree returns the worktree at the cursor, or nil.
func (p *Panel) selectedWorktree() *git.Worktree {
	item, ok := p.currentItem()
	if !ok {
		return nil
	}
	if item.kind != kindWorktree {
		return nil
	}
	wt := item.worktree
	return &wt
}

// worktreeSelectedCmd returns a Cmd that emits WorktreeSelectedMsg for the
// worktree under the cursor. Returns nil if not on worktrees tab or no item.
func (p *Panel) worktreeSelectedCmd() tea.Cmd {
	if p.activeTab != tabWorktrees {
		return nil
	}
	wt := p.selectedWorktree()
	if wt == nil {
		return nil
	}
	path := wt.Path
	branch := wt.Branch
	return func() tea.Msg {
		return panels.WorktreeSelectedMsg{Path: path, Branch: branch}
	}
}

func (p *Panel) requestWorktreeSwitch() (panels.Panel, tea.Cmd) {
	// Use pendingPath captured at double-click time if available (survives
	// cursor resets from async data reloads during modal display).
	var path string
	if p.pendingPath != "" {
		path = p.pendingPath
		p.pendingPath = ""
	} else {
		wt := p.selectedWorktree()
		if wt == nil {
			return p, nil
		}
		path = wt.Path
	}
	if p.cfg.WorktreeOpenMode == openModeNewTerminal {
		return p, func() tea.Msg {
			if err := panels.OpenInTerminal(p.ctx, path); err != nil {
				errMsg := err.Error()
				return notify.ShowToastMsg{Message: "Terminal error: " + errMsg, Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened terminal at " + path, Level: notify.Success}
		}
	}
	return p, func() tea.Msg {
		return opResultMsg{op: eventWorktreeSwitch, name: path}
	}
}

func (p *Panel) renderWorktree(item listItem, width int, isCursor bool) string {
	wt := item.worktree
	// Right side: branch + short hash — always shown.
	rightSide := ""
	if wt.Branch != "" {
		rightSide = " " + panels.StripANSI(wt.Branch)
	}
	short := wt.Head
	if len(short) > git.ShortHashLen {
		short = short[:git.ShortHashLen]
	}
	if short != "" {
		rightSide += " " + short
	}
	// Truncate path (left side) to fit, never truncate hash.
	prefix := "  "
	rightLen := lipgloss.Width(rightSide)
	pathWidth := width - len(prefix) - rightLen - 1
	path := wt.Path
	if pathWidth > 0 && len(path) > pathWidth {
		if pathWidth > 1 {
			path = path[:pathWidth-1] + "…"
		} else {
			path = path[:pathWidth]
		}
	} else if pathWidth <= 0 {
		path = ""
	}
	leftSide := prefix + path
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.Worktree))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

// worktreePath is an alias to the canonical implementation in the git package.
// See git.WorktreePath for the convention details.
func worktreePath(repoRoot, branch string) string {
	return git.WorktreePath(repoRoot, branch)
}
