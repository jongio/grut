package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jongio/grut/internal/extension"
	lua "github.com/yuin/gopher-lua"
)

// runtimeNameLua is the canonical identifier for the Lua runtime.
const runtimeNameLua = "lua"

// DefaultTimeout is the maximum time a Lua script may execute before being
// cancelled. The value mirrors the default from config (lua_timeout_ms = 100).
const DefaultTimeout = 100 * time.Millisecond

// dangerousModules lists the Lua standard-library modules that are removed
// before any user code executes. This prevents filesystem, process, and
// dynamic-loading access from extension scripts.
var dangerousModules = []string{"os", "io", "debug", "coroutine"}

// dangerousGlobals lists the global functions that are removed from the Lua
// state to prevent unchecked code loading and sandbox escape via bytecode
// compilation or metatable manipulation (CWE-265).
var dangerousGlobals = []string{
	"loadfile", "dofile", "load", "loadstring",
	"rawset", "rawget", "rawequal", "rawlen",
	"getmetatable", "setmetatable",
	"collectgarbage",
}

// LuaRuntime executes extension code inside a sandboxed gopher-lua VM.
type LuaRuntime struct {
	state    *lua.LState
	manifest *extension.Manifest
	hostAPI  HostAPI
	timeout  time.Duration
}

// Ensure LuaRuntime satisfies the Runtime interface at compile time.
var _ Runtime = (*LuaRuntime)(nil)

// NewLuaRuntime creates a sandboxed Lua VM wired to the provided host API.
// The VM has dangerous modules removed and grut host functions registered.
func NewLuaRuntime(manifest *extension.Manifest, api HostAPI) (*LuaRuntime, error) {
	if manifest == nil {
		return nil, fmt.Errorf("lua: manifest must not be nil")
	}
	if api == nil {
		return nil, fmt.Errorf("lua: host API must not be nil")
	}

	state := lua.NewState(lua.Options{
		SkipOpenLibs:  false,
		CallStackSize: 256,
	})

	rt := &LuaRuntime{
		state:    state,
		manifest: manifest,
		hostAPI:  api,
		timeout:  DefaultTimeout,
	}

	rt.sandbox()
	rt.registerHostAPI()

	return rt, nil
}

// SetTimeout overrides the default execution timeout. A zero or negative
// duration disables the timeout (useful for testing only).
func (r *LuaRuntime) SetTimeout(d time.Duration) {
	r.timeout = d
}

// ValidateEntryPoint checks that a Lua entry-point path does not escape
// its intended directory via path traversal or contain characters that could
// be used for injection (null bytes, shell metacharacters in the filename).
func ValidateEntryPoint(entryPoint string) error {
	if entryPoint == "" {
		return fmt.Errorf("lua: entry point must not be empty")
	}
	if strings.ContainsRune(entryPoint, 0) {
		return fmt.Errorf("lua: entry point contains null byte")
	}
	// Reject path traversal: normalise to forward slashes and check each
	// segment for a literal ".." component.
	normalized := strings.ReplaceAll(entryPoint, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return fmt.Errorf("lua: entry point must not contain path traversal (..)")
		}
	}
	// Reject entries whose base name starts with "-" to prevent option
	// injection if the path is ever passed to a subprocess.
	if strings.HasPrefix(filepath.Base(entryPoint), "-") {
		return fmt.Errorf("lua: entry point filename must not start with '-'")
	}
	return nil
}

// Load reads the Lua file at entryPoint and executes it inside the sandbox.
// Execution is subject to the configured timeout.
func (r *LuaRuntime) Load(entryPoint string) error {
	if err := ValidateEntryPoint(entryPoint); err != nil {
		return err
	}

	src, err := os.ReadFile(entryPoint)
	if err != nil {
		return fmt.Errorf("lua: read entry point: %w", err)
	}

	fn, err := r.state.LoadString(string(src))
	if err != nil {
		return fmt.Errorf("lua: compile: %w", err)
	}
	r.state.Push(fn)

	if r.timeout > 0 {
		return r.execWithTimeout()
	}
	if err := r.state.PCall(0, lua.MultRet, nil); err != nil {
		return fmt.Errorf("lua: exec: %w", err)
	}
	return nil
}

// Close shuts down the Lua VM and releases associated resources.
func (r *LuaRuntime) Close() {
	if r.state != nil {
		r.state.Close()
	}
}

// Name returns the runtime identifier.
func (r *LuaRuntime) Name() string {
	return runtimeNameLua
}

// sandbox removes dangerous modules and globals from the Lua state so that
// extension code cannot access the filesystem, spawn processes, or load
// arbitrary code.
func (r *LuaRuntime) sandbox() {
	for _, mod := range dangerousModules {
		r.state.SetGlobal(mod, lua.LNil)
	}
	for _, fn := range dangerousGlobals {
		r.state.SetGlobal(fn, lua.LNil)
	}

	// Neutralise the package system entirely so that require() cannot load
	// arbitrary .lua/.so files from disk via package.path / package.cpath
	// (see issue #73).  We clear every sub-table and path string, then
	// remove require itself from the global scope.
	pkg := r.state.GetField(r.state.Get(lua.EnvironIndex), "package")
	if pkgTbl, ok := pkg.(*lua.LTable); ok {
		// Wipe package.loaded — prevents returning cached copies of any module.
		if loaded, ok := pkgTbl.RawGetString("loaded").(*lua.LTable); ok {
			loaded.ForEach(func(k, _ lua.LValue) { loaded.RawSet(k, lua.LNil) })
		}
		// Wipe package.preload — prevents deferred loaders.
		if preload, ok := pkgTbl.RawGetString("preload").(*lua.LTable); ok {
			preload.ForEach(func(k, _ lua.LValue) { preload.RawSet(k, lua.LNil) })
		}
		// Clear search paths so the file-system searcher finds nothing.
		pkgTbl.RawSetString("path", lua.LString(""))
		pkgTbl.RawSetString("cpath", lua.LString(""))
	}

	// Remove require itself — extensions must use the grut host API, not
	// the Lua module system.
	r.state.SetGlobal("require", lua.LNil)

	// Remove string.dump which can serialize function bytecode, potentially
	// enabling sandbox escape via bytecode manipulation.
	// Also wrap string.rep to cap output length, preventing memory exhaustion
	// DoS (string.rep is a single Go call that bypasses context timeout checks).
	strMod := r.state.GetGlobal("string")
	if tbl, ok := strMod.(*lua.LTable); ok {
		tbl.RawSetString("dump", lua.LNil)

		// Cap string.rep to 1 MiB output to prevent memory bomb attacks.
		const maxRepBytes = 1 << 20 // 1 MiB
		origRep := tbl.RawGetString("rep")
		tbl.RawSetString("rep", r.state.NewFunction(func(L *lua.LState) int {
			s := L.CheckString(1)
			n := L.CheckInt(2)
			if n < 0 {
				n = 0
			}
			if int64(len(s))*int64(n) > maxRepBytes {
				L.ArgError(2, "string.rep result exceeds 1 MiB sandbox limit")
				return 0
			}
			L.Push(origRep)
			L.Push(lua.LString(s))
			L.Push(lua.LNumber(n))
			L.Call(2, 1)
			return 1
		}))
	}
}

// registerHostAPI creates the "grut" table with toast, register_command, and
// set_status functions so that extension scripts can interact with the host.
func (r *LuaRuntime) registerHostAPI() {
	mod := r.state.NewTable()

	r.state.SetField(mod, "toast", r.state.NewFunction(r.luaToast))
	r.state.SetField(mod, "register_command", r.state.NewFunction(r.luaRegisterCommand))
	r.state.SetField(mod, "set_status", r.state.NewFunction(r.luaSetStatus))

	r.state.SetGlobal("grut", mod)
}

// luaToast implements grut.toast(title, message, level).
// Requires the "notify" permission.
func (r *LuaRuntime) luaToast(l *lua.LState) int {
	if !extension.ManifestHasPermission(r.manifest, extension.PermNotify) {
		l.RaiseError("permission denied: %q requires %q permission",
			"grut.toast", string(extension.PermNotify))
		return 0
	}
	title := l.CheckString(1)
	message := l.CheckString(2)
	level := l.OptString(3, logInfo)
	r.hostAPI.ShowToast(title, message, level)
	return 0
}

// luaRegisterCommand implements grut.register_command(name, desc, callback).
func (r *LuaRuntime) luaRegisterCommand(l *lua.LState) int {
	name := l.CheckString(1)
	desc := l.CheckString(2)
	fn := l.CheckFunction(3)

	r.hostAPI.RegisterCommand(name, desc, func() error {
		if err := l.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}); err != nil {
			return fmt.Errorf("lua command %q: %w", name, err)
		}
		return nil
	})
	return 0
}

// luaSetStatus implements grut.set_status(key, value).
func (r *LuaRuntime) luaSetStatus(l *lua.LState) int {
	key := l.CheckString(1)
	value := l.CheckString(2)
	r.hostAPI.SetStatusBarItem(key, value)
	return 0
}

// execWithTimeout runs the function at the top of the Lua stack with a
// context-based deadline. If the deadline fires the VM is forcefully closed.
func (r *LuaRuntime) execWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	r.state.SetContext(ctx)
	defer r.state.RemoveContext()

	if err := r.state.PCall(0, lua.MultRet, nil); err != nil {
		// Distinguish timeout from other errors.
		if ctx.Err() != nil {
			return fmt.Errorf("lua: execution timed out after %s", r.timeout)
		}
		return fmt.Errorf("lua: exec: %w", err)
	}
	return nil
}
