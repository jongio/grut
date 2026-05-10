package preview

import (
	"image/color"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/panels"
)

// builderPool reuses strings.Builder instances to reduce per-line
// allocations in the selection highlight render loop.
var builderPool = sync.Pool{
	New: func() any { return new(strings.Builder) },
}

// selectionBgColor returns the background color used for text selection
// highlighting, falling back to the default when no theme is set.
func (p *Preview) selectionBgColor() color.Color {
	return panels.ColorOf(p.themeColors().SelectionBg, "#2A2A2A")
}

// applySelectionHighlight wraps the selected portion of a rendered line
// with a background-color ANSI style. absLine is the absolute line index
// in the display lines, and the selection range [sel, selE] is
// pre-normalized. The line has already been tab-expanded but may contain
// ANSI escape sequences from syntax highlighting.
//
// The cols in sel/selE are rune offsets in the stripped (ANSI-free) text.
// This function must map those offsets back to byte positions in the
// ANSI-containing string.
func (p *Preview) applySelectionHighlight(line string, absLine int, sel, selE *selPoint) string {
	if sel == nil || selE == nil {
		return line
	}
	if absLine < sel.Line || absLine > selE.Line {
		return line
	}
	stripped := ansi.Strip(line)
	runeCount := utf8.RuneCountInString(stripped)

	// Determine selection column range for this line.
	startCol := 0
	endCol := runeCount
	if absLine == sel.Line {
		startCol = sel.Col
	}
	if absLine == selE.Line {
		endCol = selE.Col
	}
	if startCol >= endCol || startCol >= runeCount {
		return line
	}
	if endCol > runeCount {
		endCol = runeCount
	}

	// Build the highlighted line by walking through the original string
	// character by character, tracking rune position while preserving
	// ANSI sequences.
	hlStyle := lipgloss.NewStyle().Background(p.selectionBgColor())
	before, _ := builderPool.Get().(*strings.Builder)
	if before == nil {
		before = new(strings.Builder)
	}
	before.Reset()
	defer builderPool.Put(before)
	sel2, _ := builderPool.Get().(*strings.Builder)
	if sel2 == nil {
		sel2 = new(strings.Builder)
	}
	sel2.Reset()
	defer builderPool.Put(sel2)
	after, _ := builderPool.Get().(*strings.Builder)
	if after == nil {
		after = new(strings.Builder)
	}
	after.Reset()
	defer builderPool.Put(after)
	runeIdx := 0
	i := 0

	for i < len(line) {
		// Check for ANSI escape sequence.
		if line[i] == '\x1b' {
			// Find end of the sequence.
			seqEnd := i + 1
			if seqEnd < len(line) && line[seqEnd] == '[' {
				seqEnd++
				for seqEnd < len(line) && !isCSITerminator(line[seqEnd]) {
					seqEnd++
				}
				if seqEnd < len(line) {
					seqEnd++ // include the terminator
				}
			} else if seqEnd < len(line) && line[seqEnd] == ']' {
				// OSC sequence — find BEL or ST (ESC \)
				for seqEnd < len(line) && line[seqEnd] != '\x07' {
					if line[seqEnd] == '\x1b' && seqEnd+1 < len(line) && line[seqEnd+1] == '\\' {
						seqEnd += 2
						break
					}
					seqEnd++
				}
				if seqEnd < len(line) && line[seqEnd] == '\x07' {
					seqEnd++
				}
			} else if seqEnd < len(line) {
				seqEnd++ // simple escape
			}
			seq := line[i:seqEnd]
			if runeIdx < startCol {
				before.WriteString(seq)
			} else if runeIdx < endCol {
				sel2.WriteString(seq)
			} else {
				after.WriteString(seq)
			}
			i = seqEnd
			continue
		}
		// Normal rune.
		r, size := utf8.DecodeRuneInString(line[i:])
		ch := string(r)
		if runeIdx < startCol {
			before.WriteString(ch)
		} else if runeIdx < endCol {
			sel2.WriteString(ch)
		} else {
			after.WriteString(ch)
		}
		runeIdx++
		i += size
	}

	if sel2.Len() == 0 {
		return line
	}
	return before.String() + hlStyle.Render(sel2.String()) + after.String()
}

func isCSITerminator(b byte) bool {
	return b >= 0x40 && b <= 0x7E
}
