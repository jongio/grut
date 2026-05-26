package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/jongio/grut/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestManifest returns a minimal valid MCP manifest for testing.
// Includes the "process" permission required by the MCP runtime's Load
// method so existing lifecycle tests continue to pass.
func newTestManifest() *extension.Manifest {
	return &extension.Manifest{
		Name:        "test-ext",
		Version:     "1.0.0",
		Runtime:     "mcp",
		Permissions: []string{"process"},
	}
}

// newTestRuntime creates an MCPRuntime with a test manifest.
func newTestRuntime(t *testing.T) *MCPRuntime {
	t.Helper()
	rt, err := NewMCPRuntime(newTestManifest())
	require.NoError(t, err)
	return rt
}

// buildHelper compiles a Go source string into a temporary binary and
// returns the path. The binary is cleaned up when the test finishes.
func buildHelper(t *testing.T, name, code string) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "go.mod"),
		[]byte("module testhelper\ngo 1.21\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(code),
		0o644,
	))

	bin := filepath.Join(dir, name)
	if goruntime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s: %s", name, string(out))

	return bin
}

// buildBlockingHelper returns a binary that blocks reading from stdin.
func buildBlockingHelper(t *testing.T) string {
	t.Helper()
	return buildHelper(t, "blocking", `package main

import "os"

func main() {
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
}
`)
}

// buildExitingHelper returns a binary that exits immediately.
func buildExitingHelper(t *testing.T) string {
	t.Helper()
	return buildHelper(t, "exiting", `package main

func main() {}
`)
}

// ---------------------------------------------------------------------------
// resolveCommand tests
// ---------------------------------------------------------------------------

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		wantName string
		wantArgs []string
	}{
		{
			name:     "python script",
			entry:    "/path/to/server.py",
			wantName: pythonInterpreter(),
			wantArgs: []string{"/path/to/server.py"},
		},
		{
			name:     "python uppercase ext",
			entry:    "server.PY",
			wantName: pythonInterpreter(),
			wantArgs: []string{"server.PY"},
		},
		{
			name:     "javascript",
			entry:    "server.js",
			wantName: "node",
			wantArgs: []string{"server.js"},
		},
		{
			name:     "typescript",
			entry:    "server.ts",
			wantName: "npx",
			wantArgs: []string{"tsx", "server.ts"},
		},
		{
			name:     "binary no ext",
			entry:    "/usr/local/bin/mcp-server",
			wantName: "/usr/local/bin/mcp-server",
			wantArgs: nil,
		},
		{
			name:     "binary exe",
			entry:    "server.exe",
			wantName: "server.exe",
			wantArgs: nil,
		},
		{
			name:     "unknown ext treated as binary",
			entry:    "plugin.wasm",
			wantName: "plugin.wasm",
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := resolveCommand(tt.entry)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestFilterEnvForSubprocess_AllowlistOnly(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("ANTHROPIC_API_KEY", "secret-key")
	t.Setenv("CUSTOM_SECRET", "secret")

	filtered := filterEnvForSubprocess()
	joined := strings.Join(filtered, "\n")

	assert.Contains(t, joined, "PATH=")
	assert.Contains(t, joined, "HOME=")
	assert.NotContains(t, joined, "GITHUB_TOKEN=")
	assert.NotContains(t, joined, "ANTHROPIC_API_KEY=")
	assert.NotContains(t, joined, "CUSTOM_SECRET=")
}

// pythonInterpreter returns the expected Python interpreter name for the
// current platform.
func pythonInterpreter() string {
	if goruntime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNewMCPRuntime(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		rt, err := NewMCPRuntime(newTestManifest())
		require.NoError(t, err)
		assert.NotNil(t, rt)
		assert.Equal(t, "mcp", rt.Name())
	})

	t.Run("NilManifest", func(t *testing.T) {
		rt, err := NewMCPRuntime(nil)
		assert.Nil(t, rt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest is required")
	})
}

// ---------------------------------------------------------------------------
// Name test
// ---------------------------------------------------------------------------

func TestMCPRuntime_Name(t *testing.T) {
	rt := newTestRuntime(t)
	assert.Equal(t, "mcp", rt.Name())
}

// ---------------------------------------------------------------------------
// Process lifecycle tests
// ---------------------------------------------------------------------------

func TestMCPRuntime_LoadAndRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildBlockingHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))
	assert.True(t, rt.Running(), "process should be running after Load")
}

func TestMCPRuntime_CloseStopsProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildBlockingHelper(t)
	rt := newTestRuntime(t)

	require.NoError(t, rt.Load(bin))
	assert.True(t, rt.Running())

	rt.Close()
	assert.False(t, rt.Running(), "process should not be running after Close")
}

func TestMCPRuntime_CloseAlreadyExited(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildExitingHelper(t)
	rt := newTestRuntime(t)

	require.NoError(t, rt.Load(bin))

	// Wait for the process to exit on its own.
	require.Eventually(t, func() bool {
		return !rt.Running()
	}, 5*time.Second, 50*time.Millisecond, "process should exit on its own")

	// Close on an already-exited process should be a no-op.
	rt.Close()
	assert.False(t, rt.Running())
}

func TestMCPRuntime_CloseWithoutLoad(t *testing.T) {
	rt := newTestRuntime(t)
	// Should not panic or block.
	rt.Close()
	assert.False(t, rt.Running())
}

func TestMCPRuntime_CloseIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildBlockingHelper(t)
	rt := newTestRuntime(t)

	require.NoError(t, rt.Load(bin))
	rt.Close()
	assert.False(t, rt.Running())

	// Second close should be a no-op.
	rt.Close()
	assert.False(t, rt.Running())
}

func TestMCPRuntime_LoadInvalidEntryPoint(t *testing.T) {
	rt := newTestRuntime(t)
	err := rt.Load("/nonexistent/path/to/binary")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp runtime: start")
	assert.False(t, rt.Running())
}

func TestMCPRuntime_LoadTwice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildBlockingHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))
	err := rt.Load(bin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already loaded")
}

// ---------------------------------------------------------------------------
// SendRequest without load
// ---------------------------------------------------------------------------

func TestMCPRuntime_SendRequestNotLoaded(t *testing.T) {
	rt := newTestRuntime(t)
	_, err := rt.SendRequest("ping", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not loaded")
}

func TestMCPRuntime_SendRequestAfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	bin := buildBlockingHelper(t)
	rt := newTestRuntime(t)

	require.NoError(t, rt.Load(bin))
	rt.Close()

	_, err := rt.SendRequest("ping", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "process exited")
}
