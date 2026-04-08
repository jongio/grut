package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// shellMetachars contains characters that could trigger shell interpretation.
// These MUST be rejected to prevent command injection in user-provided values.
// Note: braces {}, parens () are intentionally excluded because git uses them
// legitimately (stash@{0}, format strings %(refname), etc.). Since we use
// exec.Command (no shell), these are safe.
const shellMetachars = ";|&$`\\<>\n\r"

// ValidateArg rejects git arguments containing shell metacharacters.
// This prevents command injection when constructing git CLI commands.
// For user-supplied names (branch, tag, remote), also use ValidateRef
// which additionally rejects leading dashes to prevent option injection.
func ValidateArg(arg string) error {
	if arg == "" {
		return fmt.Errorf("argument must not be empty")
	}
	if !utf8.ValidString(arg) {
		return fmt.Errorf("argument contains invalid UTF-8")
	}
	// Reject leading dashes for user-supplied positional values to prevent
	// option injection (CWE-88). The standalone "--" separator is allowed.
	if arg != "--" && strings.HasPrefix(arg, "-") {
		return fmt.Errorf("argument must not start with '-' (option injection)")
	}
	hasAtBrace := strings.Contains(arg, "@{")
	hasFormatParen := strings.Contains(arg, "%(")
	for i, r := range arg {
		if r == 0 {
			return fmt.Errorf("argument contains null byte at position %d", i)
		}
		if strings.ContainsRune(shellMetachars, r) {
			return fmt.Errorf("argument contains forbidden character %q at position %d", r, i)
		}
		if unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("argument contains Unicode format character (bidi/ZWJ) at position %d", i)
		}
		// Reject parentheses unless part of a git format specifier %(...)
		// (e.g., %(refname:short), %(objectname)).
		if (r == '(' || r == ')') && !hasFormatParen {
			return fmt.Errorf("argument contains forbidden character %q at position %d", r, i)
		}
		// Reject braces unless part of a valid @{...} reflog pattern
		// (e.g., stash@{0}, HEAD@{0}).
		if (r == '{' || r == '}') && !hasAtBrace {
			return fmt.Errorf("argument contains forbidden character %q at position %d", r, i)
		}
	}
	return nil
}

// ValidateRef validates a git ref (branch name, tag, hash).
// Allows alphanumeric, dash, underscore, dot, slash, @, ~, ^, and colon.
// Leading dashes are rejected to prevent option injection (CWE-88).
func ValidateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref must not be empty")
	}
	if !utf8.ValidString(ref) {
		return fmt.Errorf("ref contains invalid UTF-8")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("ref must not start with '-' (option injection)")
	}
	for i, r := range ref {
		if r == 0 {
			return fmt.Errorf("ref contains null byte at position %d", i)
		}
		if strings.ContainsRune(shellMetachars, r) {
			return fmt.Errorf("ref contains forbidden character %q at position %d", r, i)
		}
		if unicode.IsControl(r) {
			return fmt.Errorf("ref contains control character at position %d", i)
		}
		if unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("ref contains Unicode format character (bidi/ZWJ) at position %d", i)
		}
	}
	// Git-specific ref rules:
	// - Cannot start or end with dot.
	// - Cannot contain "..", "~{", "@{", or "//".
	// - Cannot end with ".lock".
	if strings.HasPrefix(ref, ".") || strings.HasSuffix(ref, ".") {
		return fmt.Errorf("ref must not start or end with a dot")
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("ref must not contain '..'")
	}
	if strings.Contains(ref, "//") {
		return fmt.Errorf("ref must not contain '//'")
	}
	if strings.Contains(ref, "@{") {
		return fmt.Errorf("ref must not contain '@{'")
	}
	if strings.HasSuffix(ref, ".lock") {
		return fmt.Errorf("ref must not end with '.lock'")
	}
	return nil
}

// ValidatePath validates a file path for git operations.
// It rejects shell metacharacters, null bytes, and path traversal attempts.
// Absolute paths are allowed for read-only callers that intentionally accept
// repository-anchored absolute paths.
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	if !utf8.ValidString(path) {
		return fmt.Errorf("path contains invalid UTF-8")
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("path must not start with '-' (option injection)")
	}
	for i, r := range path {
		if r == 0 {
			return fmt.Errorf("path contains null byte at position %d", i)
		}
		// Paths allow spaces and backslashes (Windows paths), so use
		// a stricter subset of the shell metachars for paths.
		if strings.ContainsRune(";|&$`(){}<>\n\r", r) {
			return fmt.Errorf("path contains forbidden character %q at position %d", r, i)
		}
		if unicode.IsControl(r) && r != '\t' {
			return fmt.Errorf("path contains control character at position %d", i)
		}
		if unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("path contains Unicode format character (bidi/ZWJ) at position %d", i)
		}
	}
	if containsPathTraversal(path) {
		return fmt.Errorf("path must not contain '..'")
	}
	return nil
}

func containsPathTraversal(path string) bool {
	// Normalise backslashes so Windows-style paths are checked on all
	// platforms (filepath.ToSlash only converts the OS separator).
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// ValidateRepoRelativePath validates a repository-relative path for mutating
// git operations that must never accept absolute filesystem paths.
func ValidateRepoRelativePath(path string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must not be absolute")
	}
	// Catch Windows absolute paths (e.g. "C:\foo") even when running on
	// Linux where filepath.IsAbs does not recognise them.
	if len(path) >= 2 && path[1] == ':' &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
		return fmt.Errorf("path must not be absolute")
	}
	return nil
}

// validateArgs validates all arguments using ValidateArg.
func validateArgs(args []string) error {
	afterDash := false
	for _, arg := range args {
		if arg == "--" {
			afterDash = true
			continue
		}
		// Allow command options before the "--" separator.
		// User-supplied positional values are validated by specific callers
		// (ValidateRef / ValidatePath / ValidateRepoRelativePath).
		if !afterDash && strings.HasPrefix(arg, "-") {
			continue
		}
		// After "--", allow literal path arguments that may begin with "-".
		if afterDash && strings.HasPrefix(arg, "-") {
			if arg == "" {
				return fmt.Errorf("argument must not be empty")
			}
			if !utf8.ValidString(arg) {
				return fmt.Errorf("argument contains invalid UTF-8")
			}
			for i, r := range arg {
				if r == 0 {
					return fmt.Errorf("argument contains null byte at position %d", i)
				}
				if strings.ContainsRune(shellMetachars, r) {
					return fmt.Errorf("argument contains forbidden character %q at position %d", r, i)
				}
			}
			continue
		}
		if err := ValidateArg(arg); err != nil {
			return err
		}
	}
	return nil
}

// validateAuthor checks that an author string ("Name <email>") is safe for
// use in --author=. It rejects shell metacharacters except angle brackets
// (which git requires for the email wrapper), null bytes, control characters,
// and Unicode format characters. Leading dashes are rejected to prevent
// option injection (CWE-88).
func validateAuthor(author string) error {
	if author == "" {
		return fmt.Errorf("author must not be empty")
	}
	if !utf8.ValidString(author) {
		return fmt.Errorf("author contains invalid UTF-8")
	}
	if strings.HasPrefix(author, "-") {
		return fmt.Errorf("author must not start with '-' (option injection)")
	}
	// authorForbidden is the shell metachar set minus < and >, which git
	// --author needs for "Name <email>" format.
	const authorForbidden = ";|&$`\\\n\r"
	for i, r := range author {
		if r == 0 {
			return fmt.Errorf("author contains null byte at position %d", i)
		}
		if strings.ContainsRune(authorForbidden, r) {
			return fmt.Errorf("author contains forbidden character %q at position %d", r, i)
		}
		if unicode.IsControl(r) && r != '\t' {
			return fmt.Errorf("author contains control character at position %d", i)
		}
		if unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("author contains Unicode format character at position %d", i)
		}
	}
	return nil
}

// CheckGitInstalled verifies git is available on PATH.
func CheckGitInstalled() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found on PATH — install: https://git-scm.com/downloads")
	}
	return nil
}
