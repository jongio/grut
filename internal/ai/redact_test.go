package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// File exclusion
// ---------------------------------------------------------------------------

func TestShouldExcludeFile(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name     string
		path     string
		excluded bool
	}{
		// Built-in exact matches.
		{".env exact", patternDotEnv, true},
		{"SSH RSA key", patternIDRSA, true},
		{"SSH Ed25519 key", patternIDEd25519, true},
		{"SSH ECDSA key", patternIDECDSA, true},

		// Built-in glob matches.
		{".env.local", ".env.local", true},
		{".env.production", ".env.production", true},
		{"private key file", "server.key", true},
		{"PEM certificate", "cert.pem", true},
		{"PKCS12 file", "cert.p12", true},
		{"PFX file", "cert.pfx", true},
		{"secret file", "creds.secret", true},
		{"secrets file", "creds.secrets", true},

		// Paths with directories — match on base name only.
		{"nested .env", "config/.env", true},
		{"nested .env.local", "deploy/.env.local", true},
		{"nested key file", "secrets/api.key", true},
		{"nested id_rsa", "home/.ssh/id_rsa", true},

		// Non-excluded files must pass through.
		{"Go source", "main.go", false},
		{"README", "README.md", false},
		{"TOML config", "config.toml", false},
		{"envfile (no dot prefix)", "envfile", false},
		{"TypeScript env file", "environment.ts", false},
		{"keyboard (contains key)", "keyboard.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.excluded, r.ShouldExcludeFile(tt.path),
				"ShouldExcludeFile(%q)", tt.path)
		})
	}
}

// ---------------------------------------------------------------------------
// AWS key redaction
// ---------------------------------------------------------------------------

func TestRedactAWSAccessKey(t *testing.T) {
	r := NewRedactor(nil)

	input := "aws_access_key_id = AKIAIOSFODNN7EXAMPLE and some text"
	got, count, _ := r.RedactContent(input)

	assert.NotContains(t, got, "AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, got, RedactedPlaceholder)
	assert.Contains(t, got, "and some text", "surrounding text preserved")
	assert.GreaterOrEqual(t, count, 1)
}

// ---------------------------------------------------------------------------
// PEM block redaction
// ---------------------------------------------------------------------------

func TestRedactPEMBlock(t *testing.T) {
	r := NewRedactor(nil)

	pem := `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHC6E9z3bPjV5Oc5
YMORn7y8k6P/0LqDx3kTt1z7/cMiRuPQ1Q==
-----END RSA PRIVATE KEY-----`
	input := "before\n" + pem + "\nafter"

	got, count, _ := r.RedactContent(input)

	assert.NotContains(t, got, "BEGIN RSA PRIVATE KEY")
	assert.NotContains(t, got, "END RSA PRIVATE KEY")
	assert.NotContains(t, got, "MIIEowIBAAKCAQEA")
	assert.Contains(t, got, "before")
	assert.Contains(t, got, "after")
	assert.Contains(t, got, RedactedPlaceholder)
	assert.GreaterOrEqual(t, count, 1)
}

func TestRedactPEMBlockECDSA(t *testing.T) {
	r := NewRedactor(nil)

	input := `-----BEGIN EC PRIVATE KEY-----
MHQCAQEEIBkg4LVWM9nuwNSk3yByxZpYRTBnVxuB1kU7FsJEIaEAA
-----END EC PRIVATE KEY-----`

	got, count, _ := r.RedactContent(input)

	assert.NotContains(t, got, "BEGIN EC PRIVATE KEY")
	assert.Equal(t, RedactedPlaceholder, got)
	assert.Equal(t, 1, count)
}

// ---------------------------------------------------------------------------
// Connection string redaction
// ---------------------------------------------------------------------------

func TestRedactConnectionStrings(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name  string
		input string
		gone  string // substring that must be absent after redaction
	}{
		{"postgres", "DATABASE_URL=postgres://admin:s3cret@db.example.com:5432/mydb", "postgres://"},
		{"postgresql", "DB=postgresql://admin:s3cret@db.example.com:5432/mydb", "postgresql://"},
		{"mongodb", "MONGO_URI=mongodb://user:pass@mongo.host:27017/test", "mongodb://"},
		{"redis", "REDIS_URL=redis://default:hunter2@redis.example.com:6379", "redis://"},
		{"mysql", "url=mysql://root:password@localhost:3306/app", "mysql://"},
		{"amqp", "amqp://guest:guest@rabbit.example.com:5672/", "amqp://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, _ := r.RedactContent(tt.input)
			assert.NotContains(t, got, tt.gone)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// GitHub token redaction
// ---------------------------------------------------------------------------

func TestRedactGitHubTokens(t *testing.T) {
	r := NewRedactor(nil)

	suffix := strings.Repeat("A", 36) // 36-char token body

	tests := []struct {
		name   string
		prefix string
	}{
		{"personal access token", "ghp_"},
		{"OAuth token", "gho_"},
		{"user-to-server token", "ghu_"},
		{"app installation token", "ghs_"},
		{"refresh token", "ghr_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.prefix + suffix
			input := "GITHUB_TOKEN=" + token
			got, count, _ := r.RedactContent(input)

			assert.NotContains(t, got, token)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// API key assignment redaction
// ---------------------------------------------------------------------------

func TestRedactAPIKeyAssignment(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"equals sign", `api_key = "sk_live_51Hb5R8K7VJUxVyN9r01"`},
		{"colon separator", `apikey: sk_live_51Hb5R8K7VJUxVyN9r01`},
		{"api-secret", `api-secret = "abc123def456ghi789jkl012mno"`},
		{"API_KEY caps", `API_KEY="ABCDEFGHIJKLMNOPQRST0123"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, _ := r.RedactContent(tt.input)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// Generic secret keyword redaction
// ---------------------------------------------------------------------------

func TestRedactGenericSecretKeywords(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"password", `password = "CorrectHorseBatteryStaple"`},
		{"token", `token: "eyJhbGciOiJIUzI1NiJ9_xxxx"`},
		{"auth_token", `auth_token = "a1b2c3d4e5f6g7h8i9j0k1l2"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, _ := r.RedactContent(tt.input)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// Redaction must never span newlines
// ---------------------------------------------------------------------------

// A secret keyword with an empty value must not swallow the following line.
// The separator class previously used \s, which matches newlines, so
// "DB_PASSWORD=\nJWT_SECRET=change-me" consumed the newline and the whole
// next line, deleting it from the content handed back to the model.
func TestRedactDoesNotSpanNewlines(t *testing.T) {
	r := NewRedactor(nil)

	input := "DB_PASSWORD=\nJWT_SECRET=change-me\nLOG_LEVEL=info\nPORT=3000"
	got, _, _ := r.RedactContent(input)

	assert.Equal(t, 4, len(strings.Split(got, "\n")),
		"no line may be removed by redaction")
	assert.Contains(t, got, "LOG_LEVEL=info")
	assert.Contains(t, got, "PORT=3000")
	assert.True(t, strings.HasPrefix(got, "DB_PASSWORD=\n"),
		"an empty value must not consume the newline")
	assert.NotContains(t, got, "change-me", "the real secret must still be redacted")
}

// Redaction must preserve line count for any input, including one where the
// keyword line has no value at all.
func TestRedactPreservesLineCount(t *testing.T) {
	r := NewRedactor(nil)

	inputs := []string{
		"secret:\nnext line survives",
		"token=\nkeep me",
		"password:\n\nblank line between",
		"access_key=\nauth_token=\nprivate_key=\ndone",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got, _, _ := r.RedactContent(in)
			assert.Equal(t, strings.Count(in, "\n"), strings.Count(got, "\n"),
				"redaction changed the number of lines")
		})
	}
}

// Guard against over-correcting the newline fix into a false negative: real
// secrets on a single line must still be redacted.
func TestRedactStillCatchesRealSecrets(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name    string
		input   string
		leaked  string
		survive string
	}{
		{"aws key with following line", "AWS_KEY=AKIAIOSFODNN7EXAMPLE\nport: 8080", "AKIAIOSFODNN7EXAMPLE", "port: 8080"},
		{"password with symbols", "DB_PASSWORD=s3cr3t!@#pass\nport: 8080", "s3cr3t!@#pass", "port: 8080"},
		{"quoted api key", "api_key: \"sk_live_abcdefghijklmnop\"\nport: 8080", "sk_live_abcdefghijklmnop", "port: 8080"},
		{"tab separated", "password:\ths3cretvalue\nport: 8080", "hs3cretvalue", "port: 8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, _ := r.RedactContent(tt.input)
			assert.GreaterOrEqual(t, count, 1)
			assert.NotContains(t, got, tt.leaked, "secret leaked through redaction")
			assert.Contains(t, got, tt.survive, "adjacent line was destroyed")
		})
	}
}

// ---------------------------------------------------------------------------
// Non-secret content passes through unchanged
// ---------------------------------------------------------------------------

func TestRedactNonSecretContent(t *testing.T) {
	r := NewRedactor(nil)

	input := `package main

import "fmt"

func main() {
	fmt.Println("Hello, world!")
	x := 42
	for i := 0; i < x; i++ {
		fmt.Printf("iteration %d\n", i)
	}
}
`
	got, count, _ := r.RedactContent(input)

	assert.Equal(t, input, got)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// Redaction count accuracy
// ---------------------------------------------------------------------------

func TestRedactContentCountAccuracy(t *testing.T) {
	r := NewRedactor(nil)

	// Three distinct secret types in a single blob.
	ghToken := "ghp_" + strings.Repeat("x", 36)
	input := "key1=AKIAIOSFODNN7EXAMPLE\n" +
		"db=postgres://user:pass@host:5432/db\n" +
		"gh=" + ghToken + "\n"

	_, count, _ := r.RedactContent(input)

	assert.GreaterOrEqual(t, count, 3, "expected at least 3 redactions")
}

// ---------------------------------------------------------------------------
// User patterns merge with built-in patterns
// ---------------------------------------------------------------------------

func TestUserPatternsMerge(t *testing.T) {
	// Add a custom pattern; duplicate ".env" to verify dedup.
	r := NewRedactor([]string{"*.custom", patternDotEnv})

	// Custom pattern works.
	assert.True(t, r.ShouldExcludeFile("data.custom"))

	// Built-in patterns still active.
	assert.True(t, r.ShouldExcludeFile(patternDotEnv))
	assert.True(t, r.ShouldExcludeFile(".env.local"))
	assert.True(t, r.ShouldExcludeFile("server.key"))
	assert.True(t, r.ShouldExcludeFile(patternIDRSA))

	// Non-matching file passes through.
	assert.False(t, r.ShouldExcludeFile("main.go"))
}

func TestUserPatternsDeduplicate(t *testing.T) {
	r := NewRedactor([]string{patternDotEnv, patternDotEnv, patternKeyFile})

	// Count occurrences of ".env" in file patterns — must be exactly 1.
	envCount := 0
	for _, p := range r.filePatterns {
		if p == ".env" {
			envCount++
		}
	}
	assert.Equal(t, 1, envCount, "duplicate patterns must be deduplicated")
}

// ---------------------------------------------------------------------------
// Nil patterns constructor
// ---------------------------------------------------------------------------

func TestNewRedactorNilPatterns(t *testing.T) {
	r := NewRedactor(nil)
	require.NotNil(t, r)
	assert.NotEmpty(t, r.filePatterns, "built-in file patterns must be present")
	assert.NotEmpty(t, r.secretRegexps, "built-in secret regexps must be compiled")
}

// ---------------------------------------------------------------------------
// Empty content
// ---------------------------------------------------------------------------

func TestRedactEmptyContent(t *testing.T) {
	r := NewRedactor(nil)
	got, count, _ := r.RedactContent("")
	assert.Equal(t, "", got)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// Overlapping patterns
// ---------------------------------------------------------------------------

func TestRedactOverlappingPatterns(t *testing.T) {
	// Content that matches multiple patterns (e.g. a GitHub token inside
	// a generic "token =" assignment). Should redact without panicking
	// and the count should reflect both pattern matches.
	r := NewRedactor(nil)

	ghToken := "ghp_" + strings.Repeat("A", 36)
	input := `token = "` + ghToken + `"`

	got, count, _ := r.RedactContent(input)
	assert.NotContains(t, got, ghToken)
	assert.Contains(t, got, RedactedPlaceholder)
	// At least 1 redaction (may be more if both patterns match).
	assert.GreaterOrEqual(t, count, 1)
}

// ---------------------------------------------------------------------------
// Multiple secrets on single line
// ---------------------------------------------------------------------------

func TestRedactMultipleSecretsOnOneLine(t *testing.T) {
	r := NewRedactor(nil)

	input := "AKIAIOSFODNN7EXAMPLE ghp_" + strings.Repeat("B", 36) + " postgres://user:pass@host/db"

	got, count, _ := r.RedactContent(input)
	assert.NotContains(t, got, "AKIAIOSFODNN7EXAMPLE")
	assert.NotContains(t, got, "postgres://")
	assert.GreaterOrEqual(t, count, 3)
	// All replaced with placeholder.
	assert.True(t, strings.Count(got, RedactedPlaceholder) >= 2)
}

// ---------------------------------------------------------------------------
// Whitespace-only content
// ---------------------------------------------------------------------------

func TestRedactWhitespaceContent(t *testing.T) {
	r := NewRedactor(nil)
	got, count, _ := r.RedactContent("   \n\t\n   ")
	assert.Equal(t, "   \n\t\n   ", got)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// Credential-in-URL redaction (H-11)
// ---------------------------------------------------------------------------

func TestRedactCredentialInURL(t *testing.T) {
	r := NewRedactor(nil)

	tests := []struct {
		name  string
		input string
		gone  string
	}{
		{"user:token@github", "remote=https://ghp_ABCDEFghijklmnopqrstuvwxyz0123456789AB@github.com/owner/repo.git", "ghp_"},
		{"user:pass@host", "origin https://admin:s3cretP4ss@github.com/org/repo.git", "admin:s3cretP4ss@"},
		{"token-only@host", "url = https://myuser:ghp_token123456@example.com/path", "myuser:ghp_token123456@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, _ := r.RedactContent(tt.input)
			assert.NotContains(t, got, tt.gone)
			assert.Contains(t, got, RedactedPlaceholder)
			assert.GreaterOrEqual(t, count, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// ShouldExcludeFile with user custom patterns
// ---------------------------------------------------------------------------

func TestShouldExcludeFile_CustomPattern(t *testing.T) {
	r := NewRedactor([]string{"*.credentials", "config.secret.*"})

	assert.True(t, r.ShouldExcludeFile("aws.credentials"))
	assert.True(t, r.ShouldExcludeFile("config.secret.json"))
	assert.False(t, r.ShouldExcludeFile("config.json"))
	// Built-in patterns still work.
	assert.True(t, r.ShouldExcludeFile(patternDotEnv))
}
