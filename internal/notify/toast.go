package notify

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// toast is an auto-dismissing notification that displays briefly and
// then disappears. Each toast has a unique ID used for tracking expiry.
type toast struct {
	id           int64
	notification Notification
}

// view renders a single toast as a compact colored bar.
func (t *toast) view(width int) string {
	maxWidth := width
	if maxWidth > 60 {
		maxWidth = 60
	}
	if maxWidth < 10 {
		maxWidth = 10
	}

	color := levelColor(t.notification.Level)

	icon := levelIcon(t.notification.Level)

	label := fmt.Sprintf(" %s %s ", icon, t.notification.Message)

	style := lipgloss.NewStyle().
		Background(color).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		MaxWidth(maxWidth).
		Padding(0, 1)

	return style.Render(label)
}

// levelIcon returns a unicode icon for the notification level.
func levelIcon(l Level) string {
	switch l {
	case Info:
		return "ℹ"
	case Warn:
		return "⚠"
	case Error:
		return "✗"
	case Success:
		return "✓"
	default:
		return "•"
	}
}
