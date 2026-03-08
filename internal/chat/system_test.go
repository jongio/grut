package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock infrastructure ---

// stubGitClient provides panic-on-call stubs for all git.GitClient methods.
// Embed this in test mocks and override only the methods under test.
type stubGitClient struct{}

func (stubGitClient) Status(context.Context) ([]git.FileStatus, error) {
	panic("stub: Status not implemented")
}

func (stubGitClient) Diff(context.Context, git.DiffOpts) ([]git.FileDiff, error) {
	panic("stub: Diff not implemented")
}

func (stubGitClient) Log(context.Context, git.LogOpts) ([]git.Commit, error) {
	panic("stub: Log not implemented")
}

func (stubGitClient) Blame(context.Context, string) ([]git.BlameLine, error) {
	panic("stub: Blame not implemented")
}

func (stubGitClient) RepoRoot(context.Context) (string, error) {
	panic("stub: RepoRoot not implemented")
}

func (stubGitClient) IsRepo(context.Context) (bool, error) {
	panic("stub: IsRepo not implemented")
}

func (stubGitClient) DiffTreeFiles(context.Context, string) ([]string, error) {
	panic("stub: DiffTreeFiles not implemented")
}

func (stubGitClient) Stage(context.Context, []string) error {
	panic("stub: Stage not implemented")
}

func (stubGitClient) Unstage(context.Context, []string) error {
	panic("stub: Unstage not implemented")
}

func (stubGitClient) StageHunk(context.Context, string, git.Hunk) error {
	panic("stub: StageHunk not implemented")
}

func (stubGitClient) UnstageHunk(context.Context, string, git.Hunk) error {
	panic("stub: UnstageHunk not implemented")
}

func (stubGitClient) StageLine(context.Context, string, git.Hunk, int) error {
	panic("stub: StageLine not implemented")
}

func (stubGitClient) UnstageLine(context.Context, string, git.Hunk, int) error {
	panic("stub: UnstageLine not implemented")
}

func (stubGitClient) Commit(context.Context, string, git.CommitOpts) (string, error) {
	panic("stub: Commit not implemented")
}

func (stubGitClient) BranchList(context.Context) ([]git.Branch, error) {
	panic("stub: BranchList not implemented")
}

func (stubGitClient) BranchCreate(context.Context, string, string) error {
	panic("stub: BranchCreate not implemented")
}

func (stubGitClient) BranchDelete(context.Context, string, bool) error {
	panic("stub: BranchDelete not implemented")
}

func (stubGitClient) BranchRename(context.Context, string, string) error {
	panic("stub: BranchRename not implemented")
}

func (stubGitClient) Checkout(context.Context, string) error {
	panic("stub: Checkout not implemented")
}

func (stubGitClient) Push(context.Context, git.PushOpts) error {
	panic("stub: Push not implemented")
}

func (stubGitClient) Pull(context.Context, git.PullOpts) error {
	panic("stub: Pull not implemented")
}

func (stubGitClient) Fetch(context.Context, git.FetchOpts) error {
	panic("stub: Fetch not implemented")
}

func (stubGitClient) WorktreeList(context.Context) ([]git.Worktree, error) {
	panic("stub: WorktreeList not implemented")
}

func (stubGitClient) WorktreeAdd(context.Context, string, string) error {
	panic("stub: WorktreeAdd not implemented")
}

func (stubGitClient) WorktreeRemove(context.Context, string, bool) error {
	panic("stub: WorktreeRemove not implemented")
}

func (stubGitClient) StashList(context.Context) ([]git.StashEntry, error) {
	panic("stub: StashList not implemented")
}

func (stubGitClient) StashShow(context.Context, int) (string, error) {
	panic("stub: StashShow not implemented")
}

func (stubGitClient) StashPush(context.Context, git.StashOpts) error {
	panic("stub: StashPush not implemented")
}

func (stubGitClient) StashPop(context.Context, int) error {
	panic("stub: StashPop not implemented")
}

func (stubGitClient) StashApply(context.Context, int) error {
	panic("stub: StashApply not implemented")
}

func (stubGitClient) StashDrop(context.Context, int) error {
	panic("stub: StashDrop not implemented")
}

func (stubGitClient) TagList(context.Context) ([]git.Tag, error) {
	panic("stub: TagList not implemented")
}

func (stubGitClient) TagCreate(context.Context, string, string, string) error {
	panic("stub: TagCreate not implemented")
}

func (stubGitClient) TagDelete(context.Context, string) error {
	panic("stub: TagDelete not implemented")
}

func (stubGitClient) TagListRemote(context.Context, string) ([]git.Tag, error) {
	panic("stub: TagListRemote not implemented")
}

func (stubGitClient) TagPush(context.Context, string, string) error {
	panic("stub: TagPush not implemented")
}

func (stubGitClient) TagPushAll(context.Context, string) error {
	panic("stub: TagPushAll not implemented")
}

func (stubGitClient) Merge(context.Context, string, git.MergeOpts) error {
	panic("stub: Merge not implemented")
}

func (stubGitClient) MergeAbort(context.Context) error {
	panic("stub: MergeAbort not implemented")
}

func (stubGitClient) Rebase(context.Context, string, git.RebaseOpts) error {
	panic("stub: Rebase not implemented")
}

func (stubGitClient) RebaseContinue(context.Context) error {
	panic("stub: RebaseContinue not implemented")
}

func (stubGitClient) RebaseAbort(context.Context) error {
	panic("stub: RebaseAbort not implemented")
}

func (stubGitClient) CherryPick(context.Context, string) error {
	panic("stub: CherryPick not implemented")
}

func (stubGitClient) BisectStart(context.Context, string, string) error {
	panic("stub: BisectStart not implemented")
}

func (stubGitClient) BisectGood(context.Context) (string, error) {
	panic("stub: BisectGood not implemented")
}

func (stubGitClient) BisectBad(context.Context) (string, error) {
	panic("stub: BisectBad not implemented")
}

func (stubGitClient) BisectReset(context.Context) error {
	panic("stub: BisectReset not implemented")
}

func (stubGitClient) Reflog(context.Context, string, int) ([]git.ReflogEntry, error) {
	panic("stub: Reflog not implemented")
}

func (stubGitClient) RemoteList(context.Context) ([]git.Remote, error) {
	panic("stub: RemoteList not implemented")
}

func (stubGitClient) RemoteAdd(context.Context, string, string) error {
	panic("stub: RemoteAdd not implemented")
}

func (stubGitClient) RemoteRemove(context.Context, string) error {
	panic("stub: RemoteRemove not implemented")
}

func (stubGitClient) DiscardFile(context.Context, string) error {
	panic("stub: DiscardFile not implemented")
}

func (stubGitClient) DiscardAllUnstaged(context.Context) error {
	panic("stub: DiscardAllUnstaged not implemented")
}
func (stubGitClient) Revert(context.Context, string) error { panic("stub: Revert not implemented") }
func (stubGitClient) RevertContinue(context.Context) error {
	panic("stub: RevertContinue not implemented")
}
func (stubGitClient) RevertAbort(context.Context) error { panic("stub: RevertAbort not implemented") }
func (stubGitClient) Reset(context.Context, string, git.ResetMode) error {
	panic("stub: Reset not implemented")
}

// mockGitClient embeds stubGitClient and overrides only the methods
// that SystemPromptBuilder uses: RepoRoot, Status, BranchList.
type mockGitClient struct {
	stubGitClient
	repoRoot    string
	repoRootErr error
	statuses    []git.FileStatus
	statusErr   error
	branches    []git.Branch
	branchErr   error
}

func (m *mockGitClient) RepoRoot(context.Context) (string, error) {
	return m.repoRoot, m.repoRootErr
}

func (m *mockGitClient) Status(context.Context) ([]git.FileStatus, error) {
	return m.statuses, m.statusErr
}

func (m *mockGitClient) BranchList(context.Context) ([]git.Branch, error) {
	return m.branches, m.branchErr
}

// --- Tests ---

func TestBuild_CleanRepo(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/home/user/project",
		branches: []git.Branch{
			{Name: "main", IsCurrent: true},
		},
		statuses: nil, // no changes
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "grut's AI assistant")
	assert.Contains(t, prompt, "Root: /home/user/project")
	assert.Contains(t, prompt, "Branch: main (clean)")
	assert.Contains(t, prompt, "Status: clean")
}

func TestBuild_DirtyRepo(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/repo",
		branches: []git.Branch{
			{Name: "feature/auth", IsCurrent: true},
		},
		statuses: []git.FileStatus{
			{Path: "main.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			{Path: "go.mod", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
			{Path: "new.go", StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusUnmodified},
			{Path: "old.go", StagedStatus: git.StatusDeleted, WorktreeStatus: git.StatusUnmodified},
			{Path: "tmp.txt", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		},
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "Branch: feature/auth (5 uncommitted changes)")
	assert.Contains(t, prompt, "2 modified")
	assert.Contains(t, prompt, "1 added")
	assert.Contains(t, prompt, "1 deleted")
	assert.Contains(t, prompt, "1 untracked")
}

func TestBuild_Override(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/repo",
		branches: []git.Branch{{Name: "main", IsCurrent: true}},
	}
	customPrompt := "You are a custom assistant. Do exactly what the user says."

	builder := NewSystemPromptBuilder(mock, customPrompt)
	prompt := builder.Build(context.Background())

	assert.Equal(t, customPrompt, prompt)
	assert.NotContains(t, prompt, "grut's AI assistant")
}

func TestBuild_AllGitErrors(t *testing.T) {
	gitErr := errors.New("git: not a repository")
	mock := &mockGitClient{
		repoRootErr: gitErr,
		branchErr:   gitErr,
		statusErr:   gitErr,
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	// Should still produce a usable prompt with fallback values
	require.Contains(t, prompt, "grut's AI assistant")
	assert.Contains(t, prompt, "Root: (unknown)")
	assert.Contains(t, prompt, "Branch: (detached) (clean)")
	assert.Contains(t, prompt, "Status: clean")
}

func TestBuild_DetachedHead(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/repo",
		branches: []git.Branch{
			{Name: "main", IsCurrent: false},
			{Name: "develop", IsCurrent: false},
		},
		statuses: nil,
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "Branch: (detached) (clean)")
}

func TestBuild_RenamedAndConflicted(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/repo",
		branches: []git.Branch{{Name: "merge-branch", IsCurrent: true}},
		statuses: []git.FileStatus{
			{Path: "new_name.go", StagedStatus: git.StatusRenamed, WorktreeStatus: git.StatusUnmodified, OrigPath: "old_name.go"},
			{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
			{Path: "conflict2.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		},
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "3 uncommitted changes")
	assert.Contains(t, prompt, "1 renamed")
	assert.Contains(t, prompt, "2 conflicted")
}

func TestBuild_PartialErrors(t *testing.T) {
	// RepoRoot works, but BranchList and Status fail
	mock := &mockGitClient{
		repoRoot:  "/repo",
		branchErr: errors.New("branch error"),
		statusErr: errors.New("status error"),
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "Root: /repo")
	assert.Contains(t, prompt, "Branch: (detached) (clean)")
	assert.Contains(t, prompt, "Status: clean")
}

func TestBuild_PromptStructure(t *testing.T) {
	mock := &mockGitClient{
		repoRoot: "/repo",
		branches: []git.Branch{{Name: "main", IsCurrent: true}},
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	// Verify all expected sections exist
	sections := []string{
		"Available capabilities:",
		"File operations:",
		"Git operations:",
		"Navigation:",
		"Search:",
		"Current repository:",
		"Rules:",
	}
	for _, section := range sections {
		assert.Contains(t, prompt, section, "missing section: %s", section)
	}
}

func TestBuildStatusSummary(t *testing.T) {
	tests := []struct {
		name     string
		statuses []git.FileStatus
		want     string
	}{
		{
			name:     "single modified",
			statuses: []git.FileStatus{{StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}},
			want:     "1 modified",
		},
		{
			name: "mixed changes",
			statuses: []git.FileStatus{
				{StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
				{StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
				{StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
			},
			want: "2 modified, 1 untracked",
		},
		{
			name:     "empty slice",
			statuses: []git.FileStatus{},
			want:     "clean",
		},
		{
			name: "all categories",
			statuses: []git.FileStatus{
				{StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
				{StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusUnmodified},
				{StagedStatus: git.StatusDeleted, WorktreeStatus: git.StatusUnmodified},
				{StagedStatus: git.StatusRenamed, WorktreeStatus: git.StatusUnmodified},
				{StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
				{StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
			},
			want: "1 modified, 1 added, 1 deleted, 1 renamed, 1 untracked, 1 conflicted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStatusSummary(tt.statuses)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildStatusSummary_ConflictPriority(t *testing.T) {
	// A file with conflict status in worktree should count as conflicted,
	// not modified, even if staged status is something else.
	statuses := []git.FileStatus{
		{StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusConflict},
	}

	got := buildStatusSummary(statuses)
	assert.Equal(t, "1 conflicted", got)
	assert.True(t, !strings.Contains(got, "modified"))
}

func TestNewSystemPromptBuilder(t *testing.T) {
	mock := &mockGitClient{}
	builder := NewSystemPromptBuilder(mock, "custom")

	require.NotNil(t, builder)
	assert.Equal(t, "custom", builder.override)
}
