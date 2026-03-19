package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// First run detection (no marker → true)
// ---------------------------------------------------------------------------

func TestIsFirstRunIn_NoMarker_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	assert.True(t, IsFirstRunIn(dir), "should be first run when marker does not exist")
}

// ---------------------------------------------------------------------------
// After marking done (marker exists → false)
// ---------------------------------------------------------------------------

func TestIsFirstRunIn_AfterMark_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, MarkFirstRunDoneIn(dir))

	assert.False(t, IsFirstRunIn(dir), "should not be first run after marking done")
}

// ---------------------------------------------------------------------------
// Mark creates file
// ---------------------------------------------------------------------------

func TestMarkFirstRunDoneIn_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, MarkFirstRunDoneIn(dir))

	path := filepath.Join(dir, firstRunMarker)
	info, err := os.Stat(path)
	require.NoError(t, err, "marker file should exist")
	assert.False(t, info.IsDir(), "marker should be a regular file")
}

// ---------------------------------------------------------------------------
// Idempotent marking
// ---------------------------------------------------------------------------

func TestMarkFirstRunDoneIn_Idempotent(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, MarkFirstRunDoneIn(dir))
	require.NoError(t, MarkFirstRunDoneIn(dir), "second call should not error")

	assert.False(t, IsFirstRunIn(dir))
}

// ---------------------------------------------------------------------------
// Mark creates intermediate directories
// ---------------------------------------------------------------------------

func TestMarkFirstRunDoneIn_CreatesDirectories(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "deep", "nested", "dir")

	require.NoError(t, MarkFirstRunDoneIn(nested))

	path := filepath.Join(nested, firstRunMarker)
	_, err := os.Stat(path)
	assert.NoError(t, err, "marker file should exist in nested directory")
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestIsFirstRunIn_MarkerIsDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a directory with the marker name (unusual case).
	require.NoError(t, os.Mkdir(filepath.Join(dir, firstRunMarker), 0o755))

	// os.Stat succeeds → not first run (the file exists, even though it's a dir).
	assert.False(t, IsFirstRunIn(dir),
		"should return false when marker path exists as a directory")
}

func TestMarkFirstRunDoneIn_FileContents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, MarkFirstRunDoneIn(dir))

	data, err := os.ReadFile(filepath.Join(dir, firstRunMarker))
	require.NoError(t, err)
	assert.Empty(t, data, "marker file should be empty")
}

func TestIsFirstRunIn_MultipleChecks(t *testing.T) {
	dir := t.TempDir()

	// Multiple calls before marking should all return true.
	assert.True(t, IsFirstRunIn(dir))
	assert.True(t, IsFirstRunIn(dir))

	require.NoError(t, MarkFirstRunDoneIn(dir))

	// Multiple calls after marking should all return false.
	assert.False(t, IsFirstRunIn(dir))
	assert.False(t, IsFirstRunIn(dir))
}

// ---------------------------------------------------------------------------
// Global IsFirstRun / MarkFirstRunDone (uses config.DataDir)
// ---------------------------------------------------------------------------

func TestFirstRunDir_NotEmpty(t *testing.T) {
	dir := firstRunDir()
	assert.NotEmpty(t, dir, "firstRunDir should return a non-empty path")
}

func TestFirstRunPath_ContainsMarker(t *testing.T) {
	path := firstRunPath()
	assert.Contains(t, path, firstRunMarker, "firstRunPath should contain the marker filename")
}

func TestIsFirstRun_Global(t *testing.T) {
	// Just verify the global function doesn't panic and returns a bool.
	_ = IsFirstRun()
}

func TestMarkFirstRunDone_Global(t *testing.T) {
	// MarkFirstRunDone writes to the real config directory; after calling it,
	// IsFirstRun should return false.
	require.NoError(t, MarkFirstRunDone())
	assert.False(t, IsFirstRun(), "should not be first run after marking done")
}

// ---------------------------------------------------------------------------
// Reset first-run (marker removed → true again)
// ---------------------------------------------------------------------------

func TestResetFirstRunIn_RemovesMarker(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, MarkFirstRunDoneIn(dir))
	assert.False(t, IsFirstRunIn(dir), "precondition: should not be first run after marking")

	require.NoError(t, ResetFirstRunIn(dir))
	assert.True(t, IsFirstRunIn(dir), "should be first run after reset")
}

func TestResetFirstRunIn_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// Reset without ever marking — should not error.
	require.NoError(t, ResetFirstRunIn(dir), "reset when no marker should not error")
	assert.True(t, IsFirstRunIn(dir))
}

func TestResetFirstRunIn_AfterDoubleMarkAndReset(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, MarkFirstRunDoneIn(dir))
	require.NoError(t, MarkFirstRunDoneIn(dir))
	require.NoError(t, ResetFirstRunIn(dir))
	assert.True(t, IsFirstRunIn(dir), "should be first run after reset regardless of double mark")
}

func TestResetFirstRun_Global(t *testing.T) {
	// Ensure marked first, then reset.
	require.NoError(t, MarkFirstRunDone())
	require.NoError(t, ResetFirstRun())
	assert.True(t, IsFirstRun(), "should be first run after global reset")

	// Re-mark so other tests relying on global state aren't affected.
	require.NoError(t, MarkFirstRunDone())
}
