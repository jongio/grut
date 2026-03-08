package layout

import "fmt"

// Tab represents a single tab with its own layout tree.
type Tab struct {
	Name string
	Tree Node
}

// TabManager manages a set of tabs, each with its own layout tree.
// Only one tab is active at a time.
type TabManager struct {
	tabs      []Tab
	activeIdx int
}

// NewTabManager creates a tab manager with a single initial tab.
func NewTabManager(name string, tree Node) *TabManager {
	return &TabManager{
		tabs: []Tab{
			{Name: name, Tree: tree},
		},
		activeIdx: 0,
	}
}

// ActiveTab returns the currently active tab.
func (tm *TabManager) ActiveTab() *Tab {
	if len(tm.tabs) == 0 {
		return nil
	}
	return &tm.tabs[tm.activeIdx]
}

// ActiveIndex returns the index of the active tab.
func (tm *TabManager) ActiveIndex() int {
	return tm.activeIdx
}

// Count returns the number of tabs.
func (tm *TabManager) Count() int {
	return len(tm.tabs)
}

// Tabs returns a copy of all tabs.
func (tm *TabManager) Tabs() []Tab {
	out := make([]Tab, len(tm.tabs))
	copy(out, tm.tabs)
	return out
}

// Add creates a new tab with the given name and layout tree,
// appended after the current active tab. The new tab becomes active.
func (tm *TabManager) Add(name string, tree Node) {
	newTab := Tab{Name: name, Tree: tree}
	insertIdx := tm.activeIdx + 1

	// Insert at insertIdx
	tm.tabs = append(tm.tabs, Tab{})
	copy(tm.tabs[insertIdx+1:], tm.tabs[insertIdx:])
	tm.tabs[insertIdx] = newTab
	tm.activeIdx = insertIdx
}

// Close removes the tab at the given index. Returns an error if it's
// the last tab or the index is out of range.
func (tm *TabManager) Close(idx int) error {
	if idx < 0 || idx >= len(tm.tabs) {
		return fmt.Errorf("tab index %d out of range [0, %d)", idx, len(tm.tabs))
	}
	if len(tm.tabs) <= 1 {
		return fmt.Errorf("cannot close the last tab")
	}

	tm.tabs = append(tm.tabs[:idx], tm.tabs[idx+1:]...)

	// Adjust active index
	if tm.activeIdx >= len(tm.tabs) {
		tm.activeIdx = len(tm.tabs) - 1
	} else if idx < tm.activeIdx {
		tm.activeIdx--
	}

	return nil
}

// Select sets the active tab to the given index. Returns an error if
// the index is out of range.
func (tm *TabManager) Select(idx int) error {
	if idx < 0 || idx >= len(tm.tabs) {
		return fmt.Errorf("tab index %d out of range [0, %d)", idx, len(tm.tabs))
	}
	tm.activeIdx = idx
	return nil
}

// Rename changes the name of the tab at the given index.
func (tm *TabManager) Rename(idx int, name string) error {
	if idx < 0 || idx >= len(tm.tabs) {
		return fmt.Errorf("tab index %d out of range [0, %d)", idx, len(tm.tabs))
	}
	tm.tabs[idx].Name = name
	return nil
}

// MoveLeft swaps the active tab with its left neighbor. No-op if
// already at the leftmost position.
func (tm *TabManager) MoveLeft() {
	if tm.activeIdx <= 0 {
		return
	}
	tm.tabs[tm.activeIdx], tm.tabs[tm.activeIdx-1] = tm.tabs[tm.activeIdx-1], tm.tabs[tm.activeIdx]
	tm.activeIdx--
}

// MoveRight swaps the active tab with its right neighbor. No-op if
// already at the rightmost position.
func (tm *TabManager) MoveRight() {
	if tm.activeIdx >= len(tm.tabs)-1 {
		return
	}
	tm.tabs[tm.activeIdx], tm.tabs[tm.activeIdx+1] = tm.tabs[tm.activeIdx+1], tm.tabs[tm.activeIdx]
	tm.activeIdx++
}

// NextTab cycles to the next tab, wrapping around.
func (tm *TabManager) NextTab() {
	if len(tm.tabs) <= 1 {
		return
	}
	tm.activeIdx = (tm.activeIdx + 1) % len(tm.tabs)
}

// PrevTab cycles to the previous tab, wrapping around.
func (tm *TabManager) PrevTab() {
	if len(tm.tabs) <= 1 {
		return
	}
	tm.activeIdx = (tm.activeIdx - 1 + len(tm.tabs)) % len(tm.tabs)
}
