package keybindings

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSections_NotEmpty(t *testing.T) {
	secs := Sections()
	require.NotEmpty(t, secs, "should have at least one section")
}

func TestSections_AllHaveTitles(t *testing.T) {
	for _, sec := range Sections() {
		assert.NotEmpty(t, sec.Title, "section %q should have a title", sec.ID)
		assert.NotEmpty(t, sec.ID, "section with title %q should have an ID", sec.Title)
	}
}

func TestSections_AllHaveBindings(t *testing.T) {
	for _, sec := range Sections() {
		assert.NotEmpty(t, sec.Bindings, "section %q should have bindings", sec.Title)
	}
}

func TestSections_ExpectedSections(t *testing.T) {
	secs := Sections()
	ids := make([]string, len(secs))
	for i, s := range secs {
		ids[i] = s.ID
	}
	expected := []string{"global", "navigation", "filetree", "gitstatus", "gitinfo", "github", "commits", "gitdiff", "preview", "edit_mode"}
	assert.Equal(t, expected, ids)
}

func TestSections_NoDuplicateIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, sec := range Sections() {
		assert.False(t, seen[sec.ID], "duplicate section ID: %s", sec.ID)
		seen[sec.ID] = true
	}
}

func TestSections_BindingsHaveKeyAndAction(t *testing.T) {
	for _, sec := range Sections() {
		for i, b := range sec.Bindings {
			assert.NotEmpty(t, b.Key, "section %q binding %d has empty key", sec.Title, i)
			assert.NotEmpty(t, b.Action, "section %q binding %d has empty action", sec.Title, i)
		}
	}
}

func TestGenerateMarkdown_NotEmpty(t *testing.T) {
	md := GenerateMarkdown()
	assert.Contains(t, md, "# Keybindings Reference")
	assert.Contains(t, md, "## Global")
	assert.Contains(t, md, "## Navigation")
	assert.Contains(t, md, "## Preview")
}

// repoRoot walks up from the test file to find the repository root (contains go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func TestKeybindingsMarkdown_UpToDate(t *testing.T) {
	root := repoRoot(t)
	mdPath := filepath.Join(root, "docs", "keybindings.md")
	existing, err := os.ReadFile(mdPath)
	require.NoError(t, err, "could not read docs/keybindings.md")

	generated := GenerateMarkdown()
	if string(existing) != generated {
		t.Error("docs/keybindings.md is out of date with internal/keybindings/keybindings.json.\n" +
			"Run: go run ./internal/keybindings/cmd/genmd")
	}
}
