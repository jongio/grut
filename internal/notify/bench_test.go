package notify_test

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/notify"
)

const benchmarkInlineCap = 50

var benchmarkNotifyCmd tea.Cmd

// BenchmarkNotifyToastChurn measures the steady-state lifecycle of adding
// a toast and processing its expiry while reusing manager slice capacity.
func BenchmarkNotifyToastChurn(b *testing.B) {
	mgr := notify.NewManager()
	benchmarkNotifyCmd = mgr.AddToast("warmup", notify.Info)
	mgr.Update(notify.ToastExpiredMsg{ID: 0})
	nextID := int64(1)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkNotifyCmd = mgr.AddToast("test message", notify.Info)
		mgr.Update(notify.ToastExpiredMsg{ID: nextID})
		nextID++
	}
	b.StopTimer()
	if count := mgr.ToastCount(); count != 0 {
		b.Fatalf("toast count is %d after lifecycle, want 0", count)
	}
}

// BenchmarkNotifyToastLifecycleCold measures manager construction plus one
// complete toast lifecycle.
func BenchmarkNotifyToastLifecycleCold(b *testing.B) {
	var lastManager *notify.Manager
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		lastManager = notify.NewManager()
		benchmarkNotifyCmd = lastManager.AddToast("test message", notify.Info)
		lastManager.Update(notify.ToastExpiredMsg{ID: 0})
	}
	b.StopTimer()
	if count := lastManager.ToastCount(); count != 0 {
		b.Fatalf("toast count is %d after cold lifecycle, want 0", count)
	}
}

// BenchmarkNotifyInlineCap measures replacement pressure after the inline map
// has reached its cap, including oldest-entry eviction on every operation.
func BenchmarkNotifyInlineCap(b *testing.B) {
	mgr := notify.NewManager()
	for i := range benchmarkInlineCap {
		mgr.AddInline(fmt.Sprintf("prefill-%d", i), "inline notification", notify.Warn)
	}
	if count := mgr.InlineCount(); count != benchmarkInlineCap {
		b.Fatalf("inline prefill count is %d, want %d", count, benchmarkInlineCap)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		mgr.AddInline(fmt.Sprintf("notif-%d", i), "inline notification", notify.Warn)
	}
	b.StopTimer()

	if count := mgr.InlineCount(); count != benchmarkInlineCap {
		b.Fatalf("inline count is %d after churn, want %d", count, benchmarkInlineCap)
	}
}
