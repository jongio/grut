package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Read tool tests ---

func TestGitTool_Status(t *testing.T) {
	mock := &mockGitClient{
		StatusFunc: func(ctx context.Context) ([]git.FileStatus, error) {
			return []git.FileStatus{
				{Path: "main.go", StagedStatus: git.StatusModified},
			}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_status", nil)
	assert.False(t, result.IsError)

	var statuses []git.FileStatus
	err := json.Unmarshal([]byte(resultText(t, result)), &statuses)
	require.NoError(t, err)
	assert.Len(t, statuses, 1)
	assert.Equal(t, "main.go", statuses[0].Path)
}

func TestGitTool_StatusError(t *testing.T) {
	mock := &mockGitClient{
		StatusFunc: func(ctx context.Context) ([]git.FileStatus, error) {
			return nil, fmt.Errorf("not a git repo")
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_status", nil)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "not a git repo")
}

func TestGitTool_Diff(t *testing.T) {
	mock := &mockGitClient{
		DiffFunc: func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			assert.True(t, opts.Staged)
			return []git.FileDiff{{Path: "main.go"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_diff", map[string]any{"staged": true})
	assert.False(t, result.IsError)
}

func TestGitTool_Log(t *testing.T) {
	mock := &mockGitClient{
		LogFunc: func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
			assert.Equal(t, 5, opts.MaxCount)
			return []git.Commit{{Hash: "abc123"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_log", map[string]any{"limit": float64(5)})
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "abc123")
}

func TestGitTool_Blame(t *testing.T) {
	mock := &mockGitClient{
		BlameFunc: func(ctx context.Context, path string) ([]git.BlameLine, error) {
			assert.Equal(t, "main.go", path)
			return []git.BlameLine{{Hash: "abc"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_blame", map[string]any{"path": "main.go"})
	assert.False(t, result.IsError)
}

func TestGitTool_BlameMissingPath(t *testing.T) {
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_blame", nil)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path is required")
}

func TestGitTool_BranchList(t *testing.T) {
	mock := &mockGitClient{
		BranchListFunc: func(ctx context.Context) ([]git.Branch, error) {
			return []git.Branch{
				{Name: "main", IsCurrent: true},
				{Name: "dev"},
			}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_branch_list", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "main")
}

func TestGitTool_StashList(t *testing.T) {
	mock := &mockGitClient{
		StashListFunc: func(ctx context.Context) ([]git.StashEntry, error) {
			return []git.StashEntry{{Index: 0, Message: "WIP"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stash_list", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "WIP")
}

func TestGitTool_TagList(t *testing.T) {
	mock := &mockGitClient{
		TagListFunc: func(ctx context.Context) ([]git.Tag, error) {
			return []git.Tag{{Name: "v1.0"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_tag_list", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "v1.0")
}

func TestGitTool_WorktreeList(t *testing.T) {
	mock := &mockGitClient{
		WorktreeListFunc: func(ctx context.Context) ([]git.Worktree, error) {
			return []git.Worktree{{Path: "/repo"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_worktree_list", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "/repo")
}

func TestGitTool_Reflog(t *testing.T) {
	mock := &mockGitClient{
		ReflogFunc: func(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
			assert.Equal(t, "main", ref)
			assert.Equal(t, 10, limit)
			return []git.ReflogEntry{{Hash: "abc"}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_reflog", map[string]any{"ref": "main", "limit": float64(10)})
	assert.False(t, result.IsError)
}

func TestGitTool_IsRepo(t *testing.T) {
	mock := &mockGitClient{
		IsRepoFunc: func(ctx context.Context) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_is_repo", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "true")
}

func TestGitTool_RepoRoot(t *testing.T) {
	mock := &mockGitClient{
		RepoRootFunc: func(ctx context.Context) (string, error) {
			return "/repo", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_repo_root", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "/repo")
}
