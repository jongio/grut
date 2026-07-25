// GitHub notifications: browse your notification threads, open them in the
// browser, and mark them read. Reuses the github client's ListNotifications
// and MarkRead. Notifications are account-wide, so unlike issues/PRs/etc they
// load all pages at once and do not use the cursor-based pagination machinery.
package gitinfo

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// ghNotificationItem holds display data for a single GitHub notification thread.
type ghNotificationItem struct {
	ThreadID     string // notification thread ID (used by MarkRead)
	RepoFullName string // "owner/repo"
	Title        string // subject title
	Type         string // subject type: "Issue", "PullRequest", "Release", ...
	Reason       string // notification reason: "mention", "review_requested", ...
	UpdatedAt    string // formatted update time
	HTMLURL      string // browsable GitHub web URL
	Unread       bool   // true when the thread is still unread
}

// ghNotificationsLoadedMsg carries the result of an async notifications load.
type ghNotificationsLoadedMsg struct {
	err           error
	notifications []ghNotificationItem
}

// notificationReadResultMsg carries the result of a mark-read operation.
type notificationReadResultMsg struct {
	threadID string
	err      error
}

// loadNotifications fetches unread notifications asynchronously.
func (p *Panel) loadNotifications() tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	ctx := p.ctx
	return guardedGitHubCmd("gitinfo.loadNotifications", func() tea.Msg {
		var result ghNotificationsLoadedMsg
		notifs, err := client.ListNotifications(ctx, &gh.NotificationListOptions{All: false})
		if err != nil {
			slog.Warn("github: fetch notifications failed", "err", err)
			result.err = err
			return result
		}
		for _, n := range notifs {
			subj := n.GetSubject()
			repo := n.GetRepository().GetFullName()
			result.notifications = append(result.notifications, ghNotificationItem{
				ThreadID:     n.GetID(),
				RepoFullName: repo,
				Title:        subj.GetTitle(),
				Type:         subj.GetType(),
				Reason:       n.GetReason(),
				UpdatedAt:    n.GetUpdatedAt().Format("Jan 2 15:04"),
				HTMLURL:      notificationHTMLURL(subj.GetURL(), repo),
				Unread:       n.GetUnread(),
			})
		}
		return result
	})
}

// handleNotificationsLoaded stores loaded notifications into the tab.
func (p *Panel) handleNotificationsLoaded(msg ghNotificationsLoadedMsg) (panels.Panel, tea.Cmd) {
	p.gh.notifLoading = false
	if msg.err != nil {
		p.gh.notifErr = msg.err
		p.tabItems[tabNotifications] = nil
		p.tabCursor[tabNotifications] = 0
		p.tabOffset[tabNotifications] = 0
		return p, nil
	}
	p.gh.notifErr = nil
	p.buildNotificationItems(msg.notifications)
	return p, nil
}

// buildNotificationItems constructs the listItem slice for the notifications tab.
func (p *Panel) buildNotificationItems(notifs []ghNotificationItem) {
	p.tabItems[tabNotifications] = nil
	for _, n := range notifs {
		p.tabItems[tabNotifications] = append(p.tabItems[tabNotifications], listItem{
			kind:  kindNotification,
			notif: n,
		})
	}
	p.clampNotificationCursor()
}

// clampNotificationCursor keeps the notifications cursor and viewport offset
// within the bounds of the current item list.
func (p *Panel) clampNotificationCursor() {
	items := p.tabItems[tabNotifications]
	if p.tabCursor[tabNotifications] >= len(items) {
		p.tabCursor[tabNotifications] = len(items) - 1
	}
	if p.tabCursor[tabNotifications] < 0 {
		p.tabCursor[tabNotifications] = 0
	}
	if p.tabOffset[tabNotifications] > p.tabCursor[tabNotifications] {
		p.tabOffset[tabNotifications] = p.tabCursor[tabNotifications]
	}
	if p.tabOffset[tabNotifications] < 0 {
		p.tabOffset[tabNotifications] = 0
	}
}

// doRefreshNotifications reloads the notifications list from the API.
func (p *Panel) doRefreshNotifications() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil {
		return p, nil
	}
	p.gh.notifLoading = true
	p.gh.notifErr = nil
	return p, p.loadNotifications()
}

// doMarkNotificationRead marks the selected notification thread as read.
func (p *Panel) doMarkNotificationRead() (panels.Panel, tea.Cmd) {
	items := p.tabItems[tabNotifications]
	cursor := p.tabCursor[tabNotifications]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	if item.kind != kindNotification || item.notif.ThreadID == "" {
		return p, nil
	}
	return p, p.markNotificationReadCmd(item.notif.ThreadID)
}

// markNotificationReadCmd returns a command that marks the given thread read.
func (p *Panel) markNotificationReadCmd(threadID string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	ctx := p.ctx
	return func() tea.Msg {
		err := client.MarkRead(ctx, threadID)
		return notificationReadResultMsg{threadID: threadID, err: err}
	}
}

// handleNotificationReadResult updates the row after a mark-read operation.
func (p *Panel) handleNotificationReadResult(msg notificationReadResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errText := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Mark read failed: " + errText, Level: notify.Error}
		}
	}
	p.markNotificationReadLocally(msg.threadID)
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Marked notification read", Level: notify.Success}
	}
}

// markNotificationReadLocally greys the matching row by clearing its unread
// flag. Returns true when a matching thread was found.
func (p *Panel) markNotificationReadLocally(threadID string) bool {
	items := p.tabItems[tabNotifications]
	for i := range items {
		if items[i].kind == kindNotification && items[i].notif.ThreadID == threadID {
			items[i].notif.Unread = false
			return true
		}
	}
	return false
}

// ghNotifCountStr returns the unread notification count for the tab bar.
func (p *Panel) ghNotifCountStr() string {
	unread := 0
	for _, it := range p.tabItems[tabNotifications] {
		if it.kind == kindNotification && it.notif.Unread {
			unread++
		}
	}
	return fmt.Sprintf("%d", unread)
}

// notificationHTMLURL converts a notification subject API URL into a browsable
// GitHub web URL. The subject URL is an api.github.com REST URL (e.g.
// ".../repos/o/r/issues/1" or ".../repos/o/r/pulls/1"); the paths for pulls and
// commits differ on the web. Types without a clean web mapping (releases,
// discussions, check suites) fall back to the repository page.
func notificationHTMLURL(apiURL, repoFullName string) string {
	repoURL := ""
	if repoFullName != "" {
		repoURL = "https://github.com/" + repoFullName
	}
	const apiPrefix = "https://api.github.com/repos/"
	if !strings.HasPrefix(apiURL, apiPrefix) {
		return repoURL
	}
	rest := strings.TrimPrefix(apiURL, apiPrefix) // "o/r/issues/1"
	switch {
	case strings.Contains(rest, "/issues/"):
		return "https://github.com/" + rest
	case strings.Contains(rest, "/pulls/"):
		return "https://github.com/" + strings.Replace(rest, "/pulls/", "/pull/", 1)
	case strings.Contains(rest, "/commits/"):
		return "https://github.com/" + strings.Replace(rest, "/commits/", "/commit/", 1)
	default:
		return repoURL
	}
}

// renderNotification renders a notification row:
// "  ● owner/repo  Subject title       PullRequest · review_requested  Jan 2 15:04"
// Read threads are greyed with a hollow marker.
func (p *Panel) renderNotification(item listItem, width int, isCursor bool) string {
	n := item.notif
	prefix := "  "
	var icon, fg string
	if n.Unread {
		icon = runDot
		fg = p.colors.Issue
	} else {
		icon = "○"
		fg = p.colors.Dim
	}
	left := icon
	if n.RepoFullName != "" {
		left += " " + panels.StripANSI(n.RepoFullName)
	}
	if n.Title != "" {
		left += "  " + panels.StripANSI(n.Title)
	}
	// Right side: "Type · reason" plus the update time.
	meta := n.Type
	if n.Reason != "" {
		if meta != "" {
			meta += " · " + n.Reason
		} else {
			meta = n.Reason
		}
	}
	rightSide := ""
	if meta != "" {
		rightSide += " " + meta
	}
	if n.UpdatedAt != "" {
		rightSide += "  " + n.UpdatedAt
	}
	maxLeft := width - lipgloss.Width(prefix) - lipgloss.Width(rightSide) - 1
	leftRunes := []rune(left)
	if maxLeft > 0 && len(leftRunes) > maxLeft {
		if maxLeft > 1 {
			left = string(leftRunes[:maxLeft-1]) + "…"
		} else {
			left = string(leftRunes[:maxLeft])
		}
	} else if maxLeft <= 0 {
		left = ""
	}
	leftSide := prefix + left
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}
