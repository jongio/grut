package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Extension subcommand error paths — argument validation
// ---------------------------------------------------------------------------

func TestExtInstallCmd_TooManyArgs(t *testing.T) {
	// install requires exactly 1 arg — more should fail.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"install", "arg1", "arg2"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtRemoveCmd_MissingArgs(t *testing.T) {
	// remove requires exactly 1 arg.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"remove"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtRemoveCmd_TooManyArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"remove", "a", "b"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtEnableCmd_MissingArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"enable"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtEnableCmd_TooManyArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"enable", "a", "b"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtDisableCmd_MissingArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"disable"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtDisableCmd_TooManyArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"disable", "a", "b"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInfoCmd_MissingArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"info"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInfoCmd_TooManyArgs(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"info", "a", "b"})
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Extension error wrapping — verify error prefixes
// ---------------------------------------------------------------------------

func TestExtRemoveCmd_ErrorContainsPrefix(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"remove", "nonexistent-ext-xyz"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ext remove:",
		"error should be wrapped with 'ext remove:' prefix")
}

func TestExtEnableCmd_ErrorContainsPrefix(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"enable", "nonexistent-ext-xyz"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ext enable:",
		"error should be wrapped with 'ext enable:' prefix")
}

func TestExtDisableCmd_ErrorContainsPrefix(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"disable", "nonexistent-ext-xyz"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ext disable:",
		"error should be wrapped with 'ext disable:' prefix")
}

func TestExtInfoCmd_ErrorContainsPrefix(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"info", "nonexistent-ext-xyz"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ext info:",
		"error should be wrapped with 'ext info:' prefix")
}

// ---------------------------------------------------------------------------
// ext create — scaffold with name
// ---------------------------------------------------------------------------

func TestExtCreateCmd_ScaffoldsProject(t *testing.T) {
	// ext create <name> should scaffold in cwd. Use a temp dir.
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create", "my-test-ext"})

	err = cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Created extension")
	assert.Contains(t, buf.String(), "my-test-ext")
}

func TestExtCreateCmd_ScaffoldsWithTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create", "--template", "lua", "templated-ext"})

	err = cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "templated-ext")
	assert.Contains(t, buf.String(), "lua")
}

func TestExtCreateCmd_InvalidTemplateReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create", "--template", "nonexistent-template", "bad-ext"})

	err = cmd.Execute()
	assert.Error(t, err, "invalid template should produce an error")
}

// ---------------------------------------------------------------------------
// ext list — structure
// ---------------------------------------------------------------------------

func TestExtListCmd_HasCorrectUse(t *testing.T) {
	cmd := newExtListCmd()
	assert.Equal(t, cmdList, cmd.Use, "Use should match cmdList constant")
}

func TestExtCmd_AllSubcommandsHaveShort(t *testing.T) {
	ext := newExtCmd()
	for _, sub := range ext.Commands() {
		assert.NotEmpty(t, sub.Short,
			"ext subcommand %q must have a Short description", sub.Name())
	}
}
