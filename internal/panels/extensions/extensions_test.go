package extensions

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/extension"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock extension manager
// ---------------------------------------------------------------------------

type mockExtManager struct {
	extensions []extension.ExtensionInfo

	enabledName  string
	disabledName string
	removedName  string
	installedSrc string

	enableErr  error
	disableErr error
	removeErr  error
	installErr error
}

var _ extManager = (*mockExtManager)(nil)

func (m *mockExtManager) List() []extension.ExtensionInfo {
	return m.extensions
}

func (m *mockExtManager) Enable(name string) error {
	m.enabledName = name
	return m.enableErr
}

func (m *mockExtManager) Disable(name string) error {
	m.disabledName = name
	return m.disableErr
}

func (m *mockExtManager) Remove(name string) error {
	m.removedName = name
	return m.removeErr
}

func (m *mockExtManager) Install(_ context.Context, source string) error {
	m.installedSrc = source
	return m.installErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func newTestPanel(t *testing.T, mock *mockExtManager) *Panel {
	t.Helper()
	p := New(mock, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return a command")
	// Execute the load command synchronously.
	msg := cmd()
	p.Update(msg)
	return p
}

func makeExt(name, version, runtime string, enabled bool) extension.ExtensionInfo {
	return extension.ExtensionInfo{
		Manifest: extension.Manifest{
			Name:    name,
			Version: version,
			Runtime: runtime,
		},
		Enabled: enabled,
	}
}

func makeExtFull(name, version, runtime, author, desc, entry string, perms []string, enabled bool) extension.ExtensionInfo {
	return extension.ExtensionInfo{
		Manifest: extension.Manifest{
			Name:        name,
			Version:     version,
			Runtime:     runtime,
			Author:      author,
			Description: desc,
			EntryPoint:  entry,
			Permissions: perms,
		},
		Enabled: enabled,
	}
}

// ---------------------------------------------------------------------------
// Tests: Panel creation and interface compliance
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)
	assert.Equal(t, "extensions", p.Title())
	assert.NotNil(t, p.expanded)
}

func TestInterfaceCompliance(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)

	// Verify Panel interface.
	var _ panels.Panel = p
}

func TestInit_ReturnsLoadCmd(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("test-ext", "1.0.0", "lua", true),
		},
	}
	p := New(mock, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Rendering with 0, 1, N extensions
// ---------------------------------------------------------------------------

func TestView_ZeroDimensions(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(0, 10))
}

func TestView_NoExtensions(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	view := p.View(80, 24)
	assert.Contains(t, view, "No extensions installed")
}

func TestView_Loading(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)
	p.loading = true
	view := p.View(80, 24)
	assert.Contains(t, view, "Loading extensions")
}

func TestView_OneExtension_Enabled(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExtFull("my-ext", "1.2.3", "lua", "Alice", "A cool extension", "main.lua",
				[]string{"file_read"}, true),
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "my-ext")
	assert.Contains(t, view, "v1.2.3")
	assert.Contains(t, view, "✓") // enabled indicator
	assert.Contains(t, view, "λ") // lua icon
	assert.Contains(t, view, "by Alice")
}

func TestView_OneExtension_Disabled(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("disabled-ext", "0.1.0", "wasm", false),
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "disabled-ext")
	assert.Contains(t, view, "✗") // disabled indicator
	assert.Contains(t, view, "◈") // wasm icon
}

func TestView_MultipleExtensions(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0.0", "lua", true),
			makeExt("beta", "2.0.0", "wasm", false),
			makeExt("gamma", "3.0.0", "mcp", true),
		},
	}
	p := newTestPanel(t, mock)
	view := p.View(120, 24)

	assert.Contains(t, view, "alpha")
	assert.Contains(t, view, "beta")
	assert.Contains(t, view, "gamma")
}

// ---------------------------------------------------------------------------
// Tests: Extension details (runtime icons, enabled/disabled indicators)
// ---------------------------------------------------------------------------

func TestRuntimeIcons(t *testing.T) {
	tests := []struct {
		runtime string
		icon    string
	}{
		{"lua", "λ"},
		{"wasm", "◈"},
		{"mcp", "⟡"},
		{"unknown", "○"},
	}
	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			assert.Equal(t, tt.icon, runtimeIcon(tt.runtime))
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: Navigation (j/k)
// ---------------------------------------------------------------------------

func TestNavigation_JK(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0.0", "lua", true),
			makeExt("beta", "2.0.0", "wasm", true),
			makeExt("gamma", "3.0.0", "mcp", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	assert.Equal(t, 0, p.cursor)

	// Move down.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// At bottom, should not go further.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// Move up.
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursor)

	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)

	// At top, should not go further.
	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)
}

func TestNavigation_ArrowKeys(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0.0", "lua", true),
			makeExt("beta", "2.0.0", "wasm", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Tests: Expand/collapse details
// ---------------------------------------------------------------------------

func TestExpand_ToggleDetails(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExtFull("my-ext", "1.0.0", "lua", "Bob", "A test extension", "init.lua",
				[]string{"file_read", "network"}, true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(120, 24)

	// Press Enter to expand.
	p.Update(keyMsg('\r'))
	assert.True(t, p.expanded["my-ext"])

	// View should include detail lines.
	view := p.View(120, 24)
	assert.Contains(t, view, "A test extension")
	assert.Contains(t, view, "Runtime: lua")
	assert.Contains(t, view, "Entry: init.lua")
	assert.Contains(t, view, "file_read")
	assert.Contains(t, view, "network")

	// Press Enter again to collapse.
	p.Update(keyMsg('\r'))
	assert.False(t, p.expanded["my-ext"])
}

func TestExpand_NoPermissions(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExtFull("simple", "1.0.0", "lua", "", "", "main.lua", nil, true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(120, 24)

	p.Update(keyMsg('\r'))
	view := p.View(120, 24)
	assert.Contains(t, view, "Permissions: none")
}

func TestExpand_EmptyList(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('\r'))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Enable/disable toggle
// ---------------------------------------------------------------------------

func TestToggleEnable_EnablesDisabledExtension(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", false),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'e' to toggle (disabled → enable).
	_, cmd := p.Update(keyMsg('e'))
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, extensionToggleResultMsg{}, msg)
	assert.Equal(t, "my-ext", mock.enabledName)
}

func TestToggleEnable_DisablesEnabledExtension(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'e' to toggle (enabled → disable).
	_, cmd := p.Update(keyMsg('e'))
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, extensionToggleResultMsg{}, msg)
	assert.Equal(t, "my-ext", mock.disabledName)
}

func TestToggleEnable_EmptyList(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('e'))
	assert.Nil(t, cmd)
}

func TestToggleEnable_Error(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", false),
		},
		enableErr: fmt.Errorf("enable failed"),
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('e'))
	require.NotNil(t, cmd)

	// Execute toggle command.
	msg := cmd()
	result, ok := msg.(extensionToggleResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)

	// Feed result back to panel — should produce a toast.
	_, cmd = p.Update(result)
	require.NotNil(t, cmd)
	toastMsg := cmd()
	toast, ok := toastMsg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Toggle failed")
}

// ---------------------------------------------------------------------------
// Tests: Remove with confirmation
// ---------------------------------------------------------------------------

func TestRemove_ShowsConfirmation(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'd' to request remove — should show confirmation.
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)
	assert.Equal(t, opRemove, p.pending)
	assert.Equal(t, "my-ext", p.pendingName)
}

func TestRemove_Confirmed(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Request remove.
	p.Update(keyMsg('d'))

	// Confirm.
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	// Execute the remove command.
	msg := cmd()
	require.IsType(t, extensionRemoveResultMsg{}, msg)
	assert.Equal(t, "my-ext", mock.removedName)
}

func TestRemove_Cancelled(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Request remove.
	p.Update(keyMsg('d'))

	// Cancel.
	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, "", mock.removedName)
}

func TestRemove_EmptyList(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('d'))
	assert.Nil(t, cmd)
}

func TestRemove_Error(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("my-ext", "1.0.0", "lua", true),
		},
		removeErr: fmt.Errorf("remove failed"),
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.Update(keyMsg('d'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extensionRemoveResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)

	// Feed result back — should produce a toast.
	_, cmd = p.Update(result)
	require.NotNil(t, cmd)
	toastMsg := cmd()
	toast, ok := toastMsg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Remove failed")
}

// ---------------------------------------------------------------------------
// Tests: Install flow
// ---------------------------------------------------------------------------

func TestInstall_ShowsInput(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Press 'i' to request install — should show input prompt.
	_, cmd := p.Update(keyMsg('i'))
	require.NotNil(t, cmd)
	assert.Equal(t, opInstall, p.pending)
}

func TestInstall_Confirmed(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Request install.
	p.Update(keyMsg('i'))

	// Provide URL.
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "https://github.com/user/ext"})
	require.NotNil(t, cmd)

	// Execute the install command.
	msg := cmd()
	require.IsType(t, extensionInstallResultMsg{}, msg)
	assert.Equal(t, "https://github.com/user/ext", mock.installedSrc)
}

func TestInstall_Cancelled(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.Update(keyMsg('i'))

	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, "", mock.installedSrc)
}

func TestInstall_EmptyValue(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.Update(keyMsg('i'))

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: ""})
	assert.Nil(t, cmd)
	assert.Equal(t, "", mock.installedSrc)
}

func TestInstall_Error(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{},
		installErr: fmt.Errorf("install failed"),
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	p.Update(keyMsg('i'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "https://example.com/ext"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extensionInstallResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)

	_, cmd = p.Update(result)
	require.NotNil(t, cmd)
	toastMsg := cmd()
	toast, ok := toastMsg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Install failed")
}

// ---------------------------------------------------------------------------
// Tests: Refresh
// ---------------------------------------------------------------------------

func TestRefresh(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("test", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('R'))
	require.NotNil(t, cmd)
	assert.True(t, p.loading)
}

// ---------------------------------------------------------------------------
// Tests: ExtensionChangedMsg triggers refresh
// ---------------------------------------------------------------------------

func TestExtensionChangedMsg_TriggersRefresh(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(panels.ExtensionChangedMsg{})
	require.NotNil(t, cmd, "ExtensionChangedMsg should return a load command")
}

// ---------------------------------------------------------------------------
// Tests: KeyBindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	assert.True(t, keys["j/↓"])
	assert.True(t, keys["k/↑"])
	assert.True(t, keys["enter"])
	assert.True(t, keys["e"])
	assert.True(t, keys["d"])
	assert.True(t, keys["i"])
	assert.True(t, keys["R"])
}

// ---------------------------------------------------------------------------
// Tests: Focus/Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	mock := &mockExtManager{}
	p := New(mock, nil)

	assert.False(t, p.Focused)
	p.Focus()
	assert.True(t, p.Focused)
	p.Blur()
	assert.False(t, p.Focused)
}

// ---------------------------------------------------------------------------
// Tests: Unfocused key ignore
// ---------------------------------------------------------------------------

func TestUnfocused_IgnoresKeys(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("test", "1.0.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	// Don't focus the panel.
	p.SetSize(80, 24)

	_, cmd := p.Update(keyMsg('j'))
	assert.Nil(t, cmd)
	assert.Equal(t, 0, p.cursor) // cursor should not move

	_, cmd = p.Update(keyMsg('e'))
	assert.Nil(t, cmd)

	_, cmd = p.Update(keyMsg('d'))
	assert.Nil(t, cmd)

	_, cmd = p.Update(keyMsg('i'))
	assert.Nil(t, cmd)

	_, cmd = p.Update(keyMsg('R'))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Cursor clamping
// ---------------------------------------------------------------------------

func TestCursorClamp_EmptyList(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)
	assert.Equal(t, 0, p.cursor)
}

func TestCursorClamp_AfterRemoval(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0.0", "lua", true),
			makeExt("beta", "2.0.0", "wasm", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	// Move to last item.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	// Simulate list shrinking.
	mock.extensions = []extension.ExtensionInfo{
		makeExt("alpha", "1.0.0", "lua", true),
	}
	msg := p.loadExtensionsCmd()()
	p.Update(msg)

	// Cursor should be clamped.
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Tests: Sorted output
// ---------------------------------------------------------------------------

func TestExtensions_SortedByName(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("zeta", "1.0.0", "lua", true),
			makeExt("alpha", "1.0.0", "wasm", true),
			makeExt("mid", "1.0.0", "mcp", true),
		},
	}
	p := newTestPanel(t, mock)

	// After loading, extensions should be sorted.
	assert.Equal(t, "alpha", p.extensions[0].Manifest.Name)
	assert.Equal(t, "mid", p.extensions[1].Manifest.Name)
	assert.Equal(t, "zeta", p.extensions[2].Manifest.Name)
}

// ---------------------------------------------------------------------------
// Tests: Toggle result triggers refresh and ExtensionChangedMsg
// ---------------------------------------------------------------------------

func TestToggleResult_Success_TriggersRefreshAndChanged(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(extensionToggleResultMsg{name: "test", err: nil})
	require.NotNil(t, cmd, "successful toggle should return batch command")
}

func TestRemoveResult_Success_TriggersRefreshAndChanged(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(extensionRemoveResultMsg{name: "test", err: nil})
	require.NotNil(t, cmd, "successful remove should return batch command")
}

func TestInstallResult_Success_TriggersRefreshAndChanged(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)

	_, cmd := p.Update(extensionInstallResultMsg{source: "https://example.com", err: nil})
	require.NotNil(t, cmd, "successful install should return batch command")
}

// ---------------------------------------------------------------------------
// Tests: Modal result with no pending operation
// ---------------------------------------------------------------------------

func TestModalResult_NoPendingOp(t *testing.T) {
	mock := &mockExtManager{extensions: []extension.ExtensionInfo{}}
	p := newTestPanel(t, mock)

	// Accept with no pending op should do nothing.
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "something"})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Mouse click
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsExtension(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0", "lua", true),
			makeExt("beta", "2.0", "wasm", false),
			makeExt("gamma", "3.0", "mcp", true),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)

	assert.Equal(t, 0, p.cursor)

	// Click on row 1 → selects second extension.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)

	// Click on row 2 → selects third extension.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 24)

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "out-of-bounds click should not move cursor")
}

// ---------------------------------------------------------------------------
// Tests: Mouse double-click
// ---------------------------------------------------------------------------

func TestMouseDoubleClick_TogglesExpand(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0", "lua", true),
			makeExt("beta", "2.0", "wasm", false),
		},
	}
	p := newTestPanel(t, mock)
	p.Focus()
	p.SetSize(80, 24)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemExtension): true}

	// Double-click on row 1 → selects and expands "beta".
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)
	assert.True(t, p.expanded["beta"], "double-click should expand extension")
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 24)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "out-of-bounds double-click should not trigger action")
}

// ---------------------------------------------------------------------------
// Tests: Mouse wheel
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollDown(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("a", "1.0", "lua", true),
			makeExt("b", "1.0", "lua", true),
			makeExt("c", "1.0", "lua", true),
			makeExt("d", "1.0", "lua", true),
			makeExt("e", "1.0", "lua", true),
			makeExt("f", "1.0", "lua", true),
			makeExt("g", "1.0", "lua", true),
			makeExt("h", "1.0", "lua", true),
			makeExt("i", "1.0", "lua", true),
			makeExt("j", "1.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 3) // Small viewport to allow scrolling.

	assert.Equal(t, 0, p.offset)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 3, p.offset, "should scroll down by delta of 3")
}

func TestMouseWheel_ScrollUp(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("a", "1.0", "lua", true),
			makeExt("b", "1.0", "lua", true),
			makeExt("c", "1.0", "lua", true),
			makeExt("d", "1.0", "lua", true),
			makeExt("e", "1.0", "lua", true),
			makeExt("f", "1.0", "lua", true),
			makeExt("g", "1.0", "lua", true),
			makeExt("h", "1.0", "lua", true),
			makeExt("i", "1.0", "lua", true),
			makeExt("j", "1.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 3)

	// Scroll down first.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Greater(t, p.offset, 0)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should scroll back to top")
}

func TestMouseWheel_ScrollUpClampsToZero(t *testing.T) {
	mock := &mockExtManager{
		extensions: []extension.ExtensionInfo{
			makeExt("alpha", "1.0", "lua", true),
		},
	}
	p := newTestPanel(t, mock)
	p.SetSize(80, 24)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "should not scroll below 0")
}
