package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigCheckSuccess(t *testing.T) {
	cmd := newConfigCheckCmd(
		func() (*config.Config, error) { return &config.Config{}, nil },
		func() string { return `C:\Users\me\AppData\Roaming\grut\config.toml` },
	)
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), `C:\Users\me\AppData\Roaming\grut\config.toml`)
	assert.Contains(t, out.String(), "OK")
}

func TestConfigCheckFailure(t *testing.T) {
	cmd := newConfigCheckCmd(
		func() (*config.Config, error) { return nil, errors.New("config preview.width: must be 1-100") },
		func() string { return `C:\Users\me\AppData\Roaming\grut\config.toml` },
	)
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config check failed")
	assert.Contains(t, err.Error(), "preview.width")
	assert.Contains(t, out.String(), `C:\Users\me\AppData\Roaming\grut\config.toml`)
}

func TestRootRegistersConfigCheck(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	configCmd, _, err := root.Find([]string{"config"})
	require.NoError(t, err)
	require.NotNil(t, configCmd)
	assert.Equal(t, "config", configCmd.Name())

	checkCmd, _, err := root.Find([]string{"config", "check"})
	require.NoError(t, err)
	require.NotNil(t, checkCmd)
	assert.Equal(t, "check", checkCmd.Name())
}

func TestConfigDefaultsPrintsEmbeddedTOML(t *testing.T) {
	cmd := newConfigDefaultsCmd(func() []byte {
		return []byte("[general]\ndefault_layout = \"git\"\n")
	})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "[general]\ndefault_layout = \"git\"\n", out.String())
}

func TestConfigDefaultsWritesOutputFile(t *testing.T) {
	cmd := newConfigDefaultsCmd(func() []byte {
		return []byte("[theme]\nname = \"default\"\n")
	})
	outPath := filepath.Join(t.TempDir(), "nested", "config.toml")
	cmd.SetArgs([]string{"--output", outPath})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "[theme]\nname = \"default\"\n", string(data))
	assert.Contains(t, out.String(), "Wrote default config")
}

func TestRootRegistersConfigDefaults(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	defaultsCmd, _, err := root.Find([]string{"config", "defaults"})
	require.NoError(t, err)
	require.NotNil(t, defaultsCmd)
	assert.Equal(t, "defaults", defaultsCmd.Name())
}
