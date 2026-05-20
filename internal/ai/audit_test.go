package ai

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Basic logging
// ---------------------------------------------------------------------------

func TestAuditLoggerBasicLogging(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	entry := AuditEntry{
		Operation:  "commit_message",
		Provider:   providerCopilot,
		FilesSent:  []string{"main.go", "go.mod"},
		Redactions: 1,
		TokensIn:   100,
		TokensOut:  50,
		Result:     "accepted",
	}

	err = al.Log(entry)
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var got AuditEntry
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "commit_message", got.Operation)
	assert.Equal(t, providerCopilot, got.Provider)
	assert.Equal(t, []string{"main.go", "go.mod"}, got.FilesSent)
	assert.Equal(t, 1, got.Redactions)
	assert.Equal(t, 100, got.TokensIn)
	assert.Equal(t, 50, got.TokensOut)
	assert.Equal(t, "accepted", got.Result)
	assert.Empty(t, got.Error)
	assert.False(t, got.Timestamp.IsZero(), "timestamp should be set automatically")
}

func TestAuditLoggerJSONLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err = al.Log(AuditEntry{
			Operation: fmt.Sprintf("op_%d", i),
			Provider:  providerCopilot,
			Result:    "accepted",
		})
		require.NoError(t, err)
	}
	require.NoError(t, al.Close())

	f, err := os.Open(logPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry AuditEntry
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "each line should be valid JSON")
		assert.Equal(t, fmt.Sprintf("op_%d", count), entry.Operation)
		count++
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, 5, count, "should have exactly 5 log entries")
}

func TestAuditLoggerErrorFieldPresent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	err = al.Log(AuditEntry{
		Operation: "chat",
		Provider:  providerClaude,
		Result:    "error",
		Error:     "connection timeout",
	})
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var got AuditEntry
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "error", got.Result)
	assert.Equal(t, "connection timeout", got.Error)
}

func TestAuditLoggerErrorFieldOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	err = al.Log(AuditEntry{
		Operation: "chat",
		Provider:  providerClaude,
		Result:    "accepted",
	})
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"error"`)
}

// ---------------------------------------------------------------------------
// Timestamp handling
// ---------------------------------------------------------------------------

func TestAuditLoggerTimestampAutoSet(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	before := time.Now()
	err = al.Log(AuditEntry{Operation: "chat", Provider: providerClaude, Result: "accepted"})
	require.NoError(t, err)
	after := time.Now()
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var got AuditEntry
	require.NoError(t, json.Unmarshal(data, &got))

	assert.False(t, got.Timestamp.Before(before), "timestamp should be >= start time")
	assert.False(t, got.Timestamp.After(after), "timestamp should be <= end time")
}

func TestAuditLoggerTimestampPreserved(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	explicit := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	err = al.Log(AuditEntry{
		Timestamp: explicit,
		Operation: "chat",
		Provider:  providerClaude,
		Result:    "accepted",
	})
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var got AuditEntry
	require.NoError(t, json.Unmarshal(data, &got))
	assert.True(t, got.Timestamp.Equal(explicit), "explicit timestamp should be preserved")
}

// ---------------------------------------------------------------------------
// Thread safety
// ---------------------------------------------------------------------------

func TestAuditLoggerConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	const goroutines = 20
	const entriesPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				logErr := al.Log(AuditEntry{
					Operation: fmt.Sprintf("op_%d_%d", id, i),
					Provider:  providerCopilot,
					Result:    "accepted",
				})
				assert.NoError(t, logErr)
			}
		}(g)
	}
	wg.Wait()
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Equal(t, goroutines*entriesPerGoroutine, len(lines), "all entries should be written")

	for _, line := range lines {
		var entry AuditEntry
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "each line should be valid JSON")
	}
}

// ---------------------------------------------------------------------------
// File rotation
// ---------------------------------------------------------------------------

func TestAuditLoggerFileRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)
	al.maxSize = 500 // trigger rotation after 500 bytes

	for i := 0; i < 50; i++ {
		err = al.Log(AuditEntry{
			Operation: "test_operation_with_padding",
			Provider:  providerCopilot,
			FilesSent: []string{"file1.go", "file2.go", "file3.go"},
			Result:    "accepted",
			TokensIn:  1000,
			TokensOut: 500,
		})
		require.NoError(t, err)
	}
	require.NoError(t, al.Close())

	// Current log should exist.
	_, err = os.Stat(logPath)
	assert.NoError(t, err, "current log file should exist")

	// At least the first rotated file should exist.
	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err, "rotated file .1 should exist")

	// Rotated file should contain valid JSON lines.
	data, err := os.ReadFile(logPath + ".1")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var entry AuditEntry
		assert.NoError(t, json.Unmarshal([]byte(line), &entry), "rotated file should contain valid JSON lines")
	}
}

func TestAuditLoggerRotationMaxFiles(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)
	al.maxSize = 200 // very small to force many rotations

	for i := 0; i < 100; i++ {
		err = al.Log(AuditEntry{
			Operation: "fill_operation_for_large_entry",
			Provider:  providerCopilot,
			FilesSent: []string{"a.go", "b.go"},
			Result:    "accepted",
			TokensIn:  999,
			TokensOut: 888,
		})
		require.NoError(t, err)
	}
	require.NoError(t, al.Close())

	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err, ".1 should exist")
	_, err = os.Stat(logPath + ".2")
	assert.NoError(t, err, ".2 should exist")
	_, err = os.Stat(logPath + ".3")
	assert.NoError(t, err, ".3 should exist")

	// .4 should NOT exist (max 3 rotated files).
	_, err = os.Stat(logPath + ".4")
	assert.True(t, errors.Is(err, fs.ErrNotExist), ".4 should not exist (max 3 rotated)")
}

// ---------------------------------------------------------------------------
// Default path resolution
// ---------------------------------------------------------------------------

func TestAuditLoggerDefaultPath(t *testing.T) {
	al, err := NewAuditLogger("")
	if err != nil {
		t.Skipf("cannot create audit logger at default path: %v", err)
	}
	defer func() { _ = al.Close() }()

	assert.Contains(t, al.path, "grut")
	assert.True(t, strings.HasSuffix(al.path, filepath.Join("grut", "ai-audit.log")))
}

// ---------------------------------------------------------------------------
// Close behaviour
// ---------------------------------------------------------------------------

func TestAuditLoggerCloseFlushes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	err = al.Log(AuditEntry{
		Operation: "close_test",
		Provider:  providerCopilot,
		Result:    "accepted",
	})
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var got AuditEntry
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "close_test", got.Operation)
}

func TestAuditLoggerCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	require.NoError(t, al.Close())
	assert.NoError(t, al.Close(), "second close should not error")
}

// ---------------------------------------------------------------------------
// Directory creation
// ---------------------------------------------------------------------------

func TestAuditLoggerCreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", "deeply", "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	err = al.Log(AuditEntry{Operation: "test", Provider: "test", Result: "accepted"})
	require.NoError(t, err)
	require.NoError(t, al.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"operation":"test"`)
}

// ---------------------------------------------------------------------------
// Concurrent rotation stress test
// ---------------------------------------------------------------------------

func TestAuditLoggerConcurrentRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)
	al.maxSize = 100 // very small to trigger rotation frequently

	const goroutines = 10
	const entriesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				logErr := al.Log(AuditEntry{
					Operation: fmt.Sprintf("concurrent_rot_%d_%d", id, i),
					Provider:  providerCopilot,
					FilesSent: []string{"a.go", "b.go"},
					Result:    "accepted",
					TokensIn:  500,
					TokensOut: 250,
				})
				assert.NoError(t, logErr)
			}
		}(g)
	}
	wg.Wait()
	require.NoError(t, al.Close())

	// All rotated files should contain valid JSON lines.
	for i := 1; i <= maxRotatedFiles; i++ {
		rotatedPath := fmt.Sprintf("%s.%d", logPath, i)
		if data, err := os.ReadFile(rotatedPath); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			for _, line := range lines {
				var entry AuditEntry
				assert.NoError(t, json.Unmarshal([]byte(line), &entry),
					"rotated file .%d should contain valid JSON lines", i)
			}
		}
	}

	// .4 must not exist.
	_, err = os.Stat(logPath + ".4")
	assert.True(t, errors.Is(err, fs.ErrNotExist), ".4 should not exist (max 3 rotated)")
}

// ---------------------------------------------------------------------------
// Log after close
// ---------------------------------------------------------------------------

func TestAuditLoggerLogAfterClose(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)
	require.NoError(t, al.Close())

	// Logging after close should fail because the file handle is nil.
	err = al.Log(AuditEntry{Operation: "post_close", Provider: "test", Result: "accepted"})
	require.Error(t, err)
}
