package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// newExtCmd — subcommand structure (see root_test.go for registration test)
// ---------------------------------------------------------------------------

func TestExtListCmd_NoExtensions(t *testing.T) {
	// Running ext list should succeed when no extensions are installed.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No extensions installed")
}

func TestExtCreateCmd_ListTemplates(t *testing.T) {
	// ext create --list should list available templates.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create", "--list"})
	err := cmd.Execute()
	assert.NoError(t, err)
	// Should have at least one template output.
	assert.NotEmpty(t, buf.String())
}

func TestExtCreateCmd_NoName(t *testing.T) {
	// ext create with no args should error.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extension name is required")
}

func TestExtRemoveCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"remove", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtEnableCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"enable", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtDisableCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"disable", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInfoCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"info", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInstallCmd_MissingArgs(t *testing.T) {
	// install requires exactly 1 arg.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"install"})
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// newExtCreateCmd — flag registration
// ---------------------------------------------------------------------------

func TestNewExtCreateCmd_FlagsRegistered(t *testing.T) {
	cmd := newExtCreateCmd()
	templateFlag := cmd.Flags().Lookup("template")
	require.NotNil(t, templateFlag, "--template flag must be registered")
	assert.Equal(t, "lua", templateFlag.DefValue)

	listFlag := cmd.Flags().Lookup("list")
	require.NotNil(t, listFlag, "--list flag must be registered")
	assert.Equal(t, "false", listFlag.DefValue)
}
