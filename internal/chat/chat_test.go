package chat

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock AI provider
// ---------------------------------------------------------------------------

type chatMockProvider struct {
	name      string
	response  ai.CompletionResponse
	streamCh  chan ai.StreamChunk
	err       error
	available bool
	lastReq   ai.CompletionRequest
}

func newChatMockProvider() *chatMockProvider {
	return &chatMockProvider{
		name:      "mock",
		available: true,
	}
}

func (p *chatMockProvider) Name() string { return p.name }

func (p *chatMockProvider) Available(_ context.Context) (bool, error) {
	return p.available, nil
}

func (p *chatMockProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	p.lastReq = req
	return p.response, p.err
}

func (p *chatMockProvider) CompleteStream(_ context.Context, req ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	p.lastReq = req
	if p.err != nil {
		return nil, p.err
	}
	if p.streamCh != nil {
		return p.streamCh, nil
	}

	// Default: return a channel that immediately provides the response.
	ch := make(chan ai.StreamChunk, 2)
	ch <- ai.StreamChunk{
		Delta:     p.response.Content,
		ToolCalls: p.response.ToolCalls,
	}
	ch <- ai.StreamChunk{Done: true}
	close(ch)
	return ch, nil
}

func (p *chatMockProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestChatModel(t *testing.T) (Model, *chatMockProvider) {
	t.Helper()

	provider := newChatMockProvider()
	provider.response = ai.CompletionResponse{
		Content:      "Hello! I'm grut's AI assistant.",
		FinishReason: "stop",
	}

	cfg := ai.NewRegistry(config.AIConfig{
		Provider: "mock",
	})
	cfg.Register("mock", provider)

	toolReg := NewToolRegistry()
	executor := &ToolExecutor{registry: toolReg}
	confirmer := NewConfirmationManager(toolReg)
	sysBuilder := NewSystemPromptBuilder(nil, "You are a test assistant.")
	redactor := ai.NewRedactor(nil)

	th := &theme.Theme{
		Name:    "test",
		Variant: "dark",
	}

	m := New(Deps{
		Registry:   cfg,
		Executor:   executor,
		Confirming: confirmer,
		SysPrompt:  sysBuilder,
		Redactor:   redactor,
		Theme:      th,
	})
	return m, provider
}

// ---------------------------------------------------------------------------
// Tests: Constructor and initial state
// ---------------------------------------------------------------------------

func TestNew_InitialState(t *testing.T) {
	m, _ := newTestChatModel(t)

	assert.False(t, m.Focused(), "should start unfocused")
	assert.False(t, m.expanded, "should start collapsed")
	assert.False(t, m.streaming, "should not be streaming")
	assert.Empty(t, m.messages, "should have no messages")
	assert.Empty(t, m.lastResponse, "should have no last response")
	assert.Nil(t, m.err, "should have no error")
	assert.Equal(t, collapsedHeight, m.Height(), "should report collapsed height")
	assert.Equal(t, "Ready", m.status, "should start with Ready status")
	assert.Zero(t, m.spinnerFrame, "should start with zero spinner frame")
	assert.False(t, m.overlayMode, "should start without overlay mode")
}

func TestInit_ReturnsNil(t *testing.T) {
	m, _ := newTestChatModel(t)
	cmd := m.Init()
	assert.Nil(t, cmd, "Init should return nil")
}

// ---------------------------------------------------------------------------
// Tests: Focus management
// ---------------------------------------------------------------------------

func TestFocus_Blur(t *testing.T) {
	m, _ := newTestChatModel(t)

	assert.False(t, m.Focused())
	assert.False(t, m.overlayMode)

	m.Focus()
	assert.True(t, m.Focused())
	assert.True(t, m.overlayMode)
	assert.True(t, m.expanded)

	m.Blur()
	assert.False(t, m.Focused())
	assert.False(t, m.overlayMode)
	assert.False(t, m.expanded)
}

func TestFocus_BlurToggle(t *testing.T) {
	m, _ := newTestChatModel(t)

	m.Focus()
	assert.True(t, m.Focused())
	assert.True(t, m.overlayMode)

	m.Focus() // Focusing again should be idempotent.
	assert.True(t, m.Focused())

	m.Blur()
	assert.False(t, m.Focused())
	assert.False(t, m.overlayMode)

	m.Blur() // Blurring again should be idempotent.
	assert.False(t, m.Focused())
}

// ---------------------------------------------------------------------------
// Tests: Expanded/collapsed toggle
// ---------------------------------------------------------------------------

func TestToggleExpanded(t *testing.T) {
	m, _ := newTestChatModel(t)

	assert.Equal(t, collapsedHeight, m.Height())
	assert.False(t, m.expanded)

	// Set focused directly (without overlay mode) to test ctrl+e in isolation.
	m.focused = true
	m.input.Focus()

	// Simulate ctrl+e.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})

	assert.True(t, m.expanded)
	assert.Equal(t, expandedHeight, m.Height())

	// Toggle back.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})

	assert.False(t, m.expanded)
	assert.Equal(t, collapsedHeight, m.Height())
}

// ---------------------------------------------------------------------------
// Tests: Clear history
// ---------------------------------------------------------------------------

func TestClearHistory(t *testing.T) {
	m, _ := newTestChatModel(t)

	// Add some messages manually.
	m.messages = append(m.messages, ai.ChatMessage{Role: "user", Content: "hello"})
	m.messages = append(m.messages, ai.ChatMessage{Role: "assistant", Content: "hi"})
	m.lastResponse = "hi"
	m.scrollOffset = 5

	m.ClearHistory()

	assert.Empty(t, m.messages)
	assert.Empty(t, m.lastResponse)
	assert.Zero(t, m.scrollOffset)
	assert.Nil(t, m.err)
}

func TestClearHistory_ViaCtrlL(t *testing.T) {
	m, _ := newTestChatModel(t)

	m.messages = append(m.messages, ai.ChatMessage{Role: "user", Content: "test"})
	m.lastResponse = "response"

	m.Focus()
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})

	assert.Empty(t, m.messages)
	assert.Empty(t, m.lastResponse)
}

// ---------------------------------------------------------------------------
// Tests: Escape blurs chat
// ---------------------------------------------------------------------------

func TestEscape_BlursChat(t *testing.T) {
	m, _ := newTestChatModel(t)

	m.Focus()
	assert.True(t, m.Focused())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, m.Focused())
}

// ---------------------------------------------------------------------------
// Tests: Message sending flow
// ---------------------------------------------------------------------------

func TestSendMessage_AddsToHistory(t *testing.T) {
	m, _ := newTestChatModel(t)

	m.Focus()

	// Simulate typing and sending.
	m, cmd := m.sendMessage("hello AI")

	require.Len(t, m.messages, 1)
	assert.Equal(t, "user", m.messages[0].Role)
	assert.Equal(t, "hello AI", m.messages[0].Content)
	assert.True(t, m.streaming, "should be in streaming state")
	assert.NotNil(t, cmd, "should return a command to start streaming")
}

func TestSendMessage_EmptyIgnored(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.Focus()

	// Set empty input.
	m.input.SetValue("")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.Empty(t, m.messages, "empty message should not be sent")
	assert.Nil(t, cmd)
}

func TestSendMessage_WhitespaceIgnored(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.Focus()

	m.input.SetValue("   ")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.Empty(t, m.messages, "whitespace-only message should not be sent")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Stream done handling
// ---------------------------------------------------------------------------

func TestStreamDone_UpdatesResponse(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.streaming = true

	m, _ = m.Update(streamDoneMsg{
		response: "AI response text",
	})

	assert.False(t, m.streaming)
	assert.Equal(t, "AI response text", m.lastResponse)
	require.Len(t, m.messages, 1)
	assert.Equal(t, "assistant", m.messages[0].Role)
	assert.Equal(t, "AI response text", m.messages[0].Content)
}

func TestStreamDone_WithError(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.streaming = true

	m, _ = m.Update(streamDoneMsg{
		err: context.Canceled,
	})

	assert.False(t, m.streaming)
	assert.ErrorIs(t, m.err, context.Canceled)
}

func TestStreamDone_WithToolCalls_EmitsToolCallMsg(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.streaming = true

	toolCalls := []ai.ToolCall{
		{ID: "call_1", Name: "git_status", Arguments: nil},
	}

	m, cmd := m.Update(streamDoneMsg{
		response: "",
		tools:    toolCalls,
	})

	assert.False(t, m.streaming)
	require.NotNil(t, cmd, "should emit a command for tool calls")

	// Execute the command to get the ToolCallMsg.
	msg := cmd()
	tcMsg, ok := msg.(ToolCallMsg)
	require.True(t, ok, "command should produce ToolCallMsg")
	assert.Len(t, tcMsg.Calls, 1)
	assert.Equal(t, "git_status", tcMsg.Calls[0].Name)
}

// ---------------------------------------------------------------------------
// Tests: Confirmation flow
// ---------------------------------------------------------------------------

func TestConfirmation_DestructiveToolShowsPrompt(t *testing.T) {
	m, _ := newTestChatModel(t)

	// Simulate a destructive tool call.
	calls := []ai.ToolCall{
		{ID: "call_del", Name: "file_delete", Arguments: map[string]any{"path": "important.go"}},
	}

	m, _ = m.Update(ToolCallMsg{Calls: calls})

	assert.True(t, m.confirming.HasPending(), "should have pending confirmation")

	// View should show the confirmation prompt.
	view := m.View()
	assert.Contains(t, view, "[y/N]")
}

func TestConfirmation_AcceptExecutes(t *testing.T) {
	m, _ := newTestChatModel(t)

	calls := []ai.ToolCall{
		{ID: "call_del", Name: "file_delete", Arguments: map[string]any{"path": "test.go"}},
	}

	m, _ = m.Update(ToolCallMsg{Calls: calls})
	require.True(t, m.confirming.HasPending())

	// Press 'y' to accept.
	m.Focus()
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})

	assert.False(t, m.confirming.HasPending(), "confirmation should be cleared")
	assert.NotNil(t, cmd, "should start tool execution")
}

func TestConfirmation_RejectCancels(t *testing.T) {
	m, _ := newTestChatModel(t)

	calls := []ai.ToolCall{
		{ID: "call_del", Name: "file_delete", Arguments: map[string]any{"path": "test.go"}},
	}

	m, _ = m.Update(ToolCallMsg{Calls: calls})
	require.True(t, m.confirming.HasPending())

	// Press 'n' to reject.
	m.Focus()
	m, _ = m.Update(tea.KeyPressMsg{Code: 'n'})

	assert.False(t, m.confirming.HasPending(), "confirmation should be cleared")
	assert.Contains(t, m.lastResponse, "Cancelled")
}

func TestConfirmation_EscapeRejects(t *testing.T) {
	m, _ := newTestChatModel(t)

	calls := []ai.ToolCall{
		{ID: "call_del", Name: "file_delete", Arguments: map[string]any{"path": "test.go"}},
	}

	m, _ = m.Update(ToolCallMsg{Calls: calls})
	require.True(t, m.confirming.HasPending())

	m.Focus()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, m.confirming.HasPending())
}

func TestConfirmation_OtherKeysSwallowed(t *testing.T) {
	m, _ := newTestChatModel(t)

	calls := []ai.ToolCall{
		{ID: "call_del", Name: "file_delete", Arguments: map[string]any{"path": "test.go"}},
	}

	m, _ = m.Update(ToolCallMsg{Calls: calls})
	require.True(t, m.confirming.HasPending())

	m.Focus()
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})

	assert.True(t, m.confirming.HasPending(), "non y/n keys should not clear confirmation")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Safe tool calls execute immediately
// ---------------------------------------------------------------------------

func TestToolCall_SafeExecutesImmediately(t *testing.T) {
	m, _ := newTestChatModel(t)

	calls := []ai.ToolCall{
		{ID: "call_status", Name: "git_status", Arguments: nil},
	}

	m, cmd := m.Update(ToolCallMsg{Calls: calls})

	assert.False(t, m.confirming.HasPending(), "safe tool should not trigger confirmation")
	assert.NotNil(t, cmd, "should start execution for safe tool")
}

// ---------------------------------------------------------------------------
// Tests: Tool results continue multi-turn loop
// ---------------------------------------------------------------------------

func TestToolResults_ContinuesConversation(t *testing.T) {
	m, _ := newTestChatModel(t)

	// Add prior conversation context.
	m.messages = append(m.messages, ai.ChatMessage{
		Role:    "user",
		Content: "Show me the status",
	})

	results := []ToolResult{
		{ToolID: "call_1", Content: "2 modified files"},
	}

	m, cmd := m.Update(ToolResultMsg{Results: results})

	// Should add tool result to messages.
	expected := ai.SanitizeExternalContent("2 modified files")
	found := false
	for _, msg := range m.messages {
		if msg.Role == "tool" && msg.Content == expected {
			found = true
			break
		}
	}
	assert.True(t, found, "tool result should be added to messages")
	assert.True(t, m.streaming, "should start streaming again for next AI turn")
	assert.NotNil(t, cmd, "should return command to continue the loop")
}

func TestToolResults_ErrorContent(t *testing.T) {
	m, _ := newTestChatModel(t)

	results := []ToolResult{
		{ToolID: "call_1", Error: "permission denied"},
	}

	m, _ = m.Update(ToolResultMsg{Results: results})

	expected := ai.SanitizeExternalContent("Error: permission denied")
	found := false
	for _, msg := range m.messages {
		if msg.Role == "tool" && msg.Content == expected {
			found = true
			break
		}
	}
	assert.True(t, found, "error tool result should include error prefix")
}

// ---------------------------------------------------------------------------
// Tests: View rendering
// ---------------------------------------------------------------------------

func TestView_CollapsedShowsInput(t *testing.T) {
	m, _ := newTestChatModel(t)

	view := m.View()
	// The view should contain the textinput placeholder.
	assert.NotEmpty(t, view)
}

func TestView_StreamingShowsThinking(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.streaming = true
	m.expanded = true

	view := m.View()
	assert.Contains(t, view, "Thinking...")
}

func TestView_ShowsError(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.err = context.DeadlineExceeded
	m.expanded = true

	view := m.View()
	assert.Contains(t, view, "Error:")
}

func TestView_ShowsLastResponse(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.lastResponse = "The status shows 2 modified files."
	m.expanded = true

	view := m.View()
	assert.Contains(t, view, "2 modified files")
}

// ---------------------------------------------------------------------------
// Tests: SetSize
// ---------------------------------------------------------------------------

func TestSetSize(t *testing.T) {
	m, _ := newTestChatModel(t)

	m.SetSize(80, 24)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

// ---------------------------------------------------------------------------
// Tests: Scroll in expanded mode
// ---------------------------------------------------------------------------

func TestScroll_ExpandedUpDown(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.expanded = true
	// Set focused directly to avoid overlay mode affecting scroll behavior.
	m.focused = true
	m.input.Focus()

	// Build a multi-line response.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line")
	}
	m.lastResponse = strings.Join(lines, "\n")

	// Scroll up.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, m.scrollOffset)

	// Scroll down.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 0, m.scrollOffset)

	// Scroll down past zero should stay at zero.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 0, m.scrollOffset)
}

// ---------------------------------------------------------------------------
// Tests: Unfocused keys are ignored
// ---------------------------------------------------------------------------

func TestUnfocused_KeysIgnored(t *testing.T) {
	m, _ := newTestChatModel(t)
	assert.False(t, m.Focused())

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.Empty(t, m.messages, "unfocused keys should not trigger send")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: SendMessageCmd
// ---------------------------------------------------------------------------

func TestSendMessageCmd_AddsMessage(t *testing.T) {
	m, _ := newTestChatModel(t)

	m, cmd := m.Update(SendMessageCmd{Content: "programmatic message"})

	require.Len(t, m.messages, 1)
	assert.Equal(t, "programmatic message", m.messages[0].Content)
	assert.True(t, m.streaming)
	assert.NotNil(t, cmd)
}

// ---------------------------------------------------------------------------
// Tests: Close
// ---------------------------------------------------------------------------

func TestClose_DoesNotPanic(t *testing.T) {
	m, _ := newTestChatModel(t)
	assert.NotPanics(t, func() {
		m.Close()
	})
}

// ---------------------------------------------------------------------------
// Tests: Streaming prevents new sends
// ---------------------------------------------------------------------------

func TestStreaming_BlocksNewSend(t *testing.T) {
	m, _ := newTestChatModel(t)
	m.Focus()
	m.streaming = true
	m.input.SetValue("another message")

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.Empty(t, m.messages, "should not send while streaming")
	assert.Nil(t, cmd)
}
