package fuzzyfinder

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
	"github.com/sahilm/fuzzy"
)

// FuzzyFinder is an overlay panel for fuzzy searching files and commands.
// It implements [panels.Panel] and is rendered as a floating overlay on
// top of the main layout by the root model.
type FuzzyFinder struct {
	query   string // current search query
	sources []Source
	items   []Item        // all items from all sources
	matches []fuzzy.Match // filtered results
	panels.BasePanel
	defaultCategories map[string]bool
	activeSourceLabel string
	cursor            int // index into matches
	offset            int // scroll offset for results
	qCursor           int // cursor position in query
	theme             *theme.Theme
	promptStyle       lipgloss.Style
	placeholderStyle  lipgloss.Style
	matchHighlight    lipgloss.Style
	descStyle         lipgloss.Style
	statusStyle       lipgloss.Style
	separatorStyle    lipgloss.Style
	cursorBg          string
}

// Compile-time interface check.
var _ panels.Panel = (*FuzzyFinder)(nil)

// New creates a new FuzzyFinder with the given sources. Items are loaded
// eagerly from all sources at construction time.
func New(th *theme.Theme, sources ...Source) *FuzzyFinder {
	return NewWithDefaultCategories(th, nil, sources...)
}

// NewWithDefaultCategories creates a FuzzyFinder whose no-prefix query is
// limited to the given categories. An empty default category list shows all
// source items when no source prefix is active.
func NewWithDefaultCategories(th *theme.Theme, defaultCategories []string, sources ...Source) *FuzzyFinder {
	tc := theme.Colors{}
	if th != nil {
		tc = th.Colors
	}
	ff := &FuzzyFinder{
		BasePanel:        panels.BasePanel{PanelTitle: "fuzzyfinder"},
		sources:          sources,
		theme:            th,
		promptStyle:      lipgloss.NewStyle().Foreground(panels.ColorOf(tc.NormalGreen, "#6B9E56")).Bold(true),
		placeholderStyle: lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		matchHighlight:   lipgloss.NewStyle().Foreground(panels.ColorOf(tc.NormalYellow, "#C9A227")).Bold(true),
		descStyle:        lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		statusStyle:      lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		separatorStyle:   lipgloss.NewStyle().Foreground(panels.ColorOf(tc.SelectionBg, "#2A2A2A")),
		cursorBg:         panels.OrDefault(tc.SelectionBg, "#2A2A2A"),
	}
	if len(defaultCategories) > 0 {
		ff.defaultCategories = categorySet(defaultCategories...)
		ff.activeSourceLabel = sourceLabelForCategories(ff.defaultCategories)
	}
	ff.loadItems()
	ff.filter()
	return ff
}

// loadItems gathers all items from all sources.
func (ff *FuzzyFinder) loadItems() {
	ff.items = ff.items[:0]
	for _, src := range ff.sources {
		ff.items = append(ff.items, src.Items()...)
	}
}

// filter runs fuzzy matching against the current query. When the query
// is empty, all items are shown in their original order. When there are
// matches, they are re-ranked so that filename-level matches sort higher.
func (ff *FuzzyFinder) filter() {
	search, categories, label := ff.parseSourceFilter()
	ff.activeSourceLabel = label
	candidateItems, candidateIndexes := ff.filteredItems(categories)
	if search == "" {
		// Show all items when query is empty.
		ff.matches = make([]fuzzy.Match, len(candidateItems))
		for i, item := range candidateItems {
			ff.matches[i] = fuzzy.Match{
				Str:   item.Text,
				Index: candidateIndexes[i],
			}
		}
	} else {
		ff.matches = fuzzy.FindFrom(search, itemList(candidateItems))
		for i := range ff.matches {
			ff.matches[i].Index = candidateIndexes[ff.matches[i].Index]
		}
		rerank(search, ff.matches)
	}
	// Reset cursor and offset after filtering.
	ff.cursor = 0
	ff.offset = 0
}

func (ff *FuzzyFinder) filteredItems(categories map[string]bool) ([]Item, []int) {
	if len(categories) == 0 {
		indexes := make([]int, len(ff.items))
		for i := range ff.items {
			indexes[i] = i
		}
		return ff.items, indexes
	}
	items := make([]Item, 0, len(ff.items))
	indexes := make([]int, 0, len(ff.items))
	for i, item := range ff.items {
		if categories[item.Category] {
			items = append(items, item)
			indexes = append(indexes, i)
		}
	}
	return items, indexes
}

func (ff *FuzzyFinder) parseSourceFilter() (string, map[string]bool, string) {
	if len(ff.query) >= 2 && ff.query[1] == ':' {
		if categories, label, ok := categoriesForPrefix(ff.query[0]); ok {
			return ff.query[2:], categories, label
		}
	}
	return ff.query, ff.defaultCategories, sourceLabelForCategories(ff.defaultCategories)
}

func categoriesForPrefix(prefix byte) (map[string]bool, string, bool) {
	switch prefix {
	case 'f', 'F':
		return categorySet(categoryFile), sourceNameFiles, true
	case 'd', 'D':
		return categorySet(categoryDirectory), sourceNameDirectories, true
	case 'c', 'C':
		return categorySet(categoryCommand), sourceNameCommands, true
	case 'b', 'B':
		return categorySet(categoryBookmark), sourceNameBookmarks, true
	case 'g', 'G':
		return categorySet(categoryGitChanged), sourceNameGitChanged, true
	default:
		return nil, "", false
	}
}

func categorySet(categories ...string) map[string]bool {
	set := make(map[string]bool, len(categories))
	for _, category := range categories {
		set[category] = true
	}
	return set
}

func sourceLabelForCategories(categories map[string]bool) string {
	if len(categories) != 1 {
		return ""
	}
	switch {
	case categories[categoryFile]:
		return sourceNameFiles
	case categories[categoryDirectory]:
		return sourceNameDirectories
	case categories[categoryCommand]:
		return sourceNameCommands
	case categories[categoryBookmark]:
		return sourceNameBookmarks
	case categories[categoryGitChanged]:
		return sourceNameGitChanged
	default:
		return ""
	}
}

// rerank re-sorts fuzzy matches so that filename-level hits rank higher
// than deep path matches. It assigns bonus scores and uses the original
// fuzzy score as a tiebreaker.
//
// Scoring:
//   - +200 if the query is a case-insensitive prefix of the filename
//   - +100 if all query characters appear in the filename (subsequence)
//   - original fuzzy score is preserved as the secondary sort key
func rerank(query string, matches []fuzzy.Match) {
	if len(matches) == 0 {
		return
	}
	lowerQ := strings.ToLower(query)
	type scored struct {
		match fuzzy.Match
		bonus int
		orig  int // original index for stability
	}
	entries := make([]scored, len(matches))
	for i, m := range matches {
		filename := strings.ToLower(path.Base(m.Str))
		bonus := 0
		if strings.HasPrefix(filename, lowerQ) {
			bonus = 200
		} else if subsequenceMatch(lowerQ, filename) {
			bonus = 100
		}
		entries[i] = scored{match: m, bonus: bonus, orig: i}
	}
	slices.SortStableFunc(entries, func(a, b scored) int {
		// Higher bonus wins (descending).
		if c := cmp.Compare(b.bonus, a.bonus); c != 0 {
			return c
		}
		// Higher fuzzy score wins (descending).
		if c := cmp.Compare(b.match.Score, a.match.Score); c != 0 {
			return c
		}
		// Preserve original order as final tiebreaker (ascending).
		return cmp.Compare(a.orig, b.orig)
	})
	for i, e := range entries {
		matches[i] = e.match
	}
}

// subsequenceMatch returns true when every character in query appears, in
// order, within target. Both strings should already be lowercased.
func subsequenceMatch(query, target string) bool {
	qi := 0
	for _, ch := range target {
		if qi < len(query) && byte(ch) == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

// itemList wraps []Item to implement the fuzzy.Source interface
// required by sahilm/fuzzy.
type itemList []Item

func (l itemList) String(i int) string { return l[i].Text }
func (l itemList) Len() int            { return len(l) }

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (ff *FuzzyFinder) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements panels.Panel.
func (ff *FuzzyFinder) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return ff.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return ff.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return ff.handleMouseDoubleClick(msg)
	}
	return ff, nil
}

// View implements panels.Panel. It renders the fuzzy finder content
// including the search input, results list with match highlighting,
// and a status bar showing match counts.
func (ff *FuzzyFinder) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var lines []string
	// Input line: "> query"
	inputLine := ff.renderInput(width)
	lines = append(lines, inputLine)
	// Separator
	sep := ff.separatorStyle.Render(strings.Repeat("─", width))
	lines = append(lines, sep)
	// Results area: total height minus input (1) + separator (1) + status (1).
	resultH := height - 3
	if resultH < 1 {
		resultH = 1
	}
	end := ff.offset + resultH
	if end > len(ff.matches) {
		end = len(ff.matches)
	}
	for i := ff.offset; i < end; i++ {
		match := ff.matches[i]
		item := ff.items[match.Index]
		line := ff.renderMatch(item, match.MatchedIndexes, width, i == ff.cursor)
		lines = append(lines, line)
	}
	// Pad remaining result area with blank lines.
	rendered := end - ff.offset
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for rendered < resultH {
		lines = append(lines, emptyLine)
		rendered++
	}
	// Status bar
	status := fmt.Sprintf(" %d/%d", len(ff.matches), len(ff.items))
	if ff.activeSourceLabel != "" {
		status = fmt.Sprintf(" %s · %d/%d", ff.activeSourceLabel, len(ff.matches), len(ff.items))
	}
	statusLine := ff.statusStyle.Width(width).Render(status)
	lines = append(lines, statusLine)
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (ff *FuzzyFinder) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "↑/ctrl+p", Description: "Previous result", Action: "cursor_up"},
		{Key: "↓/ctrl+n", Description: "Next result", Action: actionCursorDown},
		{Key: "enter", Description: "Select result", Action: "select"},
		{Key: "escape", Description: "Close", Action: "close"},
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (ff *FuzzyFinder) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "escape", "esc":
		return ff, func() tea.Msg { return panels.ToggleFuzzyFinderMsg{} }
	case "enter":
		return ff, ff.selectCurrent()
	case "up", "ctrl+p":
		ff.moveCursorUp()
		return ff, nil
	case "down", "ctrl+n":
		ff.moveCursorDown()
		return ff, nil
	case "backspace":
		if ff.qCursor > 0 {
			runes := []rune(ff.query)
			ff.query = string(runes[:ff.qCursor-1]) + string(runes[ff.qCursor:])
			ff.qCursor--
			ff.filter()
		}
		return ff, nil
	case "ctrl+u":
		ff.query = ""
		ff.qCursor = 0
		ff.filter()
		return ff, nil
	default:
		// Insert printable text.
		text := msg.Text
		if text != "" {
			runes := []rune(ff.query)
			newRunes := make([]rune, 0, len(runes)+len([]rune(text)))
			newRunes = append(newRunes, runes[:ff.qCursor]...)
			newRunes = append(newRunes, []rune(text)...)
			newRunes = append(newRunes, runes[ff.qCursor:]...)
			ff.query = string(newRunes)
			ff.qCursor += len([]rune(text))
			ff.filter()
		}
		return ff, nil
	}
}

// selectCurrent emits messages for the currently selected match.
// It sends ToggleFuzzyFinderMsg to close the overlay, plus either
// FileSelectedMsg or CommandSelectedMsg based on the item category.
func (ff *FuzzyFinder) selectCurrent() tea.Cmd {
	if len(ff.matches) == 0 || ff.cursor >= len(ff.matches) {
		return nil
	}
	match := ff.matches[ff.cursor]
	item := ff.items[match.Index]
	return tea.Batch(
		func() tea.Msg { return panels.ToggleFuzzyFinderMsg{} },
		func() tea.Msg {
			switch item.Category {
			case categoryCommand:
				action, ok := item.Value.(string)
				if !ok {
					return nil
				}
				return panels.CommandSelectedMsg{Action: action}
			default:
				path, ok := item.Value.(string)
				if !ok {
					return nil
				}
				return panels.FileSelectedMsg{Path: path}
			}
		},
		func() tea.Msg {
			if item.Category != categoryCommand {
				path, ok := item.Value.(string)
				if !ok {
					return nil
				}
				return panels.RevealFileMsg{Path: path}
			}
			return nil
		},
	)
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick selects the result at the clicked row.
// Row 0 is the input line, row 1 is the separator, so results start at row 2.
func (ff *FuzzyFinder) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	// Results begin after input (row 0) + separator (row 1).
	idx := ff.offset + msg.ContentRow - 2
	if idx < 0 || idx >= len(ff.matches) {
		return ff, nil
	}
	ff.cursor = idx
	ff.ensureCursorVisible()
	return ff, nil
}

// handleMouseDoubleClick confirms the selection at the double-clicked row.
func (ff *FuzzyFinder) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := ff.offset + msg.ContentRow - 2
	if idx < 0 || idx >= len(ff.matches) {
		return ff, nil
	}
	ff.cursor = idx
	ff.ensureCursorVisible()
	return ff, ff.selectCurrent()
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (ff *FuzzyFinder) moveCursorUp() {
	if ff.cursor > 0 {
		ff.cursor--
		ff.ensureCursorVisible()
	}
}

func (ff *FuzzyFinder) moveCursorDown() {
	if ff.cursor < len(ff.matches)-1 {
		ff.cursor++
		ff.ensureCursorVisible()
	}
}

func (ff *FuzzyFinder) ensureCursorVisible() {
	resultH := ff.Height - 3
	if resultH < 1 {
		resultH = 1
	}
	if ff.cursor < ff.offset {
		ff.offset = ff.cursor
	}
	if ff.cursor >= ff.offset+resultH {
		ff.offset = ff.cursor - resultH + 1
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (ff *FuzzyFinder) renderInput(width int) string {
	prompt := ff.promptStyle.Render("> ")
	display := ff.query
	if display == "" {
		display = ff.placeholderStyle.Render("Search...")
	}
	line := prompt + display
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)
}

func (ff *FuzzyFinder) renderMatch(item Item, matchedIndexes []int, width int, isCursor bool) string {
	// Build a set of matched character positions for O(1) lookup.
	matchSet := make(map[int]bool, len(matchedIndexes))
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}
	var b strings.Builder
	b.WriteString("  ") // indent
	for i, ch := range item.Text {
		if matchSet[i] {
			b.WriteString(ff.matchHighlight.Render(string(ch)))
		} else {
			b.WriteRune(ch)
		}
	}
	if item.Description != "" {
		b.WriteString("  ")
		b.WriteString(ff.descStyle.Render(item.Description))
	}
	content := b.String()
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isCursor {
		style = style.Background(lipgloss.Color(ff.cursorBg)).Bold(true)
	}
	return style.Render(content)
}

// Styles — matching the Forge palette used throughout grut.

// ---------------------------------------------------------------------------
// Test-only accessors (unexported; tests are in the same package)
// ---------------------------------------------------------------------------
// matchCount returns the number of current matches.
func (ff *FuzzyFinder) matchCount() int { return len(ff.matches) }

// cursorIndex returns the current cursor position.
func (ff *FuzzyFinder) cursorIndex() int { return ff.cursor }

// selectedItem returns the currently selected item, or nil if no matches.
func (ff *FuzzyFinder) selectedItem() *Item {
	if len(ff.matches) == 0 || ff.cursor >= len(ff.matches) {
		return nil
	}
	item := ff.items[ff.matches[ff.cursor].Index]
	return &item
}

// queryValue returns the current search query (for testing).
func (ff *FuzzyFinder) queryValue() string { return ff.query }
