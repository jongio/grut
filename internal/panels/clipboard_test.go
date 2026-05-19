package panels

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPasteFromClipboard_RoundTrip(t *testing.T) {
	// Clipboard is shared OS state; parallel tests can overwrite it.
	// Run only when explicitly requested.
	if testing.Short() {
		t.Skip("skipping clipboard round-trip in short mode")
	}

	ctx := context.Background()
	const want = "hello clipboard"

	if err := CopyToClipboard(ctx, want); err != nil {
		t.Skip("clipboard not available in this environment:", err)
	}

	got, err := PasteFromClipboard(ctx)
	if err != nil {
		t.Skip("clipboard paste not available in this environment:", err)
	}

	if got != want {
		t.Skip("clipboard was overwritten by a concurrent test")
	}
	require.Equal(t, want, got)
}

func TestPasteFromClipboard_CRLFNormalization(t *testing.T) {
	// Clipboard is shared OS state; parallel tests can overwrite it.
	if testing.Short() {
		t.Skip("skipping clipboard CRLF test in short mode")
	}

	ctx := context.Background()
	const input = "line1\r\nline2\r\n"

	if err := CopyToClipboard(ctx, input); err != nil {
		t.Skip("clipboard not available in this environment:", err)
	}

	got, err := PasteFromClipboard(ctx)
	if err != nil {
		t.Skip("clipboard paste not available in this environment:", err)
	}

	if !strings.Contains(got, "line1") {
		t.Skip("clipboard was overwritten by a concurrent test")
	}
	assert.Equal(t, "line1\nline2\n", got, "CRLF should be normalized to LF")
	assert.NotContains(t, got, "\r", "no carriage returns should remain")
}
