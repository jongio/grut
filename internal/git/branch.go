package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// BranchList returns all local and remote branches.
func (c *Client) BranchList(ctx context.Context) ([]Branch, error) {
	// Use for-each-ref for structured output.
	format := strings.Join([]string{
		"%(refname:short)",
		"%(objectname:short)",
		"%(upstream:short)",
		"%(upstream:track)",
		"%(HEAD)",
		"%(symref)",
		"%(refname)",
	}, FieldSep)

	out, err := c.run(ctx, "for-each-ref",
		"--format="+format,
		"refs/heads/",
		"refs/remotes/",
	)
	if err != nil {
		return nil, fmt.Errorf("branch list: %w", err)
	}

	return parseBranchList(out)
}

// parseBranchList parses for-each-ref output into Branch entries.
func parseBranchList(output string) ([]Branch, error) {
	branches := make([]Branch, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		fields := strings.Split(line, FieldSep)
		if len(fields) < 7 {
			continue
		}

		// Skip symrefs (e.g. origin/HEAD -> origin/main) — they are
		// redundant pointers that duplicate the target branch.
		if fields[5] != "" {
			continue
		}

		b := Branch{
			Name:      fields[0],
			Hash:      fields[1],
			Upstream:  fields[2],
			IsCurrent: fields[4] == "*",
		}

		// Detect remote branches via the full refname prefix.
		if strings.HasPrefix(fields[6], "refs/remotes/") {
			b.IsRemote = true
		}

		// Parse ahead/behind from track info like "[ahead 3, behind 2]".
		b.Ahead, b.Behind = parseTrackInfo(fields[3])

		branches = append(branches, b)
	}
	return branches, nil
}

// parseTrackInfo parses "[ahead N]", "[behind N]", or "[ahead N, behind N]".
func parseTrackInfo(track string) (ahead, behind int) {
	track = strings.Trim(track, "[]")
	if track == "" || track == "gone" {
		return 0, 0
	}
	for _, part := range strings.Split(track, ", ") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimPrefix(part, "ahead "))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimPrefix(part, "behind "))
		}
	}
	return ahead, behind
}

// BranchCreate creates a new branch from base (or HEAD if base is empty).
func (c *Client) BranchCreate(ctx context.Context, name, base string) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("branch create name: %w", err)
	}

	args := []string{"branch", name}
	if base != "" {
		if err := ValidateRef(base); err != nil {
			return fmt.Errorf("branch create base: %w", err)
		}
		args = append(args, base)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("branch create: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// BranchDelete deletes a branch. Use force=true for -D (force delete).
func (c *Client) BranchDelete(ctx context.Context, name string, force bool) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("branch delete: %w", err)
	}

	flag := "-d"
	if force {
		flag = "-D"
	}

	_, err := c.run(ctx, "branch", flag, name)
	if err != nil {
		return fmt.Errorf("branch delete: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// BranchRename renames a branch from oldName to newName.
// If oldName is empty, renames the current branch.
func (c *Client) BranchRename(ctx context.Context, oldName, newName string) error {
	if err := ValidateRef(newName); err != nil {
		return fmt.Errorf("branch rename new: %w", err)
	}

	var args []string
	if oldName == "" {
		args = []string{"branch", "-m", newName}
	} else {
		if err := ValidateRef(oldName); err != nil {
			return fmt.Errorf("branch rename old: %w", err)
		}
		args = []string{"branch", "-m", oldName, newName}
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("branch rename: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// Checkout switches branches or restores working tree files.
func (c *Client) Checkout(ctx context.Context, ref string) error {
	if err := ValidateRef(ref); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	_, err := c.run(ctx, "checkout", ref)
	if err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// Stage adds files to the index.
func (c *Client) Stage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("stage: no paths provided")
	}
	for _, p := range paths {
		if err := ValidateRepoRelativePath(p); err != nil {
			return fmt.Errorf("stage path: %w", err)
		}
	}

	return c.queue.Exec(ctx, func() error {
		args := append([]string{"add", "--"}, paths...)
		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("stage: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// Unstage removes files from the index (resets to HEAD).
func (c *Client) Unstage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("unstage: no paths provided")
	}
	for _, p := range paths {
		if err := ValidateRepoRelativePath(p); err != nil {
			return fmt.Errorf("unstage path: %w", err)
		}
	}

	return c.queue.Exec(ctx, func() error {
		args := append([]string{"reset", "HEAD", "--"}, paths...)
		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("unstage: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// Commit creates a new commit with the given message and options.
// Returns the commit hash on success.
func (c *Client) Commit(ctx context.Context, msg string, opts CommitOpts) (string, error) {
	if msg == "" && !opts.Amend && opts.Fixup == "" {
		return "", fmt.Errorf("commit: message must not be empty")
	}

	var hash string
	err := c.queue.Exec(ctx, func() error {
		var args []string
		if opts.Fixup != "" {
			args = []string{"commit", "--fixup=" + opts.Fixup}
		} else {
			args = []string{"commit", "-m", msg}
		}
		if opts.AllowEmpty {
			args = append(args, "--allow-empty")
		}
		if opts.Amend {
			args = append(args, "--amend")
		}
		if opts.RewordOnly {
			args = append(args, "--only")
		}
		if opts.Sign {
			args = append(args, "-S")
		}
		if opts.Author != "" {
			args = append(args, "--author="+opts.Author)
		}

		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}

		// Get the hash of the commit just created.
		out, err := c.run(ctx, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("commit rev-parse: %w", err)
		}
		hash = strings.TrimSpace(out)
		c.cache.Invalidate()
		return nil
	})
	return hash, err
}
