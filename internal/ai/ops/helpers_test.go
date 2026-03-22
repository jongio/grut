package ops

import (
	"context"
	"errors"
	"fmt"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
)

// errMockProvider is a reusable sentinel error for mock provider tests.
var errMockProvider = errors.New("mock provider error")

// ---------------------------------------------------------------------------
// Mock AI providers
// ---------------------------------------------------------------------------

// mockAIProvider is a minimal AIProvider for testing ops behaviour.
type mockAIProvider struct {
	name         string
	available    bool
	completeResp ai.CompletionResponse
	completeErr  error
}

// mockProvider is an alternative mock AIProvider used by changelog, split,
// and conflict tests with a simpler field naming convention.
type mockProvider struct {
	name      string
	available bool
	response  ai.CompletionResponse
	err       error
}

var _ ai.AIProvider = (*mockProvider)(nil)

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Available(_ context.Context) (bool, error) {
	return m.available, nil
}

func (m *mockProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	return m.response, m.err
}

func (m *mockProvider) CompleteStream(_ context.Context, _ ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockProvider) Close() error { return nil }

var _ ai.AIProvider = (*mockAIProvider)(nil)

func (m *mockAIProvider) Name() string { return m.name }
func (m *mockAIProvider) Available(_ context.Context) (bool, error) {
	return m.available, nil
}

func (m *mockAIProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	return m.completeResp, m.completeErr
}

func (m *mockAIProvider) CompleteStream(_ context.Context, _ ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	return nil, nil
}
func (m *mockAIProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// testProvider — simple AI provider mock with different field names.
// ---------------------------------------------------------------------------

// testProvider is a test double for ai.AIProvider that returns preconfigured
// responses. Used by bisect, branch, changelog, conflict, and other tests.
type testProvider struct {
	name      string
	available bool
	response  ai.CompletionResponse
	err       error
}

var _ ai.AIProvider = (*testProvider)(nil)

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) Available(_ context.Context) (bool, error) {
	return p.available, nil
}

func (p *testProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	return p.response, p.err
}

func (p *testProvider) CompleteStream(_ context.Context, _ ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (p *testProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

// mockGitClient is a test double implementing git.GitClient for ops tests.
// Supports both static fields (branches) and functional overrides.
type mockGitClient struct {
	// Static fields — returned by default when functional overrides are not set.
	branches []git.Branch

	// Functional overrides — when set, take precedence.
	StatusFunc     func(ctx context.Context) ([]git.FileStatus, error)
	DiffFunc       func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	LogFunc        func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	BlameFunc      func(ctx context.Context, path string) ([]git.BlameLine, error)
	RepoRootFunc   func(ctx context.Context) (string, error)
	BranchListFunc func(ctx context.Context) ([]git.Branch, error)
}

var _ git.GitClient = (*mockGitClient)(nil)

func (m *mockGitClient) Status(ctx context.Context) ([]git.FileStatus, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
	if m.DiffFunc != nil {
		return m.DiffFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockGitClient) Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
	if m.LogFunc != nil {
		return m.LogFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockGitClient) Blame(ctx context.Context, path string) ([]git.BlameLine, error) {
	if m.BlameFunc != nil {
		return m.BlameFunc(ctx, path)
	}
	return nil, nil
}

func (m *mockGitClient) RepoRoot(ctx context.Context) (string, error) {
	if m.RepoRootFunc != nil {
		return m.RepoRootFunc(ctx)
	}
	return "", nil
}

func (m *mockGitClient) IsRepo(_ context.Context) (bool, error) { return true, nil }
func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// IndexMutator stubs.
func (m *mockGitClient) Stage(_ context.Context, _ []string) error                        { return nil }
func (m *mockGitClient) Unstage(_ context.Context, _ []string) error                      { return nil }
func (m *mockGitClient) StageHunk(_ context.Context, _ string, _ git.Hunk) error          { return nil }
func (m *mockGitClient) UnstageHunk(_ context.Context, _ string, _ git.Hunk) error        { return nil }
func (m *mockGitClient) StageLine(_ context.Context, _ string, _ git.Hunk, _ int) error   { return nil }
func (m *mockGitClient) UnstageLine(_ context.Context, _ string, _ git.Hunk, _ int) error { return nil }
func (m *mockGitClient) Commit(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
	return "abc123", nil
}

// BranchManager stubs.
func (m *mockGitClient) BranchList(ctx context.Context) ([]git.Branch, error) {
	if m.BranchListFunc != nil {
		return m.BranchListFunc(ctx)
	}
	return m.branches, nil
}
func (m *mockGitClient) BranchCreate(_ context.Context, _, _ string) error      { return nil }
func (m *mockGitClient) BranchDelete(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockGitClient) BranchRename(_ context.Context, _, _ string) error      { return nil }
func (m *mockGitClient) Checkout(_ context.Context, _ string) error             { return nil }

// RemoteOps stubs.
func (m *mockGitClient) Push(_ context.Context, _ git.PushOpts) error   { return nil }
func (m *mockGitClient) Pull(_ context.Context, _ git.PullOpts) error   { return nil }
func (m *mockGitClient) Fetch(_ context.Context, _ git.FetchOpts) error { return nil }

// WorktreeOps stubs.
func (m *mockGitClient) WorktreeList(_ context.Context) ([]git.Worktree, error)   { return nil, nil }
func (m *mockGitClient) WorktreeAdd(_ context.Context, _, _ string) error         { return nil }
func (m *mockGitClient) WorktreeRemove(_ context.Context, _ string, _ bool) error { return nil }

// StashOps stubs.
func (m *mockGitClient) StashList(_ context.Context) ([]git.StashEntry, error) { return nil, nil }
func (m *mockGitClient) StashShow(_ context.Context, _ int) (string, error)    { return "", nil }
func (m *mockGitClient) StashPush(_ context.Context, _ git.StashOpts) error    { return nil }
func (m *mockGitClient) StashPop(_ context.Context, _ int) error               { return nil }
func (m *mockGitClient) StashApply(_ context.Context, _ int) error             { return nil }
func (m *mockGitClient) StashDrop(_ context.Context, _ int) error              { return nil }

// TagOps stubs.
func (m *mockGitClient) TagList(_ context.Context) ([]git.Tag, error)      { return nil, nil }
func (m *mockGitClient) TagCreate(_ context.Context, _, _, _ string) error { return nil }
func (m *mockGitClient) TagDelete(_ context.Context, _ string) error       { return nil }
func (m *mockGitClient) TagListRemote(_ context.Context, _ string) ([]git.Tag, error) {
	return nil, nil
}
func (m *mockGitClient) TagPush(_ context.Context, _, _ string) error { return nil }
func (m *mockGitClient) TagPushAll(_ context.Context, _ string) error { return nil }

// MergeRebaseOps stubs.
func (m *mockGitClient) Merge(_ context.Context, _ string, _ git.MergeOpts) error   { return nil }
func (m *mockGitClient) MergeAbort(_ context.Context) error                         { return nil }
func (m *mockGitClient) Rebase(_ context.Context, _ string, _ git.RebaseOpts) error { return nil }
func (m *mockGitClient) RebaseContinue(_ context.Context) error                     { return nil }
func (m *mockGitClient) RebaseAbort(_ context.Context) error                        { return nil }
func (m *mockGitClient) CherryPick(_ context.Context, _ string) error               { return nil }

// BisectOps stubs.
func (m *mockGitClient) BisectStart(_ context.Context, _, _ string) error { return nil }
func (m *mockGitClient) BisectGood(_ context.Context) (string, error)     { return "", nil }
func (m *mockGitClient) BisectBad(_ context.Context) (string, error)      { return "", nil }
func (m *mockGitClient) BisectReset(_ context.Context) error              { return nil }

// ReflogOps stubs.
func (m *mockGitClient) Reflog(_ context.Context, _ string, _ int) ([]git.ReflogEntry, error) {
	return nil, nil
}

func (m *mockGitClient) RemoteList(_ context.Context) ([]git.Remote, error)       { return nil, nil }
func (m *mockGitClient) RemoteAdd(_ context.Context, _, _ string) error           { return nil }
func (m *mockGitClient) RemoteRemove(_ context.Context, _ string) error           { return nil }
func (m *mockGitClient) DiscardFile(_ context.Context, _ string) error            { return nil }
func (m *mockGitClient) DiscardAllUnstaged(_ context.Context) error               { return nil }
func (m *mockGitClient) Revert(_ context.Context, _ string) error                 { return nil }
func (m *mockGitClient) RevertContinue(_ context.Context) error                   { return nil }
func (m *mockGitClient) RevertAbort(_ context.Context) error                      { return nil }
func (m *mockGitClient) Reset(_ context.Context, _ string, _ git.ResetMode) error { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMockGitClient returns a mock configured with a current branch and repo root.
func newMockGitClient(branch, repoRoot string) *mockGitClient {
	return &mockGitClient{
		BranchListFunc: func(_ context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: branch, IsCurrent: true}}, nil
		},
		RepoRootFunc: func(_ context.Context) (string, error) {
			return repoRoot, nil
		},
	}
}
