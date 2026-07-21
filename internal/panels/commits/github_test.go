package commits

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
)

// ---------------------------------------------------------------------------
// commitURL
// ---------------------------------------------------------------------------

func TestCommitURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		remote string
		hash   string
		want   string
	}{
		{
			name:   "https github remote",
			remote: "https://github.com/jongio/grut.git",
			hash:   "abc123",
			want:   "https://github.com/jongio/grut/commit/abc123",
		},
		{
			name:   "ssh github remote",
			remote: "git@github.com:jongio/grut.git",
			hash:   "def456",
			want:   "https://github.com/jongio/grut/commit/def456",
		},
		{
			name:   "non-github remote returns empty",
			remote: "https://gitlab.com/jongio/grut.git",
			hash:   "abc123",
			want:   "",
		},
		{
			name:   "empty remote returns empty",
			remote: "",
			hash:   "abc123",
			want:   "",
		},
		{
			name:   "empty hash returns empty",
			remote: "https://github.com/jongio/grut.git",
			hash:   "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := commitURL(tt.remote, tt.hash); got != tt.want {
				t.Errorf("commitURL(%q, %q) = %q, want %q", tt.remote, tt.hash, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// openCommitOnGitHub
// ---------------------------------------------------------------------------

func TestOpenCommitOnGitHubEmptyListNoop(t *testing.T) {
	t.Parallel()

	mock := &mockGitOps{commits: nil, root: t.TempDir()}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'o'})
	if cmd != nil {
		t.Error("expected no command when there are no commits")
	}
}

func TestOpenCommitOnGitHubNoGitHubRemote(t *testing.T) {
	t.Parallel()

	// A fresh temp dir is not a git repo, so origin lookup fails and the
	// handler reports that there is no github remote to open. This exercises
	// the guard without launching a browser.
	mock := &mockGitOps{commits: defaultCommits(), root: t.TempDir()}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'o'})
	if cmd == nil {
		t.Fatal("expected a command from o")
	}
	msg, ok := cmd().(notify.ShowToastMsg)
	if !ok {
		t.Fatalf("expected ShowToastMsg, got %T", cmd())
	}
	if msg.Level != notify.Warn {
		t.Errorf("toast level = %v, want Warn", msg.Level)
	}
}
