package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBranchList(t *testing.T) {
	input := "main\x1eabc123\x1eorigin/main\x1e[ahead 2, behind 1]\x1e*\x1e\x1erefs/heads/main\n" +
		"feature/auth\x1edef456\x1e\x1e\x1e \x1e\x1erefs/heads/feature/auth\n" +
		"origin/main\x1eghi789\x1e\x1e\x1e \x1e\x1erefs/remotes/origin/main\n"

	branches, err := parseBranchList(input)
	require.NoError(t, err)
	require.Len(t, branches, 3)

	// main — current branch with upstream tracking
	assert.Equal(t, "main", branches[0].Name)
	assert.Equal(t, "abc123", branches[0].Hash)
	assert.Equal(t, "origin/main", branches[0].Upstream)
	assert.True(t, branches[0].IsCurrent)
	assert.Equal(t, 2, branches[0].Ahead)
	assert.Equal(t, 1, branches[0].Behind)

	// feature/auth — local branch, no upstream
	assert.Equal(t, "feature/auth", branches[1].Name)
	assert.False(t, branches[1].IsCurrent)
	assert.Equal(t, 0, branches[1].Ahead)
	assert.Equal(t, 0, branches[1].Behind)

	// origin/main — remote branch
	assert.Equal(t, "origin/main", branches[2].Name)
	assert.True(t, branches[2].IsRemote)
}

func TestParseBranchList_Empty(t *testing.T) {
	branches, err := parseBranchList("")
	require.NoError(t, err)
	assert.Empty(t, branches)
}

func TestParseBranchList_SkipsSymrefs(t *testing.T) {
	// origin/HEAD is a symref pointing to origin/main — should be filtered out.
	input := "main\x1eabc123\x1eorigin/main\x1e\x1e*\x1e\x1erefs/heads/main\n" +
		"origin\x1eabc123\x1e\x1e\x1e \x1erefs/remotes/origin/main\x1erefs/remotes/origin/HEAD\n" +
		"origin/main\x1eabc123\x1e\x1e\x1e \x1e\x1erefs/remotes/origin/main\n"

	branches, err := parseBranchList(input)
	require.NoError(t, err)
	require.Len(t, branches, 2, "symref 'origin' should be filtered out")

	assert.Equal(t, "main", branches[0].Name)
	assert.False(t, branches[0].IsRemote)

	assert.Equal(t, "origin/main", branches[1].Name)
	assert.True(t, branches[1].IsRemote)
}

func TestParseTrackInfo(t *testing.T) {
	tests := []struct {
		name   string
		track  string
		ahead  int
		behind int
	}{
		{name: "ahead only", track: "[ahead 3]", ahead: 3, behind: 0},
		{name: "behind only", track: "[behind 2]", ahead: 0, behind: 2},
		{name: "both", track: "[ahead 5, behind 1]", ahead: 5, behind: 1},
		{name: "empty", track: "", ahead: 0, behind: 0},
		{name: "gone", track: "[gone]", ahead: 0, behind: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := parseTrackInfo(tt.track)
			assert.Equal(t, tt.ahead, a)
			assert.Equal(t, tt.behind, b)
		})
	}
}
