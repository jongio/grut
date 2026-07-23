// Branch-related operations for the gitinfo panel.
package gitinfo

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// selectedBranch returns the branch at the cursor, or nil.
func (p *Panel) selectedBranch() *git.Branch {
	item, ok := p.currentItem()
	if !ok {
		return nil
	}
	if item.kind != kindLocalBranch && item.kind != kindRemoteBranch {
		return nil
	}
	b := item.branch
	return &b
}

// branchSelectedCmd returns a Cmd that emits BranchSelectedMsg for the
// branch under the cursor. Returns nil if the cursor is not on a branch.
func (p *Panel) branchSelectedCmd() tea.Cmd {
	if p.activeTab != tabBranches {
		return nil
	}
	b := p.selectedBranch()
	if b == nil {
		return nil
	}
	name := b.Name
	return func() tea.Msg {
		return panels.BranchSelectedMsg{Name: name}
	}
}

// guessBranchRemoteURL returns the fetch URL of the first remote, used to
// construct a browser-openable URL for local branches.
func (p *Panel) guessBranchRemoteURL(_ git.Branch) string {
	if len(p.gitData.lastRemotes) > 0 {
		return p.gitData.lastRemotes[0].FetchURL
	}
	return ""
}

func (p *Panel) requestCheckout() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsCurrent {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Already on " + b.Name, Level: notify.Info}
		}
	}
	ref := b.Name
	if b.IsRemote {
		if idx := strings.IndexByte(ref, '/'); idx >= 0 {
			ref = ref[idx+1:]
		}
	}
	p.clearPending()
	p.pending = opBranchCheckout
	p.pendingName = ref
	return p, notify.ShowConfirm("Switch Branch", fmt.Sprintf("Switch to branch %q?", ref))
}

func (p *Panel) requestPull() (panels.Panel, tea.Cmd) {
	return p.requestBranchPull(false)
}

func (p *Panel) requestPullRebase() (panels.Panel, tea.Cmd) {
	return p.requestBranchPull(true)
}

func (p *Panel) requestBranchPull(rebase bool) (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot pull a remote branch", Level: notify.Warn}
		}
	}
	if b.Upstream == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No upstream for " + b.Name, Level: notify.Warn}
		}
	}
	title := "Pull Branch"
	prompt := fmt.Sprintf("Pull branch %q from %s?", b.Name, b.Upstream)
	op := opBranchPull
	if rebase {
		title = "Pull Branch with Rebase"
		prompt = fmt.Sprintf("Pull branch %q from %s with rebase?", b.Name, b.Upstream)
		op = opBranchPullRebase
	}
	p.clearPending()
	p.pending = op
	p.pendingName = b.Name
	p.pendingPath = b.Upstream
	return p, notify.ShowConfirm(title, prompt)
}

func (p *Panel) requestPush() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot push a remote branch", Level: notify.Warn}
		}
	}
	p.clearPending()
	p.pending = opBranchPush
	p.pendingName = b.Name
	p.pendingPath = b.Upstream
	if b.Upstream == "" {
		return p, notify.ShowConfirm("Push Branch", fmt.Sprintf("Push branch %q to origin and set upstream?", b.Name))
	}
	return p, notify.ShowConfirm("Push Branch", fmt.Sprintf("Push branch %q to %s?", b.Name, b.Upstream))
}

func (p *Panel) handleCheckoutDirty(msg checkoutDirtyMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		// Status check failed - try checkout anyway, git will report errors.
		g := p.git
		ctx := p.ctx
		ref := msg.ref
		return p, func() tea.Msg {
			err := g.Checkout(ctx, ref)
			return opResultMsg{op: opCheckout, name: ref, err: err}
		}
	}
	if msg.dirty {
		p.clearPending()
		p.pending = opBranchCheckoutStash
		p.pendingName = msg.ref
		return p, notify.ShowConfirm("Uncommitted Changes",
			fmt.Sprintf("You have uncommitted changes. Stash and switch to %q?", msg.ref))
	}
	// Clean working tree - proceed with checkout.
	g := p.git
	ctx := p.ctx
	ref := msg.ref
	return p, func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during checkout", "ref", ref, "panic", r)
			}
		}()
		err := g.Checkout(ctx, ref)
		return opResultMsg{op: opCheckout, name: ref, err: err}
	}
}

func (p *Panel) doRename() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot rename remote branch", Level: notify.Warn}
		}
	}
	p.clearPending()
	p.pending = opBranchRename
	p.pendingName = b.Name
	return p, notify.ShowInput("Rename Branch", b.Name)
}

func (p *Panel) renderBranch(item listItem, width int, isCursor bool) string {
	b := item.branch
	prefix := "  "
	if b.IsCurrent {
		prefix = "* "
	}
	// Build right side — sync metadata and hash are shown, never truncated.
	var rightParts []string
	if b.Ahead > 0 || b.Behind > 0 {
		syncText := branchSyncText(b)
		syncColor := p.colors.Dim
		switch {
		case b.Ahead > 0 && b.Behind > 0:
			syncColor = colorOrange
		case b.Ahead > 0:
			syncColor = colorGreen
		case b.Behind > 0:
			syncColor = colorYellow
		}
		rightParts = append(rightParts, lipgloss.NewStyle().Foreground(lipgloss.Color(syncColor)).Render(syncText))
	}
	if b.Upstream != "" {
		rightParts = append(rightParts, lipgloss.NewStyle().Foreground(lipgloss.Color(p.colors.Dim)).Render("→ "+b.Upstream))
	}
	rightSide := ""
	if len(rightParts) > 0 {
		rightSide = " " + strings.Join(rightParts, " ")
	}
	if b.Hash != "" {
		rightSide += " " + b.Hash
	}
	// Calculate available width for the name — truncate name, never hash.
	prefixLen := len(prefix)
	rightLen := lipgloss.Width(rightSide)
	nameWidth := width - prefixLen - rightLen - 1 // -1 for min gap
	name := panels.StripANSI(b.Name)
	if nameWidth > 0 && len(name) > nameWidth {
		if nameWidth > 1 {
			name = name[:nameWidth-1] + "…"
		} else {
			name = name[:nameWidth]
		}
	} else if nameWidth <= 0 {
		name = ""
	}
	leftSide := prefix + name
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	if b.IsCurrent {
		style = style.Foreground(lipgloss.Color(p.colors.Current)).Bold(true)
	} else if b.IsRemote {
		style = style.Foreground(lipgloss.Color(p.colors.Remote))
	} else {
		style = style.Foreground(lipgloss.Color(p.colors.Local))
	}
	return style.Render(line)
}

func branchSyncText(b git.Branch) string {
	var parts []string
	if b.Ahead > 0 && b.Behind > 0 {
		parts = append(parts, "⇕")
	}
	if b.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", b.Ahead))
	}
	if b.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", b.Behind))
	}
	return strings.Join(parts, " ")
}

// worktreePath computes the worktree directory for a branch following the
// convention: <parent>/.worktrees/<repo-name>/<branch-slug>
// currentBranch returns the name of the current (checked-out) branch, or branchMain as fallback.
func (p *Panel) currentBranch() string {
	for _, item := range p.tabItems[tabBranches] {
		if item.kind == kindLocalBranch && item.branch.IsCurrent {
			return item.branch.Name
		}
	}
	return branchMain
}
