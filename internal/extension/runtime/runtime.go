// Package runtime provides sandboxed execution environments for grut extensions.
// Each runtime (Lua, WASM, MCP) implements the Runtime interface, allowing the
// extension manager to load and execute user extensions uniformly.
package runtime

// Runtime is the common interface for all extension runtimes.
type Runtime interface {
	// Load reads and executes the extension entry-point script or module.
	Load(entryPoint string) error
	// Close releases all resources owned by the runtime.
	Close()
	// Name returns the runtime identifier (e.g. "lua", "wasm", "mcp").
	Name() string
}

// HostAPI exposes grut functionality to extension code. Implementations bridge
// calls from sandboxed extension code into the host application.
type HostAPI interface {
	// ShowToast displays a notification to the user.
	// Level should be "info", "warn", or "error".
	ShowToast(title, message, level string)
	// RegisterCommand adds a user-invokable command to the command palette.
	RegisterCommand(name, description string, handler func() error)
	// SetStatusBarItem sets a key-value pair displayed in the status bar.
	SetStatusBarItem(key, value string)
	// Log writes an informational message to the host log.
	Log(msg string)
}
