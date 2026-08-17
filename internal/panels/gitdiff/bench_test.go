package gitdiff

import (
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/git"
)

// benchResultStr prevents the compiler from optimizing away string results.
var benchResultStr string

// benchResultLines prevents the compiler from optimizing away rendered lines.
var benchResultLines []string

// ---------------------------------------------------------------------------
// Test data generators
// ---------------------------------------------------------------------------

// genFileDiffs creates realistic FileDiff data with the specified number of
// total diff lines distributed across multiple hunks.
func genFileDiffs(tb testing.TB, totalLines int) []git.FileDiff {
	tb.Helper()
	if totalLines < 1 {
		tb.Fatalf("totalLines must be positive, got %d", totalLines)
	}

	filesCount := (totalLines + 49) / 50
	diffs := make([]git.FileDiff, filesCount)
	remainingLines := totalLines
	for f := range filesCount {
		filesRemaining := filesCount - f
		linesForFile := remainingLines / filesRemaining
		remainingLines -= linesForFile
		fd := git.FileDiff{
			Path:    fmt.Sprintf("src/pkg/module%d/handler.go", f),
			OldPath: fmt.Sprintf("src/pkg/module%d/handler.go", f),
		}

		hunksPerFile := (linesForFile + 19) / 20
		remainingFileLines := linesForFile
		oldLine := 1
		newLine := 1
		for h := range hunksPerFile {
			hunksRemaining := hunksPerFile - h
			linesForHunk := remainingFileLines / hunksRemaining
			remainingFileLines -= linesForHunk
			oldStart := oldLine
			newStart := newLine
			hunk := git.Hunk{
				OldStart: oldStart,
				NewStart: newStart,
			}

			for l := range linesForHunk {
				contentIndex := oldStart + l
				switch l % 5 {
				case 0:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineContext,
						Content: fmt.Sprintf("\tresult := processItem(items[%d])", contentIndex),
						OldLine: oldLine,
						NewLine: newLine,
					})
					oldLine++
					newLine++
				case 1:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineRemoved,
						Content: fmt.Sprintf("\tlog.Printf(\"processing %%d\", %d)", contentIndex),
						OldLine: oldLine,
					})
					oldLine++
				case 2:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineAdded,
						Content: fmt.Sprintf("\tslog.Info(\"processing\", \"index\", %d)", contentIndex),
						NewLine: newLine,
					})
					newLine++
				case 3:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineContext,
						Content: fmt.Sprintf("\tif err != nil { return fmt.Errorf(\"step %%d: %%w\", %d, err) }", contentIndex),
						OldLine: oldLine,
						NewLine: newLine,
					})
					oldLine++
					newLine++
				case 4:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineAdded,
						Content: fmt.Sprintf("\tmetrics.RecordLatency(ctx, time.Since(start%d))", contentIndex),
						NewLine: newLine,
					})
					newLine++
				}
			}
			hunk.OldLines = oldLine - oldStart
			hunk.NewLines = newLine - newStart
			hunk.Header = fmt.Sprintf(
				"@@ -%d,%d +%d,%d @@ func handler%d() {",
				oldStart,
				hunk.OldLines,
				newStart,
				hunk.NewLines,
				h,
			)
			fd.Hunks = append(fd.Hunks, hunk)
		}
		diffs[f] = fd
	}
	if got := countDiffLines(diffs); got != totalLines {
		tb.Fatalf("generated %d diff lines, want %d", got, totalLines)
	}
	return diffs
}

func countDiffLines(diffs []git.FileDiff) int {
	total := 0
	for _, fd := range diffs {
		for _, hunk := range fd.Hunks {
			total += len(hunk.Lines)
		}
	}
	return total
}

func TestGenFileDiffsCardinality(t *testing.T) {
	for _, totalLines := range []int{1, 10, 50, 100, 500, 1000} {
		t.Run(fmt.Sprintf("%d_lines", totalLines), func(t *testing.T) {
			diffs := genFileDiffs(t, totalLines)
			if got := countDiffLines(diffs); got != totalLines {
				t.Fatalf("countDiffLines() = %d, want %d", got, totalLines)
			}
		})
	}
}

// newBenchDiff creates a GitDiff instance for benchmarking with pre-set diffs.
func newBenchDiff(diffs []git.FileDiff, mode viewMode, width int) *GitDiff {
	d := New(nil, nil)
	d.diffs = diffs
	d.mode = mode
	d.Width = width
	d.Height = 40
	return d
}

// ---------------------------------------------------------------------------
// Benchmarks: Inline diff rendering
// ---------------------------------------------------------------------------

func BenchmarkInlineDiffRenderExact(b *testing.B) {
	b.Run("10_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 10)
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
			benchResultLines = d.lines
		}
	})

	b.Run("100_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 100)
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
			benchResultLines = d.lines
		}
	})

	b.Run("1000_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 1000)
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
			benchResultLines = d.lines
		}
	})
}

func BenchmarkInlineDiffRenderCold(b *testing.B) {
	diffs := genFileDiffs(b, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		benchResultLines = d.lines
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: Side-by-side diff rendering
// ---------------------------------------------------------------------------

func BenchmarkSideBySideDiffRenderExact(b *testing.B) {
	b.Run("10_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 10)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		d.buildSideBySideLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
			benchResultLines = d.lines
		}
	})

	b.Run("100_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 100)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		d.buildSideBySideLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
			benchResultLines = d.lines
		}
	})

	b.Run("1000_lines", func(b *testing.B) {
		diffs := genFileDiffs(b, 1000)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		d.buildSideBySideLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
			benchResultLines = d.lines
		}
	})
}

func BenchmarkSideBySideDiffRenderCold(b *testing.B) {
	diffs := genFileDiffs(b, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		d := newBenchDiff(diffs, viewSideBySide, 160)
		d.buildSideBySideLines()
		benchResultLines = d.lines
	}
}

// ---------------------------------------------------------------------------
// Benchmark: pairDiffLines
// ---------------------------------------------------------------------------

func BenchmarkPairDiffLines(b *testing.B) {
	b.Run("20_lines_mixed", func(b *testing.B) {
		lines := make([]git.DiffLine, 20)
		for i := range lines {
			switch i % 3 {
			case 0:
				lines[i] = git.DiffLine{Type: git.DiffLineContext, Content: "context", OldLine: i, NewLine: i}
			case 1:
				lines[i] = git.DiffLine{Type: git.DiffLineRemoved, Content: "old", OldLine: i}
			case 2:
				lines[i] = git.DiffLine{Type: git.DiffLineAdded, Content: "new", NewLine: i}
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = pairDiffLines(lines)
		}
	})

	b.Run("200_lines_mixed", func(b *testing.B) {
		lines := make([]git.DiffLine, 200)
		for i := range lines {
			switch i % 3 {
			case 0:
				lines[i] = git.DiffLine{Type: git.DiffLineContext, Content: "context line with some content", OldLine: i, NewLine: i}
			case 1:
				lines[i] = git.DiffLine{Type: git.DiffLineRemoved, Content: "removed line with old content", OldLine: i}
			case 2:
				lines[i] = git.DiffLine{Type: git.DiffLineAdded, Content: "added line with new content", NewLine: i}
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = pairDiffLines(lines)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: renderViewport
// ---------------------------------------------------------------------------

func BenchmarkRenderViewportWithStats(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		diffs := genFileDiffs(b, 50)
		d := newBenchDiff(diffs, viewInline, 120)
		d.rebuildLines()
		if d.stats.files != len(diffs) {
			b.Fatalf("diff stats report %d files, want %d", d.stats.files, len(diffs))
		}
		benchResultStr = d.renderViewport(120, 40)
		if len(d.viewportBuf) != 39 {
			b.Fatalf("viewport buffer has %d lines, want 39", len(d.viewportBuf))
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = d.renderViewport(120, 40)
		}
	})

	b.Run("large_scrolled", func(b *testing.B) {
		diffs := genFileDiffs(b, 500)
		d := newBenchDiff(diffs, viewInline, 120)
		d.rebuildLines()
		if d.stats.files != len(diffs) {
			b.Fatalf("diff stats report %d files, want %d", d.stats.files, len(diffs))
		}
		d.scrollY = len(d.lines) / 2 // scroll to middle
		benchResultStr = d.renderViewport(120, 40)
		if len(d.viewportBuf) != 39 {
			b.Fatalf("viewport buffer has %d lines, want 39", len(d.viewportBuf))
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = d.renderViewport(120, 40)
		}
	})
}
