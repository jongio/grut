package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jongio/grut/internal/theme"

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

func TestThemeListCommandFiltersNames(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"catppuccin", "custom", "Catwalk"} })
	cmd.SetArgs([]string{"--filter", "cat"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "catppuccin\nCatwalk\n", out.String())
}

func TestThemeListCommandFiltersJSON(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"default", "Catppuccin", "gruvbox"} })
	cmd.SetArgs([]string{"--filter", "CAT", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var names []string
	require.NoError(t, json.Unmarshal(out.Bytes(), &names))
	assert.Equal(t, []string{"Catppuccin"}, names)
}

func TestThemeListCommandEmptyFilterPreservesNames(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"default", "gruvbox"} })
	cmd.SetArgs([]string{"--filter", ""})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "default\ngruvbox\n", out.String())
}

func TestThemeListCommandFilterNoMatchesText(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"default", "gruvbox"} })
	cmd.SetArgs([]string{"--filter", "cat"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Empty(t, out.String())
}

func TestThemeListCommandFilterNoMatchesJSON(t *testing.T) {
	cmd := newThemeListCmd(func() []string { return []string{"default", "gruvbox"} })
	cmd.SetArgs([]string{"--filter", "cat", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "[]\n", out.String())
}

func TestThemeShowCommandPrintsText(t *testing.T) {
	cmd := newThemeShowCmd(func(string) (*theme.Theme, error) {
		return &theme.Theme{Name: "default", Variant: "dark", Mode: theme.ModeColor, Colors: theme.Colors{Background: "#000000"}}, nil
	})
	cmd.SetArgs([]string{"default"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Name:    default")
	assert.Contains(t, out.String(), "Variant: dark")
	assert.Contains(t, out.String(), "Mode:    color")
}

func TestThemeShowCommandPrintsJSON(t *testing.T) {
	cmd := newThemeShowCmd(func(string) (*theme.Theme, error) {
		return &theme.Theme{Name: "gruvbox", Variant: "dark", Mode: theme.ModeColor, Colors: theme.Colors{Background: "#282828"}}, nil
	})
	cmd.SetArgs([]string{"gruvbox", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var report themeShowReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, "gruvbox", report.Name)
	assert.Equal(t, "dark", report.Variant)
	assert.Equal(t, "color", report.Mode)
	assert.Equal(t, "#282828", report.Colors.Background)
}

func TestRootRegistersThemeListCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	themeCmd, _, err := root.Find([]string{"theme", "list"})
	require.NoError(t, err)
	require.NotNil(t, themeCmd)
	assert.Equal(t, cmdList, themeCmd.Name())
}
