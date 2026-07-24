package overlayreg

import (
	"testing"

	"github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFactory(t *testing.T, actions ...config.CustomAction) *Factory {
	t.Helper()
	th, err := theme.Load("default")
	require.NoError(t, err)
	// Use an isolated config dir so the test never touches the user's real
	// bookmarks file.
	bmMgr := bookmarks.NewManagerWithDir(config.BookmarksConfig{}, t.TempDir())
	if len(actions) > 0 {
		return New(th, bmMgr, actions)
	}
	return New(th, bmMgr)
}

func TestNewCopiesCustomActions(t *testing.T) {
	th, err := theme.Load("default")
	require.NoError(t, err)
	actions := []config.CustomAction{{Name: "Test", Command: "go test ./...", Key: "ctrl+t"}}
	f := New(th, bookmarks.NewManagerWithDir(config.BookmarksConfig{}, t.TempDir()), actions)
	require.NotNil(t, f)
	require.Len(t, f.customActions, 1)

	// The factory must copy the slice so a later mutation by the caller cannot
	// leak into the panels it builds.
	actions[0].Name = "Mutated"
	assert.Equal(t, "Test", f.customActions[0].Name)
}

func TestNewWithoutCustomActions(t *testing.T) {
	f := newTestFactory(t)
	require.NotNil(t, f)
	assert.Empty(t, f.customActions)
}

func TestFactoryBuildsOverlayPanels(t *testing.T) {
	f := newTestFactory(t)

	assert.NotNil(t, f.NewBookmarkPanel())
	assert.NotNil(t, f.NewCommandLogPanel())
	assert.NotNil(t, f.NewHelpPanel())
	assert.NotNil(t, f.NewWelcomePanel())
	assert.NotNil(t, f.NewTextSearch())
	assert.NotNil(t, f.NewSettingsPanel(
		layout.PreviewRight,
		"default",
		[]string{"default"},
		config.ActionsConfig{},
	))
}

func TestFactoryFuzzyFinderModes(t *testing.T) {
	f := newTestFactory(t, config.CustomAction{Name: "Test", Command: "echo hi"})

	// Every supported mode (plus an unknown one, which falls back to files)
	// must yield a usable panel.
	for _, mode := range []string{"files", "commands", "directories", "todos", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			assert.NotNil(t, f.NewFuzzyFinder(mode, nil))
		})
	}
}
