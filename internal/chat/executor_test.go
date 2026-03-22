package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test-specific mock that embeds the shared stubGitClient
// ---------------------------------------------------------------------------

type executorMockGitClient struct {
	stubGitClient
	statusResult  []git.FileStatus
	statusErr     error
	diffResult    []git.FileDiff
	diffErr       error
	logResult     []git.Commit
	logErr        error
	stageErr      error
	stagePaths    []string
	unstageErr    error
	commitHash    string
	commitErr     error
	commitMsg     string
	pushErr       error
	pushOpts      git.PushOpts
	pullErr       error
	fetchErr      error
	checkoutRef   string
	checkoutErr   error
	branchListRes []git.Branch
	branchListErr error
	branchCreateN string
	branchCreateB string
	branchCreateE error
	branchDeleteN string
	branchDeleteF bool
	branchDeleteE error
	mergeErr      error
	rebaseErr     error
	stashListRes  []git.StashEntry
	stashListErr  error
	stashPushErr  error
	stashPopErr   error
	tagCreateErr  error
	tagDeleteErr  error
	worktreeRes   []git.Worktree
	worktreeErr   error
	blameRes      []git.BlameLine
	blameErr      error
}

func (m *executorMockGitClient) Status(context.Context) ([]git.FileStatus, error) {
	return m.statusResult, m.statusErr
}

func (m *executorMockGitClient) Diff(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
	return m.diffResult, m.diffErr
}

func (m *executorMockGitClient) Log(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
	return m.logResult, m.logErr
}

func (m *executorMockGitClient) Stage(_ context.Context, paths []string) error {
	m.stagePaths = paths
	return m.stageErr
}

func (m *executorMockGitClient) Unstage(_ context.Context, _ []string) error {
	return m.unstageErr
}

func (m *executorMockGitClient) StageHunk(_ context.Context, _ string, _ git.Hunk) error {
	return nil
}

func (m *executorMockGitClient) UnstageHunk(_ context.Context, _ string, _ git.Hunk) error {
	return nil
}

func (m *executorMockGitClient) StageLine(_ context.Context, _ string, _ git.Hunk, _ int) error {
	return nil
}

func (m *executorMockGitClient) UnstageLine(_ context.Context, _ string, _ git.Hunk, _ int) error {
	return nil
}

func (m *executorMockGitClient) Commit(_ context.Context, msg string, _ git.CommitOpts) (string, error) {
	m.commitMsg = msg
	return m.commitHash, m.commitErr
}

func (m *executorMockGitClient) Push(_ context.Context, opts git.PushOpts) error {
	m.pushOpts = opts
	return m.pushErr
}

func (m *executorMockGitClient) Pull(_ context.Context, _ git.PullOpts) error {
	return m.pullErr
}

func (m *executorMockGitClient) Fetch(_ context.Context, _ git.FetchOpts) error {
	return m.fetchErr
}

func (m *executorMockGitClient) Checkout(_ context.Context, ref string) error {
	m.checkoutRef = ref
	return m.checkoutErr
}

func (m *executorMockGitClient) BranchList(context.Context) ([]git.Branch, error) {
	return m.branchListRes, m.branchListErr
}

func (m *executorMockGitClient) BranchCreate(_ context.Context, name, base string) error {
	m.branchCreateN = name
	m.branchCreateB = base
	return m.branchCreateE
}

func (m *executorMockGitClient) BranchDelete(_ context.Context, name string, force bool) error {
	m.branchDeleteN = name
	m.branchDeleteF = force
	return m.branchDeleteE
}

func (m *executorMockGitClient) Merge(_ context.Context, _ string, _ git.MergeOpts) error {
	return m.mergeErr
}

func (m *executorMockGitClient) Rebase(_ context.Context, _ string, _ git.RebaseOpts) error {
	return m.rebaseErr
}

func (m *executorMockGitClient) StashList(context.Context) ([]git.StashEntry, error) {
	return m.stashListRes, m.stashListErr
}

func (m *executorMockGitClient) StashPush(_ context.Context, _ git.StashOpts) error {
	return m.stashPushErr
}

func (m *executorMockGitClient) StashPop(_ context.Context, _ int) error {
	return m.stashPopErr
}

func (m *executorMockGitClient) TagCreate(_ context.Context, _, _, _ string) error {
	return m.tagCreateErr
}

func (m *executorMockGitClient) TagDelete(_ context.Context, _ string) error {
	return m.tagDeleteErr
}

func (m *executorMockGitClient) TagListRemote(_ context.Context, _ string) ([]git.Tag, error) {
	return nil, nil
}

func (m *executorMockGitClient) TagPush(_ context.Context, _, _ string) error {
	return nil
}

func (m *executorMockGitClient) TagPushAll(_ context.Context, _ string) error {
	return nil
}

func (m *executorMockGitClient) WorktreeList(context.Context) ([]git.Worktree, error) {
	return m.worktreeRes, m.worktreeErr
}

func (m *executorMockGitClient) Blame(_ context.Context, _ string) ([]git.BlameLine, error) {
	return m.blameRes, m.blameErr
}

func (m *executorMockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *executorMockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helper: create a ToolExecutor with a temp-dir PathJail
// ---------------------------------------------------------------------------

func newTestExecutor(t *testing.T, client git.GitClient) (*ToolExecutor, string) {
	t.Helper()
	tmpDir := t.TempDir()

	jail, err := mcp.NewPathJail(tmpDir, false)
	require.NoError(t, err)

	// Generous rate limits so tests don't hit them by default.
	limiter := mcp.NewRateLimiter(1000, 1000)
	registry := NewToolRegistry()

	return NewToolExecutor(client, jail, limiter, registry), tmpDir
}

// ---------------------------------------------------------------------------
// Tests: NewToolExecutor
// ---------------------------------------------------------------------------

func TestNewToolExecutor(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)
	require.NotNil(t, exec)
	assert.NotNil(t, exec.client)
	assert.NotNil(t, exec.jail)
	assert.NotNil(t, exec.limiter)
	assert.NotNil(t, exec.registry)
}

// ---------------------------------------------------------------------------
// Tests: Unknown tool
// ---------------------------------------------------------------------------

func TestExecute_UnknownTool(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "call-1",
		Name: "nonexistent_tool",
	})
	assert.Equal(t, "call-1", result.ToolID)
	assert.Contains(t, result.Error, "unknown tool")
	assert.Empty(t, result.Content)
}

// ---------------------------------------------------------------------------
// Tests: file_read with PathJail validation
// ---------------------------------------------------------------------------

func TestFileRead_ValidPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Create a test file.
	testFile := filepath.Join(tmpDir, "hello.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "read-1",
		Name:      "file_read",
		Arguments: map[string]any{"path": "hello.txt"},
	})
	assert.Equal(t, "read-1", result.ToolID)
	assert.Empty(t, result.Error)
	assert.Equal(t, "hello world", result.Content)
}

func TestFileRead_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "read-2",
		Name:      "file_read",
		Arguments: map[string]any{"path": "../../etc/passwd"},
	})
	assert.Equal(t, "read-2", result.ToolID)
	assert.Contains(t, result.Error, "invalid path")
	assert.Empty(t, result.Content)
}

func TestFileRead_MissingPath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "read-3",
		Name: "file_read",
	})
	assert.Contains(t, result.Error, "path is required")
}

func TestFileRead_NonexistentFile(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "read-4",
		Name:      "file_read",
		Arguments: map[string]any{"path": "does_not_exist.txt"},
	})
	assert.Contains(t, result.Error, "open file")
}

// ---------------------------------------------------------------------------
// Tests: file_write
// ---------------------------------------------------------------------------

func TestFileWrite_CreatesFile(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "write-1",
		Name: "file_write",
		Arguments: map[string]any{
			"path":    "output.txt",
			"content": "some data",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "written 9 bytes")

	data, err := os.ReadFile(filepath.Join(tmpDir, "output.txt"))
	require.NoError(t, err)
	assert.Equal(t, "some data", string(data))
}

func TestFileWrite_CreatesParentDirs(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "write-2",
		Name: "file_write",
		Arguments: map[string]any{
			"path":    "sub/dir/file.txt",
			"content": "nested",
		},
	})
	assert.Empty(t, result.Error)

	data, err := os.ReadFile(filepath.Join(tmpDir, "sub", "dir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

// ---------------------------------------------------------------------------
// Tests: file_delete
// ---------------------------------------------------------------------------

func TestFileDelete(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	testFile := filepath.Join(tmpDir, "delete_me.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("bye"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "del-1",
		Name:      "file_delete",
		Arguments: map[string]any{"path": "delete_me.txt"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "deleted")

	_, err := os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

// ---------------------------------------------------------------------------
// Tests: file_rename
// ---------------------------------------------------------------------------

func TestFileRename(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	oldFile := filepath.Join(tmpDir, "old.txt")
	require.NoError(t, os.WriteFile(oldFile, []byte("content"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "ren-1",
		Name: "file_rename",
		Arguments: map[string]any{
			"old_path": "old.txt",
			"new_path": "new.txt",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "renamed")

	_, err := os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	data, err := os.ReadFile(filepath.Join(tmpDir, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))
}

// ---------------------------------------------------------------------------
// Tests: file_list
// ---------------------------------------------------------------------------

func TestFileList(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "list-1",
		Name:      "file_list",
		Arguments: map[string]any{"path": "."},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "subdir/")
}

// ---------------------------------------------------------------------------
// Tests: file_mkdir
// ---------------------------------------------------------------------------

func TestFileMkdir(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "mkdir-1",
		Name:      "file_mkdir",
		Arguments: map[string]any{"path": "new/nested/dir"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "created")

	info, err := os.Stat(filepath.Join(tmpDir, "new", "nested", "dir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// ---------------------------------------------------------------------------
// Tests: git_status mapping to client method
// ---------------------------------------------------------------------------

func TestGitStatus_Clean(t *testing.T) {
	mock := &executorMockGitClient{
		statusResult: nil,
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "status-1",
		Name: "git_status",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "clean")
}

func TestGitStatus_WithChanges(t *testing.T) {
	mock := &executorMockGitClient{
		statusResult: []git.FileStatus{
			{Path: "main.go", StagedStatus: git.StatusModified},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "status-2",
		Name: "git_status",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "main.go")
}

// ---------------------------------------------------------------------------
// Tests: git_diff
// ---------------------------------------------------------------------------

func TestGitDiff_NoDifferences(t *testing.T) {
	mock := &executorMockGitClient{
		diffResult: nil,
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "diff-1",
		Name: "git_diff",
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "no differences", result.Content)
}

// ---------------------------------------------------------------------------
// Tests: git_log
// ---------------------------------------------------------------------------

func TestGitLog(t *testing.T) {
	mock := &executorMockGitClient{
		logResult: []git.Commit{
			{Hash: "abc123", Subject: "initial commit", Date: time.Now()},
		},
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "log-1",
		Name:      "git_log",
		Arguments: map[string]any{"count": float64(5)},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "abc123")
	assert.Contains(t, result.Content, "initial commit")
}

// ---------------------------------------------------------------------------
// Tests: git_stage
// ---------------------------------------------------------------------------

func TestGitStage(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "stage-1",
		Name: "git_stage",
		Arguments: map[string]any{
			"paths": []any{"file1.go", "file2.go"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "staged 2 file(s)")
	assert.Equal(t, []string{"file1.go", "file2.go"}, mock.stagePaths)
}

func TestGitStage_MissingPaths(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "stage-2",
		Name: "git_stage",
	})
	assert.Contains(t, result.Error, "paths is required")
}

// ---------------------------------------------------------------------------
// Tests: git_commit
// ---------------------------------------------------------------------------

func TestGitCommit(t *testing.T) {
	mock := &executorMockGitClient{
		commitHash: "def456",
	}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "commit-1",
		Name:      "git_commit",
		Arguments: map[string]any{"message": "fix bug"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "committed: def456", result.Content)
	assert.Equal(t, "fix bug", mock.commitMsg)
}

// ---------------------------------------------------------------------------
// Tests: git_push
// ---------------------------------------------------------------------------

func TestGitPush_DefaultRemote(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "push-1",
		Name: "git_push",
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "pushed successfully", result.Content)
	assert.Equal(t, "origin", mock.pushOpts.Remote)
	assert.False(t, mock.pushOpts.Force)
}

// ---------------------------------------------------------------------------
// Tests: git_checkout
// ---------------------------------------------------------------------------

func TestGitCheckout(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "co-1",
		Name:      "git_checkout",
		Arguments: map[string]any{"ref": "develop"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "checked out develop")
	assert.Equal(t, "develop", mock.checkoutRef)
}

// ---------------------------------------------------------------------------
// Tests: git_branch_create / git_branch_delete
// ---------------------------------------------------------------------------

func TestGitBranchCreate(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-1",
		Name: "git_branch_create",
		Arguments: map[string]any{
			"name":        "feature/x",
			"start_point": "main",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "created branch feature/x")
	assert.Equal(t, "feature/x", mock.branchCreateN)
	assert.Equal(t, "main", mock.branchCreateB)
}

func TestGitBranchDelete(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "br-2",
		Name: "git_branch_delete",
		Arguments: map[string]any{
			"name":  "old-branch",
			"force": true,
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "deleted branch old-branch")
	assert.Equal(t, "old-branch", mock.branchDeleteN)
	assert.True(t, mock.branchDeleteF)
}

// ---------------------------------------------------------------------------
// Tests: Rate limiting
// ---------------------------------------------------------------------------

func TestRateLimiting(t *testing.T) {
	mock := &executorMockGitClient{}
	tmpDir := t.TempDir()

	jail, err := mcp.NewPathJail(tmpDir, false)
	require.NoError(t, err)

	// Only allow 2 read operations per minute.
	limiter := mcp.NewRateLimiter(2, 2)
	registry := NewToolRegistry()
	exec := NewToolExecutor(mock, jail, limiter, registry)

	// Create a file to read.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("ok"), 0o644))

	call := ai.ToolCall{
		ID:        "rl-1",
		Name:      "file_read",
		Arguments: map[string]any{"path": "test.txt"},
	}

	// First two calls should succeed.
	r1 := exec.Execute(context.Background(), call)
	assert.Empty(t, r1.Error)
	assert.Equal(t, "ok", r1.Content)

	r2 := exec.Execute(context.Background(), call)
	assert.Empty(t, r2.Error)

	// Third call should be rate-limited.
	r3 := exec.Execute(context.Background(), call)
	assert.Contains(t, r3.Error, "rate limit exceeded")
	assert.Empty(t, r3.Content)
}

// ---------------------------------------------------------------------------
// Tests: Argument extraction helpers
// ---------------------------------------------------------------------------

func TestGetString(t *testing.T) {
	args := map[string]any{
		"name":   "hello",
		"number": 42,
		"empty":  "",
	}
	assert.Equal(t, "hello", getString(args, "name"))
	assert.Equal(t, "42", getString(args, "number")) // numeric fallback
	assert.Equal(t, "", getString(args, "empty"))
	assert.Equal(t, "", getString(args, "missing"))
}

func TestGetBool(t *testing.T) {
	args := map[string]any{
		"flag":   true,
		"off":    false,
		"string": "true", // wrong type
	}
	assert.True(t, getBool(args, "flag"))
	assert.False(t, getBool(args, "off"))
	assert.False(t, getBool(args, "string"))  // not a bool
	assert.False(t, getBool(args, "missing")) // absent
}

func TestGetInt(t *testing.T) {
	args := map[string]any{
		"float":  float64(42),
		"int":    int(7),
		"int64":  int64(99),
		"string": "10",
	}
	assert.Equal(t, 42, getInt(args, "float"))
	assert.Equal(t, 7, getInt(args, "int"))
	assert.Equal(t, 99, getInt(args, "int64"))
	assert.Equal(t, 0, getInt(args, "string"))  // wrong type
	assert.Equal(t, 0, getInt(args, "missing")) // absent
}

func TestGetStringSlice(t *testing.T) {
	args := map[string]any{
		"native":    []string{"a", "b"},
		"interface": []any{"x", "y", "z"},
		"mixed":     []any{"ok", 42}, // non-string elements skipped
		"wrong":     "not-a-slice",
	}
	assert.Equal(t, []string{"a", "b"}, getStringSlice(args, "native"))
	assert.Equal(t, []string{"x", "y", "z"}, getStringSlice(args, "interface"))
	assert.Equal(t, []string{"ok"}, getStringSlice(args, "mixed"))
	assert.Nil(t, getStringSlice(args, "wrong"))
	assert.Nil(t, getStringSlice(args, "missing"))
}

// ---------------------------------------------------------------------------
// Tests: navigate_to
// ---------------------------------------------------------------------------

func TestNavigateTo(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Create a target directory.
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "src"), 0o755))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "nav-1",
		Name:      "navigate_to",
		Arguments: map[string]any{"path": "src"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "navigate:src", result.Content)
}

func TestNavigateTo_EscapeAttempt(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "nav-2",
		Name:      "navigate_to",
		Arguments: map[string]any{"path": "../../../etc"},
	})
	assert.Contains(t, result.Error, "invalid path")
}

// ---------------------------------------------------------------------------
// Tests: search_files
// ---------------------------------------------------------------------------

func TestSearchFiles(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# hi"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "sf-1",
		Name:      "search_files",
		Arguments: map[string]any{"pattern": "*.go"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "main.go")
	assert.NotContains(t, result.Content, "readme.md")
}

// ---------------------------------------------------------------------------
// Tests: search_content
// ---------------------------------------------------------------------------

func TestSearchContent(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "code.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "sc-1",
		Name:      "search_content",
		Arguments: map[string]any{"pattern": "Println"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "code.go")
	assert.Contains(t, result.Content, "Println")
}

func TestSearchContent_InvalidRegex(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "sc-2",
		Name:      "search_content",
		Arguments: map[string]any{"pattern": "[invalid"},
	})
	assert.Contains(t, result.Error, "invalid regex")
}

// ---------------------------------------------------------------------------
// Tests: explain (pass-through)
// ---------------------------------------------------------------------------

func TestExplain(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "exp-1",
		Name:      "explain",
		Arguments: map[string]any{"topic": "rebasing"},
	})
	assert.Empty(t, result.Error)
	assert.Equal(t, "explain: rebasing", result.Content)
}

// ---------------------------------------------------------------------------
// Tests: git_merge / git_rebase
// ---------------------------------------------------------------------------

func TestGitMerge(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "merge-1",
		Name:      "git_merge",
		Arguments: map[string]any{"branch": "feature"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "merged feature")
}

func TestGitRebase(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "rebase-1",
		Name:      "git_rebase",
		Arguments: map[string]any{"onto": "main"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "rebased onto main")
}

// ---------------------------------------------------------------------------
// Tests: git_stash_push / git_stash_pop
// ---------------------------------------------------------------------------

func TestGitStashPush(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "stash-1",
		Name:      "git_stash_push",
		Arguments: map[string]any{"message": "wip"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "stashed")
}

func TestGitStashPop(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "stash-2",
		Name: "git_stash_pop",
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "applied and dropped stash")
}

// ---------------------------------------------------------------------------
// Tests: git_tag_create / git_tag_delete
// ---------------------------------------------------------------------------

func TestGitTagCreate(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "tag-1",
		Name: "git_tag_create",
		Arguments: map[string]any{
			"name":    "v1.0.0",
			"message": "release 1.0",
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "created tag v1.0.0")
}

func TestGitTagDelete(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "tag-2",
		Name:      "git_tag_delete",
		Arguments: map[string]any{"name": "v0.9.0"},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "deleted tag v0.9.0")
}

// ---------------------------------------------------------------------------
// Tests: git_reset / git_discard (unsupported)
// ---------------------------------------------------------------------------

func TestGitReset_Unsupported(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "reset-1",
		Name:      "git_reset",
		Arguments: map[string]any{"ref": "HEAD~1"},
	})
	assert.Contains(t, result.Error, "not yet supported")
}

func TestGitDiscard_Unsupported(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "discard-1",
		Name: "git_discard",
		Arguments: map[string]any{
			"paths": []any{"file.txt"},
		},
	})
	assert.Contains(t, result.Error, "not yet supported")
}

// ---------------------------------------------------------------------------
// Tests: bulk_delete
// ---------------------------------------------------------------------------

func TestBulkDelete(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("b"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "bd-1",
		Name: "bulk_delete",
		Arguments: map[string]any{
			"paths": []any{"a.txt", "b.txt"},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "deleted 2 file(s)")
}

// ---------------------------------------------------------------------------
// Tests: bulk_rename
// ---------------------------------------------------------------------------

func TestBulkRename(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "x.txt"), []byte("x"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "brn-1",
		Name: "bulk_rename",
		Arguments: map[string]any{
			"renames": []any{
				map[string]any{"old": "x.txt", "new": "y.txt"},
			},
		},
	})
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Content, "renamed 1 file(s)")

	_, err := os.Stat(filepath.Join(tmpDir, "y.txt"))
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests: ToolResult fields
// ---------------------------------------------------------------------------

func TestToolResult_IDPassthrough(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	// Successful execution carries the ToolCall.ID through.
	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "my-unique-id",
		Name:      "explain",
		Arguments: map[string]any{"topic": "git"},
	})
	assert.Equal(t, "my-unique-id", result.ToolID)

	// Error execution also carries the ID.
	errResult := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "err-id",
		Name: "nonexistent",
	})
	assert.Equal(t, "err-id", errResult.ToolID)
}

// ---------------------------------------------------------------------------
// Tests: truncate helper
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", truncate("short", 10))
	assert.Equal(t, "hellowor…", truncate("helloworld", 9))
	assert.Equal(t, "", truncate("", 5))
}

// ---------------------------------------------------------------------------
// Tests: file_read sensitive path blocking (CR-008)
// ---------------------------------------------------------------------------

func TestFileRead_BlocksSensitivePath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	sensitiveFiles := []string{
		".env",
		".env.production",
		"id_rsa",
		"server.pem",
		".git/config",
	}
	for _, path := range sensitiveFiles {
		t.Run(path, func(t *testing.T) {
			result := exec.Execute(context.Background(), ai.ToolCall{
				ID:        "read-sensitive",
				Name:      "file_read",
				Arguments: map[string]any{"path": path},
			})
			assert.Contains(t, result.Error, "blocked",
				"file_read should block sensitive path %q", path)
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: file_write sensitive path blocking
// ---------------------------------------------------------------------------

func TestFileWrite_BlocksSensitivePath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	sensitiveFiles := []string{
		".env",
		".env.local",
		"id_rsa",
		"server.key",
	}
	for _, path := range sensitiveFiles {
		t.Run(path, func(t *testing.T) {
			result := exec.Execute(context.Background(), ai.ToolCall{
				ID:   "write-sensitive",
				Name: "file_write",
				Arguments: map[string]any{
					"path":    path,
					"content": "malicious",
				},
			})
			assert.Contains(t, result.Error, "blocked",
				"file_write should block sensitive path %q", path)
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: file_write size limit enforcement (CR-003)
// ---------------------------------------------------------------------------

func TestFileWrite_SizeLimitEnforced(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	// 10 MiB + 1 byte should be rejected.
	hugeContent := strings.Repeat("x", 10*1024*1024+1)
	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "write-huge",
		Name: "file_write",
		Arguments: map[string]any{
			"path":    "big.txt",
			"content": hugeContent,
		},
	})
	assert.Contains(t, result.Error, "content too large")
}

func TestFileWrite_PathRequired(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "write-nopath",
		Name: "file_write",
		Arguments: map[string]any{
			"content": "some data",
		},
	})
	assert.Contains(t, result.Error, "path is required")
}

// ---------------------------------------------------------------------------
// Tests: file_read directory check (TOCTOU safe open-stat-read)
// ---------------------------------------------------------------------------

func TestFileRead_RejectsDirectory(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "read-dir",
		Name:      "file_read",
		Arguments: map[string]any{"path": "subdir"},
	})
	assert.Contains(t, result.Error, "directory")
}

// ---------------------------------------------------------------------------
// Tests: file_delete sensitive path blocking
// ---------------------------------------------------------------------------

func TestFileDelete_BlocksSensitivePath(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "del-sensitive",
		Name:      "file_delete",
		Arguments: map[string]any{"path": ".env"},
	})
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "sensitive")
}

func TestFileDelete_PathRequired(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "del-nopath",
		Name: "file_delete",
	})
	assert.Contains(t, result.Error, "path is required")
}

// ---------------------------------------------------------------------------
// Tests: file_rename sensitive path blocking
// ---------------------------------------------------------------------------

func TestFileRename_BlocksSensitiveSource(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "ren-sensitive-src",
		Name: "file_rename",
		Arguments: map[string]any{
			"old_path": ".env",
			"new_path": "safe.txt",
		},
	})
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "sensitive")
}

func TestFileRename_BlocksSensitiveTarget(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, tmpDir := newTestExecutor(t, mock)

	// Create a source file.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "normal.txt"), []byte("data"), 0o644))

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "ren-sensitive-dst",
		Name: "file_rename",
		Arguments: map[string]any{
			"old_path": "normal.txt",
			"new_path": "id_rsa",
		},
	})
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "sensitive")
}

func TestFileRename_BothPathsRequired(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "ren-nopath",
		Name: "file_rename",
		Arguments: map[string]any{
			"old_path": "a.txt",
		},
	})
	assert.Contains(t, result.Error, "old_path and new_path are required")
}

// ---------------------------------------------------------------------------
// Tests: rate limiting
// ---------------------------------------------------------------------------

func TestExecute_RateLimited(t *testing.T) {
	mock := &executorMockGitClient{}
	tmpDir := t.TempDir()
	jail, err := mcp.NewPathJail(tmpDir, false)
	require.NoError(t, err)

	// Create a very restrictive rate limiter: 1 read, 1 write.
	limiter := mcp.NewRateLimiter(1, 1)
	registry := NewToolRegistry()
	exec := NewToolExecutor(mock, jail, limiter, registry)

	// First read should succeed.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("ok"), 0o644))
	r1 := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "rl-1",
		Name:      "file_read",
		Arguments: map[string]any{"path": "test.txt"},
	})
	assert.Empty(t, r1.Error)

	// Second read should be rate-limited.
	r2 := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "rl-2",
		Name:      "file_read",
		Arguments: map[string]any{"path": "test.txt"},
	})
	assert.Contains(t, r2.Error, "rate limit exceeded")
}

// ---------------------------------------------------------------------------
// Tests: helper functions coverage
// ---------------------------------------------------------------------------

func TestGetString_TypeConversion(t *testing.T) {
	// Non-string values should be fmt.Sprintf'd.
	assert.Equal(t, "42", getString(map[string]any{"k": 42}, "k"))
	assert.Equal(t, "true", getString(map[string]any{"k": true}, "k"))
	// Missing key returns empty string.
	assert.Equal(t, "", getString(map[string]any{}, "missing"))
}

func TestGetBool_TypeConversion(t *testing.T) {
	assert.True(t, getBool(map[string]any{"k": true}, "k"))
	assert.False(t, getBool(map[string]any{"k": false}, "k"))
	// Non-bool type returns false.
	assert.False(t, getBool(map[string]any{"k": "true"}, "k"))
	// Missing key returns false.
	assert.False(t, getBool(map[string]any{}, "missing"))
}

func TestGetInt_TypeConversion(t *testing.T) {
	// float64 (JSON default).
	assert.Equal(t, 42, getInt(map[string]any{"k": float64(42)}, "k"))
	// int.
	assert.Equal(t, 7, getInt(map[string]any{"k": 7}, "k"))
	// int64.
	assert.Equal(t, 99, getInt(map[string]any{"k": int64(99)}, "k"))
	// String type returns 0.
	assert.Equal(t, 0, getInt(map[string]any{"k": "not a number"}, "k"))
	// Missing key returns 0.
	assert.Equal(t, 0, getInt(map[string]any{}, "missing"))
}

func TestGetStringSlice_TypeConversion(t *testing.T) {
	// Native []string.
	assert.Equal(t, []string{"a", "b"}, getStringSlice(map[string]any{"k": []string{"a", "b"}}, "k"))
	// []any (from JSON).
	assert.Equal(t, []string{"x", "y"}, getStringSlice(map[string]any{"k": []any{"x", "y"}}, "k"))
	// Missing key returns nil.
	assert.Nil(t, getStringSlice(map[string]any{}, "missing"))
	// Wrong type returns nil.
	assert.Nil(t, getStringSlice(map[string]any{"k": "not-a-slice"}, "k"))
}
