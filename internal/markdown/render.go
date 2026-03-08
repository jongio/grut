// Package markdown provides shared markdown-to-terminal rendering
// used by both the preview pane and the chat panel.
package markdown

import (
	"strings"

	"github.com/charmbracelet/glamour"
	gansi "github.com/charmbracelet/glamour/ansi"
	gstyles "github.com/charmbracelet/glamour/styles"
)

// Style returns a glamour DarkStyleConfig with heading prefixes
// removed so that "## Heading" renders as styled "Heading" without the
// leading hash marks.
func Style() gansi.StyleConfig {
	s := gstyles.DarkStyleConfig
	s.H2.Prefix = ""
	s.H3.Prefix = ""
	s.H4.Prefix = ""
	s.H5.Prefix = ""
	s.H6.Prefix = ""
	return s
}

// RenderStatic renders markdown content using glamour and returns the
// result as a slice of lines. This is a pure function safe for
// concurrent use in tea.Cmd goroutines.
func RenderStatic(source string, width int) []string {
	if width <= 0 {
		width = 80
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(Style()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.Split(source, "\n")
	}

	rendered, err := renderer.Render(source)
	if err != nil {
		return strings.Split(source, "\n")
	}

	rendered = strings.TrimRight(rendered, "\n")
	return strings.Split(rendered, "\n")
}
