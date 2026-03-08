package chat

import (
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// PendingConfirmation holds a tool call that requires user approval
// before execution.
type PendingConfirmation struct {
	Call        ai.ToolCall
	Description string // Human-readable description of what will happen
}

// ConfirmationManager handles the approval workflow for destructive
// tool calls. Only one confirmation may be pending at a time.
type ConfirmationManager struct {
	registry *ToolRegistry
	pending  *PendingConfirmation
}

// NewConfirmationManager creates a manager that uses the given registry
// to classify tool calls as safe or destructive.
func NewConfirmationManager(registry *ToolRegistry) *ConfirmationManager {
	return &ConfirmationManager{registry: registry}
}

// Check examines a tool call and returns:
//   - (nil, true) if the tool is safe and can execute immediately
//   - (pending, false) if the tool requires confirmation
//   - (nil, false) if the tool is unknown
func (m *ConfirmationManager) Check(call ai.ToolCall) (*PendingConfirmation, bool) {
	info, ok := m.registry.Get(call.Name)
	if !ok {
		return nil, false
	}

	if info.Safety == Safe {
		return nil, true
	}

	pc := &PendingConfirmation{
		Call:        call,
		Description: describeToolCall(call),
	}
	m.pending = pc
	return pc, false
}

// Pending returns the current pending confirmation, or nil.
func (m *ConfirmationManager) Pending() *PendingConfirmation {
	return m.pending
}

// Accept approves the pending confirmation and returns the tool call.
// Clears the pending state. Returns nil if nothing is pending.
func (m *ConfirmationManager) Accept() *ai.ToolCall {
	if m.pending == nil {
		return nil
	}
	call := m.pending.Call
	m.pending = nil
	return &call
}

// Reject denies the pending confirmation and clears the pending state.
// Returns a description of what was rejected, or an empty string if
// nothing was pending.
func (m *ConfirmationManager) Reject() string {
	if m.pending == nil {
		return ""
	}
	desc := m.pending.Description
	m.pending = nil
	return desc
}

// Clear removes any pending confirmation without accepting or rejecting.
func (m *ConfirmationManager) Clear() {
	m.pending = nil
}

// HasPending reports whether there is a confirmation waiting.
func (m *ConfirmationManager) HasPending() bool {
	return m.pending != nil
}

// describeToolCall generates a human-readable description of a
// destructive tool call by extracting relevant arguments.
func describeToolCall(call ai.ToolCall) string {
	switch call.Name {
	case "file_delete": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Delete %q", argStr(call, "path"))

	case "file_write": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Overwrite %q", argStr(call, "path"))

	case "file_rename": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Rename %q → %q", argStr(call, "old_path"), argStr(call, "new_path"))

	case "git_branch_delete": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Delete branch %q", argStr(call, "name"))

	case "git_rebase": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Rebase onto %q", argStr(call, "onto"))

	case "git_reset": //nolint:goconst // inline string is more readable here
		desc := fmt.Sprintf("Reset to %q", argStr(call, "ref"))
		if argBool(call, "hard") {
			desc += " (hard)"
		}
		return desc

	case "git_push": //nolint:goconst // inline string is more readable here
		remote := argStr(call, "remote")
		if remote == "" {
			remote = "origin" //nolint:goconst // inline string is more readable here
		}
		if argBool(call, "force") {
			return fmt.Sprintf("Force push to %q", remote)
		}
		return fmt.Sprintf("Push to %q", remote)

	case "git_tag_delete": //nolint:goconst // inline string is more readable here
		return fmt.Sprintf("Delete tag %q", argStr(call, "name"))

	case "git_discard": //nolint:goconst // inline string is more readable here
		paths := argStrSlice(call, "paths")
		return fmt.Sprintf("Discard changes in %d files", len(paths))

	case "bulk_delete": //nolint:goconst // inline string is more readable here
		paths := argStrSlice(call, "paths")
		return fmt.Sprintf("Delete %d files", len(paths))

	case "bulk_rename": //nolint:goconst // inline string is more readable here
		renames := argSlice(call, "renames")
		return fmt.Sprintf("Rename %d files", len(renames))

	default:
		return fmt.Sprintf("Execute %s", call.Name)
	}
}

// argStr extracts a string argument from a tool call. Returns "" if
// the key is missing or not a string.
func argStr(call ai.ToolCall, key string) string {
	v, ok := call.Arguments[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// argBool extracts a boolean argument from a tool call. Returns false
// if the key is missing or not a bool.
func argBool(call ai.ToolCall, key string) bool {
	v, ok := call.Arguments[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// argStrSlice extracts a []string argument from a tool call. Handles
// both []string and []any (the latter is common when JSON-decoded).
func argStrSlice(call ai.ToolCall, key string) []string { //nolint:unparam // key parameterized for future use
	v, ok := call.Arguments[key]
	if !ok {
		return nil
	}

	// Direct []string
	if ss, ok := v.([]string); ok {
		return ss
	}

	// JSON-decoded []any
	items, ok := v.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// argSlice extracts a []any argument from a tool call. Returns nil if
// the key is missing or not a slice.
func argSlice(call ai.ToolCall, key string) []any {
	v, ok := call.Arguments[key]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	return items
}

// FormatConfirmationPrompt returns a user-facing prompt string for the
// pending confirmation, suitable for display in a terminal.
func FormatConfirmationPrompt(pc *PendingConfirmation) string {
	if pc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\uF071  ")
	b.WriteString(pc.Description)
	b.WriteString("\n\nProceed? [y/N] ")
	return b.String()
}
