package cmd

import (
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// newVersionCmd — execution
// ---------------------------------------------------------------------------

func TestVersionCmd_OutputContainsAppVersion(t *testing.T) {
	// The version command uses fmt.Println which writes to os.Stdout, not
	// cmd.OutOrStdout(). We verify it runs without error and check the
	// version string is set correctly via the root command's Version field.
	root, cleanup := buildRootCommand()
	defer cleanup()
	root.SetArgs([]string{"version"})

	err := root.Execute()
	require.NoError(t, err)
	// The root command's Version field is what --version and the version
	// subcommand use. Verify it's set correctly.
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

func TestVersionCmd_HasRunNotRunE(t *testing.T) {
	// version uses Run (not RunE) because it always succeeds.
	cmd := newVersionCmd()
	assert.NotNil(t, cmd.Run, "version command should have Run set")
	assert.Nil(t, cmd.RunE, "version command should not have RunE set")
}
