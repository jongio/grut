package commits

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

// mockGitOps is a test double for the gitOps interface.
type mockGitOps struct {
	commits  []git.Commit
	err      error
	lastOpts git.LogOpts
}

func (m *mockGitOps) Log(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
	m.lastOpts = opts
	if m.err != nil {
		return nil, m.err
	}

	// Simulate pagination.
	start := opts.Skip
	if start >= len(m.commits) {
		return nil, nil
	}
	end := start + opts.MaxCount
	if end > len(m.commits) {
		end = len(m.commits)
	}
	return m.commits[start:end], nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultCommits() []git.Commit {
	return []git.Commit{
		{
			Hash:        "abc1234567890",
			ShortHash:   "abc1234",
			Author:      "Alice",
			AuthorEmail: "alice@example.com",
			Date:        time.Now().Add(-1 * time.Hour),
			Subject:     "Initial commit",
		},
		{
			Hash:        "def5678901234",
			ShortHash:   "def5678",
			Author:      "Bob",
			AuthorEmail: "bob@example.com",
			Date:        time.Now().Add(-2 * time.Hour),
			Subject:     "Add feature",
		},
		{
			Hash:        "ghi9012345678",
			ShortHash:   "ghi9012",
			Author:      "Charlie",
			AuthorEmail: "charlie@example.com",
			Date:        time.Now().Add(-24 * time.Hour),
			Subject:     "Fix bug",
		},
	}
}

func newTestPanel(mock *mockGitOps) *Panel {
	p := New(mock, nil)
	cmd := p.Init(context.Background())
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}
	return p
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestInitialLoad(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	if len(p.commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(p.commits))
	}
	if p.loading {
		t.Error("expected loading=false after initial load")
	}
	if p.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", p.cursor)
	}
	// Default ref should be empty (HEAD).
	if mock.lastOpts.Ref != "" {
		t.Errorf("expected empty ref for HEAD, got %q", mock.lastOpts.Ref)
	}
}

func TestBranchChangedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// Simulate branch change.
	_, cmd := p.Update(panels.BranchChangedMsg{Name: "feature"})
	if cmd == nil {
		t.Fatal("expected a command to reload commits")
	}

	// Execute the reload command.
	msg := cmd()
	p.Update(msg)

	if p.ref != "feature" {
		t.Errorf("expected ref=%q, got %q", "feature", p.ref)
	}
	if p.refLabel != "feature" {
		t.Errorf("expected refLabel=%q, got %q", "feature", p.refLabel)
	}
	if mock.lastOpts.Ref != "feature" {
		t.Errorf("expected LogOpts.Ref=%q, got %q", "feature", mock.lastOpts.Ref)
	}
	if p.Title() != "Commits: feature" {
		t.Errorf("expected title %q, got %q", "Commits: feature", p.Title())
	}
}

func TestChangeDirectoryMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.ChangeDirectoryMsg{Path: "/test/worktree"})
	if cmd == nil {
		t.Fatal("expected a command to reload commits")
	}

	msg := cmd()
	p.Update(msg)

	if p.refLabel != "/test/worktree" {
		t.Errorf("expected refLabel=%q, got %q", "/test/worktree", p.refLabel)
	}
}

func TestWorktreeChangedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.WorktreeChangedMsg{})
	if cmd == nil {
		t.Fatal("expected a command to reload commits on WorktreeChangedMsg")
	}
}

func TestCursorNavigation(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Move down.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	if p.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", p.cursor)
	}

	// Move up.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", p.cursor)
	}

	// Can't go above 0.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 at top, got %d", p.cursor)
	}

	// Go to bottom.
	p.Update(tea.KeyPressMsg{Code: -1, Text: "G"})
	if p.cursor != 2 {
		t.Errorf("expected cursor=2 after G, got %d", p.cursor)
	}

	// Go to top.
	p.Update(tea.KeyPressMsg{Code: 'g'})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 after g, got %d", p.cursor)
	}
}

func TestSearchMode(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Enter search mode.
	p.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	if !p.searchMode {
		t.Fatal("expected searchMode=true after /")
	}

	// Type search query "bug".
	p.Update(tea.KeyPressMsg{Code: 'b'})
	p.Update(tea.KeyPressMsg{Code: 'u'})
	p.Update(tea.KeyPressMsg{Code: 'g'})

	if p.searchQuery != "bug" {
		t.Errorf("expected searchQuery=%q, got %q", "bug", p.searchQuery)
	}
	if len(p.filteredIdx) != 1 {
		t.Fatalf("expected 1 match for 'bug', got %d", len(p.filteredIdx))
	}

	// The filtered commit should be "Fix bug" (index 2).
	if p.filteredIdx[0] != 2 {
		t.Errorf("expected filteredIdx[0]=2, got %d", p.filteredIdx[0])
	}

	// Exit search with Escape clears filter.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.searchMode {
		t.Error("expected searchMode=false after Escape")
	}
	if p.filteredIdx != nil {
		t.Error("expected filteredIdx=nil after Escape")
	}
}

func TestSearchByAuthor(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Enter search and type "alice".
	p.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	for _, ch := range "alice" {
		p.Update(tea.KeyPressMsg{Code: ch})
	}

	if len(p.filteredIdx) != 1 {
		t.Fatalf("expected 1 match for 'alice', got %d", len(p.filteredIdx))
	}
	if p.filteredIdx[0] != 0 {
		t.Errorf("expected match at index 0, got %d", p.filteredIdx[0])
	}
}

func TestCommitSelection(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Select commit with Enter.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.detailMode {
		t.Fatal("Enter should select, not enter detail mode")
	}
	if p.selectedHash != "abc1234567890" {
		t.Errorf("expected selectedHash=%q, got %q", "abc1234567890", p.selectedHash)
	}

	// Verify CommitSelectedMsg is emitted.
	if cmd == nil {
		t.Fatal("expected a command from Enter")
	}
	msg := cmd()
	csm, ok := msg.(panels.CommitSelectedMsg)
	if !ok {
		t.Fatalf("expected CommitSelectedMsg, got %T", msg)
	}
	if csm.Hash != "abc1234567890" {
		t.Errorf("expected hash=%q, got %q", "abc1234567890", csm.Hash)
	}

	// Escape deselects (progressive reset).
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.selectedHash != "" {
		t.Error("expected selectedHash cleared after Escape")
	}
	if cmd == nil {
		t.Fatal("expected CommitDeselectedMsg command from Escape")
	}
	dmsg := cmd()
	if _, ok := dmsg.(panels.CommitDeselectedMsg); !ok {
		t.Fatalf("expected CommitDeselectedMsg, got %T", dmsg)
	}
}

func TestEmptyState(t *testing.T) {
	mock := &mockGitOps{commits: nil}
	p := newTestPanel(mock)

	if len(p.commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(p.commits))
	}

	view := p.View(80, 20)
	if view == "" {
		t.Error("expected non-empty view for empty state")
	}
	if !containsSubstring(view, "No commits") {
		t.Error("expected 'No commits' in empty state view")
	}
}

func TestPagination(t *testing.T) {
	// Create more commits than page size.
	var manyCommits []git.Commit
	for i := 0; i < 15; i++ {
		manyCommits = append(manyCommits, git.Commit{
			Hash:        "hash" + string(rune('a'+i)),
			ShortHash:   "h" + string(rune('a'+i)),
			Author:      "Author",
			AuthorEmail: "a@b.com",
			Date:        time.Now().Add(-time.Duration(i) * time.Hour),
			Subject:     "Commit " + string(rune('a'+i)),
		})
	}

	mock := &mockGitOps{commits: manyCommits}
	p := New(mock, nil)
	p.pageSize = 5 // Small page for testing.

	cmd := p.Init(context.Background())
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}

	if len(p.commits) != 5 {
		t.Fatalf("expected 5 commits after first page, got %d", len(p.commits))
	}
	if p.allLoaded {
		t.Error("expected allLoaded=false after first page")
	}

	// Move cursor near bottom to trigger pagination.
	p.Focus()
	p.SetSize(80, 20)
	p.cursor = 4 // Near bottom.
	_, cmd = p.moveCursorDown()

	// cursor shouldn't move past max, but loadMore should be called.
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}

	if len(p.commits) != 10 {
		t.Fatalf("expected 10 commits after second page, got %d", len(p.commits))
	}
}

func TestCopyHash(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("expected a command from y")
	}
}

func TestUnfocusedIgnoresKeys(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	// Don't focus the panel.

	p.Update(tea.KeyPressMsg{Code: 'j'})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 when unfocused, got %d", p.cursor)
	}
}

func TestViewRendersCommits(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	view := p.View(80, 20)
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !containsSubstring(view, "abc1234") {
		t.Error("expected short hash in view")
	}
	if !containsSubstring(view, "Initial commit") {
		t.Error("expected commit subject in view")
	}
}

func TestTitleDefault(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	if p.Title() != "Commits" {
		t.Errorf("expected title=%q, got %q", "Commits", p.Title())
	}
}

func TestLoadingState(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := New(mock, nil)

	// Before Init runs, loading is false and commits empty.
	cmd := p.Init(context.Background())
	if !p.loading {
		t.Error("expected loading=true after Init")
	}

	view := p.View(80, 20)
	if !containsSubstring(view, "Loading commits") {
		t.Error("expected loading indicator in view")
	}

	// Complete loading.
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}
	if p.loading {
		t.Error("expected loading=false after commits loaded")
	}
}

func TestPageDownUp(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 2) // Small viewport.

	// Page down.
	p.Update(tea.KeyPressMsg{Code: 'd'})
	if p.cursor != 1 { // height/2 = 1
		t.Errorf("expected cursor=1 after page down, got %d", p.cursor)
	}

	// Page up.
	p.Update(tea.KeyPressMsg{Code: 'u'})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 after page up, got %d", p.cursor)
	}
}

// containsSubstring checks whether s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Filter state tests (contextual commits)
// ---------------------------------------------------------------------------

func TestFileSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.FileSelectedMsg{Path: "/repo/src/main.go"})
	if cmd == nil {
		t.Fatal("expected reload command on FileSelectedMsg")
	}

	if p.filter != filterFile {
		t.Errorf("expected filter=filterFile, got %d", p.filter)
	}
	if p.filterPath != "/repo/src/main.go" {
		t.Errorf("expected filterPath=%q, got %q", "/repo/src/main.go", p.filterPath)
	}
	if p.filterLabel != "main.go" {
		t.Errorf("expected filterLabel=%q, got %q", "main.go", p.filterLabel)
	}
	if p.ref != "" {
		t.Errorf("expected ref cleared, got %q", p.ref)
	}
	if p.Title() != "Commits: main.go" {
		t.Errorf("expected title=%q, got %q", "Commits: main.go", p.Title())
	}

	// Execute the reload cmd and verify LogOpts.Path is passed.
	msg := cmd()
	p.Update(msg)
	if mock.lastOpts.Path != "/repo/src/main.go" {
		t.Errorf("expected LogOpts.Path=%q, got %q", "/repo/src/main.go", mock.lastOpts.Path)
	}
}

func TestFolderSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.FolderSelectedMsg{Path: "/repo/internal/git"})
	if cmd == nil {
		t.Fatal("expected reload command on FolderSelectedMsg")
	}

	if p.filter != filterFolder {
		t.Errorf("expected filter=filterFolder, got %d", p.filter)
	}
	if p.filterPath != "/repo/internal/git" {
		t.Errorf("expected filterPath=%q, got %q", "/repo/internal/git", p.filterPath)
	}
	// Label should include parent + base: "internal/git"
	if p.filterLabel != "internal/git" {
		t.Errorf("expected filterLabel=%q, got %q", "internal/git", p.filterLabel)
	}
	if p.Title() != "Commits: internal/git" {
		t.Errorf("expected title=%q, got %q", "Commits: internal/git", p.Title())
	}

	// Execute the reload cmd and verify LogOpts.Path is passed.
	msg := cmd()
	p.Update(msg)
	if mock.lastOpts.Path != "/repo/internal/git" {
		t.Errorf("expected LogOpts.Path=%q, got %q", "/repo/internal/git", mock.lastOpts.Path)
	}
}

func TestBranchSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.BranchSelectedMsg{Name: "feature/auth"})
	if cmd == nil {
		t.Fatal("expected reload command on BranchSelectedMsg")
	}

	if p.filter != filterBranch {
		t.Errorf("expected filter=filterBranch, got %d", p.filter)
	}
	if p.ref != "feature/auth" {
		t.Errorf("expected ref=%q, got %q", "feature/auth", p.ref)
	}
	if p.filterLabel != "feature/auth" {
		t.Errorf("expected filterLabel=%q, got %q", "feature/auth", p.filterLabel)
	}
	if p.Title() != "Commits: feature/auth" {
		t.Errorf("expected title=%q, got %q", "Commits: feature/auth", p.Title())
	}

	// Verify no path filter is set for branch filtering.
	msg := cmd()
	p.Update(msg)
	if mock.lastOpts.Path != "" {
		t.Errorf("expected empty LogOpts.Path for branch filter, got %q", mock.lastOpts.Path)
	}
	if mock.lastOpts.Ref != "feature/auth" {
		t.Errorf("expected LogOpts.Ref=%q, got %q", "feature/auth", mock.lastOpts.Ref)
	}
}

func TestWorktreeSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.WorktreeSelectedMsg{Path: "/wt/feature", Branch: "feature"})
	if cmd == nil {
		t.Fatal("expected reload command on WorktreeSelectedMsg")
	}

	if p.filter != filterWorktree {
		t.Errorf("expected filter=filterWorktree, got %d", p.filter)
	}
	if p.ref != "feature" {
		t.Errorf("expected ref=%q, got %q", "feature", p.ref)
	}
	if p.filterLabel != "feature" {
		t.Errorf("expected filterLabel=%q, got %q", "feature", p.filterLabel)
	}
	if p.Title() != "Commits: feature" {
		t.Errorf("expected title=%q, got %q", "Commits: feature", p.Title())
	}
}

func TestStashSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.StashSelectedMsg{Index: 2, Hash: "abc123"})
	if cmd == nil {
		t.Fatal("expected reload command on StashSelectedMsg")
	}

	if p.filter != filterStash {
		t.Errorf("expected filter=filterStash, got %d", p.filter)
	}
	if p.ref != "abc123" {
		t.Errorf("expected ref=%q, got %q", "abc123", p.ref)
	}
	if p.filterLabel != "stash@{2}" {
		t.Errorf("expected filterLabel=%q, got %q", "stash@{2}", p.filterLabel)
	}
	if p.Title() != "Commits: stash@{2}" {
		t.Errorf("expected title=%q, got %q", "Commits: stash@{2}", p.Title())
	}
}

func TestRemoteSelectedMsg(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	_, cmd := p.Update(panels.RemoteSelectedMsg{Name: "origin"})
	if cmd == nil {
		t.Fatal("expected reload command on RemoteSelectedMsg")
	}

	if p.filter != filterRemote {
		t.Errorf("expected filter=filterRemote, got %d", p.filter)
	}
	if p.filterLabel != "origin" {
		t.Errorf("expected filterLabel=%q, got %q", "origin", p.filterLabel)
	}
	if p.Title() != "Commits: origin" {
		t.Errorf("expected title=%q, got %q", "Commits: origin", p.Title())
	}
}

func TestEscapeClearsFilter(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Set a file filter.
	_, cmd := p.Update(panels.FileSelectedMsg{Path: "/repo/main.go"})
	if cmd != nil {
		msg := cmd()
		p.Update(msg)
	}

	if p.filter != filterFile {
		t.Fatal("expected filter to be set before Escape")
	}

	// Press Escape to clear filter.
	_, cmd = p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected reload command on Escape")
	}

	if p.filter != filterNone {
		t.Errorf("expected filter=filterNone after Escape, got %d", p.filter)
	}
	if p.filterPath != "" {
		t.Errorf("expected filterPath cleared after Escape, got %q", p.filterPath)
	}
	if p.filterLabel != "" {
		t.Errorf("expected filterLabel cleared after Escape, got %q", p.filterLabel)
	}
	if p.Title() != "Commits" {
		t.Errorf("expected title=%q after Escape, got %q", "Commits", p.Title())
	}
}

func TestEscapeNoOpWithoutFilter(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Press Escape without any filter — should be a no-op.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Error("expected no command from Escape when no filter is active")
	}
}

func TestFileSelectedMsg_IgnoredWhenCommitSelected(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Select a commit first.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.selectedHash == "" {
		t.Fatal("expected a commit to be selected")
	}

	oldFilter := p.filter
	oldLoading := p.loading

	// Now send FileSelectedMsg — should be ignored.
	_, cmd := p.Update(panels.FileSelectedMsg{Path: "/repo/src/main.go"})
	if cmd != nil {
		t.Error("expected no reload command when commit is selected")
	}
	if p.filter != oldFilter {
		t.Errorf("expected filter unchanged, got %d", p.filter)
	}
	if p.loading != oldLoading {
		t.Error("expected loading state unchanged")
	}
}

func TestFolderSelectedMsg_IgnoredWhenCommitSelected(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Select a commit first.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.selectedHash == "" {
		t.Fatal("expected a commit to be selected")
	}

	oldFilter := p.filter

	// Now send FolderSelectedMsg — should be ignored.
	_, cmd := p.Update(panels.FolderSelectedMsg{Path: "/repo/internal/git"})
	if cmd != nil {
		t.Error("expected no reload command when commit is selected")
	}
	if p.filter != oldFilter {
		t.Errorf("expected filter unchanged, got %d", p.filter)
	}
}

func TestFileSelectedMsg_WorksAfterDeselect(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Select then deselect a commit.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.selectedHash != "" {
		t.Fatal("expected commit to be deselected")
	}

	// Now FileSelectedMsg should work normally.
	_, cmd := p.Update(panels.FileSelectedMsg{Path: "/repo/src/main.go"})
	if cmd == nil {
		t.Error("expected reload command after commit is deselected")
	}
	if p.filter != filterFile {
		t.Errorf("expected filter=filterFile, got %d", p.filter)
	}
}

// ---------------------------------------------------------------------------
// Detail view tests
// ---------------------------------------------------------------------------

func TestShowDetail(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Trigger detail view by calling showDetail directly.
	cmd := p.showDetail()

	if !p.detailMode {
		t.Error("expected detailMode=true after showDetail")
	}
	if len(p.detailLines) == 0 {
		t.Error("expected detailLines to be populated")
	}
	if p.detailOffset != 0 {
		t.Errorf("expected detailOffset=0, got %d", p.detailOffset)
	}
	// showDetail should emit a CommitSelectedMsg.
	if cmd == nil {
		t.Fatal("expected a command from showDetail")
	}
	msg := cmd()
	csm, ok := msg.(panels.CommitSelectedMsg)
	if !ok {
		t.Fatalf("expected CommitSelectedMsg, got %T", msg)
	}
	if csm.Hash != "abc1234567890" {
		t.Errorf("expected hash=%q, got %q", "abc1234567890", csm.Hash)
	}

	// Detail lines should contain commit info.
	joined := ""
	for _, l := range p.detailLines {
		joined += l + "\n"
	}
	if !containsSubstring(joined, "Commit Details") {
		t.Error("expected 'Commit Details' in detail lines")
	}
	if !containsSubstring(joined, "abc1234567890") {
		t.Error("expected full hash in detail lines")
	}
	if !containsSubstring(joined, "Alice") {
		t.Error("expected author name in detail lines")
	}
	if !containsSubstring(joined, "Initial commit") {
		t.Error("expected subject in detail lines")
	}
}

func TestShowDetail_EmptyCommits(t *testing.T) {
	mock := &mockGitOps{commits: nil}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// cursor is out of range with no commits — showDetail should no-op.
	cmd := p.showDetail()
	if p.detailMode {
		t.Error("expected detailMode=false when no commits")
	}
	if cmd != nil {
		t.Error("expected nil cmd when no commits")
	}
}

func TestShowDetail_WithBodyAndRefs(t *testing.T) {
	commits := []git.Commit{
		{
			Hash:        "aaabbbccc111222",
			ShortHash:   "aaabbbc",
			Author:      "Dave",
			AuthorEmail: "dave@example.com",
			Date:        time.Now().Add(-1 * time.Hour),
			Subject:     "Big feature",
			Body:        "Detailed description\nof the change.",
			Refs:        []string{"HEAD", "main"},
			Parents:     []string{"deadbeef", "cafebabe"},
		},
	}
	mock := &mockGitOps{commits: commits}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.showDetail()
	if !p.detailMode {
		t.Fatal("expected detailMode=true")
	}

	joined := ""
	for _, l := range p.detailLines {
		joined += l + "\n"
	}
	if !containsSubstring(joined, "HEAD") {
		t.Error("expected refs in detail lines")
	}
	if !containsSubstring(joined, "deadbeef") {
		t.Error("expected parents in detail lines")
	}
	if !containsSubstring(joined, "Detailed description") {
		t.Error("expected body in detail lines")
	}
}

// ---------------------------------------------------------------------------
// Detail key handling tests
// ---------------------------------------------------------------------------

func TestHandleDetailKey_Navigation(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 10)

	// Enter detail mode.
	p.showDetail()
	if !p.detailMode {
		t.Fatal("expected detailMode=true")
	}

	// Scroll down with j.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	if p.detailOffset != 1 {
		t.Errorf("expected detailOffset=1 after j, got %d", p.detailOffset)
	}

	// Scroll down more.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	if p.detailOffset != 2 {
		t.Errorf("expected detailOffset=2 after second j, got %d", p.detailOffset)
	}

	// Scroll up with k.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	if p.detailOffset != 1 {
		t.Errorf("expected detailOffset=1 after k, got %d", p.detailOffset)
	}

	// Scroll up past 0 stays at 0.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	p.Update(tea.KeyPressMsg{Code: 'k'})
	if p.detailOffset != 0 {
		t.Errorf("expected detailOffset=0 at top, got %d", p.detailOffset)
	}
}

func TestHandleDetailKey_PageDownUp(t *testing.T) {
	// Create a commit with a long body so detailLines is long.
	commits := []git.Commit{
		{
			Hash:        "aaabbbccc111222",
			ShortHash:   "aaabbbc",
			Author:      "Dave",
			AuthorEmail: "dave@example.com",
			Date:        time.Now(),
			Subject:     "Big feature",
			Body:        "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\nline13\nline14\nline15\nline16\nline17\nline18\nline19\nline20",
		},
	}
	mock := &mockGitOps{commits: commits}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 10) // height=10

	p.showDetail()

	// Page down with d.
	p.Update(tea.KeyPressMsg{Code: 'd'})
	expected := 10 / 2 // height/2 = 5
	if p.detailOffset != expected {
		t.Errorf("expected detailOffset=%d after d, got %d", expected, p.detailOffset)
	}

	// Page up with u.
	p.Update(tea.KeyPressMsg{Code: 'u'})
	if p.detailOffset != 0 {
		t.Errorf("expected detailOffset=0 after u, got %d", p.detailOffset)
	}

	// Page up from 0 stays at 0.
	p.Update(tea.KeyPressMsg{Code: 'u'})
	if p.detailOffset != 0 {
		t.Errorf("expected detailOffset=0 stays at 0, got %d", p.detailOffset)
	}
}

func TestHandleDetailKey_ExitWithEsc(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.showDetail()
	if !p.detailMode {
		t.Fatal("expected detailMode=true")
	}

	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.detailMode {
		t.Error("expected detailMode=false after Escape")
	}
	if p.detailLines != nil {
		t.Error("expected detailLines=nil after Escape")
	}
	if p.detailOffset != 0 {
		t.Errorf("expected detailOffset=0 after Escape, got %d", p.detailOffset)
	}
}

func TestHandleDetailKey_ExitWithEnter(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.showDetail()

	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.detailMode {
		t.Error("expected detailMode=false after Enter in detail view")
	}
}

func TestHandleDetailKey_ExitWithQ(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.showDetail()

	p.Update(tea.KeyPressMsg{Code: 'q'})
	if p.detailMode {
		t.Error("expected detailMode=false after q in detail view")
	}
}

// ---------------------------------------------------------------------------
// renderDetail tests
// ---------------------------------------------------------------------------

func TestRenderDetail(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.showDetail()

	view := p.View(80, 20)
	if view == "" {
		t.Error("expected non-empty view in detail mode")
	}
	if !containsSubstring(view, "Commit Details") {
		t.Error("expected 'Commit Details' in detail view")
	}
}

func TestRenderDetail_EmptyLines(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// Force detail mode with empty lines.
	p.detailMode = true
	p.detailLines = nil

	view := p.View(80, 20)
	if view != "" {
		t.Error("expected empty view when detailLines is nil")
	}
}

func TestRenderDetail_Scrolled(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 5) // small viewport

	p.showDetail()
	// Scroll past the first line.
	p.detailOffset = 2

	view := p.View(80, 5)
	if view == "" {
		t.Error("expected non-empty view in scrolled detail mode")
	}
}

// ---------------------------------------------------------------------------
// Mouse click tests
// ---------------------------------------------------------------------------

func TestHandleMouseClick(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Click on the second commit (row 1, offset 0).
	_, cmd := p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 0})
	if p.cursor != 1 {
		t.Errorf("expected cursor=1 after click, got %d", p.cursor)
	}
	// Should select the commit.
	if cmd == nil {
		t.Fatal("expected a command from mouse click")
	}
	msg := cmd()
	csm, ok := msg.(panels.CommitSelectedMsg)
	if !ok {
		t.Fatalf("expected CommitSelectedMsg, got %T", msg)
	}
	if csm.Hash != "def5678901234" {
		t.Errorf("expected hash=%q, got %q", "def5678901234", csm.Hash)
	}
}

func TestHandleMouseClick_OutOfRange(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Click on a row past the commit list.
	_, cmd := p.Update(panels.PanelMouseClickMsg{ContentRow: 10, ContentCol: 0})
	if cmd != nil {
		t.Error("expected nil command for out-of-range click")
	}
}

func TestHandleMouseClick_Negative(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Negative row (shouldn't happen normally, but guard).
	_, cmd := p.Update(panels.PanelMouseClickMsg{ContentRow: -1, ContentCol: 0})
	if cmd != nil {
		t.Error("expected nil command for negative row click")
	}
}

// ---------------------------------------------------------------------------
// Mouse wheel tests
// ---------------------------------------------------------------------------

func TestMouseWheel(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Wheel down.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if p.cursor != 1 {
		t.Errorf("expected cursor=1 after wheel down, got %d", p.cursor)
	}

	// Wheel up.
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if p.cursor != 0 {
		t.Errorf("expected cursor=0 after wheel up, got %d", p.cursor)
	}
}

// ---------------------------------------------------------------------------
// Mouse double-click tests
// ---------------------------------------------------------------------------

func TestHandleMouseDoubleClick(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemCommit): true}

	// Double-click on the second commit (row 1, offset 0).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 0})
	if p.cursor != 1 {
		t.Errorf("expected cursor=1 after double-click, got %d", p.cursor)
	}
	// Should select the commit (same as Enter).
	if cmd == nil {
		t.Fatal("expected a command from mouse double-click")
	}
	msg := cmd()
	csm, ok := msg.(panels.CommitSelectedMsg)
	if !ok {
		t.Fatalf("expected CommitSelectedMsg, got %T", msg)
	}
	if csm.Hash != "def5678901234" {
		t.Errorf("expected hash=%q, got %q", "def5678901234", csm.Hash)
	}
}

func TestHandleMouseDoubleClick_OutOfRange(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 10, ContentCol: 0})
	if cmd != nil {
		t.Error("expected nil command for out-of-range double-click")
	}
}

// ---------------------------------------------------------------------------
// PR commits mode tests
// ---------------------------------------------------------------------------

func TestHandlePRCommitsLoaded(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	prCommits := []panels.PRCommit{
		{SHA: "aaa111bbb222ccc333", Message: "PR commit one\nExtra body", Author: "Alice", Date: "2024-06-15T10:00:00Z"},
		{SHA: "ddd444eee555fff666", Message: "PR commit two", Author: "Bob", Date: "2024-06-15T11:00:00Z"},
	}

	p.Update(panels.PRCommitsLoadedMsg{Number: 42, Commits: prCommits})

	if !p.prCommitsMode {
		t.Error("expected prCommitsMode=true")
	}
	if p.prNumber != 42 {
		t.Errorf("expected prNumber=42, got %d", p.prNumber)
	}
	if p.prLabel != "PR #42" {
		t.Errorf("expected prLabel=%q, got %q", "PR #42", p.prLabel)
	}
	if len(p.commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(p.commits))
	}
	// Subject should be first line of message.
	if p.commits[0].Subject != "PR commit one" {
		t.Errorf("expected subject=%q, got %q", "PR commit one", p.commits[0].Subject)
	}
	// Short hash truncated to 7 chars.
	if p.commits[0].ShortHash != "aaa111b" {
		t.Errorf("expected shortHash=%q, got %q", "aaa111b", p.commits[0].ShortHash)
	}
	if p.Title() != "Commits: PR #42" {
		t.Errorf("expected title=%q, got %q", "Commits: PR #42", p.Title())
	}
	if !p.allLoaded {
		t.Error("expected allLoaded=true in PR-commits mode")
	}
	if p.loading {
		t.Error("expected loading=false in PR-commits mode")
	}
	if p.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", p.cursor)
	}
}

func TestHandlePRCommitsLoaded_ShortSHA(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// SHA shorter than 7 should be used as-is.
	prCommits := []panels.PRCommit{
		{SHA: "abc", Message: "Short SHA commit", Author: "Alice", Date: ""},
	}

	p.Update(panels.PRCommitsLoadedMsg{Number: 1, Commits: prCommits})

	if p.commits[0].ShortHash != "abc" {
		t.Errorf("expected shortHash=%q, got %q", "abc", p.commits[0].ShortHash)
	}
	// Empty date should result in zero time.
	if !p.commits[0].Date.IsZero() {
		t.Error("expected zero date for empty date string")
	}
}

func TestExitPRCommitsMode(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// Enter PR-commits mode.
	prCommits := []panels.PRCommit{
		{SHA: "aaa111bbb222ccc333", Message: "PR commit", Author: "Alice", Date: "2024-06-15T10:00:00Z"},
	}
	p.Update(panels.PRCommitsLoadedMsg{Number: 42, Commits: prCommits})

	if !p.prCommitsMode {
		t.Fatal("expected prCommitsMode=true before exit")
	}

	// Exit via PRDeselectedMsg.
	_, cmd := p.Update(panels.PRDeselectedMsg{})

	if p.prCommitsMode {
		t.Error("expected prCommitsMode=false after exit")
	}
	if p.prNumber != 0 {
		t.Errorf("expected prNumber=0, got %d", p.prNumber)
	}
	if p.prLabel != "" {
		t.Errorf("expected prLabel empty, got %q", p.prLabel)
	}
	if cmd == nil {
		t.Error("expected reload command after exiting PR-commits mode")
	}
}

func TestExitPRCommitsMode_NotActive(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// Exit when not in PR-commits mode should be a no-op.
	_, cmd := p.Update(panels.PRDeselectedMsg{})
	if cmd != nil {
		t.Error("expected nil command when not in PR-commits mode")
	}
}

func TestEscapeExitsPRCommitsMode(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Enter PR-commits mode.
	prCommits := []panels.PRCommit{
		{SHA: "aaa111bbb222ccc333", Message: "PR commit", Author: "Alice", Date: "2024-06-15T10:00:00Z"},
	}
	p.Update(panels.PRCommitsLoadedMsg{Number: 42, Commits: prCommits})

	// Escape should exit PR-commits mode first (progressive reset).
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.prCommitsMode {
		t.Error("expected prCommitsMode=false after Escape")
	}
	if cmd == nil {
		t.Error("expected reload command from Escape in PR-commits mode")
	}
}

// ---------------------------------------------------------------------------
// relativeDate tests
// ---------------------------------------------------------------------------

func TestRelativeDate(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"1 minute", 90 * time.Second, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"1 hour", 90 * time.Minute, "1 hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"1 day", 36 * time.Hour, "1 day ago"},
		{"5 days", 5 * 24 * time.Hour, "5 days ago"},
		{"1 month", 35 * 24 * time.Hour, "1 month ago"},
		{"3 months", 90 * 24 * time.Hour, "3 months ago"},
		{"1 year", 400 * 24 * time.Hour, "1 year ago"},
		{"2 years", 800 * 24 * time.Hour, "2 years ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := relativeDate(time.Now().Add(-tt.d))
			if result != tt.expected {
				t.Errorf("relativeDate(%v): expected %q, got %q", tt.d, tt.expected, result)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// truncateOrPad tests
// ---------------------------------------------------------------------------

func TestTruncateOrPad(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		checkLen bool
	}{
		{"exact fit", "hello", 5, true},
		{"needs padding", "hi", 10, true},
		{"needs truncation", "a very long string indeed", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateOrPad(tt.input, tt.width)
			if result == "" && tt.width > 0 {
				t.Error("expected non-empty result")
			}
		})
	}
}

func TestTruncateOrPad_PadsShortString(t *testing.T) {
	result := truncateOrPad("hi", 10)
	// "hi" is 2 chars, should be padded to 10.
	if len(result) != 10 {
		t.Errorf("expected length 10, got %d", len(result))
	}
	if result[:2] != "hi" {
		t.Errorf("expected result to start with 'hi', got %q", result[:2])
	}
}

func TestTruncateOrPad_ExactFit(t *testing.T) {
	result := truncateOrPad("hello", 5)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

// ---------------------------------------------------------------------------
// Title variations
// ---------------------------------------------------------------------------

func TestTitle_SelectedHash(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Select a commit.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	title := p.Title()
	if title != "Commits: abc1234" {
		t.Errorf("expected title=%q, got %q", "Commits: abc1234", title)
	}
}

func TestTitle_PRCommitsMode(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	prCommits := []panels.PRCommit{
		{SHA: "aaa111bbb222ccc333", Message: "msg", Author: "A", Date: "2024-06-15T10:00:00Z"},
	}
	p.Update(panels.PRCommitsLoadedMsg{Number: 99, Commits: prCommits})

	if p.Title() != "Commits: PR #99" {
		t.Errorf("expected title=%q, got %q", "Commits: PR #99", p.Title())
	}
}

// ---------------------------------------------------------------------------
// KeyBindings test
// ---------------------------------------------------------------------------

func TestKeyBindings(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	bindings := p.KeyBindings()
	if len(bindings) == 0 {
		t.Fatal("expected non-empty key bindings")
	}

	// Check that expected actions are present.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}
	for _, expected := range []string{"cursor_down", "cursor_up", "detail", "page_down", "page_up", "go_top", "go_bottom", "copy_hash", "search"} {
		if !actions[expected] {
			t.Errorf("expected action %q in key bindings", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Search confirm with Enter
// ---------------------------------------------------------------------------

func TestSearchConfirmEnter(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Enter search mode and type.
	p.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	for _, ch := range "bug" {
		p.Update(tea.KeyPressMsg{Code: ch})
	}

	// Confirm search with Enter (keeps filter active).
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.searchMode {
		t.Error("expected searchMode=false after Enter")
	}
	if len(p.filteredIdx) != 1 {
		t.Errorf("expected filteredIdx to be preserved, got %d", len(p.filteredIdx))
	}
}

// ---------------------------------------------------------------------------
// Search backspace
// ---------------------------------------------------------------------------

func TestSearchBackspace(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	for _, ch := range "bug" {
		p.Update(tea.KeyPressMsg{Code: ch})
	}
	if p.searchQuery != "bug" {
		t.Fatalf("expected searchQuery=%q, got %q", "bug", p.searchQuery)
	}

	// Backspace removes last char.
	p.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if p.searchQuery != "bu" {
		t.Errorf("expected searchQuery=%q after backspace, got %q", "bu", p.searchQuery)
	}
}

// ---------------------------------------------------------------------------
// Right-click tests
// ---------------------------------------------------------------------------

func TestRightClickShowsActionPicker(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 1, ContentCol: 5})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for right-click on valid row")
	}

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	if !ok {
		t.Fatalf("expected ShowModalMsg, got %T", msg)
	}
	if modal.Kind != notify.ModalActionPicker {
		t.Errorf("expected ModalActionPicker, got %d", modal.Kind)
	}
}

func TestRightClickOutOfBounds(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 10, ContentCol: 5})
	if cmd != nil {
		t.Error("expected nil cmd for right-click on out-of-range row")
	}
}

// ---------------------------------------------------------------------------
// RepoChangedMsg
// ---------------------------------------------------------------------------

func TestRepoChangedMsg_ResetsStateAndReloads(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits()}
	p := newTestPanel(mock)

	// Set state that should be cleared on directory change.
	p.ref = "feature"
	p.refLabel = "feature"
	p.filter = filterBranch
	p.filterPath = "/some/path"
	p.filterLabel = "some-branch"

	// Send RepoChangedMsg — git.NewClient succeeds for any valid path,
	// so the panel creates a new client and triggers a reload.
	tmpDir := t.TempDir()
	result, cmd := p.Update(panels.RepoChangedMsg{Path: tmpDir})
	p = result.(*Panel)

	// All filter/ref state should be reset.
	if p.ref != "" {
		t.Errorf("ref should be empty, got %q", p.ref)
	}
	if p.refLabel != "" {
		t.Errorf("refLabel should be empty, got %q", p.refLabel)
	}
	if p.filter != filterNone {
		t.Errorf("filter should be filterNone, got %d", p.filter)
	}
	if p.filterPath != "" {
		t.Errorf("filterPath should be empty, got %q", p.filterPath)
	}
	if p.filterLabel != "" {
		t.Errorf("filterLabel should be empty, got %q", p.filterLabel)
	}

	// A new git client should have been created and a reload command issued.
	if p.gitClient == nil {
		t.Error("gitClient should not be nil after RepoChangedMsg with valid path")
	}
	if cmd == nil {
		t.Error("a reload command should be returned")
	}
}
