package diag

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	w := newTestWatchdog(cfg, sample{goroutines: 10, heapLive: cfg.HeapCeilingBytes, numGC: 1})
	alerts := w.checkOnce()
	if len(alerts) != 1 || alerts[0].Kind != "heap" {
		t.Fatalf("expected one heap alert, got %+v", alerts)
	}
}

func TestCheckOnce_HeapSlopeAlert(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.HeapCeilingBytes = 2 << 30
	cfg.HeapGrowthWindow = 10 * time.Minute
	cfg.HeapGrowthMinSamples = 4
	cfg.HeapGrowthMinBytes = 60 << 20
	cfg.HeapGrowthBytesPerMinute = 20 << 20
	now := time.Unix(1000, 0)
	w := newTestWatchdog(
		cfg,
		sample{goroutines: 10, heapLive: 100 << 20, numGC: 1},
		sample{goroutines: 10, heapLive: 140 << 20, numGC: 2},
		sample{goroutines: 10, heapLive: 180 << 20, numGC: 3},
		sample{goroutines: 10, heapLive: 220 << 20, numGC: 4},
	)
	w.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if got := w.checkOnce(); len(got) != 0 {
			t.Fatalf("expected no alert before sustained window fills, got %+v", got)
		}
		now = now.Add(time.Minute)
	}
	alerts := w.checkOnce()
	if len(alerts) != 1 || alerts[0].Kind != kindHeap {
		t.Fatalf("expected sustained heap slope alert, got %+v", alerts)
	}
	if alerts[0].HeapGrowthRate < float64(cfg.HeapGrowthBytesPerMinute) {
		t.Fatalf("growth rate %.0f is below configured threshold %d", alerts[0].HeapGrowthRate, cfg.HeapGrowthBytesPerMinute)
	}
}

func TestCheckOnce_HeapSlopeRequiresSustainedGrowth(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.HeapCeilingBytes = 2 << 30
	cfg.HeapGrowthWindow = 10 * time.Minute
	cfg.HeapGrowthMinSamples = 4
	cfg.HeapGrowthMinBytes = 10 << 20
	cfg.HeapGrowthBytesPerMinute = 1
	now := time.Unix(1000, 0)
	w := newTestWatchdog(
		cfg,
		sample{goroutines: 10, heapLive: 100 << 20, numGC: 1},
		sample{goroutines: 10, heapLive: 180 << 20, numGC: 2},
		sample{goroutines: 10, heapLive: 140 << 20, numGC: 3},
		sample{goroutines: 10, heapLive: 220 << 20, numGC: 4},
	)
	w.now = func() time.Time { return now }

	for i := 0; i < 4; i++ {
		if got := w.checkOnce(); len(got) != 0 {
			t.Fatalf("expected no alert for non-sustained growth, got %+v", got)
		}
		now = now.Add(time.Minute)
	}
}

func TestCheckOnce_HeapSlopeRequiresDistinctGCCycles(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.HeapCeilingBytes = 2 << 30
	cfg.HeapGrowthWindow = 10 * time.Minute
	cfg.HeapGrowthMinSamples = 3
	cfg.HeapGrowthMinBytes = 1
	cfg.HeapGrowthBytesPerMinute = 1
	now := time.Unix(1000, 0)
	w := newTestWatchdog(
		cfg,
		sample{goroutines: 10, heapLive: 100 << 20, numGC: 1},
		sample{goroutines: 10, heapLive: 180 << 20, numGC: 1},
		sample{goroutines: 10, heapLive: 260 << 20, numGC: 1},
	)
	w.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if got := w.checkOnce(); len(got) != 0 {
			t.Fatalf("expected no alert without distinct GC cycles, got %+v", got)
		}
		now = now.Add(time.Minute)
	}
}

func TestCheckOnce_NoAlertBelowThresholds(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(cfg, sample{goroutines: 40, heapLive: 10 << 20, numGC: 1})
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

func TestStart_StopIsIdempotentAndWaits(t *testing.T) {
	cfg := DefaultThresholds()
	w := newTestWatchdog(cfg, sample{goroutines: 10})
	stop := w.Start(context.Background())

	done := make(chan struct{})
	go func() {
		stop()
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog stop did not wait for shutdown")
	}
}

func TestCheckOnce_ConcurrentSafe(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.HeapCeilingBytes = 0
	cfg.HeapGrowthMinSamples = 0
	var gc atomic.Uint64
	w := newTestWatchdog(cfg, sample{goroutines: 10})
	w.sampleFn = func() sample {
		return sample{
			goroutines: 10,
			heapLive:   10 << 20,
			numGC:      gc.Add(1),
		}
	}
	w.now = time.Now

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = w.checkOnce()
			}
		}()
	}
	wg.Wait()
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

func TestReportAlert_WritesHeapProfileReference(t *testing.T) {
	dir := setupHeapProfileTest(t)
	writeHeapProfile = func(w io.Writer) error {
		_, err := w.Write([]byte("profile"))
		return err
	}

	reportAlert(Alert{
		Timestamp:     time.Unix(1000, 0),
		Kind:          kindHeap,
		Message:       "test retained growth",
		Goroutines:    10,
		Baseline:      5,
		HeapLiveBytes: 256 << 20,
		NumGC:         4,
	})

	data, err := os.ReadFile(filepath.Join(dir, "watchdog.log"))
	if err != nil {
		t.Fatalf("read watchdog log: %v", err)
	}
	if !strings.Contains(string(data), "heap_profile=") {
		t.Fatalf("heap alert did not reference captured profile: %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read diagnostics dir: %v", err)
	}
	profiles := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), heapProfilePrefix) {
			profiles++
		}
	}
	if profiles != 1 {
		t.Fatalf("expected one heap profile, got %d", profiles)
	}
}

func TestCaptureHeapProfile_RateLimited(t *testing.T) {
	dir := setupHeapProfileTest(t)
	var writes atomic.Int32
	writeHeapProfile = func(w io.Writer) error {
		writes.Add(1)
		_, err := w.Write([]byte("profile"))
		return err
	}

	at := time.Unix(1000, 0)
	if _, captured, err := captureHeapProfile(at); err != nil || !captured {
		t.Fatalf("first capture failed: captured=%v err=%v", captured, err)
	}
	if _, captured, err := captureHeapProfile(at.Add(heapProfileMinInterval - time.Second)); err != nil || captured {
		t.Fatalf("second capture should be rate-limited: captured=%v err=%v", captured, err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("expected one profile write, got %d", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read profile dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one retained profile, got %d", len(entries))
	}
}

func TestCaptureHeapProfile_BoundedRetention(t *testing.T) {
	dir := setupHeapProfileTest(t)
	writeHeapProfile = func(w io.Writer) error {
		_, err := w.Write([]byte("profile"))
		return err
	}

	at := time.Unix(1000, 0)
	for i := 0; i < maxHeapProfiles+2; i++ {
		if _, captured, err := captureHeapProfile(at); err != nil || !captured {
			t.Fatalf("capture %d failed: captured=%v err=%v", i, captured, err)
		}
		at = at.Add(heapProfileMinInterval + time.Second)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read profile dir: %v", err)
	}
	if len(entries) != maxHeapProfiles {
		t.Fatalf("expected %d retained profiles, got %d", maxHeapProfiles, len(entries))
	}
}

func TestCaptureHeapProfile_WritesBinaryProfile(t *testing.T) {
	dir := setupHeapProfileTest(t)

	path, captured, err := captureHeapProfile(time.Unix(1000, 0))
	if err != nil || !captured {
		t.Fatalf("capture failed: captured=%v err=%v", captured, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read heap profile: %v", err)
	}
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("expected gzip-compressed binary profile in %s", filepath.Base(path))
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one binary profile: entries=%d err=%v", len(entries), err)
	}
}

func setupHeapProfileTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	originalDir := diagnosticsDirPath
	originalWriter := writeHeapProfile
	diagnosticsDirPath = func() string { return dir }
	heapProfileState.Lock()
	originalLast := heapProfileState.last
	heapProfileState.last = time.Time{}
	heapProfileState.Unlock()
	t.Cleanup(func() {
		diagnosticsDirPath = originalDir
		writeHeapProfile = originalWriter
		heapProfileState.Lock()
		heapProfileState.last = originalLast
		heapProfileState.Unlock()
	})
	return dir
}
