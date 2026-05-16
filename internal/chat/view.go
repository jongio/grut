package chat

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/markdown"
)

// ---------------------------------------------------------------------------
// Rendering constants
// ---------------------------------------------------------------------------

const (
	separatorChar    = "─"
	promptPrefix     = " \uF075 › " // nf-fa-comment, padded for centering
	warningIndicator = "\uF071 "    // nf-fa-warning
	errorIndicator   = "\uEA87 "    // nf-cod-error
)

// spinnerFrames are the Braille-pattern characters cycled to produce
// an animated spinner while streaming.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Default color fallbacks used when theme colors are empty.
const (
	defaultDimColor     = "#666666"
	defaultAccentColor  = "#8BE9FD"
	defaultWarningColor = "#FFB86C"
	defaultPromptColor  = "#50FA7B"
	defaultErrorColor   = "#FF5555"
)

// ---------------------------------------------------------------------------
// Top-level renderer
// ---------------------------------------------------------------------------

// renderView composes the complete chat UI. In overlay mode the full
// conversation is rendered separately as a modal by app.go, so View()
// returns a compact footer. Otherwise it renders the normal collapsed
// or expanded footer.
func (m Model) renderView() string {
	if m.overlayMode {
		return m.renderInput()
	}

	var b strings.Builder

	// Priority (highest first): confirmation > error > streaming > response.
	switch {
	case m.confirming != nil && m.confirming.HasPending():
		b.WriteString(m.renderConfirmation())
	case m.err != nil:
		b.WriteString(m.renderError())
	case m.streaming:
		b.WriteString(m.renderStreaming())
	default:
		b.WriteString(m.renderResponse())
	}

	b.WriteString(m.renderInput())
	return b.String()
}

// ---------------------------------------------------------------------------
// Floating modal content (rendered by app.go)
// ---------------------------------------------------------------------------

// RenderModalContent renders the inner content of the floating chat modal
// dialog. The caller (app.go) applies the border, title, and positioning.
// width and height are the inner dimensions available after the border is
// subtracted. This method uses a value receiver so local width/input
// adjustments do not persist.
func (m Model) RenderModalContent(width, height int) string {
	// Override model dimensions for modal-scoped rendering.
	m.width = width
	m.input.SetWidth(width - 4) // account for prompt and padding

	// Reserve lines: status separator (1) + input (1) + optional extra.
	extraLines := 0
	if m.err != nil || (m.confirming != nil && m.confirming.HasPending()) {
		extraLines = 1
	}
	msgHeight := height - 2 - extraLines
	if msgHeight < 1 {
		msgHeight = 1
	}

	var b strings.Builder

	b.WriteString(m.renderMessageHistory(msgHeight, width))

	// Separator with embedded status label.
	b.WriteString(m.renderSeparatorWithLabel(m.statusLabel()))
	b.WriteByte('\n')

	if m.confirming != nil && m.confirming.HasPending() {
		b.WriteString(m.renderConfirmation())
	} else if m.err != nil {
		b.WriteString(m.renderError())
	}

	b.WriteString(m.renderInput())
	return b.String()
}

// renderMessageHistory renders a scrollable window of conversation messages
// for the overlay mode.
func (m Model) renderMessageHistory(height, width int) string {
	contentWidth := width - 4 // 2-char left + 2-char right padding
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Resolve theme colors.
	greenColor := defaultPromptColor
	if m.theme != nil && m.theme.Colors.NormalGreen != "" {
		greenColor = m.theme.Colors.NormalGreen
	}
	cyanColor := defaultAccentColor
	if m.theme != nil && m.theme.Colors.NormalCyan != "" {
		cyanColor = m.theme.Colors.NormalCyan
	}
	dimColor := defaultDimColor
	if m.theme != nil && m.theme.Colors.BrightBlack != "" {
		dimColor = m.theme.Colors.BrightBlack
	}

	var allLines []string

	for _, msg := range m.messages {
		var prefix, color string
		switch msg.Role {
		case RoleUser:
			prefix = "You: "
			color = greenColor
		case RoleAssistant:
			prefix = "AI: "
			color = cyanColor
		case RoleTool:
			prefix = "Tool: "
			color = dimColor
		default:
			continue
		}

		styledPrefix := lipgloss.NewStyle().
			Foreground(lipgloss.Color(color)).
			Render(prefix)

		content := msg.Content
		if content == "" {
			content = "(empty)"
		}

		// Render assistant markdown when enabled.
		if m.renderMD && msg.Role == RoleAssistant {
			rendered := markdown.RenderStatic(content, contentWidth-2)
			for i, line := range rendered {
				if i == 0 {
					allLines = append(allLines, "  "+styledPrefix+line)
				} else {
					allLines = append(allLines, "  "+strings.Repeat(" ", len([]rune(prefix)))+line)
				}
			}
			allLines = append(allLines, "") // blank line between messages
			continue
		}

		prefixWidth := len([]rune(prefix))
		wrapWidth := contentWidth - prefixWidth
		if wrapWidth < 5 {
			wrapWidth = 5
		}

		wrapped := wrapText(content, wrapWidth)
		lines := strings.Split(wrapped, "\n")
		pad := strings.Repeat(" ", prefixWidth)

		for i, line := range lines {
			if i == 0 {
				allLines = append(allLines, "  "+styledPrefix+line)
			} else {
				allLines = append(allLines, "  "+pad+line)
			}
		}
		allLines = append(allLines, "") // blank line between messages
	}

	// Append in-progress streaming content.
	if m.streaming {
		frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		spinnerPrefix := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cyanColor)).
			Render(frame + " ")

		partial := m.streamBuf.String()
		if partial == "" {
			allLines = append(allLines, "  "+spinnerPrefix+"Thinking...")
		} else {
			wrapped := wrapText(partial, contentWidth-4)
			for i, line := range strings.Split(wrapped, "\n") {
				if i == 0 {
					allLines = append(allLines, "  "+spinnerPrefix+line)
				} else {
					allLines = append(allLines, "    "+line)
				}
			}
		}
	}

	// Scroll window (bottom-anchored).
	totalLines := len(allLines)

	maxScroll := totalLines - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	offset := m.scrollOffset
	if offset > maxScroll {
		offset = maxScroll
	}

	start := totalLines - height - offset
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > totalLines {
		end = totalLines
	}

	var b strings.Builder
	linesWritten := 0
	for _, line := range allLines[start:end] {
		b.WriteString(line)
		b.WriteByte('\n')
		linesWritten++
	}

	// Pad remaining height with empty lines.
	for i := linesWritten; i < height; i++ {
		b.WriteByte('\n')
	}

	return b.String()
}

// renderSeparatorWithLabel renders a separator line with a label embedded:
//
//	── label ──────────────────
func (m Model) renderSeparatorWithLabel(label string) string {
	w := m.effectiveWidth()

	dimColor := defaultDimColor
	if m.theme != nil && m.theme.Colors.BrightBlack != "" {
		dimColor = m.theme.Colors.BrightBlack
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color(dimColor))

	prefix := separatorChar + separatorChar + " "
	suffix := " "
	contentLen := 3 + len([]rune(label)) + 1 // "── " + label + " "

	remaining := w - contentLen
	if remaining < 0 {
		remaining = 0
	}

	return style.Render(prefix + label + suffix + strings.Repeat(separatorChar, remaining))
}

// statusLabel returns the formatted status string, optionally prefixed
// with the AI provider name.
func (m Model) statusLabel() string {
	name := ""
	if m.registry != nil {
		name = m.registry.PrimaryName()
	}
	if name != "" {
		return name + " · " + m.status
	}
	return m.status
}

// ---------------------------------------------------------------------------
// Component renderers
// ---------------------------------------------------------------------------

// renderResponse renders the response area. In collapsed mode only the
// last non-empty line is shown; in expanded mode a scrollable window
// of lines is displayed. When markdown rendering is enabled, AI
// responses are formatted via glamour.
func (m Model) renderResponse() string {
	text := m.lastResponse
	if text == "" {
		return ""
	}

	if m.renderMD {
		text = strings.Join(markdown.RenderStatic(text, m.effectiveWidth()-2), "\n")
	}

	if m.expanded {
		return m.renderExpandedResponse(text)
	}
	return m.renderCollapsedResponse(text)
}

// renderCollapsedResponse shows the last non-empty line of the response,
// word-wrapped to the available width.
func (m Model) renderCollapsedResponse(text string) string {
	lines := strings.Split(text, "\n")
	last := lastNonEmptyLine(lines)
	if last == "" {
		return ""
	}

	w := m.effectiveWidth()
	wrapWidth := w - 2 // account for "  " prefix
	if wrapWidth < 5 {
		wrapWidth = 5
	}

	wrapped := wrapText(last, wrapWidth)
	wLines := strings.Split(wrapped, "\n")
	// Show the last wrapped segment for continuity.
	return "  " + wLines[len(wLines)-1] + "\n"
}

// renderExpandedResponse shows a scrollable window of response lines.
// Each line is word-wrapped and the scroll window accounts for the
// resulting visual lines.
func (m Model) renderExpandedResponse(text string) string {
	w := m.effectiveWidth()
	wrapWidth := w - 2 // account for "  " prefix
	if wrapWidth < 5 {
		wrapWidth = 5
	}

	// Wrap each source line and flatten into visual lines.
	var visualLines []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			visualLines = append(visualLines, "")
		} else {
			wrapped := wrapText(line, wrapWidth)
			visualLines = append(visualLines, strings.Split(wrapped, "\n")...)
		}
	}

	visibleLines := expandedHeight - collapsedHeight
	if visibleLines <= 0 {
		visibleLines = 1
	}

	maxScroll := len(visualLines) - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	offset := m.scrollOffset
	if offset > maxScroll {
		offset = maxScroll
	}

	start := len(visualLines) - visibleLines - offset
	if start < 0 {
		start = 0
	}
	end := start + visibleLines
	if end > len(visualLines) {
		end = len(visualLines)
	}

	var b strings.Builder
	for _, line := range visualLines[start:end] {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// renderStreaming renders the animated spinner with any accumulated
// partial response text.
func (m Model) renderStreaming() string {
	accentColor := defaultAccentColor
	if m.theme != nil && m.theme.Colors.NormalCyan != "" {
		accentColor = m.theme.Colors.NormalCyan
	}

	// Animated spinner replaces the old static indicator.
	frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Render(frame + " ")

	partial := m.streamBuf.String()
	if partial == "" {
		partial = "Thinking..."
	}

	w := m.effectiveWidth()
	wrapWidth := w - 6
	if wrapWidth < 5 {
		wrapWidth = 5
	}

	if !m.expanded {
		// Collapsed: take last non-empty line, wrap, show tail.
		lines := strings.Split(partial, "\n")
		partial = lastNonEmptyLine(lines)
		if partial == "" {
			partial = "Thinking..."
		}
		wrapped := wrapText(partial, wrapWidth)
		wLines := strings.Split(wrapped, "\n")
		return "  " + prefix + wLines[len(wLines)-1] + "\n"
	}

	// Expanded: wrap and show all lines.
	wrapped := wrapText(partial, wrapWidth)
	lines := strings.Split(wrapped, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString("  " + prefix + line + "\n")
		} else {
			b.WriteString("    " + line + "\n")
		}
	}
	return b.String()
}

// renderConfirmation renders the confirmation prompt for destructive
// operations. Uses a warning-colored ⚠ prefix and the description
// from the pending confirmation.
func (m Model) renderConfirmation() string {
	if m.confirming == nil || !m.confirming.HasPending() {
		return ""
	}

	pc := m.confirming.Pending()

	warnColor := defaultWarningColor
	if m.theme != nil && m.theme.Colors.NormalYellow != "" {
		warnColor = m.theme.Colors.NormalYellow
	}

	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(warnColor)).
		Bold(true).
		Render(warningIndicator)

	return "  " + prefix + pc.Description + " [y/N]\n"
}

// renderError renders the error message with a styled prefix.
func (m Model) renderError() string {
	if m.err == nil {
		return ""
	}

	errColor := defaultErrorColor
	if m.theme != nil && m.theme.Colors.NormalRed != "" {
		errColor = m.theme.Colors.NormalRed
	}

	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(errColor)).
		Bold(true).
		Render(errorIndicator + "Error: ")

	return "  " + prefix + m.err.Error() + "\n"
}

// renderInput renders the input line with a styled prompt prefix
// followed by the text input widget.
func (m Model) renderInput() string {
	promptColor := defaultPromptColor
	if m.theme != nil && m.theme.Colors.NormalGreen != "" {
		promptColor = m.theme.Colors.NormalGreen
	}

	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(promptColor)).
		Bold(true).
		Render(promptPrefix)

	return prefix + m.input.View()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// effectiveWidth returns the usable rendering width, defaulting to 80
// when not yet set by the layout.
func (m Model) effectiveWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

// wrapText performs word-aware line wrapping of text to fit within width
// columns. Existing newlines in text are preserved.
func wrapText(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}

	var result strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(wrapLine(line, width))
	}
	return result.String()
}

// wrapLine wraps a single line of text at word boundaries to fit within
// width columns. Words longer than width are not broken (they overflow).
func wrapLine(line string, width int) string {
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}

	words := strings.Fields(line)
	if len(words) == 0 {
		return ""
	}

	var b strings.Builder
	lineLen := 0

	for i, word := range words {
		wLen := len([]rune(word))

		if i == 0 {
			b.WriteString(word)
			lineLen = wLen
			continue
		}

		if lineLen+1+wLen <= width {
			b.WriteByte(' ')
			b.WriteString(word)
			lineLen += 1 + wLen
		} else {
			b.WriteByte('\n')
			b.WriteString(word)
			lineLen = wLen
		}
	}

	return b.String()
}

// lastNonEmptyLine returns the last line that contains non-whitespace
// characters. Returns "" if every line is blank.
func lastNonEmptyLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
