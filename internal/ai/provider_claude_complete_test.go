package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers for Claude Complete/CompleteStream tests
// ---------------------------------------------------------------------------

// newClaudeWithMockServer creates a ClaudeProvider whose SDK client is pointed
// at the given httptest server URL.
func newClaudeWithMockServer(t *testing.T, serverURL string) *ClaudeProvider {
	t.Helper()
	client := anthropic.NewClient(
		option.WithBaseURL(serverURL),
		option.WithAPIKey("sk-ant-test-key"),
		option.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
		option.WithMaxRetries(0),
	)
	return &ClaudeProvider{
		client:    &client,
		model:     defaultClaudeModel,
		maxTokens: defaultClaudeMaxTokens,
	}
}

// anthropicMessageResponse builds a minimal JSON response matching the
// Anthropic Messages API response format.
func anthropicMessageResponse(id, model, content string, stopReason string, inputTokens, outputTokens int) map[string]any {
	return map[string]any{
		"id":    id,
		"type":  "message",
		"role":  "assistant",
		"model": model,
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
}

// ---------------------------------------------------------------------------
// Tests: Complete
// ---------------------------------------------------------------------------

func TestClaudeComplete_Success(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify correct endpoint and method.
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.Header.Get("Content-Type"), "application/json")

		resp := anthropicMessageResponse(
			"msg_test_001",
			defaultClaudeModel,
			"Hello from Claude!",
			"end_turn",
			10, 5,
		)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	result, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "Say hello",
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello from Claude!", result.Content)
	assert.Equal(t, finishReasonStop, result.FinishReason)
	assert.Equal(t, 10, result.TokensUsed.InputTokens)
	assert.Equal(t, 5, result.TokensUsed.OutputTokens)
	assert.Equal(t, "msg_test_001", result.Metadata["message_id"])
}

func TestClaudeComplete_WithSystemPrompt(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := anthropicMessageResponse("msg_002", defaultClaudeModel, "OK", "end_turn", 5, 2)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	_, err := p.Complete(ctx, CompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
		UserPrompt:   "Hi",
	})

	require.NoError(t, err)
	// Verify system prompt was included in request body.
	system, ok := receivedBody["system"]
	require.True(t, ok, "request body should include 'system' field")
	systemArr, ok := system.([]any)
	require.True(t, ok)
	require.NotEmpty(t, systemArr)
}

func TestClaudeComplete_ServerError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"internal_error","message":"server is overloaded"}}`))
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	_, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "Hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude completion")
}

func TestClaudeComplete_ContextCancelled(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	// Use a server that responds slowly. The client's context timeout should
	// cause the SDK to abort the request.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait until request is cancelled by client (SDK propagates context).
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the request never completes.
	cancel()

	_, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "Hello",
	})

	require.Error(t, err)
}

func TestClaudeComplete_ToolUseResponse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"id":    "msg_tool_001",
			"type":  "message",
			"role":  "assistant",
			"model": defaultClaudeModel,
			"content": []map[string]any{
				{"type": "text", "text": "Let me check that."},
				{
					"type":  "tool_use",
					"id":    "toolu_01",
					"name":  "get_weather",
					"input": map[string]any{"city": "Seattle"},
				},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 20, "output_tokens": 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	result, err := p.Complete(ctx, CompletionRequest{
		UserPrompt: "What's the weather?",
		Tools: []ToolDefinition{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"city": map[string]any{"type": "string"}}},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Let me check that.", result.Content)
	assert.Equal(t, "tool_calls", result.FinishReason)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "toolu_01", result.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.Equal(t, "Seattle", result.ToolCalls[0].Arguments["city"])
}

func TestClaudeComplete_MaxTokensResponse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := anthropicMessageResponse("msg_length", defaultClaudeModel, "truncated text...", "max_tokens", 100, 8192)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	result, err := p.Complete(ctx, CompletionRequest{UserPrompt: "Write a long essay"})

	require.NoError(t, err)
	assert.Equal(t, "length", result.FinishReason)
	assert.Equal(t, "truncated text...", result.Content)
}

// ---------------------------------------------------------------------------
// Tests: CompleteStream
// ---------------------------------------------------------------------------

func TestClaudeCompleteStream_Success(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify streaming header.
		assert.Contains(t, r.Header.Get("Content-Type"), "application/json")

		// Send SSE stream.
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		events := []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_stream","type":"message","role":"assistant","content":[],"model":"` + defaultClaudeModel + `","stop_reason":null,"usage":{"input_tokens":15,"output_tokens":0}}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			fmt.Fprintf(w, "%s\n\n", event)
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	ch, err := p.CompleteStream(ctx, CompletionRequest{
		UserPrompt: "Say hello",
	})

	require.NoError(t, err)
	require.NotNil(t, ch)

	var content strings.Builder
	var finalChunk StreamChunk
	for chunk := range ch {
		if chunk.Done {
			finalChunk = chunk
			break
		}
		if chunk.Delta != "" {
			content.WriteString(chunk.Delta)
		}
	}

	assert.Equal(t, "Hello world", content.String())
	assert.True(t, finalChunk.Done)
	assert.NoError(t, finalChunk.Err)
	require.NotNil(t, finalChunk.TokensUsed)
	assert.Equal(t, 15, finalChunk.TokensUsed.InputTokens)
	assert.Equal(t, 3, finalChunk.TokensUsed.OutputTokens)
}

func TestClaudeCompleteStream_ContextCancellation(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Send initial event then hang.
		fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cancel\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"%s\",\"stop_reason\":null,\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n", defaultClaudeModel)
		flusher.Flush()

		// Block until the request context is cancelled.
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.CompleteStream(ctx, CompletionRequest{UserPrompt: "Hi"})
	require.NoError(t, err)

	// Cancel the context after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Drain the channel — it should close eventually after cancellation.
	var gotDone bool
	for chunk := range ch {
		if chunk.Done {
			gotDone = true
		}
	}
	// Channel must be closed (range exits).
	_ = gotDone // The channel closing proves cancellation worked.
}

func TestClaudeCompleteStream_ServerError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	ch, err := p.CompleteStream(ctx, CompletionRequest{UserPrompt: "Hi"})

	// The SDK may return either: error from CompleteStream, or send error via channel.
	if err != nil {
		// Error at call site.
		assert.Nil(t, ch)
		return
	}
	// Error comes through the channel.
	require.NotNil(t, ch)
	var gotErr bool
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "expected an error chunk from stream")
}

func TestClaudeCompleteStream_ToolUse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_tool_stream","type":"message","role":"assistant","content":[],"model":"` + defaultClaudeModel + `","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":0}}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_stream_01","name":"search"}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"test\"}"}}`,
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			fmt.Fprintf(w, "%s\n\n", event)
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := newClaudeWithMockServer(t, server.URL)
	ctx := context.Background()

	ch, err := p.CompleteStream(ctx, CompletionRequest{
		UserPrompt: "Search for test",
		Tools:      []ToolDefinition{{Name: "search", Description: "Search", Parameters: map[string]any{}}},
	})

	require.NoError(t, err)

	var toolCalls []ToolCall
	for chunk := range ch {
		if len(chunk.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.ToolCalls...)
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "toolu_stream_01", toolCalls[0].ID)
	assert.Equal(t, "search", toolCalls[0].Name)
	assert.Equal(t, "test", toolCalls[0].Arguments["q"])
}
