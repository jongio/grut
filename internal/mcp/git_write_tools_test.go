package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
)

// --- Write tool tests (staging, commit, branch, checkout, remote) ---

func TestGitTool_Stage(t *testing.T) {
	var staged []string
	mock := &mockGitClient{
		StageFunc: func(ctx context.Context, paths []string) error {
			staged = paths
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage", map[string]any{
		"paths": []any{"file1.go", "file2.go"},
	})
	assert.False(t, result.IsError)
	assert.Equal(t, []string{"file1.go", "file2.go"}, staged)
}

func TestGitTool_StageMissingPaths(t *testing.T) {
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage", nil)
	assert.True(t, result.IsError)
}

func TestGitTool_StageRejectsEmptyPathElement(t *testing.T) {
	mock := &mockGitClient{
		StageFunc: func(ctx context.Context, paths []string) error {
			t.Fatal("stage should not be called for invalid paths")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage", map[string]any{
		"paths": []any{""},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path validation")
}

func TestGitTool_StageRejectsTraversalPath(t *testing.T) {
	mock := &mockGitClient{
		StageFunc: func(ctx context.Context, paths []string) error {
			t.Fatal("stage should not be called for traversal paths")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage", map[string]any{
		"paths": []any{"../escape.txt"},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "must not contain '..'")
}

func TestGitTool_StageError(t *testing.T) {
	mock := &mockGitClient{
		StageFunc: func(ctx context.Context, paths []string) error {
			return fmt.Errorf("no paths provided")
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage", map[string]any{
		"paths": []any{"file.go"},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "no paths")
}

func TestGitTool_Unstage(t *testing.T) {
	var unstaged []string
	mock := &mockGitClient{
		UnstageFunc: func(ctx context.Context, paths []string) error {
			unstaged = paths
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_unstage", map[string]any{
		"paths": []any{"file.go"},
	})
	assert.False(t, result.IsError)
	assert.Equal(t, []string{"file.go"}, unstaged)
}

func TestGitTool_Commit(t *testing.T) {
	mock := &mockGitClient{
		CommitFunc: func(ctx context.Context, msg string, opts git.CommitOpts) (string, error) {
			assert.Equal(t, "feat: add feature", msg)
			assert.True(t, opts.Amend)
			return "abc123def456", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_commit", map[string]any{
		"message": "feat: add feature",
		"amend":   true,
	})
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "abc123def456")
}

func TestGitTool_CommitMissingMessage(t *testing.T) {
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_commit", nil)
	assert.True(t, result.IsError)
}

func TestGitTool_BranchCreate(t *testing.T) {
	var createdName, createdBase string
	mock := &mockGitClient{
		BranchCreateFunc: func(ctx context.Context, name, base string) error {
			createdName = name
			createdBase = base
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_branch_create", map[string]any{
		"name": "feature-x",
		"base": "main",
	})
	assert.False(t, result.IsError)
	assert.Equal(t, "feature-x", createdName)
	assert.Equal(t, "main", createdBase)
}

func TestGitTool_BranchDelete(t *testing.T) {
	var deletedName string
	var deletedForce bool
	mock := &mockGitClient{
		BranchDeleteFunc: func(ctx context.Context, name string, force bool) error {
			deletedName = name
			deletedForce = force
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_branch_delete", map[string]any{
		"name":  "old-branch",
		"force": true,
	})
	assert.False(t, result.IsError)
	assert.Equal(t, "old-branch", deletedName)
	assert.True(t, deletedForce)
}

func TestGitTool_BranchRename(t *testing.T) {
	mock := &mockGitClient{
		BranchRenameFunc: func(ctx context.Context, old, newName string) error {
			assert.Equal(t, "old-name", old)
			assert.Equal(t, "new-name", newName)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_branch_rename", map[string]any{
		"old_name": "old-name",
		"new_name": "new-name",
	})
	assert.False(t, result.IsError)
}

func TestGitTool_Checkout(t *testing.T) {
	mock := &mockGitClient{
		CheckoutFunc: func(ctx context.Context, ref string) error {
			assert.Equal(t, "develop", ref)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_checkout", map[string]any{"ref": "develop"})
	assert.False(t, result.IsError)
}

func TestGitTool_Push(t *testing.T) {
	mock := &mockGitClient{
		PushFunc: func(ctx context.Context, opts git.PushOpts) error {
			assert.Equal(t, "origin", opts.Remote)
			assert.Equal(t, "main", opts.Branch)
			assert.True(t, opts.Force)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_push", map[string]any{
		"remote": "origin",
		"branch": "main",
		"force":  true,
	})
	assert.False(t, result.IsError)
}

func TestGitTool_Pull(t *testing.T) {
	mock := &mockGitClient{
		PullFunc: func(ctx context.Context, opts git.PullOpts) error {
			assert.True(t, opts.Rebase)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_pull", map[string]any{"rebase": true})
	assert.False(t, result.IsError)
}

func TestGitTool_Fetch(t *testing.T) {
	mock := &mockGitClient{
		FetchFunc: func(ctx context.Context, opts git.FetchOpts) error {
			assert.True(t, opts.Prune)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_fetch", map[string]any{"prune": true})
	assert.False(t, result.IsError)
}
