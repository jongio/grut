package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

// mockGitOps implements git.GitClient with configurable return values for
// the operations used by gitops.go. Unexposed methods panic if called
// (via the embedded nil interface).
type mockGitOps struct {
	git.GitClient // embedded nil — panics on any unused method

	commitHash    string
	commitErr     error
	pushErr       error
	pullErr       error
	fetchErr      error
	discardErr    error
	unstageErr    error
	repoRoot      string
	repoRootErr   error
	commitCalls   int
	pushCalls     int
	pullCalls     int
	fetchCalls    int
	discardCalls  int
	unstageCalls  int
	repoRootCalls int
	discardPath   string
	unstagePaths  []string
}

func (m *mockGitOps) Commit(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
	m.commitCalls++
	return m.commitHash, m.commitErr
}

func (m *mockGitOps) Push(_ context.Context, _ git.PushOpts) error {
	m.pushCalls++
	return m.pushErr
}

func (m *mockGitOps) Pull(_ context.Context, _ git.PullOpts) error {
	m.pullCalls++
	return m.pullErr
}

func (m *mockGitOps) Fetch(_ context.Context, _ git.FetchOpts) error {
	m.fetchCalls++
	return m.fetchErr
}

func (m *mockGitOps) RepoRoot(_ context.Context) (string, error) {
	m.repoRootCalls++
	if m.repoRoot != "" {
		return m.repoRoot, m.repoRootErr
	}
	return "/repo", m.repoRootErr
}

func (m *mockGitOps) DiscardFile(_ context.Context, path string) error {
	m.discardCalls++
	m.discardPath = path
	return m.discardErr
}

func (m *mockGitOps) Unstage(_ context.Context, paths []string) error {
	m.unstageCalls++
	m.unstagePaths = paths
	return m.unstageErr
}

// newTestModelWithGit creates a test model with a mock git client and config.
func newTestModelWithGit(t *testing.T, mock *mockGitOps) Model {
	t.Helper()
	m := newTestModel(t)
	m = m.WithGitClient(mock)
	m = m.WithConfig(&config.Config{})
	return m
}

// ---------------------------------------------------------------------------
// Commit tests
// ---------------------------------------------------------------------------

func TestCommitActionShowsModal(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Trigger commit action.
	updated, cmd := m.handleAction("commit", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, "commit", m.pendingAction, "pendingAction should be set")
	assert.NotNil(t, cmd, "should return a command to show the modal")
}

func TestCommitActionWithoutGitShowsToast(t *testing.T) {
	m := newTestModel(t) // no git client
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("commit", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should not be set")
	assert.NotNil(t, cmd, "should return a toast command")

	// Verify the toast message.
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "command should produce ShowToastMsg")
	assert.Contains(t, toast.Message, "Git not available")
	assert.Equal(t, notify.Warn, toast.Level)
}

func TestCommitModalAcceptExecutesCommit(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	m.pendingAction = "commit"

	// Simulate modal accept with commit message.
	updated, cmd := m.Update(notify.ModalResultMsg{Accept: true, Value: "fix: test"})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
	assert.Equal(t, "committing...", m.asyncOp, "asyncOp should be set")
	assert.NotNil(t, m.asyncCancel, "asyncCancel should be set")
	assert.NotNil(t, cmd, "should return a command for the commit")
}

func TestCommitModalCancelClearsPendingAction(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	m.pendingAction = "commit"

	// Simulate modal cancel.
	updated, cmd := m.Update(notify.ModalResultMsg{Accept: false})
	m = updated.(Model)

	assert.Empty(t, m.pendingAction, "pendingAction should be cleared")
	assert.Empty(t, m.asyncOp, "no async op should start")
	assert.Nil(t, cmd, "no command on cancel")
}

func TestCommitEmptyMessageShowsWarning(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)

	updated, cmd := m.executeCommit("")
	_ = updated.(Model)

	assert.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Empty commit message")
	assert.Equal(t, notify.Warn, toast.Level)
}

func TestCommitRequestMsgTriggersCommit(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Send CommitRequestMsg.
	updated, cmd := m.Update(panels.CommitRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, "commit", m.pendingAction)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Push tests
// ---------------------------------------------------------------------------

func TestPushAction(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("push", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpPushing, m.asyncOp)
	assert.NotNil(t, m.asyncCancel)
	assert.NotNil(t, cmd)
}

func TestPushWhileAsyncOpRunningShowsWarning(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	m.asyncOp = "committing..."

	updated, cmd := m.handleAction("push", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, "committing...", m.asyncOp, "asyncOp should not change")
	assert.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Operation in progress")
}

func TestPushRequestMsg(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.PushRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpPushing, m.asyncOp)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Pull tests
// ---------------------------------------------------------------------------

func TestPullAction(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("pull", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, "pulling...", m.asyncOp)
	assert.NotNil(t, m.asyncCancel)
	assert.NotNil(t, cmd)
}

func TestPullRequestMsg(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.PullRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, "pulling...", m.asyncOp)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Fetch tests
// ---------------------------------------------------------------------------

func TestFetchAction(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleAction("fetch", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpFetching, m.asyncOp)
	assert.NotNil(t, m.asyncCancel)
	assert.NotNil(t, cmd)
}

func TestFetchRequestMsg(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.Update(panels.FetchRequestMsg{})
	m = updated.(Model)

	assert.Equal(t, asyncOpFetching, m.asyncOp)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// AsyncOpDoneMsg tests
// ---------------------------------------------------------------------------

func TestAsyncOpDoneSuccess(t *testing.T) {
	m := newTestModel(t)
	m.asyncOp = asyncOpPushing
	m.asyncCancel = func() {}

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{Description: "push", Err: nil})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp, "asyncOp should be cleared")
	assert.Nil(t, m.asyncCancel, "asyncCancel should be cleared")
	assert.NotNil(t, cmd, "should return batch command with toast and refresh")
}

func TestAsyncOpDoneError(t *testing.T) {
	m := newTestModel(t)
	m.asyncOp = asyncOpPushing
	m.asyncCancel = func() {}

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{
		Description: "push",
		Err:         fmt.Errorf("rejected: non-fast-forward"),
	})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp)
	assert.NotNil(t, cmd)
}

func TestAsyncOpDoneCancelled(t *testing.T) {
	m := newTestModel(t)
	m.asyncOp = asyncOpPushing
	m.asyncCancel = func() {}

	updated, cmd := m.Update(panels.AsyncOpDoneMsg{
		Description: "push",
		Err:         context.Canceled,
	})
	m = updated.(Model)

	assert.Empty(t, m.asyncOp)
	assert.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "cancelled")
	assert.Equal(t, notify.Info, toast.Level)
}

// ---------------------------------------------------------------------------
// Esc cancellation tests
// ---------------------------------------------------------------------------

func TestEscCancelsAsyncOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	cancelled := false
	m.asyncOp = asyncOpPushing
	m.asyncCancel = func() { cancelled = true }

	// Press Esc.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	assert.True(t, cancelled, "cancel function should have been called")
	assert.Empty(t, m.asyncOp, "asyncOp should be cleared")
	assert.Nil(t, m.asyncCancel, "asyncCancel should be cleared")
	assert.NotNil(t, cmd, "should return a toast command")
}

func TestEscDoesNotCancelWithoutAsyncOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	// Press Esc with no async op — should forward to panel.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_ = updated.(Model)
	// Should not panic and should not produce a cancel toast.
}

// ---------------------------------------------------------------------------
// Status bar tests
// ---------------------------------------------------------------------------

func TestStatusBarShowsAsyncOp(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	m.asyncOp = asyncOpPushing
	bar := m.renderStatusBar()

	assert.Contains(t, bar, asyncOpPushing, "status bar should show async op")
}

func TestStatusBarHidesAsyncOpWhenEmpty(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.Init()

	m.asyncOp = ""
	bar := m.renderStatusBar()

	assert.NotContains(t, bar, "⟳", "status bar should not show spinner when no async op")
}

// executeBatchCmd runs a tea.Cmd and, if it returns a tea.BatchMsg, executes
// each inner command (with a short timeout to skip blocking ones). Returns
// the collected messages.
func executeBatchCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()
		select {
		case m := <-ch:
			if m != nil {
				msgs = append(msgs, m)
			}
		case <-time.After(100 * time.Millisecond):
			// skip blocking commands
		}
	}
	return msgs
}

func executeFirstBatchCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "expected batch command")
	require.NotEmpty(t, batch)
	require.NotNil(t, batch[0])
	return batch[0]()
}

func TestBranchChangedMsgUpdatesStatusBar(t *testing.T) {
	m := newTestModelReady(t)

	// Simulate initial branch load (generation=0).
	updated, _ := m.Update(branchLoadedMsg{Name: "main", generation: 0})
	m = updated.(Model)
	assert.Equal(t, "main", m.currentBranch)

	// Simulate checkout → BranchChangedMsg.
	updated, _ = m.Update(panels.BranchChangedMsg{Name: "feature"})
	m = updated.(Model)
	assert.Equal(t, "feature", m.currentBranch, "BranchChangedMsg should update currentBranch")
	assert.Equal(t, uint64(1), m.branchInfoGen, "BranchChangedMsg should bump generation")

	bar := m.renderStatusBar()
	assert.Contains(t, bar, "feature", "status bar should show new branch after checkout")
	assert.NotContains(t, bar, "main", "status bar should NOT show old branch")
}

func TestStaleBranchLoadedMsgDiscarded(t *testing.T) {
	m := newTestModelReady(t)

	// BranchChangedMsg bumps generation to 1.
	updated, _ := m.Update(panels.BranchChangedMsg{Name: "feature"})
	m = updated.(Model)
	assert.Equal(t, "feature", m.currentBranch)

	// A stale branchLoadedMsg from generation=0 should be discarded.
	updated, _ = m.Update(branchLoadedMsg{Name: "main", generation: 0})
	m = updated.(Model)
	assert.Equal(t, "feature", m.currentBranch, "stale branchLoadedMsg must not overwrite")

	// A current branchLoadedMsg from generation=1 should be accepted.
	updated, _ = m.Update(branchLoadedMsg{Name: "feature", Ahead: 1, generation: 1})
	m = updated.(Model)
	assert.Equal(t, "feature", m.currentBranch)
	assert.Equal(t, 1, m.branchAhead)
}

func TestRapidSequentialCheckoutsOnlyLatestSticks(t *testing.T) {
	m := newTestModelReady(t)

	// Three rapid checkouts without any branchLoadedMsg arriving in between.
	updated, _ := m.Update(panels.BranchChangedMsg{Name: "feat-1"})
	m = updated.(Model)
	updated, _ = m.Update(panels.BranchChangedMsg{Name: "feat-2"})
	m = updated.(Model)
	updated, _ = m.Update(panels.BranchChangedMsg{Name: "feat-3"})
	m = updated.(Model)

	assert.Equal(t, "feat-3", m.currentBranch)
	assert.Equal(t, uint64(3), m.branchInfoGen)

	// Stale responses from earlier generations must all be discarded.
	updated, _ = m.Update(branchLoadedMsg{Name: "feat-1", generation: 1})
	m = updated.(Model)
	assert.Equal(t, "feat-3", m.currentBranch, "generation=1 stale msg must be discarded")

	updated, _ = m.Update(branchLoadedMsg{Name: "feat-2", generation: 2})
	m = updated.(Model)
	assert.Equal(t, "feat-3", m.currentBranch, "generation=2 stale msg must be discarded")

	// Only generation=3 should be accepted.
	updated, _ = m.Update(branchLoadedMsg{Name: "feat-3", Ahead: 2, generation: 3})
	m = updated.(Model)
	assert.Equal(t, "feat-3", m.currentBranch)
	assert.Equal(t, 2, m.branchAhead)
}

func TestEmptyBranchLoadedMsgAccepted(t *testing.T) {
	m := newTestModelReady(t)

	// Initial load sets branch.
	updated, _ := m.Update(branchLoadedMsg{Name: "main", generation: 0})
	m = updated.(Model)
	assert.Equal(t, "main", m.currentBranch)

	// Simulate error or detached HEAD returning empty Name with matching generation.
	updated, _ = m.Update(branchLoadedMsg{Name: "", generation: 0})
	m = updated.(Model)
	assert.Equal(t, "", m.currentBranch, "empty branch name should be accepted when generation matches")
	assert.Equal(t, 0, m.branchAhead)
	assert.Equal(t, 0, m.branchBehind)
}

func TestBranchChangedMsgEmitsRefreshSignals(t *testing.T) {
	m := newTestModelReady(t)

	_, cmd := m.Update(panels.BranchChangedMsg{Name: "feature"})
	require.NotNil(t, cmd, "BranchChangedMsg should return a batch command")

	// Execute all batched commands and collect the messages.
	msgs := executeBatchCmd(t, cmd)

	var hasRefreshGitStatus, hasRefreshBranches bool
	for _, msg := range msgs {
		switch msg.(type) {
		case panels.RefreshGitStatusMsg:
			hasRefreshGitStatus = true
		case panels.RefreshBranchesMsg:
			hasRefreshBranches = true
		}
	}
	assert.True(t, hasRefreshGitStatus, "BranchChangedMsg should emit RefreshGitStatusMsg")
	assert.True(t, hasRefreshBranches, "BranchChangedMsg should emit RefreshBranchesMsg")
}

// ---------------------------------------------------------------------------

func TestAutoFetchTickCmdReturnsNilWhenNotConfigured(t *testing.T) {
	m := newTestModel(t) // no config
	cmd := m.autoFetchTickCmd()
	assert.Nil(t, cmd, "should return nil when config is nil")
}

func TestAutoFetchTickCmdReturnsNilForZeroInterval(t *testing.T) {
	m := newTestModel(t)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 0},
		},
	})
	cmd := m.autoFetchTickCmd()
	assert.Nil(t, cmd, "should return nil for zero interval")
}

func TestAutoFetchTickCmdReturnsCommandForPositiveInterval(t *testing.T) {
	m := newTestModel(t)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 5 * time.Minute},
		},
	})
	cmd := m.autoFetchTickCmd()
	assert.NotNil(t, cmd, "should return a tick command for positive interval")
}

func TestAcquireAutoFetchPermitSerializesRepoFetch(t *testing.T) {
	originalDir := autoFetchCoordinationDir
	originalNow := autoFetchNow
	t.Cleanup(func() {
		autoFetchCoordinationDir = originalDir
		autoFetchNow = originalNow
	})

	tempDir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	autoFetchCoordinationDir = func() (string, error) { return tempDir, nil }
	autoFetchNow = func() time.Time { return now }

	release, acquired, err := acquireAutoFetchPermit("/repo", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, release)

	secondRelease, acquired, err := acquireAutoFetchPermit("/repo", 5*time.Minute)
	require.NoError(t, err)
	require.False(t, acquired, "active lock should suppress concurrent auto-fetch")
	require.Nil(t, secondRelease)

	release(true)

	secondRelease, acquired, err = acquireAutoFetchPermit("/repo", 5*time.Minute)
	require.NoError(t, err)
	require.False(t, acquired, "fresh successful fetch stamp should suppress redundant auto-fetch")
	require.Nil(t, secondRelease)

	now = now.Add(6 * time.Minute)
	secondRelease, acquired, err = acquireAutoFetchPermit("/repo", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired, "expired stamp should allow the next auto-fetch")
	require.NotNil(t, secondRelease)
	secondRelease(false)
}

func TestCreateAutoFetchLockRecoversStaleLock(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, autoFetchRepoKey("/repo")+".lock")
	now := time.Unix(1_700_000_000, 0)
	stale := now.Add(-autoFetchLockStaleAfter - time.Minute)

	require.NoError(t, os.WriteFile(lockPath, []byte("stale\n"), 0o600))
	require.NoError(t, os.Chtimes(lockPath, stale, stale))

	lockFile, err := createAutoFetchLock(lockPath, now)
	require.NoError(t, err)
	require.NotNil(t, lockFile)
	require.NoError(t, lockFile.Close())

	info, err := os.Stat(lockPath)
	require.NoError(t, err)
	assert.False(t, info.ModTime().Equal(stale), "stale lock should be replaced")
}

func TestAutoFetchRepoKeyNormalizesWindowsPathCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path casing is only normalized on Windows")
	}

	assert.Equal(t, autoFetchRepoKey(`C:\Code\Grut`), autoFetchRepoKey(`c:\code\grut`))
}

func TestAutoFetchTickSkipsFetchWhenRepoHasFreshStamp(t *testing.T) {
	originalDir := autoFetchCoordinationDir
	originalNow := autoFetchNow
	t.Cleanup(func() {
		autoFetchCoordinationDir = originalDir
		autoFetchNow = originalNow
	})

	tempDir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	autoFetchCoordinationDir = func() (string, error) { return tempDir, nil }
	autoFetchNow = func() time.Time { return now }

	release, acquired, err := acquireAutoFetchPermit("/repo", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	release(true)

	mock := &mockGitOps{repoRoot: "/repo"}
	m := newTestModelWithGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}

	_, cmd := m.handleAutoFetchTick()
	msg := executeFirstBatchCmd(t, cmd)

	assert.Nil(t, msg)
	assert.Equal(t, 1, mock.repoRootCalls)
	assert.Zero(t, mock.fetchCalls, "fresh repo stamp should skip redundant auto-fetch")
}

func TestAutoFetchTickFetchesAndStampsWhenPermitAvailable(t *testing.T) {
	originalDir := autoFetchCoordinationDir
	originalNow := autoFetchNow
	t.Cleanup(func() {
		autoFetchCoordinationDir = originalDir
		autoFetchNow = originalNow
	})

	tempDir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	autoFetchCoordinationDir = func() (string, error) { return tempDir, nil }
	autoFetchNow = func() time.Time { return now }

	mock := &mockGitOps{repoRoot: "/repo"}
	m := newTestModelWithGit(t, mock)
	m.cfg.Git.AutoFetchInterval = config.Duration{Duration: 5 * time.Minute}

	_, cmd := m.handleAutoFetchTick()
	msg := executeFirstBatchCmd(t, cmd)

	_, ok := msg.(panels.RefreshGitStatusMsg)
	require.True(t, ok)
	assert.Equal(t, 1, mock.fetchCalls)

	_, cmd = m.handleAutoFetchTick()
	msg = executeFirstBatchCmd(t, cmd)
	assert.Nil(t, msg)
	assert.Equal(t, 1, mock.fetchCalls, "successful stamp should suppress the next same-repo fetch")
}

func TestAutoFetchTickSkipsWhenAsyncOpRunning(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 5 * time.Minute},
		},
	})
	m.asyncOp = asyncOpPushing

	updated, cmd := m.handleAutoFetchTick()
	_ = updated.(Model)

	assert.Zero(t, mock.fetchCalls, "fetch should not be called when async op is running")
	assert.NotNil(t, cmd, "should still return next tick command")
}

func TestAutoFetchTickMsg(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 5 * time.Minute},
		},
	})

	// Send AutoFetchTickMsg.
	updated, cmd := m.Update(panels.AutoFetchTickMsg{Time: time.Now()})
	_ = updated.(Model)

	assert.NotNil(t, cmd, "should return batch command with fetch and next tick")
}

func TestInitStartsAutoFetchWhenConfigured(t *testing.T) {
	m := newTestModel(t)
	m = m.WithConfig(&config.Config{
		Git: config.GitConfig{
			AutoFetchInterval: config.Duration{Duration: 5 * time.Minute},
		},
	})

	cmd := m.Init()
	assert.NotNil(t, cmd, "Init should return a batch command including auto-fetch tick")
}

// ---------------------------------------------------------------------------
// Toast helper tests
// ---------------------------------------------------------------------------

func TestTruncateForToast(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"a long commit message that exceeds the limit", 20, "a long commit mes..."},
		{"ab", 1, "a"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}
	for _, tt := range tests {
		got := truncateForToast(tt.input, tt.maxLen)
		assert.Equal(t, tt.want, got, "truncateForToast(%q, %d)", tt.input, tt.maxLen)
	}
}

// ---------------------------------------------------------------------------
// WithGitClient / WithConfig tests
// ---------------------------------------------------------------------------

func TestWithGitClient(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.gitClient)

	mock := &mockGitOps{}
	m = m.WithGitClient(mock)
	assert.NotNil(t, m.gitClient)
}

func TestWithConfig(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.cfg)

	cfg := &config.Config{}
	m = m.WithConfig(cfg)
	assert.NotNil(t, m.cfg)
}

// ---------------------------------------------------------------------------
// AI commit suggestion tests
// ---------------------------------------------------------------------------

func TestAICommitSuggestionStored(t *testing.T) {
	m := newTestModel(t)
	assert.Nil(t, m.aiCommitSuggestion)

	suggestion := panels.AICommitSuggestionMsg{
		Subject: "add user auth",
		Body:    "Implements JWT-based auth",
		Type:    "feat",
		Scope:   "auth",
	}

	updated, _ := m.Update(suggestion)
	m = updated.(Model)

	require.NotNil(t, m.aiCommitSuggestion)
	assert.Equal(t, "add user auth", m.aiCommitSuggestion.Subject)
	assert.Equal(t, "feat", m.aiCommitSuggestion.Type)
	assert.Equal(t, "auth", m.aiCommitSuggestion.Scope)
}

func TestCommitWithAISuggestionPrefills(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Set an AI suggestion.
	m.aiCommitSuggestion = &panels.AICommitSuggestionMsg{
		Subject: "add user auth",
		Type:    "feat",
		Scope:   "auth",
	}

	// Trigger commit — should pre-fill the modal and consume the suggestion.
	updated, cmd := m.handleAction("commit", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, "commit", m.pendingAction)
	assert.Nil(t, m.aiCommitSuggestion, "suggestion should be consumed after use")
	assert.NotNil(t, cmd)

	// The command should produce a ShowModalMsg with the pre-filled value.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "feat(auth): add user auth", modal.Value)
	assert.Equal(t, notify.ModalInput, modal.Kind)
}

func TestCommitWithoutSuggestionNoPreFill(t *testing.T) {
	mock := &mockGitOps{commitHash: "abc123"}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// No AI suggestion set.
	updated, cmd := m.handleAction("commit", tea.KeyPressMsg{})
	m = updated.(Model)

	assert.Equal(t, "commit", m.pendingAction)
	assert.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Empty(t, modal.Value, "no pre-fill when no suggestion")
}

func TestFormatCommitSuggestion(t *testing.T) {
	tests := []struct {
		name string
		msg  *panels.AICommitSuggestionMsg
		want string
	}{
		{
			name: "full conventional commit",
			msg:  &panels.AICommitSuggestionMsg{Subject: "add login", Type: "feat", Scope: "auth"},
			want: "feat(auth): add login",
		},
		{
			name: "type without scope",
			msg:  &panels.AICommitSuggestionMsg{Subject: "fix crash", Type: "fix"},
			want: "fix: fix crash",
		},
		{
			name: "subject only",
			msg:  &panels.AICommitSuggestionMsg{Subject: "update readme"},
			want: "update readme",
		},
		{
			name: "nil suggestion",
			msg:  nil,
			want: "",
		},
		{
			name: "empty subject",
			msg:  &panels.AICommitSuggestionMsg{Type: "feat", Scope: "auth"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCommitSuggestion(tt.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Discard / Unstage tests
// ---------------------------------------------------------------------------

func TestDiscardFileWithoutGitShowsToast(t *testing.T) {
	m := newTestModel(t) // no git client
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleDiscardFile()
	m = updated.(Model)

	assert.Empty(t, m.pendingAction)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Git not available")
}

func TestUnstageFileWithoutGitShowsToast(t *testing.T) {
	m := newTestModel(t) // no git client
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, cmd := m.handleUnstageFile()
	m = updated.(Model)

	assert.Empty(t, m.pendingAction)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Git not available")
}

func TestDiscardFileDuringAsyncOpBlocked(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.asyncOp = "pushing..."

	_, cmd := m.handleDiscardFile()
	assert.Nil(t, cmd, "should be blocked during async op")
}

func TestUnstageFileDuringAsyncOpBlocked(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.asyncOp = "pushing..."

	_, cmd := m.handleUnstageFile()
	assert.Nil(t, cmd, "should be blocked during async op")
}

func TestDiscardFileNoSelectionShowsToast(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	// No filetree/preview panels registered → no file selected.
	updated2, cmd := m.handleDiscardFile()
	m = updated2.(Model)

	assert.Empty(t, m.pendingAction)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "No file selected")
}

func TestUnstageFileNoSelectionShowsToast(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated2, cmd := m.handleUnstageFile()
	m = updated2.(Model)

	assert.Empty(t, m.pendingAction)
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "No file selected")
}

func TestExecuteDiscardFileClearsState(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.pendingDiscardPath = "some/file.go"

	updated2, cmd := m.executeDiscardFile()
	m = updated2.(Model)

	assert.Empty(t, m.pendingDiscardPath, "pendingDiscardPath should be cleared")
	require.NotNil(t, cmd, "should return async command")
	msg := cmd()
	done, ok := msg.(discardFileDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Equal(t, 1, mock.discardCalls)
	assert.Equal(t, "some/file.go", mock.discardPath)
}

func TestExecuteDiscardFileEmptyPathNoop(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.pendingDiscardPath = ""

	_, cmd := m.executeDiscardFile()
	assert.Nil(t, cmd, "empty path should noop")
	assert.Equal(t, 0, mock.discardCalls)
}

func TestHandleFileOpDoneSuccess(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handleFileOpDone(nil, "discard", "Changes discarded")
	require.NotNil(t, cmd, "should return batch command")
}

func TestHandleFileOpDoneError(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handleFileOpDone(fmt.Errorf("checkout failed"), "discard", "Changes discarded")
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "discard failed")
	assert.Contains(t, toast.Message, "checkout failed")
}

func TestHandleFileOpDoneCancelledSilent(t *testing.T) {
	mock := &mockGitOps{}
	m := newTestModelWithGit(t, mock)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	_, cmd := m.handleFileOpDone(context.Canceled, "discard", "Changes discarded")
	assert.Nil(t, cmd, "context.Canceled should produce no toast")
}

func TestTruncateForToastMultibyte(t *testing.T) {
	// Ensure multi-byte characters are not split mid-rune.
	got := truncateForToast("日本語テスト", 5)
	assert.Equal(t, "日本...", got)

	// ASCII still works.
	got = truncateForToast("hello world", 8)
	assert.Equal(t, "hello...", got)
}
