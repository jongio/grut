package crashlog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jongio/grut/internal/config"
)

// maxCrashFiles is the default retention limit for crash report files.
const maxCrashFiles = 20

// writeMu serialises Write (and the pruneOld it calls) so concurrent crash
// reports cannot interleave their os.WriteFile / os.Remove calls. This matters
// because CaptureCmdPanic keeps the process alive and is invoked from Bubble
// Tea command goroutines that run concurrently (tea.Batch), unlike the
// pre-existing callers that wrote a single report immediately before exit.
var writeMu sync.Mutex

// writeSeq is a process-local monotonic counter that guarantees crash report
// filenames are unique even when several reports are written within the same
// clock tick (the timestamp and the short ID alone are not fine-grained enough
// on coarse-resolution clocks, e.g. Windows).
var writeSeq atomic.Uint64

// crashDir returns the directory where crash reports are stored.
func crashDir() string {
	return filepath.Join(config.DataDir(), "crashes")
}

// Write serialises report as indented JSON and writes it to the crash
// directory. It returns the full path of the created file. Old reports
// beyond the retention limit are pruned automatically.
func Write(report *CrashReport) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()

	dir := crashDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating crash directory: %w", err)
	}

	ts := report.Timestamp.Format("20060102-150405")
	shortID := report.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	// The atomic sequence disambiguates reports written within the same second
	// (the timestamp is second-resolution and shortID is stable for ~100s), so
	// concurrent command-goroutine panics do not overwrite each other's report.
	seq := writeSeq.Add(1)
	filename := fmt.Sprintf("crash-%s-%s-%d.json", ts, shortID, seq)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling crash report: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing crash report: %w", err)
	}

	if err := pruneOld(maxCrashFiles); err != nil {
		slog.Warn("failed to prune old crash reports", "error", err)
	}

	return path, nil
}

// pruneOld removes the oldest crash report files when the total count
// exceeds maxKeep. Files are sorted by name, which encodes the timestamp,
// so lexicographic order equals chronological order.
func pruneOld(maxKeep int) error {
	dir := crashDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading crash directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "crash-") && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}

	if len(files) <= maxKeep {
		return nil
	}

	slices.Sort(files)
	toRemove := files[:len(files)-maxKeep]
	for _, name := range toRemove {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("removing old crash report %s: %w", name, err)
		}
	}

	return nil
}
