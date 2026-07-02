package preview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeLines returns n placeholder content lines for scroll math tests.
func makeLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line"
	}
	return lines
}

func TestGotoLine_CentersAndClamps(t *testing.T) {
	// newTestPreview sets height = 24, so viewportHeight() = 23 and the
	// centering offset is viewportHeight()/2 = 11.
	tests := []struct {
		name    string
		lines   int
		target  int
		wantTop int
	}{
		{name: "centers a middle line", lines: 100, target: 50, wantTop: 38},
		{name: "clamps below first line to top", lines: 100, target: 1, wantTop: 0},
		{name: "near-top line clamps to top", lines: 100, target: 5, wantTop: 0},
		{name: "clamps past last line to bottom", lines: 100, target: 1000, wantTop: 77},
		{name: "negative input clamps to top", lines: 100, target: -5, wantTop: 0},
		{name: "content shorter than viewport stays at top", lines: 5, target: 3, wantTop: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPreview(makeLines(tt.lines))
			p.gotoLine(tt.target)
			assert.Equal(t, tt.wantTop, p.scrollY)
		})
	}
}

func TestGotoLine_EmptyContentNoChange(t *testing.T) {
	p := newTestPreview(nil)
	p.scrollY = 0
	p.gotoLine(42)
	assert.Equal(t, 0, p.scrollY)
}

func TestGotoLine_IndependentOfWrapAndLineNumbers(t *testing.T) {
	// The target offset is computed from source line indices, so it must be
	// the same whether word-wrap or line-number display is on or off.
	p := newTestPreview(makeLines(100))
	p.wordWrap = true
	p.lineNumbers = false
	p.gotoLine(50)
	assert.Equal(t, 38, p.scrollY)

	p.wordWrap = false
	p.lineNumbers = true
	p.gotoLine(50)
	assert.Equal(t, 38, p.scrollY)
}

func TestGotoLinePrompt_OpenTypeCommit(t *testing.T) {
	p := newTestPreview(makeLines(100))

	// Open the prompt with "L".
	_, cmd := p.Update(keyMsg("L"))
	require.True(t, p.gotoLineActive, "prompt should be active after L")
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputStartedMsg)
	assert.True(t, ok, "opening should emit PreviewInputStartedMsg")

	// Type "42".
	p.Update(keyMsg("4"))
	p.Update(keyMsg("2"))
	assert.Equal(t, "42", p.gotoLineInput)

	// Backspace removes the last digit, then retype.
	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "4", p.gotoLineInput)
	p.Update(keyMsg("2"))
	assert.Equal(t, "42", p.gotoLineInput)

	// Enter commits: scroll to line 42, centered (42-1-11 = 30), and close.
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, p.gotoLineActive, "prompt should close after Enter")
	assert.Equal(t, "", p.gotoLineInput)
	assert.Equal(t, 30, p.scrollY)
	require.NotNil(t, cmd)
	_, ok = cmd().(panels.PreviewInputEndedMsg)
	assert.True(t, ok, "closing should emit PreviewInputEndedMsg")
}

func TestGotoLinePrompt_EscapeCancels(t *testing.T) {
	p := newTestPreview(makeLines(100))
	p.scrollY = 7

	p.Update(keyMsg("L"))
	p.Update(keyMsg("5"))
	require.True(t, p.gotoLineActive)

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, p.gotoLineActive, "Escape should close the prompt")
	assert.Equal(t, "", p.gotoLineInput)
	assert.Equal(t, 7, p.scrollY, "Escape must not change the scroll position")
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputEndedMsg)
	assert.True(t, ok)
}

func TestGotoLinePrompt_EmptyInputRejected(t *testing.T) {
	p := newTestPreview(makeLines(100))
	p.scrollY = 12

	p.Update(keyMsg("L"))
	// Commit with no digits entered.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, p.gotoLineActive)
	assert.Equal(t, 12, p.scrollY, "empty input must not change the view")
}

func TestGotoLinePrompt_NonDigitsIgnored(t *testing.T) {
	p := newTestPreview(makeLines(100))
	p.Update(keyMsg("L"))
	// Letters should not be added to the numeric entry.
	p.Update(keyMsg("a"))
	p.Update(keyMsg("z"))
	assert.Equal(t, "", p.gotoLineInput)
}

func TestGotoLinePrompt_RendersInView(t *testing.T) {
	p := newTestPreview(makeLines(100))
	p.filePath = "test.go"
	p.Update(keyMsg("L"))
	p.Update(keyMsg("7"))
	out := p.View(80, 24)
	assert.Contains(t, out, "Go to line: 7")
}

func TestGotoLine_NotAvailableInGitHubMode(t *testing.T) {
	p := newTestPreview(makeLines(100))
	p.ghMode = true
	_, cmd := p.Update(keyMsg("L"))
	assert.False(t, p.gotoLineActive, "prompt should not open in GitHub content mode")
	assert.Nil(t, cmd)
}
