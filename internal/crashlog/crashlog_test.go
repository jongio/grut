package crashlog

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestDataHome overrides xdg.DataHome so that crashDir() points at a
// temp directory scoped to the current test. The original value is
// restored via t.Cleanup.
func setTestDataHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	orig := xdg.DataHome
	xdg.DataHome = tmp
	t.Cleanup(func() { xdg.DataHome = orig })
	return tmp
}

// ---------------------------------------------------------------------------
// ScrubPII
// ---------------------------------------------------------------------------

func TestScrubPII(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	input := home + "/secrets/file.txt"
	got := ScrubPII(input)
	assert.Contains(t, got, "~")
	assert.NotContains(t, got, home)
}

func TestScrubPII_HTTPSCredentials(t *testing.T) {
	input := "https://user:pass@github.com/repo"
	got := ScrubPII(input)
	assert.NotContains(t, got, "user:pass@")
	assert.Contains(t, got, "https://")
}

func TestScrubPII_TokenRedaction(t *testing.T) {
	input := "token=ghp_abc123secret"
	got := ScrubPII(input)
	assert.Contains(t, got, "[REDACTED]")
	assert.NotContains(t, got, "ghp_abc123secret")
}

func TestScrubPII_NoHomeDir(t *testing.T) {
	// A string that does not contain the home directory should pass through
	// with only token/credential scrubbing applied.
	got := ScrubPII("/some/other/path")
	assert.Equal(t, "/some/other/path", got)
}

// ---------------------------------------------------------------------------
// NewReport
// ---------------------------------------------------------------------------

func TestNewReport(t *testing.T) {
	r := NewReport("boom", []byte("fake stack"), "test-context")

	assert.NotEmpty(t, r.ID)
	assert.False(t, r.Timestamp.IsZero())
	assert.Equal(t, runtime.Version(), r.GoVersion)
	assert.Equal(t, runtime.GOOS, r.OS)
	assert.Equal(t, runtime.GOARCH, r.Arch)
	assert.Equal(t, "boom", r.PanicValue)
	assert.Equal(t, "test-context", r.Context)
	assert.Contains(t, r.StackTrace, "fake stack")
}

// ---------------------------------------------------------------------------
// FormatGitHubIssueTitle
// ---------------------------------------------------------------------------

func TestFormatGitHubIssueTitle(t *testing.T) {
	r := &CrashReport{PanicValue: "nil pointer", Version: "1.2.3"}
	title := FormatGitHubIssueTitle(r)
	assert.Equal(t, "crash: nil pointer (v1.2.3)", title)
}

func TestFormatGitHubIssueTitle_LongPanic(t *testing.T) {
	long := strings.Repeat("x", 120)
	r := &CrashReport{PanicValue: long, Version: "0.1.0"}
	title := FormatGitHubIssueTitle(r)

	// The panic value in the title should be truncated to maxPanicLen (80).
	assert.True(t, len(title) < len(long),
		"title should be shorter than the full panic value")
	assert.Contains(t, title, strings.Repeat("x", 80))
	assert.NotContains(t, title, strings.Repeat("x", 81))
}

// ---------------------------------------------------------------------------
// FormatGitHubIssueBody
// ---------------------------------------------------------------------------

func TestFormatGitHubIssueBody(t *testing.T) {
	r := &CrashReport{
		Version:    "1.0.0",
		GoVersion:  "go1.22.0",
		OS:         "linux",
		Arch:       "amd64",
		Terminal:   "iTerm2",
		Timestamp:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		PanicValue: "test panic",
		StackTrace: "goroutine 1 [running]:\nmain.main()",
		Context:    "top-level",
		LogTail:    []string{"[INFO] starting", "[ERROR] boom"},
	}

	body := FormatGitHubIssueBody(r)
	assert.Contains(t, body, "### Crash Report")
	assert.Contains(t, body, "**Version:** 1.0.0")
	assert.Contains(t, body, "**Go:** go1.22.0")
	assert.Contains(t, body, "**OS/Arch:** linux/amd64")
	assert.Contains(t, body, "### Panic")
	assert.Contains(t, body, "test panic")
	assert.Contains(t, body, "### Stack Trace")
	assert.Contains(t, body, "### Context")
	assert.Contains(t, body, "### Recent Log Entries")
	assert.Contains(t, body, "[ERROR] boom")
}

// ---------------------------------------------------------------------------
// Write / Read / List / Clear / PruneOld
// ---------------------------------------------------------------------------

func TestWriteAndRead(t *testing.T) {
	setTestDataHome(t)

	r := NewReport("write-read panic", []byte("stack"), "test")
	path, err := Write(r)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	got, err := Read(r.ID)
	require.NoError(t, err)
	assert.Equal(t, r.PanicValue, got.PanicValue)
	assert.Equal(t, r.ID, got.ID)
}

func TestWriteAndList(t *testing.T) {
	setTestDataHome(t)

	// Write two reports with distinct timestamps so filenames differ.
	r1 := NewReport("first", []byte("stack1"), "ctx1")
	r1.Timestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	r1.ID = "1000000001"
	_, err := Write(r1)
	require.NoError(t, err)

	r2 := NewReport("second", []byte("stack2"), "ctx2")
	r2.Timestamp = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	r2.ID = "2000000002"
	_, err = Write(r2)
	require.NoError(t, err)

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 2)

	// Newest first.
	assert.Equal(t, "second", reports[0].PanicValue)
	assert.Equal(t, "first", reports[1].PanicValue)
}

func TestClear(t *testing.T) {
	setTestDataHome(t)

	r := NewReport("clear-test", []byte("stack"), "ctx")
	_, err := Write(r)
	require.NoError(t, err)

	count, err := Clear()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	reports, err := List()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestPruneOld(t *testing.T) {
	setTestDataHome(t)

	// Write more reports than the retention limit.
	for i := 0; i < maxCrashFiles+5; i++ {
		r := NewReport(fmt.Sprintf("panic-%d", i), []byte("stack"), "ctx")
		r.Timestamp = time.Date(2024, 1, 1, 0, 0, i, 0, time.UTC)
		r.ID = fmt.Sprintf("%020d", i)
		_, err := Write(r)
		require.NoError(t, err)
	}

	reports, err := List()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(reports), maxCrashFiles)
}

// ---------------------------------------------------------------------------
// LogTailHandler
// ---------------------------------------------------------------------------

func TestLogTailHandler(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)
	logger := slog.New(tail)

	logger.Info("entry-1")
	logger.Info("entry-2")
	logger.Warn("entry-3")

	entries := tail.Entries()
	require.Len(t, entries, 3)
	assert.Contains(t, entries[0], "entry-1")
	assert.Contains(t, entries[1], "entry-2")
	assert.Contains(t, entries[2], "entry-3")
}

func TestLogTailHandler_RingWrap(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 3)
	logger := slog.New(tail)

	// Write 5 entries into a buffer of size 3.
	for i := 1; i <= 5; i++ {
		logger.Info(fmt.Sprintf("msg-%d", i))
	}

	entries := tail.Entries()
	require.Len(t, entries, 3)

	// Should contain only the last 3 entries in order.
	assert.Contains(t, entries[0], "msg-3")
	assert.Contains(t, entries[1], "msg-4")
	assert.Contains(t, entries[2], "msg-5")
}

// ---------------------------------------------------------------------------
// CollectDiagnostics
// ---------------------------------------------------------------------------

func TestCollectDiagnostics(t *testing.T) {
	diag := CollectDiagnostics()

	expectedKeys := []string{
		"go_version", "os", "arch", "num_cpu",
		"num_goroutine", "terminal", "version", "compiler",
	}
	for _, key := range expectedKeys {
		_, ok := diag[key]
		assert.True(t, ok, "expected key %q in diagnostics", key)
	}

	assert.Equal(t, runtime.Version(), diag["go_version"])
	assert.Equal(t, runtime.GOOS, diag["os"])
	assert.Equal(t, runtime.GOARCH, diag["arch"])
}

// ---------------------------------------------------------------------------
// ScrubPII - additional edge cases
// ---------------------------------------------------------------------------

func TestScrubPII_ColonSeparator(t *testing.T) {
	// Verify colon separator is preserved (not replaced with equals).
	input := "password:hunter2"
	got := ScrubPII(input)
	assert.Contains(t, got, "password:[REDACTED]")
	assert.NotContains(t, got, "hunter2")
}

func TestScrubPII_MultipleTokens(t *testing.T) {
	input := "token=abc123 secret:xyz key=foo"
	got := ScrubPII(input)
	assert.NotContains(t, got, "abc123")
	assert.NotContains(t, got, "xyz")
	assert.NotContains(t, got, "foo")
	assert.Equal(t, 3, strings.Count(got, "[REDACTED]"))
}

func TestScrubPII_EmptyString(t *testing.T) {
	got := ScrubPII("")
	assert.Equal(t, "", got)
}

// ---------------------------------------------------------------------------
// scrubLogTail
// ---------------------------------------------------------------------------

func TestScrubLogTail_Nil(t *testing.T) {
	got := scrubLogTail(nil)
	assert.Nil(t, got)
}

func TestScrubLogTail_WithPII(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	entries := []string{
		"[INFO] loaded " + home + "/config.toml",
		"[ERROR] token=secret123 failed",
	}
	got := scrubLogTail(entries)
	require.Len(t, got, 2)
	assert.NotContains(t, got[0], home)
	assert.Contains(t, got[0], "~")
	assert.NotContains(t, got[1], "secret123")
	assert.Contains(t, got[1], "[REDACTED]")
}

// ---------------------------------------------------------------------------
// LogTailHandler - additional coverage
// ---------------------------------------------------------------------------

func TestNewLogTailHandler_DefaultSize(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, nil)
	tail := NewLogTailHandler(inner, 0)
	// Should use defaultTailSize (50).
	assert.Equal(t, defaultTailSize, tail.buf.size)
}

func TestNewLogTailHandler_NegativeSize(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, nil)
	tail := NewLogTailHandler(inner, -5)
	assert.Equal(t, defaultTailSize, tail.buf.size)
}

func TestLogTailHandler_WithAttrs(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)

	derived := tail.WithAttrs([]slog.Attr{slog.String("component", "test")})
	logger := slog.New(derived)
	logger.Info("attr-entry", "extra", "val")

	// The entry should appear in the shared ring buffer.
	entries := tail.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], "attr-entry")
}

func TestLogTailHandler_WithGroup(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)

	derived := tail.WithGroup("mygroup")
	logger := slog.New(derived)
	logger.Info("group-entry")

	// The entry should appear in the shared ring buffer.
	entries := tail.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], "group-entry")
}

func TestLogTailHandler_ConcurrentWrites(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 100)
	logger := slog.New(tail)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info(fmt.Sprintf("concurrent-%d", n))
		}(i)
	}
	wg.Wait()

	entries := tail.Entries()
	assert.Len(t, entries, 50)
}

func TestLogTailHandler_WithAttrsSharedBuffer(t *testing.T) {
	// Verify that WithAttrs-derived handlers share the ring buffer,
	// preventing the old data-race where separate mutexes guarded
	// the same slice.
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)

	derived := tail.WithAttrs([]slog.Attr{slog.String("k", "v")})

	origLogger := slog.New(tail)
	derivedLogger := slog.New(derived)

	origLogger.Info("from-original")
	derivedLogger.Info("from-derived")

	entries := tail.Entries()
	require.Len(t, entries, 2)
	assert.Contains(t, entries[0], "from-original")
	assert.Contains(t, entries[1], "from-derived")
}

func TestFormatRecord_WithAttrs(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	r.AddAttrs(slog.String("key1", "val1"), slog.Int("key2", 42))
	got := formatRecord(r)
	assert.Contains(t, got, "[INFO] hello")
	assert.Contains(t, got, "key1=val1")
	assert.Contains(t, got, "key2=42")
}

// ---------------------------------------------------------------------------
// SetDefaultLogTail / DefaultLogTail
// ---------------------------------------------------------------------------

func TestSetDefaultLogTail_AndDefaultLogTail(t *testing.T) {
	// Clear any previous default.
	SetDefaultLogTail(nil)

	// nil handler should return nil entries.
	assert.Nil(t, DefaultLogTail())

	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)
	logger := slog.New(tail)
	logger.Info("default-test")

	SetDefaultLogTail(tail)
	entries := DefaultLogTail()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], "default-test")

	// Clean up: restore nil.
	SetDefaultLogTail(nil)
}

// ---------------------------------------------------------------------------
// WriteRecovery
// ---------------------------------------------------------------------------

func TestWriteRecovery_StringPanic(t *testing.T) {
	setTestDataHome(t)

	WriteRecovery("test panic value", "test-context", nil)

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "test panic value", reports[0].PanicValue)
	assert.Equal(t, "test-context", reports[0].Context)
}

func TestWriteRecovery_ErrorPanic(t *testing.T) {
	setTestDataHome(t)

	WriteRecovery(fmt.Errorf("something broke"), "error-ctx", nil)

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].PanicValue, "something broke")
}

func TestWriteRecovery_IntPanic(t *testing.T) {
	setTestDataHome(t)

	WriteRecovery(42, "int-ctx", nil)

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "42", reports[0].PanicValue)
}

func TestWriteRecovery_WithLogTail(t *testing.T) {
	setTestDataHome(t)

	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := NewLogTailHandler(inner, 10)
	logger := slog.New(tail)
	logger.Info("pre-crash-log")

	WriteRecovery("boom", "tail-ctx", tail)

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.NotEmpty(t, reports[0].LogTail)
	assert.Contains(t, reports[0].LogTail[0], "pre-crash-log")
}

// ---------------------------------------------------------------------------
// RecoverAndReport
// ---------------------------------------------------------------------------

func TestRecoverAndReport_NoPanic(t *testing.T) {
	setTestDataHome(t)

	// When there is no panic, RecoverAndReport should be a no-op.
	func() {
		defer RecoverAndReport("no-panic-ctx", nil)
	}()

	reports, err := List()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestRecoverAndReport_WithPanic(t *testing.T) {
	setTestDataHome(t)

	func() {
		defer RecoverAndReport("recover-ctx", nil)
		panic("deliberate test panic")
	}()

	reports, err := List()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Contains(t, reports[0].PanicValue, "deliberate test panic")
	assert.Equal(t, "recover-ctx", reports[0].Context)
}

// ---------------------------------------------------------------------------
// Reader edge cases
// ---------------------------------------------------------------------------

func TestList_EmptyDirectory(t *testing.T) {
	setTestDataHome(t)

	reports, err := List()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestList_MalformedJSON(t *testing.T) {
	setTestDataHome(t)

	// Write a valid report first.
	r := NewReport("valid", []byte("stack"), "ctx")
	_, err := Write(r)
	require.NoError(t, err)

	// Write a malformed file directly.
	dir := crashDir()
	badPath := filepath.Join(dir, "crash-99999999-999999-bad.json")
	require.NoError(t, os.WriteFile(badPath, []byte("{invalid json"), 0o644))

	reports, err := List()
	require.NoError(t, err)
	// The valid report should be returned; the malformed one skipped.
	assert.Len(t, reports, 1)
	assert.Equal(t, "valid", reports[0].PanicValue)
}

func TestRead_NotFound(t *testing.T) {
	setTestDataHome(t)

	// Write a report so the directory exists.
	r := NewReport("exists", []byte("stack"), "ctx")
	_, err := Write(r)
	require.NoError(t, err)

	_, err = Read("nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRead_NoDirectory(t *testing.T) {
	setTestDataHome(t)

	_, err := Read("any-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no crash reports found")
}

func TestClear_EmptyDirectory(t *testing.T) {
	setTestDataHome(t)

	count, err := Clear()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// FormatGitHubIssueBody edge cases
// ---------------------------------------------------------------------------

func TestFormatGitHubIssueBody_EmptyFields(t *testing.T) {
	r := &CrashReport{}
	body := FormatGitHubIssueBody(r)
	assert.Contains(t, body, "### Crash Report")
	assert.Contains(t, body, "### Panic")
	assert.Contains(t, body, "### Stack Trace")
}

func TestFormatGitHubIssueBody_LongLogTail(t *testing.T) {
	tail := make([]string, 50)
	for i := range tail {
		tail[i] = fmt.Sprintf("log-%d", i)
	}
	r := &CrashReport{LogTail: tail}
	body := FormatGitHubIssueBody(r)
	// Should contain the last maxLogTailLines (20) entries.
	assert.Contains(t, body, "log-49")
	assert.Contains(t, body, "log-30")
	assert.NotContains(t, body, "log-29")
}

func TestFormatGitHubIssueBody_LongStackTrace(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("frame-%d", i)
	}
	r := &CrashReport{StackTrace: strings.Join(lines, "\n")}
	body := FormatGitHubIssueBody(r)
	// Should contain first maxStackLines (40) lines only.
	assert.Contains(t, body, "frame-0")
	assert.Contains(t, body, "frame-39")
	assert.NotContains(t, body, "frame-40")
}

// ---------------------------------------------------------------------------
// truncateLines
// ---------------------------------------------------------------------------

func TestTruncateLines_ShortString(t *testing.T) {
	s := "line1\nline2"
	got := truncateLines(s, 10)
	assert.Equal(t, s, got)
}

func TestTruncateLines_ExactLimit(t *testing.T) {
	s := "a\nb\nc"
	got := truncateLines(s, 3)
	assert.Equal(t, s, got)
}

// ---------------------------------------------------------------------------
// Writer - file naming
// ---------------------------------------------------------------------------

func TestWrite_FileNaming(t *testing.T) {
	setTestDataHome(t)

	r := NewReport("naming-test", []byte("stack"), "ctx")
	path, err := Write(r)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(filepath.Base(path), "crash-"))
	assert.True(t, strings.HasSuffix(filepath.Base(path), ".json"))

	// The file should contain valid JSON.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded CrashReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, r.ID, decoded.ID)
}
