package git

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultCommandLogMax = 500

var (
	globalCommandLog = NewCommandLog(defaultCommandLogMax)
	urlUserinfoRe    = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)([^/\s@]+@)([^/\s]+)`)
	tokenArgRe       = regexp.MustCompile(`(?i)^(gh[pousr]_[a-z0-9_]{20,}|github_pat_[a-z0-9_]+|glpat-[a-z0-9_-]{20,})$`)
	tokenKVArgRe     = regexp.MustCompile(`(?i)^((?:access_)?token|password|passwd|secret)=.+$`)
)

// CommandEntry describes one git command execution. Args are stored redacted and
// without the leading "git"; Entries returns them oldest-first.
type CommandEntry struct {
	Timestamp  time.Time
	Args       []string
	Dir        string
	Duration   time.Duration
	Success    bool
	ErrSummary string
}

// CommandLog stores a thread-safe bounded in-memory history of git commands.
type CommandLog struct {
	mu       sync.Mutex
	entries  []CommandEntry
	capacity int
}

// NewCommandLog creates a bounded command log. Non-positive max values retain
// entries in memory only until Record is called, then keep no entries.
func NewCommandLog(capacity int) *CommandLog {
	return &CommandLog{capacity: capacity}
}

// GlobalCommandLog returns the process-wide git command log shared by all git
// clients so the TUI can show one audit trail across panels.
func GlobalCommandLog() *CommandLog {
	return globalCommandLog
}

// Record appends an entry, trimming oldest entries when the log exceeds its cap.
func (l *CommandLog) Record(entry CommandEntry) {
	if l == nil || l.capacity <= 0 {
		return
	}
	entry.Args = redactArgs(entry.Args)
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.Args = append([]string(nil), entry.Args...)
	if len(l.entries) >= l.capacity {
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = entry
		return
	}
	l.entries = append(l.entries, entry)
}

// Entries returns a copy of entries in oldest-first order.
func (l *CommandLog) Entries() []CommandEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]CommandEntry, len(l.entries))
	for i, entry := range l.entries {
		entry.Args = append([]string(nil), entry.Args...)
		out[i] = entry
	}
	return out
}

// Len returns the number of retained entries.
func (l *CommandLog) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Clear removes all entries from the log.
func (l *CommandLog) Clear() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

func redactArgs(args []string) []string {
	redacted := make([]string, len(args))
	for i, arg := range args {
		redacted[i] = redactArg(arg)
	}
	return redacted
}

func redactArg(arg string) string {
	arg = urlUserinfoRe.ReplaceAllString(arg, `${1}***@${3}`)
	if tokenArgRe.MatchString(arg) {
		return "***"
	}
	if matches := tokenKVArgRe.FindStringSubmatch(arg); len(matches) == 2 {
		return matches[1] + "=***"
	}
	return arg
}

func summarizeCommandError(stderr string, err error) string {
	summary := strings.TrimSpace(stderr)
	if summary == "" && err != nil {
		summary = err.Error()
	}
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	summary = strings.ReplaceAll(summary, "\r", "\n")
	if before, _, ok := strings.Cut(summary, "\n"); ok {
		summary = before
	}
	if len(summary) > 240 {
		summary = summary[:240]
	}
	return strings.TrimSpace(summary)
}
