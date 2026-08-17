// Package diag provides a lightweight, always-on runtime watchdog that samples
// process resource metrics (goroutine count and post-GC live heap) on an
// interval and records a diagnostic when a threshold is crossed. It exists to
// catch runaway resource growth early and leave a durable, actionable artifact
// even when structured logging via GRUT_LOG is not enabled.
//
// The watchdog is cheap: one runtime/metrics read and runtime.NumGoroutine per
// interval (default 60s), so it is safe to run for the entire session.
package diag

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/metrics"
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
	defaultHeapCeiling     = 1 << 30         // 1 GiB post-GC live heap
	defaultHeapWindow      = 5 * time.Minute // rolling post-GC observation window
	defaultHeapMinSamples  = 4               // distinct GC cycles required for a slope
	defaultHeapMinGrowth   = 128 << 20       // ignore small retained-heap increases
	defaultHeapGrowthRate  = 32 << 20        // 32 MiB retained growth per minute
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
	// HeapCeilingBytes triggers an alert when post-GC live heap reaches this
	// absolute ceiling.
	HeapCeilingBytes uint64
	// HeapGrowthWindow is the rolling interval used to calculate retained-heap
	// growth. Only samples from distinct GC cycles participate.
	HeapGrowthWindow time.Duration
	// HeapGrowthMinSamples is the number of strictly increasing post-GC samples
	// required before a growth alert can fire.
	HeapGrowthMinSamples int
	// HeapGrowthMinBytes ignores slopes whose total growth is smaller than this.
	HeapGrowthMinBytes uint64
	// HeapGrowthBytesPerMinute is the minimum sustained retained-heap slope.
	HeapGrowthBytesPerMinute uint64
	// Cooldown is the minimum time between alerts of the same kind.
	Cooldown time.Duration
}

// DefaultThresholds returns the built-in threshold configuration.
func DefaultThresholds() Thresholds {
	return Thresholds{
		GoroutineFloor:           defaultGoroutineFloor,
		GoroutineGrowth:          defaultGoroutineGrowth,
		GoroutineGrowthMin:       defaultGoroutineMin,
		HeapCeilingBytes:         defaultHeapCeiling,
		HeapGrowthWindow:         defaultHeapWindow,
		HeapGrowthMinSamples:     defaultHeapMinSamples,
		HeapGrowthMinBytes:       defaultHeapMinGrowth,
		HeapGrowthBytesPerMinute: defaultHeapGrowthRate,
		Cooldown:                 defaultCooldown,
	}
}

// sample is a single point-in-time reading of runtime resource usage.
type sample struct {
	goroutines int
	heapLive   uint64
	numGC      uint64
}

type heapPoint struct {
	at    time.Time
	bytes uint64
	numGC uint64
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
	HeapLiveBytes  uint64
	HeapGrowthRate float64
	NumGC          uint64
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
	heap      []heapPoint
	mu        sync.Mutex
}

// New creates a Watchdog with default cadence, thresholds, and reporter. The
// default reporter logs via slog and appends a durable diagnostic record under
// the app data directory, including a stack dump for goroutine alerts and a
// binary heap profile for heap alerts.
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
	samples := []metrics.Sample{
		{Name: "/gc/heap/live:bytes"},
		{Name: "/gc/cycles/total:gc-cycles"},
	}
	metrics.Read(samples)
	return sample{
		goroutines: runtime.NumGoroutine(),
		heapLive:   samples[0].Value.Uint64(),
		numGC:      samples[1].Value.Uint64(),
	}
}

// Start runs the watchdog in the background and returns an idempotent stop
// function. Stop cancels the watchdog and waits for its goroutine to exit.
func (w *Watchdog) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

// Run samples on the configured interval until ctx is cancelled. It takes an
// immediate baseline sample so the growth check has a reference point, then
// reports any breaches through the configured reporter.
func (w *Watchdog) Run(ctx context.Context) {
	// Establish the baseline right away rather than waiting a full interval.
	initial := w.sampleFn()
	now := w.now()
	w.mu.Lock()
	if w.baseline == 0 {
		w.baseline = initial.goroutines
	}
	w.recordHeapPointLocked(initial, now)
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
			Timestamp:     now,
			Kind:          kindGoroutines,
			Message:       fmt.Sprintf("goroutine count %d exceeds threshold (baseline %d)", s.goroutines, w.baseline),
			Goroutines:    s.goroutines,
			Baseline:      w.baseline,
			HeapLiveBytes: s.heapLive,
			NumGC:         s.numGC,
		})
	}

	if message, growthRate, breached := w.heapBreachedLocked(s, now); breached && w.allowAlertLocked(kindHeap, now) {
		alerts = append(alerts, Alert{
			Timestamp:      now,
			Kind:           kindHeap,
			Message:        message,
			Goroutines:     s.goroutines,
			Baseline:       w.baseline,
			HeapLiveBytes:  s.heapLive,
			HeapGrowthRate: growthRate,
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

func (w *Watchdog) heapBreachedLocked(s sample, now time.Time) (string, float64, bool) {
	w.recordHeapPointLocked(s, now)

	if w.cfg.HeapCeilingBytes > 0 && s.heapLive >= w.cfg.HeapCeilingBytes {
		return fmt.Sprintf(
			"post-GC live heap %.0f MiB exceeds ceiling %.0f MiB",
			float64(s.heapLive)/(1<<20),
			float64(w.cfg.HeapCeilingBytes)/(1<<20),
		), 0, true
	}

	minSamples := w.cfg.HeapGrowthMinSamples
	if minSamples < 2 || len(w.heap) < minSamples {
		return "", 0, false
	}
	points := w.heap[len(w.heap)-minSamples:]
	for i := 1; i < len(points); i++ {
		if points[i].bytes <= points[i-1].bytes {
			return "", 0, false
		}
	}

	elapsed := points[len(points)-1].at.Sub(points[0].at)
	if elapsed <= 0 {
		return "", 0, false
	}
	growth := points[len(points)-1].bytes - points[0].bytes
	if growth < w.cfg.HeapGrowthMinBytes {
		return "", 0, false
	}

	perMinute := float64(growth) / elapsed.Minutes()
	if perMinute < float64(w.cfg.HeapGrowthBytesPerMinute) {
		return "", perMinute, false
	}
	return fmt.Sprintf(
		"post-GC live heap grew by %.0f MiB over %s (%.1f MiB/min)",
		float64(growth)/(1<<20),
		elapsed.Round(time.Second),
		perMinute/(1<<20),
	), perMinute, true
}

func (w *Watchdog) recordHeapPointLocked(s sample, now time.Time) {
	if s.heapLive == 0 {
		return
	}
	if len(w.heap) > 0 && w.heap[len(w.heap)-1].numGC == s.numGC {
		return
	}

	w.heap = append(w.heap, heapPoint{at: now, bytes: s.heapLive, numGC: s.numGC})
	if w.cfg.HeapGrowthWindow <= 0 {
		return
	}
	cutoff := now.Add(-w.cfg.HeapGrowthWindow)
	first := 0
	for first < len(w.heap) && w.heap[first].at.Before(cutoff) {
		first++
	}
	if first > 0 {
		w.heap = append(w.heap[:0], w.heap[first:]...)
	}
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
		"heap_live_mb", fmt.Sprintf("%.1f", float64(a.HeapLiveBytes)/(1<<20)),
		"heap_growth_mb_per_min", fmt.Sprintf("%.1f", a.HeapGrowthRate/(1<<20)),
		"gc_cycles", a.NumGC,
	)

	record := formatRecord(a)
	switch a.Kind {
	case kindGoroutines:
		record += "\n\n" + goroutineDump()
	case kindHeap:
		path, captured, err := captureHeapProfile(a.Timestamp)
		if err != nil {
			slog.Warn("watchdog: cannot capture heap profile", "error", err)
		} else if captured {
			record += "\nheap_profile=" + path
		}
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
		"[%s] watchdog %s alert: %s (goroutines=%d baseline=%d heap_live_mb=%.1f heap_growth_mb_per_min=%.1f gc=%d)",
		a.Timestamp.UTC().Format(time.RFC3339),
		a.Kind,
		a.Message,
		a.Goroutines,
		a.Baseline,
		float64(a.HeapLiveBytes)/(1<<20),
		a.HeapGrowthRate/(1<<20),
		a.NumGC,
	)
}
