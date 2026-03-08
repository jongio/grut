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
// JSON parsing
// ---------------------------------------------------------------------------

func TestRebaseSuggestion_ValidJSON(t *testing.T) {
	suggestion := RebaseSuggestion{
		Commits: []CommitAction{
			{Hash: "abc123", Subject: "feat: add login", Action: "pick", Reason: "meaningful commit"},
			{Hash: "def456", Subject: "wip", Action: "squash", Reason: "WIP commit should be squashed"},
			{Hash: "ghi789", Subject: "fix tpyo", Action: "reword", Reason: "typo in message", NewSubject: "fix: typo in auth"},
		},
	}

	data, err := json.Marshal(suggestion)
	require.NoError(t, err)

	var got RebaseSuggestion
	require.NoError(t, json.Unmarshal(data, &got))

	require.Len(t, got.Commits, 3)
	assert.Equal(t, "abc123", got.Commits[0].Hash)
	assert.Equal(t, "pick", got.Commits[0].Action)
	assert.Equal(t, "squash", got.Commits[1].Action)
	assert.Equal(t, "reword", got.Commits[2].Action)
	assert.Equal(t, "fix: typo in auth", got.Commits[2].NewSubject)
}

func TestRebaseSuggestion_EmptyCommits(t *testing.T) {
	data := `{"commits":[]}`
	var got RebaseSuggestion
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Empty(t, got.Commits)
}

func TestRebaseSuggestion_NewSubjectOmitted(t *testing.T) {
	data := `{"commits":[{"hash":"aaa","subject":"feat: x","action":"pick","reason":"good"}]}`
	var got RebaseSuggestion
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Empty(t, got.Commits[0].NewSubject)
}

// ---------------------------------------------------------------------------
// With mock provider
// ---------------------------------------------------------------------------

func TestRebaseAssistant_Suggest(t *testing.T) {
	respJSON := `{"commits":[{"hash":"abc","subject":"feat: add auth","action":"pick","reason":"core feature commit"}]}`
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: respJSON},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("feature/test", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	assistant := NewRebaseAssistant(registry, builder)
	suggestion, err := assistant.Suggest(context.Background(), "main")
	require.NoError(t, err)
	require.NotNil(t, suggestion)
	require.Len(t, suggestion.Commits, 1)
	assert.Equal(t, "abc", suggestion.Commits[0].Hash)
	assert.Equal(t, "pick", suggestion.Commits[0].Action)
}

func TestRebaseAssistant_Suggest_InvalidJSON(t *testing.T) {
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: "not json at all"},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	assistant := NewRebaseAssistant(registry, builder)
	_, err := assistant.Suggest(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AI response")
}

func TestRebaseAssistant_Suggest_ProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:        "mock",
		available:   true,
		completeErr: errors.New("provider down"),
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	assistant := NewRebaseAssistant(registry, builder)
	_, err := assistant.Suggest(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion")
}

func TestRebaseAssistant_Suggest_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	assistant := NewRebaseAssistant(registry, builder)
	_, err := assistant.Suggest(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AI provider")
}

func TestNewRebaseAssistant(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	assistant := NewRebaseAssistant(registry, builder)
	assert.NotNil(t, assistant)
}
