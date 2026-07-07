// Package secretscan detects likely secrets in file content and flags
// sensitive filenames before they are staged into git. It is deliberately
// conservative: high-confidence provider patterns and sensitive filenames
// always fire, while the generic high-entropy check is gated by an allowlist
// so that everyday source code stays quiet.
package secretscan

import (
	"bytes"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding describes one reason a file was flagged.
type Finding struct {
	// Rule is a short, stable identifier for the rule that matched
	// (for example "github-token" or "sensitive-filename").
	Rule string
	// Detail is a human-readable description shown to the user. It never
	// includes the secret value itself, only the rule and location.
	Detail string
	// Line is the 1-based line number for content matches, or 0 for a
	// match based on the filename alone.
	Line int
}

// maxScanBytes caps how much file content is scanned so a very large file
// cannot stall the UI. Credentials that matter in practice sit near the top
// of config and source files, so a generous cap is safe.
const maxScanBytes = 1 << 20 // 1 MiB

// minGenericValueLen is the shortest value the generic high-entropy rule will
// consider, avoiding noise from short tokens that are unlikely to be secrets.
const minGenericValueLen = 16

// minGenericEntropy is the Shannon entropy (bits per character) a generic
// value must exceed before it is treated as a possible secret.
const minGenericEntropy = 3.5

// Rule identifiers reported in Finding.Rule.
const (
	RuleGitHubToken       = "github-token"
	RuleAWSAccessKey      = "aws-access-key"
	RulePrivateKey        = "private-key"
	RuleSlackToken        = "slack-token"
	RuleGoogleAPIKey      = "google-api-key"
	RuleStripeKey         = "stripe-key"
	RuleHighEntropyValue  = "high-entropy-value"
	RuleSensitiveFilename = "sensitive-filename"
)

// contentRule pairs a compiled pattern with the metadata reported on a match.
type contentRule struct {
	re     *regexp.Regexp
	rule   string
	detail string
}

var contentRules = []contentRule{
	{
		re:     regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b`),
		rule:   RuleGitHubToken,
		detail: "GitHub personal access or app token",
	},
	{
		re:     regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`),
		rule:   RuleGitHubToken,
		detail: "GitHub fine-grained personal access token",
	},
	{
		re:     regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		rule:   RuleAWSAccessKey,
		detail: "AWS access key id",
	},
	{
		re:     regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`),
		rule:   RulePrivateKey,
		detail: "Private key block",
	},
	{
		re:     regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z-]{10,}\b`),
		rule:   RuleSlackToken,
		detail: "Slack token",
	},
	{
		re:     regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`),
		rule:   RuleGoogleAPIKey,
		detail: "Google API key",
	},
	{
		re:     regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[0-9A-Za-z]{16,}\b`),
		rule:   RuleStripeKey,
		detail: "Stripe secret key",
	},
}

// genericAssignment matches "key = value" style declarations where the key
// name hints at a credential. The value is entropy-checked before it is
// reported so that ordinary configuration does not trip the rule.
var genericAssignment = regexp.MustCompile(
	`(?i)(api[_-]?key|secret|token|password|passwd|pwd|access[_-]?key|auth)\s*[:=]\s*["']?([^\s"'` + "`" + `]+)`,
)

// placeholderHints mark values that are obviously not real secrets. They keep
// examples, templates, and interpolation expressions from being flagged.
var placeholderHints = []string{
	"example", "changeme", "change-me", "placeholder", "your_", "your-",
	"yourkey", "xxxx", "todo", "dummy", "sample", "redacted", "<", ">",
	"${", "{{", "}}", "notreal", "fake", "test-", "insertkey", "none",
}

// exactSensitiveNames are filenames that are sensitive regardless of contents.
var exactSensitiveNames = map[string]struct{}{
	".env":             {},
	"id_rsa":           {},
	"id_dsa":           {},
	"id_ecdsa":         {},
	"id_ed25519":       {},
	"credentials.json": {},
	".netrc":           {},
	".pgpass":          {},
	".npmrc":           {},
}

// sensitiveExts are file extensions that typically hold keys or certificates.
var sensitiveExts = map[string]struct{}{
	".pem":      {},
	".key":      {},
	".pfx":      {},
	".p12":      {},
	".keystore": {},
	".jks":      {},
}

// safeEnvSuffixes keep example and template env files from being flagged even
// though they start with ".env".
var safeEnvSuffixes = []string{".example", ".sample", ".template", ".dist"}

// Scan inspects file content and its path and returns any findings. Content
// scanning is skipped for binary data, but the filename rules still apply.
// The returned slice is nil when nothing is flagged.
func Scan(content []byte, path string) []Finding {
	var findings []Finding

	if f, ok := filenameFinding(path); ok {
		findings = append(findings, f)
	}

	if len(content) == 0 || isBinary(content) {
		return findings
	}

	scan := content
	if len(scan) > maxScanBytes {
		scan = scan[:maxScanBytes]
	}
	findings = append(findings, contentFindings(scan)...)
	return findings
}

// HasFindings reports whether Scan would flag the given content or path.
func HasFindings(content []byte, path string) bool {
	return len(Scan(content, path)) > 0
}

// filenameFinding reports a sensitive-filename match based on path alone.
func filenameFinding(path string) (Finding, bool) {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	if base == "" {
		return Finding{}, false
	}
	if _, ok := exactSensitiveNames[base]; ok {
		return Finding{Rule: RuleSensitiveFilename, Detail: "sensitive filename " + base}, true
	}
	if strings.HasPrefix(base, ".env.") && !hasSafeEnvSuffix(base) {
		return Finding{Rule: RuleSensitiveFilename, Detail: "environment file " + base}, true
	}
	if _, ok := sensitiveExts[filepath.Ext(base)]; ok {
		return Finding{Rule: RuleSensitiveFilename, Detail: "sensitive filename " + base}, true
	}
	return Finding{}, false
}

func hasSafeEnvSuffix(base string) bool {
	for _, s := range safeEnvSuffixes {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return false
}

// contentFindings runs the provider patterns and the gated generic rule over
// the given content, deduplicating by rule and line.
func contentFindings(content []byte) []Finding {
	var findings []Finding
	seen := make(map[string]struct{})

	add := func(rule, detail string, line int) {
		key := rule + ":" + detail
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		findings = append(findings, Finding{Rule: rule, Detail: detail, Line: line})
	}

	for _, cr := range contentRules {
		if loc := cr.re.FindIndex(content); loc != nil {
			add(cr.rule, cr.detail, lineOf(content, loc[0]))
		}
	}

	for _, m := range genericAssignment.FindAllSubmatchIndex(content, -1) {
		keyName := string(content[m[2]:m[3]])
		value := string(content[m[4]:m[5]])
		if !looksLikeSecret(value) {
			continue
		}
		add(RuleHighEntropyValue,
			"high-entropy value assigned to "+strings.ToLower(keyName),
			lineOf(content, m[4]))
	}
	return findings
}

// looksLikeSecret reports whether a generic assignment value is long enough,
// high-entropy enough, and free of placeholder hints to be treated as a
// possible secret.
func looksLikeSecret(value string) bool {
	if len(value) < minGenericValueLen {
		return false
	}
	lower := strings.ToLower(value)
	for _, h := range placeholderHints {
		if strings.Contains(lower, h) {
			return false
		}
	}
	return shannonEntropy(value) >= minGenericEntropy
}

// shannonEntropy returns the Shannon entropy of s in bits per character.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	var entropy float64
	length := float64(len(s))
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// lineOf returns the 1-based line number of the byte at index idx.
func lineOf(content []byte, idx int) int {
	if idx < 0 || idx > len(content) {
		return 0
	}
	return bytes.Count(content[:idx], []byte{'\n'}) + 1
}

// isBinary reports whether content looks like binary data. A NUL byte within
// the sniffed prefix is a reliable signal of a non-text file.
func isBinary(content []byte) bool {
	sniff := content
	const sniffLen = 8000
	if len(sniff) > sniffLen {
		sniff = sniff[:sniffLen]
	}
	return bytes.IndexByte(sniff, 0) != -1
}
