package cmd

import (
	"os"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/session"
	"github.com/jongio/grut/internal/theme"
	"github.com/jongio/grut/internal/tui"
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

// ---------------------------------------------------------------------------
// --reset-welcome flag
// ---------------------------------------------------------------------------

func TestResetWelcomeFlagRegistered(t *testing.T) {
	root := NewRootCommand()
	flag := root.PersistentFlags().Lookup("reset-welcome")
	require.NotNil(t, flag, "--reset-welcome persistent flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestResetWelcomeFlagInheritedBySubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, sub := range root.Commands() {
		flag := sub.InheritedFlags().Lookup("reset-welcome")
		assert.NotNil(t, flag, "subcommand %q must inherit --reset-welcome", sub.Name())
	}
}

func TestResetWelcomeState_ResetsMarker(t *testing.T) {
	// Ensure first-run is marked done, then reset and verify.
	require.NoError(t, session.MarkFirstRunDone())
	assert.False(t, session.IsFirstRun(), "precondition: should not be first run")

	require.NoError(t, resetWelcomeState())
	assert.True(t, session.IsFirstRun(), "should be first run after resetWelcomeState")

	// Re-mark so other tests relying on global state aren't affected.
	require.NoError(t, session.MarkFirstRunDone())
}

// ---------------------------------------------------------------------------
// Startup smoke tests — exercise the full init chain that RunE performs
// ---------------------------------------------------------------------------

// TestDefaultConfigThemeIsValid ensures the theme name in embedded defaults
// resolves to a loadable theme. This prevents a removed/renamed theme in
// defaults.toml from silently breaking app startup.
func TestDefaultConfigThemeIsValid(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err, "LoadDefaults must succeed")

	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err, "theme %q from defaults must load", cfg.Theme.Name)
	assert.NotNil(t, th)

	// Also verify the name is in the builtin list.
	builtins := theme.BuiltinNames()
	assert.Contains(t, builtins, cfg.Theme.Name,
		"default config theme %q must be a builtin theme", cfg.Theme.Name)
}

// TestInitChainSucceeds exercises every initialization step that RunE
// performs before starting the TUI. If this test fails, the app cannot
// launch — exactly the class of bug where "all tests pass but the app
// won't start."
func TestInitChainSucceeds(t *testing.T) {
	// 1. Load config (using defaults to avoid user-config dependency).
	cfg, err := config.LoadDefaults()
	require.NoError(t, err, "config.LoadDefaults")

	// 2. Load theme.
	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err, "theme.Load(%q)", cfg.Theme.Name)

	// 3. Create panel registry with defaults.
	reg := layout.NewRegistry()
	layout.RegisterDefaults(reg, cfg, nil, th) // nil git client is valid

	// 4. Create layout engine.
	preset := layout.ExplorerPreset()
	engine, err := layout.NewEngine(reg, preset)
	require.NoError(t, err, "layout.NewEngine")

	// 5. Create keymap.
	km, err := keymap.NewKeymap(cfg.General.KeybindingScheme)
	require.NoError(t, err, "keymap.NewKeymap(%q)", cfg.General.KeybindingScheme)

	// 6. Assemble the TUI model (final step before tea.NewProgram).
	model := tui.New(engine, th, km, nil).
		WithConfig(cfg)
	assert.NotNil(t, model, "tui.New must produce a non-nil model")
}
