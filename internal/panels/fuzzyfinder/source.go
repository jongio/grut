// Package fuzzyfinder implements a fuzzy search overlay for grut.
// It provides a unified interface for searching files, commands, and
// other sources with real-time fuzzy matching and highlighted results.
package fuzzyfinder

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"github.com/jongio/grut/internal/keymap"
)

// ---------------------------------------------------------------------------
// File-list cache
// ---------------------------------------------------------------------------

// cacheTTL is the duration a cached file list remains valid.
const cacheTTL = 5 * time.Second

// fileCache holds a cached set of Items keyed by root directory.
type fileCache struct {
	mu       sync.RWMutex
	items    []Item
	root     string
	loadedAt time.Time
	valid    bool
}

// get returns the cached items if the cache is valid, the root matches,
// and the TTL has not expired. Otherwise it returns nil.
func (c *fileCache) get(root string) []Item {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.valid || c.root != root {
		return nil
	}
	if time.Since(c.loadedAt) > cacheTTL {
		return nil
	}
	// Return a copy to prevent callers from mutating the cache.
	out := make([]Item, len(c.items))
	copy(out, c.items)
	return out
}

// set stores items in the cache.
func (c *fileCache) set(root string, items []Item) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.root = root
	c.items = make([]Item, len(items))
	copy(c.items, items)
	c.loadedAt = time.Now()
	c.valid = true
}

// invalidate marks the cache as invalid.
func (c *fileCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.valid = false
}

// globalFileCache is the package-level singleton shared across fuzzy finder opens.
var globalFileCache = &fileCache{}

// InvalidateFileCache marks the global file-list cache as invalid so that
// the next fuzzy-finder open re-walks the filesystem.
func InvalidateFileCache() {
	globalFileCache.invalidate()
}

// Source provides searchable items for the fuzzy finder. Implementations
// gather items from a specific domain (files, commands, bookmarks, etc.)
// and return them as a flat list for fuzzy matching.
type Source interface {
	// Name returns the source category name (e.g. "files", "commands").
	Name() string

	// Items returns all searchable items from this source.
	Items() []Item
}

// Item represents a single searchable entry in the fuzzy finder.
type Item struct {
	Text        string // Searchable text (what fuzzy matches against)
	Description string // Secondary text shown in results
	Category    string // Source category for display grouping
	Value       any    // Arbitrary data for the result handler
}

// ---------------------------------------------------------------------------
// .gitignore support
// ---------------------------------------------------------------------------

// loadGitIgnore compiles a gitignore matcher from all .gitignore files found
// between root and the filesystem root, plus .git/info/exclude. Returns nil
// when no .gitignore files exist.
func loadGitIgnore(root string) *ignore.GitIgnore {
	var patterns []string

	// Walk upward from root collecting .gitignore files, starting from
	// the deepest directory so that closer files take precedence (appended
	// last to the pattern list).
	var ancestors []string
	dir := filepath.Clean(root)
	for {
		gi := filepath.Join(dir, ".gitignore")
		if _, err := os.Stat(gi); err == nil {
			ancestors = append(ancestors, gi)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Reverse so the root-most .gitignore comes first and deeper ones
	// can override (same semantics as git).
	for i := len(ancestors) - 1; i >= 0; i-- {
		lines := readIgnoreLines(ancestors[i])
		patterns = append(patterns, lines...)
	}

	// Also read .git/info/exclude if it exists.
	exclude := filepath.Join(root, ".git", "info", "exclude")
	if _, err := os.Stat(exclude); err == nil {
		patterns = append(patterns, readIgnoreLines(exclude)...)
	}

	if len(patterns) == 0 {
		return nil
	}
	return ignore.CompileIgnoreLines(patterns...)
}

// readIgnoreLines reads a gitignore-format file and returns non-empty,
// non-comment lines.
func readIgnoreLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// nonNavigableDirs is the set of directory names that should always be
// skipped during file and directory walks, regardless of whether a
// .gitignore exists. These are universally unwanted in navigation results.
var nonNavigableDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
}

// ---------------------------------------------------------------------------
// FileSource
// ---------------------------------------------------------------------------

// FileSource walks a directory tree and provides file paths as searchable
// items. Hidden files and directories (names starting with ".") are
// excluded from the results. Results are cached to avoid repeated walks.
type FileSource struct {
	root string
}

// NewFileSource creates a source that walks the given root directory.
func NewFileSource(root string) *FileSource {
	return &FileSource{root: root}
}

// Name implements Source.
func (fs *FileSource) Name() string { return "files" }

// Items implements Source. It checks the global cache first and falls back
// to walking the directory tree rooted at fs.root, filtering entries via
// .gitignore rules when available.
func (fs *FileSource) Items() []Item {
	// Check cache first.
	if cached := globalFileCache.get(fs.root); cached != nil {
		return cached
	}

	gi := loadGitIgnore(fs.root)

	var items []Item
	_ = filepath.WalkDir(fs.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // non-matching entries silently skipped
		}

		name := d.Name()

		// Always skip .git directory.
		if name == ".git" && d.IsDir() {
			return filepath.SkipDir
		}

		// Skip hidden directories and files.
		if name != "." && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Always skip common non-navigable directories so files inside
		// them are never included regardless of .gitignore presence.
		if d.IsDir() && nonNavigableDirs[name] {
			return filepath.SkipDir
		}

		// Skip directories — only include files.
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(fs.root, path)
		if relErr != nil {
			rel = path
		}

		// Normalize to forward slashes for consistent display.
		rel = filepath.ToSlash(rel)

		// Apply .gitignore filtering.
		if gi != nil && gi.MatchesPath(rel) {
			return nil
		}

		items = append(items, Item{
			Text:     rel,
			Category: "file",
			Value:    path,
		})
		return nil
	})

	// Populate cache.
	globalFileCache.set(fs.root, items)

	return items
}

// ---------------------------------------------------------------------------
// DirectorySource
// ---------------------------------------------------------------------------

// DefaultDirectorySourceMaxDepth is the default recursion depth used when
// presenting candidate directories in the change-directory fuzzy finder.
const DefaultDirectorySourceMaxDepth = 5

// DirectorySource walks a directory tree and provides directory paths as
// searchable items for the change-directory fuzzy finder. Hidden directories
// (names starting with ".") and directories matched by .gitignore rules are
// excluded. When no .gitignore exists, a hardcoded skip list filters common
// non-navigable directories (node_modules, vendor, __pycache__, etc.).
type DirectorySource struct {
	root     string
	maxDepth int
}

// NewDirectorySource creates a source that walks the given root directory
// and returns subdirectories up to maxDepth levels deep.
func NewDirectorySource(root string, maxDepth int) *DirectorySource {
	return &DirectorySource{root: root, maxDepth: maxDepth}
}

// Name implements Source.
func (ds *DirectorySource) Name() string { return "directories" }

// Items implements Source. It walks the directory tree rooted at ds.root
// and returns subdirectories as searchable items, skipping hidden and
// ignored directories.
func (ds *DirectorySource) Items() []Item {
	var items []Item

	// Add parent directory as first item (like "cd ..").
	parent := filepath.Dir(ds.root)
	if parent != ds.root { // not at filesystem root
		items = append(items, Item{
			Text:        "..",
			Description: parent,
			Category:    "directory",
			Value:       parent,
		})
	}

	gi := loadGitIgnore(ds.root)

	_ = filepath.WalkDir(ds.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // inaccessible entries silently skipped
		}

		name := d.Name()

		// Always skip .git directory regardless of .gitignore.
		if name == ".git" && d.IsDir() {
			return filepath.SkipDir
		}

		// Skip hidden directories (except root itself).
		if name != "." && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only include directories (not files).
		if !d.IsDir() {
			return nil
		}

		// Skip the root itself.
		if path == ds.root {
			return nil
		}

		rel, relErr := filepath.Rel(ds.root, path)
		if relErr != nil {
			rel = path
		}

		// Normalize to forward slashes.
		rel = filepath.ToSlash(rel)

		// Always skip common non-navigable directories regardless of
		// .gitignore presence. A parent .gitignore found by walking upward
		// should not disable this baseline filtering.
		if nonNavigableDirs[name] {
			return filepath.SkipDir
		}

		// Additionally apply .gitignore filtering for other directories.
		if gi != nil {
			// Use trailing slash to match directory patterns.
			if gi.MatchesPath(rel + "/") {
				return filepath.SkipDir
			}
		}

		// Enforce max depth.
		depth := strings.Count(rel, "/") + 1
		if depth > ds.maxDepth {
			return filepath.SkipDir
		}

		items = append(items, Item{
			Text:     rel,
			Category: "directory",
			Value:    path,
		})
		return nil
	})

	return items
}

// ---------------------------------------------------------------------------
// CommandSource
// ---------------------------------------------------------------------------

// CommandSource provides keymap bindings as searchable items for the
// command palette. Actions are deduplicated so each action appears once,
// using the first binding's key and description.
type CommandSource struct {
	bindings []keymap.Binding
}

// NewCommandSource creates a source from keymap bindings.
func NewCommandSource(bindings []keymap.Binding) *CommandSource {
	return &CommandSource{bindings: bindings}
}

// Name implements Source.
func (cs *CommandSource) Name() string { return "commands" }

// Items implements Source. It returns deduplicated command bindings
// as searchable items, with the action as text and the key combination
// shown in the description.
func (cs *CommandSource) Items() []Item {
	items := make([]Item, 0, len(cs.bindings))
	seen := make(map[string]bool)

	for _, b := range cs.bindings {
		if seen[b.Action] {
			continue
		}
		seen[b.Action] = true

		desc := b.Description
		if b.Key != "" {
			desc += " (" + b.Key + ")"
		}

		items = append(items, Item{
			Text:        b.Action,
			Description: desc,
			Category:    "command",
			Value:       b.Action,
		})
	}

	return items
}
