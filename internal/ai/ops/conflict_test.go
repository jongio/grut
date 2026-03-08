package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests — parseConflictResponse
// ---------------------------------------------------------------------------

func TestParseConflictResponse_ValidJSON(t *testing.T) {
	input := `{
		"resolutions": [
			{
				"file": "main.go",
				"regions": [
					{
						"start_line": 5,
						"end_line": 11,
						"resolution": "import (\n\t\"fmt\"\n\t\"os\"\n)",
						"explanation": "Both imports needed",
						"confidence": 0.95
					}
				]
			}
		]
	}`

	resp, err := parseConflictResponse(input)
	require.NoError(t, err)
	require.Len(t, resp.Resolutions, 1)

	assert.Equal(t, "main.go", resp.Resolutions[0].File)
	require.Len(t, resp.Resolutions[0].Regions, 1)

	region := resp.Resolutions[0].Regions[0]
	assert.Equal(t, 5, region.StartLine)
	assert.Equal(t, 11, region.EndLine)
	assert.Contains(t, region.Resolution, "fmt")
	assert.Contains(t, region.Resolution, "os")
	assert.Equal(t, "Both imports needed", region.Explanation)
	assert.InDelta(t, 0.95, region.Confidence, 0.001)
}

func TestParseConflictResponse_WithMarkdownFencing(t *testing.T) {
	input := "```json\n" + `{
		"resolutions": [
			{
				"file": "a.go",
				"regions": [
					{
						"start_line": 1,
						"end_line": 3,
						"resolution": "merged",
						"explanation": "trivial",
						"confidence": 1.0
					}
				]
			}
		]
	}` + "\n```"

	resp, err := parseConflictResponse(input)
	require.NoError(t, err)
	require.Len(t, resp.Resolutions, 1)
	assert.Equal(t, "a.go", resp.Resolutions[0].File)
}

func TestParseConflictResponse_InvalidJSON(t *testing.T) {
	_, err := parseConflictResponse("not json at all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestParseConflictResponse_EmptyResolutions(t *testing.T) {
	_, err := parseConflictResponse(`{"resolutions": []}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no resolutions")
}

func TestParseConflictResponse_MultipleFiles(t *testing.T) {
	input := `{
		"resolutions": [
			{
				"file": "a.go",
				"regions": [{"start_line": 1, "end_line": 5, "resolution": "a", "explanation": "", "confidence": 0.9}]
			},
			{
				"file": "b.go",
				"regions": [{"start_line": 10, "end_line": 15, "resolution": "b", "explanation": "", "confidence": 0.8}]
			}
		]
	}`

	resp, err := parseConflictResponse(input)
	require.NoError(t, err)
	require.Len(t, resp.Resolutions, 2)
	assert.Equal(t, "a.go", resp.Resolutions[0].File)
	assert.Equal(t, "b.go", resp.Resolutions[1].File)
}

// ---------------------------------------------------------------------------
// Tests — stripJSONFencing
// ---------------------------------------------------------------------------

func TestStripJSONFencing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fencing",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "json fencing",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain fencing",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "with whitespace",
			input: "  ```json\n{\"key\": \"value\"}\n```  ",
			want:  `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripCodeFences(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// Tests — reconstructFile
// ---------------------------------------------------------------------------

func TestReconstructFile_SingleConflict(t *testing.T) {
	content := `package main

import "fmt"

<<<<<<< HEAD
func hello() {
	fmt.Println("hello")
}
=======
func hello() {
	fmt.Println("hi there")
}
>>>>>>> feature
`

	regions := []ResolvedRegion{
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 5, EndLine: 14},
			Resolution: `func hello() {
	fmt.Println("hello")
	fmt.Println("hi there")
}`,
		},
	}

	result := reconstructFile(content, regions)

	assert.Contains(t, result, "package main")
	assert.Contains(t, result, `import "fmt"`)
	assert.Contains(t, result, `fmt.Println("hello")`)
	assert.Contains(t, result, `fmt.Println("hi there")`)
	assert.NotContains(t, result, "<<<<<<<")
	assert.NotContains(t, result, "=======")
	assert.NotContains(t, result, ">>>>>>>")
}

func TestReconstructFile_MultipleConflicts(t *testing.T) {
	content := `line1
<<<<<<< HEAD
ours1
=======
theirs1
>>>>>>> branch
line4
<<<<<<< HEAD
ours2
=======
theirs2
>>>>>>> branch
line7`

	regions := []ResolvedRegion{
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 2, EndLine: 6},
			Resolution:     "merged1",
		},
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 8, EndLine: 12},
			Resolution:     "merged2",
		},
	}

	result := reconstructFile(content, regions)

	assert.Equal(t, "line1\nmerged1\nline4\nmerged2\nline7", result)
}

func TestReconstructFile_EmptyContent(t *testing.T) {
	assert.Equal(t, "", reconstructFile("", []ResolvedRegion{{}}))
}

func TestReconstructFile_NoRegions(t *testing.T) {
	assert.Equal(t, "unchanged", reconstructFile("unchanged", nil))
}

func TestReconstructFile_EmptyResolution(t *testing.T) {
	content := "before\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nafter"

	regions := []ResolvedRegion{
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 2, EndLine: 6},
			Resolution:     "",
		},
	}

	result := reconstructFile(content, regions)
	assert.Equal(t, "before\nafter", result)
}

func TestReconstructFile_UnsortedRegions(t *testing.T) {
	content := "a\n<<<<<<< HEAD\nb\n=======\nc\n>>>>>>> x\nd\n<<<<<<< HEAD\ne\n=======\nf\n>>>>>>> x\ng"

	// Provide regions in reverse order — reconstructFile must sort them.
	regions := []ResolvedRegion{
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 8, EndLine: 12},
			Resolution:     "ef",
		},
		{
			OriginalRegion: ai.ConflictRegion{StartLine: 2, EndLine: 6},
			Resolution:     "bc",
		},
	}

	result := reconstructFile(content, regions)
	assert.Equal(t, "a\nbc\nd\nef\ng", result)
}

// ---------------------------------------------------------------------------
// Tests — buildConflictUserPrompt
// ---------------------------------------------------------------------------

func TestBuildConflictUserPrompt(t *testing.T) {
	gitCtx := ai.GitContext{
		CurrentBranch: "main",
		Conflicts: []ai.ConflictFile{
			{
				Path: "file.go",
				ConflictMarkers: []ai.ConflictRegion{
					{StartLine: 5, EndLine: 11, Ours: "ours\n", Theirs: "theirs\n"},
				},
			},
		},
		FileContents: map[string]string{
			"file.go": "content with markers",
		},
	}

	prompt := buildConflictUserPrompt(gitCtx)

	assert.Contains(t, prompt, "Resolve the merge conflicts")
	assert.Contains(t, prompt, `Current branch: "main"`)
	assert.Contains(t, prompt, `=== File: "file.go" ===`)
	assert.Contains(t, prompt, "content with markers")
	assert.Contains(t, prompt, "Ours:")
	assert.Contains(t, prompt, "Theirs:")
	assert.Contains(t, prompt, "lines 5")
}

func TestBuildConflictUserPrompt_WithBase(t *testing.T) {
	gitCtx := ai.GitContext{
		Conflicts: []ai.ConflictFile{
			{
				Path: "f.go",
				ConflictMarkers: []ai.ConflictRegion{
					{StartLine: 1, EndLine: 5, Ours: "a\n", Theirs: "b\n", Base: "c\n"},
				},
			},
		},
		FileContents: map[string]string{},
	}

	prompt := buildConflictUserPrompt(gitCtx)
	assert.Contains(t, prompt, "Base:")
}

// ---------------------------------------------------------------------------
// Tests — Resolve (end-to-end with mock provider)
// ---------------------------------------------------------------------------

func TestResolve_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	conflictContent := `package main

import "fmt"

<<<<<<< HEAD
func greet() { fmt.Println("hello") }
=======
func greet() { fmt.Println("hi") }
>>>>>>> feature

func main() { greet() }
`

	conflictWriteTestFile(t, tmpDir, "main.go", conflictContent)

	aiResponse := aiConflictResponse{
		Resolutions: []aiFileResolution{
			{
				File: "main.go",
				Regions: []aiRegionResolution{
					{
						StartLine:   5,
						EndLine:     9,
						Resolution:  "func greet() { fmt.Println(\"hello\"); fmt.Println(\"hi\") }",
						Explanation: "Both greetings are useful, merged them",
						Confidence:  0.92,
					},
				},
			},
		},
	}

	respJSON, err := json.Marshal(aiResponse)
	require.NoError(t, err)

	mock := &mockAIProvider{
		name:         "test",
		available:    true,
		completeResp: ai.CompletionResponse{Content: string(respJSON)},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "test"})
	registry.Register("test", mock)

	gitClient := newMockGitClient("main", tmpDir)
	builder := ai.NewBuilder(gitClient, nil, 0)

	resolver := NewConflictResolver(registry, builder)
	results, err := resolver.Resolve(ctx, []string{"main.go"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "main.go", results[0].File)
	require.Len(t, results[0].Regions, 1)
	assert.InDelta(t, 0.92, results[0].Regions[0].Confidence, 0.001)
	assert.Contains(t, results[0].Regions[0].Explanation, "merged")

	// Verify FullResolved has no conflict markers.
	assert.NotContains(t, results[0].FullResolved, "<<<<<<<")
	assert.NotContains(t, results[0].FullResolved, "=======")
	assert.NotContains(t, results[0].FullResolved, ">>>>>>>")
	assert.Contains(t, results[0].FullResolved, "package main")
	assert.Contains(t, results[0].FullResolved, "func main()")
}

func TestResolve_ProviderUnavailable(t *testing.T) {
	ctx := context.Background()

	registry := ai.NewRegistry(config.AIConfig{Provider: "none"})
	builder := ai.NewBuilder(newMockGitClient("main", ""), nil, 0)

	resolver := NewConflictResolver(registry, builder)
	_, err := resolver.Resolve(ctx, []string{"some-file.go"})

	// No conflicts found (no repo root → no file content), so nil returned.
	assert.NoError(t, err)
}

func TestResolve_ProviderReturnsError(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	conflictWriteTestFile(t, tmpDir, "f.go", "<<<<<<< HEAD\na\n=======\nb\n>>>>>>> x\n")

	tp := &testProvider{
		name:      "fail",
		available: true,
		err:       errMockProvider,
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "fail"})
	registry.Register("fail", tp)

	builder := ai.NewBuilder(newMockGitClient("main", tmpDir), nil, 0)

	resolver := NewConflictResolver(registry, builder)
	_, err := resolver.Resolve(ctx, []string{"f.go"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI completion failed")
}

func TestResolve_ProviderReturnsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	conflictWriteTestFile(t, tmpDir, "f.go", "<<<<<<< HEAD\na\n=======\nb\n>>>>>>> x\n")

	tp := &testProvider{
		name:      "bad",
		available: true,
		response:  ai.CompletionResponse{Content: "I can't resolve this conflict"},
	}

	registry := ai.NewRegistry(config.AIConfig{Provider: "bad"})
	registry.Register("bad", tp)

	builder := ai.NewBuilder(newMockGitClient("main", tmpDir), nil, 0)

	resolver := NewConflictResolver(registry, builder)
	_, err := resolver.Resolve(ctx, []string{"f.go"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing AI response")
}

func TestResolve_NoConflicts(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	conflictWriteTestFile(t, tmpDir, "clean.go", "package main\n\nfunc main() {}\n")

	registry := ai.NewRegistry(config.AIConfig{Provider: "test"})
	builder := ai.NewBuilder(newMockGitClient("main", tmpDir), nil, 0)

	resolver := NewConflictResolver(registry, builder)
	results, err := resolver.Resolve(ctx, []string{"clean.go"})
	assert.NoError(t, err)
	assert.Nil(t, results)
}

// ---------------------------------------------------------------------------
// Tests — NewConflictResolver
// ---------------------------------------------------------------------------

func TestNewConflictResolver(t *testing.T) {
	registry := ai.NewRegistry(config.AIConfig{})
	builder := ai.NewBuilder(&mockGitClient{}, nil, 0)

	resolver := NewConflictResolver(registry, builder)
	assert.NotNil(t, resolver)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// conflictWriteTestFile creates a file under dir for conflict tests.
func conflictWriteTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}
