package demo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/proctree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenarioMetadata(t *testing.T) {
	scenarios := Scenarios()
	require.Len(t, scenarios, 3)

	names := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		names = append(names, scenario.Name)
		assert.NotEmpty(t, scenario.Description, scenario.Name)
		assert.NotEmpty(t, scenario.Layout, scenario.Name)
		assert.NotEmpty(t, scenario.FocusPanel, scenario.Name)
		assert.NotEmpty(t, scenario.Guide, scenario.Name)
	}
	assert.Equal(t, []string{"branch-review", "conflict-resolution", "extensions"}, names)

	list := FormatScenarioList()
	for _, name := range names {
		assert.Contains(t, list, name)
	}
}

func TestSetupProjectWithScenarioDeterministicState(t *testing.T) {
	setup, cleanup, err := SetupProjectWithOptions(SetupOptions{Scenario: "branch-review"})
	require.NoError(t, err)
	defer cleanup()

	require.NotNil(t, setup.Scenario)
	assert.Equal(t, "branch-review", setup.Scenario.Name)
	assert.Equal(t, "explorer", setup.Scenario.Layout)
	assert.Equal(t, "gitinfo", setup.Scenario.FocusPanel)
	assert.FileExists(t, setup.GuidePath)

	branches := gitOutput(t, setup.Dir, "branch", "--list", "--format=%(refname:short)")
	for _, branch := range []string{"main", "develop", "feature/websocket-support", "feature/api-v2", "fix/rate-limit-bypass"} {
		assert.Contains(t, branches, branch)
	}

	assert.Equal(t, "16", strings.TrimSpace(gitOutput(t, setup.Dir, "rev-list", "--count", "main")))

	stashes := gitOutput(t, setup.Dir, "stash", "list")
	assert.Contains(t, stashes, "WIP: health check v2")
	assert.Contains(t, stashes, "WIP: user validation logic")
	assert.Contains(t, stashes, "WIP: API rate limit config")
	assert.Len(t, strings.Split(strings.TrimSpace(stashes), "\n"), 3)
}

func TestSetupProjectWithConflictScenario(t *testing.T) {
	setup, cleanup, err := SetupProjectWithOptions(SetupOptions{Scenario: "conflict-resolution"})
	require.NoError(t, err)
	defer cleanup()

	require.NotNil(t, setup.Scenario)
	assert.Equal(t, "conflict-resolution", setup.Scenario.Name)
	assert.Equal(t, "full", setup.Scenario.Layout)
	assert.Equal(t, "gitstatus", setup.Scenario.FocusPanel)
	assert.FileExists(t, setup.GuidePath)

	status := gitOutput(t, setup.Dir, "status", "--porcelain")
	assert.Contains(t, status, "UU src/middleware/ratelimit.go")
}

func TestSetupProjectWithUnknownScenarioErrors(t *testing.T) {
	_, _, err := SetupProjectWithOptions(SetupOptions{Scenario: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo scenario")
}

func TestSetupProjectWithKeepSkipsCleanup(t *testing.T) {
	setup, cleanup, err := SetupProjectWithOptions(SetupOptions{Scenario: "extensions", Keep: true})
	require.NoError(t, err)
	cleanup()
	assert.DirExists(t, setup.Dir)
	require.NoError(t, os.RemoveAll(filepath.Clean(setup.Dir)))
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := proctree.Command(context.Background(), "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	require.NoError(t, proctree.Run(cmd), out.String())
	return out.String()
}
