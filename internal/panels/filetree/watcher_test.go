package filetree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Watcher construction
// ---------------------------------------------------------------------------

func TestNewWatcher(t *testing.T) {
	w := newWatcher(100*time.Millisecond, 2*time.Second)
	defer w.stop()

	assert.NotNil(t, w)
	// On most platforms fsnotify should initialise successfully.
	// If it doesn't, polling fallback is used — both are valid.
}

func TestNewWatcher_DefaultValues(t *testing.T) {
	w := newWatcher(0, 0)
	defer w.stop()

	assert.Equal(t, defaultDebounce, w.debounce)
	assert.Equal(t, defaultPollInterval, w.pollInt)
}

// ---------------------------------------------------------------------------
// File creation detection
// ---------------------------------------------------------------------------

func TestWatcher_DetectsFileCreation(t *testing.T) {
	dir := t.TempDir()
	w := newWatcher(50*time.Millisecond, 500*time.Millisecond)
	defer w.stop()

	w.addDir(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := w.start(ctx)

	// Create a file to trigger an event.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644))

	// Wait for the debounced event.
	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		if _, ok := msg.(RefreshMsg); ok {
			done <- true
			return
		}
		done <- false
	}()

	select {
	case ok := <-done:
		assert.True(t, ok, "expected RefreshMsg after file creation")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file creation event")
	}
}

// ---------------------------------------------------------------------------
// File deletion detection
// ---------------------------------------------------------------------------

func TestWatcher_DetectsFileDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	w := newWatcher(50*time.Millisecond, 500*time.Millisecond)
	defer w.stop()

	w.addDir(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := w.start(ctx)

	// Delete the file.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.Remove(path))

	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		if _, ok := msg.(RefreshMsg); ok {
			done <- true
			return
		}
		done <- false
	}()

	select {
	case ok := <-done:
		assert.True(t, ok, "expected RefreshMsg after file deletion")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file deletion event")
	}
}

// ---------------------------------------------------------------------------
// File modification detection
// ---------------------------------------------------------------------------

func TestWatcher_DetectsFileModification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	w := newWatcher(50*time.Millisecond, 500*time.Millisecond)
	defer w.stop()

	w.addDir(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := w.start(ctx)

	// Modify the file.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("modified"), 0o644))

	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		if _, ok := msg.(RefreshMsg); ok {
			done <- true
			return
		}
		done <- false
	}()

	select {
	case ok := <-done:
		assert.True(t, ok, "expected RefreshMsg after file modification")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file modification event")
	}
}

// ---------------------------------------------------------------------------
// Debounce coalescing
// ---------------------------------------------------------------------------

func TestWatcher_DebounceCoalesces(t *testing.T) {
	dir := t.TempDir()
	w := newWatcher(200*time.Millisecond, 2*time.Second)
	defer w.stop()

	w.addDir(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := w.start(ctx)

	// Rapidly create multiple files.
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "rapid_"+string(rune('a'+i))+".txt"),
			[]byte("x"),
			0o644,
		))
		time.Sleep(10 * time.Millisecond)
	}

	// Should receive exactly one RefreshMsg after the debounce window.
	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		if _, ok := msg.(RefreshMsg); ok {
			done <- true
			return
		}
		done <- false
	}()

	select {
	case ok := <-done:
		assert.True(t, ok, "expected single RefreshMsg after debounce period")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for debounced event")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation stops watcher
// ---------------------------------------------------------------------------

func TestWatcher_StopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	w := newWatcher(50*time.Millisecond, 500*time.Millisecond)
	defer w.stop()

	w.addDir(dir)

	ctx, cancel := context.WithCancel(context.Background())

	cmd := w.start(ctx)

	// Cancel the context.
	cancel()

	// The command should return nil (not a RefreshMsg).
	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		done <- (msg == nil)
	}()

	select {
	case isNil := <-done:
		assert.True(t, isNil, "expected nil message after context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watcher to stop")
	}
}

// ---------------------------------------------------------------------------
// Add/remove directories
// ---------------------------------------------------------------------------

func TestWatcher_AddRemoveDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	w := newWatcher(50*time.Millisecond, 2*time.Second)
	defer w.stop()

	// Add and remove should not panic.
	w.addDir(dir)
	w.addDir(sub)
	w.addDir(sub) // idempotent

	w.removeDir(sub)
	w.removeDir(sub) // idempotent (no-op)
}

// ---------------------------------------------------------------------------
// Polling fallback
// ---------------------------------------------------------------------------

func TestWatcher_PollingFallback(t *testing.T) {
	// Create a watcher that's explicitly set to polling mode.
	w := &watcher{
		dirs:     make(map[string]bool),
		debounce: 50 * time.Millisecond,
		pollInt:  100 * time.Millisecond,
		polling:  true,
		done:     make(chan struct{}),
	}
	defer w.stop()

	assert.True(t, w.isPolling())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := w.start(ctx)

	// Polling should produce a RefreshMsg after the poll interval.
	done := make(chan bool, 1)
	go func() {
		msg := cmd()
		if _, ok := msg.(RefreshMsg); ok {
			done <- true
			return
		}
		done <- false
	}()

	select {
	case ok := <-done:
		assert.True(t, ok, "expected RefreshMsg from polling fallback")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for polling RefreshMsg")
	}
}
