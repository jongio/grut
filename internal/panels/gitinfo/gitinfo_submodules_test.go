package gitinfo

import (
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

func TestSubmoduleTabRendering(t *testing.T) {
	t.Parallel()
	const commit = "0123456789abcdef0123456789abcdef01234567"

	t.Run("hidden when repository has no submodules", func(t *testing.T) {
		t.Parallel()
		p := newTestPanel(t, defaultMock())
		view := p.View(120, 8)
		assert.NotContains(t, view, labelSubmodules)
	})

	t.Run("visible with row when repository has submodules", func(t *testing.T) {
		t.Parallel()
		mock := defaultMock()
		mock.submodules = []git.Submodule{{Path: "third party/library", Commit: commit, Initialized: true, Describe: "v1.0.0"}}
		p := newTestPanel(t, mock)
		view := p.View(120, 8)
		assert.Contains(t, view, labelSubmodules)
		p.SetActiveTab(sectionSubmodules)
		view = p.View(120, 8)
		assert.Contains(t, view, "third party/library")
		assert.Contains(t, view, commit[:git.ShortHashLen])
		assert.Contains(t, view, "clean")
	})
}

func TestSubmoduleEmptyStateAndSelection(t *testing.T) {
	t.Parallel()
	const commit = "89abcdef0123456789abcdef0123456789abcdef"
	p := newTestPanel(t, defaultMock())
	p.SetActiveTab(sectionSubmodules)
	assert.Contains(t, p.View(80, 6), "No submodules")

	p.buildItems(nil, nil, nil, nil, nil, nil, []git.Submodule{{Path: "deps/module", Commit: commit, Initialized: true, Modified: true, Describe: "heads/main"}})
	p.SetActiveTab(sectionSubmodules)
	cmd := p.activeTabSelectionCmd()
	if assert.NotNil(t, cmd) {
		msg, ok := cmd().(panels.SubmoduleSelectedMsg)
		assert.True(t, ok)
		assert.Equal(t, "deps/module", msg.Path)
		assert.Equal(t, "modified", msg.State)
		assert.Equal(t, "heads/main", msg.Describe)
	}
}
