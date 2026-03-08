package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// validCategories enumerates the Keep a Changelog categories.
var validCategories = map[string]bool{
	"added":      true,
	"changed":    true,
	"fixed":      true,
	"removed":    true,
	"security":   true,
	"deprecated": true,
}

// ChangelogGenerator produces categorized changelogs from commit ranges.
type ChangelogGenerator struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// ChangelogEntry is a single changelog item.
type ChangelogEntry struct {
	Category     string   `json:"category"`      // "added", "changed", "fixed", "removed", "security", "deprecated"
	Description  string   `json:"description"`   // Human-readable description
	CommitHashes []string `json:"commit_hashes"` // Related commit hashes
}

// NewChangelogGenerator creates a ChangelogGenerator with the given registry
// and context builder.
func NewChangelogGenerator(registry *ai.Registry, builder *ai.Builder) *ChangelogGenerator {
	return &ChangelogGenerator{
		registry: registry,
		builder:  builder,
	}
}

// changelogSystemPrompt is the system instruction for changelog generation.
const changelogSystemPrompt = `You are a changelog generator. Analyze the provided git commits and diffs, then produce a categorized changelog following the Keep a Changelog format.

Rules:
- Use ONLY these categories: "added", "changed", "fixed", "removed", "security", "deprecated".
- Write user-facing descriptions, not raw commit messages. Describe WHAT changed from the user's perspective.
- Group related commits into single entries when they address the same logical change.
- Each entry must reference the commit hashes it relates to.
- Be concise but informative. Avoid internal implementation details.

Respond with a JSON array of objects, each with:
  "category": one of the six categories above
  "description": a clear, user-facing description
  "commit_hashes": array of related short commit hashes

Example:
[
  {"category": "added", "description": "Support for OAuth2 authentication", "commit_hashes": ["abc1234", "def5678"]},
  {"category": "fixed", "description": "Resolved crash when opening empty files", "commit_hashes": ["111aaaa"]}
]

Respond ONLY with the JSON array. No markdown fences, no commentary.`

// Generate creates a changelog from commits between two refs.
func (g *ChangelogGenerator) Generate(ctx context.Context, fromRef, toRef string) ([]ChangelogEntry, error) {
	// Build git context for the commit range.
	gitCtx, err := g.builder.ForChangelog(ctx, fromRef, toRef)
	if err != nil {
		return nil, fmt.Errorf("building changelog context: %w", err)
	}

	// Resolve an available AI provider.
	provider, err := g.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting AI provider: %w", err)
	}

	// Send the completion request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "changelog",
		SystemPrompt: changelogSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   fmt.Sprintf("Generate a changelog for changes from %s to %s.", ai.QuoteUntrusted(fromRef), ai.QuoteUntrusted(toRef)),
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion for changelog: %w", err)
	}

	// Parse the JSON response.
	entries, err := parseChangelogResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parsing changelog response: %w", err)
	}

	return entries, nil
}

// parseChangelogResponse extracts ChangelogEntry values from the AI's JSON
// response. It validates that each category is one of the six allowed values.
func parseChangelogResponse(content string) ([]ChangelogEntry, error) {
	content = strings.TrimSpace(content)
	// Strip markdown code fences if the model included them.
	content = stripCodeFences(content)

	entries := make([]ChangelogEntry, 0)
	if err := json.Unmarshal([]byte(content), &entries); err != nil {
		return nil, fmt.Errorf("unmarshaling changelog JSON: %w", err)
	}

	// Validate categories.
	for i, e := range entries {
		cat := strings.ToLower(strings.TrimSpace(e.Category))
		if !validCategories[cat] {
			return nil, fmt.Errorf("entry %d: invalid category %q", i, e.Category)
		}
		entries[i].Category = cat
	}

	return entries, nil
}
