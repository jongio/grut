package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseStashListCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []StashEntry
	}{
		{
			name:  "empty input returns empty slice",
			input: "",
			want:  []StashEntry{},
		},
		{
			name:  "single stash entry with all fields",
			input: "abc123\x1estash@{0}\x1eOn main: save\x1e2024-01-15T10:30:00Z\n",
			want: []StashEntry{{
				Hash:    "abc123",
				Index:   0,
				Message: "On main: save",
				Branch:  "main",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name: "multiple entries",
			input: "abc123\x1estash@{0}\x1eOn main: save\x1e2024-01-15T10:30:00Z\n" +
				"def456\x1estash@{1}\x1eWIP on feature: stuff\x1e2024-01-16T12:00:00Z\n",
			want: []StashEntry{
				{
					Hash:    "abc123",
					Index:   0,
					Message: "On main: save",
					Branch:  "main",
					Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
				},
				{
					Hash:    "def456",
					Index:   1,
					Message: "WIP on feature: stuff",
					Branch:  "feature",
					Date:    time.Date(2024, time.January, 16, 12, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:  "entry without date field keeps zero date",
			input: "abc123\x1estash@{0}\x1eOn main: save\n",
			want: []StashEntry{{
				Hash:    "abc123",
				Index:   0,
				Message: "On main: save",
				Branch:  "main",
			}},
		},
		{
			name:  "On branch format extracts branch",
			input: "abc123\x1estash@{0}\x1eOn main: save\x1e2024-01-15T10:30:00Z\n",
			want: []StashEntry{{
				Hash:    "abc123",
				Index:   0,
				Message: "On main: save",
				Branch:  "main",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name:  "WIP on branch format extracts branch",
			input: "def456\x1estash@{1}\x1eWIP on feature: stuff\x1e2024-01-16T12:00:00Z\n",
			want: []StashEntry{{
				Hash:    "def456",
				Index:   1,
				Message: "WIP on feature: stuff",
				Branch:  "feature",
				Date:    time.Date(2024, time.January, 16, 12, 0, 0, 0, time.UTC),
			}},
		},
		{
			name:  "lines with fewer than three fields are skipped",
			input: "abc123\x1estash@{0}\n",
			want:  []StashEntry{},
		},
		{
			name:  "windows line endings are handled",
			input: "abc123\x1estash@{0}\x1eOn main: save\x1e2024-01-15T10:30:00Z\r\n",
			want: []StashEntry{{
				Hash:    "abc123",
				Index:   0,
				Message: "On main: save",
				Branch:  "main",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseStashList(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStashIndexCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ref     string
		want    int
		wantErr bool
	}{
		{name: "stash zero", ref: "stash@{0}", want: 0, wantErr: false},
		{name: "stash five", ref: "stash@{5}", want: 5, wantErr: false},
		{name: "stash forty two", ref: "stash@{42}", want: 42, wantErr: false},
		{name: "missing braces returns error", ref: "stash@0", want: -1, wantErr: true},
		{name: "empty string returns error", ref: "", want: -1, wantErr: true},
		{name: "empty braces returns error", ref: "stash@{}", want: -1, wantErr: true},
		{name: "non numeric content returns error", ref: "stash@{nope}", want: -1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseStashIndex(tt.ref)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
