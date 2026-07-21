package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThemeListCommandPrintsNames(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"catppuccin", "custom"} })
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "catppuccin\ncustom\n", out.String())
}

func TestThemeListCommandPrintsJSON(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"default", "gruvbox"} })
	cmd.SetArgs([]string{"--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var names []string
	require.NoError(t, json.Unmarshal(out.Bytes(), &names))
	assert.Equal(t, []string{"default", "gruvbox"}, names)
}

func TestRootRegistersThemeListCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	themeCmd, _, err := root.Find([]string{"theme", "list"})
	require.NoError(t, err)
	require.NotNil(t, themeCmd)
	assert.Equal(t, cmdList, themeCmd.Name())
}
