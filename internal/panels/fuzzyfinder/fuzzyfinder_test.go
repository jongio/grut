package fuzzyfinder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// staticSource is a test source that returns a fixed set of items.
type staticSource struct {
	name  string
	items []Item
}

func (s *staticSource) Name() string  { return s.name }
func (s *staticSource) Items() []Item { return s.items }

func newTestItems() []Item {
	return []Item{
		{Text: "main.go", Category: "file", Value: "/src/main.go"},
		{Text: "README.md", Category: "file", Value: "/src/README.md"},
		{Text: "internal/app.go", Category: "file", Value: "/src/internal/app.go"},
		{Text: "internal/config.go", Category: "file", Value: "/src/internal/config.go"},
		{Text: "go.mod", Category: "file", Value: "/src/go.mod"},
	}
}

func newTestFinder(items []Item) *FuzzyFinder {
	return New(&staticSource{name: "test", items: items})
}

// textKeyMsg constructs a KeyPressMsg for printable text input.
func textKeyMsg(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: -1, Text: text}
}

// ---------------------------------------------------------------------------
// Source tests
// ---------------------------------------------------------------------------

func TestFileSourceWalksDirectory(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("a"), 0o644))
	// Hidden file should be excluded.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o644))
	// Hidden directory should be excluded.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("c"), 0o644))

	src := NewFileSource(dir)
	assert.Equal(t, "files", src.Name())

	items := src.Items()
	assert.Equal(t, 2, len(items), "should include only non-hidden files")

	// All items should have category "file" and forward-slash paths.
	texts := make([]string, len(items))
	for i, item := range items {
		assert.Equal(t, "file", item.Category)
		assert.NotContains(t, item.Text, "\\", "paths should use forward slashes")
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "main.go")
	assert.Contains(t, texts, "src/app.go")
}

func TestFileSourceEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	src := NewFileSource(dir)
	items := src.Items()
	assert.Empty(t, items)
}

// ---------------------------------------------------------------------------
// DirectorySource tests
// ---------------------------------------------------------------------------

func TestDirectorySourceWalksDirectories(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	// Files should be excluded.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("a"), 0o644))

	src := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	assert.Equal(t, "directories", src.Name())

	items := src.Items()

	// Should contain ".." + "src" + "docs" = 3 items (no files).
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
		assert.Equal(t, "directory", item.Category)
	}

	assert.Contains(t, texts, "src")
	assert.Contains(t, texts, "docs")
	assert.NotContains(t, texts, "main.go", "files should be excluded")
	assert.NotContains(t, texts, "src/app.go", "files should be excluded")
}

func TestDirectorySourceSkipsHidden(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "visible"), 0o755))

	src := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	items := src.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "visible")
	assert.NotContains(t, texts, ".hidden", "hidden directories should be excluded")
}

func TestDirectorySourceSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))

	src := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	items := src.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "src")
	assert.NotContains(t, texts, "node_modules", "node_modules should be excluded")
	assert.NotContains(t, texts, "node_modules/pkg", "children of node_modules should be excluded")
}

func TestDirectorySourceIncludesParent(t *testing.T) {
	dir := t.TempDir()

	src := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	items := src.Items()

	require.NotEmpty(t, items, "should have at least the parent entry")
	first := items[0]
	assert.Equal(t, "..", first.Text)
	assert.Equal(t, "directory", first.Category)
	assert.Equal(t, filepath.Dir(dir), first.Value)
}

func TestDirectorySourceMaxDepth(t *testing.T) {
	dir := t.TempDir()

	// Create 3 levels of nesting: a/b/c
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755))

	src := NewDirectorySource(dir, 2)
	items := src.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "a")
	assert.Contains(t, texts, "a/b")
	assert.NotContains(t, texts, "a/b/c", "depth 3 should be excluded when maxDepth=2")
}

func TestDirectorySourceEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	src := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	items := src.Items()

	// Should contain only the ".." parent entry.
	require.Len(t, items, 1, "empty dir should return only parent entry")
	assert.Equal(t, "..", items[0].Text)
	assert.Equal(t, "directory", items[0].Category)
}

func TestCommandSourceDeduplicates(t *testing.T) {
	bindings := []keymap.Binding{
		{Key: "ctrl+c", Action: "quit", Description: "Quit grut", Mode: keymap.ModeGlobal},
		{Key: "q", Action: "quit", Description: "Quit", Mode: keymap.ModePanel},
		{Key: "j", Action: "cursor_down", Description: "Move down", Mode: keymap.ModePanel},
	}

	src := NewCommandSource(bindings)
	assert.Equal(t, "commands", src.Name())

	items := src.Items()
	// "quit" should be deduplicated — first binding wins.
	assert.Equal(t, 2, len(items), "duplicate actions should be deduplicated")

	for _, item := range items {
		assert.Equal(t, "command", item.Category)
	}

	// First "quit" binding should win (ctrl+c).
	assert.Equal(t, "quit", items[0].Text)
	assert.Contains(t, items[0].Description, "ctrl+c")
}

func TestCommandSourceEmpty(t *testing.T) {
	src := NewCommandSource(nil)
	items := src.Items()
	assert.Empty(t, items)
}

// ---------------------------------------------------------------------------
// FuzzyFinder construction tests
// ---------------------------------------------------------------------------

func TestNewShowsAllItems(t *testing.T) {
	items := newTestItems()
	ff := newTestFinder(items)

	assert.Equal(t, len(items), ff.matchCount(), "all items should be visible with empty query")
	assert.Equal(t, 0, ff.cursorIndex(), "cursor should start at 0")
}

func TestNewWithMultipleSources(t *testing.T) {
	files := &staticSource{name: "files", items: []Item{
		{Text: "main.go", Category: "file", Value: "main.go"},
	}}
	cmds := &staticSource{name: "commands", items: []Item{
		{Text: "quit", Category: "command", Value: "quit"},
	}}

	ff := New(files, cmds)
	assert.Equal(t, 2, ff.matchCount(), "should combine items from all sources")
}

func TestNewEmptySource(t *testing.T) {
	ff := newTestFinder([]Item{})
	assert.Equal(t, 0, ff.matchCount())
	assert.Nil(t, ff.selectedItem())
}

// ---------------------------------------------------------------------------
// Fuzzy matching tests
// ---------------------------------------------------------------------------

func TestFuzzyMatchingFiltersResults(t *testing.T) {
	ff := newTestFinder(newTestItems())

	// Type "app" — should match "internal/app.go".
	for _, ch := range "app" {
		ff.Update(textKeyMsg(string(ch)))
	}

	assert.True(t, ff.matchCount() > 0, "should have matches for 'app'")
	assert.True(t, ff.matchCount() < 5, "should filter some items")

	// Verify internal/app.go is in results.
	found := false
	for _, m := range ff.matches {
		if ff.items[m.Index].Text == "internal/app.go" {
			found = true
			break
		}
	}
	assert.True(t, found, "internal/app.go should match 'app'")
}

func TestFuzzyMatchNoResults(t *testing.T) {
	ff := newTestFinder(newTestItems())

	for _, ch := range "zzzzz" {
		ff.Update(textKeyMsg(string(ch)))
	}

	assert.Equal(t, 0, ff.matchCount(), "nonsense query should match nothing")
	assert.Nil(t, ff.selectedItem())
}

func TestEmptyQueryShowsAll(t *testing.T) {
	ff := newTestFinder(newTestItems())

	// Type something to filter.
	ff.Update(textKeyMsg("x"))
	assert.True(t, ff.matchCount() < 5)

	// Clear with ctrl+u.
	ff.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Equal(t, 5, ff.matchCount(), "ctrl+u should clear query and show all")
	assert.Equal(t, "", ff.queryValue())
}

// ---------------------------------------------------------------------------
// Navigation tests
// ---------------------------------------------------------------------------

func TestCursorDownUp(t *testing.T) {
	ff := newTestFinder(newTestItems())

	// Move down.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, ff.cursorIndex())

	ff.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, ff.cursorIndex())

	// Move up.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, ff.cursorIndex())

	// Can't go below 0.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	ff.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, ff.cursorIndex())
}

func TestCursorCannotExceedResults(t *testing.T) {
	items := []Item{
		{Text: "one", Category: "file", Value: "one"},
		{Text: "two", Category: "file", Value: "two"},
	}
	ff := newTestFinder(items)

	// Move down past the end.
	for i := 0; i < 10; i++ {
		ff.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	assert.Equal(t, 1, ff.cursorIndex(), "cursor should stop at last item")
}

func TestCtrlPCtrlNNavigation(t *testing.T) {
	ff := newTestFinder(newTestItems())

	ff.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	assert.Equal(t, 1, ff.cursorIndex(), "ctrl+n should move down")

	ff.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	assert.Equal(t, 0, ff.cursorIndex(), "ctrl+p should move up")
}

// ---------------------------------------------------------------------------
// Key handling tests
// ---------------------------------------------------------------------------

func TestEscapeEmitsToggleMsg(t *testing.T) {
	ff := newTestFinder(newTestItems())

	_, cmd := ff.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(panels.ToggleFuzzyFinderMsg)
	assert.True(t, ok, "escape should emit ToggleFuzzyFinderMsg")
}

func TestEnterWithNoResultsIsNoOp(t *testing.T) {
	ff := newTestFinder([]Item{})

	_, cmd := ff.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd, "enter with no results should do nothing")
}

func TestEnterSelectsCurrentItem(t *testing.T) {
	ff := newTestFinder(newTestItems())

	// Select first item (main.go).
	item := ff.selectedItem()
	require.NotNil(t, item)
	assert.Equal(t, "main.go", item.Text)
	assert.Equal(t, "file", item.Category)

	// Navigate down and verify selected changes.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	item = ff.selectedItem()
	require.NotNil(t, item)
	assert.Equal(t, "README.md", item.Text)
}

func TestBackspaceDeletesCharacter(t *testing.T) {
	ff := newTestFinder(newTestItems())

	// Type "abc".
	ff.Update(textKeyMsg("a"))
	ff.Update(textKeyMsg("b"))
	ff.Update(textKeyMsg("c"))
	assert.Equal(t, "abc", ff.queryValue())

	// Backspace removes last character.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "ab", ff.queryValue())

	// Another backspace.
	ff.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "a", ff.queryValue())
}

func TestBackspaceAtEmptyIsNoOp(t *testing.T) {
	ff := newTestFinder(newTestItems())

	assert.Equal(t, "", ff.queryValue())
	ff.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "", ff.queryValue())
}

func TestCtrlUClearsQuery(t *testing.T) {
	ff := newTestFinder(newTestItems())

	ff.Update(textKeyMsg("test"))
	assert.NotEmpty(t, ff.queryValue())

	ff.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Equal(t, "", ff.queryValue())
	assert.Equal(t, 5, ff.matchCount(), "should show all items after clearing")
}

// ---------------------------------------------------------------------------
// Command selection tests
// ---------------------------------------------------------------------------

func TestCommandSourceSelectReturnsCommandItem(t *testing.T) {
	bindings := []keymap.Binding{
		{Key: "ctrl+c", Action: "quit", Description: "Quit grut", Mode: keymap.ModeGlobal},
		{Key: "j", Action: "cursor_down", Description: "Move down", Mode: keymap.ModePanel},
	}

	ff := New(NewCommandSource(bindings))

	item := ff.selectedItem()
	require.NotNil(t, item)
	assert.Equal(t, "command", item.Category)
	assert.Equal(t, "quit", item.Value)
}

// ---------------------------------------------------------------------------
// View tests
// ---------------------------------------------------------------------------

func TestViewRendersWithContent(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	view := ff.View(60, 20)
	assert.NotEmpty(t, view, "view should render content")
	assert.Contains(t, view, "5/5", "status should show match count")
}

func TestViewEmptyResults(t *testing.T) {
	ff := newTestFinder([]Item{})
	ff.SetSize(60, 20)

	view := ff.View(60, 20)
	assert.NotEmpty(t, view, "view should render even with no items")
	assert.Contains(t, view, "0/0", "status should show 0 matches")
}

func TestViewZeroDimensions(t *testing.T) {
	ff := newTestFinder(newTestItems())

	assert.Empty(t, ff.View(0, 0))
	assert.Empty(t, ff.View(-1, 10))
	assert.Empty(t, ff.View(10, 0))
}

func TestViewShowsSearchPrompt(t *testing.T) {
	ff := newTestFinder(newTestItems())

	view := ff.View(60, 20)
	assert.Contains(t, view, ">", "should show prompt character")
}

func TestViewAfterFilter(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.Update(textKeyMsg("mod"))
	ff.SetSize(60, 20)

	view := ff.View(60, 20)
	assert.Contains(t, view, "mod", "filtered query should appear in results")
}

// ---------------------------------------------------------------------------
// Panel interface tests
// ---------------------------------------------------------------------------

func TestPanelInterface(t *testing.T) {
	ff := newTestFinder(newTestItems())

	assert.Equal(t, "fuzzyfinder", ff.Title())
	assert.NotNil(t, ff.KeyBindings())
	assert.Len(t, ff.KeyBindings(), 4)

	// Focus/Blur should work via embedded BasePanel.
	ff.Focus()
	assert.True(t, ff.Focused)
	ff.Blur()
	assert.False(t, ff.Focused)
}

func TestInitReturnsNil(t *testing.T) {
	ff := newTestFinder(newTestItems())
	cmd := ff.Init(context.Background())
	assert.Nil(t, cmd)
}

func TestSetSizeUpdatesFields(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(80, 24)
	assert.Equal(t, 80, ff.Width)
	assert.Equal(t, 24, ff.Height)
}

// ---------------------------------------------------------------------------
// Mouse click tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsResult(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	assert.Equal(t, 0, ff.cursorIndex())

	// Results start at row 2 (after input + separator).
	// Click on row 3 → result index 1.
	ff.Update(panels.PanelMouseClickMsg{ContentRow: 3, ContentCol: 5})
	assert.Equal(t, 1, ff.cursorIndex())

	// Click on row 4 → result index 2.
	ff.Update(panels.PanelMouseClickMsg{ContentRow: 4, ContentCol: 5})
	assert.Equal(t, 2, ff.cursorIndex())
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	ff.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, ff.cursorIndex(), "out-of-bounds click should not move cursor")
}

func TestMouseClick_OnInputRowIgnored(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	// Row 0 is the input line, row 1 is separator — both should be ignored.
	ff.Update(panels.PanelMouseClickMsg{ContentRow: 0, ContentCol: 5})
	assert.Equal(t, 0, ff.cursorIndex(), "clicking input row should not change cursor")

	ff.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, ff.cursorIndex(), "clicking separator row should not change cursor")
}

// ---------------------------------------------------------------------------
// Mouse double-click tests
// ---------------------------------------------------------------------------

func TestMouseDoubleClick_ConfirmsSelection(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	// Double-click on row 2 → result index 0 (first result).
	_, cmd := ff.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 0, ff.cursorIndex())
	require.NotNil(t, cmd, "double-click should trigger selection")
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	ff := newTestFinder(newTestItems())
	ff.SetSize(60, 20)

	_, cmd := ff.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "out-of-bounds double-click should not trigger action")
}

// ---------------------------------------------------------------------------
// itemList (fuzzy.Source adapter) tests
// ---------------------------------------------------------------------------

func TestItemListImplementsFuzzySource(t *testing.T) {
	items := newTestItems()
	list := itemList(items)

	assert.Equal(t, len(items), list.Len())
	assert.Equal(t, "main.go", list.String(0))
	assert.Equal(t, "README.md", list.String(1))
}

// ---------------------------------------------------------------------------
// DirectorySource: all skip directories are excluded
// ---------------------------------------------------------------------------

func TestDirectorySourceSkipsAllNonNavigable(t *testing.T) {
	skipDirs := []string{"vendor", "__pycache__", "dist", "build", ".next", ".git"}

	for _, name := range skipDirs {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			// Create the skip directory with a child.
			child := filepath.Join(root, name, "subdir")
			require.NoError(t, os.MkdirAll(child, 0o755))
			// Create a normal directory.
			require.NoError(t, os.Mkdir(filepath.Join(root, "keepme"), 0o755))

			ds := NewDirectorySource(root, DefaultDirectorySourceMaxDepth)
			items := ds.Items()

			texts := make([]string, 0, len(items))
			for _, it := range items {
				texts = append(texts, it.Text)
			}
			assert.Contains(t, texts, "keepme", "normal directory should be included")
			assert.NotContains(t, texts, name, "skip directory %q should be excluded", name)
			assert.NotContains(t, texts, "subdir", "child of %q should be excluded", name)
		})
	}
}

// ---------------------------------------------------------------------------
// .gitignore filtering tests
// ---------------------------------------------------------------------------

func TestFileSourceRespectsGitignore(t *testing.T) {
	// Invalidate cache so prior tests don't interfere.
	InvalidateFileCache()

	dir := t.TempDir()

	// Create .gitignore that ignores *.log and the tmp/ directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\ntmp/\n"), 0o644))

	// Create files: one kept, one ignored by pattern, one inside ignored dir.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "debug.log"), []byte("l"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tmp"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tmp", "scratch.txt"), []byte("s"), 0o644))
	// Also add a kept subdirectory with a file.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("a"), 0o644))

	src := NewFileSource(dir)
	items := src.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "main.go")
	assert.Contains(t, texts, "src/app.go")
	assert.NotContains(t, texts, "debug.log", "*.log should be excluded by .gitignore")
	assert.NotContains(t, texts, "tmp/scratch.txt", "tmp/ dir should be excluded by .gitignore")
	// .gitignore itself starts with "." so it is filtered as a hidden file.

	// Invalidate for next tests.
	InvalidateFileCache()
}

func TestFileSourceFallsBackWithoutGitignore(t *testing.T) {
	// Invalidate cache so prior tests don't interfere.
	InvalidateFileCache()

	dir := t.TempDir()

	// No .gitignore — hidden files/dirs should still be filtered.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("e"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("c"), 0o644))

	src := NewFileSource(dir)
	items := src.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "app.go")
	assert.NotContains(t, texts, ".env", "hidden files should be excluded")
	assert.NotContains(t, texts, ".git/config", ".git directory should be excluded")

	InvalidateFileCache()
}

func TestDirectorySourceRespectsGitignore(t *testing.T) {
	dir := t.TempDir()

	// Create .gitignore that ignores build/ directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "build", "out"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))

	ds := NewDirectorySource(dir, DefaultDirectorySourceMaxDepth)
	items := ds.Items()

	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	assert.Contains(t, texts, "src")
	assert.Contains(t, texts, "docs")
	assert.NotContains(t, texts, "build", "build/ should be excluded by .gitignore")
	assert.NotContains(t, texts, "build/out", "children of build/ should be excluded")
}

// ---------------------------------------------------------------------------
// Cache tests
// ---------------------------------------------------------------------------

func TestFileSourceCacheReturnsWithoutReWalk(t *testing.T) {
	// Invalidate cache so prior tests don't interfere.
	InvalidateFileCache()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("b"), 0o644))

	src := NewFileSource(dir)

	// First call — populates cache.
	items1 := src.Items()
	require.Len(t, items1, 2)

	// Add a new file to the filesystem — cache should still return the old result.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.go"), []byte("c"), 0o644))

	items2 := src.Items()
	assert.Equal(t, len(items1), len(items2), "second call should return cached items (no re-walk)")

	// After invalidation, the new file should appear.
	InvalidateFileCache()
	items3 := src.Items()
	assert.Equal(t, 3, len(items3), "after invalidation, new file should appear")

	InvalidateFileCache()
}

func TestInvalidateFileCacheForcesFreshWalk(t *testing.T) {
	InvalidateFileCache()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "only.go"), []byte("x"), 0o644))

	src := NewFileSource(dir)
	items := src.Items()
	require.Len(t, items, 1)

	// Invalidate and verify cache state via a second walk.
	InvalidateFileCache()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.go"), []byte("y"), 0o644))

	items2 := src.Items()
	assert.Len(t, items2, 2, "cache invalidation should cause a fresh walk")

	InvalidateFileCache()
}

// ---------------------------------------------------------------------------
// Filename match boost (rerank) tests
// ---------------------------------------------------------------------------

func TestFilenameBoostPrefixWins(t *testing.T) {
	items := []Item{
		{Text: "internal/domain/maintain.go", Category: "file", Value: "internal/domain/maintain.go"},
		{Text: "src/main.go", Category: "file", Value: "src/main.go"},
		{Text: "main.go", Category: "file", Value: "main.go"},
		{Text: "README.md", Category: "file", Value: "README.md"},
	}
	ff := newTestFinder(items)

	// Type "main" to filter.
	for _, ch := range "main" {
		ff.Update(textKeyMsg(string(ch)))
	}

	require.True(t, ff.matchCount() >= 2, "should have at least 2 matches for 'main'")

	// The first match should be main.go or src/main.go (both are prefix
	// matches on the filename).
	first := ff.items[ff.matches[0].Index]
	assert.Contains(t, []string{"main.go", "src/main.go"}, first.Text,
		"prefix filename match should rank first, got %q", first.Text)

	// maintain.go should rank below main.go entries.
	for i, m := range ff.matches {
		item := ff.items[m.Index]
		if item.Text == "internal/domain/maintain.go" {
			// Every main.go entry should be before this one.
			for j := 0; j < i; j++ {
				prev := ff.items[ff.matches[j].Index]
				assert.True(t, strings.HasSuffix(prev.Text, "main.go"),
					"main.go entries should rank above maintain.go, but index %d has %q", j, prev.Text)
			}
		}
	}
}

func TestFilenameBoostSubsequenceOverNoMatch(t *testing.T) {
	items := []Item{
		{Text: "docs/something/config.go", Category: "file", Value: "docs/something/config.go"},
		{Text: "cmd/cfg.go", Category: "file", Value: "cmd/cfg.go"},
	}
	ff := newTestFinder(items)

	// Type "cfg" — should match cfg.go (prefix) and config.go (subsequence).
	for _, ch := range "cfg" {
		ff.Update(textKeyMsg(string(ch)))
	}

	if ff.matchCount() >= 2 {
		first := ff.items[ff.matches[0].Index]
		assert.Equal(t, "cmd/cfg.go", first.Text,
			"prefix match cfg.go should rank above subsequence match config.go")
	}
}

func TestRerankPreservesOrderForNonFileItems(t *testing.T) {
	items := []Item{
		{Text: "quit", Category: "command", Value: "quit"},
		{Text: "quite-long-name", Category: "command", Value: "quite-long-name"},
	}
	ff := newTestFinder(items)

	for _, ch := range "quit" {
		ff.Update(textKeyMsg(string(ch)))
	}

	// Both should match; "quit" has exact prefix.
	if ff.matchCount() >= 2 {
		first := ff.items[ff.matches[0].Index]
		assert.Equal(t, "quit", first.Text,
			"exact prefix match should rank first")
	}
}
