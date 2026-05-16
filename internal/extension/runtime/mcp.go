package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jongio/grut/internal/extension"
)

// MCPRuntime manages an MCP extension server as a subprocess,
// communicating via JSON-RPC 2.0 over stdin/stdout pipes.
type MCPRuntime struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   *ringBuffer // captures stderr for diagnostics (GRT-007)
	scanner  *bufio.Scanner
	manifest *extension.Manifest
	done     chan struct{}
	cancel   context.CancelFunc // cancels the command context on Close
	mu       sync.Mutex         // guards lifecycle state (cmd, stdin, stdout, scanner)
	ioMu     sync.Mutex         // serializes SendRequest I/O
	nextID   atomic.Int64
}

// mcpSubprocessTimeout is the maximum lifetime for an MCP extension
// subprocess. This prevents runaway processes from consuming resources
// indefinitely (CWE-400).
const mcpSubprocessTimeout = 30 * time.Minute

// ringBuffer is a simple bounded buffer that keeps the last N lines
// of output for diagnostics.
type ringBuffer struct {
	lines []string
	max   int
	mu    sync.Mutex
}

func (rb *ringBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		if line == "" {
			continue
		}
		if len(rb.lines) >= rb.max {
			rb.lines = rb.lines[1:]
		}
		rb.lines = append(rb.lines, line)
	}
	return len(p), nil
}

func (rb *ringBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return strings.Join(rb.lines, "\n")
}

// Compile-time interface check.
var _ Runtime = (*MCPRuntime)(nil)

// jsonRPCRequest is a JSON-RPC 2.0 request sent to the subprocess.
type jsonRPCRequest struct {
	Params  any    `json:"params,omitempty"`
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int64  `json:"id"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response received from the subprocess.
type jsonRPCResponse struct {
	Error   *jsonRPCError   `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	ID      int64           `json:"id"`
}

// jsonRPCError describes an error returned by the subprocess.
type jsonRPCError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewMCPRuntime creates a new MCP runtime for the given extension manifest.
// The runtime is not started until Load is called.
func NewMCPRuntime(manifest *extension.Manifest) (*MCPRuntime, error) {
	if manifest == nil {
		return nil, fmt.Errorf("mcp runtime: manifest is required")
	}
	return &MCPRuntime{
		manifest: manifest,
		done:     make(chan struct{}),
	}, nil
}

// Name returns the runtime type identifier.
func (m *MCPRuntime) Name() string { return "mcp" }

// Load starts the MCP server subprocess using the given entry point.
// The interpreter is determined from the entry point file extension:
//   - .py → python3 (python on Windows)
//   - .js → node
//   - .ts → npx tsx
//   - binary (no ext or .exe) → direct execution
func (m *MCPRuntime) Load(entryPoint string) error {
	// Enforce the "process" permission before spawning a subprocess.
	// MCP extensions inherit the user's full OS privileges; without this
	// gate an extension that omits "process" could still spawn arbitrary
	// processes (CWE-862).
	if !extension.ManifestHasPermission(m.manifest, extension.PermProcess) {
		return &extension.ErrPermissionDenied{
			Extension:  m.manifest.Name,
			Permission: extension.PermProcess,
			Operation:  "spawn subprocess",
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil {
		return fmt.Errorf("mcp runtime: already loaded")
	}
	name, args := resolveCommand(entryPoint)
	// Use CommandContext with a timeout to prevent runaway subprocesses
	// (CWE-400). The context is cancelled on Close().
	ctx, cancel := context.WithTimeout(context.Background(), mcpSubprocessTimeout)
	cmd := exec.CommandContext(ctx, name, args...)
	// Place the subprocess in its own process group so the entire tree
	// can be killed on Close, preventing orphaned children (CWE-269).
	setProcGroup(cmd)
	// Filter environment to prevent secret leakage to untrusted
	// extension subprocesses (CWE-526). Only safe variables are passed.
	cmd.Env = filterEnvForSubprocess()
	// Capture stderr to a bounded ring buffer for diagnostics (CWE-223).
	stderrBuf := &ringBuffer{max: 100}
	cmd.Stderr = stderrBuf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("mcp runtime: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return fmt.Errorf("mcp runtime: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		return fmt.Errorf("mcp runtime: start %q: %w", name, err)
	}
	m.cmd = cmd
	m.stdin = stdin
	m.stdout = stdout
	m.stderr = stderrBuf
	m.scanner = bufio.NewScanner(stdout)
	m.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1 MiB max token
	m.cancel = cancel
	// Monitor process exit in background.
	go func() {
		err := cmd.Wait()
		if err != nil {
			slog.Debug("mcp runtime: process exited", "error", err, "stderr", stderrBuf.String())
		}
		close(m.done)
	}()
	return nil
}

// Close stops the MCP server subprocess. It closes stdin first, then
// kills the process and waits for it to exit. Safe to call multiple
// times or on an already-exited process.
func (m *MCPRuntime) Close() {
	m.mu.Lock()
	if m.cmd == nil || m.cmd.Process == nil {
		m.mu.Unlock()
		return
	}
	// Check if already exited.
	select {
	case <-m.done:
		m.mu.Unlock()
		return
	default:
	}
	// Close stdin to signal the subprocess to exit.
	if m.stdin != nil {
		_ = m.stdin.Close()
	}
	// Cancel the command context (triggers process kill via CommandContext).
	if m.cancel != nil {
		m.cancel()
	}
	// Kill the process group first (Unix: kills all children), then
	// fall back to killing the direct child process.
	killProcGroup(m.cmd)
	_ = m.cmd.Process.Kill()
	m.mu.Unlock()
	// Wait for the monitor goroutine to finish (outside lock to
	// avoid blocking other callers like Running).
	<-m.done
}

// Running reports whether the subprocess is still alive.
func (m *MCPRuntime) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

// SendRequest sends a JSON-RPC 2.0 request to the subprocess and returns
// the result. Requests are serialized; only one is in flight at a time.
func (m *MCPRuntime) SendRequest(method string, params any) (json.RawMessage, error) {
	// Snapshot references under lifecycle lock, then release.
	m.mu.Lock()
	if m.cmd == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("mcp runtime: not loaded")
	}
	select {
	case <-m.done:
		m.mu.Unlock()
		return nil, fmt.Errorf("mcp runtime: process exited")
	default:
	}
	stdin := m.stdin
	scanner := m.scanner
	m.mu.Unlock()
	// Serialize I/O so concurrent callers don't interleave.
	m.ioMu.Lock()
	defer m.ioMu.Unlock()
	id := m.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp runtime: marshal request: %w", err)
	}
	// Write newline-delimited JSON.
	data = append(data, '\n')
	if _, err := stdin.Write(data); err != nil {
		return nil, fmt.Errorf("mcp runtime: write request: %w", err)
	}
	// Read a single response line.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("mcp runtime: read response: %w", err)
		}
		return nil, fmt.Errorf("mcp runtime: unexpected EOF")
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("mcp runtime: unmarshal response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp runtime: server error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// resolveCommand determines the interpreter and arguments for the given
// entry point based on its file extension.
func resolveCommand(entryPoint string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(entryPoint))
	switch ext {
	case ".py":
		interpreter := "python3"
		if goruntime.GOOS == "windows" { //nolint:goconst // standard platform literal
			interpreter = "python"
		}
		return interpreter, []string{entryPoint}
	case ".js":
		return "node", []string{entryPoint}
	case ".ts":
		return "npx", []string{"tsx", entryPoint}
	default:
		// Binary (no extension or .exe) — execute directly.
		return entryPoint, nil
	}
}

// safeEnvAllowlist lists the exact environment variable names that are safe
// to pass to untrusted extension subprocesses. Secrets and credentials are
// intentionally excluded (CWE-526).
var safeEnvAllowlist = map[string]struct{}{
	"PATH":               {},
	"HOME":               {},
	"TMPDIR":             {},
	"TMP":                {},
	"TEMP":               {},
	"LANG":               {},
	"LC_ALL":             {},
	"TERM":               {},
	"USER":               {},
	"USERNAME":           {},
	"SHELL":              {},
	"COMSPEC":            {},
	"SYSTEMROOT":         {},
	"WINDIR":             {},
	"USERPROFILE":        {},
	"LOCALAPPDATA":       {},
	"APPDATA":            {},
	"PROGRAMFILES":       {},
	"PROGRAMFILES(X86)":  {},
	"COMMONPROGRAMFILES": {},
	"PROGRAMDATA":        {},
}

// filterEnvForSubprocess returns a filtered copy of the current environment
// containing only safe variables. Secrets and tokens are excluded.
func filterEnvForSubprocess() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		name, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, allowed := safeEnvAllowlist[strings.ToUpper(name)]; allowed {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
