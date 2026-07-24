// Package textsearch implements a repository-wide text search overlay.
package textsearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/proctree"
	"github.com/jongio/grut/internal/theme"
)

const (
	DefaultMaxResults = 200
	maxSnippetRunes   = 120
)

// Result is one git-grep match.
type Result struct {
	Path    string
	Snippet string
	Line    int
}

type searchDoneMsg struct {
	err     error
	results []Result
	omitted bool
}

// TextSearch is an overlay panel for repository-wide text search.
type TextSearch struct {
	searchFn func(context.Context, string, int) ([]Result, bool, error)
	query    string
	results  []Result
	lastRun  string
	status   string
	root     string
	cursor   int
	offset   int
	qCursor  int
	max      int
	omitted  bool
	panels.BasePanel
	promptStyle      lipgloss.Style
	placeholderStyle lipgloss.Style
	descStyle        lipgloss.Style
	statusStyle      lipgloss.Style
	separatorStyle   lipgloss.Style
	cursorBg         string
}

var _ panels.Panel = (*TextSearch)(nil)

// New creates a text search overlay rooted at root.
func New(root string, th *theme.Theme) *TextSearch {
	tc := theme.Colors{}
	if th != nil {
		tc = th.Colors
	}
	ts := &TextSearch{
		BasePanel:        panels.BasePanel{PanelTitle: "repo text search"},
		root:             root,
		max:              DefaultMaxResults,
		promptStyle:      lipgloss.NewStyle().Foreground(panels.ColorOf(tc.NormalGreen, "#6B9E56")).Bold(true),
		placeholderStyle: lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		descStyle:        lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		statusStyle:      lipgloss.NewStyle().Foreground(panels.ColorOf(tc.BrightBlack, "#555555")),
		separatorStyle:   lipgloss.NewStyle().Foreground(panels.ColorOf(tc.SelectionBg, "#2A2A2A")),
		cursorBg:         panels.OrDefault(tc.SelectionBg, "#2A2A2A"),
	}
	ts.searchFn = ts.runSearch
	ts.status = "Type a query, then press Enter to search"
	return ts
}

// Init implements panels.Panel.
func (ts *TextSearch) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements panels.Panel.
func (ts *TextSearch) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return ts.handleKey(msg)
	case searchDoneMsg:
		ts.results = msg.results
		ts.omitted = msg.omitted
		ts.cursor = 0
		ts.offset = 0
		if msg.err != nil {
			ts.status = "Search failed: " + msg.err.Error()
		} else {
			ts.status = fmt.Sprintf("%d results", len(ts.results))
			if ts.omitted {
				ts.status += " (more results omitted)"
			}
		}
		return ts, nil
	case panels.PanelMouseClickMsg:
		return ts.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return ts.handleMouseDoubleClick(msg)
	}
	return ts, nil
}

// View implements panels.Panel.
func (ts *TextSearch) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{ts.renderInput(width), ts.separatorStyle.Render(strings.Repeat("─", width))}
	resultH := height - 3
	if resultH < 1 {
		resultH = 1
	}
	end := min(ts.offset+resultH, len(ts.results))
	for i := ts.offset; i < end; i++ {
		lines = append(lines, ts.renderResult(ts.results[i], width, i == ts.cursor))
	}
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for rendered := end - ts.offset; rendered < resultH; rendered++ {
		lines = append(lines, emptyLine)
	}
	lines = append(lines, ts.statusStyle.Width(width).Render(" "+ts.status))
	return strings.Join(lines, "\n")
}

// KeyBindings implements panels.Panel.
func (ts *TextSearch) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "enter", Description: "Search or select result", Action: "search_select"},
		{Key: "↑/ctrl+p", Description: "Previous result", Action: "cursor_up"},
		{Key: "↓/ctrl+n", Description: "Next result", Action: "cursor_down"},
		{Key: "escape", Description: "Close", Action: "close"},
	}
}

func (ts *TextSearch) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "escape", "esc":
		return ts, func() tea.Msg { return panels.ToggleTextSearchMsg{} }
	case "enter":
		if ts.query != "" && ts.query == ts.lastRun && len(ts.results) > 0 {
			return ts, ts.selectCurrent()
		}
		return ts, ts.search()
	case "up", "ctrl+p":
		ts.moveCursorUp()
		return ts, nil
	case "down", "ctrl+n":
		ts.moveCursorDown()
		return ts, nil
	case "backspace":
		if ts.qCursor > 0 {
			runes := []rune(ts.query)
			ts.query = string(runes[:ts.qCursor-1]) + string(runes[ts.qCursor:])
			ts.qCursor--
			ts.results = nil
			ts.omitted = false
			ts.status = "Press Enter to search"
		}
		return ts, nil
	case "ctrl+u":
		ts.query = ""
		ts.qCursor = 0
		ts.results = nil
		ts.omitted = false
		ts.status = "Type a query, then press Enter to search"
		return ts, nil
	default:
		if msg.Text != "" {
			runes := []rune(ts.query)
			textRunes := []rune(msg.Text)
			newRunes := make([]rune, 0, len(runes)+len(textRunes))
			newRunes = append(newRunes, runes[:ts.qCursor]...)
			newRunes = append(newRunes, textRunes...)
			newRunes = append(newRunes, runes[ts.qCursor:]...)
			ts.query = string(newRunes)
			ts.qCursor += len(textRunes)
			ts.results = nil
			ts.omitted = false
			ts.status = "Press Enter to search"
		}
		return ts, nil
	}
}

func (ts *TextSearch) search() tea.Cmd {
	query := strings.TrimSpace(ts.query)
	if query == "" {
		ts.status = "Enter a search query"
		return nil
	}
	ts.lastRun = query
	ts.status = "Searching..."
	return func() tea.Msg {
		results, omitted, err := ts.searchFn(context.Background(), query, ts.max)
		return searchDoneMsg{results: results, omitted: omitted, err: err}
	}
}

func (ts *TextSearch) selectCurrent() tea.Cmd {
	if len(ts.results) == 0 || ts.cursor >= len(ts.results) {
		return nil
	}
	result := ts.results[ts.cursor]
	return func() tea.Msg {
		return panels.FileSelectedMsg{Path: result.Path, Line: result.Line}
	}
}

func (ts *TextSearch) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	idx := ts.offset + msg.ContentRow - 2
	if idx < 0 || idx >= len(ts.results) {
		return ts, nil
	}
	ts.cursor = idx
	ts.ensureCursorVisible()
	return ts, nil
}

func (ts *TextSearch) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	idx := ts.offset + msg.ContentRow - 2
	if idx < 0 || idx >= len(ts.results) {
		return ts, nil
	}
	ts.cursor = idx
	ts.ensureCursorVisible()
	return ts, ts.selectCurrent()
}

func (ts *TextSearch) moveCursorUp() {
	if ts.cursor > 0 {
		ts.cursor--
		ts.ensureCursorVisible()
	}
}

func (ts *TextSearch) moveCursorDown() {
	if ts.cursor < len(ts.results)-1 {
		ts.cursor++
		ts.ensureCursorVisible()
	}
}

func (ts *TextSearch) ensureCursorVisible() {
	resultH := ts.Height - 3
	if resultH < 1 {
		resultH = 1
	}
	ts.offset = panels.EnsureCursorVisible(ts.cursor, ts.offset, resultH)
}

func (ts *TextSearch) renderInput(width int) string {
	display := ts.query
	if display == "" {
		display = ts.placeholderStyle.Render("Search repository text...")
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(ts.promptStyle.Render("> ") + display)
}

func (ts *TextSearch) renderResult(result Result, width int, isCursor bool) string {
	content := fmt.Sprintf("  %s:%d:%s", result.Path, result.Line, ts.descStyle.Render(result.Snippet))
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isCursor {
		style = style.Background(lipgloss.Color(ts.cursorBg)).Bold(true)
	}
	return style.Render(content)
}

func (ts *TextSearch) runSearch(ctx context.Context, query string, maxResults int) ([]Result, bool, error) {
	cmd := proctree.Command(ctx, "git", "grep", "-n", "-I", "--", query)
	cmd.Dir = ts.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := proctree.Run(cmd)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, false, nil
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, false, errors.New(errMsg)
		}
		return nil, false, err
	}
	return capResults(parseGrepOutput(stdout.String()), maxResults)
}

func parseGrepOutput(raw string) []Result {
	lines := strings.Split(raw, "\n")
	results := make([]Result, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		result, ok := parseGrepLine(line)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}

func parseGrepLine(line string) (Result, bool) {
	for i := 0; i < len(line); i++ {
		if line[i] != ':' || isWindowsDriveColon(line, i) {
			continue
		}
		next := strings.IndexByte(line[i+1:], ':')
		if next < 0 {
			return Result{}, false
		}
		lineEnd := i + 1 + next
		lineNo, err := strconv.Atoi(line[i+1 : lineEnd])
		if err != nil || lineNo < 1 {
			continue
		}
		return Result{
			Path:    line[:i],
			Line:    lineNo,
			Snippet: capSnippet(line[lineEnd+1:]),
		}, true
	}
	return Result{}, false
}

func isWindowsDriveColon(line string, idx int) bool {
	if idx != 1 || len(line) < 3 {
		return false
	}
	drive := line[0]
	return ((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) &&
		(line[2] == '\\' || line[2] == '/')
}

func capResults(results []Result, maxResults int) ([]Result, bool, error) {
	if maxResults <= 0 || len(results) <= maxResults {
		return results, false, nil
	}
	return results[:maxResults], true, nil
}

func capSnippet(snippet string) string {
	snippet = strings.TrimSpace(snippet)
	if utf8.RuneCountInString(snippet) <= maxSnippetRunes {
		return snippet
	}
	runes := []rune(snippet)
	return string(runes[:maxSnippetRunes-3]) + "..."
}
