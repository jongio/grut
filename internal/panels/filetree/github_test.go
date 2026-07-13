package filetree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongio/grut/internal/notify"
)

func TestBlobTreeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		remote string
		sha    string
		rel    string
		isDir  bool
		want   string
	}{
		{
			name:   "file blob",
			remote: "https://github.com/jongio/grut.git",
			sha:    "abc123",
			rel:    "internal/git/url.go",
			isDir:  false,
			want:   "https://github.com/jongio/grut/blob/abc123/internal/git/url.go",
		},
		{
			name:   "directory tree",
			remote: "git@github.com:jongio/grut.git",
			sha:    "def456",
			rel:    "internal/panels",
			isDir:  true,
			want:   "https://github.com/jongio/grut/tree/def456/internal/panels",
		},
		{
			name:   "repository root",
			remote: "https://github.com/jongio/grut.git",
			sha:    "abc123",
			rel:    ".",
			isDir:  true,
			want:   "https://github.com/jongio/grut/tree/abc123",
		},
		{
			name:   "path with space is escaped",
			remote: "https://github.com/jongio/grut.git",
			sha:    "abc123",
			rel:    "docs/my notes.md",
			isDir:  false,
			want:   "https://github.com/jongio/grut/blob/abc123/docs/my%20notes.md",
		},
		{
			name:   "non-github remote returns empty",
			remote: "https://gitlab.com/jongio/grut.git",
			sha:    "abc123",
			rel:    "main.go",
			isDir:  false,
			want:   "",
		},
		{
			name:   "empty sha returns empty",
			remote: "https://github.com/jongio/grut.git",
			sha:    "",
			rel:    "main.go",
			isDir:  false,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := blobTreeURL(tt.remote, tt.sha, tt.rel, tt.isDir)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenOnGitHubEmptyTreeNoop(t *testing.T) {
	ft := initFT(t, t.TempDir())
	require.Nil(t, ft.cursorNode())

	_, cmd := ft.Update(keyMsg('B'))
	assert.Nil(t, cmd, "expected no command when nothing is selected")
}

func TestOpenOnGitHubNoGitClient(t *testing.T) {
	dir := createTestTree(t)
	ft := initFT(t, dir)
	require.NotNil(t, ft.cursorNode())

	_, cmd := ft.Update(keyMsg('B'))
	require.NotNil(t, cmd)

	msg := runCmd(t, ft, cmd)
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
}
