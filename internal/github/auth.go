package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	gh "github.com/google/go-github/v89/github"
)

// ghCLITimeout is the maximum time to wait for the `gh` CLI to return an
// auth token. This prevents the application from hanging indefinitely when
// the CLI is installed but unresponsive (e.g. waiting for keyring unlock).
const ghCLITimeout = 10 * time.Second

// NewClient creates a new GitHub API client. It resolves authentication
// by first trying `gh auth token` (GitHub CLI), then falling back to
// the GITHUB_TOKEN environment variable.
func NewClient(ctx context.Context) (*clientImpl, error) {
	token, err := resolveToken(ctx)
	if err != nil {
		return nil, err
	}
	ghClient, err := gh.NewClient(gh.WithAuthToken(token))
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}
	return &clientImpl{gh: ghClient, cache: newCache()}, nil
}

// resolveToken discovers a GitHub personal access token from available sources.
func resolveToken(ctx context.Context) (string, error) {
	// 1. Try gh auth token (GitHub CLI) with a bounded timeout so we
	// never hang waiting for an unresponsive CLI (CWE-400).
	ghCtx, cancel := context.WithTimeout(ctx, ghCLITimeout)
	defer cancel()
	out, err := exec.CommandContext(ghCtx, "gh", "auth", "token").Output()
	if err == nil {
		token := strings.TrimSpace(string(out))
		if token != "" {
			return token, nil
		}
	}
	// 2. Fallback to GITHUB_TOKEN environment variable
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("no GitHub auth: run 'gh auth login' or set GITHUB_TOKEN")
}
