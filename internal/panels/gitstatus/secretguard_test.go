package gitstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Assembled from split literals so no contiguous token appears in source.
var secretContent = "token = ghp_" + "0123456789abcdefghijklmnopqrstuvwxyz\n"

func guardedPanel(t *testing.T, mode string, content []byte, readErr error) (*GitStatus, *mockGitClient) {
	t.Helper()
	mock := newMockGitClient(mockGitClientOptions{})
	mock.WorktreeFileFunc = func(_ context.Context, _ string) ([]byte, error) {
		return content, readErr
	}
	p := newTestPanel(t, mock)
	p.SetGitCfg(config.GitConfig{SecretGuard: true, SecretGuardMode: mode})
	return p, mock
}

func TestSecretGuard_DisabledStagesDirectly(t *testing.T) {
	mock := newMockGitClient(mockGitClientOptions{})
	p := newTestPanel(t, mock)
	// Guard defaults to disabled (SetGitCfg not called).
	_, cmd := p.stage([]string{"a.txt"})
	require.NotNil(t, cmd)
	cmd() // executes Stage on the mock
	assert.Equal(t, []string{"a.txt"}, mock.stagedPaths)
}

func TestSecretGuard_NoFindingsStages(t *testing.T) {
	p, mock := guardedPanel(t, "warn", []byte("just some ordinary code\n"), nil)
	_, cmd := p.stage([]string{"clean.go"})
	require.NotNil(t, cmd)
	msg := cmd()
	_, next := p.Update(msg)
	require.NotNil(t, next)
	next()
	assert.Equal(t, []string{"clean.go"}, mock.stagedPaths)
	assert.Empty(t, p.pendingOp)
}

func TestSecretGuard_WarnPromptsThenStagesOnConfirm(t *testing.T) {
	p, mock := guardedPanel(t, "warn", []byte(secretContent), nil)
	_, cmd := p.stage([]string{"secrets.txt"})
	require.NotNil(t, cmd)
	_, next := p.Update(cmd())
	require.NotNil(t, next)
	next() // ShowConfirm command; harmless to run

	assert.Empty(t, mock.stagedPaths, "must not stage before confirmation")
	assert.Equal(t, opStageGuard, p.pendingOp)
	assert.Equal(t, []string{"secrets.txt"}, p.pendingStagePaths)

	_, stageCmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, stageCmd)
	stageCmd()
	assert.Equal(t, []string{"secrets.txt"}, mock.stagedPaths)
	assert.Empty(t, p.pendingOp, "pending state cleared after confirm")
}

func TestSecretGuard_WarnCancelDoesNotStage(t *testing.T) {
	p, mock := guardedPanel(t, "warn", []byte(secretContent), nil)
	_, cmd := p.stage([]string{"secrets.txt"})
	require.NotNil(t, cmd)
	p.Update(cmd())

	p.Update(notify.ModalResultMsg{Accept: false})
	assert.Empty(t, mock.stagedPaths, "cancel must not stage")
	assert.Empty(t, p.pendingOp)
}

func TestSecretGuard_BlockRefuses(t *testing.T) {
	p, mock := guardedPanel(t, "block", []byte(secretContent), nil)
	_, cmd := p.stage([]string{"secrets.txt"})
	require.NotNil(t, cmd)
	_, next := p.Update(cmd())
	require.NotNil(t, next)
	toast, ok := next().(notify.ShowToastMsg)
	require.True(t, ok, "block mode should emit a toast")
	assert.Equal(t, notify.Error, toast.Level)
	assert.Empty(t, mock.stagedPaths, "block mode must never stage")
	assert.Empty(t, p.pendingOp, "block mode does not arm a confirmation")
}

func TestSecretGuard_ScanErrorCancelsStage(t *testing.T) {
	p, mock := guardedPanel(t, "warn", nil, errors.New("read failed"))
	_, cmd := p.stage([]string{"gone.txt"})
	require.NotNil(t, cmd)
	_, next := p.Update(cmd())
	require.NotNil(t, next)
	toast, ok := next().(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Empty(t, mock.stagedPaths, "scan error must not silently stage")
}

func TestSecretGuard_SensitiveFilenameFlagged(t *testing.T) {
	// Content is clean, but the filename is sensitive.
	p, mock := guardedPanel(t, "warn", []byte("nothing here\n"), nil)
	_, cmd := p.stage([]string{".env"})
	require.NotNil(t, cmd)
	p.Update(cmd())
	assert.Equal(t, opStageGuard, p.pendingOp, "sensitive filename should prompt")
	assert.Empty(t, mock.stagedPaths)
}

func TestSecretGuard_EmptyPathsNoop(t *testing.T) {
	p, _ := guardedPanel(t, "warn", []byte(secretContent), nil)
	_, cmd := p.stage(nil)
	assert.Nil(t, cmd)
}
