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
// JSON parsing
// ---------------------------------------------------------------------------

func TestBranchRecommendation_ValidJSON(t *testing.T) {
	recs := branchResponse{
		Branches: []BranchRecommendation{
			{Name: "main", Action: "keep", Reason: "primary branch"},
			{Name: "old-feature", Action: "delete", Reason: "already merged"},
			{Name: "BadName", Action: "rename", Reason: "non-conventional name", NewName: "fix/bad-name"},
		},
	}

	data, err := json.Marshal(recs)
	require.NoError(t, err)

	var got branchResponse
	require.NoError(t, json.Unmarshal(data, &got))

	require.Len(t, got.Branches, 3)
	assert.Equal(t, "keep", got.Branches[0].Action)
	assert.Equal(t, "delete", got.Branches[1].Action)
	assert.Equal(t, "rename", got.Branches[2].Action)
	assert.Equal(t, "fix/bad-name", got.Branches[2].NewName)
}

func TestBranchRecommendation_EmptyList(t *testing.T) {
	data := `{"branches":[]}`
	var got branchResponse
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Empty(t, got.Branches)
}

func TestBranchRecommendation_NewNameOmitted(t *testing.T) {
	data := `{"branches":[{"name":"main","action":"keep","reason":"primary"}]}`
	var got branchResponse
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	assert.Empty(t, got.Branches[0].NewName)
}

// ---------------------------------------------------------------------------
// formatBranchList
// ---------------------------------------------------------------------------

func TestFormatBranchList(t *testing.T) {
	branches := []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "feature/auth", Upstream: "origin/feature/auth", Ahead: 2, Behind: 0},
		{Name: "old-branch"},
	}

	result := formatBranchList(branches)
	assert.Contains(t, result, `* "main"`)
	assert.Contains(t, result, `  "feature/auth"`)
	assert.Contains(t, result, `tracking: "origin/feature/auth"`)
	assert.Contains(t, result, "ahead 2, behind 0")
	assert.Contains(t, result, `  "old-branch"`)
}

func TestFormatBranchList_NoTracking(t *testing.T) {
	branches := []git.Branch{
		{Name: "dev"},
	}
	result := formatBranchList(branches)
	assert.Contains(t, result, `  "dev"`)
	assert.NotContains(t, result, "tracking")
}

// ---------------------------------------------------------------------------
// With mock provider
// ---------------------------------------------------------------------------

func TestBranchAnalyzer_Analyze(t *testing.T) {
	respJSON := `{"branches":[{"name":"main","action":"keep","reason":"primary branch"},{"name":"stale","action":"delete","reason":"merged and inactive"}]}`
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response:  ai.CompletionResponse{Content: respJSON},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return []git.Branch{
			{Name: "main", IsCurrent: true},
			{Name: "stale"},
		}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	recs, err := analyzer.Analyze(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, "keep", recs[0].Action)
	assert.Equal(t, "delete", recs[1].Action)
}

func TestBranchAnalyzer_Analyze_NoBranches(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return nil, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	recs, err := analyzer.Analyze(context.Background())
	require.NoError(t, err)
	assert.Nil(t, recs)
}

func TestBranchAnalyzer_Analyze_BranchListError(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return nil, errors.New("git error")
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	_, err := analyzer.Analyze(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing branches")
}

func TestBranchAnalyzer_Analyze_InvalidJSON(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		response:  ai.CompletionResponse{Content: "not valid json"},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return []git.Branch{{Name: "main", IsCurrent: true}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	_, err := analyzer.Analyze(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AI response")
}

func TestBranchAnalyzer_Analyze_ProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
		err:       errors.New("service unavailable"),
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	registry.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return []git.Branch{{Name: "main", IsCurrent: true}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	_, err := analyzer.Analyze(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion")
}

func TestBranchAnalyzer_Analyze_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})

	client := newMockGitClient("main", "/repo")
	client.BranchListFunc = func(_ context.Context) ([]git.Branch, error) {
		return []git.Branch{{Name: "main", IsCurrent: true}}, nil
	}
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	_, err := analyzer.Analyze(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AI provider")
}

func TestNewBranchAnalyzer(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	analyzer := NewBranchAnalyzer(registry, builder, client)
	assert.NotNil(t, analyzer)
}
