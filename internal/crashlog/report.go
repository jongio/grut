// Package crashlog captures, stores, and formats crash reports for grut.
// Reports are scrubbed of personally identifiable information before
// being written to disk or formatted for GitHub issue submission.
package crashlog

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jongio/grut/internal/config"
)

// CrashReport holds all information captured when grut crashes.
type CrashReport struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Version    string    `json:"version"`
	GoVersion  string    `json:"go_version"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Terminal   string    `json:"terminal"`
	PanicValue string    `json:"panic_value"`
	StackTrace string    `json:"stack_trace"`
	Context    string    `json:"context"`
	LogTail    []string  `json:"log_tail"`
}

// tokenPattern matches common secrets in key=value or key:value form,
// allowing optional whitespace around the delimiter.
var tokenPattern = regexp.MustCompile(`(?i)(token|key|secret|password|auth|credential)[\s]*([=:])\s*\S+`)

// embeddedCredPattern matches HTTPS URLs with embedded credentials.
var embeddedCredPattern = regexp.MustCompile(`https://[^@\s]+@`)

// standaloneTokenPattern matches known token prefixes that appear standalone
// (not necessarily in key=value form).
var standaloneTokenPattern = regexp.MustCompile(
	`(?:ghp_|gho_|ghs_|ghu_|github_pat_|glpat-|xox[bps]-|sk-|AKIA)[a-zA-Z0-9_\-]{10,}`)

// bearerPattern matches Authorization: Bearer and Basic header values.
var bearerPattern = regexp.MustCompile(`(?i)(Bearer|Basic)\s+[A-Za-z0-9+/=._\-]{10,}`)

// privateKeyPattern matches PEM private key blocks.
var privateKeyPattern = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// NewReport creates a CrashReport with auto-populated system information.
// The panicVal and stack come from the recover/debug.Stack calls, and
// context describes what grut was doing when the crash occurred.
func NewReport(panicVal any, stack []byte, context string) *CrashReport {
	now := time.Now().UTC()
	return &CrashReport{
		ID:         fmt.Sprintf("%d", now.UnixNano()),
		Timestamp:  now,
		Version:    config.AppVersion,
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Terminal:   os.Getenv("TERM_PROGRAM"),
		PanicValue: ScrubPII(fmt.Sprint(panicVal)),
		StackTrace: ScrubPII(string(stack)),
		Context:    ScrubPII(context),
		LogTail:    scrubLogTail(DefaultLogTail()),
	}
}

// scrubLogTail returns a copy of entries with PII removed from each line.
func scrubLogTail(entries []string) []string {
	if entries == nil {
		return nil
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = ScrubPII(e)
	}
	return out
}

// ScrubPII removes personally identifiable information from s.
// It replaces home directory paths with ~, strips embedded credentials
// from HTTPS URLs, and redacts common token/key/secret patterns.
func ScrubPII(s string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		// Replace forward-slash variant first (longer match wins).
		fwd := strings.ReplaceAll(home, `\`, "/")
		s = strings.ReplaceAll(s, fwd, "~")
		// Replace native (possibly backslash) variant.
		s = strings.ReplaceAll(s, home, "~")
	}

	s = embeddedCredPattern.ReplaceAllString(s, "https://")
	s = tokenPattern.ReplaceAllString(s, "${1}${2}[REDACTED]")
	s = standaloneTokenPattern.ReplaceAllString(s, "[REDACTED]")
	s = bearerPattern.ReplaceAllString(s, "${1} [REDACTED]")
	s = privateKeyPattern.ReplaceAllString(s, "-----BEGIN [REDACTED] PRIVATE KEY-----")

	return s
}

// maxPanicLen is the maximum length of PanicValue in an issue title.
const maxPanicLen = 80

// FormatGitHubIssueTitle returns a short title for a GitHub issue.
func FormatGitHubIssueTitle(r *CrashReport) string {
	pv := r.PanicValue
	if runes := []rune(pv); len(runes) > maxPanicLen {
		pv = string(runes[:maxPanicLen])
	}
	return fmt.Sprintf("crash: %s (v%s)", pv, r.Version)
}

// maxStackLines limits the stack trace length in a GitHub issue body.
const maxStackLines = 40

// maxLogTailLines limits the log tail length in a GitHub issue body.
const maxLogTailLines = 20

// FormatGitHubIssueBody returns the full markdown body for a GitHub issue.
func FormatGitHubIssueBody(r *CrashReport) string {
	stack := truncateLines(r.StackTrace, maxStackLines)
	tail := r.LogTail
	if len(tail) > maxLogTailLines {
		tail = tail[len(tail)-maxLogTailLines:]
	}

	var b strings.Builder
	b.WriteString("### Crash Report\n\n")
	fmt.Fprintf(&b, "**Version:** %s\n", r.Version)
	fmt.Fprintf(&b, "**Go:** %s\n", r.GoVersion)
	fmt.Fprintf(&b, "**OS/Arch:** %s/%s\n", r.OS, r.Arch)
	fmt.Fprintf(&b, "**Terminal:** %s\n", r.Terminal)
	fmt.Fprintf(&b, "**When:** %s\n", r.Timestamp.Format(time.RFC3339))
	b.WriteString("\n### Panic\n```\n")
	b.WriteString(r.PanicValue)
	b.WriteString("\n```\n")
	b.WriteString("\n### Stack Trace\n```\n")
	b.WriteString(stack)
	b.WriteString("\n```\n")
	b.WriteString("\n### Context\n")
	b.WriteString(r.Context)
	b.WriteString("\n")
	b.WriteString("\n### Recent Log Entries\n```\n")
	b.WriteString(strings.Join(tail, "\n"))
	b.WriteString("\n```\n")

	return b.String()
}

// truncateLines returns at most maxLines lines from s.
func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}
