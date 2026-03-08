package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/jongio/grut/internal/extension"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// defaultMemoryLimitPages is the maximum WASM linear-memory size measured in
// 64 KiB pages. 1024 pages = 64 MiB.
const defaultMemoryLimitPages = 1024

// WASMRuntime executes WASM extensions inside a sandboxed wazero instance.
type WASMRuntime struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	mod      api.Module
	manifest *extension.Manifest
	hostAPI  HostAPI
}

// Compile-time check: *WASMRuntime satisfies the Runtime interface.
var _ Runtime = (*WASMRuntime)(nil)

// NewWASMRuntime creates a new WASM runtime configured with a 64 MiB memory
// cap and no filesystem access. The returned runtime is ready for Load.
func NewWASMRuntime(manifest *extension.Manifest, hostAPI HostAPI) (*WASMRuntime, error) {
	if manifest == nil {
		return nil, fmt.Errorf("wasm: manifest is required")
	}
	if hostAPI == nil {
		return nil, fmt.Errorf("wasm: host API is required")
	}

	cfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(defaultMemoryLimitPages)

	rt := wazero.NewRuntimeWithConfig(context.Background(), cfg)

	return &WASMRuntime{
		rt:       rt,
		manifest: manifest,
		hostAPI:  hostAPI,
	}, nil
}

// Name returns the runtime identifier.
func (w *WASMRuntime) Name() string { return "wasm" }

// Load reads a WASM binary from entryPoint, compiles it, registers host
// imports, and instantiates the module.
func (w *WASMRuntime) Load(entryPoint string) error {
	data, err := os.ReadFile(entryPoint)
	if err != nil {
		return fmt.Errorf("wasm: read module %q: %w", entryPoint, err)
	}
	return w.loadBytes(data)
}

// loadBytes compiles and instantiates a WASM module from raw bytes. Exported
// only within the package (lower-case) so tests can bypass the filesystem.
func (w *WASMRuntime) loadBytes(wasmBytes []byte) error {
	ctx := context.Background()

	// Register host imports under the "grut" module before compiling the
	// guest, so that any imports resolve correctly.
	if err := w.registerHostFunctions(ctx); err != nil {
		return fmt.Errorf("wasm: register host functions: %w", err)
	}

	compiled, err := w.rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("wasm: compile module: %w", err)
	}
	w.compiled = compiled

	modCfg := wazero.NewModuleConfig().
		WithName(w.manifest.Name).
		WithStartFunctions() // do not auto-run _start

	mod, err := w.rt.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		return fmt.Errorf("wasm: instantiate module: %w", err)
	}
	w.mod = mod

	return nil
}

// Close releases all resources held by the wazero runtime.
func (w *WASMRuntime) Close() {
	ctx := context.Background()
	if w.mod != nil {
		_ = w.mod.Close(ctx)
		w.mod = nil
	}
	if w.rt != nil {
		_ = w.rt.Close(ctx)
		w.rt = nil
	}
}

// registerHostFunctions creates the "grut" host module with toast, set_status,
// and log imports.
func (w *WASMRuntime) registerHostFunctions(ctx context.Context) error {
	_, err := w.rt.NewHostModuleBuilder("grut").
		// toast(title_ptr, title_len, msg_ptr, msg_len, level)
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module,
			titlePtr, titleLen, msgPtr, msgLen, level uint32,
		) {
			title, okT := readString(mod, titlePtr, titleLen)
			msg, okM := readString(mod, msgPtr, msgLen)
			if !okT || !okM {
				return // bad pointers from WASM module — skip call
			}
			w.hostAPI.ShowToast(title, msg, levelToString(level))
		}).
		Export("toast").
		// set_status(key_ptr, key_len, val_ptr, val_len)
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module,
			keyPtr, keyLen, valPtr, valLen uint32,
		) {
			key, okK := readString(mod, keyPtr, keyLen)
			val, okV := readString(mod, valPtr, valLen)
			if !okK || !okV {
				return // bad pointers from WASM module — skip call
			}
			w.hostAPI.SetStatusBarItem(key, val)
		}).
		Export("set_status").
		// log(msg_ptr, msg_len)
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module,
			msgPtr, msgLen uint32,
		) {
			msg, ok := readString(mod, msgPtr, msgLen)
			if !ok {
				return // bad pointer from WASM module — skip call
			}
			w.hostAPI.Log(msg)
		}).
		Export("log").
		Instantiate(ctx)

	return err
}

// readString reads a UTF-8 string from the module's linear memory.
func readString(mod api.Module, ptr, length uint32) (string, bool) {
	if length == 0 {
		return "", true
	}
	data, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return "", false
	}
	return string(data), true
}

// Toast level constants for WASM host imports.
const (
	toastLevelWarn  = 1
	toastLevelError = 2
)

// levelToString converts a numeric toast level to a string.
func levelToString(level uint32) string {
	switch level {
	case toastLevelWarn:
		return "warn"
	case toastLevelError:
		return "error"
	default:
		return "info"
	}
}
