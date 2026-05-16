package notify

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// inlineNotification is a persistent notification that stays visible
// until explicitly dismissed by its ID. Used for errors or warnings
// that require user attention (e.g., "git not found").
type inlineNotification struct {
	id           string
	notification Notification
}

// inlineBaseStyle holds the invariant parts of an inline notification.
// Per-render code only sets Foreground (level-dependent) and Width.
var inlineBaseStyle = lipgloss.NewStyle().Bold(true)

// view renders an inline notification as a full-width colored bar.
func (n *inlineNotification) view(width int) string {
	if width <= 0 {
		return ""
	}

	color := levelColor(n.notification.Level)
	icon := levelIcon(n.notification.Level)
	label := fmt.Sprintf(" %s %s ", icon, n.notification.Message)

	style := inlineBaseStyle.
		Foreground(color).
		Width(width)

	return style.Render(label)
}
