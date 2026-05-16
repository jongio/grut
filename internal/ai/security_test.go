package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Security regression suite — ensures NO secret pattern survives redaction.
// These tests complement redact_test.go by acting as a single checkpoint
// that validates the complete set of secret patterns. If a pattern is
// accidentally removed from builtinSecretPatterns, this file will catch it.
// ---------------------------------------------------------------------------

func TestSecurityNoSecretSurvivesRedaction(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name       string
		input      string
		mustRedact string // substring that MUST be absent after redaction
	}{
		// AWS access key
		{"AWS access key", "AWS_KEY=AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},

		// OpenAI API key (sk- prefix)
		{"OpenAI key assignment", `api_key = "sk-proj-abc123def456ghi789jkl"`, "sk-proj-abc123def456ghi789jkl"},

		// PEM RSA private key
		{"PEM RSA key", "-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK\n-----END RSA PRIVATE KEY-----", "BEGIN RSA PRIVATE KEY"},

		// PEM generic private key
		{"PEM generic key", "-----BEGIN PRIVATE KEY-----\nMIIBVg\n-----END PRIVATE KEY-----", "BEGIN PRIVATE KEY"},

		// PostgreSQL connection string
		{"postgres conn string", "DATABASE_URL=postgres://admin:s3cret@db.host:5432/mydb", "postgres://admin"},

		// MongoDB connection string
		{"mongodb conn string", "MONGO=mongodb://user:pass@mongo:27017/db", "mongodb://user"},

		// Redis connection string
		{"redis conn string", "REDIS=redis://default:hunter2@redis:6379", "redis://default"},

		// MySQL connection string
		{"mysql conn string", "MYSQL=mysql://root:password@localhost:3306/app", "mysql://root"},

		// AMQP connection string
		{"amqp conn string", "BROKER=amqp://guest:guest@rabbit:5672/", "amqp://guest"},

		// GitHub personal access token
		{"GitHub PAT (ghp)", "GITHUB_TOKEN=ghp_" + strings.Repeat("A", 36), "ghp_"},

		// GitHub OAuth token
		{"GitHub OAuth (gho)", "TOKEN=gho_" + strings.Repeat("B", 36), "gho_"},

		// GitHub user-to-server token
		{"GitHub u2s (ghu)", "TOKEN=ghu_" + strings.Repeat("C", 36), "ghu_"},

		// GitHub app installation token
		{"GitHub app (ghs)", "TOKEN=ghs_" + strings.Repeat("D", 36), "ghs_"},

		// GitHub refresh token
		{"GitHub refresh (ghr)", "TOKEN=ghr_" + strings.Repeat("E", 36), "ghr_"},

		// Generic password assignment
		{"password assignment", `password = "SuperS3cretP@ssword!!"`, "SuperS3cretP@ssword"},

		// Generic token assignment
		{"token assignment", `auth_token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"`, "eyJhbGciOiJIUzI1NiI"},

		// API secret assignment
		{"api-secret assignment", `api-secret = "a1b2c3d4e5f6g7h8i9j0k1l2"`, "a1b2c3d4e5f6g7h8i9j0"},

		// Google Cloud API key
		{"Google Cloud API key", "GOOGLE_KEY=AIzaSyA1234567890abcdefghijklmnopqrstuvw", "AIzaSyA1234567890abcdefghijklmnopqrstuvw"},

		// SendGrid API key
		{"SendGrid API key", "SENDGRID_KEY=SG.abcdefghijklmnopqrstuv.ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrst", "SG.abcdefghijklmnopqrstuv"},

		// Twilio API key (fake test value — not a real key)
		{"Twilio API key", "TWILIO_KEY=SKdeadbeefdeadbeefdeadbeefdeadbeef", "SKdeadbeefdeadbeefdeadbeefdeadbeef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := r.RedactContent(tt.input)
			assert.NotContains(t, got, tt.mustRedact,
				"secret pattern %q should have been redacted", tt.mustRedact)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// TestSecurityAllSecretsRedactedInComposite verifies that a content blob
// containing EVERY supported secret type gets fully sanitized. This catches
// regex ordering issues where one pattern's replacement breaks another.
func TestSecurityAllSecretsRedactedInComposite(t *testing.T) {
	r := NewRedactor(nil)

	ghToken := "ghp_" + strings.Repeat("Z", 36)

	input := strings.Join([]string{
		"# Config with secrets",
		"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",
		`api_key = "sk-prod-abcdefghijklmnopqrst"`,
		"-----BEGIN RSA PRIVATE KEY-----",
		"MIIBogIBAAJBAKxdo+5e3MksMk5hlVIqlT1OZGY+Fv7IQfT5t0w=",
		"-----END RSA PRIVATE KEY-----",
		"DATABASE_URL=postgres://admin:s3cret@db.host:5432/prod",
		"MONGO=mongodb://app:password@mongo.cluster:27017/data",
		"GITHUB_TOKEN=" + ghToken,
		`password = "CorrectHorseBatteryStaple99"`,
		"# End of config",
	}, "\n")

	got, count := r.RedactContent(input)

	// Every secret type must be redacted.
	secretFragments := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"sk-prod-abcdefghijklmnopqrst",
		"BEGIN RSA PRIVATE KEY",
		"postgres://admin",
		"mongodb://app",
		ghToken,
		"CorrectHorseBatteryStaple99",
	}

	for _, frag := range secretFragments {
		assert.NotContains(t, got, frag,
			"fragment %q survived redaction in composite content", frag)
	}

	// Non-secret context must survive.
	assert.Contains(t, got, "# Config with secrets")
	assert.Contains(t, got, "# End of config")

	// At least one redaction per secret type.
	assert.GreaterOrEqual(t, count, 5,
		"expected at least 5 redactions for composite content with multiple secret types")
}

// TestSecurityNonSecretsPreserved verifies that code containing keywords that
// look superficially similar to secrets (e.g., "api_key" as a variable name
// without a real secret value) is NOT incorrectly redacted.
func TestSecurityNonSecretsPreserved(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"function named getAPIKey", "func getAPIKey() string { return os.Getenv(\"API_KEY\") }"},
		{"comment about tokens", "// This function validates the token format"},
		{"struct field Password", "type User struct {\n\tPassword string\n}"},
		{"variable AKIA not 16 chars", "x := \"AKIA123\""},
		{"url without credentials", "https://api.example.com/v1/data"},
		{"gh prefix but not token", "gh_pages_deploy := true"},
		{"normal Go code", "for i := range items {\n\tfmt.Println(items[i])\n}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := r.RedactContent(tt.input)
			assert.Equal(t, tt.input, got,
				"non-secret content should pass through unchanged")
			assert.Equal(t, 0, count)
		})
	}
}

// TestSecurityFileExclusionCoversAllSensitiveTypes verifies that every
// category of sensitive file is excluded. This is a regression guard so
// that removing a pattern from builtinFilePatterns is caught.
func TestSecurityFileExclusionCoversAllSensitiveTypes(t *testing.T) {
	r := NewRedactor(nil)

	mustExclude := []struct {
		name string
		path string
	}{
		// Environment files
		{"dotenv", patternDotEnv},
		{"dotenv local", ".env.local"},
		{"dotenv production", ".env.production"},

		// Private key files
		{"PEM file", "server.pem"},
		{"KEY file", "private.key"},
		{"PKCS12", "cert.p12"},
		{"PFX file", "cert.pfx"},

		// SSH keys
		{"SSH RSA", patternIDRSA},
		{"SSH Ed25519", patternIDEd25519},
		{"SSH ECDSA", patternIDECDSA},

		// Secret files
		{".secret extension", "database.secret"},
		{".secrets extension", "app.secrets"},

		// Nested paths — only base name matters
		{"nested dotenv", "deploy/staging/.env"},
		{"nested key", "certs/tls.key"},
		{"deep nested pem", "a/b/c/server.pem"},
	}

	for _, tt := range mustExclude {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, r.ShouldExcludeFile(tt.path),
				"file %q MUST be excluded", tt.path)
		})
	}
}

// TestSecurityBuiltinPatternsNotEmpty ensures that the Redactor always
// has its built-in patterns compiled. If someone empties builtinSecretPatterns
// or builtinFilePatterns, this test will catch it immediately.
func TestSecurityBuiltinPatternsNotEmpty(t *testing.T) {
	r := NewRedactor(nil)
	require.NotEmpty(t, r.filePatterns, "built-in file exclusion patterns must not be empty")
	require.NotEmpty(t, r.secretRegexps, "built-in secret regexps must not be empty")

	// Minimum expected counts to catch accidental pattern removal.
	assert.GreaterOrEqual(t, len(r.filePatterns), 10,
		"expected at least 10 built-in file patterns")
	assert.GreaterOrEqual(t, len(r.secretRegexps), 5,
		"expected at least 5 built-in secret regexps")
}

// TestSecurityRedactionPlaceholderConsistency ensures every redaction uses
// the same placeholder so downstream code can reliably detect redacted content.
func TestSecurityRedactionPlaceholderConsistency(t *testing.T) {
	r := NewRedactor(nil)

	inputs := []string{
		"key=AKIAIOSFODNN7EXAMPLE",
		"-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----",
		"DB=postgres://user:pass@host:5432/db",
		"TOKEN=ghp_" + strings.Repeat("A", 36),
	}

	for _, input := range inputs {
		got, count := r.RedactContent(input)
		if count > 0 {
			assert.Contains(t, got, RedactedPlaceholder,
				"all redactions must use the canonical RedactedPlaceholder")
			assert.NotContains(t, got, "***",
				"must not use asterisk-based redaction")
			assert.NotContains(t, got, "<redacted>",
				"must not use HTML-style redaction tag")
		}
	}
}
