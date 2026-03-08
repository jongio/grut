package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeafNodePanelNames(t *testing.T) {
	leaf := &LeafNode{Panel: "filetree"}
	assert.Equal(t, []string{"filetree"}, leaf.PanelNames())
}

func TestSplitNodePanelNames(t *testing.T) {
	split := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}
	names := split.PanelNames()
	assert.Equal(t, []string{"filetree", "preview"}, names)
}

func TestNestedSplitNodePanelNames(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Vertical,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "preview"},
			Second:    &LeafNode{Panel: "terminal"},
		},
	}
	names := tree.PanelNames()
	assert.Equal(t, []string{"filetree", "preview", "terminal"}, names)
}

func TestLeafNodeClone(t *testing.T) {
	leaf := &LeafNode{Panel: "filetree"}
	clone := leaf.Clone().(*LeafNode)

	assert.Equal(t, leaf.Panel, clone.Panel)
	// Verify it's a deep copy
	clone.Panel = "changed"
	assert.NotEqual(t, leaf.Panel, clone.Panel)
}

func TestSplitNodeClone(t *testing.T) {
	split := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}
	clone := split.Clone().(*SplitNode)

	assert.Equal(t, split.Ratio, clone.Ratio)
	assert.Equal(t, split.Direction, clone.Direction)

	// Verify deep copy
	clone.Ratio = 0.7
	assert.NotEqual(t, split.Ratio, clone.Ratio)

	cloneFirst := clone.First.(*LeafNode)
	cloneFirst.Panel = "changed"
	origFirst := split.First.(*LeafNode)
	assert.NotEqual(t, origFirst.Panel, cloneFirst.Panel)
}

func TestResolveLeaf(t *testing.T) {
	leaf := &LeafNode{Panel: "filetree"}
	area := Rect{X: 0, Y: 0, Width: 80, Height: 24}

	rects := Resolve(leaf, area)
	require.Len(t, rects, 1)
	assert.Equal(t, area, rects["filetree"])
}

func TestResolveHorizontalSplit(t *testing.T) {
	split := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 24}

	rects := Resolve(split, area)
	require.Len(t, rects, 2)

	// usable = 100 - 1 (separator) = 99; firstW = int(99*0.3) = 29
	ft := rects["filetree"]
	assert.Equal(t, 0, ft.X)
	assert.Equal(t, 29, ft.Width)
	assert.Equal(t, 24, ft.Height)

	// secondX = 0 + 29 + 1 (separator) = 30; secondW = 99 - 29 = 70
	pv := rects["preview"]
	assert.Equal(t, 30, pv.X)
	assert.Equal(t, 70, pv.Width)
	assert.Equal(t, 24, pv.Height)
}

func TestResolveVerticalSplit(t *testing.T) {
	split := &SplitNode{
		Direction: Vertical,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "top"},
		Second:    &LeafNode{Panel: "bottom"},
	}
	area := Rect{X: 0, Y: 0, Width: 80, Height: 20}

	rects := Resolve(split, area)
	require.Len(t, rects, 2)

	// usable = 20 - 1 (separator) = 19; topH = int(19*0.5) = 9
	top := rects["top"]
	assert.Equal(t, 0, top.Y)
	assert.Equal(t, 9, top.Height)
	assert.Equal(t, 80, top.Width)

	// bottomY = 0 + 9 + 1 = 10; bottomH = 19 - 9 = 10
	bottom := rects["bottom"]
	assert.Equal(t, 10, bottom.Y)
	assert.Equal(t, 10, bottom.Height)
	assert.Equal(t, 80, bottom.Width)
}

func TestResolveNestedSplit(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "left"},
		Second: &SplitNode{
			Direction: Vertical,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "top-right"},
			Second:    &LeafNode{Panel: "bottom-right"},
		},
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 20}

	rects := Resolve(tree, area)
	require.Len(t, rects, 3)

	// Outer horizontal: usable = 99, leftW = int(99*0.5) = 49
	left := rects["left"]
	assert.Equal(t, 0, left.X)
	assert.Equal(t, 49, left.Width)
	assert.Equal(t, 20, left.Height)

	// Right side: X = 50, W = 99 - 49 = 50
	// Inner vertical: usable = 19, topH = int(19*0.5) = 9
	topRight := rects["top-right"]
	assert.Equal(t, 50, topRight.X)
	assert.Equal(t, 50, topRight.Width)
	assert.Equal(t, 9, topRight.Height)

	// bottomY = 0 + 9 + 1 = 10; bottomH = 19 - 9 = 10
	bottomRight := rects["bottom-right"]
	assert.Equal(t, 50, bottomRight.X)
	assert.Equal(t, 50, bottomRight.Width)
	assert.Equal(t, 10, bottomRight.Height)
}

func TestResolveMinimusSizes(t *testing.T) {
	// Even with extreme ratios, minimum 1 width/height is maintained
	split := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.01, // very small first child
		First:     &LeafNode{Panel: "tiny"},
		Second:    &LeafNode{Panel: "big"},
	}
	area := Rect{X: 0, Y: 0, Width: 10, Height: 5}

	rects := Resolve(split, area)
	require.Len(t, rects, 2)

	assert.GreaterOrEqual(t, rects["tiny"].Width, 1)
	assert.GreaterOrEqual(t, rects["big"].Width, 1)
	// Total width = first + 1 (separator) + second = area.Width
	assert.Equal(t, area.Width, rects["tiny"].Width+1+rects["big"].Width)
}

func TestDirectionString(t *testing.T) {
	assert.Equal(t, "horizontal", Horizontal.String())
	assert.Equal(t, "vertical", Vertical.String())
	assert.Equal(t, "unknown", Direction(99).String())
}

func TestFindSplitContaining(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}

	split, side := FindSplitContaining(tree, "filetree")
	assert.Equal(t, tree, split)
	assert.Equal(t, "first", side)

	split, side = FindSplitContaining(tree, "preview")
	assert.Equal(t, tree, split)
	assert.Equal(t, "second", side)

	split, side = FindSplitContaining(tree, "nonexistent")
	assert.Nil(t, split)
	assert.Empty(t, side)
}

func TestFindSplitContainingNested(t *testing.T) {
	inner := &SplitNode{
		Direction: Vertical,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "top"},
		Second:    &LeafNode{Panel: "bottom"},
	}
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "left"},
		Second:    inner,
	}

	split, side := FindSplitContaining(tree, "top")
	assert.Equal(t, inner, split)
	assert.Equal(t, "first", side)
}

func TestFindSplitContainingLeaf(t *testing.T) {
	leaf := &LeafNode{Panel: "solo"}
	split, side := FindSplitContaining(leaf, "solo")
	assert.Nil(t, split)
	assert.Empty(t, side)
}

// ---------------------------------------------------------------------------
// SplitLeaf tests
// ---------------------------------------------------------------------------

func TestSplitLeafSimple(t *testing.T) {
	// Splitting a single leaf produces a SplitNode.
	root := &LeafNode{Panel: "preview"}
	result := SplitLeaf(root, "preview", Vertical, "terminal")

	split, ok := result.(*SplitNode)
	require.True(t, ok, "result should be a SplitNode")
	assert.Equal(t, Vertical, split.Direction)
	assert.InDelta(t, 0.5, split.Ratio, 0.001)

	first, ok := split.First.(*LeafNode)
	require.True(t, ok)
	assert.Equal(t, "preview", first.Panel)

	second, ok := split.Second.(*LeafNode)
	require.True(t, ok)
	assert.Equal(t, "terminal", second.Panel)
}

func TestSplitLeafInTree(t *testing.T) {
	// Split a leaf inside an existing tree.
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}

	result := SplitLeaf(root, "preview", Vertical, "terminal")

	// Root should remain the same SplitNode.
	split, ok := result.(*SplitNode)
	require.True(t, ok)
	assert.Equal(t, Horizontal, split.Direction)

	// First child unchanged.
	first, ok := split.First.(*LeafNode)
	require.True(t, ok)
	assert.Equal(t, "filetree", first.Panel)

	// Second child should now be a SplitNode.
	inner, ok := split.Second.(*SplitNode)
	require.True(t, ok, "second child should now be a SplitNode")
	assert.Equal(t, Vertical, inner.Direction)
	assert.Equal(t, "preview", inner.First.(*LeafNode).Panel)
	assert.Equal(t, "terminal", inner.Second.(*LeafNode).Panel)
}

func TestSplitLeafNotFound(t *testing.T) {
	root := &LeafNode{Panel: "filetree"}
	result := SplitLeaf(root, "nonexistent", Horizontal, "new")
	assert.Equal(t, root, result, "tree unchanged when target not found")
}

func TestSplitLeafNested(t *testing.T) {
	// Deeply nested split.
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "gitstatus"},
			Second:    &LeafNode{Panel: "preview"},
		},
	}

	result := SplitLeaf(root, "gitstatus", Vertical, "diff")

	// Navigate to the new split.
	outer := result.(*SplitNode)
	inner := outer.Second.(*SplitNode)
	newSplit, ok := inner.First.(*SplitNode)
	require.True(t, ok, "gitstatus should now be in a split")
	assert.Equal(t, Vertical, newSplit.Direction)
	assert.Equal(t, "gitstatus", newSplit.First.(*LeafNode).Panel)
	assert.Equal(t, "diff", newSplit.Second.(*LeafNode).Panel)
}

func TestSplitLeafPanelNames(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "a"},
		Second:    &LeafNode{Panel: "b"},
	}

	result := SplitLeaf(root, "b", Vertical, "c")
	names := result.PanelNames()
	assert.Equal(t, []string{"a", "b", "c"}, names)
}

// ---------------------------------------------------------------------------
// RemoveLeaf tests
// ---------------------------------------------------------------------------

func TestRemoveLeafFromSplit(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}

	result, found := RemoveLeaf(root, "preview")
	require.True(t, found)

	leaf, ok := result.(*LeafNode)
	require.True(t, ok, "removing one child should collapse to sibling")
	assert.Equal(t, "filetree", leaf.Panel)
}

func TestRemoveLeafFirstChild(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}

	result, found := RemoveLeaf(root, "filetree")
	require.True(t, found)

	leaf, ok := result.(*LeafNode)
	require.True(t, ok)
	assert.Equal(t, "preview", leaf.Panel)
}

func TestRemoveLeafNested(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "gitstatus"},
			Second:    &LeafNode{Panel: "preview"},
		},
	}

	result, found := RemoveLeaf(root, "gitstatus")
	require.True(t, found)

	// Inner split should collapse, leaving filetree | preview.
	outer, ok := result.(*SplitNode)
	require.True(t, ok)
	assert.Equal(t, "filetree", outer.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", outer.Second.(*LeafNode).Panel)
}

func TestRemoveLeafNotFound(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "a"},
		Second:    &LeafNode{Panel: "b"},
	}

	result, found := RemoveLeaf(root, "nonexistent")
	assert.False(t, found)
	assert.Equal(t, root, result)
}

func TestRemoveLeafSingleLeaf(t *testing.T) {
	root := &LeafNode{Panel: "solo"}

	result, found := RemoveLeaf(root, "solo")
	assert.True(t, found)
	assert.Nil(t, result, "removing the only leaf returns nil")
}

func TestRemoveLeafPanelNames(t *testing.T) {
	root := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "a"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "b"},
			Second:    &LeafNode{Panel: "c"},
		},
	}

	result, found := RemoveLeaf(root, "b")
	require.True(t, found)
	names := result.PanelNames()
	assert.Equal(t, []string{"a", "c"}, names)
}

// ---------------------------------------------------------------------------
// FindSplitAtBorder tests
// ---------------------------------------------------------------------------

func TestFindSplitAtBorderHorizontal(t *testing.T) {
	// Horizontal split: filetree | preview, border (separator) at X=29.
	// usable = 100 - 1 = 99; firstW = int(99*0.3) = 29; separator at col 29.
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 24}

	// Click exactly on border (separator column 29).
	split, dir, splitArea := FindSplitAtBorder(tree, 29, 12, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, tree, split)
	assert.Equal(t, Horizontal, dir)
	assert.Equal(t, area, splitArea)

	// Click 1 cell left of border (within hit zone).
	split, dir, _ = FindSplitAtBorder(tree, 28, 12, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, Horizontal, dir)

	// Click 1 cell right of border (within hit zone).
	split, dir, _ = FindSplitAtBorder(tree, 30, 12, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, Horizontal, dir)

	// Click 2 cells away (outside hit zone).
	split, _, _ = FindSplitAtBorder(tree, 31, 12, area, 1)
	assert.Nil(t, split)
}

func TestFindSplitAtBorderVertical(t *testing.T) {
	// Vertical split: top | bottom, border (separator) at Y=9.
	// usable = 20 - 1 = 19; firstH = int(19*0.5) = 9; separator at row 9.
	tree := &SplitNode{
		Direction: Vertical,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "top"},
		Second:    &LeafNode{Panel: "bottom"},
	}
	area := Rect{X: 0, Y: 0, Width: 80, Height: 20}

	// Click exactly on border (separator row 9).
	split, dir, splitArea := FindSplitAtBorder(tree, 40, 9, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, tree, split)
	assert.Equal(t, Vertical, dir)
	assert.Equal(t, area, splitArea)

	// Click 1 row above border (within hit zone).
	split, dir, _ = FindSplitAtBorder(tree, 40, 8, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, Vertical, dir)

	// Click 2 rows away (outside hit zone).
	split, _, _ = FindSplitAtBorder(tree, 40, 7, area, 1)
	assert.Nil(t, split)
}

func TestFindSplitAtBorderNested(t *testing.T) {
	// left | (top-right / bottom-right)
	// Outer horizontal separator at X=49. Inner vertical separator at Y=9 within right side.
	inner := &SplitNode{
		Direction: Vertical,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "top-right"},
		Second:    &LeafNode{Panel: "bottom-right"},
	}
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "left"},
		Second:    inner,
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 20}

	// Click on outer horizontal border (X=50).
	split, dir, _ := FindSplitAtBorder(tree, 50, 5, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, tree, split)
	assert.Equal(t, Horizontal, dir)

	// Click on inner vertical border (Y=10) in the right half (X=75).
	split, dir, splitArea := FindSplitAtBorder(tree, 75, 10, area, 1)
	require.NotNil(t, split)
	assert.Equal(t, inner, split, "inner split should be found (deeper takes precedence)")
	assert.Equal(t, Vertical, dir)
	// The inner split's area is the right half: X=50, Width=50.
	assert.Equal(t, 50, splitArea.X)
	assert.Equal(t, 50, splitArea.Width)
}

func TestFindSplitAtBorderMiss(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "a"},
		Second:    &LeafNode{Panel: "b"},
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 20}

	// Click far from border (center of left panel).
	split, _, _ := FindSplitAtBorder(tree, 25, 10, area, 1)
	assert.Nil(t, split)

	// Click far from border (center of right panel).
	split, _, _ = FindSplitAtBorder(tree, 75, 10, area, 1)
	assert.Nil(t, split)
}

func TestFindSplitAtBorderLeafNode(t *testing.T) {
	// A leaf node has no borders.
	leaf := &LeafNode{Panel: "solo"}
	area := Rect{X: 0, Y: 0, Width: 80, Height: 24}

	split, _, _ := FindSplitAtBorder(leaf, 40, 12, area, 1)
	assert.Nil(t, split)
}

func TestFindSplitAtBorderYOutOfRange(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.5,
		First:     &LeafNode{Panel: "a"},
		Second:    &LeafNode{Panel: "b"},
	}
	area := Rect{X: 0, Y: 0, Width: 100, Height: 20}

	// Click on border X but outside the Y range.
	split, _, _ := FindSplitAtBorder(tree, 50, 20, area, 1)
	assert.Nil(t, split)

	split, _, _ = FindSplitAtBorder(tree, 50, -1, area, 1)
	assert.Nil(t, split)
}

func TestIntAbs(t *testing.T) {
	assert.Equal(t, 5, intAbs(5))
	assert.Equal(t, 5, intAbs(-5))
	assert.Equal(t, 0, intAbs(0))
}
