package git

import (
	"context"
	"fmt"
	"strings"
)

// DiffTreeFiles returns the list of files changed in a commit.
func (c *Client) DiffTreeFiles(ctx context.Context, hash string) ([]string, error) {
	if err := ValidateRef(hash); err != nil {
		return nil, fmt.Errorf("diff-tree hash: %w", err)
	}

	out, err := c.run(ctx, "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", hash)
	if err != nil {
		return nil, fmt.Errorf("diff-tree: %w", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return []string{}, nil
	}

	var files []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
