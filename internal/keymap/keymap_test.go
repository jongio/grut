package keymap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Scheme loading
// ---------------------------------------------------------------------------

func TestLoadDefaultScheme(t *testing.T) {
	bindings, err := LoadScheme("default")
	require.NoError(t, err)
	assert.NotEmpty(t, bindings)

	// Verify known bindings exist.
	found := findBinding(bindings, "ctrl+c", ModeGlobal, "")
	require.NotNil(t, found, "expected ctrl+c global binding")
	assert.Equal(t, "quit", found.Action)

	found = findBinding(bindings, "j", ModePanel, "")
	require.NotNil(t, found, "expected j panel binding")
	assert.Equal(t, "cursor_down", found.Action)
}

func TestLoadVimScheme(t *testing.T) {
	bindings, err := LoadScheme("vim")
	require.NoError(t, err)
	assert.NotEmpty(t, bindings)

	// Vim scheme should have gg for cursor_top.
	found := findBinding(bindings, "g g", ModePanel, "")
	require.NotNil(t, found, "expected g g panel binding in vim scheme")
	assert.Equal(t, "cursor_top", found.Action)

	found = findBinding(bindings, "G", ModePanel, "")
	require.NotNil(t, found, "expected G panel binding in vim scheme")
	assert.Equal(t, "cursor_bottom", found.Action)
}

func TestLoadClassicScheme(t *testing.T) {
	bindings, err := LoadScheme("classic")
	require.NoError(t, err)
	assert.NotEmpty(t, bindings)

	// Classic scheme uses space to stage in gitstatus.
	found := findBinding(bindings, " ", ModePanel, "gitstatus")
	require.NotNil(t, found, "expected space stage binding in classic scheme")
	assert.Equal(t, "stage", found.Action)
}

func TestLoadSchemeFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	content := `
[[bindings]]
key = "x"
action = "custom_action"
mode = "panel"
description = "Custom binding"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	bindings, err := LoadScheme(path)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "custom_action", bindings[0].Action)
	assert.Equal(t, ModePanel, bindings[0].Mode)
}

func TestLoadSchemeUnknownReturnsError(t *testing.T) {
	_, err := LoadScheme("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestLoadSchemeInvalidTOMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte("not valid toml [[["), 0o600))

	_, err := LoadScheme(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing scheme TOML")
}

func TestLoadSchemeInvalidModeReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "badmode.toml")
	content := `
[[bindings]]
key = "x"
action = "test"
mode = "bogus"
description = "Bad mode"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	_, err := LoadScheme(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key mode")
}

// ---------------------------------------------------------------------------
// NewKeymap
// ---------------------------------------------------------------------------

func TestNewKeymapDefault(t *testing.T) {
	km, err := NewKeymap("default")
	require.NoError(t, err)
	assert.NotNil(t, km)
	assert.NotEmpty(t, km.Bindings())
}

func TestNewKeymapInvalidScheme(t *testing.T) {
	_, err := NewKeymap("does-not-exist")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Dispatch — basic
// ---------------------------------------------------------------------------

func TestDispatchGlobalBinding(t *testing.T) {
	km := testKeymap(t)

	action, ok := km.Dispatch("ctrl+c", "")
	assert.True(t, ok)
	assert.Equal(t, "quit", action)
}

func TestDispatchGlobalBindingIgnoresContext(t *testing.T) {
	km := testKeymap(t)

	// Global bindings should fire regardless of context.
	action, ok := km.Dispatch("ctrl+c", "filetree")
	assert.True(t, ok)
	assert.Equal(t, "quit", action)
}

func TestDispatchPanelBinding(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModePanel)

	action, ok := km.Dispatch("j", "")
	assert.True(t, ok)
	assert.Equal(t, "cursor_down", action)
}

func TestDispatchContextSpecificBinding(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModePanel)

	// "s" in gitstatus context → stage.
	action, ok := km.Dispatch("s", "gitstatus")
	assert.True(t, ok)
	assert.Equal(t, "stage", action)

	// "s" without matching context → not handled (no general panel "s").
	action, ok = km.Dispatch("s", "preview")
	assert.False(t, ok)
	assert.Empty(t, action)
}

func TestDispatchContextFallsBackToGeneral(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModePanel)

	// "j" is a general panel binding, should work in any context.
	action, ok := km.Dispatch("j", "gitstatus")
	assert.True(t, ok)
	assert.Equal(t, "cursor_down", action)
}

func TestDispatchUnboundKey(t *testing.T) {
	km := testKeymap(t)

	action, ok := km.Dispatch("F12", "")
	assert.False(t, ok)
	assert.Empty(t, action)
}

// ---------------------------------------------------------------------------
// Dispatch — multi-key sequences
// ---------------------------------------------------------------------------

func TestDispatchMultiKeySequence(t *testing.T) {
	km := testKeymap(t)

	// First key should buffer the prefix.
	action, ok := km.Dispatch("ctrl+b", "")
	assert.False(t, ok, "prefix key should not dispatch")
	assert.Empty(t, action)
	assert.True(t, km.HasPending())

	// Second key completes the sequence.
	action, ok = km.Dispatch("z", "")
	assert.True(t, ok)
	assert.Equal(t, "zoom_toggle", action)
	assert.False(t, km.HasPending())
}

func TestDispatchMultiKeySequenceNoMatch(t *testing.T) {
	km := testKeymap(t)

	// Buffer the prefix.
	_, _ = km.Dispatch("ctrl+b", "")
	assert.True(t, km.HasPending())

	// Non-matching second key: prefix cleared, key not handled.
	action, ok := km.Dispatch("9", "")
	assert.False(t, ok)
	assert.Empty(t, action)
	assert.False(t, km.HasPending())
}

// ---------------------------------------------------------------------------
// Mode switching
// ---------------------------------------------------------------------------

func TestSetModeAndCurrentMode(t *testing.T) {
	km := testKeymap(t)

	assert.Equal(t, ModePanel, km.CurrentMode(), "default mode should be ModePanel")

	km.SetMode(ModeInput)
	assert.Equal(t, ModeInput, km.CurrentMode())

	km.SetMode(ModeGlobal)
	assert.Equal(t, ModeGlobal, km.CurrentMode())
}

func TestSetModeClearsPending(t *testing.T) {
	km := testKeymap(t)

	// Buffer a prefix.
	_, _ = km.Dispatch("ctrl+b", "")
	assert.True(t, km.HasPending())

	// Mode switch should clear it.
	km.SetMode(ModeInput)
	assert.False(t, km.HasPending())
}

func TestInputModeDispatch(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModeInput)

	// "escape" should map to exit_input in input mode.
	action, ok := km.Dispatch("escape", "")
	assert.True(t, ok)
	assert.Equal(t, "exit_input", action)

	// Panel-mode binding should NOT fire in input mode.
	action, ok = km.Dispatch("j", "")
	assert.False(t, ok)
	assert.Empty(t, action)
}

func TestGlobalBindingsFireInInputMode(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModeInput)

	// ctrl+c is global; should still work.
	action, ok := km.Dispatch("ctrl+c", "")
	assert.True(t, ok)
	assert.Equal(t, "quit", action)
}

func TestPanelBindingsDoNotFireInInputMode(t *testing.T) {
	km := testKeymap(t)
	km.SetMode(ModeInput)

	action, ok := km.Dispatch("q", "")
	assert.False(t, ok)
	assert.Empty(t, action)
}

// ---------------------------------------------------------------------------
// Conflict detection
// ---------------------------------------------------------------------------

func TestDetectConflictsNone(t *testing.T) {
	bindings := []Binding{
		{Key: "a", Action: "one", Mode: ModePanel},
		{Key: "b", Action: "two", Mode: ModePanel},
	}
	conflicts := DetectConflicts(bindings)
	assert.Empty(t, conflicts)
}

func TestDetectConflictsFound(t *testing.T) {
	bindings := []Binding{
		{Key: "a", Action: "one", Mode: ModePanel},
		{Key: "a", Action: "two", Mode: ModePanel},
	}
	conflicts := DetectConflicts(bindings)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "a", conflicts[0].Key)
	assert.Equal(t, ModePanel, conflicts[0].Mode)
	assert.Equal(t, []string{"one", "two"}, conflicts[0].Actions)
}

func TestDetectConflictsDifferentModesAreNotConflicts(t *testing.T) {
	bindings := []Binding{
		{Key: "enter", Action: "select", Mode: ModePanel},
		{Key: "enter", Action: "submit_input", Mode: ModeInput},
	}
	conflicts := DetectConflicts(bindings)
	assert.Empty(t, conflicts)
}

func TestDetectConflictsDifferentContextsAreNotConflicts(t *testing.T) {
	bindings := []Binding{
		{Key: "s", Action: "stage", Mode: ModePanel, Context: "gitstatus"},
		{Key: "s", Action: "sort", Mode: ModePanel, Context: "filetree"},
	}
	conflicts := DetectConflicts(bindings)
	assert.Empty(t, conflicts)
}

func TestDetectConflictsDefaultScheme(t *testing.T) {
	bindings, err := LoadScheme("default")
	require.NoError(t, err)

	conflicts := DetectConflicts(bindings)
	assert.Empty(t, conflicts, "default scheme should have no conflicts")
}

// ---------------------------------------------------------------------------
// KeyMode string round-trip
// ---------------------------------------------------------------------------

func TestKeyModeString(t *testing.T) {
	assert.Equal(t, "global", ModeGlobal.String())
	assert.Equal(t, "panel", ModePanel.String())
	assert.Equal(t, "input", ModeInput.String())
}

func TestParseKeyMode(t *testing.T) {
	tests := []struct {
		input string
		want  KeyMode
	}{
		{"global", ModeGlobal},
		{"GLOBAL", ModeGlobal},
		{"panel", ModePanel},
		{"Panel", ModePanel},
		{"input", ModeInput},
		{"INPUT", ModeInput},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseKeyMode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseKeyModeInvalid(t *testing.T) {
	_, err := parseKeyMode("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key mode")
}

// ---------------------------------------------------------------------------
// Conflict.String
// ---------------------------------------------------------------------------

func TestConflictString(t *testing.T) {
	c := Conflict{
		Key:     "a",
		Mode:    ModePanel,
		Context: "filetree",
		Actions: []string{"one", "two"},
	}
	s := c.String()
	assert.Contains(t, s, "filetree")
	assert.Contains(t, s, "one")
	assert.Contains(t, s, "two")
}

func TestConflictStringEmptyContext(t *testing.T) {
	c := Conflict{
		Key:     "x",
		Mode:    ModeGlobal,
		Actions: []string{"alpha"},
	}
	s := c.String()
	assert.Contains(t, s, "(all)")
}

// ---------------------------------------------------------------------------
// WarnConflicts (smoke test — just verify it doesn't panic)
// ---------------------------------------------------------------------------

func TestWarnConflictsDoesNotPanic(t *testing.T) {
	bindings := []Binding{
		{Key: "a", Action: "one", Mode: ModePanel},
		{Key: "a", Action: "two", Mode: ModePanel},
	}
	assert.NotPanics(t, func() {
		WarnConflicts(bindings)
	})
}

// ---------------------------------------------------------------------------
// ClearPending
// ---------------------------------------------------------------------------

func TestClearPending(t *testing.T) {
	km := testKeymap(t)
	_, _ = km.Dispatch("ctrl+b", "")
	assert.True(t, km.HasPending())

	km.ClearPending()
	assert.False(t, km.HasPending())
}

// ---------------------------------------------------------------------------
// isFilePath
// ---------------------------------------------------------------------------

func TestIsFilePath(t *testing.T) {
	assert.False(t, isFilePath("default"))
	assert.False(t, isFilePath("vim"))
	assert.True(t, isFilePath("/home/user/keys.toml"))
	assert.True(t, isFilePath(`C:\Users\user\keys.toml`))
	assert.True(t, isFilePath("./custom.toml"))
}

// ---------------------------------------------------------------------------
// All schemes load without errors
// ---------------------------------------------------------------------------

func TestAllBuiltinSchemesLoad(t *testing.T) {
	for _, name := range []string{"default", "vim", "classic"} {
		t.Run(name, func(t *testing.T) {
			bindings, err := LoadScheme(name)
			require.NoError(t, err)
			assert.NotEmpty(t, bindings)

			// No conflicts.
			conflicts := DetectConflicts(bindings)
			assert.Empty(t, conflicts, "scheme %q has conflicts: %v", name, conflicts)
		})
	}
}

// ---------------------------------------------------------------------------
// CRUD bindings
// ---------------------------------------------------------------------------

func TestCRUDBindingsExistInAllSchemes(t *testing.T) {
	crudBindings := []struct {
		key     string
		action  string
		context string
	}{
		{"n", "item_create", "gitinfo"},
		{"x", "item_delete", "gitinfo"},
		{"e", "item_edit", "gitinfo"},
		{"F2", "item_edit", "gitinfo"},
		{"o", "item_open", "gitinfo"},
		{"y", "item_copy", "gitinfo"},
		{"n", "item_create", "github"},
		{"x", "item_delete", "github"},
		{"e", "item_edit", "github"},
		{"F2", "item_edit", "github"},
		{"o", "item_open", "github"},
		{"y", "item_copy", "github"},
		{"n", "item_create", "branches"},
		{"x", "item_delete", "branches"},
		{"e", "item_edit", "branches"},
		{"o", "item_open", "branches"},
		{"y", "item_copy", "branches"},
		{"n", "item_create", "filetree"},
		{"x", "item_delete", "filetree"},
		{"e", "item_edit", "filetree"},
		{"o", "item_open", "filetree"},
		{"y", "item_copy", "filetree"},
		{"n", "item_create", "stash"},
		{"x", "item_delete", "stash"},
		{"y", "item_copy", "stash"},
	}

	for _, scheme := range []string{"default", "vim", "classic"} {
		t.Run(scheme, func(t *testing.T) {
			bindings, err := LoadScheme(scheme)
			require.NoError(t, err)

			for _, cb := range crudBindings {
				found := findBinding(bindings, cb.key, ModePanel, cb.context)
				assert.NotNil(t, found,
					"scheme %q missing %q binding for context %q (expected action %q)",
					scheme, cb.key, cb.context, cb.action)
				if found != nil {
					assert.Equal(t, cb.action, found.Action,
						"scheme %q: %q in context %q has wrong action", scheme, cb.key, cb.context)
				}
			}
		})
	}
}

func TestCRUDContextOverridesVimSearchNext(t *testing.T) {
	km, err := NewKeymap("vim")
	require.NoError(t, err)
	km.SetMode(ModePanel)

	// In gitinfo context, "n" should be item_create, not search_next.
	action, ok := km.Dispatch("n", "gitinfo")
	assert.True(t, ok)
	assert.Equal(t, "item_create", action)

	// In github context, "n" should be item_create.
	action, ok = km.Dispatch("n", "github")
	assert.True(t, ok)
	assert.Equal(t, "item_create", action)

	// In branches context, "n" should be item_create.
	action, ok = km.Dispatch("n", "branches")
	assert.True(t, ok)
	assert.Equal(t, "item_create", action)

	// In a panel without a context-specific "n" override, search_next still works.
	action, ok = km.Dispatch("n", "preview")
	assert.True(t, ok)
	assert.Equal(t, "search_next", action)
}

func TestCRUDOpenKeyAvailableEverywhere(t *testing.T) {
	km, err := NewKeymap("default")
	require.NoError(t, err)
	km.SetMode(ModePanel)

	// "o" should resolve to item_open in gitinfo context.
	action, ok := km.Dispatch("o", "gitinfo")
	assert.True(t, ok)
	assert.Equal(t, "item_open", action)

	// "o" should resolve to item_open in github context.
	action, ok = km.Dispatch("o", "github")
	assert.True(t, ok)
	assert.Equal(t, "item_open", action)

	// "o" should not be bound in panels without CRUD context.
	_, ok = km.Dispatch("o", "preview")
	assert.False(t, ok)
}

func TestGitHubPanelDeleteDispatch(t *testing.T) {
	for _, scheme := range []string{"default", "vim", "classic"} {
		t.Run(scheme, func(t *testing.T) {
			km, err := NewKeymap(scheme)
			require.NoError(t, err)
			km.SetMode(ModePanel)

			// "x" in github context should dispatch to item_delete.
			action, ok := km.Dispatch("x", "github")
			assert.True(t, ok, "scheme %q: 'x' should be bound in github context", scheme)
			assert.Equal(t, "item_delete", action, "scheme %q: 'x' in github should be item_delete", scheme)

			// "x" in gitinfo context should also dispatch to item_delete.
			action, ok = km.Dispatch("x", "gitinfo")
			assert.True(t, ok, "scheme %q: 'x' should be bound in gitinfo context", scheme)
			assert.Equal(t, "item_delete", action)
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// testKeymap returns a Keymap loaded with the default scheme.
func testKeymap(t *testing.T) *Keymap {
	t.Helper()
	km, err := NewKeymap("default")
	require.NoError(t, err)
	return km
}

// findBinding searches bindings for a match on key, mode, and context.
func findBinding(bindings []Binding, key string, mode KeyMode, context string) *Binding {
	for i, b := range bindings {
		if b.Key == key && b.Mode == mode && b.Context == context {
			return &bindings[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Navigation & panel-focus bindings
// ---------------------------------------------------------------------------

func TestNavigationBindingsExistInAllSchemes(t *testing.T) {
	globalBindings := []struct {
		key    string
		action string
	}{
		{"1", "focus_panel_1"},
		{"2", "focus_panel_2"},
		{"3", "focus_panel_3"},
		{"4", "focus_panel_4"},
		{"5", "focus_panel_5"},
		{"F", "fetch"},
	}

	for _, scheme := range []string{"default", "vim", "classic"} {
		t.Run(scheme, func(t *testing.T) {
			bindings, err := LoadScheme(scheme)
			require.NoError(t, err)

			for _, nb := range globalBindings {
				found := findBinding(bindings, nb.key, ModeGlobal, "")
				assert.NotNil(t, found,
					"scheme %q missing %q global binding (expected action %q)",
					scheme, nb.key, nb.action)
				if found != nil {
					assert.Equal(t, nb.action, found.Action,
						"scheme %q: %q has wrong action", scheme, nb.key)
				}
			}
		})
	}
}

func TestGitFilterBindingChangedToG(t *testing.T) {
	bindings, err := LoadScheme("default")
	require.NoError(t, err)

	// Verify "g" → "cycle_file_filter" in filetree context.
	found := findBinding(bindings, "g", ModePanel, "filetree")
	require.NotNil(t, found, "default scheme should have 'g' binding in filetree context")
	assert.Equal(t, "cycle_file_filter", found.Action)
}

func TestVimSchemeHasCursorNavBindings(t *testing.T) {
	bindings, err := LoadScheme("vim")
	require.NoError(t, err)

	vimNavBindings := []struct {
		key    string
		action string
	}{
		{"G", "cursor_bottom"},
		{"g g", "cursor_top"},
		{"ctrl+d", "half_page_down"},
		{"ctrl+u", "half_page_up"},
	}

	for _, nb := range vimNavBindings {
		found := findBinding(bindings, nb.key, ModePanel, "")
		assert.NotNil(t, found,
			"vim scheme missing %q panel binding (expected action %q)",
			nb.key, nb.action)
		if found != nil {
			assert.Equal(t, nb.action, found.Action,
				"vim scheme: %q has wrong action", nb.key)
		}
	}
}
