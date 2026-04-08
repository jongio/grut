package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateArg(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantErr bool
		errMsg  string
	}{
		{name: "valid simple arg", arg: "main", wantErr: false},
		{name: "valid ref with slash", arg: "origin/main", wantErr: false},
		{name: "valid hash", arg: "abc123def", wantErr: false},
		{name: "valid with dash", arg: "feature-branch", wantErr: false},
		{name: "valid with underscore", arg: "my_branch", wantErr: false},
		{name: "valid with dot", arg: "v1.0.0", wantErr: false},
		{name: "valid with tilde", arg: "HEAD~3", wantErr: false},
		{name: "valid with caret", arg: "HEAD^2", wantErr: false},
		{name: "valid with at", arg: "HEAD@{0}", wantErr: false},
		{name: "valid with colon", arg: "HEAD:file.txt", wantErr: false},

		// Forbidden characters
		{name: "semicolon", arg: "foo;bar", wantErr: true, errMsg: "forbidden character"},
		{name: "pipe", arg: "foo|bar", wantErr: true, errMsg: "forbidden character"},
		{name: "ampersand", arg: "foo&bar", wantErr: true, errMsg: "forbidden character"},
		{name: "dollar", arg: "foo$bar", wantErr: true, errMsg: "forbidden character"},
		{name: "backtick", arg: "foo`bar", wantErr: true, errMsg: "forbidden character"},
		{name: "open paren", arg: "foo(bar", wantErr: true, errMsg: "forbidden character"},
		{name: "close paren", arg: "foo)bar", wantErr: true, errMsg: "forbidden character"},
		{name: "open brace", arg: "foo{bar", wantErr: true, errMsg: "forbidden character"},
		{name: "close brace", arg: "foo}bar", wantErr: true, errMsg: "forbidden character"},
		{name: "less than", arg: "foo<bar", wantErr: true, errMsg: "forbidden character"},
		{name: "greater than", arg: "foo>bar", wantErr: true, errMsg: "forbidden character"},
		{name: "newline", arg: "foo\nbar", wantErr: true, errMsg: "forbidden character"},
		{name: "carriage return", arg: "foo\rbar", wantErr: true, errMsg: "forbidden character"},
		{name: "backslash", arg: "foo\\bar", wantErr: true, errMsg: "forbidden character"},
		{name: "null byte", arg: "foo\x00bar", wantErr: true, errMsg: "null byte"},

		// Edge cases
		{name: "empty string", arg: "", wantErr: true, errMsg: "must not be empty"},
		{name: "leading dash", arg: "-main", wantErr: true, errMsg: "must not start with '-'"},
		{name: "leading double dash", arg: "--upload-pack=evil", wantErr: true, errMsg: "must not start with '-'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArg(tt.arg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
		errMsg  string
	}{
		{name: "simple branch", ref: "main", wantErr: false},
		{name: "feature branch", ref: "feature/auth", wantErr: false},
		{name: "hash", ref: "abc123", wantErr: false},
		{name: "HEAD tilde", ref: "HEAD~3", wantErr: false},
		{name: "HEAD caret", ref: "HEAD^2", wantErr: false},
		{name: "tag version", ref: "v1.2.3", wantErr: false},

		// Git-specific rules
		{name: "starts with dot", ref: ".hidden", wantErr: true, errMsg: "start or end with a dot"},
		{name: "ends with dot", ref: "branch.", wantErr: true, errMsg: "start or end with a dot"},
		{name: "double dot", ref: "a..b", wantErr: true, errMsg: "'..'"},
		{name: "double slash", ref: "a//b", wantErr: true, errMsg: "'//'"},
		{name: "at-brace", ref: "HEAD@{bad", wantErr: true, errMsg: "'@{'"},
		{name: "ends with .lock", ref: "branch.lock", wantErr: true, errMsg: "'.lock'"},

		// Shell metachars
		{name: "semicolon in ref", ref: "a;b", wantErr: true, errMsg: "forbidden character"},
		{name: "pipe in ref", ref: "a|b", wantErr: true, errMsg: "forbidden character"},

		// Leading-dash option injection (CWE-88)
		{name: "leading dash", ref: "-d", wantErr: true, errMsg: "must not start with '-'"},
		{name: "leading double dash", ref: "--force", wantErr: true, errMsg: "must not start with '-'"},
		{name: "leading dash long opt", ref: "--help", wantErr: true, errMsg: "must not start with '-'"},

		// Edge cases
		{name: "empty ref", ref: "", wantErr: true, errMsg: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRef(tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	absolutePath := filepath.Join(os.TempDir(), "escape.txt")

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{name: "simple file", path: "file.txt", wantErr: false},
		{name: "nested path", path: "src/main.go", wantErr: false},
		{name: "path with spaces", path: "my file.txt", wantErr: false},
		{name: "windows path", path: "src\\main.go", wantErr: false},

		// Forbidden characters
		{name: "semicolon", path: "f;le.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "pipe", path: "f|le.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "ampersand", path: "f&le.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "dollar", path: "f$le.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "backtick", path: "f`le.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "newline", path: "f\nle.txt", wantErr: true, errMsg: "forbidden character"},
		{name: "null byte", path: "f\x00le.txt", wantErr: true, errMsg: "null byte"},
		{name: "leading dash path", path: "--evil", wantErr: true, errMsg: "must not start with '-'"},
		{name: "parent traversal", path: "../escape.txt", wantErr: true, errMsg: "must not contain '..'"},
		{name: "nested traversal", path: "sub/../escape.txt", wantErr: true, errMsg: "must not contain '..'"},
		{name: "absolute path is allowed for read-only callers", path: absolutePath, wantErr: false},

		// Edge cases
		{name: "empty path", path: "", wantErr: true, errMsg: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRepoRelativePath(t *testing.T) {
	absolutePath := filepath.Join(os.TempDir(), "escape.txt")

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{name: "relative path", path: "file.txt", wantErr: false},
		{name: "absolute path", path: absolutePath, wantErr: true, errMsg: "must not be absolute"},
		{name: "traversal path", path: "../escape.txt", wantErr: true, errMsg: "must not contain '..'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoRelativePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{name: "safe args", args: []string{"log", "--oneline", "HEAD~1"}},
		{name: "safe args with separator", args: []string{"show", "--", "-literal-file"}},
		{name: "semicolon", args: []string{"show", "main;rm -rf /"}, wantErr: true, errMsg: "forbidden character"},
		{name: "pipe", args: []string{"show", "main|cat"}, wantErr: true, errMsg: "forbidden character"},
		{name: "null byte", args: []string{"show", "main\x00tail"}, wantErr: true, errMsg: "null byte"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArgs(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMutatingOperationsRejectUnsafePaths(t *testing.T) {
	c, err := NewClient(".")
	require.NoError(t, err)

	hunk := Hunk{Lines: []DiffLine{{Type: DiffLineAdded, Content: "x"}}}
	absolutePath := filepath.Join(os.TempDir(), "escape.txt")
	ctx := context.Background()

	tests := []struct {
		name   string
		call   func() error
		errMsg string
	}{
		{
			name:   "stage traversal",
			call:   func() error { return c.Stage(ctx, []string{"../escape.txt"}) },
			errMsg: "must not contain '..'",
		},
		{
			name:   "unstage absolute",
			call:   func() error { return c.Unstage(ctx, []string{absolutePath}) },
			errMsg: "must not be absolute",
		},
		{
			name:   "stage hunk traversal",
			call:   func() error { return c.StageHunk(ctx, "../escape.txt", hunk) },
			errMsg: "must not contain '..'",
		},
		{
			name:   "stage line absolute",
			call:   func() error { return c.StageLine(ctx, absolutePath, hunk, 0) },
			errMsg: "must not be absolute",
		},
		{
			name:   "discard traversal",
			call:   func() error { return c.DiscardFile(ctx, "../escape.txt") },
			errMsg: "must not contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestCheckGitInstalled(t *testing.T) {
	// Git is expected to be installed in the test environment.
	err := CheckGitInstalled()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Additional flag-injection tests (CWE-88)
// ---------------------------------------------------------------------------

// TestValidateRef_OptionInjectionPatterns verifies that ValidateRef rejects
// all forms of option-injection via leading dashes. Even with exec.Command,
// passing user-supplied refs that look like flags to git sub-commands would
// cause git to misinterpret them as CLI options.
func TestValidateRef_OptionInjectionPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ref    string
		errMsg string
	}{
		// --config= option injection (can override core.gitProxy, etc.)
		{"--config injection", "--config=user.email=attacker@evil.com", "must not start with '-'"},
		// --upload-pack= injection (arbitrary command execution during clone/fetch)
		{"--upload-pack injection", "--upload-pack=evil-cmd", "must not start with '-'"},
		// --exec= injection
		{"--exec injection", "--exec=malicious", "must not start with '-'"},
		// Short single-char option
		{"-o option", "-o", "must not start with '-'"},
		// Combined short options that look like flags
		{"-rf combined", "-rf", "must not start with '-'"},
		// Double-dash alone is sometimes used as a separator
		{"bare --", "--", "must not start with '-'"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRef(tt.ref)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestValidateRef_InvalidUTF8 verifies that refs containing invalid UTF-8
// byte sequences are rejected. Such inputs can bypass regex-based validators
// that operate on runes rather than raw bytes.
func TestValidateRef_InvalidUTF8(t *testing.T) {
	t.Parallel()

	invalidInputs := []struct {
		name  string
		bytes []byte
	}{
		{"lone continuation byte", []byte{0x80, 0x62, 0x72}},
		{"overlong encoding", []byte{0xc0, 0xaf, 0x62, 0x72}},
		{"invalid start byte", []byte{0xff, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68}},
		{"truncated multibyte", []byte{0xe2, 0x28, 0xa1}},
	}

	for _, tt := range invalidInputs {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRef(string(tt.bytes))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "UTF-8")
		})
	}
}

// TestValidateArg_InvalidUTF8 mirrors the ref test for general arguments.
func TestValidateArg_InvalidUTF8(t *testing.T) {
	t.Parallel()

	invalidInputs := []struct {
		name  string
		bytes []byte
	}{
		{"lone continuation byte", []byte{0x80, 0x61, 0x72, 0x67}},
		{"invalid start byte", []byte{0xff, 0xfe, 0x61, 0x72, 0x67}},
	}

	for _, tt := range invalidInputs {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateArg(string(tt.bytes))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "UTF-8")
		})
	}
}

// TestValidatePath_InvalidUTF8 mirrors the ref test for file paths.
func TestValidatePath_InvalidUTF8(t *testing.T) {
	t.Parallel()

	invalidInputs := []struct {
		name  string
		bytes []byte
	}{
		{"lone continuation byte", []byte{0x80, 0x70, 0x61, 0x74, 0x68}},
		{"invalid start byte", []byte{0xff, 0xfe, 0x70, 0x61, 0x74, 0x68}},
	}

	for _, tt := range invalidInputs {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePath(string(tt.bytes))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "UTF-8")
		})
	}
}

// TestValidateArg_NullByteVariants tests multiple null-byte injection patterns.
// Null bytes can truncate strings in C-backed git implementations (CWE-626).
func TestValidateArg_NullByteVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
	}{
		{"null at start", "\x00main"},
		{"null at end", "main\x00"},
		{"null in middle", "ma\x00in"},
		{"double null", "a\x00\x00b"},
		{"null only", "\x00"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateArg(tt.arg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "null byte")
		})
	}
}

// TestValidateRef_NullByteVariants mirrors null-byte tests for refs.
func TestValidateRef_NullByteVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
	}{
		{"null at start", "\x00main"},
		{"null at end", "main\x00"},
		{"null in middle", "ma\x00in"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRef(tt.ref)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "null byte")
		})
	}
}

// TestValidateArg_OversizedInput verifies that the validator handles extremely
// large inputs without panicking or hanging (DoS prevention).
func TestValidateArg_OversizedInput(t *testing.T) {
	t.Parallel()

	// 1 MiB of safe characters — must not panic or deadlock.
	huge := strings.Repeat("a", 1<<20)
	_ = ValidateArg(huge) // result doesn't matter; must complete quickly
}

// TestValidateRef_OversizedInput mirrors the oversized-input test for refs.
func TestValidateRef_OversizedInput(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("a", 1<<20)
	_ = ValidateRef(huge)
}

// TestValidatePath_OversizedInput mirrors the oversized-input test for paths.
func TestValidatePath_OversizedInput(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("a", 1<<20)
	_ = ValidatePath(huge)
}

// ---------------------------------------------------------------------------
// validateAuthor
// ---------------------------------------------------------------------------

func TestValidateAuthor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		author  string
		wantErr bool
		errMsg  string
	}{
		{name: "valid author", author: "John Doe <john@example.com>", wantErr: false},
		{name: "valid name only", author: "John Doe", wantErr: false},
		{name: "empty", author: "", wantErr: true, errMsg: "must not be empty"},
		{name: "semicolon", author: "John; rm -rf /", wantErr: true, errMsg: "forbidden character"},
		{name: "pipe", author: "John | cat /etc/passwd", wantErr: true, errMsg: "forbidden character"},
		{name: "ampersand", author: "John & whoami", wantErr: true, errMsg: "forbidden character"},
		{name: "dollar", author: "John $HOME", wantErr: true, errMsg: "forbidden character"},
		{name: "backtick", author: "John `id`", wantErr: true, errMsg: "forbidden character"},
		{name: "backslash", author: `John\nDoe`, wantErr: true, errMsg: "forbidden character"},
		{name: "newline", author: "John\nDoe", wantErr: true, errMsg: "forbidden character"},
		{name: "null byte", author: "John\x00Doe", wantErr: true, errMsg: "null byte"},
		{name: "leading dash", author: "--exec=evil", wantErr: true, errMsg: "must not start with '-'"},
		{name: "angle brackets allowed", author: "John <john@example.com>", wantErr: false},
		{name: "invalid utf8", author: "John\xff\xfe", wantErr: true, errMsg: "invalid UTF-8"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateAuthor(tc.author)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
