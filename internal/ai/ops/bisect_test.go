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

func TestBisectAnalysis_ValidJSON(t *testing.T) {
	analysis := BisectAnalysis{
		Summary: "Most likely culprit is commit def456",
		Candidates: []BisectCandidate{
			{Hash: "def456", Subject: "refactor: auth logic", Probability: 0.8, Reason: "touches critical auth path"},
			{Hash: "abc123", Subject: "docs: update readme", Probability: 0.1, Reason: "documentation only"},
		},
	}

	data, err := json.Marshal(analysis)
	require.NoError(t, err)

	var got BisectAnalysis
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "Most likely culprit is commit def456", got.Summary)
	require.Len(t, got.Candidates, 2)
	assert.Equal(t, "def456", got.Candidates[0].Hash)
	assert.InDelta(t, 0.8, got.Candidates[0].Probability, 0.001)
	assert.InDelta(t, 0.1, got.Candidates[1].Probability, 0.001)
}

func TestBisectAnalysis_EmptyCandidates(t *testing.T) {
	data := `{"candidates":[],"summary":"no commits in range"}`
	var got BisectAnalysis
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Empty(t, got.Candidates)
	assert.Equal(t, "no commits in range", got.Summary)
}

func TestBisectCandidate_ProbabilityBounds(t *testing.T) {
	data := `{"hash":"aaa","subject":"test","probability":0.0,"reason":"safe"}`
	var got BisectCandidate
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.InDelta(t, 0.0, got.Probability, 0.001)

	data = `{"hash":"bbb","subject":"test","probability":1.0,"reason":"certain"}`
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.InDelta(t, 1.0, got.Probability, 0.001)
}

// ---------------------------------------------------------------------------
// With mock provider
// ---------------------------------------------------------------------------

func TestBisectAnalyzer_Analyze(t *testing.T) {
	respJSON := `{"candidates":[{"hash":"abc","subject":"feat: change","probability":0.7,"reason":"risky change"}],"summary":"abc is the likely culprit"}`
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response:  ai.CompletionResponse{Content: respJSON},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBisectAnalyzer(registry, builder)
	analysis, err := analyzer.Analyze(context.Background(), "v1.0", "HEAD")
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Len(t, analysis.Candidates, 1)
	assert.Equal(t, "abc", analysis.Candidates[0].Hash)
	assert.InDelta(t, 0.7, analysis.Candidates[0].Probability, 0.001)
	assert.Equal(t, "abc is the likely culprit", analysis.Summary)
}

func TestBisectAnalyzer_Analyze_InvalidJSON(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response:  ai.CompletionResponse{Content: "{broken"},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBisectAnalyzer(registry, builder)
	_, err := analyzer.Analyze(context.Background(), "v1.0", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AI response")
}

func TestBisectAnalyzer_Analyze_ProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		err:       errors.New("timeout"),
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBisectAnalyzer(registry, builder)
	_, err := analyzer.Analyze(context.Background(), "v1.0", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion")
}

func TestBisectAnalyzer_Analyze_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})

	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBisectAnalyzer(registry, builder)
	_, err := analyzer.Analyze(context.Background(), "v1.0", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AI provider")
}

func TestNewBisectAnalyzer(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBisectAnalyzer(registry, builder)
	assert.NotNil(t, analyzer)
}
