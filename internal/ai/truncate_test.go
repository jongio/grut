package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateWithIndicator_NoTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"empty_string", "", 10, ""},
		{"short_string", "hello", 10, "hello"},
		{"exact_length", "hello", 5, "hello"},
		{"zero_max", "hello", 0, "hello"},
		{"negative_max", "hello", -1, "hello"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateWithIndicator(tt.input, tt.max)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateWithIndicator_Truncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		max   int
	}{
		{"one_char_over", "hello!", 5},
		{"much_longer", "this is a much longer string that needs truncation", 10},
		{"one_char_max", "ab", 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateWithIndicator(tt.input, tt.max)
			assert.Contains(t, got, "… [truncated]")
			// The prefix should be exactly tt.max characters.
			prefix := tt.input[:tt.max]
			assert.True(t, len(got) > tt.max, "truncated string should be longer than max due to indicator")
			assert.Equal(t, prefix+"… [truncated]", got)
		})
	}
}
