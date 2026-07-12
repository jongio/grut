package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/overlayreg"
	"github.com/jongio/grut/internal/session"
	"github.com/jongio/grut/internal/theme"
	"github.com/jongio/grut/internal/tui"
	"github.com/jongio/grut/internal/update"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoAIFlagRegistered(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()
	flag := root.PersistentFlags().Lookup("no-ai")
	require.NotNil(t, flag, "--no-ai persistent flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Disable AI features for this operation", flag.Usage)
}

func TestNoAIFlagInheritedBySubcommands(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

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
	root, cleanup := newRootCommand()
	defer cleanup()

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
// showUpdateNotification
// ---------------------------------------------------------------------------

func TestShowUpdateNotification_NilInfo(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan *update.UpdateInfo, 1)
	ch <- nil
	showUpdateNotification(&buf, ch)
	assert.Empty(t, buf.String(), "nil UpdateInfo should produce no output")
}

func TestShowUpdateNotification_WithUpdate(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan *update.UpdateInfo, 1)
	ch <- &update.UpdateInfo{
		CurrentVersion: "1.0.0",
		LatestVersion:  "2.0.0",
		ReleaseURL:     "https://github.com/jongio/grut/releases/tag/v2.0.0",
	}
	showUpdateNotification(&buf, ch)
	output := buf.String()
	assert.Contains(t, output, "1.0.0")
	assert.Contains(t, output, "2.0.0")
	assert.Contains(t, output, "grut update")
}

func TestShowUpdateNotification_EmptyChannel(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan *update.UpdateInfo, 1)
	// Don't send anything — default branch in select.
	showUpdateNotification(&buf, ch)
	assert.Empty(t, buf.String(), "empty channel should produce no output")
}

func TestShowUpdateNotification_NilWriter(t *testing.T) {
	ch := make(chan *update.UpdateInfo, 1)
	ch <- nil
	// nil writer falls back to os.Stderr; should not panic.
	assert.NotPanics(t, func() {
		showUpdateNotification(nil, ch)
	})
}

// ---------------------------------------------------------------------------
// newRootCommand — subcommand registration
// ---------------------------------------------------------------------------

func TestNewRootCommand_SubcommandsRegistered(t *testing.T) {
	cmd, cleanup := newRootCommand()
	defer cleanup()
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	assert.True(t, names["run"], "run subcommand must be registered")
	assert.True(t, names["version"], "version subcommand must be registered")
	assert.True(t, names["update"], "update subcommand must be registered")
	assert.True(t, names["mcp"], "mcp subcommand must be registered")
	assert.True(t, names["ext"], "ext subcommand must be registered")
	assert.True(t, names["report"], "report subcommand must be registered")
}

func TestNewRootCommand_VersionSet(t *testing.T) {
	cmd, cleanup := newRootCommand()
	defer cleanup()
	assert.Equal(t, config.AppVersion, cmd.Version)
}

func TestNewRootCommand_DemoFlagRegistered(t *testing.T) {
	cmd, cleanup := newRootCommand()
	defer cleanup()
	flag := cmd.PersistentFlags().Lookup("demo")
	require.NotNil(t, flag, "--demo persistent flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestRootCommandHasProfilingFlags(t *testing.T) {
	cmd, cleanup := newRootCommand()
	defer cleanup()

	cpuFlag := cmd.PersistentFlags().Lookup("cpu-profile")
	assert.NotNil(t, cpuFlag, "--cpu-profile persistent flag must be registered")
	assert.Equal(t, "string", cpuFlag.Value.Type())
	assert.Equal(t, "", cpuFlag.DefValue)

	memFlag := cmd.PersistentFlags().Lookup("mem-profile")
	assert.NotNil(t, memFlag, "--mem-profile persistent flag must be registered")
	assert.Equal(t, "string", memFlag.Value.Type())
	assert.Equal(t, "", memFlag.DefValue)
}

func TestProfilingFlags_InheritedBySubcommands(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()
	for _, sub := range root.Commands() {
		assert.NotNil(t, sub.InheritedFlags().Lookup("cpu-profile"),
			"subcommand %q must inherit --cpu-profile", sub.Name())
		assert.NotNil(t, sub.InheritedFlags().Lookup("mem-profile"),
			"subcommand %q must inherit --mem-profile", sub.Name())
	}
}

func TestCPUProfile_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	profPath := filepath.Join(tmp, "cpu.prof")

	root, cleanup := buildRootCommand()
	defer cleanup()
	root.SetArgs([]string{"--cpu-profile", profPath, "version"})
	err := root.Execute()
	assert.NoError(t, err)

	// Run cleanup now so the profile is flushed before we stat the file.
	cleanup()

	info, err := os.Stat(profPath)
	require.NoError(t, err, "CPU profile file must be created")
	assert.Greater(t, info.Size(), int64(0), "CPU profile must contain data")
}

func TestMemProfile_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	profPath := filepath.Join(tmp, "mem.prof")

	root, cleanup := buildRootCommand()
	defer cleanup()
	root.SetArgs([]string{"--mem-profile", profPath, "version"})
	err := root.Execute()
	assert.NoError(t, err)

	// Run cleanup now so the memory profile is written before we stat.
	cleanup()

	info, err := os.Stat(profPath)
	require.NoError(t, err, "memory profile file must be created")
	assert.Greater(t, info.Size(), int64(0), "memory profile must contain data")
}

func TestNewRootCommand_UseAndShort(t *testing.T) {
	cmd, cleanup := newRootCommand()
	defer cleanup()
	assert.Equal(t, "grut [path]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	// The root command accepts at most one positional path argument.
	assert.NoError(t, cmd.Args(cmd, []string{"somefile.go"}))
	assert.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

// ---------------------------------------------------------------------------
// restoreSessionOrDefault
// ---------------------------------------------------------------------------

func TestRestoreSessionOrDefault_SessionsDisabled(t *testing.T) {
	mgr := session.NewManager()
	cfg := &config.Config{}
	cfg.Session.Enabled = false

	preset := restoreSessionOrDefault(mgr, cfg, t.TempDir(), nil)
	// When sessions are disabled, should return the default ExplorerPreset.
	expected := layout.ExplorerPreset()
	assert.Equal(t, expected.Name, preset.Name)
}

func TestRestoreSessionOrDefault_NoSavedSession(t *testing.T) {
	mgr := session.NewManager()
	cfg := &config.Config{}
	cfg.Session.Enabled = true

	// Use a temp dir where no session file exists.
	preset := restoreSessionOrDefault(mgr, cfg, t.TempDir(), nil)
	expected := layout.ExplorerPreset()
	assert.Equal(t, expected.Name, preset.Name)
}

// ---------------------------------------------------------------------------
// addRestoredTabs — single-tab mode guard
// ---------------------------------------------------------------------------

func TestAddRestoredTabs_SingleTabMode(t *testing.T) {
	// In v1, SingleTabMode is true and addRestoredTabs returns immediately.
	// This test just verifies it doesn't panic.
	if !layout.SingleTabMode {
		t.Skip("SingleTabMode is false; test not applicable")
	}
	mgr := session.NewManager()
	reg := layout.NewRegistry()
	cfg := &config.Config{}
	th, err := theme.Load("default")
	require.NoError(t, err)
	layout.RegisterDefaults(context.Background(), reg, cfg, nil, th)
	engine, err := layout.NewEngine(reg, layout.ExplorerPreset())
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		addRestoredTabs(mgr, engine, t.TempDir())
	})
}

// ---------------------------------------------------------------------------
// newExtCmd — subcommands
// ---------------------------------------------------------------------------

func TestNewExtCmd_SubcommandsRegistered(t *testing.T) {
	cmd := newExtCmd()
	assert.Equal(t, "ext", cmd.Use)
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	assert.True(t, names["install"], "install subcommand must be registered")
	assert.True(t, names["remove"], "remove subcommand must be registered")
	assert.True(t, names["list"], "list subcommand must be registered")
	assert.True(t, names["enable"], "enable subcommand must be registered")
	assert.True(t, names["disable"], "disable subcommand must be registered")
	assert.True(t, names["create"], "create subcommand must be registered")
	assert.True(t, names["info"], "info subcommand must be registered")
}

// ---------------------------------------------------------------------------
// newMCPCmd — structure
// ---------------------------------------------------------------------------

func TestNewMCPCmd_SocketFlagRegistered(t *testing.T) {
	cmd := newMCPCmd()
	assert.Equal(t, "mcp", cmd.Use)
	flag := cmd.Flags().Lookup("socket")
	require.NotNil(t, flag, "--socket flag must be registered")
	assert.Equal(t, "string", flag.Value.Type())
	assert.Equal(t, "", flag.DefValue)
}

// ---------------------------------------------------------------------------
// newVersionCmd / newUpdateCmd — structure
// ---------------------------------------------------------------------------

func TestNewVersionCmd_Structure(t *testing.T) {
	cmd := newVersionCmd()
	assert.Equal(t, "version", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}

func TestNewUpdateCmd_Structure(t *testing.T) {
	cmd := newUpdateCmd()
	assert.Equal(t, "update", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

// ---------------------------------------------------------------------------
// newMCPCmd — execution paths
// ---------------------------------------------------------------------------

func TestNewMCPCmd_SocketNotImplemented(t *testing.T) {
	// mcp --socket <path> should error with "not yet implemented".
	cmd := newMCPCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--socket", "/tmp/test.sock"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

// ---------------------------------------------------------------------------
// Run subcommand — execution via root
// ---------------------------------------------------------------------------

func TestRunCmd_NoArgs_RequiresShortcutName(t *testing.T) {
	// Running "run" with no args should produce an error mentioning
	// either "shortcut name required" or "shortcuts are disabled"
	// depending on config. Either way, it exercises the RunE path.
	root, cleanup := newRootCommand()
	defer cleanup()
	root.SetArgs([]string{"run"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	err := root.Execute()
	assert.Error(t, err)
}

func TestRunCmd_DescribeUnknownShortcut(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()
	root.SetArgs([]string{"run", "--describe", "nonexistent-shortcut"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	err := root.Execute()
	// This will either fail because shortcuts are disabled, or
	// because the shortcut is unknown. Both exercise code paths.
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// --reset-welcome flag (from main)
// ---------------------------------------------------------------------------

func TestResetWelcomeFlagRegistered(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()
	flag := root.PersistentFlags().Lookup("reset-welcome")
	require.NotNil(t, flag, "--reset-welcome persistent flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestResetWelcomeFlagInheritedBySubcommands(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	for _, sub := range root.Commands() {
		flag := sub.InheritedFlags().Lookup("reset-welcome")
		assert.NotNil(t, flag, "subcommand %q must inherit --reset-welcome", sub.Name())
	}
}

func TestResetWelcomeState_ResetsMarker(t *testing.T) {
	require.NoError(t, session.MarkFirstRunDone())
	assert.False(t, session.IsFirstRun(), "precondition: should not be first run")

	require.NoError(t, resetWelcomeState())
	assert.True(t, session.IsFirstRun(), "should be first run after resetWelcomeState")

	require.NoError(t, session.MarkFirstRunDone())
}

// ---------------------------------------------------------------------------
// Startup smoke tests — exercise the full init chain that RunE performs
// ---------------------------------------------------------------------------

func TestDefaultConfigThemeIsValid(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err, "LoadDefaults must succeed")

	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err, "theme %q from defaults must load", cfg.Theme.Name)
	assert.NotNil(t, th)

	builtins := theme.BuiltinNames()
	assert.Contains(t, builtins, cfg.Theme.Name,
		"default config theme %q must be a builtin theme", cfg.Theme.Name)
}

func TestInitChainSucceeds(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err, "config.LoadDefaults")

	th, err := theme.Load(cfg.Theme.Name)
	require.NoError(t, err, "theme.Load(%q)", cfg.Theme.Name)

	reg := layout.NewRegistry()
	layout.RegisterDefaults(context.Background(), reg, cfg, nil, th)

	preset := layout.ExplorerPreset()
	engine, err := layout.NewEngine(reg, preset)
	require.NoError(t, err, "layout.NewEngine")

	km, err := keymap.NewKeymap(cfg.General.KeybindingScheme)
	require.NoError(t, err, "keymap.NewKeymap(%q)", cfg.General.KeybindingScheme)

	model := tui.New(engine, th, km, nil, overlayreg.New(th, nil)).
		WithConfig(cfg)
	assert.NotNil(t, model, "tui.New must produce a non-nil model")
}
