// Package bookmarks manages persistent directory bookmarks for the grut TUI.
// Bookmarks are stored in a separate TOML file alongside the main config
// to avoid conflicting writes with the read-only config loader.
package bookmarks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jongio/grut/internal/config"
	toml "github.com/pelletier/go-toml/v2"
)

// Bookmark represents a single bookmarked directory.
type Bookmark struct {
	Path string `toml:"path"`
	Name string `toml:"name"`
}

// bookmarksFile is the on-disk TOML structure.
type bookmarksFile struct {
	Bookmarks []Bookmark `toml:"bookmarks"`
}

// Manager provides thread-safe bookmark operations and persistence.
type Manager struct {
	mu        sync.RWMutex
	bookmarks []Bookmark
	configDir string
}

// NewManager creates a Manager seeded from the config's bookmark paths.
// Each path in cfg.Paths is converted into a Bookmark with its basename
// as the display name. The manager then attempts to load any previously
// saved bookmarks from the bookmarks file, merging them in.
func NewManager(cfg config.BookmarksConfig) *Manager {
	return newManagerWithDir(cfg, config.ConfigDir())
}

// NewManagerWithDir creates a Manager with an explicit config directory.
// This is useful for testing to avoid loading the user's real bookmarks.
func NewManagerWithDir(cfg config.BookmarksConfig, configDir string) *Manager {
	return newManagerWithDir(cfg, configDir)
}

// newManagerWithDir is the internal constructor shared by NewManager and
// NewManagerWithDir.
func newManagerWithDir(cfg config.BookmarksConfig, configDir string) *Manager {
	m := &Manager{
		configDir: configDir,
	}

	// Seed from config (initial paths from config.toml).
	seen := make(map[string]bool, len(cfg.Paths))
	for _, p := range cfg.Paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		m.bookmarks = append(m.bookmarks, Bookmark{
			Path: abs,
			Name: filepath.Base(abs),
		})
	}

	// Load saved bookmarks (overrides seed if file exists).
	if saved, err := m.load(); err == nil && len(saved) > 0 {
		m.bookmarks = saved
	}

	return m
}

// List returns a snapshot of all bookmarks.
func (m *Manager) List() []Bookmark {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Bookmark, len(m.bookmarks))
	copy(out, m.bookmarks)
	return out
}

// Add validates the path exists and is a directory, checks for duplicates,
// and appends a new bookmark.
func (m *Manager) Add(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", path, err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("stat %q: %w", abs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", abs)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, b := range m.bookmarks {
		if b.Path == abs {
			return fmt.Errorf("bookmark already exists: %s", abs)
		}
	}

	m.bookmarks = append(m.bookmarks, Bookmark{
		Path: abs,
		Name: filepath.Base(abs),
	})

	return nil
}

// Remove deletes the bookmark with the given path. Returns an error if
// the path is not bookmarked.
func (m *Manager) Remove(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, b := range m.bookmarks {
		if b.Path == abs {
			m.bookmarks = append(m.bookmarks[:i], m.bookmarks[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("bookmark not found: %s", abs)
}

// Has reports whether the given path is bookmarked.
func (m *Manager) Has(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, b := range m.bookmarks {
		if b.Path == abs {
			return true
		}
	}
	return false
}

// Save persists the current bookmarks to the bookmarks file.
func (m *Manager) Save() error {
	m.mu.RLock()
	data := bookmarksFile{Bookmarks: make([]Bookmark, len(m.bookmarks))}
	copy(data.Bookmarks, m.bookmarks)
	configDir := m.configDir
	m.mu.RUnlock()

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	b, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal bookmarks: %w", err)
	}

	path := filepath.Join(configDir, "bookmarks.toml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// filePath returns the full path to the bookmarks TOML file.
func (m *Manager) filePath() string {
	return filepath.Join(m.configDir, "bookmarks.toml")
}

// load reads and parses the bookmarks file. Returns nil slice if the
// file does not exist.
func (m *Manager) load() ([]Bookmark, error) {
	data, err := os.ReadFile(m.filePath())
	if err != nil {
		return nil, err
	}

	var f bookmarksFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse bookmarks: %w", err)
	}

	return f.Bookmarks, nil
}

// SetConfigDir overrides the config directory (used in tests).
func (m *Manager) SetConfigDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configDir = dir
}
