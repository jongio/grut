// editor.go provides inline edit-mode logic for the preview panel.
// It manages cursor movement, text input, undo/redo dispatch, and
// the atomic save flow. Functions here are called from Preview.Update
// when editMode is true.
package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// Key name constants for KeyPressMsg.String() comparisons.
const (
	keyDown = "down"
)

// fileSavedMsg is sent after a successful file save from edit mode.
// The Update handler reacts by emitting FileModifiedMsg, refreshing
// the diff, and showing a toast.
type fileSavedMsg struct {
	path string
}

// ---------------------------------------------------------------------------
// Mode transitions
// ---------------------------------------------------------------------------

// enterEditMode reads the file from disk, creates a TextBuffer, and places
// the cursor at the current scroll position. Returns a cmd that emits
// EditModeEnteredMsg. Returns nil if the file cannot be edited (binary,
// oversized, GitHub content, or empty path).
func enterEditMode(p *Preview) tea.Cmd {
	if p.isBinary || p.isLarge || p.ghMode || p.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Cannot edit: " + err.Error(),
				Level:   notify.Error,
			}
		}
	}

	// Normalize line endings: strip \r so CRLF files don't corrupt
	// the terminal rendering (lipgloss Width padding after \r overwrites
	// content from column 0).
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	p.editBuf = NewTextBuffer(strings.Split(content, "\n"))
	p.editMode = true
	p.cursorLine = p.scrollY
	p.cursorCol = 0
	p.clearSelection()

	filePath := p.filePath
	return func() tea.Msg {
		return panels.EditModeEnteredMsg{Path: filePath}
	}
}

// exitEditMode leaves edit mode, restoring the scroll position and reloading
// the file with syntax highlighting. If discard is false and the buffer has
// unsaved changes, a dirty-guard modal is shown instead.
func exitEditMode(p *Preview, discard bool) tea.Cmd {
	if !p.editMode {
		return nil
	}
	if !discard && p.editBuf != nil && p.editBuf.Dirty() {
		return dirtyGuardCmd(p, "exit")
	}

	p.editMode = false
	p.scrollY = p.cursorLine
	p.editBuf = nil

	filePath := p.filePath
	cmds := []tea.Cmd{
		func() tea.Msg {
			return panels.EditModeExitedMsg{Path: filePath}
		},
		p.loadFileCmd(filePath),
	}
	if p.gitClient != nil {
		cmds = append(cmds, p.loadDiffCmd(filePath))
	}
	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Dirty guard modal
// ---------------------------------------------------------------------------

// dirtyGuardCmd shows a modal asking the user how to handle unsaved changes.
// The action string (e.g. "exit") is encoded in the action option IDs so that
// handleModalResult can determine what to do after the user responds.
func dirtyGuardCmd(p *Preview, action string) tea.Cmd {
	baseName := filepath.Base(p.filePath)
	return func() tea.Msg {
		return notify.ShowModalMsg{
			Title:   "Unsaved Changes",
			Message: "Save changes to " + baseName + "?",
			Kind:    notify.ModalActionPicker,
			Actions: []notify.ActionOption{
				{ID: "save_" + action, Label: "Save & " + action},
				{ID: "discard_" + action, Label: "Discard"},
				{ID: actionCancel, Label: "Cancel"},
			},
		}
	}
}

// handleModalResult processes the user's response to the dirty-guard modal.
// The selected action's ID encodes both the intent (save/discard/cancel) and
// the original action (e.g. "exit").
func handleModalResult(p *Preview, msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	if !msg.Accept {
		// User cancelled (pressed Esc or chose cancel).
		return p, nil
	}
	value := msg.Value
	switch {
	case value == actionCancel:
		return p, nil
	case strings.HasPrefix(value, "save_"):
		// Save, then perform the action.
		return p, tea.Batch(saveFile(p), exitEditMode(p, true))
	case strings.HasPrefix(value, "discard_"):
		return p, exitEditMode(p, true)
	default:
		return p, nil
	}
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

// saveFile performs an atomic write-to-temp-then-rename save of the edit
// buffer contents. Returns a cmd that produces fileSavedMsg on success or
// a toast on error.
func saveFile(p *Preview) tea.Cmd {
	buf := p.editBuf
	filePath := p.filePath
	if buf == nil || filePath == "" {
		return nil
	}
	content := strings.Join(buf.Lines(), "\n")
	return func() tea.Msg {
		dir := filepath.Dir(filePath)
		tmp, err := os.CreateTemp(dir, ".grut-tmp-*")
		if err != nil {
			return notify.ShowToastMsg{
				Message: "Save failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		if _, err := tmp.WriteString(content); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return notify.ShowToastMsg{
				Message: "Save failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			return notify.ShowToastMsg{
				Message: "Save failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		if err := os.Rename(tmp.Name(), filePath); err != nil {
			os.Remove(tmp.Name())
			return notify.ShowToastMsg{
				Message: "Save failed: " + err.Error(),
				Level:   notify.Error,
			}
		}
		return fileSavedMsg{path: filePath}
	}
}

// handleFileSaved processes a successful save by marking the buffer clean,
// emitting a toast, broadcasting FileModifiedMsg, and refreshing the diff.
func handleFileSaved(p *Preview, msg fileSavedMsg) (panels.Panel, tea.Cmd) {
	if p.editBuf != nil {
		p.editBuf.MarkClean()
	}
	cmds := []tea.Cmd{
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Saved " + filepath.Base(msg.path),
				Level:   notify.Info,
			}
		},
		func() tea.Msg {
			return panels.FileModifiedMsg{Path: msg.path}
		},
	}
	if p.gitClient != nil {
		cmds = append(cmds, p.loadDiffCmd(msg.path))
	}
	return p, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Keyboard handling
// ---------------------------------------------------------------------------

// handleEditKeyPress dispatches keyboard input while the preview panel is in
// edit mode. It handles movement, editing operations, undo/redo, and mode
// transitions.
func handleEditKeyPress(p *Preview, msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	key := msg.String()
	switch key {
	// --- Mode transitions ---
	case "escape", "esc":
		if p.editBuf != nil && p.editBuf.Dirty() {
			return p, dirtyGuardCmd(p, "exit")
		}
		return p, exitEditMode(p, true)

	case "ctrl+s":
		return p, saveFile(p)

	// --- Undo / redo ---
	case "ctrl+z":
		if p.editBuf != nil {
			clearEditSelection(p)
			newLine, newCol, ok := p.editBuf.Undo(p.cursorLine, p.cursorCol)
			if ok {
				p.cursorLine = newLine
				p.cursorCol = newCol
				ensureCursorVisible(p)
			}
		}

	case "ctrl+y":
		if p.editBuf != nil {
			clearEditSelection(p)
			newLine, newCol, ok := p.editBuf.Redo(p.cursorLine, p.cursorCol)
			if ok {
				p.cursorLine = newLine
				p.cursorCol = newCol
				ensureCursorVisible(p)
			}
		}

	// --- Line manipulation ---
	case "ctrl+d":
		if p.editBuf != nil {
			p.editBuf.DuplicateLine(p.cursorLine)
			p.cursorLine++
			ensureCursorVisible(p)
		}

	case "enter":
		if p.editBuf != nil {
			p.editBuf.SplitLine(p.cursorLine, p.cursorCol, p.editCfg.AutoIndent)
			p.cursorLine++
			if p.editCfg.AutoIndent {
				newLine := p.editBuf.Line(p.cursorLine)
				p.cursorCol = countLeadingSpaces(newLine)
			} else {
				p.cursorCol = 0
			}
			ensureCursorVisible(p)
		}

	case "backspace":
		if p.editBuf != nil {
			if hasEditSelection(p) {
				start, end := editSelRange(p)
				p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(start.Line, start.Col, end.Line, end.Col)
				clearEditSelection(p)
			} else {
				newLine, newCol := p.editBuf.DeleteRune(p.cursorLine, p.cursorCol)
				p.cursorLine = newLine
				p.cursorCol = newCol
			}
			ensureCursorVisible(p)
		}

	case "delete":
		if p.editBuf != nil {
			if hasEditSelection(p) {
				start, end := editSelRange(p)
				p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(start.Line, start.Col, end.Line, end.Col)
				clearEditSelection(p)
			} else {
				p.editBuf.DeleteForward(p.cursorLine, p.cursorCol)
			}
		}

	// --- Indentation ---
	case "tab":
		if p.editBuf != nil {
			p.cursorCol = p.editBuf.InsertTab(p.cursorLine, p.cursorCol, p.editCfg.TabSize)
			ensureCursorVisible(p)
		}

	case "shift+tab":
		if p.editBuf != nil {
			p.editBuf.Dedent(p.cursorLine, p.editCfg.TabSize)
			line := p.editBuf.Line(p.cursorLine)
			lineRunes := []rune(line)
			if p.cursorCol > len(lineRunes) {
				p.cursorCol = len(lineRunes)
			}
		}

	// --- Selection (shift+arrow) ---
	case "shift+left":
		extendEditSelection(p, p.cursorLine, p.cursorCol)
		moveCursorLeft(p)
		p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}

	case "shift+right":
		extendEditSelection(p, p.cursorLine, p.cursorCol)
		moveCursorRight(p)
		p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}

	case "shift+up":
		extendEditSelection(p, p.cursorLine, p.cursorCol)
		moveCursorUp(p)
		p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}

	case "shift+down":
		extendEditSelection(p, p.cursorLine, p.cursorCol)
		moveCursorDown(p)
		p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}

	case "shift+home":
		extendEditSelection(p, p.cursorLine, p.cursorCol)
		p.cursorCol = 0
		p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}

	case "shift+end":
		if p.editBuf != nil {
			extendEditSelection(p, p.cursorLine, p.cursorCol)
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = len([]rune(line))
			p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}
		}

	case "ctrl+shift+left":
		if p.editBuf != nil {
			extendEditSelection(p, p.cursorLine, p.cursorCol)
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = findWordBoundaryLeft(line, p.cursorCol)
			p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}
		}

	case "ctrl+shift+right":
		if p.editBuf != nil {
			extendEditSelection(p, p.cursorLine, p.cursorCol)
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = findWordBoundaryRight(line, p.cursorCol)
			p.selEnd = &selPoint{Line: p.cursorLine, Col: p.cursorCol}
		}

	// --- Clipboard ---
	case "ctrl+c":
		if p.editBuf != nil {
			var text string
			if hasEditSelection(p) {
				text = editSelectedText(p)
			} else {
				// No selection: copy current line (VS Code behavior).
				text = p.editBuf.Line(p.cursorLine) + "\n"
			}
			if text != "" {
				_ = panels.CopyToClipboard(context.Background(), text)
			}
		}

	case "ctrl+x":
		if p.editBuf != nil {
			if hasEditSelection(p) {
				text := editSelectedText(p)
				_ = panels.CopyToClipboard(context.Background(), text)
				start, end := editSelRange(p)
				p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(start.Line, start.Col, end.Line, end.Col)
				clearEditSelection(p)
				ensureCursorVisible(p)
			} else {
				// No selection: cut entire current line (VS Code behavior).
				text := p.editBuf.Line(p.cursorLine) + "\n"
				_ = panels.CopyToClipboard(context.Background(), text)
				p.editBuf.DeleteLine(p.cursorLine)
				if p.cursorLine >= p.editBuf.LineCount() {
					p.cursorLine = p.editBuf.LineCount() - 1
				}
				clampCursorCol(p)
				ensureCursorVisible(p)
			}
		}

	case "ctrl+v":
		if p.editBuf != nil {
			text, err := panels.PasteFromClipboard(context.Background())
			if err == nil && text != "" {
				// Replace selection if any.
				if hasEditSelection(p) {
					start, end := editSelRange(p)
					p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(start.Line, start.Col, end.Line, end.Col)
					clearEditSelection(p)
				}
				p.cursorLine, p.cursorCol = p.editBuf.InsertText(p.cursorLine, p.cursorCol, text)
				ensureCursorVisible(p)
			}
		}

	// --- Word navigation & deletion ---
	case "ctrl+left":
		if p.editBuf != nil {
			clearEditSelection(p)
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = findWordBoundaryLeft(line, p.cursorCol)
		}

	case "ctrl+right":
		if p.editBuf != nil {
			clearEditSelection(p)
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = findWordBoundaryRight(line, p.cursorCol)
		}

	case "ctrl+backspace":
		if p.editBuf != nil {
			clearEditSelection(p)
			line := p.editBuf.Line(p.cursorLine)
			newCol := findWordBoundaryLeft(line, p.cursorCol)
			if newCol < p.cursorCol {
				p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(p.cursorLine, newCol, p.cursorLine, p.cursorCol)
			} else if p.cursorCol == 0 && p.cursorLine > 0 {
				// At start of line, join with previous (same as backspace).
				newLine, newCol := p.editBuf.DeleteRune(p.cursorLine, p.cursorCol)
				p.cursorLine = newLine
				p.cursorCol = newCol
			}
			ensureCursorVisible(p)
		}

	case "ctrl+delete":
		if p.editBuf != nil {
			clearEditSelection(p)
			line := p.editBuf.Line(p.cursorLine)
			endCol := findWordBoundaryRight(line, p.cursorCol)
			if endCol > p.cursorCol {
				p.editBuf.DeleteRange(p.cursorLine, p.cursorCol, p.cursorLine, endCol)
			} else if p.cursorCol >= len([]rune(line)) && p.cursorLine < p.editBuf.LineCount()-1 {
				// At end of line, join with next.
				p.editBuf.DeleteForward(p.cursorLine, p.cursorCol)
			}
		}

	// --- Line operations ---
	case "ctrl+shift+k":
		if p.editBuf != nil {
			clearEditSelection(p)
			p.editBuf.DeleteLine(p.cursorLine)
			if p.cursorLine >= p.editBuf.LineCount() {
				p.cursorLine = p.editBuf.LineCount() - 1
			}
			clampCursorCol(p)
			ensureCursorVisible(p)
		}

	case "alt+up":
		if p.editBuf != nil {
			clearEditSelection(p)
			if p.editBuf.MoveLine(p.cursorLine, -1) {
				p.cursorLine--
				ensureCursorVisible(p)
			}
		}

	case "alt+down":
		if p.editBuf != nil {
			clearEditSelection(p)
			if p.editBuf.MoveLine(p.cursorLine, 1) {
				p.cursorLine++
				ensureCursorVisible(p)
			}
		}

	// --- Cursor movement ---
	case "left":
		clearEditSelection(p)
		moveCursorLeft(p)

	case "right":
		clearEditSelection(p)
		moveCursorRight(p)

	case "up":
		clearEditSelection(p)
		moveCursorUp(p)

	case keyDown:
		clearEditSelection(p)
		moveCursorDown(p)

	case "ctrl+a":
		if p.editBuf != nil {
			selectAll(p)
			p.cursorLine = p.editBuf.LineCount() - 1
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = len([]rune(line))
			ensureCursorVisible(p)
		}

	case "home":
		clearEditSelection(p)
		p.cursorCol = 0

	case "end", "ctrl+e":
		clearEditSelection(p)
		if p.editBuf != nil {
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = len([]rune(line))
		}

	case "ctrl+home":
		clearEditSelection(p)
		p.cursorLine = 0
		p.cursorCol = 0
		ensureCursorVisible(p)

	case "ctrl+end":
		clearEditSelection(p)
		if p.editBuf != nil {
			p.cursorLine = p.editBuf.LineCount() - 1
			if p.cursorLine < 0 {
				p.cursorLine = 0
			}
			line := p.editBuf.Line(p.cursorLine)
			p.cursorCol = len([]rune(line))
			ensureCursorVisible(p)
		}

	default:
		// Character input - insert each rune from the text payload.
		if msg.Text != "" {
			if p.editBuf != nil {
				// Replace selection with typed text.
				if hasEditSelection(p) {
					start, end := editSelRange(p)
					p.cursorLine, p.cursorCol = p.editBuf.DeleteRange(start.Line, start.Col, end.Line, end.Col)
					clearEditSelection(p)
				}
				for _, r := range msg.Text {
					p.editBuf.InsertRune(p.cursorLine, p.cursorCol, r)
					p.cursorCol++
				}
				ensureCursorVisible(p)
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Cursor movement helpers
// ---------------------------------------------------------------------------

// moveCursorLeft moves the cursor one rune to the left, wrapping to the
// end of the previous line if at column 0.
func moveCursorLeft(p *Preview) {
	if p.cursorCol > 0 {
		p.cursorCol--
	} else if p.cursorLine > 0 {
		p.cursorLine--
		line := p.editBuf.Line(p.cursorLine)
		p.cursorCol = len([]rune(line))
		ensureCursorVisible(p)
	}
}

// moveCursorRight moves the cursor one rune to the right, wrapping to
// the start of the next line if at the end.
func moveCursorRight(p *Preview) {
	if p.editBuf == nil {
		return
	}
	line := p.editBuf.Line(p.cursorLine)
	lineLen := len([]rune(line))
	if p.cursorCol < lineLen {
		p.cursorCol++
	} else if p.cursorLine < p.editBuf.LineCount()-1 {
		p.cursorLine++
		p.cursorCol = 0
		ensureCursorVisible(p)
	}
}

// moveCursorUp moves the cursor one line up, clamping the column to
// the length of the target line.
func moveCursorUp(p *Preview) {
	if p.cursorLine > 0 {
		p.cursorLine--
		clampCursorCol(p)
		ensureCursorVisible(p)
	}
}

// moveCursorDown moves the cursor one line down, clamping the column to
// the length of the target line.
func moveCursorDown(p *Preview) {
	if p.editBuf == nil {
		return
	}
	if p.cursorLine < p.editBuf.LineCount()-1 {
		p.cursorLine++
		clampCursorCol(p)
		ensureCursorVisible(p)
	}
}

// clampCursorCol ensures the cursor column does not exceed the length
// of the current line.
func clampCursorCol(p *Preview) {
	if p.editBuf == nil {
		return
	}
	line := p.editBuf.Line(p.cursorLine)
	lineLen := len([]rune(line))
	if p.cursorCol > lineLen {
		p.cursorCol = lineLen
	}
}

// ensureCursorVisible adjusts scrollY so that the cursor line is within
// the visible viewport.
func ensureCursorVisible(p *Preview) {
	vh := p.viewportHeight()
	if vh <= 0 {
		vh = 20
	}
	if p.cursorLine < p.scrollY {
		p.scrollY = p.cursorLine
	}
	if p.cursorLine >= p.scrollY+vh {
		p.scrollY = p.cursorLine - vh + 1
	}
	if p.scrollY < 0 {
		p.scrollY = 0
	}
}

// countLeadingSpaces returns the number of leading whitespace columns in
// a string, counting tabs as 4 spaces.
func countLeadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		switch r {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count
		}
	}
	return count
}
