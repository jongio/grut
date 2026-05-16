package ai

import (
	"context"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewCopilotProvider
// ---------------------------------------------------------------------------

func TestNewCopilotProvider_DefaultOptions(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	p, err := NewCopilotProvider("gpt-4o")
	require.NoError(t, err)
	assert.NotNil(t, p.client)
	assert.Equal(t, "gpt-4o", p.model)
}

func TestNewCopilotProvider_EmptyModel(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	p, err := NewCopilotProvider("")
	require.NoError(t, err)
	assert.Equal(t, "", p.model)
}

func TestNewCopilotProvider_WithGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token_abc")

	p, err := NewCopilotProvider("gpt-4o")
	require.NoError(t, err)
	assert.NotNil(t, p.client)
	// The token is passed to the SDK client options, not stored on the
	// provider struct. We verify the provider is created successfully.
}

// ---------------------------------------------------------------------------
// Provider metadata
// ---------------------------------------------------------------------------

func TestCopilotProvider_Name(t *testing.T) {
	p := &CopilotProvider{}
	assert.Equal(t, providerCopilot, p.Name())
}

func TestCopilotProvider_Close_NotStarted(t *testing.T) {
	p := &CopilotProvider{
		client: copilot.NewClient(nil),
		// started defaults to false — Close() should be a no-op.
	}
	assert.NoError(t, p.Close())
}

// ---------------------------------------------------------------------------
// buildSessionConfig
// ---------------------------------------------------------------------------

func TestBuildSessionConfig_DefaultModel(t *testing.T) {
	p := &CopilotProvider{model: "gpt-4o"}
	cfg := p.buildSessionConfig(CompletionRequest{})

	assert.Equal(t, "gpt-4o", cfg.Model)
	assert.NotNil(t, cfg.OnPermissionRequest)
	assert.Nil(t, cfg.SystemMessage)
}

func TestBuildSessionConfig_EmptyModel(t *testing.T) {
	p := &CopilotProvider{model: ""}
	cfg := p.buildSessionConfig(CompletionRequest{})

	assert.Equal(t, "", cfg.Model)
}

func TestBuildSessionConfig_WithSystemPrompt(t *testing.T) {
	p := &CopilotProvider{model: "gpt-4o"}
	cfg := p.buildSessionConfig(CompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
	})

	require.NotNil(t, cfg.SystemMessage)
	assert.Equal(t, "append", cfg.SystemMessage.Mode)
	assert.Equal(t, "You are a helpful assistant.", cfg.SystemMessage.Content)
}

func TestBuildSessionConfig_NoSystemPrompt(t *testing.T) {
	p := &CopilotProvider{model: "gpt-4o"}
	cfg := p.buildSessionConfig(CompletionRequest{
		UserPrompt: "Hello",
	})

	assert.Nil(t, cfg.SystemMessage)
}

// ---------------------------------------------------------------------------
// buildPrompt
// ---------------------------------------------------------------------------

func TestBuildPrompt_Empty(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{})
	assert.Equal(t, "", prompt)
}

func TestBuildPrompt_UserPromptOnly(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		UserPrompt: "Generate a commit message",
	})
	assert.Equal(t, "Generate a commit message", prompt)
}

func TestBuildPrompt_GitContext(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		GitContext: GitContext{
			RepoRoot:      "/home/user/project",
			CurrentBranch: "feature/auth",
		},
		UserPrompt: "Generate a commit message.",
	})

	assert.Contains(t, prompt, `Repository: "/home/user/project"`)
	assert.Contains(t, prompt, `Current Branch: "feature/auth"`)
	assert.Contains(t, prompt, "Generate a commit message.")
}

func TestBuildPrompt_Messages(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		Messages: []ChatMessage{
			{Role: roleUser, Content: "Hello"},
			{Role: roleAssistant, Content: "Hi there"},
			{Role: roleUser, Content: "How are you?"},
		},
	})

	assert.Contains(t, prompt, "[user]: Hello")
	assert.Contains(t, prompt, "[assistant]: Hi there")
	assert.Contains(t, prompt, "[user]: How are you?")
}

func TestBuildPrompt_MessagesSkipEmpty(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		Messages: []ChatMessage{
			{Role: roleUser, Content: "Hello"},
			{Role: roleAssistant, Content: ""}, // empty content should be skipped
			{Role: roleUser, Content: "Bye"},
		},
	})

	assert.Contains(t, prompt, "[user]: Hello")
	assert.NotContains(t, prompt, "[assistant]")
	assert.Contains(t, prompt, "[user]: Bye")
}

func TestBuildPrompt_FullRequest(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		GitContext: GitContext{
			CurrentBranch: "main",
		},
		Messages: []ChatMessage{
			{Role: roleUser, Content: "previous question"},
		},
		UserPrompt: "new question",
	})

	// All three sections should be present, separated by double newlines.
	assert.Contains(t, prompt, `Current Branch: "main"`)
	assert.Contains(t, prompt, "[user]: previous question")
	assert.Contains(t, prompt, "new question")
}

func TestBuildPrompt_GitContextWithDiffs(t *testing.T) {
	p := &CopilotProvider{}
	prompt := p.buildPrompt(CompletionRequest{
		GitContext: GitContext{
			RepoRoot:      "/repo",
			CurrentBranch: "feature/auth",
			Status: []git.FileStatus{
				{Path: "src/auth.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
			},
			Log: []git.Commit{
				{ShortHash: "abc1234", Subject: "Add login page"},
			},
		},
		UserPrompt: "Generate a commit message for the staged changes.",
	})

	assert.Contains(t, prompt, "feature/auth")
	assert.Contains(t, prompt, "src/auth.go")
	assert.Contains(t, prompt, "abc1234")
	assert.Contains(t, prompt, "Generate a commit message for the staged changes.")
}

// ---------------------------------------------------------------------------
// eventToResponse
// ---------------------------------------------------------------------------

func TestEventToResponse_NilEvent(t *testing.T) {
	resp := eventToResponse(nil)

	assert.Equal(t, finishReasonStop, resp.FinishReason)
	assert.Equal(t, "", resp.Content)
	assert.Equal(t, "copilot-sdk", resp.Metadata["provider"])
}

func TestEventToResponse_WithContent(t *testing.T) {
	content := "Hello, how can I help?"
	event := &copilot.SessionEvent{
		Data: copilot.Data{
			Content: &content,
		},
	}

	resp := eventToResponse(event)

	assert.Equal(t, "Hello, how can I help?", resp.Content)
	assert.Equal(t, finishReasonStop, resp.FinishReason)
}

func TestEventToResponse_WithUsage(t *testing.T) {
	content := "response text"
	inputTokens := float64(10)
	outputTokens := float64(5)
	event := &copilot.SessionEvent{
		Data: copilot.Data{
			Content:      &content,
			InputTokens:  &inputTokens,
			OutputTokens: &outputTokens,
		},
	}

	resp := eventToResponse(event)

	assert.Equal(t, "response text", resp.Content)
	assert.Equal(t, 10, resp.TokensUsed.InputTokens)
	assert.Equal(t, 5, resp.TokensUsed.OutputTokens)
}

func TestEventToResponse_NilContent(t *testing.T) {
	event := &copilot.SessionEvent{
		Data: copilot.Data{},
	}

	resp := eventToResponse(event)

	assert.Equal(t, "", resp.Content)
}

// ---------------------------------------------------------------------------
// extractUsage
// ---------------------------------------------------------------------------

func TestExtractUsage_BothNil(t *testing.T) {
	u := extractUsage(copilot.Data{})
	assert.Nil(t, u)
}

func TestExtractUsage_InputOnly(t *testing.T) {
	input := float64(42)
	u := extractUsage(copilot.Data{InputTokens: &input})

	require.NotNil(t, u)
	assert.Equal(t, 42, u.InputTokens)
	assert.Equal(t, 0, u.OutputTokens)
}

func TestExtractUsage_OutputOnly(t *testing.T) {
	output := float64(7)
	u := extractUsage(copilot.Data{OutputTokens: &output})

	require.NotNil(t, u)
	assert.Equal(t, 0, u.InputTokens)
	assert.Equal(t, 7, u.OutputTokens)
}

func TestExtractUsage_Both(t *testing.T) {
	input := float64(100)
	output := float64(50)
	u := extractUsage(copilot.Data{
		InputTokens:  &input,
		OutputTokens: &output,
	})

	require.NotNil(t, u)
	assert.Equal(t, 100, u.InputTokens)
	assert.Equal(t, 50, u.OutputTokens)
}

func TestExtractUsage_ZeroValues(t *testing.T) {
	input := float64(0)
	output := float64(0)
	u := extractUsage(copilot.Data{
		InputTokens:  &input,
		OutputTokens: &output,
	})

	// Non-nil pointers with zero values should still return a TokenUsage.
	require.NotNil(t, u)
	assert.Equal(t, 0, u.InputTokens)
	assert.Equal(t, 0, u.OutputTokens)
}

// ---------------------------------------------------------------------------
// GitContext serialisation
// ---------------------------------------------------------------------------

func TestSerializeGitContext_Empty(t *testing.T) {
	assert.Equal(t, "", serializeGitContext(GitContext{}))
}

func TestSerializeGitContext_FullContext(t *testing.T) {
	gc := GitContext{
		RepoRoot:      "/repo",
		CurrentBranch: "main",
		TargetBranch:  "feature",
		Status: []git.FileStatus{
			{Path: "file.go", StagedStatus: git.StatusModified, WorktreeStatus: git.StatusUnmodified},
		},
		Log: []git.Commit{
			{ShortHash: "abc1234", Subject: "Initial commit"},
		},
		Diffs: []git.FileDiff{
			{
				Path: "file.go",
				Hunks: []git.Hunk{
					{
						Header: "@@ -1,3 +1,4 @@",
						Lines: []git.DiffLine{
							{Type: git.DiffLineContext, Content: "package main"},
							{Type: git.DiffLineAdded, Content: `import "fmt"`},
						},
					},
				},
			},
		},
	}

	result := serializeGitContext(gc)
	assert.Contains(t, result, `Repository: "/repo"`)
	assert.Contains(t, result, `Current Branch: "main"`)
	assert.Contains(t, result, `Target Branch: "feature"`)
	assert.Contains(t, result, `M  "file.go"`)
	assert.Contains(t, result, `"abc1234" "Initial commit"`)
	assert.Contains(t, result, "--- file.go")
	assert.Contains(t, result, "+import \"fmt\"")
	assert.Contains(t, result, " package main")
}

// ---------------------------------------------------------------------------
// ensureStarted
// ---------------------------------------------------------------------------

func TestEnsureStarted_SetsStartedFlag(t *testing.T) {
	// We cannot call ensureStarted without a real CLI server, but we
	// can verify the initial state and the mutex-protected flag.
	// Use a fake CLIPath so the SDK fails immediately without
	// spawning a real Copilot CLI process.
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli",
		}),
	}

	// Calling ensureStarted will fail (no CLI installed in test env).
	// sync.Once ensures Start is called exactly once.
	_ = p.ensureStarted(context.Background())
	// Start failed, so startErr should be non-nil.
	assert.Error(t, p.startErr)
}

// ---------------------------------------------------------------------------
// Available
// ---------------------------------------------------------------------------

func TestCopilotProvider_Available_ReturnsFalseOnStartFailure(t *testing.T) {
	// When the CLI server cannot start, Available should return false
	// rather than propagating the error.
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli",
		}),
	}

	avail, err := p.Available(context.Background())
	assert.NoError(t, err)
	assert.False(t, avail)
}
