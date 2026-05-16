package ops

import (
	"context"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestReviewer wires up a Reviewer with the given mock provider and diffs.
func newTestReviewer(mock *mockAIProvider, diffs []git.FileDiff) *Reviewer {
	reg := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	reg.Register("mock", mock)

	client := newMockGitClient("main", "/repo")
	if diffs != nil {
		client.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
			return diffs, nil
		}
	}
	builder := ai.NewBuilder(client, nil, 0)
	return NewReviewer(reg, builder)
}

// sampleDiffs returns a minimal diff for testing.
func sampleDiffs() []git.FileDiff {
	return []git.FileDiff{
		{
			Path: "main.go",
			Hunks: []git.Hunk{
				{
					OldStart: 1, OldLines: 1,
					NewStart: 1, NewLines: 1,
					Lines: []git.DiffLine{
						{Type: git.DiffLineAdded, Content: "fmt.Println(\"hello\")", NewLine: 1},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Tests — parseFindings
// ---------------------------------------------------------------------------

func TestParseFindingsValidJSON(t *testing.T) {
	raw := `[
		{"file":"main.go","line":10,"severity":"warning","category":"style","message":"unused import","suggestion":"remove it"}
	]`
	findings, err := parseFindings(raw)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "main.go", findings[0].File)
	assert.Equal(t, 10, findings[0].Line)
	assert.Equal(t, severityWarning, findings[0].Severity)
	assert.Equal(t, "style", findings[0].Category)
	assert.Equal(t, "unused import", findings[0].Message)
	assert.Equal(t, "remove it", findings[0].Suggestion)
}

func TestParseFindingsEmptyArray(t *testing.T) {
	findings, err := parseFindings("[]")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestParseFindingsCodeFences(t *testing.T) {
	raw := "```json\n" + `[{"file":"a.go","line":1,"severity":"info","category":"bug","message":"possible nil","suggestion":""}]` + "\n```"
	findings, err := parseFindings(raw)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "a.go", findings[0].File)
}

func TestParseFindingsCodeFencesNoLanguageTag(t *testing.T) {
	raw := "```\n[]\n```"
	findings, err := parseFindings(raw)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestParseFindingsInvalidJSON(t *testing.T) {
	_, err := parseFindings("this is not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestParseFindingsWhitespace(t *testing.T) {
	findings, err := parseFindings("  \n  []  \n  ")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// ---------------------------------------------------------------------------
// Tests — sortFindings
// ---------------------------------------------------------------------------

func TestSortFindingsBySeverity(t *testing.T) {
	findings := []ReviewFinding{
		{Severity: severityHint, Message: "hint-1"},
		{Severity: severityError, Message: "error-1"},
		{Severity: severityInfo, Message: "info-1"},
		{Severity: severityWarning, Message: "warning-1"},
		{Severity: severityError, Message: "error-2"},
	}
	sortFindings(findings)

	expected := []string{severityError, severityError, severityWarning, severityInfo, severityHint}
	for i, f := range findings {
		assert.Equal(t, expected[i], f.Severity, "index %d", i)
	}
}

func TestSortFindingsStableOrder(t *testing.T) {
	findings := []ReviewFinding{
		{Severity: severityError, Message: "first"},
		{Severity: severityError, Message: "second"},
		{Severity: severityError, Message: "third"},
	}
	sortFindings(findings)

	assert.Equal(t, "first", findings[0].Message)
	assert.Equal(t, "second", findings[1].Message)
	assert.Equal(t, "third", findings[2].Message)
}

func TestSortFindingsUnknownSeverity(t *testing.T) {
	findings := []ReviewFinding{
		{Severity: "unknown", Message: "unknown"},
		{Severity: severityError, Message: "error"},
		{Severity: severityHint, Message: "hint"},
	}
	sortFindings(findings)

	assert.Equal(t, severityError, findings[0].Severity)
	assert.Equal(t, severityHint, findings[1].Severity)
	assert.Equal(t, "unknown", findings[2].Severity)
}

func TestSortFindingsEmpty(t *testing.T) {
	var findings []ReviewFinding
	sortFindings(findings) // should not panic
	assert.Empty(t, findings)
}

// ---------------------------------------------------------------------------
// Tests — Review (integration with mocks)
// ---------------------------------------------------------------------------

func TestReviewReturnsFindings(t *testing.T) {
	jsonResp := `[
		{"file":"main.go","line":5,"severity":"error","category":"security","message":"SQL injection","suggestion":"use parameterized query"},
		{"file":"main.go","line":12,"severity":"info","category":"style","message":"naming convention","suggestion":""}
	]`
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: jsonResp},
	}
	rev := newTestReviewer(mock, sampleDiffs())

	findings, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	require.Len(t, findings, 2)

	// Verify sorted: error before info.
	assert.Equal(t, severityError, findings[0].Severity)
	assert.Equal(t, severityInfo, findings[1].Severity)
}

func TestReviewEmptyDiff(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: true,
	}
	rev := newTestReviewer(mock, nil) // no diffs — client returns nil

	findings, err := rev.Review(context.Background(), git.DiffOpts{})
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestReviewProviderError(t *testing.T) {
	mock := &mockAIProvider{
		name:        "mock",
		available:   true,
		completeErr: errors.New("model overloaded"),
	}
	rev := newTestReviewer(mock, sampleDiffs())

	_, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion")
	assert.Contains(t, err.Error(), "model overloaded")
}

func TestReviewNoProviderAvailable(t *testing.T) {
	mock := &mockAIProvider{
		name:      "mock",
		available: false,
	}
	rev := newTestReviewer(mock, sampleDiffs())

	_, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AI provider")
}

func TestReviewMalformedResponse(t *testing.T) {
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: "not json at all"},
	}
	rev := newTestReviewer(mock, sampleDiffs())

	_, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing review findings")
}

func TestReviewNoFindingsEmptyArray(t *testing.T) {
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: "[]"},
	}
	rev := newTestReviewer(mock, sampleDiffs())

	findings, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestReviewResponseWithCodeFences(t *testing.T) {
	wrapped := "```json\n" + `[{"file":"x.go","line":1,"severity":"warning","category":"bug","message":"off-by-one","suggestion":"use <"}]` + "\n```"
	mock := &mockAIProvider{
		name:         "mock",
		available:    true,
		completeResp: ai.CompletionResponse{Content: wrapped},
	}
	rev := newTestReviewer(mock, sampleDiffs())

	findings, err := rev.Review(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "off-by-one", findings[0].Message)
}

// ---------------------------------------------------------------------------
// Tests — NewReviewer
// ---------------------------------------------------------------------------

func TestNewReviewer(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	client := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(client, nil, 0)

	rev := NewReviewer(registry, builder)
	assert.NotNil(t, rev)
}

// ---------------------------------------------------------------------------
// Tests — severityRank
// ---------------------------------------------------------------------------

func TestSeverityRankKnown(t *testing.T) {
	assert.Equal(t, 0, severityRank(severityError))
	assert.Equal(t, 1, severityRank(severityWarning))
	assert.Equal(t, 2, severityRank(severityInfo))
	assert.Equal(t, 3, severityRank(severityHint))
}

func TestSeverityRankUnknown(t *testing.T) {
	rank := severityRank("critical")
	assert.Greater(t, rank, severityRank(severityHint), "unknown severity should sort after hint")
}
