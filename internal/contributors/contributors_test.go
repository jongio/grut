package contributors

import (
"fmt"
"strings"
"testing"
"time"

"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
)

// fakeGitRunner returns a gitRunner that responds with canned output
// keyed by the first non-"log" arg (the ref range).
func fakeGitRunner(authorOut, trailerOut string) gitRunner {
call := 0
return func(_ string, args ...string) (string, error) {
call++
if call == 1 {
return authorOut, nil
}
return trailerOut, nil
}
}

func TestExtract_BasicAuthors(t *testing.T) {
authorOut := strings.Join([]string{
"Alice\x1ealice@example.com\x1e2026-01-15T10:00:00Z\x1f",
"Bob\x1ebob@example.com\x1e2026-01-16T12:00:00Z\x1f",
"Alice\x1ealice@example.com\x1e2026-01-17T14:00:00Z\x1f",
}, "")
trailerOut := "\x1f" // no trailers

opts := Options{
RepoDir: ".",
FromRef: "v0.1.0",
ToRef:   "v0.2.0",
gitRun:  fakeGitRunner(authorOut, trailerOut),
}

contributors, err := Extract(opts)
require.NoError(t, err)
assert.Len(t, contributors, 2)

// Alice has 2 commits, should be first.
assert.Equal(t, "Alice", contributors[0].Name)
assert.Equal(t, 2, contributors[0].CommitCount)
assert.Equal(t, "Bob", contributors[1].Name)
assert.Equal(t, 1, contributors[1].CommitCount)
}

func TestExtract_CoAuthoredBy(t *testing.T) {
authorOut := "Alice\x1ealice@example.com\x1e2026-01-15T10:00:00Z\x1f"
trailerOut := "2026-01-15T10:00:00Z\nSome commit body\nCo-authored-by: Charlie <charlie@example.com>\n\x1f"

opts := Options{
RepoDir: ".",
gitRun:  fakeGitRunner(authorOut, trailerOut),
}

contributors, err := Extract(opts)
require.NoError(t, err)
assert.Len(t, contributors, 2)

names := make(map[string]bool)
for _, c := range contributors {
names[c.Name] = true
}
assert.True(t, names["Alice"])
assert.True(t, names["Charlie"])
}

func TestExtract_BotFiltering(t *testing.T) {
authorOut := strings.Join([]string{
"Alice\x1ealice@example.com\x1e2026-01-15T10:00:00Z\x1f",
"dependabot[bot]\x1edependabot[bot]@users.noreply.github.com\x1e2026-01-16T12:00:00Z\x1f",
"github-actions[bot]\x1e41898282+github-actions[bot]@users.noreply.github.com\x1e2026-01-17T14:00:00Z\x1f",
}, "")
trailerOut := "\x1f"

opts := Options{
RepoDir: ".",
gitRun:  fakeGitRunner(authorOut, trailerOut),
}

contributors, err := Extract(opts)
require.NoError(t, err)
assert.Len(t, contributors, 1)
assert.Equal(t, "Alice", contributors[0].Name)
}

func TestExtract_Deduplication(t *testing.T) {
// Same person, different case email.
authorOut := strings.Join([]string{
"Alice\x1eAlice@Example.com\x1e2026-01-15T10:00:00Z\x1f",
"Alice\x1ealice@example.com\x1e2026-01-16T12:00:00Z\x1f",
}, "")
trailerOut := "\x1f"

opts := Options{
RepoDir: ".",
gitRun:  fakeGitRunner(authorOut, trailerOut),
}

contributors, err := Extract(opts)
require.NoError(t, err)
assert.Len(t, contributors, 1)
assert.Equal(t, 2, contributors[0].CommitCount)
}

func TestIsBot(t *testing.T) {
tests := []struct {
name  string
email string
want  bool
}{
{"dependabot[bot]", "dependabot@users.noreply.github.com", true},
{"github-actions[bot]", "actions@github.com", true},
{"copilot-swe-agent[bot]", "copilot@github.com", true},
{"renovate[bot]", "renovate@whitesource.com", true},
{"Alice", "alice@example.com", false},
{"Bob", "bob@company.com", false},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
assert.Equal(t, tt.want, isBot(tt.name, tt.email))
})
}
}

func TestMarkFirstTimers(t *testing.T) {
// Simulate: Alice existed before, Bob is new.
callCount := 0
mockRun := func(_ string, args ...string) (string, error) {
callCount++
// The first two calls are for the main Extract (already done).
// MarkFirstTimers calls Extract internally for the "previous" range.
// Calls 1-2: Extract for prev range (authors + trailers).
if callCount == 1 {
// Previous authors: only Alice.
return "Alice\x1ealice@example.com\x1e2026-01-01T10:00:00Z\x1f", nil
}
// trailers for prev range
return "\x1f", nil
}

contributors := []Contributor{
{Name: "Alice", Email: "alice@example.com", CommitCount: 3},
{Name: "Bob", Email: "bob@example.com", CommitCount: 1},
}

err := MarkFirstTimers(contributors, Options{
FromRef: "v0.1.0",
gitRun:  mockRun,
})
require.NoError(t, err)
assert.False(t, contributors[0].IsFirstTime, "Alice should not be first-timer")
assert.True(t, contributors[1].IsFirstTime, "Bob should be first-timer")
}

func TestMarkFirstTimers_NoFromRef(t *testing.T) {
contributors := []Contributor{
{Name: "Alice", Email: "alice@example.com"},
}

err := MarkFirstTimers(contributors, Options{})
require.NoError(t, err)
assert.True(t, contributors[0].IsFirstTime)
}

func TestFormatChangelog(t *testing.T) {
contributors := []Contributor{
{Name: "Alice", CommitCount: 5, IsFirstTime: false},
{Name: "Bob", CommitCount: 2, IsFirstTime: true},
}

result := FormatChangelog(contributors)
assert.Contains(t, result, "### Contributors")
assert.Contains(t, result, "- **Alice**")
assert.Contains(t, result, "- **Bob**")
assert.Contains(t, result, "New contributors: **Bob**")
}

func TestFormatChangelog_Empty(t *testing.T) {
assert.Equal(t, "", FormatChangelog(nil))
}

func TestFormatReleaseNotes(t *testing.T) {
contributors := []Contributor{
{Name: "Alice", CommitCount: 5},
{Name: "Bob", CommitCount: 2, IsFirstTime: true},
}

result := FormatReleaseNotes(contributors)
assert.Contains(t, result, "## Contributors")
assert.Contains(t, result, "- **Alice**")
assert.Contains(t, result, "New contributors: **Bob**")
}

func TestFormatContributorsMD(t *testing.T) {
contributors := []Contributor{
{Name: "Alice", CommitCount: 10},
{Name: "Bob", CommitCount: 3},
}

result := FormatContributorsMD(contributors)
assert.Contains(t, result, "# Contributors")
assert.Contains(t, result, "| **Alice** | 10 |")
assert.Contains(t, result, "| **Bob** | 3 |")
assert.Contains(t, result, "Last updated:")
}

func TestExtract_EmptyOutput(t *testing.T) {
opts := Options{
RepoDir: ".",
gitRun:  fakeGitRunner("", ""),
}

contributors, err := Extract(opts)
require.NoError(t, err)
assert.Empty(t, contributors)
}

func TestExtract_GitError(t *testing.T) {
opts := Options{
RepoDir: ".",
gitRun: func(_ string, args ...string) (string, error) {
return "", fmt.Errorf("git error")
},
}

_, err := Extract(opts)
assert.Error(t, err)
}

func TestAddContributor_DateTracking(t *testing.T) {
m := make(map[string]*Contributor)
t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

addContributor(m, "Alice", "alice@example.com", t2)
addContributor(m, "Alice", "alice@example.com", t1)
addContributor(m, "Alice", "alice@example.com", t3)

c := m["alice@example.com"]
assert.Equal(t, 3, c.CommitCount)
assert.Equal(t, t1, c.FirstCommit)
assert.Equal(t, t2, c.LatestCommit)
}

func TestRefRange(t *testing.T) {
tests := []struct {
from, to, want string
}{
{"v0.1.0", "v0.2.0", "v0.1.0..v0.2.0"},
{"", "v0.2.0", "v0.2.0"},
{"v0.1.0", "", "v0.1.0..HEAD"},
{"", "", "HEAD"},
}

for _, tt := range tests {
o := Options{FromRef: tt.from, ToRef: tt.to}
assert.Equal(t, tt.want, o.refRange(), "from=%q to=%q", tt.from, tt.to)
}
}
