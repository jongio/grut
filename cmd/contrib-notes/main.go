// Command contrib-notes extracts contributor information from git history
// and outputs formatted text for changelogs, release notes, or CONTRIBUTORS.md.
package main

import (
"flag"
"fmt"
"os"

"github.com/jongio/grut/internal/contributors"
)

func main() {
from := flag.String("from", "", "Starting ref (exclusive)")
to := flag.String("to", "HEAD", "Ending ref (inclusive)")
format := flag.String("format", "changelog", "Output format: changelog, release, contributors")
repoDir := flag.String("repo", ".", "Repository directory")
flag.Parse()

opts := contributors.Options{
RepoDir: *repoDir,
FromRef: *from,
ToRef:   *to,
}

switch *format {
case "contributors":
contribs, err := contributors.ExtractAll(opts)
if err != nil {
fmt.Fprintf(os.Stderr, "error: %v\n", err)
os.Exit(1)
}
fmt.Print(contributors.FormatContributorsMD(contribs))
default:
contribs, err := contributors.Extract(opts)
if err != nil {
fmt.Fprintf(os.Stderr, "error: %v\n", err)
os.Exit(1)
}
if err := contributors.MarkFirstTimers(contribs, opts); err != nil {
fmt.Fprintf(os.Stderr, "warning: could not detect first-timers: %v\n", err)
}
switch *format {
case "release":
fmt.Print(contributors.FormatReleaseNotes(contribs))
default:
fmt.Print(contributors.FormatChangelog(contribs))
}
}
}
