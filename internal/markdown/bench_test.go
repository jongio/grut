package markdown

import "testing"

var benchmarkRenderLines []string

// BenchmarkRenderStaticCached measures the allocation benefit of the
// renderer cache introduced to reduce per-call glamour.NewTermRenderer
// overhead.
func BenchmarkRenderStaticCached(b *testing.B) {
	source := "# Heading\n\nSome **bold** and `code` text.\n\n- item 1\n- item 2\n"

	b.Run("same_width", func(b *testing.B) {
		// Warmup to populate cache.
		benchmarkRenderLines = RenderStatic(source, 80)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchmarkRenderLines = RenderStatic(source, 80)
		}
	})

	b.Run("varying_width", func(b *testing.B) {
		widths := []int{60, 80, 100, 120}
		// Warmup all widths.
		for _, w := range widths {
			benchmarkRenderLines = RenderStatic(source, w)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := range b.N {
			benchmarkRenderLines = RenderStatic(source, widths[i%len(widths)])
		}
	})
}

// BenchmarkRenderStaticCold measures full rendering when every width misses
// the renderer cache. Cycling one more width than the cache capacity ensures
// each requested renderer was evicted before that width is reused.
func BenchmarkRenderStaticCold(b *testing.B) {
	source := "## Section\n\nParagraph with **bold**, *italic*, and `code`.\n\n```go\nfmt.Println(\"hello\")\n```\n"
	widths := []int{60, 70, 80, 90, 100, 110}
	if len(widths) != maxCachedWidths+1 {
		b.Fatalf("cold width count is %d, want %d", len(widths), maxCachedWidths+1)
	}

	rendererMu.Lock()
	clear(rendererCache)
	rendererMu.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		benchmarkRenderLines = RenderStatic(source, widths[i%len(widths)])
	}
}
