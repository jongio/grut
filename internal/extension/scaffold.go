package extension

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

//go:embed templates/*/*
var templatesFS embed.FS

// Template describes a scaffold template for creating new extensions.
type Template struct {
	Files       map[string]string // relative path → content (may contain {{.Name}})
	Name        string
	Description string
	Runtime     string
}

// scaffoldData holds the values substituted into template files.
type scaffoldData struct {
	Name string
}

// builtinTemplates is lazily populated on first access from the embedded
// template files.
var (
	builtinTemplates     []Template
	builtinTemplatesOnce sync.Once
)

// getBuiltinTemplates returns the builtin templates, loading them on first call.
func getBuiltinTemplates() []Template {
	builtinTemplatesOnce.Do(func() {
		builtinTemplates = loadBuiltinTemplates()
	})
	return builtinTemplates
}

// mustReadTemplate reads an embedded template file and panics on failure.
func mustReadTemplate(path string) string {
	data, err := templatesFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embedded template %s: %v", path, err))
	}
	return string(data)
}

// loadBuiltinTemplates constructs the set of built-in scaffold templates from
// the embedded file system.
func loadBuiltinTemplates() []Template {
	return []Template{
		{
			Name:        extTypeLua,
			Description: "Lua scripting extension",
			Runtime:     extTypeLua,
			Files: map[string]string{
				configFile: mustReadTemplate("templates/lua/extension.toml"),
				"init.lua": mustReadTemplate("templates/lua/init.lua"),
			},
		},
		{
			Name:        "wasm-go",
			Description: "WebAssembly extension built with TinyGo",
			Runtime:     extTypeWasm,
			Files: map[string]string{
				configFile:  mustReadTemplate("templates/wasm-go/extension.toml"),
				"main.go":   mustReadTemplate("templates/wasm-go/main.go.tmpl"),
				"Makefile":  mustReadTemplate("templates/wasm-go/Makefile"),
				"README.md": mustReadTemplate("templates/wasm-go/README.md"),
			},
		},
		{
			Name:        "mcp-python",
			Description: "Python MCP server extension",
			Runtime:     extTypeMCP,
			Files: map[string]string{
				configFile:         mustReadTemplate("templates/mcp-python/extension.toml"),
				"server.py":        mustReadTemplate("templates/mcp-python/server.py"),
				"requirements.txt": mustReadTemplate("templates/mcp-python/requirements.txt"),
			},
		},
		{
			Name:        "mcp-node",
			Description: "Node.js MCP server extension",
			Runtime:     extTypeMCP,
			Files: map[string]string{
				configFile:     mustReadTemplate("templates/mcp-node/extension.toml"),
				"server.js":    mustReadTemplate("templates/mcp-node/server.js"),
				"package.json": mustReadTemplate("templates/mcp-node/package.json"),
			},
		},
	}
}

// ListTemplates returns all available scaffold templates.
func ListTemplates() []Template {
	templates := getBuiltinTemplates()
	out := make([]Template, len(templates))
	copy(out, templates)
	return out
}

// Scaffold creates a new extension project in dir/name using the named template.
func Scaffold(dir, name, templateName string) error {
	if name == "" {
		return fmt.Errorf("scaffold: extension name is required")
	}
	// Validate name is safe for filesystem use (no path traversal).
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("scaffold: name %q is invalid (must be lowercase alphanumeric, hyphens, underscores; 1-128 chars)", name)
	}
	tmpl := findTemplate(templateName)
	if tmpl == nil {
		return fmt.Errorf("scaffold: unknown template %q", templateName)
	}
	target := filepath.Join(dir, name)
	data := scaffoldData{Name: name}
	// Use os.Mkdir (not MkdirAll) to atomically create the directory.
	// This eliminates a TOCTOU race between stat-check and create (CWE-367):
	// an attacker could place a symlink at `target` between the two calls.
	// os.Mkdir fails if the path already exists — no separate check needed.
	if err := os.Mkdir(target, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("scaffold: directory %s already exists", target)
		}
		return fmt.Errorf("scaffold: create directory: %w", err)
	}
	// Clean up the target directory on any error after creation.
	var scaffoldErr error
	defer func() {
		if scaffoldErr != nil {
			_ = os.RemoveAll(target)
		}
	}()
	for relPath, content := range tmpl.Files {
		rendered, err := renderTemplate(relPath, content, data)
		if err != nil {
			scaffoldErr = fmt.Errorf("scaffold: render %s: %w", relPath, err)
			return scaffoldErr
		}
		fullPath := filepath.Join(target, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			scaffoldErr = fmt.Errorf("scaffold: create parent for %s: %w", relPath, err)
			return scaffoldErr
		}
		if err := os.WriteFile(fullPath, []byte(rendered), 0o600); err != nil {
			scaffoldErr = fmt.Errorf("scaffold: write %s: %w", relPath, err)
			return scaffoldErr
		}
	}
	// Validate the generated manifest to guarantee correctness.
	if _, err := LoadManifest(target); err != nil {
		scaffoldErr = fmt.Errorf("scaffold: generated manifest is invalid: %w", err)
		return scaffoldErr
	}
	return nil
}

// findTemplate looks up a template by name.
func findTemplate(name string) *Template {
	templates := getBuiltinTemplates()
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}
	return nil
}

// renderTemplate processes content through text/template with the given data.
func renderTemplate(name, content string, data scaffoldData) (string, error) {
	t, err := template.New(name).Parse(content)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
