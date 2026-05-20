// Package gittest provides shared test doubles for the git package.
package gittest

import (
	"context"

	"github.com/jongio/grut/internal/git"
)

// MockClient is a test double implementing git.GitClient.
// Each method has a corresponding exported function field; if non-nil the field
// is called, otherwise the method returns zero values.
type MockClient struct {
	StatusFunc         func(ctx context.Context) ([]git.FileStatus, error)
	DiffFunc           func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	LogFunc            func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	BlameFunc          func(ctx context.Context, path string) ([]git.BlameLine, error)
	RepoRootFunc       func(ctx context.Context) (string, error)
	IsRepoFunc         func(ctx context.Context) (bool, error)
	DiffTreeFilesFunc  func(ctx context.Context, hash string) ([]string, error)
	DiffFileNamesFunc  func(ctx context.Context, commitA, commitB string) ([]string, error)
	StageFunc          func(ctx context.Context, paths []string) error
	UnstageFunc        func(ctx context.Context, paths []string) error
	StageHunkFunc      func(ctx context.Context, path string, hunk git.Hunk) error
	UnstageHunkFunc    func(ctx context.Context, path string, hunk git.Hunk) error
	StageLineFunc      func(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	UnstageLineFunc    func(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	CommitFunc         func(ctx context.Context, msg string, opts git.CommitOpts) (string, error)
	BranchListFunc     func(ctx context.Context) ([]git.Branch, error)
	CurrentBranchFunc  func(ctx context.Context) (git.Branch, error)
	BranchCreateFunc   func(ctx context.Context, name, base string) error
	BranchDeleteFunc   func(ctx context.Context, name string, force bool) error
	BranchRenameFunc   func(ctx context.Context, oldName, newName string) error
	CheckoutFunc       func(ctx context.Context, ref string) error
	PushFunc           func(ctx context.Context, opts git.PushOpts) error
	PullFunc           func(ctx context.Context, opts git.PullOpts) error
	FetchFunc          func(ctx context.Context, opts git.FetchOpts) error
	RemoteListFunc     func(ctx context.Context) ([]git.Remote, error)
	RemoteAddFunc      func(ctx context.Context, name, url string) error
	RemoteRemoveFunc   func(ctx context.Context, name string) error
	WorktreeListFunc   func(ctx context.Context) ([]git.Worktree, error)
	WorktreeAddFunc    func(ctx context.Context, path, branch string) error
	WorktreeRemoveFunc func(ctx context.Context, path string, force bool) error
	StashListFunc      func(ctx context.Context) ([]git.StashEntry, error)
	StashShowFunc      func(ctx context.Context, index int) (string, error)
	StashPushFunc      func(ctx context.Context, opts git.StashOpts) error
	StashPopFunc       func(ctx context.Context, index int) error
	StashApplyFunc     func(ctx context.Context, index int) error
	StashDropFunc      func(ctx context.Context, index int) error
	TagListFunc        func(ctx context.Context) ([]git.Tag, error)
	TagCreateFunc      func(ctx context.Context, name, ref, message string) error
	TagDeleteFunc      func(ctx context.Context, name string) error
	TagListRemoteFunc  func(ctx context.Context, remote string) ([]git.Tag, error)
	TagPushFunc        func(ctx context.Context, remote, name string) error
	TagPushAllFunc     func(ctx context.Context, remote string) error
	MergeFunc          func(ctx context.Context, branch string, opts git.MergeOpts) error
	MergeAbortFunc     func(ctx context.Context) error
	RebaseFunc         func(ctx context.Context, onto string, opts git.RebaseOpts) error
	RebaseContinueFunc func(ctx context.Context) error
	RebaseAbortFunc    func(ctx context.Context) error
	CherryPickFunc     func(ctx context.Context, commitHash string) error
	BisectStartFunc    func(ctx context.Context, bad, good string) error
	BisectGoodFunc     func(ctx context.Context) (string, error)
	BisectBadFunc      func(ctx context.Context) (string, error)
	BisectResetFunc    func(ctx context.Context) error
	ReflogFunc         func(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error)
	DiscardFileFunc    func(ctx context.Context, path string) error
	DiscardAllFunc     func(ctx context.Context) error
	RevertFunc         func(ctx context.Context, hash string) error
	RevertContinueFunc func(ctx context.Context) error
	RevertAbortFunc    func(ctx context.Context) error
	ResetFunc          func(ctx context.Context, ref string, mode git.ResetMode) error
}

// Compile-time check that MockClient implements git.GitClient.
var _ git.GitClient = (*MockClient)(nil)

// --- StatusReader ---

func (m *MockClient) Status(ctx context.Context) ([]git.FileStatus, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
	if m.DiffFunc != nil {
		return m.DiffFunc(ctx, opts)
	}
	return nil, nil
}

func (m *MockClient) Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
	if m.LogFunc != nil {
		return m.LogFunc(ctx, opts)
	}
	return nil, nil
}

func (m *MockClient) Blame(ctx context.Context, path string) ([]git.BlameLine, error) {
	if m.BlameFunc != nil {
		return m.BlameFunc(ctx, path)
	}
	return nil, nil
}

func (m *MockClient) RepoRoot(ctx context.Context) (string, error) {
	if m.RepoRootFunc != nil {
		return m.RepoRootFunc(ctx)
	}
	return "", nil
}

func (m *MockClient) IsRepo(ctx context.Context) (bool, error) {
	if m.IsRepoFunc != nil {
		return m.IsRepoFunc(ctx)
	}
	return true, nil
}

func (m *MockClient) DiffTreeFiles(ctx context.Context, hash string) ([]string, error) {
	if m.DiffTreeFilesFunc != nil {
		return m.DiffTreeFilesFunc(ctx, hash)
	}
	return nil, nil
}

func (m *MockClient) DiffFileNames(ctx context.Context, commitA, commitB string) ([]string, error) {
	if m.DiffFileNamesFunc != nil {
		return m.DiffFileNamesFunc(ctx, commitA, commitB)
	}
	return nil, nil
}

// --- IndexMutator ---

func (m *MockClient) Stage(ctx context.Context, paths []string) error {
	if m.StageFunc != nil {
		return m.StageFunc(ctx, paths)
	}
	return nil
}

func (m *MockClient) Unstage(ctx context.Context, paths []string) error {
	if m.UnstageFunc != nil {
		return m.UnstageFunc(ctx, paths)
	}
	return nil
}

func (m *MockClient) StageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	if m.StageHunkFunc != nil {
		return m.StageHunkFunc(ctx, path, hunk)
	}
	return nil
}

func (m *MockClient) UnstageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	if m.UnstageHunkFunc != nil {
		return m.UnstageHunkFunc(ctx, path, hunk)
	}
	return nil
}

func (m *MockClient) StageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	if m.StageLineFunc != nil {
		return m.StageLineFunc(ctx, path, hunk, lineIdx)
	}
	return nil
}

func (m *MockClient) UnstageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	if m.UnstageLineFunc != nil {
		return m.UnstageLineFunc(ctx, path, hunk, lineIdx)
	}
	return nil
}

func (m *MockClient) Commit(ctx context.Context, msg string, opts git.CommitOpts) (string, error) {
	if m.CommitFunc != nil {
		return m.CommitFunc(ctx, msg, opts)
	}
	return "", nil
}

// --- BranchManager ---

func (m *MockClient) BranchList(ctx context.Context) ([]git.Branch, error) {
	if m.BranchListFunc != nil {
		return m.BranchListFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) CurrentBranch(ctx context.Context) (git.Branch, error) {
	if m.CurrentBranchFunc != nil {
		return m.CurrentBranchFunc(ctx)
	}
	return git.Branch{}, nil
}

func (m *MockClient) BranchCreate(ctx context.Context, name, base string) error {
	if m.BranchCreateFunc != nil {
		return m.BranchCreateFunc(ctx, name, base)
	}
	return nil
}

func (m *MockClient) BranchDelete(ctx context.Context, name string, force bool) error {
	if m.BranchDeleteFunc != nil {
		return m.BranchDeleteFunc(ctx, name, force)
	}
	return nil
}

func (m *MockClient) BranchRename(ctx context.Context, oldName, newName string) error {
	if m.BranchRenameFunc != nil {
		return m.BranchRenameFunc(ctx, oldName, newName)
	}
	return nil
}

func (m *MockClient) Checkout(ctx context.Context, ref string) error {
	if m.CheckoutFunc != nil {
		return m.CheckoutFunc(ctx, ref)
	}
	return nil
}

// --- RemoteOps ---

func (m *MockClient) Push(ctx context.Context, opts git.PushOpts) error {
	if m.PushFunc != nil {
		return m.PushFunc(ctx, opts)
	}
	return nil
}

func (m *MockClient) Pull(ctx context.Context, opts git.PullOpts) error {
	if m.PullFunc != nil {
		return m.PullFunc(ctx, opts)
	}
	return nil
}

func (m *MockClient) Fetch(ctx context.Context, opts git.FetchOpts) error {
	if m.FetchFunc != nil {
		return m.FetchFunc(ctx, opts)
	}
	return nil
}

// --- RemoteListOps ---

func (m *MockClient) RemoteList(ctx context.Context) ([]git.Remote, error) {
	if m.RemoteListFunc != nil {
		return m.RemoteListFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) RemoteAdd(ctx context.Context, name, url string) error {
	if m.RemoteAddFunc != nil {
		return m.RemoteAddFunc(ctx, name, url)
	}
	return nil
}

func (m *MockClient) RemoteRemove(ctx context.Context, name string) error {
	if m.RemoteRemoveFunc != nil {
		return m.RemoteRemoveFunc(ctx, name)
	}
	return nil
}

// --- WorktreeOps ---

func (m *MockClient) WorktreeList(ctx context.Context) ([]git.Worktree, error) {
	if m.WorktreeListFunc != nil {
		return m.WorktreeListFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) WorktreeAdd(ctx context.Context, path, branch string) error {
	if m.WorktreeAddFunc != nil {
		return m.WorktreeAddFunc(ctx, path, branch)
	}
	return nil
}

func (m *MockClient) WorktreeRemove(ctx context.Context, path string, force bool) error {
	if m.WorktreeRemoveFunc != nil {
		return m.WorktreeRemoveFunc(ctx, path, force)
	}
	return nil
}

// --- StashOps ---

func (m *MockClient) StashList(ctx context.Context) ([]git.StashEntry, error) {
	if m.StashListFunc != nil {
		return m.StashListFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) StashShow(ctx context.Context, index int) (string, error) {
	if m.StashShowFunc != nil {
		return m.StashShowFunc(ctx, index)
	}
	return "", nil
}

func (m *MockClient) StashPush(ctx context.Context, opts git.StashOpts) error {
	if m.StashPushFunc != nil {
		return m.StashPushFunc(ctx, opts)
	}
	return nil
}

func (m *MockClient) StashPop(ctx context.Context, index int) error {
	if m.StashPopFunc != nil {
		return m.StashPopFunc(ctx, index)
	}
	return nil
}

func (m *MockClient) StashApply(ctx context.Context, index int) error {
	if m.StashApplyFunc != nil {
		return m.StashApplyFunc(ctx, index)
	}
	return nil
}

func (m *MockClient) StashDrop(ctx context.Context, index int) error {
	if m.StashDropFunc != nil {
		return m.StashDropFunc(ctx, index)
	}
	return nil
}

// --- TagOps ---

func (m *MockClient) TagList(ctx context.Context) ([]git.Tag, error) {
	if m.TagListFunc != nil {
		return m.TagListFunc(ctx)
	}
	return nil, nil
}

func (m *MockClient) TagCreate(ctx context.Context, name, ref, message string) error {
	if m.TagCreateFunc != nil {
		return m.TagCreateFunc(ctx, name, ref, message)
	}
	return nil
}

func (m *MockClient) TagDelete(ctx context.Context, name string) error {
	if m.TagDeleteFunc != nil {
		return m.TagDeleteFunc(ctx, name)
	}
	return nil
}

func (m *MockClient) TagListRemote(ctx context.Context, remote string) ([]git.Tag, error) {
	if m.TagListRemoteFunc != nil {
		return m.TagListRemoteFunc(ctx, remote)
	}
	return nil, nil
}

func (m *MockClient) TagPush(ctx context.Context, remote, name string) error {
	if m.TagPushFunc != nil {
		return m.TagPushFunc(ctx, remote, name)
	}
	return nil
}

func (m *MockClient) TagPushAll(ctx context.Context, remote string) error {
	if m.TagPushAllFunc != nil {
		return m.TagPushAllFunc(ctx, remote)
	}
	return nil
}

// --- MergeRebaseOps ---

func (m *MockClient) Merge(ctx context.Context, branch string, opts git.MergeOpts) error {
	if m.MergeFunc != nil {
		return m.MergeFunc(ctx, branch, opts)
	}
	return nil
}

func (m *MockClient) MergeAbort(ctx context.Context) error {
	if m.MergeAbortFunc != nil {
		return m.MergeAbortFunc(ctx)
	}
	return nil
}

func (m *MockClient) Rebase(ctx context.Context, onto string, opts git.RebaseOpts) error {
	if m.RebaseFunc != nil {
		return m.RebaseFunc(ctx, onto, opts)
	}
	return nil
}

func (m *MockClient) RebaseContinue(ctx context.Context) error {
	if m.RebaseContinueFunc != nil {
		return m.RebaseContinueFunc(ctx)
	}
	return nil
}

func (m *MockClient) RebaseAbort(ctx context.Context) error {
	if m.RebaseAbortFunc != nil {
		return m.RebaseAbortFunc(ctx)
	}
	return nil
}

func (m *MockClient) CherryPick(ctx context.Context, commitHash string) error {
	if m.CherryPickFunc != nil {
		return m.CherryPickFunc(ctx, commitHash)
	}
	return nil
}

// --- BisectOps ---

func (m *MockClient) BisectStart(ctx context.Context, bad, good string) error {
	if m.BisectStartFunc != nil {
		return m.BisectStartFunc(ctx, bad, good)
	}
	return nil
}

func (m *MockClient) BisectGood(ctx context.Context) (string, error) {
	if m.BisectGoodFunc != nil {
		return m.BisectGoodFunc(ctx)
	}
	return "", nil
}

func (m *MockClient) BisectBad(ctx context.Context) (string, error) {
	if m.BisectBadFunc != nil {
		return m.BisectBadFunc(ctx)
	}
	return "", nil
}

func (m *MockClient) BisectReset(ctx context.Context) error {
	if m.BisectResetFunc != nil {
		return m.BisectResetFunc(ctx)
	}
	return nil
}

// --- ReflogOps ---

func (m *MockClient) Reflog(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
	if m.ReflogFunc != nil {
		return m.ReflogFunc(ctx, ref, limit)
	}
	return nil, nil
}

// --- DiscardOps ---

func (m *MockClient) DiscardFile(ctx context.Context, path string) error {
	if m.DiscardFileFunc != nil {
		return m.DiscardFileFunc(ctx, path)
	}
	return nil
}

func (m *MockClient) DiscardAllUnstaged(ctx context.Context) error {
	if m.DiscardAllFunc != nil {
		return m.DiscardAllFunc(ctx)
	}
	return nil
}

// --- RevertOps ---

func (m *MockClient) Revert(ctx context.Context, hash string) error {
	if m.RevertFunc != nil {
		return m.RevertFunc(ctx, hash)
	}
	return nil
}

func (m *MockClient) RevertContinue(ctx context.Context) error {
	if m.RevertContinueFunc != nil {
		return m.RevertContinueFunc(ctx)
	}
	return nil
}

func (m *MockClient) RevertAbort(ctx context.Context) error {
	if m.RevertAbortFunc != nil {
		return m.RevertAbortFunc(ctx)
	}
	return nil
}

// --- ResetOps ---

func (m *MockClient) Reset(ctx context.Context, ref string, mode git.ResetMode) error {
	if m.ResetFunc != nil {
		return m.ResetFunc(ctx, ref, mode)
	}
	return nil
}
