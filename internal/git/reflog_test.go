package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseReflogCases(t *testing.T) {
	t.Parallel()

	const defaultSep = "\x1e"

	tests := []struct {
		name  string
		sep   string
		input string
		want  []ReflogEntry
	}{
		{
			name:  "empty input returns empty slice",
			sep:   defaultSep,
			input: "",
			want:  []ReflogEntry{},
		},
		{
			name:  "single entry with valid RFC3339 date",
			sep:   defaultSep,
			input: "abc123" + defaultSep + "commit: Initial" + defaultSep + "HEAD@{0}" + defaultSep + "HEAD@{0}" + defaultSep + "2024-01-15T10:30:00Z\n",
			want: []ReflogEntry{{
				Hash:    "abc123",
				Message: "commit: Initial",
				Action:  "HEAD@{0}",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name: "multiple entries",
			sep:  defaultSep,
			input: "abc123" + defaultSep + "commit: Initial" + defaultSep + "HEAD@{0}" + defaultSep + "HEAD@{0}" + defaultSep + "2024-01-15T10:30:00Z\n" +
				"def456" + defaultSep + "checkout: moving" + defaultSep + "HEAD@{1}" + defaultSep + "HEAD@{1}" + defaultSep + "2024-01-14T09:00:00Z\n",
			want: []ReflogEntry{
				{
					Hash:    "abc123",
					Message: "commit: Initial",
					Action:  "HEAD@{0}",
					Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
				},
				{
					Hash:    "def456",
					Message: "checkout: moving",
					Action:  "HEAD@{1}",
					Date:    time.Date(2024, time.January, 14, 9, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:  "line with fewer than five fields is skipped",
			sep:   defaultSep,
			input: "abc123" + defaultSep + "commit: Initial\n",
			want:  []ReflogEntry{},
		},
		{
			name:  "empty lines are skipped",
			sep:   defaultSep,
			input: "\n\nabc123" + defaultSep + "commit: Initial" + defaultSep + "HEAD@{0}" + defaultSep + "HEAD@{0}" + defaultSep + "2024-01-15T10:30:00Z\n\n",
			want: []ReflogEntry{{
				Hash:    "abc123",
				Message: "commit: Initial",
				Action:  "HEAD@{0}",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name:  "windows line endings are handled",
			sep:   defaultSep,
			input: "abc123" + defaultSep + "commit: Initial" + defaultSep + "HEAD@{0}" + defaultSep + "HEAD@{0}" + defaultSep + "2024-01-15T10:30:00Z\r\n",
			want: []ReflogEntry{{
				Hash:    "abc123",
				Message: "commit: Initial",
				Action:  "HEAD@{0}",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name:  "invalid date is skipped gracefully",
			sep:   defaultSep,
			input: "abc123" + defaultSep + "commit: Initial" + defaultSep + "HEAD@{0}" + defaultSep + "HEAD@{0}" + defaultSep + "not-a-date\n",
			want: []ReflogEntry{{
				Hash:    "abc123",
				Message: "commit: Initial",
				Action:  "HEAD@{0}",
			}},
		},
		{
			name:  "custom separator works",
			sep:   "|",
			input: "abc123|commit: Initial|HEAD@{0}|HEAD@{0}|2024-01-15T10:30:00Z\n",
			want: []ReflogEntry{{
				Hash:    "abc123",
				Message: "commit: Initial",
				Action:  "HEAD@{0}",
				Date:    time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseReflog(tt.input, tt.sep)
			assert.Equal(t, tt.want, got)
		})
	}
}
