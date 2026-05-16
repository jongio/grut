package panels

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenInEditorUsesVISUAL(t *testing.T) {
	// Set VISUAL to a non-existent binary — Start() should return an
	// exec.Error because the binary cannot be found.
	t.Setenv("VISUAL", "this_editor_does_not_exist_67890")
	t.Setenv("EDITOR", "")

	err := OpenInEditor(context.Background(), "somefile.txt")
	if err == nil {
		t.Fatal("expected error when VISUAL is set to non-existent binary")
	}
}

func TestOpenInEditorUsesEDITOR(t *testing.T) {
	// With VISUAL unset, EDITOR should be used.
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "this_editor_does_not_exist_67890")

	err := OpenInEditor(context.Background(), "somefile.txt")
	if err == nil {
		t.Fatal("expected error when EDITOR is set to non-existent binary")
	}
}

func TestOpenInEditorFallsThroughToDefault(t *testing.T) {
	// On Windows the fallthrough uses "cmd /c start" which opens a GUI
	// dialog for unknown file types — skip to avoid interactive popups
	// during automated tests. Validation coverage is provided by the
	// other tests in this file.
	if runtime.GOOS == "windows" {
		t.Skip("skipping: platform default opens GUI dialog on Windows")
	}

	// With both VISUAL and EDITOR unset, the platform default is used.
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	f, err := os.CreateTemp(t.TempDir(), "open-test-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	err = OpenInEditor(context.Background(), f.Name())
	if err != nil {
		t.Logf("OpenInEditor returned error with platform default (may be expected in headless CI): %v", err)
	}
}

func TestOpenInBrowserDoesNotPanic(t *testing.T) {
	// OpenInBrowser should reject an empty URL via validation.
	err := OpenInBrowser(context.Background(), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

// ---------------------------------------------------------------------------
// Security: ValidateEditorPath
// ---------------------------------------------------------------------------

func TestValidateEditorPath_ValidPaths(t *testing.T) {
	valid := []string{
		"file.txt",
		"src/main.go",
		"path/to/deep/file.rs",
		"my file.txt",
		`C:\Users\dev\file.txt`,
		"file-with-dashes.go",
		"file_with_underscores.go",
		"日本語ファイル.txt",
	}
	for _, p := range valid {
		t.Run(p, func(t *testing.T) {
			assert.NoError(t, ValidateEditorPath(p))
		})
	}
}

func TestValidateEditorPath_RejectsEmpty(t *testing.T) {
	err := ValidateEditorPath("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidateEditorPath_RejectsNullByte(t *testing.T) {
	err := ValidateEditorPath("file\x00.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null byte")
}

func TestValidateEditorPath_RejectsShellMetachars(t *testing.T) {
	dangerous := []struct {
		name string
		path string
	}{
		{"semicolon", "file;rm -rf /"},
		{"pipe", "file|cat /etc/passwd"},
		{"ampersand", "file & del *"},
		{"dollar", "file$HOME"},
		{"backtick", "file`whoami`"},
		{"less-than", "file<input"},
		{"greater-than", "file>output"},
		{"newline", "file\nmalicious"},
		{"carriage-return", "file\rmalicious"},
		{"double-quote", `file"inject`},
		{"single-quote", "file'inject"},
		{"percent-env-expand", "%USERPROFILE%"},
		{"caret-cmd-escape", "^inject"},
		{"exclamation-delayed-expand", "!var!"},
	}
	for _, tt := range dangerous {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEditorPath(tt.path)
			require.Error(t, err, "path %q should be rejected", tt.path)
			assert.Contains(t, err.Error(), "forbidden character")
		})
	}
}

func TestOpenInEditor_RejectsMetacharPath(t *testing.T) {
	// Ensure the full OpenInEditor function rejects dangerous paths
	// before any command is launched.
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	err := OpenInEditor(context.Background(), "file & del *")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden character")
}

func TestOpenInEditor_RejectsEmptyPath(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	err := OpenInEditor(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

// ---------------------------------------------------------------------------
// Security: ValidateBrowserURL
// ---------------------------------------------------------------------------

func TestValidateBrowserURL_ValidURLs(t *testing.T) {
	valid := []string{
		"https://github.com",
		"http://localhost:8080",
		"https://example.com/path?q=1&r=2",
		"https://example.com/path#fragment",
	}
	for _, u := range valid {
		t.Run(u, func(t *testing.T) {
			assert.NoError(t, ValidateBrowserURL(u))
		})
	}
}

func TestValidateBrowserURL_RejectsEmpty(t *testing.T) {
	err := ValidateBrowserURL("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidateBrowserURL_RejectsNullByte(t *testing.T) {
	err := ValidateBrowserURL("https://example.com/\x00evil")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null byte")
}

func TestValidateBrowserURL_RejectsJavascript(t *testing.T) {
	err := ValidateBrowserURL("javascript:alert(1)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateBrowserURL_RejectsDataURI(t *testing.T) {
	err := ValidateBrowserURL("data:text/html,<script>alert(1)</script>")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateBrowserURL_RejectsFileScheme(t *testing.T) {
	err := ValidateBrowserURL("file:///etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateBrowserURL_RejectsVbscript(t *testing.T) {
	err := ValidateBrowserURL("vbscript:MsgBox(1)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateBrowserURL_RejectsCredentials(t *testing.T) {
	urls := []string{
		"https://user:pass@example.com",
		"https://user:pass@evil.com/path",
		"http://admin:secret@localhost:8080",
		"https://ghp_token123@github.com/owner/repo.git",
	}
	for _, u := range urls {
		t.Run(u, func(t *testing.T) {
			err := ValidateBrowserURL(u)
			require.Error(t, err, "URL %q with credentials should be rejected", u)
			assert.Contains(t, err.Error(), "credentials")
		})
	}
}

func TestValidateBrowserURL_RejectsNoScheme(t *testing.T) {
	err := ValidateBrowserURL("example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no scheme")
}

func TestValidateBrowserURL_RejectsUnknownScheme(t *testing.T) {
	err := ValidateBrowserURL("ftp://example.com/file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateBrowserURL_CaseInsensitiveScheme(t *testing.T) {
	// Ensure scheme check is case-insensitive.
	err := ValidateBrowserURL("JAVASCRIPT:alert(1)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	err = ValidateBrowserURL("FILE:///etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestOpenInBrowser_RejectsDangerousURL(t *testing.T) {
	// Ensure the full OpenInBrowser function rejects dangerous URLs
	// before any command is launched.
	err := OpenInBrowser(context.Background(), "javascript:alert(document.cookie)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestOpenInBrowser_RejectsFileURL(t *testing.T) {
	err := OpenInBrowser(context.Background(), "file:///etc/shadow")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// ---------------------------------------------------------------------------
// Security: OpenInTerminal
// ---------------------------------------------------------------------------

func TestOpenInTerminal_RejectsEmpty(t *testing.T) {
	err := OpenInTerminal(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestOpenInTerminal_RejectsNullByte(t *testing.T) {
	err := OpenInTerminal(context.Background(), "/tmp/dir\x00malicious")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null byte")
}

func TestOpenInTerminal_RejectsShellMetachars(t *testing.T) {
	dangerous := []struct {
		name string
		dir  string
	}{
		{"semicolon", "/tmp/dir;rm -rf /"},
		{"pipe", "/tmp/dir|cat /etc/passwd"},
		{"ampersand", "/tmp/dir & del *"},
		{"dollar", "/tmp/$HOME"},
		{"backtick", "/tmp/`whoami`"},
	}
	for _, tt := range dangerous {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenInTerminal(context.Background(), tt.dir)
			require.Error(t, err, "dir %q should be rejected", tt.dir)
			assert.Contains(t, err.Error(), "forbidden character")
		})
	}
}

func TestOpenInTerminal_AcceptsValidDirectory(t *testing.T) {
	// On Windows this spawns a real terminal window — skip to avoid
	// GUI side-effects during automated tests.
	if runtime.GOOS == "windows" {
		t.Skip("skipping: spawns real terminal window on Windows")
	}

	dir := t.TempDir()
	err := OpenInTerminal(context.Background(), dir)
	if err != nil {
		assert.NotContains(t, err.Error(), "forbidden character")
		assert.NotContains(t, err.Error(), "must not be empty")
		assert.NotContains(t, err.Error(), "null byte")
	}
}
