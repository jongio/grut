package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTabManager(t *testing.T) {
	tree := &LeafNode{Panel: "test"}
	tm := NewTabManager("main", tree)

	assert.Equal(t, 1, tm.Count())
	assert.Equal(t, 0, tm.ActiveIndex())

	tab := tm.ActiveTab()
	require.NotNil(t, tab)
	assert.Equal(t, "main", tab.Name)
}

func TestTabAdd(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})

	assert.Equal(t, 2, tm.Count())
	assert.Equal(t, 1, tm.ActiveIndex()) // new tab is active
	assert.Equal(t, "tab2", tm.ActiveTab().Name)
}

func TestTabAddAfterActive(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	tm.Add("tab3", &LeafNode{Panel: "p3"})

	// tab3 should be inserted after tab2 (current active)
	assert.Equal(t, 3, tm.Count())
	tabs := tm.Tabs()
	assert.Equal(t, "tab1", tabs[0].Name)
	assert.Equal(t, "tab2", tabs[1].Name)
	assert.Equal(t, "tab3", tabs[2].Name)
}

func TestTabClose(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	tm.Add("tab3", &LeafNode{Panel: "p3"})

	// Close the middle tab (tab2 at index 1)
	err := tm.Close(1)
	require.NoError(t, err)

	assert.Equal(t, 2, tm.Count())
	tabs := tm.Tabs()
	assert.Equal(t, "tab1", tabs[0].Name)
	assert.Equal(t, "tab3", tabs[1].Name)
}

func TestTabCloseLastFails(t *testing.T) {
	tm := NewTabManager("only", &LeafNode{Panel: "p1"})
	err := tm.Close(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last tab")
}

func TestTabCloseOutOfRange(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Close(5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestTabCloseAdjustsActiveIndex(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	tm.Add("tab3", &LeafNode{Panel: "p3"})

	// Active is tab3 (index 2). Close tab1 (index 0).
	err := tm.Close(0)
	require.NoError(t, err)

	// Active index should shift down since a tab before it was removed
	assert.Equal(t, 1, tm.ActiveIndex())
	assert.Equal(t, "tab3", tm.ActiveTab().Name)
}

func TestTabCloseActiveMovesBack(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})

	// Active is tab2 (index 1). Close it.
	err := tm.Close(1)
	require.NoError(t, err)

	assert.Equal(t, 0, tm.ActiveIndex())
	assert.Equal(t, "tab1", tm.ActiveTab().Name)
}

func TestTabSelect(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})

	err := tm.Select(0)
	require.NoError(t, err)
	assert.Equal(t, 0, tm.ActiveIndex())
	assert.Equal(t, "tab1", tm.ActiveTab().Name)
}

func TestTabSelectOutOfRange(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Select(5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestTabRename(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Rename(0, "renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", tm.ActiveTab().Name)
}

func TestTabRenameOutOfRange(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Rename(5, "renamed")
	require.Error(t, err)
}

func TestTabMoveLeft(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	tm.Add("tab3", &LeafNode{Panel: "p3"})

	// Active is tab3 (index 2)
	tm.MoveLeft()
	assert.Equal(t, 1, tm.ActiveIndex())
	tabs := tm.Tabs()
	assert.Equal(t, "tab1", tabs[0].Name)
	assert.Equal(t, "tab3", tabs[1].Name)
	assert.Equal(t, "tab2", tabs[2].Name)
}

func TestTabMoveLeftAtStart(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	require.NoError(t, tm.Select(0))

	tm.MoveLeft() // no-op
	assert.Equal(t, 0, tm.ActiveIndex())
}

func TestTabMoveRight(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	require.NoError(t, tm.Select(0))

	tm.MoveRight()
	assert.Equal(t, 1, tm.ActiveIndex())
	tabs := tm.Tabs()
	assert.Equal(t, "tab2", tabs[0].Name)
	assert.Equal(t, "tab1", tabs[1].Name)
}

func TestTabMoveRightAtEnd(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})

	tm.MoveRight() // no-op, already at end
	assert.Equal(t, 1, tm.ActiveIndex())
}

func TestTabNextPrev(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	tm.Add("tab3", &LeafNode{Panel: "p3"})
	require.NoError(t, tm.Select(0))

	tm.NextTab()
	assert.Equal(t, 1, tm.ActiveIndex())

	tm.NextTab()
	assert.Equal(t, 2, tm.ActiveIndex())

	// Wrap around
	tm.NextTab()
	assert.Equal(t, 0, tm.ActiveIndex())

	// Prev wraps around
	tm.PrevTab()
	assert.Equal(t, 2, tm.ActiveIndex())

	tm.PrevTab()
	assert.Equal(t, 1, tm.ActiveIndex())
}

func TestTabNextPrevSingleTab(t *testing.T) {
	tm := NewTabManager("only", &LeafNode{Panel: "p1"})

	tm.NextTab() // no-op
	assert.Equal(t, 0, tm.ActiveIndex())

	tm.PrevTab() // no-op
	assert.Equal(t, 0, tm.ActiveIndex())
}

func TestTabsCopy(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tabs := tm.Tabs()

	// Modifying the copy shouldn't affect the manager
	tabs[0].Name = "modified"
	assert.Equal(t, "tab1", tm.ActiveTab().Name)
}
