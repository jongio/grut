package preview

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongio/grut/internal/panels"
)

// loadFileAtLine mirrors loadFile but carries a target line, running the async
// load command so the fileLoadedMsg is fed back into the panel.
func loadFileAtLine(t *testing.T, p *Preview, path string, line int) {
	t.Helper()
	_, cmd := p.Update(panels.FileSelectedMsg{Path: path, Line: line})
	require.NotNil(t, cmd)
	msg := cmd()
	if msg != nil {
		p.Update(msg)
	}
}

func makeNumberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestFileSelected_JumpsToLine(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "big.txt", makeNumberedLines(100))

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	loadFileAtLine(t, p, path, 50)

	assert.Equal(t, 0, p.pendingGotoLine, "pending line should reset after load")
	assert.Positive(t, p.scrollY, "should scroll down toward the target line")

	// The target line (index 49, 0-based) must be inside the viewport.
	vh := p.viewportHeight()
	assert.LessOrEqual(t, p.scrollY, 49)
	assert.Less(t, 49, p.scrollY+vh)
}

func TestFileSelected_NoLineStaysAtTop(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "big.txt", makeNumberedLines(100))

	cfg := defaultCfg()
	cfg.SyntaxHighlighting = false
	p := New(cfg, defaultEditorCfg(), nil)
	p.SetSize(60, 20)

	loadFileAtLine(t, p, path, 0)

	assert.Equal(t, 0, p.scrollY, "no target line should leave the view at the top")
	assert.Equal(t, 0, p.pendingGotoLine)
}
