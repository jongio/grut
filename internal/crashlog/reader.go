package crashlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// List reads all crash reports from the crash directory and returns them
// sorted newest-first. Files that fail to parse are skipped with a
// warning logged.
func List() ([]*CrashReport, error) {
	dir := crashDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading crash directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "crash-") && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}

	// Sort descending so newest reports come first.
	slices.Sort(names)
	slices.Reverse(names)

	var reports []*CrashReport
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			slog.Warn("skipping unreadable crash report", "file", name, "error", err)
			continue
		}
		var r CrashReport
		if err := json.Unmarshal(data, &r); err != nil {
			slog.Warn("skipping malformed crash report", "file", name, "error", err)
			continue
		}
		reports = append(reports, &r)
	}

	return reports, nil
}

// Clear deletes all crash report files from the crash directory and
// returns the number of files removed.
func Clear() (int, error) {
	dir := crashDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading crash directory: %w", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "crash-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return count, fmt.Errorf("removing crash report %s: %w", e.Name(), err)
		}
		count++
	}

	return count, nil
}
