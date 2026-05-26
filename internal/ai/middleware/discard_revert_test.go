package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Enhanced mock that tracks discard/revert/reset calls with error support
// ---------------------------------------------------------------------------

type discardRevertMock struct {
	mockGitClient

	discardFilePath string
	discardFileErr  error

	discardAllErr error

	revertHash string
	revertErr  error

	revertContinueErr error
	revertAbortErr    error

	resetRef  string
	resetMode git.ResetMode
	resetErr  error
}

func (m *discardRevertMock) DiscardFile(_ context.Context, path string) error {
	m.record("DiscardFile")
	m.discardFilePath = path
	return m.discardFileErr
}

func (m *discardRevertMock) DiscardAllUnstaged(_ context.Context) error {
	m.record("DiscardAllUnstaged")
	return m.discardAllErr
}

func (m *discardRevertMock) Revert(_ context.Context, hash string) error {
	m.record("Revert")
	m.revertHash = hash
	return m.revertErr
}

func (m *discardRevertMock) RevertContinue(_ context.Context) error {
	m.record("RevertContinue")
	return m.revertContinueErr
}

func (m *discardRevertMock) RevertAbort(_ context.Context) error {
	m.record("RevertAbort")
	return m.revertAbortErr
}

func (m *discardRevertMock) Reset(_ context.Context, ref string, mode git.ResetMode) error {
	m.record("Reset")
	m.resetRef = ref
	m.resetMode = mode
	return m.resetErr
}

func newDiscardRevertMock() *discardRevertMock {
	return &discardRevertMock{
		mockGitClient: mockGitClient{
			commitHash: "abc1234",
			calls:      make(map[string]int),
		},
	}
}

// ---------------------------------------------------------------------------
// Tests: DiscardFile
// ---------------------------------------------------------------------------

func TestDiscardFile_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	// Override the inner to use our enhanced mock.
	client.inner = inner
	ctx := context.Background()

	err := client.DiscardFile(ctx, "src/main.go")
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["DiscardFile"])
	assert.Equal(t, "src/main.go", inner.discardFilePath)
}

func TestDiscardFile_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.discardFileErr = errors.New("cannot discard: file has conflicts")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.DiscardFile(ctx, "conflict.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot discard")
	assert.Equal(t, 1, inner.calls["DiscardFile"])
}

// ---------------------------------------------------------------------------
// Tests: DiscardAllUnstaged
// ---------------------------------------------------------------------------

func TestDiscardAllUnstaged_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.DiscardAllUnstaged(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["DiscardAllUnstaged"])
}

func TestDiscardAllUnstaged_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.discardAllErr = errors.New("fatal: not a git repository")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.DiscardAllUnstaged(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

// ---------------------------------------------------------------------------
// Tests: Revert
// ---------------------------------------------------------------------------

func TestRevert_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.Revert(ctx, "abc1234")
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["Revert"])
	assert.Equal(t, "abc1234", inner.revertHash)
}

func TestRevert_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.revertErr = errors.New("revert failed: conflict")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.Revert(ctx, "deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revert failed")
	assert.Equal(t, "deadbeef", inner.revertHash)
}

// ---------------------------------------------------------------------------
// Tests: RevertContinue
// ---------------------------------------------------------------------------

func TestRevertContinue_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.RevertContinue(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["RevertContinue"])
}

func TestRevertContinue_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.revertContinueErr = errors.New("no revert in progress")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.RevertContinue(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no revert in progress")
}

// ---------------------------------------------------------------------------
// Tests: RevertAbort
// ---------------------------------------------------------------------------

func TestRevertAbort_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.RevertAbort(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["RevertAbort"])
}

func TestRevertAbort_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.revertAbortErr = errors.New("no revert in progress")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.RevertAbort(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no revert in progress")
}

// ---------------------------------------------------------------------------
// Tests: Reset
// ---------------------------------------------------------------------------

func TestReset_DelegatesToInner(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.Reset(ctx, "HEAD~1", git.ResetHard)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["Reset"])
	assert.Equal(t, "HEAD~1", inner.resetRef)
	assert.Equal(t, git.ResetHard, inner.resetMode)
}

func TestReset_PropagatesError(t *testing.T) {
	inner := newDiscardRevertMock()
	inner.resetErr = errors.New("fatal: ambiguous argument")
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.Reset(ctx, "nonexistent", git.ResetSoft)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous argument")
	assert.Equal(t, "nonexistent", inner.resetRef)
	assert.Equal(t, git.ResetSoft, inner.resetMode)
}

func TestReset_MixedMode(t *testing.T) {
	inner := newDiscardRevertMock()
	client := newTestMiddleware(&inner.mockGitClient, nil, config.AIConfig{})
	client.inner = inner
	ctx := context.Background()

	err := client.Reset(ctx, "HEAD", git.ResetMixed)
	require.NoError(t, err)
	assert.Equal(t, git.ResetMixed, inner.resetMode)
}
