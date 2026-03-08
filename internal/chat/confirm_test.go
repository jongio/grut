package chat

import (
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager() *ConfirmationManager {
	return NewConfirmationManager(NewToolRegistry())
}

func makeCall(name string, args map[string]any) ai.ToolCall {
	return ai.ToolCall{
		ID:        "call_" + name,
		Name:      name,
		Arguments: args,
	}
}

// ---------------------------------------------------------------------------
// Safe tools pass through
// ---------------------------------------------------------------------------

func TestCheck_SafeToolPassesThrough(t *testing.T) {
	m := newTestManager()

	safeTools := []string{"git_status", "file_read", "git_log", "file_list"}
	for _, name := range safeTools {
		t.Run(name, func(t *testing.T) {
			pc, ok := m.Check(makeCall(name, nil))
			assert.True(t, ok, "%s should be immediately executable", name)
			assert.Nil(t, pc, "%s should not create a pending confirmation", name)
			assert.False(t, m.HasPending(), "no pending confirmation after safe tool")
		})
	}
}

// ---------------------------------------------------------------------------
// Destructive tools create pending confirmation
// ---------------------------------------------------------------------------

func TestCheck_DestructiveToolCreatesPending(t *testing.T) {
	m := newTestManager()

	destructiveTools := []struct {
		name string
		args map[string]any
	}{
		{"file_delete", map[string]any{"path": "foo.txt"}},
		{"git_reset", map[string]any{"ref": "HEAD~1"}},
		{"file_write", map[string]any{"path": "bar.txt", "content": "data"}},
		{"git_branch_delete", map[string]any{"name": "old-branch"}},
		{"git_push", map[string]any{"force": true}},
	}

	for _, tc := range destructiveTools {
		t.Run(tc.name, func(t *testing.T) {
			m.Clear() // reset state between subtests
			pc, ok := m.Check(makeCall(tc.name, tc.args))
			assert.False(t, ok, "%s should require confirmation", tc.name)
			require.NotNil(t, pc, "%s should create a pending confirmation", tc.name)
			assert.Equal(t, tc.name, pc.Call.Name)
			assert.NotEmpty(t, pc.Description)
			assert.True(t, m.HasPending())
		})
	}
}

// ---------------------------------------------------------------------------
// Unknown tool returns false
// ---------------------------------------------------------------------------

func TestCheck_UnknownToolReturnsFalse(t *testing.T) {
	m := newTestManager()

	pc, ok := m.Check(makeCall("nonexistent_tool", nil))
	assert.False(t, ok, "unknown tool should return false")
	assert.Nil(t, pc, "unknown tool should not create pending confirmation")
	assert.False(t, m.HasPending())
}

// ---------------------------------------------------------------------------
// Accept flow
// ---------------------------------------------------------------------------

func TestAccept_ReturnsPendingCall(t *testing.T) {
	m := newTestManager()

	call := makeCall("file_delete", map[string]any{"path": "remove-me.txt"})
	pc, ok := m.Check(call)
	require.False(t, ok)
	require.NotNil(t, pc)

	result := m.Accept()
	require.NotNil(t, result, "Accept should return the tool call")
	assert.Equal(t, call.Name, result.Name)
	assert.Equal(t, call.ID, result.ID)
	assert.False(t, m.HasPending(), "pending should be cleared after Accept")
}

func TestAccept_NothingPending(t *testing.T) {
	m := newTestManager()
	result := m.Accept()
	assert.Nil(t, result, "Accept with nothing pending should return nil")
}

// ---------------------------------------------------------------------------
// Reject flow
// ---------------------------------------------------------------------------

func TestReject_ReturnsDescription(t *testing.T) {
	m := newTestManager()

	_, _ = m.Check(makeCall("file_delete", map[string]any{"path": "secret.txt"}))
	desc := m.Reject()

	assert.Contains(t, desc, "secret.txt")
	assert.False(t, m.HasPending(), "pending should be cleared after Reject")
}

func TestReject_NothingPending(t *testing.T) {
	m := newTestManager()
	desc := m.Reject()
	assert.Empty(t, desc, "Reject with nothing pending should return empty string")
}

// ---------------------------------------------------------------------------
// HasPending and Pending
// ---------------------------------------------------------------------------

func TestHasPending_Lifecycle(t *testing.T) {
	m := newTestManager()

	assert.False(t, m.HasPending(), "initial state: no pending")
	assert.Nil(t, m.Pending(), "initial state: Pending() returns nil")

	_, _ = m.Check(makeCall("git_reset", map[string]any{"ref": "HEAD~3"}))
	assert.True(t, m.HasPending(), "after Check: has pending")
	require.NotNil(t, m.Pending())
	assert.Equal(t, "git_reset", m.Pending().Call.Name)

	m.Accept()
	assert.False(t, m.HasPending(), "after Accept: no pending")
	assert.Nil(t, m.Pending())
}

// ---------------------------------------------------------------------------
// Clear
// ---------------------------------------------------------------------------

func TestClear_RemovesPending(t *testing.T) {
	m := newTestManager()

	_, _ = m.Check(makeCall("file_delete", map[string]any{"path": "gone.txt"}))
	require.True(t, m.HasPending())

	m.Clear()
	assert.False(t, m.HasPending(), "Clear should remove pending confirmation")
	assert.Nil(t, m.Pending())
}

func TestClear_NoopWhenEmpty(t *testing.T) {
	m := newTestManager()
	m.Clear() // should not panic
	assert.False(t, m.HasPending())
}

// ---------------------------------------------------------------------------
// Only one pending at a time — new Check replaces old
// ---------------------------------------------------------------------------

func TestCheck_ReplacesExistingPending(t *testing.T) {
	m := newTestManager()

	first, _ := m.Check(makeCall("file_delete", map[string]any{"path": "first.txt"}))
	require.NotNil(t, first)

	second, _ := m.Check(makeCall("git_reset", map[string]any{"ref": "HEAD"}))
	require.NotNil(t, second)

	assert.Equal(t, "git_reset", m.Pending().Call.Name,
		"second Check should replace the first pending confirmation")
}

// ---------------------------------------------------------------------------
// Description generation for each destructive tool
// ---------------------------------------------------------------------------

func TestDescription_FileDelete(t *testing.T) {
	call := makeCall("file_delete", map[string]any{"path": "src/old.go"})
	desc := describeToolCall(call)
	assert.Equal(t, `Delete "src/old.go"`, desc)
}

func TestDescription_FileWrite(t *testing.T) {
	call := makeCall("file_write", map[string]any{"path": "config.yaml", "content": "key: val"})
	desc := describeToolCall(call)
	assert.Equal(t, `Overwrite "config.yaml"`, desc)
}

func TestDescription_FileRename(t *testing.T) {
	call := makeCall("file_rename", map[string]any{"old_path": "a.txt", "new_path": "b.txt"})
	desc := describeToolCall(call)
	assert.Equal(t, `Rename "a.txt" → "b.txt"`, desc)
}

func TestDescription_GitBranchDelete(t *testing.T) {
	call := makeCall("git_branch_delete", map[string]any{"name": "feature/old"})
	desc := describeToolCall(call)
	assert.Equal(t, `Delete branch "feature/old"`, desc)
}

func TestDescription_GitRebase(t *testing.T) {
	call := makeCall("git_rebase", map[string]any{"onto": "main"})
	desc := describeToolCall(call)
	assert.Equal(t, `Rebase onto "main"`, desc)
}

func TestDescription_GitReset(t *testing.T) {
	call := makeCall("git_reset", map[string]any{"ref": "HEAD~2"})
	desc := describeToolCall(call)
	assert.Equal(t, `Reset to "HEAD~2"`, desc)
}

func TestDescription_GitResetHard(t *testing.T) {
	call := makeCall("git_reset", map[string]any{"ref": "HEAD~1", "hard": true})
	desc := describeToolCall(call)
	assert.Equal(t, `Reset to "HEAD~1" (hard)`, desc)
}

func TestDescription_GitPushForce(t *testing.T) {
	call := makeCall("git_push", map[string]any{"force": true})
	desc := describeToolCall(call)
	assert.Equal(t, `Force push to "origin"`, desc)
}

func TestDescription_GitPushForceCustomRemote(t *testing.T) {
	call := makeCall("git_push", map[string]any{"remote": "upstream", "force": true})
	desc := describeToolCall(call)
	assert.Equal(t, `Force push to "upstream"`, desc)
}

func TestDescription_GitPushNonForce(t *testing.T) {
	call := makeCall("git_push", map[string]any{"remote": "origin"})
	desc := describeToolCall(call)
	assert.Equal(t, `Push to "origin"`, desc)
}

func TestDescription_GitTagDelete(t *testing.T) {
	call := makeCall("git_tag_delete", map[string]any{"name": "v1.0.0"})
	desc := describeToolCall(call)
	assert.Equal(t, `Delete tag "v1.0.0"`, desc)
}

func TestDescription_GitDiscard(t *testing.T) {
	call := makeCall("git_discard", map[string]any{
		"paths": []any{"file1.go", "file2.go", "file3.go"},
	})
	desc := describeToolCall(call)
	assert.Equal(t, "Discard changes in 3 files", desc)
}

func TestDescription_BulkDelete(t *testing.T) {
	call := makeCall("bulk_delete", map[string]any{
		"paths": []any{"a.txt", "b.txt"},
	})
	desc := describeToolCall(call)
	assert.Equal(t, "Delete 2 files", desc)
}

func TestDescription_BulkRename(t *testing.T) {
	call := makeCall("bulk_rename", map[string]any{
		"renames": []any{
			map[string]any{"old": "a.txt", "new": "x.txt"},
			map[string]any{"old": "b.txt", "new": "y.txt"},
		},
	})
	desc := describeToolCall(call)
	assert.Equal(t, "Rename 2 files", desc)
}

func TestDescription_UnknownDestructive(t *testing.T) {
	call := makeCall("future_destructive_tool", map[string]any{})
	desc := describeToolCall(call)
	assert.Equal(t, "Execute future_destructive_tool", desc)
}

// ---------------------------------------------------------------------------
// FormatConfirmationPrompt
// ---------------------------------------------------------------------------

func TestFormatConfirmationPrompt_WithPending(t *testing.T) {
	pc := &PendingConfirmation{
		Call:        makeCall("file_delete", map[string]any{"path": "x.go"}),
		Description: `Delete "x.go"`,
	}
	prompt := FormatConfirmationPrompt(pc)
	assert.Contains(t, prompt, `Delete "x.go"`)
	assert.Contains(t, prompt, "[y/N]")
}

func TestFormatConfirmationPrompt_Nil(t *testing.T) {
	prompt := FormatConfirmationPrompt(nil)
	assert.Empty(t, prompt)
}

// ---------------------------------------------------------------------------
// argStr / argBool / argStrSlice edge cases
// ---------------------------------------------------------------------------

func TestArgStr_MissingKey(t *testing.T) {
	call := makeCall("test", map[string]any{})
	assert.Empty(t, argStr(call, "missing"))
}

func TestArgStr_WrongType(t *testing.T) {
	call := makeCall("test", map[string]any{"key": 42})
	assert.Empty(t, argStr(call, "key"))
}

func TestArgBool_MissingKey(t *testing.T) {
	call := makeCall("test", map[string]any{})
	assert.False(t, argBool(call, "missing"))
}

func TestArgBool_WrongType(t *testing.T) {
	call := makeCall("test", map[string]any{"key": "true"})
	assert.False(t, argBool(call, "key"))
}

func TestArgStrSlice_DirectStringSlice(t *testing.T) {
	call := makeCall("test", map[string]any{
		"paths": []string{"a.go", "b.go"},
	})
	result := argStrSlice(call, "paths")
	assert.Equal(t, []string{"a.go", "b.go"}, result)
}

func TestArgStrSlice_JSONDecoded(t *testing.T) {
	call := makeCall("test", map[string]any{
		"paths": []any{"a.go", "b.go"},
	})
	result := argStrSlice(call, "paths")
	assert.Equal(t, []string{"a.go", "b.go"}, result)
}

func TestArgStrSlice_MissingKey(t *testing.T) {
	call := makeCall("test", map[string]any{})
	assert.Nil(t, argStrSlice(call, "paths"))
}

func TestArgStrSlice_WrongType(t *testing.T) {
	call := makeCall("test", map[string]any{"paths": "not-a-slice"})
	assert.Nil(t, argStrSlice(call, "paths"))
}

func TestArgSlice_MissingKey(t *testing.T) {
	call := makeCall("test", map[string]any{})
	assert.Nil(t, argSlice(call, "items"))
}

func TestArgSlice_WrongType(t *testing.T) {
	call := makeCall("test", map[string]any{"items": "not-a-slice"})
	assert.Nil(t, argSlice(call, "items"))
}
