package panels

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// shellMetachars contains characters that could be interpreted by cmd.exe or
// POSIX shells when a path is passed through platform launchers.
const shellMetachars = ";|&$`<>\"'\n\r%^!"

// ValidateEditorPath validates a file path before passing it to an external
// editor. It rejects null bytes, shell metacharacters, and empty paths.
func ValidateEditorPath(path string) error {
	if path == "" {
		return fmt.Errorf("editor path must not be empty")
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("editor path contains null byte")
	}
	if strings.ContainsAny(path, shellMetachars) {
		return fmt.Errorf("editor path contains forbidden character")
	}
	return nil
}

// dangerousSchemes lists URL schemes that must not be opened in a browser.
var dangerousSchemes = map[string]bool{
	"javascript": true,
	"data":       true,
	"vbscript":   true,
	"file":       true,
}

// ValidateBrowserURL validates a URL before opening it in the default browser.
// Only http and https schemes are allowed.
func ValidateBrowserURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("browser URL must not be empty")
	}
	if strings.ContainsRune(rawURL, 0) {
		return fmt.Errorf("browser URL contains null byte")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browser URL is malformed: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return fmt.Errorf("browser URL has no scheme (expected http or https)")
	}
	if dangerousSchemes[scheme] {
		return fmt.Errorf("browser URL scheme %q is not allowed", scheme)
	}
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("browser URL scheme %q is not allowed (expected http or https)", scheme)
	}
	// Reject credential-embedded URLs (e.g. https://user:pass@evil.com).
	if parsed.User != nil {
		return fmt.Errorf("browser URL must not contain credentials")
	}
	return nil
}

// detachedProcessTimeout bounds how long the reaper goroutine waits for a
// detached process to exit. Detached processes (editors, browsers, terminals)
// are expected to outlive this timeout; it exists only to prevent goroutine
// leaks from processes that hang indefinitely.
const detachedProcessTimeout = 30 * time.Second

// StartDetachedFn starts a command and reaps it in the background to prevent
// zombie processes. The caller does not wait for the process to exit.
// It is a variable so tests can replace it with a no-op stub.
// The reaper goroutine is bounded by detachedProcessTimeout to prevent leaks.
var StartDetachedFn = func(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached process: %w", err)
	}
	go func() {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(detachedProcessTimeout):
		}
	}()
	return nil
}

// OpenInDefaultApp opens a file using the OS-registered default application
// for its file type. This is the "file manager" behavior: .pdf opens in a PDF
// viewer, .png opens in an image viewer, etc.
func OpenInDefaultApp(ctx context.Context, path string) error {
	if err := ValidateEditorPath(path); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return StartDetachedFn(exec.CommandContext(ctx, "cmd", "/c", "start", "", path))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(ctx, "open", path))
	default:
		return StartDetachedFn(exec.CommandContext(ctx, "xdg-open", path))
	}
}

// OpenInEditor opens a file in the user's preferred editor.
// It checks $VISUAL, $EDITOR, then tries common developer editors,
// and finally falls back to platform defaults.
func OpenInEditor(ctx context.Context, path string) error {
	if err := ValidateEditorPath(path); err != nil {
		return err
	}
	// Check VISUAL first (GUI editor preference).
	if editor := os.Getenv("VISUAL"); editor != "" {
		return StartDetachedFn(exec.CommandContext(ctx, editor, path))
	}
	// Check EDITOR (terminal editor preference).
	if editor := os.Getenv("EDITOR"); editor != "" {
		return StartDetachedFn(exec.CommandContext(ctx, editor, path))
	}
	// Try common developer editors in preference order.
	for _, editor := range []string{"code", "code-insiders", "cursor"} {
		if _, err := exec.LookPath(editor); err == nil {
			return StartDetachedFn(exec.CommandContext(ctx, editor, path))
		}
	}
	// Platform defaults.
	switch runtime.GOOS {
	case "windows":
		return StartDetachedFn(exec.CommandContext(ctx, "cmd", "/c", "start", "", path))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(ctx, "open", path))
	default:
		return StartDetachedFn(exec.CommandContext(ctx, "xdg-open", path))
	}
}

// OpenInBrowser opens a URL in the default browser.
func OpenInBrowser(ctx context.Context, rawURL string) error {
	if err := ValidateBrowserURL(rawURL); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return StartDetachedFn(exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", rawURL))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(ctx, "open", rawURL))
	default:
		return StartDetachedFn(exec.CommandContext(ctx, "xdg-open", rawURL))
	}
}

// OpenInTerminal spawns a new terminal window at the given directory path.
func OpenInTerminal(ctx context.Context, dir string) error {
	if err := ValidateEditorPath(dir); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", "cmd", "/k", "cd", "/d", dir)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", "-a", "Terminal", dir)
	default: // linux/freebsd
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "xterm"} {
			if _, err := exec.LookPath(term); err == nil {
				if term == "gnome-terminal" {
					cmd = exec.CommandContext(ctx, term, "--working-directory="+dir)
				} else {
					// Use sh -c with proper argument separation to avoid
					// shell injection via directory names containing metacharacters.
					cmd = exec.CommandContext(ctx, "sh", "-c", `cd "$1" && exec "$SHELL"`, "sh", dir)
				}
				break
			}
		}
	}
	if cmd == nil {
		return fmt.Errorf("no terminal emulator found")
	}
	return StartDetachedFn(cmd)
}

// RevealInFileManager opens the OS file manager with the given path selected
// (highlighted) inside its parent directory. On Windows it uses explorer
// /select, on macOS it uses open -R, and on Linux it uses the freedesktop
// D-Bus FileManager1.ShowItems interface when available, otherwise it opens the
// parent directory with xdg-open.
func RevealInFileManager(ctx context.Context, path string) error {
	if err := ValidateEditorPath(path); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		// explorer highlights the item when passed /select,<path>. It exits
		// with a non-zero status even on success, but StartDetachedFn ignores
		// the reaped exit status so that does not surface as an error.
		return StartDetachedFn(exec.CommandContext(ctx, "explorer", "/select,"+path))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(ctx, "open", "-R", path))
	default:
		if _, err := exec.LookPath("dbus-send"); err == nil {
			fileURI := (&url.URL{Scheme: "file", Path: path}).String()
			return StartDetachedFn(exec.CommandContext(
				ctx, "dbus-send",
				"--session",
				"--dest=org.freedesktop.FileManager1",
				"--type=method_call",
				"/org/freedesktop/FileManager1",
				"org.freedesktop.FileManager1.ShowItems",
				"array:string:"+fileURI,
				"string:",
			))
		}
		return StartDetachedFn(exec.CommandContext(ctx, "xdg-open", filepath.Dir(path)))
	}
}
