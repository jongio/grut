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

func TestLog_Pickaxe(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = testGitEnv()
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	const token = "ZZ_UNIQUE_TOKEN"

	// Commit that introduces the token.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.go"),
		[]byte("package main\n\nconst apiToken = \""+token+"\"\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "add token")

	// Commit that removes it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.go"),
		[]byte("package main\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "remove token")

	// Unrelated commit that never touches the token.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"),
		[]byte("hello\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "other")

	client, err := NewClient(dir)
	require.NoError(t, err)

	// -S finds only the commits that changed the occurrence count of the token.
	got, err := client.Log(ctx, LogOpts{Pickaxe: token})
	require.NoError(t, err)
	subjects := make([]string, len(got))
	for i, c := range got {
		subjects[i] = c.Subject
	}
	assert.ElementsMatch(t, []string{"add token", "remove token"}, subjects)

	// -G matches the diff against a regular expression.
	gotG, err := client.Log(ctx, LogOpts{Pickaxe: "ZZ_.*_TOKEN", PickaxeRegex: true})
	require.NoError(t, err)
	assert.Len(t, gotG, 2)

	// Scoping to an unrelated path yields no pickaxe matches.
	gotScoped, err := client.Log(ctx, LogOpts{Pickaxe: token, Path: "other.txt"})
	require.NoError(t, err)
	assert.Empty(t, gotScoped)

	// A term starting with a dash is rejected before reaching git.
	_, err = client.Log(ctx, LogOpts{Pickaxe: "-x"})
	require.Error(t, err)
}
