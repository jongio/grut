package chat

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests: git_blame
// ---------------------------------------------------------------------------

func TestGitBlame_Success(t *testing.T) {
	mock := &executorMockGitClient{
		blameRes: []git.BlameLine{
			{Hash: "aaa111", Author: "alice", LineNo: 1, Content: "package main"},
			{Hash: "bbb222", Author: "bob", LineNo: 2, Content: "func main() {}"},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "blame-1",
		Name:      "git_blame",
		Arguments: map[string]any{"path": "main.go"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "aaa111")
	assert.Contains(t, result.Content, "alice")
	assert.Contains(t, result.Content, "bob")
}

func TestGitBlame_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "blame-2",
		Name: "git_blame",
	})
	assert.Contains(t, result.Error, "path is required")
}

func TestGitBlame_Error(t *testing.T) {
	mock := &executorMockGitClient{
		blameErr: errors.New("file not tracked"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "blame-3",
		Name:      "git_blame",
		Arguments: map[string]any{"path": "unknown.go"},
	})
	assert.Contains(t, result.Error, "git blame")
	assert.Contains(t, result.Error, "file not tracked")
}

// ---------------------------------------------------------------------------
// Tests: git_branch_list
// ---------------------------------------------------------------------------

func TestGitBranchList_Success(t *testing.T) {
	mock := &executorMockGitClient{
		branchListRes: []git.Branch{
			{Name: "main", IsCurrent: true},
			{Name: "develop", IsCurrent: false},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bl-1",
		Name: "git_branch_list",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "main")
	assert.Contains(t, result.Content, "develop")
}

func TestGitBranchList_Error(t *testing.T) {
	mock := &executorMockGitClient{
		branchListErr: errors.New("not a git repo"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bl-2",
		Name: "git_branch_list",
	})
	assert.Contains(t, result.Error, "git branch list")
}

// ---------------------------------------------------------------------------
// Tests: git_stash_list
// ---------------------------------------------------------------------------

func TestGitStashList_Success(t *testing.T) {
	mock := &executorMockGitClient{
		stashListRes: []git.StashEntry{
			{Index: 0, Message: "WIP on main"},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sl-1",
		Name: "git_stash_list",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "WIP on main")
}

func TestGitStashList_Empty(t *testing.T) {
	mock := &executorMockGitClient{
		stashListRes: nil,
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sl-2",
		Name: "git_stash_list",
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "no stash entries", result.Content)
}

func TestGitStashList_Error(t *testing.T) {
	mock := &executorMockGitClient{
		stashListErr: errors.New("stash error"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sl-3",
		Name: "git_stash_list",
	})
	assert.Contains(t, result.Error, "git stash list")
}

// ---------------------------------------------------------------------------
// Tests: git_worktree_list
// ---------------------------------------------------------------------------

func TestGitWorktreeList_Success(t *testing.T) {
	mock := &executorMockGitClient{
		worktreeRes: []git.Worktree{
			{Path: "/repo", Branch: "main", Head: "abc123"},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "wt-1",
		Name: "git_worktree_list",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "main")
	assert.Contains(t, result.Content, "abc123")
}

func TestGitWorktreeList_Error(t *testing.T) {
	mock := &executorMockGitClient{
		worktreeErr: errors.New("worktree fail"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "wt-2",
		Name: "git_worktree_list",
	})
	assert.Contains(t, result.Error, "git worktree list")
}

// ---------------------------------------------------------------------------
// Tests: git_unstage
// ---------------------------------------------------------------------------

func TestGitUnstage_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "unstage-1",
		Name: "git_unstage",
		Arguments: map[string]any{
			"paths": []any{"file1.go", "file2.go"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "unstaged 2 file(s)")
}

func TestGitUnstage_MissingPaths(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "unstage-2",
		Name: "git_unstage",
	})
	assert.Contains(t, result.Error, "paths is required")
}

func TestGitUnstage_Error(t *testing.T) {
	mock := &executorMockGitClient{
		unstageErr: errors.New("unstage failed"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "unstage-3",
		Name: "git_unstage",
		Arguments: map[string]any{
			"paths": []any{"file1.go"},
		},
	})
	assert.Contains(t, result.Error, "git unstage")
}

// ---------------------------------------------------------------------------
// Tests: git_pull
// ---------------------------------------------------------------------------

func TestGitPull_DefaultRemote(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "pull-1",
		Name: "git_pull",
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "pulled successfully", result.Content)
}

func TestGitPull_CustomRemote(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "pull-2",
		Name:      "git_pull",
		Arguments: map[string]any{"remote": "upstream"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "pulled successfully", result.Content)
}

func TestGitPull_Error(t *testing.T) {
	mock := &executorMockGitClient{
		pullErr: errors.New("merge conflict"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "pull-3",
		Name: "git_pull",
	})
	assert.Contains(t, result.Error, "git pull")
	assert.Contains(t, result.Error, "merge conflict")
}

// ---------------------------------------------------------------------------
// Tests: git_fetch
// ---------------------------------------------------------------------------

func TestGitFetch_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fetch-1",
		Name: "git_fetch",
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "fetched successfully", result.Content)
}

func TestGitFetch_WithRemote(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "fetch-2",
		Name:      "git_fetch",
		Arguments: map[string]any{"remote": "upstream"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "fetched successfully", result.Content)
}

func TestGitFetch_Error(t *testing.T) {
	mock := &executorMockGitClient{
		fetchErr: errors.New("network error"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fetch-3",
		Name: "git_fetch",
	})
	assert.Contains(t, result.Error, "git fetch")
	assert.Contains(t, result.Error, "network error")
}

// ---------------------------------------------------------------------------
// Tests: git_tag_create
// ---------------------------------------------------------------------------

func TestGitTagCreate_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "tag-c1",
		Name: "git_tag_create",
		Arguments: map[string]any{
			"name":    "v1.0.0",
			"ref":     "HEAD",
			"message": "Release 1.0.0",
		},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "created tag v1.0.0", result.Content)
}

func TestGitTagCreate_MissingName(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "tag-c2",
		Name: "git_tag_create",
	})
	assert.Contains(t, result.Error, "name is required")
}

func TestGitTagCreate_Error(t *testing.T) {
	mock := &executorMockGitClient{
		tagCreateErr: errors.New("tag exists"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "tag-c3",
		Name:      "git_tag_create",
		Arguments: map[string]any{"name": "v1.0.0"},
	})
	assert.Contains(t, result.Error, "git tag create")
}

// ---------------------------------------------------------------------------
// Tests: git_tag_delete
// ---------------------------------------------------------------------------

func TestGitTagDelete_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "tag-d1",
		Name:      "git_tag_delete",
		Arguments: map[string]any{"name": "v0.9.0"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "deleted tag v0.9.0", result.Content)
}

func TestGitTagDelete_MissingName(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "tag-d2",
		Name: "git_tag_delete",
	})
	assert.Contains(t, result.Error, "name is required")
}

func TestGitTagDelete_Error(t *testing.T) {
	mock := &executorMockGitClient{
		tagDeleteErr: errors.New("no such tag"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "tag-d3",
		Name:      "git_tag_delete",
		Arguments: map[string]any{"name": "v0.9.0"},
	})
	assert.Contains(t, result.Error, "git tag delete")
}

// ---------------------------------------------------------------------------
// Tests: git_stash_push / git_stash_pop
// ---------------------------------------------------------------------------

func TestGitStashPush_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "sp-1",
		Name:      "git_stash_push",
		Arguments: map[string]any{"message": "WIP"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "stashed changes", result.Content)
}

func TestGitStashPush_Error(t *testing.T) {
	mock := &executorMockGitClient{
		stashPushErr: errors.New("no changes"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sp-2",
		Name: "git_stash_push",
	})
	assert.Contains(t, result.Error, "git stash push")
}

func TestGitStashPop_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "spop-1",
		Name:      "git_stash_pop",
		Arguments: map[string]any{"index": float64(0)},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "applied and dropped stash", result.Content)
}

func TestGitStashPop_Error(t *testing.T) {
	mock := &executorMockGitClient{
		stashPopErr: errors.New("conflict"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "spop-2",
		Name: "git_stash_pop",
	})
	assert.Contains(t, result.Error, "git stash pop")
}

// ---------------------------------------------------------------------------
// Tests: git_reset (not-yet-supported)
// ---------------------------------------------------------------------------

func TestGitReset_NotSupported(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "reset-1",
		Name:      "git_reset",
		Arguments: map[string]any{"ref": "HEAD~1"},
	})
	assert.Contains(t, result.Error, "not yet supported")
}

func TestGitReset_MissingRef(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "reset-2",
		Name: "git_reset",
	})
	assert.Contains(t, result.Error, "ref is required")
}

// ---------------------------------------------------------------------------
// Tests: git_discard (not-yet-supported)
// ---------------------------------------------------------------------------

func TestGitDiscard_NotSupported(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "discard-1",
		Name: "git_discard",
		Arguments: map[string]any{
			"paths": []any{"main.go"},
		},
	})
	assert.Contains(t, result.Error, "not yet supported")
}

func TestGitDiscard_MissingPaths(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "discard-2",
		Name: "git_discard",
	})
	assert.Contains(t, result.Error, "paths is required")
}

// ---------------------------------------------------------------------------
// Tests: bulk_stage
// ---------------------------------------------------------------------------

func TestBulkStage_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Create files matching the glob pattern.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# hi"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bs-1",
		Name: "bulk_stage",
		Arguments: map[string]any{
			"patterns": []any{"*.go"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "staged 2 file(s)")
	assert.Equal(t, 2, len(mock.stagePaths))
}

func TestBulkStage_NoMatch(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bs-2",
		Name: "bulk_stage",
		Arguments: map[string]any{
			"patterns": []any{"*.nonexistent"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "no files matched the patterns", result.Content)
}

func TestBulkStage_MissingPatterns(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bs-3",
		Name: "bulk_stage",
	})
	assert.Contains(t, result.Error, "patterns is required")
}

func TestBulkStage_StageError(t *testing.T) {
	mock := &executorMockGitClient{
		stageErr: errors.New("stage failed"),
	}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "f.go"), []byte("x"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bs-4",
		Name: "bulk_stage",
		Arguments: map[string]any{
			"patterns": []any{"*.go"},
		},
	})
	assert.Contains(t, result.Error, "git stage")
}

// ---------------------------------------------------------------------------
// Tests: bulk_rename (additional edge cases)
// ---------------------------------------------------------------------------

func TestBulkRename_InvalidEntry(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-inv1",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": []any{"not-a-map"},
		},
	})
	assert.Contains(t, result.Content, "renamed 0")
	assert.Contains(t, result.Content, "invalid rename entry")
}

func TestBulkRename_MissingOldNew(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-inv2",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": []any{
				map[string]any{"old": "a.txt"},
			},
		},
	})
	assert.Contains(t, result.Content, "missing old or new path")
}

func TestBulkRename_NotAnArray(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-inv3",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": "not-an-array",
		},
	})
	assert.Contains(t, result.Error, "renames must be an array")
}

func TestBulkRename_MissingRenames(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-inv4",
		Name: "bulk_rename",
	})
	assert.Contains(t, result.Error, "renames is required")
}

func TestBulkRename_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-esc",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": []any{
				map[string]any{
					"old": "../../etc/escape.txt",
					"new": "safe.txt",
				},
			},
		},
	})
	assert.Contains(t, result.Content, "renamed 0")
	assert.Contains(t, result.Content, "invalid path")
}

// ---------------------------------------------------------------------------
// Tests: file_list recursive mode
// ---------------------------------------------------------------------------

func TestFileList_Recursive(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Build a small directory tree.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("r"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "pkg", "util.go"), []byte("u"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "list-r",
		Name: "file_list",
		Arguments: map[string]any{
			"path":      ".",
			"recursive": true,
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "root.txt")
	assert.Contains(t, result.Content, "src/")
	assert.Contains(t, result.Content, "src/main.go")
	assert.Contains(t, result.Content, "src/pkg/util.go")
}

func TestFileList_EmptyDirectory(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	// Temp dir is empty initially.
	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "list-empty",
		Name: "file_list",
		Arguments: map[string]any{
			"path": ".",
		},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "(empty directory)", result.Content)
}

func TestFileList_InvalidPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "list-esc",
		Name:      "file_list",
		Arguments: map[string]any{"path": "../../etc"},
	})
	assert.Contains(t, result.Error, "invalid path")
}

func TestFileList_SkipsGitDir(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Create a .git dir (should be skipped).
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".git", "objects"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("x"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "list-git",
		Name: "file_list",
		Arguments: map[string]any{
			"path": ".",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "file.txt")
	assert.NotContains(t, result.Content, ".git")
}

// ---------------------------------------------------------------------------
// Tests: git_commit error and missing message
// ---------------------------------------------------------------------------

func TestGitCommit_MissingMessage(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "commit-miss",
		Name: "git_commit",
	})
	assert.Contains(t, result.Error, "message is required")
}

func TestGitCommit_Error(t *testing.T) {
	mock := &executorMockGitClient{
		commitErr: errors.New("nothing to commit"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "commit-err",
		Name:      "git_commit",
		Arguments: map[string]any{"message": "test"},
	})
	assert.Contains(t, result.Error, "git commit")
}

// ---------------------------------------------------------------------------
// Tests: git_push with custom remote and error
// ---------------------------------------------------------------------------

func TestGitPush_CustomRemote(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "push-custom",
		Name: "git_push",
		Arguments: map[string]any{
			"remote": "upstream",
			"force":  true,
		},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "pushed successfully", result.Content)
	assert.Equal(t, "upstream", mock.pushOpts.Remote)
	assert.True(t, mock.pushOpts.Force)
}

func TestGitPush_Error(t *testing.T) {
	mock := &executorMockGitClient{
		pushErr: errors.New("rejected"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "push-err",
		Name: "git_push",
	})
	assert.Contains(t, result.Error, "git push")
}

// ---------------------------------------------------------------------------
// Tests: git_checkout error and missing ref
// ---------------------------------------------------------------------------

func TestGitCheckout_MissingRef(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "co-miss",
		Name: "git_checkout",
	})
	assert.Contains(t, result.Error, "ref is required")
}

func TestGitCheckout_Error(t *testing.T) {
	mock := &executorMockGitClient{
		checkoutErr: errors.New("branch not found"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "co-err",
		Name:      "git_checkout",
		Arguments: map[string]any{"ref": "nonexistent"},
	})
	assert.Contains(t, result.Error, "git checkout")
}

// ---------------------------------------------------------------------------
// Tests: git_branch_create/delete errors
// ---------------------------------------------------------------------------

func TestGitBranchCreate_MissingName(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "brc-miss",
		Name: "git_branch_create",
	})
	assert.Contains(t, result.Error, "name is required")
}

func TestGitBranchCreate_Error(t *testing.T) {
	mock := &executorMockGitClient{
		branchCreateE: errors.New("already exists"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "brc-err",
		Name:      "git_branch_create",
		Arguments: map[string]any{"name": "main"},
	})
	assert.Contains(t, result.Error, "git branch create")
}

func TestGitBranchDelete_MissingName(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "brd-miss",
		Name: "git_branch_delete",
	})
	assert.Contains(t, result.Error, "name is required")
}

func TestGitBranchDelete_Error(t *testing.T) {
	mock := &executorMockGitClient{
		branchDeleteE: errors.New("cannot delete current"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "brd-err",
		Name:      "git_branch_delete",
		Arguments: map[string]any{"name": "main"},
	})
	assert.Contains(t, result.Error, "git branch delete")
}

// ---------------------------------------------------------------------------
// Tests: git_merge/rebase errors
// ---------------------------------------------------------------------------

func TestGitMerge_MissingBranch(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "merge-miss",
		Name: "git_merge",
	})
	assert.Contains(t, result.Error, "branch is required")
}

func TestGitMerge_Error(t *testing.T) {
	mock := &executorMockGitClient{
		mergeErr: errors.New("CONFLICT"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "merge-err",
		Name:      "git_merge",
		Arguments: map[string]any{"branch": "feature"},
	})
	assert.Contains(t, result.Error, "git merge")
}

func TestGitRebase_MissingOnto(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "rebase-miss",
		Name: "git_rebase",
	})
	assert.Contains(t, result.Error, "onto is required")
}

func TestGitRebase_Error(t *testing.T) {
	mock := &executorMockGitClient{
		rebaseErr: errors.New("CONFLICT"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "rebase-err",
		Name:      "git_rebase",
		Arguments: map[string]any{"onto": "main"},
	})
	assert.Contains(t, result.Error, "git rebase")
}

// ---------------------------------------------------------------------------
// Tests: git_status/diff/log errors
// ---------------------------------------------------------------------------

func TestGitStatus_Error(t *testing.T) {
	mock := &executorMockGitClient{
		statusErr: errors.New("not a repo"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "status-err",
		Name: "git_status",
	})
	assert.Contains(t, result.Error, "git status")
}

func TestGitDiff_Error(t *testing.T) {
	mock := &executorMockGitClient{
		diffErr: errors.New("diff failed"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "diff-err",
		Name: "git_diff",
	})
	assert.Contains(t, result.Error, "git diff")
}

func TestGitLog_Error(t *testing.T) {
	mock := &executorMockGitClient{
		logErr: errors.New("log failed"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "log-err",
		Name: "git_log",
	})
	assert.Contains(t, result.Error, "git log")
}

// ---------------------------------------------------------------------------
// Tests: git_stage error
// ---------------------------------------------------------------------------

func TestGitStage_Error(t *testing.T) {
	mock := &executorMockGitClient{
		stageErr: errors.New("stage failed"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "stage-err",
		Name: "git_stage",
		Arguments: map[string]any{
			"paths": []any{"file.go"},
		},
	})
	assert.Contains(t, result.Error, "git stage")
}

// ---------------------------------------------------------------------------
// Tests: file_write / file_delete / file_rename errors
// ---------------------------------------------------------------------------

func TestFileWrite_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fw-miss",
		Name: "file_write",
		Arguments: map[string]any{
			"content": "data",
		},
	})
	assert.Contains(t, result.Error, "path is required")
}

func TestFileWrite_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fw-esc",
		Name: "file_write",
		Arguments: map[string]any{
			"path":    "../../etc/evil",
			"content": "hack",
		},
	})
	assert.Contains(t, result.Error, "invalid path")
}

func TestFileDelete_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fd-miss",
		Name: "file_delete",
	})
	assert.Contains(t, result.Error, "path is required")
}

func TestFileDelete_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "fd-esc",
		Name:      "file_delete",
		Arguments: map[string]any{"path": "../../etc/escape.txt"},
	})
	assert.Contains(t, result.Error, "invalid path")
}

func TestFileRename_MissingPaths(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "fr-miss",
		Name: "file_rename",
	})
	assert.Contains(t, result.Error, "old_path and new_path are required")
}

func TestFileMkdir_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "mk-miss",
		Name: "file_mkdir",
	})
	assert.Contains(t, result.Error, "path is required")
}

func TestFileMkdir_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "mk-esc",
		Name:      "file_mkdir",
		Arguments: map[string]any{"path": "../../etc/evil"},
	})
	assert.Contains(t, result.Error, "invalid path")
}

// ---------------------------------------------------------------------------
// Tests: toJSON error case
// ---------------------------------------------------------------------------

func TestToJSON_MarshalError(t *testing.T) {
	// Use a channel, which cannot be marshalled to JSON.
	_, err := toJSON(make(chan int))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal result")
}

// ---------------------------------------------------------------------------
// Tests: truncate helper
// ---------------------------------------------------------------------------

func TestTruncate_Short(t *testing.T) {
	assert.Equal(t, "short", truncate("short", 100))
}

func TestTruncate_ExactLimit(t *testing.T) {
	s := "12345"
	assert.Equal(t, s, truncate(s, 5))
}

func TestTruncate_Truncated(t *testing.T) {
	result := truncate("hello world this is long", 10)
	// Should be <=10 chars and end with "…"
	assert.LessOrEqual(t, len(result), 13) // "…" is 3 bytes in UTF-8
	assert.Contains(t, result, "…")
}

// ---------------------------------------------------------------------------
// Tests: search_files missing pattern
// ---------------------------------------------------------------------------

func TestSearchFiles_MissingPattern(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sf-miss",
		Name: "search_files",
	})
	assert.Contains(t, result.Error, "pattern is required")
}

func TestSearchFiles_NoMatch(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "sf-none",
		Name:      "search_files",
		Arguments: map[string]any{"pattern": "*.xyz"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "no files matched", result.Content)
}

// ---------------------------------------------------------------------------
// Tests: search_content missing pattern
// ---------------------------------------------------------------------------

func TestSearchContent_MissingPattern(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sc-miss",
		Name: "search_content",
	})
	assert.Contains(t, result.Error, "pattern is required")
}

// ---------------------------------------------------------------------------
// Tests: navigate_to missing path
// ---------------------------------------------------------------------------

func TestNavigateTo_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "nav-miss",
		Name: "navigate_to",
	})
	assert.Contains(t, result.Error, "path is required")
}

// ---------------------------------------------------------------------------
// Tests: explain missing topic
// ---------------------------------------------------------------------------

func TestExplain_MissingTopic(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "exp-miss",
		Name: "explain",
	})
	assert.Contains(t, result.Error, "topic is required")
}

// ---------------------------------------------------------------------------
// Tests: bulk_delete
// ---------------------------------------------------------------------------

func TestBulkDelete_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	f1 := filepath.Join(tmpDir, "a.txt")
	f2 := filepath.Join(tmpDir, "b.txt")
	require.NoError(t, os.WriteFile(f1, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("b"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bd-1",
		Name: "bulk_delete",
		Arguments: map[string]any{
			"paths": []any{"a.txt", "b.txt"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "deleted 2 file(s)")

	_, err := os.Stat(f1)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestBulkDelete_MissingPaths(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bd-miss",
		Name: "bulk_delete",
	})
	assert.Contains(t, result.Error, "paths is required")
}

func TestBulkDelete_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bd-esc",
		Name: "bulk_delete",
		Arguments: map[string]any{
			"paths": []any{"../../etc/escape.txt"},
		},
	})
	assert.Contains(t, result.Content, "deleted 0 file(s)")
	assert.Contains(t, result.Content, "invalid path")
}

func TestBulkDelete_NonexistentFile(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bd-notfound",
		Name: "bulk_delete",
		Arguments: map[string]any{
			"paths": []any{"does_not_exist.txt"},
		},
	})
	assert.Contains(t, result.Content, "deleted 0 file(s)")
	assert.Contains(t, result.Content, "errors:")
}

// ---------------------------------------------------------------------------
// Tests: rateCategory edge cases
// ---------------------------------------------------------------------------

func TestRateCategory_UnknownTool(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	category := exec.rateCategory("totally_unknown_tool")
	assert.Equal(t, "read", category)
}

func TestRateCategory_DestructiveTool(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	// file_delete is marked as Destructive in the registry.
	category := exec.rateCategory("file_delete")
	assert.Equal(t, "write", category)
}

func TestRateCategory_SafeReadTool(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	category := exec.rateCategory("file_read")
	assert.Equal(t, "read", category)
}

// ---------------------------------------------------------------------------
// Tests: Write-rate-limited tools
// ---------------------------------------------------------------------------

func TestWriteRateLimit(t *testing.T) {
	mock := &executorMockGitClient{}
	tmpDir := t.TempDir()

	jail, err := mcp.NewPathJail(tmpDir, false)
	require.NoError(t, err)

	// 1000 read ops, but only 1 write op.
	limiter := mcp.NewRateLimiter(1000, 1)
	registry := NewToolRegistry()
	exec := NewToolExecutor(mock, jail, limiter, mcp.IsSensitivePath, registry)

	// First call uses the single write token.
	r1 := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "wrl-1",
		Name:      "git_reset",
		Arguments: map[string]any{"ref": "HEAD"},
	})
	// git_reset returns an error (not supported) but still consumes rate.
	assert.NotEmpty(t, r1.Error) // "not yet supported" error

	// Second write-classified call should be rate-limited.
	r2 := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "wrl-2",
		Name:      "git_reset",
		Arguments: map[string]any{"ref": "HEAD~1"},
	})
	assert.Contains(t, r2.Error, "rate limit exceeded")
}

// ---------------------------------------------------------------------------
// Tests: git_diff with staged flag
// ---------------------------------------------------------------------------

func TestGitDiff_Staged(t *testing.T) {
	mock := &executorMockGitClient{
		diffResult: []git.FileDiff{
			{Path: "main.go"},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "diff-staged",
		Name: "git_diff",
		Arguments: map[string]any{
			"staged": true,
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "main.go")
}

// ---------------------------------------------------------------------------
// Tests: search_content with path and no results
// ---------------------------------------------------------------------------

func TestSearchContent_WithPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src", "lib.go"), []byte("func Hello() {}"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sc-path",
		Name: "search_content",
		Arguments: map[string]any{
			"pattern": "Hello",
			"path":    "src",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "lib.go")
}

func TestSearchContent_NoResults(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "code.go"), []byte("package main"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sc-none",
		Name: "search_content",
		Arguments: map[string]any{
			"pattern": "ZZZZZZZ_NEVER_FOUND",
		},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "no matches found", result.Content)
}

// ---------------------------------------------------------------------------
// Tests: bulk_rename success
// ---------------------------------------------------------------------------

func TestBulkRename_Success(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "old1.txt"), []byte("1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "old2.txt"), []byte("2"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-ok",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": []any{
				map[string]any{"old": "old1.txt", "new": "new1.txt"},
				map[string]any{"old": "old2.txt", "new": "new2.txt"},
			},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "renamed 2 file(s)")

	_, err := os.Stat(filepath.Join(tmpDir, "new1.txt"))
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests: searchFiles with custom path
// ---------------------------------------------------------------------------

func TestSearchFiles_WithPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "sub", "match.go"), []byte("pkg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.go"), []byte("pkg"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "sf-path",
		Name: "search_files",
		Arguments: map[string]any{
			"pattern": "*.go",
			"path":    "sub",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "sub/match.go")
	assert.NotContains(t, result.Content, "root.go")
}

// ---------------------------------------------------------------------------
// Tests: ToolResult fields for errors
// ---------------------------------------------------------------------------

func TestExecute_ErrorResult_HasToolID(t *testing.T) {
	mock := &executorMockGitClient{
		statusErr: fmt.Errorf("mock error"),
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "id-check",
		Name: "git_status",
	})
	assert.Equal(t, "id-check", result.ToolID)
	assert.NotEmpty(t, result.Error)
	assert.Empty(t, result.Content)
}
