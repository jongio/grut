package filetree

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

// blobTreeURL builds a github.com URL for a file (blob) or directory (tree)
// pinned to sha. relPath is repo-relative; an empty or "." relPath refers to
// the repository root. Path segments are percent-escaped while slashes are
// preserved. Returns "" when the remote is not a github.com remote or when sha
// is empty.
func blobTreeURL(remoteURL, sha, relPath string, isDir bool) string {
	base := git.RemoteToHTTPS(remoteURL)
	if base == "" || sha == "" {
		return ""
	}
	if !strings.HasPrefix(base, "https://github.com/") {
		return ""
	}
	rel := filepath.ToSlash(relPath)
	if rel == "" || rel == "." {
		return fmt.Sprintf("%s/tree/%s", base, sha)
	}
	segments := strings.Split(rel, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	escaped := strings.Join(segments, "/")
	kind := "blob"
	if isDir {
		kind = "tree"
	}
	return fmt.Sprintf("%s/%s/%s/%s", base, kind, sha, escaped)
}

// originRemoteURL returns the origin remote URL for the repo at root, or "".
func originRemoteURL(ctx context.Context, root string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// headSHA returns the full HEAD commit SHA for the repo at root, or "".
func headSHA(ctx context.Context, root string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// openOnGitHub opens the selected file (blob) or directory (tree) on
// github.com in the default browser, pinned to the current HEAD commit. It is a
// no-op when nothing is selected and shows a clear toast when the repository
// has no github remote.
func (ft *FileTree) openOnGitHub() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil {
		return ft, nil
	}
	path := n.path
	isDir := n.isDir
	gc := ft.gitClient
	ctx := ft.safeCtx()
	return ft, func() tea.Msg {
		if gc == nil {
			return notify.ShowToastMsg{Message: "No git repository", Level: notify.Warn}
		}
		root, err := gc.RepoRoot(ctx)
		if err != nil || root == "" {
			return notify.ShowToastMsg{Message: "No git repository", Level: notify.Warn}
		}
		relPath := path
		if rel, err := filepath.Rel(root, path); err == nil {
			relPath = filepath.ToSlash(rel)
		}
		if strings.HasPrefix(relPath, "..") {
			return notify.ShowToastMsg{Message: "Entry is outside the repository", Level: notify.Warn}
		}
		link := blobTreeURL(originRemoteURL(ctx, root), headSHA(ctx, root), relPath, isDir)
		if link == "" {
			return notify.ShowToastMsg{Message: "No github remote to open", Level: notify.Warn}
		}
		if err := panels.OpenInBrowser(ctx, link); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened on GitHub", Level: notify.Success}
	}
}
