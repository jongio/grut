package commits

import (
	"context"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// commitURL builds a github.com URL for a commit given a raw git remote URL
// and a commit hash. It returns "" when the remote is not a github.com remote
// or when the hash is empty.
func commitURL(remoteURL, hash string) string {
	base := git.RemoteToHTTPS(remoteURL)
	if base == "" || hash == "" {
		return ""
	}
	if !strings.HasPrefix(base, "https://github.com/") {
		return ""
	}
	return base + "/commit/" + hash
}

// originRemoteURL returns the origin remote URL for the repo at root, or "".
func originRemoteURL(ctx context.Context, root string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// openCommitOnGitHub opens the selected commit's github.com page in the
// browser. It is a no-op when no commit is selected and shows a clear toast
// when the repository has no github remote.
func (p *Panel) openCommitOnGitHub() (panels.Panel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= p.activeLen() {
		return p, nil
	}
	c := p.commitAt(p.cursor)
	if c.Hash == "" {
		return p, nil
	}
	hash := c.Hash
	gc := p.gitClient
	remoteURL := p.remoteURL
	ctx := p.ctx
	return p, func() tea.Msg {
		if gc == nil {
			return notify.ShowToastMsg{Message: "No git repository", Level: notify.Warn}
		}
		root, err := gc.RepoRoot(ctx)
		if err != nil || root == "" {
			return notify.ShowToastMsg{Message: "No git repository", Level: notify.Warn}
		}
		link := commitURL(remoteURL(ctx, root), hash)
		if link == "" {
			return notify.ShowToastMsg{Message: "No github remote to open", Level: notify.Warn}
		}
		if err := panels.OpenInBrowser(ctx, link); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened commit on GitHub", Level: notify.Success}
	}
}
