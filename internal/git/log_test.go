package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogOutput(t *testing.T) {
	const sep = "\x1e"
	// Simulate one commit with all fields.
	// Records are terminated by \x1f (US) as produced by the format string.
	input := "abc123def456" + sep +
		"abc123d" + sep +
		"John Doe" + sep +
		"john@example.com" + sep +
		"2024-01-15T10:30:00Z" + sep +
		"Add feature X" + sep +
		"Detailed body text" + sep +
		"parent1 parent2" + sep +
		"HEAD -> main, origin/main" + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1)

	c := commits[0]
	assert.Equal(t, "abc123def456", c.Hash)
	assert.Equal(t, "abc123d", c.ShortHash)
	assert.Equal(t, "John Doe", c.Author)
	assert.Equal(t, "john@example.com", c.AuthorEmail)
	assert.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), c.Date)
	assert.Equal(t, "Add feature X", c.Subject)
	assert.Equal(t, "Detailed body text", c.Body)
	assert.Equal(t, []string{"parent1", "parent2"}, c.Parents)
	assert.Equal(t, []string{"HEAD -> main", "origin/main"}, c.Refs)
}

func TestParseLogOutput_MultipleCommits(t *testing.T) {
	const sep = "\x1e"
	record1 := "hash1" + sep + "h1" + sep + "Alice" + sep + "alice@test.com" + sep +
		"2024-01-15T10:30:00Z" + sep + "First commit" + sep + "" + sep + "" + sep + ""
	record2 := "hash2" + sep + "h2" + sep + "Bob" + sep + "bob@test.com" + sep +
		"2024-01-14T09:00:00Z" + sep + "Second commit" + sep + "" + sep + "hash1" + sep + ""
	input := record1 + "\x1f\n" + record2 + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 2)

	assert.Equal(t, "hash1", commits[0].Hash)
	assert.Equal(t, "hash2", commits[1].Hash)
	assert.Empty(t, commits[0].Parents)
	assert.Equal(t, []string{"hash1"}, commits[1].Parents)
}

func TestParseLogOutput_Empty(t *testing.T) {
	commits, err := parseLogOutput("", "\x1e")
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestParseLogOutput_NoBody(t *testing.T) {
	const sep = "\x1e"
	input := "hash1" + sep + "h1" + sep + "Dev" + sep + "dev@test.com" + sep +
		"2024-03-01T12:00:00Z" + sep + "Quick fix" + sep + "" + sep + "" + sep + "" + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "", commits[0].Body)
}

func TestParseLogOutput_MalformedRecordSkipped(t *testing.T) {
	const sep = "\x1e"
	// A malformed record with fewer than 9 fields should be silently skipped.
	malformed := "only" + sep + "three" + sep + "fields"
	valid := "hash1" + sep + "h1" + sep + "Dev" + sep + "dev@test.com" + sep +
		"2024-03-01T12:00:00Z" + sep + "Valid commit" + sep + "" + sep + "" + sep + ""
	input := malformed + "\x1f\n" + valid + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1, "malformed record should be skipped")
	assert.Equal(t, "Valid commit", commits[0].Subject)
}

func TestParseLogOutput_InvalidDate(t *testing.T) {
	const sep = "\x1e"
	input := "hash1" + sep + "h1" + sep + "Dev" + sep + "dev@test.com" + sep +
		"not-a-date" + sep + "Subject" + sep + "" + sep + "" + sep + "" + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.True(t, commits[0].Date.IsZero(), "invalid date should produce zero time")
}

func TestParseLogOutput_WhitespaceOnlyInput(t *testing.T) {
	commits, err := parseLogOutput("   \n\t  ", "\x1e")
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestParseLogOutput_MultiLineBody(t *testing.T) {
	const sep = "\x1e"
	body := "Line 1 of body\nLine 2 of body\n\nLine 4 after blank"
	input := "hash1" + sep + "h1" + sep + "Dev" + sep + "dev@test.com" + sep +
		"2024-06-01T08:00:00Z" + sep + "feat: multi-line" + sep + body + sep + "" + sep + "" + "\x1f\n"

	commits, err := parseLogOutput(input, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, body, commits[0].Body)
}

// ---------------------------------------------------------------------------
// Log() option validation — injection prevention via ValidateArg/Ref/Path
// ---------------------------------------------------------------------------

func TestLogOpts_SinceValidation(t *testing.T) {
	// Verify that shell metacharacters in Since are rejected.
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Since: "2024-01-01;rm -rf /"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log since")
}

func TestLogOpts_UntilValidation(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Until: "2024-01-01|cat /etc/passwd"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log until")
}

func TestLogOpts_AuthorValidation(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Author: "alice$(whoami)"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log author")
}

func TestLogOpts_GrepValidation(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Grep: "fix&echo pwned"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log grep")
}

func TestLogOpts_RefValidation(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Ref: "--upload-pack=evil"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log ref")
}

func TestLogOpts_PathValidation(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)
	_, err = c.Log(t.Context(), LogOpts{Path: "file;rm -rf /"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log path")
}
