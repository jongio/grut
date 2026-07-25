package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jongio/grut/internal/crashlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_EmptyString(t *testing.T) {
	assert.Equal(t, "", truncate("", 10))
}

func TestTruncate_ShorterThanMax(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
}

func TestTruncate_ExactlyMax(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 5))
}

func TestTruncate_LongerThanMax(t *testing.T) {
	assert.Equal(t, "hel", truncate("hello world", 3))
}

func TestTruncate_MultibyteRunes(t *testing.T) {
	// Each Chinese character is one rune but multi-byte in UTF-8.
	assert.Equal(t, "中文", truncate("中文测试数据", 2))
}

func TestTruncate_MaxRunesZero(t *testing.T) {
	assert.Equal(t, "", truncate("hello", 0))
}

func TestTruncate_MaxRunesOne(t *testing.T) {
	assert.Equal(t, "h", truncate("hello", 1))
}

func TestTruncate_EmojiRunes(t *testing.T) {
	// Emoji are typically one rune.
	assert.Equal(t, "🎉🚀", truncate("🎉🚀🔥💡", 2))
}

func TestTruncate_PreservesOriginalWhenShorter(t *testing.T) {
	// Verify original string is returned (not a copy) when within limit.
	s := "short"
	result := truncate(s, 100)
	assert.Equal(t, s, result)
}

// ---------------------------------------------------------------------------
// buildIssueURL
// ---------------------------------------------------------------------------

func TestBuildIssueURL_StartsWithHTTPS(t *testing.T) {
	r := &crashlog.CrashReport{
		ID:         "test-id-123",
		Timestamp:  time.Now(),
		Version:    "1.0.0",
		PanicValue: "runtime error: index out of range",
		StackTrace: "goroutine 1 [running]:\nmain.main()",
	}
	u := buildIssueURL(r)
	assert.True(t, strings.HasPrefix(u, "https://"), "URL must start with https://")
}

func TestBuildIssueURL_ContainsGitHubRepo(t *testing.T) {
	r := &crashlog.CrashReport{
		ID:         "abc",
		Version:    "1.0.0",
		PanicValue: "nil pointer dereference",
	}
	u := buildIssueURL(r)
	assert.Contains(t, u, "github.com")
	assert.Contains(t, u, "jongio/grut")
}

func TestBuildIssueURL_ContainsRequiredQueryParams(t *testing.T) {
	r := &crashlog.CrashReport{
		ID:         "def",
		Version:    "2.0.0",
		PanicValue: "test panic",
	}
	u := buildIssueURL(r)
	assert.Contains(t, u, "title=")
	assert.Contains(t, u, "body=")
	assert.Contains(t, u, "labels=")
}

func TestBuildIssueURL_EncodesSpecialCharacters(t *testing.T) {
	r := &crashlog.CrashReport{
		PanicValue: "error with spaces & special=chars",
		Version:    "1.0.0",
	}
	u := buildIssueURL(r)
	// url.QueryEscape converts spaces to "+", so no literal spaces in URL.
	assert.NotContains(t, u, " ", "spaces should be URL-encoded")
}

func TestBuildIssueURL_ContainsCrashLabel(t *testing.T) {
	r := &crashlog.CrashReport{
		PanicValue: "boom",
		Version:    "1.0.0",
	}
	u := buildIssueURL(r)
	assert.Contains(t, u, "labels=crash")
}

func TestBuildIssueURL_IncludesVersionInTitle(t *testing.T) {
	r := &crashlog.CrashReport{
		PanicValue: "fault",
		Version:    "3.5.7",
	}
	u := buildIssueURL(r)
	// The title contains the version via FormatGitHubIssueTitle.
	assert.Contains(t, u, "3.5.7")
}

func TestBuildIssueURL_EmptyReport(t *testing.T) {
	r := &crashlog.CrashReport{}
	u := buildIssueURL(r)
	// Should still produce a valid URL even with empty fields.
	assert.True(t, strings.HasPrefix(u, "https://"))
	assert.Contains(t, u, "title=")
}

// ---------------------------------------------------------------------------
// newReportCmd — flag registration
// ---------------------------------------------------------------------------

func TestNewReportCmd_FlagsRegistered(t *testing.T) {
	cmd := newReportCmd()

	tests := []struct {
		name     string
		flagType string
	}{
		{"latest", "bool"},
		{"list", "bool"},
		{"show", "string"},
		{"clear", "bool"},
		{"no-browser", "bool"},
		{"json", "bool"},
	}

	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.name)
		if assert.NotNil(t, f, "flag --%s must be registered", tt.name) {
			assert.Equal(t, tt.flagType, f.Value.Type(), "flag --%s type mismatch", tt.name)
		}
	}
}

func TestNewReportCmd_UseAndShort(t *testing.T) {
	cmd := newReportCmd()
	assert.Equal(t, "report", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

// ---------------------------------------------------------------------------
// runReport — flag dispatch integration
// ---------------------------------------------------------------------------

func TestRunReport_ListFlag_NoReports(t *testing.T) {
	// When no crash reports exist, --list should succeed and print
	// "No crash reports found."
	cmd := newReportCmd()
	cmd.SetArgs([]string{"--list"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestRunReport_ClearFlag_NoReports(t *testing.T) {
	// When no crash reports exist, --clear should succeed.
	cmd := newReportCmd()
	cmd.SetArgs([]string{"--clear"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestRunReport_ShowFlag_NonexistentID(t *testing.T) {
	// --show with a nonexistent ID should return an error.
	cmd := newReportCmd()
	cmd.SetArgs([]string{"--show", "nonexistent-id-12345"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestFindCrashReport_ExactID(t *testing.T) {
	reports := []*crashlog.CrashReport{
		{ID: "123456789"},
		{ID: "123400000"},
	}

	report, err := findCrashReport("123456789", reports)

	require.NoError(t, err)
	assert.Equal(t, "123456789", report.ID)
}

func TestFindCrashReport_UnambiguousPrefix(t *testing.T) {
	reports := []*crashlog.CrashReport{
		{ID: "123456789"},
		{ID: "987654321"},
	}

	report, err := findCrashReport("1234", reports)

	require.NoError(t, err)
	assert.Equal(t, "123456789", report.ID)
}

func TestFindCrashReport_AmbiguousPrefix(t *testing.T) {
	reports := []*crashlog.CrashReport{
		{ID: "123456789"},
		{ID: "123400000"},
	}

	_, err := findCrashReport("1234", reports)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "matches 2 reports")
}

func TestFindCrashReport_MissingPrefix(t *testing.T) {
	reports := []*crashlog.CrashReport{{ID: "123456789"}}

	_, err := findCrashReport("999", reports)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunReport_JSONWithoutListReturnsError(t *testing.T) {
	cmd := newReportCmd()
	cmd.SetArgs([]string{"--json"})

	err := cmd.Execute()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--json can only be used with --list")
}

func TestWriteCrashReportListJSON_Empty(t *testing.T) {
	var out bytes.Buffer

	err := writeCrashReportListJSON(&out, nil)

	assert.NoError(t, err)
	assert.JSONEq(t, `[]`, out.String())
}

func TestWriteCrashReportListJSON_SummaryFields(t *testing.T) {
	when := time.Date(2026, 7, 11, 18, 30, 0, 0, time.UTC)
	reports := []*crashlog.CrashReport{{
		ID:         "crash-1",
		Timestamp:  when,
		Version:    "1.2.3",
		GoVersion:  "go1.26.5",
		OS:         "windows",
		Arch:       "amd64",
		Terminal:   "Windows Terminal",
		PanicValue: "nil pointer",
		Context:    "preview render",
		StackTrace: "full stack stays out of the summary",
	}}
	var out bytes.Buffer

	err := writeCrashReportListJSON(&out, reports)

	require.NoError(t, err)
	var got []crashReportSummary
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "crash-1", got[0].ID)
	assert.Equal(t, "2026-07-11T18:30:00Z", got[0].Timestamp)
	assert.Equal(t, "nil pointer", got[0].PanicValue)
	assert.Equal(t, "preview render", got[0].Context)
}

func TestRunReport_LatestDefault_NoReports(t *testing.T) {
	// Default behavior (no flags) when no crash reports exist should
	// print "No crash reports found." and succeed.
	cmd := newReportCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestRunReport_LatestWithNoBrowser_NoReports(t *testing.T) {
	// --latest --no-browser when no crash reports exist.
	cmd := newReportCmd()
	cmd.SetArgs([]string{"--latest", "--no-browser"})
	err := cmd.Execute()
	assert.NoError(t, err)
}
