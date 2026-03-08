package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newViewTestModel creates a minimal Model for view tests with optional
// theme colors populated.
func newViewTestModel(t *testing.T) Model {
	t.Helper()

	provider := newChatMockProvider()
	provider.response = ai.CompletionResponse{Content: "ok", FinishReason: "stop"}

	cfg := ai.NewRegistry(config.AIConfig{Provider: "mock"})
	cfg.Register("mock", provider)

	toolReg := NewToolRegistry()
	executor := &ToolExecutor{registry: toolReg}
	confirmer := NewConfirmationManager(toolReg)
	sysBuilder := NewSystemPromptBuilder(nil, "test")
	redactor := ai.NewRedactor(nil)

	th := &theme.Theme{
		Name:    "test",
		Variant: "dark",
		Colors: theme.Colors{
			BrightBlack:  "#555555",
			NormalCyan:   "#00FFFF",
			NormalYellow: "#FFFF00",
			NormalGreen:  "#00FF00",
			NormalRed:    "#FF0000",
		},
	}

	m := New(Deps{
		Registry:   cfg,
		Executor:   executor,
		Confirming: confirmer,
		SysPrompt:  sysBuilder,
		Redactor:   redactor,
		Theme:      th,
	})
	m.SetSize(80, 24)
	return m
}

// viewLines splits the rendered view into lines, trimming the trailing
// empty line that results from a final newline.
func viewLines(view string) []string {
	lines := strings.Split(view, "\n")
	// Remove trailing empty string from final "\n".
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ---------------------------------------------------------------------------
// Tests: Collapsed rendering
// ---------------------------------------------------------------------------

func TestCollapsed_ShowsFourLogicalSections(t *testing.T) {
	m := newViewTestModel(t)
	m.lastResponse = "The status shows 2 modified files."

	view := m.View()
	lines := viewLines(view)

	// Should have: response line + input line = 2 lines.
	require.Len(t, lines, 2, "collapsed view should produce 2 lines")
}

func TestCollapsed_ShowsLastResponseLine(t *testing.T) {
	m := newViewTestModel(t)
	m.lastResponse = "line one\nline two\nline three"

	view := m.View()
	assert.Contains(t, view, "line three",
		"collapsed should show the last non-empty response line")
	assert.NotContains(t, view, "line one",
		"collapsed should not show earlier lines")
}

func TestCollapsed_WrapsLongLine(t *testing.T) {
	m := newViewTestModel(t)
	m.SetSize(30, 24)
	m.lastResponse = "the quick brown fox jumps over the lazy dog and then more text"

	view := m.View()
	// Long line is word-wrapped; the tail end should be visible.
	assert.Contains(t, view, "more text",
		"wrapped long line should show its tail")
}

func TestCollapsed_EmptyResponse(t *testing.T) {
	m := newViewTestModel(t)
	// No lastResponse, not streaming, no error.

	view := m.View()
	lines := viewLines(view)

	// Should have: input line = 1 line (no separator, no status, no response section).
	require.Len(t, lines, 1, "empty state should have just the input line")
}

func TestCollapsed_SkipsTrailingBlankLines(t *testing.T) {
	m := newViewTestModel(t)
	m.lastResponse = "content here\n\n\n"

	view := m.View()
	assert.Contains(t, view, "content here",
		"should find the last non-empty line")
}

// ---------------------------------------------------------------------------
// Tests: Expanded rendering
// ---------------------------------------------------------------------------

func TestExpanded_ShowsMultipleResponseLines(t *testing.T) {
	m := newViewTestModel(t)
	m.expanded = true

	// Build a response with enough lines to fill the expanded view.
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("response line %d", i))
	}
	m.lastResponse = strings.Join(lines, "\n")

	view := m.View()

	// The expanded area should show expandedHeight - collapsedHeight lines.
	visibleLines := expandedHeight - collapsedHeight
	for i := 30; i > 30-visibleLines; i-- {
		expected := fmt.Sprintf("response line %d", i)
		assert.Contains(t, view, expected,
			"expanded view should show line %d", i)
	}
}

func TestExpanded_LineCount(t *testing.T) {
	m := newViewTestModel(t)
	m.expanded = true

	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line")
	}
	m.lastResponse = strings.Join(lines, "\n")

	view := m.View()
	rendered := viewLines(view)

	// visible response lines + input(1)
	visibleResponse := expandedHeight - collapsedHeight
	expected := visibleResponse + 1
	assert.Equal(t, expected, len(rendered),
		"expanded view should have %d lines", expected)
}

// ---------------------------------------------------------------------------
// Tests: Scroll offset in expanded mode
// ---------------------------------------------------------------------------

func TestExpanded_ScrollOffset(t *testing.T) {
	m := newViewTestModel(t)
	m.expanded = true
	m.scrollOffset = 5

	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	m.lastResponse = strings.Join(lines, "\n")

	view := m.View()

	// With scroll offset 5, the bottom of the visible window should NOT
	// include the very last lines.
	assert.NotContains(t, view, "line-50",
		"scrolled-up view should not show the last line")
	assert.NotContains(t, view, "line-49",
		"scrolled-up view should not show the second-to-last line")
}

func TestExpanded_ScrollOffsetClamped(t *testing.T) {
	m := newViewTestModel(t)
	m.expanded = true
	m.scrollOffset = 9999 // absurdly high

	m.lastResponse = "short response"

	// Should not panic; offset is clamped internally.
	view := m.View()
	assert.Contains(t, view, "short response")
}

// ---------------------------------------------------------------------------
// Tests: Streaming indicator
// ---------------------------------------------------------------------------

func TestStreaming_ShowsThinking(t *testing.T) {
	m := newViewTestModel(t)
	m.streaming = true

	view := m.View()
	assert.Contains(t, view, "Thinking...",
		"streaming with empty buffer should show Thinking...")

	// Should contain one of the spinner frames.
	found := false
	for _, frame := range spinnerFrames {
		if strings.Contains(view, frame) {
			found = true
			break
		}
	}
	assert.True(t, found, "streaming should show a spinner frame")
}

func TestStreaming_ShowsPartialResponse(t *testing.T) {
	m := newViewTestModel(t)
	m.streaming = true
	m.streamBuf.WriteString("partial AI response")

	view := m.View()
	assert.Contains(t, view, "partial AI response",
		"streaming should show accumulated text")
}

func TestStreaming_CollapsedShowsLastLine(t *testing.T) {
	m := newViewTestModel(t)
	m.streaming = true
	m.streamBuf.WriteString("first line\nsecond line\nthird line")

	view := m.View()
	assert.Contains(t, view, "third line",
		"collapsed streaming should show last line")
}

// ---------------------------------------------------------------------------
// Tests: Confirmation rendering
// ---------------------------------------------------------------------------

func TestConfirmation_ShowsWarningAndDescription(t *testing.T) {
	m := newViewTestModel(t)

	// Trigger a destructive tool call.
	m.confirming.Check(ai.ToolCall{
		ID:        "call_del",
		Name:      "file_delete",
		Arguments: map[string]any{"path": "important.go"},
	})

	view := m.View()
	assert.Contains(t, view, "[y/N]",
		"confirmation should show [y/N] prompt")
	assert.Contains(t, view, warningIndicator,
		"confirmation should show warning indicator")
	assert.Contains(t, view, "important.go",
		"confirmation should include the file name")
}

func TestConfirmation_NoPending(t *testing.T) {
	m := newViewTestModel(t)

	result := m.renderConfirmation()
	assert.Empty(t, result,
		"renderConfirmation with no pending should return empty")
}

// ---------------------------------------------------------------------------
// Tests: Error rendering
// ---------------------------------------------------------------------------

func TestError_ShowsErrorMessage(t *testing.T) {
	m := newViewTestModel(t)
	m.err = context.DeadlineExceeded

	view := m.View()
	assert.Contains(t, view, "Error:",
		"error view should contain Error: prefix")
	assert.Contains(t, view, "deadline exceeded",
		"error view should contain the error text")
}

func TestError_TakesPriorityOverResponse(t *testing.T) {
	m := newViewTestModel(t)
	m.err = fmt.Errorf("connection failed")
	m.lastResponse = "old response that should not appear"

	view := m.View()
	assert.Contains(t, view, "connection failed")
	assert.NotContains(t, view, "old response",
		"error should take priority over last response")
}

// ---------------------------------------------------------------------------
// Tests: Input rendering
// ---------------------------------------------------------------------------

func TestRenderInput_ContainsPromptPrefix(t *testing.T) {
	m := newViewTestModel(t)

	input := m.renderInput()
	assert.Contains(t, input, promptPrefix,
		"input should contain the chat prompt prefix")
}

// ---------------------------------------------------------------------------
// Tests: Empty state (no messages, no response)
// ---------------------------------------------------------------------------

func TestEmptyState_RendersWithoutPanic(t *testing.T) {
	m := newViewTestModel(t)

	assert.NotPanics(t, func() {
		view := m.View()
		assert.NotEmpty(t, view, "empty state should still render separator + input")
	})
}

func TestEmptyState_HasInput(t *testing.T) {
	m := newViewTestModel(t)

	view := m.View()
	assert.Contains(t, view, promptPrefix,
		"empty state should have the prompt")
}

// ---------------------------------------------------------------------------
// Tests: Helper functions
// ---------------------------------------------------------------------------

func TestLastNonEmptyLine_Normal(t *testing.T) {
	lines := []string{"first", "second", "third"}
	assert.Equal(t, "third", lastNonEmptyLine(lines))
}

func TestLastNonEmptyLine_TrailingBlanks(t *testing.T) {
	lines := []string{"first", "second", "", "  ", ""}
	assert.Equal(t, "second", lastNonEmptyLine(lines))
}

func TestLastNonEmptyLine_AllEmpty(t *testing.T) {
	lines := []string{"", "  ", ""}
	assert.Equal(t, "", lastNonEmptyLine(lines))
}

func TestLastNonEmptyLine_SingleLine(t *testing.T) {
	assert.Equal(t, "only", lastNonEmptyLine([]string{"only"}))
}

// ---------------------------------------------------------------------------
// Tests: Effective width
// ---------------------------------------------------------------------------

func TestEffectiveWidth_UsesModelWidth(t *testing.T) {
	m := newViewTestModel(t)
	m.SetSize(120, 24)
	assert.Equal(t, 120, m.effectiveWidth())
}

func TestEffectiveWidth_DefaultsTo80(t *testing.T) {
	m := newViewTestModel(t)
	m.width = 0
	assert.Equal(t, 80, m.effectiveWidth())
}

// ---------------------------------------------------------------------------
// Tests: Theme color fallbacks
// ---------------------------------------------------------------------------

func TestView_NilTheme_DoesNotPanic(t *testing.T) {
	m := newViewTestModel(t)
	m.theme = nil
	m.lastResponse = "some response"

	assert.NotPanics(t, func() {
		view := m.View()
		assert.NotEmpty(t, view)
	})
}

func TestView_EmptyThemeColors_UsesFallbacks(t *testing.T) {
	m := newViewTestModel(t)
	m.theme = &theme.Theme{
		Name:    "empty",
		Variant: "dark",
		Colors:  theme.Colors{}, // all empty strings
	}
	m.lastResponse = "test response"

	assert.NotPanics(t, func() {
		view := m.View()
		assert.Contains(t, view, "test response")
	})
}

// ---------------------------------------------------------------------------
// Tests: Spinner frame cycling (Change 1)
// ---------------------------------------------------------------------------

func TestSpinnerFrames_Cycling(t *testing.T) {
	m := newViewTestModel(t)
	m.streaming = true

	// Frame 0 → first spinner char.
	view0 := m.renderStreaming()
	assert.Contains(t, view0, spinnerFrames[0])

	// Frame 1 → second spinner char.
	m.spinnerFrame = 1
	view1 := m.renderStreaming()
	assert.Contains(t, view1, spinnerFrames[1])

	// Frame wraps around.
	m.spinnerFrame = len(spinnerFrames) + 2
	viewWrapped := m.renderStreaming()
	assert.Contains(t, viewWrapped, spinnerFrames[2])
}

func TestStatusLabel_NoRegistry(t *testing.T) {
	m := newViewTestModel(t)
	m.registry = nil
	m.status = "Ready"
	assert.Equal(t, "Ready", m.statusLabel())
}

func TestStatusLabel_WithProvider(t *testing.T) {
	m := newViewTestModel(t)
	m.status = "Streaming..."
	label := m.statusLabel()
	assert.Contains(t, label, "mock · Streaming...")
}

func TestStatus_NotInCollapsedView(t *testing.T) {
	m := newViewTestModel(t)
	view := m.View()
	assert.NotContains(t, view, "Ready", "collapsed view should not contain the status")
}

// ---------------------------------------------------------------------------
// Tests: Text wrapping (Change 4)
// ---------------------------------------------------------------------------

func TestWrapText_ShortLine(t *testing.T) {
	result := wrapText("hello world", 80)
	assert.Equal(t, "hello world", result)
}

func TestWrapText_WrapsAtWordBoundary(t *testing.T) {
	result := wrapText("the quick brown fox jumps over", 15)
	lines := strings.Split(result, "\n")
	require.True(t, len(lines) >= 2, "should wrap into multiple lines")
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 15,
			"each wrapped line should fit within width")
	}
}

func TestWrapText_PreservesNewlines(t *testing.T) {
	result := wrapText("line one\nline two", 80)
	assert.Equal(t, "line one\nline two", result)
}

func TestWrapText_Empty(t *testing.T) {
	assert.Equal(t, "", wrapText("", 80))
}

func TestWrapText_ZeroWidth(t *testing.T) {
	assert.Equal(t, "hello", wrapText("hello", 0))
}

func TestWrapLine_SingleWord(t *testing.T) {
	// Single word longer than width should not be broken.
	result := wrapLine("superlongword", 5)
	assert.Equal(t, "superlongword", result,
		"single word should not be broken")
}

func TestWrapLine_MultipleWords(t *testing.T) {
	result := wrapLine("hello beautiful world", 16)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "hello beautiful", lines[0])
	assert.Equal(t, "world", lines[1])
}

// ---------------------------------------------------------------------------
// Tests: Overlay / modal rendering (Change 5)
// ---------------------------------------------------------------------------

func TestOverlayMode_ViewReturnsFooter(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)

	view := m.View()
	// In overlay mode, View() returns just the input line.
	assert.Contains(t, view, promptPrefix, "footer should have input prompt")
	assert.NotContains(t, view, "Ready", "footer should not show status")
}

func TestRenderModalContent_RendersMessages(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)
	m.messages = append(m.messages,
		ai.ChatMessage{Role: "user", Content: "what files changed?"},
		ai.ChatMessage{Role: "assistant", Content: "Two files were modified."},
	)

	content := m.RenderModalContent(60, 30)
	assert.Contains(t, content, "You:")
	assert.Contains(t, content, "what files changed?")
	assert.Contains(t, content, "AI:")
	assert.Contains(t, content, "Two files were modified.")
}

func TestRenderModalContent_RendersToolMessages(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)
	m.messages = append(m.messages,
		ai.ChatMessage{Role: "tool", Content: "git status output"},
	)

	content := m.RenderModalContent(60, 30)
	assert.Contains(t, content, "Tool:")
	assert.Contains(t, content, "git status output")
}

func TestRenderModalContent_ShowsStatusLabel(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)
	m.status = "Streaming..."

	content := m.RenderModalContent(60, 30)
	assert.Contains(t, content, "Streaming...")
}

func TestRenderModalContent_ShowsInput(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)

	content := m.RenderModalContent(60, 30)
	assert.Contains(t, content, promptPrefix, "modal should have input prompt")
}

func TestRenderModalContent_EmptyHistory(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.SetSize(80, 40)

	assert.NotPanics(t, func() {
		content := m.RenderModalContent(60, 30)
		assert.NotEmpty(t, content)
	})
}

func TestRenderModalContent_StreamingShowsSpinner(t *testing.T) {
	m := newViewTestModel(t)
	m.overlayMode = true
	m.expanded = true
	m.streaming = true
	m.SetSize(80, 40)

	content := m.RenderModalContent(60, 30)
	assert.Contains(t, content, "Thinking...")
}

// ---------------------------------------------------------------------------
// Tests: renderSeparatorWithLabel
// ---------------------------------------------------------------------------

func TestRenderSeparatorWithLabel(t *testing.T) {
	m := newViewTestModel(t)
	m.SetSize(40, 24)

	result := m.renderSeparatorWithLabel("Chat")
	assert.Contains(t, result, "Chat")
	assert.Contains(t, result, separatorChar)
}

func TestRenderSeparatorWithLabel_NarrowWidth(t *testing.T) {
	m := newViewTestModel(t)
	m.SetSize(10, 24)

	assert.NotPanics(t, func() {
		result := m.renderSeparatorWithLabel("Some Long Label")
		assert.Contains(t, result, "Some Long Label")
	})
}
