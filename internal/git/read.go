package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WorktreeFileMaxBytes caps how much of a working-tree file is read, so a very
// large file cannot exhaust memory during a pre-stage scan.
const WorktreeFileMaxBytes = 5 << 20 // 5 MiB

// WorktreeFile reads the working-tree content of a repository-relative path.
// It exists so callers can inspect a file before it is staged. The path is
// validated to reject absolute paths and directory traversal, the resolved
// path is confirmed to stay within the repository, and the read is capped at
// WorktreeFileMaxBytes.
func (c *Client) WorktreeFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateRepoRelativePath(path); err != nil {
		return nil, fmt.Errorf("worktree file %q: %w", path, err)
	}

	root, err := filepath.Abs(c.repoDir)
	if err != nil {
		return nil, fmt.Errorf("worktree file: %w", err)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("worktree file %q: path escapes repository", path)
	}

	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("worktree file %q: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, WorktreeFileMaxBytes))
	if err != nil {
		return nil, fmt.Errorf("worktree file %q: %w", path, err)
	}
	return data, nil
}
