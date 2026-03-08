package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedToolCount is the total number of tools registered by
// NewToolRegistry. Update this constant when adding or removing tools.
const expectedToolCount = 47

func TestNewToolRegistry_ToolCount(t *testing.T) {
	r := NewToolRegistry()
	defs := r.Definitions()
	assert.Len(t, defs, expectedToolCount,
		"expected %d tools, got %d — update expectedToolCount if tools were added/removed",
		expectedToolCount, len(defs))
}

func TestNewToolRegistry_AllToolsHaveValidSchema(t *testing.T) {
	r := NewToolRegistry()
	for _, def := range r.Definitions() {
		t.Run(def.Name, func(t *testing.T) {
			require.NotEmpty(t, def.Name, "tool name must not be empty")
			require.NotEmpty(t, def.Description, "tool %q must have a description", def.Name)
			require.NotNil(t, def.Parameters, "tool %q must have parameters", def.Name)
			typ, ok := def.Parameters["type"]
			require.True(t, ok, "tool %q parameters must have a 'type' key", def.Name)
			assert.Equal(t, "object", typ, "tool %q parameters type must be 'object'", def.Name)
			_, ok = def.Parameters["properties"]
			assert.True(t, ok, "tool %q parameters must have a 'properties' key", def.Name)
		})
	}
}

func TestGet_KnownTool(t *testing.T) {
	r := NewToolRegistry()

	info, ok := r.Get("file_read")
	require.True(t, ok, "file_read should be registered")
	assert.Equal(t, "file_read", info.Definition.Name)
	assert.Equal(t, Safe, info.Safety)
}

func TestGet_UnknownTool(t *testing.T) {
	r := NewToolRegistry()

	_, ok := r.Get("nonexistent_tool")
	assert.False(t, ok, "nonexistent tool should not be found")
}

func TestIsSafe_SafeTool(t *testing.T) {
	r := NewToolRegistry()
	assert.True(t, r.IsSafe("file_read"))
	assert.True(t, r.IsSafe("git_status"))
	assert.True(t, r.IsSafe("git_commit"))
}

func TestIsSafe_DestructiveTool(t *testing.T) {
	r := NewToolRegistry()
	assert.False(t, r.IsSafe("file_write"))
	assert.False(t, r.IsSafe("file_delete"))
	assert.False(t, r.IsSafe("git_push"))
	assert.False(t, r.IsSafe("git_reset"))
}

func TestIsSafe_UnknownTool(t *testing.T) {
	r := NewToolRegistry()
	assert.False(t, r.IsSafe("unknown"), "unknown tools should not be considered safe")
}

func TestDefinitions_AllPresent(t *testing.T) {
	r := NewToolRegistry()
	defs := r.Definitions()

	names := make(map[string]bool, len(defs))
	for _, d := range defs {
		names[d.Name] = true
	}

	// Spot-check a representative set from each category.
	expected := []string{
		// File operations
		"file_read", "file_write", "file_delete", "file_rename", "file_list", "file_mkdir",
		// Git read
		"git_status", "git_diff", "git_log", "git_blame",
		"git_branch_list", "git_stash_list", "git_worktree_list",
		// Git write
		"git_stage", "git_unstage", "git_commit", "git_push", "git_pull",
		"git_fetch", "git_checkout", "git_branch_create", "git_branch_delete",
		"git_merge", "git_rebase", "git_stash_push", "git_stash_pop",
		"git_reset", "git_tag_create", "git_tag_delete", "git_discard",
		// Nav & search
		"navigate_to", "search_files", "search_content", "explain",
		// Bulk
		"bulk_stage", "bulk_delete", "bulk_rename",
	}

	for _, name := range expected {
		assert.True(t, names[name], "expected tool %q to be registered", name)
	}
}

// TestSafety_FileTools verifies the safety classification of file tools.
func TestSafety_FileTools(t *testing.T) {
	r := NewToolRegistry()

	safeCases := []string{"file_read", "file_list", "file_mkdir"}
	for _, name := range safeCases {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Safe, info.Safety, "%s should be Safe", name)
	}

	destructiveCases := []string{"file_write", "file_delete", "file_rename"}
	for _, name := range destructiveCases {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Destructive, info.Safety, "%s should be Destructive", name)
	}
}

// TestSafety_GitReadTools verifies that all git read tools are Safe.
func TestSafety_GitReadTools(t *testing.T) {
	r := NewToolRegistry()

	readTools := []string{
		"git_status", "git_diff", "git_log", "git_blame",
		"git_branch_list", "git_stash_list", "git_worktree_list",
	}
	for _, name := range readTools {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Safe, info.Safety, "%s should be Safe", name)
	}
}

// TestSafety_GitWriteTools verifies the safety classification of git write tools.
func TestSafety_GitWriteTools(t *testing.T) {
	r := NewToolRegistry()

	safeWrite := []string{
		"git_stage", "git_unstage", "git_commit", "git_pull", "git_fetch",
		"git_checkout", "git_branch_create", "git_merge",
		"git_stash_push", "git_stash_pop", "git_tag_create",
	}
	for _, name := range safeWrite {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Safe, info.Safety, "%s should be Safe", name)
	}

	destructiveWrite := []string{
		"git_push", "git_branch_delete", "git_rebase",
		"git_reset", "git_tag_delete", "git_discard",
	}
	for _, name := range destructiveWrite {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Destructive, info.Safety, "%s should be Destructive", name)
	}
}

// TestSafety_NavSearchTools verifies that all navigation/search tools are Safe.
func TestSafety_NavSearchTools(t *testing.T) {
	r := NewToolRegistry()

	navTools := []string{"navigate_to", "search_files", "search_content", "explain"}
	for _, name := range navTools {
		info, ok := r.Get(name)
		require.True(t, ok, "%s should be registered", name)
		assert.Equal(t, Safe, info.Safety, "%s should be Safe", name)
	}
}

// TestSafety_BulkTools verifies the safety classification of bulk tools.
func TestSafety_BulkTools(t *testing.T) {
	r := NewToolRegistry()

	info, ok := r.Get("bulk_stage")
	require.True(t, ok)
	assert.Equal(t, Safe, info.Safety, "bulk_stage should be Safe")

	info, ok = r.Get("bulk_delete")
	require.True(t, ok)
	assert.Equal(t, Destructive, info.Safety, "bulk_delete should be Destructive")

	info, ok = r.Get("bulk_rename")
	require.True(t, ok)
	assert.Equal(t, Destructive, info.Safety, "bulk_rename should be Destructive")
}

// TestSchema_RequiredFields verifies that tools with required parameters
// declare them correctly in the JSON Schema.
func TestSchema_RequiredFields(t *testing.T) {
	r := NewToolRegistry()

	tests := []struct {
		tool     string
		required []string
	}{
		{"file_read", []string{"path"}},
		{"file_write", []string{"path", "content"}},
		{"file_rename", []string{"old_path", "new_path"}},
		{"git_blame", []string{"path"}},
		{"git_checkout", []string{"ref"}},
		{"git_commit", []string{"message"}},
		{"git_reset", []string{"ref"}},
		{"bulk_rename", []string{"renames"}},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			info, ok := r.Get(tc.tool)
			require.True(t, ok, "%s should be registered", tc.tool)

			req, ok := info.Definition.Parameters["required"]
			require.True(t, ok, "%s should have 'required' in schema", tc.tool)

			reqSlice, ok := req.([]string)
			require.True(t, ok, "%s 'required' should be []string", tc.tool)
			assert.ElementsMatch(t, tc.required, reqSlice,
				"%s required fields mismatch", tc.tool)
		})
	}
}

// TestSchema_NoRequiredForOptionalOnly verifies that tools with only
// optional parameters omit the "required" key entirely.
func TestSchema_NoRequiredForOptionalOnly(t *testing.T) {
	r := NewToolRegistry()

	optionalOnly := []string{
		"git_status", "git_diff", "git_log", "git_branch_list",
		"git_stash_list", "git_worktree_list", "git_push",
		"git_pull", "git_fetch", "git_stash_push", "git_stash_pop",
	}

	for _, name := range optionalOnly {
		t.Run(name, func(t *testing.T) {
			info, ok := r.Get(name)
			require.True(t, ok, "%s should be registered", name)
			_, hasRequired := info.Definition.Parameters["required"]
			assert.False(t, hasRequired,
				"%s should not have 'required' when all parameters are optional", name)
		})
	}
}

// TestSchema_BulkRenameItemsShape verifies the nested object schema for
// the bulk_rename tool's renames parameter.
func TestSchema_BulkRenameItemsShape(t *testing.T) {
	r := NewToolRegistry()

	info, ok := r.Get("bulk_rename")
	require.True(t, ok)

	props, ok := info.Definition.Parameters["properties"].(map[string]any)
	require.True(t, ok)

	renames, ok := props["renames"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", renames["type"])

	items, ok := renames["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", items["type"])

	itemProps, ok := items["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, itemProps, "old")
	assert.Contains(t, itemProps, "new")

	itemReq, ok := items["required"].([]string)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"old", "new"}, itemReq)
}

// TestDefinitions_NoDuplicateNames verifies that no two tools share the
// same name.
func TestDefinitions_NoDuplicateNames(t *testing.T) {
	r := NewToolRegistry()
	defs := r.Definitions()

	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		assert.False(t, seen[d.Name], "duplicate tool name: %s", d.Name)
		seen[d.Name] = true
	}
}
