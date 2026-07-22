package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleToggleBlameReturnsBlameLoadedMsg(t *testing.T) {
	t.Parallel()

	want := []git.BlameLine{{
		Hash:    "abcdef1234567890abcdef1234567890abcdef12",
		Author:  "Alice",
		Date:    time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
		Content: "line one",
	}}
	mock := &gittest.MockClient{
		BlameFunc: func(_ context.Context, path string) ([]git.BlameLine, error) {
			assert.Equal(t, "x", path)
			return want, nil
		},
	}
	m := newTestModel(t).WithGitClient(mock)

	_, cmd := m.handleToggleBlame(panels.ToggleBlameMsg{Path: "x"})
	require.NotNil(t, cmd)

	msg, ok := cmd().(panels.BlameLoadedMsg)
	require.True(t, ok)
	assert.NoError(t, msg.Err)
	assert.Equal(t, want, msg.Lines)
}

func TestHandleToggleBlameReturnsErrorMsg(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("blame failed")
	mock := &gittest.MockClient{
		BlameFunc: func(_ context.Context, _ string) ([]git.BlameLine, error) {
			return nil, wantErr
		},
	}
	m := newTestModel(t).WithGitClient(mock)

	_, cmd := m.handleToggleBlame(panels.ToggleBlameMsg{Path: "x"})
	require.NotNil(t, cmd)

	msg, ok := cmd().(panels.BlameLoadedMsg)
	require.True(t, ok)
	assert.ErrorIs(t, msg.Err, wantErr)
	assert.Nil(t, msg.Lines)
}

func TestHandleToggleBlameNilGitClient(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	_, cmd := m.handleToggleBlame(panels.ToggleBlameMsg{Path: "x"})
	assert.Nil(t, cmd)
}

func TestToggleBlameLoadedBroadcastReachesPreview(t *testing.T) {
	t.Parallel()

	want := []git.BlameLine{{
		Hash:    "abcdef1234567890abcdef1234567890abcdef12",
		Author:  "Alice",
		Date:    time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
		Content: "line one",
	}}
	mock := &gittest.MockClient{
		BlameFunc: func(_ context.Context, _ string) ([]git.BlameLine, error) {
			return want, nil
		},
	}
	m := newTestModel(t).WithGitClient(mock)
	m.engine.FocusByName("preview")
	_ = m.engine.Update(panels.FileSelectedMsg{Path: "x"})
	_ = m.engine.Update(tea.KeyPressMsg{Text: "B", Code: 'B'})

	updated, cmd := m.Update(panels.ToggleBlameMsg{Path: "x"})
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	previewPanel := m.engine.Panels()["preview"]
	require.NotNil(t, previewPanel)
	previewValue := reflect.ValueOf(previewPanel).Elem()
	assert.True(t, previewValue.FieldByName("blameMode").Bool())
	assert.Equal(t, 1, previewValue.FieldByName("blameLines").Len())
}
