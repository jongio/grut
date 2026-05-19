package ai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

// mockGitClient is a test double implementing git.GitClient.
// Each method field can be set to control the mock's behaviour; unset
// methods return zero values.
type mockGitClient struct {
	StatusFunc     func(ctx context.Context) ([]git.FileStatus, error)
	DiffFunc       func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	LogFunc        func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	BlameFunc      func(ctx context.Context, path string) ([]git.BlameLine, error)
	RepoRootFunc   func(ctx context.Context) (string, error)
	BranchListFunc func(ctx context.Context) ([]git.Branch, error)
}

// Compile-time check.
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
func (m *mockGitClient) Stage(_ context.Context, _ []string) error { return nil }

func (m *mockGitClient) Unstage(_ context.Context, _ []string) error { return nil }

func (m *mockGitClient) StageHunk(_ context.Context, _ string, _ git.Hunk) error { return nil }

func (m *mockGitClient) UnstageHunk(_ context.Context, _ string, _ git.Hunk) error { return nil }

func (m *mockGitClient) StageLine(_ context.Context, _ string, _ git.Hunk, _ int) error { return nil }

func (m *mockGitClient) UnstageLine(_ context.Context, _ string, _ git.Hunk, _ int) error { return nil }

func (m *mockGitClient) Commit(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
	return "abc123", nil
}

// BranchManager stubs.
func (m *mockGitClient) BranchList(ctx context.Context) ([]git.Branch, error) {
	if m.BranchListFunc != nil {
		return m.BranchListFunc(ctx)
	}
	return nil, nil
}
func (m *mockGitClient) CurrentBranch(_ context.Context) (git.Branch, error) {
	return git.Branch{IsCurrent: true}, nil
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

// RemoteListOps stubs.
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

// newMock returns a mock configured with a current branch and repo root.
func newMock(branch, repoRoot string) *mockGitClient {
	return &mockGitClient{
		BranchListFunc: func(_ context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: branch, IsCurrent: true}}, nil
		},
		RepoRootFunc: func(_ context.Context) (string, error) {
			return repoRoot, nil
		},
	}
}

// sampleDiff returns a minimal FileDiff for testing.
func sampleDiff(path, content string) git.FileDiff {
	return git.FileDiff{
		Path: path,
		Hunks: []git.Hunk{{
			Header:   "@@ -1,3 +1,4 @@",
			OldStart: 1, OldLines: 3,
			NewStart: 1, NewLines: 4,
			Lines: []git.DiffLine{
				{Type: git.DiffLineContext, Content: "package main"},
				{Type: git.DiffLineAdded, Content: content},
			},
		}},
	}
}

// sampleCommit returns a minimal Commit for testing.
func sampleCommit(hash, subject string) git.Commit {
	return git.Commit{Hash: hash, Author: "dev", Subject: subject}
}

// writeTestFile creates a file under dir for testing readFileContent.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, estimateTokens(""))
	assert.Equal(t, 1, estimateTokens("abcd"))
	assert.Equal(t, 2, estimateTokens("12345678"))
	assert.Equal(t, 0, estimateTokens("abc")) // 3/4 rounds down
}

func TestParseConflictMarkers(t *testing.T) {
	content := strings.Join([]string{
		"normal line",
		"<<<<<<< HEAD",
		"our change",
		"=======",
		"their change",
		">>>>>>> feature",
		"after conflict",
	}, "\n")

	regions := parseConflictMarkers(content)
	require.Len(t, regions, 1)
	assert.Equal(t, 2, regions[0].StartLine)
	assert.Equal(t, 6, regions[0].EndLine)
	assert.Equal(t, "our change\n", regions[0].Ours)
	assert.Equal(t, "their change\n", regions[0].Theirs)
}

func TestParseConflictMarkers_Multiple(t *testing.T) {
	content := strings.Join([]string{
		"<<<<<<< HEAD",
		"ours1",
		"=======",
		"theirs1",
		">>>>>>> br",
		"gap",
		"<<<<<<< HEAD",
		"ours2",
		"=======",
		"theirs2",
		">>>>>>> br",
	}, "\n")

	regions := parseConflictMarkers(content)
	require.Len(t, regions, 2)
	assert.Equal(t, "ours1\n", regions[0].Ours)
	assert.Equal(t, "ours2\n", regions[1].Ours)
}

func TestParseConflictMarkers_NoMarkers(t *testing.T) {
	regions := parseConflictMarkers("just normal content\nno conflicts here")
	assert.Empty(t, regions)
}

func TestForCommit_Basic(t *testing.T) {
	mock := newMock("feature/auth", "/repo")
	mock.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{
			{Path: "main.go", StagedStatus: git.StatusModified},
		}, nil
	}
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{sampleDiff("main.go", "// new line")}, nil
		}
		return nil, nil
	}
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{
			sampleCommit("aaa111", "feat: add login"),
			sampleCommit("bbb222", "fix: typo"),
		}, nil
	}

	b := NewBuilder(mock, nil, 0) // unlimited budget
	gc, err := b.ForCommit(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "feature/auth", gc.CurrentBranch)
	assert.Equal(t, "/repo", gc.RepoRoot)

	// Staged diff included.
	require.Len(t, gc.Diffs, 1)
	assert.Equal(t, "main.go", gc.Diffs[0].Path)

	// Recent log included.
	require.Len(t, gc.Log, 2)
	assert.Equal(t, "feat: add login", gc.Log[0].Subject)

	// Status included.
	require.Len(t, gc.Status, 1)
}

func TestForChat_Lightweight(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{
			{Path: "a.go", WorktreeStatus: git.StatusModified},
			{Path: "b.go", WorktreeStatus: git.StatusUntracked},
		}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForChat(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "main", gc.CurrentBranch)
	assert.Equal(t, "/repo", gc.RepoRoot)

	// Status present.
	require.Len(t, gc.Status, 2)

	// No diffs, log, file contents, or conflicts.
	assert.Nil(t, gc.Diffs)
	assert.Nil(t, gc.Log)
	assert.Nil(t, gc.FileContents)
	assert.Nil(t, gc.Conflicts)
}

func TestForReview_IncludesFileContents(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "app.go", "package main\nfunc main() {}\n")

	mock := newMock("dev", root)
	mock.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{sampleDiff("app.go", "// added")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForReview(context.Background(), git.DiffOpts{})
	require.NoError(t, err)

	require.Len(t, gc.Diffs, 1)
	assert.Contains(t, gc.FileContents, "app.go")
	assert.Equal(t, "package main\nfunc main() {}\n", gc.FileContents["app.go"])
}

func TestRedaction_FileExcluded(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "app.go", "package main\n")
	writeTestFile(t, root, "creds.secret", "password=hunter2\n")

	mock := newMock("main", root)
	mock.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{
			sampleDiff("app.go", "// ok"),
			sampleDiff("creds.secret", "password=hunter2"),
		}, nil
	}

	redactor := NewRedactor([]string{"*.secret"})
	b := NewBuilder(mock, redactor, 0)
	gc, err := b.ForReview(context.Background(), git.DiffOpts{})
	require.NoError(t, err)

	// Excluded file should NOT appear in diffs.
	require.Len(t, gc.Diffs, 1)
	assert.Equal(t, "app.go", gc.Diffs[0].Path)

	// Excluded file should NOT appear in file contents.
	assert.NotContains(t, gc.FileContents, "creds.secret")
	assert.Contains(t, gc.FileContents, "app.go")
}

func TestRedaction_NilRedactor(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "creds.secret", "password=hunter2\n")

	mock := newMock("main", root)
	mock.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{sampleDiff("creds.secret", "secret")}, nil
	}

	// A nil redactor triggers fail-closed behavior: a default redactor with
	// built-in secret patterns is created, so sensitive files are excluded
	// rather than leaked to the AI provider (CWE-200).
	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForReview(context.Background(), git.DiffOpts{})
	require.NoError(t, err)

	// creds.secret matches *.secret builtin pattern → excluded.
	assert.Empty(t, gc.Diffs, "sensitive file should be filtered by default redactor")
	assert.NotContains(t, gc.FileContents, "creds.secret")
}

func TestTokenBudget_TrimsCommits(t *testing.T) {
	// Create a diff that consumes a known amount of budget, then verify
	// that commits are trimmed when the budget runs low.
	//
	// Diff content: path "x.go" (4 chars) + header "@@" (2 chars) +
	// one line "a" (2 chars with newline) ≈ 8 chars total → 2 tokens.
	// Commit: hash(7) + author(3) + subject(10) + body(0) + overhead(20) = 40 chars → 10 tokens.
	//
	// With maxTokens = 25, the diff (2 tokens) fits. Remaining = 23.
	// Two commits (2×10 = 20 tokens) fit. Third (30 > 23) does not.

	mock := newMock("main", "/repo")
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{{
				Path: "x.go",
				Hunks: []git.Hunk{{
					Header: "@@",
					Lines:  []git.DiffLine{{Content: "a"}},
				}},
			}}, nil
		}
		return nil, nil
	}
	mock.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{
			sampleCommit("aaa1111", "commit one"),
			sampleCommit("bbb2222", "commit two"),
			sampleCommit("ccc3333", "commit thr"),
		}, nil
	}

	b := NewBuilder(mock, nil, 25)
	gc, err := b.ForCommit(context.Background())
	require.NoError(t, err)

	// Diff should be present (fits in budget).
	require.Len(t, gc.Diffs, 1)

	// Only some commits should fit; not all three.
	assert.Less(t, len(gc.Log), 3, "expected fewer than 3 commits due to budget")
	assert.Greater(t, len(gc.Log), 0, "expected at least 1 commit")
}

func TestTokenBudget_Unlimited(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{sampleDiff("big.go", strings.Repeat("x", 10000))}, nil
		}
		return nil, nil
	}
	mock.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		commits := make([]git.Commit, 50)
		for i := range commits {
			commits[i] = sampleCommit("hash", "subject")
		}
		return commits, nil
	}

	// maxTokens = 0 means unlimited.
	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForCommit(context.Background())
	require.NoError(t, err)

	require.Len(t, gc.Diffs, 1)
	assert.Len(t, gc.Log, 50) // all 50 commits included
}

func TestTokenBudget_DiffExceedsBudget(t *testing.T) {
	// When the diff alone exceeds the entire budget, it should be excluded
	// but the method should still return branch and status.
	mock := newMock("main", "/repo")
	mock.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{{Path: "a.go"}}, nil
	}
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{sampleDiff("huge.go", strings.Repeat("x", 10000))}, nil
		}
		return nil, nil
	}

	b := NewBuilder(mock, nil, 5) // tiny budget
	gc, err := b.ForCommit(context.Background())
	require.NoError(t, err)

	// Metadata still populated.
	assert.Equal(t, "main", gc.CurrentBranch)
	require.Len(t, gc.Status, 1)

	// Diff excluded because it exceeds budget.
	assert.Nil(t, gc.Diffs)
}

func TestForConflict_ParsesMarkers(t *testing.T) {
	root := t.TempDir()
	conflictContent := strings.Join([]string{
		"line 1",
		"<<<<<<< HEAD",
		"our version",
		"=======",
		"their version",
		">>>>>>> feature",
		"line 7",
	}, "\n")
	writeTestFile(t, root, "conflict.go", conflictContent)

	mock := newMock("main", root)

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForConflict(context.Background(), []string{"conflict.go"})
	require.NoError(t, err)

	require.Len(t, gc.Conflicts, 1)
	assert.Equal(t, "conflict.go", gc.Conflicts[0].Path)
	require.Len(t, gc.Conflicts[0].ConflictMarkers, 1)
	assert.Equal(t, "our version\n", gc.Conflicts[0].ConflictMarkers[0].Ours)
	assert.Equal(t, "their version\n", gc.Conflicts[0].ConflictMarkers[0].Theirs)

	// Full file content also available.
	assert.Contains(t, gc.FileContents, "conflict.go")
}

func TestForConflict_ExcludesRedacted(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "ok.go", "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> br\n")
	writeTestFile(t, root, "secret.key", "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> br\n")

	mock := newMock("main", root)
	redactor := NewRedactor([]string{patternKeyFile})

	b := NewBuilder(mock, redactor, 0)
	gc, err := b.ForConflict(context.Background(), []string{"ok.go", "secret.key"})
	require.NoError(t, err)

	// Only non-excluded file should be present.
	require.Len(t, gc.Conflicts, 1)
	assert.Equal(t, "ok.go", gc.Conflicts[0].Path)
	assert.NotContains(t, gc.FileContents, "secret.key")
}

func TestForPR_SetsBranches(t *testing.T) {
	mock := newMock("feature/x", "/repo")
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		assert.Equal(t, "main", opts.CommitA)
		assert.Equal(t, "HEAD", opts.CommitB)
		return []git.FileDiff{sampleDiff("pr.go", "change")}, nil
	}
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, "main..HEAD", opts.Ref)
		return []git.Commit{sampleCommit("abc", "feat: pr")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForPR(context.Background(), "main")
	require.NoError(t, err)

	assert.Equal(t, "feature/x", gc.CurrentBranch)
	assert.Equal(t, "main", gc.TargetBranch)
	require.Len(t, gc.Diffs, 1)
	require.Len(t, gc.Log, 1)
}

func TestForRebase_SetsBranches(t *testing.T) {
	mock := newMock("feature/y", "/repo")
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, "main..HEAD", opts.Ref)
		return []git.Commit{sampleCommit("r1", "rebase commit")}, nil
	}
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		assert.Equal(t, "main", opts.CommitA)
		return []git.FileDiff{sampleDiff("r.go", "rebased")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForRebase(context.Background(), "main")
	require.NoError(t, err)

	assert.Equal(t, "feature/y", gc.CurrentBranch)
	assert.Equal(t, "main", gc.TargetBranch)
	require.Len(t, gc.Log, 1)
	require.Len(t, gc.Diffs, 1)
}

func TestForBisect_DiffsBetweenRefs(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		assert.Equal(t, "goodhash", opts.CommitA)
		assert.Equal(t, "badhash", opts.CommitB)
		return []git.FileDiff{sampleDiff("bug.go", "bugfix")}, nil
	}
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, "goodhash..badhash", opts.Ref)
		return []git.Commit{sampleCommit("mid", "suspect")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForBisect(context.Background(), "goodhash", "badhash")
	require.NoError(t, err)

	require.Len(t, gc.Diffs, 1)
	require.Len(t, gc.Log, 1)
}

func TestForChangelog_CommitsInRange(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, "v1.0..v2.0", opts.Ref)
		return []git.Commit{
			sampleCommit("a1", "feat: new"),
			sampleCommit("a2", "fix: old"),
		}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForChangelog(context.Background(), "v1.0", "v2.0")
	require.NoError(t, err)

	require.Len(t, gc.Log, 2)
}

func TestForSplit_CommitDiff(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		assert.Equal(t, "abc123^", opts.CommitA)
		assert.Equal(t, "abc123", opts.CommitB)
		return []git.FileDiff{
			sampleDiff("a.go", "change A"),
			sampleDiff("b.go", "change B"),
		}, nil
	}
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, "abc123", opts.Ref)
		assert.Equal(t, 1, opts.MaxCount)
		return []git.Commit{sampleCommit("abc123", "big commit")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForSplit(context.Background(), "abc123")
	require.NoError(t, err)

	require.Len(t, gc.Diffs, 2)
	require.Len(t, gc.Log, 1)
}

func TestGracefulDegradation_DiffError(t *testing.T) {
	// When Diff fails, ForCommit should still return branch and status.
	mock := newMock("main", "/repo")
	mock.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{{Path: "ok.go"}}, nil
	}
	mock.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return nil, assert.AnError
	}
	mock.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{sampleCommit("a", "s")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForCommit(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "main", gc.CurrentBranch)
	require.Len(t, gc.Status, 1)
	assert.Nil(t, gc.Diffs) // diff failed gracefully
	require.Len(t, gc.Log, 1)
}

func TestGracefulDegradation_BranchListError(t *testing.T) {
	mock := &mockGitClient{
		BranchListFunc: func(_ context.Context) ([]git.Branch, error) {
			return nil, assert.AnError
		},
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForChat(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "", gc.CurrentBranch)
}

func TestTokenBudget_Helpers(t *testing.T) {
	tb := newTokenBudget(100)
	assert.False(t, tb.unlimited())
	assert.Equal(t, 100, tb.remaining())
	assert.True(t, tb.canFit(50))
	assert.True(t, tb.canFit(100))
	assert.False(t, tb.canFit(101))

	tb.consume(60)
	assert.Equal(t, 40, tb.remaining())
	assert.True(t, tb.canFit(40))
	assert.False(t, tb.canFit(41))

	// Unlimited budget.
	tbUnlimited := newTokenBudget(0)
	assert.True(t, tbUnlimited.unlimited())
	assert.True(t, tbUnlimited.canFit(999999))
}

func TestTrimCommitsToBudget(t *testing.T) {
	commits := []git.Commit{
		sampleCommit("aaa1111", "commit one"),
		sampleCommit("bbb2222", "commit two"),
		sampleCommit("ccc3333", "commit thr"),
	}

	t.Run("unlimited", func(t *testing.T) {
		budget := newTokenBudget(0)
		result := trimCommitsToBudget(commits, budget)
		assert.Len(t, result, 3)
	})

	t.Run("tight_budget", func(t *testing.T) {
		// Each commit: (7+3+10+0+20)/4 = 10 tokens.
		// Budget of 15 fits 1 commit (10 tokens), not 2 (20 tokens).
		budget := newTokenBudget(15)
		result := trimCommitsToBudget(commits, budget)
		require.Len(t, result, 1)
		assert.Equal(t, "commit one", result[0].Subject)
	})
}

func TestFilterDiffs_ExcludesPattern(t *testing.T) {
	redactor := NewRedactor([]string{"*.env"})
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	diffs := []git.FileDiff{
		{Path: "main.go"},
		{Path: patternDotEnv},
		{Path: "app.go"},
	}

	filtered := b.filterDiffs(diffs)
	require.Len(t, filtered, 2)
	assert.Equal(t, "main.go", filtered[0].Path)
	assert.Equal(t, "app.go", filtered[1].Path)
}

func TestFilterDiffs_NilRedactor(t *testing.T) {
	// A nil redactor triggers fail-closed behavior: a default redactor is
	// created with built-in patterns, so .env files are still filtered.
	b := NewBuilder(&mockGitClient{}, nil, 0)

	diffs := []git.FileDiff{{Path: patternDotEnv}, {Path: "main.go"}}
	filtered := b.filterDiffs(diffs)
	require.Len(t, filtered, 1, ".env should be excluded by default redactor")
	assert.Equal(t, "main.go", filtered[0].Path)
}

// ---------------------------------------------------------------------------
// estimateTokensForStatus
// ---------------------------------------------------------------------------

func TestEstimateTokensForStatus(t *testing.T) {
	statuses := []git.FileStatus{
		{Path: "main.go"},       // 7 chars + 5 = 12
		{Path: "go.mod"},        // 6 chars + 5 = 11
		{Path: "internal/x.go"}, // 13 chars + 5 = 18
	}
	// Total: 12 + 11 + 18 = 41, / 4 = 10
	tokens := estimateTokensForStatus(statuses)
	assert.Equal(t, 10, tokens)

	// Empty list.
	assert.Equal(t, 0, estimateTokensForStatus(nil))
}

// ---------------------------------------------------------------------------
// tokenBudget.remaining exhausted
// ---------------------------------------------------------------------------

func TestTokenBudget_Exhausted(t *testing.T) {
	tb := newTokenBudget(10)
	tb.consume(10)
	assert.Equal(t, 0, tb.remaining())
	assert.False(t, tb.canFit(1))

	// Over-consume — remaining should clamp to 0.
	tb.consume(5)
	assert.Equal(t, 0, tb.remaining())
}

func TestTokenBudget_UnlimitedRemaining(t *testing.T) {
	tb := newTokenBudget(0) // unlimited
	// remaining() should return max int.
	assert.True(t, tb.remaining() > 1000000000)
}

// ---------------------------------------------------------------------------
// readFileContent edge cases
// ---------------------------------------------------------------------------

func TestReadFileContent_Excluded(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env", "SECRET=hunter2")

	redactor := NewRedactor(nil)
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	content := b.readFileContent(root, ".env")
	assert.Empty(t, content, "excluded file should return empty string")
}

func TestReadFileContent_NonExistentFile(t *testing.T) {
	root := t.TempDir()
	b := NewBuilder(&mockGitClient{}, nil, 0)

	content := b.readFileContent(root, "does_not_exist.go")
	assert.Empty(t, content, "missing file should return empty string")
}

func TestReadFileContent_AppliesRedaction(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "config.go", `password = "SuperSecret1234567"`)

	redactor := NewRedactor(nil)
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	content := b.readFileContent(root, "config.go")
	assert.Contains(t, content, RedactedPlaceholder)
	assert.NotContains(t, content, "SuperSecret1234567")
}

// ---------------------------------------------------------------------------
// redactString fail-closed on RedactContent error (#77)
// ---------------------------------------------------------------------------

func TestRedactString_ReturnsPlaceholderOnError(t *testing.T) {
	redactor := NewRedactor(nil)
	redactor.forceErr = errors.New("simulated redaction failure")
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	secret := `password = "SuperSecret1234567"`
	result := b.redactString(secret)

	assert.Equal(t, RedactionFailedPlaceholder, result,
		"redactString must return safe placeholder when RedactContent fails")
	assert.NotContains(t, result, "SuperSecret",
		"original secret must never appear in output on error")
}

func TestReadFileContent_ReturnsPlaceholderOnRedactionError(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "secret.go", `token = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"`)

	redactor := NewRedactor(nil)
	redactor.forceErr = errors.New("simulated redaction failure")
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	content := b.readFileContent(root, "secret.go")
	assert.Equal(t, RedactionFailedPlaceholder, content,
		"readFileContent must return safe placeholder when redaction fails")
	assert.NotContains(t, content, "ghp_",
		"secrets must never leak when redaction fails")
}

func TestRedactString_SuccessPath(t *testing.T) {
	redactor := NewRedactor(nil)
	b := NewBuilder(&mockGitClient{}, redactor, 0)

	// Content with a secret should be redacted normally.
	result := b.redactString(`api_key = "AKIAIOSFODNN7EXAMPLE"`)
	assert.Contains(t, result, RedactedPlaceholder)
	assert.NotContains(t, result, "AKIAIOSFODNN7EXAMPLE")

	// Clean content passes through unchanged.
	clean := "func main() {}"
	assert.Equal(t, clean, b.redactString(clean))
}

// ---------------------------------------------------------------------------
// estimateTokensForDiffs with OldPath
// ---------------------------------------------------------------------------

func TestEstimateTokensForDiffs_WithOldPath(t *testing.T) {
	diffs := []git.FileDiff{
		{
			Path:    "new_name.go",
			OldPath: "old_name.go",
			Hunks: []git.Hunk{{
				Header: "@@",
				Lines:  []git.DiffLine{{Content: "x"}},
			}},
		},
	}

	tokens := estimateTokensForDiffs(diffs)
	// new_name.go (11) + old_name.go (11) + @@ (2) + x+newline (2) = 26 / 4 = 6
	assert.Equal(t, 6, tokens)

	// Without OldPath — should be less.
	diffs[0].OldPath = ""
	tokensWithout := estimateTokensForDiffs(diffs)
	assert.Less(t, tokensWithout, tokens)
}

// ---------------------------------------------------------------------------
// ForReview with tight budget skips file contents
// ---------------------------------------------------------------------------

func TestForReview_TightBudgetSkipsFileContents(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "big.go", strings.Repeat("x", 1000))

	mock := newMock("dev", root)
	mock.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{sampleDiff("big.go", "small change")}, nil
	}

	// Budget big enough for diff but not for file content.
	b := NewBuilder(mock, nil, 20)
	gc, err := b.ForReview(context.Background(), git.DiffOpts{})
	require.NoError(t, err)

	// Diff present, file content skipped due to budget.
	require.Len(t, gc.Diffs, 1)
	assert.Empty(t, gc.FileContents, "file contents should be skipped when budget is tight")
}

// ---------------------------------------------------------------------------
// ForConflict with tight budget
// ---------------------------------------------------------------------------

func TestForConflict_TightBudgetSkipsFileContents(t *testing.T) {
	root := t.TempDir()
	conflictContent := strings.Repeat("x", 500) + "\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> br\n"
	writeTestFile(t, root, "big_conflict.go", conflictContent)

	mock := newMock("main", root)

	// Budget too small for file content.
	b := NewBuilder(mock, nil, 5)
	gc, err := b.ForConflict(context.Background(), []string{"big_conflict.go"})
	require.NoError(t, err)

	require.Len(t, gc.Conflicts, 1)
	// FileContents should be empty — budget prevents inclusion.
	assert.Empty(t, gc.FileContents)
}

// ---------------------------------------------------------------------------
// ForChangelog with diffs
// ---------------------------------------------------------------------------

func TestForChangelog_IncludesDiffs(t *testing.T) {
	mock := newMock("main", "/repo")
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{sampleCommit("a1", "feat: new feature")}, nil
	}
	mock.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		assert.Equal(t, "v1.0", opts.CommitA)
		assert.Equal(t, "v2.0", opts.CommitB)
		return []git.FileDiff{sampleDiff("new.go", "added")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForChangelog(context.Background(), "v1.0", "v2.0")
	require.NoError(t, err)

	require.Len(t, gc.Log, 1)
	require.Len(t, gc.Diffs, 1)
	assert.Equal(t, "new.go", gc.Diffs[0].Path)
}

// ---------------------------------------------------------------------------
// parseConflictMarkers edge cases
// ---------------------------------------------------------------------------

func TestParseConflictMarkers_UnterminatedConflict(t *testing.T) {
	// Conflict that starts but never ends (no >>>>>>>).
	content := "<<<<<<< HEAD\nours\n=======\ntheirs\n"
	regions := parseConflictMarkers(content)
	assert.Empty(t, regions, "unterminated conflict should not produce a region")
}

func TestParseConflictMarkers_EmptyConflict(t *testing.T) {
	// Conflict with empty ours and theirs sections.
	content := "<<<<<<< HEAD\n=======\n>>>>>>> branch\n"
	regions := parseConflictMarkers(content)
	require.Len(t, regions, 1)
	assert.Empty(t, regions[0].Ours)
	assert.Empty(t, regions[0].Theirs)
}

// ---------------------------------------------------------------------------
// Coverage: Registry.PrimaryName (registry.go:77)
// ---------------------------------------------------------------------------

func TestRegistryPrimaryName(t *testing.T) {
	r := &Registry{primary: providerClaude}
	assert.Equal(t, providerClaude, r.PrimaryName())
}

// ---------------------------------------------------------------------------
// Coverage: ClaudeProvider.Close (provider_claude.go:92)
// ---------------------------------------------------------------------------

func TestClaudeProviderClose(t *testing.T) {
	p := NewClaudeProvider("", 0)
	assert.NoError(t, p.Close())
}

// ---------------------------------------------------------------------------
// Coverage: buildParams with Tools (provider_claude.go:128-130)
// ---------------------------------------------------------------------------

func TestBuildParams_WithToolDefinitions(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{
		UserPrompt: "Call a tool",
		Tools: []ToolDefinition{
			{Name: "greet", Description: "Say hello", Parameters: map[string]any{}},
		},
	})

	assert.NotEmpty(t, params.Tools, "tools should be forwarded to params")
}

// ---------------------------------------------------------------------------
// Coverage: currentBranch with no branch marked current (context.go:131)
// ---------------------------------------------------------------------------

func TestCurrentBranch_NoBranchIsCurrent(t *testing.T) {
	mock := &mockGitClient{
		BranchListFunc: func(_ context.Context) ([]git.Branch, error) {
			return []git.Branch{
				{Name: "main", IsCurrent: false},
				{Name: "dev", IsCurrent: false},
			}, nil
		},
	}
	b := NewBuilder(mock, nil, 0)
	assert.Equal(t, "", b.currentBranch(context.Background()))
}

// ---------------------------------------------------------------------------
// Coverage: readFileContent path-jail violation (context.go:172-174)
// ---------------------------------------------------------------------------

func TestReadFileContent_PathTraversal(t *testing.T) {
	root := t.TempDir()
	// Write a file outside root to ensure the jail catches traversal.
	writeTestFile(t, root, "safe.go", "package safe")

	b := NewBuilder(&mockGitClient{}, nil, 0)

	// Attempt to escape the repo root via directory traversal.
	content := b.readFileContent(root, "../../../etc/passwd")
	assert.Empty(t, content, "path-jail should block traversal")
}

// ---------------------------------------------------------------------------
// Coverage: ForConflict — empty content continue (context.go:273-274)
// ---------------------------------------------------------------------------

func TestForConflict_NonExistentFileSkipped(t *testing.T) {
	root := t.TempDir()
	// Don't create any file — readFileContent will return "".
	mock := newMock("main", root)

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForConflict(context.Background(), []string{"does_not_exist.go"})
	require.NoError(t, err)

	assert.Empty(t, gc.Conflicts, "non-existent file should be skipped")
	assert.Empty(t, gc.FileContents)
}

// ---------------------------------------------------------------------------
// Coverage: ForConflict — no markers continue (context.go:277-278)
// ---------------------------------------------------------------------------

func TestForConflict_FileWithNoMarkers(t *testing.T) {
	root := t.TempDir()
	// File exists but has no conflict markers.
	writeTestFile(t, root, "clean.go", "package main\nfunc main() {}\n")

	mock := newMock("main", root)

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForConflict(context.Background(), []string{"clean.go"})
	require.NoError(t, err)

	assert.Empty(t, gc.Conflicts, "file without markers should produce no conflicts")
}

// ---------------------------------------------------------------------------
// Coverage: ForConflict — log trimming path (context.go:292-294)
// ---------------------------------------------------------------------------

func TestForConflict_IncludesLogEntries(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "c.go", "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> br\n")

	mock := newMock("main", root)
	mock.LogFunc = func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
		assert.Equal(t, 5, opts.MaxCount)
		return []git.Commit{sampleCommit("abc", "fix: conflict")}, nil
	}

	b := NewBuilder(mock, nil, 0)
	gc, err := b.ForConflict(context.Background(), []string{"c.go"})
	require.NoError(t, err)

	require.Len(t, gc.Conflicts, 1)
	require.Len(t, gc.Log, 1)
	assert.Equal(t, "fix: conflict", gc.Log[0].Subject)
}

// ---------------------------------------------------------------------------
// Coverage: serializeGitContext DiffLineRemoved (provider_copilot.go:349-350)
// ---------------------------------------------------------------------------

func TestSerializeGitContext_DiffLineRemoved(t *testing.T) {
	gc := GitContext{
		CurrentBranch: "main",
		Diffs: []git.FileDiff{{
			Path: "change.go",
			Hunks: []git.Hunk{{
				Header: "@@ -1,3 +1,2 @@",
				Lines: []git.DiffLine{
					{Type: git.DiffLineRemoved, Content: "old line"},
					{Type: git.DiffLineAdded, Content: "new line"},
				},
			}},
		}},
	}

	out := serializeGitContext(gc)
	assert.Contains(t, out, "-old line")
	assert.Contains(t, out, "+new line")
}

// ---------------------------------------------------------------------------
// Coverage: AuditLogger.Close sync-error path (audit.go:107-111)
// ---------------------------------------------------------------------------

func TestAuditLogger_CloseHandlesSyncError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	// Sabotage the underlying file so Sync fails.
	_ = al.file.Close()

	err = al.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syncing audit log on close")
}

// ---------------------------------------------------------------------------
// Coverage: AuditLogger.Log write/sync error (audit.go:87-89, 91-93)
// ---------------------------------------------------------------------------

func TestAuditLogger_LogFailsOnClosedFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	// Sabotage the file handle.
	_ = al.file.Close()

	err = al.Log(AuditEntry{Operation: "test", Provider: "test", Result: "accepted"})
	require.Error(t, err, "logging to a closed file should fail")
}

// ---------------------------------------------------------------------------
// Coverage: AuditLogger.Log write error (audit.go:87-89)
// ---------------------------------------------------------------------------

func TestAuditLogger_LogWriteError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(logPath)
	require.NoError(t, err)

	// Replace writable handle with a read-only handle so Write fails
	// but Stat succeeds (rotateIfNeeded passes through).
	_ = al.file.Close()
	al.file, err = os.Open(logPath)
	require.NoError(t, err)
	defer func() { _ = al.file.Close() }()

	err = al.Log(AuditEntry{Operation: "test", Provider: "test", Result: "accepted"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing audit entry")
}

// ---------------------------------------------------------------------------
// Coverage: NewAuditLogger OpenFile error (audit.go:57-59)
// ---------------------------------------------------------------------------

func TestNewAuditLogger_InvalidPath(t *testing.T) {
	// Use a directory path so OpenFile fails (can't open dir as file).
	dir := t.TempDir()
	_, err := NewAuditLogger(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening audit log")
}

// ---------------------------------------------------------------------------
// Coverage: NewAuditLogger MkdirAll error (audit.go:52-54)
// ---------------------------------------------------------------------------

func TestNewAuditLogger_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file that blocks directory creation.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	// MkdirAll will fail because "blocker" is a file, not a directory.
	_, err := NewAuditLogger(filepath.Join(blocker, "sub", "audit.log"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating audit log directory")
}

// ---------------------------------------------------------------------------
// Coverage: CopilotProvider.ensureStarted already-started path
// (provider_copilot.go:214-216)
// ---------------------------------------------------------------------------

func TestCopilotProvider_EnsureStartedNoop(t *testing.T) {
	// After once.Do has executed, ensureStarted returns the cached
	// startErr without touching the client again.
	p := &CopilotProvider{}
	p.once.Do(func() {}) // simulate already-started (no error)
	err := p.ensureStarted(context.Background())
	assert.NoError(t, err)
}
