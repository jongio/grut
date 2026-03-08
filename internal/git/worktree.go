package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// WorktreeList returns all worktrees for the repository.
func (c *Client) WorktreeList(ctx context.Context) ([]Worktree, error) {
	out, err := c.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("worktree list: %w", err)
	}
	return parseWorktreeList(out), nil
}

// parseWorktreeList parses "git worktree list --porcelain" output.
// Each worktree is separated by a blank line. Fields:
//
//	worktree <path>
//	HEAD <hash>
//	branch refs/heads/<name>
//	bare (optional)
func parseWorktreeList(output string) []Worktree {
	worktrees := make([]Worktree, 0)
	var current Worktree
	started := false

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")

		switch {
		case strings.HasPrefix(line, "worktree "):
			if started {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: filepath.FromSlash(strings.TrimPrefix(line, "worktree "))}
			started = true

		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")

		case strings.HasPrefix(line, "branch "):
			// Strip refs/heads/ prefix for branch name.
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")

		case line == "bare":
			current.Bare = true

		case line == "":
			if started {
				worktrees = append(worktrees, current)
				current = Worktree{}
				started = false
			}
		}
	}

	// Append the last one if not already added.
	if started {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// WorktreeAdd creates a new worktree at the given path for the specified branch.
func (c *Client) WorktreeAdd(ctx context.Context, path, branch string) error {
	if err := ValidatePath(path); err != nil {
		return fmt.Errorf("worktree add path: %w", err)
	}
	if branch != "" {
		if err := ValidateRef(branch); err != nil {
			return fmt.Errorf("worktree add branch: %w", err)
		}
	}

	args := []string{"worktree", "add", "--", path}
	if branch != "" {
		args = append(args, branch)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("worktree add: %w", err)
	}
	return nil
}

// WorktreePath computes the conventional filesystem path for a new worktree
// given the repository root and branch name.  It follows the grut convention:
//
//	<parent-of-repo>/.worktrees/<repo-name>/<branch-slug>
//
// where branch-slug replaces "/" with "-".
func WorktreePath(repoRoot, branch string) string {
	parent := filepath.Dir(repoRoot)
	repoName := filepath.Base(repoRoot)
	slug := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(parent, ".worktrees", repoName, slug)
}

// WorktreeRemove removes a worktree at the given path.
func (c *Client) WorktreeRemove(ctx context.Context, path string, force bool) error {
	if err := ValidatePath(path); err != nil {
		return fmt.Errorf("worktree remove path: %w", err)
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", path)

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("worktree remove: %w", err)
	}
	return nil
}
