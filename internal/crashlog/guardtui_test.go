package crashlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGuardTUI_WritesReportAndRepanics verifies that a deferred GuardTUI
// recovers an in-flight panic, writes a crash report with the preserved panic
// value and stack, records the path via LastCrashPath, and then re-panics with
// the original value so the caller's framework can restore the terminal.
func TestGuardTUI_WritesReportAndRepanics(t *testing.T) {
	dir := setTestDataHome(t)

	var repanicked any
	func() {
		defer func() { repanicked = recover() }()
		defer GuardTUI("test.Update")
		panic("boom-from-guardtui")
	}()

	require.Equal(t, "boom-from-guardtui", repanicked, "GuardTUI must re-panic the original value")

	crashes := filepath.Join(dir, "grut", "crashes")
	entries, err := os.ReadDir(crashes)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one crash report should be written")

	data, err := os.ReadFile(filepath.Join(crashes, entries[0].Name()))
	require.NoError(t, err)
	var report CrashReport
	require.NoError(t, json.Unmarshal(data, &report))

	assert.Contains(t, report.PanicValue, "boom-from-guardtui")
	assert.Equal(t, "test.Update", report.Context)
	assert.Contains(t, report.StackTrace, "TestGuardTUI_WritesReportAndRepanics",
		"stack must point at the original panic site, not just the guard")
	assert.Contains(t, LastCrashPath(), "crash-", "LastCrashPath should reference the written report")
}

// TestGuardTUI_NoPanicIsNoop verifies GuardTUI does nothing (writes no report)
// when the guarded function returns normally.
func TestGuardTUI_NoPanicIsNoop(t *testing.T) {
	dir := setTestDataHome(t)

	func() {
		defer GuardTUI("test.Update")
		// no panic
	}()

	crashes := filepath.Join(dir, "grut", "crashes")
	_, err := os.ReadDir(crashes)
	assert.True(t, os.IsNotExist(err), "no crash directory should be created without a panic")
}
