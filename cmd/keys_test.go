package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jongio/grut/internal/keybindings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeysCommandPrintsText(t *testing.T) {
	cmd := newKeysCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Global")
	assert.Contains(t, out.String(), "ctrl+c")
}

func TestKeysCommandFiltersBindings(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--filter", "workflow"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Dispatch workflow")
	assert.NotContains(t, out.String(), "File Tree\n")
}

func TestKeysCommandFiltersBySection(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--section", "filetree"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "File Tree")
	assert.NotContains(t, out.String(), "Global\n")
}

func TestKeysCommandFiltersBySectionJSON(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--section", "global", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var sections []keybindings.Section
	require.NoError(t, json.Unmarshal(out.Bytes(), &sections))
	require.Len(t, sections, 1)
	assert.Equal(t, "global", sections[0].ID)
}

func TestKeysCommandUnknownSectionReturnsEmptyJSON(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--section", "missing", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var sections []keybindings.Section
	require.NoError(t, json.Unmarshal(out.Bytes(), &sections))
	assert.Empty(t, sections)
}

func TestKeysCommandPrintsJSON(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--filter", "global", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var sections []keybindings.Section
	require.NoError(t, json.Unmarshal(out.Bytes(), &sections))
	require.Len(t, sections, 1)
	assert.Equal(t, "Global", sections[0].Title)
}

func TestRootRegistersKeysCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	keysCmd, _, err := root.Find([]string{"keys"})
	require.NoError(t, err)
	require.NotNil(t, keysCmd)
	assert.Equal(t, "keys", keysCmd.Name())
}
