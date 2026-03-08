package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// CommitSplitter suggests how to split large commits into smaller,
// logically cohesive pieces.
type CommitSplitter struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// SplitPlan is the AI's recommendation for splitting a commit.
type SplitPlan struct {
	OriginalHash string       `json:"original_hash"`
	Pieces       []SplitPiece `json:"pieces"`
}

// SplitPiece is one logical grouping of changes.
type SplitPiece struct {
	Files         []string `json:"files"`          // Files in this piece
	CommitMessage string   `json:"commit_message"` // Suggested commit message
	Reason        string   `json:"reason"`         // Why these files are grouped
	Order         int      `json:"order"`          // Suggested commit order
}

// NewCommitSplitter creates a CommitSplitter with the given registry and
// context builder.
func NewCommitSplitter(registry *ai.Registry, builder *ai.Builder) *CommitSplitter {
	return &CommitSplitter{
		registry: registry,
		builder:  builder,
	}
}

// splitSystemPrompt is the system instruction for commit splitting.
const splitSystemPrompt = `You are a commit splitting advisor. Analyze the provided commit diff and suggest how to split it into smaller, logically cohesive commits.

Rules:
- Group files that belong to the same logical change together.
- Each piece should be independently buildable/testable when possible.
- Order pieces so that dependencies come before dependents (e.g., types before implementations, migrations before code).
- Write conventional commit messages for each piece.
- Every file in the original commit must appear in exactly one piece.
- Explain why the files in each piece belong together.

Respond with a JSON object:
{
  "pieces": [
    {
      "files": ["path/to/file1.go", "path/to/file2.go"],
      "commit_message": "feat: add user authentication types",
      "reason": "These files define the shared types used by the auth subsystem",
      "order": 1
    }
  ]
}

Respond ONLY with the JSON object. No markdown fences, no commentary.`

// Suggest analyzes a commit and suggests how to split it into smaller,
// logically cohesive commits.
func (s *CommitSplitter) Suggest(ctx context.Context, commitHash string) (*SplitPlan, error) {
	// Build git context for the commit.
	gitCtx, err := s.builder.ForSplit(ctx, commitHash)
	if err != nil {
		return nil, fmt.Errorf("building split context: %w", err)
	}

	// Resolve an available AI provider.
	provider, err := s.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting AI provider: %w", err)
	}

	// Send the completion request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "split",
		SystemPrompt: splitSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   fmt.Sprintf("Suggest how to split commit %s into smaller commits.", ai.QuoteUntrusted(commitHash)),
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion for split: %w", err)
	}

	// Parse the JSON response.
	plan, err := parseSplitResponse(resp.Content, commitHash)
	if err != nil {
		return nil, fmt.Errorf("parsing split response: %w", err)
	}

	return plan, nil
}

// parseSplitResponse extracts a SplitPlan from the AI's JSON response.
func parseSplitResponse(content, commitHash string) (*SplitPlan, error) {
	content = strings.TrimSpace(content)
	content = stripCodeFences(content)

	// The AI returns an object with a "pieces" key.
	var raw struct {
		Pieces []SplitPiece `json:"pieces"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling split JSON: %w", err)
	}

	if len(raw.Pieces) == 0 {
		return nil, fmt.Errorf("split plan contains no pieces")
	}

	// Validate that each piece has at least one file.
	for i, p := range raw.Pieces {
		if len(p.Files) == 0 {
			return nil, fmt.Errorf("piece %d has no files", i)
		}
	}

	return &SplitPlan{
		OriginalHash: commitHash,
		Pieces:       raw.Pieces,
	}, nil
}
