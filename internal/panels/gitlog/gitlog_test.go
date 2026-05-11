package gitlog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitClient implements git.StatusReader for testing.
type mockGitClient struct {
	logFn func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
}

func (m *mockGitClient) Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
	if m.logFn != nil {
		return m.logFn(ctx, opts)
	}
	return nil, nil
}

func (m *mockGitClient) Status(ctx context.Context) ([]git.FileStatus, error) {
	return nil, nil
}

func (m *mockGitClient) Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
	return nil, nil
}

func (m *mockGitClient) Blame(ctx context.Context, path string) ([]git.BlameLine, error) {
	return nil, nil
}

func (m *mockGitClient) RepoRoot(ctx context.Context) (string, error) {
	return "/repo", nil
}

func (m *mockGitClient) IsRepo(ctx context.Context) (bool, error) {
	return true, nil
}

func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// makeCommits generates n sequential test commits.
func makeCommits(n int) []git.Commit {
	commits := make([]git.Commit, n)
	base := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		hash := fmt.Sprintf("%040x", i)
		var parents []string
		if i < n-1 {
			parents = []string{fmt.Sprintf("%040x", i+1)}
		}
		commits[i] = git.Commit{
			Hash:        hash,
			ShortHash:   hash[:7],
			Author:      fmt.Sprintf("Author%d", i%5),
			AuthorEmail: fmt.Sprintf("author%d@test.com", i%5),
			Date:        base.Add(-time.Duration(i) * time.Hour),
			Subject:     fmt.Sprintf("Commit message %d", i),
			Parents:     parents,
		}
	}
	return commits
}

func TestNew(t *testing.T) {
	client := &mockGitClient{}
	cfg := config.GitConfig{MaxLogEntries: 100}
	p := New(client, cfg, nil)

	assert.Equal(t, 100, p.pageSize)
	assert.Equal(t, "Commits", p.Title())
}

func TestNew_DefaultPageSize(t *testing.T) {
	client := &mockGitClient{}
	cfg := config.GitConfig{}
	p := New(client, cfg, nil)

	assert.Equal(t, defaultPageSize, p.pageSize)
}

func TestInit_LoadsCommits(t *testing.T) {
	commits := makeCommits(10)
	client := &mockGitClient{
		logFn: func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
			return commits, nil
		},
	}

	p := New(client, config.GitConfig{}, nil)
	cmd := p.Init(context.Background())

	require.NotNil(t, cmd)
	assert.True(t, p.loading)

	// Execute the command to get the message.
	msg := cmd()
	loaded, ok := msg.(commitsLoadedMsg)
	require.True(t, ok)
	assert.Len(t, loaded.commits, 10)
	assert.False(t, loaded.append)
}

func TestHandleCommitsLoaded(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.loading = true
	p.pageSize = 500

	commits := makeCommits(10)
	result, cmd := p.handleCommitsLoaded(commitsLoadedMsg{
		commits: commits,
		append:  false,
	})

	panel := result.(*Panel)
	assert.False(t, panel.loading)
	assert.True(t, panel.allLoaded) // 10 < 500
	assert.Len(t, panel.commits, 10)
	assert.Nil(t, cmd)

	// Display should have entries for commits.
	assert.True(t, len(panel.display) >= 10)
	assert.Len(t, panel.commitY, 10)
}

func TestHandleCommitsLoaded_Append(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 5

	// First load.
	first := makeCommits(5)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: first, append: false})
	assert.Len(t, p.commits, 5)
	assert.False(t, p.allLoaded) // 5 == pageSize

	// Append load.
	second := makeCommits(3)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: second, append: true})
	assert.Len(t, p.commits, 8)
	assert.True(t, p.allLoaded) // 3 < 5
}

func TestView_EmptyState(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()

	view := p.View(80, 24)
	assert.Contains(t, view, "No commits")
}

func TestView_LoadingState(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.loading = true

	view := p.View(80, 24)
	assert.Contains(t, view, "Loading commits")
}

func TestView_ZeroDimensions(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	assert.Equal(t, "", p.View(0, 0))
	assert.Equal(t, "", p.View(-1, 10))
	assert.Equal(t, "", p.View(10, -1))
}

func TestCommitEntryFormatting(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()

	c := git.Commit{
		Hash:        "abc123def456789000000000000000000000000",
		ShortHash:   "abc123d",
		Author:      "John Doe",
		AuthorEmail: "john@example.com",
		Date:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Subject:     "feat: add fuzzy finder",
		Refs:        []string{"HEAD -> main", "origin/main"},
		Parents:     []string{"parent1"},
	}

	line := p.renderCommitLine(c, "*", 120, false)

	// Check key parts are present.
	assert.Contains(t, line, "abc123d")
	assert.Contains(t, line, "2024-01-15")
	assert.Contains(t, line, "John Doe")
	assert.Contains(t, line, "feat: add fuzzy finder")
	assert.Contains(t, line, "HEAD -> main")
	assert.Contains(t, line, "origin/main")
}

func TestCommitEntryFormatting_NoRefs(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)

	c := git.Commit{
		ShortHash: "abc123d",
		Author:    "Jane",
		Date:      time.Date(2024, 1, 14, 9, 0, 0, 0, time.UTC),
		Subject:   "fix: preview scroll",
	}

	line := p.renderCommitLine(c, "| *", 120, false)

	assert.Contains(t, line, "abc123d")
	assert.Contains(t, line, "Jane")
	assert.Contains(t, line, "fix: preview scroll")
	assert.NotContains(t, line, "(")
}

func TestRefDecorations(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)

	tests := []struct {
		name string
		refs []string
		want string
	}{
		{
			name: "single ref",
			refs: []string{"main"},
			want: "(main)",
		},
		{
			name: "multiple refs",
			refs: []string{"HEAD -> main", "origin/main"},
			want: "(HEAD -> main, origin/main)",
		},
		{
			name: "tag ref",
			refs: []string{"tag: v1.0.0"},
			want: "(tag: v1.0.0)",
		},
		{
			name: "no refs",
			refs: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := git.Commit{
				ShortHash: "abc123d",
				Author:    "Dev",
				Date:      time.Now(),
				Subject:   "test",
				Refs:      tt.refs,
			}
			line := p.renderCommitLine(c, "*", 200, false)
			if tt.want == "" {
				assert.NotContains(t, line, "(")
			} else {
				assert.Contains(t, line, tt.want)
			}
		})
	}
}

func TestNavigation(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.focused = true
	p.height = 20
	p.pageSize = 500

	commits := makeCommits(20)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Initial state.
	assert.Equal(t, 0, p.cursor)

	// Move down.
	p.moveCursorDown()
	assert.Equal(t, 1, p.cursor)

	// Move up.
	p.moveCursorUp()
	assert.Equal(t, 0, p.cursor)

	// Don't go below 0.
	p.moveCursorUp()
	assert.Equal(t, 0, p.cursor)

	// Go to bottom.
	p.goToBottom()
	assert.Equal(t, 19, p.cursor)

	// Don't go past end.
	p.moveCursorDown()
	assert.Equal(t, 19, p.cursor)

	// Go to top.
	p.goToTop()
	assert.Equal(t, 0, p.cursor)
}

func TestPageNavigation(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.focused = true
	p.height = 10
	p.pageSize = 500

	commits := makeCommits(50)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Page down.
	p.pageDown()
	assert.Equal(t, 5, p.cursor) // height/2 = 5

	// Page up.
	p.pageUp()
	assert.Equal(t, 0, p.cursor)

	// Page up from 0 stays at 0.
	p.pageUp()
	assert.Equal(t, 0, p.cursor)
}

func TestPagination_LoadMore(t *testing.T) {
	callCount := 0
	client := &mockGitClient{
		logFn: func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
			callCount++
			return makeCommits(5), nil
		},
	}

	p := New(client, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.focused = true
	p.height = 20
	p.pageSize = 5

	// First load.
	p.handleCommitsLoaded(commitsLoadedMsg{
		commits: makeCommits(5),
		append:  false,
	})
	assert.False(t, p.allLoaded)

	// Move cursor near bottom.
	p.cursor = 4
	cmd := p.loadMore()
	assert.NotNil(t, cmd)
	assert.True(t, p.loading)
}

func TestPagination_Debounce(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 5

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})
	p.lastLoadAt = time.Now()

	// Should be debounced.
	cmd := p.loadMore()
	assert.Nil(t, cmd)
}

func TestPagination_AllLoaded(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.allLoaded = true

	cmd := p.loadMore()
	assert.Nil(t, cmd)
}

func TestSearch(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(20)
	commits[5].Subject = "fix: critical bug"
	commits[10].Subject = "fix: another bug"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Apply search.
	p.searchQuery = "bug"
	p.applySearch()

	assert.Len(t, p.filteredIdx, 2)
	assert.Equal(t, 5, p.filteredIdx[0])
	assert.Equal(t, 10, p.filteredIdx[1])
}

func TestSearch_ByAuthor(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(10)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// makeCommits uses Author0..Author4 cycling.
	p.searchQuery = "Author0"
	p.applySearch()

	assert.Equal(t, 2, len(p.filteredIdx))
}

func TestSearch_ClearResets(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchQuery = "message"
	p.applySearch()
	assert.NotNil(t, p.filteredIdx)

	// Clear search.
	p.searchQuery = ""
	p.applySearch()
	assert.Nil(t, p.filteredIdx)
}

func TestDetailView(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := []git.Commit{
		{
			Hash:        "abc123def456789000000000000000000000000",
			ShortHash:   "abc123d",
			Author:      "John Doe",
			AuthorEmail: "john@example.com",
			Date:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Subject:     "feat: add feature",
			Body:        "Detailed description\nof the feature.",
			Parents:     []string{"parent1"},
			Refs:        []string{"main"},
		},
	}
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.cursor = 0
	p.showDetail()

	assert.True(t, p.detailMode)
	require.NotEmpty(t, p.detailLines)

	// Verify detail content.
	content := strings.Join(p.detailLines, "\n")
	assert.Contains(t, content, "abc123def456789000000000000000000000000")
	assert.Contains(t, content, "John Doe")
	assert.Contains(t, content, "john@example.com")
	assert.Contains(t, content, "feat: add feature")
	assert.Contains(t, content, "Detailed description")
	assert.Contains(t, content, "main")
}

func TestKeyBindings(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	bindings := p.KeyBindings()
	assert.NotEmpty(t, bindings)

	// Verify expected bindings exist.
	actions := make(map[string]bool)
	for _, b := range bindings {
		actions[b.Action] = true
	}

	assert.True(t, actions["cursor_down"])
	assert.True(t, actions["cursor_up"])
	assert.True(t, actions["detail"])
	assert.True(t, actions["page_down"])
	assert.True(t, actions["page_up"])
	assert.True(t, actions["go_top"])
	assert.True(t, actions["go_bottom"])
	assert.True(t, actions["copy_hash"])
	assert.True(t, actions["search"])
}

func TestRebuildDisplay_LinearHistory(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(5)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Linear history: no connectors, each commit is one display line.
	assert.Len(t, p.display, 5)
	assert.Len(t, p.commitY, 5)

	for i, d := range p.display {
		assert.Equal(t, i, d.commitIdx)
		assert.Equal(t, "*", d.text) // linear = single column
	}
}

func TestRebuildDisplay_WithBranch(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	// aaaa → cccc
	// bbbb → cccc  (branch)
	commits := []git.Commit{
		{Hash: "aaaa", ShortHash: "aaaa", Parents: []string{"cccc"}},
		{Hash: "bbbb", ShortHash: "bbbb", Parents: []string{"cccc"}},
		{Hash: "cccc", ShortHash: "cccc", Parents: []string{"dddd"}},
	}
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// aaaa: * (1 line)
	// bbbb: | * (1 line) + |/ connector (1 line) = 2 lines
	// cccc: * (1 line)
	// Total: 4 display lines
	assert.Equal(t, 4, len(p.display))
	assert.Len(t, p.commitY, 3)

	// Verify commit positions.
	assert.Equal(t, 0, p.commitY[0]) // aaaa at display line 0
	assert.Equal(t, 1, p.commitY[1]) // bbbb at display line 1
	assert.Equal(t, 3, p.commitY[2]) // cccc at display line 3 (after connector)
}

func TestTruncateOrPad(t *testing.T) {
	// Shorter than width: pad.
	result := truncateOrPad("abc", 10)
	assert.Equal(t, 10, len(result))
	assert.True(t, strings.HasPrefix(result, "abc"))

	// Exact width: no change.
	result = truncateOrPad("1234567890", 10)
	assert.Equal(t, "1234567890", result)
}

func TestRenderLog_WithCommits(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.height = 20
	p.width = 100
	p.pageSize = 500

	commits := makeCommits(10)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	view := p.renderLog(100, 20)
	assert.NotEmpty(t, view)

	lines := strings.Split(view, "\n")
	assert.Equal(t, 20, len(lines)) // should be padded to height
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestHandleBranchSelected_NewBranch(t *testing.T) {
	client := &mockGitClient{
		logFn: func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
			return makeCommits(5), nil
		},
	}
	p := New(client, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	// Load initial commits.
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})
	p.cursor = 5
	p.offset = 3
	p.searchMode = true
	p.searchQuery = "test"

	// Select a branch.
	result, cmd := p.handleBranchSelected("feature/test")
	panel := result.(*Panel)

	assert.Equal(t, "feature/test", panel.selectedRef)
	assert.Equal(t, 0, panel.cursor, "cursor should reset")
	assert.Equal(t, 0, panel.offset, "offset should reset")
	assert.False(t, panel.searchMode, "search should be cleared")
	assert.Empty(t, panel.searchQuery, "search query should be cleared")
	assert.Nil(t, panel.filteredIdx, "filter should be cleared")
	assert.True(t, panel.loading, "should start loading")
	assert.NotNil(t, cmd, "should return a load command")
}

func TestHandleBranchSelected_SameBranch(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.selectedRef = "main"
	p.cursor = 3

	result, cmd := p.handleBranchSelected("main")
	panel := result.(*Panel)

	assert.Equal(t, 3, panel.cursor, "cursor should not change for same branch")
	assert.Nil(t, cmd, "no command for same branch")
}

func TestHandleBranchSelected_ResetToHead(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.selectedRef = "feature/test"
	p.cursor = 5

	result, cmd := p.handleBranchSelected("")
	panel := result.(*Panel)

	assert.Empty(t, panel.selectedRef, "should reset to HEAD")
	assert.Equal(t, 0, panel.cursor)
	assert.NotNil(t, cmd)
}

func TestHandleSearchKey_Enter(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchMode = true
	p.searchQuery = "message"
	p.applySearch()
	filterCount := len(p.filteredIdx)

	// Press Enter to finish search (keeps filter active).
	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, p.searchMode, "search mode should exit")
	assert.Len(t, p.filteredIdx, filterCount, "filter should remain active")
}

func TestHandleSearchKey_Escape(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchMode = true
	p.searchQuery = "message"
	p.applySearch()

	// Press Escape to cancel search.
	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, p.searchMode, "search mode should exit")
	assert.Empty(t, p.searchQuery, "search query should be cleared")
	assert.Nil(t, p.filteredIdx, "filter should be cleared")
}

func TestHandleSearchKey_Backspace(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchMode = true
	p.searchQuery = "abc"

	// Backspace removes last character.
	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "ab", p.searchQuery)

	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "a", p.searchQuery)

	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "", p.searchQuery)

	// Backspace on empty is a no-op.
	p.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "", p.searchQuery)
}

func TestHandleSearchKey_TypeCharacter(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchMode = true
	p.searchQuery = ""

	// Type printable characters.
	p.handleSearchKey(tea.KeyPressMsg{Code: 'b'})
	assert.Equal(t, "b", p.searchQuery)

	p.handleSearchKey(tea.KeyPressMsg{Code: 'u'})
	assert.Equal(t, "bu", p.searchQuery)

	p.handleSearchKey(tea.KeyPressMsg{Code: 'g'})
	assert.Equal(t, "bug", p.searchQuery)
}

func TestHandleDetailKey_ScrollDown(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.height = 10
	p.focused = true

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})
	p.cursor = 0
	p.showDetail()

	assert.True(t, p.detailMode)
	assert.Equal(t, 0, p.detailOffset)

	// Scroll down.
	p.handleDetailKey(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.detailOffset)

	p.handleDetailKey(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, p.detailOffset)
}

func TestHandleDetailKey_ScrollUp(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.height = 10
	p.focused = true

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})
	p.showDetail()

	// Scroll down first then up.
	p.handleDetailKey(tea.KeyPressMsg{Code: 'j'})
	p.handleDetailKey(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.detailOffset)

	p.handleDetailKey(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.detailOffset)

	// Scroll up past top clamped.
	p.handleDetailKey(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.detailOffset)
	p.handleDetailKey(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.detailOffset)
}

func TestHandleDetailKey_PageDown(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.height = 4
	p.focused = true

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(1)})
	p.showDetail()
	totalLines := len(p.detailLines)

	// Page down = height/2.
	p.handleDetailKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Equal(t, 2, p.detailOffset) // 4/2 = 2

	// Multiple page downs should clamp.
	for range 20 {
		p.handleDetailKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	assert.LessOrEqual(t, p.detailOffset, totalLines-1)
}

func TestHandleDetailKey_PageUp(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.height = 4
	p.focused = true

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(1)})
	p.showDetail()

	// Go down first.
	p.detailOffset = 6

	// Page up = height/2.
	p.handleDetailKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	assert.Equal(t, 4, p.detailOffset) // 6 - 4/2 = 4

	// Multiple page ups should clamp at 0.
	for range 20 {
		p.handleDetailKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	assert.Equal(t, 0, p.detailOffset)
}

func TestHandleDetailKey_Exit(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(1)})
	p.showDetail()
	assert.True(t, p.detailMode)

	// Exit with 'q' key.
	p.handleDetailKey(tea.KeyPressMsg{Code: 'q'})
	assert.False(t, p.detailMode, "pressing q should exit detail mode")
	assert.Nil(t, p.detailLines)
	assert.Equal(t, 0, p.detailOffset)

	// Re-enter and exit with escape.
	p.showDetail()
	assert.True(t, p.detailMode)
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, p.detailMode, "pressing escape should exit detail mode")

	// Re-enter and exit with enter.
	p.showDetail()
	assert.True(t, p.detailMode)
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, p.detailMode, "pressing enter should exit detail mode")
}

func TestFocusBlur(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.Focus()
	assert.True(t, p.focused)

	p.Blur()
	assert.False(t, p.focused)
}

func TestHandleKey_IgnoredWhenBlurred(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})
	p.focused = false

	_, cmd := p.handleKey(tea.KeyPressMsg{Code: 'j'})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, p.cursor, "unfocused panel should ignore keys")
}

func TestSetSize(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.SetSize(120, 40)
	assert.Equal(t, 120, p.width)
	assert.Equal(t, 40, p.height)
}

func TestCopyHash(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true

	commits := makeCommits(5)
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.cursor = 2
	_, cmd := p.copyHash()
	require.NotNil(t, cmd)

	msg := cmd()
	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Message, commits[2].ShortHash)
}

func TestCopyHash_OutOfBounds(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.cursor = -1

	_, cmd := p.copyHash()
	assert.Nil(t, cmd)
}

func TestCopyHash_InSearchMode(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true

	commits := makeCommits(10)
	commits[3].Subject = "fix: special bug"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.searchQuery = "special"
	p.applySearch()
	require.Len(t, p.filteredIdx, 1)

	// Cursor 0 in filtered view = commit index 3.
	p.cursor = 0
	_, cmd := p.copyHash()
	require.NotNil(t, cmd)

	msg := cmd()
	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, commits[3].ShortHash)
}

func TestSearch_NoMatches(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(10)})

	p.searchQuery = "zzz_no_match_zzz"
	p.applySearch()

	assert.Empty(t, p.filteredIdx)
	assert.Equal(t, 0, p.cursor)
}

func TestSearch_ByHash(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(10)
	// Set a unique short hash for commit 5 to ensure search finds it.
	commits[5].ShortHash = "deadbee"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.searchQuery = "deadbee"
	p.applySearch()

	require.Len(t, p.filteredIdx, 1)
	assert.Equal(t, 5, p.filteredIdx[0])
}

func TestSearch_CaseInsensitive(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(5)
	commits[0].Subject = "Fix: UPPERCASE Bug"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.searchQuery = "uppercase"
	p.applySearch()
	assert.Len(t, p.filteredIdx, 1)

	p.searchQuery = "UPPERCASE"
	p.applySearch()
	assert.Len(t, p.filteredIdx, 1)
}

func TestShowDetail_WithFilteredView(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true

	commits := makeCommits(10)
	commits[7].Subject = "fix: unique target"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	p.searchQuery = "unique target"
	p.applySearch()
	require.Len(t, p.filteredIdx, 1)

	p.cursor = 0
	p.showDetail()
	assert.True(t, p.detailMode)

	content := strings.Join(p.detailLines, "\n")
	assert.Contains(t, content, "fix: unique target")
}

func TestShowDetail_NoCommits(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.cursor = 0

	p.showDetail()
	assert.False(t, p.detailMode, "should not enter detail mode with no commits")
}

func TestShowDetail_CursorOutOfBounds(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})

	p.cursor = 100
	p.showDetail()
	assert.False(t, p.detailMode)
}

func TestRenderCommitLine_NarrowWidth(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)

	c := git.Commit{
		ShortHash: "abc123d",
		Author:    "Very Long Author Name",
		Date:      time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Subject:   "Very long subject that should be truncated",
	}

	// Very narrow width — just enough for basic content.
	line := p.renderCommitLine(c, "*", 30, false)
	assert.NotEmpty(t, line)
	assert.Contains(t, line, "abc123d")
}

func TestRenderCommitLine_CursorHighlight(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)

	c := git.Commit{
		ShortHash: "abc123d",
		Author:    "Dev",
		Date:      time.Now(),
		Subject:   "test commit",
	}

	lineCursor := p.renderCommitLine(c, "*", 100, true)
	lineNoCursor := p.renderCommitLine(c, "*", 100, false)

	// Both should contain the commit info.
	assert.Contains(t, lineCursor, "abc123d")
	assert.Contains(t, lineNoCursor, "abc123d")
	// Cursor line should be different (has background color).
	assert.NotEqual(t, lineCursor, lineNoCursor)
}

func TestEnsureCursorVisible_ScrollsDown(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.height = 5
	p.pageSize = 500

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(20)})

	// Move cursor past the visible viewport.
	p.cursor = 10
	p.offset = 0
	p.ensureCursorVisible()

	assert.Greater(t, p.offset, 0, "offset should increase to keep cursor visible")
}

func TestEnsureCursorVisible_ScrollsUp(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.height = 5
	p.pageSize = 500

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(20)})

	// Set offset past cursor.
	p.cursor = 2
	p.offset = 10
	p.ensureCursorVisible()

	assert.LessOrEqual(t, p.offset, p.commitY[2], "offset should decrease to keep cursor visible")
}

func TestEnsureCursorVisible_ZeroHeight(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.height = 0
	p.pageSize = 500

	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})
	p.cursor = 3
	p.offset = 0

	// Should not panic.
	p.ensureCursorVisible()
	assert.Equal(t, 0, p.offset)
}

func TestMaxCursor_WithFilter(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(20)
	commits[5].Subject = "fix: bug"
	commits[10].Subject = "fix: another bug"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Without filter.
	assert.Equal(t, 19, p.maxCursor())

	// With filter.
	p.searchQuery = "bug"
	p.applySearch()
	assert.Equal(t, 1, p.maxCursor()) // 2 results → maxCursor = 1
}

func TestActiveDisplay_Filtered(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500

	commits := makeCommits(10)
	commits[0].Subject = "fix: a bug"
	p.handleCommitsLoaded(commitsLoadedMsg{commits: commits})

	// Without filter, use main display.
	assert.Equal(t, p.display, p.activeDisplay())
	assert.Equal(t, p.commitY, p.activeCommitY())

	// With filter, use filtered display.
	p.searchQuery = "a bug"
	p.applySearch()
	assert.Equal(t, p.filteredDL, p.activeDisplay())
	assert.Equal(t, p.filteredCmtY, p.activeCommitY())
}

func TestView_DetailMode(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})

	p.showDetail()
	view := p.View(80, 20)
	assert.Contains(t, view, "Commit Details")
}

func TestHandleKey_SearchMode(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.focused = true
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})

	// Enter search mode with '/'.
	p.handleKey(tea.KeyPressMsg{Code: '/'})
	assert.True(t, p.searchMode)
	assert.Empty(t, p.searchQuery)
}

func TestUpdate_BranchSelectedMsg(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})

	result, cmd := p.Update(panels.BranchSelectedMsg{Name: "develop"})
	panel := result.(*Panel)
	assert.Equal(t, "develop", panel.selectedRef)
	assert.NotNil(t, cmd)
}

func TestUpdate_BranchChangedMsg(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.selectedRef = "feature/x"
	p.pageSize = 500
	p.handleCommitsLoaded(commitsLoadedMsg{commits: makeCommits(5)})

	result, cmd := p.Update(panels.BranchChangedMsg{})
	panel := result.(*Panel)
	assert.Empty(t, panel.selectedRef, "should reset to HEAD")
	assert.NotNil(t, cmd)
}

func TestTruncateOrPad_Truncate(t *testing.T) {
	result := truncateOrPad("this is a long string", 5)
	// Should be truncated to width 5 (lipgloss MaxWidth).
	assert.Equal(t, 5, lipgloss.Width(result))
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

// newTestPanelWithCommits creates a Panel populated with commits for mouse tests.
func newTestPanelWithCommits(t *testing.T, n int) *Panel {
	t.Helper()
	commits := makeCommits(n)
	client := &mockGitClient{
		logFn: func(_ context.Context, opts git.LogOpts) ([]git.Commit, error) {
			return commits, nil
		},
	}
	p := New(client, config.GitConfig{}, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)
	msg := cmd()
	p.handleCommitsLoaded(msg.(commitsLoadedMsg))
	return p
}

func TestMouseClick_SelectsCommit(t *testing.T) {
	p := newTestPanelWithCommits(t, 10)
	p.Focus()
	p.SetSize(80, 40)

	// The first display line (row 0) should correspond to commit 0.
	p.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)

	// Click a further row that maps to a commit.
	if len(p.display) > 1 && p.display[1].commitIdx >= 0 {
		p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
		assert.Equal(t, p.display[1].commitIdx, p.cursor)
	}
}

func TestMouseClick_OutOfBounds(t *testing.T) {
	p := newTestPanelWithCommits(t, 5)
	p.Focus()
	p.SetSize(80, 40)

	originalCursor := p.cursor
	p.Update(panels.PanelMouseClickMsg{ContentRow: 999, ContentCol: 5})
	assert.Equal(t, originalCursor, p.cursor, "out-of-bounds click should not move cursor")
}

func TestMouseDoubleClick_ShowsDetail(t *testing.T) {
	p := newTestPanelWithCommits(t, 5)
	p.Focus()
	p.SetSize(80, 40)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemLogCommit): true}

	assert.False(t, p.detailMode)

	// Double-click on the first commit row.
	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 0, ContentCol: 5})
	assert.True(t, p.detailMode, "double-click should enter detail mode")
}

func TestMouseDoubleClick_OutOfBounds(t *testing.T) {
	p := newTestPanelWithCommits(t, 5)
	p.Focus()
	p.SetSize(80, 40)

	p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 999, ContentCol: 5})
	assert.False(t, p.detailMode, "out-of-bounds double-click should not enter detail mode")
}

func TestMouseWheel_Down(t *testing.T) {
	p := newTestPanelWithCommits(t, 20)
	p.Focus()
	p.SetSize(80, 5)

	assert.Equal(t, 0, p.offset)
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, p.offset, 0, "wheel down should increase offset")
}

func TestMouseWheel_Up(t *testing.T) {
	p := newTestPanelWithCommits(t, 20)
	p.Focus()
	p.SetSize(80, 5)

	p.offset = 5
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Less(t, p.offset, 5, "wheel up should decrease offset")
}

func TestMouseWheel_UpAtZero(t *testing.T) {
	p := newTestPanelWithCommits(t, 5)
	p.Focus()
	p.SetSize(80, 40)

	p.offset = 0
	p.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, p.offset, "offset should not go below 0")
}

// ---------------------------------------------------------------------------
// RepoChangedMsg
// ---------------------------------------------------------------------------

func TestRepoChangedMsg_ResetsAndReloads(t *testing.T) {
	p := newTestPanelWithCommits(t, 5)

	// Set some state that should be cleared.
	p.selectedRef = "feature"
	p.cursor = 3
	p.offset = 10

	tmpDir := t.TempDir()
	result, cmd := p.Update(panels.RepoChangedMsg{Path: tmpDir})
	p = result.(*Panel)

	// git.NewClient succeeds for any valid path, so we get a new client and reload.
	assert.NotNil(t, p.gitClient, "gitClient should be set after RepoChangedMsg")
	assert.Nil(t, p.commits, "commits should be cleared before reload")
	assert.Nil(t, p.display, "display should be cleared before reload")
	assert.Equal(t, "", p.selectedRef, "selectedRef should be empty")
	assert.Equal(t, 0, p.cursor, "cursor should be reset")
	assert.Equal(t, 0, p.offset, "offset should be reset")
	assert.False(t, p.searchMode, "searchMode should be false")
	assert.False(t, p.detailMode, "detailMode should be false")
	assert.NotNil(t, cmd, "a reload command should be returned")
}

// ---------------------------------------------------------------------------
// ANSI escape-sequence injection regression tests (CWE-150)
// ---------------------------------------------------------------------------

// TestRenderCommitLine_ANSIInjection verifies that ANSI escape sequences
// in untrusted git data (subject, author, refs) are stripped before display.
func TestRenderCommitLine_ANSIInjection(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)

	c := git.Commit{
		ShortHash: "abc1234",
		Author:    "Evil\x1b[31mRED\x1b[0m",
		Date:      time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Subject:   "feat: \x1b[1mbold injection\x1b[0m",
		Refs:      []string{"\x1b]0;pwned-title\x07main"},
	}

	line := p.renderCommitLine(c, "*", 120, false)
	stripped := panels.StripANSI(line)

	// After stripping lipgloss styling, no raw escape sequences should remain.
	assert.NotContains(t, stripped, "\x1b", "ANSI escape in rendered commit line")
	assert.Contains(t, stripped, "bold injection", "subject text preserved")
	assert.Contains(t, stripped, "EvilRED", "author text preserved without ANSI")
	assert.Contains(t, stripped, "main", "ref text preserved")
}

// TestDetailView_ANSIInjection verifies that commit detail view strips ANSI
// from subject, author, email, refs, and body.
func TestDetailView_ANSIInjection(t *testing.T) {
	p := New(&mockGitClient{}, config.GitConfig{}, nil)
	p.ctx = context.Background()
	p.width = 80
	p.height = 40

	p.commits = []git.Commit{{
		Hash:        "abc123def456789000000000000000000000000",
		ShortHash:   "abc123d",
		Author:      "\x1b[2J\x1b[HClearScreen",
		AuthorEmail: "evil\x1b[31m@example.com",
		Date:        time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Subject:     "feat: \x1b]0;title-attack\x07injection",
		Body:        "Body with \x1b[1mbold\x1b[0m escape",
		Refs:        []string{"\x1b[31mred-ref\x1b[0m"},
		Parents:     []string{"parent1"},
	}}

	p.showDetail()
	require.True(t, p.detailMode, "detail mode should be active")

	// Join all detail lines and strip lipgloss styling.
	combined := strings.Join(p.detailLines, "\n")
	stripped := panels.StripANSI(combined)

	assert.NotContains(t, stripped, "\x1b", "no raw ANSI escapes in detail view")
	assert.Contains(t, stripped, "ClearScreen", "author name preserved")
	assert.Contains(t, stripped, "injection", "subject preserved")
	assert.Contains(t, stripped, "bold", "body text preserved")
	assert.Contains(t, stripped, "red-ref", "ref text preserved")
}
