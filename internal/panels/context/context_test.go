package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	ctxbuilder "github.com/jongio/grut/internal/context"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPanel creates a focused context panel with a temp-dir builder.
func newTestPanel(t *testing.T) (*Panel, string) {
	t.Helper()
	root := t.TempDir()
	builder, err := ctxbuilder.NewBuilder(root)
	require.NoError(t, err)
	p := New(builder, nil)
	p.Focus()
	p.SetSize(80, 24)
	p.Init(context.Background())
	return p, root
}

// writeFile creates a file under dir with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// keyMsg constructs a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestPanel_Creation(t *testing.T) {
	p, _ := newTestPanel(t)
	assert.Equal(t, "context", p.Title())
	assert.Equal(t, 0, p.fileCount())
}

func TestPanel_EmptyView(t *testing.T) {
	p, _ := newTestPanel(t)
	view := p.View(80, 24)
	assert.Contains(t, view, "No files in context")
	assert.Contains(t, view, "0 files")
	assert.Contains(t, view, "0 tokens")
}

func TestPanel_AddFile(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "main.go", "package main\n")

	updated, cmd := p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "main.go")})
	p = updated.(*Panel)

	assert.Equal(t, 1, p.fileCount())
	require.NotNil(t, cmd)

	msg := cmd()
	ctxMsg, ok := msg.(panels.ContextUpdatedMsg)
	require.True(t, ok, "expected ContextUpdatedMsg, got %T", msg)
	assert.Equal(t, 1, ctxMsg.FileCount)
	assert.Greater(t, ctxMsg.TokenCount, 0)
}

func TestPanel_AddFileError(t *testing.T) {
	p, _ := newTestPanel(t)

	updated, cmd := p.Update(panels.AddToContextMsg{Path: "nonexistent.go"})
	p = updated.(*Panel)

	assert.Equal(t, 0, p.fileCount())
	require.NotNil(t, cmd)

	msg := cmd()
	toastMsg, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Error, toastMsg.Level)
}

func TestPanel_ViewShowsFiles(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "hello.go", "package hello\n")

	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "hello.go")})

	view := p.View(80, 24)
	assert.Contains(t, view, "hello.go")
	assert.Contains(t, view, "tokens")
	assert.Contains(t, view, "1 files")
}

func TestPanel_Navigation(t *testing.T) {
	p, root := newTestPanel(t)
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		writeFile(t, root, name, "package "+name+"\n")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, name)})
	}

	// Start at top.
	assert.Equal(t, 0, p.cursorIndex())

	// Move down.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursorIndex())

	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursorIndex())

	// Can't go past end.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursorIndex())

	// Move up.
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursorIndex())

	// Go to top.
	p.Update(keyMsg('g'))
	assert.Equal(t, 0, p.cursorIndex())

	// Go to bottom.
	p.Update(keyMsg('G'))
	assert.Equal(t, 2, p.cursorIndex())
}

func TestPanel_RemoveFile(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "b.go", "package b\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "b.go")})

	assert.Equal(t, 2, p.fileCount())

	// Remove by pressing 'd' on first file.
	updated, cmd := p.Update(keyMsg('d'))
	p = updated.(*Panel)

	assert.Equal(t, 1, p.fileCount())
	require.NotNil(t, cmd)

	msg := cmd()
	ctxMsg, ok := msg.(panels.ContextUpdatedMsg)
	require.True(t, ok, "expected ContextUpdatedMsg, got %T", msg)
	assert.Equal(t, 1, ctxMsg.FileCount)
}

func TestPanel_RemoveViaMessage(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	updated, _ := p.Update(panels.RemoveFromContextMsg{Path: filepath.Join(root, "a.go")})
	p = updated.(*Panel)

	assert.Equal(t, 0, p.fileCount())
}

func TestPanel_Clear(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "b.go", "package b\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "b.go")})

	// Clear via keypress.
	updated, cmd := p.Update(keyMsg('c'))
	p = updated.(*Panel)

	assert.Equal(t, 0, p.fileCount())
	assert.Equal(t, 0, p.cursorIndex())

	require.NotNil(t, cmd)
	msg := cmd()
	ctxMsg, ok := msg.(panels.ContextUpdatedMsg)
	require.True(t, ok, "expected ContextUpdatedMsg, got %T", msg)
	assert.Equal(t, 0, ctxMsg.FileCount)
	assert.Equal(t, 0, ctxMsg.TokenCount)
}

func TestPanel_ClearViaMessage(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	updated, _ := p.Update(panels.ClearContextMsg{})
	p = updated.(*Panel)

	assert.Equal(t, 0, p.fileCount())
}

func TestPanel_Export(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "main.go", "package main\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "main.go")})

	updated, cmd := p.Update(keyMsg('e'))
	_ = updated.(*Panel)

	require.NotNil(t, cmd)
	msg := cmd()
	toastMsg, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Success, toastMsg.Level)
	assert.Contains(t, toastMsg.Message, "1 files")
}

func TestPanel_ExportEmpty(t *testing.T) {
	p, _ := newTestPanel(t)

	updated, cmd := p.Update(keyMsg('e'))
	_ = updated.(*Panel)

	require.NotNil(t, cmd)
	msg := cmd()
	toastMsg, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Warn, toastMsg.Level)
}

func TestPanel_ExportViaMessage(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	updated, cmd := p.Update(panels.ExportContextMsg{})
	_ = updated.(*Panel)

	require.NotNil(t, cmd)
	msg := cmd()
	toastMsg, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Success, toastMsg.Level)
}

func TestPanel_EnterEmitsFileSelected(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "main.go", "package main\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "main.go")})

	updated, cmd := p.Update(keyMsg('\r')) // enter
	_ = updated.(*Panel)

	require.NotNil(t, cmd)
	msg := cmd()
	fsMsg, ok := msg.(panels.FileSelectedMsg)
	require.True(t, ok, "expected FileSelectedMsg, got %T", msg)
	assert.Equal(t, "main.go", fsMsg.Path)
}

func TestPanel_EnterOnEmpty(t *testing.T) {
	p, _ := newTestPanel(t)

	_, cmd := p.Update(keyMsg('\r'))
	assert.Nil(t, cmd)
}

func TestPanel_RemoveOnEmpty(t *testing.T) {
	p, _ := newTestPanel(t)

	_, cmd := p.Update(keyMsg('d'))
	assert.Nil(t, cmd)
}

func TestPanel_IgnoresKeysWhenBlurred(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})
	p.Blur()

	_, cmd := p.Update(keyMsg('j'))
	assert.Nil(t, cmd)
	assert.Equal(t, 0, p.cursorIndex())
}

func TestPanel_TokenCountDisplay(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "big.go", "one two three four five six seven eight nine ten\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "big.go")})

	view := p.View(80, 24)
	assert.Contains(t, view, "tokens")
}

func TestPanel_KeyBindings(t *testing.T) {
	p, _ := newTestPanel(t)

	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/↓"])
	assert.True(t, keys["d/del"])
	assert.True(t, keys["c"])
	assert.True(t, keys["e"])
	assert.True(t, keys["enter"])
}

func TestPanel_ViewDimensions(t *testing.T) {
	p, _ := newTestPanel(t)

	assert.Equal(t, "", p.View(0, 0))
	assert.Equal(t, "", p.View(-1, 10))
	assert.Equal(t, "", p.View(10, -1))
}

func TestPanel_CursorClampsAfterRemove(t *testing.T) {
	p, root := newTestPanel(t)
	for _, name := range []string{"a.go", "b.go"} {
		writeFile(t, root, name, "package "+name+"\n")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, name)})
	}

	// Move to last item.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursorIndex())

	// Remove it — cursor should clamp.
	p.Update(keyMsg('d'))
	assert.Equal(t, 0, p.cursorIndex())
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

func TestPanel_MouseClick_SelectsFile(t *testing.T) {
	p, root := newTestPanel(t)
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		writeFile(t, root, name, "package "+name+"\n")
		p.Update(panels.AddToContextMsg{Path: filepath.Join(root, name)})
	}

	assert.Equal(t, 0, p.cursorIndex())

	// Click on row 1 (second file).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursorIndex())

	// Click on row 2 (third file).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursorIndex())
}

func TestPanel_MouseClick_OutOfBoundsIgnored(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursorIndex(), "out-of-bounds click should not move cursor")
}

func TestPanel_MouseDoubleClick_PreviewsFile(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "b.go", "package b\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "b.go")})
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemContextFile): true}

	// Double-click on row 1 (second file).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursorIndex())
	require.NotNil(t, cmd, "double-click should trigger preview command")

	msg := cmd()
	fsMsg, ok := msg.(panels.FileSelectedMsg)
	require.True(t, ok, "expected FileSelectedMsg, got %T", msg)
	assert.Contains(t, fsMsg.Path, "b.go")
}

func TestPanel_MouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	p, root := newTestPanel(t)
	writeFile(t, root, "a.go", "package a\n")
	p.Update(panels.AddToContextMsg{Path: filepath.Join(root, "a.go")})

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursorIndex())
	assert.Nil(t, cmd)
}
