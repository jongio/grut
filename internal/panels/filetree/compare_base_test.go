package filetree

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func filetreeCompareBaseMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	msgs := make([]tea.Msg, 0, len(batch))
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		msgs = append(msgs, batchCmd())
	}
	return msgs
}

func TestCompareBaseOverridesBranchDiffBase(t *testing.T) {
	t.Parallel()
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.SetBaseBranch("main")
	var gotBase string
	ft.SetGitClient(&mockGitClient{
		DiffFileNamesFunc: func(_ context.Context, commitA, commitB string) ([]string, error) {
			gotBase = commitA
			assert.Equal(t, "HEAD", commitB)
			return []string{"main.go"}, nil
		},
	})

	_, cmd := ft.Update(panels.SetCompareBaseMsg{Ref: "origin/release"})
	require.Nil(t, cmd)
	_, cmd = ft.activateBranchDiffFilter()
	require.NotNil(t, cmd)
	loaded, ok := cmd().(branchFilesLoadedMsg)
	require.True(t, ok)

	assert.Equal(t, "origin/release", gotBase)
	assert.Equal(t, "origin/release", loaded.branch)
}

func TestCompareBaseClearRestoresDefaultBase(t *testing.T) {
	t.Parallel()
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.SetBaseBranch("main")
	var gotBase string
	ft.SetGitClient(&mockGitClient{
		DiffFileNamesFunc: func(_ context.Context, commitA, _ string) ([]string, error) {
			gotBase = commitA
			return []string{"README.md"}, nil
		},
	})

	_, cmd := ft.Update(panels.SetCompareBaseMsg{Ref: "v1.0.0"})
	require.Nil(t, cmd)
	_, cmd = ft.Update(panels.ClearCompareBaseMsg{})
	require.Nil(t, cmd)
	_, cmd = ft.activateBranchDiffFilter()
	require.NotNil(t, cmd)
	_ = cmd()

	assert.Equal(t, "main", ft.filter.baseBranch)
	assert.Equal(t, "main", gotBase)
}

func TestCompareBaseMissingRefWarnsAndExitsBranchDiff(t *testing.T) {
	t.Parallel()
	dir := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), dir)
	ft.SetBaseBranch("main")
	ft.SetGitClient(&mockGitClient{
		DiffFileNamesFunc: func(_ context.Context, _, _ string) ([]string, error) {
			return nil, errors.New("unknown revision")
		},
	})
	_, cmd := ft.activateBranchDiffFilter()
	require.NotNil(t, cmd)
	loaded, ok := cmd().(branchFilesLoadedMsg)
	require.True(t, ok)

	_, cmd = ft.Update(loaded)
	msgs := filetreeCompareBaseMessages(t, cmd)

	assert.False(t, ft.filter.branchDiffFilter)
	assert.False(t, ft.filter.branchFilesMode)
	var warned, deactivated bool
	for _, msg := range msgs {
		switch typed := msg.(type) {
		case notify.ShowToastMsg:
			warned = typed.Level == notify.Warn && typed.Message == "Compare base main not found"
		case panels.BranchDiffFilterActiveMsg:
			deactivated = !typed.Active
		}
	}
	assert.True(t, warned)
	assert.True(t, deactivated)
}
