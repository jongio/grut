package diag

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jongio/grut/internal/config"
)

// maxDiagLogBytes caps the watchdog diagnostics log before it is rotated. One
// previous generation is kept (watchdog.log.1), bounding disk use to ~4 MiB.
const maxDiagLogBytes = 2 << 20 // 2 MiB

const (
	maxHeapProfiles        = 3
	heapProfileMinInterval = 5 * time.Minute
	heapProfilePrefix      = "watchdog-heap-"
	heapProfileSuffix      = ".pprof"
)

// diagWriteMu serialises rotation-plus-append so concurrent reporters cannot
// interleave a rotation with a write.
var diagWriteMu sync.Mutex

var diagnosticsDirPath = func() string {
	return filepath.Join(config.DataDir(), "diagnostics")
}

// diagLogPath returns the path to the watchdog diagnostics log. It is a
// package variable so tests can redirect writes to a temporary location.
var diagLogPath = func() string {
	return filepath.Join(diagnosticsDirPath(), "watchdog.log")
}

var writeHeapProfile = func(w io.Writer) error {
	runtime.GC()
	profile := pprof.Lookup("heap")
	if profile == nil {
		return fmt.Errorf("heap profile is unavailable")
	}
	return profile.WriteTo(w, 0)
}

var heapProfileState struct {
	sync.Mutex
	last time.Time
}

// writeDiag appends a diagnostic record to the durable watchdog log, rotating
// the file first if it has grown past the size cap. Failures are logged and
// otherwise ignored: diagnostics must never take down the app.
func writeDiag(record string) {
	diagWriteMu.Lock()
	defer diagWriteMu.Unlock()

	path := diagLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.Warn("watchdog: cannot create diagnostics dir", "error", err)
		return
	}

	if fi, err := os.Stat(path); err == nil && fi.Size() >= maxDiagLogBytes {
		// Keep a single previous generation; ignore rename errors (the
		// subsequent append still succeeds).
		_ = os.Rename(path, path+".1")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("watchdog: cannot open diagnostics log", "error", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(record + "\n\n"); err != nil {
		slog.Warn("watchdog: cannot write diagnostics log", "error", err)
	}
}

func captureHeapProfile(at time.Time) (string, bool, error) {
	heapProfileState.Lock()
	defer heapProfileState.Unlock()

	if !heapProfileState.last.IsZero() && at.Sub(heapProfileState.last) < heapProfileMinInterval {
		return "", false, nil
	}

	dir := diagnosticsDirPath()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("create diagnostics dir: %w", err)
	}
	if err := pruneHeapProfiles(dir, maxHeapProfiles-1); err != nil {
		return "", false, err
	}
	name := heapProfilePrefix + at.UTC().Format("20060102T150405.000000000Z") + heapProfileSuffix
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", false, fmt.Errorf("create heap profile: %w", err)
	}
	if err := writeHeapProfile(f); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", false, fmt.Errorf("write heap profile: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", false, fmt.Errorf("close heap profile: %w", err)
	}
	heapProfileState.last = at
	return path, true, nil
}

func pruneHeapProfiles(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read diagnostics dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, heapProfilePrefix) && strings.HasSuffix(name, heapProfileSuffix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return fmt.Errorf("remove old heap profile: %w", err)
		}
		names = names[1:]
	}
	return nil
}
