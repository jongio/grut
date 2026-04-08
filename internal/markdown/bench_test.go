package markdown_test

import (
	"runtime"
	"testing"

	"github.com/jongio/grut/internal/markdown"
)

// BenchmarkRenderStaticCached measures the allocation benefit of the
// renderer cache introduced to reduce per-call glamour.NewTermRenderer
// overhead.
func BenchmarkRenderStaticCached(b *testing.B) {
	source := "# Heading\n\nSome **bold** and `code` text.\n\n- item 1\n- item 2\n"

	b.Run("same_width", func(b *testing.B) {
		// Warmup to populate cache.
		markdown.RenderStatic(source, 80)

		runtime.GC()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			markdown.RenderStatic(source, 80)
		}
	})

	b.Run("varying_width", func(b *testing.B) {
		widths := []int{60, 80, 100, 120}
		// Warmup all widths.
		for _, w := range widths {
			markdown.RenderStatic(source, w)
		}

		runtime.GC()
		b.ReportAllocs()
		b.ResetTimer()
		for i := range b.N {
			markdown.RenderStatic(source, widths[i%len(widths)])
		}
	})
}

// BenchmarkRenderStaticMemoryGrowth tracks heap-inuse delta across
// repeated renders to detect retained allocations or leaks, matching
// the pattern used by internal/panels/gitdiff/bench_test.go.
func BenchmarkRenderStaticMemoryGrowth(b *testing.B) {
	source := "## Section\n\nParagraph with **bold**, *italic*, and `code`.\n\n```go\nfmt.Println(\"hello\")\n```\n"

	// Warmup to reach steady state.
	for range 5 {
		markdown.RenderStatic(source, 80)
	}

	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	b.ResetTimer()
	for range b.N {
		markdown.RenderStatic(source, 80)
	}
	b.StopTimer()

	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	heapDelta := int64(memAfter.HeapInuse) - int64(memBefore.HeapInuse)
	gcCycles := memAfter.NumGC - memBefore.NumGC

	b.ReportMetric(float64(heapDelta)/float64(b.N), "heap-inuse-b/op")
	b.ReportMetric(float64(gcCycles)/float64(b.N), "gc-cycles/op")
}
