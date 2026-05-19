package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain isolates the entire test process from user/system git configuration.
// Without this, git commands spawned by the production Client.run() method
// inherit the user's global config (GPG signing, LFS filters, credential
// helpers) which can cause interactive prompts and indefinite hangs.
func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", "")
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	os.Exit(m.Run())
}

// testGitEnv returns environment variables for running git in tests.
// It isolates test repos from user/system git configuration to prevent
// interactive prompts (GPG signing, credential helpers, editors) and
// slow filters (git-lfs) from causing test hangs or flakiness.
func testGitEnv() []string {
	overrides := map[string]string{
		"GIT_AUTHOR_NAME":     "Test",
		"GIT_AUTHOR_EMAIL":    "test@example.com",
		"GIT_COMMITTER_NAME":  "Test",
		"GIT_COMMITTER_EMAIL": "test@example.com",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   "",
	}

	// Build env from os.Environ(), replacing any keys we override.
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if _, skip := overrides[k]; !skip {
			env = append(env, e)
		}
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

// initTestRepo creates a temporary git repository with an initial commit.
// Returns the repo path. The repo is cleaned up automatically by t.TempDir.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = testGitEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	run("init")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	// Disable autocrlf to ensure consistent line endings across platforms.
	run("config", "core.autocrlf", "false")
	// Disable GPG signing to prevent interactive prompts in CI/test.
	run("config", "commit.gpgsign", "false")
	run("config", "tag.gpgsign", "false")

	// Create initial file and commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "Initial commit")

	return dir
}

// detectDefaultBranch returns the name of the default branch in the test repo.
// Modern git defaults to "main" but older installations may use "master".
func detectDefaultBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	cmd.Env = testGitEnv()
	out, err := cmd.Output()
	if err != nil {
		// Fallback: try symbolic-ref.
		cmd = exec.Command("git", "symbolic-ref", "--short", "HEAD")
		cmd.Dir = dir
		cmd.Env = testGitEnv()
		out, err = cmd.Output()
		require.NoError(t, err, "cannot detect default branch")
	}
	branch := strings.TrimSpace(string(out))
	require.NotEmpty(t, branch, "default branch name is empty")
	return branch
}

func TestClient_IsRepo(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	isRepo, err := client.IsRepo(ctx)
	require.NoError(t, err)
	assert.True(t, isRepo)

	// Non-repo directory — use os.MkdirTemp (not t.TempDir) to create a
	// temp dir in the OS temp directory, bypassing GOTMPDIR which may
	// point inside this git repository. Place an empty .git file inside
	// to prevent git from searching parent directories for a repository,
	// making the assertion robust regardless of where the OS temp lives.
	tmpDir, mkErr := os.MkdirTemp("", "grut-test-nonrepo-*")
	require.NoError(t, mkErr)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git"), []byte(""), 0o644))
	nonRepoClient, err := NewClient(tmpDir)
	require.NoError(t, err)

	isRepo, err = nonRepoClient.IsRepo(ctx)
	require.NoError(t, err)
	assert.False(t, isRepo)
}

func TestClient_RepoRoot(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	root, err := client.RepoRoot(ctx)
	require.NoError(t, err)

	// Normalize paths for comparison (resolve symlinks on some OS).
	expectedDir, _ := filepath.EvalSymlinks(dir)
	actualRoot, _ := filepath.EvalSymlinks(root)
	assert.Equal(t, filepath.Clean(expectedDir), filepath.Clean(actualRoot))
}

func TestClient_Status(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Clean status — no changes.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	assert.Empty(t, statuses)

	// Create an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0o644))

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "new.txt", statuses[0].Path)
	assert.Equal(t, StatusUntracked, statuses[0].StagedStatus)

	// Stage the file.
	require.NoError(t, client.Stage(ctx, []string{"new.txt"}))

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)
	assert.Equal(t, StatusUnmodified, statuses[0].WorktreeStatus)
}

func TestClient_StageAndUnstage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create and stage a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("data\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"staged.txt"}))

	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)

	// Unstage.
	require.NoError(t, client.Unstage(ctx, []string{"staged.txt"}))

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusUntracked, statuses[0].StagedStatus)
}

func TestClient_Commit(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create and stage a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "committed.txt"), []byte("data\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"committed.txt"}))

	hash, err := client.Commit(ctx, "Add committed.txt", CommitOpts{})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 40) // Full SHA-1 hash

	// Verify clean status.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	assert.Empty(t, statuses)
}

func TestClient_Log(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Add a second commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"second.txt"}))
	_, err = client.Commit(ctx, "Second commit", CommitOpts{})
	require.NoError(t, err)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 10})
	require.NoError(t, err)
	require.Len(t, commits, 2)

	assert.Equal(t, "Second commit", commits[0].Subject)
	assert.Equal(t, "Initial commit", commits[1].Subject)
	assert.Equal(t, "Test", commits[0].Author)
}

func TestClient_Diff(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Modify an existing file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\nModified\n"), 0o644))

	diffs, err := client.Diff(ctx, DiffOpts{})
	require.NoError(t, err)
	require.Len(t, diffs, 1)

	assert.Equal(t, "README.md", diffs[0].Path)
	assert.False(t, diffs[0].IsBinary)
	require.NotEmpty(t, diffs[0].Hunks)
}

func TestClient_BranchOperations(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create a new branch.
	require.NoError(t, client.BranchCreate(ctx, "feature-test", ""))

	branches, err := client.BranchList(ctx)
	require.NoError(t, err)

	// Find our new branch.
	var found bool
	for _, b := range branches {
		if b.Name == "feature-test" {
			found = true
			break
		}
	}
	assert.True(t, found, "feature-test branch should exist")

	// Rename the branch.
	require.NoError(t, client.BranchRename(ctx, "feature-test", "feature-renamed"))

	branches, err = client.BranchList(ctx)
	require.NoError(t, err)

	var renamedFound bool
	for _, b := range branches {
		if b.Name == "feature-renamed" {
			renamedFound = true
		}
		if b.Name == "feature-test" {
			t.Error("old branch name should not exist")
		}
	}
	assert.True(t, renamedFound, "feature-renamed branch should exist")

	// Delete the branch.
	require.NoError(t, client.BranchDelete(ctx, "feature-renamed", false))

	branches, err = client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		assert.NotEqual(t, "feature-renamed", b.Name)
	}
}

func TestClient_Checkout(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create and checkout a new branch.
	require.NoError(t, client.BranchCreate(ctx, "dev", ""))
	require.NoError(t, client.Checkout(ctx, "dev"))

	branches, err := client.BranchList(ctx)
	require.NoError(t, err)

	for _, b := range branches {
		if b.Name == "dev" {
			assert.True(t, b.IsCurrent)
		}
	}

	// Switch back to the default branch (may be "main" or "master"
	// depending on git configuration).
	defaultBranch := detectDefaultBranch(t, dir)
	require.NoError(t, client.Checkout(ctx, defaultBranch))
}

func TestClient_TagOperations(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create lightweight tag.
	require.NoError(t, client.TagCreate(ctx, "v0.1.0", "", ""))

	// Create annotated tag.
	require.NoError(t, client.TagCreate(ctx, "v1.0.0", "", "Release 1.0"))

	tags, err := client.TagList(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tags), 2)

	tagNames := make(map[string]bool)
	for _, tag := range tags {
		tagNames[tag.Name] = true
	}
	assert.True(t, tagNames["v0.1.0"])
	assert.True(t, tagNames["v1.0.0"])

	// Delete a tag.
	require.NoError(t, client.TagDelete(ctx, "v0.1.0"))

	tags, err = client.TagList(ctx)
	require.NoError(t, err)
	for _, tag := range tags {
		assert.NotEqual(t, "v0.1.0", tag.Name)
	}
}

func TestClient_Blame(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	blameLines, err := client.Blame(ctx, "README.md")
	require.NoError(t, err)
	require.Len(t, blameLines, 1) // Single line: "# Test"

	assert.Equal(t, "# Test", blameLines[0].Content)
	assert.Equal(t, "Test", blameLines[0].Author)
	assert.Equal(t, 1, blameLines[0].LineNo)
}

func TestClient_Reflog(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	entries, err := client.Reflog(ctx, "HEAD", 10)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	assert.NotEmpty(t, entries[0].Hash)
}

func TestClient_StashOperations(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Modify a tracked file for stashing.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0o644))

	// Stash push.
	require.NoError(t, client.StashPush(ctx, StashOpts{Message: "WIP changes"}))

	// Verify file is restored.
	content, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(content))

	// Stash list.
	entries, err := client.StashList(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "WIP changes")

	// Stash pop.
	require.NoError(t, client.StashPop(ctx, 0))

	// Verify file is modified again.
	content, err = os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Modified\n", string(content))

	// Verify stash list is empty.
	entries, err = client.StashList(ctx)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestClient_ContextCancellation(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = client.Status(ctx)
	assert.Error(t, err)
}

func TestClient_ValidationErrors(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	// Stage with no paths.
	err = client.Stage(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no paths")

	// Stage with invalid path.
	err = client.Stage(ctx, []string{"file;rm -rf /"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden character")

	// Checkout with invalid ref.
	err = client.Checkout(ctx, "branch..name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "'..'")

	// Commit with empty message.
	_, err = client.Commit(ctx, "", CommitOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message must not be empty")
}

func TestClient_WorktreeList(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	worktrees, err := client.WorktreeList(ctx)
	require.NoError(t, err)
	require.Len(t, worktrees, 1) // Main worktree only.

	assert.NotEmpty(t, worktrees[0].Path)
	assert.NotEmpty(t, worktrees[0].Head)
}

func TestCache_StatusRoundTrip(t *testing.T) {
	cache := NewCache(time.Minute)

	// Empty cache.
	data, fresh := cache.GetStatus()
	assert.Nil(t, data)
	assert.False(t, fresh)

	// Set data.
	status := []FileStatus{
		{Path: "file.go", StagedStatus: StatusModified},
	}
	cache.SetStatus(status)

	data, fresh = cache.GetStatus()
	require.Len(t, data, 1)
	assert.True(t, fresh)
	assert.Equal(t, "file.go", data[0].Path)

	// Verify it's a copy (mutation safety).
	data[0].Path = "mutated.go"
	data2, _ := cache.GetStatus()
	assert.Equal(t, "file.go", data2[0].Path)
}

func TestCache_Invalidate(t *testing.T) {
	cache := NewCache(time.Minute)

	cache.SetStatus([]FileStatus{{Path: "a.go"}})
	cache.SetBranches([]Branch{{Name: "main"}})

	cache.Invalidate()

	data, fresh := cache.GetStatus()
	assert.Nil(t, data)
	assert.False(t, fresh)

	branches, fresh := cache.GetBranches()
	assert.Nil(t, branches)
	assert.False(t, fresh)
}

func TestCache_Expiration(t *testing.T) {
	cache := NewCache(time.Millisecond) // Very short TTL.

	cache.SetStatus([]FileStatus{{Path: "a.go"}})

	// Wait for expiration.
	time.Sleep(5 * time.Millisecond)

	data, fresh := cache.GetStatus()
	assert.NotNil(t, data) // Data still returned
	assert.False(t, fresh) // But marked as stale
}

func TestOpQueue_Serialization(t *testing.T) {
	q := &OpQueue{}
	ctx := context.Background()

	results := make([]int, 0, 3)
	done := make(chan struct{})

	go func() {
		for i := range 3 {
			i := i
			_ = q.Exec(ctx, func() error {
				results = append(results, i)
				return nil
			})
		}
		close(done)
	}()

	<-done
	assert.Equal(t, []int{0, 1, 2}, results)
}

func TestOpQueue_ContextCancelled(t *testing.T) {
	q := &OpQueue{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Exec(ctx, func() error {
		t.Fatal("should not execute")
		return nil
	})
	assert.Error(t, err)
}

func TestNewClient_EmptyDir(t *testing.T) {
	_, err := NewClient("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repoDir must not be empty")
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "simple", input: "a\nb\nc\n", want: []string{"a", "b", "c"}},
		{name: "with CR", input: "a\r\nb\r\n", want: []string{"a", "b"}},
		{name: "empty lines filtered", input: "a\n\nb\n", want: []string{"a", "b"}},
		{name: "empty string", input: "", want: []string{}},
		{name: "single line", input: "hello", want: []string{"hello"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseWorktreeList(t *testing.T) {
	input := `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main

worktree /path/to/feature
HEAD def456abc789
branch refs/heads/feature

`
	worktrees := parseWorktreeList(input)
	require.Len(t, worktrees, 2)

	assert.Equal(t, filepath.FromSlash("/path/to/main"), worktrees[0].Path)
	assert.Equal(t, "abc123def456", worktrees[0].Head)
	assert.Equal(t, "main", worktrees[0].Branch)
	assert.False(t, worktrees[0].Bare)

	assert.Equal(t, filepath.FromSlash("/path/to/feature"), worktrees[1].Path)
	assert.Equal(t, "feature", worktrees[1].Branch)
}

func TestParseWorktreeList_Bare(t *testing.T) {
	input := `worktree /path/to/bare
HEAD abc123
bare

`
	worktrees := parseWorktreeList(input)
	require.Len(t, worktrees, 1)
	assert.True(t, worktrees[0].Bare)
}

func TestParseStashList(t *testing.T) {
	input := "abc123\x1estash@{0}\x1eWIP on main: first stash\n" +
		"def456\x1estash@{1}\x1eWIP on main: second stash\n"

	entries := parseStashList(input)
	require.Len(t, entries, 2)

	assert.Equal(t, "abc123", entries[0].Hash)
	assert.Equal(t, 0, entries[0].Index)
	assert.Equal(t, "WIP on main: first stash", entries[0].Message)

	assert.Equal(t, 1, entries[1].Index)
}

func TestParseStashIndex(t *testing.T) {
	tests := []struct {
		ref     string
		want    int
		wantErr bool
	}{
		{"stash@{0}", 0, false},
		{"stash@{3}", 3, false},
		{"stash@{12}", 12, false},
		{"invalid", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := parseStashIndex(tt.ref)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTagList(t *testing.T) {
	const sep = "\x1e"
	input := "v1.0.0" + sep + "abc123" + sep + "tag" + sep + "Release 1.0" + sep + "John Doe" + sep + "2024-01-15 10:30:00 +0000\n" +
		"v0.1.0" + sep + "def456" + sep + "commit" + sep + "" + sep + "" + sep + "2024-01-10 09:00:00 +0000\n"

	tags := parseTagList(input, sep)
	require.Len(t, tags, 2)

	assert.Equal(t, "v1.0.0", tags[0].Name)
	assert.True(t, tags[0].IsAnnotated)
	assert.Equal(t, "Release 1.0", tags[0].Message)
	assert.Equal(t, "John Doe", tags[0].Tagger)

	assert.Equal(t, "v0.1.0", tags[1].Name)
	assert.False(t, tags[1].IsAnnotated)
}

func TestParseReflog(t *testing.T) {
	const sep = "\x1e"
	input := "abc123" + sep + "commit: Initial" + sep + "HEAD@{0}" + sep + "HEAD@{0}" + sep + "2024-01-15T10:30:00Z\n" +
		"def456" + sep + "checkout: moving" + sep + "HEAD@{1}" + sep + "HEAD@{1}" + sep + "2024-01-14T09:00:00Z\n"

	entries := parseReflog(input, sep)
	require.Len(t, entries, 2)

	assert.Equal(t, "abc123", entries[0].Hash)
	assert.Equal(t, "commit: Initial", entries[0].Message)
	assert.Equal(t, "HEAD@{0}", entries[0].Action)
}

func TestParseBlame(t *testing.T) {
	input := `abc123def456789012345678901234567890abcd 1 1 1
author John Doe
author-mail <john@example.com>
author-time 1705315800
author-tz +0000
committer John Doe
committer-mail <john@example.com>
committer-time 1705315800
committer-tz +0000
summary Initial commit
filename README.md
	# Test
`

	blameLines, err := parseBlame(input)
	require.NoError(t, err)
	require.Len(t, blameLines, 1)

	assert.Equal(t, "abc123def456789012345678901234567890abcd", blameLines[0].Hash)
	assert.Equal(t, "John Doe", blameLines[0].Author)
	assert.Equal(t, 1, blameLines[0].LineNo)
	assert.Equal(t, "# Test", blameLines[0].Content)
	assert.False(t, blameLines[0].Date.IsZero())
}

// ---------------------------------------------------------------------------
// Merge operations
// ---------------------------------------------------------------------------

func TestClient_MergeFFOnly(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "ff-branch", ""))
	require.NoError(t, client.Checkout(ctx, "ff-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ff.txt"), []byte("ff\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"ff.txt"}))
	_, err = client.Commit(ctx, "ff commit", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, client.Checkout(ctx, defaultBranch))
	require.NoError(t, client.Merge(ctx, "ff-branch", MergeOpts{FFOnly: true}))

	_, err = os.Stat(filepath.Join(dir, "ff.txt"))
	assert.NoError(t, err)
}

func TestClient_MergeNoFFWithMessage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "noff-branch", ""))
	require.NoError(t, client.Checkout(ctx, "noff-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noff.txt"), []byte("noff\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"noff.txt"}))
	_, err = client.Commit(ctx, "noff commit", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, client.Checkout(ctx, defaultBranch))
	require.NoError(t, client.Merge(ctx, "noff-branch", MergeOpts{
		NoFF:    true,
		Message: "Merge noff-branch",
	}))

	commits, err := client.Log(ctx, LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.NotEmpty(t, commits)
	assert.Equal(t, "Merge noff-branch", commits[0].Subject)
}

func TestClient_MergeSquash(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "sq-branch", ""))
	require.NoError(t, client.Checkout(ctx, "sq-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sq.txt"), []byte("sq\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"sq.txt"}))
	_, err = client.Commit(ctx, "squash commit", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, client.Checkout(ctx, defaultBranch))
	require.NoError(t, client.Merge(ctx, "sq-branch", MergeOpts{Squash: true}))

	// After squash merge, changes are staged but not committed.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, statuses)
}

func TestClient_MergeValidation(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	err = client.Merge(ctx, "", MergeOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "merge branch")
}

func TestClient_MergeAbortAfterConflict(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	// Create conflicting changes on main and a branch.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("main content\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"conflict.txt"}))
	_, err = client.Commit(ctx, "main version", CommitOpts{})
	require.NoError(t, err)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 2})
	require.NoError(t, err)
	baseHash := commits[1].Hash

	require.NoError(t, client.BranchCreate(ctx, "conflict-branch", baseHash))
	require.NoError(t, client.Checkout(ctx, "conflict-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("branch content\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"conflict.txt"}))
	_, err = client.Commit(ctx, "branch version", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, client.Checkout(ctx, defaultBranch))
	err = client.Merge(ctx, "conflict-branch", MergeOpts{})
	require.Error(t, err) // Conflict expected.

	// Abort resolves the conflict state.
	require.NoError(t, client.MergeAbort(ctx))

	content, err := os.ReadFile(filepath.Join(dir, "conflict.txt"))
	require.NoError(t, err)
	assert.Equal(t, "main content\n", string(content))
}

// ---------------------------------------------------------------------------
// Rebase operations
// ---------------------------------------------------------------------------

func TestClient_Rebase(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "rebase-branch", ""))
	require.NoError(t, client.Checkout(ctx, "rebase-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rebase.txt"), []byte("rebase\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"rebase.txt"}))
	_, err = client.Commit(ctx, "rebase commit", CommitOpts{})
	require.NoError(t, err)

	require.NoError(t, client.Rebase(ctx, defaultBranch, RebaseOpts{}))
}

func TestClient_RebaseValidation(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	err = client.Rebase(ctx, "", RebaseOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rebase onto")
}

func TestClient_RebaseAbortContinueErrors(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// No rebase in progress — both should error.
	assert.Error(t, client.RebaseAbort(ctx))
	assert.Error(t, client.RebaseContinue(ctx))
}

// ---------------------------------------------------------------------------
// Cherry-pick
// ---------------------------------------------------------------------------

func TestClient_CherryPick(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "cp-branch", ""))
	require.NoError(t, client.Checkout(ctx, "cp-branch"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cp.txt"), []byte("cherry-pick\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"cp.txt"}))
	_, err = client.Commit(ctx, "cp commit", CommitOpts{})
	require.NoError(t, err)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 1})
	require.NoError(t, err)
	cpHash := commits[0].Hash

	require.NoError(t, client.Checkout(ctx, defaultBranch))
	require.NoError(t, client.CherryPick(ctx, cpHash))

	_, err = os.Stat(filepath.Join(dir, "cp.txt"))
	assert.NoError(t, err)
}

func TestClient_CherryPickValidation(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	err = client.CherryPick(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cherry-pick hash")
}

// ---------------------------------------------------------------------------
// Bisect
// ---------------------------------------------------------------------------

func TestClient_BisectLifecycle(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Create several commits for bisect to work with.
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644))
		require.NoError(t, client.Stage(ctx, []string{name}))
		_, err = client.Commit(ctx, "add "+name, CommitOpts{})
		require.NoError(t, err)
	}

	// Session 1: start + good + reset.
	require.NoError(t, client.BisectStart(ctx, "HEAD", "HEAD~5"))
	_, err = client.BisectGood(ctx)
	require.NoError(t, err)
	require.NoError(t, client.BisectReset(ctx))

	// Session 2: start + bad + reset.
	require.NoError(t, client.BisectStart(ctx, "HEAD", "HEAD~5"))
	_, err = client.BisectBad(ctx)
	require.NoError(t, err)
	require.NoError(t, client.BisectReset(ctx))
}

func TestClient_BisectValidation(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	err = client.BisectStart(ctx, "", "HEAD")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bisect bad ref")

	err = client.BisectStart(ctx, "HEAD", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bisect good ref")
}

// ---------------------------------------------------------------------------
// Stash apply, show, drop
// ---------------------------------------------------------------------------

func TestClient_StashApplyShowDrop(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Modify a tracked file and stash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Stash test\n"), 0o644))
	require.NoError(t, client.StashPush(ctx, StashOpts{Message: "test stash"}))

	// StashShow returns the diff.
	diff, err := client.StashShow(ctx, 0)
	require.NoError(t, err)
	assert.Contains(t, diff, "README.md")

	// StashApply applies without removing the stash.
	require.NoError(t, client.StashApply(ctx, 0))
	content, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Stash test\n", string(content))

	// Stash entry still exists after apply.
	entries, err := client.StashList(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// StashDrop removes without applying.
	require.NoError(t, client.StashDrop(ctx, 0))
	entries, err = client.StashList(ctx)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// ---------------------------------------------------------------------------
// Cache: branches round-trip
// ---------------------------------------------------------------------------

func TestCache_BranchesRoundTrip(t *testing.T) {
	c := NewCache(time.Second)

	// Empty cache.
	branches, fresh := c.GetBranches()
	assert.Nil(t, branches)
	assert.False(t, fresh)

	// Set and get.
	c.SetBranches([]Branch{
		{Name: "main", IsCurrent: true, Hash: "abc123"},
		{Name: "dev", Hash: "def456"},
	})
	branches, fresh = c.GetBranches()
	require.Len(t, branches, 2)
	assert.True(t, fresh)
	assert.Equal(t, "main", branches[0].Name)

	// Returned slice is a copy; mutation does not affect cache.
	branches[0].Name = "mutated"
	cached, _ := c.GetBranches()
	assert.Equal(t, "main", cached[0].Name)
}

// ---------------------------------------------------------------------------
// NewClientWithCache, RepoDir, InvalidateCache
// ---------------------------------------------------------------------------

func TestClient_NewClientWithCacheHelpers(t *testing.T) {
	dir := initTestRepo(t)
	cache := NewCache(time.Minute)
	client, err := NewClientWithCache(dir, cache)
	require.NoError(t, err)

	assert.Equal(t, dir, client.RepoDir())

	cache.SetStatus([]FileStatus{{Path: "test.txt"}})
	client.InvalidateCache()
	status, fresh := cache.GetStatus()
	assert.Nil(t, status)
	assert.False(t, fresh)

	// Empty dir should error.
	_, err = NewClientWithCache("", cache)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// TagCreate with ref
// ---------------------------------------------------------------------------

func TestTagCreate_WithRef(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 1})
	require.NoError(t, err)
	hash := commits[0].Hash

	require.NoError(t, client.TagCreate(ctx, "v0.0.1", hash, ""))

	tags, err := client.TagList(ctx)
	require.NoError(t, err)
	var found bool
	for _, tag := range tags {
		if tag.Name == "v0.0.1" {
			found = true
		}
	}
	assert.True(t, found)
}

// ---------------------------------------------------------------------------
// Parse edge cases
// ---------------------------------------------------------------------------

func TestParseTagList_EdgeCases(t *testing.T) {
	sep := "\x1e"
	assert.Empty(t, parseTagList("", sep))
	assert.Empty(t, parseTagList("short\x1eline", sep))
	assert.Empty(t, parseTagList("\n\n", sep))
}

func TestParseWorktreeList_DetachedHead(t *testing.T) {
	input := "worktree /tmp/wt\nHEAD abc123\n\n"
	wts := parseWorktreeList(input)
	require.Len(t, wts, 1)
	assert.Equal(t, filepath.FromSlash("/tmp/wt"), wts[0].Path)
	assert.Equal(t, "abc123", wts[0].Head)
	assert.Equal(t, "", wts[0].Branch)
	assert.False(t, wts[0].Bare)
}

func TestParseBlame_EmptyInput(t *testing.T) {
	lines, err := parseBlame("")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestParseReflog_EdgeCases(t *testing.T) {
	sep := "\x1e"
	assert.Empty(t, parseReflog("", sep))
	assert.Empty(t, parseReflog("abc\x1eshort", sep))
}

func TestParseStashList_BranchPrefixes(t *testing.T) {
	// "WIP on branch:" format.
	entries := parseStashList("abc\x1estash@{0}\x1eWIP on main: msg\x1e2024-01-15T12:00:00Z")
	require.Len(t, entries, 1)
	assert.Equal(t, "main", entries[0].Branch)

	// "On branch:" at start.
	entries = parseStashList("def\x1estash@{0}\x1eOn feature: stash\x1e2024-01-15T12:00:00Z")
	require.Len(t, entries, 1)
	assert.Equal(t, "feature", entries[0].Branch)
}

// ---------------------------------------------------------------------------
// OpQueue: post-lock context cancellation
// ---------------------------------------------------------------------------

func TestOpQueue_PostLockContextCancel(t *testing.T) {
	q := &OpQueue{}
	lockHeld := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = q.Exec(context.Background(), func() error {
			close(lockHeld)
			<-release
			return nil
		})
	}()
	<-lockHeld

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context while the second Exec waits for the lock,
	// then release the lock so it can proceed to the post-lock check.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
		time.Sleep(10 * time.Millisecond)
		close(release)
	}()

	err := q.Exec(ctx, func() error {
		t.Error("fn should not run when context cancelled after lock")
		return nil
	})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Remote add, list, remove
// ---------------------------------------------------------------------------

func TestClient_RemoteAddListRemove(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	// Add a remote.
	require.NoError(t, client.RemoteAdd(ctx, "upstream", "https://example.com/repo.git"))

	// List remotes.
	remotes, err := client.RemoteList(ctx)
	require.NoError(t, err)
	require.Len(t, remotes, 1)
	assert.Equal(t, "upstream", remotes[0].Name)
	assert.Contains(t, remotes[0].FetchURL, "example.com")

	// Remove the remote.
	require.NoError(t, client.RemoteRemove(ctx, "upstream"))

	remotes, err = client.RemoteList(ctx)
	require.NoError(t, err)
	assert.Empty(t, remotes)
}

// ---------------------------------------------------------------------------
// Worktree add and remove
// ---------------------------------------------------------------------------

func TestClient_WorktreeAddRemove(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	require.NoError(t, client.BranchCreate(ctx, "wt-branch", ""))

	wtDir := filepath.ToSlash(filepath.Join(t.TempDir(), "my-worktree"))
	require.NoError(t, client.WorktreeAdd(ctx, wtDir, "wt-branch"))

	// Verify worktree exists in the list.
	wts, err := client.WorktreeList(ctx)
	require.NoError(t, err)
	var found bool
	for _, wt := range wts {
		if wt.Branch == "wt-branch" {
			found = true
		}
	}
	assert.True(t, found)

	// Remove the worktree.
	require.NoError(t, client.WorktreeRemove(ctx, wtDir, false))
}
