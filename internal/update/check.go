// Package update implements background version checking and self-updating
// for the grut CLI. Versions are fetched from GitHub Releases and
// verified via SHA-256 checksums before replacing the running binary.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// cacheTTL defines how long a version check result remains valid
	// before re-querying the GitHub API.
	cacheTTL = 24 * time.Hour

	// cacheFileName is the name of the update-check cache file stored
	// in the user's config directory.
	cacheFileName = "update-check.json"

	// apiTimeout limits the HTTP request duration for API calls.
	apiTimeout = 10 * time.Second

	// releaseURLTemplate formats a GitHub releases page URL from a
	// version string (without leading "v").
	releaseURLTemplate = "https://github.com/jongio/grut/releases/tag/v%s"

	// configDirPerm restricts the config directory to owner read/write/execute.
	configDirPerm = 0o700

	// cacheFilePerm restricts the cache file to owner read/write.
	cacheFilePerm = 0o600
)

// latestReleaseURL is the GitHub API endpoint for the latest release.
// Declared as var (not const) to enable test overrides via httptest.
var latestReleaseURL = "https://api.github.com/repos/jongio/grut/releases/latest"

// UpdateInfo describes an available update.
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
}

// updateCache is the on-disk JSON format for caching version check results.
type updateCache struct {
	CheckedAt      time.Time `json:"checkedAt"`
	LatestVersion  string    `json:"latestVersion"`
	CurrentVersion string    `json:"currentVersion"`
}

// githubRelease is the subset of the GitHub Releases API response used
// for version checking.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdate checks whether a newer version of grut is available
// on GitHub. It returns nil if the current version is up to date, is a
// dev build, or any error occurs. This function is safe to call from a
// goroutine — it never panics and silently returns nil on all errors.
func CheckForUpdate(currentVersion string) *UpdateInfo {
	if isDevVersion(currentVersion) {
		return nil
	}

	// Read cached result; return early if still valid.
	cache, cacheFile := readCache()
	if cache != nil &&
		cache.CurrentVersion == currentVersion &&
		time.Since(cache.CheckedAt) < cacheTTL {
		if CompareVersions(cache.LatestVersion, currentVersion) > 0 {
			return &UpdateInfo{
				CurrentVersion: currentVersion,
				LatestVersion:  cache.LatestVersion,
				ReleaseURL:     fmt.Sprintf(releaseURLTemplate, cache.LatestVersion),
			}
		}
		return nil
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		return nil
	}
	if err := validateVersion(latest); err != nil {
		return nil
	}

	// Persist the result for future invocations.
	writeCache(cacheFile, &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  latest,
		CurrentVersion: currentVersion,
	})

	if CompareVersions(latest, currentVersion) > 0 {
		return &UpdateInfo{
			CurrentVersion: currentVersion,
			LatestVersion:  latest,
			ReleaseURL:     fmt.Sprintf(releaseURLTemplate, latest),
		}
	}
	return nil
}

// isDevVersion returns true for development builds that should skip
// update checks entirely.
func isDevVersion(v string) bool {
	return v == "dev" || strings.HasPrefix(v, "dev-")
}

// CompareVersions compares two semantic version strings (e.g. "1.2.3").
// Returns -1 if a < b, 0 if a == b, 1 if a > b. A leading "v" prefix
// is stripped. Non-numeric parts are treated as 0.
func CompareVersions(a, b string) int {
	aParts := splitVersion(a)
	bParts := splitVersion(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := range maxLen {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// splitVersion splits a version string into numeric parts. A leading
// "v" is stripped. Non-numeric segments are treated as 0.
func splitVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			nums[i] = n
		}
	}
	return nums
}

// fetchLatestVersion queries the GitHub Releases API and returns the
// latest release version (without leading "v").
func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: apiTimeout}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := resolveGitHubToken(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("querying GitHub API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decoding release response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// cachePath returns the path to the update check cache file.
func cachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving config directory: %w", err)
	}
	return filepath.Join(dir, "grut", cacheFileName), nil
}

// readCache reads the cached update check result from disk. Returns nil
// on any error (missing file, corrupt JSON, etc.) along with the resolved
// cache path for subsequent writes.
func readCache() (*updateCache, string) {
	path, err := cachePath()
	if err != nil {
		return nil, ""
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path
	}

	var cache updateCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return nil, path
	}
	return &cache, path
}

// writeCache persists an update check result to disk. Errors are silently
// ignored because caching is best-effort.
func writeCache(path string, cache *updateCache) {
	if path == "" {
		p, err := cachePath()
		if err != nil {
			return
		}
		path = p
	}

	raw, err := json.Marshal(cache)
	if err != nil {
		return
	}

	// Ensure the config directory exists.
	if err := os.MkdirAll(filepath.Dir(path), configDirPerm); err != nil {
		return
	}

	_ = os.WriteFile(path, raw, cacheFilePerm)
}

// ghTokenTimeout limits how long we wait for `gh auth token`.
const ghTokenTimeout = 3 * time.Second

// resolveGitHubToken attempts to discover a GitHub token for authenticated
// API requests. Returns empty string if no token is available (caller falls
// back to unauthenticated). This avoids 403 rate-limit errors on the
// releases/latest endpoint (60 req/hour unauthenticated vs 5000 authenticated).
func resolveGitHubToken(ctx context.Context) string {
	// 1. Try gh auth token (GitHub CLI).
	ghCtx, cancel := context.WithTimeout(ctx, ghTokenTimeout)
	defer cancel()
	out, err := exec.CommandContext(ghCtx, "gh", "auth", "token").Output()
	if err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return token
		}
	}
	// 2. Fallback to environment variables.
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return ""
}
