package panels

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// OpenInEditor — exercise LookPath loop body (common-editor discovery)
// ---------------------------------------------------------------------------

// TestOpenInEditor_CommonEditorLookPath exercises the for-loop that searches
// for common developer editors (code, code-insiders, cursor) via LookPath.
// A dummy "code" script is placed on PATH so the inner loop body executes,
// covering the startDetached call from within the LookPath hit path.
func TestOpenInEditor_CommonEditorLookPath(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	dir := t.TempDir()

	if runtime.GOOS == "windows" {
		// Windows resolves .bat via PATHEXT; create a no-op batch file.
		err := os.WriteFile(filepath.Join(dir, "code.bat"), []byte("@exit /b 0\r\n"), 0o755)
		require.NoError(t, err)
	} else {
		err := os.WriteFile(filepath.Join(dir, "code"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
		require.NoError(t, err)
	}

	// Prepend dir so LookPath("code") finds our dummy.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := OpenInEditor("somefile.txt")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// OpenInEditor — exercise platform-default fallback (Windows)
// ---------------------------------------------------------------------------

// TestOpenInEditor_PlatformDefaultWindows covers the platform-specific
// fallback when VISUAL, EDITOR, and all common editors are absent.
// On Windows this executes: cmd /c start "" <path>.
func TestOpenInEditor_PlatformDefaultWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		// Non-Windows default is covered by TestOpenInEditorFallsThroughToDefault.
		t.Skip("Windows-only platform-default test")
	}

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	// Minimal PATH — only system32, so code/code-insiders/cursor are absent.
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	t.Setenv("PATH", filepath.Join(sysRoot, "system32"))

	f, err := os.CreateTemp(t.TempDir(), "platform-default-*.txt")
	require.NoError(t, err)
	_ = f.Close()

	// startDetached returns nil after Start() succeeds; the window spawned
	// by "cmd /c start" is cleaned up when the CI runner terminates.
	err = OpenInEditor(f.Name())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// OpenInBrowser — exercise Windows platform branch
// ---------------------------------------------------------------------------

// TestOpenInBrowser_WindowsPlatform exercises the rundll32 code path that is
// only reached when validation passes on Windows.
func TestOpenInBrowser_WindowsPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only platform test")
	}

	// Use loopback address with unlikely port — rundll32 starts but the
	// URL goes nowhere. startDetached returns nil after Start() succeeds.
	err := OpenInBrowser("https://127.0.0.1:1")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// OpenInTerminal — exercise Windows platform branch
// ---------------------------------------------------------------------------

// TestOpenInTerminal_WindowsPlatform exercises the Windows cmd.exe code path
// including the var declaration, switch, assignment, nil-check, and
// startDetached return.
func TestOpenInTerminal_WindowsPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only platform test")
	}

	dir := t.TempDir()
	err := OpenInTerminal(dir)
	// cmd.exe /c start cmd /k ... — Start() succeeds, returns nil.
	assert.NoError(t, err)
}
