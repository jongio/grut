package ops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// JSON round-trip
// ---------------------------------------------------------------------------

func TestCommitSuggestion_ValidJSON(t *testing.T) {
	suggestion := CommitSuggestion{
		Type:    "feat",
		Scope:   "auth",
		Subject: "add login endpoint",
		Body:    "Implements OAuth2 login flow",
	}

	data, err := json.Marshal(suggestion)
	require.NoError(t, err)

	var got CommitSuggestion
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "feat", got.Type)
	assert.Equal(t, "auth", got.Scope)
	assert.Equal(t, "add login endpoint", got.Subject)
	assert.Equal(t, "Implements OAuth2 login flow", got.Body)
}

func TestCommitSuggestion_NoScopeNoBody(t *testing.T) {
	data := `{"type":"fix","subject":"correct nil pointer dereference"}`
	var got CommitSuggestion
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Equal(t, "fix", got.Type)
	assert.Empty(t, got.Scope)
	assert.Equal(t, "correct nil pointer dereference", got.Subject)
	assert.Empty(t, got.Body)
}

// ---------------------------------------------------------------------------
// parseCommitResponse
// ---------------------------------------------------------------------------

func TestParseCommitResponse_Valid(t *testing.T) {
	input := `{"type": "feat", "scope": "api", "subject": "add health endpoint", "body": "Returns 200 OK"}`
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "feat", suggestion.Type)
	assert.Equal(t, "api", suggestion.Scope)
	assert.Equal(t, "add health endpoint", suggestion.Subject)
	assert.Equal(t, "Returns 200 OK", suggestion.Body)
}

func TestParseCommitResponse_InvalidJSON(t *testing.T) {
	_, err := parseCommitResponse("{broken")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling commit JSON")
}

func TestParseCommitResponse_InvalidType(t *testing.T) {
	input := `{"type":"banana","subject":"something"}`
	_, err := parseCommitResponse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid commit type")
}

func TestParseCommitResponse_EmptySubject(t *testing.T) {
	input := `{"type":"fix","subject":""}`
	_, err := parseCommitResponse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty commit subject")
}

func TestParseCommitResponse_WhitespaceOnlySubject(t *testing.T) {
	input := `{"type":"fix","subject":"   "}`
	_, err := parseCommitResponse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty commit subject")
}

func TestParseCommitResponse_SubjectTruncation(t *testing.T) {
	long := "this subject line is definitely longer than fifty characters total"
	input := `{"type":"feat","subject":"` + long + `"}`
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Len(t, suggestion.Subject, maxSubjectLength)
	assert.Equal(t, long[:maxSubjectLength], suggestion.Subject)
}

func TestParseCommitResponse_SubjectExactly50Chars(t *testing.T) {
	// Exactly 50 characters — should not be truncated.
	subject := "12345678901234567890123456789012345678901234567890"
	require.Len(t, subject, 50)

	input := `{"type":"fix","subject":"` + subject + `"}`
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Equal(t, subject, suggestion.Subject)
}

func TestParseCommitResponse_NormalizesType(t *testing.T) {
	input := `{"type":" FEAT ","subject":"add something"}`
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "feat", suggestion.Type)
}

func TestParseCommitResponse_NormalizesScope(t *testing.T) {
	input := `{"type":"fix","scope":" API ","subject":"correct endpoint"}`
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "api", suggestion.Scope)
}

func TestParseCommitResponse_StripsCodeFences(t *testing.T) {
	input := "```json\n" + `{"type":"fix","subject":"correct bug"}` + "\n```"
	suggestion, err := parseCommitResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "fix", suggestion.Type)
	assert.Equal(t, "correct bug", suggestion.Subject)
}

func TestParseCommitResponse_AllValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "docs", "style", "refactor", "test", "chore"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			input := `{"type":"` + typ + `","subject":"do something"}`
			suggestion, err := parseCommitResponse(input)
			require.NoError(t, err)
			assert.Equal(t, typ, suggestion.Type)
		})
	}
}

// ---------------------------------------------------------------------------
// CommitSuggestion.String
// ---------------------------------------------------------------------------

func TestCommitSuggestion_String_WithScope(t *testing.T) {
	s := CommitSuggestion{Type: "feat", Scope: "auth", Subject: "add login", Body: "OAuth2 support"}
	assert.Equal(t, "feat(auth): add login\n\nOAuth2 support", s.String())
}

func TestCommitSuggestion_String_WithoutScope(t *testing.T) {
	s := CommitSuggestion{Type: "fix", Subject: "correct typo"}
	assert.Equal(t, "fix: correct typo", s.String())
}

func TestCommitSuggestion_String_WithoutBody(t *testing.T) {
	s := CommitSuggestion{Type: "docs", Scope: "readme", Subject: "update installation"}
	assert.Equal(t, "docs(readme): update installation", s.String())
}

// ---------------------------------------------------------------------------
// With mock provider
// ---------------------------------------------------------------------------

func TestCommitGenerator_Generate(t *testing.T) {
	respJSON := `{"type":"feat","scope":"api","subject":"add health endpoint","body":"Returns 200 OK"}`
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: respJSON,
		},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{{Path: "main.go"}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	suggestion, err := gen.Generate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, suggestion)
	assert.Equal(t, "feat", suggestion.Type)
	assert.Equal(t, "api", suggestion.Scope)
	assert.Equal(t, "add health endpoint", suggestion.Subject)
	assert.Equal(t, "Returns 200 OK", suggestion.Body)
}

func TestCommitGenerator_Generate_NoStagedChanges(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	// No DiffFunc set → returns nil diffs by default.
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	suggestion, err := gen.Generate(context.Background())
	require.NoError(t, err)
	assert.Nil(t, suggestion)
}

func TestCommitGenerator_Generate_InvalidJSON(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: "{broken",
		},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{{Path: "main.go"}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	_, err := gen.Generate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing commit response")
}

func TestCommitGenerator_Generate_ProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:        "mock",
		available:   true,
		completeErr: errors.New("rate limited"),
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{{Path: "main.go"}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	_, err := gen.Generate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion for commit")
}

func TestCommitGenerator_Generate_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})

	client := newMockGitClient("main", "/repo")
	client.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{{Path: "main.go"}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	_, err := gen.Generate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting AI provider")
}

func TestNewCommitGenerator(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	gen := NewCommitGenerator(registry, builder)
	assert.NotNil(t, gen)
}
