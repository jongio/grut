package ai

import (
	"strings"
	"testing"
)

// benchResultStr prevents the compiler from optimizing away string results.
var benchResultStr string

// benchResultInt prevents the compiler from optimizing away int results.
var benchResultInt int

// ---------------------------------------------------------------------------
// Test data generators
// ---------------------------------------------------------------------------

func contentWithSecrets(size int) string {
	var b strings.Builder
	b.Grow(size)

	secrets := []string{
		`AKIAIOSFODNN7EXAMPLE`,
		`api_key = "sk_live_abcdefghijklmnopqrstuvwxyz1234"`,
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn\n-----END RSA PRIVATE KEY-----",
		`postgresql://admin:s3cret@db.example.com:5432/prod`,
		`ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl`,
		`password = "hunter2supersecret"`,
		`https://deploy:ghp_token1234567890abcdef@github.com/org/repo`,
		`https://ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl@github.com/user/repo`,
		`api-secret: "very-long-secret-value-here-1234567890"`,
		`token = "gho_abcdefghijklmnopqrstuvwxyz1234567890"`,
	}

	// Interleave secrets with realistic code content.
	for b.Len() < size {
		for _, s := range secrets {
			b.WriteString("// configuration loader\n")
			b.WriteString("func LoadConfig() error {\n")
			b.WriteString("    " + s + "\n")
			b.WriteString("    return nil\n}\n\n")
		}
	}
	return b.String()[:size]
}

func cleanContent(size int) string {
	var b strings.Builder
	b.Grow(size)
	for b.Len() < size {
		b.WriteString("package main\n\nimport \"fmt\"\n\n")
		b.WriteString("func main() {\n")
		b.WriteString("    fmt.Println(\"Hello, world!\")\n")
		b.WriteString("    x := 42\n")
		b.WriteString("    for i := range x {\n")
		b.WriteString("        fmt.Printf(\"iteration %d\\n\", i)\n")
		b.WriteString("    }\n}\n\n")
	}
	return b.String()[:size]
}

// ---------------------------------------------------------------------------
// Benchmarks: RedactContent
// ---------------------------------------------------------------------------

func BenchmarkRedactContent(b *testing.B) {
	b.Run("small_500B", func(b *testing.B) {
		r := NewRedactor(nil)
		content := contentWithSecrets(500)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})

	b.Run("medium_5KB", func(b *testing.B) {
		r := NewRedactor(nil)
		content := contentWithSecrets(5 * 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})

	b.Run("large_50KB", func(b *testing.B) {
		r := NewRedactor(nil)
		content := contentWithSecrets(50 * 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})
}

func BenchmarkRedactContent_NoMatch(b *testing.B) {
	b.Run("small_500B", func(b *testing.B) {
		r := NewRedactor(nil)
		content := cleanContent(500)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})

	b.Run("medium_5KB", func(b *testing.B) {
		r := NewRedactor(nil)
		content := cleanContent(5 * 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})

	b.Run("large_50KB", func(b *testing.B) {
		r := NewRedactor(nil)
		content := cleanContent(50 * 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr, benchResultInt, _ = r.RedactContent(content)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: ShouldExcludeFile
// ---------------------------------------------------------------------------

func BenchmarkShouldExcludeFile(b *testing.B) {
	b.Run("match_builtin", func(b *testing.B) {
		r := NewRedactor([]string{"*.custom", "secret_*"})
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = r.ShouldExcludeFile("config/.env.local")
		}
	})

	b.Run("match_user_pattern", func(b *testing.B) {
		r := NewRedactor([]string{"*.custom", "secret_*"})
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = r.ShouldExcludeFile("deploy/secret_prod.yaml")
		}
	})

	b.Run("no_match", func(b *testing.B) {
		r := NewRedactor([]string{"*.custom", "secret_*"})
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = r.ShouldExcludeFile("src/internal/handler.go")
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: NewRedactor construction
// ---------------------------------------------------------------------------

func BenchmarkNewRedactor(b *testing.B) {
	patterns := []string{"*.custom", "secret_*", "*.credentials", ".vault"}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = NewRedactor(patterns)
	}
}
