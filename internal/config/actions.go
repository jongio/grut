package config

import "github.com/jongio/grut/internal/actions"

// GetDoubleClickAction returns the configured double-click action for an
// item type, falling back to the registry default if no override is set.
func (c *ActionsConfig) GetDoubleClickAction(itemType string) string {
	if c.DoubleClick != nil {
		if action, ok := c.DoubleClick[itemType]; ok {
			return action
		}
	}
	return string(actions.DefaultAction(actions.ItemType(itemType)))
}

// IsConfirmed returns true if the user has confirmed (chosen "Always") for
// the given item type, meaning the first-use prompt should be skipped.
func (c *ActionsConfig) IsConfirmed(itemType string) bool {
	if c.Confirmed != nil {
		return c.Confirmed[itemType]
	}
	return false
}

// SetActionConfirmed persists the "always perform" flag for an item type.
func SetActionConfirmed(itemType string) error {
	return SaveUserSettingBool("actions.confirmed."+itemType, true)
}

// SetDoubleClickAction persists a double-click action override for an
// item type.
func SetDoubleClickAction(itemType, action string) error {
	return SaveUserSetting("actions.double_click."+itemType, action)
}

// SaveDoubleClickChoice persists the user's double-click action choice and
// updates the in-memory config so subsequent double-clicks use it immediately.
func SaveDoubleClickChoice(cfg *ActionsConfig, itemType, actionID string) {
	_ = SetDoubleClickAction(itemType, actionID)
	_ = SetActionConfirmed(itemType)
	if cfg.DoubleClick == nil {
		cfg.DoubleClick = make(map[string]string)
	}
	cfg.DoubleClick[itemType] = actionID
	if cfg.Confirmed == nil {
		cfg.Confirmed = make(map[string]bool)
	}
	cfg.Confirmed[itemType] = true
}

// ResetAllActionConfirmations sets all confirmed flags to false by
// iterating every registered item type.
func ResetAllActionConfirmations() error {
	for _, it := range actions.AllItemTypes() {
		key := "actions.confirmed." + string(it)
		if err := SaveUserSettingBool(key, false); err != nil {
			return err
		}
	}
	return nil
}

// GetRightClickAction returns the configured right-click action for an item
// type, falling back to the registry default if no override is set.
func (c *ActionsConfig) GetRightClickAction(it actions.ItemType) actions.ActionID {
	if c.RightClick != nil {
		if override, ok := c.RightClick[string(it)]; ok {
			id := actions.ActionID(override)
			if actions.IsValidAction(it, id) || id == actions.ActionShowContextMenu {
				return id
			}
		}
	}
	return actions.RightClickAction(it)
}

// SetRightClickAction persists a right-click action override for an item type.
func SetRightClickAction(it actions.ItemType, action actions.ActionID) error {
	return SaveUserSetting("actions.right_click."+string(it), string(action))
}
