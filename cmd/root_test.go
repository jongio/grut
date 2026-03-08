package cmd

import (
	"os"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoAIFlagRegistered(t *testing.T) {
	root := NewRootCommand()
	flag := root.PersistentFlags().Lookup("no-ai")
	require.NotNil(t, flag, "--no-ai persistent flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Disable AI features for this operation", flag.Usage)
}

func TestNoAIFlagInheritedBySubcommands(t *testing.T) {
	root := NewRootCommand()

	// Every subcommand should inherit the persistent --no-ai flag.
	for _, sub := range root.Commands() {
		flag := sub.InheritedFlags().Lookup("no-ai")
		assert.NotNil(t, flag, "subcommand %q must inherit --no-ai", sub.Name())
	}
}

func TestApplyNoAIFlag_WhenSet(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("no-ai", false, "")
	require.NoError(t, cmd.Flags().Set("no-ai", "true"))

	cfg := &config.Config{}
	cfg.AI.Enabled = true

	applyNoAIFlag(cmd, cfg)
	assert.False(t, cfg.AI.Enabled, "AI should be disabled when --no-ai is set")
}

func TestApplyNoAIFlag_WhenNotSet(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("no-ai", false, "")

	cfg := &config.Config{}
	cfg.AI.Enabled = true

	applyNoAIFlag(cmd, cfg)
	assert.True(t, cfg.AI.Enabled, "AI should remain enabled when --no-ai is not set")
}

func TestApplyNoAIFlag_PreservesDisabledState(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("no-ai", false, "")

	cfg := &config.Config{}
	cfg.AI.Enabled = false

	applyNoAIFlag(cmd, cfg)
	assert.False(t, cfg.AI.Enabled, "AI should stay disabled when already disabled")
}

func TestNoAIFlagParsing(t *testing.T) {
	root := NewRootCommand()

	// Simulate: grut --no-ai version
	// Version subcommand doesn't start the TUI so it's safe to execute.
	root.SetArgs([]string{"--no-ai", "version"})
	err := root.Execute()
	assert.NoError(t, err, "running 'grut --no-ai version' should succeed")

	flag, err := root.Flags().GetBool("no-ai")
	require.NoError(t, err)
	assert.True(t, flag, "--no-ai should be true after parsing")
}

// ---------------------------------------------------------------------------
// openLogPath (GRUT_LOG validation)
// ---------------------------------------------------------------------------

func TestOpenLogPath_ValidAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	logPath := tmp + string(os.PathSeparator) + "grut.log"

	f := openLogPath(logPath)
	require.NotNil(t, f, "absolute path should open successfully")
	defer f.Close()

	// File should be created with append mode.
	_, err := f.WriteString("test\n")
	assert.NoError(t, err)
}

func TestOpenLogPath_RelativePathRejected(t *testing.T) {
	f := openLogPath("relative/path/grut.log")
	assert.Nil(t, f, "relative path must be rejected")
}

func TestOpenLogPath_EmptyStringRejected(t *testing.T) {
	f := openLogPath("")
	assert.Nil(t, f, "empty string must be rejected")
}

func TestOpenLogPath_UNCBackslashRejected(t *testing.T) {
	f := openLogPath(`\\evil-server\share\grut.log`)
	assert.Nil(t, f, "UNC backslash path must be rejected")
}

func TestOpenLogPath_UNCForwardSlashRejected(t *testing.T) {
	f := openLogPath("//evil-server/share/grut.log")
	assert.Nil(t, f, "UNC forward-slash path must be rejected")
}

func TestOpenLogPath_NonexistentDirReturnsNil(t *testing.T) {
	f := openLogPath(t.TempDir() + string(os.PathSeparator) + "does-not-exist" + string(os.PathSeparator) + "grut.log")
	assert.Nil(t, f, "non-existent directory should return nil (silent fallback)")
}
