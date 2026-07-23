package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPanelManager struct {
	layout.PanelManager
	messages []tea.Msg
}

func (r *recordingPanelManager) Update(msg tea.Msg) tea.Cmd {
	r.messages = append(r.messages, msg)
	return nil
}

func TestCompareBaseMsgUpdatesStatusAndForwards(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	recorder := &recordingPanelManager{PanelManager: m.engine}
	m.engine = recorder

	updated, cmd := m.Update(panels.SetCompareBaseMsg{Ref: "origin/main"})
	require.Nil(t, cmd)
	m = updated.(Model)

	assert.Equal(t, "origin/main", m.compareBase)
	require.Len(t, recorder.messages, 1)
	assert.IsType(t, panels.SetCompareBaseMsg{}, recorder.messages[0])

	updated, cmd = m.Update(panels.ClearCompareBaseMsg{})
	require.Nil(t, cmd)
	m = updated.(Model)

	assert.Empty(t, m.compareBase)
	require.Len(t, recorder.messages, 2)
	assert.IsType(t, panels.ClearCompareBaseMsg{}, recorder.messages[1])
}

func TestStatusBarCompareBaseSegment(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120

	assert.NotContains(t, m.renderStatusBar(), "⇄ base:")

	m.compareBase = "v1.0.0"
	bar := m.renderStatusBar()

	assert.Contains(t, bar, "⇄ base: v1.0.0")
}
