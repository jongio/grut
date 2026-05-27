package panelreg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepsCwd(t *testing.T) {
	d := Deps{}
	cwd := d.Cwd()

	// Must return an absolute, existing directory.
	assert.True(t, filepath.IsAbs(cwd))
	info, err := os.Stat(cwd)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDepsNewGitClientInRepo(t *testing.T) {
	// The worktree root is a valid git repo, so NewGitClient should succeed
	// when the working directory is inside one.
	d := Deps{}
	client, cwd, err := d.NewGitClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotEmpty(t, cwd)
}

func TestDepsNewGitClientNonGitDir(t *testing.T) {
	// Switch to a temp dir that is NOT a git repo.
	// NewGitClient still succeeds (it validates the dir exists, not that it's a repo).
	tmp := t.TempDir()
	// Resolve symlinks so path matches os.Getwd() on macOS (/var → /private/var).
	tmp, err := filepath.EvalSymlinks(tmp)
	require.NoError(t, err)
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	d := Deps{}
	client, cwd, err := d.NewGitClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, tmp, cwd)
}

// mockPanel is a plain panels.Panel that does NOT implement SetActionsCfg.
type mockPanel struct {
	panels.BasePanel
}

func (m *mockPanel) Init(_ context.Context) tea.Cmd           { return nil }
func (m *mockPanel) Update(_ tea.Msg) (panels.Panel, tea.Cmd) { return m, nil }
func (m *mockPanel) View(_, _ int) string                     { return "" }

// mockPanelWithActions implements the optional SetActionsCfg interface.
type mockPanelWithActions struct {
	panels.BasePanel
	applied bool
	cfg     config.ActionsConfig
}

func (m *mockPanelWithActions) Init(_ context.Context) tea.Cmd           { return nil }
func (m *mockPanelWithActions) Update(_ tea.Msg) (panels.Panel, tea.Cmd) { return m, nil }
func (m *mockPanelWithActions) View(_, _ int) string                     { return "" }

func (m *mockPanelWithActions) SetActionsCfg(c config.ActionsConfig) {
	m.applied = true
	m.cfg = c
}

func TestDepsApplyActionsCfgWithInterface(t *testing.T) {
	cfg := config.ActionsConfig{
		DoubleClick: map[string]string{"go": "edit"},
	}
	d := Deps{Config: &config.Config{Actions: cfg}}

	p := &mockPanelWithActions{}
	result := d.ApplyActionsCfg(p)

	assert.True(t, p.applied, "SetActionsCfg should have been called")
	assert.Equal(t, cfg, p.cfg)
	assert.Same(t, p, result, "ApplyActionsCfg must return the same panel")
}

func TestDepsApplyActionsCfgWithoutInterface(t *testing.T) {
	d := Deps{Config: &config.Config{}}

	p := &mockPanel{BasePanel: panels.BasePanel{PanelTitle: "plain"}}
	result := d.ApplyActionsCfg(p)

	// Panel without SetActionsCfg is returned unchanged.
	assert.Same(t, p, result)
}

func TestDepsPlaceholder(t *testing.T) {
	th := &theme.Theme{}
	d := Deps{Theme: th}

	p := d.Placeholder("test-panel")
	assert.Equal(t, "test-panel", p.Title())
}

func TestDepsPlaceholderNilTheme(t *testing.T) {
	d := Deps{Theme: nil}

	p := d.Placeholder("nil-theme")
	assert.Equal(t, "nil-theme", p.Title())
}
