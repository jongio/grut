package notify_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/jongio/grut/internal/notify"
)

// BenchmarkNotifyToastChurn measures allocation overhead of creating
// many short-lived toasts (the common hot path in the TUI).
func BenchmarkNotifyToastChurn(b *testing.B) {
	mgr := notify.NewManager()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cmd := mgr.AddToast("test message", notify.Info)
		_ = cmd
	}
}

// BenchmarkNotifyInlineCap verifies that the inline notification cap
// prevents unbounded map growth when many distinct IDs are added.
func BenchmarkNotifyInlineCap(b *testing.B) {
	mgr := notify.NewManager()

	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		mgr.AddInline(fmt.Sprintf("notif-%d", i), "inline notification", notify.Warn)
	}
	b.StopTimer()

	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	heapDelta := int64(memAfter.HeapInuse) - int64(memBefore.HeapInuse)
	b.ReportMetric(float64(heapDelta)/float64(b.N), "heap-inuse-b/op")

	// After many inserts the map should be capped at maxInlineNotifications (50).
	if count := mgr.InlineCount(); count > 50 {
		b.Errorf("inline count %d exceeds cap of 50", count)
	}
}
