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

// --- Hand-crafted minimal WASM binaries for testing ---

// minimalWASM is the smallest valid WASM module: just the 8-byte header plus a
// memory section (1 page min, 1 page max).
var minimalWASM = []byte{
	0x00, 0x61, 0x73, 0x6d, // magic "\0asm"
	0x01, 0x00, 0x00, 0x00, // version 1
	// Memory section (id=5): 1 memory, has-max, min=1, max=1
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01,
}

// toastCallWASM is a module that imports grut.toast and exports a function
// "test_toast" which calls toast("Hello", "World!", warn). The module's data
// section pre-populates linear memory with the two strings at known offsets.
//
//	(module
//	  (import "grut" "toast" (func $toast (param i32 i32 i32 i32 i32)))
//	  (memory (export "memory") 1 1)
//	  (data (i32.const 0) "Hello")
//	  (data (i32.const 5) "World!")
//	  (func (export "test_toast")
//	    i32.const 0   ;; title_ptr
//	    i32.const 5   ;; title_len
//	    i32.const 5   ;; msg_ptr
//	    i32.const 6   ;; msg_len
//	    i32.const 1   ;; level = warn
//	    call $toast
//	  )
//	)
var toastCallWASM = []byte{
	// Header
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,

	// Type section (id=1, size=12)
	0x01, 0x0c, 0x02,
	0x60, 0x05, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x00, // type 0: (i32 i32 i32 i32 i32)->()
	0x60, 0x00, 0x00, // type 1: ()->()

	// Import section (id=2, size=14)
	0x02, 0x0e, 0x01,
	0x04, 0x67, 0x72, 0x75, 0x74, // "grut"
	0x05, 0x74, 0x6f, 0x61, 0x73, 0x74, // "toast"
	0x00, 0x00, // func, type 0

	// Function section (id=3, size=2)
	0x03, 0x02, 0x01, 0x01, // 1 func, type index 1

	// Memory section (id=5, size=4)
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01, // 1 mem, has-max, min=1, max=1

	// Export section (id=7, size=23)
	0x07, 0x17, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, // "memory"
	0x02, 0x00, // memory, index 0
	0x0a, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x74, 0x6f, 0x61, 0x73, 0x74, // "test_toast"
	0x00, 0x01, // func, index 1

	// Code section (id=10, size=16)
	0x0a, 0x10, 0x01,
	0x0e, 0x00, // body size=14, 0 locals
	0x41, 0x00, // i32.const 0  (title_ptr → "Hello")
	0x41, 0x05, // i32.const 5  (title_len)
	0x41, 0x05, // i32.const 5  (msg_ptr → "World!")
	0x41, 0x06, // i32.const 6  (msg_len)
	0x41, 0x01, // i32.const 1  (level = warn)
	0x10, 0x00, // call 0 (imported toast)
	0x0b, // end

	// Data section (id=11, size=22)
	0x0b, 0x16, 0x02,
	0x00, 0x41, 0x00, 0x0b, // segment 0: active, offset=0
	0x05, 0x48, 0x65, 0x6c, 0x6c, 0x6f, // "Hello"
	0x00, 0x41, 0x05, 0x0b, // segment 1: active, offset=5
	0x06, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21, // "World!"
}

// logCallWASM is a module that imports grut.log and exports "test_log" which
// calls log("ping").
//
//	(module
//	  (import "grut" "log" (func $log (param i32 i32)))
//	  (memory (export "memory") 1 1)
//	  (data (i32.const 0) "ping")
//	  (func (export "test_log")
//	    i32.const 0
//	    i32.const 4
//	    call $log
//	  )
//	)
var logCallWASM = []byte{
	// Header
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,

	// Type section (id=1, size=9)
	0x01, 0x09, 0x02,
	0x60, 0x02, 0x7f, 0x7f, 0x00, // type 0: (i32 i32)->()
	0x60, 0x00, 0x00, // type 1: ()->()

	// Import section (id=2, size=12): count(1) + "grut"(5) + "log"(4) + desc(2) = 12
	0x02, 0x0c, 0x01,
	0x04, 0x67, 0x72, 0x75, 0x74, // "grut"
	0x03, 0x6c, 0x6f, 0x67, // "log"
	0x00, 0x00, // func, type 0

	// Function section (id=3, size=2)
	0x03, 0x02, 0x01, 0x01,

	// Memory section (id=5, size=4)
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01,

	// Export section (id=7, size=21): count(1) + "memory"(9) + "test_log"(11) = 21
	0x07, 0x15, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, // "memory"
	0x02, 0x00, // memory, index 0
	0x08, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x6c, 0x6f, 0x67, // "test_log"
	0x00, 0x01, // func, index 1

	// Code section (id=10, size=10)
	0x0a, 0x0a, 0x01,
	0x08, 0x00, // body size=8, 0 locals
	0x41, 0x00, // i32.const 0 (msg_ptr)
	0x41, 0x04, // i32.const 4 (msg_len)
	0x10, 0x00, // call 0
	0x0b, // end

	// Data section (id=11, size=10): count(1) + segment(9) = 10
	0x0b, 0x0a, 0x01,
	0x00, 0x41, 0x00, 0x0b, // active, offset=0
	0x04, 0x70, 0x69, 0x6e, 0x67, // "ping"
}

// oversizedMemoryWASM declares a memory of 2048 pages min (128 MiB), exceeding
// the runtime's 1024-page (64 MiB) limit.
var oversizedMemoryWASM = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// Memory section: 1 memory, has-max, min=2048, max=2048 (LEB128)
	0x05, 0x06, 0x01, 0x01, 0x80, 0x10, 0x80, 0x10,
}

// wasmManifest returns a minimal manifest for WASM tests.
func wasmManifest() *extension.Manifest {
	return &extension.Manifest{
		Name:    "wasm-test",
		Version: "1.0.0",
		Runtime: "wasm",
	}
}

// --- Tests ---

func TestNewWASMRuntime(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	require.NotNil(t, rt)
	defer rt.Close()

	assert.Equal(t, "wasm", rt.Name())
}

func TestNewWASMRuntime_NilManifest(t *testing.T) {
	_, err := NewWASMRuntime(nil, newMockHostAPI())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest is required")
}

func TestNewWASMRuntime_NilHostAPI(t *testing.T) {
	_, err := NewWASMRuntime(wasmManifest(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host API is required")
}

func TestWASMRuntime_LoadMinimalModule(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.loadBytes(minimalWASM)
	require.NoError(t, err)
}

func TestWASMRuntime_LoadFromFile(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// Write minimal WASM to a temp file and load via the filesystem path.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wasm")
	require.NoError(t, os.WriteFile(path, minimalWASM, 0o644))

	require.NoError(t, rt.Load(path))
}

func TestWASMRuntime_LoadNonexistentFile(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.Load(filepath.Join(t.TempDir(), "missing.wasm"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read module")
}

func TestWASMRuntime_InvalidWASM(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	err = rt.loadBytes([]byte{0xde, 0xad, 0xbe, 0xef})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile module")
}

func TestWASMRuntime_HostImportToast(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	require.NoError(t, rt.loadBytes(toastCallWASM))

	// Call the exported "test_toast" function.
	fn := rt.mod.ExportedFunction("test_toast")
	require.NotNil(t, fn, "test_toast should be exported")

	_, err = fn.Call(context.Background())
	require.NoError(t, err)

	// The WASM module passes title="Hello", msg="World!", level=1 (warn).
	api.mu.Lock()
	defer api.mu.Unlock()

	require.Len(t, api.toasts, 1)
	assert.Equal(t, "Hello", api.toasts[0].Title)
	assert.Equal(t, "World!", api.toasts[0].Message)
	assert.Equal(t, "warn", api.toasts[0].Level)
}

func TestWASMRuntime_HostImportLog(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	require.NoError(t, rt.loadBytes(logCallWASM))

	fn := rt.mod.ExportedFunction("test_log")
	require.NotNil(t, fn, "test_log should be exported")

	_, err = fn.Call(context.Background())
	require.NoError(t, err)

	api.mu.Lock()
	defer api.mu.Unlock()

	require.Len(t, api.logs, 1)
	assert.Equal(t, "ping", api.logs[0])
}

func TestWASMRuntime_MemoryLimitEnforced(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	// A module requesting 2048 pages (128 MiB) should fail when the runtime
	// caps memory at 1024 pages (64 MiB).
	err = rt.loadBytes(oversizedMemoryWASM)
	require.Error(t, err, "loading a module that exceeds memory limit should fail")
}

func TestWASMRuntime_Close(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)

	require.NoError(t, rt.loadBytes(minimalWASM))

	// First close should succeed without panic.
	rt.Close()

	// Second close (idempotent) should also be safe.
	rt.Close()
}

func TestWASMRuntime_CloseWithoutLoad(t *testing.T) {
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)

	// Closing without ever loading should not panic.
	rt.Close()
}

func TestWASMRuntime_InterfaceCompliance(t *testing.T) {
	// Verify that *WASMRuntime can be assigned to a Runtime variable.
	api := newMockHostAPI()
	rt, err := NewWASMRuntime(wasmManifest(), api)
	require.NoError(t, err)
	defer rt.Close()

	var iface Runtime = rt
	assert.Equal(t, "wasm", iface.Name())
}

// ---------------------------------------------------------------------------
// levelToString edge cases
// ---------------------------------------------------------------------------

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level uint32
		want  string
	}{
		{0, "info"},
		{1, "warn"},
		{2, "error"},
		{3, "info"},   // default branch
		{99, "info"},  // default branch
		{255, "info"}, // default branch
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, levelToString(tt.level), "level=%d", tt.level)
	}
}
