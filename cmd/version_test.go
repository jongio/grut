package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// newVersionCmd — execution
// ---------------------------------------------------------------------------

func TestVersionCmd_OutputContainsAppVersion(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), config.AppVersion)
	assert.Equal(t, config.AppVersion, root.Version)
}

func TestVersionCmd_ExecutesWithoutError(t *testing.T) {
	// Execute "grut version" via the root command — should always succeed.
	root, cleanup := buildRootCommand()
	defer cleanup()

	root.SetArgs([]string{"version"})
	err := root.Execute()
	assert.NoError(t, err, "version command must succeed")
}

func TestVersionCmd_StructureHasCorrectUse(t *testing.T) {
	cmd := newVersionCmd()
	assert.Equal(t, cmdVersion, cmd.Use, "Use should match cmdVersion constant")
	assert.Equal(t, "version", cmd.Use)
}

func TestVersionCmd_JSONOutput(t *testing.T) {
	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var got versionInfo
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, config.AppVersion, got.Version)
}

func TestVersionCmd_JSONFlagRegistered(t *testing.T) {
	cmd := newVersionCmd()
	flag := cmd.Flags().Lookup("json")
	require.NotNil(t, flag)
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestVersionCmd_HasRunNotRunE(t *testing.T) {
	// version uses Run (not RunE) because it always succeeds.
	cmd := newVersionCmd()
	assert.NotNil(t, cmd.Run, "version command should have Run set")
	assert.Nil(t, cmd.RunE, "version command should not have RunE set")
}
