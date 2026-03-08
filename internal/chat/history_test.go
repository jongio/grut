package chat

import "testing"

func TestPushStoresEntries(t *testing.T) {
	var h InputHistory
	h.Push("hello")
	h.Push("world")

	if len(h.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(h.entries))
	}
	if h.entries[0] != "hello" || h.entries[1] != "world" {
		t.Fatalf("unexpected entries: %v", h.entries)
	}
}

func TestPushDeduplicatesConsecutive(t *testing.T) {
	var h InputHistory
	h.Push("hello")
	h.Push("hello")
	h.Push("world")
	h.Push("world")

	if len(h.entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(h.entries))
	}
}

func TestPushTrimsWhitespace(t *testing.T) {
	var h InputHistory
	h.Push("  hello  ")

	if h.entries[0] != "hello" {
		t.Fatalf("expected trimmed entry, got %q", h.entries[0])
	}
}

func TestPushSkipsEmpty(t *testing.T) {
	var h InputHistory
	h.Push("")
	h.Push("   ")

	if len(h.entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(h.entries))
	}
}

func TestUpNavigatesNewestToOldest(t *testing.T) {
	var h InputHistory
	h.Push("first")
	h.Push("second")
	h.Push("third")

	text, ok := h.Up("")
	if !ok || text != "third" {
		t.Fatalf("first Up: got %q, %v", text, ok)
	}
	text, ok = h.Up("")
	if !ok || text != "second" {
		t.Fatalf("second Up: got %q, %v", text, ok)
	}
	text, ok = h.Up("")
	if !ok || text != "first" {
		t.Fatalf("third Up: got %q, %v", text, ok)
	}
	// At oldest, stays at oldest.
	text, ok = h.Up("")
	if !ok || text != "first" {
		t.Fatalf("fourth Up: got %q, %v", text, ok)
	}
}

func TestUpSavesDraft(t *testing.T) {
	var h InputHistory
	h.Push("old")

	text, ok := h.Up("my draft")
	if !ok || text != "old" {
		t.Fatalf("Up: got %q, %v", text, ok)
	}
	if h.draft != "my draft" {
		t.Fatalf("expected draft %q, got %q", "my draft", h.draft)
	}
}

func TestDownNavigatesOldestToNewest(t *testing.T) {
	var h InputHistory
	h.Push("first")
	h.Push("second")
	h.Push("third")

	// Navigate to oldest.
	h.Up("")
	h.Up("")
	h.Up("")

	text, ok := h.Down()
	if !ok || text != "second" {
		t.Fatalf("first Down: got %q, %v", text, ok)
	}
	text, ok = h.Down()
	if !ok || text != "third" {
		t.Fatalf("second Down: got %q, %v", text, ok)
	}
}

func TestDownRestoresDraft(t *testing.T) {
	var h InputHistory
	h.Push("old")

	h.Up("my draft")

	text, ok := h.Down()
	if !ok || text != "my draft" {
		t.Fatalf("Down past newest: got %q, %v", text, ok)
	}
	if h.index != -1 {
		t.Fatalf("expected index -1 after restoring draft, got %d", h.index)
	}
}

func TestResetClearsBrowsingState(t *testing.T) {
	var h InputHistory
	h.Push("entry")
	h.Up("draft")

	h.Reset()

	if h.index != -1 {
		t.Fatalf("expected index -1, got %d", h.index)
	}
	if h.draft != "" {
		t.Fatalf("expected empty draft, got %q", h.draft)
	}
}

func TestUpOnEmptyHistory(t *testing.T) {
	var h InputHistory
	_, ok := h.Up("text")
	if ok {
		t.Fatal("Up on empty history should return false")
	}
}

func TestDownWhenNotBrowsing(t *testing.T) {
	var h InputHistory
	h.Push("entry")

	_, ok := h.Down()
	if ok {
		t.Fatal("Down when not browsing should return false")
	}
}
