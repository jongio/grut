// Package contributors extracts, deduplicates, and formats contributor
// information from git history for use in changelogs, release notes,
// and the CONTRIBUTORS.md hall of fame.
package contributors

import (
"bufio"
"fmt"
"os/exec"
"regexp"
"sort"
"strings"
"time"
)

// Contributor represents a single project contributor.
type Contributor struct {
Name         string
Email        string
CommitCount  int
IsFirstTime  bool
FirstCommit  time.Time
LatestCommit time.Time
}

// botPatterns identifies automated accounts to exclude from contributor lists.
var botPatterns = []string{
"[bot]",
"noreply@github.com",
"github-actions",
"dependabot",
"copilot-swe-agent",
"renovate",
"greenkeeper",
"snyk-bot",
}

// coAuthorRegex matches Co-authored-by trailers in commit messages.
// Format: Co-authored-by: Name <email>
var coAuthorRegex = regexp.MustCompile("(?i)^co-authored-by:\\s*(.+?)\\s*<([^>]+)>")

// gitRunner abstracts git command execution for testability.
type gitRunner func(repoDir string, args ...string) (string, error)

// defaultGitRunner shells out to the git binary.
func defaultGitRunner(repoDir string, args ...string) (string, error) {
cmd := exec.Command("git", args...)
cmd.Dir = repoDir
out, err := cmd.Output()
if err != nil {
if exitErr, ok := err.(*exec.ExitError); ok {
return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
}
return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}
return string(out), nil
}

// Options configures contributor extraction.
type Options struct {
// RepoDir is the repository root. Defaults to "." if empty.
RepoDir string

// FromRef is the starting ref (exclusive). If empty, includes all history.
FromRef string

// ToRef is the ending ref (inclusive). Defaults to "HEAD" if empty.
ToRef string

// gitRun overrides git execution for testing.
gitRun gitRunner
}

func (o *Options) runner() gitRunner {
if o.gitRun != nil {
return o.gitRun
}
return defaultGitRunner
}

func (o *Options) repoDir() string {
if o.RepoDir != "" {
return o.RepoDir
}
return "."
}

func (o *Options) refRange() string {
to := o.ToRef
if to == "" {
to = "HEAD"
}
if o.FromRef == "" {
return to
}
return o.FromRef + ".." + to
}

// Extract returns contributors for the given ref range.
// It parses commit authors AND Co-authored-by trailers, deduplicates by
// email, and filters out bot accounts.
func Extract(opts Options) ([]Contributor, error) {
run := opts.runner()
dir := opts.repoDir()
rng := opts.refRange()

// Get commit authors with dates.
authorOut, err := run(dir,
"log", rng,
"--format=%aN\x1e%aE\x1e%aI\x1f",
)
if err != nil {
return nil, fmt.Errorf("extract commit authors: %w", err)
}

// Get commit bodies to parse Co-authored-by trailers.
trailerOut, err := run(dir,
"log", rng,
"--format=%aI%n%b\x1f",
)
if err != nil {
return nil, fmt.Errorf("extract trailers: %w", err)
}

byEmail := make(map[string]*Contributor)

// Parse commit authors.
for _, record := range strings.Split(authorOut, "\x1f") {
record = strings.TrimSpace(record)
if record == "" {
continue
}
fields := strings.SplitN(record, "\x1e", 3)
if len(fields) < 3 {
continue
}
name := strings.TrimSpace(fields[0])
email := strings.TrimSpace(fields[1])
dateStr := strings.TrimSpace(fields[2])

if isBot(name, email) {
continue
}

date, _ := time.Parse(time.RFC3339, dateStr)
addContributor(byEmail, name, email, date)
}

// Parse Co-authored-by trailers.
for _, record := range strings.Split(trailerOut, "\x1f") {
record = strings.TrimSpace(record)
if record == "" {
continue
}
// First line is the commit date.
lines := strings.SplitN(record, "\n", 2)
dateStr := strings.TrimSpace(lines[0])
date, _ := time.Parse(time.RFC3339, dateStr)

if len(lines) < 2 {
continue
}
body := lines[1]

scanner := bufio.NewScanner(strings.NewReader(body))
for scanner.Scan() {
line := scanner.Text()
m := coAuthorRegex.FindStringSubmatch(line)
if m == nil {
continue
}
name := strings.TrimSpace(m[1])
email := strings.TrimSpace(m[2])
if isBot(name, email) {
continue
}
addContributor(byEmail, name, email, date)
}
}

contributors := make([]Contributor, 0, len(byEmail))
for _, c := range byEmail {
contributors = append(contributors, *c)
}

// Sort by commit count descending, then name ascending.
sort.Slice(contributors, func(i, j int) bool {
if contributors[i].CommitCount != contributors[j].CommitCount {
return contributors[i].CommitCount > contributors[j].CommitCount
}
return contributors[i].Name < contributors[j].Name
})

return contributors, nil
}

// addContributor upserts a contributor into the map, merging counts and dates.
func addContributor(m map[string]*Contributor, name, email string, date time.Time) {
key := strings.ToLower(email)
if c, ok := m[key]; ok {
c.CommitCount++
if !date.IsZero() && (c.FirstCommit.IsZero() || date.Before(c.FirstCommit)) {
c.FirstCommit = date
}
if date.After(c.LatestCommit) {
c.LatestCommit = date
}
} else {
m[key] = &Contributor{
Name:         name,
Email:        email,
CommitCount:  1,
FirstCommit:  date,
LatestCommit: date,
}
}
}

// isBot returns true if the name or email matches a known bot pattern.
func isBot(name, email string) bool {
lower := strings.ToLower(name + " " + email)
for _, pat := range botPatterns {
if strings.Contains(lower, strings.ToLower(pat)) {
return true
}
}
return false
}

// MarkFirstTimers marks contributors whose first commit to the project
// falls within the given range. It compares against all-time contributors
// extracted from the full history up to fromRef.
func MarkFirstTimers(contributors []Contributor, opts Options) error {
if opts.FromRef == "" {
// No previous history to compare against; all are first-timers.
for i := range contributors {
contributors[i].IsFirstTime = true
}
return nil
}

// Get all previous contributors (before the range).
prevOpts := Options{
RepoDir: opts.RepoDir,
ToRef:   opts.FromRef,
gitRun:  opts.gitRun,
}
prev, err := Extract(prevOpts)
if err != nil {
return fmt.Errorf("mark first timers: %w", err)
}
prevSet := make(map[string]bool, len(prev))
for _, c := range prev {
prevSet[strings.ToLower(c.Email)] = true
}

for i := range contributors {
if !prevSet[strings.ToLower(contributors[i].Email)] {
contributors[i].IsFirstTime = true
}
}
return nil
}

// ExtractAll returns all-time contributors across the full git history.
func ExtractAll(opts Options) ([]Contributor, error) {
allOpts := Options{
RepoDir: opts.RepoDir,
ToRef:   "HEAD",
gitRun:  opts.gitRun,
}
return Extract(allOpts)
}