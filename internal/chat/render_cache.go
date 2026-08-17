package chat

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/markdown"
)

type messageIdentity struct {
	role    string
	content string
}

type messageRenderContext struct {
	themeName      string
	themeVariant   string
	greenColor     string
	cyanColor      string
	dimColor       string
	contentWidth   int
	renderMarkdown bool
}

type messageLineCache struct {
	context     messageRenderContext
	identities  []messageIdentity
	lines       map[messageIdentity][]string
	initialized bool
}

func (c *messageLineCache) reset() {
	*c = messageLineCache{}
}

func (c *messageLineCache) sync(messages []ai.ChatMessage, context messageRenderContext) {
	if !c.initialized || c.context != context {
		c.context = context
		c.identities = nil
		c.lines = make(map[messageIdentity][]string, len(messages))
		c.initialized = true
	}

	unchanged := len(c.identities) == len(messages)
	if unchanged {
		for i, msg := range messages {
			if c.identities[i] != (messageIdentity{role: msg.Role, content: msg.Content}) {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return
	}

	identities := make([]messageIdentity, len(messages))
	lines := make(map[messageIdentity][]string, len(messages))
	for i, msg := range messages {
		identity := messageIdentity{role: msg.Role, content: msg.Content}
		identities[i] = identity
		if cached, ok := c.lines[identity]; ok {
			lines[identity] = cached
		}
	}
	c.identities = identities
	c.lines = lines
}

type streamLineCache struct {
	currentLine    string
	lines          []string
	width          int
	sourceLen      int
	finalizedLines int
}

func (c *streamLineCache) reset() {
	*c = streamLineCache{}
}

func (c *streamLineCache) append(delta string) {
	if delta == "" || c.width <= 0 {
		return
	}
	c.sourceLen += len(delta)
	parts := strings.Split(delta, "\n")
	c.currentLine += parts[0]

	for _, part := range parts[1:] {
		c.rewrapCurrentLine()
		c.finalizedLines = len(c.lines)
		c.currentLine = part
	}
	c.rewrapCurrentLine()
}

func (c *streamLineCache) rewrapCurrentLine() {
	c.lines = c.lines[:c.finalizedLines]
	c.lines = append(c.lines, strings.Split(wrapLine(c.currentLine, c.width), "\n")...)
}

func (c *streamLineCache) wrappedLines(source string, width int) []string {
	if width <= 0 {
		return strings.Split(source, "\n")
	}
	if c.width != width || c.sourceLen > len(source) {
		c.reset()
		c.width = width
	}
	if c.sourceLen < len(source) {
		c.append(source[c.sourceLen:])
	}
	return c.lines
}

func renderMessageLines(msg ai.ChatMessage, context messageRenderContext) []string {
	var prefix, color string
	switch msg.Role {
	case RoleUser:
		prefix = "You: "
		color = context.greenColor
	case RoleAssistant:
		prefix = "AI: "
		color = context.cyanColor
	case RoleTool:
		prefix = "Tool: "
		color = context.dimColor
	default:
		return nil
	}

	styledPrefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Render(prefix)

	content := msg.Content
	if content == "" {
		content = "(empty)"
	}

	var lines []string
	if context.renderMarkdown && msg.Role == RoleAssistant {
		rendered := markdown.RenderStatic(content, context.contentWidth-2)
		for i, line := range rendered {
			if i == 0 {
				lines = append(lines, "  "+styledPrefix+line)
			} else {
				lines = append(lines, "  "+strings.Repeat(" ", len([]rune(prefix)))+line)
			}
		}
		return append(lines, "")
	}

	prefixWidth := len([]rune(prefix))
	wrapWidth := context.contentWidth - prefixWidth
	if wrapWidth < 5 {
		wrapWidth = 5
	}
	wrapped := wrapText(content, wrapWidth)
	pad := strings.Repeat(" ", prefixWidth)
	for i, line := range strings.Split(wrapped, "\n") {
		if i == 0 {
			lines = append(lines, "  "+styledPrefix+line)
		} else {
			lines = append(lines, "  "+pad+line)
		}
	}
	return append(lines, "")
}
