package diag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // pprof is opt-in and bound to a caller-provided loopback address
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

const pprofShutdownTimeout = 5 * time.Second

// SamplingRate distinguishes an omitted sampling flag from an explicitly
// requested zero rate.
type SamplingRate struct {
	Enabled bool
	Rate    int
}

// ProfilingOptions configures process profiling for one command execution.
type ProfilingOptions struct {
	CPUProfilePath       string
	MemoryProfilePath    string
	PprofAddress         string
	MutexProfileFraction SamplingRate
	BlockProfileRate     SamplingRate
}

// Profiler owns all profiling resources for one command execution.
type Profiler struct {
	stderr      io.Writer
	cpuFile     *os.File
	memoryPath  string
	pprof       *pprofRuntime
	sampling    samplingRestore
	cleanupOnce sync.Once
}

// StartProfiling applies requested sampling rates and starts file and HTTP
// profiling. Startup failures are warnings because profiling is diagnostic and
// must not prevent the requested command from running.
func StartProfiling(options ProfilingOptions, stderr io.Writer) *Profiler {
	if stderr == nil {
		stderr = os.Stderr
	}
	profiler := &Profiler{
		stderr:     stderr,
		memoryPath: options.MemoryProfilePath,
	}
	profiler.sampling = configureRuntimeSampling(
		options.MutexProfileFraction,
		options.BlockProfileRate,
		stderr,
	)
	profiler.startCPUProfile(options.CPUProfilePath)
	profiler.startPprof(options.PprofAddress)
	return profiler
}

// Close stops and waits for all profiling activity, restores runtime sampling
// rates, and flushes requested file profiles. It is safe to call concurrently.
func (p *Profiler) Close() {
	if p == nil {
		return
	}
	p.cleanupOnce.Do(func() {
		p.pprof.stop(p.stderr)
		restoreRuntimeSampling(p.sampling)
		if p.cpuFile != nil {
			pprof.StopCPUProfile()
			_ = p.cpuFile.Close()
		}
		p.writeMemoryProfile()
	})
}

func (p *Profiler) startCPUProfile(path string) {
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(p.stderr, "Warning: could not create CPU profile %q: %v\n", path, err)
		return
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(p.stderr, "Warning: could not start CPU profile: %v\n", err)
		_ = f.Close()
		return
	}
	p.cpuFile = f
}

func (p *Profiler) startPprof(addr string) {
	if addr == "" {
		return
	}
	server, err := startPprofRuntime(addr)
	if err != nil {
		fmt.Fprintf(p.stderr, "Warning: could not bind pprof server to %s: %v\n", addr, err)
		return
	}
	p.pprof = server
	slog.Info("pprof server starting", "addr", "http://"+addr+"/debug/pprof/")
}

func (p *Profiler) writeMemoryProfile() {
	if p.memoryPath == "" {
		return
	}
	f, err := os.Create(p.memoryPath)
	if err != nil {
		fmt.Fprintf(p.stderr, "Warning: could not create memory profile %q: %v\n", p.memoryPath, err)
		return
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		fmt.Fprintf(p.stderr, "Warning: could not write memory profile: %v\n", err)
	}
}

type pprofRuntime struct {
	server         *http.Server
	serveDone      chan error
	memstatsCancel context.CancelFunc
	memstatsDone   chan struct{}
}

func startPprofRuntime(addr string) (*pprofRuntime, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, err
	}

	server := &http.Server{ //nolint:gosec // pprof is an opt-in loopback-only development tool
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	memstatsCtx, cancelMemstats := context.WithCancel(context.Background())
	profiling := &pprofRuntime{
		server:         server,
		serveDone:      make(chan error, 1),
		memstatsCancel: cancelMemstats,
		memstatsDone:   make(chan struct{}),
	}

	go func() {
		profiling.serveDone <- server.Serve(listener)
		close(profiling.serveDone)
	}()
	go func() {
		defer close(profiling.memstatsDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-memstatsCtx.Done():
				return
			case <-ticker.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				slog.Info(
					"memstats",
					"heap_alloc_mb", fmt.Sprintf("%.1f", float64(ms.HeapAlloc)/(1024*1024)),
					"heap_inuse_mb", fmt.Sprintf("%.1f", float64(ms.HeapInuse)/(1024*1024)),
					"heap_objects", ms.HeapObjects,
					"goroutines", runtime.NumGoroutine(),
					"gc_cycles", ms.NumGC,
				)
			}
		}
	}()

	return profiling, nil
}

func (p *pprofRuntime) stop(w io.Writer) {
	if p == nil {
		return
	}

	p.memstatsCancel()
	<-p.memstatsDone

	ctx, cancel := context.WithTimeout(context.Background(), pprofShutdownTimeout)
	err := p.server.Shutdown(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(w, "Warning: could not gracefully shut down pprof server: %v\n", err)
		if closeErr := p.server.Close(); closeErr != nil {
			fmt.Fprintf(w, "Warning: could not close pprof server: %v\n", closeErr)
		}
	}

	if serveErr := <-p.serveDone; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		fmt.Fprintf(w, "Warning: pprof server stopped unexpectedly: %v\n", serveErr)
	}
}

type blockProfileRateController struct {
	mu   sync.Mutex
	rate int
}

func (c *blockProfileRateController) set(rate int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.rate
	runtime.SetBlockProfileRate(rate)
	c.rate = rate
	return previous
}

func (c *blockProfileRateController) current() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rate
}

// runtime.SetBlockProfileRate has no getter. Grut is the only code in this
// process that changes it, so retain the managed rate to restore nested command
// lifecycles correctly. The Go runtime starts with block profiling disabled.
var managedBlockProfileRate blockProfileRateController

type samplingRestore struct {
	mutexChanged bool
	mutexRate    int
	blockChanged bool
	blockRate    int
}

func configureRuntimeSampling(mutexRate, blockRate SamplingRate, stderr io.Writer) samplingRestore {
	var restore samplingRestore
	if mutexRate.Enabled {
		if mutexRate.Rate < 0 {
			fmt.Fprintf(stderr, "Warning: invalid mutex profile fraction %d (must be at least 0)\n", mutexRate.Rate)
		} else {
			restore.mutexChanged = true
			restore.mutexRate = runtime.SetMutexProfileFraction(mutexRate.Rate)
		}
	}
	if blockRate.Enabled {
		if blockRate.Rate < 0 {
			fmt.Fprintf(stderr, "Warning: invalid block profile rate %d (must be at least 0)\n", blockRate.Rate)
		} else {
			restore.blockChanged = true
			restore.blockRate = managedBlockProfileRate.set(blockRate.Rate)
		}
	}
	return restore
}

func restoreRuntimeSampling(restore samplingRestore) {
	if restore.blockChanged {
		managedBlockProfileRate.set(restore.blockRate)
	}
	if restore.mutexChanged {
		runtime.SetMutexProfileFraction(restore.mutexRate)
	}
}
