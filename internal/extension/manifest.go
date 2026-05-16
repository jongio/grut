package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Manifest describes an extension's metadata, loaded from extension.toml.
type Manifest struct {
	Name        string   `toml:"name"`
	Version     string   `toml:"version"`
	Description string   `toml:"description"`
	Author      string   `toml:"author"`
	License     string   `toml:"license"`
	Runtime     string   `toml:"runtime"`
	EntryPoint  string   `toml:"entry_point"`
	MinGrut     string   `toml:"min_grut"`
	Permissions []string `toml:"permissions"`
}

// validRuntimes lists the runtimes an extension may declare.
var validRuntimes = map[string]struct{}{
	extTypeLua:  {},
	extTypeWasm: {},
	extTypeMCP:  {},
}

// semverRe matches a basic semver string (major.minor.patch with optional
// pre-release and build metadata).
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// safeNameRe matches a name that is safe for use as a directory component:
// lowercase letters, digits, hyphens, and underscores; 1-128 chars.
var safeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

// ParseManifest decodes TOML bytes into a Manifest and validates it.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// LoadManifest reads extension.toml from dir and returns the parsed manifest.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return ParseManifest(data)
}

// Validate checks that all required fields are present and values are valid.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	// Reject names that could cause path traversal or filesystem issues.
	if !safeNameRe.MatchString(m.Name) {
		return fmt.Errorf("manifest: name %q is invalid (must be lowercase alphanumeric, hyphens, underscores; 1-128 chars)", m.Name)
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version %q is not valid semver", m.Version)
	}
	if m.Runtime == "" {
		return fmt.Errorf("manifest: runtime is required")
	}
	if _, ok := validRuntimes[m.Runtime]; !ok {
		return fmt.Errorf("manifest: invalid runtime %q (want %s, %s, or %s)", m.Runtime, extTypeLua, extTypeWasm, extTypeMCP)
	}
	// Validate entry_point: reject path traversal and absolute paths.
	if m.EntryPoint != "" {
		if filepath.IsAbs(m.EntryPoint) || strings.HasPrefix(m.EntryPoint, "/") || strings.HasPrefix(m.EntryPoint, `\`) {
			return fmt.Errorf("manifest: entry_point must be a relative path")
		}
		if strings.Contains(m.EntryPoint, "..") {
			return fmt.Errorf("manifest: entry_point must not contain '..'")
		}
		if strings.ContainsAny(m.EntryPoint, ";&|<>$`'\"\\\n\r") {
			return fmt.Errorf("manifest: entry_point contains invalid shell metacharacters")
		}
	}
	for _, p := range m.Permissions {
		if !ValidPermission(p) {
			return fmt.Errorf("manifest: unknown permission %q", p)
		}
	}
	if m.MinGrut != "" && !semverRe.MatchString(m.MinGrut) {
		return fmt.Errorf("manifest: min_grut %q is not valid semver", m.MinGrut)
	}
	return nil
}
