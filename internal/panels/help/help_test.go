package help

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPanel creates a focused help panel with default dimensions.
func newTestPanel(t *testing.T) *Panel {
	t.Helper()
	p := New()
	p.Focus()
	p.SetSize(60, 30)
	p.Init(context.Background())
	return p
}

// ---------------------------------------------------------------------------
// Panel creation and interface compliance
// ---------------------------------------------------------------------------

func TestNew_CreatesPanel(t *testing.T) {
	p := New()
	assert.NotNil(t, p)
	assert.Equal(t, "help", p.Title())
	assert.True(t, p.lineCount() > 0, "should have content lines")
}

func TestPanel_ImplementsPanelInterface(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

func TestPanel_FocusBlur(t *testing.T) {
	p := New()
	p.Focus()
	assert.True(t, p.Focused)
	p.Blur()
	assert.False(t, p.Focused)
}

func TestPanel_SetSize(t *testing.T) {
	p := New()
	p.SetSize(80, 24)
	assert.Equal(t, 80, p.Width)
	assert.Equal(t, 24, p.Height)
}

func TestPanel_InitReturnsNil(t *testing.T) {
	p := New()
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestView_ContainsGlobalSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 30)
	assert.Contains(t, view, "Global")
}

func TestView_ContainsNavigationSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 60)
	assert.Contains(t, view, "Navigation")
}

func TestView_ContainsFileTreeSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 100)
	assert.Contains(t, view, "File Tree")
}

func TestView_ContainsGitInfoSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 100)
	assert.Contains(t, view, "Git Info")
}

func TestView_ContainsGitHubSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 150)
	assert.Contains(t, view, "GitHub")
}

func TestView_ContainsCommitsSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 200)
	assert.Contains(t, view, "Commits")
}

func TestView_ContainsPreviewSection(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 200)
	assert.Contains(t, view, "Preview")
}

func TestView_ContainsCloseHint(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 200)
	assert.Contains(t, view, "Press ? or Esc to close")
}

func TestView_ContainsKeyBindings(t *testing.T) {
	p := newTestPanel(t)
	view := p.View(60, 200)

	// Spot-check key bindings across sections.
	assert.Contains(t, view, "1-5")
	assert.Contains(t, view, "Focus panel by number")
	assert.Contains(t, view, "j/k")
	assert.Contains(t, view, "Cursor down/up")
	assert.Contains(t, view, "ctrl+c")
	assert.Contains(t, view, "Quit")
}

func TestView_ZeroDimensions(t *testing.T) {
	p := New()
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(-5, -5))
}

func TestView_SmallDimensions(t *testing.T) {
	p := New()
	p.SetSize(10, 3)
	view := p.View(10, 3)
	assert.NotEmpty(t, view, "should render something even with small dimensions")
}

// ---------------------------------------------------------------------------
// Scrolling
// ---------------------------------------------------------------------------

func TestScroll_Down(t *testing.T) {
	p := newTestPanel(t)
	// Set small height so scrolling is possible.
	p.SetSize(60, 5)
	assert.Equal(t, 0, p.scrollOffset())

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.scrollOffset())

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.scrollOffset())
}

func TestScroll_Up(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)

	// Scroll down first.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.scrollOffset())

	// Scroll back up.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.scrollOffset())
}

func TestScroll_UpAtTop(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)
	assert.Equal(t, 0, p.scrollOffset())

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.scrollOffset(), "should not scroll above 0")
}

func TestScroll_DownBeyondContent(t *testing.T) {
	p := newTestPanel(t)
	// Set height larger than content — scrolling should be a no-op.
	p.SetSize(60, 500)

	for i := 0; i < 100; i++ {
		p.Update(tea.KeyPressMsg{Code: 'j'})
	}
	assert.Equal(t, 0, p.scrollOffset(), "should not scroll when content fits")
}

func TestScroll_DownWithArrowKey(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)

	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, p.scrollOffset())
}

func TestScroll_UpWithArrowKey(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)

	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, p.scrollOffset())
}

// ---------------------------------------------------------------------------
// Close on Esc
// ---------------------------------------------------------------------------

func TestClose_Escape(t *testing.T) {
	p := newTestPanel(t)

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(panels.ToggleHelpMsg)
	assert.True(t, ok, "escape should emit ToggleHelpMsg, got %T", msg)
}

// ---------------------------------------------------------------------------
// Close on ?
// ---------------------------------------------------------------------------

func TestClose_QuestionMark(t *testing.T) {
	p := newTestPanel(t)

	_, cmd := p.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(panels.ToggleHelpMsg)
	assert.True(t, ok, "? should emit ToggleHelpMsg, got %T", msg)
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	p := New()
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)
	assert.Len(t, bindings, 4)

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/↓"])
	assert.True(t, keys["k/↑"])
	assert.True(t, keys["?"])
	assert.True(t, keys["escape"])
}

// ---------------------------------------------------------------------------
// Content has all major sections
// ---------------------------------------------------------------------------

func TestContent_AllSections(t *testing.T) {
	p := New()
	// Render full content at large height to see everything.
	view := p.View(80, 500)

	expectedSections := []string{
		"Global", "Navigation", "File Tree",
		"Git Info", "GitHub", "Commits", "Preview",
	}
	for _, sec := range expectedSections {
		assert.Contains(t, view, sec, "should contain section: %s", sec)
	}
}

func TestContent_AllKeyBindings(t *testing.T) {
	p := New()
	view := p.View(80, 500)

	expectedKeys := []string{
		"1-5", "Tab", "R", "P", "F", "?",
		"j/k", "g/G", "d/u", "Enter",
		"h/l", "f", "n", "N", "x",
		"ctrl+c",
	}
	for _, key := range expectedKeys {
		assert.Contains(t, view, key, "should contain key binding: %s", key)
	}
}

// ---------------------------------------------------------------------------
// Non-key messages are no-ops
// ---------------------------------------------------------------------------

func TestUpdate_NonKeyMsg(t *testing.T) {
	p := newTestPanel(t)
	updated, cmd := p.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	assert.Equal(t, p, updated)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Mouse wheel tests
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollDown(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5) // Small height so scrolling is possible.

	assert.Equal(t, 0, p.scrollOffset())

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 3, p.scrollOffset(), "mouse wheel down should scroll by 3")
}

func TestMouseWheel_ScrollUp(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)

	// Scroll down first.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Greater(t, p.scrollOffset(), 0)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.scrollOffset(), "mouse wheel up should scroll back to 0")
}

func TestMouseWheel_ScrollUpClampsToZero(t *testing.T) {
	p := newTestPanel(t)
	p.SetSize(60, 5)

	assert.Equal(t, 0, p.scrollOffset())

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.scrollOffset(), "should not scroll below 0")
}

func TestMouseWheel_ScrollDownClampsToMax(t *testing.T) {
	p := newTestPanel(t)
	// Set height larger than content — scrolling should clamp.
	p.SetSize(60, 500)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 0, p.scrollOffset(), "should not scroll when content fits")
}
