package terminal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock runner
// ---------------------------------------------------------------------------

type mockRunner struct {
	mu        sync.Mutex
	lines     []string
	exitCode  int
	done      chan struct{}
	closeOnce sync.Once
	written   []byte
	writeErr  error
	closed    bool
}

func newMockRunner(lines []string) *mockRunner {
	return &mockRunner{
		lines:    lines,
		done:     make(chan struct{}),
		exitCode: -1,
	}
}

func (m *mockRunner) Write(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, data...)
	return nil
}

func (m *mockRunner) Lines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.lines))
	copy(cp, m.lines)
	return cp
}

func (m *mockRunner) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockRunner) Done() <-chan struct{} {
	return m.done
}

func (m *mockRunner) ExitCode() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitCode
}

func (m *mockRunner) setExited(code int) {
	m.mu.Lock()
	m.exitCode = code
	m.mu.Unlock()
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

func (m *mockRunner) addLines(newLines ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, newLines...)
}

func (m *mockRunner) getWritten() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(m.written))
	copy(cp, m.written)
	return cp
}

func (m *mockRunner) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultCfg() config.TerminalConfig {
	return config.TerminalConfig{
		Shell:      "test-shell",
		Scrollback: 10000,
		RenderFPS:  30,
		PrefixKey:  "ctrl+b",
	}
}

func newTestPanel(runner *mockRunner) *Panel {
	cfg := defaultCfg()
	p := New(cfg, runner, "test-shell", nil)
	p.SetSize(80, 24)
	return p
}

// runCmd executes a tea.Cmd and returns the resulting message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

func TestPanelImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

func TestPanelImplementsCloser(t *testing.T) {
	var _ panels.Closer = (*Panel)(nil)
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	runner := newMockRunner(nil)
	p := New(defaultCfg(), runner, "bash", nil)
	assert.Equal(t, "terminal", p.Title())
	assert.NotNil(t, p.KeyBindings())
	assert.Equal(t, modeNormal, p.Mode())
	assert.Empty(t, p.Input())
}

func TestNewNilRunner(t *testing.T) {
	p := New(defaultCfg(), nil, "", nil)
	assert.Equal(t, "terminal", p.Title())
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func TestInit(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return a tick command")
}

func TestInitNilRunner(t *testing.T) {
	p := New(defaultCfg(), nil, "", nil)
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd, "Init with nil runner should return nil")
}

// ---------------------------------------------------------------------------
// Normal mode key handling
// ---------------------------------------------------------------------------

func TestNormalModeScrollDown(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 5

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 4, p.Offset())
}

func TestNormalModeScrollDownAtBottom(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 0

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 0, p.Offset(), "should not scroll past bottom")
}

func TestNormalModeScrollUp(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 0

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.Offset())
}

func TestNormalModeScrollUpAtTop(t *testing.T) {
	// With 50 lines and contentHeight=23 (24-1 status), maxOffset = 50-23 = 27.
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 27 // at max

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 27, p.Offset(), "should not scroll past top")
}

func TestNormalModeScrollToBottom(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 10

	p.Update(tea.KeyPressMsg{Code: 'G'})
	assert.Equal(t, 0, p.Offset())
}

func TestNormalModeScrollToTop(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 0

	p.Update(tea.KeyPressMsg{Code: 'g'})
	assert.Equal(t, 27, p.Offset()) // 50 - 23 = 27
}

func TestNormalModeArrowDown(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 5

	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 4, p.Offset())
}

func TestNormalModeArrowUp(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 0

	p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, p.Offset())
}

// ---------------------------------------------------------------------------
// Mode switching
// ---------------------------------------------------------------------------

func TestEnterInsertModeWithI(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, modeInsert, p.Mode())
}

func TestEnterInsertModeWithEnter(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, modeInsert, p.Mode())
}

func TestInsertModeResetsOffset(t *testing.T) {
	runner := newMockRunner(make([]string, 50))
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()
	p.offset = 10

	p.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, 0, p.Offset(), "entering insert mode should scroll to bottom")
}

func TestExitInsertModeWithPrefixKey(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, modeNormal, p.Mode())
}

func TestExitInsertModeCustomPrefixKey(t *testing.T) {
	runner := newMockRunner(nil)
	cfg := defaultCfg()
	cfg.PrefixKey = "ctrl+a"
	p := New(cfg, runner, "test-shell", nil)
	p.SetSize(80, 24)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	assert.Equal(t, modeNormal, p.Mode())
}

// ---------------------------------------------------------------------------
// Insert mode key forwarding
// ---------------------------------------------------------------------------

func TestInsertModeTyping(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: 'h'})
	p.Update(tea.KeyPressMsg{Code: 'e'})
	p.Update(tea.KeyPressMsg{Code: 'l'})
	p.Update(tea.KeyPressMsg{Code: 'l'})
	p.Update(tea.KeyPressMsg{Code: 'o'})

	assert.Equal(t, "hello", p.Input())
}

func TestInsertModeSpace(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: 'h'})
	p.Update(tea.KeyPressMsg{Code: 'i'})
	p.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	p.Update(tea.KeyPressMsg{Code: '!'})

	assert.Equal(t, "hi !", p.Input())
}

func TestInsertModeBackspace(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: 'a'})
	p.Update(tea.KeyPressMsg{Code: 'b'})
	p.Update(tea.KeyPressMsg{Code: 'c'})
	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})

	assert.Equal(t, "ab", p.Input())
}

func TestInsertModeBackspaceOnEmpty(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Empty(t, p.Input())
}

func TestInsertModeTab(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "\t", p.Input())
}

func TestInsertModeEnterSendsInput(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	// Type a command.
	p.Update(tea.KeyPressMsg{Code: 'l'})
	p.Update(tea.KeyPressMsg{Code: 's'})
	assert.Equal(t, "ls", p.Input())

	// Press Enter to send.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Input should be cleared.
	assert.Empty(t, p.Input())

	// Data should have been written to the runner.
	written := runner.getWritten()
	assert.Equal(t, "ls\n", string(written))
}

func TestInsertModeEnterEmptyInput(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Should send just a newline.
	written := runner.getWritten()
	assert.Equal(t, "\n", string(written))
}

func TestInsertModeEnterWriteError(t *testing.T) {
	runner := newMockRunner(nil)
	runner.mu.Lock()
	runner.writeErr = fmt.Errorf("broken pipe")
	runner.mu.Unlock()

	p := newTestPanel(runner)
	p.Focus()
	p.mode = modeInsert

	// Type a command.
	p.Update(tea.KeyPressMsg{Code: 'x'})

	// Press Enter — write will fail but should not panic.
	result, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
	// Input buffer should still be cleared.
	assert.Empty(t, p.Input())
}

// ---------------------------------------------------------------------------
// Unfocused key handling
// ---------------------------------------------------------------------------

func TestUnfocusedIgnoresKeys(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	// Not focused.

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, modeNormal, p.Mode())
	assert.Equal(t, 0, p.Offset())
}

// ---------------------------------------------------------------------------
// Output rendering
// ---------------------------------------------------------------------------

func TestViewZeroDimensions(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	assert.Empty(t, p.View(0, 24))
	assert.Empty(t, p.View(80, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(80, -5))
}

func TestViewNilRunner(t *testing.T) {
	p := New(defaultCfg(), nil, "", nil)
	p.SetSize(80, 24)
	view := p.View(80, 24)
	assert.Contains(t, view, "No terminal")
}

func TestViewWithOutput(t *testing.T) {
	runner := newMockRunner([]string{"line 1", "line 2", "line 3"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	view := p.View(80, 24)
	assert.Contains(t, view, "line 1")
	assert.Contains(t, view, "line 2")
	assert.Contains(t, view, "line 3")
}

func TestViewStatusBarNormalMode(t *testing.T) {
	runner := newMockRunner([]string{"a", "b"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	view := p.View(80, 10)
	assert.Contains(t, view, "NORMAL")
	assert.Contains(t, view, "test-shell")
	assert.Contains(t, view, "2 lines")
}

func TestViewStatusBarInsertMode(t *testing.T) {
	runner := newMockRunner([]string{"a"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.mode = modeInsert

	view := p.View(80, 10)
	assert.Contains(t, view, "INSERT")
}

func TestViewInputPrompt(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.mode = modeInsert
	p.input = []rune("hello")

	view := p.View(80, 10)
	assert.Contains(t, view, "> hello")
}

func TestViewNoInputPromptInNormalMode(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	p.mode = modeNormal
	p.input = []rune("leftover")

	view := p.View(80, 10)
	// Should not show the input prompt.
	assert.NotContains(t, view, "> leftover")
}

func TestViewExitedProcess(t *testing.T) {
	runner := newMockRunner([]string{"done"})
	runner.setExited(0)
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	view := p.View(80, 10)
	assert.Contains(t, view, "exit=0")
}

func TestViewScrollback(t *testing.T) {
	// Create many lines to test scrolling.
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d", i)
	}
	runner := newMockRunner(lines)
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	// With height=24, status=1, content=23, we see lines 77-99 (at offset=0).
	view := p.View(80, 24)
	assert.Contains(t, view, "output line 99")
	assert.NotContains(t, view, "output line 0")

	// Scroll up to see older lines.
	p.offset = 50
	view = p.View(80, 24)
	assert.Contains(t, view, "output line 49")
	assert.NotContains(t, view, "output line 99")
}

// ---------------------------------------------------------------------------
// Tick handling
// ---------------------------------------------------------------------------

func TestTickUpdatesLines(t *testing.T) {
	runner := newMockRunner([]string{"initial"})
	p := newTestPanel(runner)
	p.Init(context.Background())

	// Simulate adding output.
	runner.addLines("new line")

	// Process a tick.
	_, cmd := p.Update(tickMsg{time: time.Now()})

	// Lines should be updated.
	assert.Contains(t, p.lines, "new line")

	// A new tick should be scheduled.
	require.NotNil(t, cmd)
}

func TestTickDetectsProcessExit(t *testing.T) {
	runner := newMockRunner([]string{"final output"})
	p := newTestPanel(runner)
	p.ticking = true

	// Mark process as exited.
	runner.setExited(42)

	_, cmd := p.Update(tickMsg{time: time.Now()})
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	exitMsg, ok := msg.(panels.TerminalExitedMsg)
	require.True(t, ok, "expected TerminalExitedMsg, got %T", msg)
	assert.Equal(t, 42, exitMsg.ExitCode)
	assert.False(t, p.ticking, "ticking should stop after process exit")
}

func TestTickNilRunner(t *testing.T) {
	p := New(defaultCfg(), nil, "", nil)
	p.ticking = true

	_, cmd := p.Update(tickMsg{time: time.Now()})
	assert.Nil(t, cmd)
	assert.False(t, p.ticking)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)

	p.Close()
	assert.True(t, runner.isClosed())
}

func TestCloseNilRunner(t *testing.T) {
	p := New(defaultCfg(), nil, "", nil)
	// Should not panic.
	p.Close()
}

// ---------------------------------------------------------------------------
// Focus / Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	runner := newMockRunner(nil)
	p := newTestPanel(runner)
	assert.False(t, p.Focused)

	p.Focus()
	assert.True(t, p.Focused)

	p.Blur()
	assert.False(t, p.Focused)
}

// ---------------------------------------------------------------------------
// KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	p := newTestPanel(newMockRunner(nil))
	bindings := p.KeyBindings()
	require.NotEmpty(t, bindings)

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
		assert.NotEmpty(t, b.Description)
		assert.NotEmpty(t, b.Action)
	}

	assert.True(t, keys["i/enter"], "should have insert mode binding")
	assert.True(t, keys["ctrl+b"], "should have prefix key binding")
	assert.True(t, keys["j/↓"], "should have scroll down binding")
	assert.True(t, keys["k/↑"], "should have scroll up binding")
	assert.True(t, keys["G"], "should have scroll to bottom binding")
	assert.True(t, keys["g"], "should have scroll to top binding")
}

func TestKeyBindingsCustomPrefixKey(t *testing.T) {
	cfg := defaultCfg()
	cfg.PrefixKey = "ctrl+a"
	p := New(cfg, newMockRunner(nil), "sh", nil)
	bindings := p.KeyBindings()

	found := false
	for _, b := range bindings {
		if b.Key == "ctrl+a" {
			found = true
			break
		}
	}
	assert.True(t, found, "should use custom prefix key in bindings")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestScrollWithFewLines(t *testing.T) {
	runner := newMockRunner([]string{"only one"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.Focus()

	// Scrolling should be no-op when content fits.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.Offset())
}

func TestViewSmallHeight(t *testing.T) {
	runner := newMockRunner([]string{"line1", "line2"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	// Height=1 means only status bar, no content area.
	view := p.View(80, 1)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "NORMAL")
}

func TestViewHeight2NormalMode(t *testing.T) {
	runner := newMockRunner([]string{"visible"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()

	// Height=2: 1 for content + 1 for status.
	view := p.View(80, 2)
	assert.Contains(t, view, "visible")
	assert.Contains(t, view, "NORMAL")
}

func TestViewHeight2InsertMode(t *testing.T) {
	runner := newMockRunner([]string{"out"})
	p := newTestPanel(runner)
	p.lines = runner.Lines()
	p.mode = modeInsert

	// Height=2: 1 for input + 1 for status (0 for content).
	view := p.View(80, 2)
	assert.Contains(t, view, "INSERT")
}
