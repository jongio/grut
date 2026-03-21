// Package extension implements grut's plugin system. Extensions are
// small programs (Lua scripts, WASM modules, or MCP servers) installed
// from https:// git URLs or local directories. Each extension declares
// a manifest (extension.toml) that specifies its runtime, entry point,
// and required permissions. The Manager handles install, remove,
// enable/disable, and state persistence.
package extension

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// ExtensionInfo holds runtime state for an installed extension.
type ExtensionInfo struct {
	InstalledAt time.Time `toml:"installed_at"`
	Manifest    Manifest  `toml:"manifest"`
	Dir         string    `toml:"-"`
	Enabled     bool      `toml:"enabled"`
}

// Manager handles extension installation, removal, and state tracking.
type Manager struct {
	installed map[string]*ExtensionInfo
	extDir    string
	mu        sync.RWMutex
}

// stateFileName is the file inside extDir that persists enabled/disabled state.
const stateFileName = "extensions.toml"

// extensionState is the serialisable representation stored in stateFileName.
type extensionState struct {
	Extensions []extensionEntry `toml:"extensions"`
}
type extensionEntry struct {
	InstalledAt time.Time `toml:"installed_at"`
	Name        string    `toml:"name"`
	Enabled     bool      `toml:"enabled"`
}

// NewManager creates a Manager rooted at extDir, creating the directory if
// needed. It does NOT scan for installed extensions — call LoadAll explicitly.
func NewManager(extDir string) *Manager {
	_ = os.MkdirAll(extDir, 0o755)
	return &Manager{
		extDir:    extDir,
		installed: make(map[string]*ExtensionInfo),
	}
}

// Install adds an extension from a git URL (https:// only) or local path.
// The manifest inside the source is validated before the installation is
// considered successful.
func (m *Manager) Install(ctx context.Context, source string) error {
	isURL := strings.HasPrefix(source, "https://")
	if strings.Contains(source, "://") && !isURL {
		return fmt.Errorf("install: only https:// URLs are allowed")
	}
	// Reject SSH-style git URLs (git@host:path).
	if strings.HasPrefix(source, "git@") {
		return fmt.Errorf("install: only https:// URLs are allowed")
	}
	// Determine destination by cloning/copying into a temp dir first so we
	// can read the manifest before deciding the final directory name.
	tmpDir, err := os.MkdirTemp(m.extDir, ".install-*")
	if err != nil {
		return fmt.Errorf("install: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() // clean up on any error path
	if isURL {
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", "--no-recurse-submodules", source, tmpDir)
		cmd.Env = safeGitCloneEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("install: git clone: %s: %w", strings.TrimSpace(string(out)), err)
		}
	} else {
		// Local path: copy contents into tmpDir.
		if err := copyDir(source, tmpDir); err != nil {
			return fmt.Errorf("install: copy local: %w", err)
		}
	}
	manifest, err := LoadManifest(tmpDir)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	// CR-010: Validate extension name to prevent path traversal via
	// manifest.Name (e.g. Name: "../../../tmp/evil" would escape extDir
	// when used in filepath.Join).
	if !isValidExtensionName(manifest.Name) {
		return fmt.Errorf("install: invalid extension name %q: must be 1-128 alphanumeric, dash, or underscore characters", manifest.Name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.installed[manifest.Name]; exists {
		return fmt.Errorf("install: extension %q is already installed", manifest.Name)
	}
	destDir := filepath.Join(m.extDir, manifest.Name)
	if err := os.Rename(tmpDir, destDir); err != nil {
		// Cross-device rename; fall back to copy + remove.
		if err := copyDir(tmpDir, destDir); err != nil {
			return fmt.Errorf("install: move to final dir: %w", err)
		}
	}
	info := &ExtensionInfo{
		Manifest:    *manifest,
		Dir:         destDir,
		Enabled:     true,
		InstalledAt: time.Now().UTC(),
	}
	m.installed[manifest.Name] = info
	return m.saveStateLocked()
}

// Remove deletes the extension directory and removes it from state.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.installed[name]
	if !ok {
		return fmt.Errorf("remove: extension %q not found", name)
	}
	if err := os.RemoveAll(info.Dir); err != nil {
		return fmt.Errorf("remove: delete dir: %w", err)
	}
	delete(m.installed, name)
	return m.saveStateLocked()
}

// Enable marks the extension as enabled and persists state.
func (m *Manager) Enable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.installed[name]
	if !ok {
		return fmt.Errorf("enable: extension %q not found", name)
	}
	info.Enabled = true
	return m.saveStateLocked()
}

// Disable marks the extension as disabled and persists state.
func (m *Manager) Disable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.installed[name]
	if !ok {
		return fmt.Errorf("disable: extension %q not found", name)
	}
	info.Enabled = false
	return m.saveStateLocked()
}

// List returns all installed extensions. The returned slice is a snapshot;
// callers may iterate without holding a lock.
func (m *Manager) List() []ExtensionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ExtensionInfo, 0, len(m.installed))
	for _, info := range m.installed {
		out = append(out, *info)
	}
	return out
}

// Get returns a single extension by name.
func (m *Manager) Get(name string) (*ExtensionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.installed[name]
	if !ok {
		return nil, fmt.Errorf("get: extension %q not found", name)
	}
	cp := *info
	return &cp, nil
}

// LoadAll scans extDir for subdirectories containing extension.toml,
// loads each manifest, and restores persisted enabled/disabled state.
func (m *Manager) LoadAll() error {
	entries, err := os.ReadDir(m.extDir)
	if err != nil {
		return fmt.Errorf("load extensions: %w", err)
	}
	saved := m.loadState()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(m.extDir, entry.Name())
		manifest, err := LoadManifest(dir)
		if err != nil {
			continue // skip directories without valid manifests
		}
		info := &ExtensionInfo{
			Manifest:    *manifest,
			Dir:         dir,
			Enabled:     true,
			InstalledAt: time.Now().UTC(),
		}
		// Restore persisted state if available.
		if s, ok := saved[manifest.Name]; ok {
			info.Enabled = s.Enabled
			info.InstalledAt = s.InstalledAt
		}
		m.installed[manifest.Name] = info
	}
	return nil
}

// saveStateLocked writes the current enabled/disabled state to the state file.
// Caller MUST hold m.mu (write lock).
func (m *Manager) saveStateLocked() error {
	state := extensionState{
		Extensions: make([]extensionEntry, 0, len(m.installed)),
	}
	for name, info := range m.installed {
		state.Extensions = append(state.Extensions, extensionEntry{
			Name:        name,
			Enabled:     info.Enabled,
			InstalledAt: info.InstalledAt,
		})
	}
	data, err := toml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	path := filepath.Join(m.extDir, stateFileName)
	return os.WriteFile(path, data, 0o600)
}

// loadState reads the persisted state file and returns a map keyed by
// extension name. Returns an empty map on any error.
func (m *Manager) loadState() map[string]extensionEntry {
	data, err := os.ReadFile(filepath.Join(m.extDir, stateFileName))
	if err != nil {
		return make(map[string]extensionEntry)
	}
	var state extensionState
	if err := toml.Unmarshal(data, &state); err != nil {
		return make(map[string]extensionEntry)
	}
	out := make(map[string]extensionEntry, len(state.Extensions))
	for _, e := range state.Extensions {
		out[e.Name] = e
	}
	return out
}

// copyDir recursively copies src into dst, creating dst if needed.
// Symlinks are rejected to prevent directory traversal attacks.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s: %w", path, err)
		}
		// Reject symlinks to prevent traversal outside the source directory.
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks not allowed in extension: %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Double-check the entry is a regular file (not a symlink that was
		// not caught by WalkDir's type — defensive check).
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("checking file type %s: %w", path, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks not allowed in extension: %s", path)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat source %s: %w", src, err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("creating destination %s: %w", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, copyErr)
	}
	return closeErr
}

// safeGitCloneEnv returns a minimal environment for git clone operations,
// excluding secrets and tokens that could leak to malicious remote servers.
func safeGitCloneEnv() []string {
	allow := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LANG": true,
		"LC_ALL": true, "TMPDIR": true, "TEMP": true, "TMP": true,
		"SYSTEMROOT": true, "HOMEDRIVE": true, "HOMEPATH": true,
		"USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
		"PROGRAMFILES": true, "COMSPEC": true, "SHELL": true,
		"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
		"GIT_TERMINAL_PROMPT": true, "GIT_SSL_CAINFO": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	}
	var filtered []string
	for _, env := range os.Environ() {
		name, _, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		if allow[strings.ToUpper(name)] {
			filtered = append(filtered, env)
		}
	}
	// Suppress interactive auth prompts from malicious servers.
	filtered = append(filtered, "GIT_TERMINAL_PROMPT=0")
	return filtered
}

// isValidExtensionName checks that name is safe for use in filepath.Join.
// Only ASCII alphanumerics, dashes, and underscores are allowed. This
// prevents path traversal attacks via malicious manifest names like
// "../../../tmp/evil" (CWE-22).
func isValidExtensionName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
