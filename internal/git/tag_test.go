package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTagListCases(t *testing.T) {
	t.Parallel()

	const sep = "\x1e"

	tests := []struct {
		name  string
		input string
		want  []Tag
	}{
		{
			name:  "empty input returns empty slice",
			input: "",
			want:  []Tag{},
		},
		{
			name:  "single lightweight tag",
			input: "v0.1.0" + sep + "abc123" + sep + "commit" + sep + "Lightweight release" + sep + "" + sep + "2024-01-10 09:00:00 +0000\n",
			want: []Tag{{
				Name:        "v0.1.0",
				Hash:        "abc123",
				Message:     "Lightweight release",
				IsAnnotated: false,
				Tagger:      "",
				Date:        time.Date(2024, time.January, 10, 9, 0, 0, 0, time.UTC),
			}},
		},
		{
			name:  "single annotated tag",
			input: "v1.0.0" + sep + "def456" + sep + "tag" + sep + "Release 1.0" + sep + "John Doe" + sep + "2024-01-15 10:30:00 +0000\n",
			want: []Tag{{
				Name:        "v1.0.0",
				Hash:        "def456",
				Message:     "Release 1.0",
				IsAnnotated: true,
				Tagger:      "John Doe",
				Date:        time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
		{
			name: "multiple tags mixed",
			input: "v1.0.0" + sep + "def456" + sep + "tag" + sep + "Release 1.0" + sep + "John Doe" + sep + "2024-01-15 10:30:00 +0000\n" +
				"v0.1.0" + sep + "abc123" + sep + "commit" + sep + "Lightweight release" + sep + "" + sep + "2024-01-10 09:00:00 +0000\n",
			want: []Tag{
				{
					Name:        "v1.0.0",
					Hash:        "def456",
					Message:     "Release 1.0",
					IsAnnotated: true,
					Tagger:      "John Doe",
					Date:        time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
				},
				{
					Name:        "v0.1.0",
					Hash:        "abc123",
					Message:     "Lightweight release",
					IsAnnotated: false,
					Tagger:      "",
					Date:        time.Date(2024, time.January, 10, 9, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:  "lines with fewer than six fields are skipped",
			input: "short" + sep + "line\n",
			want:  []Tag{},
		},
		{
			name:  "invalid date is handled gracefully",
			input: "v1.0.0" + sep + "def456" + sep + "tag" + sep + "Release 1.0" + sep + "John Doe" + sep + "not-a-date\n",
			want: []Tag{{
				Name:        "v1.0.0",
				Hash:        "def456",
				Message:     "Release 1.0",
				IsAnnotated: true,
				Tagger:      "John Doe",
			}},
		},
		{
			name:  "windows line endings are handled",
			input: "v1.0.0" + sep + "def456" + sep + "tag" + sep + "Release 1.0" + sep + "John Doe" + sep + "2024-01-15 10:30:00 +0000\r\n",
			want: []Tag{{
				Name:        "v1.0.0",
				Hash:        "def456",
				Message:     "Release 1.0",
				IsAnnotated: true,
				Tagger:      "John Doe",
				Date:        time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC),
			}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseTagList(tt.input, sep)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].Name, got[i].Name)
				assert.Equal(t, tt.want[i].Hash, got[i].Hash)
				assert.Equal(t, tt.want[i].Message, got[i].Message)
				assert.Equal(t, tt.want[i].IsAnnotated, got[i].IsAnnotated)
				assert.Equal(t, tt.want[i].Tagger, got[i].Tagger)
				if tt.want[i].Date.IsZero() {
					assert.True(t, got[i].Date.IsZero())
				} else {
					assert.True(t, tt.want[i].Date.Equal(got[i].Date))
				}
			}
		})
	}
}

func TestParseRemoteTagsCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		localSet map[string]bool
		want     []Tag
	}{
		{
			name:     "empty input returns empty slice",
			input:    "",
			localSet: map[string]bool{},
			want:     []Tag{},
		},
		{
			name:     "single remote tag not in local set",
			input:    "0123456789abcdef refs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			want: []Tag{{
				Name: "v1.0.0",
				Hash: "0123456",
			}},
		},
		{
			name:     "tag in local set is filtered out",
			input:    "0123456789abcdef refs/tags/v1.0.0\n",
			localSet: map[string]bool{"v1.0.0": true},
			want:     []Tag{},
		},
		{
			name:     "dereference line is skipped",
			input:    "0123456789abcdef refs/tags/v1.0.0^{}\n",
			localSet: map[string]bool{},
			want:     []Tag{},
		},
		{
			name:     "hash already seven or fewer chars stays unchanged",
			input:    "abc1234 refs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			want: []Tag{{
				Name: "v1.0.0",
				Hash: "abc1234",
			}},
		},
		{
			name:     "duplicate names are deduplicated",
			input:    "0123456789abcdef refs/tags/v1.0.0\n89abcdef01234567 refs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			want: []Tag{{
				Name: "v1.0.0",
				Hash: "0123456",
			}},
		},
		{
			name:     "lines with fewer than two fields are skipped",
			input:    "broken-line\n",
			localSet: map[string]bool{},
			want:     []Tag{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseRemoteTags(tt.input, tt.localSet)
			assert.Equal(t, tt.want, got)
		})
	}
}
