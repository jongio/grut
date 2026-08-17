package ai

import (
	"path/filepath"
	"regexp"
	"strings"
)

// RedactedPlaceholder is the replacement text for detected secrets.
const RedactedPlaceholder = "[REDACTED]"

// RedactionFailedPlaceholder is returned instead of the original content when
// redaction fails. This ensures fail-closed behavior: secrets are never leaked
// to AI providers even when the redactor encounters an error (CWE-200).
const RedactionFailedPlaceholder = "[redaction failed]"

// builtinFilePatterns lists file-name globs that must never be forwarded to
// an AI provider. They use filepath.Match syntax and are matched against the
// base name of each path.
var builtinFilePatterns = []string{
	patternDotEnv,
	".env.*",
	patternKeyFile,
	"*.pem",
	"*.p12",
	"*.pfx",
	"*.secret",
	"*.secrets",
	patternIDRSA,
	patternIDEd25519,
	patternIDECDSA,
}

// builtinSecretRegexps detect common secret formats inside file content.
// They are compiled once during package initialization and never mutated.
// Ordering matters: more specific patterns run first so that broader
// patterns do not inflate the count.
type secretRegexpSet [17]*regexp.Regexp

var builtinSecretRegexps = secretRegexpSet{
	// AWS access key IDs (exactly AKIA + 16 uppercase-alphanumeric chars).
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),

	// Generic API key / secret assignments
	// (e.g. api_key = "sk_live_…", API-SECRET: "…").
	// Separators are restricted to tabs and spaces rather than \s so that a
	// keyword with an empty value cannot reach across a line break and consume
	// the following line. See the note on the high-entropy pattern below.
	regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret)['":\t ]*[=:][\t ]*['"]?[a-zA-Z0-9_\-]{20,}['"]?`),

	// PEM private-key blocks, including the BEGIN/END markers.
	// The optional prefix (e.g. "RSA ", "EC ") uses (?:[A-Z]+ )* so that
	// both "BEGIN RSA PRIVATE KEY" and bare "BEGIN PRIVATE KEY" are caught.
	regexp.MustCompile(`-----BEGIN (?:[A-Z]+ )*PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z]+ )*PRIVATE KEY-----`),

	// Database and message-broker connection strings containing credentials.
	regexp.MustCompile(`(?i)(mongodb|postgres(?:ql)?|mysql|redis|amqp):\/\/[^\s'"]+`),

	// GitHub personal / OAuth / user / app / refresh tokens.
	regexp.MustCompile(`(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}`),

	// High-entropy values assigned to common secret-related keywords.
	// The value class uses [^\s'"]+ to capture passwords containing special
	// characters (e.g. @, !, #) that are common in real-world secrets.
	// Minimum length is 6 (not 16) because passwords can be short.
	//
	// Separators and the value are both confined to a single line. Using \s
	// here let "DB_PASSWORD=\nJWT_SECRET=change-me" match across the newline,
	// so an empty value consumed the entire following line and deleted it
	// rather than redacting a value. Matching must never span lines.
	regexp.MustCompile(`(?i)(password|passwd|secret|token|access[_-]?key|private[_-]?key|auth[_-]?token)['":\t ]*[=:][\t ]*['"]?[^\s'"]{6,}['"]?`),

	// Credentials embedded in HTTPS URLs (e.g. https://user:token@host).
	regexp.MustCompile(`https?://[a-zA-Z0-9_\-]+:[a-zA-Z0-9_\-]+@[^\s'"]+`),

	// GitHub personal access tokens embedded in URLs (ghp_xxx@github.com).
	regexp.MustCompile(`https?://ghp_[a-zA-Z0-9_]{36,}@[^\s'"]+`),

	// Azure Storage account keys (connection strings containing AccountKey).
	regexp.MustCompile(`DefaultEndpointsProtocol=.*AccountKey=[A-Za-z0-9+/=]{86,}`),

	// Azure SAS tokens (query-string signature parameter).
	regexp.MustCompile(`[?&]sig=[A-Za-z0-9%+/=]+`),

	// Slack bot/user/app tokens.
	regexp.MustCompile(`xox[bpsa]-[0-9A-Za-z-]+`),

	// Stripe live keys (secret and restricted).
	regexp.MustCompile(`[sr]k_live_[0-9a-zA-Z]{24,}`),

	// npm authentication tokens.
	regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),

	// JWT tokens (three base64url-encoded segments).
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+`),

	// Google Cloud API keys.
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),

	// SendGrid API keys.
	regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),

	// Twilio API keys.
	regexp.MustCompile(`SK[a-f0-9]{32}`),
}

// Redactor sanitises content before it reaches AI providers, removing
// secrets and excluding sensitive files.
type Redactor struct {
	filePatterns  []string        // filepath.Match patterns for file exclusion
	secretRegexps secretRegexpSet // compiled content-level secret patterns
	forceErr      error           // test-only: when non-nil, RedactContent returns this error
}

// NewRedactor creates a Redactor from the given user-supplied file-exclusion
// patterns, merged with the built-in patterns that are always active.
func NewRedactor(patterns []string) *Redactor {
	// Merge built-in patterns with user-supplied patterns, deduplicating.
	seen := make(map[string]struct{}, len(builtinFilePatterns)+len(patterns))
	merged := make([]string, 0, len(builtinFilePatterns)+len(patterns))

	for _, p := range builtinFilePatterns {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			merged = append(merged, p)
		}
	}
	for _, p := range patterns {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			merged = append(merged, p)
		}
	}

	return &Redactor{
		filePatterns:  merged,
		secretRegexps: builtinSecretRegexps,
	}
}

// Redactor returns the configured redactor used by the builder.
func (b *Builder) Redactor() *Redactor {
	if b == nil {
		return nil
	}
	return b.redactor
}

// ShouldExcludeFile reports whether the given file path matches any exclusion
// pattern and should not be sent to AI. Matching is performed against the base
// name of the path, so "config/.env.local" is caught by the ".env.*" pattern.
func (r *Redactor) ShouldExcludeFile(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range r.filePatterns {
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
	}
	return false
}

// RedactContent scans content for secret patterns and replaces each match
// with [REDACTED]. It returns the sanitised content, the total number of
// redactions performed, and any error encountered during redaction.
func (r *Redactor) RedactContent(content string) (string, int, error) {
	if r.forceErr != nil {
		return "", 0, r.forceErr
	}

	count := 0
	result := content

	for _, re := range r.secretRegexps {
		var replacements int
		result, replacements = replaceMatches(re, result)
		count += replacements
	}

	return result, count, nil
}

func replaceMatches(re *regexp.Regexp, content string) (string, int) {
	matches := re.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, 0
	}

	var result strings.Builder
	result.Grow(len(content))
	last := 0
	for _, match := range matches {
		result.WriteString(content[last:match[0]])
		result.WriteString(RedactedPlaceholder)
		last = match[1]
	}
	result.WriteString(content[last:])
	return result.String(), len(matches)
}
