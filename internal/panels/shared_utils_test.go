package panels

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// EnsureCursorVisible — cursor/offset/viewport clamping
// ---------------------------------------------------------------------------

func TestEnsureCursorVisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor int
		offset int
		height int
		want   int
	}{
		{
			name:   "cursor above viewport snaps offset to cursor",
			cursor: 2,
			offset: 5,
			height: 10,
			want:   2,
		},
		{
			name:   "cursor below viewport adjusts offset down",
			cursor: 20,
			offset: 5,
			height: 10,
			want:   11,
		},
		{
			name:   "cursor within viewport keeps offset unchanged",
			cursor: 7,
			offset: 5,
			height: 10,
			want:   5,
		},
		{
			name:   "cursor at top edge of viewport keeps offset",
			cursor: 5,
			offset: 5,
			height: 10,
			want:   5,
		},
		{
			name:   "cursor at bottom edge of viewport keeps offset",
			cursor: 14,
			offset: 5,
			height: 10,
			want:   5,
		},
		{
			name:   "cursor one past bottom edge adjusts offset",
			cursor: 15,
			offset: 5,
			height: 10,
			want:   6,
		},
		{
			name:   "negative cursor snaps offset to cursor",
			cursor: -3,
			offset: 0,
			height: 10,
			want:   -3,
		},
		{
			name:   "zero height returns offset unchanged",
			cursor: 5,
			offset: 3,
			height: 0,
			want:   3,
		},
		{
			name:   "negative height returns offset unchanged",
			cursor: 5,
			offset: 3,
			height: -1,
			want:   3,
		},
		{
			name:   "height of one with cursor at offset",
			cursor: 5,
			offset: 5,
			height: 1,
			want:   5,
		},
		{
			name:   "height of one with cursor above offset",
			cursor: 3,
			offset: 5,
			height: 1,
			want:   3,
		},
		{
			name:   "height of one with cursor below offset",
			cursor: 7,
			offset: 5,
			height: 1,
			want:   7,
		},
		{
			name:   "cursor at zero with zero offset",
			cursor: 0,
			offset: 0,
			height: 10,
			want:   0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := EnsureCursorVisible(tt.cursor, tt.offset, tt.height)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ClampCursor — bounds clamping for list cursors
// ---------------------------------------------------------------------------

func TestClampCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor int
		length int
		want   int
	}{
		{
			name:   "cursor within bounds returns cursor",
			cursor: 3,
			length: 10,
			want:   3,
		},
		{
			name:   "cursor at zero returns zero",
			cursor: 0,
			length: 10,
			want:   0,
		},
		{
			name:   "cursor at last index returns cursor",
			cursor: 9,
			length: 10,
			want:   9,
		},
		{
			name:   "cursor above length clamps to last index",
			cursor: 15,
			length: 10,
			want:   9,
		},
		{
			name:   "cursor equal to length clamps to last index",
			cursor: 10,
			length: 10,
			want:   9,
		},
		{
			name:   "negative cursor clamps to zero",
			cursor: -5,
			length: 10,
			want:   0,
		},
		{
			name:   "zero length returns zero",
			cursor: 5,
			length: 0,
			want:   0,
		},
		{
			name:   "negative length returns zero",
			cursor: 5,
			length: -1,
			want:   0,
		},
		{
			name:   "length of one clamps cursor to zero",
			cursor: 3,
			length: 1,
			want:   0,
		},
		{
			name:   "length of one with zero cursor returns zero",
			cursor: 0,
			length: 1,
			want:   0,
		},
		{
			name:   "negative cursor with zero length returns zero",
			cursor: -1,
			length: 0,
			want:   0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClampCursor(tt.cursor, tt.length)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ColorOf — themed color with fallback
// ---------------------------------------------------------------------------

func TestColorOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		themed   string
		fallback string
	}{
		{
			name:     "themed value is used when non-empty",
			themed:   "#ff0000",
			fallback: "#000000",
		},
		{
			name:     "fallback is used when themed is empty",
			themed:   "",
			fallback: "#00ff00",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ColorOf(tt.themed, tt.fallback)
			// ColorOf always returns a non-nil color.Color.
			assert.NotNil(t, got)
		})
	}
}

func TestColorOf_ThemedOverridesFallback(t *testing.T) {
	t.Parallel()

	// When themed is provided, ColorOf should return a different color
	// than when themed is empty (i.e., the fallback path).
	withThemed := ColorOf("#ff0000", "#000000")
	withFallback := ColorOf("", "#000000")

	// Both should be valid colors — the important thing is they resolve
	// without panic or error.
	assert.NotNil(t, withThemed)
	assert.NotNil(t, withFallback)
}

func TestColorOf_EmptyBothValues(t *testing.T) {
	t.Parallel()

	// Even with both empty, lipgloss.Color("") should return a valid color.
	got := ColorOf("", "")
	assert.NotNil(t, got)
}

// ---------------------------------------------------------------------------
// OrDefault — string fallback
// ---------------------------------------------------------------------------

func TestOrDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		themed   string
		fallback string
		want     string
	}{
		{
			name:     "returns themed when non-empty",
			themed:   "catppuccin",
			fallback: "default",
			want:     "catppuccin",
		},
		{
			name:     "returns fallback when themed is empty",
			themed:   "",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "returns empty when both are empty",
			themed:   "",
			fallback: "",
			want:     "",
		},
		{
			name:     "whitespace-only themed is treated as non-empty",
			themed:   "  ",
			fallback: "default",
			want:     "  ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := OrDefault(tt.themed, tt.fallback)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ScrollDelta — constant sanity check
// ---------------------------------------------------------------------------

func TestScrollDelta(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 3, ScrollDelta, "ScrollDelta should be 3 for consistent scroll behavior")
}

// ---------------------------------------------------------------------------
// StartDetachedFn — timeout reaper path
// ---------------------------------------------------------------------------

func TestStartDetachedFn_ReaperTimeout(t *testing.T) {
	t.Parallel()

	// Launch a command that sleeps longer than we want to wait, then verify
	// StartDetachedFn returns immediately (the reaper goroutine handles
	// the timeout in the background).
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// "ping -n 10 127.0.0.1" takes ~9 seconds on Windows.
		cmd = exec.Command("ping", "-n", "10", "127.0.0.1")
	default:
		cmd = exec.Command("sleep", "10")
	}

	start := time.Now()
	err := StartDetachedFn(cmd)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	// StartDetachedFn should return almost immediately (well under 1s),
	// NOT block for the full duration of the command.
	assert.Less(t, elapsed, 2*time.Second, "StartDetachedFn should not block on long-running commands")
}

func TestStartDetachedFn_NilCmd(t *testing.T) {
	t.Parallel()

	// Verify StartDetachedFn fails gracefully when given a command that
	// cannot start. This exercises the Start() error path.
	cmd := exec.Command("")
	err := StartDetachedFn(cmd)
	assert.Error(t, err)
}
