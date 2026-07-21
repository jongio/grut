package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolInventoryIncludesReadWriteAndFileTools(t *testing.T) {
	tools := ToolInventory()
	require.NotEmpty(t, tools)

	byName := make(map[string]ToolInfo, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	assert.Equal(t, categoryRead, byName["git_status"].Category)
	assert.Equal(t, categoryWrite, byName["git_stage"].Category)
	assert.Equal(t, categoryRead, byName["file_read"].Category)
	assert.Equal(t, categoryWrite, byName["file_write"].Category)
	assert.NotEmpty(t, byName["git_status"].Description)
}

func TestServerToolsReturnsCopy(t *testing.T) {
	tools := ToolInventory()
	require.NotEmpty(t, tools)

	tools[0].Name = "changed"
	again := ToolInventory()

	assert.NotEqual(t, "changed", again[0].Name)
}
