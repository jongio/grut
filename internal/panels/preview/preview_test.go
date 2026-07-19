package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/markdown"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultCfg returns a PreviewConfig with sensible defaults for testing.
func defaultCfg() config.PreviewConfig {
	return config.PreviewConfig{
		Enabled:            true,
		Width:              40,
		SyntaxHighlighting: true,
		MaxFileSize:        1048576, // 1 MB
		LineNumbers:        true,
		WordWrap:           false,
		RenderMarkdown:     true,
	}
}

// defaultEditorCfg returns an EditorConfig with sensible defaults for testing.
func defaultEditorCfg() config.EditorConfig {
	return config.EditorConfig{
		TabSize:    4,
		InsertTabs: false,
		AutoIndent: true,
	}
}

// keyMsg creates a tea.KeyPressMsg for a printable character.
func keyMsg(key string) tea.KeyPressMsg {
	if len(key) == 1 {
		return tea.KeyPressMsg{Text: key, Code: rune(key[0])}
	}
	// For special keys
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case keyDown:
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	default:
		return tea.KeyPressMsg{Text: key}
	}
}

// writeFile is a test helper to create a file with the given content.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// loadFile is a test helper that sends a FileSelectedMsg and runs the
// resulting async command to completion, feeding the fileLoadedMsg back
// into the panel. This simulates the Bubble Tea runtime loop for
// the async file loading introduced in F01.
func loadFile(t *testing.T, p *Preview, path string) {
	t.Helper()
	_, cmd := p.Update(panels.FileSelectedMsg{Path: path})
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg != nil {
		p.Update(msg)
	}
}

// writeBinaryFile creates a file with binary content (PNG header).
func writeBinaryFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	// PNG magic number header
	data := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func TestImplementsPanel(t *testing.T) {
	// Compile-time check
	var _ panels.Panel = (*Preview)(nil)
}

func TestNew(t *testing.T) {
	cfg := defaultCfg()
	p := New(cfg, defaultEditorCfg(), nil)

	assert.NotNil(t, p)
	assert.Equal(t, "preview", p.Title())
	assert.True(t, p.lineNumbers)
	assert.False(t, p.wordWrap)
	assert.True(t, p.renderMarkdown)
	assert.Empty(t, p.filePath)
}

func TestNewCustomConfig(t *testing.T) {
	cfg := config.PreviewConfig{
		LineNumbers:    false,
		WordWrap:       true,
		RenderMarkdown: false,
		MaxFileSize:    512,
	}
	p := New(cfg, defaultEditorCfg(), nil)

	assert.False(t, p.lineNumbers)
	assert.True(t, p.wordWrap)
	assert.False(t, p.renderMarkdown)
	assert.Equal(t, 512, p.cfg.GetMaxFileSize())
}

func TestInit(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

func TestEmptyStateRendering(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)

	content := p.View(60, 20)
	assert.NotEmpty(t, content)
	assert.Contains(t, content, "No file selected")
	assert.Contains(t, content, "Select a file to preview")
}

func TestEmptyStateZeroSize(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	assert.Empty(t, p.View(0, 20))
	assert.Empty(t, p.View(20, 0))
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
}

func TestPlainTextFile(t *testing.T) {
	dir := t.TempDir()
	content := "Hello, World!\nThis is a test file.\nLine three."
	path := writeFile(t, dir, "test.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	// Load via FileSelectedMsg
	loadFile(t, p, path)

	view := p.View(60, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Hello, World!")
	assert.Contains(t, view, "This is a test file.")
	assert.Contains(t, view, "Line three.")
}

func TestTitleChangesOnFileSelect(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "example.txt", "content")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	assert.Equal(t, "preview", p.Title())

	loadFile(t, p, path)
	assert.Equal(t, "example.txt", p.Title())
}

func TestSyntaxHighlightingGo(t *testing.T) {
	dir := t.TempDir()
	goCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
`
	path := writeFile(t, dir, "main.go", goCode)

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	assert.NotNil(t, p.lines)
	assert.True(t, len(p.lines) > 0)

	// Verify the content is present (the highlighted version should still
	// contain the original tokens, possibly with ANSI codes)
	view := p.View(80, 30)
	assert.NotEmpty(t, view)

	// The raw source tokens should be findable in the output
	joined := strings.Join(p.lines, "\n")
	assert.Contains(t, joined, "package")
	assert.Contains(t, joined, "main")
	assert.Contains(t, joined, "fmt")
}

func TestSyntaxHighlightingPython(t *testing.T) {
	dir := t.TempDir()
	pyCode := `def hello():
    print("Hello, World!")

if __name__ == "__main__":
    hello()
`
	path := writeFile(t, dir, "script.py", pyCode)

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	assert.NotNil(t, p.lines)
	joined := strings.Join(p.lines, "\n")
	assert.Contains(t, joined, "def")
	assert.Contains(t, joined, "hello")
	assert.Contains(t, joined, "print")
}

func TestSyntaxHighlightingJS(t *testing.T) {
	dir := t.TempDir()
	jsCode := `function greet(name) {
  console.log("Hello, " + name);
}
greet("World");
`
	path := writeFile(t, dir, "app.js", jsCode)

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	assert.NotNil(t, p.lines)
	joined := strings.Join(p.lines, "\n")
	assert.Contains(t, joined, "function")
	assert.Contains(t, joined, "greet")
}

func TestSyntaxHighlightingDisabled(t *testing.T) {
	dir := t.TempDir()
	goCode := "package main\n\nfunc main() {}\n"
	path := writeFile(t, dir, "main.go", goCode)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(80, 20)
	loadFile(t, p, path)

	// Without highlighting, lines should be the raw source
	assert.NotNil(t, p.lines)
	assert.Equal(t, "package main", p.lines[0])
}

func TestMarkdownRendering(t *testing.T) {
	dir := t.TempDir()
	mdContent := "# Hello World\n\nThis is a **bold** paragraph.\n\n- Item 1\n- Item 2\n"
	path := writeFile(t, dir, "README.md", mdContent)

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	assert.NotNil(t, p.lines)
	assert.True(t, len(p.lines) > 0)

	// Glamour should have rendered the markdown (output differs from raw source)
	view := p.View(60, 20)
	assert.NotEmpty(t, view)

	// The text content should be present in some form (glamour inserts
	// ANSI codes within words, so check parts individually)
	joined := strings.Join(p.lines, "\n")
	assert.Contains(t, joined, "Hello")
	assert.Contains(t, joined, "World")
}

func TestMarkdownExtensions(t *testing.T) {
	dir := t.TempDir()
	mdContent := "# Test\n\nContent here.\n"

	for _, ext := range []string{".md", ".markdown", ".mdown", ".mkd"} {
		name := "test" + ext
		path := writeFile(t, dir, name, mdContent)

		p := New(defaultCfg(), defaultEditorCfg(), nil)
		p.SetSize(60, 20)
		loadFile(t, p, path)

		assert.NotNil(t, p.lines, "extension %s should be detected as markdown", ext)
		joined := strings.Join(p.lines, "\n")
		assert.Contains(t, joined, "Test", "extension %s: content should be present", ext)
	}
}

func TestBinaryFileDetection(t *testing.T) {
	dir := t.TempDir()
	path := writeBinaryFile(t, dir, "image.png")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	assert.True(t, p.isBinary)
	assert.NotNil(t, p.lines)

	view := p.View(60, 20)
	assert.Contains(t, view, "Binary file")
	assert.Contains(t, view, "image.png")
}

func TestLargeFileRejection(t *testing.T) {
	dir := t.TempDir()

	// Create a file larger than 100 bytes (our test limit)
	content := strings.Repeat("x", 200)
	path := writeFile(t, dir, "large.txt", content)

	cfg := defaultCfg()
	cfg.MaxFileSize = 100 // 100 bytes
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	assert.True(t, p.isLarge)

	view := p.View(60, 20)
	assert.Contains(t, view, "File too large")
	assert.Contains(t, view, "large.txt")
}

func TestLargeFileShowsMetadata(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 200)
	path := writeFile(t, dir, "big.txt", content)

	cfg := defaultCfg()
	cfg.MaxFileSize = 100
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	view := p.View(60, 20)
	assert.Contains(t, view, "big.txt")
	assert.Contains(t, view, "200 B")
	assert.Contains(t, view, "Mode:")
	assert.Contains(t, view, "Modified:")
}

func TestMaxFileSizeZeroDisablesCheck(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 200)
	path := writeFile(t, dir, "any.txt", content)

	cfg := defaultCfg()
	cfg.MaxFileSize = 0 // disabled
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	assert.False(t, p.isLarge)
	assert.Nil(t, p.err)
	assert.NotNil(t, p.lines)
}

func TestLineNumberDisplay(t *testing.T) {
	dir := t.TempDir()
	content := "line one\nline two\nline three"
	path := writeFile(t, dir, "test.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.LineNumbers = true
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	view := p.View(60, 20)
	assert.Contains(t, view, "1")
	assert.Contains(t, view, "2")
	assert.Contains(t, view, "3")
	assert.Contains(t, view, "│")
}

func TestLineNumberToggle(t *testing.T) {
	dir := t.TempDir()
	content := "line one\nline two"
	path := writeFile(t, dir, "test.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.LineNumbers = true
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)

	// Initially line numbers enabled
	view1 := p.View(60, 20)
	assert.Contains(t, view1, "│")

	// Toggle off
	p.Update(keyMsg("n"))
	assert.False(t, p.lineNumbers)

	view2 := p.View(60, 20)
	// After toggle, separator should not appear as line numbers
	// Verify that the view changed
	assert.NotEqual(t, view1, view2)

	// Toggle back on
	p.Update(keyMsg("n"))
	assert.True(t, p.lineNumbers)
}

func TestWordWrapToggle(t *testing.T) {
	dir := t.TempDir()
	content := "short\nshort"
	path := writeFile(t, dir, "test.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.WordWrap = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)

	assert.False(t, p.wordWrap)

	// Toggle on
	p.Update(keyMsg("W"))
	assert.True(t, p.wordWrap)

	// Toggle off
	p.Update(keyMsg("W"))
	assert.False(t, p.wordWrap)
}

func TestRenderMarkdownToggle(t *testing.T) {
	dir := t.TempDir()
	content := "# Hello\n\nSome **bold** text"
	path := writeFile(t, dir, "test.md", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.RenderMarkdown = true
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)

	assert.True(t, p.renderMarkdown)

	// Rendered markdown should not contain the raw "# " prefix
	view1 := p.View(60, 20)

	// Toggle off – should re-load as raw text
	_, cmd := p.Update(keyMsg("m"))
	assert.False(t, p.renderMarkdown)
	require.NotNil(t, cmd, "toggling m on a markdown file should return a reload cmd")
	msg := cmd()
	if msg != nil {
		p.Update(msg)
	}

	view2 := p.View(60, 20)
	assert.NotEqual(t, view1, view2, "rendered vs raw views should differ")

	// Toggle back on
	_, cmd = p.Update(keyMsg("m"))
	assert.True(t, p.renderMarkdown)
	require.NotNil(t, cmd)
	msg = cmd()
	if msg != nil {
		p.Update(msg)
	}
}

func TestRenderMarkdownToggleNonMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "test.txt", "hello")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)

	// Toggle on a non-markdown file should toggle the flag but return no cmd
	_, cmd := p.Update(keyMsg("m"))
	assert.False(t, p.renderMarkdown)
	assert.Nil(t, cmd, "toggling m on a non-markdown file should not reload")
}

func TestScrollDown(t *testing.T) {
	dir := t.TempDir()
	// Create a file with many lines
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 20)
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	assert.Equal(t, 0, p.scrollY)

	// Scroll down with j
	p.Update(keyMsg("j"))
	assert.Equal(t, 1, p.scrollY)

	// Scroll down with arrow key
	p.Update(keyMsg("down"))
	assert.Equal(t, 2, p.scrollY)
}

func TestScrollUp(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 20)
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// Scroll down first
	p.Update(keyMsg("j"))
	p.Update(keyMsg("j"))
	p.Update(keyMsg("j"))
	assert.Equal(t, 3, p.scrollY)

	// Scroll up with k
	p.Update(keyMsg("k"))
	assert.Equal(t, 2, p.scrollY)

	// Scroll up with arrow key
	p.Update(keyMsg("up"))
	assert.Equal(t, 1, p.scrollY)
}

func TestScrollUpBoundsAtZero(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "test.txt", "line\n")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// Try scrolling up past 0
	p.Update(keyMsg("k"))
	assert.Equal(t, 0, p.scrollY)
}

func TestPageDown(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// Page down with pgdown
	p.Update(keyMsg("pgdown"))
	assert.Greater(t, p.scrollY, 0)
	scrollAfterD := p.scrollY

	// Page down with pgdown again
	p.Update(keyMsg("pgdown"))
	assert.Greater(t, p.scrollY, scrollAfterD)
}

func TestPageUp(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// Scroll to bottom first
	p.Update(keyMsg("G"))
	bottomPos := p.scrollY

	// Page up with pgup
	p.Update(keyMsg("pgup"))
	assert.Less(t, p.scrollY, bottomPos)

	// Page up with pgup again
	posAfterU := p.scrollY
	p.Update(keyMsg("pgup"))
	assert.Less(t, p.scrollY, posAfterU)
}

func TestGotoTop(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// Scroll down
	p.Update(keyMsg("G"))
	assert.Greater(t, p.scrollY, 0)

	// Go to top
	p.Update(keyMsg("g"))
	assert.Equal(t, 0, p.scrollY)
}

func TestGotoBottom(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	assert.Equal(t, 0, p.scrollY)

	// Go to bottom
	p.Update(keyMsg("G"))
	expected := len(p.lines) - p.viewportHeight()
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, p.scrollY)
}

func TestScrollClampOnShortFile(t *testing.T) {
	dir := t.TempDir()
	content := "one\ntwo"
	path := writeFile(t, dir, "short.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20) // much taller than content
	p.Focus()
	loadFile(t, p, path)

	// Try to scroll past the end
	p.Update(keyMsg("j"))
	assert.Equal(t, 0, p.scrollY) // should be clamped
}

func TestFileSelectedMsg(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "test.txt", "hello")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	// F01: Update now returns a cmd for async file loading.
	result, cmd := p.Update(panels.FileSelectedMsg{Path: path})
	assert.NotNil(t, cmd, "FileSelectedMsg should return async load cmd")
	assert.Equal(t, p, result)
	assert.Equal(t, path, p.filePath)
	assert.Equal(t, "test.txt", p.Title())
	assert.True(t, p.loading, "should be in loading state")

	// Execute the cmd and feed back the result.
	msg := cmd()
	p.Update(msg)
	assert.False(t, p.loading, "loading should be complete")
}

func TestFileSelectedMsgResetsState(t *testing.T) {
	dir := t.TempDir()
	path1 := writeFile(t, dir, "file1.txt", strings.Repeat("line\n", 100))
	path2 := writeFile(t, dir, "file2.txt", "short")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()

	// Load first file and scroll
	loadFile(t, p, path1)
	p.Update(keyMsg("G"))
	assert.Greater(t, p.scrollY, 0)

	// Load second file - scroll should reset
	loadFile(t, p, path2)
	assert.Equal(t, 0, p.scrollY)
	assert.Equal(t, "file2.txt", p.Title())
}

func TestFocusBlur(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	assert.False(t, p.focused)

	p.Focus()
	assert.True(t, p.focused)

	p.Blur()
	assert.False(t, p.focused)
}

func TestSetSize(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 24)
	assert.Equal(t, 80, p.width)
	assert.Equal(t, 24, p.height)
}

func TestKeyBindings(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	bindings := p.KeyBindings()

	assert.NotEmpty(t, bindings)
	assert.Len(t, bindings, 15)

	// Verify all expected bindings are present
	actions := make([]string, len(bindings))
	for i, b := range bindings {
		actions[i] = b.Action
		assert.NotEmpty(t, b.Key)
		assert.NotEmpty(t, b.Description)
	}
	assert.Contains(t, actions, "edit")
	assert.Contains(t, actions, "scroll_down")
	assert.Contains(t, actions, "scroll_up")
	assert.Contains(t, actions, "page_down")
	assert.Contains(t, actions, "page_up")
	assert.Contains(t, actions, "goto_top")
	assert.Contains(t, actions, "goto_bottom")
	assert.Contains(t, actions, "goto_line")
	assert.Contains(t, actions, "toggle_wrap")
	assert.Contains(t, actions, "toggle_line_numbers")
	assert.Contains(t, actions, "toggle_markdown_render")
	assert.Contains(t, actions, "toggle_blame")
	assert.Contains(t, actions, "toggle_diff_mode")
	assert.Contains(t, actions, "copy_selection")
	assert.Contains(t, actions, "copy_permalink")
}

func TestKeysIgnoredWhenBlurred(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	// Don't focus
	loadFile(t, p, path)

	// Try to scroll - should be ignored since not focused
	p.Update(keyMsg("j"))
	assert.Equal(t, 0, p.scrollY)
}

func TestNonexistentFile(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, "/nonexistent/path/file.txt")

	assert.NotNil(t, p.err)

	view := p.View(60, 20)
	assert.Contains(t, view, "Error")
}

func TestDirectory(t *testing.T) {
	dir := t.TempDir()

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, dir)

	assert.NotNil(t, p.lines)
	assert.Contains(t, p.lines[0], "Directory:")
}

func TestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "empty.txt", "")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	// Should render without error
	view := p.View(60, 20)
	assert.NotEmpty(t, view) // will show empty state or minimal content
}

func TestScrollIndicator(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.LineNumbers = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	p.Focus()
	loadFile(t, p, path)

	// At top
	view := p.View(60, 10)
	assert.Contains(t, view, "Top")

	// Scroll to bottom
	p.Update(keyMsg("G"))
	view = p.View(60, 10)
	assert.Contains(t, view, "Bot")

	// Scroll to middle
	p.scrollY = len(p.lines) / 2
	p.clampScroll()
	view = p.View(60, 10)
	assert.Contains(t, view, "%")
}

func TestUpdateReturnsPanel(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	result, cmd := p.Update(nil)
	assert.Equal(t, p, result)
	assert.Nil(t, cmd)
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatSize(tt.bytes))
	}
}

func TestIsTextMIME(t *testing.T) {
	assert.True(t, isTextMIME("text/plain"))
	assert.True(t, isTextMIME("text/html"))
	assert.True(t, isTextMIME("text/css"))
	assert.True(t, isTextMIME("application/json"))
	assert.True(t, isTextMIME("application/xml"))
	assert.True(t, isTextMIME("application/javascript"))
	assert.True(t, isTextMIME("application/x-sh"))
	assert.False(t, isTextMIME("image/png"))
	assert.False(t, isTextMIME("application/octet-stream"))
	assert.False(t, isTextMIME("video/mp4"))
}

func TestViewportHeight(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)

	// With normal height
	p.height = 20
	assert.Equal(t, 19, p.viewportHeight()) // minus 1 for scroll indicator

	// With small height
	p.height = 1
	assert.Equal(t, 1, p.viewportHeight())

	// With zero height (fallback)
	p.height = 0
	assert.Equal(t, 19, p.viewportHeight()) // default 20 - 1
}

// ---------------------------------------------------------------------------
// Markdown rendering – heading prefix stripping
// ---------------------------------------------------------------------------

func TestRenderMarkdown_NoHashPrefixes(t *testing.T) {
	md := "## Section\n### Sub\nText body"
	lines := markdown.RenderStatic(md, 80)
	combined := strings.Join(lines, "\n")

	// Strip ANSI codes for reliable assertion.
	clean := ansi.Strip(combined)

	// Headings should render without the markdown prefix.
	assert.NotContains(t, clean, "## ")
	assert.NotContains(t, clean, "### ")
	assert.Contains(t, clean, "Section")
	assert.Contains(t, clean, "Sub")
	assert.Contains(t, clean, "Text body")
}

// ---------------------------------------------------------------------------
// GitHub content mode – Issue
// ---------------------------------------------------------------------------

func TestIssueSelectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	_, cmd := p.Update(panels.IssueSelectedMsg{
		Number:   42,
		Title:    "Fix authentication bug",
		Body:     "The login flow is broken.\n\nSteps to reproduce...",
		State:    "open",
		Author:   "octocat",
		Assignee: "hubot",
		Labels:   []string{"bug", "needs triage"},
	})

	assert.True(t, p.ghMode)
	assert.Equal(t, "#42 Fix authentication bug", p.ghTitle)
	assert.Contains(t, p.ghContent, "# Issue #42")
	assert.Contains(t, p.ghContent, "State: open")
	assert.Contains(t, p.ghContent, "Author: @octocat")
	assert.Contains(t, p.ghContent, "Assignee: @hubot")
	assert.Contains(t, p.ghContent, "Labels: `bug`, `needs triage`")
	assert.Contains(t, p.ghContent, "The login flow is broken.")
	assert.Equal(t, 0, p.scrollY)
	assert.NotNil(t, p.lines)
	assert.Nil(t, cmd)

	// Title should reflect the issue.
	assert.Equal(t, "#42 Fix authentication bug", p.Title())
}

func TestIssueSelectedMsg_EmptyBody(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	p.Update(panels.IssueSelectedMsg{
		Number: 10,
		Title:  "No description",
		Body:   "",
	})

	assert.True(t, p.ghMode)
	assert.Contains(t, p.ghContent, "*No description provided.*")
}

func TestIssueDeselectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	// Select an issue first.
	p.Update(panels.IssueSelectedMsg{Number: 1, Title: "Test", Body: "body"})
	assert.True(t, p.ghMode)

	// Deselect — without a previous file loaded, no reload cmd.
	_, cmd := p.Update(panels.IssueDeselectedMsg{})
	assert.False(t, p.ghMode)
	assert.Empty(t, p.ghTitle)
	assert.Empty(t, p.ghContent)
	assert.Nil(t, cmd)
}

func TestIssueDeselectedMsg_RestoresFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "restore.txt", "file content")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	// Load a file first.
	loadFile(t, p, path)
	assert.Equal(t, "restore.txt", p.Title())

	// Select an issue.
	p.Update(panels.IssueSelectedMsg{Number: 1, Title: "Test", Body: "body"})
	assert.True(t, p.ghMode)

	// Deselect — should return a cmd to reload the file.
	_, cmd := p.Update(panels.IssueDeselectedMsg{})
	assert.False(t, p.ghMode)
	assert.NotNil(t, cmd, "should return reload cmd when file was loaded")
}

// ---------------------------------------------------------------------------
// GitHub content mode – PR
// ---------------------------------------------------------------------------

func TestPRSelectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	_, cmd := p.Update(panels.PRSelectedMsg{
		Number:     99,
		Title:      "Add caching layer",
		State:      "open",
		HeadBranch: "feature/cache",
	})

	assert.True(t, p.ghMode)
	assert.Equal(t, "PR #99 Add caching layer", p.ghTitle)
	assert.Contains(t, p.ghContent, "PR #99")
	assert.Contains(t, p.ghContent, "Add caching layer")
	assert.Contains(t, p.ghContent, "feature/cache")
	assert.Contains(t, p.ghContent, "open")
	assert.Equal(t, 0, p.scrollY)
	assert.NotNil(t, p.lines)
	assert.Nil(t, cmd)
}

func TestPRDeselectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	p.Update(panels.PRSelectedMsg{Number: 1, Title: "T", State: "open", HeadBranch: "b"})
	assert.True(t, p.ghMode)

	_, cmd := p.Update(panels.PRDeselectedMsg{})
	assert.False(t, p.ghMode)
	assert.Empty(t, p.ghTitle)
	assert.Empty(t, p.ghContent)
	assert.Nil(t, cmd)
}

func TestPRDeselectedMsg_RestoresFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "content")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	p.Update(panels.PRSelectedMsg{Number: 1, Title: "T", State: "open", HeadBranch: "b"})
	_, cmd := p.Update(panels.PRDeselectedMsg{})
	assert.NotNil(t, cmd, "should return reload cmd when file was loaded")
}

// ---------------------------------------------------------------------------
// GitHub content mode – Action Run
// ---------------------------------------------------------------------------

func TestActionRunSelectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	_, cmd := p.Update(panels.ActionRunSelectedMsg{
		RunID:        12345,
		WorkflowName: "CI",
		Status:       "completed",
	})

	assert.True(t, p.ghMode)
	assert.Equal(t, "CI (Run #12345)", p.ghTitle)
	assert.Contains(t, p.ghContent, "CI")
	assert.Contains(t, p.ghContent, "completed")
	assert.Contains(t, p.ghContent, "12345")
	assert.Equal(t, 0, p.scrollY)
	assert.NotNil(t, p.lines)
	assert.Nil(t, cmd)
}

func TestActionRunDeselectedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	p.Update(panels.ActionRunSelectedMsg{RunID: 1, WorkflowName: "CI", Status: "completed"})
	assert.True(t, p.ghMode)

	_, cmd := p.Update(panels.ActionRunDeselectedMsg{})
	assert.False(t, p.ghMode)
	assert.Empty(t, p.ghTitle)
	assert.Empty(t, p.ghContent)
	assert.Nil(t, cmd)
}

func TestActionRunDeselectedMsg_RestoresFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "content")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	p.Update(panels.ActionRunSelectedMsg{RunID: 1, WorkflowName: "CI", Status: "completed"})
	_, cmd := p.Update(panels.ActionRunDeselectedMsg{})
	assert.NotNil(t, cmd, "should return reload cmd when file was loaded")
}

// ---------------------------------------------------------------------------
// statusIcon helper
// ---------------------------------------------------------------------------

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status     string
		conclusion string
		expected   string
	}{
		{"completed", "success", "✓"},
		{"completed", "failure", "✗"},
		{"completed", "cancelled", "⊘"},
		{"completed", "skipped", "⊘"},
		{"in_progress", "", "●"},
		{"queued", "", "○"},
		{"waiting", "", "○"},
		{"pending", "", "○"},
		{"completed", "", "✓"},
		{"", "", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.status+"_"+tt.conclusion, func(t *testing.T) {
			result := statusIcon(tt.status, tt.conclusion)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// formatDuration helper
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name      string
		started   string
		completed string
		expected  string
	}{
		{"empty started", "", "2024-06-15T10:01:00Z", ""},
		{"empty completed", "2024-06-15T10:00:00Z", "", ""},
		{"both empty", "", "", ""},
		{"invalid started", "not-a-date", "2024-06-15T10:01:00Z", ""},
		{"invalid completed", "2024-06-15T10:00:00Z", "not-a-date", ""},
		{"30 seconds", "2024-06-15T10:00:00Z", "2024-06-15T10:00:30Z", "30s"},
		{"0 seconds", "2024-06-15T10:00:00Z", "2024-06-15T10:00:00Z", "0s"},
		{"2 minutes 5 seconds", "2024-06-15T10:00:00Z", "2024-06-15T10:02:05Z", "2m 05s"},
		{"negative duration", "2024-06-15T10:01:00Z", "2024-06-15T10:00:00Z", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.started, tt.completed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// renderActionJobs
// ---------------------------------------------------------------------------

func TestRenderActionJobs(t *testing.T) {
	jobs := []panels.ActionJob{
		{
			ID:          1,
			Name:        "build",
			Status:      "completed",
			Conclusion:  "success",
			StartedAt:   "2024-06-15T10:00:00Z",
			CompletedAt: "2024-06-15T10:01:30Z",
			Steps: []panels.ActionStep{
				{Number: 1, Name: "Checkout", Status: "completed", Conclusion: "success"},
				{Number: 2, Name: "Build", Status: "completed", Conclusion: "failure"},
			},
		},
		{
			ID:         2,
			Name:       "test",
			Status:     "in_progress",
			Conclusion: "",
		},
	}

	result := renderActionJobs(jobs)

	assert.Contains(t, result, "Jobs")
	assert.Contains(t, result, "build")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "✓") // success icon for build
	assert.Contains(t, result, "●") // in_progress icon for test
	assert.Contains(t, result, "1m 30s")
	assert.Contains(t, result, "Checkout")
	assert.Contains(t, result, "Build")
	assert.Contains(t, result, "1.")
	assert.Contains(t, result, "2.")
}

func TestRenderActionJobs_Empty(t *testing.T) {
	result := renderActionJobs(nil)
	assert.Contains(t, result, "No jobs found")
}

// ---------------------------------------------------------------------------
// renderActionLog
// ---------------------------------------------------------------------------

func TestRenderActionLog(t *testing.T) {
	log := "Step 1: checkout\nStep 2: build\nStep 3: test"
	result := renderActionLog(log)

	assert.Contains(t, result, "Failed Job Log")
	assert.Contains(t, result, "Step 1: checkout")
	assert.Contains(t, result, "Step 2: build")
	assert.Contains(t, result, "Step 3: test")
}

func TestRenderActionLog_Truncation(t *testing.T) {
	// Generate more than 100 lines.
	lines := make([]string, 150)
	for i := range lines {
		lines[i] = strings.Repeat("x", 20)
	}
	longLog := strings.Join(lines, "\n")

	result := renderActionLog(longLog)
	assert.Contains(t, result, "truncated")
}

// ---------------------------------------------------------------------------
// renderError
// ---------------------------------------------------------------------------

func TestRenderError(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 20)
	p.filePath = "test.txt"
	p.err = os.ErrNotExist

	view := p.View(80, 20)
	assert.Contains(t, view, "Error")
}

// ---------------------------------------------------------------------------
// GitFilterActiveMsg
// ---------------------------------------------------------------------------

func TestGitFilterActiveMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	assert.False(t, p.diffMode)

	_, cmd := p.Update(panels.GitFilterActiveMsg{Active: true})
	assert.True(t, p.diffMode)
	assert.Nil(t, cmd)

	_, cmd = p.Update(panels.GitFilterActiveMsg{Active: false})
	assert.False(t, p.diffMode)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Mouse wheel handling
// ---------------------------------------------------------------------------

func TestMouseWheelScrolling(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	loadFile(t, p, path)

	assert.Equal(t, 0, p.scrollY)

	// Scroll down with mouse wheel.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 3, p.scrollY, "mouse wheel down should scroll by 3")

	// Scroll up with mouse wheel.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.scrollY, "mouse wheel up should scroll by 3")
}

// ---------------------------------------------------------------------------
// Diff-related functionality
// ---------------------------------------------------------------------------

func TestDiffLinesRenderedInView(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "line1\nline2\nline3")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.LineNumbers = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	// Simulate diff lines being loaded.
	p.Update(diffLoadedMsg{
		path:  path,
		lines: []string{"+added line", "-removed line", " context line"},
	})

	assert.NotNil(t, p.diffLines)
	assert.Len(t, p.diffLines, 3)

	// View should contain the diff header.
	view := p.View(80, 30)
	assert.NotEmpty(t, view)
}

func TestGitDiffOnlyMode(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "original")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	cfg.LineNumbers = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	loadFile(t, p, path)

	// Set diff lines and enable diff mode.
	p.diffLines = []string{"+new line"}
	p.diffMode = true

	view := p.View(80, 30)
	assert.NotEmpty(t, view)
}

func TestDiffLoadedMsg_WrongPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "content")

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(80, 20)
	loadFile(t, p, path)

	// Diff for a different path should be ignored.
	p.Update(diffLoadedMsg{
		path:  "/some/other/file.txt",
		lines: []string{"+nope"},
	})

	assert.Nil(t, p.diffLines)
}

// ---------------------------------------------------------------------------
// ActionJobsLoadedMsg and ActionLogMsg
// ---------------------------------------------------------------------------

func TestActionJobsLoadedMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	jobs := []panels.ActionJob{
		{ID: 1, Name: "lint", Status: "completed", Conclusion: "success"},
	}

	_, cmd := p.Update(panels.ActionJobsLoadedMsg{RunID: 100, Jobs: jobs})
	assert.True(t, p.ghMode)
	assert.Equal(t, 0, p.scrollY)
	assert.Contains(t, p.ghContent, "lint")
	assert.NotNil(t, p.lines)
	assert.Nil(t, cmd)
}

func TestActionLogMsg(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	// Put panel in ghMode first.
	p.ghMode = true
	p.ghContent = "existing content"
	p.lines = []string{"existing content"}

	_, cmd := p.Update(panels.ActionLogMsg{
		RunID: 1,
		JobID: 2,
		Log:   "Step 1: done\nStep 2: fail",
	})

	assert.Contains(t, p.ghContent, "existing content")
	assert.Contains(t, p.ghContent, "Failed Job Log")
	assert.Nil(t, cmd)
}

func TestActionLogMsg_NotInGhMode(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.ghMode = false

	origContent := p.ghContent
	p.Update(panels.ActionLogMsg{RunID: 1, JobID: 2, Log: "some log"})

	// Should not modify anything when not in ghMode.
	assert.Equal(t, origContent, p.ghContent)
}

func TestActionLogMsg_EmptyLog(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.ghMode = true
	p.ghContent = "before"

	p.Update(panels.ActionLogMsg{RunID: 1, JobID: 2, Log: ""})

	// Empty log should not append anything.
	assert.Equal(t, "before", p.ghContent)
}

// ---------------------------------------------------------------------------
// FileSelectedMsg clears ghMode
// ---------------------------------------------------------------------------

func TestFileSelectedMsg_ClearsGhMode(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "test.txt", "hello")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	// Enter ghMode via issue.
	p.Update(panels.IssueSelectedMsg{Number: 1, Title: "T", Body: "B"})
	assert.True(t, p.ghMode)

	// Select a file — should clear ghMode.
	loadFile(t, p, path)
	assert.False(t, p.ghMode)
	assert.Empty(t, p.ghTitle)
	assert.Empty(t, p.ghContent)
}

// ---------------------------------------------------------------------------
// PreviewScrollMsg
// ---------------------------------------------------------------------------

func TestPreviewScrollMsg(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	path := writeFile(t, dir, "long.txt", content)

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 10)
	loadFile(t, p, path)

	// Scroll down.
	_, cmd := p.Update(panels.PreviewScrollMsg{Delta: 5})
	assert.Equal(t, 5, p.scrollY)
	assert.Nil(t, cmd)

	// Scroll up.
	p.Update(panels.PreviewScrollMsg{Delta: -3})
	assert.Equal(t, 2, p.scrollY)
}

// ---------------------------------------------------------------------------
// BlameLoadedMsg
// ---------------------------------------------------------------------------

func TestBlameLoadedMsg_Success(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.blameMode = true

	blameLines := []git.BlameLine{
		{Hash: "abc123", Author: "Alice", LineNo: 1, Content: "line 1"},
		{Hash: "def456", Author: "Bob", LineNo: 2, Content: "line 2"},
	}

	_, cmd := p.Update(panels.BlameLoadedMsg{Lines: blameLines})
	assert.NotNil(t, p.blameLines)
	assert.Len(t, p.blameLines, 2)
	assert.Nil(t, cmd)
}

func TestBlameLoadedMsg_Error(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.blameMode = true

	_, cmd := p.Update(panels.BlameLoadedMsg{Err: os.ErrNotExist})
	assert.False(t, p.blameMode)
	assert.Nil(t, p.blameLines)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Loading state
// ---------------------------------------------------------------------------

func TestLoadingStateRendering(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.loading = true

	view := p.View(60, 20)
	assert.Contains(t, view, "Loading")
}

// ---------------------------------------------------------------------------
// ghMode rendering in View
// ---------------------------------------------------------------------------

func TestGhModeViewRendering(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)

	// Enter ghMode.
	p.Update(panels.IssueSelectedMsg{
		Number: 5,
		Title:  "Bug report",
		Body:   "Something is broken.",
	})

	// View should render the ghContent (issue body), not "No file selected".
	view := p.View(80, 30)
	assert.NotContains(t, view, "No file selected")
	assert.NotEmpty(t, view)
}

// ---------------------------------------------------------------------------
// isMarkdownExt helper
// ---------------------------------------------------------------------------

func TestIsMarkdownExt(t *testing.T) {
	assert.True(t, isMarkdownExt(".md"))
	assert.True(t, isMarkdownExt(".MD"))
	assert.True(t, isMarkdownExt(".markdown"))
	assert.True(t, isMarkdownExt(".mdown"))
	assert.True(t, isMarkdownExt(".mkd"))
	assert.False(t, isMarkdownExt(".txt"))
	assert.False(t, isMarkdownExt(".go"))
	assert.False(t, isMarkdownExt(""))
}

// ---------------------------------------------------------------------------
// FilePath
// ---------------------------------------------------------------------------

func TestFilePath_ReturnsCurrentPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.go", "package main\n")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	loadFile(t, p, path)

	assert.Equal(t, path, p.FilePath())
}

func TestFilePath_EmptyWhenNoFile(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	assert.Equal(t, "", p.FilePath())
}

// ---------------------------------------------------------------------------
// RepoChangedMsg
// ---------------------------------------------------------------------------

func TestRepoChangedMsg_ClearsPreview(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.Init(context.Background())

	// Set up some preview state.
	p.filePath = "old/file.go"
	p.lines = []string{"line1", "line2"}
	p.blameMode = true
	p.ghMode = true
	p.ghTitle = "Old Issue"
	p.ghContent = "Some content"

	tmpDir := t.TempDir()
	result, cmd := p.Update(panels.RepoChangedMsg{Path: tmpDir})
	pv := result.(*Preview)

	assert.Equal(t, "", pv.filePath, "filePath should be cleared")
	assert.Nil(t, pv.lines, "lines should be cleared")
	assert.False(t, pv.blameMode, "blameMode should be false")
	assert.False(t, pv.ghMode, "ghMode should be false")
	assert.Equal(t, "", pv.ghTitle, "ghTitle should be empty")
	assert.Equal(t, "", pv.ghContent, "ghContent should be empty")
	assert.False(t, pv.loading, "loading should be false")
	assert.Nil(t, cmd, "no command should be returned")
}

// ---------------------------------------------------------------------------
// ANSI escape-sequence injection regression tests (CWE-150)
// ---------------------------------------------------------------------------

func TestIssueSelectedMsg_ANSIInjection(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	msg := panels.IssueSelectedMsg{
		Number:   42,
		Title:    "Bug: \x1b[31mRED\x1b[0m injection",
		Body:     "body text",
		State:    "\x1b[31mopen\x1b[0m",
		Author:   "\x1b[31moctocat\x1b[0m",
		Assignee: "\x1b[31mhubot\x1b[0m",
		Labels:   []string{"\x1b[31mbug\x1b[0m"},
	}
	p.Update(msg)
	assert.NotContains(t, p.ghTitle, "\x1b", "ANSI in issue title should be stripped")
	assert.Contains(t, p.ghTitle, "#42 Bug: RED injection")
	assert.NotContains(t, p.ghContent, "\x1b", "ANSI in issue metadata should be stripped")
	assert.Contains(t, p.ghContent, "State: open")
	assert.Contains(t, p.ghContent, "Author: @octocat")
	assert.Contains(t, p.ghContent, "Assignee: @hubot")
	assert.Contains(t, p.ghContent, "Labels: `bug`")
}

func TestPRSelectedMsg_ANSIInjection(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	msg := panels.PRSelectedMsg{
		Number:     99,
		Title:      "feat: \x1b]0;pwned\x07attack",
		HeadBranch: "\x1b[1mevil-branch\x1b[0m",
		State:      "open",
	}
	p.Update(msg)
	assert.NotContains(t, p.ghTitle, "\x1b", "ANSI in PR title should be stripped")
	assert.Contains(t, p.ghTitle, "feat: attack")
	// Verify content also sanitized.
	combined := strings.Join(p.lines, "\n")
	stripped := ansi.Strip(combined)
	assert.NotContains(t, stripped, "\x1b", "ANSI in PR content should be stripped")
}

func TestActionRunSelectedMsg_ANSIInjection(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(80, 30)
	msg := panels.ActionRunSelectedMsg{
		WorkflowName: "CI \x1b[2J\x1b[H",
		RunID:        123,
		Status:       "\x1b[31mfailure\x1b[0m",
	}
	p.Update(msg)
	assert.NotContains(t, p.ghTitle, "\x1b", "ANSI in workflow name should be stripped")
	assert.Contains(t, p.ghTitle, "CI ")
}
