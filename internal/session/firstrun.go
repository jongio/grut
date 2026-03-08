package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jongio/grut/internal/config"
)

// firstRunMarker is the filename used to track whether the first-run
// help overlay has been shown.
const firstRunMarker = ".first-run-done"

// firstRunDir returns the directory that contains the first-run marker file.
// This is the XDG data directory for grut (~/.local/share/grut).
func firstRunDir() string {
	return config.DataDir()
}

// firstRunPath returns the full path to the first-run marker file.
func firstRunPath() string {
	return filepath.Join(firstRunDir(), firstRunMarker)
}

// IsFirstRun returns true when the first-run marker file does not exist,
// meaning the user has never launched grut before (or has not seen the
// help overlay).
func IsFirstRun() bool {
	return isFirstRunAt(firstRunPath())
}

// IsFirstRunIn checks for the marker file in the given directory.
// Used by tests that need to control the marker location.
func IsFirstRunIn(dir string) bool {
	return isFirstRunAt(filepath.Join(dir, firstRunMarker))
}

// isFirstRunAt reports whether the marker file at path is absent.
func isFirstRunAt(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// MarkFirstRunDone creates the first-run marker file, indicating that
// the help overlay has been shown. The operation is idempotent — calling
// it multiple times has no effect beyond the initial file creation.
func MarkFirstRunDone() error {
	return markFirstRunDoneAt(firstRunDir(), firstRunPath())
}

// MarkFirstRunDoneIn creates the marker file in the given directory.
// Used by tests that need to control the marker location.
func MarkFirstRunDoneIn(dir string) error {
	return markFirstRunDoneAt(dir, filepath.Join(dir, firstRunMarker))
}

// markFirstRunDoneAt creates the marker file at path, ensuring dir exists.
func markFirstRunDoneAt(dir, path string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create first-run dir: %w", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		return fmt.Errorf("write first-run marker: %w", err)
	}
	return nil
}
