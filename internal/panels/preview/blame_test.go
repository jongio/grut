package preview

import (
	"fmt"
	"testing"
	"time"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

// --- Blame recency color tests ---

func TestBlameRecencyColor(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		date     time.Time
		expected string
	}{
		{
			name:     "1_day_ago_bright_white",
			date:     now.Add(-24 * time.Hour),
			expected: "#FFFFFF",
		},
		{
			name:     "5_days_ago_bright_white",
			date:     now.Add(-5 * 24 * time.Hour),
			expected: "#FFFFFF",
		},
		{
			name:     "14_days_ago_light_gray",
			date:     now.Add(-14 * 24 * time.Hour),
			expected: "#CCCCCC",
		},
		{
			name:     "90_days_ago_medium_gray",
			date:     now.Add(-90 * 24 * time.Hour),
			expected: "#999999",
		},
		{
			name:     "270_days_ago_dim_gray",
			date:     now.Add(-270 * 24 * time.Hour),
			expected: "#777777",
		},
		{
			name:     "730_days_ago_very_dim",
			date:     now.Add(-730 * 24 * time.Hour),
			expected: "#555555",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := blameRecencyColor(tc.date, now)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBlameRecencyColorBoundaries(t *testing.T) {
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Exactly at the 7-day boundary — should be light gray, not white
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	assert.Equal(t, "#CCCCCC", blameRecencyColor(sevenDaysAgo, now))

	// Just under 7 days — bright white
	justUnderWeek := now.Add(-6*24*time.Hour - 23*time.Hour)
	assert.Equal(t, "#FFFFFF", blameRecencyColor(justUnderWeek, now))
}

// --- Blame annotation formatting tests ---

func TestFormatBlameAnnotation(t *testing.T) {
	bl := git.BlameLine{
		Hash:    "abc1234def5678",
		Author:  "John",
		Date:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		LineNo:  1,
		Content: "hello world",
	}

	result := formatBlameAnnotation(bl)
	assert.Contains(t, result, "abc1234")
	assert.Contains(t, result, "John")
	assert.Contains(t, result, "2024-01")
	assert.Contains(t, result, "│")
}

func TestFormatBlameAnnotationTruncatesHash(t *testing.T) {
	bl := git.BlameLine{
		Hash:   "abcdef1234567890abcdef1234567890",
		Author: "Alice",
		Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	result := formatBlameAnnotation(bl)
	// Hash should be truncated to 7 chars
	assert.Contains(t, result, "abcdef1")
	assert.NotContains(t, result, "abcdef12")
}

func TestFormatBlameAnnotationTruncatesAuthor(t *testing.T) {
	bl := git.BlameLine{
		Hash:   "abc1234",
		Author: "Very Long Author Name",
		Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	result := formatBlameAnnotation(bl)
	// Author should be truncated to 10 chars
	assert.Contains(t, result, "Very Long ")
	assert.NotContains(t, result, "Very Long Author")
}

func TestFormatBlameAnnotationShortHash(t *testing.T) {
	bl := git.BlameLine{
		Hash:   "abc",
		Author: "Bob",
		Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	}

	result := formatBlameAnnotation(bl)
	// Short hash should be padded
	assert.Contains(t, result, "abc")
	assert.Contains(t, result, "Bob")
	assert.Contains(t, result, "2024-03")
}

// --- Blame toggle tests ---

func TestBlameToggleOn(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.Focus()
	p.filePath = "test.go"
	p.lines = []string{"line 1", "line 2"}

	assert.False(t, p.blameMode)

	// Toggle ON
	_, cmd := p.Update(keyMsg("B"))
	assert.True(t, p.blameMode)
	assert.NotNil(t, cmd, "should produce a ToggleBlameMsg command")

	// The cmd should produce a ToggleBlameMsg
	msg := cmd()
	toggleMsg, ok := msg.(panels.ToggleBlameMsg)
	assert.True(t, ok, "expected ToggleBlameMsg")
	assert.Equal(t, "test.go", toggleMsg.Path)
}

func TestBlameToggleOff(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.Focus()
	p.filePath = "test.go"
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "abc", Author: "A", LineNo: 1, Content: "x"},
	}

	// Toggle OFF
	_, cmd := p.Update(keyMsg("B"))
	assert.False(t, p.blameMode)
	assert.Nil(t, p.blameLines, "blame lines should be cleared on toggle off")
	assert.Nil(t, cmd, "no command needed when toggling off")
}

func TestBlameToggleIgnoredWithNoFile(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.Focus()
	// No file loaded

	_, cmd := p.Update(keyMsg("B"))
	assert.False(t, p.blameMode, "should not toggle without a file")
	assert.Nil(t, cmd)
}

func TestBlameToggleIgnoredWhenBlurred(t *testing.T) {
	p := New(defaultCfg(), nil)
	// Not focused
	p.filePath = "test.go"

	_, cmd := p.Update(keyMsg("B"))
	assert.False(t, p.blameMode, "should not toggle when blurred")
	assert.Nil(t, cmd)
}

// --- BlameLoadedMsg tests ---

func TestBlameLoadedMsgSuccess(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.blameMode = true

	blameData := []git.BlameLine{
		{Hash: "abc1234", Author: "Alice", Date: time.Now(), LineNo: 1, Content: "line 1"},
		{Hash: "def5678", Author: "Bob", Date: time.Now().Add(-90 * 24 * time.Hour), LineNo: 2, Content: "line 2"},
	}

	p.Update(panels.BlameLoadedMsg{Lines: blameData})
	assert.Equal(t, blameData, p.blameLines)
	assert.True(t, p.blameMode)
}

func TestBlameLoadedMsgError(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.blameMode = true

	p.Update(panels.BlameLoadedMsg{Err: fmt.Errorf("not a tracked file")})
	assert.False(t, p.blameMode, "blame mode should be disabled on error")
	assert.Nil(t, p.blameLines, "blame lines should be nil on error")
}

// --- Blame rendering tests ---

func TestBlameRendering(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.SetSize(80, 10)
	p.filePath = "test.go"
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "abc1234", Author: "Alice", Date: time.Now(), LineNo: 1, Content: "package main"},
		{Hash: "def5678", Author: "Bob", Date: time.Now(), LineNo: 2, Content: ""},
		{Hash: "abc1234", Author: "Alice", Date: time.Now(), LineNo: 3, Content: "func main() {}"},
	}
	p.lines = []string{"package main", "", "func main() {}"}

	view := p.View(80, 10)
	assert.Contains(t, view, "abc1234")
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "package main")
	assert.Contains(t, view, "def5678")
	assert.Contains(t, view, "Bob")
	assert.Contains(t, view, "func main()")
}

func TestBlameRenderingEmptyBlameLines(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.SetSize(80, 10)
	p.filePath = "test.go"
	p.blameMode = true
	p.blameLines = nil
	p.lines = []string{"line 1"}

	// With no blame lines, should fall through to normal rendering
	view := p.View(80, 10)
	assert.Contains(t, view, "line 1")
}

func TestBlameRenderingNarrowWidth(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.SetSize(40, 5)
	p.filePath = "test.go"
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "abc1234", Author: "Alice", Date: time.Now(), LineNo: 1, Content: "x"},
	}

	// Should not panic even with narrow width
	view := p.View(40, 5)
	assert.NotEmpty(t, view)
}

// --- Blame reset on file change ---

func TestBlameResetOnFileChange(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "abc", Author: "A", Date: time.Now(), LineNo: 1, Content: "x"},
	}

	p.Update(panels.FileSelectedMsg{Path: "new_file.go"})
	assert.False(t, p.blameMode, "blame mode should reset on file change")
	assert.Nil(t, p.blameLines, "blame lines should be cleared on file change")
}

// --- Content line count tests ---

func TestContentLineCountNormalMode(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.lines = []string{"a", "b", "c"}

	assert.Equal(t, 3, p.contentLineCount())
}

func TestContentLineCountBlameMode(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.lines = []string{"a", "b", "c"}
	p.blameMode = true
	p.blameLines = []git.BlameLine{
		{Hash: "1", Content: "a"},
		{Hash: "2", Content: "b"},
	}

	assert.Equal(t, 2, p.contentLineCount(), "should use blame line count in blame mode")
}

func TestContentLineCountBlameModeEmptyLines(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.lines = []string{"a", "b"}
	p.blameMode = true
	p.blameLines = nil // blame mode but no data yet

	assert.Equal(t, 2, p.contentLineCount(), "should fall back to normal lines when blame data empty")
}

// --- Blame scrolling tests ---

func TestBlameScrolling(t *testing.T) {
	p := New(defaultCfg(), nil)
	p.Focus()
	p.SetSize(80, 5) // viewport height = 4 (minus scroll indicator)
	p.filePath = "test.go"
	p.blameMode = true

	// 10 blame lines
	p.blameLines = make([]git.BlameLine, 10)
	for i := range p.blameLines {
		p.blameLines[i] = git.BlameLine{
			Hash:    fmt.Sprintf("hash%03d", i),
			Author:  "Author",
			Date:    time.Now(),
			LineNo:  i + 1,
			Content: fmt.Sprintf("line %d", i+1),
		}
	}

	// Scroll down
	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.scrollY)

	// Scroll to bottom
	p.Update(keyMsg("G"))
	assert.Equal(t, 6, p.scrollY) // 10 - 4 = 6

	// Scroll to top
	p.Update(keyMsg("g"))
	assert.Equal(t, 0, p.scrollY)
}

// --- Bisect message tests ---

func TestBisectStatusMsg(t *testing.T) {
	msg := panels.BisectStatusMsg{
		Active:         true,
		Current:        "abc1234",
		StepsRemaining: 5,
	}
	assert.True(t, msg.Active)
	assert.Equal(t, "abc1234", msg.Current)
	assert.Equal(t, 5, msg.StepsRemaining)
}

func TestBisectStatusMsgInactive(t *testing.T) {
	msg := panels.BisectStatusMsg{}
	assert.False(t, msg.Active)
	assert.Equal(t, "", msg.Current)
	assert.Equal(t, 0, msg.StepsRemaining)
}

// --- KeyBindings includes blame ---

func TestKeyBindingsIncludesBlame(t *testing.T) {
	p := New(defaultCfg(), nil)
	bindings := p.KeyBindings()

	found := false
	for _, b := range bindings {
		if b.Key == "B" && b.Action == "toggle_blame" {
			found = true
			break
		}
	}
	assert.True(t, found, "KeyBindings should include B for toggle_blame")
}
