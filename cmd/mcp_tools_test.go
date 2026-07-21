package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	grut_mcp "github.com/jongio/grut/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPToolsCommandListsTools(t *testing.T) {
	cmd := newMCPCmd()
	cmd.SetArgs([]string{"tools"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "NAME")
	assert.Contains(t, out.String(), "git_status")
	assert.Contains(t, out.String(), "file_read")
}

func TestMCPToolsCommandJSON(t *testing.T) {
	cmd := newMCPCmd()
	cmd.SetArgs([]string{"tools", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	var tools []grut_mcp.ToolInfo
	require.NoError(t, json.Unmarshal(out.Bytes(), &tools))
	assert.NotEmpty(t, tools)
	assert.Equal(t, "git_status", tools[0].Name)
}
