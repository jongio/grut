package welcome

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPanel(t *testing.T) *Panel {
	t.Helper()
	p := New(nil)
	p.Focus()
	p.SetSize(80, 30)
	return p
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNew_ReturnsPanel(t *testing.T) {
	p := New(nil)
	require.NotNil(t, p)
	assert.Equal(t, "welcome", p.PanelTitle)
}

func TestNew_BuildsLines(t *testing.T) {
	p := New(nil)
	assert.Greater(t, len(p.lines), 0, "buildLines should populate content")
}

func TestNew_HeaderCount(t *testing.T) {
	p := New(nil)
	assert.Greater(t, p.headerCount, 0, "headerCount should be set")
	// Header includes banner lines, empty lines, and subtitle.
	assert.LessOrEqual(t, p.headerCount, len(p.lines))
}

func TestNew_StylesCached(t *testing.T) {
	p := New(nil)
	// Verify styles were initialized (non-zero value).
	var zeroStyle lipgloss.Style
	assert.NotEqual(t, zeroStyle, p.bannerStyle)
	assert.NotEqual(t, zeroStyle, p.keyStyle)
}

func TestNew_InitialState(t *testing.T) {
	p := New(nil)
	assert.Equal(t, 0, p.offset)
	assert.Equal(t, 0, p.animFrame)
	assert.False(t, p.animDone)
	assert.Equal(t, btnOK, p.focusedBtn)
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestPanel_ImplementsPanelInterface(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func TestInit_ReturnsTickCommand(t *testing.T) {
	p := newTestPanel(t)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return animation tick command")
}

// ---------------------------------------------------------------------------
// Animation
// ---------------------------------------------------------------------------

func TestAnimation_AdvancesFrame(t *testing.T) {
	p := newTestPanel(t)
	assert.Equal(t, 0, p.animFrame)
	assert.False(t, p.animDone)

	// Simulate one tick.
	updated, cmd := p.Update(AnimTickMsg{})
	p = updated.(*Panel)
	assert.Equal(t, 1, p.animFrame)
	assert.False(t, p.animDone)
	assert.NotNil(t, cmd, "should schedule next tick")
}

func TestAnimation_CompletesAtHeaderCount(t *testing.T) {
	p := newTestPanel(t)
	headerCount := p.headerCount

	// Animation completes when animFrame exceeds headerCount.
	for i := 0; i <= headerCount; i++ {
		updated, _ := p.Update(AnimTickMsg{})
		p = updated.(*Panel)
	}
	assert.True(t, p.animDone, "animation should be done after headerCount+1 ticks")
}

func TestAnimation_NoTickAfterDone(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	_, cmd := p.Update(AnimTickMsg{})
	assert.Nil(t, cmd, "no tick should be scheduled after animation done")
}

func TestAnimation_KeypressSkipsAnimation(t *testing.T) {
	p := newTestPanel(t)
	// Advance partially.
	_, _ = p.Update(AnimTickMsg{})
	assert.False(t, p.animDone)

	// Any keypress should skip to end.
	updated, _ := p.Update(tea.KeyPressMsg{Code: 'j'})
	p = updated.(*Panel)
	assert.True(t, p.animDone, "keypress should skip animation")
}

// ---------------------------------------------------------------------------
// Key handling — button cycling
// ---------------------------------------------------------------------------

func TestTab_CyclesButtonForward(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	assert.Equal(t, btnOK, p.focusedBtn)

	_, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, btnHelp, p.focusedBtn)

	// Wraps around.
	_, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, btnOK, p.focusedBtn)
}

func TestShiftTab_CyclesButtonBackward(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	assert.Equal(t, btnOK, p.focusedBtn)

	// Wraps to last button.
	updated, _ := p.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	p = updated.(*Panel)
	assert.Equal(t, btnHelp, p.focusedBtn)

	updated, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	p = updated.(*Panel)
	assert.Equal(t, btnOK, p.focusedBtn)
}

// ---------------------------------------------------------------------------
// Key handling — button activation
// ---------------------------------------------------------------------------

func TestEnter_OKButton_Dismisses(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.focusedBtn = btnOK

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(DismissMsg)
	require.True(t, ok, "OK button should produce DismissMsg")
}

func TestEnter_HelpButton_DismissesAndTogglesHelp(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.focusedBtn = btnHelp

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "Help button should produce a batch command")
}

func TestEscape_Dismisses(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(DismissMsg)
	require.True(t, ok, "Esc should produce DismissMsg")
}

func TestQuestionMark_DismissesAndTogglesHelp(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	_, cmd := p.Update(tea.KeyPressMsg{Code: '?'})
	require.NotNil(t, cmd, "? should produce a batch command")
}

// ---------------------------------------------------------------------------
// Scrolling
// ---------------------------------------------------------------------------

func TestScrollDown_AdvancesOffset(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.SetSize(80, 5) // Short viewport to enable scrolling.
	assert.Equal(t, 0, p.offset)

	_, _ = p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.offset)
}

func TestScrollUp_DecreasesOffset(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.SetSize(80, 5)
	p.offset = 5

	_, _ = p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 4, p.offset)
}

func TestScrollUp_ClampsAtZero(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.offset = 0

	_, _ = p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.offset)
}

func TestScrollDown_ClampsAtMax(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.SetSize(80, 5)

	// Scroll well past the end.
	for i := 0; i < 200; i++ {
		p.scrollDown()
	}

	maxOffset := len(p.lines) - p.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	assert.LessOrEqual(t, p.offset, maxOffset)
}

func TestMouseWheel_Scrolls(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.SetSize(80, 5)

	// Wheel down.
	_, _ = p.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	assert.Greater(t, p.offset, 0, "mouse wheel down should scroll")

	// Wheel up.
	_, _ = p.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	assert.Equal(t, 0, p.offset, "mouse wheel up should scroll back")
}

func TestSetSize_ReclampsOffset(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true
	p.SetSize(80, 5) // Short viewport.

	// Scroll down several times.
	for i := 0; i < 20; i++ {
		p.scrollDown()
	}
	scrolled := p.offset
	assert.Greater(t, scrolled, 0)

	// Grow the terminal so all content fits.
	p.SetSize(80, len(p.lines)+10)
	assert.Equal(t, 0, p.offset, "offset should reclamp to 0 when terminal grows")
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestView_ZeroDimensions(t *testing.T) {
	p := New(nil)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(10, -1))
}

func TestView_ContainsBanner(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	view := p.View(80, 30)
	assert.Contains(t, view, "╭──╮", "banner should contain box drawing chars")
}

func TestView_ContainsSubtitle(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	view := p.View(80, 30)
	assert.Contains(t, view, "file explorer")
}

func TestView_ContainsSections(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	view := p.View(80, 40)
	assert.Contains(t, view, "Panel Focus")
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Commands")
	assert.Contains(t, view, "Git")
}

func TestView_ContainsKeyBindings(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	view := p.View(80, 40)
	assert.Contains(t, view, "Fuzzy finder")
	assert.Contains(t, view, "Stage / unstage file")
}

func TestView_ContainsButtons(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	view := p.View(80, 30)
	assert.Contains(t, view, "OK")
	assert.Contains(t, view, "Help")
	assert.NotContains(t, view, "Show Later")
}

func TestView_AnimationHidesLinesBeforeFrame(t *testing.T) {
	p := newTestPanel(t)
	p.animFrame = 1 // Only first line visible.
	p.animDone = false

	view := p.View(80, 30)
	// Should have content (at least one line + buttons).
	assert.NotEmpty(t, view)
}

func TestView_NarrowWidth(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	// Should not panic even with very narrow width.
	view := p.View(10, 30)
	assert.NotEmpty(t, view)
}

func TestView_MinimalHeight(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	// height=1: contentHeight=0, only button bar area exists but gets skipped.
	view := p.View(80, 1)
	assert.Empty(t, view, "height=1 produces no room for content or buttons")

	// height=2: contentHeight=1, should render at least one line + button bar.
	view = p.View(80, 2)
	assert.NotEmpty(t, view, "height=2 should render content")
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings_ReturnsBindings(t *testing.T) {
	p := New(nil)
	bindings := p.KeyBindings()
	assert.Greater(t, len(bindings), 0)

	// Check expected keys exist.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["Tab/S-Tab"], "should have Tab binding")
	assert.True(t, keys["Enter"], "should have Enter binding")
	assert.True(t, keys["Esc"], "should have Esc binding")
}

// ---------------------------------------------------------------------------
// Display-width consistency (regression test for Unicode padding bug)
// ---------------------------------------------------------------------------

func TestBuildLines_BindLinesUseTabSeparator(t *testing.T) {
	p := New(nil)
	for _, line := range p.lines {
		if strings.HasPrefix(line, "bind:") {
			content := strings.TrimPrefix(line, "bind:")
			parts := strings.SplitN(content, "\t", 2)
			assert.Len(t, parts, 2, "bind line should have tab separator: %q", line)
			assert.NotEmpty(t, parts[0], "key should not be empty: %q", line)
			assert.NotEmpty(t, parts[1], "description should not be empty: %q", line)
		}
	}
}

func TestBuildLines_AccentLinesUseTabSeparator(t *testing.T) {
	p := New(nil)
	for _, line := range p.lines {
		if strings.HasPrefix(line, "accent:") {
			content := strings.TrimPrefix(line, "accent:")
			parts := strings.SplitN(content, "\t", 2)
			assert.Len(t, parts, 2, "accent line should have tab separator: %q", line)
		}
	}
}

// ---------------------------------------------------------------------------
// Unrecognized messages
// ---------------------------------------------------------------------------

func TestUpdate_UnknownMessage_NoOp(t *testing.T) {
	p := newTestPanel(t)
	p.animDone = true

	type unknownMsg struct{}
	updated, cmd := p.Update(unknownMsg{})
	assert.Equal(t, p, updated.(*Panel))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestButtonConstants(t *testing.T) {
	assert.Equal(t, 0, btnOK)
	assert.Equal(t, 1, btnHelp)
	assert.Equal(t, 2, btnCount)
	assert.Len(t, buttonLabels, btnCount)
}
