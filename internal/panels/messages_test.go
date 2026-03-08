package panels

import "testing"

func TestNewMessageTypes(t *testing.T) {
	// Verify all new message types can be constructed with expected fields.
	t.Run("FolderSelectedMsg", func(t *testing.T) {
		msg := FolderSelectedMsg{Path: "/repo/internal"}
		if msg.Path != "/repo/internal" {
			t.Errorf("expected Path=%q, got %q", "/repo/internal", msg.Path)
		}
	})

	t.Run("WorktreeSelectedMsg", func(t *testing.T) {
		msg := WorktreeSelectedMsg{Path: "/wt/feature", Branch: "feature"}
		if msg.Path != "/wt/feature" {
			t.Errorf("expected Path=%q, got %q", "/wt/feature", msg.Path)
		}
		if msg.Branch != "feature" {
			t.Errorf("expected Branch=%q, got %q", "feature", msg.Branch)
		}
	})

	t.Run("RemoteSelectedMsg", func(t *testing.T) {
		msg := RemoteSelectedMsg{Name: "origin"}
		if msg.Name != "origin" {
			t.Errorf("expected Name=%q, got %q", "origin", msg.Name)
		}
	})

	t.Run("StashSelectedMsg", func(t *testing.T) {
		msg := StashSelectedMsg{Index: 3, Hash: "abc123"}
		if msg.Index != 3 {
			t.Errorf("expected Index=3, got %d", msg.Index)
		}
		if msg.Hash != "abc123" {
			t.Errorf("expected Hash=%q, got %q", "abc123", msg.Hash)
		}
	})

	t.Run("CommitSelectedMsg_Subject", func(t *testing.T) {
		msg := CommitSelectedMsg{Hash: "abc123", Subject: "Fix bug"}
		if msg.Hash != "abc123" {
			t.Errorf("expected Hash=%q, got %q", "abc123", msg.Hash)
		}
		if msg.Subject != "Fix bug" {
			t.Errorf("expected Subject=%q, got %q", "Fix bug", msg.Subject)
		}
	})

	t.Run("ChangeDirectoryMsg", func(t *testing.T) {
		msg := ChangeDirectoryMsg{Path: "/tmp/newdir"}
		if msg.Path != "/tmp/newdir" {
			t.Errorf("expected Path=%q, got %q", "/tmp/newdir", msg.Path)
		}
	})
}
