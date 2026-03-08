package git

import (
	"context"
	"fmt"
)

// Merge merges a branch into the current branch.
func (c *Client) Merge(ctx context.Context, branch string, opts MergeOpts) error {
	if err := ValidateRef(branch); err != nil {
		return fmt.Errorf("merge branch: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		args := []string{"merge", branch}
		if opts.NoFF {
			args = append(args, "--no-ff")
		}
		if opts.FFOnly {
			args = append(args, "--ff-only")
		}
		if opts.Squash {
			args = append(args, "--squash")
		}
		if opts.Message != "" {
			args = append(args, "-m", opts.Message)
		}

		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("merge: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// Rebase rebases the current branch onto the given ref.
func (c *Client) Rebase(ctx context.Context, onto string, opts RebaseOpts) error {
	if err := ValidateRef(onto); err != nil {
		return fmt.Errorf("rebase onto: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		args := []string{"rebase", onto}
		if opts.Interactive {
			args = append(args, "--interactive")
		}

		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("rebase: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// RebaseContinue continues a rebase in progress.
func (c *Client) RebaseContinue(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "rebase", "--continue")
		if err != nil {
			return fmt.Errorf("rebase continue: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// RebaseAbort aborts a rebase in progress.
func (c *Client) RebaseAbort(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "rebase", "--abort")
		if err != nil {
			return fmt.Errorf("rebase abort: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// CherryPick applies a commit by hash to the current branch.
func (c *Client) CherryPick(ctx context.Context, commitHash string) error {
	if err := ValidateRef(commitHash); err != nil {
		return fmt.Errorf("cherry-pick hash: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "cherry-pick", commitHash)
		if err != nil {
			return fmt.Errorf("cherry-pick: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// MergeAbort aborts a merge in progress.
func (c *Client) MergeAbort(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "merge", "--abort")
		if err != nil {
			return fmt.Errorf("merge abort: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}
