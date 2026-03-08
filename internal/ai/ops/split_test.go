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

func TestParseSplitResponse_ValidJSON(t *testing.T) {
	input := `{
		"pieces": [
			{
				"files": ["cmd/main.go", "cmd/root.go"],
				"commit_message": "refactor: reorganize CLI entrypoint",
				"reason": "Both files are part of the CLI layer",
				"order": 1
			},
			{
				"files": ["internal/auth/token.go"],
				"commit_message": "feat: add token refresh logic",
				"reason": "Standalone auth change",
				"order": 2
			}
		]
	}`

	plan, err := parseSplitResponse(input, "abc123")
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Equal(t, "abc123", plan.OriginalHash)
	require.Len(t, plan.Pieces, 2)

	assert.Equal(t, []string{"cmd/main.go", "cmd/root.go"}, plan.Pieces[0].Files)
	assert.Equal(t, "refactor: reorganize CLI entrypoint", plan.Pieces[0].CommitMessage)
	assert.Equal(t, "Both files are part of the CLI layer", plan.Pieces[0].Reason)
	assert.Equal(t, 1, plan.Pieces[0].Order)

	assert.Equal(t, []string{"internal/auth/token.go"}, plan.Pieces[1].Files)
	assert.Equal(t, 2, plan.Pieces[1].Order)
}

func TestParseSplitResponse_EmptyPieces(t *testing.T) {
	input := `{"pieces": []}`
	_, err := parseSplitResponse(input, "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pieces")
}

func TestParseSplitResponse_PieceWithNoFiles(t *testing.T) {
	input := `{"pieces": [{"files": [], "commit_message": "empty", "reason": "test", "order": 1}]}`
	_, err := parseSplitResponse(input, "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "piece 0 has no files")
}

func TestParseSplitResponse_InvalidJSON(t *testing.T) {
	_, err := parseSplitResponse("not json", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling split JSON")
}

func TestParseSplitResponse_StripCodeFences(t *testing.T) {
	input := "```json\n" + `{"pieces": [{"files": ["a.go"], "commit_message": "fix", "reason": "r", "order": 1}]}` + "\n```"
	plan, err := parseSplitResponse(input, "def456")
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "def456", plan.OriginalHash)
	require.Len(t, plan.Pieces, 1)
}

func TestParseSplitResponse_AllFilesCovered(t *testing.T) {
	input := `{
		"pieces": [
			{"files": ["a.go", "b.go"], "commit_message": "m1", "reason": "r1", "order": 1},
			{"files": ["c.go"], "commit_message": "m2", "reason": "r2", "order": 2}
		]
	}`

	plan, err := parseSplitResponse(input, "xyz")
	require.NoError(t, err)

	// Collect all files across pieces.
	allFiles := make(map[string]bool)
	for _, p := range plan.Pieces {
		for _, f := range p.Files {
			allFiles[f] = true
		}
	}
	assert.Len(t, allFiles, 3)
	assert.True(t, allFiles["a.go"])
	assert.True(t, allFiles["b.go"])
	assert.True(t, allFiles["c.go"])
}

func TestParseSplitResponse_OrderSequential(t *testing.T) {
	input := `{
		"pieces": [
			{"files": ["a.go"], "commit_message": "m1", "reason": "r1", "order": 1},
			{"files": ["b.go"], "commit_message": "m2", "reason": "r2", "order": 2},
			{"files": ["c.go"], "commit_message": "m3", "reason": "r3", "order": 3}
		]
	}`

	plan, err := parseSplitResponse(input, "xyz")
	require.NoError(t, err)

	for i, p := range plan.Pieces {
		assert.Equal(t, i+1, p.Order, "piece %d should have order %d", i, i+1)
	}
}

// ---------------------------------------------------------------------------
// Suggest with mock provider
// ---------------------------------------------------------------------------

func TestSuggest_Success(t *testing.T) {
	provider := &mockAIProvider{
		name:      "stub",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: `{"pieces": [{"files": ["main.go"], "commit_message": "feat: init", "reason": "entrypoint", "order": 1}]}`,
		},
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	splitter := NewCommitSplitter(registry, builder)
	plan, err := splitter.Suggest(context.Background(), "abc123")
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "abc123", plan.OriginalHash)
	require.Len(t, plan.Pieces, 1)
	assert.Equal(t, "main.go", plan.Pieces[0].Files[0])
}

func TestSuggest_ProviderError(t *testing.T) {
	provider := &mockAIProvider{
		name:        "stub",
		available:   true,
		completeErr: fmt.Errorf("AI down"),
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	splitter := NewCommitSplitter(registry, builder)
	_, err := splitter.Suggest(context.Background(), "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion for split")
}

func TestSuggest_InvalidResponse(t *testing.T) {
	provider := &mockAIProvider{
		name:      "stub",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: `[1, 2, 3]`,
		},
	}
	registry := ai.NewRegistry(config.AIConfig{Provider: "stub"})
	registry.Register("stub", provider)
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	splitter := NewCommitSplitter(registry, builder)
	_, err := splitter.Suggest(context.Background(), "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing split response")
}

func TestSuggest_NoProvider(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{Provider: "missing"})
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	splitter := NewCommitSplitter(registry, builder)
	_, err := splitter.Suggest(context.Background(), "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting AI provider")
}

// ---------------------------------------------------------------------------
// NewCommitSplitter
// ---------------------------------------------------------------------------

func TestNewCommitSplitter(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	splitter := NewCommitSplitter(registry, builder)
	require.NotNil(t, splitter)
	assert.Equal(t, registry, splitter.registry)
	assert.Equal(t, builder, splitter.builder)
}
