package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewClaudeProvider_Defaults(t *testing.T) {
	p := NewClaudeProvider("", 0)

	assert.Equal(t, defaultClaudeModel, p.model)
	assert.Equal(t, defaultClaudeMaxTokens, p.maxTokens)
	assert.NotNil(t, p.client)
}

func TestNewClaudeProvider_Custom(t *testing.T) {
	p := NewClaudeProvider("claude-opus-4-20250514", 4096)

	assert.Equal(t, "claude-opus-4-20250514", p.model)
	assert.Equal(t, 4096, p.maxTokens)
}

func TestClaudeName(t *testing.T) {
	p := NewClaudeProvider("", 0)
	assert.Equal(t, providerClaude, p.Name())
}

// ---------------------------------------------------------------------------
// Available
// ---------------------------------------------------------------------------

func TestClaudeAvailable_WithKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	p := NewClaudeProvider("", 0)
	ok, err := p.Available(context.Background())

	require.NoError(t, err)
	assert.True(t, ok)
}

func TestClaudeAvailable_WithoutKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	p := NewClaudeProvider("", 0)
	ok, err := p.Available(context.Background())

	require.NoError(t, err)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// buildParams
// ---------------------------------------------------------------------------

func TestBuildParams_Defaults(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{})

	assert.Equal(t, anthropic.Model(defaultClaudeModel), params.Model)
	assert.Equal(t, int64(defaultClaudeMaxTokens), params.MaxTokens)
	assert.Empty(t, params.System)
	assert.Empty(t, params.Messages)
	assert.Empty(t, params.Tools)
}

func TestBuildParams_SystemPrompt(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
	})

	require.Len(t, params.System, 1)
	assert.Equal(t, "You are a helpful assistant.", params.System[0].Text)
}

func TestBuildParams_UserPrompt(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{
		UserPrompt: "What is Go?",
	})

	require.Len(t, params.Messages, 1)
	assert.Equal(t, anthropic.MessageParamRoleUser, params.Messages[0].Role)
	require.Len(t, params.Messages[0].Content, 1)
	require.NotNil(t, params.Messages[0].Content[0].OfText)
	assert.Equal(t, "What is Go?", params.Messages[0].Content[0].OfText.Text)
}

func TestBuildParams_Temperature(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{
		Temperature: 0.7,
		UserPrompt:  "test",
	})

	assert.True(t, params.Temperature.Valid())
	assert.InDelta(t, 0.7, params.Temperature.Value, 0.001)
}

func TestBuildParams_MaxTokensOverride(t *testing.T) {
	p := NewClaudeProvider("", 8192)
	params := p.buildParams(CompletionRequest{
		MaxTokens:  2048,
		UserPrompt: "test",
	})

	assert.Equal(t, int64(2048), params.MaxTokens)
}

func TestBuildParams_MessagesBeforeUserPrompt(t *testing.T) {
	p := NewClaudeProvider("", 0)
	params := p.buildParams(CompletionRequest{
		Messages: []ChatMessage{
			{Role: roleUser, Content: "First question"},
			{Role: roleAssistant, Content: "First answer"},
		},
		UserPrompt: "Follow-up question",
	})

	// 2 from Messages + 1 from UserPrompt.
	require.Len(t, params.Messages, 3)
	// Last message is the UserPrompt.
	last := params.Messages[2]
	assert.Equal(t, anthropic.MessageParamRoleUser, last.Role)
	require.Len(t, last.Content, 1)
	require.NotNil(t, last.Content[0].OfText)
	assert.Equal(t, "Follow-up question", last.Content[0].OfText.Text)
}

// ---------------------------------------------------------------------------
// convertMessages
// ---------------------------------------------------------------------------

func TestConvertMessages_UserAndAssistant(t *testing.T) {
	p := NewClaudeProvider("", 0)
	msgs := p.convertMessages([]ChatMessage{
		{Role: roleUser, Content: "Hello"},
		{Role: roleAssistant, Content: "Hi there!"},
	})

	require.Len(t, msgs, 2)

	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[0].Role)
	require.NotNil(t, msgs[0].Content[0].OfText)
	assert.Equal(t, "Hello", msgs[0].Content[0].OfText.Text)

	assert.Equal(t, anthropic.MessageParamRoleAssistant, msgs[1].Role)
	require.NotNil(t, msgs[1].Content[0].OfText)
	assert.Equal(t, "Hi there!", msgs[1].Content[0].OfText.Text)
}

func TestConvertMessages_AssistantWithToolCalls(t *testing.T) {
	p := NewClaudeProvider("", 0)
	msgs := p.convertMessages([]ChatMessage{
		{
			Role:    roleAssistant,
			Content: "Let me check the weather.",
			ToolCalls: []ToolCall{
				{
					ID:        "toolu_01",
					Name:      "get_weather",
					Arguments: map[string]any{"city": "Seattle"},
				},
			},
		},
	})

	require.Len(t, msgs, 1)
	assert.Equal(t, anthropic.MessageParamRoleAssistant, msgs[0].Role)

	// Text block + tool-use block.
	require.Len(t, msgs[0].Content, 2)
	require.NotNil(t, msgs[0].Content[0].OfText)
	assert.Equal(t, "Let me check the weather.", msgs[0].Content[0].OfText.Text)

	require.NotNil(t, msgs[0].Content[1].OfToolUse)
	assert.Equal(t, "toolu_01", msgs[0].Content[1].OfToolUse.ID)
	assert.Equal(t, "get_weather", msgs[0].Content[1].OfToolUse.Name)
}

func TestConvertMessages_ToolResult(t *testing.T) {
	p := NewClaudeProvider("", 0)
	msgs := p.convertMessages([]ChatMessage{
		{
			Role:    "tool",
			Content: `{"temperature": 72}`,
			ToolID:  "toolu_01",
		},
	})

	require.Len(t, msgs, 1)
	// Tool results are sent as user messages in the Anthropic API.
	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[0].Role)
	require.NotNil(t, msgs[0].Content[0].OfToolResult)
	assert.Equal(t, "toolu_01", msgs[0].Content[0].OfToolResult.ToolUseID)
}

func TestConvertMessages_SkipsSystem(t *testing.T) {
	p := NewClaudeProvider("", 0)
	msgs := p.convertMessages([]ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: roleUser, Content: "Hi"},
	})

	// System message is skipped.
	require.Len(t, msgs, 1)
	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[0].Role)
}

// ---------------------------------------------------------------------------
// convertTools
// ---------------------------------------------------------------------------

func TestConvertTools(t *testing.T) {
	p := NewClaudeProvider("", 0)
	tools := p.convertTools([]ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get current weather for a location",
			Parameters: map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "City name",
				},
			},
		},
	})

	require.Len(t, tools, 1)
	require.NotNil(t, tools[0].OfTool)

	tool := tools[0].OfTool
	assert.Equal(t, "get_weather", tool.Name)
	assert.True(t, tool.Description.Valid())
	assert.Equal(t, "Get current weather for a location", tool.Description.Value)
	assert.NotNil(t, tool.InputSchema.Properties)
}

func TestConvertTools_NoDescription(t *testing.T) {
	p := NewClaudeProvider("", 0)
	tools := p.convertTools([]ToolDefinition{
		{Name: "do_thing", Parameters: map[string]any{}},
	})

	require.Len(t, tools, 1)
	require.NotNil(t, tools[0].OfTool)
	assert.False(t, tools[0].OfTool.Description.Valid())
}

// ---------------------------------------------------------------------------
// parseResponse
// ---------------------------------------------------------------------------

func TestParseResponse_TextContent(t *testing.T) {
	p := NewClaudeProvider("", 0)

	msg := &anthropic.Message{
		ID:    "msg_123",
		Model: "claude-sonnet-4-20250514",
		Content: []anthropic.ContentBlockUnion{
			{Type: blockTypeText, Text: "Hello, world!"},
		},
		StopReason: anthropic.StopReasonEndTurn,
		Usage: anthropic.Usage{
			InputTokens:  15,
			OutputTokens: 4,
		},
	}

	resp := p.parseResponse(msg)

	assert.Equal(t, "Hello, world!", resp.Content)
	assert.Equal(t, finishReasonStop, resp.FinishReason)
	assert.Equal(t, 15, resp.TokensUsed.InputTokens)
	assert.Equal(t, 4, resp.TokensUsed.OutputTokens)
	assert.Equal(t, "msg_123", resp.Metadata["message_id"])
	assert.Equal(t, "claude-sonnet-4-20250514", resp.Metadata["model"])
	assert.Empty(t, resp.ToolCalls)
}

func TestParseResponse_ToolUseContent(t *testing.T) {
	p := NewClaudeProvider("", 0)

	toolInput, _ := json.Marshal(map[string]any{"city": "Seattle"})

	msg := &anthropic.Message{
		ID:    "msg_456",
		Model: "claude-sonnet-4-20250514",
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  blockTypeToolUse,
				ID:    "toolu_01",
				Name:  "get_weather",
				Input: json.RawMessage(toolInput),
			},
		},
		StopReason: anthropic.StopReasonToolUse,
		Usage: anthropic.Usage{
			InputTokens:  20,
			OutputTokens: 10,
		},
	}

	resp := p.parseResponse(msg)

	assert.Empty(t, resp.Content)
	assert.Equal(t, "tool_calls", resp.FinishReason)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "toolu_01", resp.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", resp.ToolCalls[0].Name)
	assert.Equal(t, "Seattle", resp.ToolCalls[0].Arguments["city"])
}

func TestParseResponse_MixedContent(t *testing.T) {
	p := NewClaudeProvider("", 0)

	toolInput, _ := json.Marshal(map[string]any{"q": "weather"})

	msg := &anthropic.Message{
		ID:    "msg_789",
		Model: "claude-sonnet-4-20250514",
		Content: []anthropic.ContentBlockUnion{
			{Type: blockTypeText, Text: "Let me search for that."},
			{
				Type:  blockTypeToolUse,
				ID:    "toolu_02",
				Name:  "search",
				Input: json.RawMessage(toolInput),
			},
		},
		StopReason: anthropic.StopReasonToolUse,
		Usage:      anthropic.Usage{InputTokens: 30, OutputTokens: 15},
	}

	resp := p.parseResponse(msg)

	assert.Equal(t, "Let me search for that.", resp.Content)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "search", resp.ToolCalls[0].Name)
}

func TestParseResponse_StopReasons(t *testing.T) {
	p := NewClaudeProvider("", 0)

	tests := []struct {
		stopReason anthropic.StopReason
		expected   string
	}{
		{anthropic.StopReasonEndTurn, finishReasonStop},
		{anthropic.StopReasonMaxTokens, "length"},
		{anthropic.StopReasonToolUse, "tool_calls"},
		{anthropic.StopReasonStopSequence, "stop_sequence"},
	}

	for _, tt := range tests {
		t.Run(string(tt.stopReason), func(t *testing.T) {
			msg := &anthropic.Message{
				StopReason: tt.stopReason,
				Usage:      anthropic.Usage{},
			}
			resp := p.parseResponse(msg)
			assert.Equal(t, tt.expected, resp.FinishReason)
		})
	}
}

func TestParseResponse_EmptyInput(t *testing.T) {
	p := NewClaudeProvider("", 0)

	msg := &anthropic.Message{
		ID:         "msg_empty",
		Model:      "claude-sonnet-4-20250514",
		Content:    []anthropic.ContentBlockUnion{},
		StopReason: anthropic.StopReasonEndTurn,
		Usage:      anthropic.Usage{},
	}

	resp := p.parseResponse(msg)

	assert.Empty(t, resp.Content)
	assert.Empty(t, resp.ToolCalls)
	assert.Equal(t, finishReasonStop, resp.FinishReason)
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestClaudeProviderImplementsAIProvider(t *testing.T) {
	var _ AIProvider = (*ClaudeProvider)(nil)
}
