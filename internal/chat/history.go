package chat

import "strings"

const maxInputHistoryEntries = 200

// InputHistory provides terminal-style input history with Up/Down navigation.
type InputHistory struct {
	draft   string   // in-progress text saved when user starts browsing
	entries []string // past inputs, oldest first
	index   int      // current position (-1 = not browsing)
}

// Push adds a sent message to history. It trims whitespace, skips empty
// strings, and suppresses consecutive duplicates.
func (h *InputHistory) Push(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > maxInputHistoryEntries {
		start := len(h.entries) - maxInputHistoryEntries
		entries := make([]string, maxInputHistoryEntries)
		copy(entries, h.entries[start:])
		h.entries = entries
	}
	h.index = -1
}

// Up moves to an older history entry. On the first call (index == -1) it
// saves currentInput as the draft and jumps to the newest entry. Subsequent
// calls decrement the index (minimum 0). Returns the entry and true, or
// ("", false) when history is empty.
func (h *InputHistory) Up(currentInput string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.index == -1 {
		h.draft = currentInput
		h.index = len(h.entries) - 1
	} else if h.index > 0 {
		h.index--
	}
	return h.entries[h.index], true
}

// Down moves to a newer history entry. If we move past the newest entry,
// it resets the index and returns the saved draft. Returns ("", false) when
// not currently browsing history.
func (h *InputHistory) Down() (string, bool) {
	if h.index == -1 {
		return "", false
	}
	h.index++
	if h.index >= len(h.entries) {
		h.index = -1
		return h.draft, true
	}
	return h.entries[h.index], true
}

// Reset clears the browsing state, returning to "not browsing" mode.
func (h *InputHistory) Reset() {
	h.index = -1
	h.draft = ""
}
