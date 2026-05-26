package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock infrastructure ---

type mockGitClient = gittest.MockClient

// --- Tests ---

func TestBuild_CleanRepo(t *testing.T) {
	mock := &mockGitClient{
		RepoRootFunc: func(context.Context) (string, error) { return "/home/user/project", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: "main", IsCurrent: true}}, nil
		},
		StatusFunc: func(context.Context) ([]git.FileStatus, error) { return nil, nil }, // no changes
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
		RepoRootFunc: func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: "feature/auth", IsCurrent: true}}, nil
		},
		StatusFunc: func(context.Context) ([]git.FileStatus, error) {
			return []git.FileStatus{
				{Path: "main.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
				{Path: "go.mod", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
				{Path: "new.go", StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusUnmodified},
				{Path: "old.go", StagedStatus: git.StatusDeleted, WorktreeStatus: git.StatusUnmodified},
				{Path: "tmp.txt", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
			}, nil
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
		RepoRootFunc: func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: "main", IsCurrent: true}}, nil
		},
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
		RepoRootFunc:   func(context.Context) (string, error) { return "", gitErr },
		BranchListFunc: func(context.Context) ([]git.Branch, error) { return nil, gitErr },
		StatusFunc:     func(context.Context) ([]git.FileStatus, error) { return nil, gitErr },
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
		RepoRootFunc: func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{
				{Name: "main", IsCurrent: false},
				{Name: "develop", IsCurrent: false},
			}, nil
		},
		StatusFunc: func(context.Context) ([]git.FileStatus, error) { return nil, nil },
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "Branch: (detached) (clean)")
}

func TestBuild_RenamedAndConflicted(t *testing.T) {
	mock := &mockGitClient{
		RepoRootFunc: func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: "merge-branch", IsCurrent: true}}, nil
		},
		StatusFunc: func(context.Context) ([]git.FileStatus, error) {
			return []git.FileStatus{
				{Path: "new_name.go", StagedStatus: git.StatusRenamed, WorktreeStatus: git.StatusUnmodified, OrigPath: "old_name.go"},
				{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
				{Path: "conflict2.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
			}, nil
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
		RepoRootFunc:   func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) { return nil, errors.New("branch error") },
		StatusFunc:     func(context.Context) ([]git.FileStatus, error) { return nil, errors.New("status error") },
	}

	builder := NewSystemPromptBuilder(mock, "")
	prompt := builder.Build(context.Background())

	assert.Contains(t, prompt, "Root: /repo")
	assert.Contains(t, prompt, "Branch: (detached) (clean)")
	assert.Contains(t, prompt, "Status: clean")
}

func TestBuild_PromptStructure(t *testing.T) {
	mock := &mockGitClient{
		RepoRootFunc: func(context.Context) (string, error) { return "/repo", nil },
		BranchListFunc: func(context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: "main", IsCurrent: true}}, nil
		},
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
