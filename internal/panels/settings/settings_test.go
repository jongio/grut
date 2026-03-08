package settings

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/layout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testThemes = []string{"default", "catppuccin", "tokyonight", "gruvbox"}

func TestNew(t *testing.T) {
	t.Run("stores current position and theme", func(t *testing.T) {
		p := New(layout.PreviewBottom, "catppuccin", testThemes, config.ActionsConfig{})
		assert.Equal(t, fieldPreviewPosition, p.cursorIndex())
		assert.Equal(t, layout.PreviewBottom, p.currentPosition())
		assert.Equal(t, "catppuccin", p.currentThemeName())
	})

	t.Run("title is settings", func(t *testing.T) {
		p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
		assert.Equal(t, "settings", p.Title())
	})
}

func TestNavigationKeys(t *testing.T) {
	t.Run("j moves cursor down", func(t *testing.T) {
		p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
		assert.Equal(t, fieldPreviewPosition, p.cursorIndex())

		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		panel := updated.(*Panel)
		assert.Equal(t, fieldTheme, panel.cursorIndex())
	})

	t.Run("k moves cursor up", func(t *testing.T) {
		p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
		// Move to theme first.
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		panel := updated.(*Panel)
		assert.Equal(t, fieldTheme, panel.cursorIndex())

		updated, _ = panel.Update(tea.KeyPressMsg{Code: 'k'})
		panel = updated.(*Panel)
		assert.Equal(t, fieldPreviewPosition, panel.cursorIndex())
	})

	t.Run("cursor does not go below last field", func(t *testing.T) {
		p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
		// Navigate to the very last field.
		lastField := p.fieldCount() - 1
		for p.cursorIndex() < lastField {
			updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
			p = updated.(*Panel)
		}
		assert.Equal(t, lastField, p.cursorIndex())

		// Try to go further.
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		panel := updated.(*Panel)
		assert.Equal(t, lastField, panel.cursorIndex())
	})

	t.Run("cursor does not go above first field", func(t *testing.T) {
		p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
		assert.Equal(t, fieldPreviewPosition, p.cursorIndex())

		updated, _ := p.Update(tea.KeyPressMsg{Code: 'k'})
		panel := updated.(*Panel)
		assert.Equal(t, fieldPreviewPosition, panel.cursorIndex())
	})
}

func TestEnterCyclesPreviewPosition(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter should produce a command")

	msg := cmd()
	posMsg, ok := msg.(SetPreviewPositionMsg)
	require.True(t, ok, "command should return SetPreviewPositionMsg")
	assert.Equal(t, layout.PreviewBottom, posMsg.Position, "should cycle Right→Bottom")
}

func TestEnterCyclesTheme(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	// Move to theme field.
	updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
	panel := updated.(*Panel)

	_, cmd := panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter should produce a command")

	msg := cmd()
	themeMsg, ok := msg.(SetThemeMsg)
	require.True(t, ok, "command should return SetThemeMsg")
	assert.Equal(t, "catppuccin", themeMsg.Name, "should cycle default→catppuccin")
}

func TestEscEmitsToggleSettingsMsg(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "escape should produce a command")

	msg := cmd()
	_, ok := msg.(ToggleSettingsMsg)
	assert.True(t, ok, "command should return ToggleSettingsMsg")
}

func TestViewRendersNonEmpty(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	view := p.View(60, 40)
	assert.NotEmpty(t, view, "view should produce output")
	assert.Contains(t, view, "Preview Position", "view should show preview section")
	assert.Contains(t, view, "Theme", "view should show theme section")
	assert.Contains(t, view, "Right", "view should show current preview position")
	assert.Contains(t, view, "default", "view should show current theme")
}

func TestViewReturnsEmptyForZeroDimensions(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(0, 0))
}

func TestKeyBindingsReturnsBindings(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	bindings := p.KeyBindings()
	assert.Len(t, bindings, 4)
}

func TestInitReturnsNil(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

func TestCycleThemeWraps(t *testing.T) {
	p := New(layout.PreviewRight, "gruvbox", testThemes, config.ActionsConfig{})
	// Move to theme field.
	updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
	panel := updated.(*Panel)

	_, cmd := panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd()
	themeMsg := msg.(SetThemeMsg)
	assert.Equal(t, "default", themeMsg.Name, "should wrap gruvbox→default")
}

// ---------------------------------------------------------------------------
// Double-Click Actions tests
// ---------------------------------------------------------------------------

func TestDoubleClickActionsDisplayed(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	view := p.View(60, 80)
	assert.Contains(t, view, "Double-Click Actions", "view should show double-click actions heading")
	assert.Contains(t, view, "Confirmations", "view should show confirmations heading")
	assert.Contains(t, view, "Reset all prompts", "view should show reset prompts option")

	// Should contain at least one configurable item label.
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items, "there should be configurable items")
	label := actions.ItemLabel(items[0])
	assert.Contains(t, view, label, "view should show first configurable item label")
}

func TestCycleDoubleClickAction(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	// Navigate to the first action field.
	for p.cursorIndex() < fieldActionsStart {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}
	assert.Equal(t, fieldActionsStart, p.cursorIndex())

	// Press enter to cycle.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter should produce a command")

	msg := cmd()
	actionMsg, ok := msg.(SetDoubleClickActionMsg)
	require.True(t, ok, "command should return SetDoubleClickActionMsg")
	assert.Equal(t, string(items[0]), actionMsg.ItemType)

	// The action should be the second action (cycled from default).
	allActs := actions.AllActions(items[0])
	require.True(t, len(allActs) >= 2, "first configurable item should have alternatives")
	assert.Equal(t, string(allActs[1]), actionMsg.Action, "should cycle to second action")
}

func TestDoubleClickActionCycleWraps(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	allActs := actions.AllActions(items[0])
	require.True(t, len(allActs) >= 2)

	// Set override to the last action so the next cycle wraps to first.
	p.actionOverrides[string(items[0])] = string(allActs[len(allActs)-1])

	// Navigate to the first action field.
	for p.cursorIndex() < fieldActionsStart {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	actionMsg := msg.(SetDoubleClickActionMsg)
	assert.Equal(t, string(allActs[0]), actionMsg.Action, "should wrap to first action")
}

func TestResetPrompts(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	// Navigate to reset prompts field (after all action fields).
	resetField := p.fieldResetPrompts()
	for p.cursorIndex() < resetField {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}
	assert.Equal(t, resetField, p.cursorIndex())

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter should produce a command")

	msg := cmd()
	_, ok := msg.(ResetActionPromptsMsg)
	assert.True(t, ok, "command should return ResetActionPromptsMsg")
}

func TestNavigationWithActions(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()

	// Total fields: preview + theme + len(items) double-click + len(items) right-click + resetPrompts
	expectedCount := fieldActionsStart + 2*settingField(len(items)) + 1
	assert.Equal(t, expectedCount, p.fieldCount())

	// Navigate all the way down.
	for i := settingField(0); i < p.fieldCount()-1; i++ {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}
	assert.Equal(t, p.fieldCount()-1, p.cursorIndex(), "should reach last field")

	// Navigate all the way back up.
	for p.cursorIndex() > 0 {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'k'})
		p = updated.(*Panel)
	}
	assert.Equal(t, fieldPreviewPosition, p.cursorIndex(), "should return to first field")
}

func TestActionOverridesFromConfig(t *testing.T) {
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	// Create config with an override for the first item.
	allActs := actions.AllActions(items[0])
	require.True(t, len(allActs) >= 2)
	overrideAction := string(allActs[1])

	cfg := config.ActionsConfig{
		DoubleClick: map[string]string{
			string(items[0]): overrideAction,
		},
	}

	p := New(layout.PreviewRight, "default", testThemes, cfg)
	assert.Equal(t, overrideAction, p.currentActionOverride(string(items[0])),
		"should load override from config")
}

// ---------------------------------------------------------------------------
// Right-Click Actions tests
// ---------------------------------------------------------------------------

func TestSettingsShowsRightClickSection(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})

	view := p.View(60, 80)
	assert.Contains(t, view, "Right-Click Actions", "view should show right-click actions heading")

	// Should contain at least one configurable item label in the right-click section.
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)
	label := actions.ItemLabel(items[0])
	assert.Contains(t, view, label, "view should show configurable item label")

	// Should show default "context menu" label.
	contextMenuLabel := actions.ActionLabel(actions.ActionShowContextMenu)
	assert.Contains(t, view, contextMenuLabel, "view should show context menu as default right-click action")
}

func TestSettingsRightClickDefaultIsContextMenu(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	// No overrides set, so right-click override should be empty (defaults to context_menu at render).
	assert.Empty(t, p.currentRightClickOverride(string(items[0])),
		"right-click override should be empty when no config is set")

	// The rendered view should show "context menu" for each right-click item.
	view := p.View(60, 80)
	contextMenuLabel := actions.ActionLabel(actions.ActionShowContextMenu)
	assert.Contains(t, view, contextMenuLabel, "view should display context menu as default")
}

func TestSettingsRightClickCycleAction(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	// Navigate to the first right-click action field.
	rcStart := p.fieldRightClickStart()
	for p.cursorIndex() < rcStart {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}
	assert.Equal(t, rcStart, p.cursorIndex())

	// Press enter to cycle from context_menu to next action.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter should produce a command")

	msg := cmd()
	rcMsg, ok := msg.(SetRightClickActionMsg)
	require.True(t, ok, "command should return SetRightClickActionMsg")
	assert.Equal(t, string(items[0]), rcMsg.ItemType)

	// The action should be the second in AllRightClickActions (after context_menu).
	allActs := actions.AllRightClickActions(items[0])
	require.True(t, len(allActs) >= 2, "first configurable item should have right-click alternatives")
	assert.Equal(t, string(allActs[1]), rcMsg.Action, "should cycle to second right-click action")
}

func TestSettingsRightClickCycleWraps(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	allActs := actions.AllRightClickActions(items[0])
	require.True(t, len(allActs) >= 2)

	// Set override to the last action so the next cycle wraps to first (context_menu).
	p.rightClickOverrides[string(items[0])] = string(allActs[len(allActs)-1])

	// Navigate to the first right-click action field.
	rcStart := p.fieldRightClickStart()
	for p.cursorIndex() < rcStart {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	rcMsg := msg.(SetRightClickActionMsg)
	assert.Equal(t, string(allActs[0]), rcMsg.Action, "should wrap to first action (context_menu)")
}

func TestSettingsRightClickOverridesFromConfig(t *testing.T) {
	items := actions.ConfigurableItems()
	require.NotEmpty(t, items)

	allActs := actions.AllRightClickActions(items[0])
	require.True(t, len(allActs) >= 2)
	overrideAction := string(allActs[1])

	cfg := config.ActionsConfig{
		RightClick: map[string]string{
			string(items[0]): overrideAction,
		},
	}

	p := New(layout.PreviewRight, "default", testThemes, cfg)
	assert.Equal(t, overrideAction, p.currentRightClickOverride(string(items[0])),
		"should load right-click override from config")
}

func TestSettingsNavigationWithRightClickActions(t *testing.T) {
	p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{})
	items := actions.ConfigurableItems()

	// Total fields: preview + theme + len(items) double-click + len(items) right-click + resetPrompts
	expectedCount := fieldActionsStart + 2*settingField(len(items)) + 1
	assert.Equal(t, expectedCount, p.fieldCount())

	// Navigate all the way down.
	for p.cursorIndex() < p.fieldCount()-1 {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
		p = updated.(*Panel)
	}
	assert.Equal(t, p.fieldCount()-1, p.cursorIndex(), "should reach last field")

	// Navigate all the way back up.
	for p.cursorIndex() > 0 {
		updated, _ := p.Update(tea.KeyPressMsg{Code: 'k'})
		p = updated.(*Panel)
	}
	assert.Equal(t, fieldPreviewPosition, p.cursorIndex(), "should return to first field")
}
