package layout

import (
	"context"
	"fmt"
	"sync"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

// PanelFactory is a constructor function for creating panels by name.
type PanelFactory func() panels.Panel

// Registry maps panel names to their factory functions, enabling dynamic
// panel creation by the layout engine.
type Registry struct {
	factories map[string]PanelFactory
	mu        sync.RWMutex
}

// NewRegistry creates a new empty panel registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]PanelFactory),
	}
}

// Register adds a panel factory under the given name. If a factory with
// that name already exists, it is replaced.
func (r *Registry) Register(name string, factory PanelFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Create instantiates a panel by name using its registered factory.
// Returns an error if the name is not registered.
func (r *Registry) Create(name string) (panels.Panel, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown panel: %s", name)
	}
	return factory(), nil
}

// Has returns true if a factory is registered for the given name.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

// Names returns all registered panel names in no particular order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// RegisterDefaults registers the built-in panels. Panel packages self-register
// their builders via init() into the panelreg global registry. This function
// iterates those builders, supplying runtime dependencies, and registers the
// resulting factories into the layout Registry.
//
// Panels still under development that lack a self-registered builder are
// registered as placeholders here.
func RegisterDefaults(ctx context.Context, r *Registry, cfg *config.Config, gc git.GitClient, th *theme.Theme) {
	deps := panelreg.Deps{
		Ctx:       ctx,
		Config:    cfg,
		GitClient: gc,
		Theme:     th,
	}

	// Self-registered panels: each panel package's init() called
	// panelreg.Register with a builder that accepts Deps.
	for name, builder := range panelreg.Builders() {
		b := builder // capture for closure
		r.Register(name, func() panels.Panel {
			return b(deps)
		})
	}

	// TODO: status panel is intentionally a placeholder awaiting implementation.
	// Panels still using placeholders (no self-registered builder yet).
	for _, name := range []string{slotStatus} {
		if !r.Has(name) {
			n := name // capture for closure
			r.Register(n, func() panels.Panel {
				return panels.NewPlaceholder(n, th)
			})
		}
	}
}
