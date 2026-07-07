// Package gitdiff implements the diff viewer panel for the grut TUI.
// It displays file diffs with syntax-highlighted additions and deletions,
// supporting both inline (unified) and side-by-side view modes.
package gitdiff

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// viewMode selects between inline (unified) and side-by-side diff display.
type viewMode int

const (
	viewInline     viewMode = iota // default: unified diff format
	viewSideBySide                 // two-column old/new view
)

// maxRenderedLines caps the total number of pre-rendered lines to prevent
// excessive memory usage and sluggish rendering for very large diffs.
const maxRenderedLines = 50000

// GitDiff is the diff viewer panel. It displays file diffs with
// syntax-highlighted additions/deletions in inline or side-by-side format.
type GitDiff struct {
	// Right-click context menu config
	actionsCfg config.ActionsConfig
	// Dependencies
	gitClient git.StatusReader
	// Context for async operations (set in Init)
	ctx   context.Context
	err   error // last error from diff fetch
	theme *theme.Theme
	// Request state
	path string // file path being diffed
	// Ref comparison state (for branch/tag diff mode)
	compareCommitA string // base ref (e.g., "main")
	compareCommitB string // head ref (e.g., "HEAD")
	compareMode    bool   // when true, use ref comparison instead of working tree
	compareThree   bool   // use three-dot notation
	// Diff data
	diffs []git.FileDiff // all file diffs returned by git
	// Pre-rendered content (rebuilt on data/mode/size change)
	lines       []string // rendered lines with ANSI styling
	hunkStarts  []int    // line indices where hunks begin (for n/N)
	fileStarts  []int    // line indices where files begin (for [/])
	viewportBuf []string // reusable scratch buffer for renderViewport
	// AI review annotations
	reviewFindings []panels.AIReviewFinding // findings for the current file
	panels.BasePanel
	fileIdx int // current file index for multi-file navigation
	// contextLines is the number of surrounding lines requested via -U<n>.
	contextLines int
	// Display state
	scrollY int      // viewport scroll offset (lines)
	mode    viewMode // inline or side-by-side
	// Generation counter to discard stale async results (CWE-362).
	diffGen               uint64
	staged                bool // whether viewing staged (index) changes
	loading               bool // true while async diff fetch is in progress
	showReviewAnnotations bool // toggle for inline annotation display
	// preserveOnLoad keeps scroll/file position across a context reload.
	preserveOnLoad   bool
	ignoreWhitespace bool // when true, pass -w to git diff to hide whitespace-only changes
	wordHighlight    bool // toggle for intra-line (word-level) diff emphasis
}

// diffLoadedMsg is the result of an async diff fetch (F01: no blocking in Update).
type diffLoadedMsg struct {
	err        error
	path       string
	diffs      []git.FileDiff
	generation uint64
}

// Compile-time check that *GitDiff implements panels.Panel.
var _ panels.Panel = (*GitDiff)(nil)

// New creates a new GitDiff panel. gitClient may be nil (the panel
// will show an error until a client is available). th may be nil
// (fallback colors are used).
func New(gitClient git.StatusReader, th *theme.Theme) *GitDiff {
	return &GitDiff{
		BasePanel:    panels.BasePanel{PanelTitle: "gitdiff"},
		gitClient:    gitClient,
		theme:        th,
		contextLines: defaultDiffContext,
	}
}

// SetActionsCfg stores the right-click context menu configuration.
func (d *GitDiff) SetActionsCfg(cfg config.ActionsConfig) {
	d.actionsCfg = cfg
}

// SetWordHighlight sets whether intra-line (word-level) emphasis is applied to
// changed lines in the inline diff view. It is wired from the git config at
// startup; the "w" key toggles it for the session.
func (d *GitDiff) SetWordHighlight(enabled bool) {
	if d.wordHighlight == enabled {
		return
	}
	d.wordHighlight = enabled
	d.rebuildLines()
	d.clampScroll()
}

// Init implements panels.Panel.
func (d *GitDiff) Init(ctx context.Context) tea.Cmd {
	d.ctx = ctx
	return nil
}

// Update implements panels.Panel. It handles diff-related messages and
// keyboard events for scrolling, navigation, and view mode toggling.
func (d *GitDiff) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panels.ShowDiffMsg:
		if msg.CommitA != "" && msg.CommitB != "" {
			d.compareMode = true
			d.compareCommitA = msg.CommitA
			d.compareCommitB = msg.CommitB
			d.compareThree = msg.ThreeDot
			return d, d.startRefDiffLoad(msg.Path, msg.CommitA, msg.CommitB, msg.ThreeDot)
		}
		d.compareMode = false
		return d, d.startDiffLoad(msg.Path, msg.Staged)
	case panels.FileSelectedMsg:
		if d.compareMode {
			// In compare mode, ShowDiffMsg handles diff loading with proper
			// ref comparison context. Ignore FileSelectedMsg to avoid
			// overwriting with a working tree diff.
			return d, nil
		}
		return d, d.startDiffLoad(msg.Path, false)
	case panels.BranchDiffFilterActiveMsg:
		if !msg.Active {
			d.compareMode = false
			d.compareCommitA = ""
			d.compareCommitB = ""
			d.compareThree = false
		}
		return d, nil
	case panels.BranchDeselectedMsg:
		d.compareMode = false
		d.compareCommitA = ""
		d.compareCommitB = ""
		d.compareThree = false
		return d, nil
	case panels.GitFilterActiveMsg:
		if msg.Active {
			d.compareMode = false
		}
		return d, nil
	case panels.AIReviewReadyMsg:
		d.reviewFindings = d.filterFindingsForFile(msg.Findings)
		d.rebuildLines()
		return d, nil
	case diffLoadedMsg:
		// Only apply if the loaded result matches the current request
		// and is from the latest generation (discard stale results).
		if msg.path == d.path && msg.generation == d.diffGen {
			d.loading = false
			d.diffs = msg.diffs
			d.err = msg.err
			if d.preserveOnLoad {
				// Context reload: keep the reader roughly where they were.
				d.preserveOnLoad = false
				d.rebuildLines()
				d.clampScroll()
			} else {
				d.fileIdx = 0
				d.scrollY = 0
				d.rebuildLines()
			}
		}
		return d, nil
	case panels.PanelMouseRightClickMsg:
		// No-op: gitdiff is a viewport panel without individually selectable items.
		return d, nil
	case panels.RepoChangedMsg:
		return d.handleRepoChanged(msg)
	case tea.KeyPressMsg:
		if !d.Focused {
			return d, nil
		}
		return d.handleKey(msg)
	case tea.MouseWheelMsg:
		return d.handleMouseWheel(msg)
	}
	return d, nil
}

// handleRepoChanged replaces the git client and clears the diff display
// for the new repository after a directory change.
func (d *GitDiff) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		d.gitClient = nil
	} else {
		d.gitClient = client
	}
	d.diffs = nil
	d.lines = nil
	d.hunkStarts = nil
	d.fileStarts = nil
	d.path = ""
	d.scrollY = 0
	d.fileIdx = 0
	d.err = nil
	d.loading = false
	d.reviewFindings = nil
	d.showReviewAnnotations = false
	d.compareMode = false
	d.compareCommitA = ""
	d.compareCommitB = ""
	d.compareThree = false
	d.contextLines = defaultDiffContext
	d.preserveOnLoad = false
	return d, nil
}

// startDiffLoad resets state and returns a tea.Cmd to fetch the diff async.
func (d *GitDiff) startDiffLoad(path string, staged bool) tea.Cmd {
	d.path = path
	d.staged = staged
	d.scrollY = 0
	d.fileIdx = 0
	d.err = nil
	d.diffs = nil
	d.lines = nil
	d.hunkStarts = nil
	d.fileStarts = nil
	d.loading = true
	return d.loadDiffCmd(path, staged)
}

// startRefDiffLoad loads a diff between two refs (branch comparison mode).
func (d *GitDiff) startRefDiffLoad(path, commitA, commitB string, threeDot bool) tea.Cmd {
	d.path = path
	d.staged = false
	d.scrollY = 0
	d.fileIdx = 0
	d.err = nil
	d.diffs = nil
	d.lines = nil
	d.hunkStarts = nil
	d.fileStarts = nil
	d.loading = true
	return d.loadRefDiffCmd(path, commitA, commitB, threeDot)
}

// loadRefDiffCmd fetches a ref-comparison diff async without resetting
// scroll or file position, so it can back both initial loads and reloads.
func (d *GitDiff) loadRefDiffCmd(path, commitA, commitB string, threeDot bool) tea.Cmd {
	d.diffGen++
	gen := d.diffGen
	gitClient := d.gitClient
	ctx := d.ctx
	contextLines := d.contextLines
	ignoreWS := d.ignoreWhitespace
	return func() tea.Msg {
		result := diffLoadedMsg{path: path, generation: gen}
		if gitClient == nil {
			result.err = fmt.Errorf("no git client configured")
			return result
		}
		if ctx == nil {
			ctx = context.Background()
		}
		diffs, err := gitClient.Diff(ctx, git.DiffOpts{
			CommitA:   commitA,
			CommitB:   commitB,
			ThreeDot:  threeDot,
			Path:      path,
			IgnoreAll: ignoreWS,
			Context:   contextLines,
		})
		result.diffs = diffs
		result.err = err
		return result
	}
}

// handleKey processes keyboard input when the panel is focused.
func (d *GitDiff) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		d.scrollDown(1)
	case "k", "up":
		d.scrollUp(1)
	case "pgdown":
		d.scrollDown(d.pageSize())
	case "pgup":
		d.scrollUp(d.pageSize())
	case "g":
		d.scrollY = 0
	case "G":
		d.scrollToBottom()
	case "t":
		d.toggleViewMode()
	case "n":
		d.nextHunk()
	case "N":
		d.prevHunk()
	case "]":
		d.nextFile()
	case "[":
		d.prevFile()
	case "R":
		d.toggleReviewAnnotations()
	case "+", "=":
		return d, d.increaseContext()
	case "-", "_":
		return d, d.decreaseContext()
	case "0":
		return d, d.resetContext()
	case "y":
		return d.copyCurrentHunk()
	case "w":
		d.toggleWordHighlight()
	case "W":
		return d, d.toggleIgnoreWhitespace()
	}
	return d, nil
}

// increaseContext widens the diff context by one step and reloads.
func (d *GitDiff) increaseContext() tea.Cmd {
	n := d.contextLines + contextStep
	if n > maxDiffContext {
		n = maxDiffContext
	}
	if n == d.contextLines {
		return nil
	}
	d.contextLines = n
	return d.reloadWithContext()
}

// decreaseContext narrows the diff context by one step (floor 0) and reloads.
func (d *GitDiff) decreaseContext() tea.Cmd {
	n := d.contextLines - contextStep
	if n < 0 {
		n = 0
	}
	if n == d.contextLines {
		return nil
	}
	d.contextLines = n
	return d.reloadWithContext()
}

// resetContext restores the default context and reloads if it changed.
func (d *GitDiff) resetContext() tea.Cmd {
	if d.contextLines == defaultDiffContext {
		return nil
	}
	d.contextLines = defaultDiffContext
	return d.reloadWithContext()
}

// reloadWithContext re-fetches the current diff with the current context
// value, preserving scroll/file position. Returns nil when nothing is loaded.
func (d *GitDiff) reloadWithContext() tea.Cmd {
	if d.path == "" && !d.compareMode {
		return nil
	}
	d.loading = true
	d.preserveOnLoad = true
	if d.compareMode {
		return d.loadRefDiffCmd(d.path, d.compareCommitA, d.compareCommitB, d.compareThree)
	}
	return d.loadDiffCmd(d.path, d.staged)
}

// copyCurrentHunk copies the raw unified-diff text of the hunk under the
// current scroll position to the clipboard. It is a safe no-op (with a
// warning toast) when no diff is loaded.
func (d *GitDiff) copyCurrentHunk() (panels.Panel, tea.Cmd) {
	hunk, ok := d.currentHunk()
	if !ok {
		return d, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No hunk to copy", Level: notify.Warn}
		}
	}
	patch := hunkToPatch(hunk)
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return d, func() tea.Msg {
		if err := panels.CopyToClipboard(ctx, patch); err != nil {
			return notify.ShowToastMsg{Message: "Copy failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Copied hunk to clipboard", Level: notify.Success}
	}
}

// currentHunk returns the diff hunk under the current scroll position and
// true, or a zero hunk and false when no diff is loaded. The hunk is located
// by mapping d.scrollY through d.hunkStarts, which is built in the same
// file-then-hunk order as the flattened hunk list below.
func (d *GitDiff) currentHunk() (git.Hunk, bool) {
	var flat []git.Hunk
	for _, fd := range d.diffs {
		flat = append(flat, fd.Hunks...)
	}
	if len(flat) == 0 {
		return git.Hunk{}, false
	}
	idx := 0
	for i, start := range d.hunkStarts {
		if start <= d.scrollY {
			idx = i
		} else {
			break
		}
	}
	if idx >= len(flat) {
		idx = len(flat) - 1
	}
	return flat[idx], true
}

// hunkToPatch renders a hunk as raw unified-diff text: the "@@" header
// followed by each line prefixed with ' ', '+', or '-'.
func hunkToPatch(h git.Hunk) string {
	var b strings.Builder
	b.WriteString(h.Header)
	for _, ln := range h.Lines {
		b.WriteByte('\n')
		b.WriteString(diffLinePrefix(ln.Type))
		b.WriteString(ln.Content)
	}
	return b.String()
}

// diffLinePrefix returns the single-character unified-diff prefix for a line.
func diffLinePrefix(t git.DiffLineType) string {
	switch t {
	case git.DiffLineAdded:
		return "+"
	case git.DiffLineRemoved:
		return "-"
	default:
		return " "
	}
}

// handleMouseWheel scrolls the diff viewport.
func (d *GitDiff) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	switch m.Button {
	case tea.MouseWheelUp:
		d.scrollUp(panels.ScrollDelta)
	case tea.MouseWheelDown:
		d.scrollDown(panels.ScrollDelta)
	}
	return d, nil
}

// View implements panels.Panel. It renders the diff content into the
// given width×height area.
func (d *GitDiff) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if d.loading {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(d.themeColors().BrightBlack, "#666666")).
			Render("Loading diff...")
	}
	if d.err != nil {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(d.themeColors().NormalRed, "#C44B4B")).
			Render(fmt.Sprintf("Error: %s", d.err))
	}
	if len(d.diffs) == 0 {
		msg := "No changes"
		if d.path == "" {
			msg = "No file selected\n\nSelect a file to view diff"
		}
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(d.themeColors().BrightBlack, "#666666")).
			Render(msg)
	}
	return d.renderViewport(width, height)
}

// Title implements panels.Panel. Returns a dynamic title showing the
// current file, or "gitdiff" when no file is selected.
func (d *GitDiff) Title() string {
	if d.path == "" {
		return "gitdiff"
	}
	base := filepath.Base(d.path)
	if d.staged {
		base += " (staged)"
	}
	if d.ignoreWhitespace {
		base += " [ignore ws]"
	}
	return base
}

// KeyBindings implements panels.Panel.
func (d *GitDiff) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "j/k", Description: "Scroll up/down", Action: "scroll"},
		{Key: "d/u", Description: "Page down/up", Action: "page"},
		{Key: "g/G", Description: "Top/bottom", Action: "goto"},
		{Key: "t", Description: "Toggle inline/side-by-side", Action: "toggle_view"},
		{Key: "n/N", Description: "Next/previous hunk", Action: "hunk_nav"},
		{Key: "[/]", Description: "Previous/next file", Action: "file_nav"},
		{Key: "y", Description: "Copy hunk to clipboard", Action: "copy_hunk"},
		{Key: "R", Description: "Toggle review annotations", Action: "toggle_review"},
		{Key: "+/-", Description: "More/less diff context", Action: "context_adjust"},
		{Key: "0", Description: "Reset diff context", Action: "context_reset"},
		{Key: "w", Description: "Toggle word-level diff", Action: "toggle_word_diff"},
		{Key: "W", Description: "Toggle ignore whitespace", Action: "toggle_whitespace"},
	}
}

// SetSize implements panels.Panel. Overrides BasePanel to trigger a
// re-render when dimensions change (side-by-side needs width info).
func (d *GitDiff) SetSize(width, height int) {
	d.BasePanel.SetSize(width, height)
	if len(d.diffs) > 0 {
		d.rebuildLines()
	}
}

// SetDiffs directly injects diff data without a git client, primarily
// for testing. It resets scroll and rebuilds rendered lines.
func (d *GitDiff) SetDiffs(diffs []git.FileDiff) {
	d.diffs = diffs
	d.err = nil
	d.loading = false
	d.fileIdx = 0
	d.scrollY = 0
	d.rebuildLines()
}

// --- Async loading ---
// loadDiffCmd returns a tea.Cmd that fetches diff data asynchronously.
// The git I/O happens in a background goroutine managed by Bubble Tea (F01).
func (d *GitDiff) loadDiffCmd(path string, staged bool) tea.Cmd {
	d.diffGen++
	gen := d.diffGen
	gitClient := d.gitClient
	ctx := d.ctx
	contextLines := d.contextLines
	ignoreWS := d.ignoreWhitespace
	return func() tea.Msg {
		result := diffLoadedMsg{path: path, generation: gen}
		if gitClient == nil {
			result.err = fmt.Errorf("no git client configured")
			return result
		}
		if ctx == nil {
			ctx = context.Background()
		}
		diffs, err := gitClient.Diff(ctx, git.DiffOpts{
			Staged:    staged,
			Path:      path,
			IgnoreAll: ignoreWS,
			Context:   contextLines,
		})
		result.diffs = diffs
		result.err = err
		return result
	}
}

// --- Pre-rendering ---
// rebuildLines pre-renders all diff content into d.lines based on the
// current view mode and panel dimensions. Also populates hunkStarts
// and fileStarts for navigation. Slice resets are handled by the
// individual build functions to allow direct calls from benchmarks.
func (d *GitDiff) rebuildLines() {
	if len(d.diffs) == 0 {
		d.lines = d.lines[:0]
		d.hunkStarts = d.hunkStarts[:0]
		d.fileStarts = d.fileStarts[:0]
		return
	}
	switch d.mode {
	case viewSideBySide:
		d.buildSideBySideLines()
	default:
		d.buildInlineLines()
	}
}

// buildInlineLines renders diffs in unified (inline) format:
//
//	── file.go ──────────────
//	@@ -10,3 +10,4 @@
//	  context line
//	- removed line
//	+ added line
func (d *GitDiff) buildInlineLines() {
	// Reuse struct slices — reset to zero length, keeping backing arrays
	// so appends avoid heap allocation when capacity is sufficient.
	d.lines = d.lines[:0]
	d.hunkStarts = d.hunkStarts[:0]
	d.fileStarts = d.fileStarts[:0]
	findingsMap := d.buildFindingsMap()
	for _, fd := range d.diffs {
		d.fileStarts = append(d.fileStarts, len(d.lines))
		// File header
		header := d.fileHeader(fd)
		d.lines = append(d.lines, d.headerStyle().Render(header))
		if fd.IsBinary {
			d.lines = append(d.lines, d.dimStyle().Render("Binary file differs"))
			d.lines = append(d.lines, "") // blank separator
			continue
		}
		if len(fd.Hunks) == 0 {
			d.lines = append(d.lines, d.dimStyle().Render("No changes"))
			d.lines = append(d.lines, "")
			continue
		}
		for _, hunk := range fd.Hunks {
			d.hunkStarts = append(d.hunkStarts, len(d.lines))
			d.lines = append(d.lines, d.headerStyle().Render(hunk.Header))
			var emph map[int][]wordSeg
			if d.wordHighlight {
				emph = computeHunkWordEmphasis(hunk.Lines)
			}
			for li, line := range hunk.Lines {
				if len(d.lines) >= maxRenderedLines {
					d.lines = append(d.lines, d.dimStyle().Render("[Diff truncated — too large to display]"))
					return
				}
				var rendered string
				switch line.Type {
				case git.DiffLineAdded:
					if segs := emph[li]; len(segs) > 0 {
						rendered = d.renderWordLine("+ ", d.addedStyle(), segs)
					} else {
						rendered = d.addedStyle().Render("+ " + line.Content)
					}
				case git.DiffLineRemoved:
					if segs := emph[li]; len(segs) > 0 {
						rendered = d.renderWordLine("- ", d.removedStyle(), segs)
					} else {
						rendered = d.removedStyle().Render("- " + line.Content)
					}
				default:
					rendered = d.contextStyle().Render("  " + line.Content)
				}
				d.lines = append(d.lines, rendered)
				// Inject review annotations for this line number
				if d.showReviewAnnotations {
					lineNum := line.NewLine
					if line.Type == git.DiffLineRemoved {
						lineNum = line.OldLine
					}
					if findings, ok := findingsMap[lineNum]; ok {
						for _, f := range findings {
							d.lines = append(d.lines, d.renderAnnotation(f))
						}
					}
				}
			}
		}
		d.lines = append(d.lines, "") // blank separator between files
	}
}

// linePair pairs an old-side line with a new-side line for side-by-side display.
type linePair struct {
	old *git.DiffLine
	new *git.DiffLine
}

// buildSideBySideLines renders diffs in a two-column format with
// old content on the left and new content on the right.
func (d *GitDiff) buildSideBySideLines() {
	// Reuse struct slices — reset to zero length, keeping backing arrays
	// so appends avoid heap allocation when capacity is sufficient.
	d.lines = d.lines[:0]
	d.hunkStarts = d.hunkStarts[:0]
	d.fileStarts = d.fileStarts[:0]
	colWidth := d.Width / 2
	if colWidth < 10 {
		colWidth = 10
	}
	const numWidth = 4                      // line number width
	contentWidth := colWidth - numWidth - 3 // "NNNN │ "
	if contentWidth < 1 {
		contentWidth = 1
	}
	// Cache the empty column string used for nil side-by-side lines.
	emptyCol := strings.Repeat(" ", numWidth+3+contentWidth)
	findingsMap := d.buildFindingsMap()
	for _, fd := range d.diffs {
		d.fileStarts = append(d.fileStarts, len(d.lines))
		// File headers (side-by-side)
		leftHeader := fmt.Sprintf("── %s (old) ", fd.Path)
		rightHeader := fmt.Sprintf("── %s (new) ", fd.Path)
		leftHeader = d.padOrTruncate(leftHeader, colWidth, '─')
		rightHeader = d.padOrTruncate(rightHeader, colWidth, '─')
		d.lines = append(d.lines, d.headerStyle().Render(leftHeader+" "+rightHeader))
		if fd.IsBinary {
			left := padRight("Binary file differs", colWidth)
			right := padRight("Binary file differs", colWidth)
			d.lines = append(d.lines, d.dimStyle().Render(left+" "+right))
			d.lines = append(d.lines, "")
			continue
		}
		if len(fd.Hunks) == 0 {
			left := padRight("No changes", colWidth)
			right := padRight("No changes", colWidth)
			d.lines = append(d.lines, d.dimStyle().Render(left+" "+right))
			d.lines = append(d.lines, "")
			continue
		}
		for _, hunk := range fd.Hunks {
			d.hunkStarts = append(d.hunkStarts, len(d.lines))
			d.lines = append(d.lines, d.headerStyle().Render(hunk.Header))
			pairs := pairDiffLines(hunk.Lines)
			for _, pair := range pairs {
				if len(d.lines) >= maxRenderedLines {
					d.lines = append(d.lines, d.dimStyle().Render("[Diff truncated — too large to display]"))
					return
				}
				leftStr := d.fmtSideCol(pair.old, numWidth, contentWidth, emptyCol, false)
				rightStr := d.fmtSideCol(pair.new, numWidth, contentWidth, emptyCol, true)
				d.lines = append(d.lines, leftStr+" │ "+rightStr)
				// Inject review annotations for lines in this pair
				if d.showReviewAnnotations {
					annotated := make(map[int]bool)
					for _, side := range []*git.DiffLine{pair.old, pair.new} {
						if side == nil {
							continue
						}
						lineNum := side.NewLine
						if side.Type == git.DiffLineRemoved {
							lineNum = side.OldLine
						}
						if annotated[lineNum] {
							continue
						}
						if findings, ok := findingsMap[lineNum]; ok {
							annotated[lineNum] = true
							for _, f := range findings {
								d.lines = append(d.lines, d.renderAnnotation(f))
							}
						}
					}
				}
			}
		}
		d.lines = append(d.lines, "") // blank separator
	}
}

// pairDiffLines aligns removed/added line blocks for side-by-side display.
// Consecutive removed lines are paired with consecutive added lines.
// Context lines appear on both sides.
func pairDiffLines(lines []git.DiffLine) []linePair {
	// Pre-allocate with estimated capacity: worst case is 1 pair per line.
	pairs := make([]linePair, 0, len(lines))
	var removed, added []git.DiffLine
	flush := func() {
		maxLen := len(removed)
		if len(added) > maxLen {
			maxLen = len(added)
		}
		for i := range maxLen {
			var o, n *git.DiffLine
			if i < len(removed) {
				o = &removed[i]
			}
			if i < len(added) {
				n = &added[i]
			}
			pairs = append(pairs, linePair{old: o, new: n})
		}
		removed = removed[:0]
		added = added[:0]
	}
	for i := range lines {
		line := lines[i]
		switch line.Type {
		case git.DiffLineRemoved:
			removed = append(removed, line)
		case git.DiffLineAdded:
			// If we have pending removed lines and now see added, they
			// form a change block. If we only have added lines with no
			// preceding removed, they still accumulate.
			added = append(added, line)
		case git.DiffLineContext:
			flush()
			pairs = append(pairs, linePair{old: &lines[i], new: &lines[i]})
		}
	}
	flush()
	return pairs
}

// fmtSideCol renders one column of a side-by-side line.
// emptyCol is the pre-computed padding string for nil lines.
func (d *GitDiff) fmtSideCol(line *git.DiffLine, numWidth, contentWidth int, emptyCol string, isNew bool) string {
	if line == nil {
		return emptyCol
	}
	lineNum := line.OldLine
	if isNew {
		lineNum = line.NewLine
	}
	// Right-align line number using strconv instead of fmt.Sprintf.
	numStr := strconv.Itoa(lineNum)
	if pad := numWidth - len(numStr); pad > 0 {
		numStr = strings.Repeat(" ", pad) + numStr
	}
	content := truncate(line.Content, contentWidth)
	content = padRight(content, contentWidth)
	full := numStr + " │ " + content
	switch line.Type {
	case git.DiffLineAdded:
		return d.addedStyle().Render(full)
	case git.DiffLineRemoved:
		return d.removedStyle().Render(full)
	default:
		return d.contextStyle().Render(full)
	}
}

// --- Viewport rendering ---
// renderViewport renders the visible portion of pre-rendered lines
// into the given width×height area with a scroll indicator.
func (d *GitDiff) renderViewport(_ int, height int) string {
	if len(d.lines) == 0 {
		return ""
	}
	d.clampScroll()
	contentHeight := height - 1 // reserve one line for scroll indicator
	if contentHeight < 1 {
		contentHeight = 1
	}
	start := d.scrollY
	end := start + contentHeight
	if end > len(d.lines) {
		end = len(d.lines)
	}
	visible := append(d.viewportBuf[:0], d.lines[start:end]...)
	// Pad with empty lines to fill height
	for len(visible) < contentHeight {
		visible = append(visible, "")
	}
	d.viewportBuf = visible // retain backing array for next frame
	content := strings.Join(visible, "\n")
	// Add scroll indicator plus the current diff context value.
	indicator := d.scrollIndicator(len(d.lines), height)
	status := indicator
	if status != "" {
		status += "   "
	}
	status += fmt.Sprintf("ctx %d", d.contextLines)
	content += "\n" + d.dimStyle().Render(status)
	return content
}

// fileHeader formats the file header line for a FileDiff.
func (d *GitDiff) fileHeader(fd git.FileDiff) string {
	path := fd.Path
	if fd.OldPath != "" && fd.OldPath != fd.Path {
		path = fd.OldPath + " → " + fd.Path
	}
	padding := 40
	if padding > 0 {
		return "── " + path + " " + strings.Repeat("─", padding)
	}
	return "── " + path + " ──"
}

// --- Scrolling ---
func (d *GitDiff) scrollDown(n int) {
	d.scrollY += n
	d.clampScroll()
}

func (d *GitDiff) scrollUp(n int) {
	d.scrollY -= n
	if d.scrollY < 0 {
		d.scrollY = 0
	}
}

func (d *GitDiff) scrollToBottom() {
	maxScroll := len(d.lines) - d.pageSize()
	if maxScroll < 0 {
		maxScroll = 0
	}
	d.scrollY = maxScroll
}

func (d *GitDiff) clampScroll() {
	maxScroll := len(d.lines) - d.pageSize()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scrollY > maxScroll {
		d.scrollY = maxScroll
	}
	if d.scrollY < 0 {
		d.scrollY = 0
	}
}

func (d *GitDiff) pageSize() int {
	h := d.Height
	if h <= 0 {
		h = 20
	}
	// Reserve one line for scroll indicator
	if h > 1 {
		return h - 1
	}
	return h
}

// --- Hunk/file navigation ---
func (d *GitDiff) toggleViewMode() {
	if d.mode == viewInline {
		d.mode = viewSideBySide
	} else {
		d.mode = viewInline
	}
	d.rebuildLines()
	d.clampScroll()
}

// nextHunk scrolls to the first hunk start after the current scroll position.
func (d *GitDiff) nextHunk() {
	for _, hs := range d.hunkStarts {
		if hs > d.scrollY {
			d.scrollY = hs
			return
		}
	}
}

// prevHunk scrolls to the last hunk start before the current scroll position.
func (d *GitDiff) prevHunk() {
	for i := len(d.hunkStarts) - 1; i >= 0; i-- {
		if d.hunkStarts[i] < d.scrollY {
			d.scrollY = d.hunkStarts[i]
			return
		}
	}
}

// nextFile moves to the next file in a multi-file diff.
func (d *GitDiff) nextFile() {
	if d.fileIdx < len(d.diffs)-1 {
		d.fileIdx++
		if d.fileIdx < len(d.fileStarts) {
			d.scrollY = d.fileStarts[d.fileIdx]
		}
	}
}

// prevFile moves to the previous file in a multi-file diff.
func (d *GitDiff) prevFile() {
	if d.fileIdx > 0 {
		d.fileIdx--
		if d.fileIdx < len(d.fileStarts) {
			d.scrollY = d.fileStarts[d.fileIdx]
		}
	}
}

// --- Review annotations ---
// toggleReviewAnnotations toggles inline annotation display and rebuilds lines.
func (d *GitDiff) toggleReviewAnnotations() {
	d.showReviewAnnotations = !d.showReviewAnnotations
	d.rebuildLines()
	d.clampScroll()
}

// toggleIgnoreWhitespace flips ignore-whitespace mode and re-runs the current
// diff so whitespace-only changes are hidden or shown. It works for
// working-tree, staged, and ref-comparison (commit, branch, and PR) diffs.
// The flag is stored on the panel, so it persists as the user navigates
// between files in the same session view.
func (d *GitDiff) toggleIgnoreWhitespace() tea.Cmd {
	d.ignoreWhitespace = !d.ignoreWhitespace
	if d.path == "" {
		return nil
	}
	if d.compareMode {
		return d.startRefDiffLoad(d.path, d.compareCommitA, d.compareCommitB, d.compareThree)
	}
	return d.startDiffLoad(d.path, d.staged)
}

// toggleWordHighlight toggles intra-line (word-level) emphasis for the session
// and rebuilds lines. The startup default comes from git.diff_word_highlight.
func (d *GitDiff) toggleWordHighlight() {
	d.wordHighlight = !d.wordHighlight
	d.rebuildLines()
	d.clampScroll()
}

// filterFindingsForFile returns only findings whose File matches the
// currently displayed path. If path is empty, no findings are returned.
func (d *GitDiff) filterFindingsForFile(all []panels.AIReviewFinding) []panels.AIReviewFinding {
	if d.path == "" {
		return nil
	}
	var filtered []panels.AIReviewFinding
	for _, f := range all {
		if f.File == d.path {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// buildFindingsMap groups the current reviewFindings by line number for
// O(1) lookup during line rendering.
func (d *GitDiff) buildFindingsMap() map[int][]panels.AIReviewFinding {
	if !d.showReviewAnnotations || len(d.reviewFindings) == 0 {
		return nil
	}
	m := make(map[int][]panels.AIReviewFinding, len(d.reviewFindings))
	for _, f := range d.reviewFindings {
		m[f.Line] = append(m[f.Line], f)
	}
	return m
}

// renderAnnotation formats a single finding as an inline annotation line.
// Format: "  ⚠ [category] message — suggestion"
func (d *GitDiff) renderAnnotation(f panels.AIReviewFinding) string {
	icon := severityIcon(f.Severity)
	text := fmt.Sprintf("  %s [%s] %s", icon, f.Category, f.Message)
	if f.Suggestion != "" {
		text += " — " + f.Suggestion
	}
	return d.severityStyle(f.Severity).Render(text)
}

// severityIcon returns a unicode icon for the given severity level.
func severityIcon(severity string) string {
	switch severity {
	case severityError:
		return "✗"
	case severityWarning:
		return "⚠"
	case severityInfo:
		return "ℹ"
	case severityHint:
		return "›"
	default:
		return "⚠"
	}
}

// themeColors returns the theme's color palette, or an empty Colors{} when
// no theme is set (all fields are zero-value empty strings).
func (d *GitDiff) themeColors() theme.Colors {
	if d.theme != nil {
		return d.theme.Colors
	}
	return theme.Colors{}
}

// colorOf returns a lipgloss.Color from the themed value if non-empty,
// otherwise falls back to the hardcoded default.

// severityStyle returns a lipgloss.Style colored by severity.
func (d *GitDiff) severityStyle(severity string) lipgloss.Style {
	tc := d.themeColors()
	var c color.Color
	switch severity {
	case severityError:
		c = panels.ColorOf(tc.NormalRed, "#C44B4B")
	case severityWarning:
		c = panels.ColorOf(tc.NormalYellow, "#C9A227")
	case severityInfo:
		c = panels.ColorOf(tc.NormalBlue, "#7A9EBF")
	case severityHint:
		c = panels.ColorOf(tc.BrightBlack, "#555555")
	default:
		c = panels.ColorOf(tc.NormalYellow, "#C9A227")
	}
	return lipgloss.NewStyle().Foreground(c)
}

// --- Theme-aware styles ---
func (d *GitDiff) addedStyle() lipgloss.Style {
	if d.theme != nil {
		return d.theme.Styles.DiffAdded
	}
	return lipgloss.NewStyle().Foreground(panels.ColorOf(d.themeColors().DiffAdded, "#6B9E56"))
}

func (d *GitDiff) removedStyle() lipgloss.Style {
	if d.theme != nil {
		return d.theme.Styles.DiffRemoved
	}
	return lipgloss.NewStyle().Foreground(panels.ColorOf(d.themeColors().DiffRemoved, "#C44B4B"))
}

func (d *GitDiff) contextStyle() lipgloss.Style {
	if d.theme != nil {
		return d.theme.Styles.DiffContext
	}
	return lipgloss.NewStyle().Foreground(panels.ColorOf(d.themeColors().DiffContext, "#999999"))
}

// renderWordLine renders a changed diff line with intra-line emphasis. The
// prefix ("+ " or "- ") and unchanged segments use base; changed segments are
// reversed so the emphasis reads on any theme without guessing colors.
func (d *GitDiff) renderWordLine(prefix string, base lipgloss.Style, segs []wordSeg) string {
	emph := base.Reverse(true)
	var b strings.Builder
	b.WriteString(base.Render(prefix))
	for _, s := range segs {
		if s.Changed {
			b.WriteString(emph.Render(s.Text))
		} else {
			b.WriteString(base.Render(s.Text))
		}
	}
	return b.String()
}

func (d *GitDiff) headerStyle() lipgloss.Style {
	if d.theme != nil {
		return d.theme.Styles.DiffHeader
	}
	return lipgloss.NewStyle().Foreground(panels.ColorOf(d.themeColors().DiffHeader, "#7A9EBF")).Bold(true)
}

func (d *GitDiff) dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(panels.ColorOf(d.themeColors().BrightBlack, "#555555"))
}

// --- Helpers ---
func (d *GitDiff) scrollIndicator(totalLines, viewHeight int) string {
	contentHeight := viewHeight - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	if totalLines <= contentHeight {
		return ""
	}
	maxScroll := totalLines - contentHeight
	if maxScroll <= 0 {
		return "100%"
	}
	if d.scrollY == 0 {
		return "Top"
	}
	if d.scrollY >= maxScroll {
		return "Bot"
	}
	pct := d.scrollY * 100 / maxScroll
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%d%%", pct)
}

// padOrTruncate pads s with fillChar to reach width, or truncates if longer.
func (d *GitDiff) padOrTruncate(s string, width int, fillChar rune) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(string(fillChar), width-len(runes))
}

// truncate shortens s to maxLen runes, adding an ellipsis if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

// padRight pads s with spaces to reach at least width characters.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
