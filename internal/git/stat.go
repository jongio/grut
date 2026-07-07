package git

import "fmt"

// DiffStat summarizes the size of a set of file diffs.
type DiffStat struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// Stat computes a DiffStat from parsed file diffs. FilesChanged counts every
// file with a diff entry (including binary files); Insertions and Deletions
// count added and removed lines, matching the totals reported by
// `git diff --shortstat` for the same range.
func Stat(diffs []FileDiff) DiffStat {
	s := DiffStat{FilesChanged: len(diffs)}
	for _, d := range diffs {
		for _, h := range d.Hunks {
			for _, l := range h.Lines {
				switch l.Type {
				case DiffLineAdded:
					s.Insertions++
				case DiffLineRemoved:
					s.Deletions++
				default:
					// Context and other line types don't affect the counts.
				}
			}
		}
	}
	return s
}

// IsZero reports whether the stat represents no changes.
func (s DiffStat) IsZero() bool {
	return s.FilesChanged == 0 && s.Insertions == 0 && s.Deletions == 0
}

// Summary returns a plain-text one-line summary such as
// "5 files changed, +128 -34". It returns an empty string for a zero stat.
func (s DiffStat) Summary() string {
	if s.IsZero() {
		return ""
	}
	noun := "files"
	if s.FilesChanged == 1 {
		noun = "file"
	}
	return fmt.Sprintf("%d %s changed, +%d -%d", s.FilesChanged, noun, s.Insertions, s.Deletions)
}
