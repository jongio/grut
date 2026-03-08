package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validManifestTOML() string {
	return `name = "test-ext"
version = "1.0.0"
description = "A test extension"
author = "tester"
license = "MIT"
runtime = "lua"
entry_point = "main.lua"
permissions = ["file_read", "git_read"]
min_grut = "0.1.0"
`
}

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(validManifestTOML()))
	require.NoError(t, err)

	assert.Equal(t, "test-ext", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "A test extension", m.Description)
	assert.Equal(t, "tester", m.Author)
	assert.Equal(t, "MIT", m.License)
	assert.Equal(t, "lua", m.Runtime)
	assert.Equal(t, "main.lua", m.EntryPoint)
	assert.Equal(t, []string{"file_read", "git_read"}, m.Permissions)
	assert.Equal(t, "0.1.0", m.MinGrut)
}

func TestParseManifest_MissingName(t *testing.T) {
	data := `version = "1.0.0"
runtime = "lua"
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParseManifest_MissingVersion(t *testing.T) {
	data := `name = "test"
runtime = "lua"
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestParseManifest_MissingRuntime(t *testing.T) {
	data := `name = "test"
version = "1.0.0"
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime is required")
}

func TestParseManifest_InvalidRuntime(t *testing.T) {
	data := `name = "test"
version = "1.0.0"
runtime = "python"
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid runtime")
}

func TestParseManifest_InvalidPermission(t *testing.T) {
	data := `name = "test"
version = "1.0.0"
runtime = "lua"
permissions = ["file_read", "launch_missiles"]
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown permission")
}

func TestParseManifest_AllValidPermissions(t *testing.T) {
	data := `name = "full-perms"
version = "2.0.0"
runtime = "wasm"
permissions = ["file_read", "file_write", "git_read", "git_write", "network", "process", "clipboard", "notify"]
`
	m, err := ParseManifest([]byte(data))
	require.NoError(t, err)
	assert.Len(t, m.Permissions, 8)
}

func TestParseManifest_InvalidSemver(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"no patch", "1.0"},
		{"no minor", "1"},
		{"leading v", "v1.0.0"},
		{"letters", "abc"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `name = "test"
runtime = "lua"
version = "` + tt.version + `"
`
			// empty version is caught by "version is required"
			if tt.version == "" {
				data = `name = "test"
runtime = "lua"
`
			}
			_, err := ParseManifest([]byte(data))
			require.Error(t, err)
		})
	}
}

func TestParseManifest_ValidSemverVariants(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"basic", "1.0.0"},
		{"prerelease", "1.0.0-alpha.1"},
		{"build", "1.0.0+build.42"},
		{"prerelease+build", "1.0.0-rc.1+20240101"},
		{"large numbers", "123.456.789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `name = "test"
runtime = "lua"
version = "` + tt.version + `"
`
			m, err := ParseManifest([]byte(data))
			require.NoError(t, err)
			assert.Equal(t, tt.version, m.Version)
		})
	}
}

func TestParseManifest_InvalidMinGrut(t *testing.T) {
	data := `name = "test"
version = "1.0.0"
runtime = "lua"
min_grut = "nope"
`
	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_grut")
}

func TestParseManifest_AllRuntimes(t *testing.T) {
	for _, rt := range []string{"lua", "wasm", "mcp"} {
		t.Run(rt, func(t *testing.T) {
			data := `name = "rt-test"
version = "1.0.0"
runtime = "` + rt + `"
`
			m, err := ParseManifest([]byte(data))
			require.NoError(t, err)
			assert.Equal(t, rt, m.Runtime)
		})
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "extension.toml"), []byte(validManifestTOML()), 0o644)
	require.NoError(t, err)

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, "test-ext", m.Name)
	assert.Equal(t, "lua", m.Runtime)
}

func TestLoadManifest_NoFile(t *testing.T) {
	_, err := LoadManifest(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest")
}

func TestParseManifest_InvalidTOML(t *testing.T) {
	_, err := ParseManifest([]byte(`not valid [[[toml`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestParseManifest_MinimalValid(t *testing.T) {
	data := `name = "minimal"
version = "0.0.1"
runtime = "mcp"
`
	m, err := ParseManifest([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "minimal", m.Name)
	assert.Empty(t, m.Permissions)
	assert.Empty(t, m.EntryPoint)
	assert.Empty(t, m.MinGrut)
}

func TestParseManifest_PathTraversalNames(t *testing.T) {
	tests := []struct {
		name    string
		extName string
	}{
		{"dot-dot-slash", "../escape"},
		{"absolute-unix", "/etc/passwd"},
		{"backslash", `bad\name`},
		{"forward-slash", "bad/name"},
		{"uppercase", "BadName"},
		{"spaces", "bad name"},
		{"dot-only", "."},
		{"dot-dot", ".."},
		{"special-chars", "ext@1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `name = "` + tt.extName + `"
version = "1.0.0"
runtime = "lua"
`
			_, err := ParseManifest([]byte(data))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "manifest: name")
		})
	}
}

func TestParseManifest_ValidNames(t *testing.T) {
	tests := []struct {
		name    string
		extName string
	}{
		{"simple", "myext"},
		{"with-hyphen", "my-ext"},
		{"with-underscore", "my_ext"},
		{"with-numbers", "ext123"},
		{"starts-number", "1ext"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `name = "` + tt.extName + `"
version = "1.0.0"
runtime = "lua"
`
			m, err := ParseManifest([]byte(data))
			require.NoError(t, err)
			assert.Equal(t, tt.extName, m.Name)
		})
	}
}

func TestParseManifest_RejectsAbsoluteEntryPoint(t *testing.T) {
	data := `name = "test-ext"
version = "1.0.0"
runtime = "lua"
entry_point = "/etc/passwd"
`

	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry_point must be a relative path")
}

func TestParseManifest_RejectsTraversalEntryPoint(t *testing.T) {
	data := `name = "test-ext"
version = "1.0.0"
runtime = "lua"
entry_point = "../../.ssh/id_rsa"
`

	_, err := ParseManifest([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry_point must not contain '..'")
}

func TestParseManifest_AcceptsRelativeEntryPoint(t *testing.T) {
	data := `name = "test-ext"
version = "1.0.0"
runtime = "lua"
entry_point = "main.lua"
`

	m, err := ParseManifest([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "main.lua", m.EntryPoint)
}
