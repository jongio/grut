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

// ---------------------------------------------------------------------------
// Integration: runStatus against a real temporary repository.
// ---------------------------------------------------------------------------

func TestRunStatus_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := newStatusCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := runStatus(cmd, nil)
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
