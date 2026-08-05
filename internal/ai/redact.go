package ai

import (
	"path/filepath"
	"regexp"
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

// builtinSecretPatterns are regular expressions that detect common secret
// formats inside file content. They are compiled once in NewRedactor and
// applied sequentially by RedactContent. Ordering matters: more specific
// patterns run first so that broader patterns do not inflate the count.
var builtinSecretPatterns = []string{
	// AWS access key IDs (exactly AKIA + 16 uppercase-alphanumeric chars).
	`AKIA[0-9A-Z]{16}`,

	// Generic API key / secret assignments
	// (e.g. api_key = "sk_live_…", API-SECRET: "…").
	// Separators are restricted to tabs and spaces rather than \s so that a
	// keyword with an empty value cannot reach across a line break and consume
	// the following line. See the note on the high-entropy pattern below.
	`(?i)(api[_-]?key|apikey|api[_-]?secret)['":\t ]*[=:][\t ]*['"]?[a-zA-Z0-9_\-]{20,}['"]?`,

	// PEM private-key blocks, including the BEGIN/END markers.
	// The optional prefix (e.g. "RSA ", "EC ") uses (?:[A-Z]+ )* so that
	// both "BEGIN RSA PRIVATE KEY" and bare "BEGIN PRIVATE KEY" are caught.
	`-----BEGIN (?:[A-Z]+ )*PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z]+ )*PRIVATE KEY-----`,

	// Database and message-broker connection strings containing credentials.
	`(?i)(mongodb|postgres(?:ql)?|mysql|redis|amqp):\/\/[^\s'"]+`,

	// GitHub personal / OAuth / user / app / refresh tokens.
	`(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}`,

	// High-entropy values assigned to common secret-related keywords.
	// The value class uses [^\s'"]+ to capture passwords containing special
	// characters (e.g. @, !, #) that are common in real-world secrets.
	// Minimum length is 6 (not 16) because passwords can be short.
	//
	// Separators and the value are both confined to a single line. Using \s
	// here let "DB_PASSWORD=\nJWT_SECRET=change-me" match across the newline,
	// so an empty value consumed the entire following line and deleted it
	// rather than redacting a value. Matching must never span lines.
	`(?i)(password|passwd|secret|token|access[_-]?key|private[_-]?key|auth[_-]?token)['":\t ]*[=:][\t ]*['"]?[^\s'"]{6,}['"]?`,
	`(?i)(password|passwd|secret|token|access[_-]?key|private[_-]?key|auth[_-]?token)['":\t ]*[=:][\t ]*['"]?[^\s'"]{6,}['"]?`,

	// Credentials embedded in HTTPS URLs (e.g. https://user:token@host).
	`https?://[a-zA-Z0-9_\-]+:[a-zA-Z0-9_\-]+@[^\s'"]+`,

	// GitHub personal access tokens embedded in URLs (ghp_xxx@github.com).
	`https?://ghp_[a-zA-Z0-9_]{36,}@[^\s'"]+`,

	// Azure Storage account keys (connection strings containing AccountKey).
	`DefaultEndpointsProtocol=.*AccountKey=[A-Za-z0-9+/=]{86,}`,

	// Azure SAS tokens (query-string signature parameter).
	`[?&]sig=[A-Za-z0-9%+/=]+`,

	// Slack bot/user/app tokens.
	`xox[bpsa]-[0-9A-Za-z-]+`,

	// Stripe live keys (secret and restricted).
	`[sr]k_live_[0-9a-zA-Z]{24,}`,

	// npm authentication tokens.
	`npm_[A-Za-z0-9]{36}`,

	// JWT tokens (three base64url-encoded segments).
	`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+`,

	// Google Cloud API keys.
	`AIza[0-9A-Za-z_-]{35}`,

	// SendGrid API keys.
	`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`,

	// Twilio API keys.
	`SK[a-f0-9]{32}`,
}

// Redactor sanitises content before it reaches AI providers, removing
// secrets and excluding sensitive files.
type Redactor struct {
	filePatterns  []string         // filepath.Match patterns for file exclusion
	secretRegexps []*regexp.Regexp // compiled content-level secret patterns
	forceErr      error            // test-only: when non-nil, RedactContent returns this error
}

// NewRedactor creates a Redactor from the given user-supplied file-exclusion
// patterns, merged with the built-in patterns that are always active. All
// secret-matching regexes are compiled once at construction time.
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

	// Compile secret-matching regexes.
	regexps := make([]*regexp.Regexp, 0, len(builtinSecretPatterns))
	for _, raw := range builtinSecretPatterns {
		regexps = append(regexps, regexp.MustCompile(raw))
	}

	return &Redactor{
		filePatterns:  merged,
		secretRegexps: regexps,
	}
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
		matches := re.FindAllStringIndex(result, -1)
		if len(matches) > 0 {
			count += len(matches)
			result = re.ReplaceAllString(result, RedactedPlaceholder)
		}
	}

	return result, count, nil
}
