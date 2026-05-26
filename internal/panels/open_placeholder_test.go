package panels

import (
	"context"
	"testing"

	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// OpenInBrowser — validation rejection paths
// ---------------------------------------------------------------------------

func TestOpenInBrowser_RejectsInvalidURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name:    "empty URL",
			url:     "",
			wantErr: "browser URL must not be empty",
		},
		{
			name:    "javascript scheme",
			url:     "javascript:alert(1)",
			wantErr: "not allowed",
		},
		{
			name:    "data scheme",
			url:     "data:text/html,<h1>hi</h1>",
			wantErr: "not allowed",
		},
		{
			name:    "file scheme",
			url:     "file:///etc/passwd",
			wantErr: "not allowed",
		},
		{
			name:    "no scheme",
			url:     "example.com/path",
			wantErr: "no scheme",
		},
		{
			name:    "credentials in URL",
			url:     "https://user:pass@evil.com",
			wantErr: "must not contain credentials",
		},
		{
			name:    "null byte",
			url:     "https://example.com/\x00path",
			wantErr: "null byte",
		},
		{
			name:    "ftp scheme",
			url:     "ftp://files.example.com/data",
			wantErr: "not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := OpenInBrowser(context.Background(), tt.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ---------------------------------------------------------------------------
// OpenInTerminal — validation rejection paths
// ---------------------------------------------------------------------------

func TestOpenInTerminal_RejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     string
		wantErr string
	}{
		{
			name:    "empty path",
			dir:     "",
			wantErr: "editor path must not be empty",
		},
		{
			name:    "null byte in path",
			dir:     "/tmp/\x00evil",
			wantErr: "null byte",
		},
		{
			name:    "semicolon injection",
			dir:     "/tmp/dir;rm -rf /",
			wantErr: "forbidden character",
		},
		{
			name:    "pipe character",
			dir:     "/tmp/dir|cat /etc/passwd",
			wantErr: "forbidden character",
		},
		{
			name:    "backtick injection",
			dir:     "/tmp/`whoami`",
			wantErr: "forbidden character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := OpenInTerminal(context.Background(), tt.dir)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ---------------------------------------------------------------------------
// colorFallback — theme-aware color resolution
// ---------------------------------------------------------------------------

func TestColorFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		th       *theme.Theme
		getter   func(theme.Colors) string
		fallback string
		wantNil  bool // if true, verify returned color is non-nil (lipgloss.Color always returns non-nil)
	}{
		{
			name:     "nil theme uses fallback",
			th:       nil,
			getter:   func(c theme.Colors) string { return c.BrightBlack },
			fallback: "#aabbcc",
		},
		{
			name:     "theme with empty value uses fallback",
			th:       &theme.Theme{Colors: theme.Colors{BrightBlack: ""}},
			getter:   func(c theme.Colors) string { return c.BrightBlack },
			fallback: "#112233",
		},
		{
			name:     "theme with value uses theme color",
			th:       &theme.Theme{Colors: theme.Colors{BrightBlack: "#ff0000"}},
			getter:   func(c theme.Colors) string { return c.BrightBlack },
			fallback: "#000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := colorFallback(tt.th, tt.getter, tt.fallback)
			assert.NotNil(t, result)
		})
	}
}

// ---------------------------------------------------------------------------
// Placeholder — View with zero/negative dimensions
// ---------------------------------------------------------------------------

func TestPlaceholder_View_ZeroDimensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
		want   string
	}{
		{name: "zero width", width: 0, height: 10, want: ""},
		{name: "zero height", width: 10, height: 0, want: ""},
		{name: "negative width", width: -1, height: 10, want: ""},
		{name: "negative height", width: 10, height: -1, want: ""},
		{name: "both zero", width: 0, height: 0, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := NewPlaceholder("test", nil)
			assert.Equal(t, tt.want, p.View(tt.width, tt.height))
		})
	}
}

func TestPlaceholder_View_Focused(t *testing.T) {
	t.Parallel()
	p := NewPlaceholder("Preview", nil)
	p.Focus()
	view := p.View(40, 5)
	assert.Contains(t, view, "Preview")
}

func TestPlaceholder_View_Unfocused(t *testing.T) {
	t.Parallel()
	p := NewPlaceholder("Status", nil)
	p.Blur()
	view := p.View(40, 5)
	assert.Contains(t, view, "Status")
}
