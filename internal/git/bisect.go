package git

import (
	"context"
	"fmt"
	"strings"
)

// BisectStart begins a bisect session between a bad and good commit.
func (c *Client) BisectStart(ctx context.Context, bad, good string) error {
	if err := ValidateRef(bad); err != nil {
		return fmt.Errorf("bisect bad ref: %w", err)
	}
	if err := ValidateRef(good); err != nil {
		return fmt.Errorf("bisect good ref: %w", err)
	}

	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "bisect", "start", bad, good)
		if err != nil {
			return fmt.Errorf("bisect start: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// BisectGood marks the current revision as good. Returns the next revision to test
// or a message indicating the bisect is done.
func (c *Client) BisectGood(ctx context.Context) (string, error) {
	var result string
	err := c.queue.Exec(ctx, func() error {
		out, err := c.run(ctx, "bisect", "good")
		if err != nil {
			return fmt.Errorf("bisect good: %w", err)
		}
		result = strings.TrimSpace(out)
		c.cache.Invalidate()
		return nil
	})
	return result, err
}

// BisectBad marks the current revision as bad. Returns the next revision to test
// or a message indicating the bisect is done.
func (c *Client) BisectBad(ctx context.Context) (string, error) {
	var result string
	err := c.queue.Exec(ctx, func() error {
		out, err := c.run(ctx, "bisect", "bad")
		if err != nil {
			return fmt.Errorf("bisect bad: %w", err)
		}
		result = strings.TrimSpace(out)
		c.cache.Invalidate()
		return nil
	})
	return result, err
}

// BisectReset ends the bisect session and returns to the original HEAD.
func (c *Client) BisectReset(ctx context.Context) error {
	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "bisect", "reset")
		if err != nil {
			return fmt.Errorf("bisect reset: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}
