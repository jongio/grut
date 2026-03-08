package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jongio/grut/internal/config"
)

// Registry manages registered AI providers and resolves the active one
// based on configuration (primary + fallback).
type Registry struct {
	mu        sync.RWMutex
	providers map[string]AIProvider
	primary   string
	fallback  string
}

// NewRegistry creates a registry configured with the given AI settings.
// It does NOT register any providers — call Register() to add them.
func NewRegistry(cfg config.AIConfig) *Registry {
	return &Registry{
		providers: make(map[string]AIProvider),
		primary:   cfg.Provider,
		fallback:  cfg.FallbackProvider,
	}
}

// Register adds a provider under the given name. If a provider with that
// name already exists, it is replaced.
func (r *Registry) Register(name string, p AIProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Get returns the first available provider: tries primary, then fallback.
// Returns an error if neither is registered or available.
func (r *Registry) Get(ctx context.Context) (AIProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try the primary provider.
	if r.primary != "" {
		if p, ok := r.providers[r.primary]; ok {
			avail, err := p.Available(ctx)
			if err == nil && avail {
				return p, nil
			}
		}
	}

	// Try the fallback provider.
	if r.fallback != "" {
		if p, ok := r.providers[r.fallback]; ok {
			avail, err := p.Available(ctx)
			if err == nil && avail {
				return p, nil
			}
		}
	}

	return nil, fmt.Errorf("no available AI provider (primary=%q, fallback=%q)", r.primary, r.fallback)
}

// GetByName returns a specific provider by name, or false if not found.
func (r *Registry) GetByName(name string) (AIProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// PrimaryName returns the configured primary provider name.
func (r *Registry) PrimaryName() string {
	return r.primary
}

// Close shuts down all registered providers, collecting any errors.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, p := range r.providers {
		if err := p.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing provider %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
