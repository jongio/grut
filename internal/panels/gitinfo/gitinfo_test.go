package gitinfo

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock gitOps
// ---------------------------------------------------------------------------

type mockGitOps struct {
	branches     []git.Branch
	worktrees    []git.Worktree
	remotes      []git.Remote
	stashes      []git.StashEntry
	tags         []git.Tag
	submodules   []git.Submodule
	pullFunc     func(context.Context, git.PullOpts) error
	pushFunc     func(context.Context, git.PushOpts) error
	fetches      []git.FetchOpts
	checkouts    []string
	status       []git.FileStatus
	worktreeAdds []struct {
		path   string
		branch string
	}
	branchErr error
}

type countingGitOps struct {
	mockGitOps
	branchCalls    int
	worktreeCalls  int
	remoteCalls    int
	stashCalls     int
	tagCalls       int
	reflogCalls    int
	submoduleCalls int
}

func (m *mockGitOps) BranchList(_ context.Context) ([]git.Branch, error) {
	return m.branches, m.branchErr
}
func (m *mockGitOps) BranchCreate(_ context.Context, _, _ string) error { return nil }
func (m *mockGitOps) BranchDelete(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockGitOps) BranchRename(_ context.Context, _, _ string) error { return nil }
func (m *mockGitOps) Checkout(_ context.Context, ref string) error {
	m.checkouts = append(m.checkouts, ref)
	return nil
}

func (m *mockGitOps) Status(_ context.Context) ([]git.FileStatus, error) {
	return m.status, nil
}
func (m *mockGitOps) StashPush(_ context.Context, _ git.StashOpts) error { return nil }
func (m *mockGitOps) WorktreeList(_ context.Context) ([]git.Worktree, error) {
	return m.worktrees, nil
}

func (m *mockGitOps) WorktreeAdd(_ context.Context, path, branch string) error {
	m.worktreeAdds = append(m.worktreeAdds, struct {
		path   string
		branch string
	}{path: path, branch: branch})
	return nil
}
func (m *mockGitOps) WorktreeRemove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockGitOps) RemoteList(_ context.Context) ([]git.Remote, error) {
	return m.remotes, nil
}
func (m *mockGitOps) RemoteAdd(_ context.Context, _, _ string) error { return nil }
func (m *mockGitOps) RemoteRemove(_ context.Context, _ string) error { return nil }
func (m *mockGitOps) Fetch(_ context.Context, opts git.FetchOpts) error {
	m.fetches = append(m.fetches, opts)
	return nil
}

func (m *mockGitOps) Pull(ctx context.Context, opts git.PullOpts) error {
	if m.pullFunc != nil {
		return m.pullFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitOps) Push(ctx context.Context, opts git.PushOpts) error {
	if m.pushFunc != nil {
		return m.pushFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitOps) StashList(_ context.Context) ([]git.StashEntry, error) {
	return m.stashes, nil
}
func (m *mockGitOps) StashApply(_ context.Context, _ int) error { return nil }
func (m *mockGitOps) StashPop(_ context.Context, _ int) error   { return nil }
func (m *mockGitOps) StashDrop(_ context.Context, _ int) error  { return nil }
func (m *mockGitOps) TagList(_ context.Context) ([]git.Tag, error) {
	return m.tags, nil
}
func (m *mockGitOps) TagCreate(_ context.Context, _, _, _ string) error { return nil }
func (m *mockGitOps) TagDelete(_ context.Context, _ string) error       { return nil }
func (m *mockGitOps) TagPush(_ context.Context, _, _ string) error      { return nil }
func (m *mockGitOps) Reflog(_ context.Context, _ string, _ int) ([]git.ReflogEntry, error) {
	return nil, nil
}

func (m *mockGitOps) Submodules(_ context.Context) ([]git.Submodule, error) {
	return m.submodules, nil
}

func (m *countingGitOps) BranchList(ctx context.Context) ([]git.Branch, error) {
	m.branchCalls++
	return m.mockGitOps.BranchList(ctx)
}

func (m *countingGitOps) WorktreeList(ctx context.Context) ([]git.Worktree, error) {
	m.worktreeCalls++
	return m.mockGitOps.WorktreeList(ctx)
}

func (m *countingGitOps) RemoteList(ctx context.Context) ([]git.Remote, error) {
	m.remoteCalls++
	return m.mockGitOps.RemoteList(ctx)
}

func (m *countingGitOps) StashList(ctx context.Context) ([]git.StashEntry, error) {
	m.stashCalls++
	return m.mockGitOps.StashList(ctx)
}

func (m *countingGitOps) TagList(ctx context.Context) ([]git.Tag, error) {
	m.tagCalls++
	return m.mockGitOps.TagList(ctx)
}

func (m *countingGitOps) Reflog(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
	m.reflogCalls++
	return m.mockGitOps.Reflog(ctx, ref, limit)
}

func (m *countingGitOps) Submodules(ctx context.Context) ([]git.Submodule, error) {
	m.submoduleCalls++
	return m.mockGitOps.Submodules(ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestPanel(t *testing.T, mock *mockGitOps) *Panel {
	t.Helper()
	p := New(mock, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.lastWidth = 200 // wide enough that tab labels are never abbreviated
	cmd := p.Init(t.Context())
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}
	return p
}

// newTestGitHubPanel creates a Panel in ModeGitHub (only GitHub tabs visible).
func newTestGitHubPanel(t *testing.T, mock *mockGitOps) *Panel {
	t.Helper()
	p := NewGitHub(mock, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	p.lastWidth = 200 // wide enough that tab labels are never abbreviated
	cmd := p.Init(t.Context())
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}
	return p
}

// confirmedAllActions returns an ActionsConfig with all gitinfo item types
// pre-confirmed so that existing tests bypass the first-use prompt.
func confirmedAllActions() config.ActionsConfig {
	return config.ActionsConfig{
		Confirmed: map[string]bool{
			string(actions.ItemLocalBranch):  true,
			string(actions.ItemRemoteBranch): true,
			string(actions.ItemWorktree):     true,
			string(actions.ItemRemote):       true,
			string(actions.ItemStashEntry):   true,
			string(actions.ItemIssue):        true,
			string(actions.ItemPR):           true,
			string(actions.ItemActionRun):    true,
			string(actions.ItemTag):          true,
		},
	}
}

func defaultMock() *mockGitOps {
	return &mockGitOps{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, Hash: "abc1234"},
			{Name: "feature", Hash: "def5678"},
			{Name: "origin/main", IsRemote: true, Hash: "abc1234"},
		},
		worktrees: []git.Worktree{
			{Path: "/test/repo", Branch: "main", Head: "abc1234567890"},
			{Path: "/test/.worktrees/repo/feature", Branch: "feature", Head: "def5678901234"},
		},
		remotes: []git.Remote{
			{Name: "origin", FetchURL: "https://github.com/user/repo", PushURL: "https://github.com/user/repo"},
		},
		stashes: []git.StashEntry{
			{Index: 0, Message: "WIP on main"},
			{Index: 1, Message: "experimental change"},
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewPanel(t *testing.T) {
	p := New(&mockGitOps{}, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, config.ActionsConfig{}, "/test/repo", "ascii", nil)
	assert.Equal(t, "Git", p.Title())
}

func TestPanelImplementsInterface(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

func TestInitialLoad(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	// Branches tab should have 2 items (local only — ModeGit filters out remote).
	assert.Equal(t, 2, len(p.tabItems[tabBranches]))

	// Worktrees tab should have 2 items.
	assert.Equal(t, 2, len(p.tabItems[tabWorktrees]))

	// Remotes tab should have 2 items (1 remote + 1 fetch URL sub).
	assert.Equal(t, 2, len(p.tabItems[tabRemotes]))

	// Stash tab should have 2 items.
	assert.Equal(t, 2, len(p.tabItems[tabStash]))

	// Default tab is branches, cursor on current branch ("main").
	assert.Equal(t, tabBranches, p.activeTab)
	branchItems := p.tabItems[tabBranches]
	assert.Equal(t, kindLocalBranch, branchItems[p.tabCursor[tabBranches]].kind)
	assert.Equal(t, "main", branchItems[p.tabCursor[tabBranches]].branch.Name)
	assert.True(t, branchItems[p.tabCursor[tabBranches]].branch.IsCurrent)
}

func TestCursorNavigation(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	initial := p.tabCursor[tabBranches]

	// Move down.
	p.moveCursorDown()
	assert.Greater(t, p.tabCursor[tabBranches], initial)

	// Move back up.
	p.moveCursorUp()
	assert.Equal(t, initial, p.tabCursor[tabBranches])
}

func TestTabSwitching(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	assert.Equal(t, tabBranches, p.activeTab)

	// Use SetActiveTab (b/w/r/s are now global app-level shortcuts).
	p.SetActiveTab("worktrees")
	assert.Equal(t, tabWorktrees, p.activeTab)

	p.SetActiveTab("remotes")
	assert.Equal(t, tabRemotes, p.activeTab)

	p.SetActiveTab("stash")
	assert.Equal(t, tabStash, p.activeTab)

	p.SetActiveTab("branches")
	assert.Equal(t, tabBranches, p.activeTab)
}

func TestDataRefreshOnMessages(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	// BranchChangedMsg should trigger reload.
	_, cmd := p.Update(panels.BranchChangedMsg{Name: "feature"})
	assert.NotNil(t, cmd)

	// WorktreeChangedMsg should trigger reload.
	_, cmd = p.Update(panels.WorktreeChangedMsg{})
	assert.NotNil(t, cmd)

	// RemoteChangedMsg should trigger reload.
	_, cmd = p.Update(panels.RemoteChangedMsg{})
	assert.NotNil(t, cmd)

	// RefreshBranchesMsg should trigger reload.
	_, cmd = p.Update(panels.RefreshBranchesMsg{})
	assert.NotNil(t, cmd)

	// StashChangedMsg should trigger reload.
	_, cmd = p.Update(panels.StashChangedMsg{})
	assert.NotNil(t, cmd)
}

func TestEmptyData(t *testing.T) {
	mock := &mockGitOps{}
	p := newTestPanel(t, mock)

	// All tabs should have zero items.
	assert.Equal(t, 0, len(p.tabItems[tabBranches]))
	assert.Equal(t, 0, len(p.tabItems[tabWorktrees]))
	assert.Equal(t, 0, len(p.tabItems[tabRemotes]))
	assert.Equal(t, 0, len(p.tabItems[tabStash]))
}

func TestViewRendering(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	view := p.View(60, 20)
	assert.NotEmpty(t, view)
	// Branch names should appear in the rendered output.
	assert.Contains(t, view, "main")
	assert.Contains(t, view, "feature")
}

func TestViewEmptyDimensions(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(-1, -1))
}

func TestKeyBindings(t *testing.T) {
	p := New(&mockGitOps{}, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, config.ActionsConfig{}, "/test/repo", "ascii", nil)
	bindings := p.KeyBindings()
	require.NotEmpty(t, bindings)

	// Check that expected bindings exist.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["cursor_down"])
	assert.True(t, actions["tab_branches"])
	assert.True(t, actions["tab_worktrees"])
	assert.True(t, actions["tab_remotes"])
	assert.True(t, actions["tab_stash"])
	assert.True(t, actions["item_copy"])
}

func TestStashTabSwitchViaKey(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	assert.Equal(t, tabBranches, p.activeTab)

	// Press 's' to switch to stash tab.
	p.Update(tea.KeyPressMsg{Code: 's'})
	assert.Equal(t, tabStash, p.activeTab)

	// Stash tab should have the expected items.
	assert.Equal(t, 2, len(p.tabItems[tabStash]))
	assert.Equal(t, kindStashEntry, p.tabItems[tabStash][0].kind)
	assert.Equal(t, "WIP on main", p.tabItems[tabStash][0].stash.Message)
	assert.Equal(t, 0, p.tabItems[tabStash][0].stash.Index)
	assert.Equal(t, "experimental change", p.tabItems[tabStash][1].stash.Message)
	assert.Equal(t, 1, p.tabItems[tabStash][1].stash.Index)
}

func TestStashTabRendering(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	// Render with branches as active tab — stash tab label rendered as inactive.
	// At width 60 the tab bar abbreviates names (full bar width > 60).
	view := p.View(60, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "St 2")

	// Switch to stash tab and verify content items render.
	p.activeTab = tabStash
	view = p.View(60, 20)
	assert.Contains(t, view, "stash@{0}")
	assert.Contains(t, view, "WIP on main")
	assert.Contains(t, view, "stash@{1}")
	assert.Contains(t, view, "experimental change")
}

func TestKeyNavigationIntegration(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	// Record initial cursor.
	initial := p.tabCursor[tabBranches]

	// Press 'j' to move down.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Greater(t, p.tabCursor[tabBranches], initial)

	// Press 'k' to move up.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, initial, p.tabCursor[tabBranches])

	// Press 'g' to go to first.
	p.Update(tea.KeyPressMsg{Code: 'g'})
	assert.Equal(t, 0, p.tabCursor[tabBranches])

	// Press 'G' to go to last.
	p.Update(tea.KeyPressMsg{Code: -1, Text: "G"})
	assert.Equal(t, len(p.tabItems[tabBranches])-1, p.tabCursor[tabBranches])
}

func TestLoadError(t *testing.T) {
	mock := &mockGitOps{
		branchErr: assert.AnError,
	}
	p := New(mock, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, config.ActionsConfig{}, "/test/repo", "ascii", nil)
	cmd := p.Init(t.Context())
	require.NotNil(t, cmd)

	msg := cmd()
	_, resultCmd := p.Update(msg)
	// Should produce a toast error.
	require.NotNil(t, resultCmd)
}

func TestHashAlwaysShown(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)

	// Render a branch line at a narrow width.
	view := p.View(30, 10)
	// Hash should be visible in the output.
	assert.Contains(t, view, "abc1234")
}

func TestPerTabCursorPreservation(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	// Move cursor down in branches tab.
	p.moveCursorDown()
	branchCursor := p.tabCursor[tabBranches]

	// Switch to worktrees tab.
	p.activeTab = tabWorktrees
	assert.Equal(t, 0, p.tabCursor[tabWorktrees])

	// Switch back — branches cursor should be preserved.
	p.activeTab = tabBranches
	assert.Equal(t, branchCursor, p.tabCursor[tabBranches])
}

// ---------------------------------------------------------------------------
// Mouse interaction
// ---------------------------------------------------------------------------

func TestMouseClick_TabBarSwitchesTabs(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)

	// Initially on branches tab.
	assert.Equal(t, tabBranches, p.activeTab)

	// Click on worktrees tab label area.
	// Tab bar: " Branches 3 · Worktrees 2 · Remotes 1 · Stash 2"
	// " Branches 3" is 11 chars (pos 1-11), " · " separator (3 chars) -> worktrees starts at 15
	// Click row 0, col 16 should land in worktrees label.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 16})
	assert.Equal(t, tabWorktrees, p.activeTab)
}

func TestMouseClick_ContentRowSelectsItem(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)

	// branches tab has 2 items (local only in ModeGit). Cursor starts on current branch.
	assert.Equal(t, tabBranches, p.activeTab)

	// Click on content row 1 (item index 0).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.tabCursor[tabBranches])

	// Click on content row 2 (item index 1 = "feature").
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 1, p.tabCursor[tabBranches])
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)

	cursor := p.tabCursor[tabBranches]

	// Click on row beyond available items.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 50, ContentCol: 5})
	assert.Equal(t, cursor, p.tabCursor[tabBranches], "out-of-bounds click should not move cursor")
}

func TestMouseDoubleClick_ContentRowTriggersAction(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)
	p.Focused = true

	// Double-click on content row 1 (item 0 = first branch).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.tabCursor[tabBranches])
	// doAction on a branch should return a command (checkout request).
	// The current branch ("main") is already current, so it shows a toast.
	assert.NotNil(t, cmd)
}

func TestMouseDoubleClick_TabBarIgnored_NoGitHub(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)

	// Double-click on tab bar row is always a no-op (tab switching is
	// handled by single-click; repo-open by PanelHeaderDoubleClickMsg).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Nil(t, cmd)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.SetSize(80, 20)

	cursor := p.tabCursor[tabBranches]
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, cursor, p.tabCursor[tabBranches])
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Header double-click opens repo in browser (#33, restricted in #39)
// ---------------------------------------------------------------------------

func TestHeaderDoubleClick_OpensRepoInBrowser(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = "myorg"
	p.gh.repo = "myrepo"
	p.SetSize(80, 20)

	// Stub the browser launcher so the test never opens a real browser
	// tab (xdg-open may not exist in headless/WSL environments).
	orig := panels.StartDetachedFn
	panels.StartDetachedFn = func(*exec.Cmd) error { return nil }
	t.Cleanup(func() { panels.StartDetachedFn = orig })

	// PanelHeaderDoubleClickMsg is sent by the layout engine when the user
	// double-clicks on the panel border title (e.g. "GitHub").
	_, cmd := p.Update(panels.PanelHeaderDoubleClickMsg{ContentCol: 0})
	require.NotNil(t, cmd, "header double-click with GitHub info should open browser")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Contains(t, toast.Message, "myorg/myrepo")
}

func TestHeaderDoubleClick_NoOwner_Noop(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = ""
	p.gh.repo = "repo"
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelHeaderDoubleClickMsg{ContentCol: 0})
	assert.Nil(t, cmd, "no ghOwner → no-op")
}

func TestHeaderDoubleClick_NoRepo_Noop(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = "owner"
	p.gh.repo = ""
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelHeaderDoubleClickMsg{ContentCol: 0})
	assert.Nil(t, cmd, "no ghRepo → no-op")
}

func TestMouseDoubleClick_TabBar_Noop_WithGitHub(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.SetSize(80, 20)

	// Even with ghOwner/ghRepo set, double-clicking on the tab bar row
	// should NOT open the repo (#39). Only header double-click does that.
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Nil(t, cmd, "tab bar double-click should be no-op even with GitHub info")
}

func TestOpenRepoInBrowser_ConstructsCorrectURL(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = "jongio"
	p.gh.repo = "grut"

	// Stub the browser launcher so the test never opens a real browser tab.
	orig := panels.StartDetachedFn
	panels.StartDetachedFn = func(*exec.Cmd) error { return nil }
	t.Cleanup(func() { panels.StartDetachedFn = orig })

	_, cmd := p.openRepoInBrowser()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "jongio/grut")
}

func TestMouseDoubleClick_ContentRow_StillWorks(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.SetSize(80, 20)
	p.Focused = true

	// Content area double-click (row 1 = first item) should still trigger
	// the item action, not the browser open.
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.tabCursor[tabBranches])
	assert.NotNil(t, cmd, "content double-click should trigger item action")
}

// ---------------------------------------------------------------------------
// Remotes count
// ---------------------------------------------------------------------------

func TestRemoteCount_SingleRemote(t *testing.T) {
	mock := defaultMock() // 1 remote, same fetch/push URL
	p := newTestPanel(t, mock)

	// tabItems includes the remote + its fetch sub-row.
	assert.Equal(t, 2, len(p.tabItems[tabRemotes]))
	// remoteCount should reflect actual number of remotes.
	assert.Equal(t, 1, p.remoteCount)
}

func TestRemoteCount_MultipleRemotes(t *testing.T) {
	mock := &mockGitOps{
		remotes: []git.Remote{
			{Name: "origin", FetchURL: "https://github.com/a/b", PushURL: "https://github.com/a/b"},
			{Name: "upstream", FetchURL: "https://github.com/c/d", PushURL: "git@github.com:c/d"},
		},
	}
	p := newTestPanel(t, mock)

	// origin = 2 items (remote + fetch), upstream = 3 items (remote + fetch + push)
	assert.Equal(t, 5, len(p.tabItems[tabRemotes]))
	// remoteCount should be 2.
	assert.Equal(t, 2, p.remoteCount)
}

func TestRemoteCount_RenderedInTabBar(t *testing.T) {
	mock := defaultMock() // 1 remote
	p := newTestPanel(t, mock)

	view := p.View(80, 20)
	// Tab bar should say "Remotes 1" not "Remotes 2".
	assert.Contains(t, view, "Remotes 1")
	assert.NotContains(t, view, "Remotes 2")
}

// ---------------------------------------------------------------------------
// Refresh key ('r' / 'R')
// ---------------------------------------------------------------------------

func TestRKey_SwitchesToRemotesTab(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	assert.Equal(t, tabBranches, p.activeTab)
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	// 'r' should switch to remotes tab in ModeGit.
	assert.Equal(t, tabRemotes, p.activeTab)
	require.NotNil(t, cmd, "'r' should produce a tab-selection command")
}

func TestKeyBindings_RRemotesAction(t *testing.T) {
	p := New(&mockGitOps{}, config.GitConfig{WorktreeOpenMode: "current"}, config.GitHubConfig{}, config.ActionsConfig{}, "/test/repo", "ascii", nil)
	bindings := p.KeyBindings()

	// 'r' should be documented as remotes/rerun, not refresh.
	for _, b := range bindings {
		if b.Key == "r" {
			assert.Contains(t, strings.ToLower(b.Description), "remotes")
			return
		}
	}
	t.Fatal("'r' key binding not found")
}

// ---------------------------------------------------------------------------
// GitHub test helpers
// ---------------------------------------------------------------------------

// setGHAvailable marks the panel as having a valid GitHub client.
// We use a trick: assign a non-nil interface value via unsafe-ish reflection,
// but the simplest approach is to use the panel's own handleGHDataLoaded
// to populate GitHub state.
func populateGH(p *Panel, issues []ghIssueItem, prs []ghPRItem, actions []ghActionItem) {
	p.gh.user = "testuser"
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.buildGitHubItems(issues, prs, actions, nil, nil)
}

func sampleIssues() []ghIssueItem {
	return []ghIssueItem{
		{Number: 1, Title: "Bug report", Body: "body1", State: "open", Labels: []string{"bug"}, Author: "testuser", Assignee: "testuser"},
		{Number: 2, Title: "Feature req", Body: "body2", State: "open", Labels: []string{"enhancement"}, Author: "other", Assignee: "other"},
		{Number: 3, Title: "Docs fix", Body: "body3", State: "open", Author: "testuser", Assignee: ""},
	}
}

func samplePRs() []ghPRItem {
	return []ghPRItem{
		{Number: 10, Title: "Add auth", State: "open", HeadBranch: "auth", Author: "testuser"},
		{Number: 11, Title: "Draft PR", State: "draft", HeadBranch: "draft-branch", Author: "other"},
		{Number: 12, Title: "Merged PR", State: "merged", HeadBranch: "merged-branch", Author: "other"},
	}
}

func sampleActions() []ghActionItem {
	return []ghActionItem{
		{RunID: 100, WorkflowName: "CI", RunNumber: 42, Status: "completed", Conclusion: "success", Branch: "main", CreatedAt: "Jan 1 10:00"},
		{RunID: 101, WorkflowName: "Deploy", RunNumber: 5, Status: "in_progress", Conclusion: "", Branch: "feature", CreatedAt: "Jan 2 11:00"},
	}
}

// ---------------------------------------------------------------------------
// 1. IssueFilterKind.String() and PRFilterKind.String()
// ---------------------------------------------------------------------------

func TestIssueFilterKind_String(t *testing.T) {
	tests := []struct {
		kind IssueFilterKind
		want string
	}{
		{issueFilterAll, "All"},
		{issueFilterAssigned, "Assigned"},
		{issueFilterCreated, "Created"},
		{IssueFilterKind(99), "All"}, // default
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.kind.String())
	}
}

func TestPRFilterKind_String(t *testing.T) {
	tests := []struct {
		kind PRFilterKind
		want string
	}{
		{prFilterAll, "All"},
		{prFilterNeedsReview, "Needs Review"},
		{prFilterMine, "Mine"},
		{prFilterDraft, "Draft"},
		{PRFilterKind(99), "All"}, // default
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.kind.String())
	}
}

// ---------------------------------------------------------------------------
// 2. handleGHDataLoaded
// ---------------------------------------------------------------------------

func TestHandleGHDataLoaded_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleGHDataLoaded(ghDataLoadedMsg{err: assert.AnError})
	assert.Nil(t, cmd) // error is stored on panel, no cmd returned
	assert.NotNil(t, p.gh.err)
}

func TestHandleGHDataLoaded_ValidData(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.user = ""
	issues := sampleIssues()
	prs := samplePRs()
	actions := sampleActions()

	_, cmd := p.handleGHDataLoaded(ghDataLoadedMsg{
		issues:  issues,
		prs:     prs,
		actions: actions,
		user:    "testuser",
	})
	// sampleActions contains an in_progress run → watch tick starts.
	assert.NotNil(t, cmd, "should start watch tick for in-progress runs")
	assert.True(t, p.actionsWatching, "should be watching with in-progress runs")
	assert.Equal(t, "testuser", p.gh.user)
	assert.Equal(t, len(issues), len(p.tabItems[tabIssues]))
	assert.Equal(t, len(prs), len(p.tabItems[tabPRs]))
	assert.Equal(t, len(actions), len(p.tabItems[tabActions]))
}

func TestHandleGHDataLoaded_EmptyData(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleGHDataLoaded(ghDataLoadedMsg{})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, len(p.tabItems[tabIssues]))
	assert.Equal(t, 0, len(p.tabItems[tabPRs]))
	assert.Equal(t, 0, len(p.tabItems[tabActions]))
}

// ---------------------------------------------------------------------------
// 3. handleOpResult — all operation types + error case
// ---------------------------------------------------------------------------

func TestHandleOpResult_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleOpResult(opResultMsg{op: "checkout", name: "main", err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "checkout error")
}

func TestHandleOpResult_AllOps(t *testing.T) {
	ops := []struct {
		op      string
		name    string
		contain string
	}{
		{"checkout", "feature", "Switched to feature"},
		{"branch_created", "new-branch", "Branch created: new-branch"},
		{"branch_deleted", "old-branch", "Branch deleted: old-branch"},
		{"branch_renamed", "renamed", "Branch renamed to: renamed"},
		{"worktree_added", "wt-branch", "Worktree created: wt-branch"},
		{"worktree_removed", "/path/wt", "Worktree removed: /path/wt"},
		{"worktree_switch", "/path/switch", ""},
		{"remote_added", "upstream", "Remote added: upstream"},
		{"remote_removed", "upstream", "Remote removed: upstream"},
		{"fetched", "origin", "Fetched: origin"},
		{"unknown_op", "something", "unknown_op: something"},
	}

	for _, tt := range ops {
		t.Run(tt.op, func(t *testing.T) {
			p := newTestPanel(t, defaultMock())
			_, cmd := p.handleOpResult(opResultMsg{op: tt.op, name: tt.name})
			require.NotNil(t, cmd, "op %s should return a command", tt.op)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. handlePRDetailsLoaded
// ---------------------------------------------------------------------------

func TestHandlePRDetailsLoaded_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handlePRDetailsLoaded(prDetailsLoadedMsg{number: 1, err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestHandlePRDetailsLoaded_ValidData(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	files := []panels.PRFile{{Filename: "main.go", Status: "modified", Additions: 10, Deletions: 2}}
	commits := []panels.PRCommit{{SHA: "abc123", Message: "fix bug", Author: "user", Date: "Jan 1 10:00"}}

	_, cmd := p.handlePRDetailsLoaded(prDetailsLoadedMsg{number: 42, files: files, commits: commits})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 5. handleActionJobsLoaded
// ---------------------------------------------------------------------------

func TestHandleActionJobsLoaded_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionJobsLoaded(actionJobsLoadedMsg{runID: 1, err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestHandleActionJobsLoaded_ValidJobs(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	jobs := []panels.ActionJob{
		{ID: 1, Name: "build", Status: "completed", Conclusion: "success"},
	}
	_, cmd := p.handleActionJobsLoaded(actionJobsLoadedMsg{runID: 100, jobs: jobs})
	require.NotNil(t, cmd)
}

func TestHandleActionJobsLoaded_FailedJobNoClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	// No ghClient set, so even with a failed job, no log-fetch Cmd is appended.
	jobs := []panels.ActionJob{
		{ID: 1, Name: "build", Status: "completed", Conclusion: "failure"},
	}
	_, cmd := p.handleActionJobsLoaded(actionJobsLoadedMsg{runID: 100, jobs: jobs})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 6. handleActionLogLoaded
// ---------------------------------------------------------------------------

func TestHandleActionLogLoaded_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionLogLoaded(actionLogLoadedMsg{runID: 1, jobID: 2, err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Logs:")
}

func TestHandleActionLogLoaded_ValidLog(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionLogLoaded(actionLogLoadedMsg{runID: 1, jobID: 2, log: "build output..."})
	require.NotNil(t, cmd)
	msg := cmd()
	logMsg, ok := msg.(panels.ActionLogMsg)
	require.True(t, ok)
	assert.Equal(t, int64(1), logMsg.RunID)
	assert.Equal(t, int64(2), logMsg.JobID)
	assert.Equal(t, "build output...", logMsg.Log)
}

// ---------------------------------------------------------------------------
// 7. handleActionRerunResult
// ---------------------------------------------------------------------------

func TestHandleActionRerunResult_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionRerunResult(actionRerunResultMsg{runID: 1, err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Rerun:")
}

func TestHandleActionRerunResult_Success(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionRerunResult(actionRerunResultMsg{runID: 1})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 8. handleActionCancelResult
// ---------------------------------------------------------------------------

func TestHandleActionCancelResult_Error(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionCancelResult(actionCancelResultMsg{runID: 1, err: assert.AnError})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Cancel:")
}

func TestHandleActionCancelResult_Success(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleActionCancelResult(actionCancelResultMsg{runID: 1})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 9. handleMouseWheel
// ---------------------------------------------------------------------------

func TestHandleMouseWheel_ScrollUp(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 20)
	p.tabOffset[tabBranches] = 2

	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.tabOffset[tabBranches], "should scroll up, clamped to 0")
}

func TestHandleMouseWheel_ScrollDown(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 5) // small height to allow scrolling

	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	// Offset should have increased (or stayed at 0 if items fit).
	assert.GreaterOrEqual(t, p.tabOffset[tabBranches], 0)
}

// ---------------------------------------------------------------------------
// 10. handleModalResult — all pending operations
// ---------------------------------------------------------------------------

func TestHandleModalResult_Accept_BranchCreate(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCreate
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "new-branch"})
	require.NotNil(t, cmd)
	assert.Equal(t, opNone, p.pending) // cleared
}

func TestHandleModalResult_Accept_BranchDelete(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchDelete
	p.pendingName = "feature"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Accept_BranchRename(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchRename
	p.pendingName = "old-name"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "new-name"})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Accept_BranchRename_SameName(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchRename
	p.pendingName = "same"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "same"})
	assert.Nil(t, cmd, "same name should be a no-op")
}

func TestHandleModalResult_Accept_WorktreeCreate(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opWorktreeCreate
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "wt-branch"})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Accept_WorktreeDelete(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opWorktreeDelete
	p.pendingName = "/path/wt"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Accept_RemoteAdd(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAdd
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "upstream"})
	require.NotNil(t, cmd)
	// Should now be in opRemoteAddURL state.
	assert.Equal(t, opRemoteAddURL, p.pending)
	assert.Equal(t, "upstream", p.pendingName)
}

func TestHandleModalResult_Accept_RemoteAddURL(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteAddURL
	p.pendingName = "upstream"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "https://github.com/user/repo"})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Accept_RemoteDelete(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opRemoteDelete
	p.pendingName = "origin"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
}

func TestHandleModalResult_Reject(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCreate
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_EmptyValue(t *testing.T) {
	ops := []pendingOp{opBranchCreate, opBranchRename, opWorktreeCreate, opRemoteAdd, opRemoteAddURL}
	for _, op := range ops {
		p := newTestPanel(t, defaultMock())
		p.pending = op
		p.pendingName = "old"
		_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: ""})
		assert.Nil(t, cmd, "empty value for op %d should be no-op", op)
	}
}

// ---------------------------------------------------------------------------
// 11. doAction — local branch (current/non-current), worktree, remote, OOB
// ---------------------------------------------------------------------------

func TestDoAction_CurrentBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	// Cursor is on "main" (current branch).
	_, cmd := p.doAction()
	require.NotNil(t, cmd, "current branch action should produce toast")
}

func TestDoAction_NonCurrentBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.tabCursor[tabBranches] = 1 // "feature"
	_, cmd := p.doAction()
	require.NotNil(t, cmd, "non-current branch should trigger checkout")
}

func TestDoAction_Worktree(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabWorktrees
	p.tabCursor[tabWorktrees] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd)
}

func TestExecuteRightClick_Worktree_OpenInEditor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	p.tabItems[tabWorktrees] = []listItem{
		{kind: kindWorktree, worktree: git.Worktree{Path: "/test/repo", Branch: "main"}},
	}
	p.tabCursor[tabWorktrees] = 0
	p.pendingPath = "/test/repo"

	// Stub the editor launcher so the test never spawns a real editor.
	orig := panels.StartDetachedFn
	panels.StartDetachedFn = func(*exec.Cmd) error { return nil }
	t.Cleanup(func() { panels.StartDetachedFn = orig })

	_, cmd := p.executeRightClickAction(actions.ActionOpenInEditor)
	require.NotNil(t, cmd)
	assert.Empty(t, p.pendingPath, "pendingPath should be cleared after opening editor")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Equal(t, notify.Success, toast.Level)
	assert.Contains(t, toast.Message, "/test/repo")
}

func TestDoAction_Remote(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd)
}

func TestDoAction_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.tabCursor[tabBranches] = 999
	_, cmd := p.doAction()
	assert.Nil(t, cmd)
}

func TestDoAction_Issue(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 1, Title: "Bug", HTMLURL: "https://github.com/o/r/issues/1"}},
	}
	p.tabCursor[tabIssues] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd, "issue with HTMLURL should produce open command")
}

func TestDoAction_IssueNoURL(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 1, Title: "Bug"}},
	}
	p.tabCursor[tabIssues] = 0
	_, cmd := p.doAction()
	assert.Nil(t, cmd, "issue without HTMLURL should be no-op")
}

func TestDoAction_PR(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Feature", State: "open", HTMLURL: "https://github.com/o/r/pull/10"}},
	}
	p.tabCursor[tabPRs] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd, "PR with HTMLURL should produce open command")
}

func TestDoAction_PRNoURL(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Feature", State: "open"}},
	}
	p.tabCursor[tabPRs] = 0
	_, cmd := p.doAction()
	assert.Nil(t, cmd, "PR without HTMLURL should be no-op")
}

func TestDoAction_ActionRun(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabActions
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{RunID: 1, RunNumber: 42, HTMLURL: "https://github.com/o/r/actions/runs/1"}},
	}
	p.tabCursor[tabActions] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd, "action run with HTMLURL should produce open command")
}

func TestDoAction_ActionRunNoURL(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabActions
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{RunID: 1, RunNumber: 42}},
	}
	p.tabCursor[tabActions] = 0
	_, cmd := p.doAction()
	assert.Nil(t, cmd, "action run without HTMLURL should be no-op")
}

// ---------------------------------------------------------------------------
// 12. doCreate — branches, worktrees, remotes, stash (no-op)
// ---------------------------------------------------------------------------

func TestDoCreate_Branches(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	_, cmd := p.doCreate()
	require.NotNil(t, cmd)
	assert.Equal(t, opBranchCreate, p.pending)
}

func TestDoCreate_Worktrees(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	_, cmd := p.doCreate()
	require.NotNil(t, cmd)
	assert.Equal(t, opWorktreeCreate, p.pending)
}

func TestDoCreate_Remotes(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	_, cmd := p.doCreate()
	require.NotNil(t, cmd)
	assert.Equal(t, opRemoteAdd, p.pending)
}

func TestDoCreate_Stash_NoOp(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabStash
	_, cmd := p.doCreate()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// 13. doDelete — local (current/non-current), remote branch, worktree, remote, OOB
// ---------------------------------------------------------------------------

func TestDoDelete_CurrentBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	// cursor on "main" (current)
	_, cmd := p.doDelete()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Cannot delete current branch")
}

func TestDoDelete_NonCurrentBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.tabCursor[tabBranches] = 1 // "feature"
	_, cmd := p.doDelete()
	require.NotNil(t, cmd)
	assert.Equal(t, opBranchDelete, p.pending)
	assert.Equal(t, "feature", p.pendingName)
}

func TestDoDelete_RemoteBranch(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 0 // "origin/main" (remote — only branch in ModeGitHub)
	_, cmd := p.doDelete()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Cannot delete remote branch")
}

func TestDoDelete_Worktree(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	p.tabCursor[tabWorktrees] = 0
	_, cmd := p.doDelete()
	require.NotNil(t, cmd)
	assert.Equal(t, opWorktreeDelete, p.pending)
}

func TestDoDelete_Remote(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 0
	_, cmd := p.doDelete()
	require.NotNil(t, cmd)
	assert.Equal(t, opRemoteDelete, p.pending)
	assert.Equal(t, "origin", p.pendingName)
}

func TestDoDelete_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.tabCursor[tabBranches] = 999
	_, cmd := p.doDelete()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// 14. doRename — local branch, remote branch, nil branch
// ---------------------------------------------------------------------------

func TestDoRename_LocalBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.tabCursor[tabBranches] = 1 // "feature" (local, non-current)
	_, cmd := p.doRename()
	require.NotNil(t, cmd)
	assert.Equal(t, opBranchRename, p.pending)
	assert.Equal(t, "feature", p.pendingName)
}

func TestDoRename_RemoteBranch(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 0 // "origin/main" (remote — only branch in ModeGitHub)
	_, cmd := p.doRename()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, "Cannot rename remote branch")
}

func TestDoRename_NilBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees // no branch at cursor
	_, cmd := p.doRename()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// 15. doFetch — with selected remote, without (fetch all)
// ---------------------------------------------------------------------------

func TestDoFetch_WithRemote(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 0 // "origin" remote
	_, cmd := p.doFetch()
	require.NotNil(t, cmd)
}

func TestDoFetch_FetchAll(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches // not on a remote
	_, cmd := p.doFetch()
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 16. buildItems — cursor defaults to current branch
// ---------------------------------------------------------------------------

func TestBuildItems_CursorOnCurrentBranch(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "alpha", Hash: "aaa"},
			{Name: "beta", Hash: "bbb"},
			{Name: "main", IsCurrent: true, Hash: "ccc"},
		},
	}
	p := newTestPanel(t, mock)
	// Current branch "main" is at index 2 in local branches.
	assert.Equal(t, 2, p.tabCursor[tabBranches])
	assert.Equal(t, "main", p.tabItems[tabBranches][p.tabCursor[tabBranches]].branch.Name)
}

// ---------------------------------------------------------------------------
// 16b. buildItems — mode-aware branch filtering (#46)
// ---------------------------------------------------------------------------

func TestBuildItems_ModeGitShowsOnlyLocalBranches(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, Hash: "aaa"},
			{Name: "feature", Hash: "bbb"},
			{Name: "origin/main", IsRemote: true, Hash: "ccc"},
			{Name: "origin/dev", IsRemote: true, Hash: "ddd"},
		},
	}
	p := newTestPanel(t, mock) // ModeGit

	assert.Equal(t, 2, len(p.tabItems[tabBranches]), "ModeGit should show only local branches")
	for _, item := range p.tabItems[tabBranches] {
		assert.Equal(t, kindLocalBranch, item.kind, "all branches in ModeGit should be local")
		assert.False(t, item.branch.IsRemote, "no remote branch should appear in ModeGit")
	}
}

func TestBuildItems_ModeGitHubShowsOnlyRemoteBranches(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, Hash: "aaa"},
			{Name: "feature", Hash: "bbb"},
			{Name: "origin/main", IsRemote: true, Hash: "ccc"},
			{Name: "origin/dev", IsRemote: true, Hash: "ddd"},
		},
	}
	p := newTestGitHubPanel(t, mock) // ModeGitHub

	assert.Equal(t, 2, len(p.tabItems[tabBranches]), "ModeGitHub should show only remote branches")
	for _, item := range p.tabItems[tabBranches] {
		assert.Equal(t, kindRemoteBranch, item.kind, "all branches in ModeGitHub should be remote")
		assert.True(t, item.branch.IsRemote, "no local branch should appear in ModeGitHub")
	}
}

func TestBuildItems_ModeAllShowsBothBranches(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, Hash: "aaa"},
			{Name: "feature", Hash: "bbb"},
			{Name: "origin/main", IsRemote: true, Hash: "ccc"},
			{Name: "origin/dev", IsRemote: true, Hash: "ddd"},
		},
	}
	p := newTestPanel(t, mock)
	p.mode = ModeAll
	p.doBuildItems()

	assert.Equal(t, 4, len(p.tabItems[tabBranches]), "ModeAll should show all branches")
	localCount := 0
	remoteCount := 0
	for _, item := range p.tabItems[tabBranches] {
		switch item.kind {
		case kindLocalBranch:
			localCount++
		case kindRemoteBranch:
			remoteCount++
		}
	}
	assert.Equal(t, 2, localCount, "ModeAll should include local branches")
	assert.Equal(t, 2, remoteCount, "ModeAll should include remote branches")
}

func TestBuildItems_GitHubPanelBranchCountReflectsFiltering(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{
			{Name: "main", IsCurrent: true, Hash: "aaa"},
			{Name: "feature", Hash: "bbb"},
			{Name: "origin/main", IsRemote: true, Hash: "ccc"},
		},
	}
	// Git panel: 2 local branches.
	pGit := newTestPanel(t, mock)
	assert.Equal(t, 2, len(pGit.tabItems[tabBranches]))

	// GitHub panel: 1 remote branch.
	pGH := newTestGitHubPanel(t, mock)
	assert.Equal(t, 1, len(pGH.tabItems[tabBranches]))
}

// ---------------------------------------------------------------------------
// 17. rebuildFromCurrent — cursor clamping when items shrink
// ---------------------------------------------------------------------------

func TestRebuildFromCurrent_CursorClamped(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.tabCursor[tabBranches] = 2 // beyond last item (ModeGit has 2 local branches)

	// Shrink branches.
	p.gitData.lastBranches = []git.Branch{{Name: "only", Hash: "xxx"}}
	p.rebuildFromCurrent()

	// Cursor should be clamped to len-1 = 0.
	assert.Equal(t, 0, p.tabCursor[tabBranches])
}

func TestRebuildFromCurrent_EmptyList(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.tabCursor[tabBranches] = 2

	p.gitData.lastBranches = nil
	p.rebuildFromCurrent()
	assert.Equal(t, 0, p.tabCursor[tabBranches])
}

// ---------------------------------------------------------------------------
// 18. renderLine — dispatching to correct render function for each kind
// ---------------------------------------------------------------------------

func TestRenderLine_AllKinds(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	width := 80

	tests := []struct {
		name string
		item listItem
	}{
		{"localBranch", listItem{kind: kindLocalBranch, branch: git.Branch{Name: "main", Hash: "abc"}}},
		{"remoteBranch", listItem{kind: kindRemoteBranch, branch: git.Branch{Name: "origin/main", IsRemote: true, Hash: "abc"}}},
		{"worktree", listItem{kind: kindWorktree, worktree: git.Worktree{Path: "/test/repo", Branch: "main", Head: "abc1234567"}}},
		{"remote", listItem{kind: kindRemote, remote: git.Remote{Name: "origin"}}},
		{"remoteSub", listItem{kind: kindRemoteSub, text: "fetch: https://example.com"}},
		{"stashEntry", listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "WIP"}}},
		{"issue", listItem{kind: kindIssue, issue: ghIssueItem{Number: 1, Title: "Bug"}}},
		{"pr", listItem{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Add feature", State: "open"}}},
		{"actionRun", listItem{kind: kindActionRun, actionRun: ghActionItem{RunID: 1, WorkflowName: "CI", RunNumber: 1, Status: "completed", Conclusion: "success"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := p.renderLine(tt.item, width, false)
			assert.NotEmpty(t, line)
		})
	}
}

func TestRenderLine_UnknownKind(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	line := p.renderLine(listItem{kind: itemKind(99)}, 80, false)
	assert.Empty(t, line)
}

// ---------------------------------------------------------------------------
// 19. renderIssue, renderPR, renderActionRun — content checks
// ---------------------------------------------------------------------------

func TestRenderIssue_Content(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindIssue, issue: ghIssueItem{Number: 42, Title: "Fix auth token", Labels: []string{"bug"}}}
	line := p.renderIssue(item, 80, false)
	assert.Contains(t, line, "#42")
	assert.Contains(t, line, "Fix auth token")
	assert.Contains(t, line, "bug")
}

func TestRenderPR_Content(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Add auth", State: "open"}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "#10")
	assert.Contains(t, line, "Add auth")
	assert.Contains(t, line, "open")
}

func TestRenderPR_DraftState(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{Number: 11, Title: "Draft PR", State: "draft"}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "draft")
}

func TestRenderPR_MergedState(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{Number: 12, Title: "Merged PR", State: "merged"}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "merged")
}

func TestRenderActionRun_Content(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 42, Status: "completed",
		Conclusion: "success", Branch: "main", CreatedAt: "Jan 1 10:00",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "CI")
	assert.Contains(t, line, "#42")
	assert.Contains(t, line, "main")
	assert.Contains(t, line, "✓")
}

func TestRenderActionRun_Failure(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 101, WorkflowName: "Deploy", RunNumber: 5, Conclusion: "failure",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "✗")
}

func TestRenderActionRun_InProgress(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 102, WorkflowName: "Test", RunNumber: 3, Status: "in_progress",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "●")
}

// ---------------------------------------------------------------------------
// 20. actionsStatusIcon — all cases
// ---------------------------------------------------------------------------

func TestActionsStatusIcon(t *testing.T) {
	tests := []struct {
		name    string
		actions []ghActionItem
		want    string
	}{
		{"empty", nil, "0"},
		{"success", []ghActionItem{{Conclusion: "success"}}, "✓"},
		{"failure", []ghActionItem{{Conclusion: "failure"}}, "✗"},
		{"timed_out", []ghActionItem{{Conclusion: "timed_out"}}, "✗"},
		{"in_progress", []ghActionItem{{Status: "in_progress"}}, "●"},
		{"queued", []ghActionItem{{Status: "queued"}}, "●"},
		{"count", []ghActionItem{{Conclusion: "neutral"}, {Conclusion: "neutral"}}, "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanel(t, defaultMock())
			p.tabItems[tabActions] = nil
			for _, a := range tt.actions {
				p.tabItems[tabActions] = append(p.tabItems[tabActions], listItem{kind: kindActionRun, actionRun: a})
			}
			assert.Equal(t, tt.want, p.actionsStatusIcon())
		})
	}
}

// ---------------------------------------------------------------------------
// 21. worktreePath
// ---------------------------------------------------------------------------

func TestWorktreePath(t *testing.T) {
	path := worktreePath("/home/user/repo", "feature/auth")
	// filepath.Join produces OS-native separators
	assert.Contains(t, path, ".worktrees")
	assert.Contains(t, path, "repo")
	assert.Contains(t, path, "feature-auth")
}

func TestWorktreePath_WindowsStyle(t *testing.T) {
	path := worktreePath("C:\\Users\\dev\\repo", "my-branch")
	assert.Contains(t, path, ".worktrees")
	assert.Contains(t, path, "repo")
	assert.Contains(t, path, "my-branch")
}

// ---------------------------------------------------------------------------
// remoteToHTTPS
// ---------------------------------------------------------------------------

func TestRemoteToHTTPS_SSH(t *testing.T) {
	assert.Equal(t, "https://github.com/user/repo", remoteToHTTPS("git@github.com:user/repo.git"))
}

func TestRemoteToHTTPS_HTTPS(t *testing.T) {
	assert.Equal(t, "https://github.com/user/repo", remoteToHTTPS("https://github.com/user/repo.git"))
}

func TestRemoteToHTTPS_HTTPS_NoGit(t *testing.T) {
	assert.Equal(t, "https://github.com/user/repo", remoteToHTTPS("https://github.com/user/repo"))
}

func TestRemoteToHTTPS_SSHProtocol(t *testing.T) {
	assert.Equal(t, "https://github.com/user/repo", remoteToHTTPS("ssh://git@github.com/user/repo.git"))
}

func TestRemoteToHTTPS_Empty(t *testing.T) {
	assert.Equal(t, "", remoteToHTTPS(""))
}

func TestRemoteToHTTPS_Unknown(t *testing.T) {
	assert.Equal(t, "", remoteToHTTPS("file:///local/path"))
}

// ---------------------------------------------------------------------------
// 22. cycleIssueFilter, cyclePRFilter
// ---------------------------------------------------------------------------

func TestCycleIssueFilter(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)

	assert.Equal(t, issueFilterAll, p.gh.issueFilter)

	p.cycleIssueFilter() // All -> Assigned
	assert.Equal(t, issueFilterAssigned, p.gh.issueFilter)

	p.cycleIssueFilter() // Assigned -> Created
	assert.Equal(t, issueFilterCreated, p.gh.issueFilter)

	p.cycleIssueFilter() // Created -> All
	assert.Equal(t, issueFilterAll, p.gh.issueFilter)
}

func TestCyclePRFilter(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, samplePRs(), nil)

	assert.Equal(t, prFilterAll, p.gh.prFilter)

	p.cyclePRFilter() // All -> NeedsReview
	assert.Equal(t, prFilterNeedsReview, p.gh.prFilter)

	p.cyclePRFilter() // NeedsReview -> Mine
	assert.Equal(t, prFilterMine, p.gh.prFilter)

	p.cyclePRFilter() // Mine -> Draft
	assert.Equal(t, prFilterDraft, p.gh.prFilter)

	p.cyclePRFilter() // Draft -> All
	assert.Equal(t, prFilterAll, p.gh.prFilter)
}

func TestCycleIssueFilter_FiltersItems(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)

	// All: all 3 issues.
	assert.Equal(t, 3, len(p.tabItems[tabIssues]))

	// Assigned: only issue 1 (assignee = testuser).
	p.cycleIssueFilter()
	assert.Equal(t, issueFilterAssigned, p.gh.issueFilter)
	assert.Equal(t, 1, len(p.tabItems[tabIssues]))

	// Created: issues by testuser (1 and 3).
	p.cycleIssueFilter()
	assert.Equal(t, issueFilterCreated, p.gh.issueFilter)
	assert.Equal(t, 2, len(p.tabItems[tabIssues]))
}

func TestCyclePRFilter_FiltersItems(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, samplePRs(), nil)

	// All: 3 PRs.
	assert.Equal(t, 3, len(p.tabItems[tabPRs]))

	// NeedsReview: open PRs not by testuser → #11 is draft, so only open non-authored.
	p.cyclePRFilter()
	// PR #11 (draft, other) → State is "draft", not "open", so doesn't match.
	// PR #12 (merged, other) → State is "merged", not "open", so doesn't match.
	// So 0 match NeedsReview.
	assert.Equal(t, 0, len(p.tabItems[tabPRs]))

	// Mine: authored by testuser → PR #10.
	p.cyclePRFilter()
	assert.Equal(t, 1, len(p.tabItems[tabPRs]))

	// Draft: state == "draft" → PR #11.
	p.cyclePRFilter()
	assert.Equal(t, 1, len(p.tabItems[tabPRs]))
}

// ---------------------------------------------------------------------------
// 23. matchesIssueFilter, matchesPRFilter
// ---------------------------------------------------------------------------

func TestMatchesIssueFilter_AllCases(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.user = "testuser"

	iss := ghIssueItem{Author: "testuser", Assignee: "testuser"}

	p.gh.issueFilter = issueFilterAll
	assert.True(t, p.matchesIssueFilter(iss))

	p.gh.issueFilter = issueFilterAssigned
	assert.True(t, p.matchesIssueFilter(iss))

	p.gh.issueFilter = issueFilterAssigned
	assert.False(t, p.matchesIssueFilter(ghIssueItem{Assignee: "other"}))

	p.gh.issueFilter = issueFilterCreated
	assert.True(t, p.matchesIssueFilter(iss))

	p.gh.issueFilter = issueFilterCreated
	assert.False(t, p.matchesIssueFilter(ghIssueItem{Author: "other"}))
}

func TestMatchesPRFilter_AllCases(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.user = "testuser"

	pr := ghPRItem{Author: "other", State: "open"}

	p.gh.prFilter = prFilterAll
	assert.True(t, p.matchesPRFilter(pr))

	p.gh.prFilter = prFilterNeedsReview
	assert.True(t, p.matchesPRFilter(pr))

	p.gh.prFilter = prFilterNeedsReview
	assert.False(t, p.matchesPRFilter(ghPRItem{Author: "testuser", State: "open"}))

	p.gh.prFilter = prFilterMine
	assert.False(t, p.matchesPRFilter(pr))
	assert.True(t, p.matchesPRFilter(ghPRItem{Author: "testuser"}))

	p.gh.prFilter = prFilterDraft
	assert.False(t, p.matchesPRFilter(pr))
	assert.True(t, p.matchesPRFilter(ghPRItem{State: "draft"}))
}

// ---------------------------------------------------------------------------
// 24. Tab-specific selection commands — returns nil when wrong tab
// ---------------------------------------------------------------------------

func TestBranchSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	assert.Nil(t, p.branchSelectedCmd())
}

func TestWorktreeSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.worktreeSelectedCmd())
}

func TestRemoteSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.remoteSelectedCmd())
}

func TestStashSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.stashSelectedCmd())
}

func TestIssueSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.issueSelectedCmd())
}

func TestPRSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.prSelectedCmd())
}

func TestActionRunSelectedCmd_WrongTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.actionRunSelectedCmd())
}

func TestBranchSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	cmd := p.branchSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.BranchSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "main", sel.Name)
}

func TestWorktreeSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	cmd := p.worktreeSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.WorktreeSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "/test/repo", sel.Path)
}

func TestRemoteSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	cmd := p.remoteSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.RemoteSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, "origin", sel.Name)
}

func TestStashSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabStash
	cmd := p.stashSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.StashSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, 0, sel.Index)
}

func TestIssueSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)
	p.activeTab = tabIssues
	cmd := p.issueSelectedCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(panels.IssueSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, 1, sel.Number)
	assert.Equal(t, "testuser", sel.Author)
	assert.Equal(t, "testuser", sel.Assignee)
	assert.Equal(t, []string{"bug"}, sel.Labels)
}

func TestPRSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, samplePRs(), nil)
	p.activeTab = tabPRs
	cmd := p.prSelectedCmd()
	require.NotNil(t, cmd)
}

func TestActionRunSelectedCmd_ValidCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	cmd := p.actionRunSelectedCmd()
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// 25. selectedBranch, selectedWorktree, selectedRemote — valid/wrong kind
// ---------------------------------------------------------------------------

func TestSelectedBranch_WrongKind(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	assert.Nil(t, p.selectedBranch())
}

func TestSelectedWorktree_WrongKind(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.selectedWorktree())
}

func TestSelectedRemote_WrongKind(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	assert.Nil(t, p.selectedRemote())
}

func TestSelectedRemote_OnRemoteSub(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 1 // fetch URL sub-item
	assert.Nil(t, p.selectedRemote())
}

// ---------------------------------------------------------------------------
// 26. handleGitHubTabBarClick — clicking each GitHub tab
// ---------------------------------------------------------------------------

func TestHandleGitHubTabBarClick_Issues(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.handleGitHubTabBarClick(1) // " Issues ..." starts at pos 1
	assert.Equal(t, tabIssues, p.activeTab)
}

func TestHandleGitHubTabBarClick_PRs(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	// PRs label starts after "Issues N" + " · " separator.
	issueLabel := len("Issues 3")
	prsStart := 1 + issueLabel + 3 // leading space + label + separator
	p.handleGitHubTabBarClick(prsStart)
	assert.Equal(t, tabPRs, p.activeTab)
}

func TestHandleGitHubTabBarClick_Actions(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	// Click far enough right to land in Actions.
	issueLabel := len("Issues 3")
	prsLabel := len("PRs 3")
	actionsStart := 1 + issueLabel + 3 + prsLabel + 3
	p.handleGitHubTabBarClick(actionsStart)
	assert.Equal(t, tabActions, p.activeTab)
}

func TestHandleGitHubTabBarClick_Abbreviated(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	// Force abbreviated mode by setting a narrow width.
	p.lastWidth = 30

	// With abbreviated labels: " Iss 3 · PRs 3 · Act ✓ · Wf 0 · Rel 0"
	// Positions: Iss starts at 1, length = len("Iss 3") = 5
	p.handleGitHubTabBarClick(1)
	assert.Equal(t, tabIssues, p.activeTab, "click on abbreviated Issues")

	// PRs starts at 1 + 5 + 3 = 9, length = len("PRs 3") = 5
	p.handleGitHubTabBarClick(9)
	assert.Equal(t, tabPRs, p.activeTab, "click on abbreviated PRs")

	// Actions starts at 9 + 5 + 3 = 17
	p.handleGitHubTabBarClick(17)
	assert.Equal(t, tabActions, p.activeTab, "click on abbreviated Actions")
}

// ---------------------------------------------------------------------------
// 27. ensureCursorVisible — cursor below and above viewport
// ---------------------------------------------------------------------------

func TestEnsureCursorVisible_Below(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 5) // small height → viewH = 4 (5 - 1 tab bar)
	p.tabCursor[tabBranches] = 2
	p.tabOffset[tabBranches] = 0
	p.ensureCursorVisible()
	// Cursor 2 should be visible within 4-line viewport starting at 0.
	assert.LessOrEqual(t, p.tabOffset[tabBranches], p.tabCursor[tabBranches])
}

func TestEnsureCursorVisible_Above(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 5)
	p.tabCursor[tabBranches] = 0
	p.tabOffset[tabBranches] = 2
	p.ensureCursorVisible()
	assert.Equal(t, 0, p.tabOffset[tabBranches])
}

func TestEnsureCursorVisible_ZeroHeight(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.SetSize(80, 0)
	// Should not panic.
	p.ensureCursorVisible()
}

// ---------------------------------------------------------------------------
// 28. githubPollTickCmd — nil when ghClient is nil or PollInterval <= 0
// ---------------------------------------------------------------------------

func TestGithubPollTickCmd_NilClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.cfg.PollInterval = 30
	// ghClient is nil by default.
	cmd := p.githubPollTickCmd()
	assert.Nil(t, cmd)
}

func TestGithubPollTickCmd_ZeroPollInterval(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.cfg.PollInterval = 0
	cmd := p.githubPollTickCmd()
	assert.Nil(t, cmd)
}

func TestGithubPollTickCmd_NegativePollInterval(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.cfg.PollInterval = -1
	cmd := p.githubPollTickCmd()
	assert.Nil(t, cmd)
}

func TestInit_ModeGitDoesNotCreateGitHubClient(t *testing.T) {
	p := New(
		defaultMock(),
		config.GitConfig{WorktreeOpenMode: "current"},
		config.GitHubConfig{Owner: "owner", Repo: "repo", PollInterval: 60},
		confirmedAllActions(),
		"/test/repo",
		"ascii",
		nil,
	)

	cmd := p.Init(t.Context())

	assert.NotNil(t, cmd)
	assert.Equal(t, "owner", p.gh.owner)
	assert.Equal(t, "repo", p.gh.repo)
	assert.Nil(t, p.gh.client, "ModeGit should not create a GitHub client")
	assert.Zero(t, p.gh.pageSize)
	assert.False(t, p.tabPaging[tabIssues].loading)
}

func TestStartGitHubLoad_ModeGitSkipsGitHubWork(t *testing.T) {
	p := New(
		defaultMock(),
		config.GitConfig{WorktreeOpenMode: "current"},
		config.GitHubConfig{PollInterval: 60, PageSize: 25},
		confirmedAllActions(),
		"/test/repo",
		"ascii",
		nil,
	)
	p.ctx = t.Context()
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"

	cmd := p.startGitHubLoad()

	assert.NotNil(t, cmd)
	assert.Zero(t, p.gh.pageSize, "ModeGit should not initialize GitHub pagination")
	assert.False(t, p.tabPaging[tabIssues].loading)
	assert.False(t, p.tabPaging[tabPRs].loading)
	assert.False(t, p.tabPaging[tabActions].loading)
}

func TestStartGitHubLoad_ModeGitHubStartsGitHubWork(t *testing.T) {
	p := NewGitHub(
		defaultMock(),
		config.GitConfig{WorktreeOpenMode: "current"},
		config.GitHubConfig{PollInterval: 60, PageSize: 25},
		confirmedAllActions(),
		"/test/repo",
		"ascii",
		nil,
	)
	p.ctx = t.Context()
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"

	cmd := p.startGitHubLoad()

	assert.NotNil(t, cmd)
	assert.Equal(t, 25, p.gh.pageSize)
	assert.True(t, p.tabPaging[tabIssues].loading)
	assert.True(t, p.tabPaging[tabPRs].loading)
	assert.True(t, p.tabPaging[tabActions].loading)
}

func TestLoadData_ModeGitHubSkipsHiddenGitTabs(t *testing.T) {
	mock := &countingGitOps{mockGitOps: *defaultMock()}
	p := NewGitHub(
		mock,
		config.GitConfig{WorktreeOpenMode: "current"},
		config.GitHubConfig{},
		confirmedAllActions(),
		"/test/repo",
		"ascii",
		nil,
	)
	p.ctx = t.Context()

	msg := p.loadData()()
	loaded, ok := msg.(dataLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)

	assert.Equal(t, 1, mock.branchCalls)
	assert.Equal(t, 1, mock.tagCalls)
	assert.Zero(t, mock.worktreeCalls, "ModeGitHub should not load hidden worktree tab data")
	assert.Zero(t, mock.remoteCalls, "ModeGitHub should not load hidden remote tab data")
	assert.Zero(t, mock.stashCalls, "ModeGitHub should not load hidden stash tab data")
	assert.Zero(t, mock.reflogCalls, "ModeGitHub should not load hidden reflog tab data")
}

// ---------------------------------------------------------------------------
// 29. Key handling for tab switches (b, w, s, i, p, a)
// ---------------------------------------------------------------------------

func TestKeyTabSwitch_B(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabWorktrees
	p.Update(tea.KeyPressMsg{Code: 'b'})
	assert.Equal(t, tabBranches, p.activeTab)
}

func TestKeyTabSwitch_W(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'w'})
	assert.Equal(t, tabWorktrees, p.activeTab)
}

func TestKeyTabSwitch_S(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 's'})
	assert.Equal(t, tabStash, p.activeTab)
}

func TestKeyTabSwitch_I_NoGHClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'i'})
	// Without ghClient, should not switch to issues.
	assert.NotEqual(t, tabIssues, p.activeTab)
}

func TestKeyTabSwitch_P_NoGHClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'p'})
	assert.NotEqual(t, tabPRs, p.activeTab)
}

func TestKeyTabSwitch_A_NoGHClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'a'})
	assert.NotEqual(t, tabActions, p.activeTab)
}

// ---------------------------------------------------------------------------
// 30. Key handling for navigation — focused=false returns nil
// ---------------------------------------------------------------------------

func TestKeyNavigation_NotFocused_ReturnsNil(t *testing.T) {
	keys := []tea.KeyPressMsg{
		{Code: 'j'},
		{Code: 'k'},
		{Code: 'g'},
		{Code: -1, Text: "G"},
		{Code: tea.KeyEnter},
		{Code: 'y'},
		{Code: 'n'},
		{Code: 'd'},
		{Code: tea.KeyF2},
		{Code: 'f'},
	}
	for _, key := range keys {
		p := newTestPanel(t, defaultMock())
		p.Focused = false
		_, cmd := p.Update(key)
		assert.Nil(t, cmd, "key %v with Focused=false should return nil cmd", key)
	}
}

// ---------------------------------------------------------------------------
// 31. Esc key from issues/prs/actions tabs returning to branches
// ---------------------------------------------------------------------------

func TestEscKey_IssuesTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabIssues
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, tabBranches, p.activeTab)
	assert.NotNil(t, cmd)
}

func TestEscKey_PRsTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabPRs
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, tabBranches, p.activeTab)
	assert.NotNil(t, cmd)
}

func TestEscKey_ActionsTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabActions
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, tabBranches, p.activeTab)
	assert.NotNil(t, cmd)
}

func TestEscKey_BranchesTab_NoOp(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabBranches
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, cmd)
	assert.Equal(t, tabBranches, p.activeTab)
}

// ---------------------------------------------------------------------------
// 32. View with GitHub unavailable showing "GitHub unavailable"
// ---------------------------------------------------------------------------

func TestView_GitHubUnavailable(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.gh.err = assert.AnError
	p.activeTab = tabIssues
	// No items in issues tab + ghErr set → should show "GitHub unavailable".
	view := p.View(80, 20)
	assert.Contains(t, view, "GitHub unavailable")
}

func TestView_GitHubTab_NoItems(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabIssues
	// No ghErr, no items → should show "No items".
	view := p.View(80, 20)
	assert.Contains(t, view, "No items")
}

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

func TestSetActiveTab_AllNames(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	names := map[string]tabID{
		"branches":  tabBranches,
		"worktrees": tabWorktrees,
		"remotes":   tabRemotes,
		"stash":     tabStash,
		"issues":    tabIssues,
		"prs":       tabPRs,
		"actions":   tabActions,
	}
	for name, tab := range names {
		p.SetActiveTab(name)
		assert.Equal(t, tab, p.activeTab, "SetActiveTab(%q)", name)
	}
}

func TestSetActiveTab_InvalidName(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.SetActiveTab("invalid")
	assert.Equal(t, tabBranches, p.activeTab, "invalid name should not change tab")
}

func TestActiveTabSelectionCmd_AllTabs(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())

	tabs := []tabID{tabBranches, tabWorktrees, tabRemotes, tabStash, tabIssues, tabPRs, tabActions}
	for _, tab := range tabs {
		p.activeTab = tab
		cmd := p.activeTabSelectionCmd()
		assert.NotNil(t, cmd, "activeTabSelectionCmd for tab %d should not be nil", tab)
	}
}

func TestUpdate_GithubPollTickMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	// Without ghClient, the poll tick should be handled without panic.
	// Since ghClient is nil, load functions return nil (no-op).
	_, cmd := p.Update(githubPollTickMsg{Time: time.Now()})
	// All load functions return nil when client is nil, so Batch produces nil.
	assert.Nil(t, cmd)
}

func TestRenderTabBar_WithAndWithoutGH(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	bar := p.renderTabBar(80)
	// Tab bar content is rendered with ANSI codes; check for partial text.
	assert.NotEmpty(t, bar)
	// The bar should contain the tab labels (possibly ANSI-styled).
	// Just verify it's non-empty and doesn't contain GitHub tabs without ghClient.
	assert.NotContains(t, bar, "Issues")
}

// ---------------------------------------------------------------------------
// doActionsRerun, doActionsCancel — with action items
// ---------------------------------------------------------------------------

func TestDoActionsRerun_NoItems(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 0
	// No items in actions tab.
	_, cmd := p.doActionsRerun()
	assert.Nil(t, cmd)
}

func TestDoActionsRerun_WithItem(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 0
	// Without ghClient, rerunFailedJobsCmd correctly returns nil (no-op).
	_, cmd := p.doActionsRerun()
	assert.Nil(t, cmd)
}

func TestDoActionsCancel_NoItems(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabActions
	_, cmd := p.doActionsCancel()
	assert.Nil(t, cmd)
}

func TestDoActionsCancel_WithItem(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 0
	_, cmd := p.doActionsCancel()
	assert.NotNil(t, cmd)
}

func TestDoActionsRerun_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 999
	_, cmd := p.doActionsRerun()
	assert.Nil(t, cmd)
}

func TestDoActionsCancel_OutOfBounds(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 999
	_, cmd := p.doActionsCancel()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// doFetch — more coverage (the fetch cmd returns a batch)
// ---------------------------------------------------------------------------

func TestDoFetch_OnRemoteSubItem(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 1 // fetch URL sub-item, not a remote
	_, cmd := p.doFetch()
	require.NotNil(t, cmd, "fetch on non-remote should still work (fetch all)")
}

// ---------------------------------------------------------------------------
// handleModalResult — edge case: opBranchRename with whitespace-only value
// ---------------------------------------------------------------------------

func TestHandleModalResult_BranchRename_WhitespaceOnly(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchRename
	p.pendingName = "old-name"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "   "})
	assert.Nil(t, cmd, "whitespace-only rename should be no-op")
}

// ---------------------------------------------------------------------------
// View rendering edge cases
// ---------------------------------------------------------------------------

func TestView_WithGHTabsHasFilterLabels(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	// Simulate ghClient non-nil by checking renderTabBar behavior.
	// Since we can't easily set ghClient, test the View path for GitHub tabs.
	p.activeTab = tabIssues
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
}

func TestView_ActionsTabContent(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "CI")
}

func TestView_PRsTabContent(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, samplePRs(), nil)
	p.activeTab = tabPRs
	view := p.View(80, 20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Add auth")
}

// ---------------------------------------------------------------------------
// renderStashEntry edge cases (narrow width)
// ---------------------------------------------------------------------------

func TestRenderStashEntry_NarrowWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "A very long message that should be truncated"}}
	line := p.renderStashEntry(item, 15, false)
	assert.NotEmpty(t, line)
}

func TestRenderStashEntry_VeryNarrow(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "msg"}}
	line := p.renderStashEntry(item, 3, false)
	assert.NotEmpty(t, line)
}

// ---------------------------------------------------------------------------
// Full mock ghclient.Client — enables testing all GitHub-dependent code paths
// ---------------------------------------------------------------------------

type mockGHClientFull struct {
	user             *gh.User
	userErr          error
	issues           []*gh.Issue
	issuesErr        error
	prs              []*gh.PullRequest
	prsErr           error
	pr               *gh.PullRequest
	prErr            error
	runs             []*gh.WorkflowRun
	run              *gh.WorkflowRun
	runErr           error
	runsErr          error
	prFiles          []*gh.CommitFile
	prFilesErr       error
	prCommits        []*gh.RepositoryCommit
	prCommErr        error
	comparison       *gh.CommitsComparison
	compareErr       error
	compareOwner     string
	compareRepo      string
	compareBase      string
	compareHead      string
	compareCalls     int
	jobs             []*gh.WorkflowJob
	jobsErr          error
	jobLog           string
	jobLogErr        error
	jobLogFreshErr   error
	jobLogCalls      int
	jobLogFreshCalls int
	rerunErr         error
	cancelErr        error
	mergeErr         error
	addAssigneesErr  error
	getPRCalls       int

	lastIssuesOpts *gh.IssueListByRepoOptions
	lastPRsOpts    *gh.PullRequestListOptions

	notifications []*gh.Notification
	notifErr      error
	markReadErr   error
	markReadCalls int
	markReadID    string

	reviewersErr          error
	requestedReviewers    []string
	requestReviewersCalls int

	// New-issue create recording.
	createIssueReq   *gh.IssueRequest
	createIssueResp  *gh.Issue
	createIssueErr   error
	createIssueCalls int

	// Comment recording.
	commentCalls  int
	commentNumber int
	commentBody   string
	commentErr    error

	// Review-comment recording.
	reviewCommentCalls    int
	reviewCommentPR       int
	reviewCommentCommitID string
	reviewCommentPath     string
	reviewCommentLine     int
	reviewCommentBody     string
	reviewCommentErr      error

	// Create-PR controls.
	createdPR   *gh.PullRequest    // returned by CreatePR when createErr is nil
	createErr   error              // returned by CreatePR
	createReq   *gh.NewPullRequest // captured request passed to CreatePR
	createCalls int                // number of times CreatePR was called
	repoInfo    *gh.Repository     // returned by RepoInfo

	// Workflow-dispatch controls (issue #361).
	workflowInputs []ghclient.WorkflowInput
	dispatchErr    error
	dispatchCalls  int
	dispatchID     int64
	dispatchRef    string
	dispatchInputs map[string]any
}

func (m *mockGHClientFull) CurrentUser(_ context.Context) (*gh.User, error) {
	return m.user, m.userErr
}

func (m *mockGHClientFull) ListIssues(_ context.Context, _, _ string, _ *gh.IssueListByRepoOptions) ([]*gh.Issue, error) {
	return m.issues, m.issuesErr
}

func (m *mockGHClientFull) GetIssue(_ context.Context, _, _ string, _ int) (*gh.Issue, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetIssueComments(_ context.Context, _, _ string, _ int) ([]*gh.IssueComment, error) {
	return nil, nil
}

func (m *mockGHClientFull) CreateIssue(_ context.Context, _, _ string, req *gh.IssueRequest) (*gh.Issue, error) {
	m.createIssueCalls++
	m.createIssueReq = req
	return m.createIssueResp, m.createIssueErr
}

func (m *mockGHClientFull) EditIssue(_ context.Context, _, _ string, _ int, _ *gh.IssueRequest) error {
	return nil
}

func (m *mockGHClientFull) CommentOnIssue(_ context.Context, _, _ string, number int, body string) error {
	m.commentCalls++
	m.commentNumber = number
	m.commentBody = body
	return m.commentErr
}
func (m *mockGHClientFull) CloseIssue(_ context.Context, _, _ string, _ int) error  { return nil }
func (m *mockGHClientFull) ReopenIssue(_ context.Context, _, _ string, _ int) error { return nil }
func (m *mockGHClientFull) AddAssignees(_ context.Context, _, _ string, _ int, _ []string) error {
	return m.addAssigneesErr
}

func (m *mockGHClientFull) ListPRs(_ context.Context, _, _ string, _ *gh.PullRequestListOptions) ([]*gh.PullRequest, error) {
	return m.prs, m.prsErr
}

func (m *mockGHClientFull) GetPR(_ context.Context, _, _ string, _ int) (*gh.PullRequest, error) {
	m.getPRCalls++
	return m.pr, m.prErr
}

func (m *mockGHClientFull) GetPRFiles(_ context.Context, _, _ string, _ int) ([]*gh.CommitFile, error) {
	return m.prFiles, m.prFilesErr
}

func (m *mockGHClientFull) GetPRComments(_ context.Context, _, _ string, _ int) ([]*gh.PullRequestComment, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetPRReviews(_ context.Context, _, _ string, _ int) ([]*gh.PullRequestReview, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetPRDiff(_ context.Context, _, _ string, _ int) (string, error) {
	return "", nil
}

func (m *mockGHClientFull) GetPRCommits(_ context.Context, _, _ string, _ int) ([]*gh.RepositoryCommit, error) {
	return m.prCommits, m.prCommErr
}

func (m *mockGHClientFull) CreatePR(_ context.Context, _, _ string, req *gh.NewPullRequest) (*gh.PullRequest, error) {
	m.createCalls++
	m.createReq = req
	return m.createdPR, m.createErr
}

func (m *mockGHClientFull) MergePR(_ context.Context, _, _ string, _ int, _ string, _ *gh.PullRequestOptions) error {
	return m.mergeErr
}

func (m *mockGHClientFull) DeleteBranch(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockGHClientFull) CommentOnPR(_ context.Context, _, _ string, _ int, _, _ string, _ int) error {
	return nil
}

func (m *mockGHClientFull) CreateReviewComment(_ context.Context, _, _ string, number int, commitID, path string, line int, body string) error {
	m.reviewCommentCalls++
	m.reviewCommentPR = number
	m.reviewCommentCommitID = commitID
	m.reviewCommentPath = path
	m.reviewCommentLine = line
	m.reviewCommentBody = body
	return m.reviewCommentErr
}

func (m *mockGHClientFull) SubmitReview(_ context.Context, _, _ string, _ int, _ *gh.PullRequestReviewRequest) error {
	return nil
}

func (m *mockGHClientFull) RequestReviewers(_ context.Context, _, _ string, _ int, reviewers []string) error {
	m.requestReviewersCalls++
	m.requestedReviewers = reviewers
	return m.reviewersErr
}

func (m *mockGHClientFull) ListWorkflowRuns(_ context.Context, _, _ string, _ *gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRun, error) {
	return m.runs, m.runsErr
}

func (m *mockGHClientFull) GetWorkflowRun(_ context.Context, _, _ string, _ int64) (*gh.WorkflowRun, error) {
	return m.run, m.runErr
}

func (m *mockGHClientFull) ListWorkflowJobs(_ context.Context, _, _ string, _ int64) ([]*gh.WorkflowJob, error) {
	return m.jobs, m.jobsErr
}

func (m *mockGHClientFull) GetJobLogs(_ context.Context, _, _ string, _ int64) (string, error) {
	m.jobLogCalls++
	return m.jobLog, m.jobLogErr
}

func (m *mockGHClientFull) GetJobLogsFresh(_ context.Context, _, _ string, _ int64) (string, error) {
	m.jobLogFreshCalls++
	if m.jobLogFreshErr != nil {
		return "", m.jobLogFreshErr
	}
	return m.jobLog, m.jobLogErr
}

func (m *mockGHClientFull) RerunFailedJobs(_ context.Context, _, _ string, _ int64) error {
	return m.rerunErr
}

func (m *mockGHClientFull) CancelWorkflowRun(_ context.Context, _, _ string, _ int64) error {
	return m.cancelErr
}

func (m *mockGHClientFull) ListNotifications(_ context.Context, _ *gh.NotificationListOptions) ([]*gh.Notification, error) {
	return m.notifications, m.notifErr
}

func (m *mockGHClientFull) MarkRead(_ context.Context, threadID string) error {
	m.markReadCalls++
	m.markReadID = threadID
	return m.markReadErr
}

func (m *mockGHClientFull) RepoInfo(_ context.Context, _, _ string) (*gh.Repository, error) {
	return m.repoInfo, nil
}

func (m *mockGHClientFull) ListWorkflows(_ context.Context, _, _ string, _ *gh.ListOptions) ([]*gh.Workflow, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetWorkflowInputs(_ context.Context, _, _, _, _ string) ([]ghclient.WorkflowInput, error) {
	return m.workflowInputs, nil
}

func (m *mockGHClientFull) DispatchWorkflow(_ context.Context, _, _ string, id int64, ref string, inputs map[string]any) error {
	m.dispatchCalls++
	m.dispatchID = id
	m.dispatchRef = ref
	m.dispatchInputs = inputs
	return m.dispatchErr
}

func (m *mockGHClientFull) RerunWorkflow(_ context.Context, _, _ string, _ int64) error {
	return nil
}

func (m *mockGHClientFull) ListReleases(_ context.Context, _, _ string, _ *gh.ListOptions) ([]*gh.RepositoryRelease, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetRelease(_ context.Context, _, _ string, _ int64) (*gh.RepositoryRelease, error) {
	return nil, nil
}

func (m *mockGHClientFull) GetReleaseByTag(_ context.Context, _, _, _ string) (*gh.RepositoryRelease, error) {
	return nil, nil
}

func (m *mockGHClientFull) CompareCommits(_ context.Context, owner, repo, base, head string) (*gh.CommitsComparison, error) {
	m.compareCalls++
	m.compareOwner = owner
	m.compareRepo = repo
	m.compareBase = base
	m.compareHead = head
	return m.comparison, m.compareErr
}

func (m *mockGHClientFull) ListIssuesPage(_ context.Context, _, _ string, opts *gh.IssueListByRepoOptions) ([]*gh.Issue, ghclient.PageResult, error) {
	m.lastIssuesOpts = opts
	return m.issues, ghclient.PageResult{}, m.issuesErr
}

func (m *mockGHClientFull) ListPRsPage(_ context.Context, _, _ string, opts *gh.PullRequestListOptions) ([]*gh.PullRequest, ghclient.PageResult, error) {
	m.lastPRsOpts = opts
	return m.prs, ghclient.PageResult{}, m.prsErr
}

func (m *mockGHClientFull) ListWorkflowRunsPage(_ context.Context, _, _ string, _ *gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRun, ghclient.PageResult, error) {
	return m.runs, ghclient.PageResult{}, m.runsErr
}

func (m *mockGHClientFull) ListWorkflowsPage(_ context.Context, _, _ string, _ *gh.ListOptions) ([]*gh.Workflow, ghclient.PageResult, error) {
	return nil, ghclient.PageResult{}, nil
}

func (m *mockGHClientFull) ListReleasesPage(_ context.Context, _, _ string, _ *gh.ListOptions) ([]*gh.RepositoryRelease, ghclient.PageResult, error) {
	return nil, ghclient.PageResult{}, nil
}

// newGHPanelWithClient creates a panel with a real mock ghClient for full GitHub testing.
func newGHPanelWithClient(t *testing.T, mock *mockGitOps, ghMock *mockGHClientFull) *Panel {
	t.Helper()
	p := newTestGitHubPanel(t, mock)
	p.gh.client = ghMock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()
	return p
}

func TestLoadPRsPageDoesNotEnrichEveryPR(t *testing.T) {
	num1, num2 := 1, 2
	title1, title2 := "First PR", "Second PR"
	state := prStateOpen
	head1, head2 := "feature/one", "feature/two"
	ghMock := &mockGHClientFull{
		prs: []*gh.PullRequest{
			{Number: &num1, Title: &title1, State: &state, Head: &gh.PullRequestBranch{Ref: &head1}, User: ghUser("a")},
			{Number: &num2, Title: &title2, State: &state, Head: &gh.PullRequestBranch{Ref: &head2}, User: ghUser("b")},
		},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.pageSize = 25

	msg := p.loadPRsPage(1, true)()
	loaded, ok := msg.(ghPRsPageMsg)
	require.True(t, ok)

	require.Len(t, loaded.prs, 2)
	assert.Zero(t, ghMock.getPRCalls, "PR list refresh should not make one detail request per PR")
}

// helper to create a gh.User pointer
func ghUser(login string) *gh.User {
	return &gh.User{Login: &login}
}

// ---------------------------------------------------------------------------
// loadGitHubData — the 9.8% coverage function
// ---------------------------------------------------------------------------

func TestLoadGitHubData_FullPath(t *testing.T) {
	login := "testuser"
	num1, num2 := 1, 2
	title1, title2 := "Bug report", "Feature req"
	body1 := "body1"
	state := "open"
	url1 := "https://github.com/o/r/issues/1"
	url2 := "https://github.com/o/r/issues/2"
	labelName := "bug"
	assigneeLogin := "testuser"

	ghMock := &mockGHClientFull{
		user: ghUser(login),
		issues: []*gh.Issue{
			{
				Number:    &num1,
				Title:     &title1,
				Body:      &body1,
				State:     &state,
				User:      ghUser("testuser"),
				Labels:    []*gh.Label{{Name: &labelName}},
				Assignees: []*gh.User{{Login: &assigneeLogin}},
				HTMLURL:   &url1,
			},
			{
				Number:  &num2,
				Title:   &title2,
				State:   &state,
				HTMLURL: &url2,
				// No User, no Labels, no Assignees — exercises nil branches
			},
		},
		prs: func() []*gh.PullRequest {
			prNum := 10
			prTitle := "Add auth"
			prState := "open"
			prURL := "https://github.com/o/r/pull/10"
			headRef := "auth-branch"
			head := &gh.PullRequestBranch{Ref: &headRef}
			return []*gh.PullRequest{
				{
					Number:  &prNum,
					Title:   &prTitle,
					State:   &prState,
					Head:    head,
					User:    ghUser("testuser"),
					HTMLURL: &prURL,
				},
			}
		}(),
		runs: func() []*gh.WorkflowRun {
			runID := int64(100)
			name := "CI"
			runNum := 42
			status := "completed"
			conclusion := "success"
			branch := "main"
			url := "https://github.com/o/r/actions/runs/100"
			created := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
			ts := &gh.Timestamp{Time: created}
			return []*gh.WorkflowRun{
				{
					ID:         &runID,
					Name:       &name,
					RunNumber:  &runNum,
					Status:     &status,
					Conclusion: &conclusion,
					HeadBranch: &branch,
					HTMLURL:    &url,
					CreatedAt:  ts,
				},
			}
		}(),
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.loadGitHubData()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(ghDataLoadedMsg)
	require.True(t, ok)

	assert.Equal(t, "testuser", result.user)
	assert.Len(t, result.issues, 2)
	assert.Equal(t, 1, result.issues[0].Number)
	assert.Equal(t, "Bug report", result.issues[0].Title)
	assert.Equal(t, []string{"bug"}, result.issues[0].Labels)
	assert.Equal(t, "testuser", result.issues[0].Author)
	assert.Equal(t, "testuser", result.issues[0].Assignee)
	// Second issue has no User/Assignees — should have empty strings.
	assert.Equal(t, "", result.issues[1].Author)
	assert.Equal(t, "", result.issues[1].Assignee)

	assert.Len(t, result.prs, 1)
	assert.Equal(t, 10, result.prs[0].Number)
	assert.Equal(t, "testuser", result.prs[0].Author)

	assert.Len(t, result.actions, 1)
	assert.Equal(t, int64(100), result.actions[0].RunID)
	assert.Equal(t, "success", result.actions[0].Conclusion)
}

func TestLoadGitHubData_SkipsPRsInIssueList(t *testing.T) {
	num := 5
	title := "PR disguised as issue"
	state := "open"
	prLinks := &gh.PullRequestLinks{}

	ghMock := &mockGHClientFull{
		user: ghUser("user"),
		issues: []*gh.Issue{
			{Number: &num, Title: &title, State: &state, PullRequestLinks: prLinks},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)
	assert.Empty(t, result.issues, "PRs in issue list should be skipped")
}

func TestLoadGitHubData_DraftAndMergedPRs(t *testing.T) {
	draftNum := 11
	draftTitle := "WIP"
	draftState := "open"
	draftVal := true
	mergedNum := 12
	mergedTitle := "Done"
	mergedState := "closed"
	mergedVal := true

	ghMock := &mockGHClientFull{
		user: ghUser("user"),
		prs: []*gh.PullRequest{
			{Number: &draftNum, Title: &draftTitle, State: &draftState, Draft: &draftVal, Head: &gh.PullRequestBranch{}},
			{Number: &mergedNum, Title: &mergedTitle, State: &mergedState, Merged: &mergedVal, Head: &gh.PullRequestBranch{}},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)
	assert.Equal(t, "draft", result.prs[0].State)
	assert.Equal(t, "merged", result.prs[1].State)
}

func TestLoadGitHubData_APIErrors(t *testing.T) {
	// Errors on issues/prs/runs should not crash — they're silently skipped.
	ghMock := &mockGHClientFull{
		userErr:   assert.AnError,
		issuesErr: assert.AnError,
		prsErr:    assert.AnError,
		runsErr:   assert.AnError,
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadGitHubData()()
	result := msg.(ghDataLoadedMsg)
	assert.Empty(t, result.user)
	assert.Empty(t, result.issues)
	assert.Empty(t, result.prs)
	assert.Empty(t, result.actions)
}

// ---------------------------------------------------------------------------
// loadPRDetails — 0% coverage
// ---------------------------------------------------------------------------

func TestLoadPRDetails_Success(t *testing.T) {
	filename := "main.go"
	status := "modified"
	additions := 10
	deletions := 2
	patch := "@@ -1,2 +1,3 @@"
	sha := "abc123"
	commitMsg := "fix bug"
	authorLogin := "user"
	authorName := "User Name"
	commitDate := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	ts := &gh.Timestamp{Time: commitDate}

	ghMock := &mockGHClientFull{
		prFiles: []*gh.CommitFile{
			{Filename: &filename, Status: &status, Additions: &additions, Deletions: &deletions, Patch: &patch},
		},
		prCommits: []*gh.RepositoryCommit{
			{
				SHA:    &sha,
				Commit: &gh.Commit{Message: &commitMsg, Author: &gh.CommitAuthor{Name: &authorName, Date: ts}},
				Author: ghUser(authorLogin),
			},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.loadPRDetails(42)
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(prDetailsLoadedMsg)
	require.True(t, ok)
	assert.Nil(t, result.err)
	assert.Equal(t, 42, result.number)
	assert.Len(t, result.files, 1)
	assert.Equal(t, "main.go", result.files[0].Filename)
	assert.Len(t, result.commits, 1)
	assert.Equal(t, "abc123", result.commits[0].SHA)
	assert.Equal(t, "user", result.commits[0].Author)
}

func TestLoadPRDetails_FilesError(t *testing.T) {
	ghMock := &mockGHClientFull{prFilesErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadPRDetails(1)()
	result := msg.(prDetailsLoadedMsg)
	assert.NotNil(t, result.err)
	assert.Contains(t, result.err.Error(), "PR files")
}

func TestLoadPRDetails_CommitsError(t *testing.T) {
	ghMock := &mockGHClientFull{prCommErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadPRDetails(1)()
	result := msg.(prDetailsLoadedMsg)
	assert.NotNil(t, result.err)
	assert.Contains(t, result.err.Error(), "PR commits")
}

func TestLoadPRDetails_CommitWithoutAuthor(t *testing.T) {
	sha := "def456"
	commitMsg := "auto commit"
	authorName := "Bot"
	commitDate := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	ts := &gh.Timestamp{Time: commitDate}

	ghMock := &mockGHClientFull{
		prCommits: []*gh.RepositoryCommit{
			{
				SHA:    &sha,
				Commit: &gh.Commit{Message: &commitMsg, Author: &gh.CommitAuthor{Name: &authorName, Date: ts}},
				// Author (gh.User) is nil — should fall back to Commit.Author.Name
			},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadPRDetails(1)()
	result := msg.(prDetailsLoadedMsg)
	require.Nil(t, result.err)
	assert.Equal(t, "Bot", result.commits[0].Author)
}

func TestLoadPRDetails_CommitNilCommit(t *testing.T) {
	sha := "ghi789"

	ghMock := &mockGHClientFull{
		prCommits: []*gh.RepositoryCommit{
			{SHA: &sha}, // Commit field is nil
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadPRDetails(1)()
	result := msg.(prDetailsLoadedMsg)
	require.Nil(t, result.err)
	assert.Equal(t, "", result.commits[0].Message)
	assert.Equal(t, "", result.commits[0].Author)
	assert.Equal(t, "", result.commits[0].Date)
}

// ---------------------------------------------------------------------------
// loadActionJobs — 0% coverage
// ---------------------------------------------------------------------------

func TestLoadActionJobs_Success(t *testing.T) {
	jobID := int64(200)
	jobName := "build"
	jobStatus := "completed"
	jobConclusion := "success"
	started := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	completed := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC)
	startedTs := &gh.Timestamp{Time: started}
	completedTs := &gh.Timestamp{Time: completed}
	stepNum := int64(1)
	stepName := "Checkout"
	stepStatus := "completed"
	stepConclusion := "success"

	ghMock := &mockGHClientFull{
		jobs: []*gh.WorkflowJob{
			{
				ID:          &jobID,
				Name:        &jobName,
				Status:      &jobStatus,
				Conclusion:  &jobConclusion,
				StartedAt:   startedTs,
				CompletedAt: completedTs,
				Steps: []*gh.TaskStep{
					{Number: &stepNum, Name: &stepName, Status: &stepStatus, Conclusion: &stepConclusion},
				},
			},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.loadActionJobs(100)
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(actionJobsLoadedMsg)
	require.True(t, ok)
	assert.Nil(t, result.err)
	assert.Equal(t, int64(100), result.runID)
	assert.Len(t, result.jobs, 1)
	assert.Equal(t, "build", result.jobs[0].Name)
	assert.Len(t, result.jobs[0].Steps, 1)
}

func TestLoadActionJobs_Error(t *testing.T) {
	ghMock := &mockGHClientFull{jobsErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadActionJobs(100)()
	result := msg.(actionJobsLoadedMsg)
	assert.NotNil(t, result.err)
}

func TestLoadActionJobs_NoTimestamps(t *testing.T) {
	jobID := int64(201)
	jobName := "test"
	jobStatus := "queued"
	jobConclusion := ""

	ghMock := &mockGHClientFull{
		jobs: []*gh.WorkflowJob{
			{ID: &jobID, Name: &jobName, Status: &jobStatus, Conclusion: &jobConclusion},
		},
	}

	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadActionJobs(100)()
	result := msg.(actionJobsLoadedMsg)
	assert.Nil(t, result.err)
	assert.Equal(t, "", result.jobs[0].StartedAt)
	assert.Equal(t, "", result.jobs[0].CompletedAt)
}

// ---------------------------------------------------------------------------
// loadActionLog — 0% coverage
// ---------------------------------------------------------------------------

func TestLoadActionLog_Success(t *testing.T) {
	ghMock := &mockGHClientFull{jobLog: "Step 1: checkout\nStep 2: build\nDone."}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.loadActionLog(100, 200)
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(actionLogLoadedMsg)
	require.True(t, ok)
	assert.Nil(t, result.err)
	assert.Equal(t, int64(100), result.runID)
	assert.Equal(t, int64(200), result.jobID)
	assert.Contains(t, result.log, "Step 1")
}

func TestLoadActionLog_Error(t *testing.T) {
	ghMock := &mockGHClientFull{jobLogErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.loadActionLog(100, 200)()
	result := msg.(actionLogLoadedMsg)
	assert.NotNil(t, result.err)
}

// ---------------------------------------------------------------------------
// handleKey — branches needing ghClient
// ---------------------------------------------------------------------------

func TestHandleKey_I_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'i'})
	assert.Equal(t, tabIssues, p.activeTab)
}

func TestHandleKey_P_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'p'})
	assert.Equal(t, tabPRs, p.activeTab)
}

func TestHandleKey_A_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: 'a'})
	assert.Equal(t, tabActions, p.activeTab)
}

func TestHandleKey_R_ActionsTab_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 0
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	assert.NotNil(t, cmd, "'r' on actions tab with ghClient should rerun")
}

func TestHandleKey_X_ActionsTab_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	p.tabCursor[tabActions] = 0
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	assert.NotNil(t, cmd, "'x' on actions tab should cancel")
}

func TestHandleKey_X_BranchesTab_DeletesCurrentBranchWarns(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabBranches
	// Cursor is on "main" (IsCurrent), so delete should warn.
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	require.NotNil(t, cmd, "'x' on current branch should return a warning command")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Cannot delete current branch")
}

func TestHandleKey_X_ActionsTab_NoGHClient(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	assert.Nil(t, cmd, "'x' on actions without ghClient should be no-op")
}

func TestHandleKey_F_IssuesTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	populateGH(p, sampleIssues(), nil, nil)
	p.activeTab = tabIssues
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'f'})
	assert.NotNil(t, cmd, "'f' on issues tab should cycle filter")
	assert.Equal(t, issueFilterAssigned, p.gh.issueFilter)
}

func TestHandleKey_F_PRsTab(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	populateGH(p, nil, samplePRs(), nil)
	p.activeTab = tabPRs
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'f'})
	assert.NotNil(t, cmd, "'f' on PRs tab should cycle filter")
	assert.Equal(t, prFilterNeedsReview, p.gh.prFilter)
}

func TestHandleKey_R_Remotes_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.Focused = true
	p.activeTab = tabBranches // not actions tab, so 'r' = remotes tab
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'r'})
	// In ModeGitHub, 'r' falls through (remotes switch is only for ModeGit).
	// It should not crash and may return nil.
	_ = cmd
}

// ---------------------------------------------------------------------------
// requestCheckout — remote branch prefix stripping
// ---------------------------------------------------------------------------

func TestRequestCheckout_RemoteBranch(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 0 // "origin/main" (remote — only branch in ModeGitHub)
	_, cmd := p.requestCheckout()
	require.NotNil(t, cmd)
	// Should show a confirmation dialog (not direct checkout).
	msg := cmd()
	_, isModal := msg.(notify.ShowModalMsg)
	assert.True(t, isModal, "expected confirmation dialog, got %T", msg)
	// The pendingName should have the remote prefix stripped.
	assert.Equal(t, "main", p.pendingName, "remote branch should have prefix stripped")
}

func TestRequestCheckout_NilBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees // no branch at cursor
	_, cmd := p.requestCheckout()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// requestWorktreeSwitch — new_terminal mode
// ---------------------------------------------------------------------------

func TestRequestWorktreeSwitch_NilWorktree(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches // no worktree at cursor
	_, cmd := p.requestWorktreeSwitch()
	assert.Nil(t, cmd)
}

func TestRequestWorktreeSwitch_NewTerminal(t *testing.T) {
	mock := defaultMock()
	p := New(mock, config.GitConfig{WorktreeOpenMode: "new_terminal"}, config.GitHubConfig{}, confirmedAllActions(), "/test/repo", "ascii", nil)
	cmd := p.Init(t.Context())
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}
	p.activeTab = tabWorktrees
	p.tabCursor[tabWorktrees] = 0
	_, resultCmd := p.requestWorktreeSwitch()
	assert.NotNil(t, resultCmd, "new_terminal mode should return a command")
}

func TestRequestWorktreeSwitch_CurrentMode(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	p.tabCursor[tabWorktrees] = 0
	_, cmd := p.requestWorktreeSwitch()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "worktree_switch", result.op)
}

// ---------------------------------------------------------------------------
// renderTabBar — with ghClient (two-row tab bar) + filter labels
// ---------------------------------------------------------------------------

func TestRenderTabBar_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())

	bar := panels.StripANSI(p.renderTabBar(120))
	assert.Contains(t, bar, "Issues")
	assert.Contains(t, bar, "PRs")
	assert.Contains(t, bar, "Actions")
	assert.NotContains(t, bar, "\n", "ModeGitHub should have one row (no git tabs)")
}

func TestRenderTabBar_WithIssueFilter(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.gh.issueFilter = issueFilterAssigned
	p.activeTab = tabIssues

	bar := panels.StripANSI(p.renderTabBar(80))
	assert.Contains(t, bar, "Assigned")
}

func TestRenderTabBar_WithPRFilter(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.gh.prFilter = prFilterMine
	p.activeTab = tabPRs

	bar := panels.StripANSI(p.renderTabBar(80))
	assert.Contains(t, bar, "Mine")
}

func TestRenderTabBar_ActiveGHTab(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.activeTab = tabActions

	bar := p.renderTabBar(80)
	assert.NotEmpty(t, bar)
}

// ---------------------------------------------------------------------------
// ensureCursorVisible with ghClient — 2-line tab bar
// ---------------------------------------------------------------------------

func TestEnsureCursorVisible_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.SetSize(80, 5) // height 5, tab bar is 1 line (ModeGitHub) → viewH = 4
	p.tabCursor[tabIssues] = 2
	p.tabOffset[tabIssues] = 0
	p.ensureCursorVisible()
	assert.LessOrEqual(t, p.tabOffset[tabIssues], p.tabCursor[tabIssues])
}

func TestEnsureCursorVisible_SmallHeight_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.SetSize(80, 1)        // height 1, tab bar is 1 line → viewH = 0 → early return
	p.ensureCursorVisible() // should not panic
}

// ---------------------------------------------------------------------------
// KeyBindings — with ghClient adds GitHub bindings
// ---------------------------------------------------------------------------

func TestKeyBindings_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	bindings := p.KeyBindings()

	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	assert.True(t, actions["tab_issues"], "should have issues tab binding")
	assert.True(t, actions["tab_prs"], "should have PRs tab binding")
	assert.True(t, actions["tab_actions"], "should have actions tab binding")
}

// ---------------------------------------------------------------------------
// githubPollTickCmd — valid case (non-nil client + positive interval)
// ---------------------------------------------------------------------------

func TestGithubPollTickCmd_ValidConfig(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.cfg.PollInterval = 30
	cmd := p.githubPollTickCmd()
	assert.NotNil(t, cmd, "valid ghClient + positive interval should return a tick cmd")
}

// ---------------------------------------------------------------------------
// handleActionJobsLoaded — with ghClient and failed job triggers log fetch
// ---------------------------------------------------------------------------

func TestHandleActionJobsLoaded_FailedJobWithClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u"), jobLog: "error log"}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	jobs := []panels.ActionJob{
		{ID: 1, Name: "build", Status: "completed", Conclusion: "success"},
		{ID: 2, Name: "test", Status: "completed", Conclusion: "failure"},
	}
	_, cmd := p.handleActionJobsLoaded(actionJobsLoadedMsg{runID: 100, jobs: jobs})
	require.NotNil(t, cmd, "failed job with ghClient should trigger log fetch")
}

func TestHandleActionJobsLoaded_NoFailedJobs(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	jobs := []panels.ActionJob{
		{ID: 1, Name: "build", Status: "completed", Conclusion: "success"},
	}
	_, cmd := p.handleActionJobsLoaded(actionJobsLoadedMsg{runID: 100, jobs: jobs})
	require.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// View — with ghClient (2-line tab bar)
// ---------------------------------------------------------------------------

func TestView_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())

	view := panels.StripANSI(p.View(120, 20))
	assert.Contains(t, view, "Issues")
	assert.Contains(t, view, "Bug report")
}

func TestView_WithGHClient_TinyHeight(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	// Height 2 = exactly the tab bar, no content room.
	view := p.View(80, 2)
	assert.NotEmpty(t, view)
}

func TestView_WithGHClient_GHUnavailable(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.err = assert.AnError
	p.activeTab = tabPRs
	view := p.View(80, 20)
	assert.Contains(t, view, "GitHub unavailable")
}

// ---------------------------------------------------------------------------
// Update — message dispatch coverage
// ---------------------------------------------------------------------------

func TestUpdate_PRDetailsLoadedMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update(prDetailsLoadedMsg{number: 1, files: nil, commits: nil})
	assert.NotNil(t, cmd)
}

func TestUpdate_ActionJobsLoadedMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update(actionJobsLoadedMsg{runID: 1, jobs: nil})
	assert.NotNil(t, cmd)
}

func TestUpdate_ActionLogLoadedMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update(actionLogLoadedMsg{runID: 1, jobID: 2, log: "log"})
	assert.NotNil(t, cmd)
}

func TestUpdate_ActionRerunResultMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update(actionRerunResultMsg{runID: 1})
	assert.NotNil(t, cmd)
}

func TestUpdate_ActionCancelResultMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update(actionCancelResultMsg{runID: 1})
	assert.NotNil(t, cmd)
}

func TestUpdate_UnhandledMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.Update("some random string msg")
	assert.Nil(t, cmd, "unhandled message should return nil cmd")
}

// ---------------------------------------------------------------------------
// mouseClick / mouseDoubleClick with ghClient (2-line tab bar)
// ---------------------------------------------------------------------------

func TestMouseClick_GHTabBarClick(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.SetSize(80, 20)

	// In ModeGitHub, row 0 is the GitHub tab bar (single row).
	// Branches is now the first tab, so clicking col 1 selects it.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 1})
	assert.Equal(t, tabBranches, p.activeTab, "clicking GH tab bar row should switch to branches (first tab)")
}

func TestMouseClick_ContentRow_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.SetSize(80, 20)

	// In ModeGitHub, tab bar is 1 row. Row 1 = first content item (Issues tab).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.tabCursor[tabIssues])
}

func TestMouseDoubleClick_WithGHClient_TabBar_Noop(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.SetSize(80, 20)

	// In ModeGitHub with ghOwner/ghRepo set, double-click on the tab bar
	// row should be a no-op (#39). Repo-open only via header double-click.
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Nil(t, cmd, "tab bar double-click should be no-op, not open repo")
}

func TestHeaderDoubleClick_WithGHClient_OpensRepo(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.SetSize(80, 20)

	// Stub the browser launcher so the test never opens a real browser
	// tab (xdg-open may not exist in headless/WSL environments).
	orig := panels.StartDetachedFn
	panels.StartDetachedFn = func(*exec.Cmd) error { return nil }
	t.Cleanup(func() { panels.StartDetachedFn = orig })

	// PanelHeaderDoubleClickMsg (from layout engine when double-clicking
	// the panel border title) should open the repo in the browser.
	_, cmd := p.Update(panels.PanelHeaderDoubleClickMsg{ContentCol: 5})
	require.NotNil(t, cmd, "header double-click should open repo in browser")
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	assert.Contains(t, toast.Message, "owner/repo")
}

func TestMouseWheel_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, sampleIssues(), samplePRs(), sampleActions())
	p.SetSize(80, 5)

	// Scroll down with 1-line tab bar (ModeGitHub).
	p.handleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.GreaterOrEqual(t, p.tabOffset[tabIssues], 0)
}

// ---------------------------------------------------------------------------
// prSelectedCmd with ghClient — triggers loadPRDetails
// ---------------------------------------------------------------------------

func TestPRSelectedCmd_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, nil, samplePRs(), nil)
	p.activeTab = tabPRs
	cmd := p.prSelectedCmd()
	require.NotNil(t, cmd, "prSelectedCmd with ghClient should trigger loadPRDetails")
}

// ---------------------------------------------------------------------------
// actionRunSelectedCmd with ghClient — triggers loadActionJobs
// ---------------------------------------------------------------------------

func TestActionRunSelectedCmd_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	populateGH(p, nil, nil, sampleActions())
	p.activeTab = tabActions
	cmd := p.actionRunSelectedCmd()
	require.NotNil(t, cmd, "actionRunSelectedCmd with ghClient should trigger loadActionJobs")
}

// ---------------------------------------------------------------------------
// rerunFailedJobsCmd, cancelWorkflowRunCmd — actually execute the cmds
// ---------------------------------------------------------------------------

func TestRerunFailedJobsCmd_Success(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.rerunFailedJobsCmd(100)
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(actionRerunResultMsg)
	require.True(t, ok)
	assert.Nil(t, result.err)
}

func TestRerunFailedJobsCmd_Error(t *testing.T) {
	ghMock := &mockGHClientFull{rerunErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.rerunFailedJobsCmd(100)()
	result := msg.(actionRerunResultMsg)
	assert.NotNil(t, result.err)
}

func TestCancelWorkflowRunCmd_Success(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	cmd := p.cancelWorkflowRunCmd(100)
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(actionCancelResultMsg)
	require.True(t, ok)
	assert.Nil(t, result.err)
}

func TestCancelWorkflowRunCmd_Error(t *testing.T) {
	ghMock := &mockGHClientFull{cancelErr: assert.AnError}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	msg := p.cancelWorkflowRunCmd(100)()
	result := msg.(actionCancelResultMsg)
	assert.NotNil(t, result.err)
}

// ---------------------------------------------------------------------------
// Rendering edge cases — narrow widths triggering truncation
// ---------------------------------------------------------------------------

func TestRenderIssue_NarrowWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindIssue, issue: ghIssueItem{Number: 42, Title: "A very long issue title that needs truncation", Labels: []string{"bug"}}}
	// Width of 20 forces title truncation.
	line := p.renderIssue(item, 20, true)
	assert.NotEmpty(t, line)
}

func TestRenderIssue_NoLabels(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindIssue, issue: ghIssueItem{Number: 1, Title: "No labels"}}
	line := p.renderIssue(item, 80, false)
	assert.Contains(t, line, "#1")
}

func TestRenderIssue_TinyWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindIssue, issue: ghIssueItem{Number: 1, Title: "Bug", Labels: []string{"big-label"}}}
	line := p.renderIssue(item, 5, false)
	assert.NotEmpty(t, line)
}

func TestRenderPR_NarrowWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{Number: 10, Title: "A very long PR title that needs truncation", State: "open"}}
	line := p.renderPR(item, 20, true)
	assert.NotEmpty(t, line)
}

func TestRenderPR_TinyWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{Number: 10, Title: "PR", State: "open"}}
	line := p.renderPR(item, 5, false)
	assert.NotEmpty(t, line)
}

func TestRenderActionRun_NarrowWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "A Very Long Workflow Name", RunNumber: 42,
		Status: "completed", Conclusion: "success", Branch: "main", CreatedAt: "Jan 1 10:00",
	}}
	line := p.renderActionRun(item, 20, true)
	assert.NotEmpty(t, line)
}

func TestRenderActionRun_TinyWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 1, Conclusion: "success", Branch: "main", CreatedAt: "Jan 1",
	}}
	line := p.renderActionRun(item, 5, false)
	assert.NotEmpty(t, line)
}

func TestRenderActionRun_UnknownStatus(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 1, Status: "waiting", Conclusion: "",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "●")
}

func TestRenderActionRun_TimedOut(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 1, Conclusion: "timed_out",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "✗")
}

func TestRenderActionRun_Queued(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 1, Status: "queued",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.Contains(t, line, "●")
}

func TestRenderActionRun_NoBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindActionRun, actionRun: ghActionItem{
		RunID: 100, WorkflowName: "CI", RunNumber: 1, Conclusion: "success",
	}}
	line := p.renderActionRun(item, 80, false)
	assert.NotEmpty(t, line)
}

// ---------------------------------------------------------------------------
// renderBranch — narrow and cursor edge cases
// ---------------------------------------------------------------------------

func TestRenderBranch_NarrowTruncation(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindLocalBranch, branch: git.Branch{Name: "a-very-long-branch-name-that-exceeds-width", Hash: "abc1234"}}
	line := p.renderBranch(item, 20, false)
	assert.NotEmpty(t, line)
}

func TestRenderBranch_AheadBehind(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindLocalBranch, branch: git.Branch{Name: "feature", Hash: "abc1234", Ahead: 3, Behind: 2}}
	line := p.renderBranch(item, 80, false)
	assert.Contains(t, line, "↑3")
	assert.Contains(t, line, "↓2")
}

func TestRenderBranch_TinyWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindLocalBranch, branch: git.Branch{Name: "main", Hash: "abc1234"}}
	line := p.renderBranch(item, 3, false)
	assert.NotEmpty(t, line)
}

// ---------------------------------------------------------------------------
// renderWorktree — narrow and cursor edge cases
// ---------------------------------------------------------------------------

func TestRenderWorktree_NarrowTruncation(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindWorktree, worktree: git.Worktree{Path: "/very/long/path/to/worktree/directory", Branch: "main", Head: "abc1234567"}}
	line := p.renderWorktree(item, 20, false)
	assert.NotEmpty(t, line)
}

func TestRenderWorktree_TinyWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindWorktree, worktree: git.Worktree{Path: "/p", Branch: "b", Head: "abc1234567"}}
	line := p.renderWorktree(item, 3, false)
	assert.NotEmpty(t, line)
}

func TestRenderWorktree_NoBranch(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindWorktree, worktree: git.Worktree{Path: "/test", Head: "abc"}}
	line := p.renderWorktree(item, 80, false)
	assert.NotEmpty(t, line)
}

func TestRenderWorktree_ShortHead(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindWorktree, worktree: git.Worktree{Path: "/test", Branch: "main", Head: "abc"}}
	line := p.renderWorktree(item, 80, false)
	assert.Contains(t, line, "abc")
}

// ---------------------------------------------------------------------------
// renderRemote / renderRemoteSub with cursor
// ---------------------------------------------------------------------------

func TestRenderRemote_WithCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemote, remote: git.Remote{Name: "origin"}}
	line := p.renderRemote(item, 80, true)
	assert.NotEmpty(t, line)
}

func TestRenderRemoteSub_WithCursor(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemoteSub, text: "fetch: https://example.com"}
	line := p.renderRemoteSub(item, 80, true)
	assert.NotEmpty(t, line)
}

// ---------------------------------------------------------------------------
// renderStashEntry with zero/negative width
// ---------------------------------------------------------------------------

func TestRenderStashEntry_ZeroWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "msg"}}
	line := p.renderStashEntry(item, 0, false)
	assert.NotEmpty(t, line) // lipgloss renders empty string but width=0
}

// ---------------------------------------------------------------------------
// doAction — stash entry and remoteSub
// ---------------------------------------------------------------------------

func TestDoAction_StashEntry(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabStash
	p.tabCursor[tabStash] = 0
	_, cmd := p.doAction()
	// Stash entry should now show an action prompt.
	require.NotNil(t, cmd, "stash double-click should show action prompt")
	msg := cmd()
	_, isModal := msg.(notify.ShowModalMsg)
	assert.True(t, isModal, "expected stash action prompt, got %T", msg)
}

func TestDoAction_RemoteSub(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 1 // remoteSub item
	_, cmd := p.doAction()
	assert.Nil(t, cmd, "remoteSub has no action")
}

// ---------------------------------------------------------------------------
// Tab / Shift+Tab cycling within panel
// ---------------------------------------------------------------------------

func TestTabCyclesGitTabs(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	assert.Equal(t, tabBranches, p.activeTab)

	expected := []tabID{tabWorktrees, tabRemotes, tabStash, tabTags, tabReflog, tabBranches}
	for _, want := range expected {
		p.Update(tea.KeyPressMsg{Code: '\t'})
		assert.Equal(t, want, p.activeTab, "expected tab %d after pressing Tab", want)
	}
}

func TestShiftTabCyclesGitTabsBackward(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	assert.Equal(t, tabBranches, p.activeTab)

	expected := []tabID{tabReflog, tabTags, tabStash, tabRemotes, tabWorktrees, tabBranches}
	for _, want := range expected {
		p.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		assert.Equal(t, want, p.activeTab, "expected tab %d after pressing Shift+Tab", want)
	}
}

func TestTabCyclesGitHubTabs(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.Focused = true
	// ModeGitHub starts on tabIssues (set by NewGitHub).
	assert.Equal(t, tabIssues, p.activeTab)

	expected := []tabID{tabPRs, tabActions, tabWorkflows, tabReleases, tabNotifications, tabBranches, tabTags, tabIssues}
	for _, want := range expected {
		p.Update(tea.KeyPressMsg{Code: '\t'})
		assert.Equal(t, want, p.activeTab, "expected tab %d after pressing Tab", want)
	}
}

func TestShiftTabCyclesGitHubTabsBackward(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.Focused = true
	assert.Equal(t, tabIssues, p.activeTab)

	expected := []tabID{tabTags, tabBranches, tabNotifications, tabReleases, tabWorkflows, tabActions, tabPRs, tabIssues}
	for _, want := range expected {
		p.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		assert.Equal(t, want, p.activeTab, "expected tab %d after pressing Shift+Tab", want)
	}
}

func TestTabNotFocusedDoesNothing(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.Focused = false
	p.activeTab = tabBranches
	p.Update(tea.KeyPressMsg{Code: '\t'})
	assert.Equal(t, tabBranches, p.activeTab, "Tab should be ignored when panel is not focused")
}

func TestVisibleTabsGitMode(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	tabs := p.visibleTabs()
	assert.Equal(t, []tabID{tabBranches, tabWorktrees, tabRemotes, tabStash, tabTags, tabReflog}, tabs)
}

func TestVisibleTabsGitHubMode(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	tabs := p.visibleTabs()
	assert.Equal(t, []tabID{tabBranches, tabTags, tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases, tabNotifications}, tabs)
}

// ---------------------------------------------------------------------------
// doDelete — stash tab and issues tab (no handler)
// ---------------------------------------------------------------------------

func TestDoDelete_StashEntry(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabStash
	p.tabCursor[tabStash] = 0
	_, cmd := p.doDelete()
	assert.Nil(t, cmd, "stash delete not supported")
}

// ---------------------------------------------------------------------------
// Update dispatches through GH poll tick with ghClient
// ---------------------------------------------------------------------------

func TestUpdate_GithubPollTickMsg_WithGHClient(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.cfg.PollInterval = 30
	_, cmd := p.Update(githubPollTickMsg{Time: time.Now()})
	assert.NotNil(t, cmd, "poll tick with ghClient should trigger loadGitHubData + next tick")
}

// ---------------------------------------------------------------------------
// Branch checkout confirmation dialog
// ---------------------------------------------------------------------------

func TestRequestCheckout_ShowsConfirmation(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{{Name: "main", IsCurrent: true}, {Name: "develop"}},
	}
	p := newTestPanel(t, mock)
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 1 // develop
	_, cmd := p.requestCheckout()
	require.NotNil(t, cmd)
	msg := cmd()
	// Should be a ShowModalMsg (confirmation), not an opResultMsg.
	_, isModal := msg.(notify.ShowModalMsg)
	assert.True(t, isModal, "expected confirmation dialog, got %T", msg)
	assert.Equal(t, opBranchCheckout, p.pending)
	assert.Equal(t, "develop", p.pendingName)
}

func TestHandleModalResult_BranchCheckout_Accept(t *testing.T) {
	mock := &mockGitOps{
		branches: []git.Branch{{Name: "main"}, {Name: "develop"}},
	}
	p := newTestPanel(t, mock)
	p.pending = opBranchCheckout
	p.pendingName = "develop"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd, "accepting checkout should produce a cmd")
	msg := cmd()
	// First step: dirty-check message (mock Status returns nil = clean).
	dirtyMsg, ok := msg.(checkoutDirtyMsg)
	require.True(t, ok, "expected checkoutDirtyMsg, got %T", msg)
	assert.Equal(t, "develop", dirtyMsg.ref)
	assert.False(t, dirtyMsg.dirty)
	assert.NoError(t, dirtyMsg.err)

	// Second step: handleCheckoutDirty proceeds with checkout.
	_, cmd2 := p.handleCheckoutDirty(dirtyMsg)
	require.NotNil(t, cmd2)
	msg2 := cmd2()
	result, ok := msg2.(opResultMsg)
	require.True(t, ok, "expected opResultMsg, got %T", msg2)
	assert.Equal(t, "checkout", result.op)
	assert.Equal(t, "develop", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_BranchCheckout_Decline(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCheckout
	p.pendingName = "develop"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "declining checkout should produce no cmd")
	assert.Equal(t, opNone, p.pending)
}

func TestCheckoutDirty_CleanTree_ProceedsWithCheckout(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleCheckoutDirty(checkoutDirtyMsg{ref: "feature", dirty: false})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok, "expected opResultMsg, got %T", msg)
	assert.Equal(t, "checkout", result.op)
	assert.Equal(t, "feature", result.name)
	assert.NoError(t, result.err)
}

func TestCheckoutDirty_DirtyTree_ShowsStashDialog(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleCheckoutDirty(checkoutDirtyMsg{ref: "feature", dirty: true})
	require.NotNil(t, cmd)
	msg := cmd()
	_, isModal := msg.(notify.ShowModalMsg)
	assert.True(t, isModal, "expected stash confirm dialog, got %T", msg)
	assert.Equal(t, opBranchCheckoutStash, p.pending)
	assert.Equal(t, "feature", p.pendingName)
}

func TestCheckoutDirty_StatusError_FallsBackToCheckout(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	_, cmd := p.handleCheckoutDirty(checkoutDirtyMsg{ref: "feature", err: fmt.Errorf("status failed")})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok, "expected opResultMsg fallback, got %T", msg)
	assert.Equal(t, "checkout", result.op)
	assert.Equal(t, "feature", result.name)
}

func TestHandleModalResult_BranchCheckoutStash_Accept(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCheckoutStash
	p.pendingName = "feature"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok, "expected opResultMsg, got %T", msg)
	assert.Equal(t, "checkout_stashed", result.op)
	assert.Equal(t, "feature", result.name)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_BranchCheckoutStash_Decline(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opBranchCheckoutStash
	p.pendingName = "feature"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "declining stash-and-switch should produce no cmd")
	assert.Equal(t, opNone, p.pending)
}

// ---------------------------------------------------------------------------
// Stash action prompt
// ---------------------------------------------------------------------------

func TestDoAction_StashEntry_ShowsPrompt(t *testing.T) {
	mock := &mockGitOps{
		stashes: []git.StashEntry{{Index: 0, Message: "WIP"}},
	}
	p := newTestPanel(t, mock)
	p.activeTab = tabStash
	p.tabCursor[tabStash] = 0
	_, cmd := p.doAction()
	require.NotNil(t, cmd)
	msg := cmd()
	_, isModal := msg.(notify.ShowModalMsg)
	assert.True(t, isModal, "expected stash action prompt, got %T", msg)
	assert.Equal(t, opStashAction, p.pending)
}

func TestHandleModalResult_StashAction_Apply(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "apply"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_applied", result.op)
	assert.NoError(t, result.err)
}

func TestHandleModalResult_StashAction_Pop(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "pop"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_popped", result.op)
}

func TestHandleModalResult_StashAction_Drop(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "drop"})
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok)
	assert.Equal(t, "stash_dropped", result.op)
}

func TestHandleModalResult_StashAction_Unknown(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.pending = opStashAction
	p.pendingName = "0"
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "unknown"})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected toast for unknown action, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
}

// ---------------------------------------------------------------------------
// Remote double-click opens browser
// ---------------------------------------------------------------------------

func TestDoAction_Remote_ReturnsCmd(t *testing.T) {
	mock := &mockGitOps{
		remotes: []git.Remote{{Name: "origin", FetchURL: "https://github.com/user/repo"}},
	}
	p := newTestPanel(t, mock)
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 0
	_, cmd := p.doAction()
	// On a headless test we can't actually open a browser, but the cmd should be non-nil.
	assert.NotNil(t, cmd, "remote double-click should produce a cmd")
}

// Suppress unused import warnings.
var (
	_ = time.Now
	_ = notify.ModalResultMsg{}
	_ = actions.ItemLocalBranch
)

// ---------------------------------------------------------------------------
// First-use confirmation prompt tests
// ---------------------------------------------------------------------------

func TestDoAction_FirstUsePrompt(t *testing.T) {
	// With no confirmations, doAction should show the confirmation modal
	// instead of executing the action.
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{} // no confirmations
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 1 // "feature" (non-current)

	_, cmd := p.doAction()
	require.NotNil(t, cmd, "unconfirmed action should produce a command")

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, "Double-Click: Branch", modal.Title)
	assert.Equal(t, notify.ModalActionPickerWithCheckbox, modal.Kind)
	assert.Equal(t, opFirstUseConfirm, p.pending)
	assert.Equal(t, string(actions.ItemLocalBranch), p.pendingName)
}

func TestDoAction_FirstUseAcceptWithRemember(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{} // no confirmations

	// Simulate the first-use prompt state for a stash entry.
	p.activeTab = tabStash
	p.tabCursor[tabStash] = 0
	p.pending = opFirstUseConfirm
	p.pendingName = string(actions.ItemStashEntry)

	// User picks the default action (prompt_action) and checks "always".
	defaultAction := string(actions.DefaultAction(actions.ItemStashEntry))
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: defaultAction, Remember: true})
	require.NotNil(t, cmd, "accepting first-use should produce a command")

	// Should have marked the item type confirmed in-memory.
	assert.True(t, p.actionsCfg.Confirmed[string(actions.ItemStashEntry)],
		"action picker choice with Remember should be confirmed in actionsCfg")

	// The returned cmd should be the stash action prompt (executeRightClickAction for stash).
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "stash executeRightClickAction should show stash action prompt, got %T", msg)
	assert.Equal(t, "Stash Action", modal.Title)
}

func TestDoAction_FirstUseAcceptWithoutRemember(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{} // no confirmations

	// Simulate the first-use prompt state for a remote.
	p.activeTab = tabRemotes
	p.tabCursor[tabRemotes] = 0
	p.pending = opFirstUseConfirm
	p.pendingName = string(actions.ItemRemote)

	// User picks the default action (open_in_browser) without checking "always".
	defaultAction := string(actions.DefaultAction(actions.ItemRemote))
	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: defaultAction})
	require.NotNil(t, cmd, "accepting first-use should produce a command")

	// Without Remember, the choice should NOT be confirmed.
	confirmed := false
	if p.actionsCfg.Confirmed != nil {
		confirmed = p.actionsCfg.Confirmed[string(actions.ItemRemote)]
	}
	assert.False(t, confirmed,
		"action picker choice without Remember should not mark confirmed")
}

func TestDoAction_FirstUseReject(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{}

	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 1
	p.pending = opFirstUseConfirm
	p.pendingName = string(actions.ItemLocalBranch)

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "rejecting first-use should produce no command")
}

func TestDoAction_AlreadyConfirmed(t *testing.T) {
	// Pre-confirmed item type should bypass the prompt and execute directly.
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{
		Confirmed: map[string]bool{
			string(actions.ItemStashEntry): true,
		},
	}
	p.activeTab = tabStash
	p.tabCursor[tabStash] = 0

	_, cmd := p.doAction()
	require.NotNil(t, cmd, "confirmed action should execute directly")

	// For stash entries, the direct action is the stash action prompt.
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected stash action prompt, got %T", msg)
	assert.Equal(t, "Stash Action", modal.Title)
	assert.Equal(t, opStashAction, p.pending)
}

func TestDoAction_BlockedWhilePending(t *testing.T) {
	// doAction must be a no-op when a pending operation is already active.
	// This prevents Enter key-repeat after a modal dismissal from
	// triggering duplicate workflow dispatches (or any other action).
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{
		Confirmed: map[string]bool{
			string(actions.ItemLocalBranch): true,
		},
	}
	p.activeTab = tabBranches
	p.tabCursor[tabBranches] = 1 // "feature" branch

	// Simulate an in-flight pending operation (e.g. workflow dispatch).
	p.pending = opWorkflowDispatch
	p.pendingName = "123:ci"

	_, cmd := p.doAction()
	assert.Nil(t, cmd, "doAction should be blocked when pending != opNone")
	// pending state must be untouched.
	assert.Equal(t, opWorkflowDispatch, p.pending)
	assert.Equal(t, "123:ci", p.pendingName)
}

func TestDoAction_FirstUseWorktreeCursorReset(t *testing.T) {
	// Regression test: when a user double-clicks a worktree (first use),
	// the worktree path must be captured at double-click time. If a data
	// reload resets the cursor to 0 before the modal result arrives, the
	// panel should still use the originally-clicked worktree path.
	p := newTestPanel(t, defaultMock())
	p.actionsCfg = config.ActionsConfig{} // unconfirmed
	p.activeTab = tabWorktrees

	// Ensure there are multiple worktrees and cursor is on the second one.
	p.tabItems[tabWorktrees] = []listItem{
		{kind: kindWorktree, worktree: git.Worktree{Path: "/repo", Branch: "main"}},
		{kind: kindWorktree, worktree: git.Worktree{Path: "/worktrees/feature", Branch: "feature"}},
	}
	p.tabCursor[tabWorktrees] = 1 // user clicked "feature" worktree

	// Step 1: doAction shows the first-use modal.
	_, cmd := p.doAction()
	require.NotNil(t, cmd)
	assert.Equal(t, opFirstUseConfirm, p.pending)
	assert.Equal(t, "/worktrees/feature", p.pendingPath)

	// Simulate a data reload that resets cursor to 0 (e.g. from fsnotify).
	p.tabCursor[tabWorktrees] = 0

	// Step 2: User selects "change_directory" from the modal.
	_, cmd = p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  string(actions.ActionChangeDirectory),
	})
	require.NotNil(t, cmd, "accepting worktree change_directory should produce a command")

	// The command should produce an opResultMsg with the ORIGINAL path,
	// not the path at cursor 0.
	msg := cmd()
	result, ok := msg.(opResultMsg)
	require.True(t, ok, "expected opResultMsg, got %T", msg)
	assert.Equal(t, "worktree_switch", result.op)
	assert.Equal(t, "/worktrees/feature", result.name,
		"should use path captured at double-click time, not stale cursor")
}

// ---------------------------------------------------------------------------
// CI watch animation tests
// ---------------------------------------------------------------------------

func TestWatchFrames(t *testing.T) {
	// watchFrames should contain exactly the 4 expected Unicode frames.
	expected := []string{"●", "◐", "○", "◑"}
	assert.Equal(t, expected, watchFrames)
}

func TestActionsStatusIcon_WatchingCycles(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Status: "in_progress"}},
	}
	p.actionsWatching = true

	// Each frame index should produce the corresponding watchFrame character.
	for i, want := range watchFrames {
		p.actionsWatchFrame = i
		assert.Equal(t, want, p.actionsStatusIcon(), "frame %d", i)
	}
}

func TestActionsStatusIcon_NotWatchingFallback(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Status: "in_progress"}},
	}
	p.actionsWatching = false
	// When not watching, in-progress status should return the static "●".
	assert.Equal(t, "●", p.actionsStatusIcon())
}

func TestActionsStatusIcon_WatchingOverriddenByConclusion(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	p.actionsWatchFrame = 2

	// A completed (success) run should still show ✓ even if actionsWatching is true.
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Conclusion: "success"}},
	}
	assert.Equal(t, "✓", p.actionsStatusIcon())

	// Same for failure.
	p.tabItems[tabActions] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{Conclusion: "failure"}},
	}
	assert.Equal(t, "✗", p.actionsStatusIcon())
}

func TestActionsWatchTickCmd_WhenWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	cmd := p.actionsWatchTickCmd()
	require.NotNil(t, cmd, "actionsWatchTickCmd should return a command when watching")
}

func TestActionsWatchTickCmd_WhenNotWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = false
	cmd := p.actionsWatchTickCmd()
	assert.Nil(t, cmd, "actionsWatchTickCmd should return nil when not watching")
}

func TestActionsWatchTickMsg_AdvancesFrame(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	p.actionsWatchFrame = 0

	_, cmd := p.Update(actionsWatchTickMsg{time.Now()})
	assert.Equal(t, 1, p.actionsWatchFrame, "frame should advance from 0 to 1")
	assert.NotNil(t, cmd, "should schedule next tick")

	// Advance again — should wrap around after len(watchFrames)-1.
	p.actionsWatchFrame = len(watchFrames) - 1
	p.Update(actionsWatchTickMsg{time.Now()})
	assert.Equal(t, 0, p.actionsWatchFrame, "frame should wrap around to 0")
}

func TestActionsWatchTickMsg_StopsWhenNotWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = false
	p.actionsWatchFrame = 2

	_, cmd := p.Update(actionsWatchTickMsg{time.Now()})
	assert.Equal(t, 2, p.actionsWatchFrame, "frame should not advance when not watching")
	assert.Nil(t, cmd, "should not schedule next tick when not watching")
}

func TestHandleGHDataLoaded_StartsWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = false

	msg := ghDataLoadedMsg{
		actions: []ghActionItem{
			{RunID: 1, Status: "in_progress"},
			{RunID: 2, Status: "completed", Conclusion: "success"},
		},
	}

	_, cmd := p.Update(msg)
	assert.True(t, p.actionsWatching, "should start watching with in-progress runs")
	assert.Equal(t, 0, p.actionsWatchFrame, "frame should reset to 0 on start")
	assert.NotNil(t, cmd, "should start the watch tick")
}

func TestHandleGHDataLoaded_StopsWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	p.actionsWatchFrame = 3

	msg := ghDataLoadedMsg{
		actions: []ghActionItem{
			{RunID: 1, Status: "completed", Conclusion: "success"},
			{RunID: 2, Status: "completed", Conclusion: "failure"},
		},
	}

	_, cmd := p.Update(msg)
	assert.False(t, p.actionsWatching, "should stop watching when no in-progress runs")
	assert.Nil(t, cmd, "should not schedule watch tick")
}

func TestHandleGHDataLoaded_QueuedStartsWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = false

	msg := ghDataLoadedMsg{
		actions: []ghActionItem{
			{RunID: 1, Status: "queued"},
		},
	}

	_, cmd := p.Update(msg)
	assert.True(t, p.actionsWatching, "queued runs should also trigger watching")
	assert.NotNil(t, cmd, "should start the watch tick for queued runs")
}

func TestHandleGHDataLoaded_AlreadyWatchingNoRestart(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	p.actionsWatchFrame = 2

	msg := ghDataLoadedMsg{
		actions: []ghActionItem{
			{RunID: 1, Status: "in_progress"},
		},
	}

	_, cmd := p.Update(msg)
	assert.True(t, p.actionsWatching, "should remain watching")
	// When already watching, we don't restart the tick (frame stays at 2).
	assert.Equal(t, 2, p.actionsWatchFrame, "frame should not reset when already watching")
	assert.Nil(t, cmd, "should not restart the tick when already watching")
}

func TestHandleGHDataLoaded_ErrorDoesNotCrash(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true

	msg := ghDataLoadedMsg{
		err: fmt.Errorf("API error"),
	}

	_, cmd := p.Update(msg)
	// On error, actionsWatching should remain unchanged (not touched by error path).
	assert.True(t, p.actionsWatching, "error should not alter watching state")
	assert.Nil(t, cmd, "error should not produce commands")
}

func TestHandleGHDataLoaded_EmptyActionsStopsWatching(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true

	msg := ghDataLoadedMsg{
		actions: []ghActionItem{},
	}

	_, cmd := p.Update(msg)
	assert.False(t, p.actionsWatching, "empty actions should stop watching")
	assert.Nil(t, cmd, "should not schedule tick for empty actions")
}

// ---------------------------------------------------------------------------
// TargetedPanelMsg wrapping verification
// ---------------------------------------------------------------------------

func TestActionsWatchTickCmd_WrapsInTargetedPanelMsg(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.actionsWatching = true
	cmd := p.actionsWatchTickCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	targeted, ok := msg.(panels.TargetedPanelMsg)
	require.True(t, ok, "actionsWatchTickCmd should produce a TargetedPanelMsg, got %T", msg)
	assert.Equal(t, "gitinfo", targeted.Target)
	_, isInner := targeted.Inner.(actionsWatchTickMsg)
	assert.True(t, isInner, "Inner should be actionsWatchTickMsg, got %T", targeted.Inner)
}

func TestGithubPollTickCmd_WrapsInTargetedPanelMsg(t *testing.T) {
	ghMock := &mockGHClientFull{user: ghUser("u")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.gh.cfg.PollInterval = 1 // 1 second — fast for test
	cmd := p.githubPollTickCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	targeted, ok := msg.(panels.TargetedPanelMsg)
	require.True(t, ok, "githubPollTickCmd should produce a TargetedPanelMsg, got %T", msg)
	assert.Equal(t, "gitinfo", targeted.Target)
	_, isInner := targeted.Inner.(githubPollTickMsg)
	assert.True(t, isInner, "Inner should be githubPollTickMsg, got %T", targeted.Inner)
}

// ---------------------------------------------------------------------------
// Tab switching via keys (w, l) and pagination (d, u, D)
// ---------------------------------------------------------------------------

func TestWKey_SwitchesToWorktreesTab(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	assert.Equal(t, tabBranches, p.activeTab)
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'w'})
	assert.Equal(t, tabWorktrees, p.activeTab)
	assert.NotNil(t, cmd, "'w' should produce a tab-selection command")
}

func TestWKey_SwitchesToWorkflowsInGitHub(t *testing.T) {
	mock := defaultMock()
	p := newTestGitHubPanel(t, mock)
	p.Focused = true

	p.Update(tea.KeyPressMsg{Code: 'w'})
	assert.Equal(t, tabWorkflows, p.activeTab)
}

func TestLKey_SwitchesToReflogTab(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	p.Update(tea.KeyPressMsg{Code: 'l'})
	assert.Equal(t, tabReflog, p.activeTab)
}

func TestLKey_SwitchesToReleasesInGitHub(t *testing.T) {
	mock := defaultMock()
	p := newTestGitHubPanel(t, mock)
	p.Focused = true

	p.Update(tea.KeyPressMsg{Code: 'l'})
	assert.Equal(t, tabReleases, p.activeTab)
}

func TestDKey_PageDown(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	// Pressing PgDn should not panic; page down on the active tab.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.NotNil(t, cmd, "PgDn should produce a tab-selection command")
}

func TestUKey_PageUp(t *testing.T) {
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.Focused = true

	// Pressing PgUp should not panic; page up on the active tab.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	assert.NotNil(t, cmd, "PgUp should produce a tab-selection command")
}

func TestDKey_DispatchWorkflow(t *testing.T) {
	mock := defaultMock()
	p := newTestGitHubPanel(t, mock)
	p.Focused = true

	// Switch to workflows tab first.
	p.Update(tea.KeyPressMsg{Code: 'w'})
	assert.Equal(t, tabWorkflows, p.activeTab)

	// 'D' on workflows tab without ghClient should be a no-op (smoke test).
	_, cmd := p.Update(tea.KeyPressMsg{Code: -1, Text: "D"})
	// Without a real GH client, dispatch won't fire — just verify no panic.
	_ = cmd
}

// ---------------------------------------------------------------------------
// prColor — mergeable-state color mapping
// ---------------------------------------------------------------------------

func TestPRColor_OpenClean(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: "clean"}
	assert.Equal(t, defaultColors.PR, prColor(pr))
}

func TestPRColor_OpenEmpty(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: ""}
	assert.Equal(t, defaultColors.PR, prColor(pr), "empty mergeable state should default to green")
}

func TestPRColor_OpenDirty(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: "dirty"}
	assert.Equal(t, defaultColors.PRConflict, prColor(pr))
}

func TestPRColor_OpenUnstable(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: "unstable"}
	assert.Equal(t, defaultColors.PRUnstable, prColor(pr))
}

func TestPRColor_OpenBlocked(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: "blocked"}
	assert.Equal(t, defaultColors.PRBlocked, prColor(pr))
}

func TestPRColor_OpenUnknown(t *testing.T) {
	pr := ghPRItem{State: "open", MergeableState: "unknown"}
	assert.Equal(t, defaultColors.PRUnknown, prColor(pr))
}

func TestPRColor_Draft(t *testing.T) {
	pr := ghPRItem{State: "draft", MergeableState: "clean"}
	assert.Equal(t, defaultColors.PRDraft, prColor(pr), "draft state ignores mergeable")
}

func TestPRColor_Merged(t *testing.T) {
	pr := ghPRItem{State: prStateMerged}
	assert.Equal(t, defaultColors.PRMerged, prColor(pr))
}

func TestPRColor_Closed(t *testing.T) {
	pr := ghPRItem{State: "closed"}
	assert.Equal(t, defaultColors.PRClosed, prColor(pr))
}

// ---------------------------------------------------------------------------
// prActionIcon — action run status icon mapping
// ---------------------------------------------------------------------------

func TestPRActionIcon_Success(t *testing.T) {
	pr := ghPRItem{ActionConclusion: "success"}
	icon, color := prActionIcon(pr)
	assert.Equal(t, checkMark, icon)
	assert.Equal(t, defaultColors.ActionOK, color)
}

func TestPRActionIcon_Failure(t *testing.T) {
	pr := ghPRItem{ActionConclusion: "failure"}
	icon, color := prActionIcon(pr)
	assert.Equal(t, "✗", icon)
	assert.Equal(t, defaultColors.ActionFail, color)
}

func TestPRActionIcon_TimedOut(t *testing.T) {
	pr := ghPRItem{ActionConclusion: "timed_out"}
	icon, color := prActionIcon(pr)
	assert.Equal(t, "✗", icon)
	assert.Equal(t, defaultColors.ActionFail, color)
}

func TestPRActionIcon_InProgress(t *testing.T) {
	pr := ghPRItem{ActionStatus: "in_progress"}
	icon, color := prActionIcon(pr)
	assert.Equal(t, "●", icon)
	assert.Equal(t, defaultColors.ActionRun, color)
}

func TestPRActionIcon_Queued(t *testing.T) {
	pr := ghPRItem{ActionStatus: "queued"}
	icon, color := prActionIcon(pr)
	assert.Equal(t, "●", icon)
	assert.Equal(t, defaultColors.ActionRun, color)
}

func TestPRActionIcon_None(t *testing.T) {
	pr := ghPRItem{}
	icon, color := prActionIcon(pr)
	assert.Empty(t, icon)
	assert.Empty(t, color)
}

func TestPRActionIcon_ConclusionOverridesStatus(t *testing.T) {
	// When both conclusion and status are set, conclusion takes precedence.
	pr := ghPRItem{ActionStatus: "in_progress", ActionConclusion: "success"}
	icon, _ := prActionIcon(pr)
	assert.Equal(t, checkMark, icon, "conclusion should take precedence over status")
}

// ---------------------------------------------------------------------------
// renderPR — mergeable state coloring in rendered output
// ---------------------------------------------------------------------------

func TestRenderPR_OpenDirty_ContainsOpen(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 42, Title: "Fix conflicts", State: "open", MergeableState: "dirty",
	}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "#42")
	assert.Contains(t, line, "Fix conflicts")
	assert.Contains(t, line, "open")
}

func TestRenderPR_Closed(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 99, Title: "Old PR", State: "closed",
	}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "closed")
}

func TestRenderPR_WithActionIcon_Success(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 38, Title: "Update deps", State: "open",
		ActionStatus: "completed", ActionConclusion: "success",
	}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, checkMark, "success icon should appear")
}

func TestRenderPR_WithActionIcon_Failure(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 35, Title: "Refactor auth", State: "open",
		ActionStatus: "completed", ActionConclusion: "failure",
	}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "✗", "failure icon should appear")
}

func TestRenderPR_WithActionIcon_InProgress(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 42, Title: "Fix login redirect", State: "open",
		ActionStatus: "in_progress",
	}}
	line := p.renderPR(item, 80, false)
	assert.Contains(t, line, "●", "in-progress icon should appear")
}

func TestRenderPR_NoActionIcon_WhenNoRun(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 10, Title: "Simple PR", State: "open",
	}}
	line := p.renderPR(item, 80, false)
	assert.NotContains(t, line, checkMark)
	assert.NotContains(t, line, "✗")
	assert.NotContains(t, line, "●")
}

func TestRenderPR_ActionIcon_NarrowWidth(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindPR, pr: ghPRItem{
		Number: 42, Title: "Very long title that needs truncation", State: "open",
		ActionConclusion: "success",
	}}
	line := p.renderPR(item, 30, false)
	assert.Contains(t, line, "#42")
	assert.Contains(t, line, checkMark)
}

// ---------------------------------------------------------------------------
// loadGitHubData — action cross-reference
// ---------------------------------------------------------------------------

func TestBuildGitHubItems_ActionCrossReference(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	prs := []ghPRItem{
		{Number: 10, Title: "Auth", State: "open", HeadBranch: "main"},
		{Number: 11, Title: "Deploy", State: "open", HeadBranch: "feature"},
		{Number: 12, Title: "Other", State: "open", HeadBranch: "no-match"},
	}
	actions := []ghActionItem{
		{RunID: 100, Branch: "main", Status: "completed", Conclusion: "success"},
		{RunID: 101, Branch: "feature", Status: "in_progress", Conclusion: ""},
		{RunID: 102, Branch: "main", Status: "completed", Conclusion: "failure"}, // older, should not override
	}

	// Cross-reference manually (same logic as loadGitHubData)
	actionByBranch := make(map[string]ghActionItem, len(actions))
	for _, action := range actions {
		if _, exists := actionByBranch[action.Branch]; !exists {
			actionByBranch[action.Branch] = action
		}
	}
	for i, pr := range prs {
		if action, ok := actionByBranch[pr.HeadBranch]; ok {
			prs[i].ActionStatus = action.Status
			prs[i].ActionConclusion = action.Conclusion
		}
	}

	// Verify cross-reference results
	assert.Equal(t, "completed", prs[0].ActionStatus)
	assert.Equal(t, "success", prs[0].ActionConclusion, "should use first (most recent) action for main")
	assert.Equal(t, "in_progress", prs[1].ActionStatus)
	assert.Empty(t, prs[1].ActionConclusion)
	assert.Empty(t, prs[2].ActionStatus, "no matching action run for no-match branch")

	// Build items and verify render
	populateGH(p, nil, prs, actions)
	items := p.tabItems[tabPRs]
	require.Len(t, items, 3)

	line0 := p.renderPR(items[0], 80, false)
	assert.Contains(t, line0, checkMark, "PR #10 on main should show success icon")

	line1 := p.renderPR(items[1], 80, false)
	assert.Contains(t, line1, "●", "PR #11 on feature should show in-progress icon")

	line2 := p.renderPR(items[2], 80, false)
	assert.NotContains(t, line2, checkMark)
	assert.NotContains(t, line2, "✗")
	assert.NotContains(t, line2, "●")
}

// ---------------------------------------------------------------------------
// ANSI escape-sequence injection regression tests (CWE-150)
// ---------------------------------------------------------------------------

func TestRenderBranch_ANSIInjection(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindLocalBranch, branch: git.Branch{
		Name: "\x1b[31mevil-branch\x1b[0m",
		Hash: "abc1234",
	}}
	line := p.renderBranch(item, 80, false)
	stripped := panels.StripANSI(line)
	assert.NotContains(t, stripped, "\x1b", "ANSI in branch name should be stripped")
	assert.Contains(t, stripped, "evil-branch")
}

func TestRightClickLabel_ANSIInjection(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	tests := []struct {
		name string
		item listItem
		want string
	}{
		{
			name: "issue title",
			item: listItem{kind: kindIssue, issue: ghIssueItem{
				Number: 1,
				Title:  "\x1b[31mRed\x1b[0m title",
			}},
			want: "#1 Red title",
		},
		{
			name: "PR title",
			item: listItem{kind: kindPR, pr: ghPRItem{
				Number: 2,
				Title:  "feat: \x1b]0;pwned\x07attack",
			}},
			want: "#2 feat: attack",
		},
		{
			name: "branch name",
			item: listItem{kind: kindLocalBranch, branch: git.Branch{
				Name: "\x1b[1mbold-branch\x1b[0m",
			}},
			want: "bold-branch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := p.rightClickLabel(tt.item)
			assert.NotContains(t, label, "\x1b", "ANSI escapes should be stripped")
			assert.Contains(t, label, tt.want)
		})
	}
}

// ---------------------------------------------------------------------------
// Create pull request from the PRs tab (issue #249)
// ---------------------------------------------------------------------------

// pushedFeatureBranches returns a branch set with "feature" as the current,
// pushed local branch plus its remote counterpart and origin/main.
func pushedFeatureBranches() []git.Branch {
	return []git.Branch{
		{Name: "feature", IsCurrent: true, Upstream: "origin/feature"},
		{Name: "origin/feature", IsRemote: true},
		{Name: "origin/main", IsRemote: true},
	}
}

func TestDoCreatePR_PrefillsHeadAndBase(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabPRs
	p.gh.defaultBranch = "main"
	p.gitData.lastBranches = pushedFeatureBranches()

	_, cmd := p.doCreatePR()
	require.NotNil(t, cmd, "should open the head-branch input")
	assert.Equal(t, opPRCreateHead, p.pending)
	assert.Equal(t, "feature", p.prDraft.head, "head should prefill with current branch")
	assert.Equal(t, "main", p.prDraft.base, "base should prefill with default branch")
}

func TestDoCreatePR_DefaultBranchFallback(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabPRs
	p.gh.defaultBranch = "" // unknown default → fall back to main
	p.gitData.lastBranches = pushedFeatureBranches()

	_, cmd := p.doCreatePR()
	require.NotNil(t, cmd)
	assert.Equal(t, branchMain, p.prDraft.base)
}

func TestDoCreatePR_NoCurrentBranchWarns(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabPRs
	p.gitData.lastBranches = []git.Branch{{Name: "origin/main", IsRemote: true}}

	_, cmd := p.doCreatePR()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Equal(t, opNone, p.pending, "no modal flow should start")
}

func TestDoCreatePR_UnpushedBranchWarns(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabPRs
	// Current branch has no upstream and no matching remote branch.
	p.gitData.lastBranches = []git.Branch{{Name: "feature", IsCurrent: true}}

	_, cmd := p.doCreatePR()
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "Push branch")
	assert.Equal(t, opNone, p.pending)
}

func TestDoCreatePR_NilClientNoop(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock()) // no gh client wired
	p.gh.client = nil
	p.activeTab = tabPRs

	_, cmd := p.doCreatePR()
	assert.Nil(t, cmd, "no client should be a no-op")
}

func TestPRCreateFlow_EmptyTitleRejected(t *testing.T) {
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabPRs
	p.gh.defaultBranch = "main"
	p.gitData.lastBranches = pushedFeatureBranches()

	p.doCreatePR()
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "feature"}) // head
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"})    // base
	require.Equal(t, opPRCreateTitle, p.pending)

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "   "}) // blank title
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Equal(t, prCreateDraft{}, p.prDraft, "draft should reset on abort")
	assert.Zero(t, ghMock.createCalls, "CreatePR must not be called with an empty title")
}

func TestPRCreateFlow_HeadEqualsBaseRejected(t *testing.T) {
	ghMock := &mockGHClientFull{}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabPRs
	p.gh.defaultBranch = "main"
	p.gitData.lastBranches = pushedFeatureBranches()

	p.doCreatePR()
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"}) // head edited to main
	require.Equal(t, opPRCreateBase, p.pending)

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"}) // base == head
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast, got %T", msg)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Contains(t, toast.Message, "head and base")
	assert.Equal(t, prCreateDraft{}, p.prDraft)
	assert.Zero(t, ghMock.createCalls)
}

func TestPRCreateFlow_Success(t *testing.T) {
	num := 42
	createdPR := &gh.PullRequest{
		Number: &num,
		Title:  gh.Ptr("My new PR"),
		State:  gh.Ptr(prStateOpen),
		Head:   &gh.PullRequestBranch{Ref: gh.Ptr("feature")},
		User:   ghUser("me"),
	}
	ghMock := &mockGHClientFull{createdPR: createdPR, prs: []*gh.PullRequest{createdPR}}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.activeTab = tabPRs
	p.gh.defaultBranch = "main"
	p.gh.prFilter = prFilterMine // start on a filtered view to prove it resets
	p.gitData.lastBranches = pushedFeatureBranches()

	_, cmd := p.doCreatePR()
	require.NotNil(t, cmd)
	require.Equal(t, opPRCreateHead, p.pending)

	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "feature"})
	require.Equal(t, opPRCreateBase, p.pending)
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "main"})
	require.Equal(t, opPRCreateTitle, p.pending)
	p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "My new PR"})
	require.Equal(t, opPRCreateBody, p.pending)

	_, createCmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "Closes #249"})
	require.NotNil(t, createCmd)
	assert.Equal(t, prCreateDraft{}, p.prDraft, "draft should reset once the request fires")

	msg := createCmd()
	res, ok := msg.(prCreateResultMsg)
	require.True(t, ok, "expected prCreateResultMsg, got %T", msg)
	require.NoError(t, res.err)
	assert.Equal(t, 42, res.pr.Number)

	// The request should carry the exact head, base, title, and body entered.
	require.NotNil(t, ghMock.createReq)
	assert.Equal(t, 1, ghMock.createCalls)
	assert.Equal(t, "My new PR", ghMock.createReq.GetTitle())
	assert.Equal(t, "feature", ghMock.createReq.GetHead())
	assert.Equal(t, "main", ghMock.createReq.GetBase())
	assert.Equal(t, "Closes #249", ghMock.createReq.GetBody())

	// Handling the result inserts the PR, resets the filter, and queues reselect.
	_, afterCmd := p.handlePRCreateResult(res)
	require.NotNil(t, afterCmd)
	assert.Equal(t, prFilterAll, p.gh.prFilter, "filter should reset so the new PR is visible")
	require.NotEmpty(t, p.gh.allPRs)
	assert.Equal(t, 42, p.gh.allPRs[0].Number, "new PR inserted at the front")
	assert.Equal(t, 42, p.gh.pendingSelectPR, "reselect queued for the post-refresh load")
	// The visible cursor should land on the new PR.
	require.NotEmpty(t, p.tabItems[tabPRs])
	assert.Equal(t, 42, p.tabItems[tabPRs][p.tabCursor[tabPRs]].pr.Number)
}

func TestCreatePRCmd_OmitsEmptyBody(t *testing.T) {
	ghMock := &mockGHClientFull{createdPR: &gh.PullRequest{Number: gh.Ptr(1), Title: gh.Ptr("t"), State: gh.Ptr(prStateOpen), Head: &gh.PullRequestBranch{Ref: gh.Ptr("feature")}}}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.createPRCmd("feature", "main", "t", "")()
	res, ok := msg.(prCreateResultMsg)
	require.True(t, ok)
	require.NoError(t, res.err)
	require.NotNil(t, ghMock.createReq)
	assert.Nil(t, ghMock.createReq.Body, "empty body should be omitted from the request")
}

func TestCreatePRCmd_ErrorPropagates(t *testing.T) {
	ghMock := &mockGHClientFull{createErr: fmt.Errorf("api down")}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.createPRCmd("feature", "main", "t", "")()
	res, ok := msg.(prCreateResultMsg)
	require.True(t, ok)
	require.Error(t, res.err)
}

func TestCreatePRCmd_NilResultIsError(t *testing.T) {
	ghMock := &mockGHClientFull{} // createdPR nil, createErr nil
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.createPRCmd("feature", "main", "t", "")()
	res, ok := msg.(prCreateResultMsg)
	require.True(t, ok)
	require.Error(t, res.err, "a nil PR with no error should surface as an error")
}

func TestHandlePRCreateResult_ErrorShowsToast(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.activeTab = tabPRs

	_, cmd := p.handlePRCreateResult(prCreateResultMsg{err: fmt.Errorf("boom")})
	require.NotNil(t, cmd)
	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected a toast, got %T", msg)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "boom")
	assert.Empty(t, p.gh.allPRs, "no PR should be inserted on failure")
}

func TestHandleModalResultCancel_ResetsPRDraft(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.pending = opPRCreateTitle
	p.prDraft = prCreateDraft{head: "feature", base: "main"}

	p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Equal(t, prCreateDraft{}, p.prDraft, "cancel should clear the in-progress draft")
	assert.Equal(t, opNone, p.pending)
}

func TestHandleGHDataLoaded_ConsumesPendingSelectPR(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.activeTab = tabPRs
	p.gh.prFilter = prFilterAll
	p.gh.pendingSelectPR = 7

	p.Update(ghDataLoadedMsg{prs: []ghPRItem{
		{Number: 9, State: prStateOpen},
		{Number: 7, State: prStateOpen},
	}})

	require.Len(t, p.tabItems[tabPRs], 2)
	assert.Equal(t, 7, p.tabItems[tabPRs][p.tabCursor[tabPRs]].pr.Number, "cursor should land on the queued PR")
	assert.Zero(t, p.gh.pendingSelectPR, "pendingSelectPR should be consumed")
}

func TestHandleMetaLoaded_StoresDefaultBranch(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.Update(ghMetaLoadedMsg{user: "me", defaultBranch: "develop", repoPrivate: true})
	assert.Equal(t, "develop", p.gh.defaultBranch)
}

func TestNKey_OnPRsTabStartsCreateFlow(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.Focused = true
	p.activeTab = tabPRs
	p.gh.defaultBranch = "main"
	p.gitData.lastBranches = pushedFeatureBranches()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'n'})
	require.NotNil(t, cmd, "'n' on the PRs tab should open the create-PR flow")
	assert.Equal(t, opPRCreateHead, p.pending)
	assert.Equal(t, "feature", p.prDraft.head)
}

// GitHub notifications
// ---------------------------------------------------------------------------

func ghNotif(id, repo, title, typ, reason, apiURL string, unread bool) *gh.Notification {
	return &gh.Notification{
		ID:         &id,
		Reason:     &reason,
		Unread:     &unread,
		UpdatedAt:  &gh.Timestamp{Time: time.Date(2024, time.January, 2, 15, 4, 0, 0, time.UTC)},
		Repository: &gh.Repository{FullName: &repo},
		Subject:    &gh.NotificationSubject{Title: &title, Type: &typ, URL: &apiURL},
	}
}

func TestNotificationHTMLURL(t *testing.T) {
	tests := []struct {
		name   string
		apiURL string
		repo   string
		want   string
	}{
		{"issue", "https://api.github.com/repos/o/r/issues/1", "o/r", "https://github.com/o/r/issues/1"},
		{"pull maps to pull", "https://api.github.com/repos/o/r/pulls/9", "o/r", "https://github.com/o/r/pull/9"},
		{"commit maps to commit", "https://api.github.com/repos/o/r/commits/abc123", "o/r", "https://github.com/o/r/commit/abc123"},
		{"release falls back to repo", "https://api.github.com/repos/o/r/releases/5", "o/r", "https://github.com/o/r"},
		{"empty falls back to repo", "", "o/r", "https://github.com/o/r"},
		{"non-api falls back to repo", "https://example.com/x", "o/r", "https://github.com/o/r"},
		{"no repo no url", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, notificationHTMLURL(tt.apiURL, tt.repo))
		})
	}
}

func TestLoadNotifications_BuildsItems(t *testing.T) {
	ghMock := &mockGHClientFull{
		notifications: []*gh.Notification{
			ghNotif("1", "octo/repo", "Fix the bug", "Issue", "mention", "https://api.github.com/repos/octo/repo/issues/7", true),
			ghNotif("2", "octo/repo", "Add feature", "PullRequest", "review_requested", "https://api.github.com/repos/octo/repo/pulls/8", true),
		},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)

	msg := p.loadNotifications()()
	loaded, ok := msg.(ghNotificationsLoadedMsg)
	require.True(t, ok, "expected ghNotificationsLoadedMsg, got %T", msg)
	require.NoError(t, loaded.err)
	p.handleNotificationsLoaded(loaded)

	items := p.tabItems[tabNotifications]
	require.Len(t, items, 2)
	assert.Equal(t, kindNotification, items[0].kind)
	assert.Equal(t, "octo/repo", items[0].notif.RepoFullName)
	assert.Equal(t, "Fix the bug", items[0].notif.Title)
	assert.Equal(t, "Issue", items[0].notif.Type)
	assert.Equal(t, "mention", items[0].notif.Reason)
	assert.Equal(t, "https://github.com/octo/repo/issues/7", items[0].notif.HTMLURL)
	assert.True(t, items[0].notif.Unread)
	assert.Equal(t, "https://github.com/octo/repo/pull/8", items[1].notif.HTMLURL)
	assert.False(t, p.gh.notifLoading)
	assert.NoError(t, p.gh.notifErr)
}

func TestHandleNotificationsLoaded_Error(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.buildNotificationItems([]ghNotificationItem{{ThreadID: "x", Unread: true}})
	require.Len(t, p.tabItems[tabNotifications], 1)

	p.handleNotificationsLoaded(ghNotificationsLoadedMsg{err: fmt.Errorf("boom")})
	assert.Error(t, p.gh.notifErr)
	assert.Empty(t, p.tabItems[tabNotifications])
	assert.False(t, p.gh.notifLoading)
}

func TestMarkNotificationRead_StateChange(t *testing.T) {
	ghMock := &mockGHClientFull{
		notifications: []*gh.Notification{
			ghNotif("thread-1", "octo/repo", "Fix the bug", "Issue", "mention", "https://api.github.com/repos/octo/repo/issues/7", true),
		},
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	loaded := p.loadNotifications()().(ghNotificationsLoadedMsg)
	p.handleNotificationsLoaded(loaded)
	p.activeTab = tabNotifications
	p.tabCursor[tabNotifications] = 0

	_, cmd := p.doMarkNotificationRead()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(notificationReadResultMsg)
	require.True(t, ok, "expected notificationReadResultMsg, got %T", msg)
	require.NoError(t, result.err)
	assert.Equal(t, 1, ghMock.markReadCalls)
	assert.Equal(t, "thread-1", ghMock.markReadID)

	// Handling the result greys the row (Unread=false).
	assert.True(t, p.tabItems[tabNotifications][0].notif.Unread)
	p.handleNotificationReadResult(result)
	assert.False(t, p.tabItems[tabNotifications][0].notif.Unread, "row should be marked read")
}

func TestMarkNotificationRead_Error(t *testing.T) {
	ghMock := &mockGHClientFull{
		notifications: []*gh.Notification{
			ghNotif("t1", "octo/repo", "Bug", "Issue", "mention", "", true),
		},
		markReadErr: fmt.Errorf("network down"),
	}
	p := newGHPanelWithClient(t, defaultMock(), ghMock)
	p.handleNotificationsLoaded(p.loadNotifications()().(ghNotificationsLoadedMsg))
	p.activeTab = tabNotifications

	_, cmd := p.doMarkNotificationRead()
	require.NotNil(t, cmd)
	result := cmd().(notificationReadResultMsg)
	require.Error(t, result.err)

	// On error the row stays unread.
	p.handleNotificationReadResult(result)
	assert.True(t, p.tabItems[tabNotifications][0].notif.Unread, "row should stay unread on error")
}

func TestClampNotificationCursor(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})

	// Cursor beyond the list is clamped to the last item.
	p.tabCursor[tabNotifications] = 99
	p.buildNotificationItems([]ghNotificationItem{
		{ThreadID: "a", Unread: true},
		{ThreadID: "b", Unread: true},
	})
	assert.Equal(t, 1, p.tabCursor[tabNotifications])

	// Empty list clamps to 0 rather than a negative index.
	p.tabCursor[tabNotifications] = 5
	p.buildNotificationItems(nil)
	assert.Equal(t, 0, p.tabCursor[tabNotifications])
}

func TestRenderNotification_Content(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	item := listItem{kind: kindNotification, notif: ghNotificationItem{
		RepoFullName: "octo/repo", Title: "Fix the bug", Type: "Issue",
		Reason: "mention", UpdatedAt: "Jan 2 15:04", Unread: true,
	}}
	line := p.renderNotification(item, 120, false)
	assert.Contains(t, line, "octo/repo")
	assert.Contains(t, line, "Fix the bug")
	assert.Contains(t, line, "Issue")
	assert.Contains(t, line, "mention")
	assert.Contains(t, line, "Jan 2 15:04")
	assert.Contains(t, line, "●", "unread notifications use a filled marker")
}

func TestRenderNotification_ReadMarker(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	item := listItem{kind: kindNotification, notif: ghNotificationItem{
		RepoFullName: "octo/repo", Title: "Done", Unread: false,
	}}
	line := p.renderNotification(item, 120, false)
	assert.Contains(t, line, "○", "read notifications use a hollow marker")
}

func TestRenderLine_Notification(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	item := listItem{kind: kindNotification, notif: ghNotificationItem{
		RepoFullName: "o/r", Title: "Hi", Unread: true,
	}}
	line := p.renderLine(item, 120, false)
	assert.Contains(t, line, "o/r")
	assert.Contains(t, line, "Hi")
}

func TestNotificationsView_EmptyLoadingAndError(t *testing.T) {
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabNotifications
	p.tabItems[tabNotifications] = nil

	// Empty state.
	p.gh.notifLoading = false
	p.gh.notifErr = nil
	assert.Contains(t, p.View(80, 10), "No unread notifications")

	// Error state.
	p.gh.notifErr = fmt.Errorf("nope")
	assert.Contains(t, p.View(80, 10), "Notifications unavailable")

	// Loading state.
	p.gh.notifErr = nil
	p.gh.notifLoading = true
	assert.Contains(t, p.View(80, 10), "Loading...")
}

func TestKeyBindings_IncludesNotifications(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	found := false
	for _, b := range p.KeyBindings() {
		if b.Action == "tab_notifications" {
			found = true
		}
	}
	assert.True(t, found, "KeyBindings should include tab_notifications when a GitHub client is present")
}

func TestKeyTabSwitch_N(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.Focused = true
	p.Update(tea.KeyPressMsg{Code: -1, Text: "N"})
	assert.Equal(t, tabNotifications, p.activeTab)
}

func TestGhNotifCountStr_CountsUnread(t *testing.T) {
	p := newGHPanelWithClient(t, defaultMock(), &mockGHClientFull{})
	p.buildNotificationItems([]ghNotificationItem{
		{ThreadID: "a", Unread: true},
		{ThreadID: "b", Unread: false},
		{ThreadID: "c", Unread: true},
	})
	assert.Equal(t, "2", p.ghNotifCountStr())
}

func newFollowTestPanel(t *testing.T, ghMock *mockGHClientFull) *Panel {
	t.Helper()
	p := newTestGitHubPanel(t, defaultMock())
	p.gh.client = ghMock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.activeTab = tabActions
	p.tabItems[tabActions] = []listItem{{kind: kindActionRun, actionRun: ghActionItem{RunID: 100, Status: statusInProgress, RunNumber: 1}}}
	p.tabCursor[tabActions] = 0
	return p
}

func TestActionFollowRefreshSchedulingUsesFreshLogs(t *testing.T) {
	ghMock := &mockGHClientFull{
		jobs:   []*gh.WorkflowJob{{ID: gh.Ptr(int64(200)), Status: gh.Ptr(statusInProgress)}},
		jobLog: "first log",
		run:    &gh.WorkflowRun{Status: gh.Ptr(statusInProgress)},
	}
	p := newFollowTestPanel(t, ghMock)

	_, cmd := p.doActionsFollowToggle()
	require.NotNil(t, cmd)
	assert.True(t, p.actionFollowing)

	msg := p.fetchActionFollowLogCmd(p.actionFollowRunID, p.actionFollowJobID)()
	result, ok := msg.(actionFollowLogMsg)
	require.True(t, ok)
	_, cmd = p.handleActionFollowLog(result)
	assert.NotNil(t, cmd)
	assert.Equal(t, 1, ghMock.jobLogFreshCalls)
	assert.Equal(t, int64(200), p.actionFollowJobID)

	_, cmd = p.Update(actionFollowTickMsg{})
	require.NotNil(t, cmd)
	msg = cmd()
	_, ok = msg.(actionFollowLogMsg)
	require.True(t, ok)
	assert.Equal(t, 2, ghMock.jobLogFreshCalls)
}

func TestActionFollowStopsOnTerminalRun(t *testing.T) {
	ghMock := &mockGHClientFull{
		jobs:   []*gh.WorkflowJob{{ID: gh.Ptr(int64(200)), Status: gh.Ptr("completed")}},
		jobLog: "final log",
		run:    &gh.WorkflowRun{Status: gh.Ptr("completed")},
	}
	p := newFollowTestPanel(t, ghMock)
	p.actionFollowing = true
	p.actionFollowRunID = 100

	msg := p.fetchActionFollowLogCmd(100, 200)()
	result, ok := msg.(actionFollowLogMsg)
	require.True(t, ok)
	assert.True(t, result.terminal)

	_, cmd := p.handleActionFollowLog(result)
	assert.False(t, p.actionFollowing)
	assert.NotNil(t, cmd)

	_, cmd = p.Update(actionFollowTickMsg{})
	assert.Nil(t, cmd)
}

func TestActionFollowErrorDisplaysToastAndRetries(t *testing.T) {
	p := newFollowTestPanel(t, &mockGHClientFull{})
	p.actionFollowing = true
	p.actionFollowRunID = 100

	_, cmd := p.handleActionFollowLog(actionFollowLogMsg{runID: 100, err: assert.AnError})
	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)

	var foundToast bool
	for _, batchedCmd := range batch {
		if batchedCmd == nil {
			continue
		}
		toast, ok := batchedCmd().(notify.ShowToastMsg)
		if ok {
			foundToast = true
			assert.Equal(t, notify.Error, toast.Level)
			assert.Contains(t, toast.Message, "Follow logs:")
		}
	}
	assert.True(t, foundToast, "expected error toast in follow error batch")
	assert.True(t, p.actionFollowing)
}

func TestActionFollowPausedSkipsFetch(t *testing.T) {
	ghMock := &mockGHClientFull{
		jobs:   []*gh.WorkflowJob{{ID: gh.Ptr(int64(200)), Status: gh.Ptr(statusInProgress)}},
		jobLog: "log",
		run:    &gh.WorkflowRun{Status: gh.Ptr(statusInProgress)},
	}
	p := newFollowTestPanel(t, ghMock)
	p.actionFollowing = true
	p.actionFollowPaused = true
	p.actionFollowRunID = 100
	p.actionFollowJobID = 200

	_, cmd := p.Update(actionFollowTickMsg{})
	assert.NotNil(t, cmd)
	assert.Zero(t, ghMock.jobLogFreshCalls)
}
