package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ────────────── git_stash_show ──────────────

func TestGitTool_StashShow(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_stash_show", map[string]any{})
	require.NotNil(t, result)
	// Default mock returns empty string → "empty stash diff"
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "empty stash diff")
}

func TestGitTool_StashShowWithIndex(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_stash_show", map[string]any{"index": 2})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

// ────────────── git_discard ──────────────

func TestGitTool_Discard(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_discard", map[string]any{"path": "main.go"})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "discarded changes")
}

// ────────────── git_discard_all ──────────────

func TestGitTool_DiscardAll(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_discard_all", map[string]any{})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "discarded all unstaged changes")
}

// ────────────── git_revert ──────────────

func TestGitTool_Revert(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_revert", map[string]any{"hash": "abc123"})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "reverted: abc123")
}

func TestGitTool_RevertMissingHash(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_revert", map[string]any{})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "hash is required")
}

// ────────────── git_revert_continue ──────────────

func TestGitTool_RevertContinue(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_revert_continue", map[string]any{})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "revert continued")
}

// ────────────── git_revert_abort ──────────────

func TestGitTool_RevertAbort(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_revert_abort", map[string]any{})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "revert aborted")
}

// ────────────── git_reset ──────────────

func TestGitTool_Reset(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_reset", map[string]any{
		"ref":  "HEAD~1",
		"mode": "soft",
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "reset --soft to HEAD~1")
}

func TestGitTool_ResetMissingRef(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_reset", map[string]any{"mode": "mixed"})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "ref is required")
}

func TestGitTool_ResetMissingMode(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_reset", map[string]any{"ref": "HEAD"})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "mode is required")
}

// ────────────── git_stage_hunk (success path) ──────────────

func TestGitTool_StageHunkSuccess(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{
				{
					Path: opts.Path,
					Hunks: []git.Hunk{
						{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4, Header: "@@ -1,3 +1,4 @@"},
					},
				},
			}, nil
		},
		StageHunkFunc: func(_ context.Context, _ string, _ git.Hunk) error {
			return nil
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "staged hunk 0")
}

func TestGitTool_StageHunkDiffError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return nil, fmt.Errorf("diff failed")
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "diff")
}

func TestGitTool_StageHunkOutOfRange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{{Path: "main.go", Hunks: nil}}, nil
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 5,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "out of range")
}

func TestGitTool_StageHunkGitError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{
				{Path: "main.go", Hunks: []git.Hunk{{Header: "@@"}}},
			}, nil
		},
		StageHunkFunc: func(_ context.Context, _ string, _ git.Hunk) error {
			return fmt.Errorf("apply failed")
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_stage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "stage hunk")
}

// ────────────── git_unstage_hunk ──────────────

func TestGitTool_UnstageHunk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			assert.True(t, opts.Staged, "unstage_hunk should request staged diff")
			return []git.FileDiff{
				{
					Path: opts.Path,
					Hunks: []git.Hunk{
						{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4, Header: "@@ -1,3 +1,4 @@"},
					},
				},
			}, nil
		},
		UnstageHunkFunc: func(_ context.Context, _ string, _ git.Hunk) error {
			return nil
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "unstaged hunk 0")
}

func TestGitTool_UnstageHunkMissingPath(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path is required")
}

func TestGitTool_UnstageHunkNegativeIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": -1,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "hunk_index is required")
}

func TestGitTool_UnstageHunkOutOfRange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{}, nil // empty diffs
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "out of range")
}

func TestGitTool_UnstageHunkGitError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return []git.FileDiff{
				{Path: "main.go", Hunks: []git.Hunk{{Header: "@@"}}},
			}, nil
		},
		UnstageHunkFunc: func(_ context.Context, _ string, _ git.Hunk) error {
			return fmt.Errorf("apply failed")
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "unstage hunk")
}

func TestGitTool_UnstageHunkDiffError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return nil, fmt.Errorf("diff failed")
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_unstage_hunk", map[string]any{
		"path":       "main.go",
		"hunk_index": 0,
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "diff")
}

// ────────────── git_diff path validation ──────────────

func TestGitTool_DiffWithPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		DiffFunc: func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			assert.NotEmpty(t, opts.Path, "path should be set")
			return []git.FileDiff{}, nil
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_diff", map[string]any{
		"path": "main.go",
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestGitTool_DiffPathTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_diff", map[string]any{
		"path": "../../etc/passwd",
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path validation")
}

// ────────────── git_log path validation ──────────────

func TestGitTool_LogWithPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{
		LogFunc: func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
			assert.NotEmpty(t, opts.Path, "path should be set")
			return []git.Commit{}, nil
		},
	}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_log", map[string]any{
		"path": "main.go",
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestGitTool_LogPathTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mock := &mockGitClient{}
	srv := newTestServer(t, mock, root)
	result := callTool(t, srv, "git_log", map[string]any{
		"path": "../../etc/passwd",
	})
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path validation")
}

// ────────────── git_reflog with params ──────────────

func TestGitTool_ReflogWithParams(t *testing.T) {
	t.Parallel()
	mock := &mockGitClient{
		ReflogFunc: func(_ context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
			assert.Equal(t, "main", ref)
			assert.Equal(t, 5, limit)
			return []git.ReflogEntry{}, nil
		},
	}
	srv := newTestServer(t, mock, t.TempDir())
	result := callTool(t, srv, "git_reflog", map[string]any{
		"ref":   "main",
		"limit": 5,
	})
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}
