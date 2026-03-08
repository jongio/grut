package git

import (
	"context"
	"fmt"
)

// ResetMode specifies how git reset affects the index and working tree.
type ResetMode string

const (
	ResetSoft  ResetMode = "soft"
	ResetMixed ResetMode = "mixed"
	ResetHard  ResetMode = "hard"
)

// Reset moves the current branch tip to ref and optionally updates the index
// and working tree based on mode:
//   - soft:  HEAD only — staged changes and working tree are untouched
//   - mixed: HEAD + index — staged changes become unstaged (default git behavior)
//   - hard:  HEAD + index + working tree — all changes are discarded
func (c *Client) Reset(ctx context.Context, ref string, mode ResetMode) error {
	if err := ValidateRef(ref); err != nil {
		return fmt.Errorf("reset ref: %w", err)
	}

	flag, err := resetFlag(mode)
	if err != nil {
		return err
	}

	return c.queue.Exec(ctx, func() error {
		_, err := c.run(ctx, "reset", flag, ref)
		if err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// resetFlag converts a ResetMode to the corresponding git flag.
func resetFlag(mode ResetMode) (string, error) {
	switch mode {
	case ResetSoft:
		return "--soft", nil
	case ResetMixed:
		return "--mixed", nil
	case ResetHard:
		return "--hard", nil
	default:
		return "", fmt.Errorf("reset: invalid mode %q (must be soft, mixed, or hard)", mode)
	}
}
