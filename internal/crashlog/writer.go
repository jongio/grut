package crashlog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jongio/grut/internal/config"
)

// maxCrashFiles is the default retention limit for crash report files.
const maxCrashFiles = 20

// crashDir returns the directory where crash reports are stored.
func crashDir() string {
	return filepath.Join(config.DataDir(), "crashes")
}

// Write serialises report as indented JSON and writes it to the crash
// directory. It returns the full path of the created file. Old reports
// beyond the retention limit are pruned automatically.
func Write(report *CrashReport) (string, error) {
	dir := crashDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating crash directory: %w", err)
	}

	ts := report.Timestamp.Format("20060102-150405")
	shortID := report.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	filename := fmt.Sprintf("crash-%s-%s.json", ts, shortID)
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
