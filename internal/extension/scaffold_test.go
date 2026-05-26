package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	require.Len(t, templates, 4)

	names := make([]string, len(templates))
	for i, tmpl := range templates {
		names[i] = tmpl.Name
	}
	assert.Contains(t, names, "lua")
	assert.Contains(t, names, "wasm-go")
	assert.Contains(t, names, "mcp-python")
	assert.Contains(t, names, "mcp-node")
}

func TestListTemplates_HasDescriptions(t *testing.T) {
	for _, tmpl := range ListTemplates() {
		assert.NotEmpty(t, tmpl.Description, "template %s has empty description", tmpl.Name)
		assert.NotEmpty(t, tmpl.Runtime, "template %s has empty runtime", tmpl.Name)
		assert.NotEmpty(t, tmpl.Files, "template %s has no files", tmpl.Name)
	}
}

func TestScaffold_Lua(t *testing.T) {
	dir := t.TempDir()
	err := Scaffold(dir, "my-lua-ext", "lua")
	require.NoError(t, err)

	target := filepath.Join(dir, "my-lua-ext")
	assert.DirExists(t, target)
	assert.FileExists(t, filepath.Join(target, "extension.toml"))
	assert.FileExists(t, filepath.Join(target, "init.lua"))
}

func TestScaffold_WasmGo(t *testing.T) {
	dir := t.TempDir()
	err := Scaffold(dir, "my-wasm-ext", "wasm-go")
	require.NoError(t, err)

	target := filepath.Join(dir, "my-wasm-ext")
	assert.DirExists(t, target)
	assert.FileExists(t, filepath.Join(target, "extension.toml"))
	assert.FileExists(t, filepath.Join(target, "main.go"))
	assert.FileExists(t, filepath.Join(target, "Makefile"))
	assert.FileExists(t, filepath.Join(target, "README.md"))
}

func TestScaffold_MCPPython(t *testing.T) {
	dir := t.TempDir()
	err := Scaffold(dir, "my-py-ext", "mcp-python")
	require.NoError(t, err)

	target := filepath.Join(dir, "my-py-ext")
	assert.DirExists(t, target)
	assert.FileExists(t, filepath.Join(target, "extension.toml"))
	assert.FileExists(t, filepath.Join(target, "server.py"))
	assert.FileExists(t, filepath.Join(target, "requirements.txt"))
}

func TestScaffold_MCPNode(t *testing.T) {
	dir := t.TempDir()
	err := Scaffold(dir, "my-node-ext", "mcp-node")
	require.NoError(t, err)

	target := filepath.Join(dir, "my-node-ext")
	assert.DirExists(t, target)
	assert.FileExists(t, filepath.Join(target, "extension.toml"))
	assert.FileExists(t, filepath.Join(target, "server.js"))
	assert.FileExists(t, filepath.Join(target, "package.json"))
}

func TestScaffold_ManifestIsValid(t *testing.T) {
	templates := ListTemplates()
	for _, tmpl := range templates {
		t.Run(tmpl.Name, func(t *testing.T) {
			dir := t.TempDir()
			err := Scaffold(dir, "valid-ext", tmpl.Name)
			require.NoError(t, err)

			m, err := LoadManifest(filepath.Join(dir, "valid-ext"))
			require.NoError(t, err)
			assert.Equal(t, "valid-ext", m.Name)
			assert.Equal(t, "0.1.0", m.Version)
			assert.Equal(t, tmpl.Runtime, m.Runtime)
			require.NoError(t, m.Validate())
		})
	}
}

func TestScaffold_ExistingDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	err := os.Mkdir(filepath.Join(dir, "exists"), 0o755)
	require.NoError(t, err)

	err = Scaffold(dir, "exists", "lua")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestScaffold_EmptyNameFails(t *testing.T) {
	err := Scaffold(t.TempDir(), "", "lua")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestScaffold_PathTraversalNameFails(t *testing.T) {
	tests := []struct {
		name    string
		extName string
	}{
		{"dot-dot-slash", "../escape"},
		{"absolute", "/etc/malicious"},
		{"backslash", `bad\name`},
		{"uppercase", "BadName"},
		{"spaces", "bad name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Scaffold(t.TempDir(), tt.extName, "lua")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestScaffold_InvalidTemplateFails(t *testing.T) {
	err := Scaffold(t.TempDir(), "test", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown template")
}

func TestScaffold_FilesContainExpectedContent(t *testing.T) {
	t.Run("lua init.lua", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "content-test", "lua"))

		data, err := os.ReadFile(filepath.Join(dir, "content-test", "init.lua"))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "grut.toast")
		assert.Contains(t, content, "grut.register_command")
		assert.Contains(t, content, `"hello"`)
	})

	t.Run("lua manifest has name", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "name-check", "lua"))

		data, err := os.ReadFile(filepath.Join(dir, "name-check", "extension.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), `name = "name-check"`)
	})

	t.Run("wasm-go main.go has package main", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "wasm-test", "wasm-go"))

		data, err := os.ReadFile(filepath.Join(dir, "wasm-test", "main.go"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "package main")
	})

	t.Run("wasm-go Makefile has tinygo", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "wasm-mk", "wasm-go"))

		data, err := os.ReadFile(filepath.Join(dir, "wasm-mk", "Makefile"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "tinygo build")
	})

	t.Run("wasm-go README has name", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "readme-test", "wasm-go"))

		data, err := os.ReadFile(filepath.Join(dir, "readme-test", "README.md"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "readme-test")
	})

	t.Run("mcp-python server.py has handle_request", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "py-test", "mcp-python"))

		data, err := os.ReadFile(filepath.Join(dir, "py-test", "server.py"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "handle_request")
		assert.Contains(t, string(data), "initialize")
		assert.Contains(t, string(data), "json.JSONDecodeError")
		assert.Contains(t, string(data), "Parse error")
	})

	t.Run("mcp-node server.js has handleRequest", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "node-test", "mcp-node"))

		data, err := os.ReadFile(filepath.Join(dir, "node-test", "server.js"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "handleRequest")
		assert.Contains(t, string(data), "initialize")
		assert.Contains(t, string(data), "try")
		assert.Contains(t, string(data), "Parse error")
	})

	t.Run("mcp-node package.json has name", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, Scaffold(dir, "pkg-test", "mcp-node"))

		data, err := os.ReadFile(filepath.Join(dir, "pkg-test", "package.json"))
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pkg-test"`)
	})
}

func TestScaffold_NameSubstitution(t *testing.T) {
	// Verify every template substitutes the name into extension.toml correctly.
	for _, tmpl := range ListTemplates() {
		t.Run(tmpl.Name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, Scaffold(dir, "subst-check", tmpl.Name))

			data, err := os.ReadFile(filepath.Join(dir, "subst-check", "extension.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(data), `name = "subst-check"`)
			assert.NotContains(t, string(data), "{{.Name}}")
		})
	}
}
