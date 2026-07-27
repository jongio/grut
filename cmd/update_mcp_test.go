package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// newUpdateCmd — structure and execution paths
// ---------------------------------------------------------------------------

func TestUpdateCmd_StructureHasRunE(t *testing.T) {
	cmd := newUpdateCmd()
	assert.NotNil(t, cmd.RunE, "update command must have RunE set")
	assert.Equal(t, "update", cmd.Use)
}

func TestUpdateCmd_HasLongDescription(t *testing.T) {
	cmd := newUpdateCmd()
	assert.NotEmpty(t, cmd.Long, "update command should have a detailed description")
	assert.Contains(t, cmd.Long, "SHA-256",
		"long description should mention checksum verification")
}

func TestUpdateCmd_ErrorWrapping(t *testing.T) {
	// Point os.UserConfigDir at a temp directory. RunUpdate takes an
	// exclusive lock under the user config dir, and go test runs package
	// binaries concurrently, so without this the internal/update package's
	// RunUpdate tests race with this one over the same lock file.
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Unix
	t.Setenv("HOME", dir)            // macOS, and Unix fallback

	// Running "grut update" will call update.RunUpdate which fails for
	// dev builds. This exercises the error wrapping path in RunE.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"update"})

	err := root.Execute()
	// update.RunUpdate should fail for test/dev builds because AppVersion
	// won't match a real release. This exercises the error path.
	if err != nil {
		assert.Contains(t, err.Error(), "update:",
			"error should be wrapped with 'update:' prefix")
	}
	// If it somehow succeeds (unlikely in tests), that's also fine.
}

func TestUpdateCmd_ViaRootCommand(t *testing.T) {
	// Verify the update subcommand is reachable through the root.
	root, cleanup := buildRootCommand()
	defer cleanup()

	found := false
	for _, sub := range root.Commands() {
		if sub.Name() == "update" {
			found = true
			break
		}
	}
	require.True(t, found, "update subcommand must be registered on root")
}

// ---------------------------------------------------------------------------
// newMCPCmd — execution paths
// ---------------------------------------------------------------------------

func TestMCPCmd_StructureHasRunE(t *testing.T) {
	cmd := newMCPCmd()
	assert.NotNil(t, cmd.RunE, "mcp command must have RunE set")
}

func TestMCPCmd_HasLongDescription(t *testing.T) {
	cmd := newMCPCmd()
	assert.NotEmpty(t, cmd.Long, "mcp command should have a detailed description")
	assert.Contains(t, cmd.Long, "MCP",
		"long description should mention MCP")
	assert.Contains(t, cmd.Long, "stdin/stdout",
		"long description should mention stdin/stdout transport")
}

func TestMCPCmd_SocketFlagDefault(t *testing.T) {
	cmd := newMCPCmd()
	flag := cmd.Flags().Lookup("socket")
	require.NotNil(t, flag)
	assert.Equal(t, "", flag.DefValue, "socket flag should default to empty")
}

func TestMCPCmd_NoSocketUsesStdio(t *testing.T) {
	// Running mcp without --socket should attempt stdio mode.
	// It will fail because there's no real git repo in the test environment,
	// but this exercises the config load → git client path.
	cmd := newMCPCmd()
	// Add --no-ai flag since it's a persistent flag from root
	cmd.PersistentFlags().Bool("no-ai", false, "")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	// Should fail at config.Load(), git.NewClient(), or similar — not at
	// the socket check. The error should NOT contain "not yet implemented".
	if err != nil {
		assert.NotContains(t, err.Error(), "not yet implemented",
			"without --socket, should not hit socket-not-implemented error")
	}
}

func TestMCPCmd_SocketNotImplementedError(t *testing.T) {
	// Verify the exact error message for socket mode.
	cmd := newMCPCmd()
	cmd.PersistentFlags().Bool("no-ai", false, "")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--socket", "/var/run/grut.sock"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "socket mode is not yet implemented")
}
