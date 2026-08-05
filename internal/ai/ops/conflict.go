// Package ops implements AI-powered git operations that compose the core
// provider and context types from the ai package.
package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jongio/grut/internal/ai"
)

// conflictSystemPrompt instructs the AI model on how to resolve merge
// conflicts and what response format to produce.
const conflictSystemPrompt = `You are an expert at resolving Git merge conflicts. Analyze each conflict region considering both branches' intent.

Rules:
1. Prefer merging both changes when possible over choosing one side.
2. Preserve the intent and functionality of both branches.
3. When changes are independent and non-overlapping, include both.
4. When changes are truly incompatible, choose the most complete version and explain why.
5. Return valid JSON only — no markdown fencing, no commentary outside the JSON.

Respond with this exact JSON structure:
{
  "resolutions": [
    {
      "file": "path/to/file",
      "regions": [
        {
          "start_line": 10,
          "end_line": 20,
          "resolution": "the resolved content for this region",
          "explanation": "why this resolution was chosen",
          "confidence": 0.95
        }
      ]
    }
  ]
}`

// ConflictResolver uses AI to resolve merge conflicts.
type ConflictResolver struct {
	registry *ai.Registry
	builder  *ai.Builder
}

// ConflictResolution holds the AI's resolution for a conflicted file.
type ConflictResolution struct {
	// File is the repository-relative path of the conflicted file.
	File string
	// FullResolved is the complete file content with all conflict markers
	// replaced by the AI's resolutions.
	FullResolved string
	// Regions contains the AI's resolution for each conflict region.
	Regions []ResolvedRegion
}

// ResolvedRegion contains the AI's resolution for a single conflict region.
type ResolvedRegion struct {
	// OriginalRegion is the conflict region from the original file.
	OriginalRegion ai.ConflictRegion
	// Resolution is the resolved content for this region.
	Resolution string
	// Explanation describes why the AI chose this resolution.
	Explanation string
	// Confidence is a score from 0.0 to 1.0 indicating the AI's certainty.
	Confidence float64
}

// NewConflictResolver creates a resolver that uses the given registry to
// obtain an AI provider and the builder to construct conflict context.
func NewConflictResolver(registry *ai.Registry, builder *ai.Builder) *ConflictResolver {
	return &ConflictResolver{
		registry: registry,
		builder:  builder,
	}
}

// Resolve resolves conflicts in the given files using AI.
// Returns one ConflictResolution per file that has conflicts.
func (r *ConflictResolver) Resolve(ctx context.Context, files []string) ([]ConflictResolution, error) {
	// Build git context with conflict information.
	gitCtx, err := r.builder.ForConflict(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("building conflict context: %w", err)
	}
	if len(gitCtx.Conflicts) == 0 {
		return nil, nil
	}
	// Obtain an AI provider.
	provider, err := r.registry.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting AI provider: %w", err)
	}
	// Build the user prompt from conflict details.
	userPrompt := buildConflictUserPrompt(gitCtx)
	// Send the completion request.
	resp, err := provider.Complete(ctx, ai.CompletionRequest{
		Operation:    "conflict_resolve",
		SystemPrompt: conflictSystemPrompt,
		GitContext:   gitCtx,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion failed: %w", err)
	}
	// Parse the AI response.
	aiResolutions, err := parseConflictResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}
	// Map AI resolutions to our output types and reconstruct full files.
	return buildResolutions(gitCtx, aiResolutions)
}

// ---------------------------------------------------------------------------
// AI response types (match the JSON schema in conflictSystemPrompt)
// ---------------------------------------------------------------------------
// aiConflictResponse is the top-level JSON structure returned by the AI.
type aiConflictResponse struct {
	Resolutions []aiFileResolution `json:"resolutions"`
}

// aiFileResolution is the AI's resolution for a single file.
type aiFileResolution struct {
	File    string               `json:"file"`
	Regions []aiRegionResolution `json:"regions"`
}

// aiRegionResolution is the AI's resolution for a single conflict region.
type aiRegionResolution struct {
	Resolution  string  `json:"resolution"`
	Explanation string  `json:"explanation"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Confidence  float64 `json:"confidence"`
}

// ---------------------------------------------------------------------------
// Prompt construction
// ---------------------------------------------------------------------------
// buildConflictUserPrompt formats the conflict details into a human-readable
// prompt for the AI model.
func buildConflictUserPrompt(gitCtx ai.GitContext) string {
	var sb strings.Builder
	sb.WriteString("Resolve the merge conflicts in the following files.\n\n")
	if gitCtx.CurrentBranch != "" {
		sb.WriteString("Current branch: ")
		sb.WriteString(ai.QuoteUntrusted(ai.SanitizeBranchName(gitCtx.CurrentBranch)))
		sb.WriteString("\n\n")
	}
	for _, cf := range gitCtx.Conflicts {
		sb.WriteString("=== File: ")
		sb.WriteString(ai.QuoteUntrusted(ai.SanitizeFilePath(cf.Path)))
		sb.WriteString(" ===\n")
		// Include full file content if available.
		if content, ok := gitCtx.FileContents[cf.Path]; ok {
			sb.WriteString("Full content (with conflict markers):\n")
			sb.WriteString(ai.SanitizeExternalContent(content))
			sb.WriteString("\n\n")
		}
		// List individual conflict regions.
		for i, region := range cf.ConflictMarkers {
			fmt.Fprintf(&sb, "Conflict region %d (lines %d–%d):\n",
				i+1, region.StartLine, region.EndLine)
			// Region text is attacker-controllable (it comes from the branches
			// being merged), so it gets the same boundary-marker treatment as
			// the full file content above. Without this it was the one path
			// where raw repository text reached the prompt unwrapped.
			var regionText strings.Builder
			writeConflictSide(&regionText, "Ours", region.Ours)
			writeConflictSide(&regionText, "Theirs", region.Theirs)
			if region.Base != "" {
				writeConflictSide(&regionText, "Base", region.Base)
			}
			sb.WriteString(ai.SanitizeExternalContent(regionText.String()))
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// writeConflictSide writes one labelled side of a conflict region, indenting
// each line so the model can tell the sides apart.
func writeConflictSide(sb *strings.Builder, label, text string) {
	sb.WriteString("  ")
	sb.WriteString(label)
	sb.WriteString(":\n")
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		sb.WriteString("    ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------
// parseConflictResponse parses the AI model's JSON response into structured
// resolution data.
func parseConflictResponse(content string) (*aiConflictResponse, error) {
	// Strip optional markdown fencing the model might add despite instructions.
	content = stripCodeFences(content)
	var resp aiConflictResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON in AI response: %w", err)
	}
	if len(resp.Resolutions) == 0 {
		return nil, fmt.Errorf("AI response contains no resolutions")
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Resolution assembly
// ---------------------------------------------------------------------------
// buildResolutions matches AI resolutions to conflict files and reconstructs
// the full resolved content for each file.
func buildResolutions(gitCtx ai.GitContext, aiResp *aiConflictResponse) ([]ConflictResolution, error) {
	// Index AI resolutions by file path.
	aiByFile := make(map[string]*aiFileResolution, len(aiResp.Resolutions))
	for i := range aiResp.Resolutions {
		aiByFile[aiResp.Resolutions[i].File] = &aiResp.Resolutions[i]
	}
	results := make([]ConflictResolution, 0)
	for _, cf := range gitCtx.Conflicts {
		aiFile, ok := aiByFile[cf.Path]
		if !ok {
			continue // AI didn't resolve this file — skip.
		}
		// Index AI regions by (start_line, end_line) for matching.
		aiRegions := make(map[[2]int]*aiRegionResolution, len(aiFile.Regions))
		for i := range aiFile.Regions {
			key := [2]int{aiFile.Regions[i].StartLine, aiFile.Regions[i].EndLine}
			aiRegions[key] = &aiFile.Regions[i]
		}
		// Build resolved regions by matching against original conflict markers.
		resolved := make([]ResolvedRegion, 0)
		for _, original := range cf.ConflictMarkers {
			key := [2]int{original.StartLine, original.EndLine}
			aiRegion, ok := aiRegions[key]
			if !ok {
				continue
			}
			resolved = append(resolved, ResolvedRegion{
				OriginalRegion: original,
				Resolution:     aiRegion.Resolution,
				Explanation:    aiRegion.Explanation,
				Confidence:     aiRegion.Confidence,
			})
		}
		// Reconstruct full file content with conflicts resolved.
		fileContent := gitCtx.FileContents[cf.Path]
		fullResolved := reconstructFile(fileContent, resolved)
		results = append(results, ConflictResolution{
			File:         cf.Path,
			Regions:      resolved,
			FullResolved: fullResolved,
		})
	}
	return results, nil
}

// reconstructFile replaces conflict markers in the original file content
// with the AI's resolutions. Regions must correspond to conflict marker
// boundaries (StartLine = <<<<<<< line, EndLine = >>>>>>> line).
func reconstructFile(content string, regions []ResolvedRegion) string {
	if content == "" || len(regions) == 0 {
		return content
	}
	// Sort regions by start line to process in order.
	sorted := make([]ResolvedRegion, len(regions))
	copy(sorted, regions)
	slices.SortFunc(sorted, func(a, b ResolvedRegion) int {
		return a.OriginalRegion.StartLine - b.OriginalRegion.StartLine
	})
	lines := strings.Split(content, "\n")
	var result []string
	ri := 0 // index into sorted regions
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1 // 1-based
		if ri < len(sorted) && lineNo == sorted[ri].OriginalRegion.StartLine {
			// Insert resolution lines in place of the conflict block.
			resText := strings.TrimRight(sorted[ri].Resolution, "\n")
			if resText != "" {
				result = append(result, strings.Split(resText, "\n")...)
			}
			// Skip all original conflict lines through EndLine.
			i = sorted[ri].OriginalRegion.EndLine - 1 // -1 because loop increments
			ri++
		} else {
			result = append(result, lines[i])
		}
	}
	return strings.Join(result, "\n")
}
