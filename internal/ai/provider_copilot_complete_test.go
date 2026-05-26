package ai

import (
	"context"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests: CopilotProvider Complete/CompleteStream
// ---------------------------------------------------------------------------
// The Copilot SDK requires a running CLI process (client.Start) that cannot
// be mocked with httptest. These tests exercise the error handling paths when
// the SDK client fails to start, verifying that Complete and CompleteStream
// return appropriate errors rather than panicking.

func TestCopilotComplete_FailsWhenClientCannotStart(t *testing.T) {
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "Hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "copilot complete")
}

func TestCopilotCompleteStream_FailsWhenClientCannotStart(t *testing.T) {
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()
	ch, err := p.CompleteStream(ctx, CompletionRequest{
		UserPrompt: "Hello",
	})

	require.Error(t, err)
	assert.Nil(t, ch)
	assert.Contains(t, err.Error(), "copilot stream")
}

func TestCopilotComplete_CachedStartError(t *testing.T) {
	// Test that sync.Once caches the start error and returns it on
	// subsequent calls.
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()

	// First call triggers ensureStarted.
	_, err1 := p.Complete(ctx, CompletionRequest{UserPrompt: "call 1"})
	require.Error(t, err1)

	// Second call should get the same cached error.
	_, err2 := p.Complete(ctx, CompletionRequest{UserPrompt: "call 2"})
	require.Error(t, err2)
	assert.Equal(t, err1.Error(), err2.Error())
}

func TestCopilotCompleteStream_CachedStartError(t *testing.T) {
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()

	// First call triggers ensureStarted.
	_, err1 := p.CompleteStream(ctx, CompletionRequest{UserPrompt: "call 1"})
	require.Error(t, err1)

	// Second call should get the same cached error.
	_, err2 := p.CompleteStream(ctx, CompletionRequest{UserPrompt: "call 2"})
	require.Error(t, err2)
	assert.Equal(t, err1.Error(), err2.Error())
}

func TestCopilotComplete_WithToolsLogsDebug(t *testing.T) {
	// Verify that providing tools does not panic or cause errors beyond
	// the start failure. Tools are logged as debug and not forwarded.
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()
	_, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "Hello",
		Tools: []ToolDefinition{
			{Name: "test_tool", Description: "A test tool"},
		},
	})

	// Should still fail at start, not panic on tools.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copilot complete")
}

func TestCopilotCompleteStream_WithToolsLogsDebug(t *testing.T) {
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()
	_, err := p.CompleteStream(ctx, CompletionRequest{
		UserPrompt: "Hello",
		Tools: []ToolDefinition{
			{Name: "test_tool", Description: "A test tool"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "copilot stream")
}

func TestCopilotClose_AfterFailedStart(t *testing.T) {
	// After a failed start, Close should not panic.
	p := &CopilotProvider{
		client: copilot.NewClient(&copilot.ClientOptions{
			CLIPath: "/nonexistent/copilot-cli-binary",
		}),
		model: "gpt-4o",
	}

	ctx := context.Background()
	// Trigger start failure.
	_, _ = p.Complete(ctx, CompletionRequest{UserPrompt: "Hello"})

	// Close should handle the failed-start state gracefully.
	err := p.Close()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests: Interface compliance
// ---------------------------------------------------------------------------

func TestCopilotProviderImplementsAIProvider(t *testing.T) {
	var _ AIProvider = (*CopilotProvider)(nil)
}
