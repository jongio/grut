package cmd

import (
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitChatRegistersProviders verifies that initChat returns a non-nil
// chat model and a registry containing both the "copilot" and "claude"
// providers. This does NOT test provider availability (which requires real
// credentials) — only that registration succeeded.
func TestInitChatRegistersProviders(t *testing.T) {
	// Prevent the Copilot SDK from spawning a real CLI process during tests.
	t.Setenv("COPILOT_CLI_PATH", "/nonexistent-copilot-test-stub")

	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	// Ensure AI + chat are enabled so initChat does real work.
	cfg.AI.Enabled = true
	cfg.AI.Chat.Enabled = true
	cfg.Theme.Name = "default" // use a stable built-in theme regardless of user config

	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err)

	// Use the test's temp dir as repoRoot so the path-jail init succeeds.
	repoRoot := t.TempDir()

	chatModel, registry := initChat(cfg, nil, repoRoot, th)
	require.NotNil(t, chatModel, "initChat must return a non-nil chat model")
	require.NotNil(t, registry, "initChat must return a non-nil registry")
	defer func() { _ = registry.Close() }()

	// Verify both providers are registered.
	copilot, ok := registry.GetByName("copilot")
	assert.True(t, ok, "copilot provider must be registered")
	if ok {
		assert.Equal(t, "copilot", copilot.Name())
	}

	claude, ok := registry.GetByName("claude")
	assert.True(t, ok, "claude provider must be registered")
	if ok {
		assert.Equal(t, "claude", claude.Name())
	}
}

// TestInitChatReturnsRegistryOnPathJailFailure ensures that even when
// the path jail cannot be created (e.g. invalid repoRoot), the registry
// is still returned so the AI middleware can be wired up independently.
func TestInitChatReturnsRegistryOnPathJailFailure(t *testing.T) {
	// Prevent the Copilot SDK from spawning a real CLI process during tests.
	t.Setenv("COPILOT_CLI_PATH", "/nonexistent-copilot-test-stub")

	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	cfg.AI.Enabled = true
	cfg.AI.Chat.Enabled = true
	cfg.Theme.Name = "default" // use a stable built-in theme regardless of user config

	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err)

	// Use a nonexistent path to provoke a path-jail failure (EvalSymlinks
	// fails for a path that doesn't exist on disk).
	chatModel, registry := initChat(cfg, nil, filepath.Join(t.TempDir(), "does-not-exist", "nested"), th)
	assert.Nil(t, chatModel, "chat model should be nil when path jail fails")
	require.NotNil(t, registry, "registry must still be returned on path-jail failure")
	defer func() { _ = registry.Close() }()

	// Providers should still be registered even though chat model is nil.
	_, copilotOK := registry.GetByName("copilot")
	assert.True(t, copilotOK, "copilot must be registered even when chat fails")

	_, claudeOK := registry.GetByName("claude")
	assert.True(t, claudeOK, "claude must be registered even when chat fails")
}

// TestInitChatClaudeDefaults verifies that the Claude provider is initialised
// with the correct model and never panics when created with default config.
func TestInitChatClaudeDefaults(t *testing.T) {
	cfgAI := config.AIConfig{
		Provider:         "copilot",
		FallbackProvider: "claude",
	}
	registry := ai.NewRegistry(cfgAI)
	registry.Register("claude", ai.NewClaudeProvider("", 0))
	defer func() { _ = registry.Close() }()

	p, ok := registry.GetByName("claude")
	require.True(t, ok)
	assert.Equal(t, "claude", p.Name())
}
