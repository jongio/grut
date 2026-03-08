// Package rightclick provides a shared helper for building context menu
// commands from the action registry and user configuration.
package rightclick

import (
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"

	tea "charm.land/bubbletea/v2"
)

// Cmd builds the tea.Cmd for a right-click on the given item type.
// If the user has configured a direct action override, it returns nil and
// the action ID so the caller can execute it inline.
// If the default context_menu action applies, it returns a command to show
// the action picker modal.
func Cmd(cfg config.ActionsConfig, itemType actions.ItemType, label string) (tea.Cmd, actions.ActionID) {
	action := cfg.GetRightClickAction(itemType)
	if action != actions.ActionShowContextMenu {
		return nil, action
	}

	// Build action list from registry.
	ids := actions.AllRightClickActions(itemType)
	opts := make([]notify.ActionOption, 0, len(ids))
	for _, id := range ids {
		if id == actions.ActionShowContextMenu {
			continue
		}
		opts = append(opts, notify.ActionOption{
			ID:    string(id),
			Label: actions.ActionLabel(id),
		})
	}
	if len(opts) == 0 {
		return nil, ""
	}
	return notify.ShowActionPicker(label, opts), ""
}

// FirstUseCmd returns a tea.Cmd that shows an action picker with a
// "remember" checkbox for first-use double-click configuration. The picker
// lists all available actions for the item type with the default
// pre-selected (listed first).
func FirstUseCmd(it actions.ItemType) tea.Cmd {
	allActs := actions.AllActions(it)
	if len(allActs) == 0 {
		return nil
	}
	opts := make([]notify.ActionOption, len(allActs))
	for i, a := range allActs {
		opts[i] = notify.ActionOption{ID: string(a), Label: actions.ActionLabel(a)}
	}
	return notify.ShowActionPickerWithCheckbox(
		"Double-Click: "+actions.ItemLabel(it),
		"Choose what happens when you double-click",
		"Always do this action on double click",
		opts,
	)
}
