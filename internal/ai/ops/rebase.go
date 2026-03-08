package ops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jongio/grut/internal/ai"
)

// RebaseAssistant provides AI-powered rebase suggestions by analyzing commits
// on the current branch relative to the target ("onto") ref.
type RebaseAssistant struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// RebaseSuggestion holds the AI's recommendations for a rebase.
type RebaseSuggestion struct {
	Commits []CommitAction `json:"commits"`
}

// CommitAction is the AI's recommendation for a single commit during rebase.
type CommitAction struct {
	Hash       string `json:"hash"`                  // Commit hash
	Subject    string `json:"subject"`               // Original subject
	Action     string `json:"action"`                // "pick", "squash", "fixup", "reword", "drop"
	Reason     string `json:"reason"`                // Why this action was suggested
	NewSubject string `json:"new_subject,omitempty"` // If action is "reword", the suggested new subject
}

// NewRebaseAssistant creates a RebaseAssistant backed by the given registry
// and context builder.
func NewRebaseAssistant(registry *ai.Registry, builder *ai.Builder) *RebaseAssistant {
	return &RebaseAssistant{
		registry: registry,
		builder:  builder,
	}
}

const rebaseSystemPrompt = `You are a git rebase assistant. Analyze the commits being rebased and suggest actions for each.

For each commit, recommend one of:
- "pick": keep the commit as-is
- "squash": combine with the previous commit
- "fixup": combine with the previous commit, discarding this commit's message
- "reword": keep the commit but suggest a better commit message
- "drop": remove the commit entirely

Respond with valid JSON matching this schema:
{
  "commits": [
    {
      "hash": "string",
      "subject": "string",
      "action": "pick|squash|fixup|reword|drop",
      "reason": "string",
      "new_subject": "string (only if action is reword)"
    }
  ]
}

Guidelines:
- Group related fixup commits with their parent
- Squash WIP commits into meaningful ones
- Suggest reword for unclear or non-conventional commit messages
- Preserve logical commit boundaries
- Drop commits that are entirely superseded`

// Suggest analyzes the commits between the "onto" ref and HEAD, returning
// AI-generated recommendations for each commit's rebase action.
func (a *RebaseAssistant) Suggest(ctx context.Context, onto string) (*RebaseSuggestion, error) {
	gitCtx, err := a.builder.ForRebase(ctx, onto)
	if err != nil {
		return nil, fmt.Errorf("building rebase context: %w", err)
	}

	provider, err := a.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving AI provider: %w", err)
	}

	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "rebase_suggest",
		SystemPrompt: rebaseSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   fmt.Sprintf("Analyze these commits being rebased onto %s and suggest an action for each.", ai.QuoteUntrusted(onto)),
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion: %w", err)
	}

	var suggestion RebaseSuggestion
	if err := json.Unmarshal([]byte(resp.Content), &suggestion); err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}

	return &suggestion, nil
}
