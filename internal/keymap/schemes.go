package keymap

import (
	"embed"
	"fmt"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed schemes/default.toml schemes/vim.toml schemes/classic.toml
var schemesFS embed.FS

// schemeFile is the top-level TOML structure for a keybinding scheme file.
type schemeFile struct {
	Bindings []bindingEntry `toml:"bindings"`
}

// bindingEntry is a single binding as serialised in TOML.
type bindingEntry struct {
	Key         string `toml:"key"`
	Action      string `toml:"action"`
	Mode        string `toml:"mode"`
	Context     string `toml:"context,omitempty"`
	Description string `toml:"description"`
}

// LoadScheme loads key bindings from a built-in scheme name or a filesystem
// path. Built-in names: "default", "classic", "vim".
// Any name containing "/" or "\" is treated as a file path.
func LoadScheme(name string) ([]Binding, error) {
	var data []byte
	var err error

	if isFilePath(name) {
		data, err = os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading scheme file %s: %w", name, err)
		}
	} else {
		filename := "schemes/" + name + ".toml"
		data, err = schemesFS.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("unknown built-in scheme %q: %w", name, err)
		}
	}

	return parseScheme(data)
}

// parseScheme unmarshals TOML data into Binding structs.
func parseScheme(data []byte) ([]Binding, error) {
	var sf schemeFile
	if err := toml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing scheme TOML: %w", err)
	}

	bindings := make([]Binding, 0, len(sf.Bindings))
	for i, entry := range sf.Bindings {
		mode, err := parseKeyMode(entry.Mode)
		if err != nil {
			return nil, fmt.Errorf("binding %d (%s): %w", i, entry.Key, err)
		}
		bindings = append(bindings, Binding{
			Key:         entry.Key,
			Action:      entry.Action,
			Mode:        mode,
			Context:     entry.Context,
			Description: entry.Description,
		})
	}

	return bindings, nil
}

// isFilePath reports whether name looks like a filesystem path rather than
// a built-in scheme name.
func isFilePath(name string) bool {
	return strings.ContainsAny(name, `/\`)
}
