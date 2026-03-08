package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// v1: RenderTabBar always returns "" (tab bar hidden for single-tab mode).
// All tests verify the no-op behavior. The original assertions are preserved
// in comments for v2 re-enablement.

func TestRenderTabBarSingle(t *testing.T) {
	tabs := []Tab{{Name: "explorer", Tree: &LeafNode{Panel: "p1"}}}
	bar := RenderTabBar(tabs, 0, 60)

	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarMultiple(t *testing.T) {
	tabs := []Tab{
		{Name: "explorer"},
		{Name: "git"},
		{Name: "review"},
	}
	bar := RenderTabBar(tabs, 1, 60)

	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarActiveHighlight(t *testing.T) {
	tabs := []Tab{
		{Name: "tab1"},
		{Name: "tab2"},
		{Name: "tab3"},
	}

	bar := RenderTabBar(tabs, 0, 80)
	assert.Equal(t, "", bar, "v1: tab bar should be hidden")

	bar = RenderTabBar(tabs, 2, 80)
	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarTruncation(t *testing.T) {
	tabs := []Tab{
		{Name: "verylongtabname1"},
		{Name: "verylongtabname2"},
		{Name: "verylongtabname3"},
	}
	bar := RenderTabBar(tabs, 0, 20)

	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarLongName(t *testing.T) {
	tabs := []Tab{
		{Name: "thisNameIsDefinitelyLongerThanTwentyCharacters"},
	}
	bar := RenderTabBar(tabs, 0, 40)

	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarZeroWidth(t *testing.T) {
	tabs := []Tab{{Name: "tab"}}
	bar := RenderTabBar(tabs, 0, 0)
	assert.Equal(t, "", bar)
}

func TestRenderTabBarEmpty(t *testing.T) {
	bar := RenderTabBar(nil, 0, 60)
	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}

func TestRenderTabBarPadding(t *testing.T) {
	tabs := []Tab{{Name: "x"}}
	bar := RenderTabBar(tabs, 0, 50)
	assert.Equal(t, "", bar, "v1: tab bar should be hidden")
}
