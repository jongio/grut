package gitdiff

import (
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/git"
)

// benchResultStr prevents the compiler from optimizing away string results.
var benchResultStr string

// ---------------------------------------------------------------------------
// Test data generators
// ---------------------------------------------------------------------------

// genFileDiffs creates realistic FileDiff data with the specified number of
// total diff lines distributed across multiple hunks.
func genFileDiffs(totalLines int) []git.FileDiff {
	filesCount := 1 + totalLines/50
	linesPerFile := totalLines / filesCount
	if linesPerFile < 1 {
		linesPerFile = 1
	}

	diffs := make([]git.FileDiff, filesCount)
	for f := range filesCount {
		fd := git.FileDiff{
			Path:    fmt.Sprintf("src/pkg/module%d/handler.go", f),
			OldPath: fmt.Sprintf("src/pkg/module%d/handler.go", f),
		}

		hunksPerFile := 1 + linesPerFile/20
		linesPerHunk := linesPerFile / hunksPerFile
		if linesPerHunk < 1 {
			linesPerHunk = 1
		}

		for h := range hunksPerFile {
			startLine := h*linesPerHunk + 1
			hunk := git.Hunk{
				Header:   fmt.Sprintf("@@ -%d,%d +%d,%d @@ func handler%d() {", startLine, linesPerHunk, startLine, linesPerHunk+2, h),
				OldStart: startLine,
				OldLines: linesPerHunk,
				NewStart: startLine,
				NewLines: linesPerHunk + 2,
			}

			oldLine := startLine
			newLine := startLine
			for l := range linesPerHunk {
				switch l % 5 {
				case 0:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineContext,
						Content: fmt.Sprintf("\tresult := processItem(items[%d])", startLine+l),
						OldLine: oldLine,
						NewLine: newLine,
					})
					oldLine++
					newLine++
				case 1:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineRemoved,
						Content: fmt.Sprintf("\tlog.Printf(\"processing %%d\", %d)", startLine+l),
						OldLine: oldLine,
					})
					oldLine++
				case 2:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineAdded,
						Content: fmt.Sprintf("\tslog.Info(\"processing\", \"index\", %d)", startLine+l),
						NewLine: newLine,
					})
					newLine++
				case 3:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineContext,
						Content: fmt.Sprintf("\tif err != nil { return fmt.Errorf(\"step %%d: %%w\", %d, err) }", startLine+l),
						OldLine: oldLine,
						NewLine: newLine,
					})
					oldLine++
					newLine++
				case 4:
					hunk.Lines = append(hunk.Lines, git.DiffLine{
						Type:    git.DiffLineAdded,
						Content: fmt.Sprintf("\tmetrics.RecordLatency(ctx, time.Since(start%d))", startLine+l),
						NewLine: newLine,
					})
					newLine++
				}
			}
			fd.Hunks = append(fd.Hunks, hunk)
		}
		diffs[f] = fd
	}
	return diffs
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

func BenchmarkInlineDiffRender(b *testing.B) {
	b.Run("10_lines", func(b *testing.B) {
		diffs := genFileDiffs(10)
		d := newBenchDiff(diffs, viewInline, 120)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
		}
	})

	b.Run("100_lines", func(b *testing.B) {
		diffs := genFileDiffs(100)
		d := newBenchDiff(diffs, viewInline, 120)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
		}
	})

	b.Run("1000_lines", func(b *testing.B) {
		diffs := genFileDiffs(1000)
		d := newBenchDiff(diffs, viewInline, 120)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildInlineLines()
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmarks: Side-by-side diff rendering
// ---------------------------------------------------------------------------

func BenchmarkSideBySideDiffRender(b *testing.B) {
	b.Run("10_lines", func(b *testing.B) {
		diffs := genFileDiffs(10)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
		}
	})

	b.Run("100_lines", func(b *testing.B) {
		diffs := genFileDiffs(100)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
		}
	})

	b.Run("1000_lines", func(b *testing.B) {
		diffs := genFileDiffs(1000)
		d := newBenchDiff(diffs, viewSideBySide, 160)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			d.buildSideBySideLines()
		}
	})
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

func BenchmarkRenderViewport(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		diffs := genFileDiffs(50)
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = d.renderViewport(120, 40)
		}
	})

	b.Run("large_scrolled", func(b *testing.B) {
		diffs := genFileDiffs(500)
		d := newBenchDiff(diffs, viewInline, 120)
		d.buildInlineLines()
		d.scrollY = len(d.lines) / 2 // scroll to middle
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = d.renderViewport(120, 40)
		}
	})
}
