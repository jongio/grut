package chat

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/theme"
)

// collapsedHeight is the number of terminal rows the chat occupies when
// collapsed (input line only).
const collapsedHeight = 1

// expandedHeight is the number of terminal rows the chat occupies when
// expanded (input + scrollable response area).
const expandedHeight = 20

// maxStreamResponseSize is the maximum size (in bytes) of an accumulated
// streaming AI response. This prevents OOM from a malicious or compromised
// AI provider streaming an enormous response (CWE-400).
const (
	maxStreamResponseSize = 1 * 1024 * 1024 // 1 MiB
	// maxChatMessages caps the conversation history length. Tool results can be
	// up to 10 MiB each, so unbounded growth risks OOM (CWE-400). When exceeded,
	// the oldest non-system messages are discarded, keeping the most recent half.
	maxChatMessages = 200
)

// ---------------------------------------------------------------------------
// Custom message types
// ---------------------------------------------------------------------------
// StreamChunkMsg delivers an incremental piece of a streaming AI response.
type StreamChunkMsg struct{ Chunk ai.StreamChunk }

// ToolCallMsg signals that the AI response contains tool invocations.
type ToolCallMsg struct{ Calls []ai.ToolCall }

// ToolResultMsg delivers results from executed tool calls back to the model.
type ToolResultMsg struct{ Results []ToolResult }

// SendMessageCmd is an external command to programmatically send a chat
// message, as if the user typed it and pressed Enter.
type SendMessageCmd struct{ Content string }

// streamDoneMsg is an internal signal that the streaming goroutine finished.
type streamDoneMsg struct {
	err      error
	response string
	tools    []ai.ToolCall
}

// spinnerTickMsg drives the animated spinner during streaming.
type spinnerTickMsg struct{}

// toolExecDoneMsg is an internal signal that tool execution completed.
type toolExecDoneMsg struct {
	results []ToolResult
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------
// Model is the Bubble Tea model for the chat footer. It manages user input,
// streams AI responses, orchestrates multi-turn tool calling, and handles
// destructive-operation confirmations.
type Model struct {
	ctx          context.Context
	err          error                // Last error for display
	registry     *ai.Registry         // AI provider registry
	tools        *ToolRegistry        // Available chat tools
	executor     *ToolExecutor        // Executes tool calls
	confirming   *ConfirmationManager // Destructive op confirmation
	sysPrompt    *SystemPromptBuilder // Context-aware system prompt
	redactor     *ai.Redactor         // Content redaction
	audit        *ai.AuditLogger      // Audit logging
	theme        *theme.Theme
	cancel       context.CancelFunc
	streamCancel context.CancelFunc // Cancel in-flight stream
	history      InputHistory       // Terminal-style input history
	lastResponse string             // Last complete AI response for display
	status       string             // Current status label (Ready, Streaming…, etc.)
	streamBuf    strings.Builder    // Accumulated streamed text
	messages     []ai.ChatMessage   // Conversation history
	input        textinput.Model    // Text input widget
	scrollOffset int                // Response scroll position (lines from bottom)
	width        int                // Available width
	height       int                // Available height (full terminal height)
	spinnerFrame int                // Animated spinner frame index
	streaming    bool               // Currently receiving streamed response
	focused      bool               // Whether chat has keyboard focus
	expanded     bool               // Expanded vs collapsed view
	overlayMode  bool               // Full-screen conversation overlay when focused
	renderMD     bool               // Render AI responses as formatted markdown
}

// Deps bundles the required dependencies for creating a chat Model,
// keeping the constructor signature short and readable.
type Deps struct {
	Registry   *ai.Registry
	Executor   *ToolExecutor
	Confirming *ConfirmationManager
	SysPrompt  *SystemPromptBuilder
	Redactor   *ai.Redactor
	Audit      *ai.AuditLogger
	Theme      *theme.Theme
	ChatCfg    config.ChatConfig
}

// New creates a new chat Model with all required dependencies.
func New(d Deps) Model {
	ti := textinput.New()
	ti.Placeholder = "Ask grut anything..."
	ti.Prompt = ""
	if c := ti.Cursor(); c != nil {
		c.Blink = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		input:      ti,
		messages:   make([]ai.ChatMessage, 0, 16),
		registry:   d.Registry,
		tools:      d.Executor.registry,
		executor:   d.Executor,
		confirming: d.Confirming,
		sysPrompt:  d.SysPrompt,
		redactor:   d.Redactor,
		audit:      d.Audit,
		theme:      d.Theme,
		ctx:        ctx,
		cancel:     cancel,
		status:     "Ready",
		renderMD:   d.ChatCfg.RenderMarkdown,
	}
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------
// Init implements tea.Model. No initial command is needed.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and returns the updated model and any commands.
// This model is designed to be embedded within the app model — it does not
// implement tea.Model directly. The parent model calls Update and View.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinnerTickMsg:
		if m.streaming {
			m.spinnerFrame++
			return m, tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
		}
		return m, nil
	case StreamChunkMsg:
		return m.handleStreamChunk(msg)
	case streamDoneMsg:
		return m.handleStreamDone(msg)
	case ToolCallMsg:
		return m.handleToolCalls(msg)
	case toolExecDoneMsg:
		return m.handleToolExecDone(msg)
	case ToolResultMsg:
		return m.handleToolResults(msg)
	case SendMessageCmd:
		return m.sendMessage(msg.Content)
	}
	// Pass unhandled messages to the textinput when focused.
	if m.focused {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model. Returns the chat UI as a string.
// Rendering is delegated to renderView() in view.go.
func (m Model) View() string {
	return m.renderView()
}

// ---------------------------------------------------------------------------
// Focus management
// ---------------------------------------------------------------------------
// Focused reports whether the chat has keyboard focus.
func (m Model) Focused() bool {
	return m.focused
}

// Focus gives keyboard focus to the chat input and enables overlay mode.
func (m *Model) Focus() {
	m.focused = true
	m.overlayMode = true
	m.expanded = true
	m.input.Focus()
}

// Blur removes keyboard focus from the chat input and disables overlay mode.
func (m *Model) Blur() {
	m.focused = false
	m.overlayMode = false
	m.expanded = false
	m.input.Blur()
}

// Height returns the current height of the chat model in terminal rows.
// In overlay mode the modal is rendered separately by the parent (app.go),
// so the chat footer stays at its collapsed height.
func (m Model) Height() int {
	if m.overlayMode {
		return collapsedHeight
	}
	if m.expanded {
		return expandedHeight
	}
	return collapsedHeight
}

// SetSize updates the available dimensions for the chat model.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.input.SetWidth(width - 4) // Account for prompt and padding
}

// ClearHistory removes all conversation history and resets the display.
func (m *Model) ClearHistory() {
	m.messages = m.messages[:0]
	m.lastResponse = ""
	m.streamBuf.Reset()
	m.scrollOffset = 0
	m.err = nil
	m.status = "Ready" //nolint:goconst // inline status string
}

// Close cancels any in-flight operations and releases resources.
func (m *Model) Close() {
	if m.streamCancel != nil {
		m.streamCancel()
	}
	m.cancel()
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	key := msg.String()
	// Confirmation mode: only accept y/n.
	if m.confirming.HasPending() {
		switch key {
		case "y", "Y":
			return m.acceptConfirmation()
		case "n", "N", "escape", "esc":
			return m.rejectConfirmation()
		}
		// Swallow all other keys during confirmation.
		return m, nil
	}
	switch key {
	case "enter":
		content := strings.TrimSpace(m.input.Value())
		if content == "" || m.streaming {
			return m, nil
		}
		m.history.Push(content)
		m.history.Reset()
		m.input.SetValue("")
		return m.sendMessage(content)
	case "escape", "esc":
		if m.streaming && m.streamCancel != nil {
			m.streamCancel()
			m.streaming = false
			m.lastResponse = m.streamBuf.String()
			m.streamBuf.Reset()
			return m, nil
		}
		m.Blur()
		return m, nil
	case "ctrl+e":
		m.expanded = !m.expanded
		m.scrollOffset = 0
		return m, nil
	case "ctrl+l":
		m.ClearHistory()
		return m, nil
	case "up":
		if m.expanded {
			m.scrollOffset++
			return m, nil
		}
		if text, ok := m.history.Up(m.input.Value()); ok {
			m.input.SetValue(text)
			m.input.CursorEnd()
		}
		return m, nil
	case "down":
		if m.expanded {
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		}
		if text, ok := m.history.Down(); ok {
			m.input.SetValue(text)
			m.input.CursorEnd()
		}
		return m, nil
	case "pgup":
		if m.expanded {
			m.scrollOffset += 10
			return m, nil
		}
	case "pgdown":
		if m.expanded {
			m.scrollOffset = max(0, m.scrollOffset-10)
			return m, nil
		}
	}
	// Pass to textinput.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// ---------------------------------------------------------------------------
// Message sending and streaming
// ---------------------------------------------------------------------------
// trimMessages enforces the maxChatMessages cap by discarding the oldest
// messages and retaining the most recent half. This prevents unbounded
// memory growth from long-running chat sessions (CWE-400).
func trimMessages(msgs []ai.ChatMessage) []ai.ChatMessage {
	if len(msgs) <= maxChatMessages {
		return msgs
	}
	keep := maxChatMessages / 2
	trimmed := make([]ai.ChatMessage, keep)
	copy(trimmed, msgs[len(msgs)-keep:])
	return trimmed
}

func (m Model) sendMessage(content string) (Model, tea.Cmd) {
	// Redact user content before storing.
	redacted, _ := m.redactor.RedactContent(content)
	m.messages = append(m.messages, ai.ChatMessage{
		Role:    "user",
		Content: redacted,
	})
	m.messages = trimMessages(m.messages)
	m.streaming = true
	m.streamBuf.Reset()
	m.err = nil
	m.scrollOffset = 0
	m.status = "Connecting..."
	// Create a cancellable context for this stream so users can abort via
	// Escape. Previously the cancel func was assigned to _ inside the Cmd
	// closure, making in-flight stream cancellation impossible.
	streamCtx, streamCancel := context.WithCancel(m.ctx)
	m.streamCancel = streamCancel
	return m, tea.Batch(
		m.startStreamCmd(streamCtx),
		tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} }),
	)
}

// startStreamCmd returns a tea.Cmd that initiates streaming from the AI
// provider. It reads all chunks and delivers a single streamDoneMsg when
// finished, emitting StreamChunkMsg for each incremental chunk.
func (m Model) startStreamCmd(streamCtx context.Context) tea.Cmd {
	reg := m.registry
	sysPrompt := m.sysPrompt
	msgs := make([]ai.ChatMessage, len(m.messages))
	copy(msgs, m.messages)
	toolDefs := m.tools.Definitions()
	return func() tea.Msg {
		ctx := streamCtx
		provider, err := reg.Get(ctx)
		if err != nil {
			return streamDoneMsg{err: err}
		}
		prompt := sysPrompt.Build(ctx)
		req := ai.CompletionRequest{
			Operation:    "chat",
			SystemPrompt: prompt,
			Messages:     msgs,
			Tools:        toolDefs,
			Temperature:  0.7,
		}
		ch, err := provider.CompleteStream(ctx, req)
		if err != nil {
			return streamDoneMsg{err: err}
		}
		var buf strings.Builder
		var toolCalls []ai.ToolCall
		for chunk := range ch {
			if chunk.Err != nil {
				return streamDoneMsg{
					response: buf.String(),
					err:      chunk.Err,
				}
			}
			if chunk.Delta != "" {
				// Cap accumulated response size to prevent OOM from a
				// malicious AI provider streaming unbounded data (CWE-400).
				if buf.Len()+len(chunk.Delta) <= maxStreamResponseSize {
					buf.WriteString(chunk.Delta)
				}
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
		}
		return streamDoneMsg{
			response: buf.String(),
			tools:    toolCalls,
		}
	}
}

func (m Model) handleStreamChunk(msg StreamChunkMsg) (Model, tea.Cmd) {
	if msg.Chunk.Delta != "" {
		// Cap accumulated response size to prevent OOM from unbounded
		// streaming responses (CWE-400).
		if m.streamBuf.Len()+len(msg.Chunk.Delta) <= maxStreamResponseSize {
			m.streamBuf.WriteString(msg.Chunk.Delta)
		}
		m.status = "Streaming..." //nolint:goconst // inline status string
	}
	return m, nil
}

func (m Model) handleStreamDone(msg streamDoneMsg) (Model, tea.Cmd) {
	m.streaming = false
	m.spinnerFrame = 0
	if msg.err != nil {
		m.err = msg.err
		m.lastResponse = m.streamBuf.String()
		m.streamBuf.Reset()
		m.status = "Error"
		return m, nil
	}
	m.lastResponse = msg.response
	m.streamBuf.Reset()
	m.status = "Ready"
	// Add the assistant response to conversation history.
	m.messages = append(m.messages, ai.ChatMessage{
		Role:      "assistant",
		Content:   msg.response,
		ToolCalls: msg.tools,
	})
	m.messages = trimMessages(m.messages)
	// Log the interaction.
	if m.audit != nil {
		_ = m.audit.Log(ai.AuditEntry{
			Operation: "chat",
			Result:    "accepted",
		})
	}
	// If the AI requested tool calls, process them.
	if len(msg.tools) > 0 {
		return m, func() tea.Msg {
			return ToolCallMsg{Calls: msg.tools}
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Tool calling
// ---------------------------------------------------------------------------
func (m Model) handleToolCalls(msg ToolCallMsg) (Model, tea.Cmd) {
	// Process tool calls sequentially. For each call, check safety first.
	var safeCalls []ai.ToolCall
	for _, call := range msg.Calls {
		pc, safe := m.confirming.Check(call)
		if safe {
			// Safe tool: queue for immediate execution.
			safeCalls = append(safeCalls, call)
		} else if pc != nil {
			// Destructive tool: enter confirmation mode.
			// Only one confirmation at a time; remaining calls are deferred.
			return m, nil // View will show the confirmation prompt
		}
		// Unknown tool (pc == nil, safe == false): skip silently.
	}
	if len(safeCalls) == 0 {
		return m, nil
	}
	// Execute all safe calls.
	m.status = "Running tools..."
	return m, m.executeToolsCmd(safeCalls)
}

func (m Model) executeToolsCmd(calls []ai.ToolCall) tea.Cmd {
	executor := m.executor
	ctx := m.ctx
	return func() tea.Msg {
		results := make([]ToolResult, 0, len(calls))
		for _, call := range calls {
			result := executor.Execute(ctx, call)
			results = append(results, result)
		}
		return toolExecDoneMsg{results: results}
	}
}

func (m Model) handleToolExecDone(msg toolExecDoneMsg) (Model, tea.Cmd) {
	return m, func() tea.Msg {
		return ToolResultMsg{Results: msg.results}
	}
}

func (m Model) handleToolResults(msg ToolResultMsg) (Model, tea.Cmd) {
	// Add tool results to conversation history.
	// Tool results contain repository-sourced content (file contents, git output)
	// which could carry prompt injection payloads, so we sanitize them.
	for _, result := range msg.Results {
		content := result.Content
		if result.Error != "" {
			content = "Error: " + result.Error
		}
		content = ai.SanitizeExternalContent(content)
		m.messages = append(m.messages, ai.ChatMessage{
			Role:    "tool",
			Content: content,
			ToolID:  result.ToolID,
		})
	}
	m.messages = trimMessages(m.messages)
	m.streaming = true
	m.streamBuf.Reset()
	m.status = "Connecting..."
	// Create a new cancellable context for the continuation stream.
	streamCtx, streamCancel := context.WithCancel(m.ctx)
	m.streamCancel = streamCancel
	return m, tea.Batch(
		m.startStreamCmd(streamCtx),
		tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} }),
	)
}

// ---------------------------------------------------------------------------
// Confirmation handling
// ---------------------------------------------------------------------------
func (m Model) acceptConfirmation() (Model, tea.Cmd) {
	call := m.confirming.Accept()
	if call == nil {
		return m, nil
	}
	m.status = "Running tools..."
	return m, m.executeToolsCmd([]ai.ToolCall{*call})
}

func (m Model) rejectConfirmation() (Model, tea.Cmd) {
	desc := m.confirming.Reject()
	if desc == "" {
		return m, nil
	}
	// Add a tool result indicating rejection.
	m.messages = append(m.messages, ai.ChatMessage{
		Role:    "tool",
		Content: "User rejected: " + desc,
	})
	m.messages = trimMessages(m.messages)
	m.lastResponse = "Cancelled: " + desc
	return m, nil
}
