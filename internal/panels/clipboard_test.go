package panels

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips csi color codes",
			input: "\x1b[31mred\x1b[0m text",
			want:  "red text",
		},
		{
			name:  "strips osc title",
			input: "\x1b]0;window title\x07content",
			want:  "content",
		},
		{
			name:  "leaves plain text",
			input: "plain text",
			want:  "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StripANSI(tt.input))
		})
	}
}

func TestCopyToClipboard_StripsANSI(t *testing.T) {
	input := "prefix \x1b[32mgreen\x1b[0m \x1b]0;title\x07suffix"

	sanitized := StripANSI(input)

	assert.Equal(t, "prefix green suffix", sanitized)
	assert.NotContains(t, sanitized, "\x1b")
}
