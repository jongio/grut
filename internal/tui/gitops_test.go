package tui

import (
	"context"
	"fmt"
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

	commitHash  string
	commitErr   error
	pushErr     error
	pullErr     error
	fetchErr    error
	commitCalls int
	pushCalls   int
	pullCalls   int
	fetchCalls  int
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

// ---------------------------------------------------------------------------
// Auto-fetch tests
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
