package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// prSystemPrompt instructs the AI to analyze the branch diff and commits,
// then produce a structured PR description as JSON.
const prSystemPrompt = `You are a senior engineer writing a pull request description. Analyze the provided diff and commit history to generate a clear, informative PR description.

Respond ONLY with valid JSON matching this schema:
{
  "title": "string — imperative-mood PR title (e.g. 'Add user authentication')",
  "summary": "string — 1-3 sentence overview of what this PR does and why",
  "changes": ["string — one bullet per logical change"],
  "testing_notes": "string — how a reviewer can verify the changes (empty string if obvious)",
  "breaking_changes": ["string — one bullet per breaking change, empty array if none"]
}

Guidelines:
- Title should be concise and use imperative mood (e.g. "Add", "Fix", "Refactor")
- Summary should explain the motivation, not just repeat the title
- Changes should be grouped logically, not listed per-file
- Testing notes should be actionable instructions for a reviewer
- Only list breaking changes if there genuinely are any
- Do not include any text outside the JSON object.`

// PRDescriptionGenerator creates AI-powered PR descriptions.
type PRDescriptionGenerator struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// PRDescription holds the AI-generated PR metadata.
type PRDescription struct {
	Title           string   `json:"title"`                      // PR title
	Summary         string   `json:"summary"`                    // Brief summary
	Changes         []string `json:"changes"`                    // List of changes made
	TestingNotes    string   `json:"testing_notes,omitempty"`    // How to test
	BreakingChanges []string `json:"breaking_changes,omitempty"` // Breaking changes, if any
	FullMarkdown    string   `json:"-"`                          // Complete PR description in markdown
}

// NewPRDescriptionGenerator creates a PRDescriptionGenerator backed by the
// given provider registry and context builder.
func NewPRDescriptionGenerator(registry *ai.Registry, builder *ai.Builder) *PRDescriptionGenerator {
	return &PRDescriptionGenerator{
		registry: registry,
		builder:  builder,
	}
}

// Generate creates a PR description for the current branch vs target.
func (g *PRDescriptionGenerator) Generate(ctx context.Context, targetBranch string) (*PRDescription, error) {
	// Build PR context from repository state.
	gitCtx, err := g.builder.ForPR(ctx, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("building PR context: %w", err)
	}

	// Resolve an available provider.
	provider, err := g.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving AI provider: %w", err)
	}

	// Send the PR description request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "pr_description",
		SystemPrompt: prSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   fmt.Sprintf("Generate a PR description for merging into %s.", ai.QuoteUntrusted(targetBranch)),
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion: %w", err)
	}

	// Parse the structured response.
	desc, err := parsePRResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}

	// Assemble the full markdown from structured fields.
	desc.FullMarkdown = buildPRMarkdown(desc)

	return desc, nil
}

// parsePRResponse extracts a PRDescription from raw AI response text.
// It tolerates responses wrapped in markdown code fences.
func parsePRResponse(raw string) (*PRDescription, error) {
	text := strings.TrimSpace(raw)

	// Strip optional markdown code fences that models sometimes add.
	if strings.HasPrefix(text, "```") {
		// Remove opening fence (with optional language tag).
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		}
		// Remove closing fence.
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	var desc PRDescription
	if err := json.Unmarshal([]byte(text), &desc); err != nil {
		return nil, fmt.Errorf("invalid JSON in AI response: %w", err)
	}
	return &desc, nil
}

// buildPRMarkdown assembles a complete markdown PR description from the
// structured fields.
func buildPRMarkdown(desc *PRDescription) string {
	var sb strings.Builder

	// Summary section.
	sb.WriteString("## Summary\n\n")
	sb.WriteString(desc.Summary)
	sb.WriteString("\n")

	// Changes section.
	if len(desc.Changes) > 0 {
		sb.WriteString("\n## Changes\n\n")
		for _, change := range desc.Changes {
			sb.WriteString("- ")
			sb.WriteString(change)
			sb.WriteString("\n")
		}
	}

	// Testing notes section.
	if desc.TestingNotes != "" {
		sb.WriteString("\n## Testing\n\n")
		sb.WriteString(desc.TestingNotes)
		sb.WriteString("\n")
	}

	// Breaking changes section.
	if len(desc.BreakingChanges) > 0 {
		sb.WriteString("\n## Breaking Changes\n\n")
		for _, bc := range desc.BreakingChanges {
			sb.WriteString("- ")
			sb.WriteString(bc)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
