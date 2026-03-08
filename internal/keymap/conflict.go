package keymap

import (
	"fmt"
	"log/slog"
)

// Conflict describes a duplicate key+mode+context combination where
// multiple actions are bound to the same key.
type Conflict struct {
	Key     string
	Mode    KeyMode
	Context string
	Actions []string
}

// String returns a human-readable description of the conflict.
func (c Conflict) String() string {
	ctx := c.Context
	if ctx == "" {
		ctx = "(all)"
	}
	return fmt.Sprintf("key %q in mode %s context %s: actions %v",
		c.Key, c.Mode, ctx, c.Actions)
}

// DetectConflicts finds duplicate key+mode+context combinations in bindings.
// A conflict exists when two or more bindings share the same key, mode, and
// context but map to different actions.
func DetectConflicts(bindings []Binding) []Conflict {
	type entry struct {
		mode    KeyMode
		context string
		key     string
	}

	seen := make(map[string][]string) // indexKey → actions
	order := make([]entry, 0)         // preserve discovery order

	for _, b := range bindings {
		ik := indexKey(b.Mode, b.Context, b.Key)
		if _, exists := seen[ik]; !exists {
			order = append(order, entry{mode: b.Mode, context: b.Context, key: b.Key})
		}
		seen[ik] = append(seen[ik], b.Action)
	}

	var conflicts []Conflict
	for _, e := range order {
		ik := indexKey(e.mode, e.context, e.key)
		actions := seen[ik]
		if len(actions) > 1 {
			conflicts = append(conflicts, Conflict{
				Key:     e.key,
				Mode:    e.mode,
				Context: e.context,
				Actions: actions,
			})
		}
	}

	return conflicts
}

// WarnConflicts detects conflicts in bindings and logs each one as a
// warning via slog.
func WarnConflicts(bindings []Binding) {
	for _, c := range DetectConflicts(bindings) {
		slog.Warn("duplicate key binding",
			"key", c.Key,
			"mode", c.Mode.String(),
			"context", c.Context,
			"actions", c.Actions,
		)
	}
}
