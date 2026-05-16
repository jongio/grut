package ops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// JSON round-trip
// ---------------------------------------------------------------------------

func TestPRDescription_ValidJSON(t *testing.T) {
	desc := PRDescription{
		Title:           "Add user authentication",
		Summary:         "Implements JWT-based auth for the API.",
		Changes:         []string{"Add auth middleware", "Add login endpoint"},
		TestingNotes:    "Run the auth tests with `go test ./...`",
		BreakingChanges: []string{"API endpoints now require a Bearer token"},
	}

	data, err := json.Marshal(desc)
	require.NoError(t, err)

	var got PRDescription
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "Add user authentication", got.Title)
	assert.Equal(t, "Implements JWT-based auth for the API.", got.Summary)
	require.Len(t, got.Changes, 2)
	assert.Equal(t, "Add auth middleware", got.Changes[0])
	assert.Equal(t, "Add login endpoint", got.Changes[1])
	assert.Equal(t, "Run the auth tests with `go test ./...`", got.TestingNotes)
	require.Len(t, got.BreakingChanges, 1)
	assert.Equal(t, "API endpoints now require a Bearer token", got.BreakingChanges[0])
}

func TestPRDescription_EmptyOptionalFields(t *testing.T) {
	data := `{"title":"Fix typo","summary":"Corrects a typo in README.","changes":["Fix typo in README"]}`
	var got PRDescription
	require.NoError(t, json.Unmarshal([]byte(data), &got))

	assert.Equal(t, "Fix typo", got.Title)
	assert.Empty(t, got.TestingNotes)
	assert.Empty(t, got.BreakingChanges)
}

func TestPRDescription_FullMarkdownOmittedFromJSON(t *testing.T) {
	desc := PRDescription{
		Title:        "Test",
		Summary:      "Test summary",
		FullMarkdown: "should not appear in JSON",
	}

	data, err := json.Marshal(desc)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "FullMarkdown")
	assert.NotContains(t, string(data), "should not appear")
}

// ---------------------------------------------------------------------------
// parsePRResponse
// ---------------------------------------------------------------------------

func TestParsePRResponse_ValidJSON(t *testing.T) {
	raw := `{"title":"Add feature","summary":"Adds a new feature.","changes":["Add handler"],"testing_notes":"Run tests","breaking_changes":[]}`
	desc, err := parsePRResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, "Add feature", desc.Title)
	assert.Equal(t, "Adds a new feature.", desc.Summary)
	require.Len(t, desc.Changes, 1)
	assert.Equal(t, "Run tests", desc.TestingNotes)
	assert.Empty(t, desc.BreakingChanges)
}

func TestParsePRResponse_CodeFences(t *testing.T) {
	raw := "```json\n" + `{"title":"Fix bug","summary":"Fixes a crash.","changes":["Fix nil check"],"testing_notes":"","breaking_changes":[]}` + "\n```"
	desc, err := parsePRResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, "Fix bug", desc.Title)
	assert.Equal(t, "Fixes a crash.", desc.Summary)
}

func TestParsePRResponse_CodeFencesNoLanguageTag(t *testing.T) {
	raw := "```\n" + `{"title":"T","summary":"S","changes":[],"testing_notes":"","breaking_changes":[]}` + "\n```"
	desc, err := parsePRResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, "T", desc.Title)
}

func TestParsePRResponse_InvalidJSON(t *testing.T) {
	_, err := parsePRResponse("this is not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestParsePRResponse_Whitespace(t *testing.T) {
	raw := `  
  {"title":"T","summary":"S","changes":[],"testing_notes":"","breaking_changes":[]}  
  `
	desc, err := parsePRResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, "T", desc.Title)
}

// ---------------------------------------------------------------------------
// buildPRMarkdown
// ---------------------------------------------------------------------------

func TestBuildPRMarkdown_AllSections(t *testing.T) {
	desc := &PRDescription{
		Title:           "Add auth",
		Summary:         "Adds JWT authentication.",
		Changes:         []string{"Add middleware", "Add login route"},
		TestingNotes:    "Run `go test ./...`",
		BreakingChanges: []string{"Endpoints require tokens"},
	}

	md := buildPRMarkdown(desc)

	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "Adds JWT authentication.")
	assert.Contains(t, md, "## Changes")
	assert.Contains(t, md, "- Add middleware")
	assert.Contains(t, md, "- Add login route")
	assert.Contains(t, md, "## Testing")
	assert.Contains(t, md, "Run `go test ./...`")
	assert.Contains(t, md, "## Breaking Changes")
	assert.Contains(t, md, "- Endpoints require tokens")
}

func TestBuildPRMarkdown_NoOptionalSections(t *testing.T) {
	desc := &PRDescription{
		Summary: "Simple fix.",
		Changes: []string{"Fix typo"},
	}

	md := buildPRMarkdown(desc)

	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "## Changes")
	assert.NotContains(t, md, "## Testing")
	assert.NotContains(t, md, "## Breaking Changes")
}

func TestBuildPRMarkdown_NoChanges(t *testing.T) {
	desc := &PRDescription{
		Summary: "Cleanup.",
	}

	md := buildPRMarkdown(desc)

	assert.Contains(t, md, "## Summary")
	assert.NotContains(t, md, "## Changes")
}

// ---------------------------------------------------------------------------
// Generate (integration with mocks)
// ---------------------------------------------------------------------------

func TestPRDescriptionGenerator_Generate(t *testing.T) {
	respJSON := `{"title":"Add feature X","summary":"Implements feature X for users.","changes":["Add handler","Add tests"],"testing_notes":"Run go test","breaking_changes":[]}`
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response: ai.CompletionResponse{
			Content: respJSON,
		},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("feature/x", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	desc, err := gen.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.NotNil(t, desc)
	assert.Equal(t, "Add feature X", desc.Title)
	assert.Equal(t, "Implements feature X for users.", desc.Summary)
	require.Len(t, desc.Changes, 2)
	assert.Equal(t, "Add handler", desc.Changes[0])
	assert.Equal(t, "Run go test", desc.TestingNotes)
	assert.Empty(t, desc.BreakingChanges)

	// FullMarkdown should be populated.
	assert.Contains(t, desc.FullMarkdown, "## Summary")
	assert.Contains(t, desc.FullMarkdown, "Implements feature X for users.")
	assert.Contains(t, desc.FullMarkdown, "- Add handler")
}

func TestPRDescriptionGenerator_Generate_WithBreakingChanges(t *testing.T) {
	respJSON := `{"title":"Remove deprecated API","summary":"Drops v1 endpoints.","changes":["Remove v1 routes"],"testing_notes":"","breaking_changes":["v1 API no longer available"]}`
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response: ai.CompletionResponse{
			Content: respJSON,
		},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("breaking/remove-v1", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	desc, err := gen.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, desc.BreakingChanges, 1)
	assert.Equal(t, "v1 API no longer available", desc.BreakingChanges[0])
	assert.Contains(t, desc.FullMarkdown, "## Breaking Changes")
}

func TestPRDescriptionGenerator_Generate_InvalidJSON(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response: ai.CompletionResponse{
			Content: "not json at all",
		},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AI response")
}

func TestPRDescriptionGenerator_Generate_ProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		err:       errors.New("provider down"),
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion")
}

func TestPRDescriptionGenerator_Generate_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	_, err := gen.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AI provider")
}

func TestNewPRDescriptionGenerator(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewPRDescriptionGenerator(registry, builder)
	assert.NotNil(t, gen)
}
