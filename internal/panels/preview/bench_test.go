package preview

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
)

var previewBenchmarkCmd tea.Cmd

func BenchmarkPreviewUpdateIssueBody(b *testing.B) {
	for _, sizeKB := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("%d_KB", sizeKB), func(b *testing.B) {
			p := New(defaultCfg(), defaultEditorCfg(), nil)
			p.SetSize(100, 40)
			msg := panels.IssueSelectedMsg{
				Number: 42,
				Title:  "Benchmark issue",
				Body:   strings.Repeat("markdown body content\n", sizeKB*1024/22+1)[:sizeKB*1024],
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, previewBenchmarkCmd = p.Update(msg)
			}
		})
	}
}
