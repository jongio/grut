// Package markdown provides shared markdown-to-terminal rendering
// used by both the preview pane and the chat panel.
package markdown

import (
	"strings"
	"sync"

	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	gstyles "charm.land/glamour/v2/styles"
)

var (
	rendererMu    sync.Mutex
	rendererCache = make(map[int]*glamour.TermRenderer)
)

const maxCachedWidths = 5

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
// result as a slice of lines. This is safe for concurrent use — the
// underlying renderer is cached per width and protected by a mutex.
func RenderStatic(source string, width int) []string {
	if width <= 0 {
		width = 80
	}

	renderer, err := getRenderer(width)
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

// getRenderer returns a cached glamour.TermRenderer for the given width,
// creating one if none exists. If the cache exceeds maxCachedWidths, all
// entries are evicted (widths rarely vary in practice).
func getRenderer(width int) (*glamour.TermRenderer, error) {
	rendererMu.Lock()
	defer rendererMu.Unlock()

	if r, ok := rendererCache[width]; ok {
		return r, nil
	}

	// Evict all entries if cache is full (simple strategy — widths rarely vary).
	if len(rendererCache) >= maxCachedWidths {
		clear(rendererCache)
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(Style()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	rendererCache[width] = r
	return r, nil
}
