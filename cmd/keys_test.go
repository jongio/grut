package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
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

func TestKeysCommandPrintsSectionIndex(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--sections"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.NotEmpty(t, lines)
	assert.Contains(t, lines[0], "global")
	assert.Contains(t, lines[0], "Global")
	assert.NotContains(t, out.String(), "ctrl+c")
}

func TestKeysCommandPrintsSectionIndexWithFilter(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--sections", "--filter", "workflow"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "github")
	assert.Contains(t, out.String(), "GitHub")
	assert.NotContains(t, out.String(), "File Tree")
	assert.NotContains(t, out.String(), "Dispatch workflow")
}

func TestKeysCommandPrintsSectionIndexJSON(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--sections", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var sections []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &sections))
	require.NotEmpty(t, sections)
	assert.Equal(t, "global", sections[0]["id"])
	assert.Equal(t, "Global", sections[0]["title"])
	assert.NotContains(t, sections[0], "bindings")
	assert.NotContains(t, sections[0], "key")
	assert.NotContains(t, sections[0], "action")
}

func TestKeysCommandRejectsSectionAndSectionsTogether(t *testing.T) {
	cmd := newKeysCmd()
	cmd.SetArgs([]string{"--section", "global", "--sections"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
	assert.NotContains(t, out.String(), "ctrl+c")
}

func TestRootRegistersKeysCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	keysCmd, _, err := root.Find([]string{"keys"})
	require.NoError(t, err)
	require.NotNil(t, keysCmd)
	assert.Equal(t, "keys", keysCmd.Name())
}
