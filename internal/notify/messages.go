// Package notify provides a notification system for the grut TUI.
// It supports three tiers: toasts (auto-dismiss), inline (persistent),
// and modal (blocking) notifications.
package notify

import "time"

// ToastTickMsg is sent on a regular interval to drive auto-dismiss timing
// for active toasts.
type ToastTickMsg struct {
	// Time is the time at which the tick fired.
	Time time.Time
}

// ToastExpiredMsg indicates that a specific toast has expired and should
// be removed from the display.
type ToastExpiredMsg struct {
	// ID uniquely identifies the toast that expired.
	ID int64
}

// ModalResultMsg is sent when a modal dialog is dismissed by the user.
// Accept is true if the user confirmed, false if they cancelled.
// Value holds the text input value for input modals.
// Remember is true when the checkbox was checked (ConfirmWithCheckbox only);
// it is always false when Accept is false.
type ModalResultMsg struct {
	Value    string
	Accept   bool
	Remember bool
}

// ShowToastMsg is a command message that any panel can produce to trigger
// a toast notification. The root model intercepts this and routes it to
// the notification manager.
type ShowToastMsg struct {
	Message  string
	Level    Level
	Duration time.Duration // 0 means use default (3s)
}

// ActionOption represents a selectable action in the action picker modal.
type ActionOption struct {
	ID    string // action ID (e.g., "checkout", "copy_name")
	Label string // human-readable label (e.g., "checkout", "copy name")
}

// ShowModalMsg is a command message that triggers a modal dialog.
// The root model intercepts this and routes it to the notification manager.
type ShowModalMsg struct {
	Title         string
	Message       string
	Placeholder   string         // used only for input modals
	Value         string         // initial value for input modals (pre-fill)
	CheckboxLabel string         // label for the checkbox (ConfirmWithCheckbox only)
	Actions       []ActionOption // list of actions (ModalActionPicker only)
	Kind          ModalKind      // confirm, input, confirmWithCheckbox, or actionPicker
}

// ModalKind distinguishes between confirmation and input modals.
type ModalKind int

const (
	// ModalConfirm displays a yes/no confirmation dialog.
	ModalConfirm ModalKind = iota
	// ModalInput displays a text input dialog.
	ModalInput
	// ModalConfirmWithCheckbox displays a yes/no confirmation dialog with
	// a "remember this choice" checkbox.
	ModalConfirmWithCheckbox
	// ModalActionPicker displays a selectable list of actions.
	ModalActionPicker
	// ModalActionPickerWithCheckbox displays a selectable list of actions
	// with a "remember this choice" checkbox.
	ModalActionPickerWithCheckbox
)
