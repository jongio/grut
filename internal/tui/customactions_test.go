package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/customactions"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCustomActionDispatchInvokesRunner(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	cfg.CustomActions = []config.CustomAction{{
		Name:    "Test",
		Command: "go test ./...",
		WorkDir: "services/api",
	}}
	m := newTestModel(t).WithConfig(cfg)

	oldRunner := runCustomAction
	defer func() { runCustomAction = oldRunner }()
	var gotAction config.CustomAction
	var gotDir string
	runCustomAction = func(_ context.Context, action config.CustomAction, defaultDir, _ string) customactions.Result {
		gotAction = action
		gotDir = defaultDir
		return customactions.Result{Action: action, Output: "ok"}
	}

	updated, cmd := m.Update(panels.RunCustomActionMsg{Name: "Test"})
	m = updated.(Model)

	require.NotNil(t, cmd)
	assert.Equal(t, "running Test...", m.asyncOp)
	msg := cmd()
	done, ok := msg.(customActionDoneMsg)
	require.True(t, ok)
	assert.Equal(t, "Test", gotAction.Name)
	assert.Equal(t, "services/api", gotAction.WorkDir)
	assert.NotEmpty(t, gotDir)
	assert.Equal(t, "ok", done.result.Output)
}

func TestRunCustomActionConfirmGatesRunner(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	cfg.CustomActions = []config.CustomAction{{
		Name:    "Deploy",
		Command: "make deploy",
		Prompt:  "Deploy now?",
		Confirm: true,
	}}
	m := newTestModel(t).WithConfig(cfg)

	updated, cmd := m.Update(panels.RunCustomActionMsg{Name: "Deploy"})
	m = updated.(Model)

	require.NotNil(t, cmd)
	assert.Equal(t, pendingActionCustom, m.pendingAction)
	assert.Equal(t, "Deploy", m.pendingCustomAction)
	assert.Empty(t, m.asyncOp)
	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Equal(t, "Run Deploy?", modal.Title)
	assert.Equal(t, "Deploy now?", modal.Message)
}

func TestCustomActionConfirmAcceptRunsAction(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	cfg.CustomActions = []config.CustomAction{{
		Name:    "Clean",
		Command: "git clean -fd",
		Confirm: true,
	}}
	m := newTestModel(t).WithConfig(cfg)
	m.pendingAction = pendingActionCustom
	m.pendingCustomAction = "Clean"

	oldRunner := runCustomAction
	defer func() { runCustomAction = oldRunner }()
	called := false
	runCustomAction = func(_ context.Context, action config.CustomAction, _, _ string) customactions.Result {
		called = true
		return customactions.Result{Action: action}
	}

	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true})
	m = updated.(Model)

	require.NotNil(t, cmd)
	assert.Equal(t, "running Clean...", m.asyncOp)
	_ = cmd()
	assert.True(t, called)
}

func TestCustomActionKeyDispatchesAction(t *testing.T) {
	cfg, err := config.LoadDefaults()
	require.NoError(t, err)
	cfg.CustomActions = []config.CustomAction{{
		Name:    "Generate",
		Command: "go generate ./...",
		Key:     "ctrl+g",
	}}
	m := newTestModel(t).WithConfig(cfg)
	m.keys = nil

	oldRunner := runCustomAction
	defer func() { runCustomAction = oldRunner }()
	called := false
	runCustomAction = func(_ context.Context, action config.CustomAction, _, _ string) customactions.Result {
		called = action.Name == "Generate"
		return customactions.Result{Action: action}
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(Model)

	require.NotNil(t, cmd)
	assert.Equal(t, "running Generate...", m.asyncOp)
	_ = cmd()
	assert.True(t, called)
}
