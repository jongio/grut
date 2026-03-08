package gitlog

import (
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commit(hash string, parents ...string) git.Commit {
	return git.Commit{Hash: hash, ShortHash: hash[:4], Parents: parents}
}

func TestGraphRenderer_LinearHistory(t *testing.T) {
	g := NewGraphRenderer()

	e1 := g.RenderCommit(commit("aaaa", "bbbb"))
	e2 := g.RenderCommit(commit("bbbb", "cccc"))
	e3 := g.RenderCommit(commit("cccc", "dddd"))

	assert.Equal(t, "*", e1.Prefix)
	assert.Empty(t, e1.Connectors)

	assert.Equal(t, "*", e2.Prefix)
	assert.Empty(t, e2.Connectors)

	assert.Equal(t, "*", e3.Prefix)
	assert.Empty(t, e3.Connectors)
}

func TestGraphRenderer_RootCommit(t *testing.T) {
	g := NewGraphRenderer()

	e1 := g.RenderCommit(commit("aaaa", "bbbb"))
	e2 := g.RenderCommit(commit("bbbb")) // root commit, no parents

	assert.Equal(t, "*", e1.Prefix)
	assert.Equal(t, "*", e2.Prefix)
	assert.Empty(t, e2.Connectors)
}

func TestGraphRenderer_MergeCommit(t *testing.T) {
	g := NewGraphRenderer()

	// Merge commit with two parents
	e1 := g.RenderCommit(commit("mmmm", "aaaa", "bbbb"))

	assert.Equal(t, "*", e1.Prefix)
	require.Len(t, e1.Connectors, 1)
	assert.Equal(t, "|\\", e1.Connectors[0])
}

func TestGraphRenderer_BranchAndMerge(t *testing.T) {
	g := NewGraphRenderer()

	// Linear commits on main
	e1 := g.RenderCommit(commit("aaaa", "bbbb"))
	assert.Equal(t, "*", e1.Prefix)
	assert.Empty(t, e1.Connectors)

	e2 := g.RenderCommit(commit("bbbb", "dddd"))
	assert.Equal(t, "*", e2.Prefix)
	assert.Empty(t, e2.Connectors)

	// Branch head appears (from --all)
	e3 := g.RenderCommit(commit("cccc", "dddd"))
	assert.Equal(t, "| *", e3.Prefix)
	// cccc's parent dddd matches column 0 → converge connector
	require.Len(t, e3.Connectors, 1)
	assert.Equal(t, "|/", e3.Connectors[0])

	// Common ancestor
	e4 := g.RenderCommit(commit("dddd", "eeee"))
	assert.Equal(t, "*", e4.Prefix)
	assert.Empty(t, e4.Connectors)
}

func TestGraphRenderer_FullMergeFlow(t *testing.T) {
	// Simulates:
	// * merge (parents: A, B)
	// |\
	// * | A (parent: C)
	// | * B (parent: C)
	// |/
	// * C
	g := NewGraphRenderer()

	e1 := g.RenderCommit(commit("MERG", "AAAA", "BBBB"))
	assert.Equal(t, "*", e1.Prefix)
	require.Len(t, e1.Connectors, 1)
	assert.Equal(t, "|\\", e1.Connectors[0])

	e2 := g.RenderCommit(commit("AAAA", "CCCC"))
	assert.Equal(t, "* |", e2.Prefix)
	assert.Empty(t, e2.Connectors)

	e3 := g.RenderCommit(commit("BBBB", "CCCC"))
	assert.Equal(t, "| *", e3.Prefix)
	require.Len(t, e3.Connectors, 1)
	assert.Equal(t, "|/", e3.Connectors[0])

	e4 := g.RenderCommit(commit("CCCC", "DDDD"))
	assert.Equal(t, "*", e4.Prefix)
	assert.Empty(t, e4.Connectors)
}

func TestGraphRenderer_EmptyCommit(t *testing.T) {
	g := NewGraphRenderer()

	// Single root commit
	e := g.RenderCommit(commit("aaaa"))
	assert.Equal(t, "*", e.Prefix)
	assert.Empty(t, e.Connectors)
}

func TestGraphRenderer_MultipleRootBranches(t *testing.T) {
	g := NewGraphRenderer()

	// First branch tip
	e1 := g.RenderCommit(commit("aaaa", "cccc"))
	assert.Equal(t, "*", e1.Prefix)

	// Second branch tip (not a child of aaaa)
	e2 := g.RenderCommit(commit("bbbb", "cccc"))
	assert.Equal(t, "| *", e2.Prefix)
	// bbbb's parent cccc matches column 0 → converge
	require.Len(t, e2.Connectors, 1)
	assert.Equal(t, "|/", e2.Connectors[0])

	// Common ancestor
	e3 := g.RenderCommit(commit("cccc", "dddd"))
	assert.Equal(t, "*", e3.Prefix)
}

func TestGraphRenderer_ThreeParentMerge(t *testing.T) {
	g := NewGraphRenderer()

	// Octopus merge with three parents
	e := g.RenderCommit(commit("mmmm", "aaaa", "bbbb", "cccc"))
	assert.Equal(t, "*", e.Prefix)
	require.Len(t, e.Connectors, 2)
	assert.Equal(t, "|\\", e.Connectors[0])
	// Non-adjacent connector: col 0 to col 2, char at col 2 position.
	assert.Equal(t, "| | \\", e.Connectors[1])
}

func TestGraphRenderer_ParentAlreadyInLane(t *testing.T) {
	g := NewGraphRenderer()

	// Merge where second parent is already tracked
	e1 := g.RenderCommit(commit("aaaa", "cccc"))
	assert.Equal(t, "*", e1.Prefix)

	// New branch that also points to cccc
	e2 := g.RenderCommit(commit("bbbb", "cccc"))
	assert.Equal(t, "| *", e2.Prefix)
	// Converge
	require.Len(t, e2.Connectors, 1)
	assert.Equal(t, "|/", e2.Connectors[0])

	// Merge commit whose second parent is already the only lane
	g2 := NewGraphRenderer()
	e3 := g2.RenderCommit(commit("xxxx", "yyyy"))
	assert.Equal(t, "*", e3.Prefix)

	// A merge commit where one parent is already active
	e4 := g2.RenderCommit(commit("yyyy", "zzzz", "yyyy"))
	assert.Equal(t, "*", e4.Prefix)
	// Second parent "yyyy" is the hash we just consumed — findIndex returns -1
	// because the column was updated to "zzzz". So it allocates a new lane.
}

func TestConnectorLine_AdjacentColumns(t *testing.T) {
	g := &GraphRenderer{cols: []string{"a", "b"}}

	line := g.connectorLine(0, 1, '\\')
	assert.Equal(t, "|\\", line)

	line = g.connectorLine(0, 1, '/')
	assert.Equal(t, "|/", line)
}

func TestConnectorLine_NonAdjacentColumns(t *testing.T) {
	g := &GraphRenderer{cols: []string{"a", "b", "c"}}

	line := g.connectorLine(0, 2, '\\')
	assert.Equal(t, "| | \\", line)
}
