package filetree

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandAllAndCollapseAll(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	top := ft.visibleCount()
	require.Equal(t, 4, top, "expected 4 top-level entries")

	// Expand every directory.
	ft.Update(keyMsg('L'))
	assert.Equal(t, 7, ft.visibleCount(),
		"expand all should reveal nested files in docs/ and src/")

	// Collapse every directory.
	ft.Update(keyMsg('H'))
	assert.Equal(t, top, ft.visibleCount(),
		"collapse all should return to top-level entries only")
}

func TestCollapseAllKeepsCursorOnAncestor(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	ft.Update(keyMsg('L')) // expand all

	// Move the cursor onto src/app.go.
	found := false
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "app.go" {
			ft.viewport.cursor = i
			found = true
			break
		}
	}
	require.True(t, found, "app.go should be visible after expand all")

	ft.Update(keyMsg('H')) // collapse all

	// The cursor should rest on the nearest visible ancestor directory.
	assert.Equal(t, "src", filepath.Base(ft.CursorPath()),
		"cursor should fall back to the src ancestor after collapse all")
}

func TestExpandAllKeepsCursorOnNode(t *testing.T) {
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.Focus()

	// Put the cursor on the src directory.
	for i := 0; i < ft.visibleCount(); i++ {
		if ft.visibleName(i) == "src" {
			ft.viewport.cursor = i
			break
		}
	}
	require.Equal(t, "src", filepath.Base(ft.CursorPath()))

	ft.Update(keyMsg('L')) // expand all

	assert.Equal(t, "src", filepath.Base(ft.CursorPath()),
		"cursor should stay on src after expand all")
}
