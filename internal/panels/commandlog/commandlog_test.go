package commandlog

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanelRendersEmptyState(t *testing.T) {
	t.Parallel()
	p := NewWithLog(git.NewCommandLog(10), nil)
	p.SetSize(80, 10)
	p.Init(context.Background())

	view := p.View(80, 10)

	assert.Contains(t, view, "No git commands recorded yet")
}

func TestPanelRendersSuccessAndFailureEntries(t *testing.T) {
	t.Parallel()
	log := git.NewCommandLog(10)
	log.Record(git.CommandEntry{
		Timestamp: time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC),
		Args:      []string{"status", "--short"},
		Dir:       "repo",
		Duration:  12 * time.Millisecond,
		Success:   true,
	})
	log.Record(git.CommandEntry{
		Timestamp:  time.Date(2026, 7, 22, 17, 1, 0, 0, time.UTC),
		Args:       []string{"rev-parse", "--bad"},
		Dir:        "repo",
		Duration:   3 * time.Millisecond,
		Success:    false,
		ErrSummary: "fatal: bad revision",
	})
	p := NewWithLog(log, nil)
	p.SetSize(120, 10)
	p.Init(context.Background())

	view := p.View(120, 10)

	assert.Contains(t, view, successSymbol)
	assert.Contains(t, view, failureSymbol)
	assert.Contains(t, view, "git status --short")
	assert.Contains(t, view, "git rev-parse --bad")
	assert.Contains(t, view, "fatal: bad revision")
}

func TestPanelScrollsWithinBounds(t *testing.T) {
	t.Parallel()
	log := git.NewCommandLog(30)
	for i := range 20 {
		log.Record(git.CommandEntry{
			Timestamp: time.Date(2026, 7, 22, 17, i, 0, 0, time.UTC),
			Args:      []string{"show", "--stat"},
			Dir:       "repo",
			Duration:  time.Millisecond,
			Success:   true,
		})
	}
	p := NewWithLog(log, nil)
	p.SetSize(80, 5)
	p.Init(context.Background())

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	require.Nil(t, cmd)
	assert.Equal(t, 5, p.scrollOffset())

	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	require.Nil(t, cmd)
	assert.Equal(t, 15, p.scrollOffset())

	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	require.Nil(t, cmd)
	assert.Equal(t, 15, p.scrollOffset())

	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	require.Nil(t, cmd)
	assert.Equal(t, 0, p.scrollOffset())
}

func TestPanelEscapeRequestsClose(t *testing.T) {
	t.Parallel()
	p := NewWithLog(git.NewCommandLog(10), nil)

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.ToggleCommandLogMsg)
	assert.True(t, ok)
}
