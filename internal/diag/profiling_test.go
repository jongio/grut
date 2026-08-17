package diag

import (
	"io"
	"runtime"
	"sync"
	"testing"
)

func TestProfiler_RestoresRuntimeSampling(t *testing.T) {
	originalMutexRate := runtime.SetMutexProfileFraction(7)
	defer runtime.SetMutexProfileFraction(originalMutexRate)
	originalBlockRate := managedBlockProfileRate.set(12345)
	defer managedBlockProfileRate.set(originalBlockRate)

	profiler := StartProfiling(ProfilingOptions{
		MutexProfileFraction: SamplingRate{Enabled: true, Rate: 1},
		BlockProfileRate:     SamplingRate{Enabled: true, Rate: 1000},
	}, io.Discard)
	if got := runtime.SetMutexProfileFraction(-1); got != 1 {
		t.Fatalf("mutex sampling rate = %d, want 1", got)
	}
	if got := managedBlockProfileRate.current(); got != 1000 {
		t.Fatalf("block sampling rate = %d, want 1000", got)
	}

	profiler.Close()
	if got := runtime.SetMutexProfileFraction(-1); got != 7 {
		t.Fatalf("restored mutex sampling rate = %d, want 7", got)
	}
	if got := managedBlockProfileRate.current(); got != 12345 {
		t.Fatalf("restored block sampling rate = %d, want 12345", got)
	}
}

func TestProfiler_CloseConcurrentSafe(t *testing.T) {
	profiler := StartProfiling(ProfilingOptions{}, io.Discard)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			profiler.Close()
		}()
	}
	wg.Wait()
}
