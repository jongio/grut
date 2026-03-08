package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock provider
// ---------------------------------------------------------------------------

// mockProvider is a minimal AIProvider for testing registry behaviour.
type mockProvider struct {
	name      string
	available bool
	closeErr  error
	closed    bool
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Available(_ context.Context) (bool, error) {
	return m.available, nil
}

func (m *mockProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, nil
}

func (m *mockProvider) CompleteStream(_ context.Context, _ CompletionRequest) (<-chan StreamChunk, error) {
	return nil, nil
}

func (m *mockProvider) Close() error {
	m.closed = true
	return m.closeErr
}

// ---------------------------------------------------------------------------
// NewRegistry
// ---------------------------------------------------------------------------

func TestNewRegistry(t *testing.T) {
	cfg := config.AIConfig{Provider: "openai", FallbackProvider: "ollama"}
	r := NewRegistry(cfg)

	assert.NotNil(t, r)
	assert.Equal(t, "openai", r.primary)
	assert.Equal(t, "ollama", r.fallback)
	assert.Empty(t, r.providers)
}

// ---------------------------------------------------------------------------
// Register + GetByName
// ---------------------------------------------------------------------------

func TestRegisterAndGetByName(t *testing.T) {
	r := NewRegistry(config.AIConfig{})
	p := &mockProvider{name: "test"}

	r.Register("test", p)

	got, ok := r.GetByName("test")
	require.True(t, ok)
	assert.Equal(t, p, got)
}

func TestGetByNameNotFound(t *testing.T) {
	r := NewRegistry(config.AIConfig{})

	_, ok := r.GetByName("nonexistent")
	assert.False(t, ok)
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewRegistry(config.AIConfig{})
	original := &mockProvider{name: "original"}
	replacement := &mockProvider{name: "replacement"}

	r.Register("slot", original)
	r.Register("slot", replacement)

	got, ok := r.GetByName("slot")
	require.True(t, ok)
	assert.Equal(t, "replacement", got.Name())
}

// ---------------------------------------------------------------------------
// Get (primary / fallback resolution)
// ---------------------------------------------------------------------------

func TestGetReturnsPrimary(t *testing.T) {
	cfg := config.AIConfig{Provider: "primary", FallbackProvider: "fallback"}
	r := NewRegistry(cfg)
	r.Register("primary", &mockProvider{name: "primary", available: true})
	r.Register("fallback", &mockProvider{name: "fallback", available: true})

	got, err := r.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "primary", got.Name())
}

func TestGetFallsBackWhenPrimaryUnavailable(t *testing.T) {
	cfg := config.AIConfig{Provider: "primary", FallbackProvider: "fallback"}
	r := NewRegistry(cfg)
	r.Register("primary", &mockProvider{name: "primary", available: false})
	r.Register("fallback", &mockProvider{name: "fallback", available: true})

	got, err := r.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fallback", got.Name())
}

func TestGetFallsBackWhenPrimaryNotRegistered(t *testing.T) {
	cfg := config.AIConfig{Provider: "missing", FallbackProvider: "fallback"}
	r := NewRegistry(cfg)
	r.Register("fallback", &mockProvider{name: "fallback", available: true})

	got, err := r.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fallback", got.Name())
}

func TestGetErrorWhenBothUnavailable(t *testing.T) {
	cfg := config.AIConfig{Provider: "primary", FallbackProvider: "fallback"}
	r := NewRegistry(cfg)
	r.Register("primary", &mockProvider{name: "primary", available: false})
	r.Register("fallback", &mockProvider{name: "fallback", available: false})

	_, err := r.Get(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available AI provider")
	assert.Contains(t, err.Error(), "primary")
	assert.Contains(t, err.Error(), "fallback")
}

func TestGetErrorWhenNoneRegistered(t *testing.T) {
	cfg := config.AIConfig{Provider: "primary", FallbackProvider: "fallback"}
	r := NewRegistry(cfg)

	_, err := r.Get(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available AI provider")
}

func TestGetErrorWhenNoProvidersConfigured(t *testing.T) {
	r := NewRegistry(config.AIConfig{})

	_, err := r.Get(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available AI provider")
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestCloseAllProviders(t *testing.T) {
	r := NewRegistry(config.AIConfig{})
	p1 := &mockProvider{name: "one"}
	p2 := &mockProvider{name: "two"}

	r.Register("one", p1)
	r.Register("two", p2)

	err := r.Close()
	require.NoError(t, err)
	assert.True(t, p1.closed)
	assert.True(t, p2.closed)
}

func TestCloseCollectsErrors(t *testing.T) {
	r := NewRegistry(config.AIConfig{})
	r.Register("good", &mockProvider{name: "good"})
	r.Register("bad", &mockProvider{name: "bad", closeErr: errors.New("boom")})

	err := r.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), `closing provider "bad"`)
}

func TestCloseEmptyRegistry(t *testing.T) {
	r := NewRegistry(config.AIConfig{})

	err := r.Close()
	assert.NoError(t, err)
}
