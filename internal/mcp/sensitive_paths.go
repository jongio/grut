package mcp

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// IsSensitivePath reports whether the given path (relative to the repo root)
// points to a file that should never be read or written through MCP or chat
// file operations. This blocks access to credential stores, secret keys, and
// internal git metadata while still allowing benign dot-files such as
// .gitignore and .gitattributes.
func IsSensitivePath(path string) error {
	// Normalise to forward slashes for consistent matching.
	normalized := filepath.ToSlash(path)

	// On Windows, strip NTFS alternate data stream markers (":stream_name")
	// and trailing dots/spaces from path components. The OS silently strips
	// these, allowing bypasses like ".env." or "id_rsa:$DATA" (CWE-41).
	if runtime.GOOS == "windows" {
		normalized = stripNTFSArtifacts(normalized)
	}

	// Normalize Unicode homoglyphs (e.g. Cyrillic 'е' U+0435 vs Latin 'e')
	// that could bypass sensitive pattern detection (CWE-176).
	asciiNormalized := foldToASCII(normalized)

	// Block .git/ internals (except a small allowlist of benign root files).
	if isGitInternalPath(normalized) || isGitInternalPath(asciiNormalized) {
		return fmt.Errorf("access to .git internals is not allowed: %s", path)
	}

	// Block sensitive file patterns by basename — check both original and
	// ASCII-folded forms to catch homoglyph attacks.
	base := filepath.Base(normalized)
	asciiBase := filepath.Base(asciiNormalized)
	if isSensitiveBasename(base) || isSensitiveBasename(asciiBase) {
		return fmt.Errorf("access to sensitive file is not allowed: %s", path)
	}

	// Block known credential subpaths (e.g. .docker/config.json).
	lower := strings.ToLower(normalized)
	asciiLower := strings.ToLower(asciiNormalized)
	for _, sub := range sensitiveSubpaths {
		if strings.HasSuffix(lower, sub) || strings.Contains(lower, sub+"/") {
			return fmt.Errorf("access to sensitive file is not allowed: %s", path)
		}
		if strings.HasSuffix(asciiLower, sub) || strings.Contains(asciiLower, sub+"/") {
			return fmt.Errorf("access to sensitive file is not allowed: %s", path)
		}
	}

	// Block any path component that is a sensitive directory.
	parts := strings.Split(lower, "/")
	for _, part := range parts {
		if sensitiveDirectories[part] {
			return fmt.Errorf("access to sensitive directory is not allowed: %s", path)
		}
	}
	asciiParts := strings.Split(asciiLower, "/")
	for _, part := range asciiParts {
		if sensitiveDirectories[part] {
			return fmt.Errorf("access to sensitive directory is not allowed: %s", path)
		}
	}

	return nil
}

// stripNTFSArtifacts removes NTFS alternate data stream markers and trailing
// dots/spaces from each path component. On Windows, "file.txt:$DATA" resolves
// to "file.txt" and ".env." resolves to ".env", enabling bypass of string-based
// sensitive path detection (CWE-41).
func stripNTFSArtifacts(slashPath string) string {
	parts := strings.Split(slashPath, "/")
	for i, part := range parts {
		// Strip ADS marker — everything from the first ':' in the filename.
		if idx := strings.IndexByte(part, ':'); idx >= 0 {
			part = part[:idx]
		}
		// Trim trailing dots and spaces (Windows silently strips these).
		part = strings.TrimRight(part, ". ")
		parts[i] = part
	}
	return strings.Join(parts, "/")
}

// foldToASCII replaces non-ASCII characters with nothing, producing a
// stripped ASCII-only version for comparison against sensitive patterns.
// This defeats Unicode homoglyph attacks (e.g. Cyrillic 'е' U+0435 posing
// as Latin 'e') by ensuring sensitive pattern matching operates on ASCII.
func foldToASCII(s string) string {
	return strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII {
			return -1 // drop non-ASCII
		}
		return r
	}, s)
}

// isGitInternalPath returns true for paths inside .git/ that are NOT in the
// benign allowlist (.gitignore, .gitattributes, .gitmodules).
func isGitInternalPath(slashPath string) bool {
	// Exact ".git" directory itself.
	if slashPath == ".git" { //nolint:goconst // inline string is more readable here
		return true
	}
	// Anything under .git/ prefix.
	if strings.HasPrefix(slashPath, ".git/") {
		return true
	}
	// Handle absolute-style or nested .git paths (e.g. repo/.git/config).
	parts := strings.Split(slashPath, "/")
	for i, part := range parts {
		if part == ".git" && i < len(parts)-1 {
			return true
		}
	}
	return false
}

// benignGitFiles are dot-prefixed git files that live in the repo root and
// are safe to read (they never contain credentials).
var benignGitFiles = map[string]bool{
	".gitignore":     true,
	".gitattributes": true,
	".gitmodules":    true,
	".gitkeep":       true,
}

// sensitiveExtensions are file extensions associated with private keys and
// certificates.
var sensitiveExtensions = map[string]bool{
	".pem": true,
	".key": true,
	".p12": true,
	".pfx": true,
	".crt": true,
}

// sensitiveExactNames are exact basenames of files that must be blocked.
var sensitiveExactNames = map[string]bool{
	"id_rsa":           true,
	"id_ed25519":       true,
	"id_ecdsa":         true,
	".npmrc":           true,
	".netrc":           true,
	".pgpass":          true,
	".my.cnf":          true,
	".htpasswd":        true,
	"wp-config.php":    true,
	"shadow":           true,
	"passwd":           true,
	"credentials.json": true,
}

// sensitiveDirectories are directory names (lowercased) that contain
// credentials and must be blocked entirely.
var sensitiveDirectories = map[string]bool{
	".docker": true,
	".kube":   true,
	".aws":    true,
	".azure":  true,
	".gcloud": true,
	".ssh":    true,
}

// sensitiveSubpaths are path segments (lowercased, forward-slashed) that
// indicate credential files inside known config directories.
var sensitiveSubpaths = []string{
	".docker/config.json",
	".kube/config",
	".aws/credentials",
}

// isSensitiveBasename checks the file's base name against known sensitive
// patterns: .env files, private key files, SSH identity files, and
// credential stores.
func isSensitiveBasename(base string) bool {
	lower := strings.ToLower(base)

	// Allow benign git root files.
	if benignGitFiles[lower] {
		return false
	}

	// .env or .env.* pattern.
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}

	// Private key / certificate extensions.
	ext := strings.ToLower(filepath.Ext(base))
	if sensitiveExtensions[ext] {
		return true
	}

	// SSH identity files and other exact-name matches.
	if sensitiveExactNames[lower] {
		return true
	}

	// Sensitive directory names (e.g. .docker, .kube, .aws).
	if sensitiveDirectories[lower] {
		return true
	}

	return false
}
