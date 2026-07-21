package preview

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

const msgNoGitRepo = "No git repository"

// buildPermalink constructs a github.com blob permalink for relPath pinned to
// the given commit sha, with a line anchor. startLine and endLine are 1-based;
// when endLine <= startLine a single-line anchor (#L{start}) is used, otherwise
// a range anchor (#L{start}-L{end}). Path segments are percent-escaped while
// slashes are preserved. Returns "" when remoteURL is not a github.com remote
// or when sha is empty.
func buildPermalink(remoteURL, sha, relPath string, startLine, endLine int) string {
	base := git.RemoteToHTTPS(remoteURL)
	if base == "" || sha == "" {
		return ""
	}
	if !strings.HasPrefix(base, "https://github.com/") {
		return ""
	}
	segments := strings.Split(filepath.ToSlash(relPath), "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	escaped := strings.Join(segments, "/")
	link := fmt.Sprintf("%s/blob/%s/%s", base, sha, escaped)
	switch {
	case startLine <= 0:
		// no anchor
	case endLine > startLine:
		link += fmt.Sprintf("#L%d-L%d", startLine, endLine)
	default:
		link += fmt.Sprintf("#L%d", startLine)
	}
	return link
}

// permalinkLineRange returns the 1-based line range to anchor a permalink to.
// When text is selected the selection range is used; otherwise the top visible
// line is used as a single-line anchor.
func (p *Preview) permalinkLineRange() (int, int) {
	if s, e := p.selRange(); s != nil && e != nil {
		return s.Line + 1, e.Line + 1
	}
	line := p.scrollY + 1
	return line, line
}

// gitRemoteURL returns the origin remote URL for the repo at root, or "".
func gitRemoteURL(ctx context.Context, root string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitHeadSHA returns the full HEAD commit SHA for the repo at root, or "".
func gitHeadSHA(ctx context.Context, root string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// copyPermalink builds a github.com permalink for the current file and line
// (or selected range) and copies it to the clipboard. It is a no-op with a
// clear message when the preview is not showing an on-disk file or when the
// repository has no github remote.
func (p *Preview) copyPermalink() (panels.Panel, tea.Cmd) {
	if p.ghMode || p.filePath == "" {
		return p, nil
	}
	path := p.filePath
	gc := p.gitClient
	startLine, endLine := p.permalinkLineRange()
	return p, func() tea.Msg {
		ctx := context.Background()
		if gc == nil {
			return notify.ShowToastMsg{Message: msgNoGitRepo, Level: notify.Warn}
		}
		root, err := gc.RepoRoot(ctx)
		if err != nil || root == "" {
			return notify.ShowToastMsg{Message: msgNoGitRepo, Level: notify.Warn}
		}
		relPath := path
		if rel, err := filepath.Rel(root, path); err == nil {
			relPath = filepath.ToSlash(rel)
		}
		if strings.HasPrefix(relPath, "..") {
			return notify.ShowToastMsg{Message: "File is outside the repository", Level: notify.Warn}
		}
		link := buildPermalink(gitRemoteURL(ctx, root), gitHeadSHA(ctx, root), relPath, startLine, endLine)
		if link == "" {
			return notify.ShowToastMsg{Message: "No github remote for permalink", Level: notify.Warn}
		}
		if err := panels.CopyToClipboard(ctx, link); err != nil {
			return notify.ShowToastMsg{Message: "Copy failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Copied GitHub permalink", Level: notify.Info}
	}
}

// openOnGitHub builds a github.com permalink for the current file and line
// (or selected range) and opens it in the default browser. It is a no-op with
// a clear message when the preview is not showing an on-disk file or when the
// repository has no github remote.
func (p *Preview) openOnGitHub() (panels.Panel, tea.Cmd) {
	if p.ghMode || p.filePath == "" {
		return p, nil
	}
	path := p.filePath
	gc := p.gitClient
	startLine, endLine := p.permalinkLineRange()
	return p, func() tea.Msg {
		ctx := context.Background()
		if gc == nil {
			return notify.ShowToastMsg{Message: msgNoGitRepo, Level: notify.Warn}
		}
		root, err := gc.RepoRoot(ctx)
		if err != nil || root == "" {
			return notify.ShowToastMsg{Message: msgNoGitRepo, Level: notify.Warn}
		}
		relPath := path
		if rel, err := filepath.Rel(root, path); err == nil {
			relPath = filepath.ToSlash(rel)
		}
		if strings.HasPrefix(relPath, "..") {
			return notify.ShowToastMsg{Message: "File is outside the repository", Level: notify.Warn}
		}
		link := buildPermalink(gitRemoteURL(ctx, root), gitHeadSHA(ctx, root), relPath, startLine, endLine)
		if link == "" {
			return notify.ShowToastMsg{Message: "No github remote to open", Level: notify.Warn}
		}
		if err := panels.OpenInBrowser(ctx, link); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened file on GitHub", Level: notify.Info}
	}
}
