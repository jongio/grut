package git

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client wraps the git CLI and implements GitClient.
// It shells out to the git binary for all operations.
type Client struct {
	queue   *OpQueue
	cache   *Cache
	repoDir string
}

const maxGitOutputSize = 50 * 1024 * 1024 // 50 MiB
// limitedBuffer wraps bytes.Buffer with a size cap to prevent OOM from
// unexpectedly large git output (e.g., diffing large binary files).
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.buf.Len()+len(p) > lb.max {
		return 0, fmt.Errorf("git output exceeded %d bytes", lb.max)
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) String() string {
	return lb.buf.String()
}

// Ensure Client implements GitClient at compile time.
var _ GitClient = (*Client)(nil)

// NewClient creates a new Client for the repository at repoDir.
// It verifies that git is installed and the directory exists.
func NewClient(repoDir string) (*Client, error) {
	if err := CheckGitInstalled(); err != nil {
		return nil, err
	}
	if repoDir == "" {
		return nil, fmt.Errorf("repoDir must not be empty")
	}
	return &Client{
		repoDir: repoDir,
		queue:   &OpQueue{},
		cache:   NewCache(0), // cache disabled by default; callers set maxAge
	}, nil
}

// NewClientWithCache creates a new Client with caching enabled.
func NewClientWithCache(repoDir string, cache *Cache) (*Client, error) {
	if err := CheckGitInstalled(); err != nil {
		return nil, err
	}
	if repoDir == "" {
		return nil, fmt.Errorf("repoDir must not be empty")
	}
	if cache == nil {
		cache = NewCache(0)
	}
	return &Client{
		repoDir: repoDir,
		queue:   &OpQueue{},
		cache:   cache,
	}, nil
}

// RepoDir returns the repository working directory.
func (c *Client) RepoDir() string {
	return c.repoDir
}

// InvalidateCache clears all cached data.
func (c *Client) InvalidateCache() {
	c.cache.Invalidate()
}

// run executes a git command and returns stdout.
// All git commands go through this method. Arguments are validated,
// context cancellation is respected, and stderr is captured for errors.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	// On Windows, file paths after "--" contain backslashes which
	// ValidateArg rejects. Convert to forward slashes (git handles both)
	// before validation, since exec.Command doesn't use a shell.
	normalized := make([]string, len(args))
	afterDash := false
	for i, a := range args {
		if a == "--" {
			afterDash = true
			normalized[i] = a
			continue
		}
		if afterDash {
			normalized[i] = filepath.ToSlash(a)
		} else {
			normalized[i] = a
		}
	}
	if err := validateArgs(normalized); err != nil {
		return "", fmt.Errorf("invalid git argument: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", normalized...)
	cmd.Dir = c.repoDir
	stdout := &limitedBuffer{max: maxGitOutputSize}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	slog.Debug("git exec", "args", normalized, "dir", c.repoDir)
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			return "", fmt.Errorf("git %s: %w", normalized[0], err)
		}
		return "", fmt.Errorf("git %s: %s: %w", normalized[0], errMsg, err)
	}
	return stdout.String(), nil
}

// splitLines splits s into non-empty, trimmed lines.
// Returns an empty slice (not nil) for empty input so JSON marshaling
// produces [] rather than null.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	raw := strings.Split(s, "\n")
	var lines []string
	for _, line := range raw {
		if trimmed := strings.TrimRight(line, "\r"); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	// CR-011: Ensure non-nil return so JSON marshaling produces []
	// rather than null when all lines are whitespace after trimming.
	if lines == nil {
		return []string{}
	}
	return lines
}

// RepoRoot returns the top-level directory of the repository.
func (c *Client) RepoRoot(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo root: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

// IsRepo checks whether the working directory is inside a git repository.
func (c *Client) IsRepo(ctx context.Context) (bool, error) {
	_, err := c.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		// Propagate context cancellation so callers can distinguish
		// "not a repo" from "operation was cancelled" (CWE-391).
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	return true, nil
}
