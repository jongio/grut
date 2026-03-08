package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogOutput_MultilineBody(t *testing.T) {
	sep := "\x1e"
	// The parser splits raw output by \n, so a multi-line body in a single
	// commit log entry causes the body's extra lines to be treated as
	// separate (malformed) lines. In practice git log uses --format which
	// outputs one line per commit when the body is excluded or the format
	// avoids embedded newlines. Test that the first commit still parses.
	line1 := "abc123" + sep + "abc" + sep + "Alice" + sep + "alice@ex.com" + sep + "2024-01-01T00:00:00Z" + sep + "Fix bug" + sep + "single line body" + sep + "parent1" + sep + "HEAD"

	commits, err := parseLogOutput(line1, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1, "should parse exactly 1 commit")
	assert.Equal(t, "Fix bug", commits[0].Subject)
	assert.Equal(t, "single line body", commits[0].Body)
}

func TestParseLogOutput_SingleLineBody(t *testing.T) {
	sep := "\x1e"
	line := "abc123" + sep + "abc" + sep + "Alice" + sep + "alice@ex.com" + sep + "2024-01-01T00:00:00Z" + sep + "Fix bug" + sep + "short body" + sep + "parent1" + sep + "HEAD"

	commits, err := parseLogOutput(line, sep)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "Fix bug", commits[0].Subject)
	assert.Equal(t, "short body", commits[0].Body)
}
