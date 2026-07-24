package textsearch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withResults(ts *TextSearch) {
	ts.results = []Result{
		{Path: "first.go", Line: 10, Snippet: "first match"},
		{Path: "second.go", Line: 20, Snippet: "second match"},
	}
	ts.query = "needle"
	ts.lastRun = "needle"
	ts.status = "2 results"
}

func TestInit(t *testing.T) {
	assert.Nil(t, New(".", nil).Init(context.Background()))
}

func TestKeyBindings(t *testing.T) {
	kb := New(".", nil).KeyBindings()
	require.Len(t, kb, 4)
	actions := make([]string, len(kb))
	for i, b := range kb {
		actions[i] = b.Action
	}
	assert.Equal(t, []string{"search_select", "cursor_up", "cursor_down", "close"}, actions)
}

func TestView(t *testing.T) {
	ts := New(".", nil)
	withResults(ts)
	out := ts.View(80, 20)
	assert.Contains(t, out, "first.go")
	assert.Contains(t, out, "second.go")
	assert.Contains(t, out, "2 results")

	// Non-positive dimensions render nothing.
	assert.Empty(t, ts.View(0, 10))
	assert.Empty(t, ts.View(10, 0))
}

func TestHandleKeyTyping(t *testing.T) {
	ts := New(".", nil)
	for _, r := range "abc" {
		ts.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	assert.Equal(t, "abc", ts.query)
	assert.Equal(t, 3, ts.qCursor)
	assert.Contains(t, ts.status, "Press Enter to search")
}

func TestHandleKeyBackspaceAndClear(t *testing.T) {
	ts := New(".", nil)
	ts.query = "abc"
	ts.qCursor = 3

	ts.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "ab", ts.query)
	assert.Equal(t, 2, ts.qCursor)

	// ctrl+u clears the whole query.
	ts.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Empty(t, ts.query)
	assert.Equal(t, 0, ts.qCursor)
}

func TestHandleKeyNavigation(t *testing.T) {
	ts := New(".", nil)
	withResults(ts)
	assert.Equal(t, 0, ts.cursor)

	ts.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, ts.cursor)

	// Cannot move past the last result.
	ts.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, ts.cursor)

	ts.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, ts.cursor)

	// Cannot move above the first result.
	ts.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, ts.cursor)
}

func TestHandleKeyEscapeClosesPanel(t *testing.T) {
	_, cmd := New(".", nil).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.ToggleTextSearchMsg)
	assert.True(t, ok, "escape should emit ToggleTextSearchMsg")
}

func TestHandleMouseClickSetsCursor(t *testing.T) {
	ts := New(".", nil)
	withResults(ts)
	// ContentRow 3 maps to result index 1 (offset 0 + 3 - 2).
	ts.Update(panels.PanelMouseClickMsg{ContentRow: 3})
	assert.Equal(t, 1, ts.cursor)

	// Out-of-range rows are ignored.
	ts.Update(panels.PanelMouseClickMsg{ContentRow: 99})
	assert.Equal(t, 1, ts.cursor)
}

func TestHandleMouseDoubleClickSelects(t *testing.T) {
	ts := New(".", nil)
	withResults(ts)
	_, cmd := ts.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 2})
	require.NotNil(t, cmd)
	got, ok := cmd().(panels.FileSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "first.go", got.Path)
	assert.Equal(t, 10, got.Line)
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
	run("init")
	return dir
}

func TestRunSearchFindsMatch(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "file.go"),
		[]byte("package main\n// needle is here\n"),
		0o644,
	))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	ts := New(dir, nil)
	results, omitted, err := ts.runSearch(context.Background(), "needle", DefaultMaxResults)
	require.NoError(t, err)
	assert.False(t, omitted)
	require.Len(t, results, 1)
	assert.Equal(t, "file.go", results[0].Path)
	assert.Equal(t, 2, results[0].Line)
	assert.Contains(t, results[0].Snippet, "needle")
}

func TestRunSearchNoMatchReturnsEmpty(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello world"), 0o644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	ts := New(dir, nil)
	// git grep exits 1 on no match; runSearch must translate that to an empty,
	// error-free result rather than a failure.
	results, omitted, err := ts.runSearch(context.Background(), "zzz-no-such-token", DefaultMaxResults)
	require.NoError(t, err)
	assert.False(t, omitted)
	assert.Empty(t, results)
}
