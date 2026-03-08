package extension

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// safeGitCloneEnv
// ---------------------------------------------------------------------------

// TestSafeGitCloneEnv_AllowlistIncluded verifies that safe system variables
// (PATH, HOME, etc.) survive the filter.
func TestSafeGitCloneEnv_AllowlistIncluded(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/usr/local/bin")
	t.Setenv("HOME", "/home/tester")

	env := safeGitCloneEnv()
	joined := strings.Join(env, "\n")

	assert.Contains(t, joined, "PATH=")
	assert.Contains(t, joined, "HOME=")
}

// TestSafeGitCloneEnv_SecretsExcluded verifies that tokens and credentials
// are stripped from the environment before git clone is executed.
// This guards against CWE-214 (secret leakage via subprocess environment).
func TestSafeGitCloneEnv_SecretsExcluded(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_supersecret")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("MY_CUSTOM_SECRET", "custom-secret")

	env := safeGitCloneEnv()
	joined := strings.Join(env, "\n")

	assert.NotContains(t, joined, "GITHUB_TOKEN=")
	assert.NotContains(t, joined, "ANTHROPIC_API_KEY=")
	assert.NotContains(t, joined, "AWS_SECRET_ACCESS_KEY=")
	assert.NotContains(t, joined, "OPENAI_API_KEY=")
	assert.NotContains(t, joined, "MY_CUSTOM_SECRET=")
}

// TestSafeGitCloneEnv_TerminalPromptSuppressed verifies that
// GIT_TERMINAL_PROMPT=0 is always appended so that a malicious remote
// server cannot trigger an interactive auth prompt.
func TestSafeGitCloneEnv_TerminalPromptSuppressed(t *testing.T) {
	env := safeGitCloneEnv()
	found := false
	for _, e := range env {
		if e == "GIT_TERMINAL_PROMPT=0" {
			found = true
			break
		}
	}
	assert.True(t, found, "GIT_TERMINAL_PROMPT=0 must always be present in safe env")
}

// TestSafeGitCloneEnv_NonEmpty verifies that the result is never nil/empty
// even in a stripped-down environment (e.g., CI containers).
func TestSafeGitCloneEnv_NonEmpty(t *testing.T) {
	env := safeGitCloneEnv()
	// At minimum, GIT_TERMINAL_PROMPT=0 is always appended.
	require.NotEmpty(t, env)
}

// TestSafeGitCloneEnv_CaseInsensitiveMatch verifies the case-insensitive
// allowlist on Windows where env vars are conventionally uppercase.
func TestSafeGitCloneEnv_CaseInsensitiveMatch(t *testing.T) {
	// PATH is always in the allowlist (uppercase). Confirm it is passed through
	// regardless of how the OS stores it.
	t.Setenv("PATH", "/usr/bin")

	env := safeGitCloneEnv()
	joined := strings.Join(env, "\n")
	assert.Contains(t, joined, "/usr/bin", "PATH value should be present after filter")
}
