package gitdiff

import (
	"context"
	"sync"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureDiffOpts returns a mock client that records the DiffOpts of the most
// recent Diff call. Access is guarded because the diff closure runs in a
// Bubble Tea goroutine.
func captureDiffOpts() (*mockGitClient, func() git.DiffOpts) {
	var (
		mu   sync.Mutex
		last git.DiffOpts
	)
	m := &mockGitClient{
		DiffFunc: func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
			mu.Lock()
			last = opts
			mu.Unlock()
			return []git.FileDiff{sampleDiff()}, nil
		},
	}
	return m, func() git.DiffOpts {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

func TestToggleIgnoreWhitespaceSetsDiffOpt(t *testing.T) {
	mock, lastOpts := captureDiffOpts()
	p := New(mock, nil)
	p.Init(context.Background())
	p.SetSize(80, 24)
	p.Focus()

	// Load a working-tree diff so a path is set.
	_, cmd := p.Update(panels.FileSelectedMsg{Path: "file.go"})
	require.NotNil(t, cmd)
	p.Update(cmd())
	assert.False(t, lastOpts().IgnoreAll, "ignore-whitespace should be off by default")

	// Toggle on: re-runs the diff with IgnoreAll set.
	_, cmd = p.Update(keyMsg("W"))
	require.NotNil(t, cmd, "toggling whitespace should re-run the diff")
	assert.True(t, p.ignoreWhitespace)
	p.Update(cmd())
	assert.True(t, lastOpts().IgnoreAll, "IgnoreAll should be set when active")

	// Toggle off: re-runs the diff without IgnoreAll.
	_, cmd = p.Update(keyMsg("W"))
	require.NotNil(t, cmd)
	assert.False(t, p.ignoreWhitespace)
	p.Update(cmd())
	assert.False(t, lastOpts().IgnoreAll, "IgnoreAll should be cleared when inactive")
}

func TestToggleIgnoreWhitespaceInCompareMode(t *testing.T) {
	mock, lastOpts := captureDiffOpts()
	p := New(mock, nil)
	p.Init(context.Background())
	p.SetSize(80, 24)
	p.Focus()

	// Enter compare mode (branch/PR diff) with a path.
	_, cmd := p.Update(panels.ShowDiffMsg{Path: "file.go", CommitA: "main", CommitB: "HEAD", ThreeDot: true})
	require.NotNil(t, cmd)
	p.Update(cmd())
	require.True(t, p.compareMode)

	_, cmd = p.Update(keyMsg("W"))
	require.NotNil(t, cmd)
	p.Update(cmd())

	opts := lastOpts()
	assert.True(t, opts.IgnoreAll, "IgnoreAll should be set for compare-mode diffs")
	assert.Equal(t, "main", opts.CommitA, "compare refs should be preserved")
	assert.Equal(t, "HEAD", opts.CommitB)
	assert.True(t, opts.ThreeDot)
}

func TestToggleIgnoreWhitespaceNoPathIsNoOp(t *testing.T) {
	p := New(nil, nil)
	p.Init(context.Background())
	p.Focus()

	_, cmd := p.Update(keyMsg("W"))
	assert.True(t, p.ignoreWhitespace, "flag should still flip without a path")
	assert.Nil(t, cmd, "no diff should be issued when no file is selected")
}

func TestTitleShowsIgnoreWhitespaceIndicator(t *testing.T) {
	p := New(nil, nil)
	p.path = "/path/to/file.go"
	p.ignoreWhitespace = true
	assert.Equal(t, "file.go [ignore ws]", p.Title())

	p.staged = true
	assert.Equal(t, "file.go (staged) [ignore ws]", p.Title())
}

func TestIgnoreWhitespaceKeybindingPresent(t *testing.T) {
	p := New(nil, nil)
	found := false
	for _, b := range p.KeyBindings() {
		if b.Action == "toggle_whitespace" {
			found = true
		}
	}
	assert.True(t, found, "should expose a toggle_whitespace binding")
}
