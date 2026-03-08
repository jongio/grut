// Package session manages persistent session state for grut.
// Each working directory gets its own session file stored in the XDG
// data directory. Sessions capture tab layout and focus state so the
// TUI can restore its previous configuration on the next launch.
package session

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/jongio/grut/internal/config"
	toml "github.com/pelletier/go-toml/v2"
)

// currentVersion is the session file schema version. Bump when the
// on-disk format changes in a backward-incompatible way.
const currentVersion = 1

// SessionState is the top-level structure persisted to a TOML file.
type SessionState struct {
	Version   int        `toml:"version"`
	WorkDir   string     `toml:"work_dir"`
	ActiveTab int        `toml:"active_tab"`
	Tabs      []TabState `toml:"tabs"`
	SavedAt   time.Time  `toml:"saved_at"`
}

// TabState captures the minimal state needed to restore a single tab.
type TabState struct {
	Name         string `toml:"name"`
	Preset       string `toml:"preset"`
	FocusedPanel string `toml:"focused_panel"`
}

// sessionFile is the on-disk TOML wrapper (mirrors the bookmarks pattern).
type sessionFile struct {
	Session SessionState `toml:"session"`
}

// Manager provides session persistence backed by TOML files in the
// XDG data directory. Each working directory maps to a unique file via
// a truncated SHA-256 hash.
type Manager struct {
	dataDir string
}

// NewManager creates a session manager that stores files under the
// standard XDG data path (~/.local/share/grut/sessions).
func NewManager() *Manager {
	return &Manager{
		dataDir: filepath.Join(config.DataDir(), "sessions"),
	}
}

// SessionPath returns the file path for the session associated with
// workDir. The filename is the first 16 hex characters of the
// SHA-256 hash of workDir.
func (m *Manager) SessionPath(workDir string) string {
	h := sha256.Sum256([]byte(workDir))
	hex := fmt.Sprintf("%x", h[:8]) // 8 bytes = 16 hex chars
	return filepath.Join(m.dataDir, hex+".toml")
}

// Save writes the given session state to disk, creating intermediate
// directories as needed.
func (m *Manager) Save(state SessionState) error {
	state.Version = currentVersion
	state.SavedAt = time.Now()

	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	f := sessionFile{Session: state}
	b, err := toml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	path := m.SessionPath(state.WorkDir)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write session %s: %w", path, err)
	}

	return nil
}

// Load reads the session for the given working directory. It returns
// (nil, nil) when no session file exists for that directory, allowing
// callers to fall back to the default layout without error handling.
func (m *Manager) Load(workDir string) (*SessionState, error) {
	path := m.SessionPath(workDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session %s: %w", path, err)
	}

	var f sessionFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse session %s: %w", path, err)
	}

	// Reject sessions with an incompatible schema version.
	if f.Session.Version != currentVersion {
		return nil, nil
	}

	return &f.Session, nil
}

// Delete removes the session file for the given working directory.
// It returns nil when the file does not exist.
func (m *Manager) Delete(workDir string) error {
	path := m.SessionPath(workDir)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete session %s: %w", path, err)
	}
	return nil
}

// SetDataDir overrides the data directory (used in tests).
func (m *Manager) SetDataDir(dir string) {
	m.dataDir = dir
}
