package gitinfo

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// reflogRelativeDate — 0% coverage, pure function
// ---------------------------------------------------------------------------

func TestReflogRelativeDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just_now", 10 * time.Second, "just now"},
		{"one_min", 1 * time.Minute, "1 min ago"},
		{"five_mins", 5 * time.Minute, "5 mins ago"},
		{"one_hour", 1 * time.Hour, "1 hour ago"},
		{"three_hours", 3 * time.Hour, "3 hours ago"},
		{"one_day", 24 * time.Hour, "1 day ago"},
		{"seven_days", 7 * 24 * time.Hour, "7 days ago"},
		{"one_month", 31 * 24 * time.Hour, "1 month ago"},
		{"three_months", 90 * 24 * time.Hour, "3 months ago"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := reflogRelativeDate(time.Now().Add(-tt.ago))
			assert.Equal(t, tt.want, result)
		})
	}
}

// ---------------------------------------------------------------------------
// rightClickLabel — 0% coverage, pure function on listItem
// ---------------------------------------------------------------------------

func TestRightClickLabel(t *testing.T) {
	t.Parallel()

	p := &Panel{}

	tests := []struct {
		name string
		item listItem
		want string
	}{
		{
			"local_branch",
			listItem{kind: kindLocalBranch, branch: git.Branch{Name: "main"}},
			"main",
		},
		{
			"remote_branch",
			listItem{kind: kindRemoteBranch, branch: git.Branch{Name: "origin/feature"}},
			"origin/feature",
		},
		{
			"worktree",
			listItem{kind: kindWorktree, worktree: git.Worktree{Branch: "dev"}},
			"dev",
		},
		{
			"remote",
			listItem{kind: kindRemote, remote: git.Remote{Name: "upstream"}},
			"upstream",
		},
		{
			"stash_entry",
			listItem{kind: kindStashEntry, stash: git.StashEntry{Index: 2}},
			"stash@{2}",
		},
		{
			"issue",
			listItem{kind: kindIssue, issue: ghIssueItem{Number: 42, Title: "Fix bug"}},
			"#42 Fix bug",
		},
		{
			"pr",
			listItem{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Add feature"}},
			"#10 Add feature",
		},
		{
			"action_run",
			listItem{kind: kindActionRun, actionRun: ghActionItem{RunNumber: 5, WorkflowName: "CI"}},
			"#5 CI",
		},
		{
			"tag",
			listItem{kind: kindTag, tag: git.Tag{Name: "v1.0.0"}},
			"v1.0.0",
		},
		{
			"remote_tag",
			listItem{kind: kindRemoteTag, tag: git.Tag{Name: "v2.0.0"}},
			"v2.0.0",
		},
		{
			"default_text",
			listItem{kind: kindRemoteSub, text: "Section Header"},
			"Section Header",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := p.rightClickLabel(tt.item)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// itemTypeForKind — 81.8% coverage, cover remaining cases
// ---------------------------------------------------------------------------

func TestItemTypeForKind_AllCases(t *testing.T) {
	t.Parallel()

	p := &Panel{}

	tests := []struct {
		kind itemKind
		want actions.ItemType
	}{
		{kindLocalBranch, actions.ItemLocalBranch},
		{kindRemoteBranch, actions.ItemRemoteBranch},
		{kindWorktree, actions.ItemWorktree},
		{kindRemote, actions.ItemRemote},
		{kindStashEntry, actions.ItemStashEntry},
		{kindIssue, actions.ItemIssue},
		{kindPR, actions.ItemPR},
		{kindActionRun, actions.ItemActionRun},
		{kindTag, actions.ItemTag},
		{kindRemoteTag, actions.ItemTag},
		{kindRemoteSub, ""},   // default
		{kindReflogEntry, ""}, // default
		{kindRemoteSub, ""},   // default
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("kind_%d", tt.kind), func(t *testing.T) {
			t.Parallel()
			got := p.itemTypeForKind(tt.kind)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// renderTag — 0% coverage
// ---------------------------------------------------------------------------

func TestRenderTag(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())

	tests := []struct {
		name     string
		item     listItem
		width    int
		isCursor bool
		contains string
	}{
		{
			"local_tag",
			listItem{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc1234"}},
			80, false, "v1.0.0",
		},
		{
			"annotated_tag",
			listItem{kind: kindTag, tag: git.Tag{Name: "v2.0.0", Hash: "def5678", IsAnnotated: true}},
			80, false, "[annotated]",
		},
		{
			"remote_tag",
			listItem{kind: kindRemoteTag, tag: git.Tag{Name: "v3.0.0", Hash: "111aaaa"}},
			80, false, "v3.0.0",
		},
		{
			"cursor_tag",
			listItem{kind: kindTag, tag: git.Tag{Name: "v4.0.0", Hash: "222bbbb"}},
			80, true, "v4.0.0",
		},
		{
			"narrow_tag",
			listItem{kind: kindTag, tag: git.Tag{Name: "very-long-tag-name-that-should-truncate", Hash: "333cccc"}},
			30, false, "…",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := p.renderTag(tt.item, tt.width, tt.isCursor)
			assert.Contains(t, got, tt.contains)
		})
	}
}

// ---------------------------------------------------------------------------
// renderReflogEntry — 0% coverage
// ---------------------------------------------------------------------------

func TestRenderReflogEntry(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())

	tests := []struct {
		name     string
		item     listItem
		width    int
		isCursor bool
		contains string
	}{
		{
			"basic_reflog",
			listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{
				Hash: "abcdef1234567890", Action: "commit", Message: "initial",
				Date: time.Now().Add(-5 * time.Minute),
			}},
			80, false, "abcdef1",
		},
		{
			"short_hash",
			listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{
				Hash: "abc", Action: "reset", Message: "HEAD@{1}",
				Date: time.Now().Add(-2 * time.Hour),
			}},
			80, false, "abc",
		},
		{
			"narrow_reflog",
			listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{
				Hash: "1234567890abcdef", Action: "commit", Message: "a very long commit message here",
				Date: time.Now().Add(-24 * time.Hour),
			}},
			20, false, "...",
		},
		{
			"very_narrow_reflog",
			listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{
				Hash: "abcdef1234567890", Action: "commit", Message: "msg",
				Date: time.Now(),
			}},
			3, false, "",
		},
		{
			"zero_width",
			listItem{kind: kindReflogEntry, reflog: git.ReflogEntry{
				Hash: "abcdef1234567890", Action: "commit", Message: "msg",
				Date: time.Now(),
			}},
			0, false, "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := p.renderReflogEntry(tt.item, tt.width, tt.isCursor)
			if tt.contains != "" {
				assert.Contains(t, got, tt.contains)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// guessBranchRemoteURL — 0% coverage
// ---------------------------------------------------------------------------

func TestGuessBranchRemoteURL_WithRemotes(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.gitData.lastRemotes = []git.Remote{
		{Name: "origin", FetchURL: "https://github.com/owner/repo.git"},
		{Name: "upstream", FetchURL: "https://github.com/other/repo.git"},
	}

	url := p.guessBranchRemoteURL(git.Branch{Name: "main"})
	assert.Equal(t, "https://github.com/owner/repo.git", url)
}

func TestGuessBranchRemoteURL_NoRemotes(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.gitData.lastRemotes = nil

	url := p.guessBranchRemoteURL(git.Branch{Name: "main"})
	assert.Equal(t, "", url)
}

// ---------------------------------------------------------------------------
// copyHashToClipboard — 0% coverage
// ---------------------------------------------------------------------------

func TestCopyHashToClipboard_OutOfBounds(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = nil
	p.tabCursor[tabBranches] = 0

	result, cmd := p.copyHashToClipboard()
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
}

func TestCopyHashToClipboard_EmptyHash(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "main"}, hash: ""},
	}
	p.tabCursor[tabBranches] = 0

	result, cmd := p.copyHashToClipboard()
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
}

func TestCopyHashToClipboard_WithHash(t *testing.T) {
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "main"}, hash: "abc1234"},
	}
	p.tabCursor[tabBranches] = 0

	result, cmd := p.copyHashToClipboard()
	assert.NotNil(t, result)
	// cmd will either be a toast success or failure (depending on clipboard availability).
	// Either way, a cmd should be returned because the hash is non-empty.
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// doTagPush — 0% coverage
// ---------------------------------------------------------------------------

func TestDoTagPush_OutOfBounds(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = nil
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagPush()
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
}

func TestDoTagPush_WrongKind(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindRemoteSub, text: "Tags"},
	}
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagPush()
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
}

func TestDoTagPush_ValidTag(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0"}},
	}
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagPush()
	assert.NotNil(t, result)
	assert.NotNil(t, cmd) // Should show confirmation dialog.
	pp := result.(*Panel)
	assert.Equal(t, opTagPush, pp.pending)
	assert.Equal(t, "v1.0.0", pp.pendingName)
}

// ---------------------------------------------------------------------------
// doTagDelete — 0% coverage
// ---------------------------------------------------------------------------

func TestDoTagDelete_OutOfBounds(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = nil
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagDelete()
	assert.NotNil(t, result)
	assert.Nil(t, cmd)
}

func TestDoTagDelete_RemoteTag(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindRemoteTag, tag: git.Tag{Name: "v2.0.0"}},
	}
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagDelete()
	assert.NotNil(t, result)
	assert.NotNil(t, cmd) // Should show toast error about remote tag.
}

func TestDoTagDelete_ValidTag(t *testing.T) {
	t.Parallel()

	p := newTestPanel(t, defaultMock())
	p.activeTab = tabTags
	p.tabItems[tabTags] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0"}},
	}
	p.tabCursor[tabTags] = 0

	result, cmd := p.doTagDelete()
	assert.NotNil(t, result)
	assert.NotNil(t, cmd) // Should show confirmation dialog.
	pp := result.(*Panel)
	assert.Equal(t, opTagDelete, pp.pending)
	assert.Equal(t, "v1.0.0", pp.pendingName)
}

// ---------------------------------------------------------------------------
// copyAndToast — 0% coverage
// ---------------------------------------------------------------------------

func TestCopyAndToast_Empty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	result, cmd := p.copyAndToast("")
	assert.NotNil(t, result)
	assert.Nil(t, cmd, "empty text should be no-op")
}

func TestCopyAndToast_Short(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	_, cmd := p.copyAndToast("abc123")
	// On CI, clipboard might fail; either way we get a toast.
	assert.NotNil(t, cmd)
}

func TestCopyAndToast_LongTruncates(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	longText := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLM"
	_, cmd := p.copyAndToast(longText)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// openURLAndToast — 25% coverage
// ---------------------------------------------------------------------------

func TestOpenURLAndToast(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	_, cmd := p.openURLAndToast("https://example.com", "Example")
	assert.NotNil(t, cmd)
	// Do NOT execute cmd() — it would open a real browser to example.com.
	// The cmd is a closure over panels.OpenInBrowser which is tested
	// separately via ValidateBrowserURL in open_test.go.
}

// ---------------------------------------------------------------------------
// doReflogCheckout — 0% coverage
// ---------------------------------------------------------------------------

func TestDoReflogCheckout_Empty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabReflog
	p.tabItems[tabReflog] = nil
	p.tabCursor[tabReflog] = 0
	_, cmd := p.doReflogCheckout()
	assert.Nil(t, cmd, "empty reflog should be no-op")
}

func TestDoReflogCheckout_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabReflog
	p.tabItems[tabReflog] = []listItem{
		{kind: kindReflogEntry, reflog: git.ReflogEntry{Hash: "abc123", Message: "test"}},
	}
	p.tabCursor[tabReflog] = 5 // out of bounds
	_, cmd := p.doReflogCheckout()
	assert.Nil(t, cmd, "out-of-bounds cursor should be no-op")
}

func TestDoReflogCheckout_Valid(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabReflog
	p.tabItems[tabReflog] = []listItem{
		{kind: kindReflogEntry, reflog: git.ReflogEntry{Hash: "abc123def456789", Message: "checkout: test"}},
	}
	p.tabCursor[tabReflog] = 0
	_, cmd := p.doReflogCheckout()
	assert.NotNil(t, cmd)
	assert.Equal(t, opBranchCheckout, p.pending)
	assert.Equal(t, "abc123def456789", p.pendingName)
}

// ---------------------------------------------------------------------------
// handleOpResult — 36.2% coverage (many switch cases uncovered)
// ---------------------------------------------------------------------------

func TestHandleOpResult_StashOps(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	msg := opResultMsg{op: "checkout", err: fmt.Errorf("failed")}
	_, cmd := p.handleOpResult(msg)
	assert.NotNil(t, cmd)
}

func TestHandleOpResult_ExtendedOps(t *testing.T) {
	t.Parallel()

	ops := []struct {
		op   string
		name string
	}{
		{"stash_applied", "stash@{0}"},
		{"stash_popped", "stash@{0}"},
		{"stash_dropped", "stash@{0}"},
		{"tag_created", "v1.0.0"},
		{"tag_deleted", "v1.0.0"},
		{"tag_pushed", "v1.0.0"},
		{"tag_checkout", "v1.0.0"},
	}

	for _, tt := range ops {
		tt := tt
		t.Run(tt.op, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			msg := opResultMsg{op: tt.op, name: tt.name}
			_, cmd := p.handleOpResult(msg)
			assert.NotNil(t, cmd, "op %q should produce a command", tt.op)
		})
	}
}

// ---------------------------------------------------------------------------
// executeAction — partial coverage, test more item kinds
// ---------------------------------------------------------------------------

func TestExecuteAction_StashEntry(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:  kindStashEntry,
		stash: git.StashEntry{Index: 2, Message: "WIP"},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
	assert.Equal(t, opStashAction, p.pending)
	assert.Equal(t, "2", p.pendingName)
}

func TestExecuteAction_RemoteWithURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:   kindRemote,
		remote: git.Remote{Name: "origin", FetchURL: "git@github.com:user/repo.git"},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
}

func TestExecuteAction_RemoteNoURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:   kindRemote,
		remote: git.Remote{Name: "empty", FetchURL: ""},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
}

func TestExecuteAction_Issue(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:  kindIssue,
		issue: ghIssueItem{HTMLURL: "https://github.com/user/repo/issues/1", Number: 1},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
}

func TestExecuteAction_IssueNoURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:  kindIssue,
		issue: ghIssueItem{Number: 1},
	}
	_, cmd := p.executeAction(item)
	assert.Nil(t, cmd)
}

func TestExecuteAction_PR(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind: kindPR,
		pr:   ghPRItem{HTMLURL: "https://github.com/user/repo/pull/1", Number: 1},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
}

func TestExecuteAction_PRNoURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind: kindPR,
		pr:   ghPRItem{Number: 1},
	}
	_, cmd := p.executeAction(item)
	assert.Nil(t, cmd)
}

func TestExecuteAction_ActionRun(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:      kindActionRun,
		actionRun: ghActionItem{HTMLURL: "https://github.com/user/repo/actions/runs/1", RunNumber: 42},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
}

func TestExecuteAction_ActionRunNoURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind:      kindActionRun,
		actionRun: ghActionItem{RunNumber: 42},
	}
	_, cmd := p.executeAction(item)
	assert.Nil(t, cmd)
}

func TestExecuteAction_Tag(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind: kindTag,
		tag:  git.Tag{Name: "v2.0.0"},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
	assert.Equal(t, opTagCheckout, p.pending)
	assert.Equal(t, "v2.0.0", p.pendingName)
}

func TestExecuteAction_RemoteTag(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{
		kind: kindRemoteTag,
		tag:  git.Tag{Name: "v2.0.0"},
	}
	_, cmd := p.executeAction(item)
	assert.NotNil(t, cmd)
	assert.Equal(t, opTagCheckout, p.pending)
}

func TestExecuteAction_UnhandledKind(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	item := listItem{kind: kindRemoteSub} // not in executeAction switch
	_, cmd := p.executeAction(item)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// executeRightClickAction — 34.6% coverage (many nested switch cases)
// ---------------------------------------------------------------------------

func TestExecuteRightClickAction_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = nil
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckout)
	assert.Nil(t, cmd)
}

func TestExecuteRightClickAction_LocalBranch_Checkout(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "feature"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckout)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_LocalBranch_CopyName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "feature"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyName)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_RemoteBranch_CopyName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindRemoteBranch, branch: git.Branch{Name: "origin/main"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyName)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Worktree_ChangeDirectory(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindWorktree, worktree: git.Worktree{Path: "/tmp/wt", Branch: "feature"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionChangeDirectory)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Worktree_CopyPath(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindWorktree, worktree: git.Worktree{Path: "/tmp/wt", Branch: "feature"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyPath)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Remote_CopyURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindRemote, remote: git.Remote{Name: "origin", FetchURL: "https://github.com/user/repo.git"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyURL)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Remote_CopyURL_Empty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindRemote, remote: git.Remote{Name: "empty"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyURL)
	assert.Nil(t, cmd)
}

func TestExecuteRightClickAction_Stash_Apply(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "WIP"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionApply)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Stash_Pop(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "WIP"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionPop)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Stash_Drop(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindStashEntry, stash: git.StashEntry{Index: 0, Message: "WIP"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionDrop)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Stash_PromptAction(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindStashEntry, stash: git.StashEntry{Index: 1, Message: "WIP"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionPromptAction)
	assert.NotNil(t, cmd)
	assert.Equal(t, opStashAction, p.pending)
}

func TestExecuteRightClickAction_Issue_OpenInBrowser(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{HTMLURL: "https://github.com/user/repo/issues/1", Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionOpenInBrowser)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Issue_CopyURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{HTMLURL: "https://github.com/user/repo/issues/1", Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyURL)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Issue_CopyNumber(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindIssue, issue: ghIssueItem{Number: 42}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyNumber)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_PR_OpenInBrowser(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{HTMLURL: "https://github.com/user/repo/pull/1", Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionOpenInBrowser)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_PR_CopyURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{HTMLURL: "https://github.com/user/repo/pull/1", Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyURL)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_PR_CopyNumber(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 99}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyNumber)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_PR_CheckoutBranch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{HeadBranch: "feature-branch", Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckoutBranch)
	assert.NotNil(t, cmd)
	assert.Equal(t, opBranchCheckout, p.pending)
	assert.Equal(t, "feature-branch", p.pendingName)
}

func TestExecuteRightClickAction_PR_CheckoutBranch_Empty(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckoutBranch)
	assert.Nil(t, cmd)
}

func TestExecuteRightClickAction_ActionRun_OpenInBrowser(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{HTMLURL: "https://github.com/user/repo/actions/runs/1", RunNumber: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionOpenInBrowser)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_ActionRun_CopyURL(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindActionRun, actionRun: ghActionItem{HTMLURL: "https://github.com/user/repo/actions/runs/1", RunNumber: 1}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyURL)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Tag_Checkout(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc123"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckout)
	assert.NotNil(t, cmd)
	assert.Equal(t, opTagCheckout, p.pending)
}

func TestExecuteRightClickAction_Tag_CopyName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc123"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyName)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Tag_CopyHash(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc123"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCopyHash)
	assert.NotNil(t, cmd)
}

func TestExecuteRightClickAction_Tag_Delete(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc123"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionDelete)
	assert.NotNil(t, cmd)
	assert.Equal(t, opTagDelete, p.pending)
}

func TestExecuteRightClickAction_Tag_Push(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindTag, tag: git.Tag{Name: "v1.0.0", Hash: "abc123"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionPush)
	assert.NotNil(t, cmd)
	assert.Equal(t, opTagPush, p.pending)
}

func TestExecuteRightClickAction_UnhandledKind(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindRemoteSub},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionCheckout)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleMouseRightClick — 0% coverage
// ---------------------------------------------------------------------------

func TestHandleMouseRightClick_InTabBar(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// ContentRow 0 is in the tab bar — should be no-op.
	msg := panels.PanelMouseRightClickMsg{ContentRow: 0, ContentCol: 5}
	_, cmd := p.handleMouseRightClick(msg)
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_OutOfBounds(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// A row beyond the items.
	msg := panels.PanelMouseRightClickMsg{ContentRow: 100, ContentCol: 5}
	_, cmd := p.handleMouseRightClick(msg)
	assert.Nil(t, cmd)
}

func TestHandleMouseRightClick_ValidItem(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// Populate some items.
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "main"}},
		{kind: kindLocalBranch, branch: git.Branch{Name: "feature"}},
	}
	p.tabOffset[p.activeTab] = 0
	// Click on the first data row (row 1, since row 0 is tab bar).
	msg := panels.PanelMouseRightClickMsg{ContentRow: 1, ContentCol: 5}
	_, cmd := p.handleMouseRightClick(msg)
	// Should produce a command (context menu or direct action) and update cursor.
	assert.Equal(t, 0, p.tabCursor[p.activeTab])
	// cmd may or may not be nil depending on actionsCfg; we just verify no panic.
	_ = cmd
}

// ---------------------------------------------------------------------------
// requestWorktreeSwitch — 66.7% (missing "new_terminal" mode branch)
// ---------------------------------------------------------------------------

func TestRequestWorktreeSwitch_NewTerminalMode(t *testing.T) {
	t.Parallel()
	mock := defaultMock()
	p := newTestPanel(t, mock)
	p.cfg.WorktreeOpenMode = "new_terminal"
	p.activeTab = tabWorktrees
	p.tabItems[tabWorktrees] = []listItem{
		{kind: kindWorktree, worktree: git.Worktree{Path: "/tmp/wt", Branch: "feature"}},
	}
	p.tabCursor[tabWorktrees] = 0
	_, cmd := p.requestWorktreeSwitch()
	assert.NotNil(t, cmd)
}

func TestRequestWorktreeSwitch_NoWorktreeSelected(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.activeTab = tabWorktrees
	p.tabItems[tabWorktrees] = nil
	p.tabCursor[tabWorktrees] = 0
	_, cmd := p.requestWorktreeSwitch()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// doFetch — 53.8% coverage
// ---------------------------------------------------------------------------

func TestDoFetch_NoRemoteSelected(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// No remotes tab selected — fetches all.
	_, cmd := p.doFetch()
	assert.NotNil(t, cmd)
}

func TestDoFetch_SingleRemote(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.remoteCount = 1
	p.activeTab = tabRemotes
	p.tabItems[tabRemotes] = []listItem{
		{kind: kindRemote, remote: git.Remote{Name: "origin"}},
	}
	_, cmd := p.doFetch()
	assert.NotNil(t, cmd)
}

func TestDoFetch_MultipleRemotes(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.remoteCount = 2
	p.activeTab = tabRemotes
	p.tabItems[tabRemotes] = []listItem{
		{kind: kindRemote, remote: git.Remote{Name: "origin"}},
		{kind: kindRemote, remote: git.Remote{Name: "upstream"}},
	}
	p.tabCursor[tabRemotes] = 0
	_, cmd := p.doFetch()
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// ActionMergePR — registry tests
// ---------------------------------------------------------------------------

func TestActionMergePR_InRegistry(t *testing.T) {
	t.Parallel()
	allActs := actions.AllActions(actions.ItemPR)
	found := false
	for _, a := range allActs {
		if a == actions.ActionMergePR {
			found = true
			break
		}
	}
	assert.True(t, found, "ActionMergePR should be in ItemPR actions")
}

func TestActionMergePR_Label(t *testing.T) {
	t.Parallel()
	label := actions.ActionLabel(actions.ActionMergePR)
	assert.Equal(t, "merge PR", label)
}

func TestActionMergePR_IsValid(t *testing.T) {
	t.Parallel()
	assert.True(t, actions.IsValidAction(actions.ItemPR, actions.ActionMergePR))
}

// ---------------------------------------------------------------------------
// executeRightClickAction — ActionMergePR
// ---------------------------------------------------------------------------

func TestExecuteRightClickAction_PR_MergePR_Open(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Add auth", State: "open", HeadBranch: "feature-auth"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionMergePR)
	assert.NotNil(t, cmd, "should show merge strategy picker for open PR")
	assert.Equal(t, opPRMergeStrategy, p.pending)
	assert.Contains(t, p.pendingName, "42:")
}

func TestExecuteRightClickAction_PR_MergePR_Draft(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 7, Title: "WIP", State: "draft", HeadBranch: "wip"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionMergePR)
	assert.NotNil(t, cmd, "should return a warning toast command for draft PR")
	assert.Equal(t, opNone, p.pending, "should not set pending for draft PR")
}

func TestExecuteRightClickAction_PR_MergePR_Merged(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 10, Title: "Done", State: prStateMerged}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionMergePR)
	assert.NotNil(t, cmd, "should return a warning toast for already-merged PR")
	assert.Equal(t, opNone, p.pending)
}

func TestExecuteRightClickAction_PR_MergePR_Closed(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 5, Title: "Closed", State: "closed"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionMergePR)
	assert.NotNil(t, cmd)
	assert.Equal(t, opNone, p.pending, "should not set pending for closed PR")
}

func TestExecuteRightClickAction_PR_MergePR_NoGHClient(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	// ghClient is nil
	p.tabItems[p.activeTab] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Test", State: "open"}},
	}
	p.tabCursor[p.activeTab] = 0
	_, cmd := p.executeRightClickAction(actions.ActionMergePR)
	assert.Nil(t, cmd, "should return nil when ghClient is nil")
}

// ---------------------------------------------------------------------------
// doMergePR — guard rails
// ---------------------------------------------------------------------------

func TestDoMergePR_WrongTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabBranches
	p.tabItems[tabBranches] = []listItem{
		{kind: kindLocalBranch, branch: git.Branch{Name: "main"}},
	}
	p.tabCursor[tabBranches] = 0
	_, cmd := p.doMergePR()
	assert.Nil(t, cmd)
}

func TestDoMergePR_EmptyCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{} // empty
	_, cmd := p.doMergePR()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// mergeStrategyLabel
// ---------------------------------------------------------------------------

func TestMergeStrategyLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		strategy string
		want     string
	}{
		{"merge", "merge commit"},
		{"squash", "squash and merge"},
		{"rebase", "rebase and merge"},
		{"unknown", "merge commit"},
		{"", "merge commit"},
	}
	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mergeStrategyLabel(tt.strategy))
		})
	}
}

// ---------------------------------------------------------------------------
// handleModalResult — opPRMergeStrategy
// ---------------------------------------------------------------------------

func TestHandleModalResult_PRMergeStrategy_Accept(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.pending = opPRMergeStrategy
	p.pendingName = "42:feature-auth:Add authentication"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{
		Accept: true,
		Value:  "squash",
	})
	assert.NotNil(t, cmd, "should produce confirmation dialog")
	assert.Equal(t, opPRMergeConfirm, p.pending)
	assert.Contains(t, p.pendingName, "42:squash:feature-auth")
}

func TestHandleModalResult_PRMergeStrategy_AllStrategies(t *testing.T) {
	t.Parallel()
	for _, strategy := range []string{"merge", "squash", "rebase"} {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()
			p := newTestPanel(t, defaultMock())
			p.gh.client = &mockGHClientFull{}
			p.pending = opPRMergeStrategy
			p.pendingName = "42:feature-auth:Add authentication"

			_, cmd := p.handleModalResult(notify.ModalResultMsg{
				Accept: true,
				Value:  strategy,
			})
			assert.NotNil(t, cmd)
			assert.Equal(t, opPRMergeConfirm, p.pending)
			assert.Contains(t, p.pendingName, fmt.Sprintf("42:%s:feature-auth", strategy))
		})
	}
}

func TestHandleModalResult_PRMergeStrategy_Cancel(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRMergeStrategy
	p.pendingName = "42:feature-auth:Add auth"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancel should produce nil cmd")
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_PRMergeStrategy_BadPendingName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRMergeStrategy
	p.pendingName = "bad" // not enough parts

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "merge"})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handleModalResult — opPRMergeConfirm
// ---------------------------------------------------------------------------

func TestHandleModalResult_PRMergeConfirm_Accept(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()
	p.pending = opPRMergeConfirm
	p.pendingName = "42:squash:feature-auth"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	assert.NotNil(t, cmd, "should produce merge command")
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_PRMergeConfirm_Cancel(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRMergeConfirm
	p.pendingName = "42:squash:feature-auth"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_PRMergeConfirm_BadPendingName(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRMergeConfirm
	p.pendingName = "bad"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// handlePRMergeResult
// ---------------------------------------------------------------------------

func TestHandlePRMergeResult_Success(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{
		{Number: 42, Title: "Add auth", State: "open", HeadBranch: "feature-auth"},
	}
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: p.gh.allPRs[0]},
	}

	_, cmd := p.handlePRMergeResult(prMergeResultMsg{
		number:   42,
		strategy: "squash",
	})
	assert.NotNil(t, cmd)
	// Verify local state was updated.
	assert.Equal(t, prStateMerged, p.gh.allPRs[0].State)
	assert.Equal(t, prStateMerged, p.tabItems[tabPRs][0].pr.State)
}

func TestHandlePRMergeResult_Error(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRMergeResult(prMergeResultMsg{
		number:   42,
		strategy: "merge",
		err:      errors.New("merge conflict"),
	})
	assert.NotNil(t, cmd, "should produce error toast")
}

func TestHandlePRMergeResult_ShowsDeleteBranchPrompt(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{
		{Number: 42, Title: "Test", State: "open", HeadBranch: "feature-x"},
	}
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: p.gh.allPRs[0]},
	}

	_, cmd := p.handlePRMergeResult(prMergeResultMsg{
		number:     42,
		strategy:   "rebase",
		headBranch: "feature-x",
	})
	assert.NotNil(t, cmd)
	assert.Equal(t, prStateMerged, p.gh.allPRs[0].State)
	// After successful merge with headBranch, the panel should prompt for deletion.
	assert.Equal(t, opPRDeleteBranchAfterMerge, p.pending)
	assert.Equal(t, "feature-x", p.pendingName)
}

func TestHandlePRMergeResult_NoPromptWhenNoHeadBranch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.allPRs = []ghPRItem{
		{Number: 42, Title: "Test", State: "open"},
	}
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: p.gh.allPRs[0]},
	}

	_, cmd := p.handlePRMergeResult(prMergeResultMsg{
		number:   42,
		strategy: "squash",
	})
	assert.NotNil(t, cmd)
	assert.Equal(t, prStateMerged, p.gh.allPRs[0].State)
	// No headBranch means no deletion prompt — pending should NOT be set.
	assert.NotEqual(t, opPRDeleteBranchAfterMerge, p.pending)
}

// ---------------------------------------------------------------------------
// handleModalResult — opPRDeleteBranchAfterMerge
// ---------------------------------------------------------------------------

func TestHandleModalResult_PRDeleteBranchAfterMerge_Confirm(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()
	p.pending = opPRDeleteBranchAfterMerge
	p.pendingName = "feature-auth"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	assert.NotNil(t, cmd, "confirming branch deletion should produce a command")
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_PRDeleteBranchAfterMerge_Cancel(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.pending = opPRDeleteBranchAfterMerge
	p.pendingName = "feature-auth"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancelling branch deletion should be no-op")
	assert.Equal(t, opNone, p.pending)
}

func TestHandleModalResult_PRDeleteBranchAfterMerge_EmptyBranch(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.ctx = t.Context()
	p.pending = opPRDeleteBranchAfterMerge
	p.pendingName = ""

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true})
	assert.Nil(t, cmd, "empty branch name should produce nil cmd")
}

// ---------------------------------------------------------------------------
// Comment on issue/PR (issue #252) — distinct from inline review comments (#212)
// ---------------------------------------------------------------------------

// collectCmdMsgs runs a cmd and recursively expands tea.BatchMsg so tests can
// inspect every message a batched command would emit.
func collectCmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collectCmdMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestDoCommentOnItem_Issue(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabIssues
	p.tabCursor[tabIssues] = 0

	_, cmd := p.doCommentOnItem()
	assert.NotNil(t, cmd, "should open the composer")
	assert.Equal(t, opIssuePRComment, p.pending)
	assert.Equal(t, "issue:1", p.pendingName)
}

func TestDoCommentOnItem_PR(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	populateGH(p, nil, samplePRs(), nil)
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabCursor[tabPRs] = 0

	_, cmd := p.doCommentOnItem()
	assert.NotNil(t, cmd, "should open the composer")
	assert.Equal(t, opIssuePRComment, p.pending)
	assert.Equal(t, "PR:10", p.pendingName)
}

func TestDoCommentOnItem_NoClient(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)
	p.gh.client = nil
	p.activeTab = tabIssues
	p.tabCursor[tabIssues] = 0

	_, cmd := p.doCommentOnItem()
	assert.Nil(t, cmd, "no client should be a no-op")
	assert.Equal(t, opNone, p.pending)
}

func TestDoCommentOnItem_EmptyCursor(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabIssues
	p.tabItems[tabIssues] = []listItem{}

	_, cmd := p.doCommentOnItem()
	assert.Nil(t, cmd, "empty list should be a no-op")
	assert.Equal(t, opNone, p.pending)
}

// TestHandleModalResult_IssuePRComment_EmptyBody verifies that submitting a
// blank comment is rejected client-side and never calls the API.
func TestHandleModalResult_IssuePRComment_EmptyBody(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{}
	p := newTestPanel(t, defaultMock())
	p.gh.client = mock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()
	p.pending = opIssuePRComment
	p.pendingName = "issue:42"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "   \n  "})
	assert.NotNil(t, cmd, "empty body should still surface a warning toast")
	assert.Equal(t, opNone, p.pending, "pending state must be cleared")

	// Draining the toast cmd must not hit the API.
	for _, msg := range collectCmdMsgs(cmd) {
		if toast, ok := msg.(notify.ShowToastMsg); ok {
			assert.Equal(t, notify.Warn, toast.Level)
		}
	}
	assert.Equal(t, 0, mock.commentCalls, "empty body must not call CommentOnIssue")
}

// TestHandleModalResult_IssuePRComment_Cancel verifies Escape cancels without
// calling the API.
func TestHandleModalResult_IssuePRComment_Cancel(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{}
	p := newTestPanel(t, defaultMock())
	p.gh.client = mock
	p.pending = opIssuePRComment
	p.pendingName = "issue:42"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd, "cancel should be a no-op")
	assert.Equal(t, opNone, p.pending)
	assert.Equal(t, 0, mock.commentCalls)
}

func TestHandleModalResult_IssuePRComment_BadPendingName(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{}
	p := newTestPanel(t, defaultMock())
	p.gh.client = mock
	p.pending = opIssuePRComment
	p.pendingName = "bad" // missing ":number"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "hi"})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, mock.commentCalls)
}

// TestHandleModalResult_IssuePRComment_PostsAndRefreshes exercises the full
// comment-then-refresh flow: submit a body, confirm CommentOnIssue is called
// with the right args, then confirm the conversation preview is refreshed.
func TestHandleModalResult_IssuePRComment_PostsAndRefreshes(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{}
	p := newTestPanel(t, defaultMock())
	populateGH(p, sampleIssues(), nil, nil)
	p.gh.client = mock
	p.ctx = t.Context()
	p.activeTab = tabIssues
	p.tabCursor[tabIssues] = 0
	p.pending = opIssuePRComment
	p.pendingName = "issue:1"

	_, cmd := p.handleModalResult(notify.ModalResultMsg{Accept: true, Value: "  LGTM, shipping it  "})
	assert.NotNil(t, cmd, "accepting a non-empty body should post the comment")
	assert.Equal(t, opNone, p.pending)

	// Run the post command; it should call the API with the trimmed body.
	postMsg := cmd()
	result, ok := postMsg.(commentResultMsg)
	assert.True(t, ok, "post cmd should yield a commentResultMsg")
	assert.NoError(t, result.err)
	assert.Equal(t, 1, mock.commentCalls)
	assert.Equal(t, 1, mock.commentNumber)
	assert.Equal(t, "LGTM, shipping it", mock.commentBody, "body should be trimmed")

	// Handling the result should show a success toast and refresh the preview.
	_, resultCmd := p.handleCommentResult(result)
	assert.NotNil(t, resultCmd)
	var sawSuccess, sawRefresh bool
	for _, msg := range collectCmdMsgs(resultCmd) {
		switch m := msg.(type) {
		case notify.ShowToastMsg:
			if m.Level == notify.Success {
				sawSuccess = true
			}
		case panels.IssueSelectedMsg:
			sawRefresh = true
		}
	}
	assert.True(t, sawSuccess, "should show a success toast")
	assert.True(t, sawRefresh, "should refresh the conversation preview")
}

func TestHandleCommentResult_Error(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handleCommentResult(commentResultMsg{
		number: 42,
		kind:   commentKindIssue,
		err:    errors.New("network down"),
	})
	assert.NotNil(t, cmd, "should produce an error toast")
	var sawError bool
	for _, msg := range collectCmdMsgs(cmd) {
		if toast, ok := msg.(notify.ShowToastMsg); ok && toast.Level == notify.Error {
			sawError = true
		}
	}
	assert.True(t, sawError, "failed comment should surface an error toast")
}

func TestCommentCmd_PostsBody(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{}
	p := newTestPanel(t, defaultMock())
	p.gh.client = mock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()

	cmd := p.commentCmd(7, "hello world", commentKindPR)
	msg := cmd()
	result, ok := msg.(commentResultMsg)
	assert.True(t, ok)
	assert.NoError(t, result.err)
	assert.Equal(t, 7, result.number)
	assert.Equal(t, commentKindPR, result.kind)
	assert.Equal(t, 1, mock.commentCalls)
	assert.Equal(t, "hello world", mock.commentBody)
}

func TestCommentCmd_PropagatesError(t *testing.T) {
	t.Parallel()
	mock := &mockGHClientFull{commentErr: errors.New("403 forbidden")}
	p := newTestPanel(t, defaultMock())
	p.gh.client = mock
	p.gh.owner = "owner"
	p.gh.repo = "repo"
	p.ctx = t.Context()

	cmd := p.commentCmd(7, "hello", commentKindIssue)
	msg := cmd()
	result, ok := msg.(commentResultMsg)
	assert.True(t, ok)
	assert.Error(t, result.err)
}

// ---------------------------------------------------------------------------
// handlePRBranchDeleteResult
// ---------------------------------------------------------------------------

func TestHandlePRBranchDeleteResult_Success(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRBranchDeleteResult(prBranchDeleteResultMsg{
		branch: "feature-x",
	})
	assert.NotNil(t, cmd, "successful deletion should produce toast + data reload")
}

func TestHandlePRBranchDeleteResult_RemoteError(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRBranchDeleteResult(prBranchDeleteResultMsg{
		branch:    "feature-x",
		remoteErr: errors.New("forbidden"),
	})
	assert.NotNil(t, cmd, "remote-only error should produce warn toast + data reload")
}

func TestHandlePRBranchDeleteResult_LocalError(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRBranchDeleteResult(prBranchDeleteResultMsg{
		branch:   "feature-x",
		localErr: errors.New("branch not found"),
	})
	assert.NotNil(t, cmd, "local-only error should still report remote success")
}

func TestHandlePRBranchDeleteResult_BothError(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())

	_, cmd := p.handlePRBranchDeleteResult(prBranchDeleteResultMsg{
		branch:    "feature-x",
		remoteErr: errors.New("forbidden"),
		localErr:  errors.New("not found"),
	})
	assert.NotNil(t, cmd, "both errors should produce error toast")
}

// ---------------------------------------------------------------------------
// Key binding — 'm' triggers merge on PRs tab
// ---------------------------------------------------------------------------

func TestHandleKey_M_PRsTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.gh.client = &mockGHClientFull{}
	p.activeTab = tabPRs
	p.tabItems[tabPRs] = []listItem{
		{kind: kindPR, pr: ghPRItem{Number: 42, Title: "Test", State: "open", HeadBranch: "test-branch"}},
	}
	p.tabCursor[tabPRs] = 0

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: 'm'})
	assert.NotNil(t, cmd, "pressing 'm' on PRs tab should trigger merge flow")
	assert.Equal(t, opPRMergeStrategy, p.pending)
}

func TestHandleKey_M_NotPRsTab(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabBranches

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: 'm'})
	assert.Nil(t, cmd, "pressing 'm' outside PRs tab should be no-op")
}

func TestHandleKey_M_NoGHClient(t *testing.T) {
	t.Parallel()
	p := newTestPanel(t, defaultMock())
	p.Focused = true
	p.activeTab = tabPRs
	// ghClient is nil

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: 'm'})
	assert.Nil(t, cmd, "pressing 'm' without ghClient should be no-op")
}

// ---------------------------------------------------------------------------
// PRMergeRequestedMsg / PRMergeFailedMsg message types exist
// ---------------------------------------------------------------------------

func TestPRMessageTypes(t *testing.T) {
	t.Parallel()
	// Compile-time check that message types exist and have correct fields.
	_ = panels.PRMergeRequestedMsg{Number: 1, Title: "test", HeadBranch: "main"}
	_ = panels.PRMergeFailedMsg{Number: 1, Err: errors.New("fail")}
	_ = panels.PRMergedMsg{Number: 1, Strategy: "squash"}
}
