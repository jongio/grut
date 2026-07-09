package diag

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/jongio/grut/internal/config"
)

// maxDiagLogBytes caps the watchdog diagnostics log before it is rotated. One
// previous generation is kept (watchdog.log.1), bounding disk use to ~4 MiB.
const maxDiagLogBytes = 2 << 20 // 2 MiB

// diagWriteMu serialises rotation-plus-append so concurrent reporters cannot
// interleave a rotation with a write.
var diagWriteMu sync.Mutex

// diagLogPath returns the path to the watchdog diagnostics log. It is a
// package variable so tests can redirect writes to a temporary location.
var diagLogPath = func() string {
	return filepath.Join(config.DataDir(), "diagnostics", "watchdog.log")
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
