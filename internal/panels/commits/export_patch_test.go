package commits

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/notify"
)

// ---------------------------------------------------------------------------
// patchFileName / slugify
// ---------------------------------------------------------------------------

func TestPatchFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subject string
		want    string
	}{
		{name: "simple", subject: "Add a feature", want: "0001-add-a-feature.patch"},
		{name: "punctuation collapses", subject: "Fix: the (broken) thing!", want: "0001-fix-the-broken-thing.patch"},
		{name: "leading and trailing junk", subject: "  --wip-- ", want: "0001-wip.patch"},
		{name: "keeps digits", subject: "Bump v2 to v3", want: "0001-bump-v2-to-v3.patch"},
		{name: "empty falls back", subject: "", want: "0001-patch.patch"},
		{name: "only symbols falls back", subject: "!!! @@@ ###", want: "0001-patch.patch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := patchFileName(tt.subject); got != tt.want {
				t.Errorf("patchFileName(%q) = %q, want %q", tt.subject, got, tt.want)
			}
		})
	}
}

func TestPatchFileNameTruncatesLongSubject(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("word ", 40) // far longer than the slug cap
	got := patchFileName(long)

	if !strings.HasPrefix(got, "0001-") || !strings.HasSuffix(got, ".patch") {
		t.Fatalf("unexpected shape: %q", got)
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(got, "0001-"), ".patch")
	if len(slug) > 50 {
		t.Errorf("slug too long: %d chars (%q)", len(slug), slug)
	}
	if strings.HasSuffix(slug, "-") {
		t.Errorf("slug should not end with a dash: %q", slug)
	}
}

// ---------------------------------------------------------------------------
// exportPatch
// ---------------------------------------------------------------------------

func TestExportPatchWritesFile(t *testing.T) {
	root := t.TempDir()
	const patch = "From abc123 Mon Sep 17 00:00:00 2001\nSubject: [PATCH] Initial commit\n"
	mock := &mockGitOps{commits: defaultCommits(), patch: patch, root: root}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Fatal("expected a command from x")
	}

	// The commit under the cursor is the first one.
	if mock.lastHash != "abc1234567890" {
		t.Errorf("FormatPatch got hash %q, want the cursor commit hash", mock.lastHash)
	}

	dest := filepath.Join(root, "0001-initial-commit.patch")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("expected patch file at %s: %v", dest, err)
	}
	if string(got) != patch {
		t.Errorf("patch file content = %q, want %q", string(got), patch)
	}

	msg, ok := cmd().(notify.ShowToastMsg)
	if !ok {
		t.Fatalf("expected ShowToastMsg, got %T", cmd())
	}
	if msg.Level != notify.Success {
		t.Errorf("toast level = %v, want Success", msg.Level)
	}
	if !strings.Contains(msg.Message, dest) {
		t.Errorf("toast %q should mention written path %q", msg.Message, dest)
	}
}

func TestExportPatchFormatError(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits(), patchErr: os.ErrPermission, root: t.TempDir()}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Fatal("expected a command from x")
	}
	msg, ok := cmd().(notify.ShowToastMsg)
	if !ok {
		t.Fatalf("expected ShowToastMsg, got %T", cmd())
	}
	if msg.Level != notify.Error {
		t.Errorf("toast level = %v, want Error", msg.Level)
	}
	if !strings.Contains(msg.Message, "Export failed") {
		t.Errorf("toast %q should report the failure", msg.Message)
	}
}

func TestExportPatchRepoRootError(t *testing.T) {
	mock := &mockGitOps{commits: defaultCommits(), patch: "x", rootErr: os.ErrNotExist}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Fatal("expected a command from x")
	}
	msg, ok := cmd().(notify.ShowToastMsg)
	if !ok {
		t.Fatalf("expected ShowToastMsg, got %T", cmd())
	}
	if msg.Level != notify.Error {
		t.Errorf("toast level = %v, want Error", msg.Level)
	}
}

func TestExportPatchEmptyListNoop(t *testing.T) {
	mock := &mockGitOps{commits: nil, root: t.TempDir()}
	p := newTestPanel(mock)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd != nil {
		t.Error("expected no command when there are no commits")
	}
	if mock.lastHash != "" {
		t.Errorf("FormatPatch should not be called on an empty list, got hash %q", mock.lastHash)
	}
}
