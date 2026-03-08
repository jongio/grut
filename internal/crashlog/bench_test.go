package crashlog

import (
	"strings"
	"testing"
)

// benchResult prevents the compiler from optimizing away benchmark calls.
var benchResult string

// benchResultSlice prevents the compiler from optimizing away slice results.
var benchResultSlice []string

// ---------------------------------------------------------------------------
// Test data generators
// ---------------------------------------------------------------------------

func smallPII() string {
	return `error at /home/alice/project/src/main.go: token=ghp_abcdefghij1234567890abcdefghij123456`
}

func mediumPII() string {
	var b strings.Builder
	b.Grow(1024)
	b.WriteString("=== crash report ===\n")
	b.WriteString("user: /home/alice/.config/grut/config.toml\n")
	b.WriteString("secret=mysupersecretvalue123\n")
	b.WriteString("https://user:pass@github.com/repo.git\n")
	b.WriteString("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.verylongtoken\n")
	b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
	b.WriteString("token: ghp_abcdefghij1234567890abcdefghij123456\n")
	// Pad to ~1KB with realistic stack trace lines.
	for b.Len() < 1024 {
		b.WriteString("goroutine 1 [running]:\nmain.main()\n\t/home/alice/grut/main.go:42 +0x1a0\n")
	}
	return b.String()
}

func largePII() string {
	var b strings.Builder
	b.Grow(10240)
	// Embed multiple secret types throughout a large block.
	patterns := []string{
		"credential=supersecret12345\n",
		"https://admin:hunter2@internal.corp.example.com/api\n",
		"AKIA1234567890ABCDEF is the key\n",
		"Authorization: Bearer sk-proj-longapikey1234567890abcdef\n",
		"-----BEGIN EC PRIVATE KEY-----\n",
		"password: ghp_abcdefghij1234567890abcdefghij123456\n",
		"glpat-xxxxxxxxxxxxxxxxxxxxxxxx\n",
		"xoxb-1234567890-abcdefghijklmn\n",
	}
	for b.Len() < 10240 {
		for _, p := range patterns {
			b.WriteString(p)
			// Interleave with clean stack trace lines.
			b.WriteString("runtime/debug.Stack()\n\t/usr/local/go/src/runtime/debug/stack.go:24\n")
		}
	}
	return b.String()
}

func cleanString(size int) string {
	var b strings.Builder
	b.Grow(size)
	for b.Len() < size {
		b.WriteString("goroutine 12 [select]:\nnet/http.(*persistConn).roundTrip(0xc000230000)\n")
		b.WriteString("\t/usr/local/go/src/net/http/transport.go:2612 +0x9cb\n")
	}
	return b.String()[:size]
}

// ---------------------------------------------------------------------------
// Benchmarks: ScrubPII
// ---------------------------------------------------------------------------

func BenchmarkScrubPII(b *testing.B) {
	b.Run("small_100B", func(b *testing.B) {
		s := smallPII()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})

	b.Run("medium_1KB", func(b *testing.B) {
		s := mediumPII()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})

	b.Run("large_10KB", func(b *testing.B) {
		s := largePII()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})
}

func BenchmarkScrubPII_NoSecrets(b *testing.B) {
	b.Run("100B", func(b *testing.B) {
		s := cleanString(100)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})

	b.Run("1KB", func(b *testing.B) {
		s := cleanString(1024)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})

	b.Run("10KB", func(b *testing.B) {
		s := cleanString(10240)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResult = ScrubPII(s)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: scrubLogTail (combined tail extraction + scrubbing)
// ---------------------------------------------------------------------------

func BenchmarkScrubLogTail(b *testing.B) {
	b.Run("10_entries", func(b *testing.B) {
		entries := make([]string, 10)
		for i := range entries {
			entries[i] = smallPII()
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultSlice = scrubLogTail(entries)
		}
	})

	b.Run("50_entries", func(b *testing.B) {
		entries := make([]string, 50)
		for i := range entries {
			entries[i] = smallPII()
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultSlice = scrubLogTail(entries)
		}
	})

	b.Run("100_entries_mixed", func(b *testing.B) {
		entries := make([]string, 100)
		for i := range entries {
			if i%3 == 0 {
				entries[i] = mediumPII()
			} else {
				entries[i] = cleanString(200)
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultSlice = scrubLogTail(entries)
		}
	})
}
