package filetree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

const actionBatchRename = "batch_rename"

type batchRenameState struct {
	editor  textarea.Model
	err     string
	oldRel  []string
	enabled bool
}

type batchRenameResultMsg struct {
	errs    []string
	renamed int
	total   int
}

func (ft *FileTree) requestBatchRename() (panels.Panel, tea.Cmd) {
	paths := ft.selectedPaths()
	if len(paths) == 0 {
		return ft, nil
	}

	relPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(ft.rootPath, path)
		if err != nil || rel == "." || hasParentTraversal(rel) {
			return ft, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Cannot batch rename path outside root", Level: notify.Error}
			}
		}
		relPaths = append(relPaths, filepath.Clean(rel))
	}
	sort.Strings(relPaths)

	editor := textarea.New()
	editor.Prompt = ""
	editor.ShowLineNumbers = true
	editor.Placeholder = "path/to/file.txt"
	editor.SetWidth(max(1, ft.viewport.width-2))
	editor.SetHeight(max(3, ft.viewport.height-4))
	editor.SetValue(strings.Join(relPaths, "\n"))

	ft.batchRename = batchRenameState{
		editor:  editor,
		oldRel:  relPaths,
		enabled: true,
	}
	return ft, ft.batchRename.editor.Focus()
}

func (ft *FileTree) handleBatchRenameKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		ft.exitBatchRenameMode()
		return ft, nil
	case "ctrl+s":
		newRel := splitBatchRenameLines(ft.batchRename.editor.Value())
		if err := validateBatchRename(ft.rootPath, ft.batchRename.oldRel, newRel); err != nil {
			ft.batchRename.err = err.Error()
			return ft, nil
		}
		return ft.executeBatchRename(newRel)
	default:
		var cmd tea.Cmd
		ft.batchRename.editor, cmd = ft.batchRename.editor.Update(msg)
		ft.batchRename.err = ""
		return ft, cmd
	}
}

func (ft *FileTree) executeBatchRename(newRel []string) (panels.Panel, tea.Cmd) {
	oldRel := append([]string(nil), ft.batchRename.oldRel...)
	newRel = append([]string(nil), newRel...)
	rootPath := ft.rootPath
	ctx := ft.safeCtx()
	ft.exitBatchRenameMode()
	return ft, func() tea.Msg {
		return applyBatchRenames(ctx, rootPath, oldRel, newRel)
	}
}

func (ft *FileTree) exitBatchRenameMode() {
	ft.batchRename.editor.Blur()
	ft.batchRename = batchRenameState{}
}

func (ft *FileTree) handleBatchRenameResult(msg batchRenameResultMsg) (panels.Panel, tea.Cmd) {
	ft.selected = make(map[string]bool)
	ft.reloadTree()

	cmds := []tea.Cmd{
		ft.loadGitFileStatus(),
		ft.emitCursorFileSelected(),
		func() tea.Msg { return panels.RefreshGitStatusMsg{} },
		func() tea.Msg { return panels.RefreshPreviewMsg{} },
	}
	if ft.filter.gitFilter && ft.gitClient != nil {
		ft.savedCursorPath = ft.CursorPath()
		cmds = append(cmds, ft.loadGitChangedFiles())
	}

	message := fmt.Sprintf("Renamed %d item(s)", msg.renamed)
	level := notify.Success
	if len(msg.errs) > 0 {
		message = fmt.Sprintf("Renamed %d, %d failed: %s", msg.renamed, len(msg.errs), strings.Join(msg.errs, "; "))
		level = notify.Warn
		if msg.renamed == 0 {
			level = notify.Error
		}
	}
	cmds = append(cmds, func() tea.Msg {
		return notify.ShowToastMsg{Message: message, Level: level}
	})
	return ft, tea.Batch(cmds...)
}

func (ft *FileTree) renderBatchRename(width, height int) string {
	ft.batchRename.editor.SetWidth(max(1, width-2))
	ft.batchRename.editor.SetHeight(max(3, height-4))

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ft.colors.Directory)).
		Render("Batch rename — edit paths, Ctrl+S apply, Esc cancel")

	lines := []string{title, ft.batchRename.editor.View()}
	if ft.batchRename.err != "" {
		errLine := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Render(ft.batchRename.err)
		lines = append(lines, errLine)
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func validateBatchRename(root string, oldRel, newRel []string) error {
	if len(newRel) != len(oldRel) {
		return fmt.Errorf("line count changed: expected %d, got %d", len(oldRel), len(newRel))
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	sources := make(map[string]bool, len(oldRel))
	for _, rel := range oldRel {
		sources[filepath.Clean(filepath.Join(absRoot, rel))] = true
	}

	seen := make(map[string]string, len(newRel))
	for i, rel := range newRel {
		line := i + 1
		name := strings.TrimSpace(rel)
		if name == "" {
			return fmt.Errorf("empty name on line %d", line)
		}
		if filepath.IsAbs(name) || hasParentTraversal(name) {
			return fmt.Errorf("name outside repo root: %s", rel)
		}

		target := filepath.Clean(filepath.Join(absRoot, name))
		if !isWithinRoot(absRoot, target) {
			return fmt.Errorf("name outside repo root: %s", rel)
		}
		if prev, ok := seen[target]; ok {
			return fmt.Errorf("duplicate destination: %s", prev)
		}
		seen[target] = name

		if _, statErr := os.Lstat(target); statErr == nil && !sources[target] {
			return fmt.Errorf("destination already exists: %s", name)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("check destination %s: %w", name, statErr)
		}
	}

	return nil
}

func hasParentTraversal(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func splitBatchRenameLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Split(value, "\n")
}

func applyBatchRenames(ctx context.Context, root string, oldRel, newRel []string) batchRenameResultMsg {
	if err := validateBatchRename(root, oldRel, newRel); err != nil {
		return batchRenameResultMsg{total: len(oldRel), errs: []string{err.Error()}}
	}

	result := batchRenameResultMsg{total: len(oldRel)}
	for i, oldName := range oldRel {
		newName := strings.TrimSpace(newRel[i])
		src := filepath.Clean(filepath.Join(root, oldName))
		dst := filepath.Clean(filepath.Join(root, newName))
		if src == dst {
			result.renamed++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			result.errs = append(result.errs, fmt.Sprintf("%s: %v", newName, err))
			continue
		}
		if err := moveFile(ctx, root, src, dst); err != nil {
			result.errs = append(result.errs, fmt.Sprintf("%s: %v", oldName, err))
			continue
		}
		result.renamed++
	}
	return result
}
