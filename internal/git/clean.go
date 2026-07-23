package git

import (
	"context"
	"fmt"
	"strings"
)

// CleanCandidate is a single path that "git clean" would remove.
type CleanCandidate struct {
	// Path is the repository-relative path, exactly as git reports it.
	// Directories carry a trailing slash.
	Path string
	// Ignored is true when the path is a candidate only because ignored
	// files were included (git clean -X), not a plain untracked file.
	Ignored bool
}

// CleanOpts controls a clean preview or removal.
type CleanOpts struct {
	// IncludeIgnored also considers files matched by .gitignore.
	IncludeIgnored bool
	// Paths restricts a removal to the given repository-relative paths.
	// It is ignored by CleanPreview.
	Paths []string
}

// forceCLocale pins git's human-readable output to English so the dry-run
// "Would remove" lines can be parsed regardless of the user's locale.
var forceCLocale = []string{"LC_ALL=C", "LANG=C"}

// CleanPreview runs "git clean" in dry-run mode and returns the paths that
// would be removed. Plain untracked candidates are listed first; when
// IncludeIgnored is set, ignored candidates are appended and marked Ignored.
func (c *Client) CleanPreview(ctx context.Context, opts CleanOpts) ([]CleanCandidate, error) {
	untracked, err := c.cleanDryRun(ctx, false)
	if err != nil {
		return nil, err
	}
	candidates := make([]CleanCandidate, 0, len(untracked))
	for _, p := range untracked {
		candidates = append(candidates, CleanCandidate{Path: p})
	}

	if opts.IncludeIgnored {
		ignored, err := c.cleanDryRun(ctx, true)
		if err != nil {
			return nil, err
		}
		for _, p := range ignored {
			candidates = append(candidates, CleanCandidate{Path: p, Ignored: true})
		}
	}
	return candidates, nil
}

// cleanDryRun runs "git clean -nd" (or "-ndX" for ignored-only) and returns
// the reported paths. Using -X (uppercase) for the ignored pass keeps ignored
// candidates cleanly separated from plain untracked ones.
func (c *Client) cleanDryRun(ctx context.Context, ignoredOnly bool) ([]string, error) {
	args := []string{gitCleanCommand, "-n", "-d"}
	if ignoredOnly {
		args = append(args, "-X")
	}
	out, err := c.runWithEnv(ctx, forceCLocale, args...)
	if err != nil {
		return nil, fmt.Errorf("clean preview: %w", err)
	}
	return parseCleanDryRun(out), nil
}

// parseCleanDryRun extracts the removable paths from "git clean -n" output.
// Under the C locale each removable entry is printed as "Would remove <path>".
// Lines like "Would skip repository <path>" are intentionally ignored because
// git will not remove nested repositories.
func parseCleanDryRun(out string) []string {
	const prefix = "Would remove "
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, prefix) {
			p := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// Clean removes the given untracked paths with "git clean -fd" (adding -x when
// IncludeIgnored is set). It deletes only the paths in opts.Paths; an empty
// list is a no-op so a removal can never wipe the whole working tree by
// accident. Every path is validated before it reaches git.
func (c *Client) Clean(ctx context.Context, opts CleanOpts) error {
	if len(opts.Paths) == 0 {
		return nil
	}
	for _, p := range opts.Paths {
		if err := ValidateRepoRelativePath(p); err != nil {
			return fmt.Errorf("clean path %q: %w", p, err)
		}
	}

	return c.queue.Exec(ctx, func() error {
		args := []string{"clean", "-f", "-d"}
		if opts.IncludeIgnored {
			args = append(args, "-x")
		}
		args = append(args, "--")
		args = append(args, opts.Paths...)
		if _, err := c.run(ctx, args...); err != nil {
			return fmt.Errorf("clean: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}
