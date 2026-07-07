package git

import (
	"context"
	"fmt"
)

// FormatPatch returns the patch for a single commit, as produced by
// "git format-patch -1 --stdout <hash>". The result is a mailbox-formatted
// patch that can be reapplied elsewhere with "git am".
func (c *Client) FormatPatch(ctx context.Context, hash string) (string, error) {
	if err := ValidateRef(hash); err != nil {
		return "", fmt.Errorf("format-patch hash: %w", err)
	}
	out, err := c.run(ctx, "format-patch", "-1", "--stdout", hash)
	if err != nil {
		return "", fmt.Errorf("format-patch: %w", err)
	}
	return out, nil
}
