package fuzzyfinder

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Code-annotation markers recognized by the todo source.
const (
	// maxTodoFileSize is the largest file the scanner reads. Files above this
	// size are skipped to keep the scan fast and avoid pulling large blobs
	// into memory.
	maxTodoFileSize = 1 << 20 // 1 MiB
	// maxTodoItems caps the number of results so a repository full of markers
	// cannot produce an unbounded list.
	maxTodoItems = 5000
	// binarySniffLen is the number of leading bytes inspected for a NUL byte
	// when deciding whether a file is binary.
	binarySniffLen = 8192
)

// todoMarkerRe matches TODO, FIXME, HACK, BUG, and XXX when they appear as
// standalone uppercase tokens (not as part of a longer identifier). The
// surrounding boundaries are checked with lookarounds emulated by requiring a
// non-word character or string edge on each side.
var todoMarkerRe = regexp.MustCompile(`(^|[^0-9A-Za-z_])(TODO|FIXME|HACK|BUG|XXX)([^0-9A-Za-z_]|$)`)

// TodoSource scans tracked text files for code-annotation markers (TODO,
// FIXME, HACK, BUG, XXX) and lists each occurrence as a file:line entry. It
// reuses the same hidden-directory, non-navigable-directory, and .gitignore
// filtering as the file source so results stay scoped to source you actually
// track.
type TodoSource struct {
	root string
}

// NewTodoSource creates a source that scans the given root directory for code
// annotations.
func NewTodoSource(root string) *TodoSource {
	return &TodoSource{root: root}
}

// Name implements Source.
func (ts *TodoSource) Name() string { return sourceNameTodos }

// Items implements Source. It walks the tree rooted at ts.root, reads each
// tracked text file, and emits one item per annotation marker found.
func (ts *TodoSource) Items() []Item {
	if ts == nil || ts.root == "" {
		return nil
	}
	gi := loadGitIgnore(ts.root)
	var items []Item
	_ = filepath.WalkDir(ts.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // non-matching entries silently skipped
		}
		if len(items) >= maxTodoItems {
			return filepath.SkipAll
		}
		// Do not follow symlinks. A symlink in the tree can point outside the
		// repository root, and reading its target would leak content from an
		// arbitrary local file (CWE-59). Skipping keeps results scoped to the
		// tracked tree, consistent with this source's contract.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		name := d.Name()
		if name == dirGit && d.IsDir() {
			return filepath.SkipDir
		}
		// Skip hidden directories and files.
		if name != "." && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if nonNavigableDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(ts.root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if gi != nil && gi.MatchesPath(rel) {
			return nil
		}
		items = append(items, scanFileTodos(path, rel)...)
		if len(items) >= maxTodoItems {
			items = items[:maxTodoItems]
			return filepath.SkipAll
		}
		return nil
	})
	return items
}

// scanFileTodos reads a single file and returns an item for every annotation
// marker it contains. It skips files that are too large or that look binary.
func scanFileTodos(path, rel string) []Item {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxTodoFileSize {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || isBinary(data) {
		return nil
	}
	var items []Item
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		marker, message, ok := matchTodoLine(line)
		if !ok {
			continue
		}
		lineNo := i + 1
		items = append(items, Item{
			Text:        fmt.Sprintf("%s:%d  %s %s", rel, lineNo, marker, message),
			Description: marker,
			Category:    categoryTodo,
			Value:       path,
			Line:        lineNo,
		})
	}
	return items
}

// matchTodoLine returns the marker and trailing annotation text for the first
// marker found on a line. ok is false when the line has no marker.
func matchTodoLine(line string) (marker, message string, ok bool) {
	loc := todoMarkerRe.FindStringSubmatchIndex(line)
	if loc == nil {
		return "", "", false
	}
	// Submatch group 2 is the marker token (indices loc[4]:loc[5]).
	marker = line[loc[4]:loc[5]]
	// The annotation message is everything from the marker to the end of the
	// line, with a leading colon and surrounding whitespace trimmed.
	rest := strings.TrimSpace(line[loc[5]:])
	rest = strings.TrimPrefix(rest, ":")
	message = strings.TrimSpace(rest)
	const maxMessageLen = 200
	if len(message) > maxMessageLen {
		message = message[:maxMessageLen]
	}
	return marker, message, true
}

// isBinary reports whether data looks like a binary file by checking the first
// chunk for a NUL byte, which almost never appears in text.
func isBinary(data []byte) bool {
	if len(data) > binarySniffLen {
		data = data[:binarySniffLen]
	}
	return bytes.IndexByte(data, 0) >= 0
}
