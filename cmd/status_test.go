package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStatusCmd_Wiring(t *testing.T) {
	cmd := newStatusCmd()
	assert.Equal(t, "status", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	require.NotNil(t, cmd.Flags().Lookup("json"), "--json flag should be registered")
	require.NotNil(t, cmd.Flags().Lookup("check"), "--check flag should be registered")
	require.NotNil(t, cmd.Flags().Lookup("short"), "--short flag should be registered")
}

func TestBuildStatusReport_Clean(t *testing.T) {
	report := buildStatusReport(git.StatusBranch{Head: "main", Upstream: "origin/main"}, nil)
	assert.True(t, report.Clean)
	assert.False(t, report.Detached)
	assert.Equal(t, "main", report.Branch)
	assert.Equal(t, "origin/main", report.Upstream)
	assert.Equal(t, 0, report.Counts.Staged)
	assert.Equal(t, 0, report.Counts.Unstaged)
	// Slices are non-nil so JSON renders [] rather than null.
	assert.NotNil(t, report.Staged)
	assert.NotNil(t, report.Untracked)
}

func TestBuildStatusReport_Detached(t *testing.T) {
	for _, head := range []string{"", "(detached)"} {
		report := buildStatusReport(git.StatusBranch{Head: head}, nil)
		assert.True(t, report.Detached, "head %q should be detached", head)
	}
}

func TestBuildStatusReport_Classification(t *testing.T) {
	files := []git.FileStatus{
		{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		{Path: "both.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusModified},
		{Path: "new.txt", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		{Path: "conflict.txt", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
	}
	report := buildStatusReport(git.StatusBranch{Head: "main"}, files)

	assert.False(t, report.Clean)
	// staged.go and both.go are staged.
	assert.Equal(t, 2, report.Counts.Staged)
	// unstaged.go and both.go are unstaged.
	assert.Equal(t, 2, report.Counts.Unstaged)
	assert.Equal(t, 1, report.Counts.Untracked)
	assert.Equal(t, 1, report.Counts.Conflicted)
	assert.Equal(t, []string{"new.txt"}, report.Untracked)
	assert.Equal(t, []string{"conflict.txt"}, report.Conflicted)

	stagedPaths := entryPaths(report.Staged)
	assert.ElementsMatch(t, []string{"staged.go", "both.go"}, stagedPaths)
	unstagedPaths := entryPaths(report.Unstaged)
	assert.ElementsMatch(t, []string{"unstaged.go", "both.go"}, unstagedPaths)
}

func entryPaths(entries []statusEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}

func TestWriteStatusText_Clean(t *testing.T) {
	var buf bytes.Buffer
	writeStatusText(&buf, buildStatusReport(git.StatusBranch{Head: "main", Upstream: "origin/main", Ahead: 2}, nil))
	out := buf.String()
	assert.Contains(t, out, "On branch main")
	assert.Contains(t, out, "Tracking origin/main [ahead 2]")
	assert.Contains(t, out, "Working tree clean")
}

func TestWriteStatusText_Sections(t *testing.T) {
	files := []git.FileStatus{
		{Path: "a.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		{Path: "b.txt", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
	}
	var buf bytes.Buffer
	writeStatusText(&buf, buildStatusReport(git.StatusBranch{Head: "main"}, files))
	out := buf.String()
	assert.Contains(t, out, "Staged (1):")
	assert.Contains(t, out, "  M a.go")
	assert.Contains(t, out, "Untracked (1):")
	assert.Contains(t, out, "  ? b.txt")
	assert.NotContains(t, out, "Working tree clean")
}

func TestWriteStatusText_DetachedHead(t *testing.T) {
	var buf bytes.Buffer
	writeStatusText(&buf, buildStatusReport(git.StatusBranch{Head: "(detached)"}, nil))
	assert.Contains(t, buf.String(), "HEAD detached")
}

func TestWriteStatusShort_Clean(t *testing.T) {
	var buf bytes.Buffer
	writeStatusShort(&buf, git.StatusBranch{Head: "main", Upstream: "origin/main", Ahead: 2}, nil, true)
	assert.Equal(t, "## main...origin/main [ahead 2] clean\n", buf.String())
}

func TestWriteStatusShort_CompactFiles(t *testing.T) {
	files := []git.FileStatus{
		{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified},
		{Path: "both.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusModified},
		{Path: "new.txt", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
		{Path: "conflict.txt", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		{Path: "new-name.go", OrigPath: "old-name.go", StagedStatus: git.StatusRenamed, WorktreeStatus: git.StatusUnmodified},
	}

	var buf bytes.Buffer
	writeStatusShort(&buf, git.StatusBranch{Head: "main"}, files, false)

	assert.Equal(t, strings.Join([]string{
		"## main",
		"M  staged.go",
		" M unstaged.go",
		"MM both.go",
		"?? new.txt",
		"UU conflict.txt",
		"R  old-name.go -> new-name.go",
		"",
	}, "\n"), buf.String())
}

func TestWriteStatusJSON_RoundTrip(t *testing.T) {
	files := []git.FileStatus{
		{Path: "a.go", StagedStatus: git.StatusAdded, WorktreeStatus: git.StatusUnmodified},
	}
	report := buildStatusReport(git.StatusBranch{Head: "main", Upstream: "origin/main", Behind: 3}, files)

	var buf bytes.Buffer
	require.NoError(t, writeStatusJSON(&buf, report))

	var decoded statusReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "main", decoded.Branch)
	assert.Equal(t, 3, decoded.Behind)
	assert.Equal(t, 1, decoded.Counts.Staged)
	require.Len(t, decoded.Staged, 1)
	assert.Equal(t, "a.go", decoded.Staged[0].Path)
	assert.Equal(t, "A", decoded.Staged[0].Status)
}

func TestRunStatus_CheckCleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("hi"), 0o644))
	runGit(t, dir, "add", "tracked.txt")
	runGit(t, dir, "commit", "-m", "initial")

	t.Chdir(dir)
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Flags().Set("check", "true"))

	require.NoError(t, runStatus(cmd, nil))
	assert.Contains(t, out.String(), "Working tree clean")
}

func TestRunStatus_CheckDirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("hi"), 0o644))

	t.Chdir(dir)
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Flags().Set("check", "true"))

	err := runStatus(cmd, nil)
	require.ErrorIs(t, err, errStatusDirty)
	assert.Contains(t, out.String(), "Untracked (1):")
}

func TestRunStatus_ShortCleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("hi"), 0o644))
	runGit(t, dir, "add", "tracked.txt")
	runGit(t, dir, "commit", "-m", "initial")

	t.Chdir(dir)
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--short"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "## main clean\n", out.String())
}

func TestRunStatus_ShortCheckDirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("hi"), 0o644))

	t.Chdir(dir)
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--short", "--check"})

	err := cmd.Execute()
	require.ErrorIs(t, err, errStatusDirty)
	assert.Equal(t, "## main\n?? loose.txt\n", out.String())
}

func TestRunStatus_ShortJSONMutualExclusion(t *testing.T) {
	cmd := newStatusCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--short", "--json"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--short cannot be combined with --json")
}

// ---------------------------------------------------------------------------
// Integration: runStatus against a real temporary repository.
// ---------------------------------------------------------------------------

func TestRunStatus_NotARepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "grut-status-nonrepo-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte(""), 0o644))
	t.Chdir(dir)

	cmd := newStatusCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = runStatus(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRunStatus_RealRepo_JSON(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("bye"), 0o644))
	runGit(t, dir, "add", "staged.txt")

	t.Chdir(dir)
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, runStatus(cmd, nil))

	var report statusReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, "main", report.Branch)
	assert.False(t, report.Clean)
	assert.Equal(t, 1, report.Counts.Staged)
	assert.Equal(t, []string{"loose.txt"}, report.Untracked)
	assert.Equal(t, "staged.txt", report.Staged[0].Path)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
