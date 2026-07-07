package gitdiff

import (
	"unicode"

	"github.com/jongio/grut/internal/git"
)

// maxWordTokens caps the token count per line for the intra-line diff. Lines
// longer than this fall back to plain line-level coloring so the O(n*m) LCS
// stays bounded on pathological single-line files (e.g. minified assets).
const maxWordTokens = 400

// wordSeg is a run of characters within a diff line, flagged as changed or
// unchanged relative to its paired line on the other side of the diff.
type wordSeg struct {
	Text    string
	Changed bool
}

// isWordRune reports whether r is part of an identifier-like token. Runs of
// these are grouped into a single token; every other rune is its own token so
// punctuation and whitespace changes are highlighted precisely.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// tokenizeWord splits s into tokens: maximal runs of word runes, plus one
// token per non-word rune.
func tokenizeWord(s string) []string {
	if s == "" {
		return nil
	}
	toks := make([]string, 0, len(s))
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			toks = append(toks, string(buf))
			buf = buf[:0]
		}
	}
	for _, r := range s {
		if isWordRune(r) {
			buf = append(buf, r)
			continue
		}
		flush()
		toks = append(toks, string(r))
	}
	flush()
	return toks
}

// appendSeg appends text to segs, coalescing with the previous segment when it
// carries the same Changed flag. This keeps the styled output compact.
func appendSeg(segs []wordSeg, text string, changed bool) []wordSeg {
	if n := len(segs); n > 0 && segs[n-1].Changed == changed {
		segs[n-1].Text += text
		return segs
	}
	return append(segs, wordSeg{Text: text, Changed: changed})
}

// diffWords computes intra-line change segments for a removed/added line pair
// using a token-level longest common subsequence. It returns the segments for
// the old line and the new line. When either line is empty, or a line exceeds
// maxWordTokens, it returns (nil, nil) so the caller renders plain lines.
func diffWords(oldLine, newLine string) (oldSegs, newSegs []wordSeg) {
	a := tokenizeWord(oldLine)
	b := tokenizeWord(newLine)
	if len(a) == 0 || len(b) == 0 {
		return nil, nil
	}
	if len(a) > maxWordTokens || len(b) > maxWordTokens {
		return nil, nil
	}

	n, m := len(a), len(b)
	// dp[i][j] = length of the LCS of a[i:] and b[j:].
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
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
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			oldSegs = appendSeg(oldSegs, a[i], false)
			newSegs = appendSeg(newSegs, b[j], false)
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			oldSegs = appendSeg(oldSegs, a[i], true)
			i++
		default:
			newSegs = appendSeg(newSegs, b[j], true)
			j++
		}
	}
	for ; i < n; i++ {
		oldSegs = appendSeg(oldSegs, a[i], true)
	}
	for ; j < m; j++ {
		newSegs = appendSeg(newSegs, b[j], true)
	}
	return oldSegs, newSegs
}

// computeHunkWordEmphasis pairs consecutive removed lines with consecutive
// added lines inside a hunk and computes their intra-line change segments. The
// result maps a line's index within lines to its segments. Only lines that are
// part of a removed/added pair get an entry; pure additions and pure deletions
// are left out so they render with plain line-level coloring.
func computeHunkWordEmphasis(lines []git.DiffLine) map[int][]wordSeg {
	out := make(map[int][]wordSeg)
	var removed, added []int
	flush := func() {
		pairs := len(removed)
		if len(added) < pairs {
			pairs = len(added)
		}
		for k := 0; k < pairs; k++ {
			oldSegs, newSegs := diffWords(lines[removed[k]].Content, lines[added[k]].Content)
			if len(oldSegs) == 0 || len(newSegs) == 0 {
				continue
			}
			out[removed[k]] = oldSegs
			out[added[k]] = newSegs
		}
		removed = removed[:0]
		added = added[:0]
	}
	for i := range lines {
		switch lines[i].Type {
		case git.DiffLineRemoved:
			removed = append(removed, i)
		case git.DiffLineAdded:
			added = append(added, i)
		default:
			flush()
		}
	}
	flush()
	if len(out) == 0 {
		return nil
	}
	return out
}
