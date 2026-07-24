package tui

import (
	"context"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavHistoryRecordsSelectionMessages(t *testing.T) {
	tests := []struct {
		name      string
		msg       tea.Msg
		wantKind  string
		wantKey   string
		wantFocus string
	}{
		{
			name:      "file",
			msg:       panels.FileSelectedMsg{Path: "internal/tui/app.go", Line: 12},
			wantKind:  navKindFile,
			wantKey:   "internal/tui/app.go\x0012",
			wantFocus: panelFileTree,
		},
		{
			name:      "commit",
			msg:       panels.CommitSelectedMsg{Hash: "abc123", Subject: "subject"},
			wantKind:  navKindCommit,
			wantKey:   "abc123",
			wantFocus: panelCommits,
		},
		{
			name:      "branch",
			msg:       panels.BranchSelectedMsg{Name: "main"},
			wantKind:  navKindBranch,
			wantKey:   "main",
			wantFocus: panelBranches,
		},
		{
			name:      "issue",
			msg:       panels.IssueSelectedMsg{Number: 205, Title: "issue"},
			wantKind:  navKindIssue,
			wantKey:   "205",
			wantFocus: panelGitHub,
		},
		{
			name:      "pr",
			msg:       panels.PRSelectedMsg{Number: 42, Title: "pr"},
			wantKind:  navKindPR,
			wantKey:   "42",
			wantFocus: panelGitHub,
		},
		{
			name:      "workflow",
			msg:       panels.WorkflowSelectedMsg{Name: "CI", Path: ".github/workflows/ci.yml"},
			wantKind:  navKindWorkflow,
			wantKey:   ".github/workflows/ci.yml\x00CI",
			wantFocus: panelGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &navRecordingPanelManager{}
			m := Model{engine: engine}

			updated, _ := m.Update(tt.msg)
			m = updated.(Model)

			require.Len(t, m.nav.entries, 1)
			entry := m.nav.entries[0]
			assert.Equal(t, tt.wantKind, entry.kind)
			assert.Equal(t, tt.wantKey, entry.key)
			assert.Equal(t, tt.wantFocus, entry.focusPanel)
			assert.Equal(t, tt.msg, entry.msg)
		})
	}
}

func TestNavHistoryCollapsesConsecutiveDuplicates(t *testing.T) {
	history := navHistory{}
	entry := navEntry{kind: navKindCommit, key: "abc123", msg: panels.CommitSelectedMsg{Hash: "abc123"}}

	history.record(entry)
	history.record(entry)

	require.Len(t, history.entries, 1)
	assert.Equal(t, 0, history.index)
}

func TestNavHistoryTruncatesForwardEntriesAfterBackAndNewRecord(t *testing.T) {
	history := navHistory{}
	history.record(navEntry{kind: navKindFile, key: "a"})
	history.record(navEntry{kind: navKindFile, key: "b"})
	history.record(navEntry{kind: navKindFile, key: "c"})

	entry, ok := history.back()
	require.True(t, ok)
	require.Equal(t, "b", entry.key)

	history.record(navEntry{kind: navKindFile, key: "d"})

	require.Len(t, history.entries, 3)
	assert.Equal(t, []string{"a", "b", "d"}, navHistoryKeys(history))
	assert.Equal(t, 2, history.index)
	_, ok = history.forward()
	assert.False(t, ok)
}

func TestNavHistoryBackForwardRestoresSelectionThroughEngineAndFocus(t *testing.T) {
	engine := &navRecordingPanelManager{}
	m := Model{engine: engine}
	updated, _ := m.Update(panels.FileSelectedMsg{Path: "README.md"})
	m = updated.(Model)
	updated, _ = m.Update(panels.PRSelectedMsg{Number: 7, Title: "Add feature"})
	m = updated.(Model)

	updated, cmd := m.handleAction(actionNavBack, nil)
	m = updated.(Model)

	assert.Nil(t, cmd)
	assert.Equal(t, panelFileTree, engine.focusedName)
	assert.Equal(t, panels.FileSelectedMsg{Path: "README.md"}, engine.lastMsg)

	updated, cmd = m.handleAction(actionNavForward, nil)
	_ = updated

	assert.Nil(t, cmd)
	assert.Equal(t, panelGitHub, engine.focusedName)
	assert.Equal(t, panels.PRSelectedMsg{Number: 7, Title: "Add feature"}, engine.lastMsg)
}

func TestNavHistoryResetOnRepoRootChange(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	targetDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	engine := &navRecordingPanelManager{}
	m := Model{engine: engine}
	updated, _ := m.Update(panels.FileSelectedMsg{Path: "README.md"})
	m = updated.(Model)
	require.Len(t, m.nav.entries, 1)

	updated, _ = m.handleChangeDirectoryMsg(panels.ChangeDirectoryMsg{Path: targetDir})
	m = updated.(Model)

	assert.Empty(t, m.nav.entries)
	assert.Equal(t, 0, m.nav.index)
}

func navHistoryKeys(history navHistory) []string {
	keys := make([]string, 0, len(history.entries))
	for _, entry := range history.entries {
		keys = append(keys, entry.key)
	}
	return keys
}

type navRecordingPanelManager struct {
	lastMsg     tea.Msg
	focusedName string
}

func (r *navRecordingPanelManager) Init(context.Context) tea.Cmd {
	return nil
}

func (r *navRecordingPanelManager) SetSize(int, int) {}

func (r *navRecordingPanelManager) Update(msg tea.Msg) tea.Cmd {
	r.lastMsg = msg
	return nil
}

func (r *navRecordingPanelManager) FocusedPanel() panels.Panel {
	return nil
}

func (r *navRecordingPanelManager) FocusedName() string {
	return r.focusedName
}

func (r *navRecordingPanelManager) FocusNext() {
	r.focusedName = "next"
}

func (r *navRecordingPanelManager) FocusPrev() {
	r.focusedName = "prev"
}

func (r *navRecordingPanelManager) FocusByName(name string) bool {
	r.focusedName = name
	return true
}

func (r *navRecordingPanelManager) ToggleZoom() {}

func (r *navRecordingPanelManager) IsZoomed() bool {
	return false
}

func (r *navRecordingPanelManager) ResizeGrow() {}

func (r *navRecordingPanelManager) ResizeShrink() {}

func (r *navRecordingPanelManager) RotatePreviewPosition() {}

func (r *navRecordingPanelManager) CurrentPreviewPosition() layout.PreviewPosition {
	return layout.PreviewRight
}

func (r *navRecordingPanelManager) SetPreviewPosition(layout.PreviewPosition) {}

func (r *navRecordingPanelManager) Panels() map[string]panels.Panel {
	return nil
}

func (r *navRecordingPanelManager) PanelRects() map[string]layout.Rect {
	return nil
}

func (r *navRecordingPanelManager) InnerArea() layout.Rect {
	return layout.Rect{}
}

func (r *navRecordingPanelManager) TabManager() *layout.TabManager {
	return nil
}

func (r *navRecordingPanelManager) AddTab(layout.Preset) (tea.Cmd, error) {
	return nil, nil
}

func (r *navRecordingPanelManager) CloseActiveTab() error {
	return nil
}

func (r *navRecordingPanelManager) SwitchTab(int) error {
	return nil
}

func (r *navRecordingPanelManager) SplitFocusedVertical(string) (tea.Cmd, error) {
	return nil, nil
}

func (r *navRecordingPanelManager) SplitFocusedHorizontal(string) (tea.Cmd, error) {
	return nil, nil
}

func (r *navRecordingPanelManager) CloseFocusedPanel() error {
	return nil
}
