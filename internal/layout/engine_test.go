package layout

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConfig returns a Config built from embedded defaults only, without
// reading the user's real config file. This keeps layout tests isolated from
// the host filesystem and prevents flakiness when xdg.ConfigHome is mutated
// concurrently by config package tests.
func testConfig() (*config.Config, error) {
	return config.LoadDefaults()
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	preset := ExplorerPreset()
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)
	return engine
}

func TestNewEngine(t *testing.T) {
	engine := newTestEngine(t)

	assert.NotNil(t, engine)
	assert.Len(t, engine.Panels(), 5) // filetree, gitinfo, github, commits, preview
	assert.Equal(t, []string{"filetree", "gitinfo", "github", "commits", "preview"}, engine.PanelOrder())
}

func TestNewEngineUnknownPanel(t *testing.T) {
	reg := NewRegistry()
	// Don't register any panels
	preset := ExplorerPreset()

	_, err := NewEngine(reg, preset)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "instantiate panel")
}

func TestEngineSetSize(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(80, 24)

	assert.Equal(t, 80, engine.Width())
	assert.Equal(t, 24, engine.Height())
}

func TestEngineFocusCycling(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(80, 24)

	assert.Equal(t, "filetree", engine.FocusedName())

	engine.FocusNext()
	assert.Equal(t, "gitinfo", engine.FocusedName())

	engine.FocusNext()
	assert.Equal(t, "github", engine.FocusedName())

	engine.FocusNext()
	assert.Equal(t, "commits", engine.FocusedName())

	engine.FocusNext()
	assert.Equal(t, "preview", engine.FocusedName())

	engine.FocusNext()
	assert.Equal(t, "filetree", engine.FocusedName()) // wraps

	engine.FocusPrev()
	assert.Equal(t, "preview", engine.FocusedName()) // wraps back
}

func TestEngineFocusedPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(80, 24)

	p := engine.FocusedPanel()
	require.NotNil(t, p)
	assert.Equal(t, "Files", p.Title())
}

func TestEnginePanelRects(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25) // 25 - 1 status bar - 1 hints bar - 0 tab bar (SingleTabMode) = 23 panel area
	// Inner area: 100-2=98 wide, 23-2=21 high (single outer border)

	rects := engine.PanelRects()
	require.Len(t, rects, 5)

	// usable = 98 - 1(sep) = 97; firstW = int(97*0.3) = 29
	// Left column: 29 wide, vertically split 0.35: filetree / (gitinfo/(github/commits))
	// usableH = 21 - 1(sep) = 20; filetreeH = int(20*0.35) = 7, bottomH = 20 - 7 = 13
	ft := rects["filetree"]
	assert.Equal(t, 0, ft.X)
	assert.Equal(t, 29, ft.Width)
	assert.Equal(t, 7, ft.Height)

	// Bottom-left: gitinfo/(github/commits) split 0.385
	// usableBottomH = 13 - 1(sep) = 12; gitinfoH = int(12*0.385) = 4, restH = 12 - 4 = 8
	gi := rects["gitinfo"]
	assert.Equal(t, 0, gi.X)
	assert.Equal(t, 29, gi.Width)
	assert.Equal(t, 4, gi.Height)

	// github/commits split 0.5
	// usableRestH = 8 - 1(sep) = 7; githubH = int(7*0.5) = 3, commitsH = 7 - 3 = 4
	gh := rects["github"]
	assert.Equal(t, 0, gh.X)
	assert.Equal(t, 29, gh.Width)
	assert.Equal(t, 3, gh.Height)

	cm := rects["commits"]
	assert.Equal(t, 0, cm.X)
	assert.Equal(t, 29, cm.Width)
	assert.Equal(t, 4, cm.Height)

	// secondX = 29 + 1 = 30; secondW = 97 - 29 = 68
	// Right column: preview at full height = 21
	pv := rects["preview"]
	assert.Equal(t, 30, pv.X)
	assert.Equal(t, 68, pv.Width)
	assert.Equal(t, 21, pv.Height)
}

func TestEngineZoom(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	assert.False(t, engine.IsZoomed())

	engine.ToggleZoom()
	assert.True(t, engine.IsZoomed())

	rects := engine.PanelRects()
	require.Len(t, rects, 1, "zoomed mode should show only focused panel")
	_, ok := rects["filetree"]
	assert.True(t, ok, "focused panel should be filetree")

	// Verify zoomed panel gets full inner area (outer border subtracted)
	r := rects["filetree"]
	assert.Equal(t, 98, r.Width)  // 100 - 2 (outer border)
	assert.Equal(t, 21, r.Height) // 23 - 2 (outer border)

	// Toggle back
	engine.ToggleZoom()
	assert.False(t, engine.IsZoomed())
	rects = engine.PanelRects()
	assert.Len(t, rects, 5)
}

func TestEngineResize(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 50) // use taller terminal so resize steps are measurable

	// Filetree is inside a vertical sub-split (filetree/(gitinfo/(github/commits))).
	// ResizeGrow on filetree adjusts the vertical ratio → changes height.
	rectsBefore := engine.PanelRects()
	ftHeightBefore := rectsBefore["filetree"].Height

	// Grow focused panel (filetree)
	engine.ResizeGrow()
	rectsAfter := engine.PanelRects()
	ftHeightAfter := rectsAfter["filetree"].Height
	assert.Greater(t, ftHeightAfter, ftHeightBefore)

	// Shrink it back
	engine.ResizeShrink()
	rectsAfterShrink := engine.PanelRects()
	ftHeightShrunk := rectsAfterShrink["filetree"].Height
	assert.Less(t, ftHeightShrunk, ftHeightAfter)
}

func TestEngineResizeClamp(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Shrink many times — should clamp at minRatio
	for range 50 {
		engine.ResizeShrink()
	}
	rects := engine.PanelRects()
	assert.Greater(t, rects["filetree"].Width, 0)
	assert.Greater(t, rects["preview"].Width, 0)

	// Grow many times — should clamp at maxRatio
	for range 100 {
		engine.ResizeGrow()
	}
	rects = engine.PanelRects()
	assert.Greater(t, rects["filetree"].Width, 0)
	assert.Greater(t, rects["preview"].Width, 0)
}

func TestEngineTabManager(t *testing.T) {
	engine := newTestEngine(t)
	tm := engine.TabManager()
	assert.NotNil(t, tm)
	assert.Equal(t, 1, tm.Count())
}

func TestEngineStatusBarHeight(t *testing.T) {
	engine := newTestEngine(t)
	assert.Equal(t, 1, engine.StatusBarHeight())
}

func TestEngineTabBarHeight(t *testing.T) {
	engine := newTestEngine(t)

	// In single-tab mode the tab bar takes 0 rows.
	assert.Equal(t, 0, engine.TabBarHeight())

	// Still 0 in single-tab mode even after adding a second tab.
	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)
	assert.Equal(t, 0, engine.TabBarHeight())
}

func TestEngineAddTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)

	tm := engine.TabManager()
	assert.Equal(t, 2, tm.Count())
	assert.Equal(t, 1, tm.ActiveIndex())
	assert.Equal(t, "git", tm.ActiveTab().Name)
	assert.Equal(t, []string{"filetree", "preview"}, engine.PanelOrder())
}

func TestEngineCloseActiveTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)
	assert.Equal(t, 2, engine.TabManager().Count())

	err = engine.CloseActiveTab()
	require.NoError(t, err)
	assert.Equal(t, 1, engine.TabManager().Count())
	assert.Equal(t, "explorer", engine.TabManager().ActiveTab().Name)
}

func TestEngineCloseLastTabFails(t *testing.T) {
	engine := newTestEngine(t)
	err := engine.CloseActiveTab()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last tab")
}

func TestEngineNextPrevTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)

	// We're on tab 1 (git). Go to next (wraps to 0).
	engine.NextTab()
	assert.Equal(t, 0, engine.TabManager().ActiveIndex())
	assert.Equal(t, "explorer", engine.TabManager().ActiveTab().Name)

	// Go prev (wraps to 1).
	engine.PrevTab()
	assert.Equal(t, 1, engine.TabManager().ActiveIndex())
	assert.Equal(t, "git", engine.TabManager().ActiveTab().Name)
}

func TestEngineSwitchTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)

	err = engine.SwitchTab(0)
	require.NoError(t, err)
	assert.Equal(t, 0, engine.TabManager().ActiveIndex())
	assert.Equal(t, "explorer", engine.TabManager().ActiveTab().Name)
}

func TestEngineSwitchTabOutOfRange(t *testing.T) {
	engine := newTestEngine(t)
	err := engine.SwitchTab(5)
	require.Error(t, err)
}

func TestEngineSplitFocusedVertical(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Focus is on "filetree". Split it vertically.
	_, err := engine.SplitFocusedVertical("preview")
	require.NoError(t, err)

	// Panel order should now include 6 panels (was 5 + 1 new).
	order := engine.PanelOrder()
	assert.Len(t, order, 6)
	assert.Equal(t, "filetree", order[0])

	// Focus should remain on filetree.
	assert.Equal(t, "filetree", engine.FocusedName())

	// The new panel name should be "preview:N" (unique).
	foundUnique := false
	for _, name := range order {
		if name != "filetree" && name != "preview" && name != "gitinfo" && name != "github" && name != "commits" {
			foundUnique = true
			assert.Contains(t, name, "preview:")
		}
	}
	assert.True(t, foundUnique, "should have a uniquely named panel")
}

func TestEngineSplitFocusedHorizontal(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Split filetree horizontally.
	_, err := engine.SplitFocusedHorizontal("preview")
	require.NoError(t, err)

	order := engine.PanelOrder()
	assert.Len(t, order, 6)
}

func TestEngineCloseFocusedPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Split first to have 6 panels (5 + 1 new).
	_, err := engine.SplitFocusedVertical("preview")
	require.NoError(t, err)
	assert.Len(t, engine.PanelOrder(), 6)

	// Close the focused panel (filetree).
	err = engine.CloseFocusedPanel()
	require.NoError(t, err)
	assert.Len(t, engine.PanelOrder(), 5)

	// Focused should be first remaining panel.
	assert.NotEmpty(t, engine.FocusedName())
}

func TestEngineCloseFocusedPanelLastFails(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)

	err = engine.CloseFocusedPanel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last panel")
}

func TestEngineRenameActiveTab(t *testing.T) {
	engine := newTestEngine(t)
	err := engine.RenameActiveTab("renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", engine.TabManager().ActiveTab().Name)
}

func TestEngineMoveTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)

	// Active is git (idx 1). Move left.
	engine.MoveTabLeft()
	assert.Equal(t, 0, engine.TabManager().ActiveIndex())
	assert.Equal(t, "git", engine.TabManager().ActiveTab().Name)

	// Move right.
	engine.MoveTabRight()
	assert.Equal(t, 1, engine.TabManager().ActiveIndex())
	assert.Equal(t, "git", engine.TabManager().ActiveTab().Name)
}

func TestEngineMultiTabPanelRects(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Single tab — in SingleTabMode, tab bar takes 0 rows.
	// Panel height = 25 - 1 (status) - 1 (hints) - 0 (tab) = 23.
	// Inner height = 23 - 2 (outer border) = 21.
	// filetree is in a vertical split (35% of left): usableH = 21-1 = 20, ftH = int(20*0.35) = 7
	rects := engine.PanelRects()
	totalHeight := rects["filetree"].Height
	assert.Equal(t, 7, totalHeight)

	// preview is the full right column = 21.
	assert.Equal(t, 21, rects["preview"].Height)

	// Add second tab — still same since SingleTabMode forces tab bar to 0.
	_, err := engine.AddTab(GitPreset())
	require.NoError(t, err)

	rects = engine.PanelRects()
	// git preset has filetree at full height (no vertical split).
	assert.Equal(t, 21, rects["filetree"].Height)
}

func TestEngineSinglePanelFocusCycling(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)

	assert.Equal(t, "solo", engine.FocusedName())

	// Cycling on single panel should be a no-op
	engine.FocusNext()
	assert.Equal(t, "solo", engine.FocusedName())

	engine.FocusPrev()
	assert.Equal(t, "solo", engine.FocusedName())
}

func TestEngineRotatePreviewPosition(t *testing.T) {
	// Use a simple 2-leaf preset (git) for rotate tests.
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	preset := GitPreset() // git preset: filetree | preview (simple 2-leaf)
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)
	engine.SetSize(100, 25)

	tab := engine.TabManager().ActiveTab()
	require.NotNil(t, tab)

	// Start: filetree | preview (Horizontal, filetree first)
	split := tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "filetree", split.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)

	// Rotate → bottom (Vertical, filetree top, preview bottom)
	engine.RotatePreviewPosition()
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "filetree", split.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)

	// Rotate → left (Horizontal, preview first)
	engine.RotatePreviewPosition()
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, "filetree", split.Second.(*LeafNode).Panel)

	// Rotate → top (Vertical, preview first)
	engine.RotatePreviewPosition()
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, "filetree", split.Second.(*LeafNode).Panel)

	// Rotate → back to right (Horizontal, filetree first)
	engine.RotatePreviewPosition()
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "filetree", split.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
}

func TestEngineCurrentPreviewPosition(t *testing.T) {
	// Use git preset (simple 2-leaf) for preview position tests.
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, GitPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	// Default is PreviewRight.
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())

	// Rotate to bottom and verify.
	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())

	// Rotate to left and verify.
	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewLeft, engine.CurrentPreviewPosition())

	// Rotate to top and verify.
	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewTop, engine.CurrentPreviewPosition())

	// Rotate back to right and verify.
	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
}

func TestEngineSetPreviewPosition(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, GitPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	tab := engine.TabManager().ActiveTab()
	require.NotNil(t, tab)

	// Start at right.
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())

	// Set to Bottom directly.
	engine.SetPreviewPosition(PreviewBottom)
	split := tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "filetree", split.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())

	// Set to Left directly.
	engine.SetPreviewPosition(PreviewLeft)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, "filetree", split.Second.(*LeafNode).Panel)
	assert.Equal(t, PreviewLeft, engine.CurrentPreviewPosition())

	// Set to Top directly.
	engine.SetPreviewPosition(PreviewTop)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, "filetree", split.Second.(*LeafNode).Panel)
	assert.Equal(t, PreviewTop, engine.CurrentPreviewPosition())

	// Set to Right directly.
	engine.SetPreviewPosition(PreviewRight)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "filetree", split.First.(*LeafNode).Panel)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
}

func TestEngineSetPreviewPositionNoOp(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, GitPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	// Already at right — SetPreviewPosition(PreviewRight) should be no-op.
	tab := engine.TabManager().ActiveTab()
	require.NotNil(t, tab)

	splitBefore := tab.Tree.(*SplitNode)
	ratioBefore := splitBefore.Ratio

	engine.SetPreviewPosition(PreviewRight)

	splitAfter := tab.Tree.(*SplitNode)
	assert.Equal(t, ratioBefore, splitAfter.Ratio, "no-op should not change ratio")
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
}

func TestEngineSetPreviewPositionAllTabs(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, GitPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	// Add a second git tab (simple 2-leaf preset).
	_, err = engine.AddTab(GitPreset())
	require.NoError(t, err)
	// AddTab switches to the new tab (index 1). Go back to tab 0.
	require.NoError(t, engine.SwitchTab(0))

	// Both tabs should start at the default (PreviewRight).
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())

	// Set to Bottom — should apply to ALL tabs.
	engine.SetPreviewPosition(PreviewBottom)
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())

	// Switch to the second tab and verify position persisted.
	require.NoError(t, engine.SwitchTab(1))
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition(),
		"preview position should apply to all tabs, not just the active one")

	// Switch back to tab 0 and confirm it's still Bottom.
	require.NoError(t, engine.SwitchTab(0))
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())
}

func TestEnginePreviewPositionExplorerPreset(t *testing.T) {
	// Explorer preset has a complex tree: H-split(leftCol, preview).
	// Preview position must work on this layout, not just simple 2-leaf.
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, ExplorerPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	tab := engine.TabManager().ActiveTab()
	require.NotNil(t, tab)

	// Start: left column | preview (Horizontal, preview is Second).
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
	split := tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)

	// Set to Bottom — rest panels on top, preview on bottom.
	engine.SetPreviewPosition(PreviewBottom)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
	// Complex rest gets 70% of height.
	assert.Equal(t, 0.7, split.Ratio)
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())
	// Non-preview panels are preserved.
	rest := split.First.(*SplitNode)
	assert.Equal(t, "filetree", rest.First.(*LeafNode).Panel)

	// Set to Left — preview on left, rest on right.
	engine.SetPreviewPosition(PreviewLeft)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, PreviewLeft, engine.CurrentPreviewPosition())

	// Set to Top — preview on top, rest on bottom.
	engine.SetPreviewPosition(PreviewTop)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Vertical, split.Direction)
	assert.Equal(t, "preview", split.First.(*LeafNode).Panel)
	assert.Equal(t, 0.3, split.Ratio)
	assert.Equal(t, PreviewTop, engine.CurrentPreviewPosition())

	// Set back to Right — rest on left, preview on right.
	engine.SetPreviewPosition(PreviewRight)
	split = tab.Tree.(*SplitNode)
	assert.Equal(t, Horizontal, split.Direction)
	assert.Equal(t, "preview", split.Second.(*LeafNode).Panel)
	assert.Equal(t, 0.3, split.Ratio)
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
}

func TestEngineRotatePreviewPositionExplorerPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)
	engine, err := NewEngine(reg, ExplorerPreset())
	require.NoError(t, err)
	engine.SetSize(100, 25)

	// Rotate through all 4 positions and back.
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())

	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewBottom, engine.CurrentPreviewPosition())

	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewLeft, engine.CurrentPreviewPosition())

	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewTop, engine.CurrentPreviewPosition())

	engine.RotatePreviewPosition()
	assert.Equal(t, PreviewRight, engine.CurrentPreviewPosition())
}

// ---------------------------------------------------------------------------
// Drag resize tests
// ---------------------------------------------------------------------------

func TestEngineDragResizeHorizontal(t *testing.T) {
	engine := newTestEngine(t)
	// Explorer preset: horizontal split at 0.3
	// Inner area: 98 × 21 (100-2 × 23-2, outer border)
	// usable = 97, firstW = int(97*0.3) = 29, separator at col 29.
	engine.SetSize(100, 25)

	assert.False(t, engine.IsDragging())

	// Click on the separator. Terminal X=30 → innerX=29 (border offset).
	clickMsg := tea.MouseClickMsg{X: 30, Y: 6, Button: tea.MouseLeft}
	engine.Update(clickMsg)

	assert.True(t, engine.IsDragging(), "click on border should start drag")

	// Drag to terminal X=42 → innerX=41, ratio=41/98≈0.4184
	// firstW = int(97*0.4184) = 40, secondW = 97-40 = 57
	motionMsg := tea.MouseMotionMsg{X: 42, Y: 6}
	engine.Update(motionMsg)

	rects := engine.PanelRects()
	assert.Equal(t, 40, rects["filetree"].Width, "filetree should now be 40 cols")
	assert.Equal(t, 57, rects["preview"].Width, "preview should now be 57 cols")

	// Release ends the drag.
	releaseMsg := tea.MouseReleaseMsg{X: 42, Y: 6, Button: tea.MouseLeft}
	engine.Update(releaseMsg)

	assert.False(t, engine.IsDragging(), "release should end drag")

	// Verify the ratio stuck after release.
	rects = engine.PanelRects()
	assert.Equal(t, 40, rects["filetree"].Width)
}

func TestEngineDragResizeClamps(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)
	// Inner area: 98 × 21. Separator at col 29.

	// Start drag on border. Terminal X=30 → innerX=29.
	clickMsg := tea.MouseClickMsg{X: 30, Y: 6, Button: tea.MouseLeft}
	engine.Update(clickMsg)
	require.True(t, engine.IsDragging())

	// Drag to extreme left (X=2) → innerX=1, ratio=1/98≈0.010, clamped to 0.1.
	// firstW = int(97*0.1) = 9
	motionMsg := tea.MouseMotionMsg{X: 2, Y: 6}
	engine.Update(motionMsg)

	rects := engine.PanelRects()
	assert.Equal(t, 9, rects["filetree"].Width, "should clamp to minRatio=10%")

	// Drag to extreme right (X=98) → innerX=97, ratio=97/98≈0.99, clamped to 0.9.
	// firstW = int(97*0.9) = 87
	motionMsg = tea.MouseMotionMsg{X: 98, Y: 6}
	engine.Update(motionMsg)

	rects = engine.PanelRects()
	assert.Equal(t, 87, rects["filetree"].Width, "should clamp to maxRatio=90%")

	engine.Update(tea.MouseReleaseMsg{X: 98, Y: 6, Button: tea.MouseLeft})
}

func TestEngineDragIgnoredWhenZoomed(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)
	engine.ToggleZoom()

	// Click on where the border would be — should not start drag.
	clickMsg := tea.MouseClickMsg{X: 30, Y: 6, Button: tea.MouseLeft}
	engine.Update(clickMsg)

	assert.False(t, engine.IsDragging(), "drag should not start when zoomed")
}

func TestEngineDragIgnoredOffBorder(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Click in the middle of filetree (X=15) — no border there.
	clickMsg := tea.MouseClickMsg{X: 15, Y: 6, Button: tea.MouseLeft}
	engine.Update(clickMsg)

	assert.False(t, engine.IsDragging(), "click away from border should not start drag")
}

func TestEngineDragRightClick(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Right-click on border — should not start drag.
	clickMsg := tea.MouseClickMsg{X: 30, Y: 6, Button: tea.MouseRight}
	engine.Update(clickMsg)

	assert.False(t, engine.IsDragging(), "right-click should not start drag")
}

func TestEngineMotionWithoutDrag(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Motion event without prior drag start should be a no-op (no panic).
	motionMsg := tea.MouseMotionMsg{X: 50, Y: 10}
	engine.Update(motionMsg)
	assert.False(t, engine.IsDragging())
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestEngineUpdatePanelAtMouse_RoutesToCorrectPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Focus is on filetree (leftmost panel). Mouse wheel on the right
	// (preview panel area) should not change focus.
	rects := engine.PanelRects()
	previewRect := rects["preview"]

	// Wheel message at the center of preview panel.
	wheelX := previewRect.X + previewRect.Width/2 + borderSize
	wheelY := previewRect.Y + previewRect.Height/2 + borderSize

	wheelMsg := tea.MouseWheelMsg{
		X:      wheelX,
		Y:      wheelY,
		Button: tea.MouseWheelDown,
	}

	engine.Update(wheelMsg)
	// Focus should remain on filetree (mouse wheel doesn't change focus).
	assert.Equal(t, "filetree", engine.FocusedName())
}

func TestEngineUpdatePanelAtMouse_Fallback(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Mouse event outside any panel rect falls back to focused panel.
	wheelMsg := tea.MouseWheelMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseWheelDown,
	}
	engine.Update(wheelMsg)
	assert.Equal(t, "filetree", engine.FocusedName())
}

func TestEngineHandleMouseClick_FocusesPanelOnClick(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Initially focused on filetree.
	assert.Equal(t, "filetree", engine.FocusedName())

	rects := engine.PanelRects()
	previewRect := rects["preview"]

	// Click inside preview panel.
	clickX := previewRect.X + previewRect.Width/2 + borderSize
	clickY := previewRect.Y + previewRect.Height/2 + borderSize

	clickMsg := tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}

	engine.Update(clickMsg)
	assert.Equal(t, "preview", engine.FocusedName(),
		"clicking on preview panel should focus it")
}

func TestEngineHandleMouseClick_RightClickRoutesToPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Initially focused on filetree.
	assert.Equal(t, "filetree", engine.FocusedName())

	rects := engine.PanelRects()
	previewRect := rects["preview"]

	clickX := previewRect.X + previewRect.Width/2 + engine.BorderSize()
	clickY := previewRect.Y + previewRect.Height/2 + engine.TabBarHeight() + engine.BorderSize()

	clickMsg := tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseRight,
	}

	engine.Update(clickMsg)
	assert.Equal(t, "preview", engine.FocusedName(),
		"right-click should focus the clicked panel")
}

func TestEngineHandleMouseRightClick_FocusesPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Initially focused on filetree.
	assert.Equal(t, "filetree", engine.FocusedName())

	rects := engine.PanelRects()
	// Find any panel that is not currently focused.
	focused := engine.FocusedName()
	for name, r := range rects {
		if name != focused {
			clickX := r.X + r.Width/2 + engine.BorderSize()
			clickY := r.Y + r.Height/2 + engine.TabBarHeight() + engine.BorderSize()
			engine.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseRight})
			assert.Equal(t, name, engine.FocusedName(),
				"right-clicking on %s should focus it", name)
			break
		}
	}
}

func TestEngineHandleMouseRightClick_SendsCorrectCoordinates(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	rects := engine.PanelRects()
	require.NotEmpty(t, rects)

	// Pick the first panel to test coordinate conversion.
	for name, r := range rects {
		// Build terminal coordinates that land at content position (2, 3).
		wantRow := 2
		wantCol := 3
		termX := r.X + PanelPadH + wantCol + engine.BorderSize()
		termY := r.Y + wantRow + engine.TabBarHeight() + engine.BorderSize()

		// Ensure the click lands within the panel.
		if termX-engine.BorderSize() < r.X || termX-engine.BorderSize() >= r.X+r.Width {
			continue
		}
		if termY-engine.TabBarHeight()-engine.BorderSize() < r.Y || termY-engine.TabBarHeight()-engine.BorderSize() >= r.Y+r.Height {
			continue
		}

		engine.Update(tea.MouseClickMsg{X: termX, Y: termY, Button: tea.MouseRight})
		assert.Equal(t, name, engine.FocusedName(),
			"right-click should focus panel %s", name)
		// The message was routed; if panel is a placeholder it silently
		// consumes it. Focus assertion is the observable effect.
		break
	}
}

func TestEngineHandleMouseClick_OutsidePanels(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Click way outside panels.
	clickMsg := tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft}
	engine.Update(clickMsg)
	// Should not panic, focus unchanged.
	assert.Equal(t, "filetree", engine.FocusedName())
}

func TestEngineUpdateBroadcast(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Non-key/mouse messages should be broadcast to all panels.
	// Use a type that isn't key/mouse.
	type customMsg struct{}
	engine.Update(customMsg{})
	// Should not panic.
}

func TestEngineUpdateFocused_EmptyPanels(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)
	engine.SetSize(80, 24)

	// Key message routes to focused panel.
	keyMsg := tea.KeyPressMsg{Code: 'j'}
	engine.Update(keyMsg)
	// Should not panic.
}

func TestEnginePanelRectsNilTab(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)
	// Don't set size → width and height are 0.
	rects := engine.PanelRects()
	// Even with zero size, engine computes rects for registered panels.
	assert.NotNil(t, rects)
	assert.Contains(t, rects, "solo")
}

func TestEngineSetSizeSmall(t *testing.T) {
	engine := newTestEngine(t)

	// Very small size should not panic.
	engine.SetSize(1, 1)
	rects := engine.PanelRects()
	// Panels should all have at least 1 width/height.
	for _, r := range rects {
		assert.GreaterOrEqual(t, r.Width, 1)
		assert.GreaterOrEqual(t, r.Height, 1)
	}
}

func TestEngineBorderSize(t *testing.T) {
	engine := newTestEngine(t)
	assert.Equal(t, 1, engine.BorderSize())
}

func TestEngineInnerArea(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	area := engine.InnerArea()
	assert.Equal(t, 0, area.X)
	assert.Equal(t, 0, area.Y)
	assert.Equal(t, 98, area.Width)  // 100 - 2*borderSize
	assert.Equal(t, 21, area.Height) // 25-1(status)-1(hints)-0(tabbar)-2(border)=21
}

func TestEngineInnerArea_SmallSize(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(3, 5)

	area := engine.InnerArea()
	assert.GreaterOrEqual(t, area.Width, 1)
	assert.GreaterOrEqual(t, area.Height, 1)
}

func TestSingleTabMode(t *testing.T) {
	assert.True(t, SingleTabMode, "SingleTabMode should be true in v1")
}

func TestPreviewPositionString(t *testing.T) {
	assert.Equal(t, "right", PreviewRight.String())
	assert.Equal(t, "bottom", PreviewBottom.String())
	assert.Equal(t, "left", PreviewLeft.String())
	assert.Equal(t, "top", PreviewTop.String())
	assert.Equal(t, "right", PreviewPosition(99).String()) // unknown defaults to right
}

func TestPreviewPositionFromString(t *testing.T) {
	assert.Equal(t, PreviewRight, PreviewPositionFromString("right"))
	assert.Equal(t, PreviewBottom, PreviewPositionFromString("bottom"))
	assert.Equal(t, PreviewLeft, PreviewPositionFromString("left"))
	assert.Equal(t, PreviewTop, PreviewPositionFromString("top"))
	assert.Equal(t, PreviewRight, PreviewPositionFromString("unknown"))
	assert.Equal(t, PreviewRight, PreviewPositionFromString(""))
}

func TestIsUniqueInstanceName(t *testing.T) {
	assert.True(t, isUniqueInstanceName("preview:1"))
	assert.True(t, isUniqueInstanceName("preview:42"))
	assert.False(t, isUniqueInstanceName("preview"))
	assert.False(t, isUniqueInstanceName("filetree"))
	assert.False(t, isUniqueInstanceName(""))
}

func TestEngineMouseRelease_NoDrag(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Release without drag should not panic.
	releaseMsg := tea.MouseReleaseMsg{X: 50, Y: 10, Button: tea.MouseLeft}
	engine.Update(releaseMsg)
	assert.False(t, engine.IsDragging())
}

func TestEngineZoomRecalcsPanelSizes(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Before zoom: 5 panels.
	rects := engine.PanelRects()
	assert.Len(t, rects, 5)

	engine.ToggleZoom()
	rects = engine.PanelRects()
	assert.Len(t, rects, 1)

	// The zoomed panel should get the full inner area.
	for _, r := range rects {
		assert.Equal(t, 98, r.Width)
		assert.Equal(t, 21, r.Height)
	}
}

func TestEngineCloseFocusedPanel_UpdatesPanelOrder(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 25)

	// Split to add a panel.
	_, err := engine.SplitFocusedVertical("preview")
	require.NoError(t, err)
	initialOrder := make([]string, len(engine.PanelOrder()))
	copy(initialOrder, engine.PanelOrder())
	assert.Len(t, initialOrder, 6)

	// Close the focused panel.
	err = engine.CloseFocusedPanel()
	require.NoError(t, err)
	assert.Len(t, engine.PanelOrder(), 5)
	assert.NotEmpty(t, engine.FocusedName())
}

func TestTabActiveTabEmpty(t *testing.T) {
	tm := &TabManager{tabs: nil, activeIdx: 0}
	assert.Nil(t, tm.ActiveTab())
	assert.Equal(t, 0, tm.Count())
}

func TestTabSelectNegativeIndex(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Select(-1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestTabRenameNegativeIndex(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	err := tm.Rename(-1, "new")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestTabCloseNegativeIndex(t *testing.T) {
	tm := NewTabManager("tab1", &LeafNode{Panel: "p1"})
	tm.Add("tab2", &LeafNode{Panel: "p2"})
	err := tm.Close(-1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

// ---------------------------------------------------------------------------
// Additional coverage: Init, FirstPanelOf, InnerArea, isNode
// ---------------------------------------------------------------------------

func TestEngineInit(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	cmd := engine.Init(t.Context())
	assert.NotNil(t, cmd, "Init should return batched commands")
	// After init, first panel should be focused.
	assert.Equal(t, "filetree", engine.FocusedName())
}

func TestFirstPanelOf_Leaf(t *testing.T) {
	leaf := &LeafNode{Panel: "myPanel"}
	assert.Equal(t, "myPanel", FirstPanelOf(leaf))
}

func TestFirstPanelOf_Split(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		First:     &LeafNode{Panel: "left"},
		Second:    &LeafNode{Panel: "right"},
	}
	assert.Equal(t, "left", FirstPanelOf(tree))
}

func TestFirstPanelOf_DeepNested(t *testing.T) {
	tree := &SplitNode{
		Direction: Horizontal,
		First: &SplitNode{
			Direction: Vertical,
			First:     &LeafNode{Panel: "topLeft"},
			Second:    &LeafNode{Panel: "bottomLeft"},
		},
		Second: &LeafNode{Panel: "right"},
	}
	assert.Equal(t, "topLeft", FirstPanelOf(tree))
}

func TestSplitNodeIsNode(t *testing.T) {
	s := &SplitNode{}
	s.isNode() // Should not panic — just satisfies interface.
}

func TestLeafNodeIsNode(t *testing.T) {
	l := &LeafNode{Panel: "p"}
	l.isNode() // Should not panic — just satisfies interface.
}

func TestInnerArea_Normal(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	area := engine.InnerArea()
	assert.True(t, area.Width > 0, "inner width should be positive")
	assert.True(t, area.Height > 0, "inner height should be positive")
	assert.True(t, area.Width < 100, "inner width should be less than total width")
	assert.True(t, area.Height < 30, "inner height should be less than total height")
}

func TestInnerArea_TinySize(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(3, 3)

	area := engine.InnerArea()
	// Width and height should be clamped to at least 1.
	assert.True(t, area.Width >= 1, "inner width should be at least 1")
	assert.True(t, area.Height >= 1, "inner height should be at least 1")
}

func TestEngineNextTab_SingleTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	before := engine.tabs.ActiveIndex()
	engine.NextTab()
	assert.Equal(t, before, engine.tabs.ActiveIndex(), "single tab should not change")
}

func TestEnginePrevTab_SingleTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	before := engine.tabs.ActiveIndex()
	engine.PrevTab()
	assert.Equal(t, before, engine.tabs.ActiveIndex(), "single tab should not change")
}

func TestEngineResizeGrow(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.ResizeGrow()
	// Should not panic; actual effect depends on split ratios.
}

func TestEngineResizeShrink(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 50) // tall enough to make shrink measurable

	// Record filetree height before shrink (filetree is the focused panel
	// inside a vertical sub-split, so ResizeShrink adjusts its ratio).
	before := engine.PanelRects()["filetree"].Height

	engine.ResizeShrink()

	after := engine.PanelRects()["filetree"].Height
	assert.LessOrEqual(t, after, before, "focused panel height must not increase after ResizeShrink")
}

// ---------------------------------------------------------------------------
// Additional coverage: AddTab, ToggleZoom, handleMouseClick
// ---------------------------------------------------------------------------

func TestEngineAddTab_NewPreset(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	// Add a second tab with the Git preset.
	gitPreset := GitPreset()
	cmd, err := engine.AddTab(gitPreset)
	require.NoError(t, err)
	_ = cmd // cmd may be nil if all panels already exist
	assert.Equal(t, 2, engine.tabs.Count())
}

func TestEngineToggleZoom(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	assert.False(t, engine.IsZoomed())
	engine.ToggleZoom()
	assert.True(t, engine.IsZoomed())

	// When zoomed, PanelRects should return only the focused panel.
	rects := engine.PanelRects()
	assert.Len(t, rects, 1)
	assert.Contains(t, rects, engine.FocusedName())

	engine.ToggleZoom()
	assert.False(t, engine.IsZoomed())
}

func TestEngineHandleMouseClick_LeftClick(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	rects := engine.PanelRects()
	require.NotEmpty(t, rects)

	// Find a panel that's not focused, click on it.
	focused := engine.FocusedName()
	for name, r := range rects {
		if name != focused {
			clickX := r.X + r.Width/2 + engine.BorderSize()
			clickY := r.Y + r.Height/2 + engine.TabBarHeight() + engine.BorderSize()
			engine.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
			assert.Equal(t, name, engine.FocusedName(), "clicking on %s should focus it", name)
			break
		}
	}
}

func TestEngineFocusedPanel_Empty(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)

	p := engine.FocusedPanel()
	assert.NotNil(t, p)
	assert.Equal(t, "solo", engine.FocusedName())
}

func TestEngineCloseActiveTab_LastTab(t *testing.T) {
	engine := newTestEngine(t)
	err := engine.CloseActiveTab()
	require.Error(t, err, "should not be able to close the last tab")
}

func TestEngineCloseActiveTab_MultipleTab(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	gitPreset := GitPreset()
	_, err := engine.AddTab(gitPreset)
	require.NoError(t, err)
	assert.Equal(t, 2, engine.tabs.Count())

	err = engine.CloseActiveTab()
	require.NoError(t, err)
	assert.Equal(t, 1, engine.tabs.Count())
}

func TestEngineRenderTabBar_SingleTabMode(t *testing.T) {
	tabs := []Tab{{Name: "test", Tree: &LeafNode{Panel: "p1"}}}
	result := RenderTabBar(tabs, 0, 80)
	assert.Equal(t, "", result, "SingleTabMode should return empty tab bar")
}

func TestEngineSwitchTab_BackToFirst(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	gitPreset := GitPreset()
	_, err := engine.AddTab(gitPreset)
	require.NoError(t, err)

	// Switch back to first tab.
	_ = engine.SwitchTab(0)
	assert.Equal(t, 0, engine.tabs.ActiveIndex())
}

func TestEngineUpdateFocused_InvalidIdx(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	// Force an invalid focusIdx.
	engine.focusIdx = 999
	cmd := engine.updateFocused(tea.KeyPressMsg{Code: 'j'})
	assert.Nil(t, cmd, "invalid focusIdx should return nil")
}

func TestEngineCloseFocusedPanel_Success(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	panelsBefore := len(engine.PanelOrder())
	err := engine.CloseFocusedPanel()
	require.NoError(t, err)
	assert.Less(t, len(engine.PanelOrder()), panelsBefore)
}

func TestEngineCloseFocusedPanel_LastPanel(t *testing.T) {
	reg := NewRegistry()
	reg.Register("solo", func() panels.Panel {
		return panels.NewPlaceholder("solo")
	})
	preset := Preset{
		Name:   "single",
		Tree:   &LeafNode{Panel: "solo"},
		Panels: []string{"solo"},
	}
	engine, err := NewEngine(reg, preset)
	require.NoError(t, err)
	engine.SetSize(100, 30)

	err = engine.CloseFocusedPanel()
	require.Error(t, err, "should not close last panel")
	assert.Contains(t, err.Error(), "last panel")
}

func TestFirstPanelOf_NilNode(t *testing.T) {
	// When given a nil-typed interface, should return "".
	result := FirstPanelOf(nil)
	assert.Equal(t, "", result)
}

func TestEngineWidth_Height(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)
	assert.Equal(t, 120, engine.Width())
	assert.Equal(t, 40, engine.Height())
}

func TestEnginePanels_Contains(t *testing.T) {
	engine := newTestEngine(t)
	p := engine.Panels()
	assert.NotEmpty(t, p)
	assert.Contains(t, p, "filetree")
}

func TestEngineRenameActiveTab_Custom(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	err := engine.RenameActiveTab("Custom Name")
	require.NoError(t, err)
	assert.Equal(t, "Custom Name", engine.tabs.ActiveTab().Name)
}

func TestEngineMoveTabLeftRight(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	// Add a second tab so we can move.
	gitPreset := GitPreset()
	_, err := engine.AddTab(gitPreset)
	require.NoError(t, err)

	// Move left (should swap).
	engine.MoveTabLeft()
	// Move right.
	engine.MoveTabRight()
	// Should not panic.
}

func TestEngineSplitHorizontal(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	panelsBefore := len(engine.PanelOrder())
	_, err := engine.SplitFocusedHorizontal("preview")
	require.NoError(t, err)
	assert.Greater(t, len(engine.PanelOrder()), panelsBefore, "split should add a panel")
}

func TestEngineSplitVertical(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	panelsBefore := len(engine.PanelOrder())
	_, err := engine.SplitFocusedVertical("preview")
	require.NoError(t, err)
	assert.Greater(t, len(engine.PanelOrder()), panelsBefore, "split should add a panel")
}

func TestFocusedPanel_EmptyPanelOrder(t *testing.T) {
	engine := newTestEngine(t)
	engine.panelOrder = nil
	assert.Nil(t, engine.FocusedPanel())
	assert.Equal(t, "", engine.FocusedName())
}

func TestEngineCloseFocusedPanel_Split(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	// Focus a non-first panel, then close it.
	engine.FocusNext()
	focused := engine.FocusedName()
	err := engine.CloseFocusedPanel()
	require.NoError(t, err)
	// The closed panel should no longer be in the order.
	for _, name := range engine.PanelOrder() {
		assert.NotEqual(t, focused, name)
	}
}

func TestEngineIsDragging(t *testing.T) {
	engine := newTestEngine(t)
	assert.False(t, engine.IsDragging())
}

func TestEngineHandleDragEnd(t *testing.T) {
	engine := newTestEngine(t)
	engine.dragging = true
	engine.handleDragEnd()
	assert.False(t, engine.dragging)
	assert.Nil(t, engine.dragSplit)
}

func TestEngineUpdateBroadcast_Branch(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	// Send a branch selected message (broadcast to all panels).
	engine.Update(panels.BranchSelectedMsg{Name: "main"})
	// Should not panic.
}

func TestEngineTabBarHeight_Zero(t *testing.T) {
	engine := newTestEngine(t)
	assert.Equal(t, 0, engine.TabBarHeight(), "SingleTabMode should give 0 tab bar height")
}

func TestPresetsList(t *testing.T) {
	all := Presets()
	assert.GreaterOrEqual(t, len(all), 4, "should have at least 4 presets")
}

func TestGitPresetPanels(t *testing.T) {
	p := GitPreset()
	assert.Equal(t, "git", p.Name)
	assert.NotEmpty(t, p.Panels)
}

func TestReviewPresetPanels(t *testing.T) {
	p := ReviewPreset()
	assert.Equal(t, "review", p.Name)
	assert.NotEmpty(t, p.Panels)
}

func TestAgentPresetPanels(t *testing.T) {
	p := AgentPreset()
	assert.Equal(t, "agent", p.Name)
	assert.NotEmpty(t, p.Panels)
}

func TestFullPresetPanels(t *testing.T) {
	p := FullPreset()
	assert.Equal(t, "full", p.Name)
	assert.NotEmpty(t, p.Panels)
}

func TestEngineHandleDragMotion_Horizontal(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	// Simulate a drag in horizontal direction.
	engine.dragging = true
	engine.dragDir = Horizontal
	engine.dragArea = Rect{X: 10, Y: 0, Width: 80, Height: 20}
	engine.dragSplit = &SplitNode{
		Direction: Horizontal, Ratio: 0.5,
		First: &LeafNode{Panel: "filetree"}, Second: &LeafNode{Panel: "preview"},
	}

	engine.handleDragMotion(tea.MouseMotionMsg{X: 50, Y: 10})
	assert.NotEqual(t, 0.5, engine.dragSplit.Ratio, "ratio should have changed")
}

func TestEngineHandleDragMotion_Vertical(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	engine.dragging = true
	engine.dragDir = Vertical
	engine.dragArea = Rect{X: 0, Y: 5, Width: 80, Height: 20}
	engine.dragSplit = &SplitNode{
		Direction: Vertical, Ratio: 0.5,
		First: &LeafNode{Panel: "filetree"}, Second: &LeafNode{Panel: "preview"},
	}

	engine.handleDragMotion(tea.MouseMotionMsg{X: 40, Y: 10})
	assert.NotEqual(t, 0.5, engine.dragSplit.Ratio)
}

func TestEngineHandleDragMotion_ZeroWidthArea(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	engine.dragging = true
	engine.dragDir = Horizontal
	engine.dragArea = Rect{X: 0, Y: 0, Width: 0, Height: 20}
	engine.dragSplit = &SplitNode{Direction: Horizontal, Ratio: 0.5}

	cmd := engine.handleDragMotion(tea.MouseMotionMsg{X: 50, Y: 10})
	assert.Nil(t, cmd, "zero width area should return nil")
}

func TestEngineCloseFocusedPanel_UniqueInstance(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)

	// Split to create a uniquely-named panel instance.
	_, err := engine.SplitFocusedHorizontal("preview")
	require.NoError(t, err)

	// Close the focused panel (which should be the split-added instance).
	err = engine.CloseFocusedPanel()
	require.NoError(t, err)
}

func TestEngineAddTab_FullPreset(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	// Adding the Full preset triggers factory closure execution for
	// gitstatus, terminal panels (covering RegisterDefaults branches).
	fullPreset := FullPreset()
	_, err := engine.AddTab(fullPreset)
	require.NoError(t, err)
	assert.Equal(t, 2, engine.tabs.Count())
}

func TestEngineAddTab_ReviewPreset(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	reviewPreset := ReviewPreset()
	_, err := engine.AddTab(reviewPreset)
	require.NoError(t, err)
}

func TestEngineAddTab_AgentPreset(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(100, 30)
	engine.Init(t.Context())

	agentPreset := AgentPreset()
	_, err := engine.AddTab(agentPreset)
	require.NoError(t, err)
}

func TestRegistryHasAndNames(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)

	// Verify all expected panel types are registered.
	names := reg.Names()
	assert.Contains(t, names, "filetree")
	assert.Contains(t, names, "preview")
	assert.Contains(t, names, "gitstatus")
	assert.Contains(t, names, "commits")
	assert.True(t, reg.Has("filetree"))
	assert.True(t, reg.Has("preview"))
	assert.False(t, reg.Has("nonexistent"))
}

func TestRegistryCreate_AllPanels(t *testing.T) {
	reg := NewRegistry()
	cfg, err := testConfig()
	require.NoError(t, err)
	RegisterDefaults(reg, cfg, nil, nil)

	// Create each registered panel to exercise the factory closures.
	for _, name := range reg.Names() {
		panel, err := reg.Create(name)
		assert.NoError(t, err, "failed to create panel %q", name)
		assert.NotNil(t, panel, "panel %q should not be nil", name)
	}
}

// ---------------------------------------------------------------------------
// TargetedPanelMsg routing
// ---------------------------------------------------------------------------

func TestUpdate_TargetedPanelMsg_RoutesToNamedPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	// Send a TargetedPanelMsg aimed at "gitinfo". The inner message is a
	// BranchChangedMsg which gitinfo handles by scheduling a data reload.
	targeted := panels.TargetedPanelMsg{
		Target: "gitinfo",
		Inner:  panels.BranchChangedMsg{Name: "main"},
	}
	cmd := engine.Update(targeted)
	// The gitinfo panel should have processed the message (returned a cmd).
	// We just verify it didn't panic and the cmd is non-nil (data reload).
	assert.NotNil(t, cmd, "TargetedPanelMsg to gitinfo should produce a command")
}

func TestUpdate_TargetedPanelMsg_UnknownTargetIsNoop(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	targeted := panels.TargetedPanelMsg{
		Target: "nonexistent",
		Inner:  panels.BranchChangedMsg{Name: "main"},
	}
	cmd := engine.Update(targeted)
	assert.Nil(t, cmd, "TargetedPanelMsg to unknown panel should return nil")
}

// ---------------------------------------------------------------------------
// FocusByName
// ---------------------------------------------------------------------------

func TestEngineFocusByName_SwitchesPanel(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	// Starts focused on "filetree" (index 0).
	assert.Equal(t, "filetree", engine.FocusedName())

	// Switch to "preview" by name.
	ok := engine.FocusByName("preview")
	assert.True(t, ok)
	assert.Equal(t, "preview", engine.FocusedName())
}

func TestEngineFocusByName_UnknownReturnsFalse(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	original := engine.FocusedName()
	ok := engine.FocusByName("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, original, engine.FocusedName(), "focus should not change for unknown panel")
}

func TestEngineFocusByName_AlreadyFocusedIsNoOp(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	assert.Equal(t, "filetree", engine.FocusedName())

	// FocusByName on the already-focused panel should return true
	// but not change the focused panel.
	ok := engine.FocusByName("filetree")
	assert.True(t, ok)
	assert.Equal(t, "filetree", engine.FocusedName())
}

func TestUpdate_TargetedPanelMsg_DoesNotBroadcast(t *testing.T) {
	engine := newTestEngine(t)
	engine.SetSize(120, 40)

	// Get the initial preview panel state. A TargetedPanelMsg aimed at
	// "gitinfo" should NOT reach the preview panel.
	previewBefore := engine.Panels()["preview"]

	targeted := panels.TargetedPanelMsg{
		Target: "gitinfo",
		Inner:  panels.BranchChangedMsg{Name: "feature-branch"},
	}
	engine.Update(targeted)

	// The preview panel reference should be unchanged (not re-assigned by
	// the engine's Update loop) since TargetedPanelMsg skips broadcast.
	previewAfter := engine.Panels()["preview"]
	assert.Equal(t, previewBefore, previewAfter,
		"preview panel should be untouched when TargetedPanelMsg targets gitinfo")
}
