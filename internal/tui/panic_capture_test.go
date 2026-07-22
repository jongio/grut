package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/jongio/grut/internal/layout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectDataHome points config.DataDir() (and thus the crash directory) at a
// temp dir for the duration of the test.
func redirectDataHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	orig := xdg.DataHome
	xdg.DataHome = tmp
	t.Cleanup(func() { xdg.DataHome = orig })
	return tmp
}

// capturePanicMsg is an otherwise-unhandled message that falls through the root
// Update switch to handleDefaultMsg, which broadcasts to the layout engine.
type capturePanicMsg struct{}

// panicOnUpdateEngine wraps a real PanelManager but panics from Update, so we
// can drive a deterministic panic through the root model's Update path.
type panicOnUpdateEngine struct{ layout.PanelManager }

func (panicOnUpdateEngine) Update(tea.Msg) tea.Cmd { panic("boom-in-engine-update") }

// panicOnRenderEngine wraps a real PanelManager but panics from PanelRects,
// which renderLayout (called by View) invokes first.
type panicOnRenderEngine struct{ layout.PanelManager }

func (panicOnRenderEngine) PanelRects() map[string]layout.Rect { panic("boom-in-engine-render") }

// panicOnInitEngine wraps a real PanelManager but panics from Init.
type panicOnInitEngine struct{ layout.PanelManager }

func (panicOnInitEngine) Init(context.Context) tea.Cmd { panic("boom-in-engine-init") }

// TestModelUpdate_PanicIsCaptured verifies that a panic raised while the root
// model's Update processes a message is captured as a crash report (issue #361
// AC #1) and still re-panics so Bubble Tea can restore the terminal.
func TestModelUpdate_PanicIsCaptured(t *testing.T) {
	dir := redirectDataHome(t)
	m := newTestModel(t)
	m.engine = panicOnUpdateEngine{m.engine}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		m.Update(capturePanicMsg{})
	}()

	require.NotNil(t, recovered, "Update must re-panic after capturing the crash")
	crashes := filepath.Join(dir, "grut", "crashes")
	entries, err := os.ReadDir(crashes)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "an Update panic must write a crash report under DataDir()/crashes")
}

// TestModelView_PanicIsCaptured verifies the same guarantee for the root
// model's View render path.
func TestModelView_PanicIsCaptured(t *testing.T) {
	dir := redirectDataHome(t)
	m := newTestModel(t)
	m.ready = true
	m.width, m.height = 80, 24
	m.engine = panicOnRenderEngine{m.engine}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = m.View()
	}()

	require.NotNil(t, recovered, "View must re-panic after capturing the crash")
	crashes := filepath.Join(dir, "grut", "crashes")
	entries, err := os.ReadDir(crashes)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "a View panic must write a crash report under DataDir()/crashes")
}

// TestModelInit_PanicIsCaptured verifies the same guarantee for the root
// model's Init path, which runs inside Bubble Tea's start-up catch scope.
func TestModelInit_PanicIsCaptured(t *testing.T) {
	dir := redirectDataHome(t)
	m := newTestModel(t)
	m.engine = panicOnInitEngine{m.engine}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		m.Init()
	}()

	require.NotNil(t, recovered, "Init must re-panic after capturing the crash")
	crashes := filepath.Join(dir, "grut", "crashes")
	entries, err := os.ReadDir(crashes)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "an Init panic must write a crash report under DataDir()/crashes")
}
