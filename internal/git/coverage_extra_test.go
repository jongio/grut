package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRun is a test helper that runs a git command in the given directory.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = testGitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

// ────────────── Diff option coverage ──────────────

func TestClient_Diff_Staged(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Modify and stage a file to create staged changes.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644))
	gitRun(t, dir, "add", "README.md")

	diffs, err := client.Diff(ctx, DiffOpts{Staged: true})
	require.NoError(t, err)
	assert.NotEmpty(t, diffs, "staged diff should have entries")
}

func TestClient_Diff_IgnoreAll(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create whitespace-only change.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test  \n"), 0o644))

	diffs, err := client.Diff(ctx, DiffOpts{IgnoreAll: true})
	require.NoError(t, err)
	// With whitespace-only changes and -w flag, diff should be empty.
	assert.Empty(t, diffs, "whitespace-only changes should be empty with IgnoreAll")
}

func TestClient_Diff_CustomContext(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644))

	diffs, err := client.Diff(ctx, DiffOpts{Context: 10})
	require.NoError(t, err)
	assert.NotEmpty(t, diffs)
}

func TestClient_Diff_Path(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create two modified files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644))

	diffs, err := client.Diff(ctx, DiffOpts{Path: "README.md"})
	require.NoError(t, err)
	assert.Len(t, diffs, 1, "path filter should return only 1 file diff")
	assert.Equal(t, "README.md", diffs[0].Path)
}

func TestClient_Diff_CommitComparison(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()
	branch := detectDefaultBranch(t, dir)

	// Create a second commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("v2\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "second")

	// Compare HEAD~1 to HEAD.
	diffs, err := client.Diff(ctx, DiffOpts{CommitA: branch + "~1", CommitB: branch})
	require.NoError(t, err)
	assert.NotEmpty(t, diffs, "commit comparison should show diffs")
}

func TestClient_Diff_InvalidCommitA(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.Diff(context.Background(), DiffOpts{CommitA: "--malicious"})
	assert.Error(t, err, "invalid commitA should be rejected")
}

func TestClient_Diff_InvalidCommitB(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.Diff(context.Background(), DiffOpts{CommitA: "HEAD", CommitB: "--bad"})
	assert.Error(t, err, "invalid commitB should be rejected")
}

func TestClient_Diff_InvalidPath(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.Diff(context.Background(), DiffOpts{Path: "--flag"})
	assert.Error(t, err, "invalid path should be rejected")
}

// ────────────── Log option coverage ──────────────

func TestClient_Log_MaxCount(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create multiple commits.
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("v"+string(rune('0'+i))+"\n"), 0o644))
		gitRun(t, dir, "add", "README.md")
		gitRun(t, dir, "commit", "-m", "commit "+string(rune('0'+i)))
	}

	commits, err := client.Log(ctx, LogOpts{MaxCount: 2})
	require.NoError(t, err)
	assert.Len(t, commits, 2, "MaxCount=2 should return exactly 2 commits")
}

func TestClient_Log_Skip(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create 3 more commits (4 total including initial).
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("v"+string(rune('0'+i))+"\n"), 0o644))
		gitRun(t, dir, "add", "README.md")
		gitRun(t, dir, "commit", "-m", "commit "+string(rune('0'+i)))
	}

	allCommits, err := client.Log(ctx, LogOpts{})
	require.NoError(t, err)

	skippedCommits, err := client.Log(ctx, LogOpts{Skip: 2})
	require.NoError(t, err)
	assert.Equal(t, len(allCommits)-2, len(skippedCommits), "Skip=2 should return 2 fewer commits")
}

func TestClient_Log_Author(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	commits, err := client.Log(ctx, LogOpts{Author: "Test"})
	require.NoError(t, err)
	assert.NotEmpty(t, commits, "author filter should match 'Test' (from initTestRepo config)")
}

func TestClient_Log_Grep(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create commit with specific message.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("xyz\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "fix: unique-grep-pattern-12345")

	commits, err := client.Log(ctx, LogOpts{Grep: "unique-grep-pattern-12345"})
	require.NoError(t, err)
	assert.Len(t, commits, 1, "grep should find exactly the matching commit")
	assert.Contains(t, commits[0].Subject, "unique-grep-pattern-12345")
}

func TestClient_Log_All(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	commits, err := client.Log(ctx, LogOpts{All: true})
	require.NoError(t, err)
	assert.NotEmpty(t, commits, "all flag should return commits from all refs")
}

func TestClient_Log_Path(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create a second file and commit it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "other.txt")
	gitRun(t, dir, "commit", "-m", "add other")

	commits, err := client.Log(ctx, LogOpts{Path: "other.txt"})
	require.NoError(t, err)
	assert.Len(t, commits, 1, "path filter should show only commits touching other.txt")
}

func TestClient_Log_Since(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	commits, err := client.Log(ctx, LogOpts{Since: "1 year ago"})
	require.NoError(t, err)
	assert.NotEmpty(t, commits, "since '1 year ago' should include recent commits")
}

func TestClient_Log_Until(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	commits, err := client.Log(ctx, LogOpts{Until: "2000-01-01"})
	require.NoError(t, err)
	assert.Empty(t, commits, "until 2000 should exclude all recent commits")
}

func TestClient_Log_InvalidRef(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.Log(context.Background(), LogOpts{Ref: "--malicious"})
	assert.Error(t, err, "invalid ref should be rejected")
}

func TestClient_Log_InvalidPath(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	_, err = client.Log(context.Background(), LogOpts{Path: "--flag"})
	assert.Error(t, err, "invalid path should be rejected")
}

// ────────────── StashPush option coverage ──────────────

func TestClient_StashPush_WithMessage(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create unstaged change.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("stash me\n"), 0o644))

	err = client.StashPush(ctx, StashOpts{Message: "my stash"})
	require.NoError(t, err)

	stashes, err := client.StashList(ctx)
	require.NoError(t, err)
	require.Len(t, stashes, 1)
	assert.Contains(t, stashes[0].Message, "my stash")
}

func TestClient_StashPush_KeepIndex(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Stage a change.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("staged\n"), 0o644))
	gitRun(t, dir, "add", "README.md")

	err = client.StashPush(ctx, StashOpts{KeepIndex: true})
	require.NoError(t, err)

	// Staged changes should still be in the index after stash with --keep-index.
	status, err := client.Status(ctx)
	require.NoError(t, err)
	// File should still be in index (staged).
	found := false
	for _, f := range status {
		if f.Path == "README.md" {
			found = true
			break
		}
	}
	assert.True(t, found, "README.md should still be in index with --keep-index")
}

func TestClient_StashPush_Paths(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create two modified files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644))

	// Stash only README.md.
	err = client.StashPush(ctx, StashOpts{Paths: []string{"README.md"}})
	require.NoError(t, err)

	// After stash, other.txt should still be modified but README.md should be clean.
	status, err := client.Status(ctx)
	require.NoError(t, err)
	hasOther := false
	hasReadme := false
	for _, f := range status {
		if f.Path == "other.txt" {
			hasOther = true
		}
		if f.Path == "README.md" {
			hasReadme = true
		}
	}
	assert.True(t, hasOther, "other.txt should still be modified")
	assert.False(t, hasReadme, "README.md should be stashed (clean)")
}

func TestClient_StashPush_InvalidPath(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)

	err = client.StashPush(context.Background(), StashOpts{Paths: []string{"--malicious"}})
	assert.Error(t, err, "invalid path should be rejected")
}

// ────────────── Blame ──────────────

func TestClient_Blame_MultiCommit(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Add more content in a second commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\nline2\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "add line")

	lines, err := client.Blame(ctx, "README.md")
	require.NoError(t, err)
	assert.Len(t, lines, 2, "blame should have 2 lines")
}

// ────────────── RemoteAdd ──────────────

func TestClient_RemoteAdd_Success(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	err = client.RemoteAdd(ctx, "upstream", "https://github.com/example/repo.git")
	require.NoError(t, err)

	remotes, err := client.RemoteList(ctx)
	require.NoError(t, err)
	found := false
	for _, r := range remotes {
		if r.Name == "upstream" {
			found = true
			break
		}
	}
	assert.True(t, found, "upstream remote should exist after RemoteAdd")
}

// ────────────── parseDiffHeader edge cases ──────────────

func TestParseDiffHeader_Rename(t *testing.T) {
	t.Parallel()
	input := "diff --git a/old.go b/new.go\n" +
		"similarity index 90%\n" +
		"rename from old.go\n" +
		"rename to new.go\n" +
		"--- a/old.go\n" +
		"+++ b/new.go\n" +
		"@@ -1,3 +1,4 @@\n" +
		" line1\n" +
		"+added\n" +
		" line2\n" +
		" line3\n"
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)
	assert.Equal(t, "new.go", diffs[0].Path)
	assert.Equal(t, "old.go", diffs[0].OldPath)
}

func TestParseDiffHeader_Binary(t *testing.T) {
	t.Parallel()
	input := "diff --git a/image.png b/image.png\n" +
		"Binary files a/image.png and b/image.png differ\n"
	diffs, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, diffs, 1)
	assert.True(t, diffs[0].IsBinary)
}

// ────────────── BranchRename ──────────────

func TestClient_BranchRename(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create a branch to rename.
	require.NoError(t, client.BranchCreate(ctx, "old-branch", ""))
	err = client.BranchRename(ctx, "old-branch", "new-branch")
	require.NoError(t, err)

	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	found := false
	for _, b := range branches {
		if b.Name == "new-branch" {
			found = true
		}
		assert.NotEqual(t, "old-branch", b.Name, "old branch name should not exist")
	}
	assert.True(t, found, "new-branch should exist after rename")
}
