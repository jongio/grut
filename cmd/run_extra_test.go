package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// newRunCmd — execution paths via root command
// ---------------------------------------------------------------------------

func TestRunCmd_ListFlag(t *testing.T) {
	// "grut run --list" should load config and list shortcuts.
	// This exercises the listFlag branch in RunE.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run", "--list"})

	err := root.Execute()
	if err != nil {
		// If shortcuts are disabled in config, the error message tells us.
		assert.Contains(t, err.Error(), "shortcuts are disabled",
			"if --list fails, it should be because shortcuts are disabled")
	} else {
		// If shortcuts are enabled, we should see the list output.
		output := buf.String()
		assert.Contains(t, output, "shortcut",
			"--list output should mention shortcuts")
	}
}

func TestRunCmd_DescribeNonexistent(t *testing.T) {
	// "grut run --describe nonexistent" should fail with unknown shortcut.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run", "--describe", "does-not-exist-xyz"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestRunCmd_DryRunNonexistent(t *testing.T) {
	// "grut run --dry-run nonexistent" should fail.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run", "--dry-run", "does-not-exist-xyz"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestRunCmd_NoArgsError(t *testing.T) {
	// "grut run" with no shortcut name should fail.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestRunCmd_UnknownShortcut(t *testing.T) {
	// "grut run unknown-xyz" should fail with unknown shortcut.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run", "unknown-shortcut-xyz"})

	err := root.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// newRunCmd — flag interaction
// ---------------------------------------------------------------------------

func TestRunCmd_NoConfirmFlag(t *testing.T) {
	cmd := newRunCmd()
	flag := cmd.Flags().Lookup("no-confirm")
	assert.NotNil(t, flag, "--no-confirm flag must be registered")
	assert.Equal(t, "false", flag.DefValue)
}

func TestRunCmd_DryRunFlag(t *testing.T) {
	cmd := newRunCmd()
	flag := cmd.Flags().Lookup("dry-run")
	assert.NotNil(t, flag, "--dry-run flag must be registered")
	assert.Equal(t, "false", flag.DefValue)
}

func TestRunCmd_ListFlagRegistered(t *testing.T) {
	cmd := newRunCmd()
	flag := cmd.Flags().Lookup("list")
	assert.NotNil(t, flag, "--list flag must be registered")
	assert.Equal(t, "false", flag.DefValue)
}

func TestRunCmd_DescribeFlagRegistered(t *testing.T) {
	cmd := newRunCmd()
	flag := cmd.Flags().Lookup("describe")
	assert.NotNil(t, flag, "--describe flag must be registered")
	assert.Equal(t, "", flag.DefValue)
}
