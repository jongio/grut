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
	tests := []struct {
		input string
		want  []string
	}{
		{input: ""},
		{input: "foo", want: []string{"foo"}},
		{input: "foo bar", want: []string{"foo", " ", "bar"}},
		{input: "a(b)", want: []string{"a", "(", "b", ")"}},
		{input: "x_1.y2", want: []string{"x_1", ".", "y2"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens, overCap := tokenizeWord(tt.input)
			assert.False(t, overCap)
			assert.Equal(t, tt.want, tokens)
		})
	}
}

func TestTokenizeWordStopsAtTokenLimit(t *testing.T) {
	tokens, overCap := tokenizeWord(strings.Repeat(".", maxWordTokens))
	assert.False(t, overCap)
	assert.Len(t, tokens, maxWordTokens)

	tokens, overCap = tokenizeWord(strings.Repeat(".", maxWordTokens+1))
	assert.True(t, overCap)
	assert.Nil(t, tokens)
}

func TestTokenizeWordPathologicalLines(t *testing.T) {
	t.Run("one_mib_over_cap", func(t *testing.T) {
		tokens, overCap := tokenizeWord(strings.Repeat(".", 1<<20))
		assert.True(t, overCap)
		assert.Nil(t, tokens)
	})

	t.Run("one_mib_single_token", func(t *testing.T) {
		line := strings.Repeat("a", 1<<20)
		tokens, overCap := tokenizeWord(line)
		assert.False(t, overCap)
		assert.Equal(t, []string{line}, tokens)
	})
}

func TestDiffWordsIdentical(t *testing.T) {
	oldSegs, newSegs := diffWords("same line here", "same line here")
	// No tokens changed; the reconstructed text is preserved on both sides.
	assert.Equal(t, "same line here", segFull(oldSegs))
	assert.Equal(t, "same line here", segFull(newSegs))
	assert.Empty(t, segText(oldSegs, true))
	assert.Empty(t, segText(newSegs, true))
}

func TestDiffWordsSingleWordChange(t *testing.T) {
	oldSegs, newSegs := diffWords("alpha beta gamma", "alpha delta gamma")
	// Full text is preserved on each side.
	assert.Equal(t, "alpha beta gamma", segFull(oldSegs))
	assert.Equal(t, "alpha delta gamma", segFull(newSegs))
	// Only the differing word is flagged; the shared words are not.
	assert.Equal(t, "beta", segText(oldSegs, true))
	assert.Equal(t, "delta", segText(newSegs, true))
	assert.Contains(t, segText(oldSegs, false), "alpha")
	assert.Contains(t, segText(oldSegs, false), "gamma")
}

func TestDiffWordsPrefixAndSuffix(t *testing.T) {
	oldSegs, newSegs := diffWords("func foo(a int)", "func foo(a, b int)")
	assert.Equal(t, "func foo(a int)", segFull(oldSegs))
	assert.Equal(t, "func foo(a, b int)", segFull(newSegs))
	// The addition is only flagged on the new side.
	assert.Empty(t, segText(oldSegs, true))
	assert.NotEmpty(t, segText(newSegs, true))
}

func TestDiffWordsFullReplacement(t *testing.T) {
	oldSegs, newSegs := diffWords("aaa", "bbb")
	assert.Equal(t, "aaa", segText(oldSegs, true))
	assert.Equal(t, "bbb", segText(newSegs, true))
}

func TestDiffWordsPreservesDeleteFirstTie(t *testing.T) {
	oldSegs, newSegs := diffWords("...", "?..")
	assert.Equal(t, []wordSeg{
		{Text: ".", Changed: true},
		{Text: "..", Changed: false},
	}, oldSegs)
	assert.Equal(t, []wordSeg{
		{Text: "?", Changed: true},
		{Text: "..", Changed: false},
	}, newSegs)
}

func legacyAppendSeg(segs []wordSeg, text string, changed bool) []wordSeg {
	if n := len(segs); n > 0 && segs[n-1].Changed == changed {
		segs[n-1].Text += text
		return segs
	}
	return append(segs, wordSeg{Text: text, Changed: changed})
}

func legacyDiffWords(oldLine, newLine string) (oldSegs, newSegs []wordSeg) {
	a, _ := tokenizeWord(oldLine)
	b, _ := tokenizeWord(newLine)
	if len(a) == 0 || len(b) == 0 {
		return nil, nil
	}

	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			oldSegs = legacyAppendSeg(oldSegs, a[i], false)
			newSegs = legacyAppendSeg(newSegs, b[j], false)
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			oldSegs = legacyAppendSeg(oldSegs, a[i], true)
			i++
		default:
			newSegs = legacyAppendSeg(newSegs, b[j], true)
			j++
		}
	}
	for ; i < len(a); i++ {
		oldSegs = legacyAppendSeg(oldSegs, a[i], true)
	}
	for ; j < len(b); j++ {
		newSegs = legacyAppendSeg(newSegs, b[j], true)
	}
	return oldSegs, newSegs
}

func punctuationTokenSequences(maxLength int) []string {
	sequences := []string{""}
	level := []string{""}
	for range maxLength {
		next := make([]string, 0, len(level)*2)
		for _, prefix := range level {
			next = append(next, prefix+".", prefix+"?")
		}
		sequences = append(sequences, next...)
		level = next
	}
	return sequences
}

func equalWordSegs(a, b []wordSeg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiffWordsMatchesLegacyOutput(t *testing.T) {
	sequences := punctuationTokenSequences(6)
	for _, oldLine := range sequences {
		for _, newLine := range sequences {
			wantOld, wantNew := legacyDiffWords(oldLine, newLine)
			gotOld, gotNew := diffWords(oldLine, newLine)
			if !equalWordSegs(wantOld, gotOld) || !equalWordSegs(wantNew, gotNew) {
				t.Fatalf(
					"diffWords(%q, %q) = (%#v, %#v), want (%#v, %#v)",
					oldLine, newLine, gotOld, gotNew, wantOld, wantNew,
				)
			}
		}
	}
}

func TestDiffWordsEmptyReturnsNil(t *testing.T) {
	oldSegs, newSegs := diffWords("", "added")
	assert.Nil(t, oldSegs)
	assert.Nil(t, newSegs)

	oldSegs, newSegs = diffWords("removed", "")
	assert.Nil(t, oldSegs)
	assert.Nil(t, newSegs)
}

func TestDiffWordsAtTokenLimits(t *testing.T) {
	tests := []struct {
		name       string
		tokenCount int
	}{
		{name: "50_tokens", tokenCount: 50},
		{name: "200_tokens", tokenCount: 200},
		{name: "400_tokens", tokenCount: maxWordTokens},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldLine := strings.Repeat(".", tt.tokenCount-1) + "a"
			newLine := strings.Repeat(".", tt.tokenCount-1) + "b"
			oldSegs, newSegs := diffWords(oldLine, newLine)

			assert.Equal(t, oldLine, segFull(oldSegs))
			assert.Equal(t, newLine, segFull(newSegs))
			assert.Equal(t, "a", segText(oldSegs, true))
			assert.Equal(t, "b", segText(newSegs, true))
		})
	}
}

func TestDiffWordsOverCapFallsBack(t *testing.T) {
	long := strings.Repeat(".", maxWordTokens+1)
	oldSegs, newSegs := diffWords(long, long)
	assert.Nil(t, oldSegs, "over-cap lines should fall back to plain rendering")
	assert.Nil(t, newSegs)
}

func TestDiffWordsPathologicalLines(t *testing.T) {
	t.Run("one_mib_over_cap", func(t *testing.T) {
		line := strings.Repeat(".", 1<<20)
		oldSegs, newSegs := diffWords(line, line)
		assert.Nil(t, oldSegs)
		assert.Nil(t, newSegs)
	})

	t.Run("one_mib_single_token", func(t *testing.T) {
		oldLine := strings.Repeat("a", 1<<20)
		newLine := strings.Repeat("b", 1<<20)
		oldSegs, newSegs := diffWords(oldLine, newLine)
		assert.Equal(t, []wordSeg{{Text: oldLine, Changed: true}}, oldSegs)
		assert.Equal(t, []wordSeg{{Text: newLine, Changed: true}}, newSegs)
	})
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

var (
	benchmarkOldWordSegs []wordSeg
	benchmarkNewWordSegs []wordSeg
)

func BenchmarkDiffWordsBoundedAllocations(b *testing.B) {
	tests := []struct {
		name    string
		oldLine string
		newLine string
	}{
		{
			name:    "50_tokens",
			oldLine: strings.Repeat(".", 50),
			newLine: strings.Repeat("?", 50),
		},
		{
			name:    "200_tokens",
			oldLine: strings.Repeat(".", 200),
			newLine: strings.Repeat("?", 200),
		},
		{
			name:    "400_tokens",
			oldLine: strings.Repeat(".", maxWordTokens),
			newLine: strings.Repeat("?", maxWordTokens),
		},
		{
			name:    "over_cap_tokens",
			oldLine: strings.Repeat(".", maxWordTokens+1),
			newLine: strings.Repeat("?", maxWordTokens+1),
		},
		{
			name:    "one_mib_over_cap",
			oldLine: strings.Repeat(".", 1<<20),
			newLine: strings.Repeat("?", 1<<20),
		},
		{
			name:    "one_mib_single_token",
			oldLine: strings.Repeat("a", 1<<20),
			newLine: strings.Repeat("b", 1<<20),
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkOldWordSegs, benchmarkNewWordSegs = diffWords(tt.oldLine, tt.newLine)
			}
		})
	}
}
