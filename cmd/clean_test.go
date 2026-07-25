package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedCleanData creates a data dir populated with the transient targets clean
// removes plus the excluded entries it must preserve. It returns the data dir
// path.
func seedCleanData(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()

	// Targets clean should remove.
	writeFile(t, filepath.Join(dataDir, "sessions", "abc", "state.json"), "session")
	writeFile(t, filepath.Join(dataDir, "diagnostics", "watchdog.log"), "diag")

	// Entries clean must leave alone.
	writeFile(t, filepath.Join(dataDir, "crashes", "crash-1.txt"), "crash")
	writeFile(t, filepath.Join(dataDir, "extensions", "ext", "main.lua"), "ext")
	writeFile(t, filepath.Join(dataDir, "mcp-audit.log"), "audit")
	writeFile(t, filepath.Join(dataDir, ".first-run-done"), "")

	return dataDir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func execClean(t *testing.T, dataDir string, args ...string) string {
	t.Helper()
	cmd := newCleanCmdWithDeps(func() string { return dataDir })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestCleanPreviewListsTargetsWithoutDeleting(t *testing.T) {
	dataDir := seedCleanData(t)

	out := execClean(t, dataDir)

	assert.Contains(t, out, "sessions")
	assert.Contains(t, out, "diagnostics")
	assert.Contains(t, out, "Run with --force to delete.")

	// Preview must not delete anything.
	assert.DirExists(t, filepath.Join(dataDir, "sessions"))
	assert.DirExists(t, filepath.Join(dataDir, "diagnostics"))
}

func TestCleanForceRemovesTargets(t *testing.T) {
	dataDir := seedCleanData(t)

	out := execClean(t, dataDir, "--force")

	assert.Contains(t, out, "removed")
	assert.Contains(t, out, "Reclaimed")
	assert.NoDirExists(t, filepath.Join(dataDir, "sessions"))
	assert.NoDirExists(t, filepath.Join(dataDir, "diagnostics"))
}

func TestCleanForcePreservesExcludedEntries(t *testing.T) {
	dataDir := seedCleanData(t)

	execClean(t, dataDir, "--force")

	assert.FileExists(t, filepath.Join(dataDir, "crashes", "crash-1.txt"))
	assert.FileExists(t, filepath.Join(dataDir, "extensions", "ext", "main.lua"))
	assert.FileExists(t, filepath.Join(dataDir, "mcp-audit.log"))
	assert.FileExists(t, filepath.Join(dataDir, ".first-run-done"))
}

func TestCleanMissingDirsPreview(t *testing.T) {
	dataDir := t.TempDir() // nothing seeded

	out := execClean(t, dataDir)

	assert.Contains(t, out, "not present")
	assert.Contains(t, out, "Nothing to clean.")
}

func TestCleanMissingDirsForce(t *testing.T) {
	dataDir := t.TempDir()

	out := execClean(t, dataDir, "--force")

	assert.Contains(t, out, "Reclaimed 0 B.")
}

func TestCleanJSONPreviewReportsTargetsWithoutDeleting(t *testing.T) {
	dataDir := seedCleanData(t)

	out := execClean(t, dataDir, "--json")

	var report cleanReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.False(t, report.Forced)
	assert.Equal(t, int64(len("session")+len("diag")), report.TotalBytes)
	assert.Equal(t, 2, report.PresentCount)
	require.Len(t, report.Targets, 2)
	assert.Equal(t, "sessions", report.Targets[0].Label)
	assert.True(t, report.Targets[0].Exists)
	assert.False(t, report.Targets[0].Removed)
	assert.DirExists(t, filepath.Join(dataDir, "sessions"))
	assert.DirExists(t, filepath.Join(dataDir, "diagnostics"))
}

func TestCleanJSONForceReportsRemovedTargets(t *testing.T) {
	dataDir := seedCleanData(t)

	out := execClean(t, dataDir, "--force", "--json")

	var report cleanReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.Forced)
	assert.Equal(t, 2, report.PresentCount)
	for _, target := range report.Targets {
		assert.True(t, target.Exists)
		assert.True(t, target.Removed)
	}
	assert.NoDirExists(t, filepath.Join(dataDir, "sessions"))
	assert.NoDirExists(t, filepath.Join(dataDir, "diagnostics"))
}

func TestCleanJSONMissingDirs(t *testing.T) {
	dataDir := t.TempDir()

	out := execClean(t, dataDir, "--json")

	var report cleanReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, int64(0), report.TotalBytes)
	assert.Equal(t, 0, report.PresentCount)
	require.Len(t, report.Targets, 2)
	for _, target := range report.Targets {
		assert.False(t, target.Exists)
		assert.False(t, target.Removed)
	}
}

func TestCleanRejectsArgs(t *testing.T) {
	cmd := newCleanCmdWithDeps(func() string { return t.TempDir() })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"unexpected"})

	require.Error(t, cmd.Execute())
}

func TestRootRegistersClean(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	cleanCmd, _, err := root.Find([]string{"clean"})
	require.NoError(t, err)
	require.NotNil(t, cleanCmd)
	assert.Equal(t, "clean", cleanCmd.Name())
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")        // 5 bytes
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "world") // 5 bytes

	size, exists, err := dirSize(dir)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, int64(10), size)

	_, missing, err := dirSize(filepath.Join(dir, "nope"))
	require.NoError(t, err)
	assert.False(t, missing)
}

func TestHumanizeBytes(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B",
		512:             "512 B",
		1024:            "1.0 KiB",
		1536:            "1.5 KiB",
		1024 * 1024:     "1.0 MiB",
		5 * 1024 * 1024: "5.0 MiB",
	}
	for in, want := range cases {
		assert.Equal(t, want, humanizeBytes(in))
	}
}
