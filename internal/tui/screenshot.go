//go:build screenshots

// screenshot.go provides programmatic screenshot capture for the web site.
//
// It drives the TUI model through each visual state and captures the
// rendered ANSI output without requiring the Bubble Tea event loop.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// mustModel safely converts a tea.Model to our Model type. In Bubble Tea
// the Update method always returns the same concrete type, so this assertion
// should never fail — but using the comma-ok pattern avoids a runtime panic
// if the contract is violated.
func mustModel(m tea.Model) Model {
	if v, ok := m.(Model); ok {
		return v
	}
	panic("screenshot: tea.Model is not tui.Model — Bubble Tea contract violated")
}

// Highlight defines a rectangular region to visually emphasise in a
// screenshot. Coordinates are in terminal cells (0-indexed col, row).
type Highlight struct {
	Row, Col   int // top-left corner (0-indexed)
	Rows, Cols int // size in terminal cells
}

// Screenshot holds a captured ANSI render of the TUI in a named state.
type Screenshot struct {
	Name       string      // file name without extension (e.g. "hero-main")
	SubDir     string      // theme subdirectory (e.g. "default", "catppuccin")
	FG         string      // theme foreground hex (e.g. "#E4E4E7")
	BG         string      // theme background hex (e.g. "#111111")
	Palette    [16]string  // ANSI palette: [0]=Black … [15]=BrightWhite
	ANSI       string      // rendered output with ANSI escape codes
	Highlights []Highlight // optional regions to emphasise
}

// CaptureScreenshots drives the TUI model through every visual state
// used on the website and returns the rendered ANSI output for each.
// It loops over all built-in themes, producing a complete set of
// screenshots per theme in separate subdirectories.
func CaptureScreenshots(width, height int) ([]Screenshot, error) {
	// Force TrueColor rendering even when stdout is not a terminal.
	os.Setenv("CLICOLOR_FORCE", "1")
	os.Setenv("COLORTERM", "truecolor")

	var allShots []Screenshot
	for _, themeName := range theme.BuiltinNames() {
		th, err := theme.Load(themeName)
		if err != nil {
			return nil, fmt.Errorf("loading theme %s: %w", themeName, err)
		}
		shots, err := captureThemeScreenshots(width, height, th)
		if err != nil {
			return nil, fmt.Errorf("theme %s: %w", themeName, err)
		}
		allShots = append(allShots, shots...)
	}
	return allShots, nil
}

// captureThemeScreenshots captures all visual states for a single theme.
func captureThemeScreenshots(width, height int, th *theme.Theme) ([]Screenshot, error) {
	var shots []Screenshot
	capture := func(name string, m *Model, highlights ...Highlight) {
		view := m.View()
		shots = append(shots, Screenshot{
			Name:       name,
			SubDir:     th.Name,
			FG:         th.Colors.Foreground,
			BG:         th.Colors.Background,
			Palette:    th.Colors.ANSIPalette(),
			ANSI:       view.Content,
			Highlights: highlights,
		})
	}

	// -- Layout presets (panels only, no overlays) -------------------------

	// hero-main: ExplorerPreset, filetree focused, nested file selected
	// to show the directory tree with expanded folders.
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		selectFileForScreenshot(m, "src/auth/jwt.go")
		capture("hero-main", m)
	}

	// file-explorer: ExplorerPreset, filetree focused
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		selectFileForScreenshot(m, "src/auth/jwt.go")
		capture("file-explorer", m)
	}

	// file-preview: ExplorerPreset, preview focused
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		selectFileForScreenshot(m, "src/auth/jwt.go")
		// Cycle focus to the preview panel.
		// Explorer layout: filetree → gitinfo → github → commits → preview.
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		capture("file-preview", m)
	}

	// git-status: FullPreset (has gitstatus panel)
	{
		m, err := newScreenshotModel(width, height, layout.FullPreset(), th)
		if err != nil {
			return nil, err
		}
		// Focus the gitstatus panel (second panel in full layout).
		m.engine.FocusNext()
		capture("git-status", m)
	}

	// git-log: Custom layout with gitlog dominant
	{
		preset := makeCustomPreset("gitlog-shot", "gitlog", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("git-log", m)
	}

	// git-branches: Custom layout with branches panel
	{
		preset := makeCustomPreset("branches-shot", "branches", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("git-branches", m)
	}

	// git-commits: ExplorerPreset (has commits panel)
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		selectFileForScreenshot(m, "README.md")
		// Focus the commits panel (4th in explorer layout: filetree → gitinfo → github → commits).
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		capture("git-commits", m)
	}

	// git-diff: Custom layout with gitdiff panel
	{
		preset := makeCustomPreset("gitdiff-shot", "gitdiff", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("git-diff", m)
	}

	// git-stash: Custom layout with stash panel
	{
		preset := makeCustomPreset("stash-shot", "stash", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("git-stash", m)
	}

	// git-worktrees: Custom layout with worktrees panel
	{
		preset := makeCustomPreset("worktrees-shot", "worktrees", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("git-worktrees", m)
	}

	// git-info: ExplorerPreset, gitinfo panel focused (branches tab)
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		// Focus gitinfo panel (second panel in explorer layout).
		m.engine.FocusNext()
		capture("git-info", m)
	}

	// git-info-issues: ExplorerPreset, github panel on issues tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		// Focus github panel (third panel: filetree → gitinfo → github).
		m.engine.FocusNext()
		m.engine.FocusNext()
		setGitInfoTab(m, "issues")
		capture("git-info-issues", m)
	}

	// git-info-prs: ExplorerPreset, github panel on PRs tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.engine.FocusNext()
		m.engine.FocusNext()
		setGitInfoTab(m, "prs")
		capture("git-info-prs", m)
	}

	// git-info-actions: ExplorerPreset, github panel on actions tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.engine.FocusNext()
		m.engine.FocusNext()
		setGitInfoTab(m, "actions")
		capture("git-info-actions", m)
	}

	// git-info-tags: ExplorerPreset, gitinfo on tags tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		m.engine.FocusNext()
		injectGitHubDemoData(m)
		setGitInfoTab(m, "tags")
		capture("git-info-tags", m)
	}

	// git-info-remotes: ExplorerPreset, gitinfo on remotes tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		m.engine.FocusNext()
		injectGitHubDemoData(m)
		setGitInfoTab(m, "remotes")
		capture("git-info-remotes", m)
	}

	// git-info-reflog: ExplorerPreset, gitinfo on reflog tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		m.engine.FocusNext()
		injectGitHubDemoData(m)
		setGitInfoTab(m, "reflog")
		capture("git-info-reflog", m)
	}

	// git-info-workflows: ExplorerPreset, github panel on workflows tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.engine.FocusNext()
		m.engine.FocusNext()
		setGitInfoTab(m, "workflows")
		capture("git-info-workflows", m)
	}

	// git-info-releases: ExplorerPreset, github panel on releases tab
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.engine.FocusNext()
		m.engine.FocusNext()
		setGitInfoTab(m, "releases")
		capture("git-info-releases", m)
	}

	// terminal: FullPreset with terminal panel focused
	{
		m, err := newScreenshotModel(width, height, layout.FullPreset(), th)
		if err != nil {
			return nil, err
		}
		// FullPreset panels: filetree, gitstatus, preview, terminal.
		// Focus the terminal panel (4th position).
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		capture("terminal", m)
	}

	// conflicts: Custom layout with conflicts panel
	{
		preset := makeCustomPreset("conflicts-shot", "conflicts", "preview")
		m, err := newScreenshotModel(width, height, preset, th)
		if err != nil {
			return nil, err
		}
		capture("conflicts", m)
	}

	// git-blame: ExplorerPreset with preview panel focused showing a file
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		selectFileForScreenshot(m, "README.md")
		// Focus the preview panel (5th in explorer: filetree, gitinfo, github, commits, preview).
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		m.engine.FocusNext()
		capture("git-blame", m)
	}

	// review-layout: ReviewPreset
	{
		m, err := newScreenshotModel(width, height, layout.ReviewPreset(), th)
		if err != nil {
			return nil, err
		}
		capture("review-layout", m)
	}

	// full-layout: FullPreset
	{
		m, err := newScreenshotModel(width, height, layout.FullPreset(), th)
		if err != nil {
			return nil, err
		}
		capture("full-layout", m)
	}

	// -- Overlay screenshots -----------------------------------------------

	// fuzzy-finder: FuzzyFinder overlay active
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.fuzzyFinder = m.overlays.NewFuzzyFinder("files", nil)
		m.fuzzyFinder.Focus()
		w, h := m.fuzzyFinderDims()
		m.fuzzyFinder.SetSize(w, h)
		capture("fuzzy-finder", m)
	}

	// settings: Settings overlay active
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.settingsShown = true
		m.settingsPanel = m.overlays.NewSettingsPanel(
			m.engine.CurrentPreviewPosition(),
			"default",
			theme.ListThemes(),
			config.ActionsConfig{},
		)
		m.settingsPanel.Focus()
		sw, sh := m.settingsOverlayDims()
		m.settingsPanel.SetSize(sw-2, sh-2)
		m.settingsPanel.Init(m.ctx)
		capture("settings", m)
	}

	// help: Help overlay active
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		m.helpShown = true
		m.helpPanel = m.overlays.NewHelpPanel()
		m.helpPanel.Focus()
		hw, hh := m.helpOverlayDims()
		m.helpPanel.SetSize(hw, hh)
		m.helpPanel.Init(m.ctx)
		capture("help", m)
	}

	// bookmarks: Bookmarks overlay active
	{
		m, err := newScreenshotModel(width, height, layout.ExplorerPreset(), th)
		if err != nil {
			return nil, err
		}
		injectGitHubDemoData(m)
		updated, _ := m.toggleBookmarks()
		*m = mustModel(updated)
		capture("bookmarks", m)
	}

	return shots, nil
}

// ansiStripRe matches ANSI CSI sequences for stripping.
var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// compositeOverlay merges an overlay (modal dialog) on top of a background
// line-by-line. Lines outside the modal border are replaced with the
// corresponding background line.
func compositeOverlay(bg, fg string) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	n := len(bgLines)
	if len(fgLines) > n {
		n = len(fgLines)
	}

	result := make([]string, n)
	insideModal := false
	for i := 0; i < n; i++ {
		var fgLine, bgLine string
		if i < len(fgLines) {
			fgLine = fgLines[i]
		}
		if i < len(bgLines) {
			bgLine = bgLines[i]
		}

		stripped := ansiStripRe.ReplaceAllString(fgLine, "")

		if strings.Contains(stripped, "\u256d") { // top-left corner
			insideModal = true
		}

		if insideModal {
			result[i] = fgLine
		} else {
			trimmed := strings.TrimRight(stripped, " ")
			if trimmed == "" {
				result[i] = bgLine
			} else {
				result[i] = fgLine
			}
		}

		if strings.Contains(stripped, "\u2570") { // bottom-left corner
			insideModal = false
		}
	}
	return strings.Join(result, "\n")
}

// newScreenshotModel creates a minimal Model suitable for off-screen
// rendering. It bypasses the bubbletea runtime and initializes all
// panels with demo data.
func newScreenshotModel(width, height int, preset layout.Preset, th *theme.Theme) (*Model, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	// Disable AI for screenshots.
	cfg.AI.Enabled = false
	// Show preview on the bottom for a wider view of file content.
	cfg.Preview.Position = "bottom"
	// Force nerd font icons for screenshot marketing assets.
	cfg.FileTree.IconMode = "nerd"

	reg := layout.NewRegistry()
	layout.RegisterDefaults(context.Background(), reg, cfg, nil, th)

	engine, err := layout.NewEngine(reg, preset)
	if err != nil {
		return nil, err
	}
	// Show preview on the bottom for a wider view of file content.
	engine.SetPreviewPosition(layout.PreviewBottom)

	km, err := keymap.NewKeymap("default")
	if err != nil {
		return nil, err
	}

	bmMgr := bm.NewManager(cfg.Bookmarks)

	m := New(engine, th, km, bmMgr)
	m.width = width
	m.height = height
	m.ready = true

	// Simulate a WindowSizeMsg so the engine allocates panel rects.
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = mustModel(updated)

	// Run Init to populate panel data.
	initCmd := m.Init()
	runCmds(&m, initCmd)
	runCmds(&m, cmd)

	// Run multiple resolution rounds so async command chains (git
	// operations, file loading) have time to fully complete. Each round
	// re-sizes the model and processes any pending commands that previous
	// rounds may have produced.
	for range 3 {
		updated, cmd = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
		m = mustModel(updated)
		runCmds(&m, cmd)
	}

	return &m, nil
}

// cmdTimeout is the maximum time to wait for a single tea.Cmd to return.
// Commands that take longer (tickers, file watchers, subscriptions) are
// assumed to be blocking and are skipped. Git operations need up to
// 500ms for the initial shell-out, so we give a generous budget.
const cmdTimeout = 500 * time.Millisecond

// maxCmdDepth limits recursive command resolution to prevent infinite loops.
const maxCmdDepth = 50

// runCmds executes synchronous commands returned by Init/Update.
// It only handles tea.BatchMsg (nested batches) and direct message-returning
// functions. Async commands (tickers, subscriptions) are skipped via a
// timeout to avoid blocking the screenshot capture process.
func runCmds(m *Model, cmd tea.Cmd) {
	runCmdsDepth(m, cmd, 0)
}

func runCmdsDepth(m *Model, cmd tea.Cmd, depth int) {
	if cmd == nil || depth > maxCmdDepth {
		return
	}

	// Execute the command in a goroutine so we can time it out.
	ch := make(chan tea.Msg, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- nil
			}
		}()
		ch <- cmd()
	}()

	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(cmdTimeout):
		// Command is blocking (ticker, subscription, file watcher) — skip.
		return
	}

	if msg == nil {
		return
	}

	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runCmdsDepth(m, c, depth+1)
		}
	default:
		updated, nextCmd := m.Update(msg)
		*m = mustModel(updated)
		if nextCmd != nil {
			runCmdsDepth(m, nextCmd, depth+1)
		}
	}
}

// makeCustomPreset creates a two-panel horizontal split preset with the
// given left (40%) and right (60%) panel names.
func makeCustomPreset(name, left, right string) layout.Preset {
	tree := &layout.SplitNode{
		Direction: layout.Horizontal,
		Ratio:     0.4,
		First:     &layout.LeafNode{Panel: left},
		Second:    &layout.LeafNode{Panel: right},
	}
	return layout.Preset{
		Name:   name,
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// setGitInfoTab attempts to switch a gitinfo-type panel to the named tab.
// For git tabs (branches, worktrees, remotes, stash, tags, reflog) it
// looks up "gitinfo"; for GitHub tabs it looks up "github", falling back
// to "gitinfo" for backward compatibility.
func setGitInfoTab(m *Model, tabName string) {
	// Determine which registered panel name to target.
	panelName := "gitinfo"
	switch tabName {
	case "issues", "prs", "actions", "workflows", "releases":
		panelName = "github"
	}

	allPanels := m.engine.Panels()
	p, ok := allPanels[panelName]
	if !ok {
		// Fallback: try the other panel name.
		if panelName == "github" {
			p, ok = allPanels["gitinfo"]
		}
		if !ok {
			return
		}
	}
	type tabSetter interface {
		SetActiveTab(name string)
	}
	if ts, ok := p.(tabSetter); ok {
		ts.SetActiveTab(tabName)
	}
}

// selectFileForScreenshot reveals a file in the file tree and triggers
// the preview panel to load it, so screenshots show real file content
// instead of "No file selected".
func selectFileForScreenshot(m *Model, name string) {
	cwd, _ := os.Getwd()
	absPath := filepath.Join(cwd, name)
	updated, cmd := m.Update(panels.RevealFileMsg{Path: absPath})
	*m = mustModel(updated)
	runCmds(m, cmd)
}
