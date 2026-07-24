package git

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandLogRecordAndEntriesCopy(t *testing.T) {
	t.Parallel()
	log := NewCommandLog(10)

	log.Record(CommandEntry{
		Timestamp: time.Unix(1, 0),
		Args:      []string{"status", "--short"},
		Dir:       "repo",
		Duration:  time.Millisecond,
		Success:   true,
	})

	entries := log.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"status", "--short"}, entries[0].Args)

	entries[0].Args[0] = "mutated"
	entries[0].Dir = "mutated"

	entries = log.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"status", "--short"}, entries[0].Args)
	assert.Equal(t, "repo", entries[0].Dir)
}

func TestCommandLogRedactsSensitiveArgs(t *testing.T) {
	t.Parallel()
	log := NewCommandLog(10)

	log.Record(CommandEntry{
		Args: []string{
			"remote",
			"set-url",
			"origin",
			"https://user:token@example.com/owner/repo.git",
			"https://user@example.com/owner/repo.git",
			"ghp_abcdefghijklmnopqrstuvwxyz",
			"token=super-secret",
		},
		Success: true,
	})

	entries := log.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "https://***@example.com/owner/repo.git", entries[0].Args[3])
	assert.Equal(t, "https://***@example.com/owner/repo.git", entries[0].Args[4])
	assert.Equal(t, "***", entries[0].Args[5])
	assert.Equal(t, "token=***", entries[0].Args[6])
}

func TestCommandLogTrimsOldestEntries(t *testing.T) {
	t.Parallel()
	log := NewCommandLog(3)

	for i := range 5 {
		log.Record(CommandEntry{Args: []string{fmt.Sprintf("cmd-%d", i)}, Success: true})
	}

	entries := log.Entries()
	require.Len(t, entries, 3)
	assert.Equal(t, "cmd-2", entries[0].Args[0])
	assert.Equal(t, "cmd-4", entries[2].Args[0])
	assert.Equal(t, 3, log.Len())
}

func TestCommandLogRecordsFailedEntry(t *testing.T) {
	t.Parallel()
	log := NewCommandLog(10)

	log.Record(CommandEntry{
		Args:       []string{"rev-parse", "--bad"},
		Success:    false,
		ErrSummary: summarizeCommandError("fatal: bad revision\nmore detail", nil),
	})

	entries := log.Entries()
	require.Len(t, entries, 1)
	assert.False(t, entries[0].Success)
	assert.Equal(t, "fatal: bad revision", entries[0].ErrSummary)
}

func TestSummarizeCommandErrorRedactsCredentials(t *testing.T) {
	got := summarizeCommandError(
		"fatal: unable to access 'https://user:ghp_secrettoken@github.com/o/r.git/': failed",
		nil,
	)
	assert.NotContains(t, got, "ghp_secrettoken")
	assert.NotContains(t, got, "user:")
	assert.Contains(t, got, "https://***@github.com")
}

func TestCommandLogConcurrentRecord(t *testing.T) {
	t.Parallel()
	log := NewCommandLog(100)
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Record(CommandEntry{Args: []string{fmt.Sprintf("cmd-%d", i)}, Success: true})
		}()
	}
	wg.Wait()

	assert.Equal(t, 50, log.Len())
}
