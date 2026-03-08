package mcp

import (
	"context"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Ops tool tests (merge, rebase, cherry-pick, stash, tag, worktree, bisect) ---

func TestGitTool_Merge(t *testing.T) {
	mock := &mockGitClient{
		MergeFunc: func(ctx context.Context, branch string, opts git.MergeOpts) error {
			assert.Equal(t, "feature", branch)
			assert.True(t, opts.NoFF)
			assert.True(t, opts.Squash)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_merge", map[string]any{
		"branch": "feature",
		"no_ff":  true,
		"squash": true,
	})
	assert.False(t, result.IsError)
}

func TestGitTool_MergeAbort(t *testing.T) {
	called := false
	mock := &mockGitClient{
		MergeAbortFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_merge_abort", nil)
	assert.False(t, result.IsError)
	assert.True(t, called)
}

func TestGitTool_Rebase(t *testing.T) {
	mock := &mockGitClient{
		RebaseFunc: func(ctx context.Context, onto string, opts git.RebaseOpts) error {
			assert.Equal(t, "main", onto)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_rebase", map[string]any{"onto": "main"})
	assert.False(t, result.IsError)
}

func TestGitTool_RebaseContinue(t *testing.T) {
	called := false
	mock := &mockGitClient{
		RebaseContinueFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_rebase_continue", nil)
	assert.False(t, result.IsError)
	assert.True(t, called)
}

func TestGitTool_RebaseAbort(t *testing.T) {
	called := false
	mock := &mockGitClient{
		RebaseAbortFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_rebase_abort", nil)
	assert.False(t, result.IsError)
	assert.True(t, called)
}

func TestGitTool_CherryPick(t *testing.T) {
	mock := &mockGitClient{
		CherryPickFunc: func(ctx context.Context, hash string) error {
			assert.Equal(t, "abc123", hash)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_cherry_pick", map[string]any{"commit": "abc123"})
	assert.False(t, result.IsError)
}

func TestGitTool_StashPush(t *testing.T) {
	mock := &mockGitClient{
		StashPushFunc: func(ctx context.Context, opts git.StashOpts) error {
			assert.Equal(t, "WIP", opts.Message)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stash_push", map[string]any{"message": "WIP"})
	assert.False(t, result.IsError)
}

func TestGitTool_StashPop(t *testing.T) {
	mock := &mockGitClient{
		StashPopFunc: func(ctx context.Context, index int) error {
			assert.Equal(t, 0, index)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stash_pop", nil)
	assert.False(t, result.IsError)
}

func TestGitTool_StashApply(t *testing.T) {
	mock := &mockGitClient{
		StashApplyFunc: func(ctx context.Context, index int) error {
			assert.Equal(t, 0, index)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stash_apply", nil)
	assert.False(t, result.IsError)
}

func TestGitTool_StashDrop(t *testing.T) {
	mock := &mockGitClient{
		StashDropFunc: func(ctx context.Context, index int) error {
			assert.Equal(t, 0, index)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stash_drop", nil)
	assert.False(t, result.IsError)
}

func TestGitTool_TagCreate(t *testing.T) {
	mock := &mockGitClient{
		TagCreateFunc: func(ctx context.Context, name, ref, message string) error {
			assert.Equal(t, "v1.0", name)
			assert.Equal(t, "abc123", ref)
			assert.Equal(t, "Release 1.0", message)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_tag_create", map[string]any{
		"name":    "v1.0",
		"ref":     "abc123",
		"message": "Release 1.0",
	})
	assert.False(t, result.IsError)
}

func TestGitTool_TagDelete(t *testing.T) {
	mock := &mockGitClient{
		TagDeleteFunc: func(ctx context.Context, name string) error {
			assert.Equal(t, "v0.9", name)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_tag_delete", map[string]any{"name": "v0.9"})
	assert.False(t, result.IsError)
}

func TestGitTool_WorktreeAdd(t *testing.T) {
	repoRoot := t.TempDir()
	worktreePath := "worktrees/wt"
	mock := &mockGitClient{
		WorktreeAddFunc: func(ctx context.Context, path, branch string) error {
			assert.Equal(t, worktreePath, path)
			assert.Equal(t, "feature", branch)
			return nil
		},
	}
	srv := newTestServer(t, mock, repoRoot)

	result := callTool(t, srv, "git_worktree_add", map[string]any{
		"path":   worktreePath,
		"branch": "feature",
	})
	assert.False(t, result.IsError)
}

func TestGitTool_WorktreeRemove(t *testing.T) {
	repoRoot := t.TempDir()
	worktreePath := "worktrees/wt"
	mock := &mockGitClient{
		WorktreeRemoveFunc: func(ctx context.Context, path string, force bool) error {
			assert.Equal(t, worktreePath, path)
			assert.True(t, force)
			return nil
		},
	}
	srv := newTestServer(t, mock, repoRoot)

	result := callTool(t, srv, "git_worktree_remove", map[string]any{
		"path":  worktreePath,
		"force": true,
	})
	assert.False(t, result.IsError)
}

func TestGitTool_WorktreeAdd_RejectsPathOutsideJail(t *testing.T) {
	mock := &mockGitClient{
		WorktreeAddFunc: func(ctx context.Context, path, branch string) error {
			t.Fatal("worktree add should not be called for path outside jail")
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_worktree_add", map[string]any{
		"path": "../outside-worktree",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path validation")
}

func TestGitTool_BisectStart(t *testing.T) {
	mock := &mockGitClient{
		BisectStartFunc: func(ctx context.Context, bad, good string) error {
			assert.Equal(t, "HEAD", bad)
			assert.Equal(t, "v1.0", good)
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_bisect_start", map[string]any{
		"bad":  "HEAD",
		"good": "v1.0",
	})
	assert.False(t, result.IsError)
}

func TestGitTool_BisectGood(t *testing.T) {
	mock := &mockGitClient{
		BisectGoodFunc: func(ctx context.Context) (string, error) {
			return "bisect: good", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_bisect_good", nil)
	assert.False(t, result.IsError)
}

func TestGitTool_BisectBad(t *testing.T) {
	mock := &mockGitClient{
		BisectBadFunc: func(ctx context.Context) (string, error) {
			return "bisect: bad", nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_bisect_bad", nil)
	assert.False(t, result.IsError)
}

func TestGitTool_BisectReset(t *testing.T) {
	called := false
	mock := &mockGitClient{
		BisectResetFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_bisect_reset", nil)
	assert.False(t, result.IsError)
	assert.True(t, called)
}

func TestGitTool_DiscardRejectsTraversalPath(t *testing.T) {
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_discard", map[string]any{"path": "../escape.txt"})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "must not contain '..'")
}

func TestGitTool_StageHunkRejectsNegativeIndex(t *testing.T) {
	mock := &mockGitClient{
		DiffFunc: func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			t.Fatal("diff should not be called for invalid hunk index")
			return nil, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "file.go",
		"hunk_index": -1,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "hunk_index is required and must be >= 0")
}

func TestGitTool_StageHunkRejectsTraversalPath(t *testing.T) {
	mock := &mockGitClient{
		DiffFunc: func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			t.Fatal("diff should not be called for traversal paths")
			return nil, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "../escape.txt",
		"hunk_index": 0,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "must not contain '..'")
}

func TestGitTool_StageHunkRejectsOutOfRangeIndex(t *testing.T) {
	mock := &mockGitClient{
		DiffFunc: func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{{Path: opts.Path, Hunks: []git.Hunk{{Header: "@@ -1,1 +1,1 @@"}}}}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())

	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "file.go",
		"hunk_index": 1,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "out of range")
}

// --- Middleware integration test ---

func TestGitTool_RateLimited(t *testing.T) {
	mock := &mockGitClient{}
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Security: config.MCPSecurityConfig{
				RateLimitRead:  2, // very low limit
				RateLimitWrite: 2,
				AuditLog:       false,
			},
		},
	}
	root := t.TempDir()
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)

	// First two calls succeed.
	r1 := callTool(t, srv, "git_status", nil)
	assert.False(t, r1.IsError)
	r2 := callTool(t, srv, "git_status", nil)
	assert.False(t, r2.IsError)

	// Third call should be rate limited.
	r3 := callTool(t, srv, "git_status", nil)
	assert.True(t, r3.IsError)
	assert.Contains(t, resultText(t, r3), "rate limit")
}
