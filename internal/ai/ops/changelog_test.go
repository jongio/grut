package ops

import (
	"context"
	"fmt"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// JSON parsing
// ---------------------------------------------------------------------------

func TestParseChangelogResponse_ValidJSON(t *testing.T) {
	input := `[
		{"category": "added", "description": "New login page", "commit_hashes": ["abc1234"]},
		{"category": "fixed", "description": "Crash on empty input", "commit_hashes": ["def5678", "ghi9012"]}
	]`

	entries, err := parseChangelogResponse(input)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Equal(t, "added", entries[0].Category)
	assert.Equal(t, "New login page", entries[0].Description)
	assert.Equal(t, []string{"abc1234"}, entries[0].CommitHashes)

	assert.Equal(t, "fixed", entries[1].Category)
	assert.Equal(t, "Crash on empty input", entries[1].Description)
	assert.Equal(t, []string{"def5678", "ghi9012"}, entries[1].CommitHashes)
}

func TestParseChangelogResponse_AllCategories(t *testing.T) {
	categories := []string{"added", "changed", "fixed", "removed", "security", "deprecated"}
	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			input := fmt.Sprintf(`[{"category": %q, "description": "desc", "commit_hashes": ["aaa"]}]`, cat)
			entries, err := parseChangelogResponse(input)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, cat, entries[0].Category)
		})
	}
}

func TestParseChangelogResponse_InvalidCategory(t *testing.T) {
	input := `[{"category": "invented", "description": "desc", "commit_hashes": ["aaa"]}]`
	_, err := parseChangelogResponse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid category")
	assert.Contains(t, err.Error(), "invented")
}

func TestParseChangelogResponse_CaseNormalization(t *testing.T) {
	input := `[{"category": "ADDED", "description": "desc", "commit_hashes": ["aaa"]}]`
	entries, err := parseChangelogResponse(input)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "added", entries[0].Category)
}

func TestParseChangelogResponse_StripCodeFences(t *testing.T) {
	input := "```json\n" + `[{"category": "fixed", "description": "bug fix", "commit_hashes": ["bbb"]}]` + "\n```"
	entries, err := parseChangelogResponse(input)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "fixed", entries[0].Category)
}

func TestParseChangelogResponse_InvalidJSON(t *testing.T) {
	_, err := parseChangelogResponse("not json at all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling changelog JSON")
}

func TestParseChangelogResponse_EmptyArray(t *testing.T) {
	entries, err := parseChangelogResponse("[]")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// ---------------------------------------------------------------------------
// Generate with mock provider
// ---------------------------------------------------------------------------

func TestGenerate_Success(t *testing.T) {
	provider := &mockAIProvider{
		name:      "stub",
		available: true,
		response: ai.CompletionResponse{
			Content: `[{"category": "added", "description": "New feature", "commit_hashes": ["aaa1111"]}]`,
		},
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	gen := NewChangelogGenerator(registry, builder)
	entries, err := gen.Generate(context.Background(), "v1.0", "v2.0")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "added", entries[0].Category)
	assert.Equal(t, "New feature", entries[0].Description)
}

func TestGenerate_ProviderError(t *testing.T) {
	provider := &mockAIProvider{
		name:      "stub",
		available: true,
		err:       fmt.Errorf("provider unavailable"),
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	gen := NewChangelogGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "v1.0", "v2.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion for changelog")
}

func TestGenerate_InvalidResponse(t *testing.T) {
	provider := &mockAIProvider{
		name:      "stub",
		available: true,
		response: ai.CompletionResponse{
			Content: `{"not": "an array"}`,
		},
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	gen := NewChangelogGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "v1.0", "v2.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing changelog response")
}

func TestGenerate_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	gen := NewChangelogGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "v1.0", "v2.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting AI provider")
}

// ---------------------------------------------------------------------------
// NewChangelogGenerator
// ---------------------------------------------------------------------------

func TestNewChangelogGenerator(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	gen := NewChangelogGenerator(registry, builder)
	require.NotNil(t, gen)
	assert.Equal(t, registry, gen.registry)
	assert.Equal(t, builder, gen.builder)
}
