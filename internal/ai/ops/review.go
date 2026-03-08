// Package ops implements AI-powered git operations built on top of the ai
// provider and context subsystems.
package ops

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
)

// reviewSystemPrompt instructs the AI to perform a code review and return
// structured JSON findings.
const reviewSystemPrompt = `You are a senior code reviewer. Analyze the provided diff for:
- Security vulnerabilities (e.g. injection, secrets, auth issues)
- Bugs and logic errors
- Performance problems (e.g. N+1 queries, unnecessary allocations)
- Style issues (naming, formatting, idiomatic usage)
- Missing or inadequate tests

Respond ONLY with a JSON array of findings. Each finding is an object with these fields:
- "file": file path (string)
- "line": line number in the new version (integer, 0 if unknown)
- "severity": one of "error", "warning", "info", "hint"
- "category": one of "security", "bug", "performance", "style", "test"
- "message": human-readable description (string)
- "suggestion": suggested fix or empty string (string)

If there are no findings, respond with an empty JSON array: []
Do not include any text outside the JSON array.`

// severityOrder maps severity strings to sort priority (lower = more severe).
var severityOrder = map[string]int{
	"error":   0,
	"warning": 1,
	"info":    2,
	"hint":    3,
}

// Reviewer performs AI-powered code review on diffs.
type Reviewer struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// ReviewFinding is a single code review annotation.
type ReviewFinding struct {
	File       string `json:"file"`                 // File path
	Line       int    `json:"line"`                 // Line number in the new version
	Severity   string `json:"severity"`             // "error", "warning", "info", "hint"
	Category   string `json:"category"`             // "security", "bug", "performance", "style", "test"
	Message    string `json:"message"`              // Human-readable description
	Suggestion string `json:"suggestion,omitempty"` // Suggested fix (optional)
}

// NewReviewer creates a Reviewer backed by the given provider registry and
// context builder.
func NewReviewer(registry *ai.Registry, builder *ai.Builder) *Reviewer {
	return &Reviewer{
		registry: registry,
		builder:  builder,
	}
}

// Review performs an AI code review on the given diff and returns a list of
// findings sorted by severity (error > warning > info > hint).
func (r *Reviewer) Review(ctx context.Context, opts git.DiffOpts) ([]ReviewFinding, error) {
	// Build review context from repository state.
	gitCtx, err := r.builder.ForReview(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("building review context: %w", err)
	}

	// Bail out early when the diff is empty.
	if len(gitCtx.Diffs) == 0 {
		return []ReviewFinding{}, nil
	}

	// Resolve an available provider.
	provider, err := r.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving AI provider: %w", err)
	}

	// Send the review request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "code_review",
		SystemPrompt: reviewSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   "Review the diff and report findings as a JSON array.",
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion: %w", err)
	}

	// Parse findings from the response.
	findings, err := parseFindings(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parsing review findings: %w", err)
	}

	sortFindings(findings)
	return findings, nil
}

// parseFindings extracts a []ReviewFinding from raw AI response text.
// It tolerates responses wrapped in markdown code fences.
func parseFindings(raw string) ([]ReviewFinding, error) {
	text := stripCodeFences(raw)

	findings := make([]ReviewFinding, 0)
	if err := json.Unmarshal([]byte(text), &findings); err != nil {
		return nil, fmt.Errorf("invalid JSON in AI response: %w", err)
	}
	return findings, nil
}

// sortFindings sorts findings by severity (error first, hint last).
// Findings with equal severity preserve their original order.
func sortFindings(findings []ReviewFinding) {
	slices.SortStableFunc(findings, func(a, b ReviewFinding) int {
		return cmp.Compare(severityRank(a.Severity), severityRank(b.Severity))
	})
}

// severityRank returns the sort priority for a severity string.
// Unknown severities sort after "hint".
func severityRank(s string) int {
	if rank, ok := severityOrder[s]; ok {
		return rank
	}
	return len(severityOrder) // unknown → sort last
}
