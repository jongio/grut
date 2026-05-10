package panels

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// shellMetachars contains characters that could be interpreted by cmd.exe or
// POSIX shells when a path is passed through platform launchers.
const shellMetachars = ";|&$`<>\"'\n\r"

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

// StartDetachedFn starts a command and reaps it in the background to prevent
// zombie processes. The caller does not wait for the process to exit.
// It is a variable so tests can replace it with a no-op stub.
var StartDetachedFn = func(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached process: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// OpenInEditor opens a file in the user's preferred editor.
// It checks $VISUAL, $EDITOR, then tries common developer editors,
// and finally falls back to platform defaults.
func OpenInEditor(path string) error {
	if err := ValidateEditorPath(path); err != nil {
		return err
	}
	// Check VISUAL first (GUI editor preference).
	if editor := os.Getenv("VISUAL"); editor != "" {
		return StartDetachedFn(exec.CommandContext(context.Background(), editor, path))
	}
	// Check EDITOR (terminal editor preference).
	if editor := os.Getenv("EDITOR"); editor != "" {
		return StartDetachedFn(exec.CommandContext(context.Background(), editor, path))
	}
	// Try common developer editors in preference order.
	for _, editor := range []string{"code", "code-insiders", "cursor"} {
		if _, err := exec.LookPath(editor); err == nil {
			return StartDetachedFn(exec.CommandContext(context.Background(), editor, path))
		}
	}
	// Platform defaults.
	switch runtime.GOOS {
	case "windows":
		return StartDetachedFn(exec.CommandContext(context.Background(), "cmd", "/c", "start", "", path))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(context.Background(), "open", path))
	default:
		return StartDetachedFn(exec.CommandContext(context.Background(), "xdg-open", path))
	}
}

// OpenInBrowser opens a URL in the default browser.
func OpenInBrowser(rawURL string) error {
	if err := ValidateBrowserURL(rawURL); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return StartDetachedFn(exec.CommandContext(context.Background(), "rundll32", "url.dll,FileProtocolHandler", rawURL))
	case "darwin":
		return StartDetachedFn(exec.CommandContext(context.Background(), "open", rawURL))
	default:
		return StartDetachedFn(exec.CommandContext(context.Background(), "xdg-open", rawURL))
	}
}

// OpenInTerminal spawns a new terminal window at the given directory path.
func OpenInTerminal(dir string) error {
	if err := ValidateEditorPath(dir); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(context.Background(), "cmd", "/c", "start", "cmd", "/k", "cd", "/d", dir)
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "open", "-a", "Terminal", dir)
	default: // linux/freebsd
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "xterm"} {
			if _, err := exec.LookPath(term); err == nil {
				if term == "gnome-terminal" {
					cmd = exec.CommandContext(context.Background(), term, "--working-directory="+dir)
				} else {
					// Use sh -c with proper argument separation to avoid
					// shell injection via directory names containing metacharacters.
					cmd = exec.CommandContext(context.Background(), "sh", "-c", `cd "$1" && exec "$SHELL"`, "sh", dir)
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
