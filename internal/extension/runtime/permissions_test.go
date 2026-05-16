package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MCP: permission enforcement
// ---------------------------------------------------------------------------

func TestMCPRuntime_LoadDeniedWithoutProcessPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-perms",
		Version:     "1.0.0",
		Runtime:     "mcp",
		Permissions: []string{}, // no "process" permission
	}
	rt, err := NewMCPRuntime(manifest)
	require.NoError(t, err)
	defer rt.Close()

	bin := buildBlockingHelper(t)
	err = rt.Load(bin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission")
	assert.Contains(t, err.Error(), "process")

	// Verify the error is the correct type.
	var permErr *extension.PermissionDeniedError
	assert.ErrorAs(t, err, &permErr)
	assert.Equal(t, extension.PermProcess, permErr.Permission)
	assert.Equal(t, "no-perms", permErr.Extension)
}

func TestMCPRuntime_LoadAllowedWithProcessPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "has-process",
		Version:     "1.0.0",
		Runtime:     "mcp",
		Permissions: []string{"process"},
	}
	rt, err := NewMCPRuntime(manifest)
	require.NoError(t, err)
	defer rt.Close()

	bin := buildBlockingHelper(t)
	require.NoError(t, rt.Load(bin))
	assert.True(t, rt.Running())
}

// ---------------------------------------------------------------------------
// Lua: permission enforcement
// ---------------------------------------------------------------------------

func TestLuaRuntime_ToastDeniedWithoutNotifyPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-notify",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{}, // no "notify" permission
	}
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(grut.toast, "Hi", "There", "info")
		if not ok then error("blocked: " .. tostring(err)) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "notify")

	// Host API must NOT have been called.
	assert.Empty(t, api.toasts)
}

func TestLuaRuntime_ToastAllowedWithNotifyPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "has-notify",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{"notify"},
	}
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.toast("Hello", "World", "info")
	`)
	require.NoError(t, rt.Load(script))
	require.Len(t, api.toasts, 1)
	assert.Equal(t, "Hello", api.toasts[0].Title)
}

// ---------------------------------------------------------------------------
// WASM: permission enforcement
// ---------------------------------------------------------------------------

func TestWASMRuntime_ToastDeniedWithoutNotifyPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-notify-wasm",
		Version:     "1.0.0",
		Runtime:     "wasm",
		Permissions: []string{}, // no "notify" permission
	}
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	require.NoError(t, rt.loadBytes(toastCallWASM))

	fn := rt.mod.ExportedFunction("test_toast")
	require.NotNil(t, fn)

	// Call succeeds (host import doesn't trap) but toast is silently denied.
	_, err = fn.Call(context.Background())
	require.NoError(t, err)

	// The host API must NOT have been called.
	api.mu.Lock()
	defer api.mu.Unlock()
	assert.Empty(t, api.toasts, "toast must be blocked without notify permission")
}

func TestWASMRuntime_ToastAllowedWithNotifyPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "has-notify-wasm",
		Version:     "1.0.0",
		Runtime:     "wasm",
		Permissions: []string{"notify"},
	}
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	require.NoError(t, rt.loadBytes(toastCallWASM))

	fn := rt.mod.ExportedFunction("test_toast")
	require.NotNil(t, fn)

	_, err = fn.Call(context.Background())
	require.NoError(t, err)

	api.mu.Lock()
	defer api.mu.Unlock()
	require.Len(t, api.toasts, 1)
	assert.Equal(t, "Hello", api.toasts[0].Title)
}

// ---------------------------------------------------------------------------
// permissions.go unit tests
// ---------------------------------------------------------------------------

func TestManifestHasPermission(t *testing.T) {
	m := &extension.Manifest{
		Name:        "perm-test",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{"notify", "network"},
	}

	assert.True(t, extension.ManifestHasPermission(m, extension.PermNotify))
	assert.True(t, extension.ManifestHasPermission(m, extension.PermNetwork))
	assert.False(t, extension.ManifestHasPermission(m, extension.PermProcess))
	assert.False(t, extension.ManifestHasPermission(m, extension.PermFileRead))
}

func TestErrPermissionDenied_Error(t *testing.T) {
	err := &extension.PermissionDeniedError{
		Extension:  "my-ext",
		Permission: extension.PermProcess,
		Operation:  "spawn subprocess",
	}
	assert.Equal(t, "extension my-ext: permission process is required for spawn subprocess", err.Error())
}

// ---------------------------------------------------------------------------
// MCP: permission check does not interfere with other error paths
// ---------------------------------------------------------------------------

func TestMCPRuntime_LoadInvalidEntryPointStillChecksPermission(t *testing.T) {
	// Even with an invalid path, the permission check fires first.
	manifest := &extension.Manifest{
		Name:        "no-perms",
		Version:     "1.0.0",
		Runtime:     "mcp",
		Permissions: []string{},
	}
	rt, err := NewMCPRuntime(manifest)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.Load("/nonexistent/path/to/binary")
	require.Error(t, err)
	// Permission error takes precedence over the file-not-found error.
	assert.Contains(t, err.Error(), "permission")
}

// ---------------------------------------------------------------------------
// WASM: log host function is unaffected (no permission required)
// ---------------------------------------------------------------------------

func TestWASMRuntime_LogAllowedWithoutPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-perms-wasm",
		Version:     "1.0.0",
		Runtime:     "wasm",
		Permissions: []string{}, // no permissions at all
	}
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	require.NoError(t, rt.loadBytes(logCallWASM))

	fn := rt.mod.ExportedFunction("test_log")
	require.NotNil(t, fn)

	_, err = fn.Call(context.Background())
	require.NoError(t, err)

	api.mu.Lock()
	defer api.mu.Unlock()
	require.Len(t, api.logs, 1)
	assert.Equal(t, "ping", api.logs[0])
}

// ---------------------------------------------------------------------------
// Lua: non-toast host functions remain accessible without notify
// ---------------------------------------------------------------------------

func TestLuaRuntime_SetStatusAllowedWithoutNotifyPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-notify",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{}, // no permissions
	}
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.set_status("branch", "main")
	`)
	require.NoError(t, rt.Load(script))
	assert.Equal(t, "main", api.statuses["branch"])
}

func TestLuaRuntime_RegisterCommandAllowedWithoutPermission(t *testing.T) {
	manifest := &extension.Manifest{
		Name:        "no-perms",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{}, // no permissions
	}
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(manifest, api)
	require.NoError(t, err)
	defer rt.Close()

	dir := t.TempDir()
	script := filepath.Join(dir, "main.lua")
	require.NoError(t, os.WriteFile(script, []byte(`
		grut.register_command("hello", "Say hello", function() end)
	`), 0o644))

	require.NoError(t, rt.Load(script))
	_, ok := api.commands["hello"]
	assert.True(t, ok, "register_command should work without special permissions")
}
