package notify

import (
	"container/list"
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/theme"
)

// maxVisibleToasts bounds the toast stack so bursts cannot consume the screen
// or retain an unbounded number of notifications. New toasts evict the oldest.
const maxVisibleToasts = 5

// toast is an auto-dismissing notification that displays briefly and
// then disappears. Each toast has a unique ID used for tracking expiry.
type toast struct {
	notification Notification
	id           int64
	expiresAt    time.Time
}

// toastQueue preserves insertion order while supporting constant-time removal
// by ID. Its size is bounded by maxVisibleToasts, so expiry scans are constant.
type toastQueue struct {
	order list.List
	byID  map[int64]*list.Element
}

func toastFromElement(element *list.Element) *toast {
	value, ok := element.Value.(*toast)
	if !ok {
		panic("notify: toast queue contains an invalid value")
	}
	return value
}

func newToastQueue() toastQueue {
	return toastQueue{byID: make(map[int64]*list.Element, maxVisibleToasts)}
}

func (q *toastQueue) add(t *toast) {
	element := q.order.PushBack(t)
	q.byID[t.id] = element
	if q.order.Len() > maxVisibleToasts {
		q.removeElement(q.order.Front())
	}
}

func (q *toastQueue) remove(id int64) bool {
	element, ok := q.byID[id]
	if !ok {
		return false
	}
	q.removeElement(element)
	return true
}

func (q *toastQueue) removeExpired(at time.Time) {
	for element := q.order.Front(); element != nil; {
		next := element.Next()
		t := toastFromElement(element)
		if !t.expiresAt.After(at) {
			q.removeElement(element)
		}
		element = next
	}
}

func (q *toastQueue) nearestExpiry() (time.Time, bool) {
	var nearest time.Time
	for element := q.order.Front(); element != nil; element = element.Next() {
		expiresAt := toastFromElement(element).expiresAt
		if nearest.IsZero() || expiresAt.Before(nearest) {
			nearest = expiresAt
		}
	}
	return nearest, !nearest.IsZero()
}

func (q *toastQueue) removeElement(element *list.Element) {
	t := toastFromElement(element)
	delete(q.byID, t.id)
	q.order.Remove(element)
}

func (q *toastQueue) len() int {
	return q.order.Len()
}

func (q *toastQueue) at(index int) *toast {
	element := q.order.Front()
	for range index {
		element = element.Next()
	}
	return toastFromElement(element)
}

func (q *toastQueue) ids() []int64 {
	ids := make([]int64, 0, q.order.Len())
	for element := q.order.Front(); element != nil; element = element.Next() {
		ids = append(ids, toastFromElement(element).id)
	}
	return ids
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
