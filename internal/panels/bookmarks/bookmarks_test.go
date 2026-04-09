package bookmarks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager creates a bookmark manager with a temp config dir,
// fully isolated from the user's real bookmarks.
func newTestManager(t *testing.T) *bm.Manager {
	t.Helper()
	return bm.NewManagerWithDir(config.BookmarksConfig{}, t.TempDir())
}

// newTestPanel creates a focused panel backed by a test manager.
func newTestPanel(t *testing.T, manager *bm.Manager) *Panel {
	t.Helper()
	p := New(manager, nil)
	p.Focus()
	p.SetSize(80, 24)
	p.Init(context.Background())
	return p
}

// keyMsg constructs a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestPanel_EmptyView(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	view := p.View(80, 24)
	assert.Contains(t, view, "No bookmarks")
}

func TestPanel_ViewShowsBookmarks(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	view := p.View(80, 24)
	assert.Contains(t, view, filepath.Base(dir))
}

func TestPanel_CursorNavigation(t *testing.T) {
	m := newTestManager(t)
	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		require.NoError(t, m.Add(dirs[i]))
	}

	p := newTestPanel(t, m)

	// Start at top.
	assert.Equal(t, 0, p.cursorIndex())

	// Move down.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursorIndex())

	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursorIndex())

	// Can't go past end.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursorIndex())

	// Move up.
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursorIndex())

	// Go to top.
	p.Update(keyMsg('g'))
	assert.Equal(t, 0, p.cursorIndex())

	// Go to bottom.
	p.Update(keyMsg('G'))
	assert.Equal(t, 2, p.cursorIndex())
}

func TestPanel_SelectEmitsNavigateMsg(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)

	_, cmd := p.Update(keyMsg('\r')) // enter
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(panels.NavigateToPathMsg)
	require.True(t, ok, "expected NavigateToPathMsg, got %T", msg)
	assert.Equal(t, filepath.Clean(dir), navMsg.Path)
}

func TestPanel_DeleteRemovesBookmark(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	require.Equal(t, 1, p.itemCount())

	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)

	msg := cmd()
	toastMsg, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Success, toastMsg.Level)
	assert.Contains(t, toastMsg.Message, "Removed bookmark")

	// Bookmark should be gone.
	assert.Equal(t, 0, p.itemCount())
	assert.False(t, m.Has(dir))
}

func TestPanel_EscapeEmitsToggle(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(panels.ToggleBookmarksMsg)
	assert.True(t, ok, "expected ToggleBookmarksMsg, got %T", msg)
}

func TestPanel_IgnoresKeysWhenBlurred(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.Blur()

	_, cmd := p.Update(keyMsg('j'))
	assert.Nil(t, cmd)
	assert.Equal(t, 0, p.cursorIndex())
}

func TestPanel_DeleteOnEmpty(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	_, cmd := p.Update(keyMsg('d'))
	assert.Nil(t, cmd)
}

func TestPanel_SelectOnEmpty(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	_, cmd := p.Update(keyMsg('\r'))
	assert.Nil(t, cmd)
}

func TestPanel_RefreshUpdatesItems(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)
	assert.Equal(t, 0, p.itemCount())

	// Add bookmark after panel creation.
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p.Refresh()
	assert.Equal(t, 1, p.itemCount())
}

func TestPanel_KeyBindings(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify key bindings include expected entries.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/↓"])
	assert.True(t, keys["enter"])
	assert.True(t, keys["d"])
	assert.True(t, keys["escape"])
}

func TestPanel_Title(t *testing.T) {
	m := newTestManager(t)
	p := New(m, nil)
	assert.Equal(t, "bookmarks", p.Title())
}

func TestPanel_CursorClampsAfterDelete(t *testing.T) {
	m := newTestManager(t)
	dirs := make([]string, 2)
	for i := range dirs {
		dirs[i] = t.TempDir()
		require.NoError(t, m.Add(dirs[i]))
	}

	p := newTestPanel(t, m)

	// Move to last item.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursorIndex())

	// Delete it — cursor should clamp.
	p.Update(keyMsg('d'))
	assert.Equal(t, 0, p.cursorIndex())
}

func TestPanel_ViewDimensions(t *testing.T) {
	m := newTestManager(t)
	p := newTestPanel(t, m)

	// Zero dimensions should return empty string.
	assert.Equal(t, "", p.View(0, 0))
	assert.Equal(t, "", p.View(-1, 10))
	assert.Equal(t, "", p.View(10, -1))
}

// Ensure unused import doesn't appear.
var _ = os.TempDir

// ---------------------------------------------------------------------------
// Mouse click tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsBookmark(t *testing.T) {
	m := newTestManager(t)
	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		require.NoError(t, m.Add(dirs[i]))
	}

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	assert.Equal(t, 0, p.cursorIndex())

	// Click on row 1 → selects second bookmark.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursorIndex())

	// Click on row 2 → selects third bookmark.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursorIndex())
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursorIndex(), "out-of-bounds click should not move cursor")
}

// ---------------------------------------------------------------------------
// Mouse double-click tests
// ---------------------------------------------------------------------------

func TestMouseDoubleClick_JumpsToBookmark(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemBookmark): true}

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	require.NotNil(t, cmd, "double-click should trigger navigation")

	msg := cmd()
	navMsg, ok := msg.(panels.NavigateToPathMsg)
	require.True(t, ok, "expected NavigateToPathMsg, got %T", msg)
	assert.Equal(t, filepath.Clean(dir), navMsg.Path)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "out-of-bounds double-click should not trigger action")
}

// ---------------------------------------------------------------------------
// Mouse wheel tests
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollDown(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 10; i++ {
		require.NoError(t, m.Add(t.TempDir()))
	}

	p := newTestPanel(t, m)
	p.SetSize(80, 3) // Small viewport to allow scrolling.

	assert.Equal(t, 0, p.offset)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 3, p.offset, "should scroll down by delta of 3")
}

func TestMouseWheel_ScrollUp(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 10; i++ {
		require.NoError(t, m.Add(t.TempDir()))
	}

	p := newTestPanel(t, m)
	p.SetSize(80, 3)

	// Scroll down first.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Greater(t, p.offset, 0)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should scroll back to top")
}

func TestMouseWheel_ScrollUpClampsToZero(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should not scroll below 0")
}

// ---------------------------------------------------------------------------
// Right-click tests
// ---------------------------------------------------------------------------

func TestRightClickShowsActionPicker(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 0, ContentCol: 5})
	require.NotNil(t, cmd, "right-click on bookmark should produce a command")

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
}

func TestRightClickOutOfBounds(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	require.NoError(t, m.Add(dir))

	p := newTestPanel(t, m)
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "right-click out of bounds should return nil cmd")
}
