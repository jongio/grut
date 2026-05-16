// Reflog-related operations for the gitinfo panel.
package gitinfo

import (
	"fmt"
	"time"
	
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

func (p *Panel) doReflogCheckout() (panels.Panel, tea.Cmd) {
	items := p.tabItems[tabReflog]
	cursor := p.tabCursor[tabReflog]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	hash := item.reflog.Hash
	if len(hash) > 10 {
		hash = hash[:10]
	}
	p.clearPending()
	p.pending = opBranchCheckout
	p.pendingName = item.reflog.Hash
	return p, notify.ShowConfirm("Checkout Reflog Entry", fmt.Sprintf("Checkout %s (%s)?", hash, item.reflog.Message))
}

func (p *Panel) renderReflogEntry(item listItem, width int, isCursor bool) string {
	r := item.reflog
	hash := r.Hash
	if len(hash) > git.ShortHashLen {
		hash = hash[:git.ShortHashLen]
	}
	age := reflogRelativeDate(r.Date)
	label := fmt.Sprintf("  %s %s %s (%s)", hash, panels.StripANSI(r.Action), panels.StripANSI(r.Message), age)
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
		Foreground(lipgloss.Color(p.colors.Dim))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(label)
}

// reflogRelativeDate formats a time.Time as a short relative date string.
func reflogRelativeDate(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}

