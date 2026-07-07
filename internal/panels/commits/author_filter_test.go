package commits

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
)

// authorFilterCommits returns commits where the author "Alice" appears twice so
// filtering produces more than one match.
func authorFilterCommits() []git.Commit {
	return []git.Commit{
		{Hash: "a000001", ShortHash: "a000001", Author: "Alice", Date: time.Now().Add(-1 * time.Hour), Subject: "Initial commit"},
		{Hash: "b000002", ShortHash: "b000002", Author: "Bob", Date: time.Now().Add(-2 * time.Hour), Subject: "Add feature"},
		{Hash: "a000003", ShortHash: "a000003", Author: "Alice", Date: time.Now().Add(-3 * time.Hour), Subject: "Alice fix"},
		{Hash: "c000004", ShortHash: "c000004", Author: "Charlie", Date: time.Now().Add(-4 * time.Hour), Subject: "Fix bug"},
	}
}

func TestAuthorFilterToggle(t *testing.T) {
	mock := &mockGitOps{commits: authorFilterCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Cursor starts at 0 (Alice). Press 'a' to filter by Alice.
	p.Update(tea.KeyPressMsg{Code: 'a'})
	if p.authorFilter != "Alice" {
		t.Fatalf("expected authorFilter=Alice, got %q", p.authorFilter)
	}
	if p.activeLen() != 2 {
		t.Fatalf("expected 2 commits by Alice, got %d", p.activeLen())
	}
	if p.filteredIdx[0] != 0 || p.filteredIdx[1] != 2 {
		t.Errorf("expected filteredIdx=[0 2], got %v", p.filteredIdx)
	}
	if !strings.Contains(p.Title(), "[@Alice]") {
		t.Errorf("expected title to show active author, got %q", p.Title())
	}

	// Press 'a' again to clear the filter (cursor still on an Alice commit).
	p.Update(tea.KeyPressMsg{Code: 'a'})
	if p.authorFilter != "" {
		t.Fatalf("expected authorFilter cleared, got %q", p.authorFilter)
	}
	if p.filteredIdx != nil {
		t.Errorf("expected filteredIdx=nil after clearing, got %v", p.filteredIdx)
	}
	if p.activeLen() != 4 {
		t.Errorf("expected all 4 commits after clearing, got %d", p.activeLen())
	}
}

func TestAuthorFilterSwitchAuthor(t *testing.T) {
	mock := &mockGitOps{commits: authorFilterCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Filter by Alice, then move the cursor to Bob within the filtered set is
	// not possible (Bob is filtered out), so clear and move to Bob first.
	p.Update(tea.KeyPressMsg{Code: 'a'}) // Alice
	p.Update(tea.KeyPressMsg{Code: 'a'}) // clear
	p.Update(tea.KeyPressMsg{Code: 'j'}) // cursor -> Bob (index 1)

	p.Update(tea.KeyPressMsg{Code: 'a'})
	if p.authorFilter != "Bob" {
		t.Fatalf("expected authorFilter=Bob, got %q", p.authorFilter)
	}
	if p.activeLen() != 1 {
		t.Errorf("expected 1 commit by Bob, got %d", p.activeLen())
	}
}

func TestAuthorFilterCombinesWithSearch(t *testing.T) {
	mock := &mockGitOps{commits: authorFilterCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	// Filter by Alice (indices 0 and 2).
	p.Update(tea.KeyPressMsg{Code: 'a'})
	// Search for "fix"; only Alice's "Alice fix" (index 2) matches both.
	p.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	for _, ch := range "fix" {
		p.Update(tea.KeyPressMsg{Code: ch})
	}
	if len(p.filteredIdx) != 1 {
		t.Fatalf("expected 1 combined match, got %d (%v)", len(p.filteredIdx), p.filteredIdx)
	}
	if p.filteredIdx[0] != 2 {
		t.Errorf("expected combined match at index 2, got %d", p.filteredIdx[0])
	}

	// Leaving search keeps the author filter active.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.authorFilter != "Alice" {
		t.Errorf("expected author filter to persist after search escape, got %q", p.authorFilter)
	}
	if p.activeLen() != 2 {
		t.Errorf("expected author filter to show 2 commits after search escape, got %d", p.activeLen())
	}
}

func TestAuthorFilterEscapeClears(t *testing.T) {
	mock := &mockGitOps{commits: authorFilterCommits()}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	p.Update(tea.KeyPressMsg{Code: 'a'})
	if p.authorFilter == "" {
		t.Fatal("expected an active author filter")
	}
	// Escape in the normal list clears the author filter.
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.authorFilter != "" {
		t.Errorf("expected Escape to clear author filter, got %q", p.authorFilter)
	}
	if p.filteredIdx != nil {
		t.Errorf("expected filteredIdx=nil after Escape, got %v", p.filteredIdx)
	}
}

func TestAuthorFilterEmptyAuthor(t *testing.T) {
	mock := &mockGitOps{commits: []git.Commit{
		{Hash: "z000001", ShortHash: "z000001", Author: "", Date: time.Now(), Subject: "No author"},
	}}
	p := newTestPanel(mock)
	p.Focus()
	p.SetSize(80, 20)

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	if p.authorFilter != "" {
		t.Errorf("expected no author filter for empty author, got %q", p.authorFilter)
	}
	if cmd == nil {
		t.Fatal("expected an info toast command")
	}
	if toast, ok := cmd().(notify.ShowToastMsg); !ok || toast.Level != notify.Info {
		t.Errorf("expected an info toast for empty author, got %#v", cmd())
	}
}
