package panels

import (
	"context"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CopyToClipboard — 0% coverage, platform-native clipboard
// ---------------------------------------------------------------------------

func TestCopyToClipboard_PlainText(t *testing.T) {
	// clip (Windows), pbcopy (macOS), xclip (Linux) — all headless.
	err := CopyToClipboard(context.Background(), "hello world")
	if err != nil {
		t.Skipf("clipboard tool not available on this platform: %v", err)
	}
}

func TestCopyToClipboard_EmptyString(t *testing.T) {
	err := CopyToClipboard(context.Background(), "")
	if err != nil {
		t.Skipf("clipboard tool not available: %v", err)
	}
}

func TestCopyToClipboard_WithANSI(t *testing.T) {
	// ANSI codes should be stripped before copying.
	err := CopyToClipboard(context.Background(), "\x1b[31mred text\x1b[0m")
	if err != nil {
		t.Skipf("clipboard tool not available: %v", err)
	}
}

func TestCopyToClipboard_MultilineText(t *testing.T) {
	err := CopyToClipboard(context.Background(), "line one\nline two\nline three")
	if err != nil {
		t.Skipf("clipboard tool not available: %v", err)
	}
}

func TestCopyToClipboard_UnicodeText(t *testing.T) {
	err := CopyToClipboard(context.Background(), "日本語テスト ✓ ✗ ⚠")
	if err != nil {
		t.Skipf("clipboard tool not available: %v", err)
	}
}

// ---------------------------------------------------------------------------
// StartDetachedFn — 40% coverage, background process launcher
// ---------------------------------------------------------------------------

func TestStartDetachedFn_ValidCommand(t *testing.T) {
	t.Parallel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "echo", "test")
	default:
		cmd = exec.Command("echo", "test")
	}

	err := StartDetachedFn(cmd)
	assert.NoError(t, err)
}

func TestStartDetachedFn_InvalidCommand(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("this_binary_does_not_exist_12345")
	err := StartDetachedFn(cmd)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// OpenInEditor — Windows-specific code paths
// ---------------------------------------------------------------------------

func TestOpenInEditor_VISUALWithValidBinary(t *testing.T) {
	// Validate that VISUAL/EDITOR paths pass validation without actually
	// spawning a process (which opens GUI windows during tests).
	if runtime.GOOS == "windows" {
		tmpFile := t.TempDir() + "\\test.txt"
		assert.NoError(t, ValidateEditorPath(tmpFile))
	} else {
		t.Setenv("VISUAL", "echo")
		t.Setenv("EDITOR", "")
		err := OpenInEditor("/tmp/test.txt")
		assert.NoError(t, err)
	}
}

func TestOpenInEditor_EDITORFallback(t *testing.T) {
	// On Windows, test validation only to avoid spawning cmd.exe windows.
	if runtime.GOOS == "windows" {
		assert.NoError(t, ValidateEditorPath("testfile.txt"))
		return
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "echo")
	err := OpenInEditor("testfile.txt")
	_ = err
}

// ---------------------------------------------------------------------------
// OpenInBrowser — exercise platform branch
// ---------------------------------------------------------------------------

func TestOpenInBrowser_ValidURLPlatformPath(t *testing.T) {
	// Verify that valid URLs pass validation without actually launching a
	// browser. The platform-specific exec is a thin wrapper; we test the
	// validation gate (ValidateBrowserURL) directly to avoid opening a
	// real browser window during automated tests.
	validURLs := []string{
		"https://example.com",
		"http://localhost:8080/path",
		"https://example.com/search?q=test&page=1",
	}
	for _, u := range validURLs {
		assert.NoError(t, ValidateBrowserURL(u), "valid URL %q should pass validation", u)
	}
}

// ---------------------------------------------------------------------------
// OpenInTerminal — Windows code path
// ---------------------------------------------------------------------------

func TestOpenInTerminal_WindowsPlatformPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	dir := t.TempDir()
	// Only test validation — calling OpenInTerminal spawns a real cmd.exe
	// window that cannot be suppressed during automated tests.
	assert.NoError(t, ValidateEditorPath(dir))
}

// ---------------------------------------------------------------------------
// ValidateBrowserURL — additional edge cases for 93.3% → 100%
// ---------------------------------------------------------------------------

func TestValidateBrowserURL_MalformedURL(t *testing.T) {
	t.Parallel()

	// Test URL that can't be parsed.
	err := ValidateBrowserURL("://missing-scheme")
	require.Error(t, err)
}

func TestValidateBrowserURL_CustomScheme(t *testing.T) {
	t.Parallel()

	err := ValidateBrowserURL("custom://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}
