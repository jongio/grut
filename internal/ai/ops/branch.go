package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
)

// BranchAnalyzer recommends branch cleanup actions by examining all local
// branches and their tracking state.
type BranchAnalyzer struct {
	registry *ai.Registry
	builder  *ai.Builder
	client   git.GitClient
}

// BranchRecommendation is the AI's suggestion for a single branch.
type BranchRecommendation struct {
	Name    string `json:"name"`               // Branch name
	Action  string `json:"action"`             // "keep", "delete", "archive", "rename"
	Reason  string `json:"reason"`             // Why this action was suggested
	NewName string `json:"new_name,omitempty"` // If action is "rename"
}

// branchResponse wraps the JSON array the AI returns.
type branchResponse struct {
	Branches []BranchRecommendation `json:"branches"`
}

// NewBranchAnalyzer creates a BranchAnalyzer backed by the given registry,
// context builder, and git client (used to list branches).
func NewBranchAnalyzer(registry *ai.Registry, builder *ai.Builder, client git.GitClient) *BranchAnalyzer {
	return &BranchAnalyzer{
		registry: registry,
		builder:  builder,
		client:   client,
	}
}

const branchSystemPrompt = `You are a git branch cleanup assistant. Analyze the list of branches and recommend an action for each.

For each branch, recommend one of:
- "keep": the branch is active or important
- "delete": the branch is stale or already merged
- "archive": the branch has historical value but is inactive
- "rename": the branch name doesn't follow conventions

Respond with valid JSON matching this schema:
{
  "branches": [
    {
      "name": "string",
      "action": "keep|delete|archive|rename",
      "reason": "string",
      "new_name": "string (only if action is rename)"
    }
  ]
}

Guidelines:
- The current branch should always be "keep"
- Branches with upstream tracking and no divergence that are merged are candidates for "delete"
- Feature branches with no recent activity are candidates for "archive" or "delete"
- Branch names should follow kebab-case with type prefixes (feature/, fix/, etc.)
- Protected branches (main, master, develop) should always be "keep"`

// Analyze lists all local branches, sends them to the AI provider for
// analysis, and returns a recommendation for each.
func (a *BranchAnalyzer) Analyze(ctx context.Context) ([]BranchRecommendation, error) {
	// Build lightweight context (branch + status only).
	gitCtx, err := a.builder.ForChat(ctx)
	if err != nil {
		return nil, fmt.Errorf("building branch context: %w", err)
	}

	// Fetch the full branch list to include in the user prompt.
	branches, err := a.client.BranchList(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing branches: %w", err)
	}

	if len(branches) == 0 {
		return nil, nil
	}

	provider, err := a.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving AI provider: %w", err)
	}

	userPrompt := formatBranchList(branches)

	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "branch_analyze",
		SystemPrompt: branchSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion: %w", err)
	}

	var parsed branchResponse
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}

	return parsed.Branches, nil
}

// formatBranchList renders a human-readable branch summary for the AI prompt.
func formatBranchList(branches []git.Branch) string {
	var sb strings.Builder
	sb.WriteString("Analyze these branches and recommend an action for each:\n\n")
	for _, br := range branches {
		marker := "  "
		if br.IsCurrent {
			marker = "* "
		}
		_, _ = fmt.Fprintf(&sb, "%s%s", marker, ai.QuoteUntrusted(ai.SanitizeBranchName(br.Name)))
		if br.Upstream != "" {
			_, _ = fmt.Fprintf(&sb, " (tracking: %s", ai.QuoteUntrusted(ai.SanitizeBranchName(br.Upstream)))
			if br.Ahead > 0 || br.Behind > 0 {
				_, _ = fmt.Fprintf(&sb, ", ahead %d, behind %d", br.Ahead, br.Behind)
			}
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
