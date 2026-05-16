package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// validCommitTypes enumerates the allowed conventional commit types.
var validCommitTypes = map[string]bool{
	commitTypeFeat: true,
	commitTypeFix:  true,
	commitTypeDocs: true,
	"style":        true,
	"refactor":     true,
	commitTypeTest: true,
	"chore":        true,
}

// maxSubjectLength is the maximum length for a commit subject line.
const maxSubjectLength = 50

// CommitGenerator generates conventional commit messages using AI.
type CommitGenerator struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// CommitSuggestion holds an AI-generated commit message.
type CommitSuggestion struct {
	Type    string `json:"type"`            // "feat", "fix", "docs", "style", "refactor", "test", "chore"
	Scope   string `json:"scope,omitempty"` // Optional scope like "auth", "api"
	Subject string `json:"subject"`         // Short commit subject line (50 chars max)
	Body    string `json:"body,omitempty"`  // Optional extended description
}

// NewCommitGenerator creates a CommitGenerator with the given registry and
// context builder.
func NewCommitGenerator(registry *ai.Registry, builder *ai.Builder) *CommitGenerator {
	return &CommitGenerator{
		registry: registry,
		builder:  builder,
	}
}

// commitSystemPrompt is the system instruction for commit message generation.
const commitSystemPrompt = `You are a commit message generator. Analyze the staged diff and recent commit history to produce a conventional commit message.

Rules:
- Use ONLY these types: "feat", "fix", "docs", "style", "refactor", "test", "chore".
- Scope is optional. Use a short, lowercase scope when the change is clearly scoped (e.g., "auth", "api", "cli").
- Subject must be imperative mood, lowercase, no trailing period, and no longer than 50 characters.
- Match the style and conventions visible in the recent commit log.
- Body is optional. Use it only when the change needs additional explanation beyond the subject.
- Body should explain WHY, not WHAT (the diff shows what changed).

Respond with a JSON object:
{
  "type": "feat",
  "scope": "auth",
  "subject": "add login endpoint",
  "body": "Optional longer explanation of why this change was made"
}

Respond ONLY with the JSON object. No markdown fences, no commentary.`

// Generate creates a commit message suggestion based on staged changes.
func (g *CommitGenerator) Generate(ctx context.Context) (*CommitSuggestion, error) {
	// Build git context for the staged changes.
	gitCtx, err := g.builder.ForCommit(ctx)
	if err != nil {
		return nil, fmt.Errorf("building commit context: %w", err)
	}

	// Nothing staged — nothing to describe.
	if len(gitCtx.Diffs) == 0 {
		return nil, nil
	}

	// Resolve an available AI provider.
	provider, err := g.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting AI provider: %w", err)
	}

	// Send the completion request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "commit_message",
		SystemPrompt: commitSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   "Generate a conventional commit message for the staged changes.",
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion for commit: %w", err)
	}

	// Parse the JSON response.
	suggestion, err := parseCommitResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parsing commit response: %w", err)
	}

	return suggestion, nil
}

// String formats the suggestion as a conventional commit message
// (e.g. "feat(auth): add login endpoint").
func (s *CommitSuggestion) String() string {
	header := s.Type
	if s.Scope != "" {
		header += "(" + s.Scope + ")"
	}
	header += ": " + s.Subject

	if s.Body == "" {
		return header
	}
	return header + "\n\n" + s.Body
}

// parseCommitResponse extracts a CommitSuggestion from the AI's JSON response.
// It validates the commit type, enforces the subject length limit, and
// normalizes whitespace.
func parseCommitResponse(content string) (*CommitSuggestion, error) {
	content = strings.TrimSpace(content)
	content = stripCodeFences(content)

	var suggestion CommitSuggestion
	if err := json.Unmarshal([]byte(content), &suggestion); err != nil {
		return nil, fmt.Errorf("unmarshaling commit JSON: %w", err)
	}

	// Normalize and validate the type.
	suggestion.Type = strings.ToLower(strings.TrimSpace(suggestion.Type))
	if !validCommitTypes[suggestion.Type] {
		return nil, fmt.Errorf("invalid commit type %q", suggestion.Type)
	}

	// Normalize subject.
	suggestion.Subject = strings.TrimSpace(suggestion.Subject)
	if suggestion.Subject == "" {
		return nil, fmt.Errorf("empty commit subject")
	}

	// Truncate subject if it exceeds the maximum length.
	if len(suggestion.Subject) > maxSubjectLength {
		suggestion.Subject = suggestion.Subject[:maxSubjectLength]
	}

	// Normalize optional fields.
	suggestion.Scope = strings.ToLower(strings.TrimSpace(suggestion.Scope))
	suggestion.Body = strings.TrimSpace(suggestion.Body)

	return &suggestion, nil
}
