package git

import (
	"context"
	"fmt"
)

// Revert creates a new commit that undoes the changes introduced by the given
// commit hash. If the revert causes conflicts, the user must resolve them and
// call RevertContinue, or call RevertAbort to cancel.
func (c *Client) Revert(ctx context.Context, hash string) error {
	if err := ValidateRef(hash); err != nil {
		return fmt.Errorf("revert hash: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "revert", "--no-edit", hash)
		if err != nil {
			return fmt.Errorf("revert: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// RevertContinue continues a revert after conflicts have been resolved.
func (c *Client) RevertContinue(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "revert", "--continue")
		if err != nil {
			return fmt.Errorf("revert continue: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// RevertAbort aborts an in-progress revert and restores the previous state.
func (c *Client) RevertAbort(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "revert", "--abort")
		if err != nil {
			return fmt.Errorf("revert abort: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}
