package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExplorerPreset(t *testing.T) {
	preset := ExplorerPreset()

	assert.Equal(t, "explorer", preset.Name)
	assert.Equal(t, []string{"filetree", "gitinfo", "github", "commits", "preview"}, preset.Panels)

	// Verify tree structure: (filetree / (gitinfo / (github / commits))) | preview
	split, ok := preset.Tree.(*SplitNode)
	assert.True(t, ok, "root should be SplitNode")
	assert.Equal(t, Horizontal, split.Direction)
	assert.InDelta(t, 0.3, split.Ratio, 0.001)

	left, ok := split.First.(*SplitNode)
	assert.True(t, ok, "left child should be SplitNode (vertical)")
	assert.Equal(t, Vertical, left.Direction)
	assert.InDelta(t, 0.35, left.Ratio, 0.001)
	assert.Equal(t, "filetree", left.First.(*LeafNode).Panel)

	leftBottom, ok := left.Second.(*SplitNode)
	assert.True(t, ok, "left-bottom should be SplitNode (vertical)")
	assert.Equal(t, Vertical, leftBottom.Direction)
	assert.InDelta(t, 0.385, leftBottom.Ratio, 0.001)
	assert.Equal(t, "gitinfo", leftBottom.First.(*LeafNode).Panel)

	leftBottomBottom, ok := leftBottom.Second.(*SplitNode)
	assert.True(t, ok, "left-bottom-bottom should be SplitNode (vertical)")
	assert.Equal(t, Vertical, leftBottomBottom.Direction)
	assert.InDelta(t, 0.5, leftBottomBottom.Ratio, 0.001)
	assert.Equal(t, "github", leftBottomBottom.First.(*LeafNode).Panel)
	assert.Equal(t, "commits", leftBottomBottom.Second.(*LeafNode).Panel)

	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
}

func TestPresetsMap(t *testing.T) {
	presets := Presets()
	assert.Contains(t, presets, "explorer")
	assert.Contains(t, presets, "git")
	assert.Contains(t, presets, "review")
	assert.Contains(t, presets, "agent")
	assert.Contains(t, presets, "full")
	assert.Len(t, presets, 5)
}

func TestGitPreset(t *testing.T) {
	preset := GitPreset()
	assert.Equal(t, "git", preset.Name)
	assert.Equal(t, []string{"filetree", "preview"}, preset.Panels)
}

func TestReviewPreset(t *testing.T) {
	preset := ReviewPreset()
	assert.Equal(t, "review", preset.Name)
	assert.Equal(t, []string{"filetree", "review", "context"}, preset.Panels)

	// Verify tree structure: filetree | (review | context)
	split, ok := preset.Tree.(*SplitNode)
	assert.True(t, ok)
	assert.Equal(t, Horizontal, split.Direction)
	assert.InDelta(t, 0.2, split.Ratio, 0.001)

	first, ok := split.First.(*LeafNode)
	assert.True(t, ok)
	assert.Equal(t, "filetree", first.Panel)

	inner, ok := split.Second.(*SplitNode)
	assert.True(t, ok)
	assert.Equal(t, Horizontal, inner.Direction)
	assert.Equal(t, "review", inner.First.(*LeafNode).Panel)
	assert.Equal(t, "context", inner.Second.(*LeafNode).Panel)
}

func TestAgentPreset(t *testing.T) {
	preset := AgentPreset()
	assert.Equal(t, "agent", preset.Name)
	assert.Equal(t, []string{"filetree", "terminal", "agents"}, preset.Panels)

	split, ok := preset.Tree.(*SplitNode)
	assert.True(t, ok)
	assert.Equal(t, Horizontal, split.Direction)
	assert.InDelta(t, 0.2, split.Ratio, 0.001)
}

func TestFullPreset(t *testing.T) {
	preset := FullPreset()
	assert.Equal(t, "full", preset.Name)
	assert.Equal(t, []string{"filetree", "gitstatus", "preview", "terminal"}, preset.Panels)

	// All panel names should be valid.
	assert.Len(t, preset.Panels, 4)
}

func TestPresetTreesAreValid(t *testing.T) {
	for name, preset := range Presets() {
		t.Run(name, func(t *testing.T) {
			assert.NotNil(t, preset.Tree, "preset %q tree should not be nil", name)
			assert.NotEmpty(t, preset.Panels, "preset %q should have panels", name)

			// PanelNames from tree should match declared Panels.
			treeNames := preset.Tree.PanelNames()
			assert.Equal(t, preset.Panels, treeNames, "preset %q tree/panels mismatch", name)

			// Resolve should work without panicking.
			area := Rect{X: 0, Y: 0, Width: 200, Height: 50}
			rects := Resolve(preset.Tree, area)
			assert.Len(t, rects, len(preset.Panels))

			for _, pName := range preset.Panels {
				r, ok := rects[pName]
				assert.True(t, ok, "panel %q should have a rect", pName)
				assert.Greater(t, r.Width, 0)
				assert.Greater(t, r.Height, 0)
			}
		})
	}
}
