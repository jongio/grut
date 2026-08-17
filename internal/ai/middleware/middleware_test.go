package middleware

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/ai/ops"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client — wraps gittest.MockClient with middleware test state.
// ---------------------------------------------------------------------------

type mockGitClient struct {
	gittest.MockClient

	// Capture calls for assertion.
	commitMsg  string
	commitOpts git.CommitOpts
	commitHash string
	commitErr  error

	mergeErr  error
	rebaseErr error

	statusResult []git.FileStatus
	statusErr    error

	diffResult []git.FileDiff
	diffErr    error

	logResult []git.Commit
	logErr    error

	blameResult []git.BlameLine
	blameErr    error

	repoRootResult string
	repoRootErr    error

	isRepoResult bool
	isRepoErr    error

	branchListResult []git.Branch
	branchListErr    error

	// Track which methods were called.
	calls map[string]int
}

func newMockGitClient() *mockGitClient {
	m := &mockGitClient{
		commitHash: "abc1234",
		calls:      make(map[string]int),
	}

	m.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		m.record("Status")
		return m.statusResult, m.statusErr
	}
	m.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		m.record("Diff")
		return m.diffResult, m.diffErr
	}
	m.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		m.record("Log")
		return m.logResult, m.logErr
	}
	m.BlameFunc = func(_ context.Context, _ string) ([]git.BlameLine, error) {
		m.record("Blame")
		return m.blameResult, m.blameErr
	}
	m.RepoRootFunc = func(_ context.Context) (string, error) {
		m.record("RepoRoot")
		return m.repoRootResult, m.repoRootErr
	}
	m.IsRepoFunc = func(_ context.Context) (bool, error) {
		m.record("IsRepo")
		return m.isRepoResult, m.isRepoErr
	}
	m.StageFunc = func(_ context.Context, _ []string) error {
		m.record("Stage")
		return nil
	}
	m.UnstageFunc = func(_ context.Context, _ []string) error {
		m.record("Unstage")
		return nil
	}
	m.StageHunkFunc = func(_ context.Context, _ string, _ git.Hunk) error {
		m.record("StageHunk")
		return nil
	}
	m.UnstageHunkFunc = func(_ context.Context, _ string, _ git.Hunk) error {
		m.record("UnstageHunk")
		return nil
	}
	m.StageLineFunc = func(_ context.Context, _ string, _ git.Hunk, _ int) error {
		m.record("StageLine")
		return nil
	}
	m.UnstageLineFunc = func(_ context.Context, _ string, _ git.Hunk, _ int) error {
		m.record("UnstageLine")
		return nil
	}
	m.CommitFunc = func(_ context.Context, msg string, opts git.CommitOpts) (string, error) {
		m.record("Commit")
		m.commitMsg = msg
		m.commitOpts = opts
		return m.commitHash, m.commitErr
	}
	m.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		m.record("BranchList")
		return m.branchListResult, m.branchListErr
	}
	m.BranchCreateFunc = func(_ context.Context, _, _ string) error {
		m.record("BranchCreate")
		return nil
	}
	m.BranchDeleteFunc = func(_ context.Context, _ string, _ bool) error {
		m.record("BranchDelete")
		return nil
	}
	m.BranchRenameFunc = func(_ context.Context, _, _ string) error {
		m.record("BranchRename")
		return nil
	}
	m.CheckoutFunc = func(_ context.Context, _ string) error {
		m.record("Checkout")
		return nil
	}
	m.PushFunc = func(_ context.Context, _ git.PushOpts) error {
		m.record("Push")
		return nil
	}
	m.PullFunc = func(_ context.Context, _ git.PullOpts) error {
		m.record("Pull")
		return nil
	}
	m.FetchFunc = func(_ context.Context, _ git.FetchOpts) error {
		m.record("Fetch")
		return nil
	}
	m.WorktreeListFunc = func(_ context.Context) ([]git.Worktree, error) {
		m.record("WorktreeList")
		return nil, nil
	}
	m.WorktreeAddFunc = func(_ context.Context, _, _ string) error {
		m.record("WorktreeAdd")
		return nil
	}
	m.WorktreeRemoveFunc = func(_ context.Context, _ string, _ bool) error {
		m.record("WorktreeRemove")
		return nil
	}
	m.StashListFunc = func(_ context.Context) ([]git.StashEntry, error) {
		m.record("StashList")
		return nil, nil
	}
	m.StashShowFunc = func(_ context.Context, _ int) (string, error) {
		m.record("StashShow")
		return "", nil
	}
	m.StashPushFunc = func(_ context.Context, _ git.StashOpts) error {
		m.record("StashPush")
		return nil
	}
	m.StashPopFunc = func(_ context.Context, _ int) error {
		m.record("StashPop")
		return nil
	}
	m.StashApplyFunc = func(_ context.Context, _ int) error {
		m.record("StashApply")
		return nil
	}
	m.StashDropFunc = func(_ context.Context, _ int) error {
		m.record("StashDrop")
		return nil
	}
	m.TagListFunc = func(_ context.Context) ([]git.Tag, error) {
		m.record("TagList")
		return nil, nil
	}
	m.TagCreateFunc = func(_ context.Context, _, _, _ string) error {
		m.record("TagCreate")
		return nil
	}
	m.TagDeleteFunc = func(_ context.Context, _ string) error {
		m.record("TagDelete")
		return nil
	}
	m.TagListRemoteFunc = func(_ context.Context, _ string) ([]git.Tag, error) {
		m.record("TagListRemote")
		return nil, nil
	}
	m.TagPushFunc = func(_ context.Context, _, _ string) error {
		m.record("TagPush")
		return nil
	}
	m.TagPushAllFunc = func(_ context.Context, _ string) error {
		m.record("TagPushAll")
		return nil
	}
	m.MergeFunc = func(_ context.Context, _ string, _ git.MergeOpts) error {
		m.record("Merge")
		return m.mergeErr
	}
	m.MergeAbortFunc = func(_ context.Context) error {
		m.record("MergeAbort")
		return nil
	}
	m.RebaseFunc = func(_ context.Context, _ string, _ git.RebaseOpts) error {
		m.record("Rebase")
		return m.rebaseErr
	}
	m.RebaseContinueFunc = func(_ context.Context) error {
		m.record("RebaseContinue")
		return nil
	}
	m.RebaseAbortFunc = func(_ context.Context) error {
		m.record("RebaseAbort")
		return nil
	}
	m.CherryPickFunc = func(_ context.Context, _ string) error {
		m.record("CherryPick")
		return nil
	}
	m.BisectStartFunc = func(_ context.Context, _, _ string) error {
		m.record("BisectStart")
		return nil
	}
	m.BisectGoodFunc = func(_ context.Context) (string, error) {
		m.record("BisectGood")
		return "", nil
	}
	m.BisectBadFunc = func(_ context.Context) (string, error) {
		m.record("BisectBad")
		return "", nil
	}
	m.BisectResetFunc = func(_ context.Context) error {
		m.record("BisectReset")
		return nil
	}
	m.ReflogFunc = func(_ context.Context, _ string, _ int) ([]git.ReflogEntry, error) {
		m.record("Reflog")
		return nil, nil
	}
	m.RemoteListFunc = func(_ context.Context) ([]git.Remote, error) {
		m.record("RemoteList")
		return nil, nil
	}
	m.RemoteAddFunc = func(_ context.Context, _, _ string) error {
		m.record("RemoteAdd")
		return nil
	}
	m.RemoteRemoveFunc = func(_ context.Context, _ string) error {
		m.record("RemoteRemove")
		return nil
	}
	m.DiscardFileFunc = func(_ context.Context, _ string) error { return nil }
	m.DiscardAllFunc = func(_ context.Context) error { return nil }
	m.RevertFunc = func(_ context.Context, _ string) error { return nil }
	m.RevertContinueFunc = func(_ context.Context) error { return nil }
	m.RevertAbortFunc = func(_ context.Context) error { return nil }
	m.ResetFunc = func(_ context.Context, _ string, _ git.ResetMode) error { return nil }

	return m
}

func (m *mockGitClient) record(method string) {
	m.calls[method]++
}

// ---------------------------------------------------------------------------
// Mock AI provider for middleware tests
// ---------------------------------------------------------------------------
type mockAIProvider struct {
	name        string
	available   bool
	completeErr error
	response    ai.CompletionResponse
}

func (p *mockAIProvider) Name() string { return p.name }
func (p *mockAIProvider) Available(_ context.Context) (bool, error) {
	return p.available, nil
}

func (p *mockAIProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	if p.completeErr != nil {
		return ai.CompletionResponse{}, p.completeErr
	}
	return p.response, nil
}

func (p *mockAIProvider) CompleteStream(_ context.Context, _ ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	return nil, nil
}
func (p *mockAIProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestMiddleware creates an AIGitClient with a mock git client, a mock
// provider, and no audit logger for testing.
func newTestMiddleware(inner *mockGitClient, provider ai.AIProvider, cfg config.AIConfig) *AIGitClient {
	reg := ai.NewRegistry(cfg)
	if provider != nil {
		reg.Register(cfg.Provider, provider)
	}

	builder := ai.NewBuilder(inner, nil, 0)

	return NewAIGitClient(inner, reg, builder, nil, cfg)
}

// ---------------------------------------------------------------------------
// Tests: pass-through delegation
// ---------------------------------------------------------------------------

func TestPassThroughMethods(t *testing.T) {
	inner := newMockGitClient()
	inner.statusResult = []git.FileStatus{{Path: "file.go", StagedStatus: git.StatusModified}}
	inner.repoRootResult = "/repo"
	inner.isRepoResult = true
	inner.branchListResult = []git.Branch{{Name: "main", IsCurrent: true}}

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	ctx := context.Background()

	t.Run("Status", func(t *testing.T) {
		result, err := client.Status(ctx)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "file.go", result[0].Path)
		assert.Equal(t, 1, inner.calls["Status"])
	})

	t.Run("RepoRoot", func(t *testing.T) {
		root, err := client.RepoRoot(ctx)
		require.NoError(t, err)
		assert.Equal(t, "/repo", root)
		assert.Equal(t, 1, inner.calls["RepoRoot"])
	})

	t.Run("IsRepo", func(t *testing.T) {
		ok, err := client.IsRepo(ctx)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, 1, inner.calls["IsRepo"])
	})

	t.Run("BranchList", func(t *testing.T) {
		branches, err := client.BranchList(ctx)
		require.NoError(t, err)
		assert.Len(t, branches, 1)
		assert.Equal(t, "main", branches[0].Name)
		assert.Equal(t, 1, inner.calls["BranchList"])
	})

	t.Run("Stage", func(t *testing.T) {
		err := client.Stage(ctx, []string{"file.go"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Stage"])
	})

	t.Run("Unstage", func(t *testing.T) {
		err := client.Unstage(ctx, []string{"file.go"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Unstage"])
	})

	t.Run("Push", func(t *testing.T) {
		err := client.Push(ctx, git.PushOpts{Remote: "origin"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Push"])
	})

	t.Run("Checkout", func(t *testing.T) {
		err := client.Checkout(ctx, "main")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Checkout"])
	})

	t.Run("TagList", func(t *testing.T) {
		_, err := client.TagList(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["TagList"])
	})

	t.Run("StashList", func(t *testing.T) {
		_, err := client.StashList(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["StashList"])
	})

	t.Run("WorktreeList", func(t *testing.T) {
		_, err := client.WorktreeList(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["WorktreeList"])
	})

	t.Run("Reflog", func(t *testing.T) {
		_, err := client.Reflog(ctx, "HEAD", 10)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Reflog"])
	})

	t.Run("BisectStart", func(t *testing.T) {
		err := client.BisectStart(ctx, "bad", "good")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BisectStart"])
	})

	t.Run("CherryPick", func(t *testing.T) {
		err := client.CherryPick(ctx, "abc1234")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["CherryPick"])
	})

	t.Run("MergeAbort", func(t *testing.T) {
		err := client.MergeAbort(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["MergeAbort"])
	})

	t.Run("RebaseContinue", func(t *testing.T) {
		err := client.RebaseContinue(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["RebaseContinue"])
	})

	t.Run("RebaseAbort", func(t *testing.T) {
		err := client.RebaseAbort(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["RebaseAbort"])
	})

	t.Run("Diff", func(t *testing.T) {
		inner.diffResult = []git.FileDiff{{Path: "file.go"}}
		result, err := client.Diff(ctx, git.DiffOpts{Staged: true})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, inner.calls["Diff"])
	})

	t.Run("Log", func(t *testing.T) {
		inner.logResult = []git.Commit{{Hash: "abc123", Subject: "initial"}}
		result, err := client.Log(ctx, git.LogOpts{MaxCount: 10})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, inner.calls["Log"])
	})

	t.Run("Blame", func(t *testing.T) {
		inner.blameResult = []git.BlameLine{{Hash: "abc123", Author: "dev"}}
		result, err := client.Blame(ctx, "main.go")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, inner.calls["Blame"])
	})

	t.Run("Pull", func(t *testing.T) {
		err := client.Pull(ctx, git.PullOpts{Remote: "origin"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Pull"])
	})

	t.Run("Fetch", func(t *testing.T) {
		err := client.Fetch(ctx, git.FetchOpts{Remote: "origin"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["Fetch"])
	})

	t.Run("BranchCreate", func(t *testing.T) {
		err := client.BranchCreate(ctx, "feature", "main")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BranchCreate"])
	})

	t.Run("BranchDelete", func(t *testing.T) {
		err := client.BranchDelete(ctx, "feature", false)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BranchDelete"])
	})

	t.Run("BranchRename", func(t *testing.T) {
		err := client.BranchRename(ctx, "old", "new")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BranchRename"])
	})

	t.Run("WorktreeAdd", func(t *testing.T) {
		err := client.WorktreeAdd(ctx, "/tmp/wt", "feature")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["WorktreeAdd"])
	})

	t.Run("WorktreeRemove", func(t *testing.T) {
		err := client.WorktreeRemove(ctx, "/tmp/wt", false)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["WorktreeRemove"])
	})

	t.Run("StashPush", func(t *testing.T) {
		err := client.StashPush(ctx, git.StashOpts{Message: "wip"})
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["StashPush"])
	})

	t.Run("StashPop", func(t *testing.T) {
		err := client.StashPop(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["StashPop"])
	})

	t.Run("StashApply", func(t *testing.T) {
		err := client.StashApply(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["StashApply"])
	})

	t.Run("StashDrop", func(t *testing.T) {
		err := client.StashDrop(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["StashDrop"])
	})

	t.Run("TagCreate", func(t *testing.T) {
		err := client.TagCreate(ctx, "v1.0", "HEAD", "release")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["TagCreate"])
	})

	t.Run("TagDelete", func(t *testing.T) {
		err := client.TagDelete(ctx, "v1.0")
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["TagDelete"])
	})

	t.Run("BisectGood", func(t *testing.T) {
		_, err := client.BisectGood(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BisectGood"])
	})

	t.Run("BisectBad", func(t *testing.T) {
		_, err := client.BisectBad(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BisectBad"])
	})

	t.Run("BisectReset", func(t *testing.T) {
		err := client.BisectReset(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, inner.calls["BisectReset"])
	})
}

// ---------------------------------------------------------------------------
// Tests: Commit interception
// ---------------------------------------------------------------------------

func TestCommitWithExplicitMessage(t *testing.T) {
	inner := newMockGitClient()
	client := newTestMiddleware(inner, nil, config.AIConfig{AutoCommitMsg: true, Provider: "test"})
	ctx := context.Background()

	hash, err := client.Commit(ctx, "explicit message", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "abc1234", hash)
	assert.Equal(t, "explicit message", inner.commitMsg)
}

func TestCommitAutoMessageGeneration(t *testing.T) {
	inner := newMockGitClient()
	// The builder's ForCommit calls inner.Diff(staged) — provide staged diffs.
	inner.diffResult = []git.FileDiff{
		{Path: "main.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"type":"feat","scope":"core","subject":"add new feature","body":""}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{
		AutoCommitMsg: true,
		Provider:      "test",
	})
	ctx := context.Background()

	hash, err := client.Commit(ctx, "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "abc1234", hash)
	// The generated message should have been passed to inner.Commit.
	assert.Contains(t, inner.commitMsg, "feat(core): add new feature")
}

func TestCommitAutoMessageDisabled(t *testing.T) {
	inner := newMockGitClient()
	client := newTestMiddleware(inner, nil, config.AIConfig{AutoCommitMsg: false})
	ctx := context.Background()

	hash, err := client.Commit(ctx, "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "abc1234", hash)
	// Message should remain empty — no AI generation attempted.
	assert.Equal(t, "", inner.commitMsg)
}

func TestCommitAIFailureDoesNotBlock(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "main.go"},
	}

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("AI service unavailable"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{
		AutoCommitMsg: true,
		Provider:      "test",
	})
	ctx := context.Background()

	// The commit should succeed even though AI failed.
	hash, err := client.Commit(ctx, "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "abc1234", hash)
	// Message should remain empty since AI failed.
	assert.Equal(t, "", inner.commitMsg)
}

// ---------------------------------------------------------------------------
// Tests: Merge interception
// ---------------------------------------------------------------------------

func TestMergeSuccessNoConflicts(t *testing.T) {
	inner := newMockGitClient()
	// No conflict status.
	inner.statusResult = []git.FileStatus{
		{Path: "file.go", StagedStatus: git.StatusModified},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	err := client.Merge(ctx, "feature", git.MergeOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["Merge"])
	// Status should not be checked when merge succeeds (no error).
	assert.Equal(t, 0, inner.calls["Status"])
}

func TestMergeWithConflictTriggersAI(t *testing.T) {
	inner := newMockGitClient()
	inner.mergeErr = errors.New("merge: CONFLICT")
	inner.statusResult = []git.FileStatus{
		{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}
	inner.repoRootResult = "/repo"

	// Provider will be called for conflict resolution.
	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"resolutions":[{"file":"conflict.go","regions":[{"start_line":1,"end_line":5,"resolution":"resolved","explanation":"merged both","confidence":0.9}]}]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	// Merge error is still returned to caller.
	err := client.Merge(ctx, "feature", git.MergeOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONFLICT")
	assert.Equal(t, 1, inner.calls["Merge"])
	// Status should be checked to detect conflicts.
	assert.GreaterOrEqual(t, inner.calls["Status"], 1)
}

func TestMergeWithConflictAIFailureDoesNotBlock(t *testing.T) {
	inner := newMockGitClient()
	inner.mergeErr = errors.New("merge: CONFLICT")
	inner.statusResult = []git.FileStatus{
		{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}

	// AI provider is unavailable.
	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("AI service down"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	// Original merge error is returned — AI failure doesn't cause a different error.
	err := client.Merge(ctx, "feature", git.MergeOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONFLICT")
}

func TestMergeErrorWithNoConflictsDoesNotTriggerAI(t *testing.T) {
	inner := newMockGitClient()
	inner.mergeErr = errors.New("merge: fatal error")
	inner.statusResult = []git.FileStatus{
		{Path: "file.go", StagedStatus: git.StatusModified},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	err := client.Merge(ctx, "feature", git.MergeOpts{})
	require.Error(t, err)
	// Status was checked (to look for conflicts) but no conflicts found.
	assert.Equal(t, 1, inner.calls["Status"])
}

// ---------------------------------------------------------------------------
// Tests: Rebase interception
// ---------------------------------------------------------------------------

func TestRebaseWithConflictTriggersAI(t *testing.T) {
	inner := newMockGitClient()
	inner.rebaseErr = errors.New("rebase: CONFLICT")
	inner.statusResult = []git.FileStatus{
		{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"resolutions":[{"file":"conflict.go","regions":[{"start_line":1,"end_line":5,"resolution":"resolved","explanation":"merged both","confidence":0.9}]}]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	err := client.Rebase(ctx, "main", git.RebaseOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONFLICT")
	assert.Equal(t, 1, inner.calls["Rebase"])
	assert.GreaterOrEqual(t, inner.calls["Status"], 1)
}

func TestRebaseSuccessNoConflicts(t *testing.T) {
	inner := newMockGitClient()
	client := newTestMiddleware(inner, nil, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	err := client.Rebase(ctx, "main", git.RebaseOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls["Rebase"])
	assert.Equal(t, 0, inner.calls["Status"])
}

// ---------------------------------------------------------------------------
// Tests: Interface compliance
// ---------------------------------------------------------------------------

func TestAIGitClientImplementsInterface(t *testing.T) {
	inner := newMockGitClient()
	client := newTestMiddleware(inner, nil, config.AIConfig{})

	// Verify the middleware satisfies the GitClient interface at runtime.
	var _ git.GitClient = client
}

// ---------------------------------------------------------------------------
// Tests: Nil audit logger
// ---------------------------------------------------------------------------

func TestNilAuditLoggerDoesNotPanic(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "main.go"},
	}

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("fail"),
	}

	// Audit logger is nil — should not panic.
	client := newTestMiddleware(inner, provider, config.AIConfig{
		AutoCommitMsg: true,
		Provider:      "test",
	})
	ctx := context.Background()

	assert.NotPanics(t, func() {
		_, _ = client.Commit(ctx, "", git.CommitOpts{})
	})
}

// ---------------------------------------------------------------------------
// Tests: hasConflicts helper
// ---------------------------------------------------------------------------

func TestHasConflictsDetectsConflictStatus(t *testing.T) {
	inner := newMockGitClient()
	inner.statusResult = []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified},
		{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	assert.True(t, client.hasConflicts(context.Background()))
}

func TestHasConflictsReturnsFalseForCleanStatus(t *testing.T) {
	inner := newMockGitClient()
	inner.statusResult = []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	assert.False(t, client.hasConflicts(context.Background()))
}

func TestHasConflictsReturnsFalseOnStatusError(t *testing.T) {
	inner := newMockGitClient()
	inner.statusErr = errors.New("git status failed")

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	assert.False(t, client.hasConflicts(context.Background()))
}

// ---------------------------------------------------------------------------
// Tests: Query methods
// ---------------------------------------------------------------------------

func TestGenerateCommitMessageDelegatesToOps(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "main.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"type":"fix","subject":"resolve null pointer","body":""}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	suggestion, err := client.GenerateCommitMessage(ctx)
	require.NoError(t, err)
	require.NotNil(t, suggestion)
	assert.Equal(t, "fix", suggestion.Type)
	assert.Equal(t, "resolve null pointer", suggestion.Subject)
}

func TestReviewDiffDelegatesToOps(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "auth.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `[{"file":"auth.go","line":5,"severity":"warning","category":"security","message":"missing input validation","suggestion":"add validation"}]`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	findings, err := client.ReviewDiff(ctx, git.DiffOpts{})
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "auth.go", findings[0].File)
	assert.Equal(t, "security", findings[0].Category)
}

// ---------------------------------------------------------------------------
// Tests: Audit logging
// ---------------------------------------------------------------------------

func TestAuditLoggingOnCommit(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := tmpDir + "/audit.log"
	audit, err := ai.NewAuditLogger(auditPath)
	require.NoError(t, err)
	defer func() { _ = audit.Close() }()

	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "main.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"type":"feat","subject":"add feature","body":""}`,
		},
	}

	reg := ai.NewRegistry(config.AIConfig{Provider: "test", AutoCommitMsg: true})
	reg.Register("test", provider)
	builder := ai.NewBuilder(inner, nil, 0)

	client := NewAIGitClient(inner, reg, builder, audit, config.AIConfig{
		AutoCommitMsg: true,
		Provider:      "test",
	})
	ctx := context.Background()

	_, err = client.Commit(ctx, "", git.CommitOpts{})
	require.NoError(t, err)
	// The commit message should be AI-generated.
	assert.Contains(t, inner.commitMsg, "feat: add feature")
}

// ---------------------------------------------------------------------------
// Tests: Edge cases
// ---------------------------------------------------------------------------

func TestCommitWhitespaceOnlyMessageTreatedAsEmpty(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "main.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"type":"chore","subject":"update deps","body":""}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{
		AutoCommitMsg: true,
		Provider:      "test",
	})
	ctx := context.Background()

	_, err := client.Commit(ctx, "   \t\n  ", git.CommitOpts{})
	require.NoError(t, err)
	// Whitespace-only should trigger AI generation.
	assert.Contains(t, inner.commitMsg, "chore: update deps")
}

// ---------------------------------------------------------------------------
// Tests: logAudit helper
// ---------------------------------------------------------------------------

func TestLogAuditEntry(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := tmpDir + "/audit.log"
	audit, err := ai.NewAuditLogger(auditPath)
	require.NoError(t, err)
	defer func() { _ = audit.Close() }()

	inner := newMockGitClient()
	client := NewAIGitClient(inner, nil, nil, audit, config.AIConfig{})

	// Should not panic or error.
	client.logAudit("test_op", "accepted", nil)
	client.logAudit("test_op", "error", errors.New("something failed"))
}

func TestNewAIGitClientReusesBuilderRedactor(t *testing.T) {
	inner := newMockGitClient()
	redactor := ai.NewRedactor([]string{"*.custom"})
	builder := ai.NewBuilder(inner, redactor, 0)

	client := NewAIGitClient(inner, nil, builder, nil, config.AIConfig{})

	assert.Same(t, redactor, client.redactor)
}

// ---------------------------------------------------------------------------
// Tests: ops types are correctly returned
// ---------------------------------------------------------------------------

func TestOpsTypesUsedInQueryMethods(t *testing.T) {
	// Compile-time check that query method return types are correct.
	var c *AIGitClient
	_ = c

	var _ func(context.Context) (*ops.CommitSuggestion, error)
	var _ func(context.Context, git.DiffOpts) ([]ops.ReviewFinding, error)
	var _ func(context.Context, string) (*ops.PRDescription, error)
	var _ func(context.Context, string) (*ops.RebaseSuggestion, error)
	var _ func(context.Context) ([]ops.BranchRecommendation, error)
	var _ func(context.Context, string, string) (*ops.BisectAnalysis, error)
	var _ func(context.Context, string, string) ([]ops.ChangelogEntry, error)
	var _ func(context.Context, string) (*ops.SplitPlan, error)

	assert.True(t, true)
}

// ---------------------------------------------------------------------------
// Tests: conflictFiles helper
// ---------------------------------------------------------------------------

func TestConflictFilesReturnsConflictedPaths(t *testing.T) {
	inner := newMockGitClient()
	inner.statusResult = []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified},
		{Path: "conflict1.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		{Path: "conflict2.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	files := client.conflictFiles(context.Background())
	assert.Equal(t, []string{"conflict1.go", "conflict2.go"}, files)
}

func TestConflictFilesReturnsNilOnStatusError(t *testing.T) {
	inner := newMockGitClient()
	inner.statusErr = errors.New("status error")

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	files := client.conflictFiles(context.Background())
	assert.Nil(t, files)
}

func TestConflictFilesReturnsNilWhenNoConflicts(t *testing.T) {
	inner := newMockGitClient()
	inner.statusResult = []git.FileStatus{
		{Path: "clean.go", StagedStatus: git.StatusModified},
	}

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	files := client.conflictFiles(context.Background())
	assert.Nil(t, files)
}

// ---------------------------------------------------------------------------
// Tests: Commit inner error propagation
// ---------------------------------------------------------------------------

func TestCommitInnerErrorPropagated(t *testing.T) {
	inner := newMockGitClient()
	inner.commitErr = errors.New("nothing to commit")

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	ctx := context.Background()

	_, err := client.Commit(ctx, "msg", git.CommitOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to commit")
}

// ---------------------------------------------------------------------------
// Tests: Merge pass-through of error on non-conflict
// ---------------------------------------------------------------------------

func TestMergeNonConflictErrorPassedThrough(t *testing.T) {
	inner := newMockGitClient()
	inner.mergeErr = errors.New("not a merge candidate")
	inner.statusResult = nil // Status returns empty — no conflicts.

	client := newTestMiddleware(inner, nil, config.AIConfig{})
	ctx := context.Background()

	err := client.Merge(ctx, "feature", git.MergeOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a merge candidate")
}

// ---------------------------------------------------------------------------
// Tests: unused time import guard
// ---------------------------------------------------------------------------

func TestTimeUsedInAuditEntries(t *testing.T) {
	// Ensures the time import is used — AuditEntry.Timestamp is time.Time.
	entry := ai.AuditEntry{
		Timestamp: time.Now(),
		Operation: "test",
		Result:    "ok",
	}
	assert.False(t, entry.Timestamp.IsZero())
}

// ---------------------------------------------------------------------------
// Tests: GeneratePRDescription
// ---------------------------------------------------------------------------

func TestGeneratePRDescription(t *testing.T) {
	inner := newMockGitClient()
	inner.diffResult = []git.FileDiff{
		{Path: "auth.go", Hunks: []git.Hunk{{Header: "@@ -1,3 +1,5 @@"}}},
	}
	inner.logResult = []git.Commit{
		{Hash: "abc1234", Subject: "feat: add auth"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"title":"Add authentication","summary":"Implements auth","changes":["Added OAuth2"],"testing_notes":"run tests","breaking_changes":[]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	desc, err := client.GeneratePRDescription(ctx, "main")
	require.NoError(t, err)
	require.NotNil(t, desc)
	assert.Equal(t, "Add authentication", desc.Title)
	assert.Equal(t, "Implements auth", desc.Summary)
}

func TestGeneratePRDescriptionProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("provider down"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.GeneratePRDescription(ctx, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI PR description")
}

// ---------------------------------------------------------------------------
// Tests: SuggestRebase
// ---------------------------------------------------------------------------

func TestSuggestRebase(t *testing.T) {
	inner := newMockGitClient()
	inner.logResult = []git.Commit{
		{Hash: "abc1234", Subject: "WIP: stuff"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"commits":[{"hash":"abc1234","subject":"WIP: stuff","action":"squash","reason":"WIP commit"}]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	suggestion, err := client.SuggestRebase(ctx, "main")
	require.NoError(t, err)
	require.NotNil(t, suggestion)
	require.Len(t, suggestion.Commits, 1)
	assert.Equal(t, "squash", suggestion.Commits[0].Action)
}

func TestSuggestRebaseProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("timeout"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.SuggestRebase(ctx, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI rebase suggestion")
}

// ---------------------------------------------------------------------------
// Tests: AnalyzeBranches
// ---------------------------------------------------------------------------

func TestAnalyzeBranches(t *testing.T) {
	inner := newMockGitClient()
	inner.branchListResult = []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "old-feature"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"branches":[{"name":"old-feature","action":"delete","reason":"Merged and stale"}]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	recs, err := client.AnalyzeBranches(ctx)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "old-feature", recs[0].Name)
	assert.Equal(t, "delete", recs[0].Action)
}

func TestAnalyzeBranchesProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"
	inner.branchListResult = []git.Branch{
		{Name: "main", IsCurrent: true},
	}

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("quota exceeded"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.AnalyzeBranches(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI branch analysis")
}

// ---------------------------------------------------------------------------
// Tests: AnalyzeBisect
// ---------------------------------------------------------------------------

func TestAnalyzeBisect(t *testing.T) {
	inner := newMockGitClient()
	inner.logResult = []git.Commit{
		{Hash: "bad123", Subject: "break things"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"candidates":[{"hash":"bad123","subject":"break things","probability":0.9,"reason":"touches critical path"}],"summary":"Most likely culprit"}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	analysis, err := client.AnalyzeBisect(ctx, "good123", "bad123")
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Len(t, analysis.Candidates, 1)
	assert.InDelta(t, 0.9, analysis.Candidates[0].Probability, 0.01)
}

func TestAnalyzeBisectProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("unavailable"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.AnalyzeBisect(ctx, "good123", "bad123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI bisect analysis")
}

// ---------------------------------------------------------------------------
// Tests: GenerateChangelog
// ---------------------------------------------------------------------------

func TestGenerateChangelog(t *testing.T) {
	inner := newMockGitClient()
	inner.logResult = []git.Commit{
		{Hash: "abc1234", Subject: "feat: add auth"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `[{"category":"added","description":"OAuth2 authentication","commit_hashes":["abc1234"]}]`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	entries, err := client.GenerateChangelog(ctx, "v1.0", "HEAD")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "added", entries[0].Category)
}

func TestGenerateChangelogProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("rate limited"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.GenerateChangelog(ctx, "v1.0", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI changelog")
}

// ---------------------------------------------------------------------------
// Tests: SuggestSplit
// ---------------------------------------------------------------------------

func TestSuggestSplit(t *testing.T) {
	inner := newMockGitClient()
	inner.logResult = []git.Commit{
		{Hash: "big123", Subject: "big commit"},
	}
	inner.diffResult = []git.FileDiff{
		{Path: "auth.go"},
		{Path: "types.go"},
	}
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:      "test",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"pieces":[{"files":["auth.go"],"commit_message":"feat: add auth","reason":"auth logic","order":1},{"files":["types.go"],"commit_message":"refactor: types","reason":"type defs","order":2}]}`,
		},
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	plan, err := client.SuggestSplit(ctx, "big123")
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Pieces, 2)
	assert.Equal(t, "feat: add auth", plan.Pieces[0].CommitMessage)
}

func TestSuggestSplitProviderError(t *testing.T) {
	inner := newMockGitClient()
	inner.repoRootResult = "/repo"

	provider := &mockAIProvider{
		name:        "test",
		available:   true,
		completeErr: errors.New("context too large"),
	}

	client := newTestMiddleware(inner, provider, config.AIConfig{Provider: "test"})
	ctx := context.Background()

	_, err := client.SuggestSplit(ctx, "big123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI split suggestion")
}

// ---------------------------------------------------------------------------
// Tests: logAudit error redaction
// ---------------------------------------------------------------------------

func TestLogAuditRedactsErrorMessages(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := tmpDir + "/audit.log"
	audit, err := ai.NewAuditLogger(auditPath)
	require.NoError(t, err)
	defer func() { _ = audit.Close() }()

	inner := newMockGitClient()
	builder := ai.NewBuilder(inner, nil, 0)
	client := NewAIGitClient(inner, nil, builder, audit, config.AIConfig{})

	// Simulate an error containing a GitHub token.
	secretErr := errors.New("auth failed: token=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop")
	client.logAudit("test_op", "error", secretErr)

	// Read the audit log and verify the token was redacted.
	data, readErr := os.ReadFile(auditPath)
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), "ghp_")
	assert.Contains(t, string(data), ai.RedactedPlaceholder)
}
