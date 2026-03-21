package markdown_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/markdown"
	"github.com/stretchr/testify/assert"
)

// stripANSI removes ANSI escape sequences so assertions can match plain text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ---------------------------------------------------------------------------
// Style()
// ---------------------------------------------------------------------------

func TestStyleReturnsNonZeroConfig(t *testing.T) {
	s := markdown.Style()
	// DarkStyleConfig has a non-empty Document block style; if the config
	// were zero-valued this would be nil/empty.
	assert.NotNil(t, s.Document.Color, "Document color should be set in dark style")
}

func TestStyleClearsHeadingPrefixes(t *testing.T) {
	s := markdown.Style()

	// The function explicitly empties H2–H6 prefixes so rendered headings
	// don't show leading "## " marks.
	assert.Empty(t, s.H2.Prefix, "H2 prefix should be empty")
	assert.Empty(t, s.H3.Prefix, "H3 prefix should be empty")
	assert.Empty(t, s.H4.Prefix, "H4 prefix should be empty")
	assert.Empty(t, s.H5.Prefix, "H5 prefix should be empty")
	assert.Empty(t, s.H6.Prefix, "H6 prefix should be empty")
}

func TestStyleDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { markdown.Style() })
}

// ---------------------------------------------------------------------------
// RenderStatic() — basic rendering
// ---------------------------------------------------------------------------

func TestRenderStaticHeadingBoldCodeBlock(t *testing.T) {
	src := "## Hello\n\n**bold text**\n\n```\ncode\n```\n"
	lines := markdown.RenderStatic(src, 80)
	assert.NotEmpty(t, lines, "should return non-empty slice for valid markdown")
}

func TestRenderStaticEmptyInput(t *testing.T) {
	lines := markdown.RenderStatic("", 80)
	// Empty source should not panic; may return an empty or single-element slice.
	assert.NotNil(t, lines)
}

func TestRenderStaticPlainText(t *testing.T) {
	lines := markdown.RenderStatic("just some plain text", 80)
	assert.NotEmpty(t, lines, "plain text should still produce output")
	// Glamour wraps output in ANSI sequences; strip them for content check.
	joined := stripANSI(strings.Join(lines, "\n"))
	assert.Contains(t, joined, "just some plain text")
}

// ---------------------------------------------------------------------------
// RenderStatic() — width edge cases
// ---------------------------------------------------------------------------

func TestRenderStaticNarrowWidth(t *testing.T) {
	// Width=1 is pathologically narrow; should not panic.
	lines := markdown.RenderStatic("hello world", 1)
	assert.NotNil(t, lines)
	assert.Greater(t, len(lines), 0, "should return at least one line")
}

func TestRenderStaticWideWidth(t *testing.T) {
	lines := markdown.RenderStatic("hello", 200)
	assert.NotNil(t, lines)
	assert.NotEmpty(t, lines)
}

func TestRenderStaticZeroWidthDefaultsToEighty(t *testing.T) {
	// Width <= 0 is normalised to 80 internally; should not panic.
	lines := markdown.RenderStatic("hello", 0)
	assert.NotNil(t, lines)
	assert.NotEmpty(t, lines)
}

func TestRenderStaticNegativeWidth(t *testing.T) {
	lines := markdown.RenderStatic("hello", -10)
	assert.NotNil(t, lines)
	assert.NotEmpty(t, lines)
}

// ---------------------------------------------------------------------------
// RenderStatic() — various markdown constructs
// ---------------------------------------------------------------------------

func TestRenderStaticInlineCode(t *testing.T) {
	lines := markdown.RenderStatic("use `fmt.Println` here", 80)
	assert.NotEmpty(t, lines)
	joined := stripANSI(strings.Join(lines, "\n"))
	assert.Contains(t, joined, "fmt.Println", "inline code content should appear in output")
}

func TestRenderStaticFencedCodeBlock(t *testing.T) {
	src := "```go\nfmt.Println(\"hi\")\n```\n"
	lines := markdown.RenderStatic(src, 80)
	assert.NotEmpty(t, lines, "fenced code block should produce non-empty output")
}

func TestRenderStaticUnorderedList(t *testing.T) {
	src := "- alpha\n- beta\n- gamma\n"
	lines := markdown.RenderStatic(src, 80)
	assert.NotEmpty(t, lines)
	joined := stripANSI(strings.Join(lines, "\n"))
	assert.Contains(t, joined, "alpha")
	assert.Contains(t, joined, "beta")
}

func TestRenderStaticMultipleParagraphs(t *testing.T) {
	src := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n"
	lines := markdown.RenderStatic(src, 80)
	assert.Greater(t, len(lines), 1, "multiple paragraphs should produce multiple lines")
}

// ---------------------------------------------------------------------------
// RenderStatic() — output trimming
// ---------------------------------------------------------------------------

func TestRenderStaticOutputHasNoTrailingNewline(t *testing.T) {
	lines := markdown.RenderStatic("hello", 80)
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		assert.False(t, strings.HasSuffix(last, "\n"), "last line should not end with newline")
	}
}
