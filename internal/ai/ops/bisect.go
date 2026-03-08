package ops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jongio/grut/internal/ai"
)

// BisectAnalyzer provides AI-enhanced bisect analysis by examining the
// commits and diffs between a known-good and known-bad ref.
type BisectAnalyzer struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// BisectAnalysis holds the AI's analysis of commits in a bisect range.
type BisectAnalysis struct {
	Candidates []BisectCandidate `json:"candidates"`
	Summary    string            `json:"summary"`
}

// BisectCandidate is a commit with a probability of being the culprit.
type BisectCandidate struct {
	Hash        string  `json:"hash"`
	Subject     string  `json:"subject"`
	Probability float64 `json:"probability"` // 0.0-1.0
	Reason      string  `json:"reason"`
}

// NewBisectAnalyzer creates a BisectAnalyzer backed by the given registry
// and context builder.
func NewBisectAnalyzer(registry *ai.Registry, builder *ai.Builder) *BisectAnalyzer {
	return &BisectAnalyzer{
		registry: registry,
		builder:  builder,
	}
}

const bisectSystemPrompt = `You are a git bisect analysis assistant. Analyze the commits between the known-good and known-bad refs to identify the most likely culprit commit.

For each commit in the range, assign a probability (0.0-1.0) indicating how likely it is to have introduced the bug. Probabilities across all candidates should reflect relative likelihood but need not sum to 1.0.

Respond with valid JSON matching this schema:
{
  "candidates": [
    {
      "hash": "string",
      "subject": "string",
      "probability": 0.0,
      "reason": "string"
    }
  ],
  "summary": "string"
}

Guidelines:
- Commits touching many files or critical paths get higher probability
- Commits with "fix", "hack", or "workaround" in the message may indicate risky changes
- Pure refactoring or formatting commits get lower probability
- Look at the diff to assess the risk of each change
- Order candidates by probability (highest first)
- Provide a concise summary of your analysis`

// Analyze examines the commits between good and bad refs, returning
// AI-generated probability assessments for each candidate commit.
func (a *BisectAnalyzer) Analyze(ctx context.Context, good, bad string) (*BisectAnalysis, error) {
	gitCtx, err := a.builder.ForBisect(ctx, good, bad)
	if err != nil {
		return nil, fmt.Errorf("building bisect context: %w", err)
	}

	provider, err := a.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving AI provider: %w", err)
	}

	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "bisect_analyze",
		SystemPrompt: bisectSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   fmt.Sprintf("Analyze commits between good ref %s and bad ref %s to identify the most likely culprit.", ai.QuoteUntrusted(good), ai.QuoteUntrusted(bad)),
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion: %w", err)
	}

	var analysis BisectAnalysis
	if err := json.Unmarshal([]byte(resp.Content), &analysis); err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}

	return &analysis, nil
}
