package config

import (
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

// errBenchSink prevents the compiler from optimizing away error results.
var errBenchSink error

// ---------------------------------------------------------------------------
// Benchmarks: Validate
// ---------------------------------------------------------------------------

func BenchmarkValidate(b *testing.B) {
	b.Run("valid_defaults", func(b *testing.B) {
		cfg := benchValidCfg(b)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			errBenchSink = Validate(cfg)
		}
	})

	b.Run("with_arrays", func(b *testing.B) {
		cfg := benchValidCfg(b)
		// Populate array fields to exercise array-size validation.
		cfg.AI.RedactPatterns = make([]string, 50)
		for i := range cfg.AI.RedactPatterns {
			cfg.AI.RedactPatterns[i] = "pattern_" + string(rune('a'+i%26))
		}
		cfg.AI.Review.Categories = make([]string, 20)
		for i := range cfg.AI.Review.Categories {
			cfg.AI.Review.Categories[i] = "category_" + string(rune('a'+i%26))
		}
		cfg.Bookmarks.Paths = make([]string, 100)
		for i := range cfg.Bookmarks.Paths {
			cfg.Bookmarks.Paths[i] = "/home/user/projects/project_" + string(rune('a'+i%26))
		}
		cfg.Shortcuts.Custom = make([]CustomShortcut, 30)
		for i := range cfg.Shortcuts.Custom {
			cfg.Shortcuts.Custom[i] = CustomShortcut{
				Name:        "shortcut_" + string(rune('a'+i%26)),
				Description: "A custom shortcut",
				Steps:       []string{"step1", "step2"},
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			errBenchSink = Validate(cfg)
		}
	})

	b.Run("invalid_multi_error", func(b *testing.B) {
		cfg := benchValidCfg(b)
		// Introduce multiple validation errors.
		cfg.FileTree.MaxDepth = maxMaxDepth + 1
		cfg.Preview.Width = 0
		cfg.Terminal.Scrollback = maxScrollback + 1
		cfg.AI.Temperature = 2.0
		cfg.General.DefaultLayout = "invalid"
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			errBenchSink = Validate(cfg)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: TOML parsing (defaults)
// ---------------------------------------------------------------------------

func BenchmarkParseDefaultsTOML(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cfg := &Config{}
		errBenchSink = toml.Unmarshal(defaultsTOML, cfg)
	}
}

// ---------------------------------------------------------------------------
// Benchmark: rejectEmbeddedKeys
// ---------------------------------------------------------------------------

func BenchmarkRejectEmbeddedKeys(b *testing.B) {
	b.Run("clean_config", func(b *testing.B) {
		cfg := benchValidCfg(b)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = rejectEmbeddedKeys(nil, "ai", cfg.AI)
		}
	})

	b.Run("with_patterns", func(b *testing.B) {
		cfg := benchValidCfg(b)
		cfg.AI.RedactPatterns = make([]string, 50)
		for i := range cfg.AI.RedactPatterns {
			cfg.AI.RedactPatterns[i] = "safe_pattern_value"
		}
		cfg.AI.Review.Categories = make([]string, 30)
		for i := range cfg.AI.Review.Categories {
			cfg.AI.Review.Categories[i] = "security"
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = rejectEmbeddedKeys(nil, "ai", cfg.AI)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmark: appendEnumErr / appendEnumOrPathErr
// ---------------------------------------------------------------------------

func BenchmarkAppendEnumErr(b *testing.B) {
	b.Run("match_first", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = appendEnumErr(nil, "field", "default", "default", "classic", "vim")
		}
	})

	b.Run("match_last", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = appendEnumErr(nil, "field", "vim", "default", "classic", "vim")
		}
	})

	b.Run("no_match", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			_ = appendEnumErr(nil, "field", "invalid", "default", "classic", "vim")
		}
	})
}

// ---------------------------------------------------------------------------
// Helper: create a valid config from embedded defaults
// ---------------------------------------------------------------------------

func benchValidCfg(b *testing.B) *Config {
	b.Helper()
	cfg := &Config{}
	require.NoError(b, toml.Unmarshal(defaultsTOML, cfg))
	return cfg
}
