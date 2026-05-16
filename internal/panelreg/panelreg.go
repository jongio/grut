// Package panelreg provides a global panel builder registry that enables
// panel packages to self-register via init() without creating import cycles
// between layout and individual panel packages.
package panelreg

import (
	"context"
	"os"
	"sync"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// Deps holds runtime dependencies available to panel builders.
// It is populated once at startup and passed to every registered builder.
type Deps struct {
	Ctx       context.Context
	Config    *config.Config
	GitClient git.GitClient // shared client; may be nil
	Theme     *theme.Theme  // may be nil
}

// Cwd returns the current working directory, falling back to "." on error.
func (d Deps) Cwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// NewGitClient creates a fresh git client for the current working directory.
// Returns (client, cwd, error).
func (d Deps) NewGitClient() (*git.Client, string, error) {
	cwd := d.Cwd()
	client, err := git.NewClient(cwd)
	return client, cwd, err
}

// ApplyActionsCfg injects the actions configuration into panels that support
// the optional SetActionsCfg method.
func (d Deps) ApplyActionsCfg(p panels.Panel) panels.Panel {
	if ac, ok := p.(interface{ SetActionsCfg(config.ActionsConfig) }); ok {
		ac.SetActionsCfg(d.Config.Actions)
	}
	return p
}

// Placeholder returns a placeholder panel for the given name, using the
// theme from deps.
func (d Deps) Placeholder(name string) panels.Panel {
	return panels.NewPlaceholder(name, d.Theme)
}

// Builder constructs a panel given runtime dependencies.
type Builder func(deps Deps) panels.Panel

var (
	mu       sync.Mutex
	builders = make(map[string]Builder)
)

// Register adds a panel builder under the given name. Panel packages call
// this from init() to self-register. If a builder with that name already
// exists, it is replaced.
func Register(name string, b Builder) {
	mu.Lock()
	defer mu.Unlock()
	builders[name] = b
}

// Builders returns a snapshot of all registered builders.
func Builders() map[string]Builder {
	mu.Lock()
	defer mu.Unlock()
	m := make(map[string]Builder, len(builders))
	for k, v := range builders {
		m[k] = v
	}
	return m
}

// Reset clears all registered builders. Used by tests only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	builders = make(map[string]Builder)
}
