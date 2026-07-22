package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	windowsConfigPath = `C:\Users\me\AppData\Roaming\grut\config.toml`
	windowsDataPath   = `C:\Users\me\AppData\Local\grut`
)

func TestConfigCheckSuccess(t *testing.T) {
	cmd := newConfigCheckCmd(
		func() (*config.Config, error) { return &config.Config{}, nil },
		func() string { return windowsConfigPath },
	)
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), windowsConfigPath)
	assert.Contains(t, out.String(), "OK")
}

func TestConfigCheckFailure(t *testing.T) {
	cmd := newConfigCheckCmd(
		func() (*config.Config, error) { return nil, errors.New("config preview.width: must be 1-100") },
		func() string { return windowsConfigPath },
	)
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config check failed")
	assert.Contains(t, err.Error(), "preview.width")
	assert.Contains(t, out.String(), windowsConfigPath)
}

func TestConfigCheckReportsKeybindingConflicts(t *testing.T) {
	cmd := newConfigCheckCmdWithKeymap(
		func() (*config.Config, error) {
			return &config.Config{General: config.GeneralConfig{KeybindingScheme: "custom"}}, nil
		},
		func() string { return windowsConfigPath },
		func(string) (*keymap.Keymap, error) {
			return keymap.NewKeymapFromBindings([]keymap.Binding{
				{Key: "x", Mode: keymap.ModePanel, Action: "one"},
				{Key: "x", Mode: keymap.ModePanel, Action: "two"},
			}), nil
		},
	)
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "keybinding conflict")
	assert.Contains(t, out.String(), "Keybinding conflicts")
	assert.Contains(t, out.String(), `key "x"`)
	assert.Contains(t, out.String(), "one")
	assert.Contains(t, out.String(), "two")
}

func TestConfigPathPrintsResolvedPaths(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		dataPath   string
		want       string
	}{
		{
			name:       "windows paths",
			configPath: windowsConfigPath,
			dataPath:   windowsDataPath,
			want:       "Config: " + windowsConfigPath + "\nData:   " + windowsDataPath + "\n",
		},
		{
			name:       "unix paths",
			configPath: "/home/me/.config/grut/config.toml",
			dataPath:   "/home/me/.local/share/grut",
			want:       "Config: /home/me/.config/grut/config.toml\nData:   /home/me/.local/share/grut\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newConfigPathCmd(
				func() string { return tt.configPath },
				func() string { return tt.dataPath },
			)
			var out bytes.Buffer
			cmd.SetOut(&out)

			err := cmd.Execute()

			require.NoError(t, err)
			assert.Equal(t, tt.want, out.String())
		})
	}
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

func TestRootRegistersConfigPath(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	pathCmd, _, err := root.Find([]string{"config", configPathCommandName})
	require.NoError(t, err)
	require.NotNil(t, pathCmd)
	assert.Equal(t, configPathCommandName, pathCmd.Name())
}

func TestRootRegistersConfigGet(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	getCmd, _, err := root.Find([]string{"config", "get"})
	require.NoError(t, err)
	require.NotNil(t, getCmd)
	assert.Equal(t, "get", getCmd.Name())
}

// getConfigWithValues returns a loader that yields a config carrying known
// values the get tests assert against.
func getConfigWithValues() configLoadFunc {
	return func() (*config.Config, error) {
		cfg := &config.Config{}
		cfg.Git.DefaultBranch = "trunk"
		cfg.Preview.Width = 42
		return cfg, nil
	}
}

func runConfigGet(t *testing.T, load configLoadFunc, key string) (string, error) {
	t.Helper()
	cmd := newConfigGetCmd(load)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{key})
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigGetScalar(t *testing.T) {
	out, err := runConfigGet(t, getConfigWithValues(), "git.default_branch")

	require.NoError(t, err)
	assert.Equal(t, "trunk\n", out)
}

func TestConfigGetNestedScalar(t *testing.T) {
	out, err := runConfigGet(t, getConfigWithValues(), "preview.width")

	require.NoError(t, err)
	assert.Equal(t, "42\n", out)
}

func TestConfigGetSection(t *testing.T) {
	out, err := runConfigGet(t, getConfigWithValues(), "preview")

	require.NoError(t, err)
	assert.Contains(t, out, "width = 42")
	assert.Contains(t, out, "position =")
}

func TestConfigGetUnknownTopLevelKey(t *testing.T) {
	out, err := runConfigGet(t, getConfigWithValues(), "nope")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key: nope")
	assert.Empty(t, out)
}

func TestConfigGetUnknownNestedKey(t *testing.T) {
	out, err := runConfigGet(t, getConfigWithValues(), "preview.nope")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key: preview.nope")
	assert.Empty(t, out)
}

func TestConfigGetDescendIntoScalar(t *testing.T) {
	// Treating a scalar leaf as a section must fail rather than panic.
	out, err := runConfigGet(t, getConfigWithValues(), "git.default_branch.extra")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key: git.default_branch.extra")
	assert.Empty(t, out)
}

func TestConfigGetLoadError(t *testing.T) {
	load := func() (*config.Config, error) { return nil, errors.New("boom") }
	out, err := runConfigGet(t, load, "git.default_branch")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
	assert.Contains(t, err.Error(), "boom")
	assert.Empty(t, out)
}

func TestWriteConfigValueSlice(t *testing.T) {
	var out bytes.Buffer
	err := writeConfigValue(&out, []any{"one", "two", "three"})

	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\nthree\n", out.String())
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
