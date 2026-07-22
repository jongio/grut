package preview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	searchTestQuery = "alpha"
	searchTestWidth = 80
)

func TestComputeSearchMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []searchMatch
	}{
		{
			name:  "multiple matches on multiple lines",
			input: searchTestQuery,
			want: []searchMatch{
				{line: 0, startCol: 0, endCol: 5},
				{line: 0, startCol: 11, endCol: 16},
				{line: 1, startCol: 6, endCol: 11},
			},
		},
		{
			name:  "case insensitive",
			input: "ALPHA",
			want: []searchMatch{
				{line: 0, startCol: 0, endCol: 5},
				{line: 0, startCol: 11, endCol: 16},
				{line: 1, startCol: 6, endCol: 11},
			},
		},
		{name: "empty query", input: "", want: nil},
		{name: "no match", input: "zeta", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPreview([]string{"Alpha beta alpha", "gamma ALPHA", "delta"})
			p.searchInput = tt.input

			p.computeSearchMatches()

			assert.Equal(t, tt.want, p.searchMatches)
		})
	}
}

func TestSearchNavigationWrapsAndPreservesVisibleScroll(t *testing.T) {
	t.Parallel()

	p := newTestPreview(makeLines(40))
	p.lines[2] = searchTestQuery
	p.lines[30] = searchTestQuery
	p.searchInput = searchTestQuery
	p.computeSearchMatches()

	p.scrollY = 0
	p.nextMatch()
	assert.Equal(t, 1, p.searchIdx)
	assert.Equal(t, 17, p.scrollY, "off-screen target should be centered and clamped")

	p.nextMatch()
	assert.Equal(t, 0, p.searchIdx, "next should wrap to first match")
	assert.Equal(t, 0, p.scrollY)

	p.scrollY = 0
	p.prevMatch()
	assert.Equal(t, 1, p.searchIdx, "previous should wrap to last match")
	assert.Equal(t, 17, p.scrollY)

	p.scrollY = 0
	p.searchIdx = 0
	p.scrollToMatch()
	assert.Equal(t, 0, p.scrollY, "visible target should preserve scroll")
}

func TestClearSearchResetsState(t *testing.T) {
	t.Parallel()

	p := newTestPreview([]string{searchTestQuery})
	p.searchActive = true
	p.searchInput = searchTestQuery
	p.searchQuery = searchTestQuery
	p.searchMatches = []searchMatch{{line: 0, startCol: 0, endCol: 5}}
	p.searchIdx = 1

	p.clearSearch()

	assert.False(t, p.searchActive)
	assert.Empty(t, p.searchInput)
	assert.Empty(t, p.searchQuery)
	assert.Nil(t, p.searchMatches)
	assert.Zero(t, p.searchIdx)
}

func TestFileSelectedMsgClearsSearch(t *testing.T) {
	t.Parallel()

	p := newTestPreview([]string{searchTestQuery})
	p.searchInput = searchTestQuery
	p.computeSearchMatches()
	require.NotEmpty(t, p.searchMatches)

	p.Update(panels.FileSelectedMsg{Path: "new-file.go"})

	assert.False(t, p.searchActive)
	assert.Empty(t, p.searchInput)
	assert.Empty(t, p.searchQuery)
	assert.Nil(t, p.searchMatches)
	assert.Zero(t, p.searchIdx)
}

func TestApplySearchHighlight(t *testing.T) {
	t.Parallel()

	p := newTestPreview([]string{"Alpha beta"})
	p.searchInput = searchTestQuery
	p.computeSearchMatches()

	got := p.applySearchHighlight("Alpha beta", 0)

	assert.NotEqual(t, "Alpha beta", got)
	assert.Contains(t, ansi.Strip(got), "Alpha")
}

func TestSearchFooterCount(t *testing.T) {
	t.Parallel()

	p := newTestPreview([]string{"Alpha beta alpha"})
	p.filePath = "test.go"
	p.searchActive = true
	p.searchInput = searchTestQuery
	p.computeSearchMatches()

	out := p.View(searchTestWidth, 5)

	assert.Contains(t, ansi.Strip(out), "Search: alpha (1/2)")
}

func TestSearchPromptKeyFlow(t *testing.T) {
	t.Parallel()

	p := newTestPreview([]string{"alpha", "beta alpha", "ALPHA"})
	p.filePath = "test.go"

	_, cmd := p.Update(tea.KeyPressMsg{Text: "ctrl+f"})
	require.True(t, p.searchActive)
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputStartedMsg)
	assert.True(t, ok)

	for _, key := range []string{"a", "l", "p", "h", "a"} {
		p.Update(keyMsg(key))
	}
	assert.Equal(t, searchTestQuery, p.searchInput)
	require.Len(t, p.searchMatches, 3)

	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, p.searchActive)
	assert.Equal(t, searchTestQuery, p.searchQuery)
	require.Len(t, p.searchMatches, 3)
	require.NotNil(t, cmd)
	_, ok = cmd().(panels.PreviewInputEndedMsg)
	assert.True(t, ok)

	p.Update(keyMsg("n"))
	assert.Equal(t, 1, p.searchIdx)
	p.Update(keyMsg("N"))
	assert.Equal(t, 0, p.searchIdx)

	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Empty(t, p.searchQuery)
	assert.Nil(t, p.searchMatches)
}
