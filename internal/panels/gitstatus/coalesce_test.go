package gitstatus

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jongio/grut/internal/panels"
)

// errCoalesceTest is a sentinel error used to exercise the error-result path.
var errCoalesceTest = errors.New("status load failed")

// TestRefreshGitStatus_CoalescesWhileLoading verifies that a refresh arriving
// while a status load is in flight is coalesced into a single pending reload
// rather than spawning another overlapping `git status`.
func TestRefreshGitStatus_CoalescesWhileLoading(t *testing.T) {
	mock := &mockGitClient{}
	p := newTestPanel(t, mock)

	// Simulate an in-flight load.
	p.loading = true
	p.reloadPending = false

	_, cmd := p.Update(panels.RefreshGitStatusMsg{})
	require.Nil(t, cmd, "refresh while loading must not spawn a new load")
	require.True(t, p.reloadPending, "refresh while loading must mark a pending reload")

	// A second refresh while still loading stays coalesced.
	_, cmd = p.Update(panels.RefreshGitStatusMsg{})
	require.Nil(t, cmd, "second refresh while loading must not spawn a new load")
	require.True(t, p.reloadPending)

	// Completing the in-flight load drains the pending reload exactly once.
	_, cmd = p.Update(statusLoadedMsg{files: nil, err: nil, generation: p.statusGen})
	require.False(t, p.reloadPending, "pending reload must be cleared after draining")
	require.True(t, p.loading, "draining a pending reload must start a new load")
	require.NotNil(t, cmd, "draining a pending reload must return a load command")
}

// TestRefreshGitStatus_SpawnsWhenIdle verifies that a refresh with no load in
// flight starts a load immediately.
func TestRefreshGitStatus_SpawnsWhenIdle(t *testing.T) {
	mock := &mockGitClient{}
	p := newTestPanel(t, mock)
	p.loading = false

	_, cmd := p.Update(panels.RefreshGitStatusMsg{})
	require.True(t, p.loading, "refresh while idle must start a load")
	require.NotNil(t, cmd, "refresh while idle must return a load command")
	require.False(t, p.reloadPending)
}

// TestRefreshGitStatus_DrainsPendingOnErrorResult verifies that a pending
// reload is still drained when the in-flight load returns an error, so a
// coalesced refresh is never lost.
func TestRefreshGitStatus_DrainsPendingOnErrorResult(t *testing.T) {
	mock := &mockGitClient{}
	p := newTestPanel(t, mock)

	p.loading = true
	_, _ = p.Update(panels.RefreshGitStatusMsg{})
	require.True(t, p.reloadPending)

	_, cmd := p.Update(statusLoadedMsg{files: nil, err: errCoalesceTest, generation: p.statusGen})
	require.False(t, p.reloadPending, "pending reload must be drained even on error")
	require.True(t, p.loading, "draining after an error must start a new load")
	require.NotNil(t, cmd)
}
