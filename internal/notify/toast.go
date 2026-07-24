package notify

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/theme"
)

// toast is an auto-dismissing notification that displays briefly and
// then disappears. Each toast has a unique ID used for tracking expiry.
type toast struct {
	notification Notification
	id           int64
}

// toastBaseStyle holds the invariant parts of a toast badge. Per-render
// code only sets Background (level-dependent) and MaxWidth (width-dependent).
var toastBaseStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Bold(true).
	Padding(0, 1)

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
	style := toastBaseStyle.
		Background(color).
		MaxWidth(maxWidth)
	return style.Render(label)
}

// levelIcon returns a unicode icon for the notification level.
func levelIcon(l Level) string {
	switch l {
	case Info:
		return theme.StatusMarker(theme.StatusInfo)
	case Warn:
		return theme.StatusMarker(theme.StatusWarning)
	case Error:
		return theme.StatusMarker(theme.StatusError)
	case Success:
		return theme.StatusMarker(theme.StatusSuccess)
	default:
		return theme.StatusMarker("")
	}
}
