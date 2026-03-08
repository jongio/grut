package config

import (
	"context"
	"os/exec"
	"strings"
)

// ResolveGitHubRepo returns the owner and repo for GitHub API calls.
// If config values are set, those are used. Otherwise, auto-detects from
// the git remote origin URL.
func (c *GitHubConfig) ResolveGitHubRepo(ctx context.Context, repoRoot string) (owner, repo string) {
	if c.Owner != "" && c.Repo != "" {
		return c.Owner, c.Repo
	}
	// Auto-detect from git remote.
	out, err := exec.CommandContext(ctx, "git", "-C", repoRoot, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", ""
	}
	return parseGitHubRemote(strings.TrimSpace(string(out)))
}

// parseGitHubRemote extracts owner/repo from a GitHub remote URL.
// Handles:
//
//	https://github.com/owner/repo.git
//	git@github.com:owner/repo.git
func parseGitHubRemote(url string) (owner, repo string) {
	for _, sep := range []string{"github.com/", "github.com:"} {
		if o, r, ok := extractOwnerRepo(url, sep); ok {
			return o, r
		}
	}
	return "", ""
}

// extractOwnerRepo splits url on sep and parses the owner/repo from the
// remainder. Returns ok=false if the URL does not contain sep or lacks
// the expected segments.
func extractOwnerRepo(url, sep string) (owner, repo string, ok bool) {
	parts := strings.Split(url, sep)
	if len(parts) != 2 {
		return "", "", false
	}
	path := strings.TrimSuffix(parts[1], ".git")
	segments := strings.SplitN(path, "/", 3)
	if len(segments) < 2 {
		return "", "", false
	}
	return segments[0], segments[1], true
}
