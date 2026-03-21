package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

const (
	// defaultMaxSize is the file size threshold (10 MB) that triggers rotation.
	defaultMaxSize = 10 * 1024 * 1024
	// maxRotatedFiles is how many rotated copies (.1, .2, .3) to keep.
	maxRotatedFiles = 3
)

// AuditLogger records every AI operation for transparency and debugging.
// Entries are appended to a structured log file with automatic rotation.
type AuditLogger struct {
	file    *os.File
	path    string
	maxSize int64 // bytes before rotation
	mu      sync.Mutex
}

// AuditEntry captures a single AI operation.
type AuditEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Operation  string    `json:"operation"` // "conflict_resolve", "commit_message", "chat", etc.
	Provider   string    `json:"provider"`  // "copilot", "claude"
	Result     string    `json:"result"`    // "accepted", "rejected", "modified", "error"
	Error      string    `json:"error,omitempty"`
	FilesSent  []string  `json:"files_sent"` // Paths sent to AI
	Redactions int       `json:"redactions"` // Number of redactions applied
	TokensIn   int       `json:"tokens_in"`
	TokensOut  int       `json:"tokens_out"`
}

// NewAuditLogger creates a logger writing to the given path.
// If path is empty, uses default: ~/.config/grut/ai-audit.log
func NewAuditLogger(path string) (*AuditLogger, error) {
	if path == "" {
		path = filepath.Join(xdg.ConfigHome, "grut", "ai-audit.log")
	}
	// Ensure parent directories exist.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating audit log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening audit log: %w", err)
	}
	return &AuditLogger{
		path:    path,
		file:    f,
		maxSize: defaultMaxSize,
	}, nil
}

// Log appends an entry to the audit log. Thread-safe.
func (a *AuditLogger) Log(entry AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling audit entry: %w", err)
	}
	data = append(data, '\n')
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.rotateIfNeeded(); err != nil {
		return fmt.Errorf("rotating audit log: %w", err)
	}
	if _, err := a.file.Write(data); err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}
	if err := a.file.Sync(); err != nil {
		return fmt.Errorf("syncing audit log: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying file.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file == nil {
		return nil
	}
	if err := a.file.Sync(); err != nil {
		_ = a.file.Close()
		a.file = nil
		return fmt.Errorf("syncing audit log on close: %w", err)
	}
	err := a.file.Close()
	a.file = nil
	if err != nil {
		return fmt.Errorf("closing audit log: %w", err)
	}
	return nil
}

// rotateIfNeeded checks the current file size and rotates if it exceeds
// maxSize. Must be called with a.mu held.
func (a *AuditLogger) rotateIfNeeded() error {
	info, err := a.file.Stat()
	if err != nil {
		return fmt.Errorf("stat audit log: %w", err)
	}
	if info.Size() < a.maxSize {
		return nil
	}
	return a.rotate()
}

// rotate shifts rotated files (.1 → .2, .2 → .3) and renames the current
// file to .1, then opens a fresh file. Must be called with a.mu held.
func (a *AuditLogger) rotate() error {
	if err := a.file.Sync(); err != nil {
		return fmt.Errorf("syncing before rotation: %w", err)
	}
	if err := a.file.Close(); err != nil {
		return fmt.Errorf("closing before rotation: %w", err)
	}
	// Shift existing rotated files: .2 → .3, .1 → .2, etc.
	for i := maxRotatedFiles; i > 1; i-- {
		src := fmt.Sprintf("%s.%d", a.path, i-1)
		dst := fmt.Sprintf("%s.%d", a.path, i)
		_ = os.Remove(dst)
		_ = os.Rename(src, dst)
	}
	// Rename current log to .1.
	if err := os.Rename(a.path, a.path+".1"); err != nil {
		f, openErr := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			return fmt.Errorf("rename failed (%w) and reopen failed: %w", err, openErr)
		}
		a.file = f
		return fmt.Errorf("renaming audit log for rotation: %w", err)
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening new audit log after rotation: %w", err)
	}
	a.file = f
	return nil
}
