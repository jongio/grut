package update

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	lockFileName      = "grut-update.lock"
	lockStaleDuration = 30 * time.Minute
)

type updateLock struct {
	path string
}
type lockMetadata struct {
	CreatedAt time.Time `json:"createdAt"`
	PID       int       `json:"pid"`
}

// acquireUpdateLock creates an exclusive lock file to prevent concurrent
// updates. If a stale lock (>30 min) exists, it is replaced.
func acquireUpdateLock(path string) (*updateLock, error) {
	metadata := lockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now().UTC(),
	}
	for range 2 {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("encoding lock metadata: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, cacheFilePerm)
		if err == nil {
			if _, writeErr := file.Write(raw); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("writing lock file: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("closing lock file: %w", closeErr)
			}
			return &updateLock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("creating lock file: %w", err)
		}
		stale, staleErr := isStaleLock(path)
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			return nil, fmt.Errorf("lock file exists at %s", path)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing stale lock file: %w", err)
		}
	}
	return nil, fmt.Errorf("lock file exists at %s", path)
}

// releaseUpdateLock removes the lock file.
func releaseUpdateLock(lock *updateLock) {
	if lock == nil || lock.path == "" {
		return
	}
	_ = os.Remove(lock.path)
}

func isStaleLock(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading lock file: %w", err)
	}
	var metadata lockMetadata
	if err := json.Unmarshal(raw, &metadata); err == nil && !metadata.CreatedAt.IsZero() {
		return time.Since(metadata.CreatedAt) > lockStaleDuration, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stating lock file: %w", err)
	}
	return time.Since(info.ModTime()) > lockStaleDuration, nil
}
