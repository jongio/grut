package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DefaultAction
// ---------------------------------------------------------------------------

func TestDefaultAction(t *testing.T) {
	tests := []struct {
		itemType ItemType
		expected ActionID
	}{
		{ItemLocalBranch, ActionCheckout},
		{ItemRemoteBranch, ActionCheckout},
		{ItemWorktree, ActionChangeDirectory},
		{ItemRemote, ActionOpenInBrowser},
		{ItemStashEntry, ActionPromptAction},
		{ItemIssue, ActionOpenInBrowser},
		{ItemPR, ActionOpenInBrowser},
		{ItemActionRun, ActionOpenInBrowser},
		{ItemFile, ActionOpenInEditor},
		{ItemDirectory, ActionExpandCollapse},
		{ItemCommit, ActionShowDetail},
		{ItemStatusFile, ActionExpandDiff},
		{ItemLogCommit, ActionShowDetail},
		{ItemStash, ActionApply},
		{ItemConflictFile, ActionOpenDiff},
		{ItemReviewFile, ActionExpandDiff},
		{ItemContextFile, ActionPreview},
		{ItemBookmark, ActionJump},
		{ItemExtension, ActionToggleDetails},
		{ItemAgent, ActionToggleOutput},
		{ItemSetting, ActionCycleValue},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			assert.Equal(t, tt.expected, DefaultAction(tt.itemType))
		})
	}
}

func TestDefaultActionUnknownType(t *testing.T) {
	assert.Equal(t, ActionID(""), DefaultAction("nonexistent"))
}

// ---------------------------------------------------------------------------
// AllActions
// ---------------------------------------------------------------------------

func TestAllActions(t *testing.T) {
	tests := []struct {
		itemType ItemType
		expected []ActionID
	}{
		{
			ItemLocalBranch,
			[]ActionID{ActionCheckout, ActionCopyName, ActionOpenInBrowser},
		},
		{
			ItemPR,
			[]ActionID{ActionOpenInBrowser, ActionMergePR, ActionCopyURL, ActionCopyNumber, ActionCheckoutBranch},
		},
		{
			ItemStashEntry,
			[]ActionID{ActionPromptAction, ActionApply, ActionPop, ActionDrop},
		},
		{
			ItemFile,
			[]ActionID{ActionOpenInEditor, ActionCopyPath, ActionStage, ActionPreview},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			got := AllActions(tt.itemType)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestAllActionsDefaultIsFirst(t *testing.T) {
	for it, entry := range Registry {
		t.Run(string(it), func(t *testing.T) {
			all := AllActions(it)
			require.NotEmpty(t, all)
			assert.Equal(t, entry.Default, all[0],
				"first element of AllActions should be the default action")
		})
	}
}

func TestAllActionsNoAlternatives(t *testing.T) {
	// Items without alternatives should return a single-element slice.
	got := AllActions(ItemAgent)
	require.Len(t, got, 1)
	assert.Equal(t, ActionToggleOutput, got[0])
}

func TestAllActionsUnknownType(t *testing.T) {
	assert.Nil(t, AllActions("nonexistent"))
}

// ---------------------------------------------------------------------------
// Description
// ---------------------------------------------------------------------------

func TestDescriptionAllRegistered(t *testing.T) {
	for it := range Registry {
		t.Run(string(it), func(t *testing.T) {
			desc := Description(it)
			assert.NotEmpty(t, desc, "registered item type %q should have a description", it)
		})
	}
}

func TestDescriptionUnknownType(t *testing.T) {
	assert.Equal(t, "", Description("nonexistent"))
}

// ---------------------------------------------------------------------------
// ActionLabel
// ---------------------------------------------------------------------------

func TestActionLabel(t *testing.T) {
	tests := []struct {
		action   ActionID
		expected string
	}{
		{ActionCheckout, "checkout"},
		{ActionOpenInBrowser, "open in browser"},
		{ActionStageUnstage, "stage/unstage"},
		{ActionCopyURL, "copy URL"},
		{ActionCheckoutBranch, "checkout branch"},
		{ActionResolveOurs, "resolve ours"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			assert.Equal(t, tt.expected, ActionLabel(tt.action))
		})
	}
}

func TestActionLabelAllDefined(t *testing.T) {
	// Collect every ActionID referenced in the registry.
	seen := make(map[ActionID]bool)
	for _, entry := range Registry {
		seen[entry.Default] = true
		for _, alt := range entry.Alternatives {
			seen[alt] = true
		}
	}

	for action := range seen {
		t.Run(string(action), func(t *testing.T) {
			label := ActionLabel(action)
			assert.NotEmpty(t, label,
				"action %q should have a non-empty label", action)
			// Verify it comes from the labels map rather than the fallback.
			_, ok := actionLabels[action]
			assert.True(t, ok,
				"action %q used in Registry should have an explicit entry in actionLabels", action)
		})
	}
}

func TestActionLabelUnknownFallback(t *testing.T) {
	assert.Equal(t, "unknown_action", ActionLabel("unknown_action"))
}

// ---------------------------------------------------------------------------
// IsValidAction
// ---------------------------------------------------------------------------

func TestIsValidAction(t *testing.T) {
	tests := []struct {
		name     string
		itemType ItemType
		action   ActionID
		valid    bool
	}{
		{"default is valid", ItemLocalBranch, ActionCheckout, true},
		{"alternative is valid", ItemLocalBranch, ActionCopyName, true},
		{"another alternative is valid", ItemLocalBranch, ActionOpenInBrowser, true},
		{"unrelated action is invalid", ItemLocalBranch, ActionApply, false},
		{"empty action is invalid", ItemLocalBranch, "", false},
		{"unknown item type is invalid", "nonexistent", ActionCheckout, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidAction(tt.itemType, tt.action))
		})
	}
}

// ---------------------------------------------------------------------------
// ConfigurableItems
// ---------------------------------------------------------------------------

func TestConfigurableItems(t *testing.T) {
	items := ConfigurableItems()

	// Every returned item should have alternatives.
	for _, it := range items {
		entry, ok := Registry[it]
		require.True(t, ok, "ConfigurableItems returned unregistered item %q", it)
		assert.NotEmpty(t, entry.Alternatives,
			"ConfigurableItems should only return items with alternatives; %q has none", it)
	}

	// Items without alternatives must NOT appear.
	for _, it := range items {
		assert.NotEqual(t, ItemAgent, it, "agent has no alternatives and should not be configurable")
		assert.NotEqual(t, ItemSetting, it, "setting has no alternatives and should not be configurable")
	}
}

func TestConfigurableItemsStableOrder(t *testing.T) {
	// Calling ConfigurableItems twice should return the same order.
	first := ConfigurableItems()
	second := ConfigurableItems()
	assert.Equal(t, first, second, "ConfigurableItems should return a stable order")
}

func TestConfigurableItemsExcludesNonConfigurable(t *testing.T) {
	items := ConfigurableItems()
	configurable := make(map[ItemType]bool, len(items))
	for _, it := range items {
		configurable[it] = true
	}

	for it, entry := range Registry {
		if len(entry.Alternatives) == 0 {
			assert.False(t, configurable[it],
				"item %q has no alternatives and should not appear in ConfigurableItems", it)
		}
	}
}

// ---------------------------------------------------------------------------
// ItemLabel
// ---------------------------------------------------------------------------

func TestItemLabel(t *testing.T) {
	tests := []struct {
		itemType ItemType
		expected string
	}{
		{ItemLocalBranch, "Branch"},
		{ItemPR, "PR"},
		{ItemStatusFile, "Status File"},
		{ItemAgent, "Agent"},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			assert.Equal(t, tt.expected, ItemLabel(tt.itemType))
		})
	}
}

func TestItemLabelAllRegistered(t *testing.T) {
	for it := range Registry {
		t.Run(string(it), func(t *testing.T) {
			label := ItemLabel(it)
			assert.NotEqual(t, string(it), label,
				"item type %q should have a human-readable label, not its raw value", it)
		})
	}
}

func TestItemLabelUnknownFallback(t *testing.T) {
	assert.Equal(t, "unknown_type", ItemLabel("unknown_type"))
}

// ---------------------------------------------------------------------------
// AllItemTypes
// ---------------------------------------------------------------------------

func TestAllItemTypes(t *testing.T) {
	all := AllItemTypes()
	assert.Len(t, all, len(Registry),
		"AllItemTypes should return exactly as many types as the Registry has entries")

	// Verify every Registry entry is present.
	set := make(map[ItemType]bool, len(all))
	for _, it := range all {
		set[it] = true
	}
	for it := range Registry {
		assert.True(t, set[it], "AllItemTypes is missing registered type %q", it)
	}
}

func TestAllItemTypesReturnsCopy(t *testing.T) {
	first := AllItemTypes()
	first[0] = "mutated"
	second := AllItemTypes()
	assert.NotEqual(t, "mutated", string(second[0]),
		"AllItemTypes should return a copy, not a reference to the internal slice")
}

// ---------------------------------------------------------------------------
// Registry consistency
// ---------------------------------------------------------------------------

func TestRegistryConsistency(t *testing.T) {
	// Every item in allItemTypes must be in the Registry.
	for _, it := range allItemTypes {
		_, ok := Registry[it]
		assert.True(t, ok, "allItemTypes contains %q which is not in Registry", it)
	}

	// Every Registry entry must be in allItemTypes.
	set := make(map[ItemType]bool, len(allItemTypes))
	for _, it := range allItemTypes {
		set[it] = true
	}
	for it := range Registry {
		assert.True(t, set[it], "Registry contains %q which is not in allItemTypes", it)
	}
}

func TestRegistryDefaultsAreNonEmpty(t *testing.T) {
	for it, entry := range Registry {
		assert.NotEmpty(t, string(entry.Default),
			"item type %q has an empty default action", it)
	}
}

// ---------------------------------------------------------------------------
// RightClickAction
// ---------------------------------------------------------------------------

func TestRightClickAction(t *testing.T) {
	for it := range Registry {
		t.Run(string(it), func(t *testing.T) {
			got := RightClickAction(it)
			assert.Equal(t, ActionShowContextMenu, got,
				"item type %q should default to context_menu right-click action", it)
		})
	}
}

func TestRightClickActionUnknownType(t *testing.T) {
	assert.Equal(t, ActionID(""), RightClickAction("nonexistent"))
}

// ---------------------------------------------------------------------------
// AllRightClickActions
// ---------------------------------------------------------------------------

func TestAllRightClickActions(t *testing.T) {
	got := AllRightClickActions(ItemLocalBranch)
	require.NotNil(t, got)
	// Should start with context_menu, then default (checkout), then alternatives.
	assert.Equal(t, ActionShowContextMenu, got[0],
		"first element should be context_menu")
	assert.Equal(t, ActionCheckout, got[1],
		"second element should be the default action")
	// Should include alternatives and be deduplicated.
	assert.Contains(t, got, ActionCopyName)
	assert.Contains(t, got, ActionOpenInBrowser)
}

func TestAllRightClickActionsDeduplicates(t *testing.T) {
	got := AllRightClickActions(ItemLocalBranch)
	seen := make(map[ActionID]bool, len(got))
	for _, id := range got {
		assert.False(t, seen[id], "duplicate action %q in AllRightClickActions", id)
		seen[id] = true
	}
}

func TestAllRightClickActionsUnknownType(t *testing.T) {
	assert.Nil(t, AllRightClickActions("nonexistent"))
}

// ---------------------------------------------------------------------------
// ActionShowContextMenu label
// ---------------------------------------------------------------------------

func TestActionShowContextMenuLabel(t *testing.T) {
	assert.Equal(t, "context menu", ActionLabel(ActionShowContextMenu))
}
