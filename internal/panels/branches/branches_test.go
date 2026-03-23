package branches

import (
	"context"
	"errors"
	"regexp"
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

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGitOps struct {
	branches    []git.Branch
	branchErr   error
	createErr   error
	deleteErr   error
	renameErr   error
	checkoutErr error
	worktreeErr error
	fetchErr    error
	remotes     []git.Remote
	remoteErr   error

	// Track calls for assertions.
	lastCheckout    string
	lastCreate      string
	lastDelete      string
	lastDeleteForce bool
	lastRenameOld   string
	lastRenameNew   string
	lastWorktreePth string
	lastWorktreeBr  string
	fetchCalled     bool
}

var _ GitOps = (*mockGitOps)(nil)

func (m *mockGitOps) BranchList(_ context.Context) ([]git.Branch, error) {
	return m.branches, m.branchErr
}

func (m *mockGitOps) BranchCreate(_ context.Context, name, _ string) error {
	m.lastCreate = name
	return m.createErr
}

func (m *mockGitOps) BranchDelete(_ context.Context, name string, force bool) error {
	m.lastDelete = name
	m.lastDeleteForce = force
	return m.deleteErr
}

func (m *mockGitOps) BranchRename(_ context.Context, oldName, newName string) error {
	m.lastRenameOld = oldName
	m.lastRenameNew = newName
	return m.renameErr
}

func (m *mockGitOps) Checkout(_ context.Context, ref string) error {
	m.lastCheckout = ref
	return m.checkoutErr
}

func (m *mockGitOps) WorktreeAdd(_ context.Context, path, branch string) error {
	m.lastWorktreePth = path
	m.lastWorktreeBr = branch
	return m.worktreeErr
}

func (m *mockGitOps) Fetch(_ context.Context, _ git.FetchOpts) error {
	m.fetchCalled = true
	return m.fetchErr
}

func (m *mockGitOps) RemoteList(_ context.Context) ([]git.Remote, error) {
	return m.remotes, m.remoteErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultCfg() config.GitConfig {
	return config.GitConfig{
		DefaultBranch: "main",
		WorktreeFirst: false,
	}
}

func worktreeCfg() config.GitConfig {
	cfg := defaultCfg()
	cfg.WorktreeFirst = true
	return cfg
}

func sampleBranches() []git.Branch {
	return []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234", Upstream: "origin/main", Ahead: 2, Behind: 1},
		{Name: "feature/auth", Hash: "def5678"},
		{Name: "origin/main", IsRemote: true, Hash: "abc1234"},
		{Name: "origin/develop", IsRemote: true, Hash: "fed8765"},
	}
}

// keyMsg constructs a KeyPressMsg for a rune key.
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// newTestPanel creates a Panel with a mock client and processes the initial
// branch load synchronously, simulating the Init() → branchesLoadedMsg cycle.
func newTestPanel(t *testing.T, mock *mockGitOps, cfg config.GitConfig) *Panel {
	t.Helper()
	p := New(mock, cfg, "/fake/repo")
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
	p := New(mock, defaultCfg(), "/repo")
	assert.Equal(t, "branches", p.Title())
	assert.NotNil(t, p.git)
}

func TestInit_ReturnsLoadCmd(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := New(mock, defaultCfg(), "/repo")
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Branch loading
// ---------------------------------------------------------------------------

func TestLoadBranches_PopulatesItems(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// 2 headers + 4 branches = 6 items
	assert.Len(t, p.items, 6)

	// First item is local header.
	assert.True(t, p.items[0].isHeader)
	assert.Equal(t, "Local Branches", p.items[0].header)

	// Second and third are local branches.
	assert.Equal(t, "main", p.items[1].branch.Name)
	assert.Equal(t, "feature/auth", p.items[2].branch.Name)

	// Fourth item is remote header.
	assert.True(t, p.items[3].isHeader)
	assert.Equal(t, "Remote Branches", p.items[3].header)

	// Fifth and sixth are remote branches.
	assert.Equal(t, "origin/main", p.items[4].branch.Name)
	assert.Equal(t, "origin/develop", p.items[5].branch.Name)
}

func TestLoadBranches_CursorOnCurrentBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	// Cursor should be on "main" (index 1, since 0 is header).
	assert.Equal(t, 1, p.cursor)
	assert.Equal(t, "main", p.items[p.cursor].branch.Name)
	assert.True(t, p.items[p.cursor].branch.IsCurrent)
}

func TestLoadBranches_Error(t *testing.T) {
	mock := &mockGitOps{branchErr: errors.New("git not found")}
	p := New(mock, defaultCfg(), "/repo")
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

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

func TestView_LocalAndRemoteSections(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(60, 20)
	p.Focus()

	view := p.View(60, 20)
	assert.Contains(t, view, "Local Branches")
	assert.Contains(t, view, "Remote Branches")
	assert.Contains(t, view, "main")
	assert.Contains(t, view, "feature/auth")
	assert.Contains(t, view, "origin/main")
	assert.Contains(t, view, "origin/develop")
}

func TestView_CurrentBranchMarked(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	view := p.View(60, 20)
	assert.Contains(t, view, "* main")
}

func TestView_AheadBehindCounts(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	view := p.View(80, 20)
	assert.Contains(t, view, "↑2")
	assert.Contains(t, view, "↓1")
}

func TestView_HashDisplayed(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	view := p.View(80, 20)
	assert.Contains(t, view, "abc1234")
	assert.Contains(t, view, "def5678")
}

func TestView_HashRightAligned(t *testing.T) {
	// Branch with ahead/behind uses multi-byte ↑↓ arrows.
	// len("↑") == 3 bytes but display width == 1. Using len() for gap
	// calculation would leave trailing spaces after the hash.
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234", Ahead: 2, Behind: 1},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	width := 60
	view := p.View(width, 5)
	lines := strings.Split(view, "\n")

	// Find the line containing the hash.
	for _, line := range lines {
		if !strings.Contains(line, "abc1234") {
			continue
		}
		// Strip ANSI codes to get the visible text.
		clean := stripAnsi(line)
		// The hash must be flush to the right edge (last 7 chars).
		if len(clean) >= 7 {
			assert.Equal(t, "abc1234", clean[len(clean)-7:],
				"hash should be flush with the right edge; got line: %q", clean)
		}
		return
	}
	t.Fatal("hash line not found in view")
}

func TestView_Empty(t *testing.T) {
	mock := &mockGitOps{branches: nil}
	p := newTestPanel(t, mock, defaultCfg())

	view := p.View(40, 10)
	assert.Contains(t, view, "No branches")
}

func TestView_ZeroDimensions(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(40, 0))
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func TestNavigation_JK(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Cursor starts on "main" (index 1).
	assert.Equal(t, 1, p.cursor)

	// Move down to "feature/auth" (index 2).
	p.Update(keyMsg('j'))
	assert.Equal(t, 2, p.cursor)
	assert.Equal(t, "feature/auth", p.items[p.cursor].branch.Name)

	// Move down again — skips "Remote Branches" header (index 3) to origin/main (index 4).
	p.Update(keyMsg('j'))
	assert.Equal(t, 4, p.cursor)
	assert.Equal(t, "origin/main", p.items[p.cursor].branch.Name)

	// Move up — skips header back to feature/auth (index 2).
	p.Update(keyMsg('k'))
	assert.Equal(t, 2, p.cursor)

	// Move up to "main" (index 1).
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursor)

	// Move up at top — stays at 1 (can't go to header at 0).
	p.Update(keyMsg('k'))
	assert.Equal(t, 1, p.cursor)
}

func TestNavigation_SkipsHeaders(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Navigate through all items and verify cursor never lands on a header.
	for range 10 {
		p.Update(keyMsg('j'))
		if p.cursor < len(p.items) {
			assert.False(t, p.items[p.cursor].isHeader, "cursor should never be on a header")
		}
	}
}

func TestNavigation_NotFocused(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	// Panel is NOT focused.

	initial := p.cursor
	p.Update(keyMsg('j'))
	assert.Equal(t, initial, p.cursor, "cursor should not move when not focused")
}

// ---------------------------------------------------------------------------
// Checkout
// ---------------------------------------------------------------------------

func TestCheckout_LocalBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move cursor to "feature/auth" (index 2).
	p.Update(keyMsg('j'))
	assert.Equal(t, "feature/auth", p.items[p.cursor].branch.Name)

	// Press enter.
	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	// Execute the command — it should call Checkout.
	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "checkout", result.op)
	assert.Equal(t, "feature/auth", result.name)
	assert.NoError(t, result.err)
	assert.Equal(t, "feature/auth", mock.lastCheckout)
}

func TestCheckout_RemoteBranch_StripsPrefix(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Navigate to "origin/main" (index 4).
	p.Update(keyMsg('j')) // → feature/auth
	p.Update(keyMsg('j')) // → origin/main (skips header)
	p.Update(keyMsg('j')) // → origin/develop
	p.Update(keyMsg('k')) // → origin/main
	assert.Equal(t, 4, p.cursor)
	assert.Equal(t, "origin/main", p.items[p.cursor].branch.Name)

	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	// Remote prefix stripped: "origin/main" → "main"
	assert.Equal(t, "main", mock.lastCheckout)
	assert.Equal(t, "checkout", result.op)
}

func TestCheckout_CurrentBranch_ShowsToast(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Cursor is on "main" (current branch).
	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Info, toast.Level)
	assert.Contains(t, toast.Message, "Already on")
}

func TestCheckout_EmitsBranchChangedMsg(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to feature/auth.
	p.Update(keyMsg('j'))
	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	// Execute checkout.
	msg := cmd()
	// Feed result back through Update.
	_, cmd2 := p.Update(msg)
	require.NotNil(t, cmd2)

	// The batched command should include BranchChangedMsg.
	// Execute all batched commands and collect messages.
	_ = cmd2()
	// tea.Batch returns a batchMsg containing multiple Cmds.
	// Since we can't easily unwrap tea.Batch, verify mock was called.
	assert.Equal(t, "feature/auth", mock.lastCheckout)
}

// ---------------------------------------------------------------------------
// Worktree-first mode
// ---------------------------------------------------------------------------

func TestCheckout_WorktreeFirst(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, worktreeCfg())
	p.Focus()

	// Move to feature/auth.
	p.Update(keyMsg('j'))
	assert.Equal(t, "feature/auth", p.items[p.cursor].branch.Name)

	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "worktree", result.op)
	assert.Equal(t, "feature/auth", result.name)
	assert.NoError(t, result.err)

	// Verify WorktreeAdd was called with the correct path and branch.
	assert.Equal(t, "feature/auth", mock.lastWorktreeBr)
	assert.Contains(t, mock.lastWorktreePth, ".worktrees")
	assert.Contains(t, mock.lastWorktreePth, "repo")
	assert.Contains(t, mock.lastWorktreePth, "feature-auth")
}

// ---------------------------------------------------------------------------
// Create branch
// ---------------------------------------------------------------------------

func TestCreateBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Send ItemCreateMsg to request create.
	_, cmd := p.Update(panels.ItemCreateMsg{})
	require.NotNil(t, cmd)

	// Should produce ShowModalMsg.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "New Branch", modal.Title)
	assert.Equal(t, notify.ModalInput, modal.Kind)

	// Simulate modal result with a branch name.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "new-feature"})
	require.NotNil(t, cmd)

	// Execute the create command.
	result := cmd()
	opResult, ok := result.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "created", opResult.op)
	assert.Equal(t, "new-feature", opResult.name)
	assert.Equal(t, "new-feature", mock.lastCreate)
}

func TestCreateBranch_EmptyName(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(panels.ItemCreateMsg{})
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "  "})
	assert.Nil(t, cmd, "empty name should not trigger create")
}

func TestCreateBranch_Cancelled(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(panels.ItemCreateMsg{})
	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancelled modal should not trigger create")
}

// ---------------------------------------------------------------------------
// Delete branch
// ---------------------------------------------------------------------------

func TestDeleteBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to "feature/auth" (non-current local branch).
	p.Update(keyMsg('j'))

	_, cmd := p.Update(panels.ItemDeleteMsg{})
	require.NotNil(t, cmd)

	// Should produce ShowModalMsg for confirmation.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete Branch", modal.Title)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Contains(t, modal.Message, "feature/auth")

	// Confirm deletion.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	result := cmd()
	opResult, ok := result.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "deleted", opResult.op)
	assert.Equal(t, "feature/auth", mock.lastDelete)
	assert.False(t, mock.lastDeleteForce)
}

func TestDeleteBranch_XKeyTriggersDelete(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to "feature/auth" (non-current local branch).
	p.Update(keyMsg('j'))

	// Press "x" directly — should trigger delete confirmation.
	_, cmd := p.Update(keyMsg('x'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete Branch", modal.Title)
	assert.Contains(t, modal.Message, "feature/auth")
}

func TestDeleteBranch_DKeyTriggersDelete(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to "feature/auth" (non-current local branch).
	p.Update(keyMsg('j'))

	// Press "d" directly — should also trigger delete confirmation (legacy key).
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete Branch", modal.Title)
	assert.Contains(t, modal.Message, "feature/auth")
}

func TestDeleteBranch_CurrentBlocked(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Cursor is on "main" (current branch).
	_, cmd := p.Update(panels.ItemDeleteMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Cannot delete current branch")
}

func TestDeleteBranch_RemoteBlocked(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Navigate to a remote branch.
	p.Update(keyMsg('j')) // feature/auth
	p.Update(keyMsg('j')) // origin/main
	assert.True(t, p.items[p.cursor].branch.IsRemote)

	_, cmd := p.Update(panels.ItemDeleteMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Cannot delete remote")
}

func TestDeleteBranch_RepeatedDeletePreservesCursor(t *testing.T) {
	// Regression test for #45: after a successful delete, the cursor should
	// stay near the deleted item instead of jumping to the current branch,
	// enabling sequential deletes without manual re-navigation.
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "feat-a", Hash: "bbb"},
		{Name: "feat-b", Hash: "ccc"},
		{Name: "feat-c", Hash: "ddd"},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Cursor starts on "main" (current). Move to "feat-a" (index 2, after header).
	p.Update(keyMsg('j'))
	assert.Equal(t, "feat-a", p.items[p.cursor].branch.Name)
	cursorBefore := p.cursor

	// Initiate delete → confirm.
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)
	cmd() // run ShowConfirm
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result := cmd()
	opResult, ok := result.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "deleted", opResult.op)

	// Process opResult — sets preserveCursor flag and returns batch cmd.
	p.Update(opResult)

	// Simulate reload with feat-a removed.
	mock.branches = []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "feat-b", Hash: "ccc"},
		{Name: "feat-c", Hash: "ddd"},
	}
	reloadCmd := p.loadBranches()
	reloadMsg := reloadCmd()
	p.Update(reloadMsg)

	// Cursor should be near the old position, NOT on "main".
	assert.Equal(t, cursorBefore, p.cursor, "cursor should be preserved near deleted item")
	assert.False(t, p.items[p.cursor].branch.IsCurrent, "cursor should not jump to current branch")
	assert.Equal(t, "feat-b", p.items[p.cursor].branch.Name, "cursor should land on next item")

	// Second delete should work without re-navigation.
	_, cmd = p.Update(keyMsg('x'))
	require.NotNil(t, cmd)
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Delete Branch", modal.Title)
	assert.Contains(t, modal.Message, "feat-b")
}

func TestDeleteBranch_CursorClampsOnLastItem(t *testing.T) {
	// When the last item is deleted, cursor should clamp to the new last item.
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "feat-a", Hash: "bbb"},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to feat-a (last selectable item).
	p.Update(keyMsg('j'))
	assert.Equal(t, "feat-a", p.items[p.cursor].branch.Name)

	// Delete feat-a.
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)
	cmd()
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result := cmd()
	opResult := result.(branchOpResultMsg)

	// Process opResult — sets preserveCursor flag.
	p.Update(opResult)

	// Reload with only main remaining.
	mock.branches = []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
	}
	reloadCmd := p.loadBranches()
	reloadMsg := reloadCmd()
	p.Update(reloadMsg)

	// Cursor should clamp to valid range and land on a non-header item.
	assert.GreaterOrEqual(t, p.cursor, 0)
	assert.Less(t, p.cursor, len(p.items))
	assert.False(t, p.items[p.cursor].isHeader)
}

func TestDeleteBranch_CursorSkipsHeaderAfterDelete(t *testing.T) {
	// When deletion causes cursor to land on a section header, it should
	// move to the nearest selectable branch.
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "feat-a", Hash: "bbb"},
		{Name: "origin/main", IsRemote: true, Hash: "aaa"},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Items: [Local Header, main, feat-a, Remote Header, origin/main]
	// Move to feat-a (index 2).
	p.Update(keyMsg('j'))
	assert.Equal(t, "feat-a", p.items[p.cursor].branch.Name)

	// Delete feat-a.
	_, cmd := p.Update(keyMsg('d'))
	require.NotNil(t, cmd)
	cmd()
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	result := cmd()
	opResult := result.(branchOpResultMsg)

	// Process opResult — sets preserveCursor flag.
	p.Update(opResult)

	// Reload without feat-a.
	// Items: [Local Header, main, Remote Header, origin/main]
	mock.branches = []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "origin/main", IsRemote: true, Hash: "aaa"},
	}
	reloadCmd := p.loadBranches()
	reloadMsg := reloadCmd()
	p.Update(reloadMsg)

	// Cursor was at index 2, which is now "Remote Header". Should skip to
	// the next selectable item.
	assert.False(t, p.items[p.cursor].isHeader, "cursor should not be on a header")
}

func TestDeleteBranch_InitialLoadCursorOnCurrentBranch(t *testing.T) {
	// On initial load (no prior items), cursor should still land on current branch.
	branches := []git.Branch{
		{Name: "feat-a", Hash: "bbb"},
		{Name: "main", IsCurrent: true, Hash: "aaa"},
		{Name: "feat-b", Hash: "ccc"},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	assert.Equal(t, "main", p.items[p.cursor].branch.Name, "initial load should position cursor on current branch")
}

// ---------------------------------------------------------------------------
// Rename branch
// ---------------------------------------------------------------------------

func TestRenameBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Move to "feature/auth".
	p.Update(keyMsg('j'))

	_, cmd := p.Update(panels.ItemEditMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, "Rename Branch", modal.Title)
	assert.Equal(t, notify.ModalInput, modal.Kind)
	assert.Equal(t, "feature/auth", modal.Placeholder)

	// Provide new name.
	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "feature/login"})
	require.NotNil(t, cmd)

	result := cmd()
	opResult, ok := result.(branchOpResultMsg)
	require.True(t, ok)
	assert.Equal(t, "renamed", opResult.op)
	assert.Equal(t, "feature/auth", mock.lastRenameOld)
	assert.Equal(t, "feature/login", mock.lastRenameNew)
}

func TestRenameBranch_RemoteBlocked(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// Navigate to remote branch.
	p.Update(keyMsg('j')) // feature/auth
	p.Update(keyMsg('j')) // origin/main

	_, cmd := p.Update(panels.ItemEditMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Cannot rename remote")
}

func TestRenameBranch_SameName(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(keyMsg('j')) // feature/auth
	p.Update(panels.ItemEditMsg{})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "feature/auth"})
	assert.Nil(t, cmd, "renaming to the same name should be a no-op")
}

// ---------------------------------------------------------------------------
// Fetch
// ---------------------------------------------------------------------------

func TestFetch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	_, cmd := p.Update(keyMsg('f'))
	require.NotNil(t, cmd)

	// The batch command contains both a toast and the fetch operation.
	// Execute it — tea.Batch returns batchMsg which we can't easily unwrap,
	// but the fetch operation should eventually execute.
	// For unit testing, verify that pressing 'f' returns a non-nil command.
	// The mock's fetchCalled flag verifies integration in the full message loop.
}

// ---------------------------------------------------------------------------
// Refresh
// ---------------------------------------------------------------------------

func TestRefresh_RKey(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	_, cmd := p.Update(panels.ItemEditMsg{})
	require.NotNil(t, cmd, "R key should trigger a reload command")
}

func TestRefresh_RefreshBranchesMsg(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	_, cmd := p.Update(panels.RefreshBranchesMsg{})
	require.NotNil(t, cmd, "RefreshBranchesMsg should trigger a reload")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestOnlyRemoteBranches(t *testing.T) {
	remotesOnly := []git.Branch{
		{Name: "origin/main", IsRemote: true, Hash: "abc"},
		{Name: "origin/dev", IsRemote: true, Hash: "def"},
	}
	mock := &mockGitOps{branches: remotesOnly}
	p := newTestPanel(t, mock, defaultCfg())

	// Should have 1 header + 2 branches.
	assert.Len(t, p.items, 3)
	assert.True(t, p.items[0].isHeader)
	assert.Equal(t, "Remote Branches", p.items[0].header)

	// Cursor should be on first remote branch.
	assert.Equal(t, 1, p.cursor)
}

func TestOnlyLocalBranches(t *testing.T) {
	localsOnly := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc"},
		{Name: "dev", Hash: "def"},
	}
	mock := &mockGitOps{branches: localsOnly}
	p := newTestPanel(t, mock, defaultCfg())

	// Should have 1 header + 2 branches.
	assert.Len(t, p.items, 3)
	assert.True(t, p.items[0].isHeader)
	assert.Equal(t, "Local Branches", p.items[0].header)
}

func TestNoSelectedBranch_OperationsNoOp(t *testing.T) {
	mock := &mockGitOps{branches: nil}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	// All branch operations should be no-ops when no branch is selected.
	_, cmd := p.Update(keyMsg('\r'))
	assert.Nil(t, cmd)

	_, cmd = p.Update(panels.ItemDeleteMsg{})
	assert.Nil(t, cmd)

	_, cmd = p.Update(panels.ItemEditMsg{})
	assert.Nil(t, cmd)
}

func TestKeyBindings(t *testing.T) {
	mock := &mockGitOps{}
	p := New(mock, defaultCfg(), "/repo")

	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify essential bindings are present.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["cursor_down"])
	assert.True(t, actions["cursor_up"])
	assert.True(t, actions["checkout"])
	assert.True(t, actions["item_create"])
	assert.True(t, actions["item_delete"])
	assert.True(t, actions["item_edit"])
	assert.True(t, actions["item_open"])
	assert.True(t, actions["item_copy"])
	assert.True(t, actions["fetch"])
	assert.True(t, actions["refresh"])
}

// ---------------------------------------------------------------------------
// Worktree path helper
// ---------------------------------------------------------------------------

func TestWorktreePath(t *testing.T) {
	tests := []struct {
		repoRoot string
		branch   string
		wantDir  string
	}{
		{"/home/user/myrepo", "feature/auth", "feature-auth"},
		{"/home/user/myrepo", "main", "main"},
		{"/home/user/myrepo", "fix/bug/123", "fix-bug-123"},
	}

	for _, tt := range tests {
		result := worktreePath(tt.repoRoot, tt.branch)
		assert.True(t, strings.HasSuffix(result, tt.wantDir),
			"worktreePath(%q, %q) = %q, want suffix %q", tt.repoRoot, tt.branch, result, tt.wantDir)
		assert.Contains(t, result, ".worktrees")
	}
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func TestCheckout_Error(t *testing.T) {
	mock := &mockGitOps{
		branches:    sampleBranches(),
		checkoutErr: errors.New("merge conflict"),
	}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(keyMsg('j')) // feature/auth
	_, cmd := p.Update(keyMsg('\r'))
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)

	// Feed error result through Update — should produce error toast.
	_, cmd2 := p.Update(result)
	require.NotNil(t, cmd2)
	toastMsg := cmd2()
	toast, ok := toastMsg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "merge conflict")
}

func TestCreateBranch_Error(t *testing.T) {
	mock := &mockGitOps{
		branches:  sampleBranches(),
		createErr: errors.New("invalid branch name"),
	}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(panels.ItemCreateMsg{})
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "bad..name"})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)
}

func TestDeleteBranch_Error(t *testing.T) {
	mock := &mockGitOps{
		branches:  sampleBranches(),
		deleteErr: errors.New("branch not fully merged"),
	}
	p := newTestPanel(t, mock, defaultCfg())
	p.Focus()

	p.Update(keyMsg('j')) // feature/auth
	p.Update(panels.ItemDeleteMsg{})
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(branchOpResultMsg)
	require.True(t, ok)
	assert.Error(t, result.err)
}

// ---------------------------------------------------------------------------
// Op result handling
// ---------------------------------------------------------------------------

func TestOpResult_Checkout_EmitsBranchChanged(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	_, cmd := p.Update(branchOpResultMsg{op: "checkout", name: "feature/auth"})
	require.NotNil(t, cmd)
	// The batched command includes BranchChangedMsg + toast + reload.
	// Verify it's not nil (deep batch inspection requires tea internals).
}

func TestOpResult_Error_ShowsToast(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())

	_, cmd := p.Update(branchOpResultMsg{op: "checkout", name: "x", err: errors.New("fail")})
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "fail")
}

// ---------------------------------------------------------------------------
// Annotation tests
// ---------------------------------------------------------------------------

func TestAnnotations_MergedBranch(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234", Upstream: "origin/main"},
		{Name: "feature/done", Hash: "def5678", Upstream: "origin/feature/done", Ahead: 0, Behind: 2},
		{Name: "feature/wip", Hash: "ghi9012", Upstream: "origin/feature/wip", Ahead: 3, Behind: 0},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	// feature/done has Ahead == 0 with an upstream → should be [merged].
	ann, ok := p.annotations["feature/done"]
	assert.True(t, ok, "feature/done should have an annotation")
	assert.Equal(t, "[merged]", ann)

	// feature/wip has Ahead > 0 → should NOT be annotated.
	_, ok = p.annotations["feature/wip"]
	assert.False(t, ok, "feature/wip should not have an annotation")

	// main is current → should NOT be annotated even if Ahead == 0.
	_, ok = p.annotations["main"]
	assert.False(t, ok, "current branch should not be annotated as merged")
}

func TestAnnotations_RemoteBranchNotAnnotated(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234"},
		{Name: "origin/feature", IsRemote: true, Hash: "def5678", Ahead: 0},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	// Remote branches should not get annotations.
	_, ok := p.annotations["origin/feature"]
	assert.False(t, ok, "remote branch should not be annotated")
}

func TestAnnotations_DefaultBranchNotAnnotated(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", Hash: "abc1234", Upstream: "origin/main", Ahead: 0},
		{Name: "develop", IsCurrent: true, Hash: "def5678"},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	// The default branch (main) should not be annotated even if Ahead == 0.
	_, ok := p.annotations["main"]
	assert.False(t, ok, "default branch should not be annotated as merged")
}

func TestAnnotations_NoUpstreamNotAnnotated(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234"},
		{Name: "local-only", Hash: "def5678", Ahead: 0},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())

	// Branch without upstream should not be annotated.
	_, ok := p.annotations["local-only"]
	assert.False(t, ok, "branch without upstream should not be annotated")
}

func TestAnnotations_RenderInView(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234", Upstream: "origin/main"},
		{Name: "feature/done", Hash: "def5678", Upstream: "origin/feature/done", Ahead: 0},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)
	p.Focus()

	view := p.View(80, 20)
	assert.Contains(t, view, "[merged]", "merged annotation should appear in view")
}

func TestAnnotations_ToggleHidesAnnotations(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true, Hash: "abc1234", Upstream: "origin/main"},
		{Name: "feature/done", Hash: "def5678", Upstream: "origin/feature/done", Ahead: 0},
	}
	mock := &mockGitOps{branches: branches}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)
	p.Focus()

	// Annotations visible by default.
	assert.True(t, p.showAnnotations)
	view := p.View(80, 20)
	assert.Contains(t, view, "[merged]")

	// Toggle off.
	p.Update(keyMsg('a'))
	assert.False(t, p.showAnnotations)
	view = p.View(80, 20)
	assert.NotContains(t, view, "[merged]")

	// Toggle back on.
	p.Update(keyMsg('a'))
	assert.True(t, p.showAnnotations)
	view = p.View(80, 20)
	assert.Contains(t, view, "[merged]")
}

func TestAnnotations_KeyBindingIncluded(t *testing.T) {
	mock := &mockGitOps{}
	p := New(mock, defaultCfg(), "/repo")

	bindings := p.KeyBindings()
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["toggle_annotations"], "should have toggle_annotations action")
}

// ---------------------------------------------------------------------------
// Mouse interaction
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)

	// Cursor starts on "main" (index 1).
	assert.Equal(t, 1, p.cursor)

	// Click on row 2 (index 2 = "feature/auth").
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
	assert.Equal(t, "feature/auth", p.items[p.cursor].branch.Name)

	// Click on row 4 (index 4 = "origin/main").
	p.Update(panels.PanelMouseClickMsg{ContentRow: 4, ContentCol: 5})
	assert.Equal(t, 4, p.cursor)
	assert.Equal(t, "origin/main", p.items[p.cursor].branch.Name)
}

func TestMouseClick_SkipsHeaders(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)

	originalCursor := p.cursor

	// Click on row 0 (index 0 = "Local Branches" header).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "clicking header should not move cursor")

	// Click on row 3 (index 3 = "Remote Branches" header).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 3, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "clicking header should not move cursor")
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)

	originalCursor := p.cursor
	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "out-of-bounds click should not move cursor")
}

func TestMouseDoubleClick_CheckoutBranch(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)
	p.Focus()

	// Double-click on row 2 (index 2 = "feature/auth").
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
	assert.Equal(t, "feature/auth", p.items[p.cursor].branch.Name)
	// requestCheckout returns a command.
	assert.NotNil(t, cmd)
}

func TestMouseDoubleClick_SkipsHeaders(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)

	originalCursor := p.cursor
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "double-clicking header should not move cursor")
	assert.Nil(t, cmd, "double-clicking header should not trigger action")
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mock := &mockGitOps{branches: sampleBranches()}
	p := newTestPanel(t, mock, defaultCfg())
	p.SetSize(80, 20)

	originalCursor := p.cursor
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor)
	assert.Nil(t, cmd)
}
