package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	grut_mcp "github.com/jongio/grut/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPToolsCommandFiltersTextOutput(t *testing.T) {
	cmd := newMCPToolsCmd()
	cmd.SetArgs([]string{"--filter", "read"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "file_read")
	assert.NotContains(t, out.String(), "file_write")
}

func TestMCPToolsCommandFiltersJSONOutput(t *testing.T) {
	cmd := newMCPToolsCmd()
	cmd.SetArgs([]string{"--filter", "write", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var tools []grut_mcp.ToolInfo
	require.NoError(t, json.Unmarshal(out.Bytes(), &tools))
	require.NotEmpty(t, tools)
	for _, tool := range tools {
		haystack := strings.ToLower(tool.Name + tool.Category + tool.Description)
		assert.Contains(t, haystack, "write")
	}
}

func TestMCPToolsCommandFilterNoMatches(t *testing.T) {
	cmd := newMCPToolsCmd()
	cmd.SetArgs([]string{"--filter", "no-such-tool", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "[]\n", out.String())
}

func TestRootRegistersMCPToolsCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	toolsCmd, _, err := root.Find([]string{"mcp", mcpToolsUse})
	require.NoError(t, err)
	require.NotNil(t, toolsCmd)
	assert.Equal(t, mcpToolsUse, toolsCmd.Name())
}
