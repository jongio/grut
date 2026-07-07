package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// gitignoreName is the name of the ignore file at the repository root.
const gitignoreName = ".gitignore"

// gitignorePatternFor returns the .gitignore pattern for target relative to
// rootPath. Paths use forward slashes so the pattern is portable across
// platforms, and directories get a trailing slash. It returns an error when
// the target is the root itself or lives outside the repository.
func gitignorePatternFor(rootPath, target string, isDir bool) (string, error) {
	rel, err := filepath.Rel(rootPath, target)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path is outside the repository")
	}
	if isDir {
		rel += "/"
	}
	return rel, nil
}

// gitignoreContains reports whether content already has pattern on its own
// line, ignoring surrounding whitespace.
func gitignoreContains(content, pattern string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
}

// appendGitignore appends pattern to the .gitignore file at path, creating the
// file if it does not exist. It reports whether a new line was written; a
// pattern that is already present is left untouched and reported as false.
func appendGitignore(path, pattern string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	content := string(data)
	if gitignoreContains(content, pattern) {
		return false, nil
	}
	var b strings.Builder
	b.WriteString(content)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(pattern + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// addToGitignore appends the entry under the cursor to the repository's
// .gitignore file and reloads the tree so the change is visible. Files are
// added by their repo-relative path; directories get a trailing slash.
func (ft *FileTree) addToGitignore() (panels.Panel, tea.Cmd) {
	n := ft.cursorNode()
	if n == nil {
		return ft, nil
	}
	pattern, err := gitignorePatternFor(ft.rootPath, n.path, n.isDir)
	if err != nil {
		errMsg := err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Ignore failed: " + errMsg, Level: notify.Error}
		}
	}
	added, err := appendGitignore(filepath.Join(ft.rootPath, gitignoreName), pattern)
	if err != nil {
		errMsg := err.Error()
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Ignore failed: " + errMsg, Level: notify.Error}
		}
	}
	ft.reloadTree()
	if !added {
		return ft, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Already ignored: " + pattern, Level: notify.Info}
		}
	}
	return ft, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Added to .gitignore: " + pattern, Level: notify.Success}
	}
}
