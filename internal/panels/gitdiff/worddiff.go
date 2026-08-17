package gitdiff

import (
	"unicode"
	"unicode/utf8"

	"github.com/jongio/grut/internal/git"
)

// maxWordTokens caps the token count per line for the intra-line diff. Lines
// longer than this fall back to plain line-level coloring so tokenization and
// LCS work stay bounded on pathological single-line files.
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
// token per non-word rune. It stops as soon as it detects token 401.
func tokenizeWord(s string) (tokens []string, overCap bool) {
	if s == "" {
		return nil, false
	}

	tokens = make([]string, 0, 16)
	wordStart := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if isWordRune(r) {
			if wordStart < 0 {
				wordStart = i
			}
			i += size
			continue
		}

		if wordStart >= 0 {
			if len(tokens) == maxWordTokens {
				return nil, true
			}
			tokens = append(tokens, s[wordStart:i])
			wordStart = -1
		}
		if len(tokens) == maxWordTokens {
			return nil, true
		}
		tokens = append(tokens, s[i:i+size])
		i += size
	}
	if wordStart >= 0 {
		if len(tokens) == maxWordTokens {
			return nil, true
		}
		tokens = append(tokens, s[wordStart:])
	}
	return tokens, false
}

// wordSegBuilder coalesces adjacent tokens by tracking byte spans in the
// original line. Segment creation therefore does not copy or concatenate text.
type wordSegBuilder struct {
	line    string
	segs    []wordSeg
	start   int
	end     int
	changed bool
	active  bool
}

func (b *wordSegBuilder) appendToken(token string, changed bool) {
	if !b.active {
		b.start = b.end
		b.changed = changed
		b.active = true
	} else if b.changed != changed {
		b.flush()
		b.start = b.end
		b.changed = changed
		b.active = true
	}
	b.end += len(token)
}

func (b *wordSegBuilder) flush() {
	if !b.active {
		return
	}
	b.segs = append(b.segs, wordSeg{
		Text:    b.line[b.start:b.end],
		Changed: b.changed,
	})
	b.active = false
}

func (b *wordSegBuilder) finish() []wordSeg {
	b.flush()
	return b.segs
}

type lcsCrossing struct {
	token   uint16
	matched bool
}

type lcsWorkspace struct {
	scores    [2][maxWordTokens + 1]uint16
	crossings [2][maxWordTokens + 1]lcsCrossing
}

// findLCSCrossing finds the exact delete or match edge where the current
// delete-first LCS path crosses the middle old-token row. It propagates that
// edge while computing suffix scores with two rows of workspace.
func findLCSCrossing(a, b []string, workspace *lcsWorkspace) lcsCrossing {
	split := len(a) / 2
	m := len(b)
	next := workspace.scores[0][:m+1]
	current := workspace.scores[1][:m+1]
	clear(next)

	for i := len(a) - 1; i >= split; i-- {
		current[m] = 0
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				current[j] = next[j+1] + 1
			} else if next[j] >= current[j+1] {
				current[j] = next[j]
			} else {
				current[j] = current[j+1]
			}
		}
		next, current = current, next
	}

	nextCrossing := workspace.crossings[0][:m+1]
	currentCrossing := workspace.crossings[1][:m+1]
	for i := split - 1; i >= 0; i-- {
		current[m] = 0
		if i == split-1 {
			currentCrossing[m] = lcsCrossing{token: uint16(m)}
		} else {
			currentCrossing[m] = nextCrossing[m]
		}

		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				current[j] = next[j+1] + 1
				if i == split-1 {
					currentCrossing[j] = lcsCrossing{token: uint16(j), matched: true}
				} else {
					currentCrossing[j] = nextCrossing[j+1]
				}
			case next[j] >= current[j+1]:
				current[j] = next[j]
				if i == split-1 {
					currentCrossing[j] = lcsCrossing{token: uint16(j)}
				} else {
					currentCrossing[j] = nextCrossing[j]
				}
			default:
				current[j] = current[j+1]
				currentCrossing[j] = currentCrossing[j+1]
			}
		}
		next, current = current, next
		nextCrossing, currentCrossing = currentCrossing, nextCrossing
	}
	return nextCrossing[0]
}

// appendWordDiff reconstructs the deterministic delete-first LCS path into
// old and new segment builders without retaining the full score matrix.
func appendWordDiff(
	a, b []string,
	oldBuilder, newBuilder *wordSegBuilder,
	workspace *lcsWorkspace,
) {
	if len(a) == 0 {
		for _, token := range b {
			newBuilder.appendToken(token, true)
		}
		return
	}
	if len(b) == 0 {
		for _, token := range a {
			oldBuilder.appendToken(token, true)
		}
		return
	}
	if len(a) == 1 {
		match := -1
		for j := range b {
			if a[0] == b[j] {
				match = j
				break
			}
		}
		if match < 0 {
			oldBuilder.appendToken(a[0], true)
			for _, token := range b {
				newBuilder.appendToken(token, true)
			}
			return
		}
		for _, token := range b[:match] {
			newBuilder.appendToken(token, true)
		}
		oldBuilder.appendToken(a[0], false)
		newBuilder.appendToken(b[match], false)
		for _, token := range b[match+1:] {
			newBuilder.appendToken(token, true)
		}
		return
	}

	split := len(a) / 2
	oldIndex := split - 1
	crossing := findLCSCrossing(a, b, workspace)
	newIndex := int(crossing.token)

	appendWordDiff(a[:oldIndex], b[:newIndex], oldBuilder, newBuilder, workspace)
	if crossing.matched {
		oldBuilder.appendToken(a[oldIndex], false)
		newBuilder.appendToken(b[newIndex], false)
		appendWordDiff(a[split:], b[newIndex+1:], oldBuilder, newBuilder, workspace)
		return
	}

	oldBuilder.appendToken(a[oldIndex], true)
	appendWordDiff(a[split:], b[newIndex:], oldBuilder, newBuilder, workspace)
}

// diffWords computes intra-line change segments for a removed/added line pair
// using a token-level longest common subsequence. It returns the segments for
// the old line and the new line. When either line is empty, or a line exceeds
// maxWordTokens, it returns (nil, nil) so the caller renders plain lines.
func diffWords(oldLine, newLine string) (oldSegs, newSegs []wordSeg) {
	a, overCap := tokenizeWord(oldLine)
	if overCap {
		return nil, nil
	}
	b, overCap := tokenizeWord(newLine)
	if overCap {
		return nil, nil
	}
	if len(a) == 0 || len(b) == 0 {
		return nil, nil
	}

	oldBuilder := wordSegBuilder{line: oldLine, segs: make([]wordSeg, 0, 8)}
	newBuilder := wordSegBuilder{line: newLine, segs: make([]wordSeg, 0, 8)}
	var workspace lcsWorkspace
	appendWordDiff(a, b, &oldBuilder, &newBuilder, &workspace)
	return oldBuilder.finish(), newBuilder.finish()
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
