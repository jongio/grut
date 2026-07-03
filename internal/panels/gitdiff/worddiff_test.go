package gitdiff

import (
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

// segText joins the text of the changed (or unchanged) segments so tests can
// assert which parts of a line were flagged.
func segText(segs []wordSeg, changed bool) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Changed == changed {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func segFull(segs []wordSeg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestTokenizeWord(t *testing.T) {
	assert.Nil(t, tokenizeWord(""))
	assert.Equal(t, []string{"foo"}, tokenizeWord("foo"))
	assert.Equal(t, []string{"foo", " ", "bar"}, tokenizeWord("foo bar"))
	assert.Equal(t, []string{"a", "(", "b", ")"}, tokenizeWord("a(b)"))
	assert.Equal(t, []string{"x_1", ".", "y2"}, tokenizeWord("x_1.y2"))
}

func TestDiffWordsIdentical(t *testing.T) {
	old, new := diffWords("same line here", "same line here")
	// No tokens changed; the reconstructed text is preserved on both sides.
	assert.Equal(t, "same line here", segFull(old))
	assert.Equal(t, "same line here", segFull(new))
	assert.Empty(t, segText(old, true))
	assert.Empty(t, segText(new, true))
}

func TestDiffWordsSingleWordChange(t *testing.T) {
	old, new := diffWords("alpha beta gamma", "alpha delta gamma")
	// Full text is preserved on each side.
	assert.Equal(t, "alpha beta gamma", segFull(old))
	assert.Equal(t, "alpha delta gamma", segFull(new))
	// Only the differing word is flagged; the shared words are not.
	assert.Equal(t, "beta", segText(old, true))
	assert.Equal(t, "delta", segText(new, true))
	assert.Contains(t, segText(old, false), "alpha")
	assert.Contains(t, segText(old, false), "gamma")
}

func TestDiffWordsPrefixAndSuffix(t *testing.T) {
	old, new := diffWords("func foo(a int)", "func foo(a, b int)")
	assert.Equal(t, "func foo(a int)", segFull(old))
	assert.Equal(t, "func foo(a, b int)", segFull(new))
	// The addition is only flagged on the new side.
	assert.Empty(t, segText(old, true))
	assert.NotEmpty(t, segText(new, true))
}

func TestDiffWordsFullReplacement(t *testing.T) {
	old, new := diffWords("aaa", "bbb")
	assert.Equal(t, "aaa", segText(old, true))
	assert.Equal(t, "bbb", segText(new, true))
}

func TestDiffWordsEmptyReturnsNil(t *testing.T) {
	old, new := diffWords("", "added")
	assert.Nil(t, old)
	assert.Nil(t, new)

	old, new = diffWords("removed", "")
	assert.Nil(t, old)
	assert.Nil(t, new)
}

func TestDiffWordsOverCapFallsBack(t *testing.T) {
	long := strings.Repeat("a ", maxWordTokens+10) // > maxWordTokens tokens
	old, new := diffWords(long, long+"x")
	assert.Nil(t, old, "over-cap lines should fall back to plain rendering")
	assert.Nil(t, new)
}

func TestComputeHunkWordEmphasisPairsChanges(t *testing.T) {
	lines := []git.DiffLine{
		{Type: git.DiffLineContext, Content: "ctx"},
		{Type: git.DiffLineRemoved, Content: "alpha beta gamma"},
		{Type: git.DiffLineAdded, Content: "alpha delta gamma"},
		{Type: git.DiffLineContext, Content: "ctx2"},
	}
	emph := computeHunkWordEmphasis(lines)
	// Indices 1 (removed) and 2 (added) form a pair and get segments.
	assert.Contains(t, emph, 1)
	assert.Contains(t, emph, 2)
	assert.Equal(t, "beta", segText(emph[1], true))
	assert.Equal(t, "delta", segText(emph[2], true))
}

func TestComputeHunkWordEmphasisSkipsUnpaired(t *testing.T) {
	// Pure additions with no preceding removed lines get no emphasis.
	lines := []git.DiffLine{
		{Type: git.DiffLineContext, Content: "ctx"},
		{Type: git.DiffLineAdded, Content: "brand new line"},
		{Type: git.DiffLineAdded, Content: "another new line"},
	}
	assert.Nil(t, computeHunkWordEmphasis(lines))
}

// wordDiffSampleDiff returns a diff whose change block swaps one word so the
// intra-line highlighter has something to emphasize.
func wordDiffSampleDiff() git.FileDiff {
	return git.FileDiff{
		Path: "file.go",
		Hunks: []git.Hunk{
			{
				Header: "@@ -1,3 +1,3 @@",
				Lines: []git.DiffLine{
					{Type: git.DiffLineContext, Content: "ctx", OldLine: 1, NewLine: 1},
					{Type: git.DiffLineRemoved, Content: "alpha beta gamma", OldLine: 2, NewLine: 0},
					{Type: git.DiffLineAdded, Content: "alpha delta gamma", OldLine: 0, NewLine: 2},
				},
			},
		},
	}
}

// findLine returns the first rendered line whose ANSI-stripped text contains
// the given substring.
func findLine(lines []string, substr string) (string, bool) {
	for _, l := range lines {
		if strings.Contains(panels.StripANSI(l), substr) {
			return l, true
		}
	}
	return "", false
}

func TestWordHighlightOnPreservesTextAndAddsStyling(t *testing.T) {
	th := loadTestTheme(t)

	on := newTestPanel(th)
	on.SetWordHighlight(true)
	on.SetSize(80, 24)
	on.SetDiffs([]git.FileDiff{wordDiffSampleDiff()})

	off := newTestPanel(th)
	off.SetSize(80, 24)
	off.SetDiffs([]git.FileDiff{wordDiffSampleDiff()})

	onAdded, ok := findLine(on.lines, "+ alpha delta gamma")
	assert.True(t, ok, "added line should still render with its + prefix and full text")
	offAdded, ok := findLine(off.lines, "+ alpha delta gamma")
	assert.True(t, ok)

	// Visible text is identical with and without highlighting.
	assert.Equal(t, panels.StripANSI(offAdded), panels.StripANSI(onAdded))
	// But the styled output differs because the changed word is emphasized.
	assert.NotEqual(t, offAdded, onAdded, "word highlight should change the ANSI styling")
}

func TestWordHighlightDefaultOff(t *testing.T) {
	// A freshly constructed panel keeps highlighting off so line-level tests
	// and callers that never opt in are unaffected.
	p := New(nil, nil)
	assert.False(t, p.wordHighlight)
}

func TestSetWordHighlightRebuilds(t *testing.T) {
	th := loadTestTheme(t)
	p := newTestPanel(th)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{wordDiffSampleDiff()})
	before, _ := findLine(p.lines, "+ alpha delta gamma")

	p.SetWordHighlight(true)
	after, ok := findLine(p.lines, "+ alpha delta gamma")
	assert.True(t, ok)
	assert.NotEqual(t, before, after, "enabling word highlight should rebuild rendered lines")
}

func TestToggleWordHighlightKey(t *testing.T) {
	th := loadTestTheme(t)
	p := newTestPanel(th)
	p.SetSize(80, 24)
	p.SetDiffs([]git.FileDiff{wordDiffSampleDiff()})
	assert.False(t, p.wordHighlight)

	_, _ = p.handleKey(keyMsg("w"))
	assert.True(t, p.wordHighlight, "pressing w should enable word highlight")

	_, _ = p.handleKey(keyMsg("w"))
	assert.False(t, p.wordHighlight, "pressing w again should disable it")
}
