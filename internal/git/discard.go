package git

import (
	"context"
	"fmt"
)

// DiscardFile discards unstaged changes for a single file, restoring it to
// the index state. For untracked files use clean; this only handles tracked
// modifications.
func (c *Client) DiscardFile(ctx context.Context, path string) error {
	if err := ValidateRepoRelativePath(path); err != nil {
		return fmt.Errorf("discard file path: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "checkout", "--", path)
		if err != nil {
			return fmt.Errorf("discard file: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// DiscardAllUnstaged discards all unstaged changes in the working tree.
func (c *Client) DiscardAllUnstaged(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "checkout", "--", ".")
		if err != nil {
			return fmt.Errorf("discard all unstaged: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}
