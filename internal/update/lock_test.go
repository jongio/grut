package update

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireUpdateLockReleaseAndReacquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), lockFileName)

	lock, err := acquireUpdateLock(lockPath)
	if err != nil {
		t.Fatalf("acquireUpdateLock() error = %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}

	if _, err := acquireUpdateLock(lockPath); err == nil {
		t.Fatal("expected second acquireUpdateLock to fail while lock is held")
	}

	releaseUpdateLock(lock)
	if _, err := os.Stat(lockPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lock file should be removed on release, stat err = %v", err)
	}

	lock, err = acquireUpdateLock(lockPath)
	if err != nil {
		t.Fatalf("reacquire after release error = %v", err)
	}
	releaseUpdateLock(lock)
}

func TestAcquireUpdateLockReplacesStaleLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), lockFileName)
	stale := lockMetadata{
		PID:       4242,
		CreatedAt: time.Now().Add(-lockStaleDuration - time.Minute).UTC(),
	}
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal stale lock: %v", err)
	}
	if err := os.WriteFile(lockPath, raw, cacheFilePerm); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	lock, err := acquireUpdateLock(lockPath)
	if err != nil {
		t.Fatalf("acquireUpdateLock() with stale lock error = %v", err)
	}
	defer releaseUpdateLock(lock)

	raw, err = os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read refreshed lock: %v", err)
	}

	var refreshed lockMetadata
	if err := json.Unmarshal(raw, &refreshed); err != nil {
		t.Fatalf("unmarshal refreshed lock: %v", err)
	}
	if refreshed.PID != os.Getpid() {
		t.Fatalf("refreshed PID = %d, want %d", refreshed.PID, os.Getpid())
	}
	if time.Since(refreshed.CreatedAt) > time.Minute {
		t.Fatalf("refreshed CreatedAt = %v, want recent time", refreshed.CreatedAt)
	}
}

func TestIsStaleLockFallsBackToModTimeForInvalidMetadata(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), lockFileName)
	if err := os.WriteFile(lockPath, []byte("not-json"), cacheFilePerm); err != nil {
		t.Fatalf("write invalid lock: %v", err)
	}
	old := time.Now().Add(-lockStaleDuration - time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("chtimes lock file: %v", err)
	}

	stale, err := isStaleLock(lockPath)
	if err != nil {
		t.Fatalf("isStaleLock() error = %v", err)
	}
	if !stale {
		t.Fatal("expected invalid old lock metadata to be treated as stale")
	}
}

func TestReleaseUpdateLock_NilSafe(t *testing.T) {
	releaseUpdateLock(nil)
	releaseUpdateLock(&updateLock{})
}
