package panels

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStartDetachedCapture replaces startDetachedFn with a no-op that captures
// that retrieves the captured command after the test exercises the code path.
func stubStartDetachedCapture(t *testing.T) func() *exec.Cmd {
	t.Helper()
	var captured *exec.Cmd
	orig := startDetachedFn
	startDetachedFn = func(cmd *exec.Cmd) error {
		captured = cmd
		return nil
	}
	t.Cleanup(func() { startDetachedFn = orig })
	return func() *exec.Cmd { return captured }
}

// ---------------------------------------------------------------------------
// OpenInEditor — exercise LookPath loop body (common-editor discovery)
// ---------------------------------------------------------------------------

func TestOpenInEditor_CommonEditorLookPath(t *testing.T) {
	getCaptured := stubStartDetachedCapture(t)

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	dir := t.TempDir()

	if runtime.GOOS == "windows" {
		err := os.WriteFile(filepath.Join(dir, "code.bat"), []byte("@exit /b 0\r\n"), 0o755)
		require.NoError(t, err)
	} else {
		err := os.WriteFile(filepath.Join(dir, "code"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
		require.NoError(t, err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := OpenInEditor("somefile.txt")
	assert.NoError(t, err)

	cmd := getCaptured()
	require.NotNil(t, cmd, "startDetachedFn should have been called")
	assert.Contains(t, cmd.Args[0], "code")
}

// ---------------------------------------------------------------------------
// OpenInEditor — exercise platform-default fallback (Windows)
// ---------------------------------------------------------------------------

func TestOpenInEditor_PlatformDefaultWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only platform-default test")
	}

	getCaptured := stubStartDetachedCapture(t)

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	t.Setenv("PATH", filepath.Join(sysRoot, "system32"))

	f, err := os.CreateTemp(t.TempDir(), "platform-default-*.txt")
	require.NoError(t, err)
	_ = f.Close()

	err = OpenInEditor(f.Name())
	assert.NoError(t, err)

	cmd := getCaptured()
	require.NotNil(t, cmd, "startDetachedFn should have been called")
	assert.Equal(t, "cmd", cmd.Args[0])
	assert.Contains(t, strings.Join(cmd.Args, " "), "start")
}

// ---------------------------------------------------------------------------
// OpenInBrowser — exercise Windows platform branch
// ---------------------------------------------------------------------------

func TestOpenInBrowser_WindowsPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only platform test")
	}

	getCaptured := stubStartDetachedCapture(t)

	err := OpenInBrowser("https://127.0.0.1:1")
	assert.NoError(t, err)

	cmd := getCaptured()
	require.NotNil(t, cmd, "startDetachedFn should have been called")
	assert.Equal(t, "rundll32", cmd.Args[0])
}

// ---------------------------------------------------------------------------
// OpenInTerminal — exercise Windows platform branch
// ---------------------------------------------------------------------------

func TestOpenInTerminal_WindowsPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only platform test")
	}

	getCaptured := stubStartDetachedCapture(t)

	dir := t.TempDir()
	err := OpenInTerminal(dir)
	assert.NoError(t, err)

	cmd := getCaptured()
	require.NotNil(t, cmd, "startDetachedFn should have been called")
	assert.Equal(t, "cmd", cmd.Args[0])
	assert.Contains(t, strings.Join(cmd.Args, " "), "/k")
}
