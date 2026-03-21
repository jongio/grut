// Package keymap provides a configurable key-dispatch system for grut.
//
// The keymap loads keybinding definitions from TOML scheme files (embedded
// or user-supplied), tracks the current input mode (global/panel/input), and
// dispatches key events to action strings. It does NOT execute actions —
// the root model's Update() maps action strings to actual behaviour.
package keymap

import (
	"fmt"
	"strings"
)

// KeyMode represents the current input mode for key dispatch.
type KeyMode int

const (
	// ModeGlobal means only global bindings are active.
	ModeGlobal KeyMode = iota
	// ModePanel means global + panel bindings are active.
	ModePanel
	// ModeInput means global + input bindings are active.
	ModeInput
)

// String returns the string representation of a KeyMode.
func (m KeyMode) String() string {
	switch m {
	case ModeGlobal:
		return "global"
	case ModePanel:
		return "panel"
	case ModeInput:
		return "input"
	default:
		return fmt.Sprintf("KeyMode(%d)", int(m))
	}
}

// parseKeyMode converts a TOML mode string to a KeyMode value.
func parseKeyMode(s string) (KeyMode, error) {
	switch strings.ToLower(s) {
	case "global":
		return ModeGlobal, nil
	case "panel":
		return ModePanel, nil
	case "input":
		return ModeInput, nil
	default:
		return 0, fmt.Errorf("unknown key mode %q", s)
	}
}

// Binding describes a single key binding with its mode, context, and action.
type Binding struct {
	Key         string  // Key combination, e.g. "ctrl+c", "j", "ctrl+b z"
	Action      string  // Internal action identifier, e.g. "quit", "cursor_down"
	Context     string  // Panel name for context-specific bindings, or "" for all
	Description string  // Human-readable description
	Mode        KeyMode // When this binding is active
}

// indexKey returns the composite string used to index a binding by
// mode, context, and key combination.
func indexKey(mode KeyMode, context, key string) string {
	return fmt.Sprintf("%d:%s:%s", int(mode), context, key)
}

// Keymap holds all key bindings and dispatches key events to action strings.
type Keymap struct {
	index         map[string]string // indexKey → action
	prefixes      map[string]bool   // known multi-key prefixes (e.g. "ctrl+b")
	pendingPrefix string            // buffered first key of a multi-key sequence
	bindings      []Binding
	mode          KeyMode
}

// NewKeymap creates a Keymap by loading the named scheme.
// The scheme can be "default", "classic", "vim", or a filesystem path.
func NewKeymap(scheme string) (*Keymap, error) {
	bindings, err := LoadScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("loading scheme %q: %w", scheme, err)
	}
	return NewKeymapFromBindings(bindings), nil
}

// NewKeymapFromBindings creates a Keymap from a pre-loaded set of bindings.
func NewKeymapFromBindings(bindings []Binding) *Keymap {
	km := &Keymap{
		bindings: bindings,
		mode:     ModePanel,
		index:    make(map[string]string),
		prefixes: make(map[string]bool),
	}
	km.buildIndex()
	return km
}

// buildIndex constructs the lookup index and prefix set from bindings.
func (km *Keymap) buildIndex() {
	for _, b := range km.bindings {
		km.index[indexKey(b.Mode, b.Context, b.Key)] = b.Action
		// Extract prefixes for multi-key sequences like "ctrl+b z".
		parts := strings.Split(b.Key, " ")
		for i := 1; i < len(parts); i++ {
			prefix := strings.Join(parts[:i], " ")
			km.prefixes[prefix] = true
		}
	}
}

// Dispatch finds the action for a key press in the given context.
// Returns the action string and whether a binding was found.
//
// Dispatch priority:
//  1. Global bindings (always checked)
//  2. Context-specific panel/input bindings (matching context)
//  3. General panel/input bindings (empty context)
func (km *Keymap) Dispatch(key, context string) (action string, handled bool) {
	// Build the full key including any pending prefix.
	fullKey := key
	if km.pendingPrefix != "" {
		fullKey = km.pendingPrefix + " " + key
		km.pendingPrefix = ""
	}
	// Check if this key starts a multi-key sequence.
	if km.prefixes[fullKey] {
		km.pendingPrefix = fullKey
		return "", false
	}
	// Priority 1: Global bindings.
	if a, ok := km.index[indexKey(ModeGlobal, "", fullKey)]; ok {
		return a, true
	}
	// Priority 2: Mode-specific bindings.
	switch km.mode { //nolint:exhaustive // only relevant cases handled
	case ModePanel:
		// Context-specific bindings first.
		if context != "" {
			if a, ok := km.index[indexKey(ModePanel, context, fullKey)]; ok {
				return a, true
			}
		}
		// General panel bindings (no context).
		if a, ok := km.index[indexKey(ModePanel, "", fullKey)]; ok {
			return a, true
		}
	case ModeInput:
		if a, ok := km.index[indexKey(ModeInput, "", fullKey)]; ok {
			return a, true
		}
	}
	return "", false
}

// SetMode switches the current input mode. Any pending multi-key prefix
// is discarded on mode change.
func (km *Keymap) SetMode(mode KeyMode) {
	km.mode = mode
	km.pendingPrefix = ""
}

// CurrentMode returns the active key mode.
func (km *Keymap) CurrentMode() KeyMode {
	return km.mode
}

// Bindings returns all loaded bindings.
func (km *Keymap) Bindings() []Binding {
	return km.bindings
}

// ClearPending discards any buffered multi-key prefix.
func (km *Keymap) ClearPending() {
	km.pendingPrefix = ""
}

// HasPending reports whether a multi-key prefix is buffered.
func (km *Keymap) HasPending() bool {
	return km.pendingPrefix != ""
}
