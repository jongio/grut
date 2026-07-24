package preview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// createGist creates a secret github gist from the previewed file and copies
// the gist URL to the clipboard. It is a no-op when the preview is not showing
// an on-disk file (for example GitHub content or an empty view). Before
// uploading, it verifies the file resolves inside the repository so a symlink
// cannot exfiltrate an out-of-repo secret.
func (p *Preview) createGist() (panels.Panel, tea.Cmd) {
	if p.ghMode || p.filePath == "" {
		return p, nil
	}
	path := p.filePath
	gc := p.gitClient
	return p, func() tea.Msg {
		ctx := context.Background()
		if err := gistPathWithinRepo(ctx, gc, path); err != nil {
			return notify.ShowToastMsg{Message: "Gist blocked: " + err.Error(), Level: notify.Warn}
		}
		link, err := github.CreateGist(ctx, path)
		if err != nil {
			return notify.ShowToastMsg{Message: "Gist failed: " + err.Error(), Level: notify.Error}
		}
		if cerr := panels.CopyToClipboard(ctx, link); cerr != nil {
			return notify.ShowToastMsg{Message: "Created gist (copy failed): " + link, Level: notify.Warn}
		}
		return notify.ShowToastMsg{Message: "Created secret gist (URL copied)", Level: notify.Success}
	}
}

// gistPathWithinRepo verifies that path, after resolving symlinks, stays inside
// the repository root. A malicious repository can ship a symlink pointing at a
// local secret (e.g. ~/.ssh/id_rsa); without this guard `gh gist create` would
// follow it and publish the target file. When no git repository is available it
// refuses symlinked files outright rather than guessing a boundary.
func gistPathWithinRepo(ctx context.Context, gc git.StatusReader, path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if gc != nil {
		if root, rerr := gc.RepoRoot(ctx); rerr == nil && root != "" {
			resolvedRoot, rrerr := filepath.EvalSymlinks(root)
			if rrerr != nil {
				resolvedRoot = filepath.Clean(root)
			}
			rel, relErr := filepath.Rel(resolvedRoot, resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("file resolves outside the repository")
			}
			return nil
		}
	}
	// No repository context: only allow non-symlinked files.
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to upload a symlink")
	}
	return nil
}
