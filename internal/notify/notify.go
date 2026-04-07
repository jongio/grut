package notify

import (
	"image/color"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Level represents the severity of a notification.
type Level int

const (
	// Info is for informational messages.
	Info Level = iota
	// Warn is for warning messages that may need attention.
	Warn
	// Error is for error messages requiring user attention.
	Error
	// Success is for positive confirmation messages.
	Success
)

// String returns the human-readable name for a notification level.
func (l Level) String() string {
	switch l {
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Success:
		return "OK"
	default:
		return "UNKNOWN"
	}
}

// DefaultToastDuration is the default auto-dismiss time for toasts.
const DefaultToastDuration = 3 * time.Second

// maxInlineNotifications is the upper bound on persistent inline entries.
// When exceeded, the oldest entry is evicted to prevent unbounded growth.
const maxInlineNotifications = 50

// Notification holds the data for a single notification.
type Notification struct {
	CreatedAt time.Time
	Message   string
	Level     Level
	Duration  time.Duration
}

// Manager manages active notifications across all three tiers:
// toasts (auto-dismiss), inline (persistent), and modals (blocking).
// It lives in the root app model and is not owned by individual panels.
type Manager struct {
	inlines      map[string]*inlineNotification
	modal        *modalState
	toasts       []toast
	nextID       int64
	screenWidth  int // terminal width (set via SetSize)
	screenHeight int // terminal height (set via SetSize)
	mu           sync.RWMutex
}

// NewManager creates a new notification manager with no active notifications.
func NewManager() *Manager {
	return &Manager{
		inlines: make(map[string]*inlineNotification),
	}
}

// AddToast adds an auto-dismissing toast notification with the default
// duration (3 seconds). The toast will be removed automatically after
// the duration elapses.
func (m *Manager) AddToast(msg string, level Level) tea.Cmd {
	return m.AddToastWithDuration(msg, level, DefaultToastDuration)
}

// AddToastWithDuration adds an auto-dismissing toast notification with
// a custom duration.
func (m *Manager) AddToastWithDuration(msg string, level Level, d time.Duration) tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	t := toast{
		id: id,
		notification: Notification{
			Message:   msg,
			Level:     level,
			CreatedAt: time.Now(),
			Duration:  d,
		},
	}
	m.toasts = append(m.toasts, t)
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return ToastExpiredMsg{ID: id}
	})
}

// AddInline adds a persistent inline notification identified by the given
// id. If an inline notification with the same id already exists, it is
// replaced. When the cap is exceeded, the oldest entry is evicted.
func (m *Manager) AddInline(id, msg string, level Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inlines[id] = &inlineNotification{
		id: id,
		notification: Notification{
			Message:   msg,
			Level:     level,
			CreatedAt: time.Now(),
		},
	}
	// Evict oldest if over cap (defensive bound against unbounded growth).
	if len(m.inlines) > maxInlineNotifications {
		var oldestID string
		var oldestTime time.Time
		for k, v := range m.inlines {
			if oldestID == "" || v.notification.CreatedAt.Before(oldestTime) {
				oldestID = k
				oldestTime = v.notification.CreatedAt
			}
		}
		if oldestID != "" {
			delete(m.inlines, oldestID)
		}
	}
}

// DismissInline removes a persistent inline notification by id.
func (m *Manager) DismissInline(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inlines, id)
}

// HasModal returns true if a modal is currently being displayed.
func (m *Manager) HasModal() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.modal != nil
}

// Update processes Bubble Tea messages relevant to the notification
// system (toast expiry, modal input, etc.) and returns any resulting
// command.
func (m *Manager) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ToastExpiredMsg:
		m.mu.Lock()
		m.removeToast(msg.ID)
		m.mu.Unlock()
		return nil
	case ShowToastMsg:
		d := msg.Duration
		if d == 0 {
			d = DefaultToastDuration
		}
		return m.AddToastWithDuration(msg.Message, msg.Level, d)
	case ShowModalMsg:
		m.mu.Lock()
		m.modal = &modalState{
			title:         msg.Title,
			message:       msg.Message,
			placeholder:   msg.Placeholder,
			kind:          msg.Kind,
			input:         msg.Value,
			cursor:        len([]rune(msg.Value)),
			selected:      msg.Kind == ModalConfirm || msg.Kind == ModalConfirmWithCheckbox,
			checkboxLabel: msg.CheckboxLabel,
			actions:       msg.Actions,
			actionCursor:  0,
		}
		m.mu.Unlock()
		return nil
	case tea.KeyPressMsg:
		return m.updateModal(msg)
	case tea.MouseClickMsg:
		return m.updateModalMouseClick(msg)
	}
	return nil
}

// updateModal handles key presses when a modal is active.
func (m *Manager) updateModal(msg tea.KeyPressMsg) tea.Cmd {
	m.mu.Lock()
	md := m.modal
	m.mu.Unlock()
	if md == nil {
		return nil
	}
	return md.handleKey(m, msg)
}

// updateModalMouseClick handles a mouse click when a modal is active.
// The Manager does not store screen dimensions, so this method uses the
// private screenWidth/screenHeight fields that are set by SetSize.
func (m *Manager) updateModalMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	m.mu.Lock()
	md := m.modal
	w := m.screenWidth
	h := m.screenHeight
	m.mu.Unlock()
	if md == nil || w <= 0 || h <= 0 {
		return nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	return md.handleMouseClick(m, mouse.X, mouse.Y, w, h)
}

// SetSize stores the terminal dimensions so mouse clicks can be mapped
// to modal-relative coordinates. The caller (app model) must call this
// whenever a WindowSizeMsg is received.
func (m *Manager) SetSize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.screenWidth = width
	m.screenHeight = height
}

// removeToast removes a toast by its ID. Must be called with mu held.
func (m *Manager) removeToast(id int64) {
	for i, t := range m.toasts {
		if t.id == id {
			m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
			return
		}
	}
}

// dismissModal clears the current modal.
func (m *Manager) dismissModal() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modal = nil
}

// View renders all active notifications into a string suitable for
// overlaying on the main layout. Toasts are rendered at the top,
// inline notifications below them.
func (m *Manager) View(width int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if width <= 0 {
		return ""
	}
	var parts []string
	// Render toasts
	for _, t := range m.toasts {
		parts = append(parts, t.view(width))
	}
	// Render inline notifications
	for _, inl := range m.inlines {
		parts = append(parts, inl.view(width))
	}
	if len(parts) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ModalView renders the modal overlay if one is active. Returns empty
// string if no modal is displayed. The caller should render this on top
// of the main content.
func (m *Manager) ModalView(width, height int) string {
	m.mu.Lock()
	md := m.modal
	m.mu.Unlock()
	if md == nil {
		return ""
	}
	return md.view(width, height)
}

// ToastCount returns the number of active toasts.
func (m *Manager) ToastCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.toasts)
}

// InlineCount returns the number of active inline notifications.
func (m *Manager) InlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.inlines)
}

// ---------------------------------------------------------------------------
// Test helpers (unexported; used by in-package tests to avoid direct mutex
// access — F16)
// ---------------------------------------------------------------------------
// toastDuration returns the duration of the i-th toast.
func (m *Manager) toastDuration(i int) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toasts[i].notification.Duration
}

// toastID returns the ID of the i-th toast.
func (m *Manager) toastID(i int) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toasts[i].id
}

// toastIDs returns all toast IDs.
func (m *Manager) toastIDs() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int64, len(m.toasts))
	for i, t := range m.toasts {
		ids[i] = t.id
	}
	return ids
}

// toastMessage returns the message of the i-th toast.
func (m *Manager) toastMessage(i int) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toasts[i].notification.Message
}

// inlineMessage returns the message of the inline notification with the given ID.
// Returns an empty string if the ID is not found.
func (m *Manager) inlineMessage(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := m.inlines[id]
	if n == nil {
		return ""
	}
	return n.notification.Message
}

// inlineLevel returns the level of the inline notification with the given ID.
// Returns Info (zero value) if the ID is not found.
func (m *Manager) inlineLevel(id string) Level {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := m.inlines[id]
	if n == nil {
		return Info
	}
	return n.notification.Level
}

// levelColor returns the lipgloss color for a notification level.
func levelColor(l Level) color.Color {
	switch l {
	case Info:
		return lipgloss.Color("#5B9BD5") // blue
	case Warn:
		return lipgloss.Color("#FFC107") // yellow
	case Error:
		return lipgloss.Color("#FF5252") // red
	case Success:
		return lipgloss.Color("#4CAF50") // green
	default:
		return lipgloss.Color("#FFFFFF")
	}
}
