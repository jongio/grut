// Package preview implements the file preview panel for the grut TUI.
// It provides syntax-highlighted code display, markdown rendering,
// binary file detection, and a scrollable viewport.
package preview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/gabriel-vasile/mimetype"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/markdown"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// Preview is the file preview panel. It displays syntax-highlighted source
// code, rendered markdown, or file metadata for binary/oversized files.
type Preview struct {
	err error
	// Git integration
	gitClient git.StatusReader
	// File state
	filePath   string
	ghTitle    string   // title for GitHub content (e.g., "#42 Fix auth")
	ghContent  string   // raw markdown body
	lines      []string // rendered lines (with ANSI for syntax highlighting)
	blameLines []git.BlameLine
	diffLines  []string // pre-rendered diff lines for current file
	cfg        Config
	scrollY    int
	// Panel state
	width   int
	height  int
	loading bool // true while async file load is in progress
	// Display flags
	isBinary bool
	isLarge  bool
	focused  bool
	// Toggleable settings (initialized from config, toggled at runtime)
	lineNumbers    bool
	wordWrap       bool
	renderMarkdown bool
	// Blame mode
	blameMode bool
	// GitHub content mode – when a GitHub item (issue/PR/action run) is
	// selected, the preview shows the item detail instead of a file.
	ghMode      bool // true when showing GitHub content instead of file
	ghPlainText bool // true when ghContent is pre-formatted (not markdown)
	// Dual-mode preview: file content vs contextual diff.
	diffMode    bool                // when true, show diff instead of file content
	diffContext *panels.DiffContext // context for diff resolution (nil = working tree)
	// Theme support
	theme *theme.Theme
	// Text selection state
	selAnchor *selPoint // where the click/drag began (absolute line + rune col)
	selEnd    *selPoint // where the drag/shift-move reached
	selecting bool      // true while mouse button is held and dragging
	// Inline editor state (edit mode)
	editMode   bool                // true when in edit mode
	editBuf    *TextBuffer         // editable text buffer (nil when not editing)
	cursorLine int                 // cursor line (0-based, in buffer)
	cursorCol  int                 // cursor column (rune offset, 0-based)
	editCfg    config.EditorConfig // editor configuration
}

// fileLoadedMsg is the result of an async file load operation (F01).
type fileLoadedMsg struct {
	err      error
	path     string
	lines    []string
	isBinary bool
	isLarge  bool
}

// diffLoadedMsg is the result of an async diff load for the current file.
type diffLoadedMsg struct {
	err   error
	path  string
	lines []string
}

// Compile-time check that *Preview implements panels.Panel.
var _ panels.Panel = (*Preview)(nil)

// New creates a new Preview panel with the given configuration.
func New(cfg Config, editorCfg config.EditorConfig, th *theme.Theme) *Preview {
	return &Preview{
		cfg:            cfg,
		editCfg:        editorCfg,
		lineNumbers:    cfg.GetLineNumbers(),
		wordWrap:       cfg.GetWordWrap(),
		renderMarkdown: cfg.GetRenderMarkdown(),
		theme:          th,
	}
}

// SetGitClient configures the git client for diff-aware preview.
func (p *Preview) SetGitClient(gc git.StatusReader) {
	p.gitClient = gc
}

// themeColors returns the theme's color palette, or an empty Colors{} when
// no theme is set.
func (p *Preview) themeColors() theme.Colors {
	if p.theme != nil {
		return p.theme.Colors
	}
	return theme.Colors{}
}

// colorOf returns a lipgloss color for the themed value if non-empty,
// otherwise falls back to the provided default hex string.

// handleRepoChanged replaces the git client and clears preview content
// for the new repository after a directory change.
func (p *Preview) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.gitClient = nil
	} else {
		p.gitClient = client
	}
	p.filePath = ""
	p.lines = nil
	p.scrollY = 0
	p.err = nil
	p.isBinary = false
	p.isLarge = false
	p.loading = false
	p.blameMode = false
	p.blameLines = nil
	p.ghMode = false
	p.ghTitle = ""
	p.ghContent = ""
	p.ghPlainText = false
	p.diffLines = nil
	p.diffMode = false
	p.diffContext = nil
	return p, nil
}

// Init implements panels.Panel.
func (p *Preview) Init(_ context.Context) tea.Cmd {
	return nil
}

// Update implements panels.Panel. It handles file selection messages and
// keyboard events for scrolling and toggling display options.
func (p *Preview) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	// Handle edit-mode-specific modal results (dirty guard).
	case notify.ModalResultMsg:
		if p.editMode {
			return handleModalResult(p, msg)
		}

	// Handle file saved result.
	case fileSavedMsg:
		if msg.path == p.filePath {
			if p.editBuf != nil {
				p.editBuf.MarkClean()
			}
			cmds := []tea.Cmd{
				func() tea.Msg {
					return notify.ShowToastMsg{Message: "Saved " + filepath.Base(p.filePath), Level: notify.Info}
				},
				func() tea.Msg {
					return panels.FileModifiedMsg{Path: p.filePath}
				},
			}
			if p.gitClient != nil {
				cmds = append(cmds, p.loadContextDiffCmd(p.filePath, p.diffContext))
			}
			return p, tea.Batch(cmds...)
		}

	case panels.FileSelectedMsg:
		if p.editMode && p.editBuf != nil && p.editBuf.Dirty() {
			return p, dirtyGuardCmd(p, "switch")
		}
		if p.editMode {
			// Clean exit from edit mode before switching files.
			p.editMode = false
			p.editBuf = nil
		}
		// Clear GitHub content mode if active — file selection takes precedence.
		p.ghMode = false
		p.ghTitle = ""
		p.ghContent = ""
		p.ghPlainText = false
		p.clearSelection()
		// Store the diff context from the filetree's current mode.
		p.diffContext = msg.DiffContext
		// Start async file load (F01: no blocking I/O in Update).
		p.filePath = msg.Path
		p.scrollY = 0
		p.err = nil
		p.isBinary = false
		p.isLarge = false
		p.lines = nil
		p.loading = true
		p.blameMode = false
		p.blameLines = nil
		p.diffLines = nil
		cmds := []tea.Cmd{p.loadFileCmd(msg.Path)}
		if p.gitClient != nil {
			cmds = append(cmds, p.loadContextDiffCmd(msg.Path, p.diffContext))
		}
		return p, tea.Batch(cmds...)
	case fileLoadedMsg:
		// Only apply if the loaded file matches current request.
		if msg.path == p.filePath {
			p.loading = false
			p.lines = msg.lines
			p.err = msg.err
			p.isBinary = msg.isBinary
			p.isLarge = msg.isLarge
		}
		return p, nil
	case diffLoadedMsg:
		// Only apply if the loaded diff matches current request.
		if msg.path == p.filePath && msg.err == nil {
			p.diffLines = msg.lines
		}
		return p, nil
	case panels.IssueSelectedMsg:
		p.clearSelection()
		p.ghMode = true
		p.ghPlainText = false
		p.ghTitle = fmt.Sprintf("#%d %s", msg.Number, ansi.Strip(msg.Title))
		p.ghContent = msg.Body
		if p.ghContent == "" {
			p.ghContent = "*No description provided.*"
		}
		p.scrollY = 0
		p.lines = markdown.RenderStatic(p.ghContent, p.width)
		return p, nil
	case panels.IssueDeselectedMsg:
		p.ghMode = false
		p.ghTitle = ""
		p.ghContent = ""
		p.ghPlainText = false
		if p.filePath != "" {
			return p, p.loadFileCmd(p.filePath)
		}
		return p, nil
	case panels.PRSelectedMsg:
		p.clearSelection()
		p.ghMode = true
		p.ghPlainText = false
		safeTitle := ansi.Strip(msg.Title)
		safeBranch := ansi.Strip(msg.HeadBranch)
		safeState := ansi.Strip(msg.State)
		p.ghTitle = fmt.Sprintf("PR #%d %s", msg.Number, safeTitle)
		content := fmt.Sprintf("# PR #%d\n\n**%s**\n\nBranch: `%s`\nState: %s", msg.Number, safeTitle, safeBranch, safeState)
		p.ghContent = content
		p.scrollY = 0
		p.lines = markdown.RenderStatic(content, p.width)
		return p, nil
	case panels.PRDeselectedMsg:
		p.ghMode = false
		p.ghTitle = ""
		p.ghContent = ""
		p.ghPlainText = false
		if p.filePath != "" {
			return p, p.loadFileCmd(p.filePath)
		}
		return p, nil
	case panels.ActionRunSelectedMsg:
		p.clearSelection()
		p.ghMode = true
		p.ghPlainText = false
		safeWfName := ansi.Strip(msg.WorkflowName)
		safeStatus := ansi.Strip(msg.Status)
		p.ghTitle = fmt.Sprintf("%s (Run #%d)", safeWfName, msg.RunID)
		content := fmt.Sprintf("# %s\n\nStatus: %s\nRun ID: %d", safeWfName, safeStatus, msg.RunID)
		p.ghContent = content
		p.scrollY = 0
		p.lines = markdown.RenderStatic(content, p.width)
		return p, nil
	case panels.ActionRunDeselectedMsg:
		p.ghMode = false
		p.ghTitle = ""
		p.ghContent = ""
		p.ghPlainText = false
		if p.filePath != "" {
			return p, p.loadFileCmd(p.filePath)
		}
		return p, nil
	case panels.WorkflowSelectedMsg:
		// Show the workflow definition file in the preview pane.
		p.clearSelection()
		p.ghMode = false
		p.ghTitle = ""
		p.ghContent = ""
		p.ghPlainText = false
		p.filePath = msg.Path
		p.scrollY = 0
		p.err = nil
		p.isBinary = false
		p.isLarge = false
		p.lines = nil
		p.loading = true
		p.blameMode = false
		p.blameLines = nil
		p.diffLines = nil
		return p, p.loadFileCmd(msg.Path)
	case panels.ActionJobsLoadedMsg:
		p.clearSelection()
		p.ghMode = true
		p.ghPlainText = true
		p.scrollY = 0
		p.ghContent = renderActionJobs(msg.Jobs)
		p.lines = strings.Split(p.ghContent, "\n")
		return p, nil
	case panels.ActionLogMsg:
		if p.ghMode && msg.Log != "" {
			p.ghPlainText = true
			logSection := renderActionLog(msg.Log)
			p.ghContent += "\n" + logSection
			p.lines = strings.Split(p.ghContent, "\n")
		}
		return p, nil
	case panels.GitFilterActiveMsg:
		p.diffMode = msg.Active
		if msg.Active {
			p.diffContext = &panels.DiffContext{Type: panels.DiffContextWorking}
		}
		return p, nil
	case panels.RefreshPreviewMsg:
		// Re-render whatever is currently showing without changing content type.
		if p.ghMode {
			if p.ghContent != "" {
				if p.ghPlainText {
					p.lines = strings.Split(p.ghContent, "\n")
				} else {
					p.lines = markdown.RenderStatic(p.ghContent, p.width)
				}
			}
			return p, nil
		}
		if p.filePath != "" {
			p.loading = true
			p.lines = nil
			cmds := []tea.Cmd{p.loadFileCmd(p.filePath)}
			if p.gitClient != nil {
				cmds = append(cmds, p.loadContextDiffCmd(p.filePath, p.diffContext))
			}
			return p, tea.Batch(cmds...)
		}
		return p, nil
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	case panels.PreviewScrollMsg:
		if msg.Delta > 0 {
			p.scrollDown(msg.Delta)
		} else if msg.Delta < 0 {
			p.scrollUp(-msg.Delta)
		}
		return p, nil
	case tea.MouseWheelMsg:
		m := msg.Mouse()
		switch m.Button {
		case tea.MouseWheelUp:
			p.scrollUp(3)
		case tea.MouseWheelDown:
			p.scrollDown(3)
		}
		return p, nil
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseMotionMsg:
		return p.handleMouseMotion(msg)
	case panels.PanelMouseReleaseMsg:
		return p.handleMouseRelease(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleDoubleClick(msg)
	case panels.BlameLoadedMsg:
		if msg.Err != nil {
			p.blameMode = false
			p.blameLines = nil
		} else {
			p.blameLines = msg.Lines
		}
		return p, nil
	case tea.KeyPressMsg:
		if !p.focused {
			return p, nil
		}
		// Edit mode keyboard handling takes priority.
		if p.editMode {
			return handleEditKeyPress(p, msg)
		}
		switch msg.String() {
		case "e":
			// Edit is only allowed in file mode (not diff) and for local files.
			if p.diffMode {
				return p, nil
			}
			return p, enterEditMode(p)
		case "f":
			p.diffMode = !p.diffMode
			p.scrollY = 0
			return p, nil
		case "j", keyDown:
			p.scrollDown(1)
		case "k", "up":
			p.scrollUp(1)
		case "pgdown":
			p.scrollDown(p.viewportHeight())
		case "pgup":
			p.scrollUp(p.viewportHeight())
		case "g":
			p.scrollY = 0
		case "G":
			p.scrollToBottom()
		case "W":
			p.wordWrap = !p.wordWrap
		case "n":
			p.lineNumbers = !p.lineNumbers
		case "m":
			p.renderMarkdown = !p.renderMarkdown
			if p.filePath != "" && isMarkdownExt(filepath.Ext(p.filePath)) {
				return p, p.loadFileCmd(p.filePath)
			}
		case "B":
			if p.filePath != "" {
				p.blameMode = !p.blameMode
				if p.blameMode {
					return p, func() tea.Msg {
						return panels.ToggleBlameMsg{Path: p.filePath}
					}
				}
				p.blameLines = nil
			}
		case "y":
			if p.hasSelection() {
				return p.copySelection()
			}
		case "escape", "esc":
			if p.hasSelection() {
				p.clearSelection()
			}
		}
	}
	return p, nil
}

// View implements panels.Panel. It renders the preview content into the
// given width×height area.
func (p *Preview) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	// Loading state
	if p.loading {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#666666")).
			Render("Loading...")
	}
	// Empty state — skip when in GitHub content mode (ghMode has its own lines).
	if p.filePath == "" && !p.ghMode {
		return p.renderEmptyState(width, height)
	}
	// Error state
	if p.err != nil {
		return p.renderError(width, height)
	}
	// Binary or large file
	if p.isBinary || p.isLarge {
		return p.renderMetadata(width, height)
	}
	// Edit mode rendering
	if p.editMode && p.editBuf != nil {
		return renderEditContent(p, width, height)
	}
	// Blame mode
	if p.blameMode && len(p.blameLines) > 0 {
		return p.renderBlameContent(width, height)
	}
	return p.renderContent(width, height)
}

// Focus implements panels.Panel.
func (p *Preview) Focus() {
	p.focused = true
}

// Blur implements panels.Panel.
func (p *Preview) Blur() {
	p.focused = false
}

// SetSize implements panels.Panel.
func (p *Preview) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// FilePath returns the path of the currently displayed file (empty if none).
func (p *Preview) FilePath() string {
	return p.filePath
}

// Title implements panels.Panel. Returns the filename when a file is loaded,
// the GitHub item title when in ghMode, or "preview" when nothing is selected.
func (p *Preview) Title() string {
	if p.ghMode && p.ghTitle != "" {
		return p.ghTitle
	}
	if p.filePath == "" {
		return "preview"
	}
	name := filepath.Base(p.filePath)
	if p.editMode && p.editBuf != nil && p.editBuf.Dirty() {
		return name + " [+]"
	}
	if p.editMode {
		return name + " [edit]"
	}
	if p.diffMode {
		return name + " [diff]"
	}
	return name
}

// KeyBindings implements panels.Panel.
func (p *Preview) KeyBindings() []panels.KeyBinding {
	if p.editMode {
		return []panels.KeyBinding{
			{Key: "Esc", Description: "Exit edit mode", Action: "exit_edit"},
			{Key: "Ctrl+S", Description: "Save file", Action: "save"},
			{Key: "Ctrl+Z", Description: "Undo", Action: "undo"},
			{Key: "Ctrl+Y", Description: "Redo", Action: "redo"},
			{Key: "Ctrl+C", Description: "Copy", Action: "copy"},
			{Key: "Ctrl+X", Description: "Cut", Action: "cut"},
			{Key: "Ctrl+V", Description: "Paste", Action: "paste"},
			{Key: "Ctrl+A", Description: "Select all", Action: "select_all"},
			{Key: "Shift+Arrows", Description: "Select text", Action: "select"},
			{Key: "Ctrl+D", Description: "Duplicate line", Action: "duplicate_line"},
			{Key: "Ctrl+Shift+K", Description: "Delete line", Action: "delete_line"},
			{Key: "Alt+\u2191/\u2193", Description: "Move line", Action: "move_line"},
			{Key: "Ctrl+\u2190/\u2192", Description: "Word jump", Action: "word_nav"},
			{Key: "Tab", Description: "Insert indent", Action: "indent"},
			{Key: "Shift+Tab", Description: "Remove indent", Action: "dedent"},
		}
	}
	return []panels.KeyBinding{
		{Key: "e", Description: "Edit file", Action: "edit"},
		{Key: "f", Description: "Toggle diff view", Action: "toggle_diff_mode"},
		{Key: "j/↓", Description: "Scroll down", Action: "scroll_down"},
		{Key: "k/↑", Description: "Scroll up", Action: "scroll_up"},
		{Key: "PgDn", Description: "Page down", Action: "page_down"},
		{Key: "PgUp", Description: "Page up", Action: "page_up"},
		{Key: "g", Description: "Go to top", Action: "goto_top"},
		{Key: "G", Description: "Go to bottom", Action: "goto_bottom"},
		{Key: "W", Description: "Toggle word wrap", Action: "toggle_wrap"},
		{Key: "n", Description: "Toggle line numbers", Action: "toggle_line_numbers"},
		{Key: "m", Description: "Toggle markdown render", Action: "toggle_markdown_render"},
		{Key: "B", Description: "Toggle blame", Action: "toggle_blame"},
		{Key: "y/Ctrl+C", Description: "Copy selection", Action: "copy_selection"},
	}
}

// --- File loading ---
// loadFileCmd returns a tea.Cmd that loads a file asynchronously (F01).
// The file I/O, MIME detection, and syntax highlighting all happen in the
// background goroutine managed by Bubble Tea's command system.
func (p *Preview) loadFileCmd(path string) tea.Cmd {
	cfg := p.cfg
	width := p.width
	renderMD := p.renderMarkdown
	return func() tea.Msg {
		result := fileLoadedMsg{path: path}
		info, err := os.Stat(path)
		if err != nil {
			result.err = err
			return result
		}
		// Reject directories
		if info.IsDir() {
			result.lines = []string{fmt.Sprintf("Directory: %s", path)}
			return result
		}
		// Check max file size
		if cfg.GetMaxFileSize() > 0 && info.Size() > int64(cfg.GetMaxFileSize()) {
			result.isLarge = true
			result.lines = buildMetadataLines(path, info)
			return result
		}
		// Detect binary content via MIME type using only the first 512 bytes
		// to avoid reading potentially large files just for type detection.
		f, err := os.Open(path)
		if err != nil {
			result.err = err
			return result
		}
		header := make([]byte, 512)
		n, readErr := f.Read(header)
		f.Close()
		if readErr != nil && n == 0 {
			result.err = readErr
			return result
		}
		header = header[:n]
		mime := mimetype.Detect(header)
		if !isTextMIME(mime.String()) {
			result.isBinary = true
			result.lines = append(buildMetadataLines(path, info), "", "Type: "+mime.String())
			return result
		}
		// Read file content
		data, err := os.ReadFile(path)
		if err != nil {
			result.err = err
			return result
		}
		// Defence-in-depth: reject files that passed the 512-byte MIME
		// check but contain null bytes (polyglot / binary-after-header).
		if bytes.ContainsRune(data, 0) {
			result.isBinary = true
			result.lines = append(buildMetadataLines(path, info), "", "Type: binary (null bytes detected)")
			return result
		}
		// Normalize line endings so \r doesn't corrupt rendering
		// (lipgloss Width padding after \r overwrites content).
		source := strings.ReplaceAll(string(data), "\r\n", "\n")
		source = strings.ReplaceAll(source, "\r", "\n")
		ext := strings.ToLower(filepath.Ext(path))
		// Render based on file type
		switch ext {
		case extMD, extMarkdown, extMdown, extMkd:
			if renderMD {
				result.lines = markdown.RenderStatic(source, width)
			} else if cfg.GetSyntaxHighlighting() {
				result.lines = renderHighlightedStatic(source, path, cfg.GetTheme())
			} else {
				result.lines = strings.Split(source, "\n")
			}
		default:
			if cfg.GetSyntaxHighlighting() {
				result.lines = renderHighlightedStatic(source, path, cfg.GetTheme())
			} else {
				result.lines = strings.Split(source, "\n")
			}
		}
		return result
	}
}

// loadDiffCmd returns a tea.Cmd that loads the git diff for a file asynchronously.
// Diff lines are pre-rendered with ANSI colors for added/removed/context lines.
// It tries unstaged first, then falls back to staged diff.
func (p *Preview) loadDiffCmd(path string) tea.Cmd {
	gc := p.gitClient
	tc := p.themeColors()
	return func() tea.Msg {
		ctx := context.Background()
		// Try unstaged diff first, then staged.
		diffs, err := gc.Diff(ctx, git.DiffOpts{Path: path})
		if err != nil || len(diffs) == 0 {
			diffs, err = gc.Diff(ctx, git.DiffOpts{Path: path, Staged: true})
		}
		if err != nil || len(diffs) == 0 {
			return diffLoadedMsg{path: path}
		}
		return diffLoadedMsg{path: path, lines: renderDiffLines(diffs, tc)}
	}
}

// loadContextDiffCmd loads the diff for a file using the provided DiffContext.
// If dc is nil or Working type, it falls back to loadDiffCmd (unstaged/staged).
// For Commit/Branch/PR contexts, it loads the ref-based diff.
func (p *Preview) loadContextDiffCmd(path string, dc *panels.DiffContext) tea.Cmd {
	if dc == nil || dc.Type == panels.DiffContextWorking {
		return p.loadDiffCmd(path)
	}
	if dc.Type == panels.DiffContextStaged {
		gc := p.gitClient
		tc := p.themeColors()
		return func() tea.Msg {
			ctx := context.Background()
			diffs, err := gc.Diff(ctx, git.DiffOpts{Path: path, Staged: true})
			if err != nil || len(diffs) == 0 {
				return diffLoadedMsg{path: path}
			}
			return diffLoadedMsg{path: path, lines: renderDiffLines(diffs, tc)}
		}
	}
	// Commit, Branch, or PR with refs: use ref-based diff.
	gc := p.gitClient
	tc := p.themeColors()
	commitA := dc.CommitA
	commitB := dc.CommitB
	threeDot := dc.ThreeDot
	return func() tea.Msg {
		ctx := context.Background()
		// Convert absolute path to repo-relative for git diff.
		relPath := path
		if root, err := gc.RepoRoot(ctx); err == nil {
			if rel, err := filepath.Rel(root, path); err == nil {
				relPath = filepath.ToSlash(rel)
			}
		}
		diffs, err := gc.Diff(ctx, git.DiffOpts{
			Path:     relPath,
			CommitA:  commitA,
			CommitB:  commitB,
			ThreeDot: threeDot,
		})
		if err != nil || len(diffs) == 0 {
			return diffLoadedMsg{path: path}
		}
		return diffLoadedMsg{path: path, lines: renderDiffLines(diffs, tc)}
	}
}

// renderDiffLines converts parsed diff hunks to styled display lines.
func renderDiffLines(diffs []git.FileDiff, tc theme.Colors) []string {
	addedStyle := lipgloss.NewStyle().Foreground(panels.ColorOf(tc.DiffAdded, "#6B9E56"))
	removedStyle := lipgloss.NewStyle().Foreground(panels.ColorOf(tc.DiffRemoved, "#C44B4B"))
	headerStyle := lipgloss.NewStyle().Foreground(panels.ColorOf(tc.DiffHeader, "#7A9EBF"))
	var lines []string
	for _, d := range diffs {
		for _, h := range d.Hunks {
			lines = append(lines, headerStyle.Render(h.Header))
			for _, l := range h.Lines {
				switch l.Type {
				case git.DiffLineAdded:
					lines = append(lines, addedStyle.Render(l.Content))
				case git.DiffLineRemoved:
					lines = append(lines, removedStyle.Render(l.Content))
				default:
					lines = append(lines, l.Content)
				}
			}
		}
	}
	return lines
}

// renderHighlightedStatic applies chroma syntax highlighting to source code.
// This is a pure function safe for concurrent use in tea.Cmd goroutines.
func renderHighlightedStatic(source, filename, theme string) []string {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	if theme == "" {
		theme = "dracula"
	}
	style := styles.Get(theme)
	if style == nil {
		style = styles.Fallback
	}
	// Use truecolor (24-bit) for rich, accurate colors.
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Get("terminal256")
	}
	if formatter == nil {
		return strings.Split(source, "\n")
	}
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return strings.Split(source, "\n")
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return strings.Split(source, "\n")
	}
	highlighted := buf.String()
	// Remove trailing newline to avoid an extra empty line
	highlighted = strings.TrimRight(highlighted, "\n")
	return strings.Split(highlighted, "\n")
}

// ---------------------------------------------------------------------------
// Action run job/step rendering
// ---------------------------------------------------------------------------
// statusIcon returns a clean Unicode icon for a job or step status/conclusion.
func statusIcon(status, conclusion string) string {
	switch conclusion {
	case statusSuccess:
		return "✓"
	case statusFailure:
		return "✗"
	case "cancelled":
		return "⊘"
	case "skipped":
		return "⊘"
	}
	switch status {
	case statusInProgress:
		return "●"
	case "queued", "waiting", "pending":
		return "○"
	case statusCompleted:
		return "✓"
	}
	return "○"
}

// formatDuration computes a human-readable duration string from ISO 8601 timestamps.
// Returns "" if either timestamp is empty or unparseable.
func formatDuration(startedAt, completedAt string) string {
	if startedAt == "" || completedAt == "" {
		return ""
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		// Try the format without timezone abbreviation.
		start, err = time.Parse("2006-01-02T15:04:05Z", startedAt)
		if err != nil {
			return ""
		}
	}
	end, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		end, err = time.Parse("2006-01-02T15:04:05Z", completedAt)
		if err != nil {
			return ""
		}
	}
	d := end.Sub(start)
	if d < 0 {
		return ""
	}
	totalSec := int(d.Seconds())
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}
	minutes := totalSec / 60
	sec := totalSec % 60
	return fmt.Sprintf("%dm %02ds", minutes, sec)
}

// renderActionJobs builds a plain-text display of jobs and their steps.
func renderActionJobs(jobs []panels.ActionJob) string {
	if len(jobs) == 0 {
		return "  No jobs found."
	}
	var b strings.Builder
	b.WriteString("Jobs\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	for _, job := range jobs {
		icon := statusIcon(job.Status, job.Conclusion)
		dur := formatDuration(job.StartedAt, job.CompletedAt)
		line := fmt.Sprintf("  %s %s", icon, job.Name)
		if dur != "" {
			line += "  " + dur
		}
		b.WriteString(line)
		b.WriteString("\n")
		// Render steps indented under the job.
		for _, step := range job.Steps {
			sIcon := statusIcon(step.Status, step.Conclusion)
			sLine := fmt.Sprintf("      %s %d. %s", sIcon, step.Number, step.Name)
			b.WriteString(sLine)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderActionLog formats raw job log output, truncating to the last 100 lines.
func renderActionLog(log string) string {
	const maxLines = 100
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	b.WriteString("Failed Job Log (last ")
	_, _ = fmt.Fprintf(&b, "%d", maxLines)
	b.WriteString(" lines)\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		b.WriteString("  … (truncated)\n")
	}
	for _, l := range lines {
		b.WriteString("  ")
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

func buildMetadataLines(path string, info os.FileInfo) []string {
	return []string{
		"File: " + filepath.Base(path),
		"Size: " + formatSize(info.Size()),
		"Mode: " + info.Mode().String(),
		"Modified: " + info.ModTime().Format(time.RFC3339),
	}
}

// --- Scrolling ---
func (p *Preview) scrollDown(n int) {
	p.scrollY += n
	p.clampScroll()
}

func (p *Preview) scrollUp(n int) {
	p.scrollY -= n
	if p.scrollY < 0 {
		p.scrollY = 0
	}
}

// contentLineCount returns the number of content lines for scrolling.
// In blame mode it uses blame data; otherwise the normal file lines.
func (p *Preview) contentLineCount() int {
	if p.blameMode && len(p.blameLines) > 0 {
		return len(p.blameLines)
	}
	if p.diffMode {
		return len(p.diffLines)
	}
	return len(p.lines)
}

func (p *Preview) scrollToBottom() {
	maxScroll := p.contentLineCount() - p.viewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	p.scrollY = maxScroll
}

func (p *Preview) clampScroll() {
	maxScroll := p.contentLineCount() - p.viewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if p.scrollY > maxScroll {
		p.scrollY = maxScroll
	}
	if p.scrollY < 0 {
		p.scrollY = 0
	}
}

func (p *Preview) viewportHeight() int {
	h := p.height
	if h <= 0 {
		h = 20
	}
	// Reserve one line for scroll indicator
	if h > 1 {
		return h - 1
	}
	return h
}

// --- Rendering ---
// newDimStyle creates the dim style for line numbers and indicators.
// Created as a local value to avoid package-level mutable state (F23).
func (p *Preview) newDimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#555555"))
}

func (p *Preview) renderEmptyState(width, height int) string {
	msg := "No file selected\n\nSelect a file to preview"
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#666666"))
	return style.Render(msg)
}

func (p *Preview) renderCenteredMessage(width, height int, msg string) string {
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(panels.ColorOf(p.themeColors().BrightBlack, "#666666"))
	return style.Render(msg)
}

func (p *Preview) renderError(width, height int) string {
	msg := fmt.Sprintf("Error: %s", p.err)
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(panels.ColorOf(p.themeColors().DiffRemoved, "#C44B4B"))
	return style.Render(msg)
}

func (p *Preview) renderMetadata(width, height int) string {
	var header string
	if p.isBinary {
		header = "Binary file"
	} else {
		header = "File too large"
	}
	metaLines := append([]string{header, ""}, p.lines...)
	content := strings.Join(metaLines, "\n")
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(panels.ColorOf(p.themeColors().FileDefault, "#888888"))
	return style.Render(content)
}

func (p *Preview) renderContent(width, height int) string {
	// Build display lines based on mode.
	var displayLines []string
	if p.diffMode {
		// Diff mode: show only the contextual diff.
		if len(p.diffLines) == 0 {
			return p.renderCenteredMessage(width, height, "No changes")
		}
		displayLines = p.diffLines
	} else {
		// File mode: show only file content.
		displayLines = p.lines
	}
	if len(displayLines) == 0 {
		return p.renderEmptyState(width, height)
	}
	p.clampScroll()
	totalLines := len(displayLines)
	// Reserve one line for scroll indicator
	contentHeight := height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	// Calculate visible range
	start := p.scrollY
	end := start + contentHeight
	if end > totalLines {
		end = totalLines
	}
	visible := make([]string, end-start)
	copy(visible, displayLines[start:end])
	// Calculate line number width
	numWidth := 0
	if p.lineNumbers {
		numWidth = len(fmt.Sprintf("%d", totalLines))
		if numWidth < 3 {
			numWidth = 3
		}
	}
	// Content width for wrapping
	contentWidth := width
	if p.lineNumbers {
		contentWidth = width - numWidth - 3 // "NNN │ "
	}
	if contentWidth < 1 {
		contentWidth = 1
	}
	// Build rendered lines
	sel, selE := p.selRange()
	rendered := make([]string, 0, len(visible))
	for i, line := range visible {
		lineNum := start + i + 1
		absLine := start + i
		// Expand tabs so truncation measures display width correctly.
		line = strings.ReplaceAll(line, "\t", "    ")
		// Apply selection highlight before truncation/wrapping so
		// the highlight covers the correct rune range.
		line = p.applySelectionHighlight(line, absLine, sel, selE)
		if p.wordWrap && contentWidth > 0 {
			line = lipgloss.NewStyle().Width(contentWidth).Render(line)
		} else {
			line = ansi.Truncate(line, contentWidth, "")
		}
		if p.lineNumbers {
			numStr := fmt.Sprintf("%*d │ ", numWidth, lineNum)
			line = p.newDimStyle().Render(numStr) + line
		}
		// Final hard-truncate to panel width so lipgloss Width() in the
		// outer container never wraps any line.
		line = ansi.Truncate(line, width, "")
		rendered = append(rendered, line)
	}
	// Pad with empty lines if needed
	for len(rendered) < contentHeight {
		if p.lineNumbers {
			rendered = append(rendered, p.newDimStyle().Render(strings.Repeat(" ", numWidth+3)))
		} else {
			rendered = append(rendered, "")
		}
	}
	content := strings.Join(rendered, "\n")
	// Add scroll indicator
	scrollInfo := p.scrollIndicator(totalLines, height)
	scrollLine := ansi.Truncate(p.newDimStyle().Render(scrollInfo), width, "")
	content += "\n" + scrollLine
	return content
}

func (p *Preview) scrollIndicator(totalLines, viewHeight int) string {
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
	if p.scrollY == 0 {
		return "Top"
	}
	if p.scrollY >= maxScroll {
		return "Bot"
	}
	pct := p.scrollY * 100 / maxScroll
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%d%%", pct)
}

// --- Helpers ---
// isMarkdownExt returns true if the file extension indicates a markdown file.
func isMarkdownExt(ext string) bool {
	switch strings.ToLower(ext) {
	case extMD, extMarkdown, extMdown, extMkd:
		return true
	}
	return false
}

// isTextMIME returns true if the MIME type indicates text content.
func isTextMIME(mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	// Common text-based MIME types that don't start with "text/"
	textMIMEs := []string{
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-sh",
		"application/x-shellscript",
		"application/toml",
		"application/yaml",
		"application/x-yaml",
		"application/xhtml+xml",
		"application/svg+xml",
		"application/graphql",
		"application/ld+json",
	}
	for _, tm := range textMIMEs {
		if strings.HasPrefix(mimeType, tm) {
			return true
		}
	}
	return false
}

// formatSize formats a byte count as a human-readable string.
func formatSize(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
