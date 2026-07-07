package git

import "testing"

func hunk(lines ...DiffLine) Hunk {
	return Hunk{Lines: lines}
}

func line(t DiffLineType) DiffLine {
	return DiffLine{Type: t}
}

func TestStat(t *testing.T) {
	tests := []struct {
		name  string
		diffs []FileDiff
		want  DiffStat
	}{
		{
			name:  "no diffs",
			diffs: nil,
			want:  DiffStat{},
		},
		{
			name: "single file additions and deletions",
			diffs: []FileDiff{
				{
					Path: "a.go",
					Hunks: []Hunk{
						hunk(
							line(DiffLineContext),
							line(DiffLineAdded),
							line(DiffLineAdded),
							line(DiffLineRemoved),
						),
					},
				},
			},
			want: DiffStat{FilesChanged: 1, Insertions: 2, Deletions: 1},
		},
		{
			name: "multiple files across multiple hunks",
			diffs: []FileDiff{
				{
					Path: "a.go",
					Hunks: []Hunk{
						hunk(line(DiffLineAdded), line(DiffLineAdded)),
						hunk(line(DiffLineRemoved)),
					},
				},
				{
					Path: "b.go",
					Hunks: []Hunk{
						hunk(line(DiffLineAdded), line(DiffLineRemoved), line(DiffLineContext)),
					},
				},
			},
			want: DiffStat{FilesChanged: 2, Insertions: 3, Deletions: 2},
		},
		{
			name: "binary file counts as changed with no line counts",
			diffs: []FileDiff{
				{Path: "image.png", IsBinary: true},
			},
			want: DiffStat{FilesChanged: 1, Insertions: 0, Deletions: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Stat(tt.diffs)
			if got != tt.want {
				t.Fatalf("Stat() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffStatIsZero(t *testing.T) {
	if !(DiffStat{}).IsZero() {
		t.Error("empty DiffStat should be zero")
	}
	if (DiffStat{FilesChanged: 1}).IsZero() {
		t.Error("DiffStat with a changed file should not be zero")
	}
	if (DiffStat{Insertions: 1}).IsZero() {
		t.Error("DiffStat with insertions should not be zero")
	}
	if (DiffStat{Deletions: 1}).IsZero() {
		t.Error("DiffStat with deletions should not be zero")
	}
}

func TestDiffStatSummary(t *testing.T) {
	tests := []struct {
		name string
		stat DiffStat
		want string
	}{
		{
			name: "zero stat is empty",
			stat: DiffStat{},
			want: "",
		},
		{
			name: "single file uses singular noun",
			stat: DiffStat{FilesChanged: 1, Insertions: 3, Deletions: 0},
			want: "1 file changed, +3 -0",
		},
		{
			name: "multiple files use plural noun",
			stat: DiffStat{FilesChanged: 5, Insertions: 128, Deletions: 34},
			want: "5 files changed, +128 -34",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stat.Summary(); got != tt.want {
				t.Fatalf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
