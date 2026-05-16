package runtime

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jongio/grut/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHostAPI records calls made by extension scripts.
type mockHostAPI struct {
	mu       sync.Mutex
	toasts   []toastCall
	commands map[string]commandEntry
	statuses map[string]string
	logs     []string
}

type toastCall struct {
	Title   string
	Message string
	Level   string
}

type commandEntry struct {
	Desc    string
	Handler func() error
}

func newMockHostAPI() *mockHostAPI {
	return &mockHostAPI{
		commands: make(map[string]commandEntry),
		statuses: make(map[string]string),
	}
}

func (m *mockHostAPI) ShowToast(title, message, level string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toasts = append(m.toasts, toastCall{Title: title, Message: message, Level: level})
}

func (m *mockHostAPI) RegisterCommand(name, description string, handler func() error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[name] = commandEntry{Desc: description, Handler: handler}
}

func (m *mockHostAPI) SetStatusBarItem(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[key] = value
}

func (m *mockHostAPI) Log(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, msg)
}

// testManifest returns a minimal valid manifest for test use.
// Includes "notify" so that grut.toast calls succeed in existing tests.
func testManifest() *extension.Manifest {
	return &extension.Manifest{
		Name:        "test-ext",
		Version:     "1.0.0",
		Runtime:     "lua",
		Permissions: []string{"notify"},
	}
}

// writeLua writes a Lua script file into a temp dir and returns the path.
func writeLua(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// --- Tests ---

func TestLuaRuntime_LoadSimpleScript(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		-- simple script that sets a global
		x = 1 + 1
	`)
	require.NoError(t, rt.Load(script))
}

func TestLuaRuntime_Toast(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.toast("Hello", "World", "warn")
	`)
	require.NoError(t, rt.Load(script))

	require.Len(t, api.toasts, 1)
	assert.Equal(t, "Hello", api.toasts[0].Title)
	assert.Equal(t, "World", api.toasts[0].Message)
	assert.Equal(t, "warn", api.toasts[0].Level)
}

func TestLuaRuntime_ToastDefaultLevel(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.toast("T", "M")
	`)
	require.NoError(t, rt.Load(script))

	require.Len(t, api.toasts, 1)
	assert.Equal(t, "info", api.toasts[0].Level)
}

func TestLuaRuntime_RegisterCommand(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.register_command("hello", "Say hello", function()
			grut.toast("cmd", "executed", "info")
		end)
	`)
	require.NoError(t, rt.Load(script))

	entry, ok := api.commands["hello"]
	require.True(t, ok, "command 'hello' should be registered")
	assert.Equal(t, "Say hello", entry.Desc)

	// Invoke the handler and verify the callback fires.
	require.NoError(t, entry.Handler())
	require.Len(t, api.toasts, 1)
	assert.Equal(t, "cmd", api.toasts[0].Title)
}

func TestLuaRuntime_SetStatus(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.set_status("branch", "main")
	`)
	require.NoError(t, rt.Load(script))

	assert.Equal(t, "main", api.statuses["branch"])
}

func TestLuaRuntime_SandboxOsBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			local v = os.execute("echo bad")
		end)
		if not ok then error("os is blocked: " .. err) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "os is blocked")
}

func TestLuaRuntime_SandboxIoBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			local f = io.open("/etc/passwd", "r")
		end)
		if not ok then error("io is blocked: " .. err) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "io is blocked")
}

func TestLuaRuntime_SandboxRequireOsBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			require("os")
		end)
		if not ok then error("require os blocked: " .. err) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require os blocked")
}

func TestLuaRuntime_SandboxLoadfileBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			loadfile("some_file.lua")()
		end)
		if not ok then error("loadfile blocked: " .. err) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loadfile blocked")
}

func TestLuaRuntime_TimeoutInfiniteLoop(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// Use a short timeout so the test doesn't hang.
	rt.SetTimeout(50 * time.Millisecond)

	script := writeLua(t, t.TempDir(), "main.lua", `
		while true do end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestLuaRuntime_InvalidScript(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		this is not valid lua!!!
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile")
}

func TestLuaRuntime_EntryPointNotFound(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.Load(filepath.Join(t.TempDir(), "nonexistent.lua"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read entry point")
}

// ---------------------------------------------------------------------------
// Security: ValidateEntryPoint — path traversal and injection prevention
// ---------------------------------------------------------------------------

func TestValidateEntryPoint_RejectsPathTraversal(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"unix-traversal", "/ext/../../../etc/passwd"},
		{"windows-traversal", `C:\ext\..\..\..\Windows\System32\config\SAM`},
		{"mid-traversal", "extensions/../secret.lua"},
		{"bare-dotdot", ".."},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEntryPoint(tt.path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "path traversal")
		})
	}
}

func TestValidateEntryPoint_RejectsNullByte(t *testing.T) {
	err := ValidateEntryPoint("init\x00.lua")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null byte")
}

func TestValidateEntryPoint_RejectsEmpty(t *testing.T) {
	err := ValidateEntryPoint("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidateEntryPoint_RejectsDashPrefix(t *testing.T) {
	err := ValidateEntryPoint("/extensions/-malicious.lua")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not start with '-'")
}

func TestValidateEntryPoint_AcceptsValidPaths(t *testing.T) {
	valid := []string{
		"/home/user/.grut/extensions/my-ext/init.lua",
		`C:\Users\dev\.grut\extensions\my-ext\init.lua`,
		"extensions/hello/main.lua",
	}
	for _, p := range valid {
		t.Run(p, func(t *testing.T) {
			assert.NoError(t, ValidateEntryPoint(p))
		})
	}
}

func TestLuaRuntime_LoadRejectsTraversal(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.Load("/ext/../../../etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestLuaRuntime_Close(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)

	// Load a trivial script to ensure state is active.
	script := writeLua(t, t.TempDir(), "main.lua", `x = 1`)
	require.NoError(t, rt.Load(script))

	// Close should not panic.
	rt.Close()
}

func TestLuaRuntime_Name(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	assert.Equal(t, "lua", rt.Name())
}

func TestNewLuaRuntime_NilManifest(t *testing.T) {
	_, err := NewLuaRuntime(nil, newMockHostAPI())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest must not be nil")
}

func TestNewLuaRuntime_NilHostAPI(t *testing.T) {
	_, err := NewLuaRuntime(testManifest(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host API must not be nil")
}

func TestLuaRuntime_SafeModulesAvailable(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// string, table, math, coroutine should all be accessible.
	script := writeLua(t, t.TempDir(), "main.lua", `
		-- string
		local s = string.upper("hello")
		assert(s == "HELLO", "string module should work")

		-- table
		local t = {3, 1, 2}
		table.sort(t)
		assert(t[1] == 1, "table module should work")

		-- math
		assert(math.abs(-5) == 5, "math module should work")
	`)
	require.NoError(t, rt.Load(script))
}

func TestLuaRuntime_MultipleToasts(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		grut.toast("A", "1", "info")
		grut.toast("B", "2", "error")
	`)
	require.NoError(t, rt.Load(script))

	require.Len(t, api.toasts, 2)
	assert.Equal(t, "A", api.toasts[0].Title)
	assert.Equal(t, "B", api.toasts[1].Title)
}

func TestLuaRuntime_RuntimeError(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		error("boom")
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// ---------------------------------------------------------------------------
// Sandbox completeness tests — verifies every entry in dangerousGlobals and
// dangerousModules is actually inaccessible from Lua scripts.
// ---------------------------------------------------------------------------

// sandboxBlocked is a test helper that loads a Lua script which probes the
// named identifier and reports whether it is blocked.
// The pattern mirrors the existing sandbox tests: pcall the dangerous call,
// raise an error if the sandbox blocked it, and expect rt.Load to return an
// error that contains "blocked".
func sandboxBlocked(t *testing.T, identifier, callExpr string) {
	t.Helper()
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// Build a Lua script:
	//   local ok, err = pcall(function() <callExpr> end)
	//   if not ok then error("<identifier> is blocked: " .. tostring(err)) end
	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			`+callExpr+`
		end)
		if not ok then
			error("`+identifier+` is blocked: " .. tostring(err))
		end
	`)
	err = rt.Load(script)
	require.Error(t, err, "%s must be blocked in the Lua sandbox", identifier)
	assert.Contains(t, err.Error(), identifier+" is blocked",
		"%s must produce a 'blocked' error message", identifier)
}

// TestLuaRuntime_SandboxDebugBlocked verifies that the `debug` module is
// inaccessible. `debug.sethook`, `debug.getinfo`, and related functions can
// be used to inspect and manipulate the VM internals.
func TestLuaRuntime_SandboxDebugBlocked(t *testing.T) {
	sandboxBlocked(t, "debug", `local _ = debug.traceback()`)
}

// TestLuaRuntime_SandboxSetmetatableBlocked verifies that `setmetatable()` is
// blocked. Metatables can be used to override `__index` / `__newindex` on
// tables and escape sandboxing restrictions (CWE-265).
func TestLuaRuntime_SandboxSetmetatableBlocked(t *testing.T) {
	sandboxBlocked(t, "setmetatable", `setmetatable({}, {__index = function() end})`)
}

// TestLuaRuntime_SandboxGetmetatableBlocked verifies that `getmetatable()` is
// blocked. Even read-only metatable access can expose internal VM state.
func TestLuaRuntime_SandboxGetmetatableBlocked(t *testing.T) {
	sandboxBlocked(t, "getmetatable", `getmetatable({})`)
}

// TestLuaRuntime_SandboxRawsetBlocked verifies that `rawset()` is blocked.
// `rawset` bypasses `__newindex` metamethods and can be used to mutate
// protected tables such as `_G`.
func TestLuaRuntime_SandboxRawsetBlocked(t *testing.T) {
	sandboxBlocked(t, "rawset", `rawset({}, "k", "v")`)
}

// TestLuaRuntime_SandboxRawgetBlocked verifies that `rawget()` is blocked.
// `rawget` bypasses `__index` metamethods, leaking protected values.
func TestLuaRuntime_SandboxRawgetBlocked(t *testing.T) {
	sandboxBlocked(t, "rawget", `rawget({}, "k")`)
}

// TestLuaRuntime_SandboxRequireDebugBlocked verifies that require("debug")
// cannot be used to obtain the debug module after it has been removed from
// both the global scope and the package.loaded / preload tables.
func TestLuaRuntime_SandboxRequireDebugBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			require("debug")
		end)
		if not ok then error("require debug blocked: " .. tostring(err)) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require debug blocked")
}

// TestLuaRuntime_SandboxRequireIoBlocked complements the existing require("os")
// test for the io module.
func TestLuaRuntime_SandboxRequireIoBlocked(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local ok, err = pcall(function()
			require("io")
		end)
		if not ok then error("require io blocked: " .. tostring(err)) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require io blocked")
}

// ---------------------------------------------------------------------------
// Issue #73 — require() and package.path must be neutralised
// ---------------------------------------------------------------------------

// TestLuaRuntime_SandboxRequireGlobalBlocked verifies that the `require`
// global itself is nil in the sandbox, not just that individual module names
// are blocked.  This is the primary fix for issue #73.
func TestLuaRuntime_SandboxRequireGlobalBlocked(t *testing.T) {
	sandboxBlocked(t, "require", `require("string")`)
}

// TestLuaRuntime_SandboxPackagePathEmpty verifies that package.path and
// package.cpath are empty strings so the filesystem searcher finds nothing.
func TestLuaRuntime_SandboxPackagePathEmpty(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		assert(package.path == "", "package.path must be empty, got: " .. tostring(package.path))
		assert(package.cpath == "", "package.cpath must be empty, got: " .. tostring(package.cpath))
	`)
	require.NoError(t, rt.Load(script))
}

// TestLuaRuntime_SandboxPackageLoadedEmpty verifies that package.loaded has
// been wiped completely — no module (including safe ones) is pre-cached.
func TestLuaRuntime_SandboxPackageLoadedEmpty(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	script := writeLua(t, t.TempDir(), "main.lua", `
		local count = 0
		for _ in pairs(package.loaded) do count = count + 1 end
		assert(count == 0, "package.loaded must be empty, had " .. count .. " entries")
	`)
	require.NoError(t, rt.Load(script))
}

// TestLuaRuntime_SandboxRequireCannotLoadFromDisk creates a .lua file on disk
// and verifies that even if package.path were somehow restored, require()
// itself is nil and cannot be called.  This is the end-to-end exploit test
// for issue #73.
func TestLuaRuntime_SandboxRequireCannotLoadFromDisk(t *testing.T) {
	dir := t.TempDir()

	// Write a "malicious" module to disk.
	writeLua(t, dir, "evil.lua", `return "pwned"`)

	api := newMockHostAPI()
	rt, err := NewLuaRuntime(testManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// The script tries to restore package.path and call require.
	script := writeLua(t, dir, "main.lua", `
		local ok, err = pcall(function()
			package.path = "`+filepath.ToSlash(dir)+`/?.lua"
			require("evil")
		end)
		if not ok then error("require disk blocked: " .. tostring(err)) end
	`)
	err = rt.Load(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require disk blocked")
}
