package gitstatus

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockGitClient{})
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"git_status_file": "stage"},
		Confirmed:  map[string]bool{"git_status_file": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to max offset", func(t *testing.T) {
		p := New(&mockGitClient{})
		p.rows = []row{{}, {}, {}, {}}
		p.Height = 1

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*GitStatus)

		assert.Nil(t, cmd)
		assert.Equal(t, 3, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := New(&mockGitClient{})
		p.rows = []row{{}, {}, {}, {}}
		p.Height = 1
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*GitStatus)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestEnsureCursorVisible(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height leaves offset unchanged", height: 0, cursor: 0, offset: 2, wantOffset: 2},
		{name: "cursor above offset moves viewport up", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport moves viewport down", height: 3, cursor: 5, offset: 1, wantOffset: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&mockGitClient{})
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset

			p.ensureCursorVisible()

			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}

func TestInvalidateDiffCaches(t *testing.T) {
	p := New(&mockGitClient{})
	p.diffCache["a:staged"] = []git.Hunk{{Header: "@@ -1 +1 @@"}}
	p.diffCache["b:unstaged"] = []git.Hunk{{Header: "@@ -2 +2 @@"}}
	p.expandedFiles["a:staged"] = true
	p.expandedFiles["b:unstaged"] = true

	p.invalidateDiffCaches()

	assert.Empty(t, p.diffCache)
	assert.Empty(t, p.expandedFiles)
}

func TestFileKey(t *testing.T) {
	p := New(&mockGitClient{})
	file := &git.FileStatus{Path: "file.go"}

	tests := []struct {
		name string
		row  *row
		want string
	}{
		{name: "nil file returns empty string", row: &row{}, want: ""},
		{name: "staged appends staged suffix", row: &row{section: sectionStaged, file: file}, want: "file.go:staged"},
		{name: "unstaged appends unstaged suffix", row: &row{section: sectionUnstaged, file: file}, want: "file.go:unstaged"},
		{name: "untracked uses path only", row: &row{section: sectionUntracked, file: file}, want: "file.go"},
		{name: "unknown section uses path only", row: &row{section: section(99), file: file}, want: "file.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.fileKey(tt.row))
		})
	}
}

func TestSectionString(t *testing.T) {
	tests := []struct {
		name    string
		section section
		want    string
	}{
		{name: "staged", section: sectionStaged, want: "Staged"},
		{name: "unstaged", section: sectionUnstaged, want: "Unstaged"},
		{name: "untracked", section: sectionUntracked, want: "Untracked"},
		{name: "unknown", section: section(99), want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.section.String())
		})
	}
}

// ---------------------------------------------------------------------------
// executeRightClickAction
// ---------------------------------------------------------------------------

func TestExecuteRightClickAction_CopyPath(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	// cursor 0 is the section header, 1 is the file
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionCopyPath)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_ExpandDiff(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionExpandDiff)
	// expandOrEnter returns a diff load cmd
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_StageUnstage_Unstaged(t *testing.T) {
	file := git.FileStatus{Path: "unstaged.go", StagedStatus: git.StatusUnmodified, WorktreeStatus: git.StatusModified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	// Rows: unstaged header (0), file (1)
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionStageUnstage)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_StageUnstage_Staged(t *testing.T) {
	file := git.FileStatus{Path: "staged.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	// Rows: staged header (0), file (1)
	p.cursor = 1

	_, cmd := p.executeRightClickAction(actions.ActionStageUnstage)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_UnknownAction(t *testing.T) {
	p := New(&mockGitClient{})
	_, cmd := p.executeRightClickAction(actions.ActionID("unknown"))
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleModalResult
// ---------------------------------------------------------------------------

func TestHandleModalResult_Rejected(t *testing.T) {
	p := New(&mockGitClient{})
	p.pendingOp = opRightClickPick

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, "", p.pendingOp)
}

func TestHandleModalResult_RightClickPick(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	p.pendingOp = opRightClickPick
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionCopyPath),
	})
	assert.NotNil(t, cmd)
	assert.Equal(t, "", p.pendingOp)
}

func TestHandleModalResult_FirstUseConfirm(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified}
	p := newTestPanel(t, &mockGitClient{
		statusResult: []git.FileStatus{file},
	})
	p.pendingOp = opFirstUseConfirm
	p.pendingName = string(actions.ItemStatusFile)
	p.cursor = 1

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept:   true,
		Value:    string(actions.ActionExpandDiff),
		Remember: true,
	})
	assert.NotNil(t, cmd)
}

func TestHandleModalResult_UnknownOp(t *testing.T) {
	p := New(&mockGitClient{})
	p.pendingOp = "something_else"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "expand_diff"})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// copyPath
// ---------------------------------------------------------------------------

func TestCopyPath_OutOfBounds(t *testing.T) {
	p := New(&mockGitClient{})
	p.cursor = -1

	_, cmd := p.copyPath()
	assert.Nil(t, cmd)
}

func TestCopyPath_NilFile(t *testing.T) {
	p := New(&mockGitClient{})
	p.rows = []row{{kind: rowSection, section: sectionStaged}}
	p.cursor = 0

	_, cmd := p.copyPath()
	assert.Nil(t, cmd)
}

func TestCopyPath_ValidFile(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified}
	p := New(&mockGitClient{})
	p.ctx = context.Background()
	p.rows = []row{{kind: rowFile, section: sectionStaged, file: &file}}
	p.cursor = 0

	_, cmd := p.copyPath()
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleMouseRightClick
// ---------------------------------------------------------------------------

func TestHandleMouseRightClick_OutOfBounds(t *testing.T) {
	p := New(&mockGitClient{})
	p.rows = []row{{kind: rowFile}}
	p.Height = 10

	_, cmd := p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 99})
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_SectionHeader(t *testing.T) {
	p := New(&mockGitClient{})
	p.rows = []row{{kind: rowSection, section: sectionStaged}}
	p.Height = 10

	_, cmd := p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 0})
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_NilFile(t *testing.T) {
	p := New(&mockGitClient{})
	p.rows = []row{{kind: rowFile, file: nil}}
	p.Height = 10

	_, cmd := p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 0})
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_ValidFile(t *testing.T) {
	file := git.FileStatus{Path: "test.go", StagedStatus: git.StatusModified}
	p := New(&mockGitClient{})
	p.rows = []row{{kind: rowFile, section: sectionStaged, file: &file}}
	p.Height = 10
	// Mark as confirmed so it triggers direct action.
	p.actionsCfg = config.ActionsConfig{
		Confirmed: map[string]bool{string(actions.ItemStatusFile): true},
	}

	_, _ = p.handleMouseRightClick(panels.PanelMouseRightClickMsg{ContentRow: 0})
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// moveCursorDown / moveCursorUp — section-header skipping
// ---------------------------------------------------------------------------

func TestMoveCursorDown_SkipsSection(t *testing.T) {
	file1 := git.FileStatus{Path: "a.go", StagedStatus: git.StatusModified}
	file2 := git.FileStatus{Path: "b.go", WorktreeStatus: git.StatusModified}
	p := New(&mockGitClient{})
	p.rows = []row{
		{kind: rowFile, section: sectionStaged, file: &file1},
		{kind: rowSection, section: sectionUnstaged},
		{kind: rowFile, section: sectionUnstaged, file: &file2},
	}
	p.cursor = 0
	p.Height = 10

	p.moveCursorDown()
	// Should skip the section header at index 1 and land on file at index 2
	assert.Equal(t, 2, p.cursor)
}

func TestMoveCursorUp_SkipsSection(t *testing.T) {
	file1 := git.FileStatus{Path: "a.go", StagedStatus: git.StatusModified}
	file2 := git.FileStatus{Path: "b.go", WorktreeStatus: git.StatusModified}
	p := New(&mockGitClient{})
	p.rows = []row{
		{kind: rowFile, section: sectionStaged, file: &file1},
		{kind: rowSection, section: sectionUnstaged},
		{kind: rowFile, section: sectionUnstaged, file: &file2},
	}
	p.cursor = 2
	p.Height = 10

	p.moveCursorUp()
	// Should skip the section header at index 1 and land on file at index 0
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// fileColor
// ---------------------------------------------------------------------------

func TestFileColor(t *testing.T) {
	p := New(&mockGitClient{})
	tests := []struct {
		name    string
		section section
		want    string
	}{
		{name: "staged", section: sectionStaged},
		{name: "unstaged", section: sectionUnstaged},
		{name: "untracked", section: sectionUntracked},
		{name: "unknown falls to default", section: section(99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.fileColor(tt.section)
			assert.NotEmpty(t, got, "fileColor should return a non-empty color string")
		})
	}
}

// ---------------------------------------------------------------------------
// statusIndicator — default branch
// ---------------------------------------------------------------------------

func TestStatusIndicator_DefaultSection(t *testing.T) {
	f := &git.FileStatus{Path: "test.go"}
	got := statusIndicator(f, section(99))
	assert.Equal(t, " ", got)
}
