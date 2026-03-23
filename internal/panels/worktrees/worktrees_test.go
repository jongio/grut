package worktrees

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGitOps struct {
	worktrees []git.Worktree
	listErr   error
	addErr    error
	removeErr error

	// Track calls for assertions.
	lastAddPath   string
	lastAddBranch string
	lastRemPath   string
	lastRemForce  bool
}

var _ GitOps = (*mockGitOps)(nil)

func (m *mockGitOps) WorktreeList(_ context.Context) ([]git.Worktree, error) {
	return m.worktrees, m.listErr
}

func (m *mockGitOps) WorktreeAdd(_ context.Context, path, branch string) error {
	m.lastAddPath = path
	m.lastAddBranch = branch
	return m.addErr
}

func (m *mockGitOps) WorktreeRemove(_ context.Context, path string, force bool) error {
	m.lastRemPath = path
	m.lastRemForce = force
	return m.removeErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sampleWorktrees() []git.Worktree {
	return []git.Worktree{
		{Path: "/home/user/grut", Branch: "main", Head: "abc1234567890"},
		{Path: "/home/user/grut-feat", Branch: "feature/foo", Head: "def5678901234"},
		{Path: "/home/user/grut-fix", Branch: "fix/bar", Head: "ghi9012345678"},
	}
}

// alwaysExists is a PathChecker that reports all paths as existing.
func alwaysExists(_ string) bool { return true }

// neverExists is a PathChecker that reports all paths as missing.
func neverExists(_ string) bool { return false }

// selectiveMissing returns a PathChecker that reports the given path as missing.
func selectiveMissing(missingPath string) PathChecker {
	return func(path string) bool {
		return path != missingPath
	}
}

// keyMsg constructs a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// testGitConfig returns a default GitConfig suitable for tests.
func testGitConfig() config.GitConfig {
	return config.GitConfig{WorktreeOpenMode: "current"}
}

// newTestPanel creates a Panel with a mock client and processes the initial
// worktree load synchronously, simulating the Init() → worktreesLoadedMsg cycle.
func newTestPanel(t *testing.T, mock *mockGitOps, checker PathChecker) *Panel {
	t.Helper()
	p := New(mock, testGitConfig(), "/fake/repo")
	if checker != nil {
		p.pathCheck = checker
	}
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd, "Init should return a command")
	msg := cmd()
	p.Update(msg)
	return p
}

// ---------------------------------------------------------------------------
// Compile-time interface check
// ---------------------------------------------------------------------------

func TestPanelImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mock := &mockGitOps{}
	p := New(mock, testGitConfig(), "/repo")
	assert.Equal(t, "worktrees", p.Title())
	assert.NotNil(t, p.git)
}

func TestInit_ReturnsLoadCmd(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := New(mock, testGitConfig(), "/repo")
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Worktree loading
// ---------------------------------------------------------------------------

func TestLoadWorktrees_PopulatesItems(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	assert.Len(t, p.items, 3)
	assert.Equal(t, "/home/user/grut", p.items[0].worktree.Path)
	assert.Equal(t, "main", p.items[0].worktree.Branch)
	assert.True(t, p.items[0].isMain)
	assert.Equal(t, "/home/user/grut-feat", p.items[1].worktree.Path)
	assert.Equal(t, "feature/foo", p.items[1].worktree.Branch)
	assert.False(t, p.items[1].isMain)
}

func TestLoadWorktrees_CursorOnFirst(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	assert.Equal(t, 0, p.cursor)
}

func TestLoadWorktrees_Error(t *testing.T) {
	mock := &mockGitOps{listErr: errors.New("git not found")}
	p := New(mock, testGitConfig(), "/repo")
	cmd := p.Init(context.Background())
	msg := cmd()

	_, cmd2 := p.Update(msg)
	require.NotNil(t, cmd2)
	result := cmd2()
	toast, ok := result.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "git not found")
}

func TestLoadWorktrees_Empty(t *testing.T) {
	mock := &mockGitOps{worktrees: nil}
	p := newTestPanel(t, mock, alwaysExists)

	assert.Empty(t, p.items)
}

// ---------------------------------------------------------------------------
// External deletion detection
// ---------------------------------------------------------------------------

func TestMissingWorktreeDetection(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	checker := selectiveMissing("/home/user/grut-fix")
	p := newTestPanel(t, mock, checker)

	assert.False(t, p.items[0].isMissing, "main worktree should exist")
	assert.False(t, p.items[1].isMissing, "feat worktree should exist")
	assert.True(t, p.items[2].isMissing, "fix worktree should be missing")
}

func TestMissingWorktreeDetection_AllMissing(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, neverExists)

	for _, item := range p.items {
		assert.True(t, item.isMissing, "all worktrees should be missing")
	}
}

func TestMissingWorktreeDetection_BareSkipped(t *testing.T) {
	mock := &mockGitOps{worktrees: []git.Worktree{
		{Path: "/home/user/grut.git", Branch: "", Bare: true},
	}}
	// Even neverExists should not flag a bare worktree as missing.
	p := newTestPanel(t, mock, neverExists)

	assert.False(t, p.items[0].isMissing, "bare worktree should not be flagged as missing")
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

func TestView_ShowsWorktrees(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)
	p.Focus()

	view := p.View(80, 20)
	assert.Contains(t, view, "/home/user/grut")
	assert.Contains(t, view, "main")
	assert.Contains(t, view, "feature/foo")
	assert.Contains(t, view, "fix/bar")
}

func TestView_MainWorktreeMarked(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	view := p.View(80, 20)
	assert.Contains(t, view, "* /home/user/grut")
}

func TestView_MissingTag(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	checker := selectiveMissing("/home/user/grut-fix")
	p := newTestPanel(t, mock, checker)

	view := p.View(80, 20)
	assert.Contains(t, view, "[MISSING]")
}

func TestView_HashDisplayed(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	view := p.View(80, 20)
	assert.Contains(t, view, "abc1234") // truncated to 7 chars
	assert.Contains(t, view, "def5678")
}

func TestView_Empty(t *testing.T) {
	mock := &mockGitOps{worktrees: nil}
	p := newTestPanel(t, mock, alwaysExists)

	view := p.View(40, 10)
	assert.Contains(t, view, "No worktrees")
}

func TestView_ZeroDimensions(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(40, 0))
}

func TestView_DetachedHead(t *testing.T) {
	mock := &mockGitOps{worktrees: []git.Worktree{
		{Path: "/home/user/grut", Branch: "", Head: "abc1234567890"},
	}}
	p := newTestPanel(t, mock, alwaysExists)

	view := p.View(80, 10)
	assert.Contains(t, view, "(detached)")
}

func TestView_BareWorktree(t *testing.T) {
	mock := &mockGitOps{worktrees: []git.Worktree{
		{Path: "/home/user/grut.git", Branch: "", Bare: true},
	}}
	p := newTestPanel(t, mock, alwaysExists)

	view := p.View(80, 10)
	assert.Contains(t, view, "(bare)")
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func TestNavigation_JK(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Cursor starts at 0.
	assert.Equal(t, 0, p.cursor)

	// Move down.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	// Move down again.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// Move down at bottom — stays.
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)

	// Move up.
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursor)

	// Move up again.
	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)

	// Move up at top — stays.
	p.Update(keyMsg('k'))
	assert.Equal(t, 0, p.cursor)
}

func TestNavigation_NotFocused(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	// Panel is NOT focused.

	initial := p.cursor
	p.Update(keyMsg('j'))
	assert.Equal(t, initial, p.cursor, "cursor should not move when not focused")
}

func TestNavigation_ViewportScrolling(t *testing.T) {
	// Create many worktrees to test scrolling.
	wts := make([]git.Worktree, 20)
	for i := range wts {
		wts[i] = git.Worktree{
			Path:   "/wt/" + string(rune('a'+i)),
			Branch: "branch-" + string(rune('a'+i)),
			Head:   "abc1234",
		}
	}
	mock := &mockGitOps{worktrees: wts}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()
	p.SetSize(80, 5) // Only 5 lines visible.

	// Navigate down past viewport.
	for range 6 {
		p.Update(keyMsg('j'))
	}

	assert.Equal(t, 6, p.cursor)
	assert.True(t, p.offset > 0, "offset should increase when cursor moves past viewport")
}

// ---------------------------------------------------------------------------
// Switch worktree
// ---------------------------------------------------------------------------

func TestSwitch_EmitsSwitchMsg(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move to second worktree.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	switchMsg, ok := msg.(panels.SwitchWorktreeMsg)
	require.True(t, ok)
	assert.Equal(t, "/home/user/grut-feat", switchMsg.Path)
}

func TestSwitch_MissingWorktree_ShowsWarning(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	checker := selectiveMissing("/home/user/grut-fix")
	p := newTestPanel(t, mock, checker)
	p.Focus()

	// Navigate to missing worktree (index 2).
	p.Update(keyMsg('j'))
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)
	assert.True(t, p.items[2].isMissing)

	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "missing")
}

// ---------------------------------------------------------------------------
// Create worktree
// ---------------------------------------------------------------------------

func TestCreateWorktree(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Press 'n' to request create.
	_, cmd := p.Update(keyMsg('n'))
	require.NotNil(t, cmd)

	// Should produce ShowModalMsg.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "New Worktree", modal.Title)
	assert.Equal(t, notify.ModalInput, modal.Kind)

	// Simulate modal result with a branch name.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "feature/new"})
	require.NotNil(t, cmd)

	// Execute the create command.
	result := cmd()
	opResult, ok := result.(worktreeOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "created", opResult.op)
	assert.Equal(t, "feature/new", opResult.name)
	assert.NoError(t, opResult.err)

	// Verify path uses worktree convention.
	assert.Contains(t, mock.lastAddPath, ".worktrees")
	assert.Contains(t, mock.lastAddPath, "repo")
	assert.Contains(t, mock.lastAddPath, "feature-new")
	assert.Equal(t, "feature/new", mock.lastAddBranch)
}

func TestCreateWorktree_EmptyName(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	p.Update(keyMsg('n'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "  "})
	assert.Nil(t, cmd, "empty name should not trigger create")
}

func TestCreateWorktree_Cancelled(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	p.Update(keyMsg('n'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancelled modal should not trigger create")
}

func TestCreateWorktree_Error(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees(), addErr: errors.New("branch exists")}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	p.Update(keyMsg('n'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "existing-branch"})
	require.NotNil(t, cmd)

	msg := cmd()
	// Feed result through Update to get the error toast.
	_, cmd2 := p.Update(msg)
	require.NotNil(t, cmd2)
	result := cmd2()
	toast, ok := result.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "branch exists")
}

// ---------------------------------------------------------------------------
// Remove worktree
// ---------------------------------------------------------------------------

func TestRemoveWorktree(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move to second worktree (not main).
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)
	assert.False(t, p.items[1].isMain)

	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)

	// Should produce confirmation modal.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Remove Worktree", modal.Title)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)

	// Confirm removal.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	result := cmd()
	opResult, ok := result.(worktreeOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "removed", opResult.op)
	assert.NoError(t, opResult.err)
	assert.Equal(t, "/home/user/grut-feat", mock.lastRemPath)
	assert.False(t, mock.lastRemForce)
}

func TestRemoveWorktree_MainBlocked(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Cursor is on main worktree (index 0).
	assert.True(t, p.items[0].isMain)

	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Cannot remove main worktree")
}

func TestRemoveWorktree_Cancelled(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	p.Update(keyMsg('j'))
	p.Update(keyMsg('d'))
	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancelled modal should not trigger remove")
}

func TestRemoveWorktree_XKeyTriggersDelete(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move to second worktree (not main).
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)
	assert.False(t, p.items[1].isMain)

	// Press "x" — should trigger delete confirmation.
	_, cmd := p.Update(keyMsg('x'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Remove Worktree", modal.Title)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
}

func TestRemoveWorktree_ItemDeleteMsgTriggersDelete(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move to second worktree (not main).
	p.Update(keyMsg('j'))

	// Send ItemDeleteMsg (dispatched by keymap for "x" → "item_delete" action).
	_, cmd := p.Update(panels.ItemDeleteMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Remove Worktree", modal.Title)
}

func TestRemoveWorktree_ItemDeleteMsgNotFocused(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	// Panel is NOT focused.
	p.Update(keyMsg('j'))

	_, cmd := p.Update(panels.ItemDeleteMsg{})
	assert.Nil(t, cmd, "unfocused panel should ignore ItemDeleteMsg")
}

func TestRemoveWorktree_RepeatedDeleteWorks(t *testing.T) {
	// Regression test for #45: repeated deletes should work without
	// manual re-navigation because cursor is preserved after deletion.
	wts := []git.Worktree{
		{Path: "/wt/main", Branch: "main", Head: "aaa"},
		{Path: "/wt/feat-a", Branch: "feat-a", Head: "bbb"},
		{Path: "/wt/feat-b", Branch: "feat-b", Head: "ccc"},
		{Path: "/wt/feat-c", Branch: "feat-c", Head: "ddd"},
	}
	mock := &mockGitOps{worktrees: wts}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move to feat-a (index 1).
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	// Delete feat-a.
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)
	cmd()
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result := cmd()
	opResult, ok := result.(worktreeOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "removed", opResult.op)

	// Process opResult (returns batch cmd with reload + notifications).
	p.Update(opResult)

	// Simulate reload with feat-a removed.
	mock.worktrees = []git.Worktree{
		{Path: "/wt/main", Branch: "main", Head: "aaa"},
		{Path: "/wt/feat-b", Branch: "feat-b", Head: "ccc"},
		{Path: "/wt/feat-c", Branch: "feat-c", Head: "ddd"},
	}
	reloadCmd := p.loadWorktrees()
	reloadMsg := reloadCmd()
	p.Update(reloadMsg)

	// Cursor should still be at index 1 (now feat-b), enabling second delete.
	assert.Equal(t, 1, p.cursor)
	assert.Equal(t, "feat-b", p.items[p.cursor].worktree.Branch)

	// Second delete should work immediately.
	_, cmd = p.Update(keyMsg('x'))
	require.NotNil(t, cmd)
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Remove Worktree", modal.Title)
	assert.Contains(t, modal.Message, "feat-b")
}

func TestCreateWorktree_ItemCreateMsgTriggersCreate(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Send ItemCreateMsg (dispatched by keymap for "n" → "item_create" action).
	_, cmd := p.Update(panels.ItemCreateMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "New Worktree", modal.Title)
	assert.Equal(t, notify.ModalInput, modal.Kind)
}

// ---------------------------------------------------------------------------
// Prune missing worktrees
// ---------------------------------------------------------------------------

func TestPruneMissing(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	checker := selectiveMissing("/home/user/grut-fix")
	p := newTestPanel(t, mock, checker)
	p.Focus()

	_, cmd := p.Update(keyMsg('p'))
	require.NotNil(t, cmd)

	// Should produce confirmation modal.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Prune Worktrees", modal.Title)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)

	// Confirm prune.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	result := cmd()
	opResult, ok := result.(worktreeOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "pruned", opResult.op)
	assert.NoError(t, opResult.err)
	// Should have force-removed the missing worktree.
	assert.Equal(t, "/home/user/grut-fix", mock.lastRemPath)
	assert.True(t, mock.lastRemForce)
}

func TestPruneMissing_NoneToRemove(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists) // all exist
	p.Focus()

	_, cmd := p.Update(keyMsg('p'))
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Info, toast.Level)
	assert.Contains(t, toast.Message, "No missing worktrees")
}

// ---------------------------------------------------------------------------
// Refresh
// ---------------------------------------------------------------------------

func TestRefresh(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	_, cmd := p.Update(keyMsg('R'))
	require.NotNil(t, cmd, "R should trigger refresh command")
}

func TestWorktreeChangedMsg_TriggersRefresh(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	_, cmd := p.Update(panels.WorktreeChangedMsg{})
	require.NotNil(t, cmd, "WorktreeChangedMsg should trigger refresh")
}

// ---------------------------------------------------------------------------
// Key bindings
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	mock := &mockGitOps{}
	p := New(mock, testGitConfig(), "/repo")

	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify expected bindings are present.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["cursor_down"])
	assert.True(t, actions["cursor_up"])
	assert.True(t, actions["switch"])
	assert.True(t, actions["create"])
	assert.True(t, actions["item_delete"])
	assert.True(t, actions["refresh"])
	assert.True(t, actions["prune"])
}

// ---------------------------------------------------------------------------
// Worktree path helper
// ---------------------------------------------------------------------------

func TestWorktreePath(t *testing.T) {
	tests := []struct {
		repoRoot string
		branch   string
		want     string
	}{
		{
			repoRoot: "/home/user/myrepo",
			branch:   "feature/auth",
			want:     "feature-auth",
		},
		{
			repoRoot: "/home/user/myrepo",
			branch:   "main",
			want:     "main",
		},
		{
			repoRoot: "/home/user/myrepo",
			branch:   "fix/bug/123",
			want:     "fix-bug-123",
		},
	}

	for _, tt := range tests {
		path := worktreePath(tt.repoRoot, tt.branch)
		assert.Contains(t, path, ".worktrees")
		assert.Contains(t, path, "myrepo")
		assert.True(t, strings.HasSuffix(path, tt.want),
			"path %q should end with %q", path, tt.want)
	}
}

// ---------------------------------------------------------------------------
// Op result handling
// ---------------------------------------------------------------------------

func TestOpResult_CreatedEmitsChangedMsg(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	_, cmd := p.Update(worktreeOpResultMsg{op: "created", name: "feat", err: nil})
	require.NotNil(t, cmd)
	// The batched command should include WorktreeChangedMsg.
	// We can verify the operation completed successfully.
	_ = cmd()
}

func TestOpResult_Error(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)

	_, cmd := p.Update(worktreeOpResultMsg{op: "removed", name: "foo", err: errors.New("in use")})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "in use")
}

// ---------------------------------------------------------------------------
// Cursor clamping after reload
// ---------------------------------------------------------------------------

func TestCursorClamp_AfterRemoval(t *testing.T) {
	wts := []git.Worktree{
		{Path: "/wt/a", Branch: "a", Head: "aaa"},
		{Path: "/wt/b", Branch: "b", Head: "bbb"},
	}
	mock := &mockGitOps{worktrees: wts}
	p := newTestPanel(t, mock, alwaysExists)
	p.Focus()

	// Move cursor to last item.
	p.Update(keyMsg('j'))
	assert.Equal(t, 1, p.cursor)

	// Simulate reload with fewer items.
	mock.worktrees = []git.Worktree{
		{Path: "/wt/a", Branch: "a", Head: "aaa"},
	}
	cmd := p.loadWorktrees()
	msg := cmd()
	p.Update(msg)

	assert.Equal(t, 0, p.cursor, "cursor should clamp to last valid index")
}

// ---------------------------------------------------------------------------
// Mouse interaction
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsWorktree(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)

	// Cursor starts at 0.
	assert.Equal(t, 0, p.cursor)

	// Click on row 1.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)

	// Click on row 2.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)

	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)

	// Click beyond items.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 50, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "out-of-bounds click should not move cursor")
}

func TestMouseDoubleClick_SwitchesWorktree(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)
	p.Focus()

	// Double-click on row 1 (second worktree).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)
	// requestSwitch returns a command.
	assert.NotNil(t, cmd)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Mouse wheel scrolling
// ---------------------------------------------------------------------------

func TestMouseWheel_ScrollDown(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 1) // Only 1 visible row so scrolling is possible.

	assert.Equal(t, 0, p.offset)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	// 3 items, height 1 → maxOffset = 2. scrollDelta = 3, clamped to 2.
	assert.Equal(t, 2, p.offset)
}

func TestMouseWheel_ScrollUp(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 1)

	// Scroll down first to create room for scrolling up.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Greater(t, p.offset, 0)

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	// scrollDelta=3 subtracted from offset=2 → clamped to 0.
	assert.Equal(t, 0, p.offset)
}

func TestMouseWheel_ScrollDownClampsToMax(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20) // Height > items → maxOffset = 0.

	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Equal(t, 0, p.offset, "offset clamped when all items fit in viewport")
}

func TestMouseWheel_ScrollUpClampsToZero(t *testing.T) {
	mock := &mockGitOps{worktrees: sampleWorktrees()}
	p := newTestPanel(t, mock, alwaysExists)
	p.SetSize(80, 20)

	// offset starts at 0, scrolling up should stay at 0.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset)
}
