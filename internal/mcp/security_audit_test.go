package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────────────────────────────────────────────────
// validateGitMessage — security tests
// ──────────────────────────────────────────────────────────────────────────

func TestValidateGitMessage_ValidMessages(t *testing.T) {
	t.Parallel()
	valid := []struct {
		name string
		msg  string
	}{
		{"simple", "fix: a bug"},
		{"multiline", "feat: add feature\n\nBody paragraph."},
		{"with tab", "fix: align\tcolumns"},
		{"with CR LF", "fix: windows\r\nline endings"},
		{"unicode", "feat: add emoji support \U0001F600"},
		{"empty", ""},
		{"maxlen", strings.Repeat("a", maxGitMessageLen)},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateGitMessage(tt.msg))
		})
	}
}

func TestValidateGitMessage_RejectsNullByte(t *testing.T) {
	t.Parallel()
	tests := []string{
		"WIP\x00malicious",
		"\x00leading",
		"trailing\x00",
		"mid\x00dle",
	}
	for _, msg := range tests {
		err := validateGitMessage(msg)
		require.Error(t, err, "null byte in %q must be rejected", msg)
		assert.Contains(t, err.Error(), "null byte")
	}
}

func TestValidateGitMessage_RejectsControlCharacters(t *testing.T) {
	t.Parallel()
	// Control chars 0x01-0x08, 0x0B, 0x0C, 0x0E-0x1F should be rejected.
	// 0x09 (\t), 0x0A (\n), 0x0D (\r) are allowed.
	rejectedControls := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x0B, 0x0C, 0x0E, 0x0F, 0x10, 0x1F}
	for _, c := range rejectedControls {
		msg := "test" + string([]byte{c}) + "end"
		err := validateGitMessage(msg)
		require.Errorf(t, err, "control char 0x%02x in message must be rejected", c)
		assert.Contains(t, err.Error(), "control character")
	}
}

func TestValidateGitMessage_RejectsInvalidUTF8(t *testing.T) {
	t.Parallel()
	invalidUTF8 := []string{
		"bad\xff\xfe",
		string([]byte{0xc0, 0xaf}),       // overlong encoding
		string([]byte{0xed, 0xa0, 0x80}), // surrogate half
	}
	for _, msg := range invalidUTF8 {
		err := validateGitMessage(msg)
		require.Error(t, err, "invalid UTF-8 in message must be rejected")
		assert.Contains(t, err.Error(), "invalid UTF-8")
	}
}

func TestValidateGitMessage_RejectsOversizedMessage(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("x", maxGitMessageLen+1)
	err := validateGitMessage(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum length")
}

// ──────────────────────────────────────────────────────────────────────────
// splitCamelCase — security tests
// ──────────────────────────────────────────────────────────────────────────

func TestSplitCamelCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single_lower", "request", []string{"request"}},
		{"single_upper", "A", []string{"A"}},
		{"camelCase", "requestBody", []string{"request", "Body"}},
		{"PascalCase", "RequestBody", []string{"Request", "Body"}},
		{"acronym_then_word", "APIToken", []string{"API", "Token"}},
		{"word_then_acronym", "tokenAPI", []string{"token", "API"}},
		{"multi_words", "userAuthTokenData", []string{"user", "Auth", "Token", "Data"}},
		{"all_upper", "API", []string{"API"}},
		{"all_lower", "password", []string{"password"}},
		{"two_chars", "aB", []string{"a", "B"}},
		{"numbers", "field123", []string{"field123"}},
		{"mixed_nums_upper", "get2FAToken", []string{"get2FA", "Token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitCamelCase(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// MCP tool-level message injection — commit, stash, tag
// ──────────────────────────────────────────────────────────────────────────

func TestGitTool_CommitRejectsNullByteMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		CommitFunc: func(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
			t.Fatal("commit should not be called for message with null byte")
			return "", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_commit", map[string]any{
		"message": "fix: bug\x00--amend",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "null byte")
}

func TestGitTool_CommitRejectsControlCharMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		CommitFunc: func(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
			t.Fatal("commit should not be called for control char message")
			return "", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_commit", map[string]any{
		"message": "fix\x01injection",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "control character")
}

func TestGitTool_CommitRejectsOversizedMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		CommitFunc: func(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
			t.Fatal("commit should not be called for oversized message")
			return "", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_commit", map[string]any{
		"message": strings.Repeat("x", maxGitMessageLen+1),
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "maximum length")
}

func TestGitTool_StashPushRejectsNullByteMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		StashPushFunc: func(_ context.Context, _ git.StashOpts) error {
			t.Fatal("stash push should not be called for null byte message")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_stash_push", map[string]any{
		"message": "WIP\x00--keep-index",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "null byte")
}

func TestGitTool_TagCreateRejectsNullByteMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		TagCreateFunc: func(_ context.Context, _, _, _ string) error {
			t.Fatal("tag create should not be called for null byte message")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_tag_create", map[string]any{
		"name":    "v1.0",
		"message": "Release\x00--force",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "null byte")
}

func TestGitTool_TagCreateRejectsControlCharMessage(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		TagCreateFunc: func(_ context.Context, _, _, _ string) error {
			t.Fatal("tag create should not be called for control char message")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_tag_create", map[string]any{
		"name":    "v1.0",
		"message": "Release\x07bell",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "control character")
}

// ──────────────────────────────────────────────────────────────────────────
// MCP tool-level ref injection — merge, rebase, cherry-pick, checkout
// ──────────────────────────────────────────────────────────────────────────

func TestGitTool_MergeRejectsMaliciousRef(t *testing.T) {
	t.Parallel()
	maliciousRefs := []string{
		"--upload-pack=evil",
		"; rm -rf /",
		"branch\x00--opt",
		"../etc/passwd",
		"branch\nnewline",
	}
	for _, ref := range maliciousRefs {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			mock := &mockGitClient{
				MergeFunc: func(_ context.Context, _ string, _ git.MergeOpts) error {
					t.Fatal("merge should not be called for malicious ref")
					return nil
				},
			}
			srv := newTestServer(t, mock, t.TempDir())
			result := callTool(t, srv, "git_merge", map[string]any{"branch": ref})
			assert.True(t, result.IsError, "ref %q must be rejected", ref)
		})
	}
}

func TestGitTool_CheckoutRejectsMaliciousRef(t *testing.T) {
	t.Parallel()
	maliciousRefs := []string{
		"--upload-pack=evil",
		"; echo pwned",
		"ref\x00injected",
	}
	for _, ref := range maliciousRefs {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			mock := &mockGitClient{
				CheckoutFunc: func(_ context.Context, _ string) error {
					t.Fatal("checkout should not be called for malicious ref")
					return nil
				},
			}
			srv := newTestServer(t, mock, t.TempDir())
			result := callTool(t, srv, "git_checkout", map[string]any{"ref": ref})
			assert.True(t, result.IsError, "ref %q must be rejected", ref)
		})
	}
}

func TestGitTool_CherryPickRejectsMaliciousRef(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		CherryPickFunc: func(_ context.Context, _ string) error {
			t.Fatal("cherry-pick should not be called for malicious ref")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_cherry_pick", map[string]any{
		"commit": "--upload-pack=evil",
	})
	assert.True(t, result.IsError)
}

func TestGitTool_RebaseRejectsMaliciousRef(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		RebaseFunc: func(_ context.Context, _ string, _ git.RebaseOpts) error {
			t.Fatal("rebase should not be called for malicious ref")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_rebase", map[string]any{
		"onto": "; rm -rf /",
	})
	assert.True(t, result.IsError)
}

// ──────────────────────────────────────────────────────────────────────────
// MCP tool-level branch name length — DoS prevention
// ──────────────────────────────────────────────────────────────────────────

func TestGitTool_BranchCreateRejectsOversizedName(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		BranchCreateFunc: func(_ context.Context, _, _ string) error {
			t.Fatal("branch create should not be called for oversized name")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_branch_create", map[string]any{
		"name": strings.Repeat("a", 251),
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "maximum length")
}
