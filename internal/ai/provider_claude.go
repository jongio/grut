package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

const (
	// defaultClaudeModel is the Anthropic model used when none is specified.
	defaultClaudeModel = "claude-sonnet-4-20250514"

	// defaultClaudeMaxTokens caps generation length when the caller does not
	// provide an explicit limit.
	defaultClaudeMaxTokens = 8192

	// claudeAPIKeyEnv is the only supported mechanism for authenticating with
	// the Anthropic API. Config-file secrets are intentionally unsupported.
	claudeAPIKeyEnv = "ANTHROPIC_API_KEY"
)

// ClaudeProvider implements AIProvider using the Anthropic Claude API
// via the anthropic-sdk-go SDK.
type ClaudeProvider struct {
	client    *anthropic.Client
	model     string
	maxTokens int
}

// NewClaudeProvider creates a ClaudeProvider. model defaults to
// claude-sonnet-4-20250514 when empty; maxTokens defaults to 8192
// when <= 0. The SDK reads ANTHROPIC_API_KEY from the environment
// automatically.
func NewClaudeProvider(model string, maxTokens int) *ClaudeProvider {
	if model == "" {
		model = defaultClaudeModel
	}
	if maxTokens <= 0 {
		maxTokens = defaultClaudeMaxTokens
	}

	client := anthropic.NewClient(
		option.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}),
	)
	return &ClaudeProvider{
		client:    &client,
		model:     model,
		maxTokens: maxTokens,
	}
}

// Name returns "claude".
func (p *ClaudeProvider) Name() string { return providerClaude }

// Available reports true when the ANTHROPIC_API_KEY env var is set.
func (p *ClaudeProvider) Available(_ context.Context) (bool, error) {
	return os.Getenv(claudeAPIKeyEnv) != "", nil
}

// Complete sends a one-shot completion request and blocks until the full
// response is ready.
func (p *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	params := p.buildParams(req)

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("claude completion: %w", err)
	}

	return p.parseResponse(msg), nil
}

// CompleteStream sends a completion request and returns a channel that
// delivers incremental StreamChunks. The channel is closed when the
// response completes or an error occurs.
func (p *ClaudeProvider) CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	params := p.buildParams(req)

	stream := p.client.Messages.NewStreaming(ctx, params)
	ch := make(chan StreamChunk, 64)

	go p.consumeStream(ctx, stream, ch)

	return ch, nil
}

// Close releases resources. The SDK client is stateless, so this is a no-op.
func (p *ClaudeProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// buildParams converts a CompletionRequest into Anthropic MessageNewParams.
func (p *ClaudeProvider) buildParams(req CompletionRequest) anthropic.MessageNewParams {
	maxTokens := int64(p.maxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: maxTokens,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	params.Messages = p.convertMessages(req.Messages)

	if req.UserPrompt != "" {
		params.Messages = append(
			params.Messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
		)
	}

	if len(req.Tools) > 0 {
		params.Tools = p.convertTools(req.Tools)
	}

	return params
}

// convertMessages maps grut ChatMessages to Anthropic MessageParams.
// System-role messages are skipped because Anthropic handles the system
// prompt separately via the System field.
func (p *ClaudeProvider) convertMessages(msgs []ChatMessage) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(msgs))

	for _, msg := range msgs {
		switch msg.Role {
		case roleUser:
			result = append(
				result,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)),
			)

		case roleAssistant:
			if len(msg.ToolCalls) > 0 {
				blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: tc.Arguments,
						},
					})
				}
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			} else {
				result = append(
					result,
					anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)),
				)
			}

		case "tool":
			result = append(
				result,
				anthropic.NewUserMessage(anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: msg.ToolID,
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: msg.Content}},
						},
					},
				}),
			)

		case "system":
			// Handled via SystemPrompt → params.System; skip here.
			continue
		}
	}

	return result
}

// convertTools maps grut ToolDefinitions to Anthropic ToolUnionParams.
func (p *ClaudeProvider) convertTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		tool := anthropic.ToolParam{
			Name: t.Name,
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.Parameters,
			},
		}
		if t.Description != "" {
			tool.Description = param.NewOpt(t.Description)
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &tool})
	}

	return result
}

// parseResponse converts an Anthropic Message into a grut CompletionResponse.
func (p *ClaudeProvider) parseResponse(msg *anthropic.Message) CompletionResponse {
	resp := CompletionResponse{
		TokensUsed: TokenUsage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
		Metadata: map[string]string{
			"model":      msg.Model,
			"message_id": msg.ID,
		},
	}

	// Map Anthropic stop reasons to grut's finish reasons.
	switch msg.StopReason {
	case anthropic.StopReasonEndTurn:
		resp.FinishReason = finishReasonStop
	case anthropic.StopReasonMaxTokens:
		resp.FinishReason = "length"
	case anthropic.StopReasonToolUse:
		resp.FinishReason = "tool_calls"
	default:
		resp.FinishReason = string(msg.StopReason)
	}

	// Extract text and tool-use blocks from the response content.
	for _, block := range msg.Content {
		switch block.Type {
		case blockTypeText:
			resp.Content += block.Text
		case blockTypeToolUse:
			var args map[string]any
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &args); err != nil {
					slog.Warn("claude: malformed tool input JSON", "error", err, "tool", block.Name)
				}
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return resp
}

// consumeStream reads from the Anthropic SSE stream and forwards chunks
// to ch. It always closes ch before returning. Callers must not write to
// ch after launching this goroutine. The context is used to abandon the
// stream if the consumer stops reading (prevents goroutine leak, CWE-404).
func (p *ClaudeProvider) consumeStream(ctx context.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion], ch chan<- StreamChunk) {
	defer close(ch)
	defer func() { _ = stream.Close() }()

	var (
		inputTokens  int64
		outputTokens int64

		// State for accumulating tool-use blocks across deltas.
		inToolUse       bool
		currentToolID   string
		currentToolName string
		currentToolJSON strings.Builder
	)

	// sendChunk sends a chunk to ch with ctx cancellation guard to
	// prevent this goroutine from blocking forever if the consumer
	// stops reading.
	sendChunk := func(chunk StreamChunk) bool {
		select {
		case ch <- chunk:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for stream.Next() {
		// Check for context cancellation between events.
		if ctx.Err() != nil {
			return
		}

		event := stream.Current()

		switch event.Type {
		case "message_start":
			inputTokens = event.Message.Usage.InputTokens

		case "content_block_start":
			if event.ContentBlock.Type == blockTypeToolUse {
				currentToolID = event.ContentBlock.ID
				currentToolName = event.ContentBlock.Name
				currentToolJSON.Reset()
				inToolUse = true
			}

		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				if !sendChunk(StreamChunk{Delta: event.Delta.Text}) {
					return
				}
			case "input_json_delta":
				const maxToolInputSize = 1 << 20 // 1 MiB
				if currentToolJSON.Len()+len(event.Delta.PartialJSON) > maxToolInputSize {
					sendChunk(StreamChunk{Err: fmt.Errorf("tool input JSON exceeded %d bytes", maxToolInputSize), Done: true})
					return
				}
				currentToolJSON.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			if inToolUse {
				var args map[string]any
				if raw := currentToolJSON.String(); raw != "" {
					if err := json.Unmarshal([]byte(raw), &args); err != nil {
						slog.Warn("claude: malformed tool input JSON", "error", err, "tool", currentToolName)
					}
				}
				if !sendChunk(StreamChunk{
					ToolCalls: []ToolCall{{
						ID:        currentToolID,
						Name:      currentToolName,
						Arguments: args,
					}},
				}) {
					return
				}
				inToolUse = false
			}

		case "message_delta":
			outputTokens = event.Usage.OutputTokens
		}
	}

	if err := stream.Err(); err != nil {
		sendChunk(StreamChunk{Err: err, Done: true})
		return
	}

	sendChunk(StreamChunk{
		Done: true,
		TokensUsed: &TokenUsage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
		},
	})
}
