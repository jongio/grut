package diag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestWatchdog builds a Watchdog with injectable sampling and clock and a
// no-op reporter, suitable for deterministic unit tests.
func newTestWatchdog(cfg Thresholds, samples ...sample) *Watchdog {
	i := 0
	return &Watchdog{
		interval: time.Millisecond,
		cfg:      cfg,
		now:      func() time.Time { return time.Unix(0, 0) },
		sampleFn: func() sample {
			s := samples[i]
			if i < len(samples)-1 {
				i++
			}
			return s
		},
		reportFn:  func(Alert) {},
		lastAlert: make(map[string]time.Time),
	}
}

func TestCheckOnce_GoroutineFloorAlert(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(cfg, sample{goroutines: cfg.GoroutineFloor + 1})
	alerts := w.checkOnce()
	if len(alerts) != 1 || alerts[0].Kind != "goroutines" {
		t.Fatalf("expected one goroutine alert, got %+v", alerts)
	}
}

func TestCheckOnce_GoroutineGrowthAlert(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(
		cfg,
		sample{goroutines: 50}, // baseline
		sample{goroutines: 400},
	)
	// First sample establishes baseline 50; below floor and growth → no alert.
	if got := w.checkOnce(); len(got) != 0 {
		t.Fatalf("expected no alert on baseline sample, got %+v", got)
	}
	// 400 > 6*50=300 and >= GoroutineGrowthMin(250) → growth alert.
	alerts := w.checkOnce()
	if len(alerts) != 1 || alerts[0].Kind != "goroutines" {
		t.Fatalf("expected goroutine growth alert, got %+v", alerts)
	}
}

func TestCheckOnce_GrowthBelowMinDoesNotAlert(t *testing.T) {
	cfg := DefaultThresholds()
	// baseline 20 → 6x = 120, but 120 < GoroutineGrowthMin(250) so no alert
	// despite exceeding the growth multiple.
	w := newTestWatchdog(
		cfg,
		sample{goroutines: 20},
		sample{goroutines: 130},
	)
	_ = w.checkOnce()
	if got := w.checkOnce(); len(got) != 0 {
		t.Fatalf("expected no alert when below growth minimum, got %+v", got)
	}
}

func TestCheckOnce_HeapAlert(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(cfg, sample{goroutines: 10, heapInuse: cfg.HeapInuseBytes})
	alerts := w.checkOnce()
	if len(alerts) != 1 || alerts[0].Kind != "heap" {
		t.Fatalf("expected one heap alert, got %+v", alerts)
	}
}

func TestCheckOnce_NoAlertBelowThresholds(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(cfg, sample{goroutines: 40, heapInuse: 10 << 20})
	if got := w.checkOnce(); len(got) != 0 {
		t.Fatalf("expected no alerts, got %+v", got)
	}
}

func TestCheckOnce_CooldownSuppressesRepeat(t *testing.T) {
	cfg := DefaultThresholds()
	now := time.Unix(1000, 0)
	w := newTestWatchdog(cfg, sample{goroutines: cfg.GoroutineFloor + 5})
	w.now = func() time.Time { return now }

	if got := w.checkOnce(); len(got) != 1 {
		t.Fatalf("expected first alert to fire, got %+v", got)
	}
	// Within cooldown: suppressed.
	now = now.Add(cfg.Cooldown - time.Second)
	if got := w.checkOnce(); len(got) != 0 {
		t.Fatalf("expected alert suppressed within cooldown, got %+v", got)
	}
	// After cooldown: fires again.
	now = now.Add(2 * time.Second)
	if got := w.checkOnce(); len(got) != 1 {
		t.Fatalf("expected alert to fire again after cooldown, got %+v", got)
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	cfg := DefaultThresholds()
	fired := make(chan Alert, 4)
	w := &Watchdog{
		interval: time.Millisecond,
		cfg:      cfg,
		now:      time.Now,
		sampleFn: func() sample { return sample{goroutines: cfg.GoroutineFloor + 1} },
		reportFn: func(a Alert) {
			select {
			case fired <- a:
			default:
			}
		},
		lastAlert: make(map[string]time.Time),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("watchdog did not report within timeout")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not stop after context cancel")
	}
}

func TestWriteDiag_AppendsAndRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchdog.log")
	orig := diagLogPath
	diagLogPath = func() string { return path }
	t.Cleanup(func() { diagLogPath = orig })

	// Write enough to exceed the rotation cap, then write once more to trigger
	// rotation into watchdog.log.1.
	big := strings.Repeat("x", maxDiagLogBytes)
	writeDiag(big)
	writeDiag("after-rotation")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected current log to exist: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated log to exist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	if !strings.Contains(string(data), "after-rotation") {
		t.Fatal("current log should contain the post-rotation record")
	}
}

func TestReportAlert_WritesGoroutineDump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchdog.log")
	orig := diagLogPath
	diagLogPath = func() string { return path }
	t.Cleanup(func() { diagLogPath = orig })

	reportAlert(Alert{
		Timestamp:  time.Unix(0, 0),
		Kind:       "goroutines",
		Message:    "test breach",
		Goroutines: 1234,
		Baseline:   50,
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "goroutines alert") {
		t.Fatalf("record missing header: %q", content)
	}
	if !strings.Contains(content, "goroutine ") {
		t.Fatal("goroutine alert record should include a stack dump")
	}
}
