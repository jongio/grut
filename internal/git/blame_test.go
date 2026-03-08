package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBlameCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []BlameLine
	}{
		{
			name:  "empty input returns empty slice",
			input: "",
			want:  []BlameLine{},
		},
		{
			name: "single blame block with author time and content",
			input: "abc123def456789012345678901234567890abcd 1 7 1\n" +
				"author John Doe\n" +
				"author-time 1705315800\n" +
				"summary Initial commit\n" +
				"\tfirst line\n",
			want: []BlameLine{{
				Hash:    "abc123def456789012345678901234567890abcd",
				Author:  "John Doe",
				Date:    time.Unix(1705315800, 0),
				LineNo:  7,
				Content: "first line",
			}},
		},
		{
			name: "multiple blame blocks",
			input: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 1 1 1\n" +
				"author Alice\n" +
				"author-time 1705315800\n" +
				"\tline one\n" +
				"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb 2 2 1\n" +
				"author Bob\n" +
				"author-time 1705315900\n" +
				"\tline two\n",
			want: []BlameLine{
				{
					Hash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Author:  "Alice",
					Date:    time.Unix(1705315800, 0),
					LineNo:  1,
					Content: "line one",
				},
				{
					Hash:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					Author:  "Bob",
					Date:    time.Unix(1705315900, 0),
					LineNo:  2,
					Content: "line two",
				},
			},
		},
		{
			name: "windows line endings are handled",
			input: "cccccccccccccccccccccccccccccccccccccccc 3 3 1\r\n" +
				"author Carol\r\n" +
				"author-time 1705316000\r\n" +
				"\tline three\r\n",
			want: []BlameLine{{
				Hash:    "cccccccccccccccccccccccccccccccccccccccc",
				Author:  "Carol",
				Date:    time.Unix(1705316000, 0),
				LineNo:  3,
				Content: "line three",
			}},
		},
		{
			name: "blocks with fewer than three fields are skipped",
			input: "too-short 1\n" +
				"author Ignored\n" +
				"\tmissing\n",
			want: []BlameLine{},
		},
		{
			name: "invalid line number is skipped",
			input: "dddddddddddddddddddddddddddddddddddddddd 1 nope 1\n" +
				"author Invalid\n" +
				"author-time 1705316100\n" +
				"\tignored\n",
			want: []BlameLine{},
		},
		{
			name: "block without content line",
			input: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee 1 9 1\n" +
				"author Empty Content\n" +
				"author-time 1705316200\n",
			want: []BlameLine{{
				Hash:   "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
				Author: "Empty Content",
				Date:   time.Unix(1705316200, 0),
				LineNo: 9,
			}},
		},
		{
			name: "content line leading tab is stripped",
			input: "ffffffffffffffffffffffffffffffffffffffff 1 10 1\n" +
				"author Tab Strip\n" +
				"author-time 1705316300\n" +
				"\t\tindented content\n",
			want: []BlameLine{{
				Hash:    "ffffffffffffffffffffffffffffffffffffffff",
				Author:  "Tab Strip",
				Date:    time.Unix(1705316300, 0),
				LineNo:  10,
				Content: "\tindented content",
			}},
		},
		{
			name: "author-time invalid timestamp is skipped gracefully",
			input: "9999999999999999999999999999999999999999 1 11 1\n" +
				"author Bad Time\n" +
				"author-time nope\n" +
				"\tline\n",
			want: []BlameLine{{
				Hash:    "9999999999999999999999999999999999999999",
				Author:  "Bad Time",
				LineNo:  11,
				Content: "line",
			}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBlame(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
