package git

import (
	"fmt"
	"strings"
	"testing"
)

// benchResultStatus prevents the compiler from optimizing away parse results.
var benchResultStatus []FileStatus

// benchResultDiff prevents the compiler from optimizing away diff results.
var benchResultDiff []FileDiff

// benchResultBranch prevents the compiler from optimizing away branch results.
var benchResultBranch StatusBranch

// benchResultBool prevents the compiler from optimizing away boolean results.
var benchResultBool bool

// benchResultStr prevents the compiler from optimizing away string-slice results.
var benchResultPaths []string

// benchResultStr prevents the compiler from optimizing away string results.
var benchResultStr string

// ---------------------------------------------------------------------------
// Status output generators
// ---------------------------------------------------------------------------

// genStatusV2 builds realistic porcelain v2 output with n ordinary changed files.
func genStatusV2(n int) string {
	var b strings.Builder
	b.WriteString("# branch.oid abc1234567890def\n")
	b.WriteString("# branch.head main\n")
	b.WriteString("# branch.upstream origin/main\n")
	b.WriteString("# branch.ab +3 -1\n")

	for i := range n {
		dir := fmt.Sprintf("src/pkg%d", i/10)
		file := fmt.Sprintf("file%d.go", i)
		switch i % 5 {
		case 0:
			// Modified in worktree only.
			fmt.Fprintf(&b, "1 .M N... 100644 100644 100644 %032x %032x %s/%s\n",
				i, i+1, dir, file)
		case 1:
			// Staged addition.
			fmt.Fprintf(&b, "1 A. N... 000000 100644 100644 %032x %032x %s/%s\n",
				0, i, dir, file)
		case 2:
			// Modified and staged.
			fmt.Fprintf(&b, "1 MM N... 100644 100644 100644 %032x %032x %s/%s\n",
				i, i+1, dir, file)
		case 3:
			// Deleted in worktree.
			fmt.Fprintf(&b, "1 .D N... 100644 100644 000000 %032x %032x %s/%s\n",
				i, 0, dir, file)
		case 4:
			// Untracked.
			fmt.Fprintf(&b, "? %s/%s\n", dir, file)
		}
	}
	return b.String()
}

// genStatusV2Renamed builds output with renamed entries.
func genStatusV2Renamed(n int) string {
	var b strings.Builder
	b.WriteString("# branch.oid abc1234567890def\n")
	b.WriteString("# branch.head feature/rename\n")

	for i := range n {
		fmt.Fprintf(&b, "2 R. N... 100644 100644 100644 %032x %032x R100 src/new%d.go\tsrc/old%d.go\n",
			i, i+1, i, i)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Diff output generators
// ---------------------------------------------------------------------------

// genDiffOutput builds realistic unified diff output with the given number
// of lines distributed across hunks.
func genDiffOutput(totalLines int) string {
	var b strings.Builder
	filesCount := 1 + totalLines/50
	linesPerFile := totalLines / filesCount

	for f := range filesCount {
		fmt.Fprintf(&b, "diff --git a/src/file%d.go b/src/file%d.go\n", f, f)
		fmt.Fprintf(&b, "index abc1234..def5678 100644\n")
		fmt.Fprintf(&b, "--- a/src/file%d.go\n", f)
		fmt.Fprintf(&b, "+++ b/src/file%d.go\n", f)

		hunksPerFile := 1 + linesPerFile/20
		linesPerHunk := linesPerFile / hunksPerFile

		for h := range hunksPerFile {
			startLine := h*linesPerHunk + 1
			fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@ func example%d() {\n",
				startLine, linesPerHunk, startLine, linesPerHunk+2, h)

			for l := range linesPerHunk {
				lineNum := startLine + l
				switch l % 5 {
				case 0:
					fmt.Fprintf(&b, " \tcontext line %d: unchanged code here\n", lineNum)
				case 1:
					fmt.Fprintf(&b, "-\told line %d: removed content\n", lineNum)
				case 2:
					fmt.Fprintf(&b, "+\tnew line %d: added replacement content\n", lineNum)
				case 3:
					fmt.Fprintf(&b, " \tcontext line %d: more unchanged code\n", lineNum)
				case 4:
					fmt.Fprintf(&b, "+\tadded line %d: new feature code\n", lineNum)
				}
			}
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Benchmarks: parseStatusV2
// ---------------------------------------------------------------------------

func BenchmarkParseStatus(b *testing.B) {
	b.Run("10_files", func(b *testing.B) {
		output := genStatusV2(10)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStatus, _ = parseStatusV2(output)
		}
	})

	b.Run("100_files", func(b *testing.B) {
		output := genStatusV2(100)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStatus, _ = parseStatusV2(output)
		}
	})

	b.Run("500_files", func(b *testing.B) {
		output := genStatusV2(500)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStatus, _ = parseStatusV2(output)
		}
	})

	b.Run("100_renamed", func(b *testing.B) {
		output := genStatusV2Renamed(100)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStatus, _ = parseStatusV2(output)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: ParseStatusBranch
// ---------------------------------------------------------------------------

func BenchmarkParseStatusBranch(b *testing.B) {
	output := genStatusV2(50) // includes branch header lines
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchResultBranch = ParseStatusBranch(output)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: parseDiff
// ---------------------------------------------------------------------------

func BenchmarkParseDiff(b *testing.B) {
	b.Run("10_lines", func(b *testing.B) {
		output := genDiffOutput(10)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultDiff, _ = parseDiff(output)
		}
	})

	b.Run("100_lines", func(b *testing.B) {
		output := genDiffOutput(100)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultDiff, _ = parseDiff(output)
		}
	})

	b.Run("1000_lines", func(b *testing.B) {
		output := genDiffOutput(1000)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultDiff, _ = parseDiff(output)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: parseHunkHeader
// ---------------------------------------------------------------------------

func BenchmarkParseHunkHeader(b *testing.B) {
	headers := []struct {
		name   string
		header string
	}{
		{"simple", "@@ -1,5 +1,7 @@"},
		{"with_context", "@@ -42,10 +45,12 @@ func ProcessData() {"},
		{"large_range", "@@ -1000,200 +1050,250 @@ func LargeFunction() {"},
	}

	for _, h := range headers {
		b.Run(h.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _, _, _ = parseHunkHeader(h.header)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: UndoManager stack operations
// ---------------------------------------------------------------------------

func BenchmarkUndoStack(b *testing.B) {
	b.Run("record_50", func(b *testing.B) {
		// Measures full lifecycle: construction + 50 record operations.
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			mgr := NewUndoManager(nil)
			for i := range 50 {
				mgr.RecordAction(UndoAction{
					Type:      "commit",
					RefBefore: fmt.Sprintf("abc%04d", i),
					Metadata:  map[string]string{"message": fmt.Sprintf("commit %d", i)},
				})
			}
		}
	})

	b.Run("record_overflow", func(b *testing.B) {
		// Measures full lifecycle with overflow: construction + 200 records (exceeds maxUndoDepth).
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			mgr := NewUndoManager(nil)
			for i := range 200 {
				mgr.RecordAction(UndoAction{
					Type:      "stage",
					RefBefore: fmt.Sprintf("ref%04d", i),
					Metadata:  map[string]string{"paths": fmt.Sprintf("file%d.go", i)},
				})
			}
		}
	})

	b.Run("peek_undo", func(b *testing.B) {
		mgr := NewUndoManager(nil)
		for i := range 50 {
			mgr.RecordAction(UndoAction{
				Type:      "commit",
				RefBefore: fmt.Sprintf("abc%04d", i),
				Metadata:  map[string]string{"message": fmt.Sprintf("commit %d", i)},
			})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_, benchResultBool = mgr.PeekUndo()
		}
	})

	b.Run("can_undo_redo", func(b *testing.B) {
		mgr := NewUndoManager(nil)
		for i := range 25 {
			mgr.RecordAction(UndoAction{
				Type:     "stage",
				Metadata: map[string]string{"paths": fmt.Sprintf("f%d.go", i)},
			})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultBool = mgr.CanUndo()
			benchResultBool = mgr.CanRedo()
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: JoinPaths / splitPaths
// ---------------------------------------------------------------------------

func BenchmarkJoinPaths(b *testing.B) {
	paths := make([]string, 100)
	for i := range paths {
		paths[i] = fmt.Sprintf("src/pkg/module%d/file%d.go", i/10, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var result string
	for range b.N {
		result = JoinPaths(paths)
	}
	benchResultStr = result
}

func BenchmarkSplitPaths(b *testing.B) {
	paths := make([]string, 100)
	for i := range paths {
		paths[i] = fmt.Sprintf("src/pkg/module%d/file%d.go", i/10, i)
	}
	joined := JoinPaths(paths)
	b.ReportAllocs()
	b.ResetTimer()
	var result []string
	for range b.N {
		result = splitPaths(joined)
	}
	benchResultPaths = result
}
