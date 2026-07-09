// Package diag provides a lightweight, always-on runtime watchdog that samples
// process resource metrics (goroutine count, heap usage) on an interval and
// records a diagnostic when a threshold is crossed. It exists to catch runaway
// resource growth — leaked goroutines or unbounded memory — early, and to leave
// a durable, actionable artifact (including a full goroutine stack dump) even
// when structured logging via GRUT_LOG is not enabled.
//
// The watchdog is cheap: one runtime.ReadMemStats and runtime.NumGoroutine per
// interval (default 60s), so it is safe to run for the entire session.
package diag

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// Defaults for the watchdog thresholds and cadence. They are deliberately
// conservative so the watchdog only fires on genuinely abnormal growth.
const (
	defaultInterval        = 60 * time.Second
	defaultGoroutineFloor  = 800             // absolute count that always warns
	defaultGoroutineGrowth = 6.0             // warn at growth * startup baseline
	defaultGoroutineMin    = 250             // growth alert requires at least this many
	defaultHeapInuseBytes  = 1 << 30         // 1 GiB resident heap
	defaultCooldown        = 5 * time.Minute // per-signal minimum spacing
)

// Thresholds configures when the watchdog reports.
type Thresholds struct {
	// GoroutineFloor is an absolute goroutine count that always triggers an
	// alert regardless of the startup baseline.
	GoroutineFloor int
	// GoroutineGrowth triggers an alert when the live goroutine count exceeds
	// GoroutineGrowth * baseline (and is at least GoroutineGrowthMin).
	GoroutineGrowth float64
	// GoroutineGrowthMin guards the growth check so a tiny baseline cannot
	// produce alerts at harmless goroutine counts.
	GoroutineGrowthMin int
	// HeapInuseBytes triggers an alert when heap-in-use exceeds this many bytes.
	HeapInuseBytes uint64
	// Cooldown is the minimum time between alerts of the same kind.
	Cooldown time.Duration
}

// DefaultThresholds returns the built-in threshold configuration.
func DefaultThresholds() Thresholds {
	return Thresholds{
		GoroutineFloor:     defaultGoroutineFloor,
		GoroutineGrowth:    defaultGoroutineGrowth,
		GoroutineGrowthMin: defaultGoroutineMin,
		HeapInuseBytes:     defaultHeapInuseBytes,
		Cooldown:           defaultCooldown,
	}
}

// sample is a single point-in-time reading of runtime resource usage.
type sample struct {
	goroutines int
	heapInuse  uint64
	numGC      uint32
}

// Alert kinds.
const (
	kindGoroutines = "goroutines"
	kindHeap       = "heap"
)

// Alert describes a threshold breach detected by the watchdog.
type Alert struct {
	Timestamp      time.Time
	Kind           string // kindGoroutines or kindHeap
	Message        string
	Goroutines     int
	Baseline       int
	HeapInuseBytes uint64
	NumGC          uint32
}

// Watchdog samples runtime metrics and reports threshold breaches.
type Watchdog struct {
	now       func() time.Time
	sampleFn  func() sample
	reportFn  func(Alert)
	lastAlert map[string]time.Time
	cfg       Thresholds
	interval  time.Duration
	baseline  int
	mu        sync.Mutex
}

// New creates a Watchdog with default cadence, thresholds, and reporter. The
// default reporter logs via slog and appends a durable diagnostic record (with
// a goroutine stack dump for goroutine alerts) under the app data directory.
func New() *Watchdog {
	return &Watchdog{
		interval:  defaultInterval,
		cfg:       DefaultThresholds(),
		now:       time.Now,
		sampleFn:  readSample,
		reportFn:  reportAlert,
		lastAlert: make(map[string]time.Time),
	}
}

// readSample reads the current runtime metrics.
func readSample() sample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return sample{
		goroutines: runtime.NumGoroutine(),
		heapInuse:  ms.HeapInuse,
		numGC:      ms.NumGC,
	}
}

// Run samples on the configured interval until ctx is cancelled. It takes an
// immediate baseline sample so the growth check has a reference point, then
// reports any breaches through the configured reporter.
func (w *Watchdog) Run(ctx context.Context) {
	// Establish the baseline right away rather than waiting a full interval.
	w.mu.Lock()
	if w.baseline == 0 {
		w.baseline = w.sampleFn().goroutines
	}
	w.mu.Unlock()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, a := range w.checkOnce() {
				w.reportFn(a)
			}
		}
	}
}

// checkOnce takes one sample and returns any alerts that fire, honoring the
// per-kind cooldown. It is separated from Run so it can be unit-tested without
// real timers or goroutines.
func (w *Watchdog) checkOnce() []Alert {
	s := w.sampleFn()
	now := w.now()

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.baseline == 0 {
		w.baseline = s.goroutines
	}

	var alerts []Alert

	if w.goroutineBreached(s) && w.allowAlertLocked(kindGoroutines, now) {
		alerts = append(alerts, Alert{
			Timestamp:      now,
			Kind:           kindGoroutines,
			Message:        fmt.Sprintf("goroutine count %d exceeds threshold (baseline %d)", s.goroutines, w.baseline),
			Goroutines:     s.goroutines,
			Baseline:       w.baseline,
			HeapInuseBytes: s.heapInuse,
			NumGC:          s.numGC,
		})
	}

	if s.heapInuse >= w.cfg.HeapInuseBytes && w.allowAlertLocked(kindHeap, now) {
		alerts = append(alerts, Alert{
			Timestamp:      now,
			Kind:           kindHeap,
			Message:        fmt.Sprintf("heap in use %.0f MiB exceeds threshold %.0f MiB", float64(s.heapInuse)/(1<<20), float64(w.cfg.HeapInuseBytes)/(1<<20)),
			Goroutines:     s.goroutines,
			Baseline:       w.baseline,
			HeapInuseBytes: s.heapInuse,
			NumGC:          s.numGC,
		})
	}

	return alerts
}

// goroutineBreached reports whether the goroutine count crosses either the
// absolute floor or the growth-relative-to-baseline threshold.
func (w *Watchdog) goroutineBreached(s sample) bool {
	if s.goroutines >= w.cfg.GoroutineFloor {
		return true
	}
	if w.baseline > 0 && w.cfg.GoroutineGrowth > 0 {
		growthLimit := int(float64(w.baseline) * w.cfg.GoroutineGrowth)
		if s.goroutines >= growthLimit && s.goroutines >= w.cfg.GoroutineGrowthMin {
			return true
		}
	}
	return false
}

// allowAlertLocked reports whether an alert of the given kind may fire now,
// recording the time when it may. Caller must hold w.mu.
func (w *Watchdog) allowAlertLocked(kind string, now time.Time) bool {
	if last, ok := w.lastAlert[kind]; ok && now.Sub(last) < w.cfg.Cooldown {
		return false
	}
	w.lastAlert[kind] = now
	return true
}

// reportAlert is the default reporter: it logs the breach via slog and writes a
// durable diagnostic record. For goroutine breaches it also captures a full
// goroutine stack dump so the leaking call site can be identified.
func reportAlert(a Alert) {
	slog.Warn(
		"resource watchdog: "+a.Message,
		"kind", a.Kind,
		"goroutines", a.Goroutines,
		"baseline", a.Baseline,
		"heap_inuse_mb", fmt.Sprintf("%.1f", float64(a.HeapInuseBytes)/(1<<20)),
		"gc_cycles", a.NumGC,
	)

	record := formatRecord(a)
	if a.Kind == kindGoroutines {
		record += "\n\n" + goroutineDump()
	}
	writeDiag(record)
}

// goroutineDump returns a snapshot of all goroutine stacks, capped in size.
func goroutineDump() string {
	const maxDump = 1 << 20 // 1 MiB
	buf := make([]byte, maxDump)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// formatRecord renders a human-readable header for a diagnostic record.
func formatRecord(a Alert) string {
	return fmt.Sprintf(
		"[%s] watchdog %s alert: %s (goroutines=%d baseline=%d heap_inuse_mb=%.1f gc=%d)",
		a.Timestamp.UTC().Format(time.RFC3339),
		a.Kind,
		a.Message,
		a.Goroutines,
		a.Baseline,
		float64(a.HeapInuseBytes)/(1<<20),
		a.NumGC,
	)
}
