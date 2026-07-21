package preview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/markdown"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownHeadings_Basic(t *testing.T) {
	source := strings.Join([]string{
		"# Title",
		"",
		"Intro paragraph.",
		"",
		"## Section One",
		"Body text.",
		"### Subsection",
		"###### Deep",
	}, "\n")

	got := parseMarkdownHeadings(source)
	require.Len(t, got, 4)

	assert.Equal(t, "Title", got[0].text)
	assert.Equal(t, 1, got[0].level)
	assert.Equal(t, 0, got[0].rawLine)

	assert.Equal(t, "Section One", got[1].text)
	assert.Equal(t, 2, got[1].level)
	assert.Equal(t, 4, got[1].rawLine)

	assert.Equal(t, "Subsection", got[2].text)
	assert.Equal(t, 3, got[2].level)
	assert.Equal(t, 6, got[2].rawLine)

	assert.Equal(t, "Deep", got[3].text)
	assert.Equal(t, 6, got[3].level)
	assert.Equal(t, 7, got[3].rawLine)
}

func TestParseMarkdownHeadings_SkipsFencedCode(t *testing.T) {
	source := strings.Join([]string{
		"# Real Heading",
		"```sh",
		"# not a heading, a shell comment",
		"## also not a heading",
		"```",
		"## After Fence",
		"~~~",
		"### hidden in tilde fence",
		"~~~",
		"#### Final",
	}, "\n")

	got := parseMarkdownHeadings(source)
	require.Len(t, got, 3)
	assert.Equal(t, "Real Heading", got[0].text)
	assert.Equal(t, "After Fence", got[1].text)
	assert.Equal(t, "Final", got[2].text)
}

func TestParseMarkdownHeadings_ClosingHashes(t *testing.T) {
	source := strings.Join([]string{
		"## Balanced ##",
		"### Trailing hashes #####",
		"# Kept#",
	}, "\n")

	got := parseMarkdownHeadings(source)
	require.Len(t, got, 3)
	assert.Equal(t, "Balanced", got[0].text)
	assert.Equal(t, "Trailing hashes", got[1].text)
	// A hash glued to a word is part of the text, not a closing sequence.
	assert.Equal(t, "Kept#", got[2].text)
}

func TestParseMarkdownHeadings_Rejects(t *testing.T) {
	source := strings.Join([]string{
		"#no-space-after-hash",
		"####### too many hashes",
		"Regular line with # in middle",
		"   ## Indented up to three spaces",
		"    # Four spaces is a code block indent",
		"## ",
	}, "\n")

	got := parseMarkdownHeadings(source)
	require.Len(t, got, 1)
	assert.Equal(t, "Indented up to three spaces", got[0].text)
	assert.Equal(t, 3, got[0].rawLine)
}

func TestMapHeadingDisplayLines_RawIdentity(t *testing.T) {
	source := strings.Join([]string{
		"# One",
		"text",
		"## Two",
		"more",
		"### Three",
	}, "\n")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.renderMarkdown = false
	p.lines = strings.Split(source, "\n")
	p.mdHeadings = parseMarkdownHeadings(source)

	targets := p.mapHeadingDisplayLines()
	require.Len(t, targets, 3)
	assert.Equal(t, 0, targets[0])
	assert.Equal(t, 2, targets[1])
	assert.Equal(t, 4, targets[2])
}

func TestMapHeadingDisplayLines_Glamour(t *testing.T) {
	source := strings.Join([]string{
		"# Overview",
		"",
		"Some introduction text that spans a little.",
		"",
		"## Installation",
		"",
		"Run the installer.",
		"",
		"## Usage",
		"",
		"Use it well.",
		"",
		"### Advanced Usage",
		"",
		"Details here.",
	}, "\n")

	width := 80
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.renderMarkdown = true
	p.lines = markdown.RenderStatic(source, width)
	p.mdHeadings = parseMarkdownHeadings(source)
	require.Len(t, p.mdHeadings, 4)

	targets := p.mapHeadingDisplayLines()
	require.Len(t, targets, 4)

	// Targets must be strictly increasing (document order preserved).
	for i := 1; i < len(targets); i++ {
		assert.Greater(t, targets[i], targets[i-1],
			"heading %d target should come after heading %d", i, i-1)
	}
	// Each resolved line must actually contain the heading text.
	for i, h := range p.mdHeadings {
		require.Less(t, targets[i], len(p.lines))
		plain := strings.TrimSpace(ansi.Strip(p.lines[targets[i]]))
		assert.Contains(t, plain, h.text,
			"rendered line for %q should contain the heading text", h.text)
	}
}

func TestOpenTOC_NonMarkdownReturnsNil(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "main.go"
	p.mdHeadings = nil

	cmd := p.openTOC()
	assert.Nil(t, cmd)
	assert.False(t, p.tocActive)
}

func TestOpenTOC_NoHeadingsReturnsNil(t *testing.T) {
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "README.md"
	p.lines = []string{"just body text", "no headings"}
	p.mdHeadings = nil

	cmd := p.openTOC()
	assert.Nil(t, cmd)
	assert.False(t, p.tocActive)
}

func TestOpenTOC_ActivatesAndRoutesInput(t *testing.T) {
	source := "# A\ntext\n## B\nmore\n"
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "README.md"
	p.renderMarkdown = false
	p.lines = strings.Split(source, "\n")
	p.mdHeadings = parseMarkdownHeadings(source)

	cmd := p.openTOC()
	require.NotNil(t, cmd)
	assert.True(t, p.tocActive)
	assert.Len(t, p.tocTargets, 2)

	msg := cmd()
	_, ok := msg.(panels.PreviewInputStartedMsg)
	assert.True(t, ok, "openTOC should emit PreviewInputStartedMsg")
}

func TestOpenTOC_GuardsForNonFileViews(t *testing.T) {
	source := "# A\n## B\n"
	base := func() *Preview {
		p := New(defaultCfg(), defaultEditorCfg(), nil)
		p.filePath = "README.md"
		p.renderMarkdown = false
		p.lines = strings.Split(source, "\n")
		p.mdHeadings = parseMarkdownHeadings(source)
		return p
	}

	cases := map[string]func(*Preview){
		"blame":  func(p *Preview) { p.blameMode = true },
		"diff":   func(p *Preview) { p.diffMode = true },
		"binary": func(p *Preview) { p.isBinary = true },
		"large":  func(p *Preview) { p.isLarge = true },
		"github": func(p *Preview) { p.ghMode = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := base()
			mutate(p)
			assert.Nil(t, p.openTOC())
			assert.False(t, p.tocActive)
		})
	}
}

func TestHandleTOCKey_NavigationAndJump(t *testing.T) {
	source := strings.Join([]string{
		"# A", "l1", "l2", "## B", "l4", "l5", "### C",
		"l7", "l8", "l9", "l10", "l11", "l12", "l13",
	}, "\n")
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "README.md"
	p.renderMarkdown = false
	p.lines = strings.Split(source, "\n")
	p.mdHeadings = parseMarkdownHeadings(source)
	// Viewport of 7 (height-1) with 14 content lines leaves room to scroll
	// to line 6 without clamping.
	p.SetSize(40, 8)

	require.NotNil(t, p.openTOC())
	require.True(t, p.tocActive)
	assert.Equal(t, 0, p.tocCursor)

	// Move down twice to the third heading (### C at raw line 6).
	p.handleTOCKey(keyMsg("j"))
	p.handleTOCKey(keyMsg(keyDown))
	assert.Equal(t, 2, p.tocCursor)
	require.Equal(t, 6, p.tocTargets[2])

	// Enter jumps and closes the overlay.
	_, cmd := p.handleTOCKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, p.tocActive)
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputEndedMsg)
	assert.True(t, ok)

	// Heading C is at display line 6 and the viewport can reach it.
	assert.Equal(t, 6, p.scrollY)
}

func TestHandleTOCKey_EscCancels(t *testing.T) {
	source := "# A\nl1\n## B\n"
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "README.md"
	p.renderMarkdown = false
	p.lines = strings.Split(source, "\n")
	p.mdHeadings = parseMarkdownHeadings(source)
	p.SetSize(40, 20)
	p.scrollY = 0

	require.NotNil(t, p.openTOC())
	p.handleTOCKey(keyMsg("j")) // move selection
	_, cmd := p.handleTOCKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, p.tocActive)
	assert.Equal(t, 0, p.scrollY, "esc must not move the viewport")
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputEndedMsg)
	assert.True(t, ok)
}

func TestRenderTOC_ShowsHeadingsAndMarker(t *testing.T) {
	source := "# Alpha\n## Bravo\n### Charlie\n"
	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.filePath = "README.md"
	p.renderMarkdown = false
	p.lines = strings.Split(source, "\n")
	p.mdHeadings = parseMarkdownHeadings(source)
	p.SetSize(40, 10)
	require.NotNil(t, p.openTOC())
	p.tocCursor = 1

	out := ansi.Strip(p.renderTOC(40, 10))
	assert.Contains(t, out, "Jump to heading")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "Bravo")
	assert.Contains(t, out, "Charlie")
	assert.Contains(t, out, "\u25b8") // selection marker on the active row
}

func TestPreview_TKeyOpensTOCForMarkdown(t *testing.T) {
	dir := t.TempDir()
	content := "# Heading One\n\nbody\n\n## Heading Two\n\nmore body\n"
	path := writeFile(t, dir, "doc.md", content)

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)
	require.NotEmpty(t, p.mdHeadings, "headings should load for markdown files")

	_, cmd := p.Update(keyMsg("t"))
	assert.True(t, p.tocActive)
	require.NotNil(t, cmd)
	_, ok := cmd().(panels.PreviewInputStartedMsg)
	assert.True(t, ok)
}

func TestPreview_TKeyIgnoredForNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	p.SetSize(60, 20)
	p.Focus()
	loadFile(t, p, path)

	p.Update(keyMsg("t"))
	assert.False(t, p.tocActive)
	assert.Empty(t, p.mdHeadings)
}

func TestTOCWindowStart(t *testing.T) {
	// Fits entirely: always start at 0.
	assert.Equal(t, 0, tocWindowStart(3, 4, 10))
	// Cursor near the top.
	assert.Equal(t, 0, tocWindowStart(0, 20, 6))
	// Cursor in the middle centers the window.
	assert.Equal(t, 7, tocWindowStart(10, 20, 6))
	// Cursor near the bottom clamps to the last full window.
	assert.Equal(t, 14, tocWindowStart(19, 20, 6))
}
