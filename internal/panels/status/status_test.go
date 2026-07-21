package status

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	status []git.FileStatus
	branch git.Branch
	err    error
	isRepo bool
}

func (m mockClient) Status(context.Context) ([]git.FileStatus, error) {
	return m.status, m.err
}

func (m mockClient) CurrentBranch(context.Context) (git.Branch, error) {
	return m.branch, nil
}

func (m mockClient) IsRepo(context.Context) (bool, error) {
	return m.isRepo, nil
}

func TestNew(t *testing.T) {
	p := New(nil, nil)
	assert.Equal(t, "status", p.Title())
	assert.NotEmpty(t, p.KeyBindings())
}

func TestViewWithoutGitClient(t *testing.T) {
	p := New(nil, nil)
	out := p.View(80, 3)
	assert.Contains(t, out, "No git repository")
}

func TestLoadShowsBranchAndCounts(t *testing.T) {
	p := New(mockClient{
		isRepo: true,
		branch: git.Branch{
			Name:   "main",
			Ahead:  1,
			Behind: 2,
		},
		status: []git.FileStatus{
			{Path: "staged.go", StagedStatus: git.StatusModified},
			{Path: "unstaged.go", WorktreeStatus: git.StatusModified},
			{Path: "new.go", StagedStatus: git.StatusUntracked, WorktreeStatus: git.StatusUntracked},
			{Path: "conflict.go", StagedStatus: git.StatusConflict, WorktreeStatus: git.StatusConflict},
		},
	}, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
	panel, next := p.Update(cmd())
	require.Nil(t, next)
	p = panel.(*Panel)

	out := p.View(80, 12)
	assert.Contains(t, out, "Branch:    main")
	assert.Contains(t, out, "ahead 1, behind 2")
	assert.Contains(t, out, "Staged:    1")
	assert.Contains(t, out, "Unstaged:  1")
	assert.Contains(t, out, "Untracked: 1")
	assert.Contains(t, out, "Conflicts: 1")
}

func TestLoadError(t *testing.T) {
	p := New(mockClient{isRepo: true, err: errors.New("boom")}, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
	p.Update(cmd())

	out := p.View(80, 4)
	assert.Contains(t, out, "Status unavailable")
	assert.Contains(t, out, "boom")
}

func TestNonRepoLoad(t *testing.T) {
	p := New(mockClient{isRepo: false}, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
	p.Update(cmd())

	out := p.View(80, 4)
	assert.Contains(t, out, "Status unavailable")
	assert.Contains(t, out, "not a git repository")
}

func TestRepoChangedToNonGitClearsClient(t *testing.T) {
	p := New(mockClient{isRepo: true}, nil)
	panel, cmd := p.Update(panels.RepoChangedMsg{Path: filepath.Join(t.TempDir(), "missing")})
	require.Nil(t, cmd)
	p = panel.(*Panel)

	out := p.View(80, 3)
	assert.Contains(t, out, "No git repository")
}

func TestTrimLine(t *testing.T) {
	out := New(nil, nil).View(5, 1)
	assert.True(t, len([]rune(strings.Split(out, "\n")[0])) <= 5)
}
